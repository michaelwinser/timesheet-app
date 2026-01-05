package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/michaelw/timesheet-app/service/internal/classification"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

// MCPHandler handles MCP protocol requests over HTTP
type MCPHandler struct {
	projects          *store.ProjectStore
	entries           *store.TimeEntryStore
	calendarEvents    *store.CalendarEventStore
	rules             *store.ClassificationRuleStore
	apiKeys           *store.APIKeyStore
	mcpOAuth          *store.MCPOAuthStore
	classificationSvc *classification.Service
	jwt               *JWTService
	baseURL           string
	tools             []mcpTool
	resources         []mcpResource
}

type mcpResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
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
	rules *store.ClassificationRuleStore,
	apiKeys *store.APIKeyStore,
	mcpOAuth *store.MCPOAuthStore,
	classificationSvc *classification.Service,
	jwt *JWTService,
	baseURL string,
) *MCPHandler {
	h := &MCPHandler{
		projects:          projects,
		entries:           entries,
		calendarEvents:    calendarEvents,
		rules:             rules,
		apiKeys:           apiKeys,
		mcpOAuth:          mcpOAuth,
		classificationSvc: classificationSvc,
		jwt:               jwt,
		baseURL:           strings.TrimSuffix(baseURL, "/"),
	}
	h.initTools()
	h.initResources()
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
		{
			Name:        "search_events",
			Description: "Search calendar events using query syntax. Read the timesheet://docs/query-syntax resource first to understand the query language. Use this to find events by status, project, attendees, title, etc.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Query string using the search syntax (e.g., 'status:pending', 'domain:acme.com', 'title:standup'). See timesheet://docs/query-syntax resource.",
					},
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
						"description": "Maximum events to return (default 50)",
						"default":     50,
					},
				},
			},
		},
		{
			Name:        "list_rules",
			Description: "List all classification rules. Rules automatically assign events to projects based on query patterns.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"include_disabled": map[string]any{
						"type":        "boolean",
						"description": "Include disabled rules",
						"default":     false,
					},
				},
			},
		},
		{
			Name:        "create_rule",
			Description: "Create a new classification rule. The rule will automatically classify matching events to the specified project. Read timesheet://docs/query-syntax first to understand query syntax.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"query", "project_id"},
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Query pattern to match events (e.g., 'domain:acme.com', 'title:standup')",
					},
					"project_id": map[string]any{
						"type":        "string",
						"description": "Project ID to assign matching events to",
					},
					"weight": map[string]any{
						"type":        "number",
						"description": "Rule priority weight (higher = stronger, default 1.0)",
						"default":     1.0,
					},
				},
			},
		},
		{
			Name:        "preview_rule",
			Description: "Test a query against events to see what would match before creating a rule. Always use this before create_rule to verify the query works as expected.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Query pattern to test",
					},
					"project_id": map[string]any{
						"type":        "string",
						"description": "Optional project ID to check for conflicts",
					},
					"start_date": map[string]any{
						"type":        "string",
						"description": "Start date for preview range (YYYY-MM-DD)",
					},
					"end_date": map[string]any{
						"type":        "string",
						"description": "End date for preview range (YYYY-MM-DD)",
					},
				},
			},
		},
		{
			Name:        "bulk_classify",
			Description: "Classify multiple events matching a query to a project (or skip them). More efficient than classifying one by one.",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Query to match events (e.g., 'status:pending domain:acme.com')",
					},
					"project_id": map[string]any{
						"type":        "string",
						"description": "Project ID to assign matching events to",
					},
					"skip": map[string]any{
						"type":        "boolean",
						"description": "If true, mark matching events as skipped instead of classifying",
						"default":     false,
					},
				},
			},
		},
		{
			Name:        "apply_rules",
			Description: "Run all enabled classification rules against pending events. This applies rules to unclassified events and creates time entries.",
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
					"dry_run": map[string]any{
						"type":        "boolean",
						"description": "If true, show what would be classified without making changes",
						"default":     false,
					},
				},
			},
		},
	}
}

