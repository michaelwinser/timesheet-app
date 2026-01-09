package sync

import (
	"testing"
	"time"
)

func TestNormalizeToWeekStart(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "Monday stays Monday",
			input:    time.Date(2025, 1, 6, 10, 30, 0, 0, time.UTC), // Monday
			expected: time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Wednesday goes to Monday",
			input:    time.Date(2025, 1, 8, 15, 45, 0, 0, time.UTC), // Wednesday
			expected: time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Sunday goes to previous Monday",
			input:    time.Date(2025, 1, 12, 23, 59, 0, 0, time.UTC), // Sunday
			expected: time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Saturday goes to Monday",
			input:    time.Date(2025, 1, 11, 12, 0, 0, 0, time.UTC), // Saturday
			expected: time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeToWeekStart(tt.input)
			if !result.Equal(tt.expected) {
				t.Errorf("NormalizeToWeekStart(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeToWeekEnd(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "Monday goes to Sunday",
			input:    time.Date(2025, 1, 6, 10, 30, 0, 0, time.UTC), // Monday
			expected: time.Date(2025, 1, 12, 23, 59, 59, 0, time.UTC),
		},
		{
			name:     "Sunday stays same Sunday",
			input:    time.Date(2025, 1, 12, 12, 0, 0, 0, time.UTC), // Sunday
			expected: time.Date(2025, 1, 12, 23, 59, 59, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeToWeekEnd(tt.input)
			if !result.Equal(tt.expected) {
				t.Errorf("NormalizeToWeekEnd(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsWeekWithinRange(t *testing.T) {
	minSynced := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)  // Monday Jan 6
	maxSynced := time.Date(2025, 1, 26, 0, 0, 0, 0, time.UTC) // Sunday Jan 26 (end of 3rd week)

	tests := []struct {
		name     string
		weekDate time.Time
		expected bool
	}{
		{
			name:     "Week within range",
			weekDate: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC), // Week of Jan 13-19
			expected: true,
		},
		{
			name:     "First week in range",
			weekDate: time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC), // Week of Jan 6-12
			expected: true,
		},
		{
			name:     "Week before range",
			weekDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), // Week before
			expected: false,
		},
		{
			name:     "Week after range",
			weekDate: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), // Week after
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWeekWithinRange(tt.weekDate, &minSynced, &maxSynced)
			if result != tt.expected {
				t.Errorf("IsWeekWithinRange(%v) = %v, want %v", tt.weekDate, result, tt.expected)
			}
		})
	}

	// Test with nil values
	t.Run("nil minSynced returns false", func(t *testing.T) {
		if IsWeekWithinRange(time.Now(), nil, &maxSynced) {
			t.Error("Expected false for nil minSynced")
		}
	})

	t.Run("nil maxSynced returns false", func(t *testing.T) {
		if IsWeekWithinRange(time.Now(), &minSynced, nil) {
			t.Error("Expected false for nil maxSynced")
		}
	})
}

func TestIsStale(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		lastSyncedAt *time.Time
		expected     bool
	}{
		{
			name:         "nil is stale",
			lastSyncedAt: nil,
			expected:     true,
		},
		{
			name:         "recent sync is not stale",
			lastSyncedAt: timePtr(now.Add(-1 * time.Hour)),
			expected:     false,
		},
		{
			name:         "old sync is stale",
			lastSyncedAt: timePtr(now.Add(-25 * time.Hour)),
			expected:     true,
		},
		{
			name:         "just under threshold is not stale",
			lastSyncedAt: timePtr(now.Add(-23 * time.Hour)),
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsStale(tt.lastSyncedAt)
			if result != tt.expected {
				t.Errorf("IsStale(%v) = %v, want %v", tt.lastSyncedAt, result, tt.expected)
			}
		})
	}
}

func TestWeeksInRange(t *testing.T) {
	start := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)  // Monday Jan 6
	end := time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC)   // Monday Jan 20

	weeks := WeeksInRange(start, end)

	if len(weeks) != 3 {
		t.Errorf("Expected 3 weeks, got %d", len(weeks))
	}

	expectedWeeks := []time.Time{
		time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC),
	}

	for i, expected := range expectedWeeks {
		if !weeks[i].Equal(expected) {
			t.Errorf("Week %d: got %v, want %v", i, weeks[i], expected)
		}
	}
}

