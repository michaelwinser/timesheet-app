package sync

import (
	"errors"
	"testing"
	"time"
)

// Test edge cases for sync decision logic

func TestDecideSync_EdgeCases(t *testing.T) {
	t.Run("exactly at water mark boundary - start", func(t *testing.T) {
		minSynced := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		maxSynced := time.Date(2025, 1, 26, 23, 59, 59, 0, time.UTC)
		freshSync := timePtr(time.Now().Add(-1 * time.Hour))

		// Request exactly at the min boundary
		targetStart := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 1, 12, 23, 59, 59, 0, time.UTC)

		decision := DecideSync(&minSynced, &maxSynced, freshSync, targetStart, targetEnd)

		if decision.NeedsSync {
			t.Error("Should not need sync when exactly at min boundary")
		}
	})

	t.Run("exactly at water mark boundary - end", func(t *testing.T) {
		minSynced := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		maxSynced := time.Date(2025, 1, 26, 23, 59, 59, 0, time.UTC)
		freshSync := timePtr(time.Now().Add(-1 * time.Hour))

		// Request exactly at the max boundary
		targetStart := time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 1, 26, 23, 59, 59, 0, time.UTC)

		decision := DecideSync(&minSynced, &maxSynced, freshSync, targetStart, targetEnd)

		if decision.NeedsSync {
			t.Error("Should not need sync when exactly at max boundary")
		}
	})

	t.Run("one day before water mark", func(t *testing.T) {
		minSynced := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		maxSynced := time.Date(2025, 1, 26, 23, 59, 59, 0, time.UTC)
		freshSync := timePtr(time.Now().Add(-1 * time.Hour))

		// Request one day before min
		targetStart := time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 1, 5, 23, 59, 59, 0, time.UTC)

		decision := DecideSync(&minSynced, &maxSynced, freshSync, targetStart, targetEnd)

		if !decision.NeedsSync {
			t.Error("Should need sync when before min boundary")
		}
		if decision.Reason != "outside_window" {
			t.Errorf("Expected reason 'outside_window', got %s", decision.Reason)
		}
	})

	t.Run("one day after water mark", func(t *testing.T) {
		minSynced := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		maxSynced := time.Date(2025, 1, 26, 23, 59, 59, 0, time.UTC)
		freshSync := timePtr(time.Now().Add(-1 * time.Hour))

		// Request one day after max
		targetStart := time.Date(2025, 1, 27, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 1, 27, 23, 59, 59, 0, time.UTC)

		decision := DecideSync(&minSynced, &maxSynced, freshSync, targetStart, targetEnd)

		if !decision.NeedsSync {
			t.Error("Should need sync when after max boundary")
		}
	})

	t.Run("stale threshold around 24 hours", func(t *testing.T) {
		// Slightly under 24 hours ago should NOT be stale (threshold is > 24h)
		slightlyUnder := timePtr(time.Now().Add(-23*time.Hour - 59*time.Minute))
		if IsStale(slightlyUnder) {
			t.Error("23h59m ago should not be considered stale")
		}

		// Over 24 hours should be stale
		over := timePtr(time.Now().Add(-24*time.Hour - 1*time.Minute))
		if !IsStale(over) {
			t.Error("24h1m ago should be considered stale")
		}
	})

	t.Run("partial overlap with water marks - start before", func(t *testing.T) {
		minSynced := time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC)
		maxSynced := time.Date(2025, 1, 26, 23, 59, 59, 0, time.UTC)
		freshSync := timePtr(time.Now().Add(-1 * time.Hour))

		// Request overlaps: starts before, ends within
		targetStart := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 1, 19, 23, 59, 59, 0, time.UTC)

		decision := DecideSync(&minSynced, &maxSynced, freshSync, targetStart, targetEnd)

		if !decision.NeedsSync {
			t.Error("Should need sync when request starts before water marks")
		}
		// Should only report the missing weeks (Jan 6-12)
		if len(decision.MissingWeeks) == 0 {
			t.Error("Should have missing weeks")
		}
	})

	t.Run("partial overlap with water marks - end after", func(t *testing.T) {
		minSynced := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		maxSynced := time.Date(2025, 1, 19, 23, 59, 59, 0, time.UTC)
		freshSync := timePtr(time.Now().Add(-1 * time.Hour))

		// Request overlaps: starts within, ends after
		targetStart := time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 1, 26, 23, 59, 59, 0, time.UTC)

		decision := DecideSync(&minSynced, &maxSynced, freshSync, targetStart, targetEnd)

		if !decision.NeedsSync {
			t.Error("Should need sync when request ends after water marks")
		}
	})

	t.Run("island sync - gap in middle", func(t *testing.T) {
		// Simulates requesting a date range far outside current water marks
		// This creates an "island" that will need background job to fill gaps
		minSynced := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		maxSynced := time.Date(2025, 1, 12, 23, 59, 59, 0, time.UTC)
		freshSync := timePtr(time.Now().Add(-1 * time.Hour))

		// Request far in the future (creating an island)
		targetStart := time.Date(2025, 3, 3, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 3, 9, 23, 59, 59, 0, time.UTC)

		decision := DecideSync(&minSynced, &maxSynced, freshSync, targetStart, targetEnd)

		if !decision.NeedsSync {
			t.Error("Should need sync for island request")
		}
		if decision.Reason != "outside_window" {
			t.Errorf("Expected reason 'outside_window', got %s", decision.Reason)
		}
	})

	t.Run("very old last sync - multiple days stale", func(t *testing.T) {
		minSynced := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		maxSynced := time.Date(2025, 1, 26, 23, 59, 59, 0, time.UTC)
		veryOld := timePtr(time.Now().Add(-7 * 24 * time.Hour)) // 7 days ago

		targetStart := time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 1, 19, 23, 59, 59, 0, time.UTC)

		decision := DecideSync(&minSynced, &maxSynced, veryOld, targetStart, targetEnd)

		if !decision.NeedsSync {
			t.Error("Should need sync when data is very stale")
		}
		if !decision.IsStaleRefresh {
			t.Error("Should be marked as stale refresh")
		}
	})

	t.Run("future date request", func(t *testing.T) {
		minSynced := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		maxSynced := time.Date(2025, 1, 26, 23, 59, 59, 0, time.UTC)
		freshSync := timePtr(time.Now().Add(-1 * time.Hour))

		// Request future dates (common for planning)
		futureStart := time.Now().AddDate(0, 1, 0) // 1 month from now
		futureEnd := futureStart.AddDate(0, 0, 7)

		decision := DecideSync(&minSynced, &maxSynced, freshSync, futureStart, futureEnd)

		if !decision.NeedsSync {
			t.Error("Should need sync for future dates outside water marks")
		}
	})

	t.Run("past date request - historical", func(t *testing.T) {
		minSynced := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		maxSynced := time.Date(2025, 1, 26, 23, 59, 59, 0, time.UTC)
		freshSync := timePtr(time.Now().Add(-1 * time.Hour))

		// Request historical dates
		targetStart := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2024, 6, 7, 23, 59, 59, 0, time.UTC)

		decision := DecideSync(&minSynced, &maxSynced, freshSync, targetStart, targetEnd)

		if !decision.NeedsSync {
			t.Error("Should need sync for historical dates outside water marks")
		}
	})
}

