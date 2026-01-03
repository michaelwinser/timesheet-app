package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/google"
	"github.com/michaelw/timesheet-app/service/internal/store"
	gcal "google.golang.org/api/calendar/v3"
)

// CalendarHandler implements the calendar endpoints
type CalendarHandler struct {
	connections *store.CalendarConnectionStore
	calendars   *store.CalendarStore
	events      *store.CalendarEventStore
	entries     *store.TimeEntryStore
	google      *google.CalendarService
	stateMu     sync.RWMutex
	stateStore  map[string]uuid.UUID // In production, use Redis
}

// NewCalendarHandler creates a new calendar handler
func NewCalendarHandler(
	connections *store.CalendarConnectionStore,
	calendars *store.CalendarStore,
	events *store.CalendarEventStore,
	entries *store.TimeEntryStore,
	googleSvc *google.CalendarService,
) *CalendarHandler {
	return &CalendarHandler{
		connections: connections,
		calendars:   calendars,
		events:      events,
		entries:     entries,
		google:      googleSvc,
		stateStore:  make(map[string]uuid.UUID),
	}
}

// HandleOAuthCallback processes the OAuth callback and returns an error message if failed
func (h *CalendarHandler) HandleOAuthCallback(ctx context.Context, code, state string) error {
	// Get user ID from state parameter
	h.stateMu.Lock()
	userID, exists := h.stateStore[state]
	if exists {
		delete(h.stateStore, state)
	}
	h.stateMu.Unlock()

	if !exists {
		return errors.New("invalid or expired state parameter")
	}

	// Exchange code for tokens
	creds, err := h.google.ExchangeCode(ctx, code)
	if err != nil {
		return errors.New("failed to exchange authorization code")
	}

	// Create connection
	_, err = h.connections.Create(ctx, userID, "google", *creds)
	if err != nil {
		if errors.Is(err, store.ErrCalendarAlreadyConnected) {
			return errors.New("Google Calendar is already connected")
		}
		return err
	}

	return nil
}

// GoogleAuthorize returns the OAuth URL
func (h *CalendarHandler) GoogleAuthorize(ctx context.Context, req api.GoogleAuthorizeRequestObject) (api.GoogleAuthorizeResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.GoogleAuthorize401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	if h.google == nil {
		return api.GoogleAuthorize401JSONResponse{
			Code:    "not_configured",
			Message: "Google Calendar integration is not configured",
		}, nil
	}

	// Generate state token
	stateBytes := make([]byte, 16)
	rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)

	h.stateMu.Lock()
	h.stateStore[state] = userID
	h.stateMu.Unlock()

	url := h.google.GetAuthURL(state)

	return api.GoogleAuthorize200JSONResponse{
		Url:   url,
		State: state,
	}, nil
}

// GoogleCallback handles OAuth callback
func (h *CalendarHandler) GoogleCallback(ctx context.Context, req api.GoogleCallbackRequestObject) (api.GoogleCallbackResponseObject, error) {
	// Get user ID from state parameter (not JWT - this is a browser redirect from Google)
	h.stateMu.Lock()
	userID, exists := h.stateStore[req.Params.State]
	if exists {
		delete(h.stateStore, req.Params.State)
	}
	h.stateMu.Unlock()

	if !exists {
		return api.GoogleCallback400JSONResponse{
			Code:    "invalid_state",
			Message: "Invalid or expired state parameter",
		}, nil
	}

	// Exchange code for tokens
	creds, err := h.google.ExchangeCode(ctx, req.Params.Code)
	if err != nil {
		return api.GoogleCallback400JSONResponse{
			Code:    "oauth_error",
			Message: "Failed to exchange authorization code",
		}, nil
	}

	// Create connection
	conn, err := h.connections.Create(ctx, userID, "google", *creds)
	if err != nil {
		if errors.Is(err, store.ErrCalendarAlreadyConnected) {
			return api.GoogleCallback400JSONResponse{
				Code:    "already_connected",
				Message: "Google Calendar is already connected",
			}, nil
		}
		return nil, err
	}

	return api.GoogleCallback201JSONResponse(calendarConnectionToAPI(conn)), nil
}

