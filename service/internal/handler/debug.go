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

// SyncStatusPage serves a simple HTML debug page with auto-refresh
func (h *DebugHandler) SyncStatusPage(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Sync Debug</title>
    <meta http-equiv="refresh" content="5">
    <style>
        body { font-family: monospace; padding: 20px; background: #1a1a1a; color: #e0e0e0; }
        h1 { color: #fff; }
        h2 { color: #aaa; margin-top: 20px; }
        .section { background: #2a2a2a; padding: 15px; margin: 10px 0; border-radius: 5px; }
        .ok { color: #4ade80; }
        .warn { color: #fbbf24; }
        .error { color: #f87171; }
        table { border-collapse: collapse; width: 100%; }
        th, td { text-align: left; padding: 8px; border-bottom: 1px solid #444; }
        th { color: #888; }
        pre { background: #111; padding: 10px; overflow-x: auto; }
        .refresh-note { color: #666; font-size: 12px; }
    </style>
</head>
<body>
    <h1>Sync Status Debug</h1>
    <p class="refresh-note">Auto-refreshes every 5 seconds. Last refresh: <span id="time"></span></p>
    <div id="content">Loading...</div>
    <script>
        document.getElementById('time').textContent = new Date().toLocaleTimeString();

        const token = localStorage.getItem('timesheet_token');
        if (!token) {
            document.getElementById('content').innerHTML = '<div class="error">Not logged in. Please <a href="/" style="color:#60a5fa">login to the app</a> first.</div>';
        } else {
        fetch('/api/debug/sync-status', {
            headers: { 'Authorization': 'Bearer ' + token }
        })
            .then(r => r.json())
            .then(data => {
                let html = '';

                // Config section
                html += '<div class="section"><h2>Configuration</h2>';
                html += '<p>Staleness: ' + data.staleness_threshold + '</p>';
                html += '<p>Initial Window: ' + data.default_initial_window.start + ' to ' + data.default_initial_window.end + ' (' + data.default_initial_window.weeks + ' weeks)</p>';
                html += '<p>Background Window: ' + data.default_background_window.start + ' to ' + data.default_background_window.end + ' (' + data.default_background_window.weeks + ' weeks)</p>';
                html += '</div>';

                // Connections
                const connections = data.connections || [];
                html += '<div class="section"><h2>Connections (' + connections.length + ')</h2>';
                if (connections.length > 0) {
                    html += '<table><tr><th>ID</th><th>Provider</th><th>Last Synced</th><th>Status</th></tr>';
                    connections.forEach(c => {
                        let status = c.is_stale ? '<span class="warn">Stale</span>' : '<span class="ok">Fresh</span>';
                        let lastSync = c.last_synced_at ? new Date(c.last_synced_at).toLocaleString() : 'never';
                        html += '<tr><td>' + c.id.slice(0,8) + '...</td><td>' + c.provider + '</td><td>' + lastSync + '</td><td>' + status + '</td></tr>';
                    });
                    html += '</table>';
                } else {
                    html += '<p>No connections</p>';
                }
                html += '</div>';

                // Calendars
                const calendars = data.calendars || [];
                html += '<div class="section"><h2>Calendars (' + calendars.length + ')</h2>';
                if (calendars.length > 0) {
                    calendars.forEach(cal => {
                        let statusClass = cal.needs_reauth ? 'error' : (cal.is_stale || cal.sync_failure_count > 0) ? 'warn' : 'ok';
                        let statusText = cal.needs_reauth ? 'NEEDS REAUTH' : cal.sync_failure_count > 0 ? cal.sync_failure_count + ' failures' : cal.is_stale ? 'Stale' : 'OK';
                        let lastSync = cal.last_synced_at ? new Date(cal.last_synced_at).toLocaleString() : 'never';
                        let watermarks = (cal.min_synced_date && cal.max_synced_date) ?
                            cal.min_synced_date.slice(0,10) + ' to ' + cal.max_synced_date.slice(0,10) : 'Not set';

                        html += '<div style="border-left: 3px solid ' + (statusClass === 'ok' ? '#4ade80' : statusClass === 'warn' ? '#fbbf24' : '#f87171') + '; padding-left: 10px; margin: 10px 0;">';
                        html += '<strong>' + cal.name + '</strong>';
                        if (cal.is_primary) html += ' [Primary]';
                        if (!cal.is_selected) html += ' <span style="color:#666">[Not Selected]</span>';
                        html += ' <span class="' + statusClass + '">[' + statusText + ']</span><br>';
                        html += 'Water Marks: ' + watermarks + '<br>';
                        html += 'Synced Weeks: ' + cal.synced_weeks + '<br>';
                        html += 'Last Synced: ' + lastSync + '<br>';
                        html += 'Sync Token: ' + (cal.sync_token_set ? 'Set' : '<span class="warn">Not set</span>') + '<br>';
                        html += '<span style="color:#666">ID: ' + cal.id + '</span>';
                        html += '</div>';
                    });
                } else {
                    html += '<p>No calendars</p>';
                }
                html += '</div>';

                // Raw JSON
                html += '<details><summary style="cursor:pointer;color:#666">Raw JSON</summary><pre>' + JSON.stringify(data, null, 2) + '</pre></details>';

                document.getElementById('content').innerHTML = html;
            })
            .catch(err => {
                document.getElementById('content').innerHTML = '<div class="error">Error: ' + err.message + '</div>';
            });
        }
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}
