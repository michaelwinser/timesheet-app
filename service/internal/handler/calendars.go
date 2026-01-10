package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	gosync "sync"
	"time"

	"github.com/google/uuid"
	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/classification"
	"github.com/michaelw/timesheet-app/service/internal/google"
	"github.com/michaelw/timesheet-app/service/internal/store"
	"github.com/michaelw/timesheet-app/service/internal/sync"
	"github.com/michaelw/timesheet-app/service/internal/timeentry"
	openapi_types "github.com/oapi-codegen/runtime/types"
	gcal "google.golang.org/api/calendar/v3"
)

// CalendarHandler implements the calendar endpoints
type CalendarHandler struct {
	connections       *store.CalendarConnectionStore
	calendars         *store.CalendarStore
	events            *store.CalendarEventStore
	entries           *store.TimeEntryStore
	projects          *store.ProjectStore
	syncJobs          *store.SyncJobStore
	google            google.CalendarClient
	classificationSvc *classification.Service
	timeEntryService  *timeentry.Service
	stateMu           gosync.RWMutex
	stateStore        map[string]uuid.UUID // In production, use Redis
}

// NewCalendarHandler creates a new calendar handler
func NewCalendarHandler(
	connections *store.CalendarConnectionStore,
	calendars *store.CalendarStore,
	events *store.CalendarEventStore,
	entries *store.TimeEntryStore,
	projects *store.ProjectStore,
	syncJobs *store.SyncJobStore,
	googleSvc google.CalendarClient,
	classificationSvc *classification.Service,
	timeEntryService *timeentry.Service,
) *CalendarHandler {
	return &CalendarHandler{
		connections:       connections,
		calendars:         calendars,
		events:            events,
		entries:           entries,
		projects:          projects,
		syncJobs:          syncJobs,
		google:            googleSvc,
		classificationSvc: classificationSvc,
		timeEntryService:  timeEntryService,
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
// Uses smart sync decision based on water marks and staleness:
// - If requested week is within synced window AND data is fresh (<24h): skip sync
// - If data is stale: incremental sync using sync token
// - If week is outside synced window: full sync for missing weeks only
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
			// Mark calendars as needing reauth if token refresh fails
			log.Printf("Token refresh failed for connection %s: %v", conn.ID, err)
			h.markConnectionNeedsReauth(ctx, conn.ID)
			// Return 401 to indicate auth is needed
			return api.SyncCalendar401JSONResponse{
				Code:    "reauth_required",
				Message: "Google Calendar authorization has expired. Please reconnect your calendar.",
			}, nil
		}
		creds = newCreds
		h.connections.UpdateCredentials(ctx, conn.ID, *creds)
	}

	// Get selected calendars
	selectedCalendars, err := h.calendars.ListSelectedByConnection(ctx, conn.ID)
	if err != nil {
		return nil, err
	}

	// Check if all selected calendars need re-auth
	// This catches cases where the needs_reauth flag was set by previous sync failures
	allNeedReauth := len(selectedCalendars) > 0
	for _, cal := range selectedCalendars {
		if !cal.NeedsReauth {
			allNeedReauth = false
			break
		}
	}
	if allNeedReauth {
		return api.SyncCalendar401JSONResponse{
			Code:    "reauth_required",
			Message: "Google Calendar authorization has expired. Please reconnect your calendar.",
		}, nil
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

	// Determine target sync window from request params or defaults
	// If explicit dates provided, this is an "on-demand" sync (fetch only requested range as island)
	var targetStart, targetEnd time.Time
	isOnDemandSync := req.Params.StartDate != nil && req.Params.EndDate != nil

	if req.Params.StartDate != nil {
		targetStart = req.Params.StartDate.Time
	} else {
		targetStart = time.Now().AddDate(0, 0, -90)
	}
	if req.Params.EndDate != nil {
		targetEnd = req.Params.EndDate.Time
	} else {
		targetEnd = time.Now().AddDate(0, 0, 30)
	}

	// Normalize to week boundaries
	targetStart = sync.NormalizeToWeekStart(targetStart)
	targetEnd = sync.NormalizeToWeekEnd(targetEnd)

	var totalCreated, totalUpdated, totalOrphaned int
	var syncSkipped bool

	// Sync each selected calendar
	for _, cal := range selectedCalendars {
		var created, updated, orphaned int
		var syncErr error

		if isOnDemandSync {
			// On-demand sync: fetch only the requested range as an "island"
			// Don't fill gaps - background sync will catch up later
			log.Printf("[SYNC] on-demand: calendar=%s range=%s to %s",
				cal.Name, targetStart.Format("2006-01-02"), targetEnd.Format("2006-01-02"))
			created, updated, orphaned, syncErr = h.syncSingleCalendar(ctx, creds, conn, cal, userID, &targetStart, &targetEnd)
		} else {
			// Regular sync: use smart decision logic to determine what to fetch
			decision := sync.DecideSync(cal.MinSyncedDate, cal.MaxSyncedDate, cal.LastSyncedAt, targetStart, targetEnd)

			if !decision.NeedsSync {
				log.Printf("[SYNC] skip: calendar=%s reason=%s", cal.Name, decision.Reason)
				syncSkipped = true
				continue
			}

			log.Printf("[SYNC] start: calendar=%s reason=%s stale=%v missing_weeks=%d",
				cal.Name, decision.Reason, decision.IsStaleRefresh, len(decision.MissingWeeks))

			if decision.IsStaleRefresh {
				// Case A': Use incremental sync to refresh stale data
				created, updated, orphaned, syncErr = h.syncCalendarIncremental(ctx, creds, conn, cal, userID)
			} else if len(decision.MissingWeeks) > 0 {
				// Case B/C: Batch contiguous missing weeks into single API calls
				batches := batchContiguousWeeks(decision.MissingWeeks)
				log.Printf("[SYNC] batching: calendar=%s weeks=%d batches=%d", cal.Name, len(decision.MissingWeeks), len(batches))

				for _, batch := range batches {
					batchStart := batch[0]
					batchEnd := sync.NormalizeToWeekEnd(batch[len(batch)-1])
					log.Printf("[SYNC] batch_fetch: calendar=%s range=%s to %s weeks=%d",
						cal.Name, batchStart.Format("2006-01-02"), batchEnd.Format("2006-01-02"), len(batch))

					c, u, o, err := h.syncSingleCalendar(ctx, creds, conn, cal, userID, &batchStart, &batchEnd)
					if err != nil {
						log.Printf("[SYNC] batch_failed: calendar=%s range=%s to %s error=%v",
							cal.Name, batchStart.Format("2006-01-02"), batchEnd.Format("2006-01-02"), err)
						syncErr = err
						continue
					}
					created += c
					updated += u
					orphaned += o
				}
			} else {
				// First sync or full range sync
				created, updated, orphaned, syncErr = h.syncSingleCalendar(ctx, creds, conn, cal, userID, &targetStart, &targetEnd)
			}
		}

		if syncErr != nil {
			log.Printf("[SYNC] calendar_failed: calendar=%s error=%v", cal.Name, syncErr)
			h.calendars.IncrementSyncFailureCount(ctx, cal.ID)
			continue
		}

		// Reset failure count on success
		h.calendars.ResetSyncFailureCount(ctx, cal.ID)
		totalCreated += created
		totalUpdated += updated
		totalOrphaned += orphaned
	}

	// Update connection last synced (only if we actually synced something)
	if !syncSkipped || totalCreated > 0 || totalUpdated > 0 {
		h.connections.UpdateLastSynced(ctx, conn.ID)
	}

	// Auto-apply classification rules to newly synced events
	if h.classificationSvc != nil && (totalCreated > 0 || totalUpdated > 0) {
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
				log.Printf("[SYNC] auto_classified: events=%d", len(result.Classified))
			}
		}
	}

	log.Printf("[SYNC] complete: connection=%s created=%d updated=%d orphaned=%d skipped=%v",
		conn.ID, totalCreated, totalUpdated, totalOrphaned, syncSkipped)

	return api.SyncCalendar200JSONResponse{
		EventsCreated:  totalCreated,
		EventsUpdated:  totalUpdated,
		EventsOrphaned: totalOrphaned,
	}, nil
}

