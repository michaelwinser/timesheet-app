package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/michaelw/timesheet-app/service/internal/store"
	"github.com/michaelw/timesheet-app/service/internal/sync"
)

// DebugHandler provides debug endpoints for sync diagnostics
type DebugHandler struct {
	calendars   *store.CalendarStore
	connections *store.CalendarConnectionStore
	jwt         *JWTService
}

// NewDebugHandler creates a new debug handler
func NewDebugHandler(
	calendars *store.CalendarStore,
	connections *store.CalendarConnectionStore,
	jwt *JWTService,
) *DebugHandler {
	return &DebugHandler{
		calendars:   calendars,
		connections: connections,
		jwt:         jwt,
	}
}

// CalendarSyncStatus represents sync status for a single calendar
type CalendarSyncStatus struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	ExternalID     string     `json:"external_id"`
	ConnectionID   string     `json:"connection_id"`
	IsSelected     bool       `json:"is_selected"`
	IsPrimary      bool       `json:"is_primary"`
	MinSyncedDate  *time.Time `json:"min_synced_date"`
	MaxSyncedDate  *time.Time `json:"max_synced_date"`
	LastSyncedAt   *time.Time `json:"last_synced_at"`
	SyncToken      *string    `json:"sync_token"`
	SyncTokenSet   bool       `json:"sync_token_set"`
	NeedsReauth    bool       `json:"needs_reauth"`
	SyncFailures   int        `json:"sync_failure_count"`
	IsStale        bool       `json:"is_stale"`
	SyncedWeeks    int        `json:"synced_weeks"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ConnectionSyncStatus represents sync status for a connection
type ConnectionSyncStatus struct {
	ID           string     `json:"id"`
	Provider     string     `json:"provider"`
	LastSyncedAt *time.Time `json:"last_synced_at"`
	IsStale      bool       `json:"is_stale"`
	CreatedAt    time.Time  `json:"created_at"`
}

// SyncStatusResponse is the response for the sync status endpoint
type SyncStatusResponse struct {
	Timestamp          time.Time              `json:"timestamp"`
	StalenessThreshold string                 `json:"staleness_threshold"`
	DefaultInitial     SyncWindowInfo         `json:"default_initial_window"`
	DefaultBackground  SyncWindowInfo         `json:"default_background_window"`
	Connections        []ConnectionSyncStatus `json:"connections"`
	Calendars          []CalendarSyncStatus   `json:"calendars"`
}

// SyncWindowInfo describes a sync window
type SyncWindowInfo struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Weeks int    `json:"weeks"`
}

// SyncStatus returns detailed sync status for debugging
func (h *DebugHandler) SyncStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user ID from JWT
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get all connections for this user
	connections, err := h.connections.List(ctx, userID)
	if err != nil {
		http.Error(w, "Failed to list connections: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Build connection status list
	connStatuses := make([]ConnectionSyncStatus, 0, len(connections))
	for _, conn := range connections {
		connStatuses = append(connStatuses, ConnectionSyncStatus{
			ID:           conn.ID.String(),
			Provider:     conn.Provider,
			LastSyncedAt: conn.LastSyncedAt,
			IsStale:      sync.IsStale(conn.LastSyncedAt),
			CreatedAt:    conn.CreatedAt,
		})
	}

	// Get all calendars for this user
	var calStatuses []CalendarSyncStatus
	for _, conn := range connections {
		calendars, err := h.calendars.ListByConnection(ctx, conn.ID)
		if err != nil {
			continue
		}

		for _, cal := range calendars {
			// Calculate synced weeks
			syncedWeeks := 0
			if cal.MinSyncedDate != nil && cal.MaxSyncedDate != nil {
				weeks := sync.WeeksInRange(*cal.MinSyncedDate, *cal.MaxSyncedDate)
				syncedWeeks = len(weeks)
			}

			calStatuses = append(calStatuses, CalendarSyncStatus{
				ID:            cal.ID.String(),
				Name:          cal.Name,
				ExternalID:    cal.ExternalID,
				ConnectionID:  cal.ConnectionID.String(),
				IsSelected:    cal.IsSelected,
				IsPrimary:     cal.IsPrimary,
				MinSyncedDate: cal.MinSyncedDate,
				MaxSyncedDate: cal.MaxSyncedDate,
				LastSyncedAt:  cal.LastSyncedAt,
				SyncToken:     nil, // Don't expose actual token
				SyncTokenSet:  cal.SyncToken != nil && *cal.SyncToken != "",
				NeedsReauth:   cal.NeedsReauth,
				SyncFailures:  cal.SyncFailureCount,
				IsStale:       sync.IsStale(cal.LastSyncedAt),
				SyncedWeeks:   syncedWeeks,
				CreatedAt:     cal.CreatedAt,
				UpdatedAt:     cal.UpdatedAt,
			})
		}
	}

	// Calculate default windows
	initialStart, initialEnd := sync.DefaultInitialWindow()
	bgStart, bgEnd := sync.DefaultBackgroundWindow()

	initialWeeks := len(sync.WeeksInRange(initialStart, initialEnd))
	bgWeeks := len(sync.WeeksInRange(bgStart, bgEnd))

	response := SyncStatusResponse{
		Timestamp:          time.Now().UTC(),
		StalenessThreshold: sync.StalenessThreshold.String(),
		DefaultInitial: SyncWindowInfo{
			Start: initialStart.Format("2006-01-02"),
			End:   initialEnd.Format("2006-01-02"),
			Weeks: initialWeeks,
		},
		DefaultBackground: SyncWindowInfo{
			Start: bgStart.Format("2006-01-02"),
			End:   bgEnd.Format("2006-01-02"),
			Weeks: bgWeeks,
		},
		Connections: connStatuses,
		Calendars:   calStatuses,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
