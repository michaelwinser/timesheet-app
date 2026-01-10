package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrTimeEntryNotFound = errors.New("time entry not found")
	ErrTimeEntryInvoiced = errors.New("time entry is invoiced")
)

// TimeEntry represents a stored time entry
type TimeEntry struct {
	ID           uuid.UUID
	UserID       uuid.UUID
	ProjectID    uuid.UUID
	Date         time.Time
	Hours        float64
	Title        *string
	Description  *string
	Source       string
	InvoiceID    *uuid.UUID
	HasUserEdits bool
	// Protection model fields
	IsPinned     bool
	IsLocked     bool
	IsStale      bool
	IsSuppressed bool // User explicitly suppressed this entry
	// Computed fields (from analyzer)
	ComputedHours         *float64
	ComputedTitle         *string
	ComputedDescription   *string
	SnapshotComputedHours *float64 // Computed hours at materialization time
	CalculationDetails    []byte   // JSONB stored as bytes
	CreatedAt             time.Time
	UpdatedAt             time.Time
	// Joined data
	Project            *Project
	ContributingEvents []uuid.UUID // From junction table
}

// TimeEntryStore provides PostgreSQL-backed time entry storage
type TimeEntryStore struct {
	pool *pgxpool.Pool
}

// NewTimeEntryStore creates a new PostgreSQL time entry store
func NewTimeEntryStore(pool *pgxpool.Pool) *TimeEntryStore {
	return &TimeEntryStore{pool: pool}
}

// Create adds a new time entry or updates if one exists for the same project/date
// On upsert, captures snapshot_computed_hours for staleness detection
func (s *TimeEntryStore) Create(ctx context.Context, userID, projectID uuid.UUID, date time.Time, hours float64, description *string) (*TimeEntry, error) {
	entry := &TimeEntry{
		ID:           uuid.New(),
		UserID:       userID,
		ProjectID:    projectID,
		Date:         date,
		Hours:        hours,
		Description:  description,
		Source:       "manual",
		HasUserEdits: true,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	// Use upsert - if entry exists for same project/date, add hours
	// On conflict, capture snapshot_computed_hours to anchor staleness detection
	_, err := s.pool.Exec(ctx, `
		INSERT INTO time_entries (id, user_id, project_id, date, hours, description, source, has_user_edits, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, project_id, date) DO UPDATE SET
			hours = time_entries.hours + EXCLUDED.hours,
			description = COALESCE(EXCLUDED.description, time_entries.description),
			has_user_edits = true,
			snapshot_computed_hours = time_entries.computed_hours,
			updated_at = EXCLUDED.updated_at
	`, entry.ID, entry.UserID, entry.ProjectID, entry.Date, entry.Hours,
		entry.Description, entry.Source, entry.HasUserEdits, entry.CreatedAt, entry.UpdatedAt)

	if err != nil {
		return nil, err
	}

	// Fetch the actual entry (in case it was an update)
	return s.GetByProjectAndDate(ctx, userID, projectID, date)
}

// GetByID retrieves a time entry by ID for a specific user
func (s *TimeEntryStore) GetByID(ctx context.Context, userID, entryID uuid.UUID) (*TimeEntry, error) {
	entry := &TimeEntry{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, project_id, date, hours, title, description, source, invoice_id, has_user_edits,
		       is_pinned, is_locked, is_stale, is_suppressed,
		       computed_hours, computed_title, computed_description, snapshot_computed_hours,
		       calculation_details, created_at, updated_at
		FROM time_entries WHERE id = $1 AND user_id = $2
	`, entryID, userID).Scan(
		&entry.ID, &entry.UserID, &entry.ProjectID, &entry.Date, &entry.Hours,
		&entry.Title, &entry.Description, &entry.Source, &entry.InvoiceID, &entry.HasUserEdits,
		&entry.IsPinned, &entry.IsLocked, &entry.IsStale, &entry.IsSuppressed,
		&entry.ComputedHours, &entry.ComputedTitle, &entry.ComputedDescription, &entry.SnapshotComputedHours,
		&entry.CalculationDetails, &entry.CreatedAt, &entry.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTimeEntryNotFound
		}
		return nil, err
	}
	return entry, nil
}

// GetByProjectAndDate retrieves a time entry by project and date
func (s *TimeEntryStore) GetByProjectAndDate(ctx context.Context, userID, projectID uuid.UUID, date time.Time) (*TimeEntry, error) {
	entry := &TimeEntry{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, project_id, date, hours, title, description, source, invoice_id, has_user_edits,
		       is_pinned, is_locked, is_stale, is_suppressed,
		       computed_hours, computed_title, computed_description, snapshot_computed_hours,
		       calculation_details, created_at, updated_at
		FROM time_entries WHERE user_id = $1 AND project_id = $2 AND date = $3
	`, userID, projectID, date).Scan(
		&entry.ID, &entry.UserID, &entry.ProjectID, &entry.Date, &entry.Hours,
		&entry.Title, &entry.Description, &entry.Source, &entry.InvoiceID, &entry.HasUserEdits,
		&entry.IsPinned, &entry.IsLocked, &entry.IsStale, &entry.IsSuppressed,
		&entry.ComputedHours, &entry.ComputedTitle, &entry.ComputedDescription, &entry.SnapshotComputedHours,
		&entry.CalculationDetails, &entry.CreatedAt, &entry.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTimeEntryNotFound
		}
		return nil, err
	}
	return entry, nil
}

