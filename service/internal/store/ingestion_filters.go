package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrIngestionFilterNotFound is returned when a filter does not exist for the user.
var ErrIngestionFilterNotFound = errors.New("ingestion filter not found")

// IngestionFilter hides matching events at ingestion. See issue #110.
type IngestionFilter struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Name      string
	Query     string
	IsEnabled bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IngestionFilterStore provides database operations for ingestion filters.
type IngestionFilterStore struct {
	pool *pgxpool.Pool
}

func NewIngestionFilterStore(pool *pgxpool.Pool) *IngestionFilterStore {
	return &IngestionFilterStore{pool: pool}
}

const ingestionFilterColumns = `id, user_id, name, query, is_enabled, created_at, updated_at`

func scanIngestionFilter(row pgx.Row) (*IngestionFilter, error) {
	f := &IngestionFilter{}
	err := row.Scan(&f.ID, &f.UserID, &f.Name, &f.Query, &f.IsEnabled, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIngestionFilterNotFound
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

// List returns a user's filters, oldest first. When enabledOnly is set, only
// filters that are actually in effect are returned.
func (s *IngestionFilterStore) List(ctx context.Context, userID uuid.UUID, enabledOnly bool) ([]*IngestionFilter, error) {
	query := `SELECT ` + ingestionFilterColumns + ` FROM ingestion_filters WHERE user_id = $1`
	if enabledOnly {
		query += ` AND is_enabled = true`
	}
	query += ` ORDER BY created_at ASC`

	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var filters []*IngestionFilter
	for rows.Next() {
		f, err := scanIngestionFilter(rows)
		if err != nil {
			return nil, err
		}
		filters = append(filters, f)
	}
	return filters, rows.Err()
}

// Get returns one filter belonging to the user.
func (s *IngestionFilterStore) Get(ctx context.Context, userID, id uuid.UUID) (*IngestionFilter, error) {
	return scanIngestionFilter(s.pool.QueryRow(ctx,
		`SELECT `+ingestionFilterColumns+` FROM ingestion_filters WHERE id = $1 AND user_id = $2`,
		id, userID))
}

// Create inserts a filter.
func (s *IngestionFilterStore) Create(ctx context.Context, f *IngestionFilter) (*IngestionFilter, error) {
	return scanIngestionFilter(s.pool.QueryRow(ctx, `
		INSERT INTO ingestion_filters (id, user_id, name, query, is_enabled)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+ingestionFilterColumns,
		uuid.New(), f.UserID, f.Name, f.Query, f.IsEnabled))
}

// Update applies non-nil fields to an existing filter.
func (s *IngestionFilterStore) Update(ctx context.Context, userID, id uuid.UUID, name, query *string, isEnabled *bool) (*IngestionFilter, error) {
	return scanIngestionFilter(s.pool.QueryRow(ctx, `
		UPDATE ingestion_filters SET
			name = COALESCE($3, name),
			query = COALESCE($4, query),
			is_enabled = COALESCE($5, is_enabled),
			updated_at = NOW()
		WHERE id = $1 AND user_id = $2
		RETURNING `+ingestionFilterColumns,
		id, userID, name, query, isEnabled))
}

// Delete removes a filter. Events it suppressed stay suppressed until
// suppression is re-evaluated.
func (s *IngestionFilterStore) Delete(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM ingestion_filters WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrIngestionFilterNotFound
	}
	return nil
}
