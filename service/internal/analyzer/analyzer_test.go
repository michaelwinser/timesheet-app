package analyzer

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRoundMinutes(t *testing.T) {
	cfg := DefaultRoundingConfig() // 15-minute granularity, 7-minute threshold

	tests := []struct {
		name        string
		minutes     int
		wantMinutes int
		wantDesc    string
	}{
		{"exact 15 minutes", 15, 15, "none"},
		{"exact 30 minutes", 30, 30, "none"},
		{"exact 60 minutes", 60, 60, "none"},
		{"exact 0 minutes", 0, 0, "none"},

		// Round down cases (remainder 0-6)
		{"6 minutes rounds down to 0", 6, 0, "-6m"},
		{"16 minutes rounds down", 16, 15, "-1m"},
		{"21 minutes rounds down", 21, 15, "-6m"},
		{"31 minutes rounds down", 31, 30, "-1m"},
		{"36 minutes rounds down", 36, 30, "-6m"},

		// Round up cases (remainder 7-14)
		{"7 minutes rounds up", 7, 15, "+8m"},
		{"14 minutes rounds up", 14, 15, "+1m"},
		{"22 minutes rounds up", 22, 30, "+8m"},
		{"23 minutes rounds up", 23, 30, "+7m"},
		{"29 minutes rounds up", 29, 30, "+1m"},
		{"37 minutes rounds up", 37, 45, "+8m"},
		{"44 minutes rounds up", 44, 45, "+1m"},

		// Larger values
		{"55 minutes (25m meeting)", 55, 60, "+5m"},
		{"50 minutes", 50, 45, "-5m"},
		{"51 minutes", 51, 45, "-6m"},
		{"52 minutes", 52, 60, "+8m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMinutes, gotDesc := RoundMinutes(tt.minutes, cfg)
			if gotMinutes != tt.wantMinutes {
				t.Errorf("RoundMinutes(%d) = %d, want %d", tt.minutes, gotMinutes, tt.wantMinutes)
			}
			if gotDesc != tt.wantDesc {
				t.Errorf("RoundMinutes(%d) desc = %q, want %q", tt.minutes, gotDesc, tt.wantDesc)
			}
		})
	}
}

func TestComputeTimeUnion(t *testing.T) {
	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		events      []Event
		wantMinutes int
		wantRanges  int
	}{
		{
			name: "single event",
			events: []Event{
				makeEvent(date, "09:00", "10:00"),
			},
			wantMinutes: 60,
			wantRanges:  1,
		},
		{
			name: "two non-overlapping events",
			events: []Event{
				makeEvent(date, "09:00", "10:00"),
				makeEvent(date, "11:00", "12:00"),
			},
			wantMinutes: 120,
			wantRanges:  2,
		},
		{
			name: "two overlapping events (PRD example)",
			events: []Event{
				makeEvent(date, "09:00", "09:30"),
				makeEvent(date, "09:15", "10:00"),
			},
			wantMinutes: 60, // union is 9:00-10:00, not 9:30+45=75
			wantRanges:  1,
		},
		{
			name: "three overlapping events",
			events: []Event{
				makeEvent(date, "09:00", "09:45"),
				makeEvent(date, "09:30", "10:15"),
				makeEvent(date, "10:00", "10:30"),
			},
			wantMinutes: 90, // 9:00-10:30
			wantRanges:  1,
		},
		{
			name: "adjacent events",
			events: []Event{
				makeEvent(date, "09:00", "10:00"),
				makeEvent(date, "10:00", "11:00"),
			},
			wantMinutes: 120,
			wantRanges:  1, // Should merge adjacent events
		},
		{
			name: "event fully contained in another",
			events: []Event{
				makeEvent(date, "09:00", "12:00"),
				makeEvent(date, "10:00", "11:00"),
			},
			wantMinutes: 180, // Just the outer event
			wantRanges:  1,
		},
		{
			name: "complex overlapping pattern",
			events: []Event{
				makeEvent(date, "09:00", "10:00"), // Range 1 starts
				makeEvent(date, "09:30", "10:30"), // Extends range 1
				makeEvent(date, "12:00", "13:00"), // Range 2 (gap)
				makeEvent(date, "12:30", "13:30"), // Extends range 2
			},
			wantMinutes: 180, // 90 + 90
			wantRanges:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ranges := computeTimeUnion(tt.events)
			if len(ranges) != tt.wantRanges {
				t.Errorf("computeTimeUnion() returned %d ranges, want %d", len(ranges), tt.wantRanges)
			}

			totalMinutes := 0
			for _, r := range ranges {
				totalMinutes += r.Minutes
			}
			if totalMinutes != tt.wantMinutes {
				t.Errorf("computeTimeUnion() total = %d minutes, want %d", totalMinutes, tt.wantMinutes)
			}
		})
	}
}

