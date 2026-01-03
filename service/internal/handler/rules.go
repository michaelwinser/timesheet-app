package handler

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/classification"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

// RulesHandler implements the classification rules endpoints
type RulesHandler struct {
	rules              *store.ClassificationRuleStore
	classificationSvc  *classification.Service
}

// NewRulesHandler creates a new rules handler
func NewRulesHandler(
	rules *store.ClassificationRuleStore,
	classificationSvc *classification.Service,
) *RulesHandler {
	return &RulesHandler{
		rules:             rules,
		classificationSvc: classificationSvc,
	}
}

// ListRules returns all classification rules for the authenticated user
func (h *RulesHandler) ListRules(ctx context.Context, req api.ListRulesRequestObject) (api.ListRulesResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ListRules401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	includeDisabled := false
	if req.Params.IncludeDisabled != nil {
		includeDisabled = *req.Params.IncludeDisabled
	}

	rules, err := h.rules.List(ctx, userID, includeDisabled)
	if err != nil {
		return nil, err
	}

	result := make([]api.ClassificationRule, len(rules))
	for i, r := range rules {
		result[i] = ruleToAPI(r)
	}

	return api.ListRules200JSONResponse(result), nil
}

// CreateRule creates a new classification rule
func (h *RulesHandler) CreateRule(ctx context.Context, req api.CreateRuleRequestObject) (api.CreateRuleResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.CreateRule401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	if req.Body == nil || req.Body.Query == "" {
		return api.CreateRule400JSONResponse{
			Code:    "invalid_request",
			Message: "Query is required",
		}, nil
	}

	// Validate query syntax
	if _, err := classification.Parse(req.Body.Query); err != nil {
		return api.CreateRule400JSONResponse{
			Code:    "invalid_query",
			Message: "Invalid query syntax: " + err.Error(),
		}, nil
	}

	// Validate that either project_id or attended is set
	if req.Body.ProjectId == nil && req.Body.Attended == nil {
		return api.CreateRule400JSONResponse{
			Code:    "invalid_request",
			Message: "Either project_id or attended must be set",
		}, nil
	}

	weight := float64(1.0)
	if req.Body.Weight != nil {
		weight = float64(*req.Body.Weight)
	}

	isEnabled := true
	if req.Body.IsEnabled != nil {
		isEnabled = *req.Body.IsEnabled
	}

	var projectID *uuid.UUID
	if req.Body.ProjectId != nil {
		id := uuid.UUID(*req.Body.ProjectId)
		projectID = &id
	}

	rule := &store.ClassificationRule{
		UserID:    userID,
		Query:     req.Body.Query,
		ProjectID: projectID,
		Attended:  req.Body.Attended,
		Weight:    weight,
		IsEnabled: isEnabled,
	}

	created, err := h.rules.Create(ctx, rule)
	if err != nil {
		return nil, err
	}

	return api.CreateRule201JSONResponse(ruleToAPI(created)), nil
}

// GetRule returns a rule by ID
func (h *RulesHandler) GetRule(ctx context.Context, req api.GetRuleRequestObject) (api.GetRuleResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.GetRule401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	rule, err := h.rules.GetByID(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrClassificationRuleNotFound) {
			return api.GetRule404JSONResponse{
				Code:    "not_found",
				Message: "Rule not found",
			}, nil
		}
		return nil, err
	}

	return api.GetRule200JSONResponse(ruleToAPI(rule)), nil
}

// UpdateRule updates a rule
func (h *RulesHandler) UpdateRule(ctx context.Context, req api.UpdateRuleRequestObject) (api.UpdateRuleResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.UpdateRule401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	if req.Body == nil {
		return api.UpdateRule400JSONResponse{
			Code:    "invalid_request",
			Message: "Request body is required",
		}, nil
	}

	// Get existing rule
	existing, err := h.rules.GetByID(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrClassificationRuleNotFound) {
			return api.UpdateRule404JSONResponse{
				Code:    "not_found",
				Message: "Rule not found",
			}, nil
		}
		return nil, err
	}

	// Apply updates
	if req.Body.Query != nil {
		// Validate query syntax
		if _, err := classification.Parse(*req.Body.Query); err != nil {
			return api.UpdateRule400JSONResponse{
				Code:    "invalid_query",
				Message: "Invalid query syntax: " + err.Error(),
			}, nil
		}
		existing.Query = *req.Body.Query
	}

	if req.Body.ProjectId != nil {
		id := uuid.UUID(*req.Body.ProjectId)
		existing.ProjectID = &id
	}

	if req.Body.Attended != nil {
		existing.Attended = req.Body.Attended
	}

	if req.Body.Weight != nil {
		existing.Weight = float64(*req.Body.Weight)
	}

	if req.Body.IsEnabled != nil {
		existing.IsEnabled = *req.Body.IsEnabled
	}

	updated, err := h.rules.Update(ctx, existing)
	if err != nil {
		if errors.Is(err, store.ErrClassificationRuleNotFound) {
			return api.UpdateRule404JSONResponse{
				Code:    "not_found",
				Message: "Rule not found",
			}, nil
		}
		return nil, err
	}

	return api.UpdateRule200JSONResponse(ruleToAPI(updated)), nil
}