// List retrieves time entries for a user with optional filters
func (s *TimeEntryStore) List(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time, projectID *uuid.UUID) ([]*TimeEntry, error) {
	query := `
		SELECT te.id, te.user_id, te.project_id, te.date, te.hours, te.title, te.description,
		       te.source, te.invoice_id, te.has_user_edits,
		       te.is_pinned, te.is_locked, te.is_stale, te.is_suppressed,
		       te.computed_hours, te.computed_title, te.computed_description, te.snapshot_computed_hours,
		       te.calculation_details, te.created_at, te.updated_at,
		       p.id, p.user_id, p.name, p.short_code, p.color, p.is_billable, p.is_archived,
		       p.is_hidden_by_default, p.does_not_accumulate_hours, p.created_at, p.updated_at
		FROM time_entries te
		JOIN projects p ON te.project_id = p.id
		WHERE te.user_id = $1
	`
	args := []interface{}{userID}
	argNum := 2

	if startDate != nil {
		query += " AND te.date >= $" + string(rune('0'+argNum))
		args = append(args, *startDate)
		argNum++
	}
	if endDate != nil {
		query += " AND te.date <= $" + string(rune('0'+argNum))
		args = append(args, *endDate)
		argNum++
	}
	if projectID != nil {
		query += " AND te.project_id = $" + string(rune('0'+argNum))
		args = append(args, *projectID)
	}

	query += " ORDER BY te.date DESC, p.name"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*TimeEntry
	for rows.Next() {
		e := &TimeEntry{Project: &Project{}}
		err := rows.Scan(
			&e.ID, &e.UserID, &e.ProjectID, &e.Date, &e.Hours, &e.Title, &e.Description,
			&e.Source, &e.InvoiceID, &e.HasUserEdits,
			&e.IsPinned, &e.IsLocked, &e.IsStale, &e.IsSuppressed,
			&e.ComputedHours, &e.ComputedTitle, &e.ComputedDescription, &e.SnapshotComputedHours,
			&e.CalculationDetails, &e.CreatedAt, &e.UpdatedAt,
			&e.Project.ID, &e.Project.UserID, &e.Project.Name, &e.Project.ShortCode,
			&e.Project.Color, &e.Project.IsBillable, &e.Project.IsArchived,
			&e.Project.IsHiddenByDefault, &e.Project.DoesNotAccumulateHours,
			&e.Project.CreatedAt, &e.Project.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}

	return entries, rows.Err()
}

// Update modifies an existing time entry
// When user edits, we capture snapshot_computed_hours for staleness detection
func (s *TimeEntryStore) Update(ctx context.Context, userID, entryID uuid.UUID, hours *float64, description *string) (*TimeEntry, error) {
	// Check if invoiced
	entry, err := s.GetByID(ctx, userID, entryID)
	if err != nil {
		return nil, err
	}
	if entry.InvoiceID != nil {
		return nil, ErrTimeEntryInvoiced
	}

	// Build update
	now := time.Now().UTC()
	if hours != nil {
		entry.Hours = *hours
	}
	if description != nil {
		entry.Description = description
	}
	entry.HasUserEdits = true
	entry.UpdatedAt = now

	// Capture snapshot_computed_hours at materialization time
	// This anchors the staleness check to know what computed_hours was when user made their edit
	_, err = s.pool.Exec(ctx, `
		UPDATE time_entries
		SET hours = $3,
		    description = $4,
		    has_user_edits = true,
		    snapshot_computed_hours = computed_hours,
		    updated_at = $5
		WHERE id = $1 AND user_id = $2
	`, entryID, userID, entry.Hours, entry.Description, now)

	if err != nil {
		return nil, err
	}

	// Re-fetch to get the updated snapshot value
	return s.GetByID(ctx, userID, entryID)
}

// CreateFromCalendar creates or updates a time entry from a calendar event
// Unlike manual creation, this accumulates hours if an entry already exists
func (s *TimeEntryStore) CreateFromCalendar(ctx context.Context, userID, projectID uuid.UUID, date time.Time, hours float64, description *string) (*TimeEntry, error) {
	entry := &TimeEntry{
		ID:           uuid.New(),
		UserID:       userID,
		ProjectID:    projectID,
		Date:         date,
		Hours:        hours,
		Description:  description,
		Source:       "calendar",
		HasUserEdits: false,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	// Use upsert - if entry exists for same project/date, add hours (unless user edited)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO time_entries (id, user_id, project_id, date, hours, description, source, has_user_edits, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, project_id, date) DO UPDATE SET
			hours = CASE
				WHEN time_entries.has_user_edits THEN time_entries.hours
				ELSE time_entries.hours + EXCLUDED.hours
			END,
			description = CASE
				WHEN time_entries.has_user_edits THEN time_entries.description
				WHEN time_entries.description IS NULL THEN EXCLUDED.description
				WHEN EXCLUDED.description IS NULL THEN time_entries.description
				ELSE time_entries.description || E'\n' || EXCLUDED.description
			END,
			updated_at = EXCLUDED.updated_at
	`, entry.ID, entry.UserID, entry.ProjectID, entry.Date, entry.Hours,
		entry.Description, entry.Source, entry.HasUserEdits, entry.CreatedAt, entry.UpdatedAt)

	if err != nil {
		return nil, err
	}

	// Fetch the actual entry (in case it was an update)
	return s.GetByProjectAndDate(ctx, userID, projectID, date)
}

// Delete removes a time entry
func (s *TimeEntryStore) Delete(ctx context.Context, userID, entryID uuid.UUID) error {
	// Check if invoiced
	entry, err := s.GetByID(ctx, userID, entryID)
	if err != nil {
		return err
	}
	if entry.InvoiceID != nil {
		return ErrTimeEntryInvoiced
	}

	result, err := s.pool.Exec(ctx,
		"DELETE FROM time_entries WHERE id = $1 AND user_id = $2",
		entryID, userID,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrTimeEntryNotFound
	}

	return nil
}

// --- Contributing Events (Junction Table) ---

// SetContributingEvents replaces the contributing events for a time entry
func (s *TimeEntryStore) SetContributingEvents(ctx context.Context, entryID uuid.UUID, eventIDs []uuid.UUID) error {
	// Delete existing
	_, err := s.pool.Exec(ctx,
		"DELETE FROM time_entry_events WHERE time_entry_id = $1",
		entryID,
	)
	if err != nil {
		return err
	}

	// Insert new
	for _, eventID := range eventIDs {
		_, err := s.pool.Exec(ctx,
			"INSERT INTO time_entry_events (time_entry_id, calendar_event_id) VALUES ($1, $2)",
			entryID, eventID,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetContributingEvents returns the event IDs that contribute to a time entry
func (s *TimeEntryStore) GetContributingEvents(ctx context.Context, entryID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx,
		"SELECT calendar_event_id FROM time_entry_events WHERE time_entry_id = $1",
		entryID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var eventIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		eventIDs = append(eventIDs, id)
	}

	return eventIDs, rows.Err()
}

// --- Protection Model ---

// Pin marks a time entry as pinned (user has edited it)
func (s *TimeEntryStore) Pin(ctx context.Context, userID, entryID uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE time_entries SET is_pinned = true, updated_at = $3
		WHERE id = $1 AND user_id = $2
	`, entryID, userID, time.Now().UTC())

	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrTimeEntryNotFound
	}
	return nil
}

