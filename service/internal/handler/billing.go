package handler

import (
	"context"
	"errors"
	"time"

	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/store"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// BillingHandler implements the billing period endpoints
type BillingHandler struct {
	periods *store.BillingPeriodStore
}

// NewBillingHandler creates a new billing handler
func NewBillingHandler(periods *store.BillingPeriodStore) *BillingHandler {
	return &BillingHandler{periods: periods}
}

// ListBillingPeriods returns all billing periods for a project
func (h *BillingHandler) ListBillingPeriods(ctx context.Context, req api.ListBillingPeriodsRequestObject) (api.ListBillingPeriodsResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ListBillingPeriods401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	periods, err := h.periods.ListByProject(ctx, userID, req.Params.ProjectId)
	if err != nil {
		return nil, err
	}

	result := make([]api.BillingPeriod, len(periods))
	for i, p := range periods {
		result[i] = billingPeriodToAPI(p)
	}

	return api.ListBillingPeriods200JSONResponse(result), nil
}

// CreateBillingPeriod creates a new billing period
func (h *BillingHandler) CreateBillingPeriod(ctx context.Context, req api.CreateBillingPeriodRequestObject) (api.CreateBillingPeriodResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.CreateBillingPeriod401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	if req.Body == nil {
		return api.CreateBillingPeriod400JSONResponse{
			Code:    "invalid_request",
			Message: "Request body required",
		}, nil
	}

	startsOn := req.Body.StartsOn.Time

	var endsOn *time.Time
	if req.Body.EndsOn != nil {
		t := req.Body.EndsOn.Time
		endsOn = &t
	}

	period, err := h.periods.Create(ctx, userID, req.Body.ProjectId, startsOn, endsOn, float64(req.Body.HourlyRate))
	if err != nil {
		if errors.Is(err, store.ErrBillingPeriodOverlap) {
			return api.CreateBillingPeriod409JSONResponse{
				Code:    "overlap",
				Message: "Billing period overlaps with existing period",
			}, nil
		}
		return nil, err
	}

	return api.CreateBillingPeriod201JSONResponse(billingPeriodToAPI(period)), nil
}

// UpdateBillingPeriod updates an existing billing period
func (h *BillingHandler) UpdateBillingPeriod(ctx context.Context, req api.UpdateBillingPeriodRequestObject) (api.UpdateBillingPeriodResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.UpdateBillingPeriod401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	if req.Body == nil {
		return api.UpdateBillingPeriod400JSONResponse{
			Code:    "invalid_request",
			Message: "Request body required",
		}, nil
	}

	updates := make(map[string]interface{})

	if req.Body.StartsOn != nil {
		updates["starts_on"] = req.Body.StartsOn.Time
	}

	if req.Body.EndsOn != nil {
		// Empty Date means clear the end date (make it ongoing)
		if req.Body.EndsOn.IsZero() {
			updates["ends_on"] = nil
		} else {
			updates["ends_on"] = req.Body.EndsOn.Time
		}
	}

	if req.Body.HourlyRate != nil {
		updates["hourly_rate"] = float64(*req.Body.HourlyRate)
	}

	if len(updates) == 0 {
		return api.UpdateBillingPeriod400JSONResponse{
			Code:    "invalid_request",
			Message: "No updates provided",
		}, nil
	}

	period, err := h.periods.Update(ctx, userID, req.Id, updates)
	if err != nil {
		if errors.Is(err, store.ErrBillingPeriodNotFound) {
			return api.UpdateBillingPeriod404JSONResponse{
				Code:    "not_found",
				Message: "Billing period not found",
			}, nil
		}
		if errors.Is(err, store.ErrBillingPeriodOverlap) {
			return api.UpdateBillingPeriod409JSONResponse{
				Code:    "overlap",
				Message: "Updated period would overlap with existing period",
			}, nil
		}
		return nil, err
	}

	return api.UpdateBillingPeriod200JSONResponse(billingPeriodToAPI(period)), nil
}

// DeleteBillingPeriod deletes a billing period
func (h *BillingHandler) DeleteBillingPeriod(ctx context.Context, req api.DeleteBillingPeriodRequestObject) (api.DeleteBillingPeriodResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.DeleteBillingPeriod401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	err := h.periods.Delete(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrBillingPeriodNotFound) {
			return api.DeleteBillingPeriod404JSONResponse{
				Code:    "not_found",
				Message: "Billing period not found",
			}, nil
		}
		return nil, err
	}

	return api.DeleteBillingPeriod204Response{}, nil
}

// billingPeriodToAPI converts a store BillingPeriod to an API BillingPeriod
func billingPeriodToAPI(p *store.BillingPeriod) api.BillingPeriod {
	period := api.BillingPeriod{
		Id:         p.ID,
		UserId:     p.UserID,
		ProjectId:  p.ProjectID,
		StartsOn:   openapi_types.Date{Time: p.StartsOn},
		HourlyRate: float32(p.HourlyRate),
		CreatedAt:  p.CreatedAt,
	}
	if p.EndsOn != nil {
		period.EndsOn = &openapi_types.Date{Time: *p.EndsOn}
	}
	updatedAt := p.UpdatedAt
	period.UpdatedAt = &updatedAt
	return period
}
