package handler

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/classification"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

// IngestionFilterHandler manages the filters that hide input noise before it
// reaches classification. See issue #110.
type IngestionFilterHandler struct {
	filters           *store.IngestionFilterStore
	events            *store.CalendarEventStore
	classificationSvc *classification.Service
}

func NewIngestionFilterHandler(
	filters *store.IngestionFilterStore,
	events *store.CalendarEventStore,
	classificationSvc *classification.Service,
) *IngestionFilterHandler {
	return &IngestionFilterHandler{filters: filters, events: events, classificationSvc: classificationSvc}
}

func ingestionFilterToAPI(f *store.IngestionFilter) api.IngestionFilter {
	return api.IngestionFilter{
		Id:        f.ID,
		Name:      f.Name,
		Query:     f.Query,
		IsEnabled: f.IsEnabled,
		CreatedAt: f.CreatedAt,
		UpdatedAt: f.UpdatedAt,
	}
}

// reevaluate re-applies the user's enabled filters across all stored events, so
// that adding, editing, disabling or deleting a filter takes effect on existing
// events rather than only on the next sync.
func (h *IngestionFilterHandler) reevaluate(ctx context.Context, userID uuid.UUID) (api.SuppressionEffect, error) {
	stored, err := h.filters.List(ctx, userID, true)
	if err != nil {
		return api.SuppressionEffect{}, err
	}

	filters := make([]classification.IngestionFilter, 0, len(stored))
	for _, f := range stored {
		filters = append(filters, classification.IngestionFilter{
			ID: f.ID.String(), Name: f.Name, Query: f.Query,
		})
	}

	result, err := h.classificationSvc.ReevaluateSuppression(ctx, userID, filters)
	if err != nil {
		return api.SuppressionEffect{}, err
	}

	effect := api.SuppressionEffect{
		Evaluated:  result.Evaluated,
		NowHidden:  result.NowHidden,
		NowVisible: result.NowVisible,
	}
	if len(result.InvalidRules) > 0 {
		names := make([]string, 0, len(result.InvalidRules))
		for _, f := range result.InvalidRules {
			names = append(names, f.Name)
		}
		effect.InvalidFilters = &names
	}
	return effect, nil
}

// ListIngestionFilters returns the user's filters.
func (h *IngestionFilterHandler) ListIngestionFilters(ctx context.Context, req api.ListIngestionFiltersRequestObject) (api.ListIngestionFiltersResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ListIngestionFilters401JSONResponse{Code: "unauthorized", Message: "Authentication required"}, nil
	}

	stored, err := h.filters.List(ctx, userID, false)
	if err != nil {
		return nil, err
	}

	result := make([]api.IngestionFilter, len(stored))
	for i, f := range stored {
		result[i] = ingestionFilterToAPI(f)
	}
	return api.ListIngestionFilters200JSONResponse(result), nil
}

// CreateIngestionFilter validates the query, stores the filter, and applies it
// to existing events.
func (h *IngestionFilterHandler) CreateIngestionFilter(ctx context.Context, req api.CreateIngestionFilterRequestObject) (api.CreateIngestionFilterResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.CreateIngestionFilter401JSONResponse{Code: "unauthorized", Message: "Authentication required"}, nil
	}
	if req.Body == nil || req.Body.Name == "" || req.Body.Query == "" {
		return api.CreateIngestionFilter400JSONResponse{Code: "invalid_request", Message: "Name and query are required"}, nil
	}
	if _, err := classification.Parse(req.Body.Query); err != nil {
		return api.CreateIngestionFilter400JSONResponse{Code: "invalid_query", Message: "Invalid query syntax: " + err.Error()}, nil
	}

	isEnabled := true
	if req.Body.IsEnabled != nil {
		isEnabled = *req.Body.IsEnabled
	}

	created, err := h.filters.Create(ctx, &store.IngestionFilter{
		UserID: userID, Name: req.Body.Name, Query: req.Body.Query, IsEnabled: isEnabled,
	})
	if err != nil {
		return nil, err
	}

	effect, err := h.reevaluate(ctx, userID)
	if err != nil {
		return nil, err
	}

	return api.CreateIngestionFilter201JSONResponse{
		Filter: ingestionFilterToAPI(created),
		Effect: effect,
	}, nil
}