// Unpin removes the pin from a time entry, returning it to auto-update mode
func (s *TimeEntryStore) Unpin(ctx context.Context, userID, entryID uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE time_entries SET is_pinned = false, is_stale = false, updated_at = $3
		WHERE id = $1 AND user_id = $2
	`, entryID, userID, time.Now().UTC())

	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrTimeEntryNotFound
	}
	return nil
}

// Refresh accepts computed values for a protected time entry (stays protected)
// Updates snapshot_computed_hours to clear staleness
func (s *TimeEntryStore) Refresh(ctx context.Context, userID, entryID uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE time_entries
		SET hours = COALESCE(computed_hours, hours),
		    title = COALESCE(computed_title, title),
		    description = COALESCE(computed_description, description),
		    snapshot_computed_hours = computed_hours,
		    is_stale = false,
		    updated_at = $3
		WHERE id = $1 AND user_id = $2
	`, entryID, userID, time.Now().UTC())

	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrTimeEntryNotFound
	}
	return nil
}

// ResetToComputed resets a time entry to computed values and unpins it.
// This is used for the "Reset to Computed" feature.
// It updates all fields and removes the pinned status, returning the entry to auto-update mode.
// Preserves is_locked but clears is_stale and is_pinned.
// Updates snapshot_computed_hours to the new computed value.
func (s *TimeEntryStore) ResetToComputed(ctx context.Context, userID, entryID uuid.UUID, hours float64, title, description string, details []byte, eventIDs []uuid.UUID) (*TimeEntry, error) {
	now := time.Now().UTC()

	// Update to computed values and unpin
	// Also update snapshot to match, clearing any staleness
	_, err := s.pool.Exec(ctx, `
		UPDATE time_entries
		SET hours = $3,
		    title = $4,
		    description = $5,
		    computed_hours = $3,
		    computed_title = $4,
		    computed_description = $5,
		    calculation_details = $6,
		    snapshot_computed_hours = $3,
		    is_pinned = false,
		    is_stale = false,
		    updated_at = $7
		WHERE id = $1 AND user_id = $2
	`, entryID, userID, hours, title, description, details, now)
	if err != nil {
		return nil, err
	}

	// Update contributing events
	if err := s.SetContributingEvents(ctx, entryID, eventIDs); err != nil {
		return nil, err
	}

	// Fetch and return the updated entry
	return s.GetByID(ctx, userID, entryID)
}

