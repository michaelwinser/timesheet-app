package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrCalendarEventNotFound = errors.New("calendar event not found")

type ClassificationStatus string
type ClassificationSource string

const (
	StatusPending    ClassificationStatus = "pending"
	StatusClassified ClassificationStatus = "classified"

	SourceRule        ClassificationSource = "rule"
	SourceFingerprint ClassificationSource = "fingerprint"
	SourceManual      ClassificationSource = "manual"
	SourceLLM         ClassificationSource = "llm"
)

// CalendarEvent represents a synced calendar event
type CalendarEvent struct {
	ID                       uuid.UUID
	ConnectionID             uuid.UUID
	CalendarID               *uuid.UUID // Reference to calendars table
	UserID                   uuid.UUID
	ExternalID               string
	Title                    string
	Description              *string
	StartTime                time.Time
	EndTime                  time.Time
	Attendees                []string
	IsRecurring              bool
	ResponseStatus           *string
	Transparency             *string
	IsOrphaned               bool
	IsSuppressed             bool
	IsLocked                 bool // Protection model: locked via Lock Day/Week
	IsSkipped                bool // Skip rules: exclude from time entries
	ClassificationStatus     ClassificationStatus
	ClassificationSource     *ClassificationSource
	ClassificationConfidence *float64
	NeedsReview              bool
	ProjectID                *uuid.UUID
	CreatedAt                time.Time
	UpdatedAt                time.Time
	// Joined data
	Project            *Project
	CalendarExternalID *string // Google Calendar ID (typically email)
	CalendarName       *string
	CalendarColor      *string
}

// CalendarEventStore provides PostgreSQL-backed event storage
type CalendarEventStore struct {
	pool *pgxpool.Pool
}

// NewCalendarEventStore creates a new store
func NewCalendarEventStore(pool *pgxpool.Pool) *CalendarEventStore {
	return &CalendarEventStore{pool: pool}
}

// Upsert creates or updates an event by external_id
func (s *CalendarEventStore) Upsert(ctx context.Context, event *CalendarEvent) (*CalendarEvent, error) {
	attendeesJSON, _ := json.Marshal(event.Attendees)
	now := time.Now().UTC()
	newID := uuid.New()

	err := s.pool.QueryRow(ctx, `
		INSERT INTO calendar_events (
			id, connection_id, calendar_id, user_id, external_id, title, description,
			start_time, end_time, attendees, is_recurring, response_status,
			transparency, is_orphaned, is_suppressed, classification_status,
			classification_source, project_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		ON CONFLICT (connection_id, external_id) DO UPDATE SET
			calendar_id = EXCLUDED.calendar_id,
			title = EXCLUDED.title,
			description = EXCLUDED.description,
			start_time = EXCLUDED.start_time,
			end_time = EXCLUDED.end_time,
			attendees = EXCLUDED.attendees,
			is_recurring = EXCLUDED.is_recurring,
			response_status = EXCLUDED.response_status,
			transparency = EXCLUDED.transparency,
			is_orphaned = false,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at, updated_at
	`,
		newID, event.ConnectionID, event.CalendarID, event.UserID, event.ExternalID,
		event.Title, event.Description, event.StartTime, event.EndTime,
		attendeesJSON, event.IsRecurring, event.ResponseStatus,
		event.Transparency, false, event.IsSuppressed, event.ClassificationStatus,
		event.ClassificationSource, event.ProjectID, now, now,
	).Scan(&event.ID, &event.CreatedAt, &event.UpdatedAt)

	if err != nil {
		return nil, err
	}

	return event, nil
}

// MarkOrphanedExcept marks events as orphaned if not in the given external IDs (legacy, uses connection_id)
func (s *CalendarEventStore) MarkOrphanedExcept(ctx context.Context, connectionID uuid.UUID, externalIDs []string) (int64, error) {
	result, err := s.pool.Exec(ctx, `
		UPDATE calendar_events
		SET is_orphaned = true, updated_at = $3
		WHERE connection_id = $1
		AND external_id != ALL($2)
		AND is_orphaned = false
	`, connectionID, externalIDs, time.Now().UTC())

	if err != nil {
		return 0, err
	}

	return result.RowsAffected(), nil
}

