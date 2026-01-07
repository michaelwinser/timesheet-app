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

// EventStore defines the interface for calendar event storage operations.
type EventStore interface {
	List(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time, status *store.ClassificationStatus, connectionID *uuid.UUID) ([]*store.CalendarEvent, error)
}

// TimeEntryStore defines the interface for time entry storage operations.
type TimeEntryStore interface {
	List(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time, projectID *uuid.UUID) ([]*store.TimeEntry, error)
	UpsertFromComputed(ctx context.Context, userID, projectID uuid.UUID, date time.Time, hours float64, title, description string, details []byte, eventIDs []uuid.UUID) (*store.TimeEntry, error)
	UpdateComputed(ctx context.Context, userID uuid.UUID, entryID uuid.UUID, hours float64, title, description string, details []byte, eventIDs []uuid.UUID) error
	Delete(ctx context.Context, userID, entryID uuid.UUID) error
}

// Service orchestrates time entry computation and persistence.
type Service struct {
	eventStore     EventStore
	timeEntryStore TimeEntryStore
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

	// Track which projects have computed entries
	computedProjects := make(map[uuid.UUID]bool)

	// Upsert each computed entry
	for _, c := range computed {
		computedProjects[c.ProjectID] = true

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

	// Clean up entries for projects that no longer have classified events
	// Get existing entries for this date
	existingEntries, err := s.timeEntryStore.List(ctx, userID, &startOfDay, &startOfDay, nil)
	if err != nil {
		return err
	}

	for _, entry := range existingEntries {
		// Skip if this project has computed entries
		if computedProjects[entry.ProjectID] {
			continue
		}

		// Skip if entry is protected (pinned, locked, or invoiced)
		if entry.IsPinned || entry.IsLocked || entry.InvoiceID != nil {
			// Update computed fields to show 0 hours and mark stale
			emptyDetails, _ := json.Marshal(map[string]interface{}{
				"events":        []interface{}{},
				"union_minutes": 0,
				"final_minutes": 0,
			})
			_ = s.timeEntryStore.UpdateComputed(ctx, userID, entry.ID, 0, "", "", emptyDetails, []uuid.UUID{})
			continue
		}

		// Delete unprotected entries that no longer have events
		_ = s.timeEntryStore.Delete(ctx, userID, entry.ID)
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

// ComputeForProjectAndDate computes time entry values for a specific project and date
// without persisting them. Used for auto-populating create forms and refresh operations.
// Returns nil if no classified events exist for the project on that date.
func (s *Service) ComputeForProjectAndDate(ctx context.Context, userID, projectID uuid.UUID, date time.Time) (*analyzer.ComputedTimeEntry, error) {
	// Get all classified events for this date and project
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.AddDate(0, 0, 1)

	// Get classified events only
	classifiedStatus := store.StatusClassified
	events, err := s.eventStore.List(ctx, userID, &startOfDay, &date, &classifiedStatus, nil)
	if err != nil {
		return nil, err
	}

	// Filter to events for this specific project
	var projectEvents []store.CalendarEvent
	for _, e := range events {
		if e.ProjectID != nil && *e.ProjectID == projectID && e.StartTime.Before(endOfDay) {
			projectEvents = append(projectEvents, *e)
		}
	}

	// If no events, return nil
	if len(projectEvents) == 0 {
		return nil, nil
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

	// Find the entry for this project
	for _, c := range computed {
		if c.ProjectID == projectID {
			return &c, nil
		}
	}

	return nil, nil
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
