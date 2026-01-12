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

	// Filter to only events that have a project assigned, are not skipped,
	// and the project accumulates hours
	var projectEvents []store.CalendarEvent
	for _, e := range events {
		if e.ProjectID != nil && !e.IsSkipped && e.StartTime.Before(endOfDay) {
			// Skip events from projects that don't accumulate hours
			if e.Project != nil && e.Project.DoesNotAccumulateHours {
				continue
			}
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

		// Skip if entry is protected (pinned, locked, invoiced, or has user edits)
		// Per PRD: preserve entries if user edited anything, just mark them stale
		if entry.IsPinned || entry.IsLocked || entry.InvoiceID != nil || entry.HasUserEdits {
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
		// This only happens for auto-created entries with no user modifications
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

	// Filter to events for this specific project that are not skipped
	var projectEvents []store.CalendarEvent
	for _, e := range events {
		if e.ProjectID != nil && *e.ProjectID == projectID && !e.IsSkipped && e.StartTime.Before(endOfDay) {
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

// ListWithEphemeral returns time entries for a date range, combining:
// - Materialized entries (stored in DB with user state)
// - Ephemeral entries (computed on-demand from classified events)
//
// Materialized entries take precedence. Ephemeral entries fill gaps where
// no materialized entry exists for a (project, date) combination.
// Suppressed entries are excluded from the result.
func (s *Service) ListWithEphemeral(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time, projectID *uuid.UUID) ([]*store.TimeEntry, error) {
	// Get materialized entries from DB
	materialized, err := s.timeEntryStore.List(ctx, userID, startDate, endDate, projectID)
	if err != nil {
		return nil, err
	}

	// Build a map of materialized entries by (project_id, date) for quick lookup
	materializedMap := make(map[string]*store.TimeEntry)
	for _, e := range materialized {
		key := e.ProjectID.String() + "|" + e.Date.Format("2006-01-02")
		materializedMap[key] = e
	}

	// If no date range provided, just return materialized entries
	// (can't compute ephemeral without knowing the range)
	if startDate == nil || endDate == nil {
		// Filter out suppressed entries
		result := make([]*store.TimeEntry, 0, len(materialized))
		for _, e := range materialized {
			if !e.IsSuppressed {
				result = append(result, e)
			}
		}
		return result, nil
	}

	// Compute ephemeral entries from classified events
	ephemeral, err := s.computeEphemeralForRange(ctx, userID, *startDate, *endDate, projectID)
	if err != nil {
		return nil, err
	}

	// Build ephemeral map for updating computed values on materialized entries
	ephemeralMap := make(map[string]*store.TimeEntry)
	for _, e := range ephemeral {
		key := e.ProjectID.String() + "|" + e.Date.Format("2006-01-02")
		ephemeralMap[key] = e
	}

	// Merge: start with ephemeral, override with materialized
	result := make([]*store.TimeEntry, 0, len(materialized)+len(ephemeral))

	// Add ephemeral entries that don't have a materialized counterpart
	for _, e := range ephemeral {
		key := e.ProjectID.String() + "|" + e.Date.Format("2006-01-02")
		if _, exists := materializedMap[key]; !exists {
			result = append(result, e)
		}
	}

	// Add materialized entries with fresh computed values (excluding suppressed)
	for _, e := range materialized {
		if e.IsSuppressed {
			continue
		}
		// Update computed values from ephemeral if available
		key := e.ProjectID.String() + "|" + e.Date.Format("2006-01-02")
		if eph, exists := ephemeralMap[key]; exists {
			// Copy fresh computed values to materialized entry
			e.ComputedHours = eph.ComputedHours
			e.ComputedTitle = eph.ComputedTitle
			e.ComputedDescription = eph.ComputedDescription
			e.CalculationDetails = eph.CalculationDetails
			e.ContributingEvents = eph.ContributingEvents
		} else {
			// No events for this entry anymore - computed is 0
			zero := 0.0
			empty := ""
			e.ComputedHours = &zero
			e.ComputedTitle = &empty
			e.ComputedDescription = &empty
			e.ContributingEvents = nil
		}
		result = append(result, e)
	}

	return result, nil
}

// computeEphemeralForRange computes ephemeral time entries from classified events
// for a date range. These are not persisted - they exist only in memory.
func (s *Service) computeEphemeralForRange(ctx context.Context, userID uuid.UUID, startDate, endDate time.Time, projectID *uuid.UUID) ([]*store.TimeEntry, error) {
	// Get classified events for the date range
	classifiedStatus := store.StatusClassified
	events, err := s.eventStore.List(ctx, userID, &startDate, &endDate, &classifiedStatus, nil)
	if err != nil {
		return nil, err
	}

	// Filter events: must have project, not skipped, project must accumulate hours
	var projectEvents []store.CalendarEvent
	for _, e := range events {
		if e.ProjectID == nil || e.IsSkipped {
			continue
		}
		// Skip events from projects that don't accumulate hours
		if e.Project != nil && e.Project.DoesNotAccumulateHours {
			continue
		}
		if projectID != nil && *e.ProjectID != *projectID {
			continue
		}
		projectEvents = append(projectEvents, *e)
	}

	if len(projectEvents) == 0 {
		return nil, nil
	}

	// Group events by date and compute entries for each date
	eventsByDate := make(map[string][]store.CalendarEvent)
	for _, e := range projectEvents {
		dateKey := e.StartTime.Format("2006-01-02")
		eventsByDate[dateKey] = append(eventsByDate[dateKey], e)
	}

	var result []*store.TimeEntry
	for dateStr, dayEvents := range eventsByDate {
		date, _ := time.Parse("2006-01-02", dateStr)
		startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

		// Convert to analyzer events
		analyzerEvents := make([]analyzer.Event, 0, len(dayEvents))
		for _, e := range dayEvents {
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

		// Compute time entries for this day
		computed := analyzer.Compute(startOfDay, analyzerEvents, s.roundingConfig)

		// Convert to store.TimeEntry (ephemeral - deterministic ID, not persisted)
		for _, c := range computed {
			details, _ := json.Marshal(c.CalculationDetails)
			hours := c.Hours
			// Generate a deterministic ID for ephemeral entries using UUID v5
			// This ensures the same (user, project, date) always gets the same ID
			ephemeralID := generateEphemeralID(userID, c.ProjectID, c.Date)
			entry := &store.TimeEntry{
				ID:                  ephemeralID,
				UserID:              userID,
				ProjectID:           c.ProjectID,
				Date:                c.Date,
				Hours:               c.Hours,
				Title:               &c.Title,
				Description:         &c.Description,
				Source:              "calendar",
				HasUserEdits:        false,
				ComputedHours:       &hours,
				ComputedTitle:       &c.Title,
				ComputedDescription: &c.Description,
				CalculationDetails:  details,
				ContributingEvents:  c.ContributingEvents,
				CreatedAt:           time.Now().UTC(),
				UpdatedAt:           time.Now().UTC(),
			}
			result = append(result, entry)
		}
	}

	return result, nil
}

// MaterializeForRange ensures all computed time entries for a date range are
// persisted in the database. This is called before invoice creation to ensure
// entries exist with proper IDs for invoice line items.
//
// For invoicing purposes, this also creates 0h placeholder entries for any
// days in the range that have no time entry. This allows the invoice to "lock"
// the entire date range and prevent inadvertent edits.
//
// Returns the list of materialized entry IDs (both newly created and existing).
func (s *Service) MaterializeForRange(ctx context.Context, userID uuid.UUID, projectID uuid.UUID, startDate, endDate time.Time) ([]uuid.UUID, error) {
	// Get materialized entries from DB
	materialized, err := s.timeEntryStore.List(ctx, userID, &startDate, &endDate, &projectID)
	if err != nil {
		return nil, err
	}

	// Build map of existing entries by date
	existingByDate := make(map[string]*store.TimeEntry)
	for _, e := range materialized {
		key := e.Date.Format("2006-01-02")
		existingByDate[key] = e
	}

	// Compute ephemeral entries
	ephemeral, err := s.computeEphemeralForRange(ctx, userID, startDate, endDate, &projectID)
	if err != nil {
		return nil, err
	}

	// Build map of ephemeral entries by date
	ephemeralByDate := make(map[string]*store.TimeEntry)
	for _, e := range ephemeral {
		key := e.Date.Format("2006-01-02")
		ephemeralByDate[key] = e
	}

	var resultIDs []uuid.UUID

	// Iterate through all days in the range
	current := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, time.UTC)
	end := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, time.UTC)

	for !current.After(end) {
		dateKey := current.Format("2006-01-02")

		if existing, exists := existingByDate[dateKey]; exists {
			// Entry already exists in DB - use its ID
			resultIDs = append(resultIDs, existing.ID)
		} else if eph, exists := ephemeralByDate[dateKey]; exists {
			// Ephemeral entry exists - materialize it
			entry, err := s.materializeEntry(ctx, userID, eph)
			if err != nil {
				return nil, err
			}
			resultIDs = append(resultIDs, entry.ID)
		} else {
			// No entry for this day - create a 0h placeholder entry
			// This allows the invoice to lock this day and prevent inadvertent edits
			emptyDetails, _ := json.Marshal(map[string]interface{}{
				"events":        []interface{}{},
				"union_minutes": 0,
				"final_minutes": 0,
			})
			entry, err := s.timeEntryStore.UpsertFromComputed(
				ctx,
				userID,
				projectID,
				current,
				0,    // 0 hours
				"",   // empty title
				"",   // empty description
				emptyDetails,
				nil, // no contributing events
			)
			if err != nil {
				return nil, err
			}
			resultIDs = append(resultIDs, entry.ID)
		}

		current = current.AddDate(0, 0, 1)
	}

	return resultIDs, nil
}

// materializeEntry creates a time entry in the database from computed values.
// This is used when invoicing to ensure ephemeral entries have proper IDs.
func (s *Service) materializeEntry(ctx context.Context, userID uuid.UUID, eph *store.TimeEntry) (*store.TimeEntry, error) {
	title := ""
	if eph.Title != nil {
		title = *eph.Title
	}
	description := ""
	if eph.Description != nil {
		description = *eph.Description
	}

	// Use UpsertFromComputed to create the entry with proper computed fields
	entry, err := s.timeEntryStore.UpsertFromComputed(
		ctx,
		userID,
		eph.ProjectID,
		eph.Date,
		eph.Hours,
		title,
		description,
		eph.CalculationDetails,
		eph.ContributingEvents,
	)
	if err != nil {
		return nil, err
	}

	return entry, nil
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

// Namespace UUID for generating ephemeral time entry IDs.
// This is a fixed UUID used as the namespace for UUID v5 generation.
var ephemeralNamespace = uuid.MustParse("a1b2c3d4-e5f6-7890-abcd-ef1234567890")

// generateEphemeralID creates a deterministic UUID for ephemeral time entries.
// The same (userID, projectID, date) will always produce the same ID.
// This allows the frontend to work with ephemeral entries consistently.
func generateEphemeralID(userID, projectID uuid.UUID, date time.Time) uuid.UUID {
	// Create a unique name from the combination of user, project, and date
	name := userID.String() + "|" + projectID.String() + "|" + date.Format("2006-01-02")
	return uuid.NewSHA1(ephemeralNamespace, []byte(name))
}
