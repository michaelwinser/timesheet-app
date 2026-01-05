package handler

import (
	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/classification"
	"github.com/michaelw/timesheet-app/service/internal/google"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

// Server implements the full StrictServerInterface
type Server struct {
	*AuthHandler
	*ProjectHandler
	*TimeEntryHandler
	*CalendarHandler
	*RulesHandler
	*APIKeyHandler
}

// NewServer creates a new server handler
func NewServer(
	users *store.UserStore,
	projects *store.ProjectStore,
	entries *store.TimeEntryStore,
	calendarConns *store.CalendarConnectionStore,
	calendars *store.CalendarStore,
	calendarEvents *store.CalendarEventStore,
	classificationRules *store.ClassificationRuleStore,
	apiKeys *store.APIKeyStore,
	jwt *JWTService,
	googleSvc *google.CalendarService,
	classificationSvc *classification.Service,
) *Server {
	return &Server{
		AuthHandler:      NewAuthHandler(users, jwt),
		ProjectHandler:   NewProjectHandler(projects),
		TimeEntryHandler: NewTimeEntryHandler(entries, projects),
		CalendarHandler:  NewCalendarHandler(calendarConns, calendars, calendarEvents, entries, projects, googleSvc, classificationSvc),
		RulesHandler:     NewRulesHandler(classificationRules, projects, classificationSvc),
		APIKeyHandler:    NewAPIKeyHandler(apiKeys),
	}
}

// Ensure Server implements StrictServerInterface
var _ api.StrictServerInterface = (*Server)(nil)