// LockDay locks all time entries and classified events for a specific day
func (s *TimeEntryStore) LockDay(ctx context.Context, userID uuid.UUID, date time.Time) error {
	now := time.Now().UTC()

	// Lock time entries
	_, err := s.pool.Exec(ctx, `
		UPDATE time_entries SET is_locked = true, updated_at = $3
		WHERE user_id = $1 AND date = $2
	`, userID, date, now)
	if err != nil {
		return err
	}

	// Lock calendar events for that day (classified events only)
	nextDay := date.AddDate(0, 0, 1)
	_, err = s.pool.Exec(ctx, `
		UPDATE calendar_events SET is_locked = true, updated_at = $4
		WHERE user_id = $1 AND start_time >= $2 AND start_time < $3
		AND classification_status != 'pending'
	`, userID, date, nextDay, now)

	return err
}

// UnlockDay unlocks all time entries and events for a specific day
func (s *TimeEntryStore) UnlockDay(ctx context.Context, userID uuid.UUID, date time.Time) error {
	now := time.Now().UTC()

	// Unlock time entries
	_, err := s.pool.Exec(ctx, `
		UPDATE time_entries SET is_locked = false, is_stale = false, updated_at = $3
		WHERE user_id = $1 AND date = $2
	`, userID, date, now)
	if err != nil {
		return err
	}

	// Unlock calendar events
	nextDay := date.AddDate(0, 0, 1)
	_, err = s.pool.Exec(ctx, `
		UPDATE calendar_events SET is_locked = false, updated_at = $4
		WHERE user_id = $1 AND start_time >= $2 AND start_time < $3
	`, userID, date, nextDay, now)

	return err
}

// --- Computed Fields Update ---

