package google

import (
	"context"
	"testing"
	"time"

	"github.com/michaelw/timesheet-app/service/internal/store"
	"google.golang.org/api/calendar/v3"
)

func TestMockCalendarClient_GetAuthURL(t *testing.T) {
	mock := NewMockCalendarClient()
	url := mock.GetAuthURL("test-state")

	expected := "https://accounts.google.com/mock-auth?state=test-state"
	if url != expected {
		t.Errorf("GetAuthURL() = %q, want %q", url, expected)
	}
}

func TestMockCalendarClient_ExchangeCode(t *testing.T) {
	ctx := context.Background()
	mock := NewMockCalendarClient()

	// Test successful exchange
	creds, err := mock.ExchangeCode(ctx, "test-code")
	if err != nil {
		t.Fatalf("ExchangeCode() error = %v", err)
	}
	if creds.AccessToken != "mock-access-token" {
		t.Errorf("ExchangeCode() access token = %q, want %q", creds.AccessToken, "mock-access-token")
	}
	if len(mock.ExchangeCalls) != 1 || mock.ExchangeCalls[0] != "test-code" {
		t.Error("Exchange call was not tracked")
	}

	// Test error case
	mock.ExchangeError = errMock
	_, err = mock.ExchangeCode(ctx, "test-code-2")
	if err == nil {
		t.Error("Expected error when ExchangeError is set")
	}
}

func TestMockCalendarClient_RefreshToken(t *testing.T) {
	ctx := context.Background()
	mock := NewMockCalendarClient()
	creds := &store.OAuthCredentials{RefreshToken: "old-token"}

	// Test successful refresh
	newCreds, err := mock.RefreshToken(ctx, creds)
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if newCreds.AccessToken != "mock-refreshed-access-token" {
		t.Errorf("RefreshToken() access token = %q, want %q", newCreds.AccessToken, "mock-refreshed-access-token")
	}
	if mock.RefreshCalls != 1 {
		t.Errorf("RefreshCalls = %d, want 1", mock.RefreshCalls)
	}

	// Test error case
	mock.RefreshError = errMock
	_, err = mock.RefreshToken(ctx, creds)
	if err == nil {
		t.Error("Expected error when RefreshError is set")
	}
}

func TestMockCalendarClient_ListCalendars(t *testing.T) {
	ctx := context.Background()
	mock := NewMockCalendarClient()
	creds := &store.OAuthCredentials{}

	// Set up mock calendars
	mock.Calendars = []*CalendarInfo{
		{ID: "primary", Name: "Primary Calendar", IsPrimary: true},
		{ID: "secondary", Name: "Work Calendar", IsPrimary: false},
	}

	// Test successful list
	calendars, err := mock.ListCalendars(ctx, creds)
	if err != nil {
		t.Fatalf("ListCalendars() error = %v", err)
	}
	if len(calendars) != 2 {
		t.Errorf("ListCalendars() returned %d calendars, want 2", len(calendars))
	}
	if mock.ListCalendarCalls != 1 {
		t.Errorf("ListCalendarCalls = %d, want 1", mock.ListCalendarCalls)
	}

	// Test error case
	mock.CalendarsError = errMock
	_, err = mock.ListCalendars(ctx, creds)
	if err == nil {
		t.Error("Expected error when CalendarsError is set")
	}
}