// markConnectionNeedsReauth marks all calendars in a connection as needing re-authentication
func (h *CalendarHandler) markConnectionNeedsReauth(ctx context.Context, connectionID uuid.UUID) {
	calendars, err := h.calendars.ListByConnection(ctx, connectionID)
	if err != nil {
		return
	}
	for _, cal := range calendars {
		h.calendars.MarkNeedsReauth(ctx, cal.ID)
	}
}

// syncCalendarIncremental performs incremental sync using sync token (for stale data refresh)
func (h *CalendarHandler) syncCalendarIncremental(ctx context.Context, creds *store.OAuthCredentials, conn *store.CalendarConnection, cal *store.Calendar, userID uuid.UUID) (created, updated, orphaned int, err error) {
	if cal.SyncToken == nil || *cal.SyncToken == "" {
		// No sync token, fall back to default window sync
		start, end := sync.DefaultInitialWindow()
		return h.syncSingleCalendar(ctx, creds, conn, cal, userID, &start, &end)
	}

	syncResult, err := h.google.FetchEventsIncremental(ctx, creds, cal.ExternalID, *cal.SyncToken)
	if err != nil {
		// Sync token expired (410 Gone), clear it and do full sync
		log.Printf("[SYNC] incremental_failed: calendar=%s fallback=full_sync error=%v", cal.Name, err)
		h.calendars.ClearSyncToken(ctx, cal.ID)
		start, end := sync.DefaultInitialWindow()
		return h.syncSingleCalendar(ctx, creds, conn, cal, userID, &start, &end)
	}

	// Process events
	for _, ge := range syncResult.Events {
		if ge.Status == "cancelled" {
			markErr := h.events.MarkOrphanedByExternalIDAndCalendar(ctx, cal.ID, ge.Id)
			if markErr != nil {
				log.Printf("Failed to mark event as orphaned: %v", markErr)
			}
			orphaned++
			continue
		}

		event := googleEventToStore(ge, conn.ID, cal.ID, userID)
		_, upsertErr := h.events.Upsert(ctx, event)
		if upsertErr != nil {
			return created, updated, orphaned, upsertErr
		}
		updated++
	}

	// Save the new sync token
	if syncResult.NextSyncToken != "" {
		h.calendars.UpdateSyncToken(ctx, cal.ID, syncResult.NextSyncToken)
	}

	// Update calendar last synced
	h.calendars.UpdateLastSynced(ctx, cal.ID)

	return created, updated, orphaned, nil
}