// UpdateComputed updates the computed fields for a time entry.
// If the entry is unlocked, it also updates the current values.
// If protected (pinned/locked), it only updates computed fields and marks as stale if different.
func (s *TimeEntryStore) UpdateComputed(ctx context.Context, userID uuid.UUID, entryID uuid.UUID, hours float64, title, description string, details []byte, eventIDs []uuid.UUID) error {
	now := time.Now().UTC()

	// Update computed fields and conditionally update current values
	_, err := s.pool.Exec(ctx, `
		UPDATE time_entries
		SET computed_hours = $3,
		    computed_title = $4,
		    computed_description = $5,
		    calculation_details = $6,
		    -- Only update current values if not protected
		    hours = CASE WHEN is_pinned OR is_locked OR invoice_id IS NOT NULL THEN hours ELSE $3 END,
		    title = CASE WHEN is_pinned OR is_locked OR invoice_id IS NOT NULL THEN title ELSE $4 END,
		    description = CASE WHEN is_pinned OR is_locked OR invoice_id IS NOT NULL THEN description ELSE $5 END,
		    -- Mark as stale if protected and values differ
		    is_stale = CASE
		        WHEN invoice_id IS NOT NULL THEN (hours != $3 OR COALESCE(title, '') != $4 OR COALESCE(description, '') != $5)
		        WHEN is_pinned OR is_locked THEN (hours != $3 OR COALESCE(title, '') != $4 OR COALESCE(description, '') != $5)
		        ELSE false
		    END,
		    updated_at = $7
		WHERE id = $1 AND user_id = $2
	`, entryID, userID, hours, title, description, details, now)
	if err != nil {
		return err
	}

	// Update contributing events
	return s.SetContributingEvents(ctx, entryID, eventIDs)
}

// RefreshComputedValues updates only the computed_hours field with a fresh value.
// This is called before Update to ensure snapshot_computed_hours captures the
// current computed value, which is needed for correct staleness detection.
func (s *TimeEntryStore) RefreshComputedValues(ctx context.Context, userID, entryID uuid.UUID, computedHours float64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE time_entries
		SET computed_hours = $3
		WHERE id = $1 AND user_id = $2
	`, entryID, userID, computedHours)
	return err
}

// UpsertFromComputed creates or updates a time entry from computed values.
// Used by the analyzer when processing classified events.
func (s *TimeEntryStore) UpsertFromComputed(ctx context.Context, userID, projectID uuid.UUID, date time.Time, hours float64, title, description string, details []byte, eventIDs []uuid.UUID) (*TimeEntry, error) {
	entryID := uuid.New()
	now := time.Now().UTC()

	// Use upsert - only update if not protected
	_, err := s.pool.Exec(ctx, `
		INSERT INTO time_entries (
			id, user_id, project_id, date, hours, title, description, source,
			computed_hours, computed_title, computed_description, calculation_details,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'calendar', $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, project_id, date) DO UPDATE SET
			computed_hours = EXCLUDED.computed_hours,
			computed_title = EXCLUDED.computed_title,
			computed_description = EXCLUDED.computed_description,
			calculation_details = EXCLUDED.calculation_details,
			-- Only update current values if not protected
			hours = CASE
				WHEN time_entries.is_pinned OR time_entries.is_locked OR time_entries.invoice_id IS NOT NULL
				THEN time_entries.hours
				ELSE EXCLUDED.hours
			END,
			title = CASE
				WHEN time_entries.is_pinned OR time_entries.is_locked OR time_entries.invoice_id IS NOT NULL
				THEN time_entries.title
				ELSE EXCLUDED.title
			END,
			description = CASE
				WHEN time_entries.is_pinned OR time_entries.is_locked OR time_entries.invoice_id IS NOT NULL
				THEN time_entries.description
				ELSE EXCLUDED.description
			END,
			-- Mark as stale if protected and values differ
			is_stale = CASE
				WHEN time_entries.invoice_id IS NOT NULL OR time_entries.is_pinned OR time_entries.is_locked
				THEN (time_entries.hours != EXCLUDED.hours OR COALESCE(time_entries.title, '') != EXCLUDED.title OR COALESCE(time_entries.description, '') != EXCLUDED.description)
				ELSE false
			END,
			updated_at = EXCLUDED.updated_at
	`, entryID, userID, projectID, date, hours, title, description, details, now, now)
	if err != nil {
		return nil, err
	}

	// Get the entry (might have been created or updated)
	entry, err := s.GetByProjectAndDate(ctx, userID, projectID, date)
	if err != nil {
		return nil, err
	}

	// Update contributing events
	if err := s.SetContributingEvents(ctx, entry.ID, eventIDs); err != nil {
		return nil, err
	}

	return entry, nil
}
