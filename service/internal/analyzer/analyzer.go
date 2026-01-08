// Package analyzer provides pure functions for computing time entries from calendar events.
// Time entries are derived from classified events using clear, auditable logic.
package analyzer

import (
	"sort"
	"time"

	"github.com/google/uuid"
)

// Event represents a calendar event for time entry computation.
// This is a simplified view of the calendar event with only the fields needed for calculation.
type Event struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	Title     string
	StartTime time.Time
	EndTime   time.Time
	IsAllDay  bool
}

// ComputedTimeEntry represents a computed time entry for a project on a specific date.
type ComputedTimeEntry struct {
	ProjectID          uuid.UUID
	Date               time.Time
	Hours              float64
	Title              string
	Description        string
	ContributingEvents []uuid.UUID
	CalculationDetails CalculationDetails
}

// CalculationDetails provides an audit trail of how hours were calculated.
type CalculationDetails struct {
	Events         []EventDetail  `json:"events"`
	TimeRanges     []TimeRange    `json:"time_ranges"`
	UnionMinutes   int            `json:"union_minutes"`
	RoundingApplied string        `json:"rounding_applied"`
	FinalMinutes   int            `json:"final_minutes"`
}

// EventDetail captures details of an event that contributed to the time entry.
type EventDetail struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Start      string `json:"start"`
	End        string `json:"end"`
	RawMinutes int    `json:"raw_minutes"`
	IsAllDay   bool   `json:"is_all_day,omitempty"`
}

// TimeRange represents a unified time range after merging overlapping events.
type TimeRange struct {
	Start   string `json:"start"`
	End     string `json:"end"`
	Minutes int    `json:"minutes"`
}

// RoundingConfig specifies how to round time entries.
type RoundingConfig struct {
	GranularityMinutes int // e.g., 15 for 15-minute increments
	ThresholdMinutes   int // e.g., 7 means 0-6 round down, 7-14 round up
}

// DefaultRoundingConfig returns the default rounding configuration.
// 15-minute granularity with 1-minute threshold (always round up).
func DefaultRoundingConfig() RoundingConfig {
	return RoundingConfig{
		GranularityMinutes: 15,
		ThresholdMinutes:   1,
	}
}

// Compute calculates time entries for a given date from a list of classified events.
// Events are grouped by project, overlaps are unioned, and rounding is applied.
func Compute(date time.Time, events []Event, roundingCfg RoundingConfig) []ComputedTimeEntry {
	// Group events by project
	byProject := make(map[uuid.UUID][]Event)
	for _, e := range events {
		// Skip all-day events (they contribute 0 hours by default)
		// but we still track them for the audit trail
		byProject[e.ProjectID] = append(byProject[e.ProjectID], e)
	}

	var entries []ComputedTimeEntry
	for projectID, projectEvents := range byProject {
		entry := computeForProject(date, projectID, projectEvents, roundingCfg)
		entries = append(entries, entry)
	}

	// Sort by project ID for consistent output
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ProjectID.String() < entries[j].ProjectID.String()
	})

	return entries
}

// computeForProject calculates a single time entry for a project from its events.
func computeForProject(date time.Time, projectID uuid.UUID, events []Event, roundingCfg RoundingConfig) ComputedTimeEntry {
	// Separate all-day events from timed events
	var timedEvents []Event
	var allDayEvents []Event
	for _, e := range events {
		if e.IsAllDay {
			allDayEvents = append(allDayEvents, e)
		} else {
			timedEvents = append(timedEvents, e)
		}
	}

	// Build event details for audit trail
	details := CalculationDetails{
		Events: make([]EventDetail, 0, len(events)),
	}

	contributingEvents := make([]uuid.UUID, 0, len(events))

	// Add all events to the audit trail
	for _, e := range events {
		rawMinutes := 0
		if !e.IsAllDay {
			rawMinutes = int(e.EndTime.Sub(e.StartTime).Minutes())
		}
		details.Events = append(details.Events, EventDetail{
			ID:         e.ID.String(),
			Title:      e.Title,
			Start:      e.StartTime.Format(time.RFC3339),
			End:        e.EndTime.Format(time.RFC3339),
			RawMinutes: rawMinutes,
			IsAllDay:   e.IsAllDay,
		})
		contributingEvents = append(contributingEvents, e.ID)
	}

	// Compute time union for timed events
	var unionMinutes int
	if len(timedEvents) > 0 {
		ranges := computeTimeUnion(timedEvents)
		details.TimeRanges = ranges
		for _, r := range ranges {
			unionMinutes += r.Minutes
		}
	}
	details.UnionMinutes = unionMinutes

	// Apply rounding
	finalMinutes, roundingApplied := RoundMinutes(unionMinutes, roundingCfg)
	details.RoundingApplied = roundingApplied
	details.FinalMinutes = finalMinutes

	// Generate title from events
	title := generateTitle(events)

	return ComputedTimeEntry{
		ProjectID:          projectID,
		Date:               date,
		Hours:              float64(finalMinutes) / 60.0,
		Title:              title,
		Description:        generateDescription(events),
		ContributingEvents: contributingEvents,
		CalculationDetails: details,
	}
}

