package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrClassificationRuleNotFound = errors.New("classification rule not found")

// ClassificationRule represents a rule for classifying calendar events
type ClassificationRule struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Query     string
	ProjectID *uuid.UUID // nil for attendance rules
	Attended  *bool      // nil for project rules
	Weight    float64
	IsEnabled bool
	CreatedAt time.Time
	UpdatedAt time.Time
	// Joined data
	ProjectName  *string
	ProjectColor *string
}

// ClassificationOverride records when a user overrides an automatic classification
type ClassificationOverride struct {
	ID            uuid.UUID
	EventID       uuid.UUID
	UserID        uuid.UUID
	FromProjectID *uuid.UUID
	ToProjectID   *uuid.UUID
	FromSource    *string
	Reason        *string
	CreatedAt     time.Time
}

// ClassificationRuleStore provides PostgreSQL-backed rule storage
type ClassificationRuleStore struct {
	pool *pgxpool.Pool
}

// NewClassificationRuleStore creates a new store
func NewClassificationRuleStore(pool *pgxpool.Pool) *ClassificationRuleStore {
	return &ClassificationRuleStore{pool: pool}
}

// Create creates a new classification rule
func (s *ClassificationRuleStore) Create(ctx context.Context, rule *ClassificationRule) (*ClassificationRule, error) {
	rule.ID = uuid.New()
	now := time.Now().UTC()
	rule.CreatedAt = now
	rule.UpdatedAt = now

	if rule.Weight == 0 {
		rule.Weight = 1.0
	}

	err := s.pool.QueryRow(ctx, `
		INSERT INTO classification_rules (id, user_id, query, project_id, attended, weight, is_enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`,
		rule.ID, rule.UserID, rule.Query, rule.ProjectID, rule.Attended,
		rule.Weight, rule.IsEnabled, rule.CreatedAt, rule.UpdatedAt,
	).Scan(&rule.ID)

	if err != nil {
		return nil, err
	}

	return rule, nil
}

// GetByID retrieves a rule by ID
func (s *ClassificationRuleStore) GetByID(ctx context.Context, userID, ruleID uuid.UUID) (*ClassificationRule, error) {
	rule := &ClassificationRule{}

	err := s.pool.QueryRow(ctx, `
		SELECT r.id, r.user_id, r.query, r.project_id, r.attended, r.weight, r.is_enabled,
		       r.created_at, r.updated_at, p.name, p.color
		FROM classification_rules r
		LEFT JOIN projects p ON r.project_id = p.id
		WHERE r.id = $1 AND r.user_id = $2
	`, ruleID, userID).Scan(
		&rule.ID, &rule.UserID, &rule.Query, &rule.ProjectID, &rule.Attended,
		&rule.Weight, &rule.IsEnabled, &rule.CreatedAt, &rule.UpdatedAt,
		&rule.ProjectName, &rule.ProjectColor,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrClassificationRuleNotFound
		}
		return nil, err
	}

	return rule, nil
}