// UpdateIngestionFilter applies changes and re-evaluates suppression, so
// disabling a filter restores what it was hiding.
func (h *IngestionFilterHandler) UpdateIngestionFilter(ctx context.Context, req api.UpdateIngestionFilterRequestObject) (api.UpdateIngestionFilterResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.UpdateIngestionFilter401JSONResponse{Code: "unauthorized", Message: "Authentication required"}, nil
	}
	if req.Body == nil {
		return api.UpdateIngestionFilter400JSONResponse{Code: "invalid_request", Message: "Request body is required"}, nil
	}
	if req.Body.Query != nil {
		if *req.Body.Query == "" {
			return api.UpdateIngestionFilter400JSONResponse{Code: "invalid_request", Message: "Query cannot be empty"}, nil
		}
		if _, err := classification.Parse(*req.Body.Query); err != nil {
			return api.UpdateIngestionFilter400JSONResponse{Code: "invalid_query", Message: "Invalid query syntax: " + err.Error()}, nil
		}
	}

	updated, err := h.filters.Update(ctx, userID, req.Id, req.Body.Name, req.Body.Query, req.Body.IsEnabled)
	if errors.Is(err, store.ErrIngestionFilterNotFound) {
		return api.UpdateIngestionFilter404JSONResponse{Code: "not_found", Message: "Filter not found"}, nil
	}
	if err != nil {
		return nil, err
	}

	effect, err := h.reevaluate(ctx, userID)
	if err != nil {
		return nil, err
	}

	return api.UpdateIngestionFilter200JSONResponse{
		Filter: ingestionFilterToAPI(updated),
		Effect: effect,
	}, nil
}

// DeleteIngestionFilter removes a filter and restores the events it hid, unless
// another filter also matches them.
func (h *IngestionFilterHandler) DeleteIngestionFilter(ctx context.Context, req api.DeleteIngestionFilterRequestObject) (api.DeleteIngestionFilterResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.DeleteIngestionFilter401JSONResponse{Code: "unauthorized", Message: "Authentication required"}, nil
	}

	if err := h.filters.Delete(ctx, userID, req.Id); errors.Is(err, store.ErrIngestionFilterNotFound) {
		return api.DeleteIngestionFilter404JSONResponse{Code: "not_found", Message: "Filter not found"}, nil
	} else if err != nil {
		return nil, err
	}

	effect, err := h.reevaluate(ctx, userID)
	if err != nil {
		return nil, err
	}
	return api.DeleteIngestionFilter200JSONResponse(effect), nil
}

// ListSuppressedEvents shows what the filters are currently hiding. Without it,
// an over-broad filter would be invisible.
func (h *IngestionFilterHandler) ListSuppressedEvents(ctx context.Context, req api.ListSuppressedEventsRequestObject) (api.ListSuppressedEventsResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ListSuppressedEvents401JSONResponse{Code: "unauthorized", Message: "Authentication required"}, nil
	}

	limit := 50
	if req.Params.Limit != nil && *req.Params.Limit > 0 {
		limit = *req.Params.Limit
	}

	events, total, err := h.events.ListSuppressed(ctx, userID, limit)
	if err != nil {
		return nil, err
	}

	result := make([]api.SuppressedEvent, len(events))
	for i, e := range events {
		result[i] = api.SuppressedEvent{Id: e.ID, Title: e.Title, StartTime: e.StartTime}
	}

	return api.ListSuppressedEvents200JSONResponse{Total: total, Events: result}, nil
}