func (h *MCPHandler) initResources() {
	h.resources = []mcpResource{
		{
			URI:         "timesheet://docs/query-syntax",
			Name:        "Query Syntax Reference",
			Description: "Complete reference for the Gmail-style query syntax used to search events and create classification rules",
			MimeType:    "text/markdown",
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
	case "search_events":
		return h.searchEvents(ctx, userID, args)
	case "list_rules":
		return h.listRules(ctx, userID, args)
	case "create_rule":
		return h.createRule(ctx, userID, args)
	case "preview_rule":
		return h.previewRule(ctx, userID, args)
	case "bulk_classify":
		return h.bulkClassify(ctx, userID, args)
	case "apply_rules":
		return h.applyRules(ctx, userID, args)
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

func (h *MCPHandler) searchEvents(ctx context.Context, userID uuid.UUID, args map[string]any) (any, error) {
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

	limit := 50
	if v, ok := args["limit"].(float64); ok {
		limit = int(v)
	}

	query, _ := args["query"].(string)

	// Get all events in range
	events, err := h.calendarEvents.List(ctx, userID, &startDate, &endDate, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	// If query provided, filter events using classifier
	var matchedEvents []*store.CalendarEvent
	if query != "" {
		// Use the classification service's preview to filter
		preview, err := h.classificationSvc.PreviewRule(ctx, userID, query, nil, &startDate, &endDate)
		if err != nil {
			return nil, fmt.Errorf("invalid query: %w", err)
		}

		// Build a set of matched event IDs
		matchedIDs := make(map[uuid.UUID]bool)
		for _, m := range preview.Matches {
			matchedIDs[m.EventID] = true
		}

		// Filter events
		for _, e := range events {
			if matchedIDs[e.ID] {
				matchedEvents = append(matchedEvents, e)
			}
		}
	} else {
		matchedEvents = events
	}

	if len(matchedEvents) == 0 {
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "No events found matching the query."},
			},
		}, nil
	}

	if len(matchedEvents) > limit {
		matchedEvents = matchedEvents[:limit]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Search Results (%d events)\n\n", len(matchedEvents)))

	for _, e := range matchedEvents {
		duration := e.EndTime.Sub(e.StartTime).Hours()
		status := string(e.ClassificationStatus)
		projectInfo := ""
		if e.Project != nil {
			projectInfo = fmt.Sprintf(" → %s", e.Project.Name)
		}

		sb.WriteString(fmt.Sprintf("## %s\n", e.Title))
		sb.WriteString(fmt.Sprintf("- **ID**: `%s`\n", e.ID))
		sb.WriteString(fmt.Sprintf("- **Date**: %s\n", e.StartTime.Format("2006-01-02 15:04")))
		sb.WriteString(fmt.Sprintf("- **Duration**: %s\n", formatHours(duration)))
		sb.WriteString(fmt.Sprintf("- **Status**: %s%s\n", status, projectInfo))
		if len(e.Attendees) > 0 {
			attendees := e.Attendees
			if len(attendees) > 3 {
				attendees = append(attendees[:3], fmt.Sprintf("... +%d more", len(e.Attendees)-3))
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

func (h *MCPHandler) listRules(ctx context.Context, userID uuid.UUID, args map[string]any) (any, error) {
	includeDisabled := false
	if v, ok := args["include_disabled"].(bool); ok {
		includeDisabled = v
	}

	rules, err := h.rules.List(ctx, userID, includeDisabled)
	if err != nil {
		return nil, fmt.Errorf("failed to list rules: %w", err)
	}

	if len(rules) == 0 {
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "No classification rules defined. Use create_rule to add rules."},
			},
		}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Classification Rules (%d)\n\n", len(rules)))

	for _, r := range rules {
		status := ""
		if !r.IsEnabled {
			status = " (disabled)"
		}

		projectName := "skip"
		if r.ProjectID != nil {
			// Look up project name
			if project, err := h.projects.GetByID(ctx, *r.ProjectID, userID); err == nil {
				projectName = project.Name
			} else {
				projectName = r.ProjectID.String()
			}
		}

		sb.WriteString(fmt.Sprintf("## Rule: `%s`%s\n", r.Query, status))
		sb.WriteString(fmt.Sprintf("- **ID**: `%s`\n", r.ID))
		sb.WriteString(fmt.Sprintf("- **Project**: %s\n", projectName))
		sb.WriteString(fmt.Sprintf("- **Weight**: %.1f\n", r.Weight))
		sb.WriteString("\n")
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": sb.String()},
		},
	}, nil
}

func (h *MCPHandler) createRule(ctx context.Context, userID uuid.UUID, args map[string]any) (any, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query is required")
	}

	projectIDStr, ok := args["project_id"].(string)
	if !ok || projectIDStr == "" {
		return nil, fmt.Errorf("project_id is required")
	}

	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid project_id: %w", err)
	}

	weight := 1.0
	if v, ok := args["weight"].(float64); ok {
		weight = v
	}

	// Validate query by trying to parse it
	if _, err := classification.Parse(query); err != nil {
		return nil, fmt.Errorf("invalid query syntax: %w", err)
	}

	// Verify project exists
	project, err := h.projects.GetByID(ctx, projectID, userID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	// Create the rule
	rule := &store.ClassificationRule{
		UserID:    userID,
		Query:     query,
		ProjectID: &projectID,
		Weight:    weight,
		IsEnabled: true,
	}

	created, err := h.rules.Create(ctx, rule)
	if err != nil {
		return nil, fmt.Errorf("failed to create rule: %w", err)
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": fmt.Sprintf("Created rule:\n- **Query**: `%s`\n- **Project**: %s\n- **ID**: `%s`\n\nUse apply_rules to run this rule against pending events.", created.Query, project.Name, created.ID)},
		},
	}, nil
}

func (h *MCPHandler) previewRule(ctx context.Context, userID uuid.UUID, args map[string]any) (any, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query is required")
	}

	var projectID *uuid.UUID
	if pidStr, ok := args["project_id"].(string); ok && pidStr != "" {
		pid, err := uuid.Parse(pidStr)
		if err != nil {
			return nil, fmt.Errorf("invalid project_id: %w", err)
		}
		projectID = &pid
	}

	var startDate, endDate *time.Time
	if v, ok := args["start_date"].(string); ok && v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			startDate = &t
		}
	}
	if v, ok := args["end_date"].(string); ok && v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			endDate = &t
		}
	}

	preview, err := h.classificationSvc.PreviewRule(ctx, userID, query, projectID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Rule Preview: `%s`\n\n", query))
	sb.WriteString(fmt.Sprintf("## Statistics\n"))
	sb.WriteString(fmt.Sprintf("- **Total matches**: %d\n", preview.Stats.TotalMatches))
	sb.WriteString(fmt.Sprintf("- **Already correct**: %d\n", preview.Stats.AlreadyCorrect))
	sb.WriteString(fmt.Sprintf("- **Would change**: %d\n", preview.Stats.WouldChange))
	if preview.Stats.ManualConflicts > 0 {
		sb.WriteString(fmt.Sprintf("- **Manual conflicts** (won't override): %d\n", preview.Stats.ManualConflicts))
	}
	sb.WriteString("\n")

	if len(preview.Matches) > 0 {
		sb.WriteString("## Matching Events (first 10)\n\n")
		limit := 10
		if len(preview.Matches) < limit {
			limit = len(preview.Matches)
		}
		for i := 0; i < limit; i++ {
			m := preview.Matches[i]
			sb.WriteString(fmt.Sprintf("- %s (%s)\n", m.Title, m.StartTime.Format("2006-01-02")))
		}
		if len(preview.Matches) > 10 {
			sb.WriteString(fmt.Sprintf("\n... and %d more\n", len(preview.Matches)-10))
		}
	} else {
		sb.WriteString("*No events match this query.*\n")
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": sb.String()},
		},
	}, nil
}

