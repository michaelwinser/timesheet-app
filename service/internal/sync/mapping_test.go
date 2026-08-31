package sync

import (
	"testing"

	"github.com/google/uuid"
	gcal "google.golang.org/api/calendar/v3"

	"github.com/michaelw/timesheet-app/service/internal/store"
)

// TestGoogleEventToStore covers the single mapping every sync path uses.
//
// The mapping previously existed as two copies in different packages with no
// direct test coverage, which is how the suppression in #108 came to be applied
// to only one of them.
// See: https://github.com/michaelwinser/timesheet-app/issues/112
func TestGoogleEventToStore(t *testing.T) {
	connID, calID, userID := uuid.New(), uuid.New(), uuid.New()

	tests := []struct {
		name   string
		event  *gcal.Event
		verify func(t *testing.T, got *store.CalendarEvent)
	}{
		{
			name: "timed event",
			event: &gcal.Event{
				Id:      "evt-1",
				Summary: "Standup",
				Start:   &gcal.EventDateTime{DateTime: "2026-08-31T10:00:00Z"},
				End:     &gcal.EventDateTime{DateTime: "2026-08-31T10:30:00Z"},
			},
			verify: func(t *testing.T, got *store.CalendarEvent) {
				if got.IsAllDay {
					t.Error("timed event should not be all-day")
				}
				if got.StartTime.UTC().Hour() != 10 {
					t.Errorf("StartTime = %v, want 10:00 UTC", got.StartTime)
				}
				if !got.EndTime.After(got.StartTime) {
					t.Error("EndTime should be after StartTime")
				}
			},
		},
		{
			name: "all-day event",
			event: &gcal.Event{
				Id:      "evt-2",
				Summary: "Conference",
				Start:   &gcal.EventDateTime{Date: "2026-08-31"},
				End:     &gcal.EventDateTime{Date: "2026-09-01"},
			},
			verify: func(t *testing.T, got *store.CalendarEvent) {
				if !got.IsAllDay {
					t.Error("date-only event should be all-day")
				}
			},
		},
		{
			name:  "missing description stays nil",
			event: &gcal.Event{Id: "evt-3", Summary: "No description"},
			verify: func(t *testing.T, got *store.CalendarEvent) {
				if got.Description != nil {
					t.Errorf("Description = %v, want nil", *got.Description)
				}
			},
		},
		{
			name: "attendees and self response status",
			event: &gcal.Event{
				Id: "evt-4",
				Attendees: []*gcal.EventAttendee{
					{Email: "alice@acme.com", ResponseStatus: "accepted"},
					{Email: "me@example.com", Self: true, ResponseStatus: "declined"},
				},
			},
			verify: func(t *testing.T, got *store.CalendarEvent) {
				if len(got.Attendees) != 2 {
					t.Fatalf("Attendees = %v, want 2", got.Attendees)
				}
				if got.ResponseStatus == nil || *got.ResponseStatus != "declined" {
					t.Errorf("ResponseStatus = %v, want the Self attendee's status", got.ResponseStatus)
				}
			},
		},
		{
			name: "organizer appended when not already an attendee",
			event: &gcal.Event{
				Id:        "evt-5",
				Attendees: []*gcal.EventAttendee{{Email: "alice@acme.com"}},
				Organizer: &gcal.EventOrganizer{Email: "boss@acme.com"},
			},
			verify: func(t *testing.T, got *store.CalendarEvent) {
				if len(got.Attendees) != 2 {
					t.Fatalf("Attendees = %v, want alice plus the organizer", got.Attendees)
				}
			},
		},
		{
			name: "organizer not duplicated when already an attendee",
			event: &gcal.Event{
				Id:        "evt-6",
				Attendees: []*gcal.EventAttendee{{Email: "boss@acme.com"}},
				Organizer: &gcal.EventOrganizer{Email: "boss@acme.com"},
			},
			verify: func(t *testing.T, got *store.CalendarEvent) {
				if len(got.Attendees) != 1 {
					t.Errorf("Attendees = %v, want no duplicate organizer", got.Attendees)
				}
			},
		},
		{
			name:  "recurring event detected from RecurringEventId",
			event: &gcal.Event{Id: "evt-7", RecurringEventId: "series-1"},
			verify: func(t *testing.T, got *store.CalendarEvent) {
				if !got.IsRecurring {
					t.Error("event with RecurringEventId should be recurring")
				}
			},
		},
		{
			name: "properties are preserved for filters to match on",
			event: &gcal.Event{
				Id:      "evt-8",
				Summary: "🔄 Acme standup",
				ExtendedProperties: &gcal.EventExtendedProperties{
					Private: map[string]string{"calendarSyncMarker": "v1", "sourceCalendarId": "other@example.com"},
				},
			},
			verify: func(t *testing.T, got *store.CalendarEvent) {
				// Suppression is decided by ingestion filters (#110), not here.
				if got.IsSuppressed {
					t.Error("the mapping should not decide suppression")
				}
				if got.ExtendedProperties == nil ||
					got.ExtendedProperties.Private["sourceCalendarId"] != "other@example.com" {
					t.Errorf("ExtendedProperties = %+v, want the private properties preserved", got.ExtendedProperties)
				}
			},
		},
		{
			name: "properties from other integrations are stored too",
			event: &gcal.Event{
				Id:      "evt-9",
				Summary: "Acme standup",
				ExtendedProperties: &gcal.EventExtendedProperties{
					Private: map[string]string{"reclaim.event.type": "MEETING"},
				},
			},
			verify: func(t *testing.T, got *store.CalendarEvent) {
				if got.ExtendedProperties == nil {
					t.Error("other integrations' properties should still be stored")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GoogleEventToStore(tt.event, connID, calID, userID)

			// Invariants that hold for every event
			if got.ConnectionID != connID || got.UserID != userID {
				t.Errorf("identity fields not carried through")
			}
			if got.CalendarID == nil || *got.CalendarID != calID {
				t.Errorf("CalendarID = %v, want %v", got.CalendarID, calID)
			}
			if got.ExternalID != tt.event.Id {
				t.Errorf("ExternalID = %q, want %q", got.ExternalID, tt.event.Id)
			}
			if got.ClassificationStatus != store.StatusPending {
				t.Errorf("ClassificationStatus = %v, want pending", got.ClassificationStatus)
			}

			tt.verify(t, got)
		})
	}
}
