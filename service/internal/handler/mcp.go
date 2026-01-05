package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

// MCPHandler handles MCP protocol requests over HTTP
type MCPHandler struct {
	projects       *store.ProjectStore
	entries        *store.TimeEntryStore
	calendarEvents *store.CalendarEventStore
	apiKeys        *store.APIKeyStore
	mcpOAuth       *store.MCPOAuthStore
	jwt            *JWTService
	baseURL        string
	tools          []mcpTool
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema"`
}

// NewMCPHandler creates a new MCP handler
func NewMCPHandler(
	projects *store.ProjectStore,
	entries *store.TimeEntryStore,
	calendarEvents *store.CalendarEventStore,
	apiKeys *store.APIKeyStore,
	mcpOAuth *store.MCPOAuthStore,
	jwt *JWTService,
	baseURL string,
) *MCPHandler {
	h := &MCPHandler{
		projects:       projects,
		entries:        entries,
		calendarEvents: calendarEvents,
		apiKeys:        apiKeys,
		mcpOAuth:       mcpOAuth,
		jwt:            jwt,
		baseURL:        strings.TrimSuffix(baseURL, "/"),
	}
	h.initTools()
	return h
}

func (h *MCPHandler) initTools() {
	h.tools = []mcpTool{
		{
			Name:        "list_projects",
			Description: "List all projects. Use this first to understand available options for classification.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"include_archived": map[string]any{
						"type":        "boolean",
						"description": "Include archived/inactive projects",
						"default":     false,
					},
				},
			},
		},
		{
			Name:        "get_time_summary",
			Description: "Get a summary of time entries grouped by project or date. Useful for analyzing time spent.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"start_date": map[string]any{
						"type":        "string",
						"description": "Start date (YYYY-MM-DD). Defaults to 7 days ago.",
					},
					"end_date": map[string]any{
						"type":        "string",
						"description": "End date (YYYY-MM-DD). Defaults to today.",
					},
					"group_by": map[string]any{
						"type":        "string",
						"description": "How to group: 'project' or 'date'",
						"enum":        []string{"project", "date"},
						"default":     "project",
					},
				},
			},
		},
		{
			Name:        "list_pending_events",
			Description: "List calendar events that need classification (assignment to a project or skip).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"start_date": map[string]any{
						"type":        "string",
						"description": "Start date (YYYY-MM-DD). Defaults to 30 days ago.",
					},
					"end_date": map[string]any{
						"type":        "string",
						"description": "End date (YYYY-MM-DD). Defaults to today.",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum events to return",
						"default":     20,
					},
				},
			},
		},
		{
			Name:        "classify_event",
			Description: "Classify a calendar event by assigning it to a project or skipping it.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"event_id"},
				"properties": map[string]any{
					"event_id": map[string]any{
						"type":        "string",
						"description": "The calendar event ID to classify",
					},
					"project_id": map[string]any{
						"type":        "string",
						"description": "Project ID to assign (from list_projects)",
					},
					"skip": map[string]any{
						"type":        "boolean",
						"description": "Set true to mark as skipped (not work time)",
						"default":     false,
					},
				},
			},
		},
		{
			Name:        "create_time_entry",
			Description: "Create a manual time entry for work not captured by calendar events.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"project_id", "date", "hours"},
				"properties": map[string]any{
					"project_id": map[string]any{
						"type":        "string",
						"description": "Project ID (from list_projects)",
					},
					"date": map[string]any{
						"type":        "string",
						"description": "Date in YYYY-MM-DD format",
					},
					"hours": map[string]any{
						"type":        "number",
						"description": "Number of hours (e.g., 1.5 for 1h 30m)",
					},
					"description": map[string]any{
						"type":        "string",
						"description": "Optional description of work done",
					},
				},
			},
		},
	}
}