// ListCalendarConnections returns all connections for the user
func (h *CalendarHandler) ListCalendarConnections(ctx context.Context, req api.ListCalendarConnectionsRequestObject) (api.ListCalendarConnectionsResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ListCalendarConnections401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	connections, err := h.connections.List(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]api.CalendarConnection, len(connections))
	for i, c := range connections {
		result[i] = calendarConnectionToAPI(c)
	}

	return api.ListCalendarConnections200JSONResponse(result), nil
}

// DeleteCalendarConnection disconnects a calendar
func (h *CalendarHandler) DeleteCalendarConnection(ctx context.Context, req api.DeleteCalendarConnectionRequestObject) (api.DeleteCalendarConnectionResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.DeleteCalendarConnection401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	err := h.connections.Delete(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrCalendarConnectionNotFound) {
			return api.DeleteCalendarConnection404JSONResponse{
				Code:    "not_found",
				Message: "Calendar connection not found",
			}, nil
		}
		return nil, err
	}

	return api.DeleteCalendarConnection204Response{}, nil
}

// SyncCalendar triggers a sync for a connection (syncs all selected calendars)
func (h *CalendarHandler) SyncCalendar(ctx context.Context, req api.SyncCalendarRequestObject) (api.SyncCalendarResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.SyncCalendar401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	// Get connection with credentials
	conn, err := h.connections.GetByID(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrCalendarConnectionNotFound) {
			return api.SyncCalendar404JSONResponse{
				Code:    "not_found",
				Message: "Calendar connection not found",
			}, nil
		}
		return nil, err
	}

	// Refresh token if needed
	creds := &conn.Credentials
	if time.Now().After(creds.Expiry.Add(-5 * time.Minute)) {
		newCreds, err := h.google.RefreshToken(ctx, creds)
		if err != nil {
			return nil, err
		}
		creds = newCreds
		h.connections.UpdateCredentials(ctx, conn.ID, *creds)
	}

	// Get selected calendars
	selectedCalendars, err := h.calendars.ListSelectedByConnection(ctx, conn.ID)
	if err != nil {
		return nil, err
	}

	// If no calendars selected, ensure we have at least primary
	if len(selectedCalendars) == 0 {
		// Fetch calendars from Google and select primary
		googleCals, err := h.google.ListCalendars(ctx, creds)
		if err != nil {
			return nil, err
		}
		for _, gc := range googleCals {
			cal := &store.Calendar{
				ConnectionID: conn.ID,
				UserID:       userID,
				ExternalID:   gc.ID,
				Name:         gc.Name,
				IsPrimary:    gc.IsPrimary,
				IsSelected:   gc.IsPrimary, // Auto-select primary
			}
			if gc.Color != "" {
				cal.Color = &gc.Color
			}
			h.calendars.Upsert(ctx, cal)
		}
		selectedCalendars, err = h.calendars.ListSelectedByConnection(ctx, conn.ID)
		if err != nil {
			return nil, err
		}
	}

	var totalCreated, totalUpdated, totalOrphaned int

	// Sync each selected calendar
	for _, cal := range selectedCalendars {
		created, updated, orphaned, err := h.syncSingleCalendar(ctx, creds, conn, cal, userID)
		if err != nil {
			log.Printf("Failed to sync calendar %s: %v", cal.Name, err)
			continue
		}
		totalCreated += created
		totalUpdated += updated
		totalOrphaned += orphaned
	}

	// Update connection last synced
	h.connections.UpdateLastSynced(ctx, conn.ID)

	return api.SyncCalendar200JSONResponse{
		EventsCreated:  totalCreated,
		EventsUpdated:  totalUpdated,
		EventsOrphaned: totalOrphaned,
	}, nil
}