func TestMockCalendarClient_FetchEvents(t *testing.T) {
	ctx := context.Background()
	mock := NewMockCalendarClient()
	creds := &store.OAuthCredentials{}
	calendarID := "test-calendar"
	minTime := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	maxTime := time.Date(2025, 1, 12, 0, 0, 0, 0, time.UTC)

	// Set up mock events for this range
	testEvents := []*calendar.Event{
		{Id: "event1", Summary: "Test Event 1"},
		{Id: "event2", Summary: "Test Event 2"},
	}
	mock.SetEventsForRange(calendarID, minTime, maxTime, testEvents, "sync-token-1")

	// Test successful fetch
	result, err := mock.FetchEvents(ctx, creds, calendarID, minTime, maxTime)
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}
	if len(result.Events) != 2 {
		t.Errorf("FetchEvents() returned %d events, want 2", len(result.Events))
	}
	if result.NextSyncToken != "sync-token-1" {
		t.Errorf("FetchEvents() sync token = %q, want %q", result.NextSyncToken, "sync-token-1")
	}
	if !result.FullSync {
		t.Error("FetchEvents() should indicate full sync")
	}
	if len(mock.FetchCalls) != 1 {
		t.Errorf("FetchCalls length = %d, want 1", len(mock.FetchCalls))
	}

	// Test fetch with no configured events returns empty result
	result, err = mock.FetchEvents(ctx, creds, "other-calendar", minTime, maxTime)
	if err != nil {
		t.Fatalf("FetchEvents() error = %v", err)
	}
	if len(result.Events) != 0 {
		t.Errorf("FetchEvents() for unconfigured calendar returned %d events, want 0", len(result.Events))
	}

	// Test error case
	mock.FetchError = errMock
	_, err = mock.FetchEvents(ctx, creds, calendarID, minTime, maxTime)
	if err == nil {
		t.Error("Expected error when FetchError is set")
	}
}

func TestMockCalendarClient_FetchEventsIncremental(t *testing.T) {
	ctx := context.Background()
	mock := NewMockCalendarClient()
	creds := &store.OAuthCredentials{}
	calendarID := "test-calendar"
	syncToken := "old-sync-token"

	// Set up mock events for this token
	testEvents := []*calendar.Event{
		{Id: "event3", Summary: "Updated Event"},
	}
	mock.SetEventsForToken(calendarID, syncToken, testEvents, "new-sync-token")

	// Test successful incremental fetch
	result, err := mock.FetchEventsIncremental(ctx, creds, calendarID, syncToken)
	if err != nil {
		t.Fatalf("FetchEventsIncremental() error = %v", err)
	}
	if len(result.Events) != 1 {
		t.Errorf("FetchEventsIncremental() returned %d events, want 1", len(result.Events))
	}
	if result.NextSyncToken != "new-sync-token" {
		t.Errorf("FetchEventsIncremental() sync token = %q, want %q", result.NextSyncToken, "new-sync-token")
	}
	if result.FullSync {
		t.Error("FetchEventsIncremental() should not indicate full sync")
	}
	if len(mock.IncrementalCalls) != 1 {
		t.Errorf("IncrementalCalls length = %d, want 1", len(mock.IncrementalCalls))
	}
	if mock.IncrementalCalls[0].SyncToken != syncToken {
		t.Errorf("IncrementalCalls sync token = %q, want %q", mock.IncrementalCalls[0].SyncToken, syncToken)
	}

	// Test error case
	mock.IncrementalError = errMock
	_, err = mock.FetchEventsIncremental(ctx, creds, calendarID, syncToken)
	if err == nil {
		t.Error("Expected error when IncrementalError is set")
	}
}

func TestMockCalendarClient_Reset(t *testing.T) {
	ctx := context.Background()
	mock := NewMockCalendarClient()
	creds := &store.OAuthCredentials{}

	// Make some calls
	mock.ExchangeCode(ctx, "code")
	mock.RefreshToken(ctx, creds)
	mock.ListCalendars(ctx, creds)
	mock.FetchEvents(ctx, creds, "cal", time.Now(), time.Now())
	mock.FetchEventsIncremental(ctx, creds, "cal", "token")

	// Verify calls were tracked
	if len(mock.ExchangeCalls) != 1 {
		t.Error("ExchangeCalls should be tracked")
	}

	// Reset
	mock.Reset()

	// Verify all call tracking was cleared
	if len(mock.ExchangeCalls) != 0 {
		t.Error("ExchangeCalls should be cleared after Reset")
	}
	if mock.RefreshCalls != 0 {
		t.Error("RefreshCalls should be cleared after Reset")
	}
	if mock.ListCalendarCalls != 0 {
		t.Error("ListCalendarCalls should be cleared after Reset")
	}
	if len(mock.FetchCalls) != 0 {
		t.Error("FetchCalls should be cleared after Reset")
	}
	if len(mock.IncrementalCalls) != 0 {
		t.Error("IncrementalCalls should be cleared after Reset")
	}
}

// Test error type
type mockError string

func (e mockError) Error() string { return string(e) }

var errMock mockError = "mock error"