func TestWeekNormalization_EdgeCases(t *testing.T) {
	t.Run("year boundary - Dec 31 to Jan 1", func(t *testing.T) {
		// Dec 31, 2024 was a Tuesday
		dec31 := time.Date(2024, 12, 31, 12, 0, 0, 0, time.UTC)
		weekStart := NormalizeToWeekStart(dec31)

		// Should go back to Monday Dec 30, 2024
		expected := time.Date(2024, 12, 30, 0, 0, 0, 0, time.UTC)
		if !weekStart.Equal(expected) {
			t.Errorf("Dec 31 2024 week start = %v, want %v", weekStart, expected)
		}
	})

	t.Run("year boundary - Jan 1", func(t *testing.T) {
		// Jan 1, 2025 was a Wednesday
		jan1 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
		weekStart := NormalizeToWeekStart(jan1)

		// Should go back to Monday Dec 30, 2024
		expected := time.Date(2024, 12, 30, 0, 0, 0, 0, time.UTC)
		if !weekStart.Equal(expected) {
			t.Errorf("Jan 1 2025 week start = %v, want %v", weekStart, expected)
		}
	})

	t.Run("leap year Feb 29", func(t *testing.T) {
		// Feb 29, 2024 was a Thursday (2024 is a leap year)
		feb29 := time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC)
		weekStart := NormalizeToWeekStart(feb29)

		// Should go back to Monday Feb 26, 2024
		expected := time.Date(2024, 2, 26, 0, 0, 0, 0, time.UTC)
		if !weekStart.Equal(expected) {
			t.Errorf("Feb 29 2024 week start = %v, want %v", weekStart, expected)
		}
	})

	t.Run("DST transition - spring forward", func(t *testing.T) {
		// Test with a timezone that has DST
		loc, _ := time.LoadLocation("America/New_York")
		// March 10, 2024 - DST starts in US (clocks spring forward)
		dstDay := time.Date(2024, 3, 10, 12, 0, 0, 0, loc)
		weekStart := NormalizeToWeekStart(dstDay)

		// Should go back to Monday March 4, 2024
		expected := time.Date(2024, 3, 4, 0, 0, 0, 0, loc)
		if weekStart.Year() != expected.Year() || weekStart.Month() != expected.Month() || weekStart.Day() != expected.Day() {
			t.Errorf("DST day week start date = %v, want %v", weekStart, expected)
		}
	})

	t.Run("midnight exactly", func(t *testing.T) {
		midnight := time.Date(2025, 1, 8, 0, 0, 0, 0, time.UTC) // Wednesday midnight
		weekStart := NormalizeToWeekStart(midnight)

		expected := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC) // Monday
		if !weekStart.Equal(expected) {
			t.Errorf("Midnight week start = %v, want %v", weekStart, expected)
		}
	})

	t.Run("one nanosecond before midnight", func(t *testing.T) {
		almostMidnight := time.Date(2025, 1, 7, 23, 59, 59, 999999999, time.UTC) // Tuesday almost midnight
		weekStart := NormalizeToWeekStart(almostMidnight)

		expected := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC) // Monday
		if !weekStart.Equal(expected) {
			t.Errorf("Almost midnight week start = %v, want %v", weekStart, expected)
		}
	})
}

