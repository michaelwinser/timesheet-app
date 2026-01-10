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
	"github.com/michaelw/timesheet-app/service/internal/mcp"
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
	// Use generated tool definitions from OpenAPI spec
	genTools := mcp.GetTools()
	h.tools = make([]mcpTool, len(genTools))
	for i, t := range genTools {
		h.tools[i] = mcpTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
}

func (h *MCPHandler) initResources() {
	// Use generated resource definitions from OpenAPI spec
	genResources := mcp.GetResources()
	h.resources = make([]mcpResource, len(genResources))
	for i, r := range genResources {
		h.resources[i] = mcpResource{
			URI:         r.URI,
			Name:        r.Name,
			Description: r.Description,
			MimeType:    r.MimeType,
		}
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
	case "explain_classification":
		return h.explainClassification(ctx, userID, args)
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

	skip := false
	if v, ok := args["skip"].(bool); ok {
		skip = v
	}

	var projectID *uuid.UUID
	projectIDStr, hasProjectID := args["project_id"].(string)
	if hasProjectID && projectIDStr != "" {
		pid, err := uuid.Parse(projectIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid project_id: %w", err)
		}
		projectID = &pid
	}

	// Must specify either project_id or skip
	if projectID == nil && !skip {
		return nil, fmt.Errorf("must provide either project_id or skip=true")
	}

	weight := 1.0
	if v, ok := args["weight"].(float64); ok {
		weight = v
	}

	// Validate query by trying to parse it
	if _, err := classification.Parse(query); err != nil {
		return nil, fmt.Errorf("invalid query syntax: %w", err)
	}

	var projectName string
	if projectID != nil {
		// Verify project exists
		project, err := h.projects.GetByID(ctx, *projectID, userID)
		if err != nil {
			return nil, fmt.Errorf("project not found: %w", err)
		}
		projectName = project.Name
	}

	// Create the rule
	rule := &store.ClassificationRule{
		UserID:    userID,
		Query:     query,
		ProjectID: projectID,
		Weight:    weight,
		IsEnabled: true,
	}

	// For skip rules, set attended=false (this is how skip rules are stored)
	if skip {
		attended := false
		rule.Attended = &attended
	}

	created, err := h.rules.Create(ctx, rule)
	if err != nil {
		return nil, fmt.Errorf("failed to create rule: %w", err)
	}

	if skip {
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("Created skip rule:\n- **Query**: `%s`\n- **ID**: `%s`\n\nUse apply_rules to run this rule against pending events.", created.Query, created.ID)},
			},
		}, nil
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": fmt.Sprintf("Created rule:\n- **Query**: `%s`\n- **Project**: %s\n- **ID**: `%s`\n\nUse apply_rules to run this rule against pending events.", created.Query, projectName, created.ID)},
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

	var classifiedCount, skippedCount int
	affectedDates := make(map[time.Time]bool)

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

		// Track affected date for recalculation
		eventDate := time.Date(event.StartTime.Year(), event.StartTime.Month(), event.StartTime.Day(), 0, 0, 0, 0, time.UTC)
		affectedDates[eventDate] = true

		if skip {
			skippedCount++
		} else {
			classifiedCount++
		}
	}

	// With ephemeral time entries, we don't reactively create/update entries.
	// Time entries are computed on-demand when ListTimeEntries is called.

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
		result = fmt.Sprintf("Bulk classification complete:\n- Query: `%s`\n- Project: %s\n- Events classified: %d\n- Time entries will be computed on demand", query, projectName, classifiedCount)
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

	sb.WriteString(fmt.Sprintf("- **Events marked as skipped**: %d\n", len(result.SkipApplied)))
	sb.WriteString(fmt.Sprintf("- **Events classified to projects**: %d\n", len(result.Classified)))
	sb.WriteString(fmt.Sprintf("- **Events with no matching project rule**: %d\n", result.Skipped))

	if len(result.SkipApplied) > 0 && len(result.SkipApplied) <= 10 {
		sb.WriteString("\n## Skipped Events\n\n")
		for _, s := range result.SkipApplied {
			sb.WriteString(fmt.Sprintf("- %s (%.0f%% confidence)\n", s.EventID, s.Confidence*100))
		}
	} else if len(result.SkipApplied) > 10 {
		sb.WriteString(fmt.Sprintf("\n*%d events marked as skipped (too many to show)*\n", len(result.SkipApplied)))
	}

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
	} else if len(result.Classified) > 10 {
		sb.WriteString(fmt.Sprintf("\n*%d events classified (too many to show)*\n", len(result.Classified)))
	}

	return map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": sb.String()},
		},
	}, nil
}

