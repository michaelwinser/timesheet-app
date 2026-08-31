package classification

import (
	"testing"

	"github.com/google/uuid"

	"github.com/michaelw/timesheet-app/service/internal/store"
)

func placeholderEvent() *store.CalendarEvent {
	return &store.CalendarEvent{
		ID:    uuid.New(),
		Title: "🔄 Acme standup",
		ExtendedProperties: &store.EventExtendedProperties{
			Private: map[string]string{
				"calendarSyncMarker": "v1",
				"sourceCalendarId":   "michaelw@winser.com",
			},
		},
	}
}

// TestIngestionFilterSet_Match carries forward the behaviour that was
// hard-coded in #108, now expressed as the seeded filter query. The critical
// case remains the version bump: matching is on key presence, so a value change
// by the upstream tool must not let the noise back in.
func TestIngestionFilterSet_Match(t *testing.T) {
	placeholders := []IngestionFilter{
		{ID: "1", Name: "Calendar sync placeholders", Query: "property:calendarSyncMarker"},
	}

	tests := []struct {
		name    string
		filters []IngestionFilter
		event   *store.CalendarEvent
		want    bool
	}{
		{"marked placeholder matches", placeholders, placeholderEvent(), true},
		{
			name:    "a future marker value still matches",
			filters: placeholders,
			event: &store.CalendarEvent{Title: "🔄 Standup", ExtendedProperties: &store.EventExtendedProperties{
				Private: map[string]string{"calendarSyncMarker": "v2"},
			}},
			want: true,
		},
		{
			name:    "emoji title alone does not match",
			filters: placeholders,
			event:   &store.CalendarEvent{Title: "🔄 Acme standup"},
			want:    false,
		},
		{
			name:    "another integration's properties do not match",
			filters: placeholders,
			event: &store.CalendarEvent{Title: "Focus time", ExtendedProperties: &store.EventExtendedProperties{
				Private: map[string]string{"reclaim.event.type": "MEETING"},
			}},
			want: false,
		},
		{"ordinary event does not match", placeholders, &store.CalendarEvent{Title: "Acme standup"}, false},
		{"no filters means nothing is hidden", nil, placeholderEvent(), false},
		{
			name: "any filter in the set can match",
			filters: []IngestionFilter{
				{ID: "1", Name: "Reclaim", Query: "property:reclaim.event.type"},
				{ID: "2", Name: "Placeholders", Query: "property:calendarSyncMarker"},
			},
			event: placeholderEvent(),
			want:  true,
		},
		{
			name:    "filters can match on things other than properties",
			filters: []IngestionFilter{{ID: "1", Name: "Out of office", Query: `title:"out of office"`}},
			event:   &store.CalendarEvent{Title: "Out of Office - Michael"},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set, invalid := NewIngestionFilterSet(tt.filters)
			if len(invalid) != 0 {
				t.Fatalf("unexpected invalid filters: %v", invalid)
			}
			got, _ := set.Match(tt.event)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

// An unparseable filter must be reported rather than silently ingesting
// everything it was meant to catch.
func TestNewIngestionFilterSet_InvalidQuery(t *testing.T) {
	set, invalid := NewIngestionFilterSet([]IngestionFilter{
		{ID: "1", Name: "Broken", Query: `title:"unclosed`},
		{ID: "2", Name: "Fine", Query: "property:calendarSyncMarker"},
	})

	if len(invalid) != 1 || invalid[0].Name != "Broken" {
		t.Fatalf("invalid = %v, want just the broken filter", invalid)
	}
	if set.Len() != 1 {
		t.Errorf("Len() = %d, want the one valid filter still compiled", set.Len())
	}
	if matched, f := set.Match(placeholderEvent()); !matched || f.Name != "Fine" {
		t.Errorf("the valid filter should still match, got matched=%v filter=%q", matched, f.Name)
	}
}

// A nil set is the "filters could not be loaded" case and must hide nothing.
func TestIngestionFilterSet_NilHidesNothing(t *testing.T) {
	var set *IngestionFilterSet
	if matched, _ := set.Match(placeholderEvent()); matched {
		t.Error("a nil filter set must not suppress anything")
	}
	if set.Len() != 0 {
		t.Errorf("Len() = %d, want 0", set.Len())
	}
}
