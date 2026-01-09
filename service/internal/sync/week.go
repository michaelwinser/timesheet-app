// Package sync provides calendar synchronization utilities and scheduling.
package sync

import "time"

// NormalizeToWeekStart returns the Monday 00:00:00 UTC of the week containing the given date.
func NormalizeToWeekStart(d time.Time) time.Time {
	d = d.UTC().Truncate(24 * time.Hour)
	weekday := int(d.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday = 7
	}
	return d.AddDate(0, 0, -(weekday - 1)) // Back to Monday
}

// NormalizeToWeekEnd returns the Sunday 23:59:59 UTC of the week containing the given date.
func NormalizeToWeekEnd(d time.Time) time.Time {
	monday := NormalizeToWeekStart(d)
	return monday.AddDate(0, 0, 6).Add(24*time.Hour - time.Second) // Sunday 23:59:59
}

// IsWeekWithinRange checks if a week (represented by any date within it) falls
// within the given min/max synced date range (water marks).
func IsWeekWithinRange(weekDate time.Time, minSynced, maxSynced *time.Time) bool {
	if minSynced == nil || maxSynced == nil {
		return false
	}

	weekStart := NormalizeToWeekStart(weekDate)
	weekEnd := NormalizeToWeekEnd(weekDate)

	// Week is within range if its start is >= minSynced and its end is <= maxSynced
	return !weekStart.Before(*minSynced) && !weekEnd.After(*maxSynced)
}

// StalenessThreshold is the duration after which synced data is considered stale.
const StalenessThreshold = 24 * time.Hour

// IsStale checks if the last sync time is older than the staleness threshold.
func IsStale(lastSyncedAt *time.Time) bool {
	if lastSyncedAt == nil {
		return true
	}
	return time.Since(*lastSyncedAt) > StalenessThreshold
}

// DefaultInitialWindow returns the default sync window for new calendars.
// Returns (startDate, endDate) representing -4 weeks to +1 week from now.
func DefaultInitialWindow() (time.Time, time.Time) {
	now := time.Now().UTC()
	start := NormalizeToWeekStart(now.AddDate(0, 0, -28)) // -4 weeks
	end := NormalizeToWeekEnd(now.AddDate(0, 0, 7))       // +1 week
	return start, end
}

// DefaultBackgroundWindow returns the target sync window for background expansion.
// Returns (startDate, endDate) representing -52 weeks to +5 weeks from now.
func DefaultBackgroundWindow() (time.Time, time.Time) {
	now := time.Now().UTC()
	start := NormalizeToWeekStart(now.AddDate(0, 0, -364)) // -52 weeks
	end := NormalizeToWeekEnd(now.AddDate(0, 0, 35))       // +5 weeks
	return start, end
}

// WeeksInRange returns a slice of week start dates between start and end (inclusive).
func WeeksInRange(start, end time.Time) []time.Time {
	start = NormalizeToWeekStart(start)
	end = NormalizeToWeekStart(end)

	var weeks []time.Time
	for current := start; !current.After(end); current = current.AddDate(0, 0, 7) {
		weeks = append(weeks, current)
	}
	return weeks
}

// MissingWeeks returns the weeks that are outside the current synced range.
// Returns weeks that need to be fetched to cover targetStart to targetEnd.
func MissingWeeks(minSynced, maxSynced *time.Time, targetStart, targetEnd time.Time) []time.Time {
	targetStart = NormalizeToWeekStart(targetStart)
	targetEnd = NormalizeToWeekStart(targetEnd)

	// If no existing range, all weeks are missing
	if minSynced == nil || maxSynced == nil {
		return WeeksInRange(targetStart, targetEnd)
	}

	var missing []time.Time

	// Weeks before the current min
	if targetStart.Before(*minSynced) {
		beforeEnd := NormalizeToWeekStart(minSynced.AddDate(0, 0, -7))
		if !beforeEnd.Before(targetStart) {
			missing = append(missing, WeeksInRange(targetStart, beforeEnd)...)
		}
	}

	// Weeks after the current max
	if targetEnd.After(*maxSynced) {
		afterStart := NormalizeToWeekStart(maxSynced.AddDate(0, 0, 7))
		if !afterStart.After(targetEnd) {
			missing = append(missing, WeeksInRange(afterStart, targetEnd)...)
		}
	}

	return missing
}
