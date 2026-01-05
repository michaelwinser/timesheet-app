package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

// APIKeyHandler implements the API key management endpoints
type APIKeyHandler struct {
	apiKeys *store.APIKeyStore
}

// NewAPIKeyHandler creates a new API key handler
func NewAPIKeyHandler(apiKeys *store.APIKeyStore) *APIKeyHandler {
	return &APIKeyHandler{
		apiKeys: apiKeys,
	}
}

// ListApiKeys returns all API keys for the authenticated user
func (h *APIKeyHandler) ListApiKeys(ctx context.Context, req api.ListApiKeysRequestObject) (api.ListApiKeysResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ListApiKeys401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	keys, err := h.apiKeys.List(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]api.ApiKey, len(keys))
	for i, k := range keys {
		result[i] = api.ApiKey{
			Id:        k.ID,
			UserId:    k.UserID,
			Name:      k.Name,
			KeyPrefix: k.KeyPrefix,
			CreatedAt: k.CreatedAt,
		}
		if k.LastUsedAt != nil {
			result[i].LastUsedAt = k.LastUsedAt
		}
	}

	return api.ListApiKeys200JSONResponse(result), nil
}

// CreateApiKey creates a new API key
func (h *APIKeyHandler) CreateApiKey(ctx context.Context, req api.CreateApiKeyRequestObject) (api.CreateApiKeyResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.CreateApiKey401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	if req.Body == nil {
		return api.CreateApiKey400JSONResponse{
			Code:    "invalid_request",
			Message: "Request body is required",
		}, nil
	}

	name := strings.TrimSpace(req.Body.Name)
	if name == "" {
		return api.CreateApiKey400JSONResponse{
			Code:    "invalid_name",
			Message: "Name is required",
		}, nil
	}

	if len(name) > 255 {
		return api.CreateApiKey400JSONResponse{
			Code:    "invalid_name",
			Message: "Name must be 255 characters or less",
		}, nil
	}

	key, err := h.apiKeys.Create(ctx, userID, name)
	if err != nil {
		if errors.Is(err, store.ErrAPIKeyNameTaken) {
			return api.CreateApiKey409JSONResponse{
				Code:    "name_taken",
				Message: "An API key with this name already exists",
			}, nil
		}
		return nil, err
	}

	return api.CreateApiKey201JSONResponse{
		Id:        key.ID,
		UserId:    key.UserID,
		Name:      key.Name,
		KeyPrefix: key.KeyPrefix,
		Key:       key.Key,
		CreatedAt: key.CreatedAt,
	}, nil
}

// DeleteApiKey revokes an API key
func (h *APIKeyHandler) DeleteApiKey(ctx context.Context, req api.DeleteApiKeyRequestObject) (api.DeleteApiKeyResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.DeleteApiKey401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	err := h.apiKeys.Delete(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrAPIKeyNotFound) {
			return api.DeleteApiKey404JSONResponse{
				Code:    "not_found",
				Message: "API key not found",
			}, nil
		}
		return nil, err
	}

	return api.DeleteApiKey204Response{}, nil
}