// List returns all rules for a user
func (s *ClassificationRuleStore) List(ctx context.Context, userID uuid.UUID, includeDisabled bool) ([]*ClassificationRule, error) {
	query := `
		SELECT r.id, r.user_id, r.query, r.project_id, r.attended, r.weight, r.is_enabled,
		       r.created_at, r.updated_at, p.name, p.color
		FROM classification_rules r
		LEFT JOIN projects p ON r.project_id = p.id
		WHERE r.user_id = $1
	`

	if !includeDisabled {
		query += " AND r.is_enabled = true"
	}

	query += " ORDER BY r.weight DESC, r.created_at ASC"

	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*ClassificationRule
	for rows.Next() {
		rule := &ClassificationRule{}
		err := rows.Scan(
			&rule.ID, &rule.UserID, &rule.Query, &rule.ProjectID, &rule.Attended,
			&rule.Weight, &rule.IsEnabled, &rule.CreatedAt, &rule.UpdatedAt,
			&rule.ProjectName, &rule.ProjectColor,
		)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	return rules, rows.Err()
}

// ListByProject returns all rules targeting a specific project
func (s *ClassificationRuleStore) ListByProject(ctx context.Context, userID, projectID uuid.UUID) ([]*ClassificationRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.user_id, r.query, r.project_id, r.attended, r.weight, r.is_enabled,
		       r.created_at, r.updated_at, p.name, p.color
		FROM classification_rules r
		LEFT JOIN projects p ON r.project_id = p.id
		WHERE r.user_id = $1 AND r.project_id = $2
		ORDER BY r.weight DESC, r.created_at ASC
	`, userID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*ClassificationRule
	for rows.Next() {
		rule := &ClassificationRule{}
		err := rows.Scan(
			&rule.ID, &rule.UserID, &rule.Query, &rule.ProjectID, &rule.Attended,
			&rule.Weight, &rule.IsEnabled, &rule.CreatedAt, &rule.UpdatedAt,
			&rule.ProjectName, &rule.ProjectColor,
		)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	return rules, rows.Err()
}

// ListAttendanceRules returns all rules targeting attendance (did not attend)
func (s *ClassificationRuleStore) ListAttendanceRules(ctx context.Context, userID uuid.UUID) ([]*ClassificationRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, query, project_id, attended, weight, is_enabled,
		       created_at, updated_at, NULL, NULL
		FROM classification_rules
		WHERE user_id = $1 AND attended IS NOT NULL AND is_enabled = true
		ORDER BY weight DESC, created_at ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*ClassificationRule
	for rows.Next() {
		rule := &ClassificationRule{}
		err := rows.Scan(
			&rule.ID, &rule.UserID, &rule.Query, &rule.ProjectID, &rule.Attended,
			&rule.Weight, &rule.IsEnabled, &rule.CreatedAt, &rule.UpdatedAt,
			&rule.ProjectName, &rule.ProjectColor,
		)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	return rules, rows.Err()
}

// Update updates a classification rule
func (s *ClassificationRuleStore) Update(ctx context.Context, rule *ClassificationRule) (*ClassificationRule, error) {
	rule.UpdatedAt = time.Now().UTC()

	result, err := s.pool.Exec(ctx, `
		UPDATE classification_rules
		SET query = $3, project_id = $4, attended = $5, weight = $6, is_enabled = $7, updated_at = $8
		WHERE id = $1 AND user_id = $2
	`,
		rule.ID, rule.UserID, rule.Query, rule.ProjectID, rule.Attended,
		rule.Weight, rule.IsEnabled, rule.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	if result.RowsAffected() == 0 {
		return nil, ErrClassificationRuleNotFound
	}

	return s.GetByID(ctx, rule.UserID, rule.ID)
}

// Delete removes a classification rule
func (s *ClassificationRuleStore) Delete(ctx context.Context, userID, ruleID uuid.UUID) error {
	result, err := s.pool.Exec(ctx, `
		DELETE FROM classification_rules WHERE id = $1 AND user_id = $2
	`, ruleID, userID)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrClassificationRuleNotFound
	}

	return nil
}

// RecordOverride records a classification override for feedback
func (s *ClassificationRuleStore) RecordOverride(ctx context.Context, override *ClassificationOverride) error {
	override.ID = uuid.New()
	override.CreatedAt = time.Now().UTC()

	_, err := s.pool.Exec(ctx, `
		INSERT INTO classification_overrides (id, event_id, user_id, from_project_id, to_project_id, from_source, reason, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`,
		override.ID, override.EventID, override.UserID, override.FromProjectID,
		override.ToProjectID, override.FromSource, override.Reason, override.CreatedAt,
	)

	return err
}

// GetRecentOverrides gets recent classification overrides for LLM training
func (s *ClassificationRuleStore) GetRecentOverrides(ctx context.Context, userID uuid.UUID, since time.Time) ([]*ClassificationOverride, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, event_id, user_id, from_project_id, to_project_id, from_source, reason, created_at
		FROM classification_overrides
		WHERE user_id = $1 AND created_at >= $2
		ORDER BY created_at DESC
	`, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var overrides []*ClassificationOverride
	for rows.Next() {
		o := &ClassificationOverride{}
		err := rows.Scan(
			&o.ID, &o.EventID, &o.UserID, &o.FromProjectID,
			&o.ToProjectID, &o.FromSource, &o.Reason, &o.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		overrides = append(overrides, o)
	}

	return overrides, rows.Err()
}