func (h *MCPHandler) bulkClassify(ctx context.Context, userID uuid.UUID, args map[string]any) (any, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query is required")
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

	// Find matching events using preview
	preview, err := h.classificationSvc.PreviewRule(ctx, userID, query, projectID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}

	var classifiedCount, skippedCount, entriesCreated int

	// Process each matching event
	for _, match := range preview.Matches {
		event, err := h.calendarEvents.GetByID(ctx, userID, match.EventID)
		if err != nil {
			continue
		}

		// Skip manually classified events
		if event.ClassificationSource != nil && *event.ClassificationSource == store.SourceManual {
			continue
		}

		// Classify the event
		_, err = h.calendarEvents.Classify(ctx, userID, match.EventID, projectID, skip)
		if err != nil {
			continue
		}

		if skip {
			skippedCount++
		} else {
			classifiedCount++

			// Create time entry
			duration := event.EndTime.Sub(event.StartTime).Hours()
			eventDate := time.Date(event.StartTime.Year(), event.StartTime.Month(), event.StartTime.Day(), 0, 0, 0, 0, time.UTC)
			if _, err := h.entries.CreateFromCalendar(ctx, userID, *projectID, eventDate, duration, &event.Title); err == nil {
				entriesCreated++
			}
		}
	}

	projectName := ""
	if projectID != nil {
		if project, err := h.projects.GetByID(ctx, *projectID, userID); err == nil {
			projectName = project.Name
		}
	}

	var result string
	if skip {
		result = fmt.Sprintf("Bulk skip complete:\n- Query: `%s`\n- Events skipped: %d", query, skippedCount)
	} else {
		result = fmt.Sprintf("Bulk classification complete:\n- Query: `%s`\n- Project: %s\n- Events classified: %d\n- Time entries created: %d", query, projectName, classifiedCount, entriesCreated)
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": result},
		},
	}, nil
}

