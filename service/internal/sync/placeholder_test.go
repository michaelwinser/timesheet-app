package sync

import (
	"testing"

	gcal "google.golang.org/api/calendar/v3"
)

// TestIsSyncPlaceholder covers the hard-coded ingestion filter from issue #108.
//
// The critical case is "marker with a different value": matching is on the
// presence of the key, so a version bump by the upstream sync tool must not
// stop placeholders being recognised.
func TestIsSyncPlaceholder(t *testing.T) {
	tests := []struct {
		name     string
		event    *gcal.Event
		expected bool
	}{
		{
			name:     "no extended properties",
			event:    &gcal.Event{Summary: "Standup"},
			expected: false,
		},
		{
			name: "empty extended properties",
			event: &gcal.Event{
				Summary:            "Standup",
				ExtendedProperties: &gcal.EventExtendedProperties{},
			},
			expected: false,
		},
		{
			name: "marker present with documented value",
			event: &gcal.Event{
				Summary: "🔄 Acme standup",
				ExtendedProperties: &gcal.EventExtendedProperties{
					Private: map[string]string{"calendarSyncMarker": "v1"},
				},
			},
			expected: true,
		},
		{
			name: "marker present with a future value",
			event: &gcal.Event{
				Summary: "🔄 Acme standup",
				ExtendedProperties: &gcal.EventExtendedProperties{
					Private: map[string]string{"calendarSyncMarker": "v2"},
				},
			},
			expected: true,
		},
		{
			name: "marker present with an empty value",
			event: &gcal.Event{
				ExtendedProperties: &gcal.EventExtendedProperties{
					Private: map[string]string{"calendarSyncMarker": ""},
				},
			},
			expected: true,
		},
		{
			name: "marker alongside other properties",
			event: &gcal.Event{
				ExtendedProperties: &gcal.EventExtendedProperties{
					Private: map[string]string{"someOtherTool": "x", "calendarSyncMarker": "v1"},
				},
			},
			expected: true,
		},
		{
			name: "unrelated private properties only",
			event: &gcal.Event{
				Summary: "Standup",
				ExtendedProperties: &gcal.EventExtendedProperties{
					Private: map[string]string{"someOtherTool": "x"},
				},
			},
			expected: false,
		},
		{
			name: "marker in the shared namespace is not the marker",
			event: &gcal.Event{
				ExtendedProperties: &gcal.EventExtendedProperties{
					Shared: map[string]string{"calendarSyncMarker": "v1"},
				},
			},
			expected: false,
		},
		{
			name:     "emoji title alone is not enough",
			event:    &gcal.Event{Summary: "🔄 Acme standup"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSyncPlaceholder(tt.event); got != tt.expected {
				t.Errorf("IsSyncPlaceholder() = %v, want %v", got, tt.expected)
			}
		})
	}
}

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
