package handler

import (
	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/classification"
	"github.com/michaelw/timesheet-app/service/internal/google"
	"github.com/michaelw/timesheet-app/service/internal/store"
	"github.com/michaelw/timesheet-app/service/internal/timeentry"
)

// Server implements the full StrictServerInterface
type Server struct {
	*AuthHandler
	*ProjectHandler
	*TimeEntryHandler
	*CalendarHandler
	*RulesHandler
	*APIKeyHandler
	*BillingHandler
	*InvoiceHandler
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
	billingPeriods *store.BillingPeriodStore,
	invoices *store.InvoiceStore,
	jwt *JWTService,
	googleSvc *google.CalendarService,
	sheetsSvc *google.SheetsService,
	classificationSvc *classification.Service,
	timeEntrySvc *timeentry.Service,
) *Server {
	return &Server{
		AuthHandler:      NewAuthHandler(users, jwt),
		ProjectHandler:   NewProjectHandler(projects),
		TimeEntryHandler: NewTimeEntryHandler(entries, projects, timeEntrySvc),
		CalendarHandler:  NewCalendarHandler(calendarConns, calendars, calendarEvents, entries, projects, googleSvc, classificationSvc),
		RulesHandler:     NewRulesHandler(classificationRules, projects, classificationSvc),
		APIKeyHandler:    NewAPIKeyHandler(apiKeys),
		BillingHandler:   NewBillingHandler(billingPeriods),
		InvoiceHandler:   NewInvoiceHandler(invoices, projects, sheetsSvc, calendarConns),
	}
}

// Ensure Server implements StrictServerInterface
var _ api.StrictServerInterface = (*Server)(nil)