// syncCalendarWeek syncs a specific week for a calendar (for expanding water marks)
func (h *CalendarHandler) syncCalendarWeek(ctx context.Context, creds *store.OAuthCredentials, conn *store.CalendarConnection, cal *store.Calendar, userID uuid.UUID, weekStart, weekEnd time.Time) (created, updated, orphaned int, err error) {
	return h.syncSingleCalendar(ctx, creds, conn, cal, userID, &weekStart, &weekEnd)
}

// RunBackgroundSync implements sync.BackgroundSyncRunner for periodic background sync
func (h *CalendarHandler) RunBackgroundSync(ctx context.Context) error {
	if h.google == nil {
		log.Println("Background sync: Google Calendar not configured, skipping")
		return nil
	}

	// Find all calendars that need sync
	calendars, err := h.calendars.ListNeedingSync(ctx, sync.StalenessThreshold)
	if err != nil {
		return err
	}

	if len(calendars) == 0 {
		log.Println("Background sync: no calendars need sync")
		return nil
	}

	log.Printf("[SYNC] background: found %d calendars needing sync", len(calendars))

	// Process each calendar
	for _, cal := range calendars {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		h.syncCalendarBackground(ctx, cal)
	}

	return nil
}

// syncCalendarBackground syncs a single calendar during background sync
func (h *CalendarHandler) syncCalendarBackground(ctx context.Context, cal *store.Calendar) {
	log.Printf("[SYNC] background: syncing calendar=%s id=%s", cal.Name, cal.ID)

	// Get connection with credentials
	conn, err := h.connections.GetByIDForSync(ctx, cal.ConnectionID)
	if err != nil {
		log.Printf("[SYNC] background_failed: calendar=%s error=%v", cal.Name, err)
		h.calendars.IncrementSyncFailureCount(ctx, cal.ID)
		return
	}

	// Refresh token if needed
	creds := &conn.Credentials
	if time.Now().After(creds.Expiry.Add(-5 * time.Minute)) {
		newCreds, err := h.google.RefreshToken(ctx, creds)
		if err != nil {
			log.Printf("[SYNC] background_token_failed: calendar=%s error=%v", cal.Name, err)
			h.calendars.MarkNeedsReauth(ctx, cal.ID)
			return
		}
		creds = newCreds
		h.connections.UpdateCredentials(ctx, conn.ID, *creds)
	}

	// Use incremental sync if we have a sync token
	var created, updated, orphaned int
	var syncErr error

	if cal.SyncToken != nil && *cal.SyncToken != "" {
		created, updated, orphaned, syncErr = h.syncCalendarIncremental(ctx, creds, conn, cal, cal.UserID)
	} else {
		// No sync token, do initial window sync
		start, end := sync.DefaultInitialWindow()
		created, updated, orphaned, syncErr = h.syncSingleCalendar(ctx, creds, conn, cal, cal.UserID, &start, &end)
	}

	if syncErr != nil {
		log.Printf("[SYNC] background_sync_failed: calendar=%s error=%v", cal.Name, syncErr)
		h.calendars.IncrementSyncFailureCount(ctx, cal.ID)
		return
	}

	// Reset failure count on success
	h.calendars.ResetSyncFailureCount(ctx, cal.ID)
	log.Printf("[SYNC] background_complete: calendar=%s created=%d updated=%d orphaned=%d", cal.Name, created, updated, orphaned)
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
			log.Printf("[SYNC] incremental_failed: calendar=%s fallback=full_sync error=%v", cal.Name, err)
			h.calendars.ClearSyncToken(ctx, cal.ID)
			syncResult = nil
			err = nil
		}
	}

	// Full sync if no token or incremental failed
	if syncResult == nil {
		// Use provided dates or default initial window (-4 weeks to +1 week per PRD)
		if minTime != nil {
			syncMinTime = *minTime
		} else {
			syncMinTime, _ = sync.DefaultInitialWindow()
		}
		if maxTime != nil {
			syncMaxTime = *maxTime
		} else {
			_, syncMaxTime = sync.DefaultInitialWindow()
		}

		log.Printf("[SYNC] fetch: calendar=%s range=%s to %s", cal.Name,
			syncMinTime.Format("2006-01-02"), syncMaxTime.Format("2006-01-02"))

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

// batchContiguousWeeks groups contiguous weeks into batches for efficient fetching.
// This reduces the number of Google Calendar API calls when syncing multiple weeks.
// For example, weeks [Jan 6, Jan 13, Jan 20, Feb 10, Feb 17] becomes:
// [[Jan 6, Jan 13, Jan 20], [Feb 10, Feb 17]]
func batchContiguousWeeks(weeks []time.Time) [][]time.Time {
	if len(weeks) == 0 {
		return nil
	}

	var batches [][]time.Time
	currentBatch := []time.Time{weeks[0]}

	for i := 1; i < len(weeks); i++ {
		// Weeks are contiguous if they're exactly 7 days apart
		expected := currentBatch[len(currentBatch)-1].AddDate(0, 0, 7)
		if weeks[i].Equal(expected) {
			currentBatch = append(currentBatch, weeks[i])
		} else {
			// Non-contiguous, start a new batch
			batches = append(batches, currentBatch)
			currentBatch = []time.Time{weeks[i]}
		}
	}

	// Don't forget the last batch
	batches = append(batches, currentBatch)

	return batches
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

// ListCalendarEvents returns events with filters.
// This endpoint transparently handles on-demand sync when the requested date range
// is outside the current water marks. The client never needs to know about water marks.
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

	// If date range is provided and Google Calendar is configured, check if we need to sync
	if h.google != nil && startDate != nil && endDate != nil {
		if err := h.ensureEventsInRange(ctx, userID, *startDate, *endDate); err != nil {
			// Log error but continue - we'll return whatever we have cached
			log.Printf("[SYNC] ensureEventsInRange failed: %v", err)
		}
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

// ensureEventsInRange checks if the requested date range is within water marks.
// If not, it fetches events from Google synchronously and queues a background job
// to expand water marks. This implements the "server owns sync complexity" principle.
func (h *CalendarHandler) ensureEventsInRange(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) error {
	// Get all connections for the user (without credentials for initial list)
	connections, err := h.connections.List(ctx, userID)
	if err != nil {
		return err
	}

	// Normalize to week boundaries for consistent water mark handling
	targetStart := sync.NormalizeToWeekStart(startDate)
	targetEnd := sync.NormalizeToWeekEnd(endDate)

	for _, conn := range connections {
		// Get selected calendars for this connection
		calendars, err := h.calendars.ListSelectedByConnection(ctx, conn.ID)
		if err != nil {
			log.Printf("[SYNC] failed to list calendars for connection %s: %v", conn.ID, err)
			continue
		}

		// Check if any calendars need sync before fetching credentials
		var calendarsNeedingSync []*store.Calendar
		for _, cal := range calendars {
			// Skip calendars that need re-auth or have too many failures
			if cal.NeedsReauth || cal.SyncFailureCount >= 3 {
				continue
			}

			// Check if requested range is outside water marks
			decision := sync.DecideSync(cal.MinSyncedDate, cal.MaxSyncedDate, cal.LastSyncedAt, targetStart, targetEnd)
			if decision.NeedsSync {
				calendarsNeedingSync = append(calendarsNeedingSync, cal)
			}
		}

		// Skip this connection if no calendars need sync
		if len(calendarsNeedingSync) == 0 {
			continue
		}

		// Fetch full connection with credentials (only if we need to sync)
		fullConn, err := h.connections.GetByID(ctx, userID, conn.ID)
		if err != nil {
			log.Printf("[SYNC] failed to get credentials for connection %s: %v", conn.ID, err)
			continue
		}

		for _, cal := range calendarsNeedingSync {
			decision := sync.DecideSync(cal.MinSyncedDate, cal.MaxSyncedDate, cal.LastSyncedAt, targetStart, targetEnd)

			log.Printf("[SYNC] on-demand fetch needed: calendar=%s reason=%s range=%s to %s",
				cal.Name, decision.Reason, targetStart.Format("2006-01-02"), targetEnd.Format("2006-01-02"))

			// Refresh token if needed
			creds := &fullConn.Credentials
			if time.Now().After(creds.Expiry.Add(-5 * time.Minute)) {
				newCreds, err := h.google.RefreshToken(ctx, creds)
				if err != nil {
					log.Printf("[SYNC] token refresh failed for calendar %s: %v", cal.Name, err)
					h.calendars.MarkNeedsReauth(ctx, cal.ID)
					continue
				}
				creds = newCreds
				h.connections.UpdateCredentials(ctx, fullConn.ID, *creds)
			}

			// Fetch events synchronously for the requested range
			_, _, _, err = h.syncSingleCalendar(ctx, creds, fullConn, cal, userID, &targetStart, &targetEnd)
			if err != nil {
				log.Printf("[SYNC] on-demand fetch failed for calendar %s: %v", cal.Name, err)
				h.calendars.IncrementSyncFailureCount(ctx, cal.ID)
				continue
			}

			// Reset failure count on success
			h.calendars.ResetSyncFailureCount(ctx, cal.ID)

			// Queue a background job to fill any gaps (if there are more missing weeks beyond what we just fetched)
			// The job covers the full range of missing weeks to fill gaps between the "island" and existing water marks
			if h.syncJobs != nil && len(decision.MissingWeeks) > 1 {
				// MissingWeeks contains week start dates (Mondays)
				// Job range: first missing week to end of last missing week
				jobMinDate := decision.MissingWeeks[0]
				jobMaxDate := decision.MissingWeeks[len(decision.MissingWeeks)-1].AddDate(0, 0, 6) // End of last week (Sunday)

				log.Printf("[SYNC] queuing background job to fill gap: calendar=%s range=%s to %s (%d weeks)",
					cal.Name, jobMinDate.Format("2006-01-02"), jobMaxDate.Format("2006-01-02"), len(decision.MissingWeeks))

				job := &store.SyncJob{
					CalendarID:    cal.ID,
					JobType:       store.SyncJobTypeExpandWatermarks,
					TargetMinDate: jobMinDate,
					TargetMaxDate: jobMaxDate,
					Priority:      10, // High priority for user-initiated
				}
				if _, err := h.syncJobs.Create(ctx, job); err != nil {
					log.Printf("[SYNC] failed to queue background job for calendar %s: %v", cal.Name, err)
				}
			}
		}
	}

	// Auto-apply classification rules to newly synced events in the requested range
	// This ensures events fetched by on-demand sync get classified like regular sync
	if h.classificationSvc != nil {
		projects, err := h.projects.List(ctx, userID, false)
		if err != nil {
			log.Printf("[SYNC] on-demand: failed to fetch projects for classification: %v", err)
		} else if len(projects) > 0 {
			targets := projectsToTargetsWithNames(projects)
			result, err := h.classificationSvc.ApplyRules(ctx, userID, targets, &startDate, &endDate, false)
			if err != nil {
				log.Printf("[SYNC] on-demand: failed to apply classification rules: %v", err)
			} else if len(result.Classified) > 0 {
				log.Printf("[SYNC] on-demand: auto_classified=%d events", len(result.Classified))
			}
		}
	}

	return nil
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

	// With ephemeral time entries, we don't reactively create/update entries.
	// Instead, compute the current entry value for this project/date.
	if !isSkip && projectID != nil {
		eventDate := time.Date(event.StartTime.Year(), event.StartTime.Month(), event.StartTime.Day(), 0, 0, 0, 0, time.UTC)
		// First check if there's a materialized entry
		entry, err := h.entries.GetByProjectAndDate(ctx, userID, *projectID, eventDate)
		if err == nil {
			// Materialized entry exists - compute fresh values and update computed fields
			computed, _ := h.timeEntryService.ComputeForProjectAndDate(ctx, userID, *projectID, eventDate)
			if computed != nil {
				entry.ComputedHours = &computed.Hours
				entry.ComputedTitle = &computed.Title
				entry.ComputedDescription = &computed.Description
			}
			apiEntry := timeEntryToAPI(entry)
			response.TimeEntry = &apiEntry
		} else {
			// No materialized entry - compute ephemeral entry
			computed, err := h.timeEntryService.ComputeForProjectAndDate(ctx, userID, *projectID, eventDate)
			if err == nil && computed != nil {
				hours32 := float32(computed.Hours)
				apiEntry := api.TimeEntry{
					ProjectId:           *projectID,
					Date:                openapi_types.Date{Time: eventDate},
					Hours:               hours32,
					Title:               &computed.Title,
					Description:         &computed.Description,
					Source:              api.TimeEntrySourceCalendar,
					ComputedHours:       &hours32,
					ComputedTitle:       &computed.Title,
					ComputedDescription: &computed.Description,
				}
				response.TimeEntry = &apiEntry
			}
		}
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

	// With ephemeral time entries, we don't reactively create/update entries.
	// Time entries are computed on-demand when ListTimeEntries is called.
	// The affectedDates tracking is no longer needed for time entry creation.

	return api.BulkClassifyEvents200JSONResponse{
		ClassifiedCount: classifiedCount,
		SkippedCount:    skippedCount,
		// TimeEntriesCreated is no longer relevant with ephemeral model
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
