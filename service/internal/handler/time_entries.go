package handler

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/store"
	"github.com/michaelw/timesheet-app/service/internal/timeentry"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// TimeEntryHandler implements the time entry endpoints
type TimeEntryHandler struct {
	entries        *store.TimeEntryStore
	projects       *store.ProjectStore
	timeEntryService *timeentry.Service
}

// NewTimeEntryHandler creates a new time entry handler
func NewTimeEntryHandler(entries *store.TimeEntryStore, projects *store.ProjectStore, timeEntryService *timeentry.Service) *TimeEntryHandler {
	return &TimeEntryHandler{
		entries:        entries,
		projects:       projects,
		timeEntryService: timeEntryService,
	}
}

// ListTimeEntries returns time entries for the authenticated user.
// Combines materialized entries (with user state) and ephemeral entries
// (computed on-demand from classified events).
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

	// Use the service to get merged materialized + ephemeral entries
	entries, err := h.timeEntryService.ListWithEphemeral(ctx, userID, startDate, endDate, req.Params.ProjectId)
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

	// If hours not provided, try to auto-populate from events
	hours := float64(req.Body.Hours)
	description := req.Body.Description

	if req.Body.Hours == 0 {
		// Try to get computed values
		computed, err := h.timeEntryService.ComputeForProjectAndDate(ctx, userID, req.Body.ProjectId, date)
		if err != nil {
			return nil, err
		}
		if computed != nil {
			// Use computed hours and description
			hours = computed.Hours
			if description == nil || *description == "" {
				description = &computed.Description
			}
		}
	}

	entry, err := h.entries.Create(ctx, userID, req.Body.ProjectId, date, hours, description)
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

	// Get the existing entry to refresh computed values before updating
	// This ensures snapshot_computed_hours captures the fresh computed value
	existing, err := h.entries.GetByID(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrTimeEntryNotFound) {
			// Entry not found - check if this is an ephemeral entry that needs to be materialized
			if req.Body.ProjectId != nil && req.Body.Date != nil {
				// Materialize the ephemeral entry first
				existing, err = h.materializeEphemeralEntry(ctx, userID, *req.Body.ProjectId, req.Body.Date.Time)
				if err != nil {
					return nil, err
				}
				if existing == nil {
					return api.UpdateTimeEntry404JSONResponse{
						Code:    "not_found",
						Message: "No events found for this project and date",
					}, nil
				}
			} else {
				return api.UpdateTimeEntry404JSONResponse{
					Code:    "not_found",
					Message: "Time entry not found",
				}, nil
			}
		} else {
			return nil, err
		}
	}

	// Refresh computed values before updating so snapshot captures fresh values
	// This makes "Keep" correctly clear staleness by acknowledging the drift
	computed, err := h.timeEntryService.ComputeForProjectAndDate(ctx, userID, existing.ProjectID, existing.Date)
	if err != nil {
		return nil, err
	}
	if computed != nil {
		// Update computed values in DB (non-blocking if fails)
		_ = h.entries.RefreshComputedValues(ctx, userID, existing.ID, computed.Hours)
	}

	var hours *float64
	if req.Body.Hours != nil {
		hVal := float64(*req.Body.Hours)
		hours = &hVal
	}

	entry, err := h.entries.Update(ctx, userID, existing.ID, hours, req.Body.Description)
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

// RefreshTimeEntry resets a time entry to computed values from events
func (h *TimeEntryHandler) RefreshTimeEntry(ctx context.Context, req api.RefreshTimeEntryRequestObject) (api.RefreshTimeEntryResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.RefreshTimeEntry401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	// Get the existing entry to check if it's invoiced
	entry, err := h.entries.GetByID(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrTimeEntryNotFound) {
			return api.RefreshTimeEntry404JSONResponse{
				Code:    "not_found",
				Message: "Time entry not found",
			}, nil
		}
		return nil, err
	}

	// Cannot refresh invoiced entries
	if entry.InvoiceID != nil {
		return api.RefreshTimeEntry400JSONResponse{
			Code:    "invalid_operation",
			Message: "Cannot refresh invoiced time entry",
		}, nil
	}

	// Compute fresh values from events
	computed, err := h.timeEntryService.ComputeForProjectAndDate(ctx, userID, entry.ProjectID, entry.Date)
	if err != nil {
		return nil, err
	}

	// If no computed values (no events), return error
	if computed == nil {
		return api.RefreshTimeEntry400JSONResponse{
			Code:    "no_events",
			Message: "No classified events found for this date and project",
		}, nil
	}

	// Marshal calculation details
	detailsJSON, err := json.Marshal(computed.CalculationDetails)
	if err != nil {
		return nil, err
	}

	// Reset to computed values via store
	refreshed, err := h.entries.ResetToComputed(
		ctx,
		userID,
		req.Id,
		computed.Hours,
		computed.Title,
		computed.Description,
		detailsJSON,
		computed.ContributingEvents,
	)
	if err != nil {
		if errors.Is(err, store.ErrTimeEntryNotFound) {
			return api.RefreshTimeEntry404JSONResponse{
				Code:    "not_found",
				Message: "Time entry not found",
			}, nil
		}
		return nil, err
	}

	return api.RefreshTimeEntry200JSONResponse(timeEntryToAPI(refreshed)), nil
}