func TestCompute(t *testing.T) {
	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	projectA := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	projectB := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	cfg := DefaultRoundingConfig()

	tests := []struct {
		name       string
		events     []Event
		wantCount  int
		wantHours  map[uuid.UUID]float64
	}{
		{
			name:      "empty events",
			events:    []Event{},
			wantCount: 0,
		},
		{
			name: "single project single event",
			events: []Event{
				{
					ID:        uuid.New(),
					ProjectID: projectA,
					Title:     "Meeting",
					StartTime: date.Add(9 * time.Hour),
					EndTime:   date.Add(10 * time.Hour),
				},
			},
			wantCount: 1,
			wantHours: map[uuid.UUID]float64{projectA: 1.0},
		},
		{
			name: "single project overlapping events",
			events: []Event{
				{
					ID:        uuid.New(),
					ProjectID: projectA,
					Title:     "Meeting 1",
					StartTime: date.Add(9 * time.Hour),
					EndTime:   date.Add(9*time.Hour + 30*time.Minute),
				},
				{
					ID:        uuid.New(),
					ProjectID: projectA,
					Title:     "Meeting 2",
					StartTime: date.Add(9*time.Hour + 15*time.Minute),
					EndTime:   date.Add(10 * time.Hour),
				},
			},
			wantCount: 1,
			wantHours: map[uuid.UUID]float64{projectA: 1.0}, // Union = 1 hour
		},
		{
			name: "two projects separate events",
			events: []Event{
				{
					ID:        uuid.New(),
					ProjectID: projectA,
					Title:     "Meeting A",
					StartTime: date.Add(9 * time.Hour),
					EndTime:   date.Add(10 * time.Hour),
				},
				{
					ID:        uuid.New(),
					ProjectID: projectB,
					Title:     "Meeting B",
					StartTime: date.Add(11 * time.Hour),
					EndTime:   date.Add(12 * time.Hour),
				},
			},
			wantCount: 2,
			wantHours: map[uuid.UUID]float64{projectA: 1.0, projectB: 1.0},
		},
		{
			name: "rounding applied",
			events: []Event{
				{
					ID:        uuid.New(),
					ProjectID: projectA,
					Title:     "Short meeting",
					StartTime: date.Add(9 * time.Hour),
					EndTime:   date.Add(9*time.Hour + 25*time.Minute), // 25 min -> rounds to 30
				},
			},
			wantCount: 1,
			wantHours: map[uuid.UUID]float64{projectA: 0.5}, // 30 minutes
		},
		{
			name: "all-day event contributes 0 hours",
			events: []Event{
				{
					ID:        uuid.New(),
					ProjectID: projectA,
					Title:     "All-day event",
					StartTime: date,
					EndTime:   date.Add(24 * time.Hour),
					IsAllDay:  true,
				},
			},
			wantCount: 1,
			wantHours: map[uuid.UUID]float64{projectA: 0.0},
		},
		{
			name: "mixed all-day and timed events",
			events: []Event{
				{
					ID:        uuid.New(),
					ProjectID: projectA,
					Title:     "All-day event",
					StartTime: date,
					EndTime:   date.Add(24 * time.Hour),
					IsAllDay:  true,
				},
				{
					ID:        uuid.New(),
					ProjectID: projectA,
					Title:     "Meeting",
					StartTime: date.Add(10 * time.Hour),
					EndTime:   date.Add(11 * time.Hour),
				},
			},
			wantCount: 1,
			wantHours: map[uuid.UUID]float64{projectA: 1.0}, // Only timed event
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := Compute(date, tt.events, cfg)

			if len(entries) != tt.wantCount {
				t.Errorf("Compute() returned %d entries, want %d", len(entries), tt.wantCount)
			}

			for _, entry := range entries {
				if wantHours, ok := tt.wantHours[entry.ProjectID]; ok {
					if entry.Hours != wantHours {
						t.Errorf("Compute() project %s hours = %v, want %v",
							entry.ProjectID, entry.Hours, wantHours)
					}
				}
			}
		})
	}
}