func (h *MCPHandler) applyRules(ctx context.Context, userID uuid.UUID, args map[string]any) (any, error) {
	var startDate, endDate *time.Time
	now := time.Now()
	defaultStart := now.AddDate(0, 0, -30)
	startDate = &defaultStart
	endDate = &now

	if v, ok := args["start_date"].(string); ok && v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			startDate = &t
		}
	}
	if v, ok := args["end_date"].(string); ok && v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			endDate = &t
		}
	}

	dryRun := false
	if v, ok := args["dry_run"].(bool); ok {
		dryRun = v
	}

	// Get projects to build targets
	projects, err := h.projects.List(ctx, userID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	// Build targets from projects
	targets := make([]classification.Target, 0, len(projects))
	for _, p := range projects {
		attrs := make(map[string]any)
		attrs["name"] = p.Name
		if p.FingerprintDomains != nil {
			attrs["domains"] = p.FingerprintDomains
		}
		if p.FingerprintEmails != nil {
			attrs["emails"] = p.FingerprintEmails
		}
		if p.FingerprintKeywords != nil {
			attrs["keywords"] = p.FingerprintKeywords
		}
		target := classification.Target{
			ID:         p.ID.String(),
			Attributes: attrs,
		}
		targets = append(targets, target)
	}

	result, err := h.classificationSvc.ApplyRules(ctx, userID, targets, startDate, endDate, dryRun)
	if err != nil {
		return nil, fmt.Errorf("failed to apply rules: %w", err)
	}

	var sb strings.Builder
	if dryRun {
		sb.WriteString("# Apply Rules (Dry Run)\n\n")
	} else {
		sb.WriteString("# Apply Rules Results\n\n")
	}

	sb.WriteString(fmt.Sprintf("- **Events classified**: %d\n", len(result.Classified)))
	sb.WriteString(fmt.Sprintf("- **Events skipped** (no matching rule): %d\n", result.Skipped))

	if len(result.Classified) > 0 && len(result.Classified) <= 10 {
		sb.WriteString("\n## Classified Events\n\n")
		for _, c := range result.Classified {
			projectName := c.TargetID.String()
			if project, err := h.projects.GetByID(ctx, c.TargetID, userID); err == nil {
				projectName = project.Name
			}
			review := ""
			if c.NeedsReview {
				review = " (needs review)"
			}
			sb.WriteString(fmt.Sprintf("- %s → %s (%.0f%% confidence)%s\n", c.EventID, projectName, c.Confidence*100, review))
		}
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": sb.String()},
		},
	}, nil
}