// MarkOrphanedExceptByCalendar marks events as orphaned if not in the given external IDs for a specific calendar
func (s *CalendarEventStore) MarkOrphanedExceptByCalendar(ctx context.Context, calendarID uuid.UUID, externalIDs []string) (int64, error) {
	result, err := s.pool.Exec(ctx, `
		UPDATE calendar_events
		SET is_orphaned = true, updated_at = $3
		WHERE calendar_id = $1
		AND external_id != ALL($2)
		AND is_orphaned = false
	`, calendarID, externalIDs, time.Now().UTC())

	if err != nil {
		return 0, err
	}

	return result.RowsAffected(), nil
}

// MarkOrphanedInRangeExceptByCalendar marks events as orphaned if not in the given external IDs,
// but only for events within the specified date range. Events outside the range are not affected.
func (s *CalendarEventStore) MarkOrphanedInRangeExceptByCalendar(ctx context.Context, calendarID uuid.UUID, externalIDs []string, minDate, maxDate time.Time) (int64, error) {
	result, err := s.pool.Exec(ctx, `
		UPDATE calendar_events
		SET is_orphaned = true, updated_at = $5
		WHERE calendar_id = $1
		AND external_id != ALL($2)
		AND is_orphaned = false
		AND start_time >= $3
		AND start_time < $4
	`, calendarID, externalIDs, minDate, maxDate, time.Now().UTC())

	if err != nil {
		return 0, err
	}

	return result.RowsAffected(), nil
}

// MarkOrphanedByExternalID marks a specific event as orphaned by its external ID (legacy, uses connection_id)
func (s *CalendarEventStore) MarkOrphanedByExternalID(ctx context.Context, connectionID uuid.UUID, externalID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE calendar_events
		SET is_orphaned = true, updated_at = $3
		WHERE connection_id = $1
		AND external_id = $2
	`, connectionID, externalID, time.Now().UTC())
	return err
}

// MarkOrphanedByExternalIDAndCalendar marks a specific event as orphaned by its external ID and calendar
func (s *CalendarEventStore) MarkOrphanedByExternalIDAndCalendar(ctx context.Context, calendarID uuid.UUID, externalID string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE calendar_events
		SET is_orphaned = true, updated_at = $3
		WHERE calendar_id = $1
		AND external_id = $2
	`, calendarID, externalID, time.Now().UTC())
	return err
}

