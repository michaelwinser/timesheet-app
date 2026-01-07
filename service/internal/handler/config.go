package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/classification"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

const configExportVersion = "1"

// ConfigHandler implements the config import/export endpoints
type ConfigHandler struct {
	projects *store.ProjectStore
	rules    *store.ClassificationRuleStore
}

// NewConfigHandler creates a new config handler
func NewConfigHandler(projects *store.ProjectStore, rules *store.ClassificationRuleStore) *ConfigHandler {
	return &ConfigHandler{
		projects: projects,
		rules:    rules,
	}
}

// ExportConfig exports all projects and rules as JSON
func (h *ConfigHandler) ExportConfig(ctx context.Context, req api.ExportConfigRequestObject) (api.ExportConfigResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ExportConfig401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	includeArchived := false
	if req.Params.IncludeArchived != nil {
		includeArchived = *req.Params.IncludeArchived
	}

	// Fetch projects
	projects, err := h.projects.List(ctx, userID, includeArchived)
	if err != nil {
		return nil, err
	}

	// Fetch rules (include disabled rules to get a complete export)
	rules, err := h.rules.List(ctx, userID, true)
	if err != nil {
		return nil, err
	}

	// Build project name lookup for rules
	projectNames := make(map[string]string) // ID -> name
	for _, p := range projects {
		projectNames[p.ID.String()] = p.Name
	}

	// Convert projects to export format
	projectExports := make([]api.ProjectExport, len(projects))
	for i, p := range projects {
		projectExports[i] = projectToExport(p)
	}

	// Convert rules to export format
	ruleExports := make([]api.RuleExport, 0, len(rules))
	for _, r := range rules {
		ruleExport := ruleToExport(r, projectNames)
		ruleExports = append(ruleExports, ruleExport)
	}

	return api.ExportConfig200JSONResponse{
		Version:    configExportVersion,
		ExportedAt: time.Now().UTC(),
		Projects:   projectExports,
		Rules:      ruleExports,
	}, nil
}

// ImportConfig imports projects and rules from JSON
func (h *ConfigHandler) ImportConfig(ctx context.Context, req api.ImportConfigRequestObject) (api.ImportConfigResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ImportConfig401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	if req.Body == nil {
		return api.ImportConfig400JSONResponse{
			Code:    "invalid_request",
			Message: "Request body required",
		}, nil
	}

	var warnings []string
	var projectsCreated, projectsUpdated int
	var rulesCreated, rulesUpdated, rulesSkipped int

	// Get existing projects to check for updates vs creates
	existingProjects, err := h.projects.List(ctx, userID, true)
	if err != nil {
		return nil, err
	}

	existingProjectsByName := make(map[string]*store.Project)
	for _, p := range existingProjects {
		existingProjectsByName[p.Name] = p
	}

	// Track project name -> ID mapping for rules
	projectIDsByName := make(map[string]string)
	for _, p := range existingProjects {
		projectIDsByName[p.Name] = p.ID.String()
	}

	// Import projects
	for _, pExport := range req.Body.Projects {
		if existing, ok := existingProjectsByName[pExport.Name]; ok {
			// Update existing project
			updates := projectExportToUpdates(&pExport)
			_, err := h.projects.Update(ctx, userID, existing.ID, updates)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Failed to update project %q: %v", pExport.Name, err))
				continue
			}
			projectsUpdated++
		} else {
			// Create new project
			color := "#6B7280"
			if pExport.Color != nil {
				color = *pExport.Color
			}

			isBillable := true
			if pExport.IsBillable != nil {
				isBillable = *pExport.IsBillable
			}

			isHiddenByDefault := false
			if pExport.IsHiddenByDefault != nil {
				isHiddenByDefault = *pExport.IsHiddenByDefault
			}

			doesNotAccumulateHours := false
			if pExport.DoesNotAccumulateHours != nil {
				doesNotAccumulateHours = *pExport.DoesNotAccumulateHours
			}

			newProject, err := h.projects.Create(ctx, userID, pExport.Name, pExport.ShortCode, color, isBillable, isHiddenByDefault, doesNotAccumulateHours)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Failed to create project %q: %v", pExport.Name, err))
				continue
			}

			// Apply additional fields that aren't in Create
			updates := projectExportToUpdates(&pExport)
			if len(updates) > 0 {
				_, err = h.projects.Update(ctx, userID, newProject.ID, updates)
				if err != nil {
					warnings = append(warnings, fmt.Sprintf("Failed to update new project %q: %v", pExport.Name, err))
				}
			}

			projectIDsByName[pExport.Name] = newProject.ID.String()
			projectsCreated++
		}
	}

	// Refresh project list after imports
	existingProjects, err = h.projects.List(ctx, userID, true)
	if err != nil {
		return nil, err
	}
	projectIDsByName = make(map[string]string)
	for _, p := range existingProjects {
		projectIDsByName[p.Name] = p.ID.String()
	}

	// Get existing rules
	existingRules, err := h.rules.List(ctx, userID, true)
	if err != nil {
		return nil, err
	}

	// Index existing rules by query for matching
	existingRulesByQuery := make(map[string]*store.ClassificationRule)
	for _, r := range existingRules {
		existingRulesByQuery[r.Query] = r
	}

	// Import rules
	for _, rExport := range req.Body.Rules {
		// Validate query syntax
		if _, err := classification.Parse(rExport.Query); err != nil {
			warnings = append(warnings, fmt.Sprintf("Invalid rule query %q: %v", rExport.Query, err))
			rulesSkipped++
			continue
		}

		// Determine if this is a skip rule or project rule
		isSkipRule := rExport.Skip != nil && *rExport.Skip

		var projectIDStr string
		if !isSkipRule {
			if rExport.ProjectName == nil || *rExport.ProjectName == "" {
				warnings = append(warnings, fmt.Sprintf("Rule %q has no project_name and is not a skip rule", rExport.Query))
				rulesSkipped++
				continue
			}
			projectIDStr = projectIDsByName[*rExport.ProjectName]
			if projectIDStr == "" {
				warnings = append(warnings, fmt.Sprintf("Rule %q references unknown project %q", rExport.Query, *rExport.ProjectName))
				rulesSkipped++
				continue
			}
		}

		if existing, ok := existingRulesByQuery[rExport.Query]; ok {
			// Update existing rule
			if rExport.Weight != nil {
				existing.Weight = float64(*rExport.Weight)
			}
			if rExport.IsEnabled != nil {
				existing.IsEnabled = *rExport.IsEnabled
			}
			if isSkipRule {
				attended := false
				existing.Attended = &attended
				existing.ProjectID = nil
			} else if projectIDStr != "" {
				projectID, _ := uuid.Parse(projectIDStr)
				existing.ProjectID = &projectID
				existing.Attended = nil
			}

			_, err := h.rules.Update(ctx, existing)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Failed to update rule %q: %v", rExport.Query, err))
				continue
			}
			rulesUpdated++
		} else {
			// Create new rule
			weight := float64(1.0)
			if rExport.Weight != nil {
				weight = float64(*rExport.Weight)
			}

			isEnabled := true
			if rExport.IsEnabled != nil {
				isEnabled = *rExport.IsEnabled
			}

			newRule := &store.ClassificationRule{
				UserID:    userID,
				Query:     rExport.Query,
				Weight:    weight,
				IsEnabled: isEnabled,
			}

			if isSkipRule {
				attended := false
				newRule.Attended = &attended
			} else if projectIDStr != "" {
				projectID, _ := uuid.Parse(projectIDStr)
				newRule.ProjectID = &projectID
			}

			_, err := h.rules.Create(ctx, newRule)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Failed to create rule %q: %v", rExport.Query, err))
				continue
			}
			rulesCreated++
		}
	}

	result := api.ConfigImportResult{
		ProjectsCreated: projectsCreated,
		ProjectsUpdated: projectsUpdated,
		RulesCreated:    rulesCreated,
		RulesUpdated:    rulesUpdated,
	}

	if rulesSkipped > 0 {
		result.RulesSkipped = &rulesSkipped
	}

	if len(warnings) > 0 {
		result.Warnings = &warnings
	}

	return api.ImportConfig200JSONResponse(result), nil
}