func TestMissingWeeks(t *testing.T) {
	t.Run("nil range returns all target weeks", func(t *testing.T) {
		targetStart := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC)

		missing := MissingWeeks(nil, nil, targetStart, targetEnd)
		if len(missing) != 3 {
			t.Errorf("Expected 3 missing weeks, got %d", len(missing))
		}
	})

	t.Run("returns weeks before existing range", func(t *testing.T) {
		minSynced := time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC)
		maxSynced := time.Date(2025, 1, 26, 0, 0, 0, 0, time.UTC)
		targetStart := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 1, 26, 0, 0, 0, 0, time.UTC)

		missing := MissingWeeks(&minSynced, &maxSynced, targetStart, targetEnd)

		// Should only return week of Jan 6 (before minSynced)
		if len(missing) != 1 {
			t.Errorf("Expected 1 missing week, got %d", len(missing))
		}
		if len(missing) > 0 && !missing[0].Equal(time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("Expected Jan 6, got %v", missing[0])
		}
	})
}

func TestDecideSync(t *testing.T) {
	// Set up a synced window: Jan 6-26, 2025 (3 weeks)
	minSynced := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	maxSynced := time.Date(2025, 1, 26, 23, 59, 59, 0, time.UTC)
	freshSync := timePtr(time.Now().Add(-1 * time.Hour)) // 1 hour ago
	staleSync := timePtr(time.Now().Add(-25 * time.Hour)) // 25 hours ago

	t.Run("Case A: fresh data within window", func(t *testing.T) {
		targetStart := time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 1, 19, 0, 0, 0, 0, time.UTC)

		decision := DecideSync(&minSynced, &maxSynced, freshSync, targetStart, targetEnd)

		if decision.NeedsSync {
			t.Error("Expected no sync needed for fresh data within window")
		}
		if decision.Reason != "fresh_data" {
			t.Errorf("Expected reason 'fresh_data', got %s", decision.Reason)
		}
	})

	t.Run("Case A': stale data within window", func(t *testing.T) {
		targetStart := time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 1, 19, 0, 0, 0, 0, time.UTC)

		decision := DecideSync(&minSynced, &maxSynced, staleSync, targetStart, targetEnd)

		if !decision.NeedsSync {
			t.Error("Expected sync needed for stale data")
		}
		if decision.Reason != "stale_data" {
			t.Errorf("Expected reason 'stale_data', got %s", decision.Reason)
		}
		if !decision.IsStaleRefresh {
			t.Error("Expected IsStaleRefresh to be true")
		}
	})

	t.Run("Case B: week before synced window", func(t *testing.T) {
		targetStart := time.Date(2024, 12, 30, 0, 0, 0, 0, time.UTC) // Week of Dec 30
		targetEnd := time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC)

		decision := DecideSync(&minSynced, &maxSynced, freshSync, targetStart, targetEnd)

		if !decision.NeedsSync {
			t.Error("Expected sync needed for week before window")
		}
		if decision.Reason != "outside_window" {
			t.Errorf("Expected reason 'outside_window', got %s", decision.Reason)
		}
		if len(decision.MissingWeeks) != 1 {
			t.Errorf("Expected 1 missing week, got %d", len(decision.MissingWeeks))
		}
	})

	t.Run("Case C: week after synced window", func(t *testing.T) {
		targetStart := time.Date(2025, 1, 27, 0, 0, 0, 0, time.UTC) // Week of Jan 27
		targetEnd := time.Date(2025, 2, 2, 0, 0, 0, 0, time.UTC)

		decision := DecideSync(&minSynced, &maxSynced, freshSync, targetStart, targetEnd)

		if !decision.NeedsSync {
			t.Error("Expected sync needed for week after window")
		}
		if decision.Reason != "outside_window" {
			t.Errorf("Expected reason 'outside_window', got %s", decision.Reason)
		}
	})

	t.Run("nil synced range returns all target weeks", func(t *testing.T) {
		targetStart := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC)

		decision := DecideSync(nil, nil, nil, targetStart, targetEnd)

		if !decision.NeedsSync {
			t.Error("Expected sync needed for nil synced range")
		}
		if decision.Reason != "no_synced_range" {
			t.Errorf("Expected reason 'no_synced_range', got %s", decision.Reason)
		}
		if len(decision.MissingWeeks) != 3 {
			t.Errorf("Expected 3 missing weeks, got %d", len(decision.MissingWeeks))
		}
	})
}

func timePtr(t time.Time) *time.Time {
	return &t
}
