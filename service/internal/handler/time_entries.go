package handler

import (
	"context"
	"errors"
	"time"

	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/store"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// TimeEntryHandler implements the time entry endpoints
type TimeEntryHandler struct {
	entries  *store.TimeEntryStore
	projects *store.ProjectStore
}

// NewTimeEntryHandler creates a new time entry handler
func NewTimeEntryHandler(entries *store.TimeEntryStore, projects *store.ProjectStore) *TimeEntryHandler {
	return &TimeEntryHandler{
		entries:  entries,
		projects: projects,
	}
}

// ListTimeEntries returns time entries for the authenticated user
func (h *TimeEntryHandler) ListTimeEntries(ctx context.Context, req api.ListTimeEntriesRequestObject) (api.ListTimeEntriesResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ListTimeEntries401JSONResponse{
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

	entries, err := h.entries.List(ctx, userID, startDate, endDate, req.Params.ProjectId)
	if err != nil {
		return nil, err
	}

	result := make([]api.TimeEntry, len(entries))
	for i, e := range entries {
		result[i] = timeEntryToAPI(e)
	}

	return api.ListTimeEntries200JSONResponse(result), nil
}

// CreateTimeEntry creates a new time entry
func (h *TimeEntryHandler) CreateTimeEntry(ctx context.Context, req api.CreateTimeEntryRequestObject) (api.CreateTimeEntryResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.CreateTimeEntry401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	if req.Body == nil {
		return api.CreateTimeEntry400JSONResponse{
			Code:    "invalid_request",
			Message: "Request body is required",
		}, nil
	}

	// Verify project exists and belongs to user
	_, err := h.projects.GetByID(ctx, userID, req.Body.ProjectId)
	if err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			return api.CreateTimeEntry404JSONResponse{
				Code:    "not_found",
				Message: "Project not found",
			}, nil
		}
		return nil, err
	}

	date := req.Body.Date.Time
	entry, err := h.entries.Create(ctx, userID, req.Body.ProjectId, date, float64(req.Body.Hours), req.Body.Description)
	if err != nil {
		return nil, err
	}

	return api.CreateTimeEntry201JSONResponse(timeEntryToAPI(entry)), nil
}

// GetTimeEntry returns a time entry by ID
func (h *TimeEntryHandler) GetTimeEntry(ctx context.Context, req api.GetTimeEntryRequestObject) (api.GetTimeEntryResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.GetTimeEntry401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	entry, err := h.entries.GetByID(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrTimeEntryNotFound) {
			return api.GetTimeEntry404JSONResponse{
				Code:    "not_found",
				Message: "Time entry not found",
			}, nil
		}
		return nil, err
	}

	return api.GetTimeEntry200JSONResponse(timeEntryToAPI(entry)), nil
}

// UpdateTimeEntry updates a time entry
func (h *TimeEntryHandler) UpdateTimeEntry(ctx context.Context, req api.UpdateTimeEntryRequestObject) (api.UpdateTimeEntryResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.UpdateTimeEntry401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	if req.Body == nil {
		return api.UpdateTimeEntry400JSONResponse{
			Code:    "invalid_request",
			Message: "Request body is required",
		}, nil
	}

	var hours *float64
	if req.Body.Hours != nil {
		h := float64(*req.Body.Hours)
		hours = &h
	}

	entry, err := h.entries.Update(ctx, userID, req.Id, hours, req.Body.Description)
	if err != nil {
		if errors.Is(err, store.ErrTimeEntryNotFound) {
			return api.UpdateTimeEntry404JSONResponse{
				Code:    "not_found",
				Message: "Time entry not found",
			}, nil
		}
		if errors.Is(err, store.ErrTimeEntryInvoiced) {
			return api.UpdateTimeEntry409JSONResponse{
				Code:    "conflict",
				Message: "Cannot edit invoiced time entry",
			}, nil
		}
		return nil, err
	}

	return api.UpdateTimeEntry200JSONResponse(timeEntryToAPI(entry)), nil
}

// DeleteTimeEntry deletes a time entry
func (h *TimeEntryHandler) DeleteTimeEntry(ctx context.Context, req api.DeleteTimeEntryRequestObject) (api.DeleteTimeEntryResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.DeleteTimeEntry401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	err := h.entries.Delete(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrTimeEntryNotFound) {
			return api.DeleteTimeEntry404JSONResponse{
				Code:    "not_found",
				Message: "Time entry not found",
			}, nil
		}
		if errors.Is(err, store.ErrTimeEntryInvoiced) {
			return api.DeleteTimeEntry409JSONResponse{
				Code:    "conflict",
				Message: "Cannot delete invoiced time entry",
			}, nil
		}
		return nil, err
	}

	return api.DeleteTimeEntry204Response{}, nil
}

// timeEntryToAPI converts a store.TimeEntry to an api.TimeEntry
func timeEntryToAPI(e *store.TimeEntry) api.TimeEntry {
	entry := api.TimeEntry{
		Id:           e.ID,
		UserId:       e.UserID,
		ProjectId:    e.ProjectID,
		Date:         openapi_types.Date{Time: e.Date},
		Hours:        float32(e.Hours),
		Source:       api.TimeEntrySource(e.Source),
		CreatedAt:    e.CreatedAt,
		Description:  e.Description,
		InvoiceId:    e.InvoiceID,
		HasUserEdits: &e.HasUserEdits,
		UpdatedAt:    &e.UpdatedAt,
	}

	if e.Project != nil {
		proj := projectToAPI(e.Project)
		entry.Project = &proj
	}

	return entry
}
