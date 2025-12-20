package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

// contextKey is used for context values
type contextKey string

const userIDKey contextKey = "userID"

// AuthHandler implements the auth endpoints
type AuthHandler struct {
	users *store.UserStore
	jwt   *JWTService
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(users *store.UserStore, jwt *JWTService) *AuthHandler {
	return &AuthHandler{
		users: users,
		jwt:   jwt,
	}
}

// Signup creates a new user account
func (h *AuthHandler) Signup(ctx context.Context, req api.SignupRequestObject) (api.SignupResponseObject, error) {
	// Validate request
	if req.Body == nil {
		return api.Signup400JSONResponse{
			Code:    "invalid_request",
			Message: "Request body is required",
		}, nil
	}

	email := string(req.Body.Email)
	if email == "" {
		return api.Signup400JSONResponse{
			Code:    "invalid_email",
			Message: "Email is required",
		}, nil
	}

	if len(req.Body.Password) < 8 {
		return api.Signup400JSONResponse{
			Code:    "invalid_password",
			Message: "Password must be at least 8 characters",
		}, nil
	}

	if strings.TrimSpace(req.Body.Name) == "" {
		return api.Signup400JSONResponse{
			Code:    "invalid_name",
			Message: "Name is required",
		}, nil
	}

	// Create user
	user, err := h.users.Create(email, req.Body.Name, req.Body.Password)
	if err != nil {
		if errors.Is(err, store.ErrEmailAlreadyTaken) {
			return api.Signup409JSONResponse{
				Code:    "email_taken",
				Message: "Email is already registered",
			}, nil
		}
		return nil, err
	}

	// Generate token
	token, err := h.jwt.GenerateToken(user.ID)
	if err != nil {
		return nil, err
	}

	return api.Signup201JSONResponse{
		Token: token,
		User: api.User{
			Id:        user.ID,
			Email:     user.Email,
			Name:      user.Name,
			CreatedAt: user.CreatedAt,
		},
	}, nil
}

// Login authenticates a user
func (h *AuthHandler) Login(ctx context.Context, req api.LoginRequestObject) (api.LoginResponseObject, error) {
	if req.Body == nil {
		return api.Login400JSONResponse{
			Code:    "invalid_request",
			Message: "Request body is required",
		}, nil
	}

	email := string(req.Body.Email)
	user, err := h.users.Authenticate(email, req.Body.Password)
	if err != nil {
		if errors.Is(err, store.ErrInvalidPassword) {
			return api.Login401JSONResponse{
				Code:    "invalid_credentials",
				Message: "Email or password is incorrect",
			}, nil
		}
		return nil, err
	}

	token, err := h.jwt.GenerateToken(user.ID)
	if err != nil {
		return nil, err
	}

	return api.Login200JSONResponse{
		Token: token,
		User: api.User{
			Id:        user.ID,
			Email:     user.Email,
			Name:      user.Name,
			CreatedAt: user.CreatedAt,
		},
	}, nil
}

// Logout ends the current session
func (h *AuthHandler) Logout(ctx context.Context, req api.LogoutRequestObject) (api.LogoutResponseObject, error) {
	// For JWT-based auth, logout is client-side (discard token)
	// In a real app, you might maintain a token blacklist
	return api.Logout204Response{}, nil
}

// GetCurrentUser returns the authenticated user's profile
func (h *AuthHandler) GetCurrentUser(ctx context.Context, req api.GetCurrentUserRequestObject) (api.GetCurrentUserResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.GetCurrentUser401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	user, err := h.users.GetByID(userID)
	if err != nil {
		return api.GetCurrentUser401JSONResponse{
			Code:    "unauthorized",
			Message: "User not found",
		}, nil
	}

	return api.GetCurrentUser200JSONResponse{
		Id:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		CreatedAt: user.CreatedAt,
	}, nil
}

// Ensure AuthHandler implements StrictServerInterface
var _ api.StrictServerInterface = (*AuthHandler)(nil)