// materializeEphemeralEntry creates a time entry in the database for an ephemeral entry.
// This is called when updating an ephemeral entry that doesn't exist in the DB yet.
func (h *TimeEntryHandler) materializeEphemeralEntry(ctx context.Context, userID, projectID openapi_types.UUID, date time.Time) (*store.TimeEntry, error) {
	// Compute fresh values from events
	computed, err := h.timeEntryService.ComputeForProjectAndDate(ctx, userID, projectID, date)
	if err != nil {
		return nil, err
	}
	if computed == nil {
		return nil, nil
	}

	// Marshal calculation details
	detailsJSON, err := json.Marshal(computed.CalculationDetails)
	if err != nil {
		return nil, err
	}

	// Create the entry using UpsertFromComputed
	entry, err := h.entries.UpsertFromComputed(
		ctx,
		userID,
		projectID,
		date,
		computed.Hours,
		computed.Title,
		computed.Description,
		detailsJSON,
		computed.ContributingEvents,
	)
	if err != nil {
		return nil, err
	}

	return entry, nil
}

// computeStale determines if a time entry is stale based on the ephemeral model formula:
// stale = materialized AND (hours != computed_hours) AND (computed_hours != snapshot_computed_hours)
//
// This means:
// - Entry must be materialized (has snapshot_computed_hours)
// - User hours differ from current computed hours
// - Computed hours have drifted since materialization
func computeStale(e *store.TimeEntry) bool {
	// Not materialized = not stale
	if e.SnapshotComputedHours == nil {
		return false
	}
	// No computed hours = can't determine staleness
	if e.ComputedHours == nil {
		return false
	}
	// User hours match computed = not stale
	if e.Hours == *e.ComputedHours {
		return false
	}
	// Computed hasn't drifted from snapshot = not stale (user intentionally differs)
	if *e.ComputedHours == *e.SnapshotComputedHours {
		return false
	}
	// All conditions met: materialized, user differs, and computed has drifted
	return true
}

// timeEntryToAPI converts a store.TimeEntry to an api.TimeEntry
func timeEntryToAPI(e *store.TimeEntry) api.TimeEntry {
	// Compute staleness using the ephemeral model formula
	isStale := computeStale(e)

	entry := api.TimeEntry{
		Id:           e.ID,
		UserId:       e.UserID,
		ProjectId:    e.ProjectID,
		Date:         openapi_types.Date{Time: e.Date},
		Hours:        float32(e.Hours),
		Title:        e.Title,
		Source:       api.TimeEntrySource(e.Source),
		CreatedAt:    e.CreatedAt,
		Description:  e.Description,
		InvoiceId:    e.InvoiceID,
		HasUserEdits: &e.HasUserEdits,
		UpdatedAt:    &e.UpdatedAt,
		// Protection model fields
		IsPinned:     &e.IsPinned,
		IsLocked:     &e.IsLocked,
		IsStale:      &isStale, // Computed, not from DB
		IsSuppressed: &e.IsSuppressed,
	}

	// Computed fields
	if e.ComputedHours != nil {
		hours := float32(*e.ComputedHours)
		entry.ComputedHours = &hours
	}
	if e.SnapshotComputedHours != nil {
		hours := float32(*e.SnapshotComputedHours)
		entry.SnapshotComputedHours = &hours
	}
	entry.ComputedTitle = e.ComputedTitle
	entry.ComputedDescription = e.ComputedDescription

	// Calculation details (stored as JSON bytes)
	if len(e.CalculationDetails) > 0 {
		var details api.CalculationDetails
		if err := json.Unmarshal(e.CalculationDetails, &details); err == nil {
			entry.CalculationDetails = &details
		}
	}

	// Contributing events
	if len(e.ContributingEvents) > 0 {
		entry.ContributingEvents = &e.ContributingEvents
	}

	if e.Project != nil {
		proj := projectToAPI(e.Project)
		entry.Project = &proj
	}

	return entry
}
