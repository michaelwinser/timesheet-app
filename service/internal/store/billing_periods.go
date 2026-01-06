package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrBillingPeriodNotFound = errors.New("billing period not found")
	ErrBillingPeriodOverlap  = errors.New("billing period overlaps with existing period")
)

// BillingPeriod represents a stored billing period
type BillingPeriod struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	ProjectID  uuid.UUID
	StartsOn   time.Time
	EndsOn     *time.Time
	HourlyRate float64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// BillingPeriodStore provides PostgreSQL-backed billing period storage
type BillingPeriodStore struct {
	pool *pgxpool.Pool
}

// NewBillingPeriodStore creates a new PostgreSQL billing period store
func NewBillingPeriodStore(pool *pgxpool.Pool) *BillingPeriodStore {
	return &BillingPeriodStore{pool: pool}
}

// Create adds a new billing period
func (s *BillingPeriodStore) Create(ctx context.Context, userID, projectID uuid.UUID, startsOn time.Time, endsOn *time.Time, hourlyRate float64) (*BillingPeriod, error) {
	period := &BillingPeriod{
		ID:         uuid.New(),
		UserID:     userID,
		ProjectID:  projectID,
		StartsOn:   startsOn,
		EndsOn:     endsOn,
		HourlyRate: hourlyRate,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO billing_periods (id, user_id, project_id, starts_on, ends_on, hourly_rate, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, period.ID, period.UserID, period.ProjectID, period.StartsOn, period.EndsOn, period.HourlyRate, period.CreatedAt, period.UpdatedAt)

	if err != nil {
		// Check if it's an overlap constraint violation
		if err.Error() == "Billing periods for project cannot overlap" {
			return nil, ErrBillingPeriodOverlap
		}
		return nil, err
	}

	return period, nil
}

// GetByID retrieves a billing period by ID
func (s *BillingPeriodStore) GetByID(ctx context.Context, userID, periodID uuid.UUID) (*BillingPeriod, error) {
	period := &BillingPeriod{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, project_id, starts_on, ends_on, hourly_rate, created_at, updated_at
		FROM billing_periods WHERE id = $1 AND user_id = $2
	`, periodID, userID).Scan(
		&period.ID, &period.UserID, &period.ProjectID, &period.StartsOn, &period.EndsOn,
		&period.HourlyRate, &period.CreatedAt, &period.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBillingPeriodNotFound
		}
		return nil, err
	}
	return period, nil
}

// ListByProject retrieves all billing periods for a project
func (s *BillingPeriodStore) ListByProject(ctx context.Context, userID, projectID uuid.UUID) ([]*BillingPeriod, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, project_id, starts_on, ends_on, hourly_rate, created_at, updated_at
		FROM billing_periods
		WHERE user_id = $1 AND project_id = $2
		ORDER BY starts_on DESC
	`, userID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var periods []*BillingPeriod
	for rows.Next() {
		p := &BillingPeriod{}
		err := rows.Scan(&p.ID, &p.UserID, &p.ProjectID, &p.StartsOn, &p.EndsOn, &p.HourlyRate, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			return nil, err
		}
		periods = append(periods, p)
	}

	return periods, rows.Err()
}

// FindPeriodForDate finds the billing period that covers a specific date
func (s *BillingPeriodStore) FindPeriodForDate(ctx context.Context, userID, projectID uuid.UUID, date time.Time) (*BillingPeriod, error) {
	period := &BillingPeriod{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, project_id, starts_on, ends_on, hourly_rate, created_at, updated_at
		FROM billing_periods
		WHERE user_id = $1 AND project_id = $2
		AND starts_on <= $3
		AND (ends_on IS NULL OR ends_on >= $3)
		LIMIT 1
	`, userID, projectID, date).Scan(
		&period.ID, &period.UserID, &period.ProjectID, &period.StartsOn, &period.EndsOn,
		&period.HourlyRate, &period.CreatedAt, &period.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBillingPeriodNotFound
		}
		return nil, err
	}
	return period, nil
}

// Update modifies an existing billing period
func (s *BillingPeriodStore) Update(ctx context.Context, userID, periodID uuid.UUID, updates map[string]interface{}) (*BillingPeriod, error) {
	updates["updated_at"] = time.Now().UTC()

	// Build dynamic update query
	setClauses := ""
	args := []interface{}{periodID, userID}
	argNum := 3

	for key, value := range updates {
		if setClauses != "" {
			setClauses += ", "
		}
		setClauses += fmt.Sprintf("%s = $%d", key, argNum)
		args = append(args, value)
		argNum++
	}

	query := "UPDATE billing_periods SET " + setClauses + " WHERE id = $1 AND user_id = $2 RETURNING id, user_id, project_id, starts_on, ends_on, hourly_rate, created_at, updated_at"

	period := &BillingPeriod{}
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&period.ID, &period.UserID, &period.ProjectID, &period.StartsOn, &period.EndsOn,
		&period.HourlyRate, &period.CreatedAt, &period.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBillingPeriodNotFound
		}
		// Check for overlap error
		if err.Error() == "Billing periods for project cannot overlap" {
			return nil, ErrBillingPeriodOverlap
		}
		return nil, err
	}

	return period, nil
}

// Delete removes a billing period
func (s *BillingPeriodStore) Delete(ctx context.Context, userID, periodID uuid.UUID) error {
	result, err := s.pool.Exec(ctx,
		"DELETE FROM billing_periods WHERE id = $1 AND user_id = $2",
		periodID, userID,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrBillingPeriodNotFound
	}

	return nil
}