// DeleteRule deletes a rule
func (h *RulesHandler) DeleteRule(ctx context.Context, req api.DeleteRuleRequestObject) (api.DeleteRuleResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.DeleteRule401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	err := h.rules.Delete(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrClassificationRuleNotFound) {
			return api.DeleteRule404JSONResponse{
				Code:    "not_found",
				Message: "Rule not found",
			}, nil
		}
		return nil, err
	}

	return api.DeleteRule204Response{}, nil
}

// PreviewRule evaluates a query against events and returns matching events
func (h *RulesHandler) PreviewRule(ctx context.Context, req api.PreviewRuleRequestObject) (api.PreviewRuleResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.PreviewRule401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	if req.Body == nil || req.Body.Query == "" {
		return api.PreviewRule400JSONResponse{
			Code:    "invalid_request",
			Message: "Query is required",
		}, nil
	}

	// Validate query syntax
	if _, err := classification.Parse(req.Body.Query); err != nil {
		return api.PreviewRule400JSONResponse{
			Code:    "invalid_query",
			Message: "Invalid query syntax: " + err.Error(),
		}, nil
	}

	var projectID *uuid.UUID
	if req.Body.ProjectId != nil {
		id := uuid.UUID(*req.Body.ProjectId)
		projectID = &id
	}

	var startDate, endDate *time.Time
	if req.Body.StartDate != nil {
		t := req.Body.StartDate.Time
		startDate = &t
	}
	if req.Body.EndDate != nil {
		t := req.Body.EndDate.Time
		endDate = &t
	}

	preview, err := h.classificationSvc.PreviewRule(ctx, userID, req.Body.Query, projectID, startDate, endDate)
	if err != nil {
		return nil, err
	}

	// Convert to API types
	matches := make([]api.MatchedEvent, len(preview.Matches))
	for i, m := range preview.Matches {
		matches[i] = api.MatchedEvent{
			EventId:   m.EventID,
			Title:     m.Title,
			StartTime: m.StartTime,
		}
	}

	conflicts := make([]api.RuleConflict, len(preview.Conflicts))
	for i, c := range preview.Conflicts {
		conflicts[i] = api.RuleConflict{
			EventId:           c.EventID,
			CurrentProjectId:  c.CurrentProjectID,
			CurrentSource:     &c.CurrentSource,
			ProposedProjectId: c.ProposedProject,
		}
	}

	return api.PreviewRule200JSONResponse{
		Matches:   matches,
		Conflicts: conflicts,
		Stats: api.PreviewStats{
			TotalMatches:    preview.Stats.TotalMatches,
			AlreadyCorrect:  preview.Stats.AlreadyCorrect,
			WouldChange:     preview.Stats.WouldChange,
			ManualConflicts: preview.Stats.ManualConflicts,
		},
	}, nil
}

// ApplyRules runs classification rules on pending events
func (h *RulesHandler) ApplyRules(ctx context.Context, req api.ApplyRulesRequestObject) (api.ApplyRulesResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ApplyRules401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	var startDate, endDate *time.Time
	if req.Body != nil {
		if req.Body.StartDate != nil {
			t := req.Body.StartDate.Time
			startDate = &t
		}
		if req.Body.EndDate != nil {
			t := req.Body.EndDate.Time
			endDate = &t
		}
	}

	dryRun := false
	if req.Body != nil && req.Body.DryRun != nil {
		dryRun = *req.Body.DryRun
	}

	result, err := h.classificationSvc.ApplyRules(ctx, userID, startDate, endDate, dryRun)
	if err != nil {
		return nil, err
	}

	// Convert to API types
	classified := make([]api.ClassifiedEvent, len(result.Classified))
	for i, c := range result.Classified {
		classified[i] = api.ClassifiedEvent{
			EventId:     c.EventID,
			ProjectId:   c.ProjectID,
			Confidence:  float32(c.Confidence),
			NeedsReview: c.NeedsReview,
		}
	}

	return api.ApplyRules200JSONResponse{
		Classified: classified,
		Skipped:    result.Skipped,
	}, nil
}

// ruleToAPI converts a store.ClassificationRule to an api.ClassificationRule
func ruleToAPI(r *store.ClassificationRule) api.ClassificationRule {
	rule := api.ClassificationRule{
		Id:        r.ID,
		UserId:    r.UserID,
		Query:     r.Query,
		Weight:    float32(r.Weight),
		IsEnabled: r.IsEnabled,
		CreatedAt: r.CreatedAt,
		UpdatedAt: &r.UpdatedAt,
	}

	if r.ProjectID != nil {
		rule.ProjectId = r.ProjectID
	}

	if r.Attended != nil {
		rule.Attended = r.Attended
	}

	if r.ProjectName != nil {
		rule.ProjectName = r.ProjectName
	}

	if r.ProjectColor != nil {
		rule.ProjectColor = r.ProjectColor
	}

	return rule
}
