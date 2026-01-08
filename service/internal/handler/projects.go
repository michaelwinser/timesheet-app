package handler

import (
	"context"
	"errors"

	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

// ProjectHandler implements the project endpoints
type ProjectHandler struct {
	projects *store.ProjectStore
}

// NewProjectHandler creates a new project handler
func NewProjectHandler(projects *store.ProjectStore) *ProjectHandler {
	return &ProjectHandler{projects: projects}
}

// ListProjects returns all projects for the authenticated user
func (h *ProjectHandler) ListProjects(ctx context.Context, req api.ListProjectsRequestObject) (api.ListProjectsResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ListProjects401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	includeArchived := false
	if req.Params.IncludeArchived != nil {
		includeArchived = *req.Params.IncludeArchived
	}

	projects, err := h.projects.List(ctx, userID, includeArchived)
	if err != nil {
		return nil, err
	}

	result := make([]api.Project, len(projects))
	for i, p := range projects {
		result[i] = projectToAPI(p)
	}

	return api.ListProjects200JSONResponse(result), nil
}

// CreateProject creates a new project
func (h *ProjectHandler) CreateProject(ctx context.Context, req api.CreateProjectRequestObject) (api.CreateProjectResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.CreateProject401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	if req.Body == nil || req.Body.Name == "" {
		return api.CreateProject400JSONResponse{
			Code:    "invalid_request",
			Message: "Name is required",
		}, nil
	}

	color := "#6B7280"
	if req.Body.Color != nil {
		color = *req.Body.Color
	}

	isBillable := true
	if req.Body.IsBillable != nil {
		isBillable = *req.Body.IsBillable
	}

	isHiddenByDefault := false
	if req.Body.IsHiddenByDefault != nil {
		isHiddenByDefault = *req.Body.IsHiddenByDefault
	}

	doesNotAccumulateHours := false
	if req.Body.DoesNotAccumulateHours != nil {
		doesNotAccumulateHours = *req.Body.DoesNotAccumulateHours
	}

	project, err := h.projects.Create(ctx, userID, req.Body.Name, req.Body.ShortCode, req.Body.Client, color, isBillable, isHiddenByDefault, doesNotAccumulateHours)
	if err != nil {
		return nil, err
	}

	return api.CreateProject201JSONResponse(projectToAPI(project)), nil
}

// GetProject returns a project by ID
func (h *ProjectHandler) GetProject(ctx context.Context, req api.GetProjectRequestObject) (api.GetProjectResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.GetProject401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	project, err := h.projects.GetByID(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			return api.GetProject404JSONResponse{
				Code:    "not_found",
				Message: "Project not found",
			}, nil
		}
		return nil, err
	}

	return api.GetProject200JSONResponse(projectToAPI(project)), nil
}

// UpdateProject updates a project
func (h *ProjectHandler) UpdateProject(ctx context.Context, req api.UpdateProjectRequestObject) (api.UpdateProjectResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.UpdateProject401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	if req.Body == nil {
		return api.UpdateProject400JSONResponse{
			Code:    "invalid_request",
			Message: "Request body is required",
		}, nil
	}

	updates := make(map[string]interface{})
	if req.Body.Name != nil {
		updates["name"] = *req.Body.Name
	}
	if req.Body.ShortCode != nil {
		updates["short_code"] = *req.Body.ShortCode
	}
	if req.Body.Color != nil {
		updates["color"] = *req.Body.Color
	}
	if req.Body.IsBillable != nil {
		updates["is_billable"] = *req.Body.IsBillable
	}
	if req.Body.IsArchived != nil {
		updates["is_archived"] = *req.Body.IsArchived
	}
	if req.Body.IsHiddenByDefault != nil {
		updates["is_hidden_by_default"] = *req.Body.IsHiddenByDefault
	}
	if req.Body.DoesNotAccumulateHours != nil {
		updates["does_not_accumulate_hours"] = *req.Body.DoesNotAccumulateHours
	}
	if req.Body.FingerprintDomains != nil {
		updates["fingerprint_domains"] = *req.Body.FingerprintDomains
	}
	if req.Body.FingerprintEmails != nil {
		updates["fingerprint_emails"] = *req.Body.FingerprintEmails
	}
	if req.Body.FingerprintKeywords != nil {
		updates["fingerprint_keywords"] = *req.Body.FingerprintKeywords
	}
	if req.Body.Client != nil {
		updates["client"] = *req.Body.Client
	}

	project, err := h.projects.Update(ctx, userID, req.Id, updates)
	if err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			return api.UpdateProject404JSONResponse{
				Code:    "not_found",
				Message: "Project not found",
			}, nil
		}
		return nil, err
	}

	return api.UpdateProject200JSONResponse(projectToAPI(project)), nil
}

// DeleteProject deletes a project
func (h *ProjectHandler) DeleteProject(ctx context.Context, req api.DeleteProjectRequestObject) (api.DeleteProjectResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.DeleteProject401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	err := h.projects.Delete(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			return api.DeleteProject404JSONResponse{
				Code:    "not_found",
				Message: "Project not found",
			}, nil
		}
		if errors.Is(err, store.ErrProjectHasEntries) {
			return api.DeleteProject409JSONResponse{
				Code:    "conflict",
				Message: "Cannot delete project with time entries",
			}, nil
		}
		return nil, err
	}

	return api.DeleteProject204Response{}, nil
}

// projectToAPI converts a store.Project to an api.Project
func projectToAPI(p *store.Project) api.Project {
	proj := api.Project{
		Id:                     p.ID,
		UserId:                 p.UserID,
		Name:                   p.Name,
		Color:                  p.Color,
		IsBillable:             p.IsBillable,
		IsArchived:             p.IsArchived,
		CreatedAt:              p.CreatedAt,
		ShortCode:              p.ShortCode,
		Client:                 p.Client,
		IsHiddenByDefault:      &p.IsHiddenByDefault,
		DoesNotAccumulateHours: &p.DoesNotAccumulateHours,
		UpdatedAt:              &p.UpdatedAt,
	}
	if len(p.FingerprintDomains) > 0 {
		proj.FingerprintDomains = &p.FingerprintDomains
	}
	if len(p.FingerprintEmails) > 0 {
		proj.FingerprintEmails = &p.FingerprintEmails
	}
	if len(p.FingerprintKeywords) > 0 {
		proj.FingerprintKeywords = &p.FingerprintKeywords
	}
	return proj
}