// getQuerySyntaxDoc returns the query syntax documentation as markdown
func (h *MCPHandler) getQuerySyntaxDoc() string {
	return `# Query Syntax Reference

This document describes the Gmail-style query syntax used for searching events and creating classification rules.

## Basic Syntax

Queries consist of property:value pairs. Multiple terms are combined with AND by default.

` + "```" + `
title:standup                    # Title contains "standup"
domain:acme.com                  # Attendee from acme.com domain
title:meeting domain:client.com  # Both conditions (implicit AND)
` + "```" + `

## Properties

| Property | Type | Description |
|----------|------|-------------|
| ` + "`title`" + ` | string | Event title (contains, case-insensitive) |
| ` + "`description`" + ` | string | Event description (contains) |
| ` + "`attendees`" + ` | string | Attendee name or email (contains) |
| ` + "`domain`" + ` | string | Attendee email domain (exact match) |
| ` + "`email`" + ` | string | Attendee email (exact match) |
| ` + "`calendar`" + ` | string | Calendar name (contains) |
| ` + "`text`" + ` | string | Searches title, description, and attendees |
| ` + "`response`" + ` | enum | User's response: accepted, declined, needsAction, tentative |
| ` + "`recurring`" + ` | boolean | yes/no - Is this a recurring event? |
| ` + "`transparency`" + ` | enum | opaque (busy) or transparent (free) |
| ` + "`is-all-day`" + ` | boolean | yes/no |
| ` + "`has-attendees`" + ` | boolean | yes/no |
| ` + "`day-of-week`" + ` | enum | mon, tue, wed, thu, fri, sat, sun |
| ` + "`time-of-day`" + ` | time | HH:MM with operators: >, >=, <, <=, = |
| ` + "`status`" + ` | enum | pending, classified, skipped |
| ` + "`project`" + ` | string | Project name (for classified events) |
| ` + "`confidence`" + ` | number | Classification confidence: >0.8, <0.5, etc. |

## Operators

### Logical Operators
` + "```" + `
title:standup title:daily        # Implicit AND
title:standup OR title:sync      # Explicit OR
(domain:a.com OR domain:b.com)   # Grouping with parentheses
` + "```" + `

### Negation
` + "```" + `
-title:canceled                  # NOT - exclude canceled events
-response:declined               # Exclude declined events
` + "```" + `

### Quoted Strings
` + "```" + `
title:"team meeting"             # Multi-word exact phrase
domain:"sub.example.com"         # Exact domain match
` + "```" + `

## Examples

### Find pending events from a specific company
` + "```" + `
status:pending domain:acme.com
` + "```" + `

### Find all standups not yet classified
` + "```" + `
status:pending title:standup
` + "```" + `

### Find events on weekends
` + "```" + `
day-of-week:sat OR day-of-week:sun
` + "```" + `

### Find events after 5 PM that you declined
` + "```" + `
time-of-day:>17:00 response:declined
` + "```" + `

### Find classified events for a project
` + "```" + `
status:classified project:"Acme Corp"
` + "```" + `

### Find low-confidence classifications that need review
` + "```" + `
status:classified confidence:<0.7
` + "```" + `

### Exclude all-day events and find meetings with external attendees
` + "```" + `
-is-all-day:yes has-attendees:yes -domain:mycompany.com
` + "```" + `

## Tips for Rule Creation

1. **Start specific, then broaden**: Begin with very specific queries and test with preview_rule
2. **Use domain for client identification**: ` + "`domain:client.com`" + ` is reliable for external meetings
3. **Combine title and attendees**: ` + "`title:standup has-attendees:no`" + ` for personal standups
4. **Use negation for exclusions**: ` + "`-title:canceled -response:declined`" + ` to skip irrelevant events
5. **Test before saving**: Always use preview_rule to see what matches before create_rule
`
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
				"tools":     map[string]any{},
				"resources": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "timesheet",
				"version": "1.0.0",
			},
			"instructions": `You are an AI assistant helping manage a timesheet application.

The user tracks their time across different projects. Calendar events are synced from
Google Calendar and need to be classified (assigned to projects or marked as skipped).

IMPORTANT: Before using search_events, create_rule, or preview_rule tools, first read the
timesheet://docs/query-syntax resource to understand the query language.

When helping the user:
1. Read timesheet://docs/query-syntax to learn the search syntax
2. List projects to understand available classification targets
3. Search for pending events to see what needs attention
4. Use preview_rule to test classification patterns
5. Create rules or use bulk_classify to classify events`,
		}

	case "initialized", "notifications/initialized":
		w.WriteHeader(http.StatusNoContent)
		return

	case "resources/list":
		result = map[string]any{
			"resources": h.resources,
		}

	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			h.sendJSONRPCError(w, req.ID, -32602, "Invalid params", err.Error())
			return
		}

		// Handle known resources
		switch params.URI {
		case "timesheet://docs/query-syntax":
			result = map[string]any{
				"contents": []map[string]any{
					{
						"uri":      params.URI,
						"mimeType": "text/markdown",
						"text":     h.getQuerySyntaxDoc(),
					},
				},
			}
		default:
			h.sendJSONRPCError(w, req.ID, -32002, "Resource not found", params.URI)
			return
		}

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