// projectToExport converts a store.Project to api.ProjectExport
func projectToExport(p *store.Project) api.ProjectExport {
	export := api.ProjectExport{
		Name:                   p.Name,
		ShortCode:              p.ShortCode,
		Client:                 p.Client,
		Color:                  &p.Color,
		IsBillable:             &p.IsBillable,
		IsArchived:             &p.IsArchived,
		IsHiddenByDefault:      &p.IsHiddenByDefault,
		DoesNotAccumulateHours: &p.DoesNotAccumulateHours,
	}

	if len(p.FingerprintDomains) > 0 {
		export.FingerprintDomains = &p.FingerprintDomains
	}
	if len(p.FingerprintEmails) > 0 {
		export.FingerprintEmails = &p.FingerprintEmails
	}
	if len(p.FingerprintKeywords) > 0 {
		export.FingerprintKeywords = &p.FingerprintKeywords
	}

	return export
}

// ruleToExport converts a store.ClassificationRule to api.RuleExport
func ruleToExport(r *store.ClassificationRule, projectNames map[string]string) api.RuleExport {
	export := api.RuleExport{
		Query:     r.Query,
		Weight:    ptrFloat32(float32(r.Weight)),
		IsEnabled: &r.IsEnabled,
	}

	// If it's a skip rule (attended=false), mark it as skip
	if r.Attended != nil && !*r.Attended {
		skip := true
		export.Skip = &skip
	} else if r.ProjectID != nil {
		// It's a project rule - look up the project name
		if name, ok := projectNames[r.ProjectID.String()]; ok {
			export.ProjectName = &name
		}
	}

	return export
}

// projectExportToUpdates converts a ProjectExport to a map of updates
func projectExportToUpdates(p *api.ProjectExport) map[string]interface{} {
	updates := make(map[string]interface{})

	if p.ShortCode != nil {
		updates["short_code"] = *p.ShortCode
	}
	if p.Client != nil {
		updates["client"] = *p.Client
	}
	if p.Color != nil {
		updates["color"] = *p.Color
	}
	if p.IsBillable != nil {
		updates["is_billable"] = *p.IsBillable
	}
	if p.IsArchived != nil {
		updates["is_archived"] = *p.IsArchived
	}
	if p.IsHiddenByDefault != nil {
		updates["is_hidden_by_default"] = *p.IsHiddenByDefault
	}
	if p.DoesNotAccumulateHours != nil {
		updates["does_not_accumulate_hours"] = *p.DoesNotAccumulateHours
	}
	if p.FingerprintDomains != nil {
		updates["fingerprint_domains"] = *p.FingerprintDomains
	}
	if p.FingerprintEmails != nil {
		updates["fingerprint_emails"] = *p.FingerprintEmails
	}
	if p.FingerprintKeywords != nil {
		updates["fingerprint_keywords"] = *p.FingerprintKeywords
	}

	return updates
}

// ptrFloat32 returns a pointer to the given float32
func ptrFloat32(f float32) *float32 {
	return &f
}
