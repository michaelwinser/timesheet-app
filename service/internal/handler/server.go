package handler

import (
	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/google"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

// Server implements the full StrictServerInterface
type Server struct {
	*AuthHandler
	*ProjectHandler
	*TimeEntryHandler
	*CalendarHandler
}

// NewServer creates a new server handler
func NewServer(
	users *store.UserStore,
	projects *store.ProjectStore,
	entries *store.TimeEntryStore,
	calendarConns *store.CalendarConnectionStore,
	calendars *store.CalendarStore,
	calendarEvents *store.CalendarEventStore,
	jwt *JWTService,
	googleSvc *google.CalendarService,
) *Server {
	return &Server{
		AuthHandler:      NewAuthHandler(users, jwt),
		ProjectHandler:   NewProjectHandler(projects),
		TimeEntryHandler: NewTimeEntryHandler(entries, projects),
		CalendarHandler:  NewCalendarHandler(calendarConns, calendars, calendarEvents, entries, googleSvc),
	}
}

// Ensure Server implements StrictServerInterface
var _ api.StrictServerInterface = (*Server)(nil)