// syncSingleCalendar syncs events from a single calendar
func (h *CalendarHandler) syncSingleCalendar(ctx context.Context, creds *store.OAuthCredentials, conn *store.CalendarConnection, cal *store.Calendar, userID uuid.UUID) (created, updated, orphaned int, err error) {
	var syncResult *google.SyncResult

	// Try incremental sync if we have a sync token
	if cal.SyncToken != nil && *cal.SyncToken != "" {
		syncResult, err = h.google.FetchEventsIncremental(ctx, creds, cal.ExternalID, *cal.SyncToken)
		if err != nil {
			// Sync token expired or invalid (410 Gone), clear it and do full sync
			log.Printf("Incremental sync failed for calendar %s, falling back to full sync: %v", cal.Name, err)
			h.calendars.ClearSyncToken(ctx, cal.ID)
			syncResult = nil
			err = nil
		}
	}

	// Full sync if no token or incremental failed
	if syncResult == nil {
		minTime := time.Now().AddDate(0, 0, -90)
		maxTime := time.Now().AddDate(0, 0, 30)

		syncResult, err = h.google.FetchEvents(ctx, creds, cal.ExternalID, minTime, maxTime)
		if err != nil {
			return 0, 0, 0, err
		}
	}

	// Process events
	externalIDs := make([]string, 0, len(syncResult.Events))

	for _, ge := range syncResult.Events {
		// Check if event was cancelled/deleted (only in incremental sync)
		if ge.Status == "cancelled" {
			// Mark as orphaned
			markErr := h.events.MarkOrphanedByExternalIDAndCalendar(ctx, cal.ID, ge.Id)
			if markErr != nil {
				log.Printf("Failed to mark event as orphaned: %v", markErr)
			}
			orphaned++
			continue
		}

		externalIDs = append(externalIDs, ge.Id)

		event := googleEventToStore(ge, conn.ID, cal.ID, userID)
		_, upsertErr := h.events.Upsert(ctx, event)
		if upsertErr != nil {
			return created, updated, orphaned, upsertErr
		}
		created++
	}

	// For full sync, mark events not in the result as orphaned
	if syncResult.FullSync && len(externalIDs) > 0 {
		orphanCount, markErr := h.events.MarkOrphanedExceptByCalendar(ctx, cal.ID, externalIDs)
		if markErr != nil {
			return created, updated, orphaned, markErr
		}
		orphaned += int(orphanCount)
	}

	// Save the new sync token
	if syncResult.NextSyncToken != "" {
		h.calendars.UpdateSyncToken(ctx, cal.ID, syncResult.NextSyncToken)
	}

	// Update calendar last synced
	h.calendars.UpdateLastSynced(ctx, cal.ID)

	return created, updated, orphaned, nil
}

// ListCalendarSources returns all available calendars for a connection
func (h *CalendarHandler) ListCalendarSources(ctx context.Context, req api.ListCalendarSourcesRequestObject) (api.ListCalendarSourcesResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ListCalendarSources401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	// Get connection with credentials
	conn, err := h.connections.GetByID(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrCalendarConnectionNotFound) {
			return api.ListCalendarSources404JSONResponse{
				Code:    "not_found",
				Message: "Calendar connection not found",
			}, nil
		}
		return nil, err
	}

	// Refresh token if needed
	creds := &conn.Credentials
	if time.Now().After(creds.Expiry.Add(-5 * time.Minute)) {
		newCreds, err := h.google.RefreshToken(ctx, creds)
		if err != nil {
			return nil, err
		}
		creds = newCreds
		h.connections.UpdateCredentials(ctx, conn.ID, *creds)
	}

	// Fetch available calendars from Google
	googleCals, err := h.google.ListCalendars(ctx, creds)
	if err != nil {
		return nil, err
	}

	// Sync calendar list to database
	for _, gc := range googleCals {
		cal := &store.Calendar{
			ConnectionID: conn.ID,
			UserID:       userID,
			ExternalID:   gc.ID,
			Name:         gc.Name,
			IsPrimary:    gc.IsPrimary,
			IsSelected:   gc.IsPrimary, // Auto-select primary on first sync
		}
		if gc.Color != "" {
			cal.Color = &gc.Color
		}
		h.calendars.Upsert(ctx, cal)
	}

	// Get calendars from database (includes selection state)
	calendars, err := h.calendars.ListByConnection(ctx, conn.ID)
	if err != nil {
		return nil, err
	}

	result := make([]api.Calendar, len(calendars))
	for i, c := range calendars {
		result[i] = calendarToAPI(c)
	}

	return api.ListCalendarSources200JSONResponse(result), nil
}