func TestComputeCalculationDetails(t *testing.T) {
	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	projectID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	cfg := DefaultRoundingConfig()

	events := []Event{
		{
			ID:        uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			ProjectID: projectID,
			Title:     "Standup",
			StartTime: date.Add(9 * time.Hour),
			EndTime:   date.Add(9*time.Hour + 15*time.Minute),
		},
		{
			ID:        uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			ProjectID: projectID,
			Title:     "Planning",
			StartTime: date.Add(10 * time.Hour),
			EndTime:   date.Add(11 * time.Hour),
		},
	}

	entries := Compute(date, events, cfg)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	entry := entries[0]
	details := entry.CalculationDetails

	// Verify events in details
	if len(details.Events) != 2 {
		t.Errorf("expected 2 events in details, got %d", len(details.Events))
	}

	// Verify time ranges (should be 2 non-overlapping ranges)
	if len(details.TimeRanges) != 2 {
		t.Errorf("expected 2 time ranges, got %d", len(details.TimeRanges))
	}

	// Verify union minutes (15 + 60 = 75)
	if details.UnionMinutes != 75 {
		t.Errorf("expected union_minutes=75, got %d", details.UnionMinutes)
	}

	// Verify final minutes (75 rounds up to 75 - wait, 75 % 15 = 0, so no rounding)
	if details.FinalMinutes != 75 {
		t.Errorf("expected final_minutes=75, got %d", details.FinalMinutes)
	}

	// Verify hours
	if entry.Hours != 1.25 {
		t.Errorf("expected hours=1.25, got %v", entry.Hours)
	}

	// Verify contributing events
	if len(entry.ContributingEvents) != 2 {
		t.Errorf("expected 2 contributing events, got %d", len(entry.ContributingEvents))
	}
}

func TestGenerateTitle(t *testing.T) {
	tests := []struct {
		name   string
		events []Event
		want   string
	}{
		{
			name:   "empty",
			events: []Event{},
			want:   "",
		},
		{
			name: "single event",
			events: []Event{
				{Title: "Weekly Sync"},
			},
			want: "Weekly Sync",
		},
		{
			name: "two events",
			events: []Event{
				{Title: "Weekly Sync"},
				{Title: "Planning"},
			},
			want: "Weekly Sync +1 more",
		},
		{
			name: "three events",
			events: []Event{
				{Title: "Weekly Sync"},
				{Title: "Planning"},
				{Title: "Review"},
			},
			want: "Weekly Sync +2 more",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateTitle(tt.events)
			if got != tt.want {
				t.Errorf("generateTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGenerateDescription(t *testing.T) {
	tests := []struct {
		name   string
		events []Event
		want   string
	}{
		{
			name:   "empty",
			events: []Event{},
			want:   "",
		},
		{
			name: "single event",
			events: []Event{
				{Title: "Weekly Sync"},
			},
			want: "Weekly Sync",
		},
		{
			name: "multiple unique events",
			events: []Event{
				{Title: "Weekly Sync"},
				{Title: "Planning"},
			},
			want: "Weekly Sync, Planning",
		},
		{
			name: "deduplicated events",
			events: []Event{
				{Title: "Weekly Sync"},
				{Title: "Weekly Sync"},
				{Title: "Planning"},
			},
			want: "Weekly Sync, Planning",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateDescription(tt.events)
			if got != tt.want {
				t.Errorf("generateDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Helper to create events for testing
func makeEvent(date time.Time, startTime, endTime string) Event {
	start := parseTime(date, startTime)
	end := parseTime(date, endTime)
	return Event{
		ID:        uuid.New(),
		ProjectID: uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		Title:     "Test Event",
		StartTime: start,
		EndTime:   end,
	}
}

func parseTime(date time.Time, timeStr string) time.Time {
	t, _ := time.Parse("15:04", timeStr)
	return time.Date(date.Year(), date.Month(), date.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)
}
