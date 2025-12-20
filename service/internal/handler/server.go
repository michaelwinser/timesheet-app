package handler

import (
	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

// Server implements the full StrictServerInterface
type Server struct {
	*AuthHandler
	*ProjectHandler
	*TimeEntryHandler
}

// NewServer creates a new server handler
func NewServer(
	users *store.UserStore,
	projects *store.ProjectStore,
	entries *store.TimeEntryStore,
	jwt *JWTService,
) *Server {
	return &Server{
		AuthHandler:      NewAuthHandler(users, jwt),
		ProjectHandler:   NewProjectHandler(projects),
		TimeEntryHandler: NewTimeEntryHandler(entries, projects),
	}
}

// Ensure Server implements StrictServerInterface
var _ api.StrictServerInterface = (*Server)(nil)
