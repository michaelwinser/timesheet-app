package sync

import (
	"testing"

	gcal "google.golang.org/api/calendar/v3"
)

func TestExtendedPropertiesToStore(t *testing.T) {
	tests := []struct {
		name        string
		event       *gcal.Event
		wantNil     bool
		wantPrivate map[string]string
		wantShared  map[string]string
	}{
		{
			name:    "absent",
			event:   &gcal.Event{},
			wantNil: true,
		},
		{
			name:    "present but empty",
			event:   &gcal.Event{ExtendedProperties: &gcal.EventExtendedProperties{}},
			wantNil: true,
		},
		{
			name: "private only",
			event: &gcal.Event{ExtendedProperties: &gcal.EventExtendedProperties{
				Private: map[string]string{"calendarSyncMarker": "v1"},
			}},
			wantPrivate: map[string]string{"calendarSyncMarker": "v1"},
		},
		{
			name: "both namespaces preserved",
			event: &gcal.Event{ExtendedProperties: &gcal.EventExtendedProperties{
				Private: map[string]string{"a": "1"},
				Shared:  map[string]string{"b": "2"},
			}},
			wantPrivate: map[string]string{"a": "1"},
			wantShared:  map[string]string{"b": "2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtendedPropertiesToStore(tt.event)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("ExtendedPropertiesToStore() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("ExtendedPropertiesToStore() = nil, want properties")
			}
			if len(got.Private) != len(tt.wantPrivate) {
				t.Errorf("Private = %v, want %v", got.Private, tt.wantPrivate)
			}
			for k, v := range tt.wantPrivate {
				if got.Private[k] != v {
					t.Errorf("Private[%q] = %q, want %q", k, got.Private[k], v)
				}
			}
			for k, v := range tt.wantShared {
				if got.Shared[k] != v {
					t.Errorf("Shared[%q] = %q, want %q", k, got.Shared[k], v)
				}
			}
		})
	}
}
