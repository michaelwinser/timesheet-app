package google

import (
	"context"
	"time"

	"github.com/michaelw/timesheet-app/service/internal/store"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// CalendarClient defines the interface for Google Calendar API operations.
// This interface enables mocking for testing.
type CalendarClient interface {
	// GetAuthURL returns the OAuth consent URL
	GetAuthURL(state string) string

	// ExchangeCode exchanges an authorization code for tokens
	ExchangeCode(ctx context.Context, code string) (*store.OAuthCredentials, error)

	// RefreshToken refreshes an expired token
	RefreshToken(ctx context.Context, creds *store.OAuthCredentials) (*store.OAuthCredentials, error)

	// ListCalendars returns all calendars the user has access to
	ListCalendars(ctx context.Context, creds *store.OAuthCredentials) ([]*CalendarInfo, error)

	// FetchEvents fetches calendar events for the given time range (full sync)
	FetchEvents(ctx context.Context, creds *store.OAuthCredentials, calendarID string, minTime, maxTime time.Time) (*SyncResult, error)

	// FetchEventsIncremental fetches only changed events since the last sync
	FetchEventsIncremental(ctx context.Context, creds *store.OAuthCredentials, calendarID string, syncToken string) (*SyncResult, error)
}

// Ensure CalendarService implements CalendarClient
var _ CalendarClient = (*CalendarService)(nil)

// CalendarService handles Google Calendar API interactions
type CalendarService struct {
	config *oauth2.Config
}

// NewCalendarService creates a new Google Calendar service
func NewCalendarService(clientID, clientSecret, redirectURL string) *CalendarService {
	return &CalendarService{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{calendar.CalendarReadonlyScope, drive.DriveFileScope},
			Endpoint:     google.Endpoint,
		},
	}
}

// GetAuthURL returns the OAuth consent URL
func (s *CalendarService) GetAuthURL(state string) string {
	return s.config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

// ExchangeCode exchanges an authorization code for tokens
func (s *CalendarService) ExchangeCode(ctx context.Context, code string) (*store.OAuthCredentials, error) {
	token, err := s.config.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}

	return &store.OAuthCredentials{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
	}, nil
}

// RefreshToken refreshes an expired token
func (s *CalendarService) RefreshToken(ctx context.Context, creds *store.OAuthCredentials) (*store.OAuthCredentials, error) {
	token := &oauth2.Token{
		AccessToken:  creds.AccessToken,
		RefreshToken: creds.RefreshToken,
		TokenType:    creds.TokenType,
		Expiry:       creds.Expiry,
	}

	src := s.config.TokenSource(ctx, token)
	newToken, err := src.Token()
	if err != nil {
		return nil, err
	}

	return &store.OAuthCredentials{
		AccessToken:  newToken.AccessToken,
		RefreshToken: newToken.RefreshToken,
		TokenType:    newToken.TokenType,
		Expiry:       newToken.Expiry,
	}, nil
}

// SyncResult contains the result of a calendar sync operation
type SyncResult struct {
	Events        []*calendar.Event
	NextSyncToken string
	FullSync      bool // True if this was a full sync (not incremental)
}

// CalendarInfo contains metadata about a Google Calendar
type CalendarInfo struct {
	ID          string // Calendar ID (e.g., "primary", "user@example.com")
	Name        string // Display name
	Description string
	Color       string // Background color
	IsPrimary   bool
}

// ListCalendars returns all calendars the user has access to
func (s *CalendarService) ListCalendars(ctx context.Context, creds *store.OAuthCredentials) ([]*CalendarInfo, error) {
	srv, err := s.getService(ctx, creds)
	if err != nil {
		return nil, err
	}

	list, err := srv.CalendarList.List().Do()
	if err != nil {
		return nil, err
	}

	var calendars []*CalendarInfo
	for _, item := range list.Items {
		calendars = append(calendars, &CalendarInfo{
			ID:          item.Id,
			Name:        item.Summary,
			Description: item.Description,
			Color:       item.BackgroundColor,
			IsPrimary:   item.Primary,
		})
	}

	return calendars, nil
}

// FetchEvents fetches calendar events for the given time range (full sync)
func (s *CalendarService) FetchEvents(ctx context.Context, creds *store.OAuthCredentials, calendarID string, minTime, maxTime time.Time) (*SyncResult, error) {
	srv, err := s.getService(ctx, creds)
	if err != nil {
		return nil, err
	}

	var allEvents []*calendar.Event
	pageToken := ""
	var syncToken string

	for {
		call := srv.Events.List(calendarID).
			TimeMin(minTime.Format(time.RFC3339)).
			TimeMax(maxTime.Format(time.RFC3339)).
			SingleEvents(true).
			OrderBy("startTime").
			MaxResults(250)

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		events, err := call.Do()
		if err != nil {
			return nil, err
		}

		allEvents = append(allEvents, events.Items...)

		pageToken = events.NextPageToken
		syncToken = events.NextSyncToken
		if pageToken == "" {
			break
		}
	}

	return &SyncResult{
		Events:        allEvents,
		NextSyncToken: syncToken,
		FullSync:      true,
	}, nil
}

// FetchEventsIncremental fetches only changed events since the last sync
func (s *CalendarService) FetchEventsIncremental(ctx context.Context, creds *store.OAuthCredentials, calendarID string, syncToken string) (*SyncResult, error) {
	srv, err := s.getService(ctx, creds)
	if err != nil {
		return nil, err
	}

	var allEvents []*calendar.Event
	pageToken := ""
	var nextSyncToken string

	for {
		call := srv.Events.List(calendarID).
			SyncToken(syncToken).
			MaxResults(250)

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		events, err := call.Do()
		if err != nil {
			// If sync token is invalid (410 Gone), return error so caller can do full sync
			return nil, err
		}

		allEvents = append(allEvents, events.Items...)

		pageToken = events.NextPageToken
		nextSyncToken = events.NextSyncToken
		if pageToken == "" {
			break
		}
	}

	return &SyncResult{
		Events:        allEvents,
		NextSyncToken: nextSyncToken,
		FullSync:      false,
	}, nil
}

func (s *CalendarService) getService(ctx context.Context, creds *store.OAuthCredentials) (*calendar.Service, error) {
	token := &oauth2.Token{
		AccessToken:  creds.AccessToken,
		RefreshToken: creds.RefreshToken,
		TokenType:    creds.TokenType,
		Expiry:       creds.Expiry,
	}

	client := s.config.Client(ctx, token)
	return calendar.NewService(ctx, option.WithHTTPClient(client))
}
