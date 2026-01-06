// Package timeentry provides the orchestration layer for computing and persisting time entries.
// It uses the analyzer package for pure computation and the store for persistence.
package timeentry

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/michaelw/timesheet-app/service/internal/analyzer"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

// Service orchestrates time entry computation and persistence.
type Service struct {
	eventStore     *store.CalendarEventStore
	timeEntryStore *store.TimeEntryStore
	roundingConfig analyzer.RoundingConfig
}

// NewService creates a new time entry service.
func NewService(eventStore *store.CalendarEventStore, timeEntryStore *store.TimeEntryStore) *Service {
	return &Service{
		eventStore:     eventStore,
		timeEntryStore: timeEntryStore,
		roundingConfig: analyzer.DefaultRoundingConfig(),
	}
}

// RecalculateForDate recomputes all time entries for a specific date.
// This is called after calendar sync or event classification changes.
func (s *Service) RecalculateForDate(ctx context.Context, userID uuid.UUID, date time.Time) error {
	// Get all classified events for this date
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.AddDate(0, 0, 1)

	// Get classified events only (not pending or skipped)
	classifiedStatus := store.StatusClassified
	events, err := s.eventStore.List(ctx, userID, &startOfDay, &date, &classifiedStatus, nil)
	if err != nil {
		return err
	}

	// Filter to only events that have a project assigned
	var projectEvents []store.CalendarEvent
	for _, e := range events {
		if e.ProjectID != nil && e.StartTime.Before(endOfDay) {
			projectEvents = append(projectEvents, *e)
		}
	}

	// Convert to analyzer events
	analyzerEvents := make([]analyzer.Event, 0, len(projectEvents))
	for _, e := range projectEvents {
		isAllDay := isAllDayEvent(e.StartTime, e.EndTime)
		analyzerEvents = append(analyzerEvents, analyzer.Event{
			ID:        e.ID,
			ProjectID: *e.ProjectID,
			Title:     e.Title,
			StartTime: e.StartTime,
			EndTime:   e.EndTime,
			IsAllDay:  isAllDay,
		})
	}

	// Compute time entries using the analyzer
	computed := analyzer.Compute(startOfDay, analyzerEvents, s.roundingConfig)

	// Upsert each computed entry
	for _, c := range computed {
		details, err := json.Marshal(c.CalculationDetails)
		if err != nil {
			return err
		}

		_, err = s.timeEntryStore.UpsertFromComputed(
			ctx,
			userID,
			c.ProjectID,
			c.Date,
			c.Hours,
			c.Title,
			c.Description,
			details,
			c.ContributingEvents,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// RecalculateForDateRange recomputes time entries for a range of dates.
// Used after bulk operations like calendar sync.
func (s *Service) RecalculateForDateRange(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time) error {
	current := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, time.UTC)

	for !current.After(end) {
		if err := s.RecalculateForDate(ctx, userID, current); err != nil {
			return err
		}
		current = current.AddDate(0, 0, 1)
	}

	return nil
}

// RecalculateForEvent recomputes the time entry affected by a specific event.
// Called after a single event is classified.
func (s *Service) RecalculateForEvent(ctx context.Context, userID uuid.UUID, event *store.CalendarEvent) error {
	// Extract the date from the event
	eventDate := time.Date(
		event.StartTime.Year(),
		event.StartTime.Month(),
		event.StartTime.Day(),
		0, 0, 0, 0, time.UTC,
	)

	return s.RecalculateForDate(ctx, userID, eventDate)
}

// isAllDayEvent heuristically determines if an event is all-day.
// All-day events typically span exactly 24 hours starting at midnight.
func isAllDayEvent(start, end time.Time) bool {
	// Check if start is at midnight (any timezone)
	if start.Hour() == 0 && start.Minute() == 0 && start.Second() == 0 {
		// Check if duration is exactly 24 hours
		if end.Sub(start) == 24*time.Hour {
			return true
		}
	}
	return false
}
