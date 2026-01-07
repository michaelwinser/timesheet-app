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
	"github.com/michaelw/timesheet-app/service/internal/classification"
	"github.com/michaelw/timesheet-app/service/internal/google"
	"github.com/michaelw/timesheet-app/service/internal/store"
	gcal "google.golang.org/api/calendar/v3"
)

// CalendarHandler implements the calendar endpoints
type CalendarHandler struct {
	connections       *store.CalendarConnectionStore
	calendars         *store.CalendarStore
	events            *store.CalendarEventStore
	entries           *store.TimeEntryStore
	projects          *store.ProjectStore
	google            *google.CalendarService
	classificationSvc *classification.Service
	stateMu           sync.RWMutex
	stateStore        map[string]uuid.UUID // In production, use Redis
}

// NewCalendarHandler creates a new calendar handler
func NewCalendarHandler(
	connections *store.CalendarConnectionStore,
	calendars *store.CalendarStore,
	events *store.CalendarEventStore,
	entries *store.TimeEntryStore,
	projects *store.ProjectStore,
	googleSvc *google.CalendarService,
	classificationSvc *classification.Service,
) *CalendarHandler {
	return &CalendarHandler{
		connections:       connections,
		calendars:         calendars,
		events:            events,
		entries:           entries,
		projects:          projects,
		google:            googleSvc,
		classificationSvc: classificationSvc,
		stateStore:        make(map[string]uuid.UUID),
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

	// Extract optional date range parameters for on-demand sync
	var minTime, maxTime *time.Time
	if req.Params.StartDate != nil {
		t := req.Params.StartDate.Time
		minTime = &t
	}
	if req.Params.EndDate != nil {
		// Add a day to end_date to include the full day
		t := req.Params.EndDate.Time.AddDate(0, 0, 1)
		maxTime = &t
	}

	var totalCreated, totalUpdated, totalOrphaned int

	// Sync each selected calendar
	for _, cal := range selectedCalendars {
		created, updated, orphaned, err := h.syncSingleCalendar(ctx, creds, conn, cal, userID, minTime, maxTime)
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

	// Auto-apply classification rules to newly synced events
	if h.classificationSvc != nil {
		// Fetch projects and convert to targets for classification
		projects, err := h.projects.List(ctx, userID, true) // Include archived
		if err != nil {
			log.Printf("Failed to fetch projects for classification: %v", err)
		} else {
			targets := projectsToTargets(projects)
			result, err := h.classificationSvc.ApplyRules(ctx, userID, targets, nil, nil, false)
			if err != nil {
				log.Printf("Failed to apply classification rules after sync: %v", err)
				// Don't fail the sync, just log the error
			} else if len(result.Classified) > 0 {
				log.Printf("Auto-classified %d events after sync", len(result.Classified))
			}
		}
	}

	return api.SyncCalendar200JSONResponse{
		EventsCreated:  totalCreated,
		EventsUpdated:  totalUpdated,
		EventsOrphaned: totalOrphaned,
	}, nil
}

// syncSingleCalendar syncs events from a single calendar
// If minTime/maxTime are nil, uses default range (-366 to +32 days) and incremental sync when available
// The calendar's synced window is expanded to track which date ranges have been synced
func (h *CalendarHandler) syncSingleCalendar(ctx context.Context, creds *store.OAuthCredentials, conn *store.CalendarConnection, cal *store.Calendar, userID uuid.UUID, minTime, maxTime *time.Time) (created, updated, orphaned int, err error) {
	var syncResult *google.SyncResult
	var syncMinTime, syncMaxTime time.Time

	// Determine if this is a default range sync or on-demand
	isDefaultRangeSync := minTime == nil && maxTime == nil

	// Try incremental sync if we have a sync token
	// Incremental sync works regardless of date range - it returns all changes since last sync
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
		// Use provided dates or defaults (-366 to +32 days)
		syncMinTime = time.Now().AddDate(0, 0, -366)
		syncMaxTime = time.Now().AddDate(0, 0, 32)
		if minTime != nil {
			syncMinTime = *minTime
		}
		if maxTime != nil {
			syncMaxTime = *maxTime
		}

		syncResult, err = h.google.FetchEvents(ctx, creds, cal.ExternalID, syncMinTime, syncMaxTime)
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

	// For full sync, mark events within the synced range as orphaned if not in the result
	// This uses the tracked sync window for accurate orphaning
	if syncResult.FullSync && len(externalIDs) > 0 {
		// Determine the orphan range based on the calendar's tracked sync window
		orphanMinTime := syncMinTime
		orphanMaxTime := syncMaxTime

		// If we have a tracked sync window, use it to bound orphaning
		// This ensures we only orphan events within dates we've actually synced
		if cal.MinSyncedDate != nil && cal.MinSyncedDate.Before(orphanMinTime) {
			orphanMinTime = *cal.MinSyncedDate
		}
		if cal.MaxSyncedDate != nil && cal.MaxSyncedDate.After(orphanMaxTime) {
			orphanMaxTime = *cal.MaxSyncedDate
		}

		// For default range sync, orphan within the full tracked window
		// For on-demand sync, only orphan within the requested range
		if isDefaultRangeSync {
			orphanCount, markErr := h.events.MarkOrphanedInRangeExceptByCalendar(ctx, cal.ID, externalIDs, orphanMinTime, orphanMaxTime)
			if markErr != nil {
				return created, updated, orphaned, markErr
			}
			orphaned += int(orphanCount)
		} else {
			// On-demand sync: only orphan within the specific requested range
			orphanCount, markErr := h.events.MarkOrphanedInRangeExceptByCalendar(ctx, cal.ID, externalIDs, syncMinTime, syncMaxTime)
			if markErr != nil {
				return created, updated, orphaned, markErr
			}
			orphaned += int(orphanCount)
		}
	}

	// Expand the tracked sync window
	if syncResult.FullSync {
		h.calendars.ExpandSyncedWindow(ctx, cal.ID, syncMinTime, syncMaxTime)
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

	// Recalculate time entries for this event's date
	// This uses the analyzer to properly compute hours with overlap handling and rounding
	if err := h.classificationSvc.RecalculateTimeEntriesForEvent(ctx, userID, updatedEvent); err != nil {
		// Log but don't fail - classification succeeded
		// In production, we might want to log this error
	}

	// If classified to a project (not skipped), fetch the updated time entry
	if !isSkip && projectID != nil {
		eventDate := time.Date(event.StartTime.Year(), event.StartTime.Month(), event.StartTime.Day(), 0, 0, 0, 0, time.UTC)
		entry, err := h.entries.GetByProjectAndDate(ctx, userID, *projectID, eventDate)
		if err == nil {
			apiEntry := timeEntryToAPI(entry)
			response.TimeEntry = &apiEntry
		}
		// If entry not found (unlikely after recalculation), just omit from response
	}

	return response, nil
}

// BulkClassifyEvents classifies multiple events matching a query
func (h *CalendarHandler) BulkClassifyEvents(ctx context.Context, req api.BulkClassifyEventsRequestObject) (api.BulkClassifyEventsResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.BulkClassifyEvents401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	// Validate request
	if req.Body.Query == "" {
		return api.BulkClassifyEvents400JSONResponse{
			Code:    "invalid_request",
			Message: "Query is required",
		}, nil
	}

	isSkip := req.Body.Skip != nil && *req.Body.Skip
	if !isSkip && req.Body.ProjectId == nil {
		return api.BulkClassifyEvents400JSONResponse{
			Code:    "invalid_request",
			Message: "Must provide project_id or set skip to true",
		}, nil
	}

	// Use classification service's preview to find matching events
	preview, err := h.classificationSvc.PreviewRule(ctx, userID, req.Body.Query, req.Body.ProjectId, nil, nil)
	if err != nil {
		return api.BulkClassifyEvents400JSONResponse{
			Code:    "invalid_query",
			Message: err.Error(),
		}, nil
	}

	var classifiedCount, skippedCount int
	affectedDates := make(map[time.Time]bool)

	// Process each matching event
	for _, match := range preview.Matches {
		// Get the full event for duration calculation
		event, err := h.events.GetByID(ctx, userID, match.EventID)
		if err != nil {
			continue // Skip events we can't fetch
		}

		// Skip manually classified events - we don't override those
		if event.ClassificationSource != nil && *event.ClassificationSource == store.SourceManual {
			continue
		}

		// Classify the event
		_, err = h.events.Classify(ctx, userID, match.EventID, req.Body.ProjectId, isSkip)
		if err != nil {
			continue
		}

		// Track affected date for recalculation
		eventDate := time.Date(event.StartTime.Year(), event.StartTime.Month(), event.StartTime.Day(), 0, 0, 0, 0, time.UTC)
		affectedDates[eventDate] = true

		if isSkip {
			skippedCount++
		} else {
			classifiedCount++
		}
	}

	// Recalculate time entries for all affected dates
	// This uses the analyzer to properly compute hours with overlap handling and rounding
	timeEntriesUpdated := 0
	for date := range affectedDates {
		if err := h.classificationSvc.RecalculateTimeEntries(ctx, userID, date); err == nil {
			timeEntriesUpdated++
		}
	}

	return api.BulkClassifyEvents200JSONResponse{
		ClassifiedCount:    classifiedCount,
		SkippedCount:       skippedCount,
		TimeEntriesCreated: &timeEntriesUpdated,
	}, nil
}

// ExplainEventClassification explains how an event was or would be classified
func (h *CalendarHandler) ExplainEventClassification(ctx context.Context, req api.ExplainEventClassificationRequestObject) (api.ExplainEventClassificationResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ExplainEventClassification401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	// Get the event
	event, err := h.events.GetByID(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrCalendarEventNotFound) {
			return api.ExplainEventClassification404JSONResponse{
				Code:    "not_found",
				Message: "Calendar event not found",
			}, nil
		}
		return nil, err
	}

	// Get projects to build targets (including name for display)
	projects, err := h.projects.List(ctx, userID, false)
	if err != nil {
		return nil, err
	}

	targets := projectsToTargetsWithNames(projects)

	// Get explain result from classification service
	result, err := h.classificationSvc.ExplainEventClassification(ctx, userID, req.Id, targets)
	if err != nil {
		return nil, err
	}

	// Build project name map for lookups
	projectNames := make(map[string]string)
	for _, p := range projects {
		projectNames[p.ID.String()] = p.Name
	}

	// Convert result to API response
	response := api.ClassificationExplanation{
		Event:   calendarEventToAPI(event),
		Outcome: result.Outcome,
	}

	// Set optional fields
	if result.WouldBeSkipped {
		response.WouldBeSkipped = &result.WouldBeSkipped
		skipConf := float32(result.SkipConfidence)
		response.SkipConfidence = &skipConf
	}

	if result.WinnerTargetID != "" {
		winnerID, err := uuid.Parse(result.WinnerTargetID)
		if err == nil {
			response.WinnerProjectId = &winnerID
		}
	}

	winnerConf := float32(result.WinnerConfidence)
	response.WinnerConfidence = &winnerConf
	totalWeight := float32(result.TotalWeight)
	response.TotalWeight = &totalWeight

	// Convert target scores
	response.TargetScores = make([]api.TargetScore, 0, len(result.TargetScores))
	for _, ts := range result.TargetScores {
		targetID, err := uuid.Parse(ts.TargetID)
		if err != nil {
			continue
		}
		score := api.TargetScore{
			TargetId:    targetID,
			TotalWeight: float32(ts.TotalWeight),
		}
		if ts.TargetName != "" {
			score.TargetName = &ts.TargetName
		} else if name, ok := projectNames[ts.TargetID]; ok {
			score.TargetName = &name
		}
		ruleWeight := float32(ts.RuleWeight)
		score.RuleWeight = &ruleWeight
		fpWeight := float32(ts.FingerprintWeight)
		score.FingerprintWeight = &fpWeight
		score.IsWinner = &ts.IsWinner
		response.TargetScores = append(response.TargetScores, score)
	}

	// Convert rule evaluations
	response.RuleEvaluations = make([]api.RuleEvaluation, 0, len(result.Evaluations))
	for _, eval := range result.Evaluations {
		re := api.RuleEvaluation{
			Query:   eval.Query,
			Matched: eval.Matched,
		}
		re.RuleId = &eval.RuleID
		re.TargetId = &eval.TargetID
		if eval.TargetName != "" {
			re.TargetName = &eval.TargetName
		} else if name, ok := projectNames[eval.TargetID]; ok {
			re.TargetName = &name
		}
		weight := float32(eval.Weight)
		re.Weight = &weight
		source := api.RuleEvaluationSource(eval.Source)
		re.Source = &source
		response.RuleEvaluations = append(response.RuleEvaluations, re)
	}

	// Convert skip evaluations
	if len(result.SkipEvaluations) > 0 {
		skipEvals := make([]api.RuleEvaluation, 0, len(result.SkipEvaluations))
		for _, eval := range result.SkipEvaluations {
			re := api.RuleEvaluation{
				Query:   eval.Query,
				Matched: eval.Matched,
			}
			re.RuleId = &eval.RuleID
			re.TargetId = &eval.TargetID
			weight := float32(eval.Weight)
			re.Weight = &weight
			source := api.RuleEvaluationSource(eval.Source)
			re.Source = &source
			skipEvals = append(skipEvals, re)
		}
		response.SkipEvaluations = &skipEvals
	}

	return api.ExplainEventClassification200JSONResponse(response), nil
}

// projectsToTargetsWithNames creates classification targets with project names included
func projectsToTargetsWithNames(projects []*store.Project) []classification.Target {
	targets := make([]classification.Target, len(projects))
	for i, p := range projects {
		attrs := make(map[string]any)
		attrs["name"] = p.Name
		if len(p.FingerprintDomains) > 0 {
			attrs["domains"] = p.FingerprintDomains
		}
		if len(p.FingerprintEmails) > 0 {
			attrs["emails"] = p.FingerprintEmails
		}
		if len(p.FingerprintKeywords) > 0 {
			attrs["keywords"] = p.FingerprintKeywords
		}
		targets[i] = classification.Target{
			ID:         p.ID.String(),
			Attributes: attrs,
		}
	}
	return targets
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
		IsSkipped:            &e.IsSkipped,
		NeedsReview:          &e.NeedsReview,
		ProjectId:            e.ProjectID,
		CreatedAt:            e.CreatedAt,
		UpdatedAt:            &e.UpdatedAt,
		CalendarId:           e.CalendarExternalID,
		CalendarName:         e.CalendarName,
		CalendarColor:        e.CalendarColor,
	}
	if e.ClassificationSource != nil {
		src := api.CalendarEventClassificationSource(*e.ClassificationSource)
		event.ClassificationSource = &src
	}
	if e.ClassificationConfidence != nil {
		conf := float32(*e.ClassificationConfidence)
		event.ClassificationConfidence = &conf
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

	// Attendees - extract emails and find user's response status
	for _, a := range ge.Attendees {
		event.Attendees = append(event.Attendees, a.Email)
		// If this is the current user (Self=true), capture their response status
		if a.Self && a.ResponseStatus != "" {
			event.ResponseStatus = &a.ResponseStatus
		}
	}

	event.IsRecurring = ge.RecurringEventId != ""

	if ge.Transparency != "" {
		event.Transparency = &ge.Transparency
	}

	return event
}