// UpdateCalendarSources updates which calendars are selected for sync
func (h *CalendarHandler) UpdateCalendarSources(ctx context.Context, req api.UpdateCalendarSourcesRequestObject) (api.UpdateCalendarSourcesResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.UpdateCalendarSources401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	// Verify connection exists and belongs to user
	conn, err := h.connections.GetByID(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrCalendarConnectionNotFound) {
			return api.UpdateCalendarSources404JSONResponse{
				Code:    "not_found",
				Message: "Calendar connection not found",
			}, nil
		}
		return nil, err
	}

	// Update selection
	err = h.calendars.UpdateSelection(ctx, conn.ID, req.Body.CalendarIds)
	if err != nil {
		return nil, err
	}

	// Return updated list
	calendars, err := h.calendars.ListByConnection(ctx, conn.ID)
	if err != nil {
		return nil, err
	}

	result := make([]api.Calendar, len(calendars))
	for i, c := range calendars {
		result[i] = calendarToAPI(c)
	}

	return api.UpdateCalendarSources200JSONResponse(result), nil
}

// ListCalendarEvents returns events with filters
func (h *CalendarHandler) ListCalendarEvents(ctx context.Context, req api.ListCalendarEventsRequestObject) (api.ListCalendarEventsResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ListCalendarEvents401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	var startDate, endDate *time.Time
	if req.Params.StartDate != nil {
		t := req.Params.StartDate.Time
		startDate = &t
	}
	if req.Params.EndDate != nil {
		t := req.Params.EndDate.Time
		endDate = &t
	}

	var status *store.ClassificationStatus
	if req.Params.ClassificationStatus != nil {
		s := store.ClassificationStatus(*req.Params.ClassificationStatus)
		status = &s
	}

	events, err := h.events.List(ctx, userID, startDate, endDate, status, req.Params.ConnectionId)
	if err != nil {
		return nil, err
	}

	result := make([]api.CalendarEvent, len(events))
	for i, e := range events {
		result[i] = calendarEventToAPI(e)
	}

	return api.ListCalendarEvents200JSONResponse(result), nil
}

// ClassifyCalendarEvent classifies an event (assigns to project or skips)
func (h *CalendarHandler) ClassifyCalendarEvent(ctx context.Context, req api.ClassifyCalendarEventRequestObject) (api.ClassifyCalendarEventResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ClassifyCalendarEvent401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	// Get the event first to calculate duration
	event, err := h.events.GetByID(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrCalendarEventNotFound) {
			return api.ClassifyCalendarEvent404JSONResponse{
				Code:    "not_found",
				Message: "Calendar event not found",
			}, nil
		}
		return nil, err
	}

	// Determine if this is a skip or a project assignment
	isSkip := req.Body.Skip != nil && *req.Body.Skip
	var projectID *uuid.UUID
	if !isSkip && req.Body.ProjectId != nil {
		projectID = req.Body.ProjectId
	}

	// Validate: must either skip or provide project_id
	if !isSkip && projectID == nil {
		return api.ClassifyCalendarEvent400JSONResponse{
			Code:    "invalid_request",
			Message: "Must provide project_id or set skip to true",
		}, nil
	}

	// Update the event's classification
	updatedEvent, err := h.events.Classify(ctx, userID, req.Id, projectID, isSkip)
	if err != nil {
		return nil, err
	}

	response := api.ClassifyCalendarEvent200JSONResponse{
		Event: calendarEventToAPI(updatedEvent),
	}

	// If classified to a project (not skipped), create/update a time entry
	if !isSkip && projectID != nil {
		// Calculate duration in hours
		duration := event.EndTime.Sub(event.StartTime).Hours()

		// Use event date (not time) for the time entry
		eventDate := time.Date(event.StartTime.Year(), event.StartTime.Month(), event.StartTime.Day(), 0, 0, 0, 0, time.UTC)

		// Create or update time entry
		entry, err := h.entries.CreateFromCalendar(ctx, userID, *projectID, eventDate, duration, &event.Title)
		if err != nil {
			return nil, err
		}

		apiEntry := timeEntryToAPI(entry)
		response.TimeEntry = &apiEntry
	}

	return response, nil
}