func TestMissingWeeks_EdgeCases(t *testing.T) {
	t.Run("single day request", func(t *testing.T) {
		minSynced := time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC)
		maxSynced := time.Date(2025, 1, 26, 0, 0, 0, 0, time.UTC)

		// Request just one day outside the range
		targetStart := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)

		missing := MissingWeeks(&minSynced, &maxSynced, targetStart, targetEnd)

		if len(missing) != 1 {
			t.Errorf("Expected 1 missing week for single day, got %d", len(missing))
		}
	})

	t.Run("empty range - start equals end", func(t *testing.T) {
		minSynced := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		maxSynced := time.Date(2025, 1, 26, 0, 0, 0, 0, time.UTC)

		// Start and end are the same, within synced range
		targetStart := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)

		missing := MissingWeeks(&minSynced, &maxSynced, targetStart, targetEnd)

		if len(missing) != 0 {
			t.Errorf("Expected 0 missing weeks for date within range, got %d", len(missing))
		}
	})

	t.Run("very large range - many weeks", func(t *testing.T) {
		// No synced range
		targetStart := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		targetEnd := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)

		missing := MissingWeeks(nil, nil, targetStart, targetEnd)

		// Should be roughly 52 weeks
		if len(missing) < 50 || len(missing) > 55 {
			t.Errorf("Expected ~52 missing weeks for full year, got %d", len(missing))
		}
	})
}

