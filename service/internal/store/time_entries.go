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
	Description  *string
	Source       string
	InvoiceID    *uuid.UUID
	HasUserEdits bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
	// Joined data
	Project *Project
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
	_, err := s.pool.Exec(ctx, `
		INSERT INTO time_entries (id, user_id, project_id, date, hours, description, source, has_user_edits, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (user_id, project_id, date) DO UPDATE SET
			hours = time_entries.hours + EXCLUDED.hours,
			description = COALESCE(EXCLUDED.description, time_entries.description),
			has_user_edits = true,
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
		SELECT id, user_id, project_id, date, hours, description, source, invoice_id, has_user_edits, created_at, updated_at
		FROM time_entries WHERE id = $1 AND user_id = $2
	`, entryID, userID).Scan(
		&entry.ID, &entry.UserID, &entry.ProjectID, &entry.Date, &entry.Hours,
		&entry.Description, &entry.Source, &entry.InvoiceID, &entry.HasUserEdits,
		&entry.CreatedAt, &entry.UpdatedAt,
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
		SELECT id, user_id, project_id, date, hours, description, source, invoice_id, has_user_edits, created_at, updated_at
		FROM time_entries WHERE user_id = $1 AND project_id = $2 AND date = $3
	`, userID, projectID, date).Scan(
		&entry.ID, &entry.UserID, &entry.ProjectID, &entry.Date, &entry.Hours,
		&entry.Description, &entry.Source, &entry.InvoiceID, &entry.HasUserEdits,
		&entry.CreatedAt, &entry.UpdatedAt,
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
		SELECT te.id, te.user_id, te.project_id, te.date, te.hours, te.description,
		       te.source, te.invoice_id, te.has_user_edits, te.created_at, te.updated_at,
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
			&e.ID, &e.UserID, &e.ProjectID, &e.Date, &e.Hours, &e.Description,
			&e.Source, &e.InvoiceID, &e.HasUserEdits, &e.CreatedAt, &e.UpdatedAt,
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

	_, err = s.pool.Exec(ctx, `
		UPDATE time_entries SET hours = $3, description = $4, has_user_edits = true, updated_at = $5
		WHERE id = $1 AND user_id = $2
	`, entryID, userID, entry.Hours, entry.Description, now)

	if err != nil {
		return nil, err
	}

	return entry, nil
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