// computeTimeUnion merges overlapping time ranges and returns the unified ranges.
// This implements the union algorithm: events that overlap are merged into a single range.
func computeTimeUnion(events []Event) []TimeRange {
	if len(events) == 0 {
		return nil
	}

	// Sort events by start time
	sorted := make([]Event, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartTime.Before(sorted[j].StartTime)
	})

	// Merge overlapping ranges
	var ranges []TimeRange
	currentStart := sorted[0].StartTime
	currentEnd := sorted[0].EndTime

	for i := 1; i < len(sorted); i++ {
		e := sorted[i]
		if e.StartTime.Before(currentEnd) || e.StartTime.Equal(currentEnd) {
			// Overlapping or adjacent - extend current range
			if e.EndTime.After(currentEnd) {
				currentEnd = e.EndTime
			}
		} else {
			// Non-overlapping - save current range and start new one
			ranges = append(ranges, TimeRange{
				Start:   currentStart.Format("15:04"),
				End:     currentEnd.Format("15:04"),
				Minutes: int(currentEnd.Sub(currentStart).Minutes()),
			})
			currentStart = e.StartTime
			currentEnd = e.EndTime
		}
	}

	// Don't forget the last range
	ranges = append(ranges, TimeRange{
		Start:   currentStart.Format("15:04"),
		End:     currentEnd.Format("15:04"),
		Minutes: int(currentEnd.Sub(currentStart).Minutes()),
	})

	return ranges
}

// RoundMinutes applies rounding rules to minutes.
// Returns the rounded minutes and a description of the rounding applied.
func RoundMinutes(minutes int, cfg RoundingConfig) (int, string) {
	if cfg.GranularityMinutes <= 0 {
		return minutes, "none"
	}

	remainder := minutes % cfg.GranularityMinutes
	if remainder == 0 {
		return minutes, "none"
	}

	var rounded int
	var description string
	if remainder < cfg.ThresholdMinutes {
		// Round down
		rounded = minutes - remainder
		description = "-" + itoa(remainder) + "m"
	} else {
		// Round up
		roundUp := cfg.GranularityMinutes - remainder
		rounded = minutes + roundUp
		description = "+" + itoa(roundUp) + "m"
	}

	return rounded, description
}

// generateTitle creates a short title from the event(s).
func generateTitle(events []Event) string {
	if len(events) == 0 {
		return ""
	}

	// Use first event's title
	title := events[0].Title
	if len(events) > 1 {
		title += " +" + itoa(len(events)-1) + " more"
	}

	// Truncate if too long
	if len(title) > 50 {
		title = title[:47] + "..."
	}

	return title
}

// generateDescription creates a detailed description from the events.
func generateDescription(events []Event) string {
	if len(events) == 0 {
		return ""
	}

	// For now, just list event titles
	// This can be enhanced later with attendee info, etc.
	seen := make(map[string]bool)
	var titles []string
	for _, e := range events {
		if !seen[e.Title] {
			titles = append(titles, e.Title)
			seen[e.Title] = true
		}
	}

	desc := ""
	for i, t := range titles {
		if i > 0 {
			desc += ", "
		}
		desc += t
	}

	return desc
}

// itoa is a simple int to string conversion for small positive integers.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	if i < 0 {
		return "-" + itoa(-i)
	}

	var result []byte
	for i > 0 {
		result = append([]byte{byte('0' + i%10)}, result...)
		i /= 10
	}
	return string(result)
}