func formatHours(hours float64) string {
	h := int(hours)
	m := int((hours - float64(h)) * 60)
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

// Tool handlers
func (h *MCPHandler) callTool(ctx context.Context, userID uuid.UUID, name string, args map[string]any) (any, error) {
	switch name {
	case "list_projects":
		return h.listProjects(ctx, userID, args)
	case "get_time_summary":
		return h.getTimeSummary(ctx, userID, args)
	case "list_pending_events":
		return h.listPendingEvents(ctx, userID, args)
	case "classify_event":
		return h.classifyEvent(ctx, userID, args)
	case "create_time_entry":
		return h.createTimeEntry(ctx, userID, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (h *MCPHandler) listProjects(ctx context.Context, userID uuid.UUID, args map[string]any) (any, error) {
	includeArchived := false
	if v, ok := args["include_archived"].(bool); ok {
		includeArchived = v
	}

	projects, err := h.projects.List(ctx, userID, includeArchived)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	var sb strings.Builder
	sb.WriteString("# Projects\n\n")
	for _, p := range projects {
		status := ""
		if p.IsArchived {
			status = " (archived)"
		} else if !p.IsBillable {
			status = " (non-billable)"
		}
		sb.WriteString(fmt.Sprintf("- **%s**%s\n", p.Name, status))
		sb.WriteString(fmt.Sprintf("  - ID: `%s`\n", p.ID))
		if p.Client != nil && *p.Client != "" {
			sb.WriteString(fmt.Sprintf("  - Client: %s\n", *p.Client))
		}
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": sb.String()},
		},
	}, nil
}

func (h *MCPHandler) getTimeSummary(ctx context.Context, userID uuid.UUID, args map[string]any) (any, error) {
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -7)

	if v, ok := args["start_date"].(string); ok && v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			startDate = t
		}
	}
	if v, ok := args["end_date"].(string); ok && v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			endDate = t
		}
	}

	groupBy := "project"
	if v, ok := args["group_by"].(string); ok {
		groupBy = v
	}

	entries, err := h.entries.List(ctx, userID, &startDate, &endDate, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list entries: %w", err)
	}

	if len(entries) == 0 {
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("No time entries found between %s and %s.", startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))},
			},
		}, nil
	}

	var totalHours float64
	for _, e := range entries {
		totalHours += e.Hours
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Time Summary (%s to %s)\n\n", startDate.Format("2006-01-02"), endDate.Format("2006-01-02")))
	sb.WriteString(fmt.Sprintf("**Total: %s**\n\n", formatHours(totalHours)))

	if groupBy == "project" {
		byProject := make(map[uuid.UUID]float64)
		projectNames := make(map[uuid.UUID]string)
		for _, e := range entries {
			byProject[e.ProjectID] += e.Hours
			if e.Project != nil {
				projectNames[e.ProjectID] = e.Project.Name
			}
		}

		sb.WriteString("## By Project\n\n")
		for pid, hours := range byProject {
			name := projectNames[pid]
			if name == "" {
				name = pid.String()
			}
			pct := 0.0
			if totalHours > 0 {
				pct = hours / totalHours * 100
			}
			sb.WriteString(fmt.Sprintf("- %s: %s (%.0f%%)\n", name, formatHours(hours), pct))
		}
	} else {
		byDate := make(map[string]float64)
		for _, e := range entries {
			dateStr := e.Date.Format("2006-01-02")
			byDate[dateStr] += e.Hours
		}

		sb.WriteString("## By Date\n\n")
		for date, hours := range byDate {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", date, formatHours(hours)))
		}
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": sb.String()},
		},
	}, nil
}

func (h *MCPHandler) listPendingEvents(ctx context.Context, userID uuid.UUID, args map[string]any) (any, error) {
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -30)

	if v, ok := args["start_date"].(string); ok && v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			startDate = t
		}
	}
	if v, ok := args["end_date"].(string); ok && v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			endDate = t
		}
	}

	limit := 20
	if v, ok := args["limit"].(float64); ok {
		limit = int(v)
	}

	status := store.StatusPending
	events, err := h.calendarEvents.List(ctx, userID, &startDate, &endDate, &status, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	if len(events) == 0 {
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "No pending events. All caught up!"},
			},
		}, nil
	}

	if len(events) > limit {
		events = events[:limit]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Pending Calendar Events (%d shown)\n\n", len(events)))

	for _, e := range events {
		duration := e.EndTime.Sub(e.StartTime).Hours()
		sb.WriteString(fmt.Sprintf("## %s\n", e.Title))
		sb.WriteString(fmt.Sprintf("- **ID**: `%s`\n", e.ID))
		sb.WriteString(fmt.Sprintf("- **Date**: %s\n", e.StartTime.Format("2006-01-02 15:04")))
		sb.WriteString(fmt.Sprintf("- **Duration**: %s\n", formatHours(duration)))
		if len(e.Attendees) > 0 {
			attendees := e.Attendees
			if len(attendees) > 5 {
				attendees = attendees[:5]
			}
			sb.WriteString(fmt.Sprintf("- **Attendees**: %s\n", strings.Join(attendees, ", ")))
		}
		sb.WriteString("\n")
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": sb.String()},
		},
	}, nil
}