// calendarConnectionToAPI converts store model to API model
func calendarConnectionToAPI(c *store.CalendarConnection) api.CalendarConnection {
	conn := api.CalendarConnection{
		Id:        c.ID,
		UserId:    c.UserID,
		Provider:  api.CalendarConnectionProvider(c.Provider),
		CreatedAt: c.CreatedAt,
		UpdatedAt: &c.UpdatedAt,
	}
	if c.LastSyncedAt != nil {
		conn.LastSyncedAt = c.LastSyncedAt
	}
	return conn
}

// calendarEventToAPI converts store model to API model
func calendarEventToAPI(e *store.CalendarEvent) api.CalendarEvent {
	event := api.CalendarEvent{
		Id:                   e.ID,
		ConnectionId:         e.ConnectionID,
		UserId:               e.UserID,
		ExternalId:           e.ExternalID,
		Title:                e.Title,
		Description:          e.Description,
		StartTime:            e.StartTime,
		EndTime:              e.EndTime,
		Attendees:            &e.Attendees,
		IsRecurring:          &e.IsRecurring,
		ResponseStatus:       e.ResponseStatus,
		Transparency:         e.Transparency,
		IsOrphaned:           &e.IsOrphaned,
		IsSuppressed:         &e.IsSuppressed,
		ClassificationStatus: api.CalendarEventClassificationStatus(e.ClassificationStatus),
		ProjectId:            e.ProjectID,
		CreatedAt:            e.CreatedAt,
		UpdatedAt:            &e.UpdatedAt,
		CalendarName:         e.CalendarName,
		CalendarColor:        e.CalendarColor,
	}
	if e.ClassificationSource != nil {
		src := api.CalendarEventClassificationSource(*e.ClassificationSource)
		event.ClassificationSource = &src
	}
	if e.Project != nil {
		proj := projectToAPI(e.Project)
		event.Project = &proj
	}
	return event
}

// calendarToAPI converts store model to API model
func calendarToAPI(c *store.Calendar) api.Calendar {
	cal := api.Calendar{
		Id:           c.ID,
		ConnectionId: c.ConnectionID,
		ExternalId:   c.ExternalID,
		Name:         c.Name,
		IsPrimary:    c.IsPrimary,
		IsSelected:   c.IsSelected,
		CreatedAt:    c.CreatedAt,
	}
	if c.Color != nil {
		cal.Color = c.Color
	}
	if c.LastSyncedAt != nil {
		cal.LastSyncedAt = c.LastSyncedAt
	}
	cal.UpdatedAt = &c.UpdatedAt
	return cal
}

// googleEventToStore converts Google Calendar event to store model
func googleEventToStore(ge *gcal.Event, connID, calID uuid.UUID, userID uuid.UUID) *store.CalendarEvent {
	event := &store.CalendarEvent{
		ConnectionID:         connID,
		CalendarID:           &calID,
		UserID:               userID,
		ExternalID:           ge.Id,
		Title:                ge.Summary,
		ClassificationStatus: store.StatusPending,
	}

	if ge.Description != "" {
		event.Description = &ge.Description
	}

	// Parse times
	if ge.Start != nil {
		if ge.Start.DateTime != "" {
			event.StartTime, _ = time.Parse(time.RFC3339, ge.Start.DateTime)
		} else if ge.Start.Date != "" {
			event.StartTime, _ = time.Parse("2006-01-02", ge.Start.Date)
		}
	}

	if ge.End != nil {
		if ge.End.DateTime != "" {
			event.EndTime, _ = time.Parse(time.RFC3339, ge.End.DateTime)
		} else if ge.End.Date != "" {
			event.EndTime, _ = time.Parse("2006-01-02", ge.End.Date)
		}
	}

	// Attendees
	for _, a := range ge.Attendees {
		event.Attendees = append(event.Attendees, a.Email)
	}

	event.IsRecurring = ge.RecurringEventId != ""

	if ge.Transparency != "" {
		event.Transparency = &ge.Transparency
	}

	return event
}