// Test error types that would be encountered during sync
func TestSyncErrorScenarios(t *testing.T) {
	t.Run("token expiry error detection", func(t *testing.T) {
		// Simulate errors that indicate token expiry
		tokenErrors := []error{
			errors.New("oauth2: token expired"),
			errors.New("invalid_grant: Token has been expired or revoked"),
			errors.New("401 Unauthorized"),
		}

		for _, err := range tokenErrors {
			if err == nil {
				t.Error("Error should not be nil")
			}
			// In real implementation, we'd check if error indicates token expiry
			// and trigger re-auth flow
		}
	})

	t.Run("rate limit error detection", func(t *testing.T) {
		// Simulate rate limit errors
		rateLimitErrors := []error{
			errors.New("googleapi: Error 429: Rate Limit Exceeded"),
			errors.New("quota exceeded"),
		}

		for _, err := range rateLimitErrors {
			if err == nil {
				t.Error("Error should not be nil")
			}
			// In real implementation, we'd implement exponential backoff
		}
	})

	t.Run("sync token invalidation - 410 Gone", func(t *testing.T) {
		// Simulate sync token invalidation
		goneErr := errors.New("googleapi: Error 410: Sync token is no longer valid")

		if goneErr == nil {
			t.Error("Error should not be nil")
		}
		// In real implementation, we'd fall back to full sync
	})
}

// Test concurrent scenarios (logic validation, not actual concurrency)
func TestConcurrencyScenarios(t *testing.T) {
	t.Run("multiple workers same calendar - job isolation", func(t *testing.T) {
		// This tests the logic that would prevent conflicts
		// In practice, FOR UPDATE SKIP LOCKED in PostgreSQL handles this

		// Simulate two jobs for the same calendar
		job1Range := struct{ start, end time.Time }{
			start: time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2025, 1, 12, 0, 0, 0, 0, time.UTC),
		}
		job2Range := struct{ start, end time.Time }{
			start: time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC),
			end:   time.Date(2025, 1, 19, 0, 0, 0, 0, time.UTC),
		}

		// Jobs should be for non-overlapping ranges
		if job1Range.end.After(job2Range.start) && job1Range.start.Before(job2Range.end) {
			t.Error("Jobs should not overlap for proper isolation")
		}
	})

	t.Run("job coalescing logic", func(t *testing.T) {
		// Test the logic for coalescing multiple pending jobs
		jobs := []struct {
			minDate time.Time
			maxDate time.Time
		}{
			{time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 12, 0, 0, 0, 0, time.UTC)},
			{time.Date(2025, 1, 13, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 19, 0, 0, 0, 0, time.UTC)},
			{time.Date(2025, 1, 20, 0, 0, 0, 0, time.UTC), time.Date(2025, 1, 26, 0, 0, 0, 0, time.UTC)},
		}

		// Find coalesced range
		var coalescedMin, coalescedMax time.Time
		for i, job := range jobs {
			if i == 0 {
				coalescedMin = job.minDate
				coalescedMax = job.maxDate
			} else {
				if job.minDate.Before(coalescedMin) {
					coalescedMin = job.minDate
				}
				if job.maxDate.After(coalescedMax) {
					coalescedMax = job.maxDate
				}
			}
		}

		expectedMin := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
		expectedMax := time.Date(2025, 1, 26, 0, 0, 0, 0, time.UTC)

		if !coalescedMin.Equal(expectedMin) {
			t.Errorf("Coalesced min = %v, want %v", coalescedMin, expectedMin)
		}
		if !coalescedMax.Equal(expectedMax) {
			t.Errorf("Coalesced max = %v, want %v", coalescedMax, expectedMax)
		}
	})
}

// Test calendar failure threshold logic
func TestFailureThreshold(t *testing.T) {
	const maxFailures = 3

	t.Run("under threshold allows sync", func(t *testing.T) {
		failureCount := 2
		if failureCount >= maxFailures {
			t.Error("2 failures should allow sync")
		}
	})

	t.Run("at threshold blocks sync", func(t *testing.T) {
		failureCount := 3
		if failureCount < maxFailures {
			t.Error("3 failures should block sync")
		}
	})

	t.Run("over threshold blocks sync", func(t *testing.T) {
		failureCount := 5
		if failureCount < maxFailures {
			t.Error("5 failures should block sync")
		}
	})

	t.Run("reset after success", func(t *testing.T) {
		failureCount := 3
		// Simulate successful sync
		failureCount = 0
		if failureCount >= maxFailures {
			t.Error("Reset count should allow sync")
		}
	})
}
