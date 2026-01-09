package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrCalendarNotFound = errors.New("calendar not found")

// Calendar represents a calendar within a connection (e.g., one of multiple Google calendars)
type Calendar struct {
	ID               uuid.UUID
	ConnectionID     uuid.UUID
	UserID           uuid.UUID
	ExternalID       string // Google Calendar ID (e.g., "primary", "user@example.com")
	Name             string
	Color            *string
	IsPrimary        bool
	IsSelected       bool
	SyncToken        *string
	LastSyncedAt     *time.Time
	MinSyncedDate    *time.Time // Earliest date that has been fully synced (low water mark)
	MaxSyncedDate    *time.Time // Latest date that has been fully synced (high water mark)
	SyncFailureCount int        // Consecutive sync failures (stop retrying after 3)
	NeedsReauth      bool       // True if OAuth token refresh failed
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CalendarStore provides PostgreSQL-backed calendar storage
type CalendarStore struct {
	pool *pgxpool.Pool
}

// NewCalendarStore creates a new store
func NewCalendarStore(pool *pgxpool.Pool) *CalendarStore {
	return &CalendarStore{pool: pool}
}

// Upsert creates or updates a calendar by external_id
func (s *CalendarStore) Upsert(ctx context.Context, cal *Calendar) (*Calendar, error) {
	now := time.Now().UTC()
	newID := uuid.New()

	err := s.pool.QueryRow(ctx, `
		INSERT INTO calendars (
			id, connection_id, user_id, external_id, name, color,
			is_primary, is_selected, sync_token, last_synced_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (connection_id, external_id) DO UPDATE SET
			name = EXCLUDED.name,
			color = EXCLUDED.color,
			is_primary = EXCLUDED.is_primary,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at, updated_at
	`,
		newID, cal.ConnectionID, cal.UserID, cal.ExternalID, cal.Name, cal.Color,
		cal.IsPrimary, cal.IsSelected, cal.SyncToken, cal.LastSyncedAt, now, now,
	).Scan(&cal.ID, &cal.CreatedAt, &cal.UpdatedAt)

	if err != nil {
		return nil, err
	}

	return cal, nil
}

// ListByConnection returns all calendars for a connection
func (s *CalendarStore) ListByConnection(ctx context.Context, connectionID uuid.UUID) ([]*Calendar, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, connection_id, user_id, external_id, name, color,
		       is_primary, is_selected, sync_token, last_synced_at,
		       min_synced_date, max_synced_date, sync_failure_count, needs_reauth,
		       created_at, updated_at
		FROM calendars
		WHERE connection_id = $1
		ORDER BY is_primary DESC, name ASC
	`, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calendars []*Calendar
	for rows.Next() {
		cal := &Calendar{}
		err := rows.Scan(
			&cal.ID, &cal.ConnectionID, &cal.UserID, &cal.ExternalID, &cal.Name, &cal.Color,
			&cal.IsPrimary, &cal.IsSelected, &cal.SyncToken, &cal.LastSyncedAt,
			&cal.MinSyncedDate, &cal.MaxSyncedDate, &cal.SyncFailureCount, &cal.NeedsReauth,
			&cal.CreatedAt, &cal.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		calendars = append(calendars, cal)
	}

	return calendars, rows.Err()
}

// ListSelectedByConnection returns only selected calendars for a connection
func (s *CalendarStore) ListSelectedByConnection(ctx context.Context, connectionID uuid.UUID) ([]*Calendar, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, connection_id, user_id, external_id, name, color,
		       is_primary, is_selected, sync_token, last_synced_at,
		       min_synced_date, max_synced_date, sync_failure_count, needs_reauth,
		       created_at, updated_at
		FROM calendars
		WHERE connection_id = $1 AND is_selected = true
		ORDER BY is_primary DESC, name ASC
	`, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calendars []*Calendar
	for rows.Next() {
		cal := &Calendar{}
		err := rows.Scan(
			&cal.ID, &cal.ConnectionID, &cal.UserID, &cal.ExternalID, &cal.Name, &cal.Color,
			&cal.IsPrimary, &cal.IsSelected, &cal.SyncToken, &cal.LastSyncedAt,
			&cal.MinSyncedDate, &cal.MaxSyncedDate, &cal.SyncFailureCount, &cal.NeedsReauth,
			&cal.CreatedAt, &cal.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		calendars = append(calendars, cal)
	}

	return calendars, rows.Err()
}

// GetByID retrieves a calendar by ID
func (s *CalendarStore) GetByID(ctx context.Context, calendarID uuid.UUID) (*Calendar, error) {
	cal := &Calendar{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, connection_id, user_id, external_id, name, color,
		       is_primary, is_selected, sync_token, last_synced_at,
		       min_synced_date, max_synced_date, sync_failure_count, needs_reauth,
		       created_at, updated_at
		FROM calendars
		WHERE id = $1
	`, calendarID).Scan(
		&cal.ID, &cal.ConnectionID, &cal.UserID, &cal.ExternalID, &cal.Name, &cal.Color,
		&cal.IsPrimary, &cal.IsSelected, &cal.SyncToken, &cal.LastSyncedAt,
		&cal.MinSyncedDate, &cal.MaxSyncedDate, &cal.SyncFailureCount, &cal.NeedsReauth,
		&cal.CreatedAt, &cal.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarNotFound
		}
		return nil, err
	}

	return cal, nil
}

// UpdateSelection updates the is_selected status for multiple calendars
func (s *CalendarStore) UpdateSelection(ctx context.Context, connectionID uuid.UUID, selectedIDs []uuid.UUID) error {
	now := time.Now().UTC()

	// First, deselect all calendars for this connection
	_, err := s.pool.Exec(ctx, `
		UPDATE calendars
		SET is_selected = false, updated_at = $2
		WHERE connection_id = $1
	`, connectionID, now)
	if err != nil {
		return err
	}

	// Then select the specified ones
	if len(selectedIDs) > 0 {
		_, err = s.pool.Exec(ctx, `
			UPDATE calendars
			SET is_selected = true, updated_at = $3
			WHERE connection_id = $1 AND id = ANY($2)
		`, connectionID, selectedIDs, now)
		if err != nil {
			return err
		}
	}

	return nil
}

// UpdateSyncToken updates the sync token for a calendar
func (s *CalendarStore) UpdateSyncToken(ctx context.Context, calendarID uuid.UUID, token string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE calendars
		SET sync_token = $2, updated_at = $3
		WHERE id = $1
	`, calendarID, token, time.Now().UTC())
	return err
}

// ClearSyncToken clears the sync token (forces full re-sync)
func (s *CalendarStore) ClearSyncToken(ctx context.Context, calendarID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE calendars
		SET sync_token = NULL, updated_at = $2
		WHERE id = $1
	`, calendarID, time.Now().UTC())
	return err
}

// UpdateLastSynced updates the last_synced_at timestamp
func (s *CalendarStore) UpdateLastSynced(ctx context.Context, calendarID uuid.UUID) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		UPDATE calendars
		SET last_synced_at = $2, updated_at = $2
		WHERE id = $1
	`, calendarID, now)
	return err
}

// ExpandSyncedWindow expands the min/max synced date window for a calendar
// The window is expanded to include the new min/max dates, never shrunk
func (s *CalendarStore) ExpandSyncedWindow(ctx context.Context, calendarID uuid.UUID, minDate, maxDate time.Time) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		UPDATE calendars
		SET
			min_synced_date = CASE
				WHEN min_synced_date IS NULL THEN $2
				WHEN $2 < min_synced_date THEN $2
				ELSE min_synced_date
			END,
			max_synced_date = CASE
				WHEN max_synced_date IS NULL THEN $3
				WHEN $3 > max_synced_date THEN $3
				ELSE max_synced_date
			END,
			updated_at = $4
		WHERE id = $1
	`, calendarID, minDate, maxDate, now)
	return err
}

// DeleteByConnection deletes all calendars for a connection
func (s *CalendarStore) DeleteByConnection(ctx context.Context, connectionID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM calendars WHERE connection_id = $1
	`, connectionID)
	return err
}