func (h *MCPHandler) classifyEvent(ctx context.Context, userID uuid.UUID, args map[string]any) (any, error) {
	eventIDStr, ok := args["event_id"].(string)
	if !ok || eventIDStr == "" {
		return nil, fmt.Errorf("event_id is required")
	}

	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid event_id: %w", err)
	}

	skip := false
	if v, ok := args["skip"].(bool); ok {
		skip = v
	}

	var projectID *uuid.UUID
	if pidStr, ok := args["project_id"].(string); ok && pidStr != "" {
		pid, err := uuid.Parse(pidStr)
		if err != nil {
			return nil, fmt.Errorf("invalid project_id: %w", err)
		}
		projectID = &pid
	}

	if projectID == nil && !skip {
		return nil, fmt.Errorf("must provide either project_id or skip=true")
	}

	// Classify the event
	event, err := h.calendarEvents.Classify(ctx, userID, eventID, projectID, skip)
	if err != nil {
		return nil, fmt.Errorf("failed to classify event: %w", err)
	}

	if skip {
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("Skipped event: **%s**", event.Title)},
			},
		}, nil
	}

	// Create time entry
	duration := event.EndTime.Sub(event.StartTime).Hours()
	_, err = h.entries.Create(ctx, userID, *projectID, event.StartTime, duration, nil)
	if err != nil {
		fmt.Printf("Warning: failed to create time entry: %v\n", err)
	}

	project, _ := h.projects.GetByID(ctx, *projectID, userID)
	projectName := projectID.String()
	if project != nil {
		projectName = project.Name
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": fmt.Sprintf("Classified event: **%s**\n- Project: %s\n- Hours: %s", event.Title, projectName, formatHours(duration))},
		},
	}, nil
}

func (h *MCPHandler) createTimeEntry(ctx context.Context, userID uuid.UUID, args map[string]any) (any, error) {
	projectIDStr, ok := args["project_id"].(string)
	if !ok || projectIDStr == "" {
		return nil, fmt.Errorf("project_id is required")
	}
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid project_id: %w", err)
	}

	dateStr, ok := args["date"].(string)
	if !ok || dateStr == "" {
		return nil, fmt.Errorf("date is required")
	}
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil, fmt.Errorf("invalid date format: %w", err)
	}

	hours, ok := args["hours"].(float64)
	if !ok || hours <= 0 {
		return nil, fmt.Errorf("hours must be a positive number")
	}

	var description *string
	if desc, ok := args["description"].(string); ok && desc != "" {
		description = &desc
	}

	entry, err := h.entries.Create(ctx, userID, projectID, date, hours, description)
	if err != nil {
		return nil, fmt.Errorf("failed to create entry: %w", err)
	}

	project, _ := h.projects.GetByID(ctx, projectID, userID)
	projectName := projectIDStr
	if project != nil {
		projectName = project.Name
	}

	result := fmt.Sprintf("Created time entry:\n- Project: %s\n- Date: %s\n- Hours: %s", projectName, entry.Date.Format("2006-01-02"), formatHours(entry.Hours))
	if description != nil {
		result += fmt.Sprintf("\n- Description: %s", *description)
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": result},
		},
	}, nil
}

// ServeHTTP handles MCP requests over HTTP using Streamable HTTP transport
func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Try to get user from context (set by auth middleware for JWT/API keys)
	userID, ok := UserIDFromContext(r.Context())

	// If not authenticated via middleware, check for MCP OAuth token
	if !ok {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer mcp_") {
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if h.mcpOAuth != nil {
				if uid, err := h.mcpOAuth.ValidateToken(r.Context(), token); err == nil {
					userID = uid
					ok = true
				}
			}
		}
	}

	if !ok {
		// Return OAuth challenge with resource metadata
		resourceMetadata := fmt.Sprintf("%s/.well-known/oauth-protected-resource", h.baseURL)
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s"`, resourceMetadata))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error":             "unauthorized",
			"error_description": "Authentication required. Complete OAuth flow or use an API key.",
		})
		return
	}

	switch r.Method {
	case "GET":
		h.handleSSE(w, r)
	case "POST":
		h.handleJSONRPC(w, r, userID)
	case "OPTIONS":
		w.Header().Set("Allow", "GET, POST, OPTIONS")
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *MCPHandler) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", r.URL.Path)
	flusher.Flush()

	<-r.Context().Done()
}

func (h *MCPHandler) handleJSONRPC(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      any             `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendJSONRPCError(w, nil, -32700, "Parse error", err.Error())
		return
	}

	var result any

	switch req.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "timesheet",
				"version": "1.0.0",
			},
			"instructions": `You are an AI assistant helping manage a timesheet application.

The user tracks their time across different projects. Calendar events are synced from
Google Calendar and need to be classified (assigned to projects or marked as skipped).

When helping the user:
1. First list projects to understand available options
2. Look at pending events to see what needs attention
3. Use time summaries to analyze patterns`,
		}

	case "initialized", "notifications/initialized":
		w.WriteHeader(http.StatusNoContent)
		return

	case "tools/list":
		result = map[string]any{
			"tools": h.tools,
		}

	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			h.sendJSONRPCError(w, req.ID, -32602, "Invalid params", err.Error())
			return
		}

		toolResult, err := h.callTool(r.Context(), userID, params.Name, params.Arguments)
		if err != nil {
			h.sendJSONRPCError(w, req.ID, -32000, "Tool error", err.Error())
			return
		}
		result = toolResult

	default:
		h.sendJSONRPCError(w, req.ID, -32601, "Method not found", req.Method)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result":  result,
	})
}

func (h *MCPHandler) sendJSONRPCError(w http.ResponseWriter, id any, code int, message, data string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
			"data":    data,
		},
	})
}