func (h *MCPHandler) explainClassification(ctx context.Context, userID uuid.UUID, args map[string]any) (any, error) {
	eventIDStr, ok := args["event_id"].(string)
	if !ok || eventIDStr == "" {
		return nil, fmt.Errorf("event_id is required")
	}

	eventID, err := uuid.Parse(eventIDStr)
	if err != nil {
		return nil, fmt.Errorf("invalid event_id: %w", err)
	}

	// Get the event first to show its details
	event, err := h.calendarEvents.GetByID(ctx, userID, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}

	// Get projects to build targets with names
	projects, err := h.projects.List(ctx, userID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	// Build targets from projects (including fingerprints and names)
	projectNames := make(map[string]string)
	targets := make([]classification.Target, 0, len(projects))
	for _, p := range projects {
		attrs := make(map[string]any)
		attrs["name"] = p.Name
		projectNames[p.ID.String()] = p.Name
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

	// Get explain result
	result, err := h.classificationSvc.ExplainEventClassification(ctx, userID, eventID, targets)
	if err != nil {
		return nil, fmt.Errorf("failed to explain classification: %w", err)
	}

	// Build markdown output
	var sb strings.Builder
	sb.WriteString("# Classification Explanation\n\n")

	// Event details
	sb.WriteString("## Event\n\n")
	sb.WriteString(fmt.Sprintf("- **Title**: %s\n", event.Title))
	sb.WriteString(fmt.Sprintf("- **Date**: %s\n", event.StartTime.Format("2006-01-02 15:04")))
	if len(event.Attendees) > 0 {
		sb.WriteString(fmt.Sprintf("- **Attendees**: %s\n", strings.Join(event.Attendees, ", ")))
	}
	if event.ClassificationStatus == store.StatusClassified && event.ProjectID != nil {
		currentProject := projectNames[event.ProjectID.String()]
		if currentProject == "" {
			currentProject = event.ProjectID.String()
		}
		sb.WriteString(fmt.Sprintf("- **Current classification**: %s\n", currentProject))
	} else {
		sb.WriteString(fmt.Sprintf("- **Current status**: %s\n", event.ClassificationStatus))
	}
	if event.IsSkipped {
		sb.WriteString("- **Skipped**: Yes (excluded from time entries)\n")
	}
	sb.WriteString("\n")

	// Skip rules section (shown first since skip is evaluated before project)
	if len(result.SkipEvaluations) > 0 {
		sb.WriteString("## Skip Rules\n\n")
		if result.WouldBeSkipped {
			sb.WriteString(fmt.Sprintf("**This event would be marked as skipped** (%.0f%% confidence)\n\n", result.SkipConfidence*100))
		} else {
			sb.WriteString("*No skip rules matched this event*\n\n")
		}

		// Show matching skip rules
		var matchingSkip, nonMatchingSkip []classification.RuleEvaluation
		for _, e := range result.SkipEvaluations {
			if e.Matched {
				matchingSkip = append(matchingSkip, e)
			} else {
				nonMatchingSkip = append(nonMatchingSkip, e)
			}
		}

		if len(matchingSkip) > 0 {
			sb.WriteString("### Matching Skip Rules\n\n")
			sb.WriteString("| Query | Weight |\n")
			sb.WriteString("|-------|--------|\n")
			for _, e := range matchingSkip {
				sb.WriteString(fmt.Sprintf("| `%s` | %.1f |\n", e.Query, e.Weight))
			}
			sb.WriteString("\n")
		}

		if len(nonMatchingSkip) > 0 {
			sb.WriteString(fmt.Sprintf("### Non-Matching Skip Rules (%d)\n\n", len(nonMatchingSkip)))
			sb.WriteString("| Query |\n")
			sb.WriteString("|-------|\n")
			for i, e := range nonMatchingSkip {
				if i >= 5 {
					sb.WriteString(fmt.Sprintf("\n*... and %d more*\n", len(nonMatchingSkip)-i))
					break
				}
				sb.WriteString(fmt.Sprintf("| `%s` |\n", e.Query))
			}
			sb.WriteString("\n")
		}
	}

	// Outcome summary
	sb.WriteString("## Project Classification Result\n\n")
	sb.WriteString(fmt.Sprintf("**%s**\n\n", result.Outcome))

	// Target scores (projects that received votes)
	if len(result.TargetScores) > 0 {
		sb.WriteString("## Score by Project\n\n")
		sb.WriteString("| Project | Total | Rules | Fingerprints | Winner |\n")
		sb.WriteString("|---------|-------|-------|--------------|--------|\n")
		for _, ts := range result.TargetScores {
			name := projectNames[ts.TargetID]
			if name == "" {
				name = ts.TargetID
			}
			winner := ""
			if ts.IsWinner {
				winner = "✓"
			}
			sb.WriteString(fmt.Sprintf("| %s | %.1f | %.1f | %.1f | %s |\n",
				name, ts.TotalWeight, ts.RuleWeight, ts.FingerprintWeight, winner))
		}
		sb.WriteString(fmt.Sprintf("\n**Total weight**: %.1f | **Confidence**: %.0f%%\n\n",
			result.TotalWeight, result.WinnerConfidence*100))
	}

	// Matching rules
	var matchingRules []classification.RuleEvaluation
	var nonMatchingRules []classification.RuleEvaluation
	for _, e := range result.Evaluations {
		if e.Matched {
			matchingRules = append(matchingRules, e)
		} else {
			nonMatchingRules = append(nonMatchingRules, e)
		}
	}

	if len(matchingRules) > 0 {
		sb.WriteString("## Matching Rules\n\n")
		sb.WriteString("| Query | Project | Weight | Source |\n")
		sb.WriteString("|-------|---------|--------|--------|\n")
		for _, e := range matchingRules {
			name := projectNames[e.TargetID]
			if name == "" {
				name = e.TargetID
			}
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %.1f | %s |\n",
				e.Query, name, e.Weight, e.Source))
		}
		sb.WriteString("\n")
	}

	// Non-matching rules (collapsed by default - just show count)
	if len(nonMatchingRules) > 0 {
		sb.WriteString(fmt.Sprintf("## Non-Matching Rules (%d)\n\n", len(nonMatchingRules)))
		// Show first few for context
		shown := 0
		sb.WriteString("| Query | Project | Source |\n")
		sb.WriteString("|-------|---------|--------|\n")
		for _, e := range nonMatchingRules {
			if shown >= 10 {
				sb.WriteString(fmt.Sprintf("\n*... and %d more non-matching rules*\n", len(nonMatchingRules)-shown))
				break
			}
			name := projectNames[e.TargetID]
			if name == "" {
				name = e.TargetID
			}
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n",
				e.Query, name, e.Source))
			shown++
		}
		sb.WriteString("\n")
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

	// If not authenticated via middleware, check for Bearer token
	if !ok {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token := strings.TrimPrefix(authHeader, "Bearer ")

			// Check for MCP OAuth token (mcp_ prefix)
			if strings.HasPrefix(token, "mcp_") {
				if h.mcpOAuth != nil {
					if uid, err := h.mcpOAuth.ValidateToken(r.Context(), token); err == nil {
						userID = uid
						ok = true
					}
				}
			}

			// Check for API key (ts_ prefix)
			if !ok && strings.HasPrefix(token, "ts_") {
				if h.apiKeys != nil {
					if uid, err := h.apiKeys.ValidateAndGetUserID(r.Context(), token); err == nil {
						userID = uid
						ok = true
					}
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
		serverInfo := mcp.GetServerInfo()
		result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools":     map[string]any{},
				"resources": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    serverInfo.Name,
				"version": serverInfo.Version,
			},
			"instructions": serverInfo.Instructions,
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
