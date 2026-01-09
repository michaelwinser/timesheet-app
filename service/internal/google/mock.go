package google

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/michaelw/timesheet-app/service/internal/store"
	"google.golang.org/api/calendar/v3"
)

// MockCalendarClient is a mock implementation of CalendarClient for testing.
type MockCalendarClient struct {
	mu sync.Mutex

	// AuthURL is the URL returned by GetAuthURL
	AuthURL string

	// Credentials returned by ExchangeCode
	ExchangeCredentials *store.OAuthCredentials
	ExchangeError       error

	// Credentials returned by RefreshToken
	RefreshCredentials *store.OAuthCredentials
	RefreshError       error

	// Calendars returned by ListCalendars
	Calendars     []*CalendarInfo
	CalendarsError error

	// EventsByRange maps "calendarID:YYYY-MM-DD:YYYY-MM-DD" to events
	EventsByRange map[string]*SyncResult
	FetchError    error

	// EventsByToken maps "calendarID:syncToken" to events
	EventsByToken    map[string]*SyncResult
	IncrementalError error

	// Call tracking
	ExchangeCalls     []string          // codes passed to ExchangeCode
	RefreshCalls      int               // number of RefreshToken calls
	ListCalendarCalls int               // number of ListCalendars calls
	FetchCalls        []FetchCall       // calls to FetchEvents
	IncrementalCalls  []IncrementalCall // calls to FetchEventsIncremental
}

// FetchCall records a call to FetchEvents
type FetchCall struct {
	CalendarID string
	MinTime    time.Time
	MaxTime    time.Time
}

// IncrementalCall records a call to FetchEventsIncremental
type IncrementalCall struct {
	CalendarID string
	SyncToken  string
}

// NewMockCalendarClient creates a new mock with sensible defaults
func NewMockCalendarClient() *MockCalendarClient {
	return &MockCalendarClient{
		AuthURL:       "https://accounts.google.com/mock-auth",
		EventsByRange: make(map[string]*SyncResult),
		EventsByToken: make(map[string]*SyncResult),
		ExchangeCredentials: &store.OAuthCredentials{
			AccessToken:  "mock-access-token",
			RefreshToken: "mock-refresh-token",
			TokenType:    "Bearer",
			Expiry:       time.Now().Add(1 * time.Hour),
		},
		RefreshCredentials: &store.OAuthCredentials{
			AccessToken:  "mock-refreshed-access-token",
			RefreshToken: "mock-refresh-token",
			TokenType:    "Bearer",
			Expiry:       time.Now().Add(1 * time.Hour),
		},
	}
}

// Ensure MockCalendarClient implements CalendarClient
var _ CalendarClient = (*MockCalendarClient)(nil)

// GetAuthURL returns the mock auth URL
func (m *MockCalendarClient) GetAuthURL(state string) string {
	return m.AuthURL + "?state=" + state
}

// ExchangeCode returns mock credentials or error
func (m *MockCalendarClient) ExchangeCode(ctx context.Context, code string) (*store.OAuthCredentials, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ExchangeCalls = append(m.ExchangeCalls, code)

	if m.ExchangeError != nil {
		return nil, m.ExchangeError
	}
	return m.ExchangeCredentials, nil
}

// RefreshToken returns mock refreshed credentials or error
func (m *MockCalendarClient) RefreshToken(ctx context.Context, creds *store.OAuthCredentials) (*store.OAuthCredentials, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.RefreshCalls++

	if m.RefreshError != nil {
		return nil, m.RefreshError
	}
	return m.RefreshCredentials, nil
}

// ListCalendars returns mock calendars or error
func (m *MockCalendarClient) ListCalendars(ctx context.Context, creds *store.OAuthCredentials) ([]*CalendarInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ListCalendarCalls++

	if m.CalendarsError != nil {
		return nil, m.CalendarsError
	}
	return m.Calendars, nil
}

// FetchEvents returns mock events for the given date range
func (m *MockCalendarClient) FetchEvents(ctx context.Context, creds *store.OAuthCredentials, calendarID string, minTime, maxTime time.Time) (*SyncResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.FetchCalls = append(m.FetchCalls, FetchCall{
		CalendarID: calendarID,
		MinTime:    minTime,
		MaxTime:    maxTime,
	})

	if m.FetchError != nil {
		return nil, m.FetchError
	}

	// Look up events by range key
	key := fmt.Sprintf("%s:%s:%s", calendarID, minTime.Format("2006-01-02"), maxTime.Format("2006-01-02"))
	if result, ok := m.EventsByRange[key]; ok {
		return result, nil
	}

	// Return empty result if no mock data configured
	return &SyncResult{
		Events:        []*calendar.Event{},
		NextSyncToken: "mock-sync-token-" + calendarID,
		FullSync:      true,
	}, nil
}

// FetchEventsIncremental returns mock incremental sync results
func (m *MockCalendarClient) FetchEventsIncremental(ctx context.Context, creds *store.OAuthCredentials, calendarID string, syncToken string) (*SyncResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.IncrementalCalls = append(m.IncrementalCalls, IncrementalCall{
		CalendarID: calendarID,
		SyncToken:  syncToken,
	})

	if m.IncrementalError != nil {
		return nil, m.IncrementalError
	}

	// Look up events by token key
	key := fmt.Sprintf("%s:%s", calendarID, syncToken)
	if result, ok := m.EventsByToken[key]; ok {
		return result, nil
	}

	// Return empty result if no mock data configured
	return &SyncResult{
		Events:        []*calendar.Event{},
		NextSyncToken: "mock-sync-token-new-" + calendarID,
		FullSync:      false,
	}, nil
}

// SetEventsForRange configures mock events for a specific date range
func (m *MockCalendarClient) SetEventsForRange(calendarID string, minTime, maxTime time.Time, events []*calendar.Event, nextToken string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s:%s", calendarID, minTime.Format("2006-01-02"), maxTime.Format("2006-01-02"))
	m.EventsByRange[key] = &SyncResult{
		Events:        events,
		NextSyncToken: nextToken,
		FullSync:      true,
	}
}

// SetEventsForToken configures mock events for a specific sync token
func (m *MockCalendarClient) SetEventsForToken(calendarID, syncToken string, events []*calendar.Event, nextToken string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := fmt.Sprintf("%s:%s", calendarID, syncToken)
	m.EventsByToken[key] = &SyncResult{
		Events:        events,
		NextSyncToken: nextToken,
		FullSync:      false,
	}
}

// Reset clears all call tracking
func (m *MockCalendarClient) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.ExchangeCalls = nil
	m.RefreshCalls = 0
	m.ListCalendarCalls = 0
	m.FetchCalls = nil
	m.IncrementalCalls = nil
}