// IncrementSyncFailureCount increments the sync failure counter
func (s *CalendarStore) IncrementSyncFailureCount(ctx context.Context, calendarID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE calendars
		SET sync_failure_count = sync_failure_count + 1, updated_at = $2
		WHERE id = $1
	`, calendarID, time.Now().UTC())
	return err
}

// ResetSyncFailureCount resets the sync failure counter to zero
func (s *CalendarStore) ResetSyncFailureCount(ctx context.Context, calendarID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE calendars
		SET sync_failure_count = 0, updated_at = $2
		WHERE id = $1
	`, calendarID, time.Now().UTC())
	return err
}

// MarkNeedsReauth marks a calendar as needing re-authentication
func (s *CalendarStore) MarkNeedsReauth(ctx context.Context, calendarID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE calendars
		SET needs_reauth = true, updated_at = $2
		WHERE id = $1
	`, calendarID, time.Now().UTC())
	return err
}

// ClearNeedsReauth clears the needs_reauth flag (after successful re-auth)
func (s *CalendarStore) ClearNeedsReauth(ctx context.Context, calendarID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE calendars
		SET needs_reauth = false, sync_failure_count = 0, updated_at = $2
		WHERE id = $1
	`, calendarID, time.Now().UTC())
	return err
}

// ListNeedingSync returns calendars that need background sync
// These are calendars that:
// - Haven't synced within the staleness threshold
// - Don't need re-auth
// - Haven't failed too many times (< 3 consecutive failures)
func (s *CalendarStore) ListNeedingSync(ctx context.Context, stalenessThreshold time.Duration) ([]*Calendar, error) {
	cutoff := time.Now().Add(-stalenessThreshold)

	rows, err := s.pool.Query(ctx, `
		SELECT id, connection_id, user_id, external_id, name, color,
		       is_primary, is_selected, sync_token, last_synced_at,
		       min_synced_date, max_synced_date, sync_failure_count, needs_reauth,
		       created_at, updated_at
		FROM calendars
		WHERE is_selected = true
		  AND needs_reauth = false
		  AND sync_failure_count < 3
		  AND (last_synced_at IS NULL OR last_synced_at < $1)
		ORDER BY last_synced_at ASC NULLS FIRST
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var calendars []*Calendar
	for rows.Next() {
		cal := &Calendar{}
		err := rows.Scan(
			&cal.ID, &cal.ConnectionID, &cal.UserID, &cal.ExternalID, &cal.Name, &cal.Color,
			&cal.IsPrimary, &cal.IsSelected, &cal.SyncToken, &cal.LastSyncedAt,
			&cal.MinSyncedDate, &cal.MaxSyncedDate, &cal.SyncFailureCount, &cal.NeedsReauth,
			&cal.CreatedAt, &cal.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		calendars = append(calendars, cal)
	}

	return calendars, rows.Err()
}