// GetExternalIDsForConnection returns all external IDs for a connection
func (s *CalendarEventStore) GetExternalIDsForConnection(ctx context.Context, connectionID uuid.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT external_id FROM calendar_events WHERE connection_id = $1
	`, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// List returns events for a user with optional filters
func (s *CalendarEventStore) List(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time, status *ClassificationStatus, connectionID *uuid.UUID) ([]*CalendarEvent, error) {
	query := `
		SELECT ce.id, ce.connection_id, ce.calendar_id, ce.user_id, ce.external_id, ce.title, ce.description,
		       ce.start_time, ce.end_time, ce.attendees, ce.is_recurring, ce.response_status,
		       ce.transparency, ce.is_orphaned, ce.is_suppressed, ce.is_locked, ce.is_skipped,
		       ce.classification_status, ce.classification_source, ce.classification_confidence, ce.needs_review,
		       ce.project_id, ce.created_at, ce.updated_at,
		       p.id, p.user_id, p.name, p.short_code, p.client, p.color, p.is_billable, p.is_archived,
		       p.is_hidden_by_default, p.does_not_accumulate_hours, p.created_at, p.updated_at,
		       c.external_id, c.name, c.color
		FROM calendar_events ce
		LEFT JOIN projects p ON ce.project_id = p.id
		LEFT JOIN calendars c ON ce.calendar_id = c.id
		WHERE ce.user_id = $1 AND ce.is_orphaned = false
	`
	args := []interface{}{userID}
	argNum := 2

	if startDate != nil {
		query += fmt.Sprintf(" AND ce.start_time >= $%d", argNum)
		args = append(args, *startDate)
		argNum++
	}
	if endDate != nil {
		// Filter by start_time < end of endDate (next day at midnight)
		// This shows all events that START on or before endDate
		nextDay := endDate.AddDate(0, 0, 1)
		query += fmt.Sprintf(" AND ce.start_time < $%d", argNum)
		args = append(args, nextDay)
		argNum++
	}
	if status != nil {
		query += fmt.Sprintf(" AND ce.classification_status = $%d", argNum)
		args = append(args, *status)
		argNum++
	}
	if connectionID != nil {
		query += fmt.Sprintf(" AND ce.connection_id = $%d", argNum)
		args = append(args, *connectionID)
	}

	query += " ORDER BY ce.start_time ASC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*CalendarEvent
	for rows.Next() {
		e := &CalendarEvent{}
		var attendeesJSON []byte
		var pID, pUserID *uuid.UUID
		var pName, pShortCode, pClient, pColor *string
		var pIsBillable, pIsArchived, pIsHidden, pNoAccum *bool
		var pCreatedAt, pUpdatedAt *time.Time

		err := rows.Scan(
			&e.ID, &e.ConnectionID, &e.CalendarID, &e.UserID, &e.ExternalID, &e.Title, &e.Description,
			&e.StartTime, &e.EndTime, &attendeesJSON, &e.IsRecurring, &e.ResponseStatus,
			&e.Transparency, &e.IsOrphaned, &e.IsSuppressed, &e.IsLocked, &e.IsSkipped,
			&e.ClassificationStatus, &e.ClassificationSource, &e.ClassificationConfidence, &e.NeedsReview,
			&e.ProjectID, &e.CreatedAt, &e.UpdatedAt,
			&pID, &pUserID, &pName, &pShortCode, &pClient, &pColor, &pIsBillable, &pIsArchived,
			&pIsHidden, &pNoAccum, &pCreatedAt, &pUpdatedAt,
			&e.CalendarExternalID, &e.CalendarName, &e.CalendarColor,
		)
		if err != nil {
			return nil, err
		}

		json.Unmarshal(attendeesJSON, &e.Attendees)

		if pID != nil {
			e.Project = &Project{
				ID:                     *pID,
				UserID:                 *pUserID,
				Name:                   *pName,
				ShortCode:              pShortCode,
				Client:                 pClient,
				Color:                  *pColor,
				IsBillable:             *pIsBillable,
				IsArchived:             *pIsArchived,
				IsHiddenByDefault:      *pIsHidden,
				DoesNotAccumulateHours: *pNoAccum,
				CreatedAt:              *pCreatedAt,
				UpdatedAt:              *pUpdatedAt,
			}
		}

		events = append(events, e)
	}

	return events, rows.Err()
}

// CountByStatus returns counts of events by classification status and skip state
func (s *CalendarEventStore) CountByStatus(ctx context.Context, connectionID uuid.UUID) (pending, classified, skipped int, err error) {
	rows, err := s.pool.Query(ctx, `
		SELECT classification_status, is_skipped, COUNT(*)
		FROM calendar_events
		WHERE connection_id = $1 AND is_orphaned = false
		GROUP BY classification_status, is_skipped
	`, connectionID)
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()

	for rows.Next() {
		var status ClassificationStatus
		var isSkipped bool
		var count int
		if err := rows.Scan(&status, &isSkipped, &count); err != nil {
			return 0, 0, 0, err
		}
		if isSkipped {
			skipped += count
		} else {
			switch status {
			case StatusPending:
				pending += count
			case StatusClassified:
				classified += count
			}
		}
	}

	return pending, classified, skipped, rows.Err()
}

// ListForReclassification returns classified events that are eligible for re-evaluation.
// These are events classified by rule or fingerprint (not manual) that are not locked.
// Per the PRD, unlocked items should update freely when rules/fingerprints change.
func (s *CalendarEventStore) ListForReclassification(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time) ([]*CalendarEvent, error) {
	query := `
		SELECT ce.id, ce.connection_id, ce.calendar_id, ce.user_id, ce.external_id, ce.title, ce.description,
		       ce.start_time, ce.end_time, ce.attendees, ce.is_recurring, ce.response_status,
		       ce.transparency, ce.is_orphaned, ce.is_suppressed, ce.is_locked, ce.is_skipped,
		       ce.classification_status, ce.classification_source, ce.classification_confidence, ce.needs_review,
		       ce.project_id, ce.created_at, ce.updated_at,
		       p.id, p.user_id, p.name, p.short_code, p.client, p.color, p.is_billable, p.is_archived,
		       p.is_hidden_by_default, p.does_not_accumulate_hours, p.created_at, p.updated_at,
		       c.external_id, c.name, c.color
		FROM calendar_events ce
		LEFT JOIN projects p ON ce.project_id = p.id
		LEFT JOIN calendars c ON ce.calendar_id = c.id
		WHERE ce.user_id = $1
		  AND ce.is_orphaned = false
		  AND ce.classification_status = 'classified'
		  AND ce.classification_source IN ('rule', 'fingerprint')
		  AND ce.is_locked = false
	`
	args := []interface{}{userID}
	argNum := 2

	if startDate != nil {
		query += fmt.Sprintf(" AND ce.start_time >= $%d", argNum)
		args = append(args, *startDate)
		argNum++
	}
	if endDate != nil {
		nextDay := endDate.AddDate(0, 0, 1)
		query += fmt.Sprintf(" AND ce.start_time < $%d", argNum)
		args = append(args, nextDay)
	}

	query += " ORDER BY ce.start_time ASC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*CalendarEvent
	for rows.Next() {
		e := &CalendarEvent{}
		var attendeesJSON []byte
		var project Project
		var projectID, projectUserID *uuid.UUID
		var projectName, projectShortCode, projectClient, projectColor *string
		var projectIsBillable, projectIsArchived, projectIsHiddenByDefault, projectDoesNotAccumulateHours *bool
		var projectCreatedAt, projectUpdatedAt *time.Time
		var calExternalID, calName, calColor *string

		err := rows.Scan(
			&e.ID, &e.ConnectionID, &e.CalendarID, &e.UserID, &e.ExternalID, &e.Title, &e.Description,
			&e.StartTime, &e.EndTime, &attendeesJSON, &e.IsRecurring, &e.ResponseStatus,
			&e.Transparency, &e.IsOrphaned, &e.IsSuppressed, &e.IsLocked, &e.IsSkipped,
			&e.ClassificationStatus, &e.ClassificationSource, &e.ClassificationConfidence, &e.NeedsReview,
			&e.ProjectID, &e.CreatedAt, &e.UpdatedAt,
			&projectID, &projectUserID, &projectName, &projectShortCode, &projectClient, &projectColor,
			&projectIsBillable, &projectIsArchived, &projectIsHiddenByDefault, &projectDoesNotAccumulateHours,
			&projectCreatedAt, &projectUpdatedAt,
			&calExternalID, &calName, &calColor,
		)
		if err != nil {
			return nil, err
		}

		if attendeesJSON != nil {
			json.Unmarshal(attendeesJSON, &e.Attendees)
		}

		if projectID != nil {
			project.ID = *projectID
			if projectUserID != nil {
				project.UserID = *projectUserID
			}
			if projectName != nil {
				project.Name = *projectName
			}
			if projectShortCode != nil {
				project.ShortCode = projectShortCode
			}
			if projectClient != nil {
				project.Client = projectClient
			}
			if projectColor != nil {
				project.Color = *projectColor
			}
			if projectIsBillable != nil {
				project.IsBillable = *projectIsBillable
			}
			if projectIsArchived != nil {
				project.IsArchived = *projectIsArchived
			}
			if projectIsHiddenByDefault != nil {
				project.IsHiddenByDefault = *projectIsHiddenByDefault
			}
			if projectDoesNotAccumulateHours != nil {
				project.DoesNotAccumulateHours = *projectDoesNotAccumulateHours
			}
			if projectCreatedAt != nil {
				project.CreatedAt = *projectCreatedAt
			}
			if projectUpdatedAt != nil {
				project.UpdatedAt = *projectUpdatedAt
			}
			e.Project = &project
		}

		if calName != nil {
			e.CalendarName = calName
		}

		events = append(events, e)
	}

	return events, rows.Err()
}

// GetByID retrieves an event by ID
func (s *CalendarEventStore) GetByID(ctx context.Context, userID, eventID uuid.UUID) (*CalendarEvent, error) {
	e := &CalendarEvent{}
	var attendeesJSON []byte

	err := s.pool.QueryRow(ctx, `
		SELECT id, connection_id, user_id, external_id, title, description,
		       start_time, end_time, attendees, is_recurring, response_status,
		       transparency, is_orphaned, is_suppressed, is_locked, is_skipped,
		       classification_status, classification_source, classification_confidence, needs_review,
		       project_id, created_at, updated_at
		FROM calendar_events
		WHERE id = $1 AND user_id = $2
	`, eventID, userID).Scan(
		&e.ID, &e.ConnectionID, &e.UserID, &e.ExternalID, &e.Title, &e.Description,
		&e.StartTime, &e.EndTime, &attendeesJSON, &e.IsRecurring, &e.ResponseStatus,
		&e.Transparency, &e.IsOrphaned, &e.IsSuppressed, &e.IsLocked, &e.IsSkipped,
		&e.ClassificationStatus, &e.ClassificationSource, &e.ClassificationConfidence, &e.NeedsReview,
		&e.ProjectID, &e.CreatedAt, &e.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarEventNotFound
		}
		return nil, err
	}

	json.Unmarshal(attendeesJSON, &e.Attendees)
	return e, nil
}

// Classify updates an event's classification status and project assignment
func (s *CalendarEventStore) Classify(ctx context.Context, userID, eventID uuid.UUID, projectID *uuid.UUID, skip bool) (*CalendarEvent, error) {
	now := time.Now().UTC()
	source := SourceManual

	// Determine classification status based on whether a project is assigned
	status := StatusPending
	if projectID != nil {
		status = StatusClassified
	}

	// Manual classification clears needs_review and sets confidence to 1.0
	result, err := s.pool.Exec(ctx, `
		UPDATE calendar_events
		SET classification_status = $3,
		    classification_source = $4,
		    classification_confidence = 1.0,
		    needs_review = false,
		    project_id = $5,
		    is_skipped = $6,
		    updated_at = $7
		WHERE id = $1 AND user_id = $2
	`, eventID, userID, status, source, projectID, skip, now)

	if err != nil {
		return nil, err
	}

	if result.RowsAffected() == 0 {
		return nil, ErrCalendarEventNotFound
	}

	return s.GetByID(ctx, userID, eventID)
}

// SetSkipped updates just the is_skipped field for an event.
// Used by the skip pass in ApplyRules.
func (s *CalendarEventStore) SetSkipped(ctx context.Context, userID, eventID uuid.UUID, skip bool, source ClassificationSource) error {
	now := time.Now().UTC()

	result, err := s.pool.Exec(ctx, `
		UPDATE calendar_events
		SET is_skipped = $3,
		    classification_source = COALESCE(classification_source, $4),
		    updated_at = $5
		WHERE id = $1 AND user_id = $2
	`, eventID, userID, skip, source, now)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrCalendarEventNotFound
	}

	return nil
}

// ClassifyByRule updates an event's classification from a rule or fingerprint.
// Unlike Classify (which is for manual classification), this sets the specified source.
func (s *CalendarEventStore) ClassifyByRule(ctx context.Context, userID, eventID uuid.UUID, projectID uuid.UUID, source ClassificationSource, confidence float64, needsReview bool) error {
	now := time.Now().UTC()

	result, err := s.pool.Exec(ctx, `
		UPDATE calendar_events
		SET classification_status = 'classified',
		    classification_source = $3,
		    classification_confidence = $4,
		    needs_review = $5,
		    project_id = $6,
		    updated_at = $7
		WHERE id = $1 AND user_id = $2
	`, eventID, userID, source, confidence, needsReview, projectID, now)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrCalendarEventNotFound
	}

	return nil
}
