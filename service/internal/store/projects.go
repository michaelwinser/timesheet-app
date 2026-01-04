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
	ErrProjectNotFound = errors.New("project not found")
	ErrProjectHasEntries = errors.New("project has time entries")
)

// Project represents a stored project
type Project struct {
	ID                     uuid.UUID
	UserID                 uuid.UUID
	Name                   string
	ShortCode              *string
	Client                 *string
	Color                  string
	IsBillable             bool
	IsArchived             bool
	IsHiddenByDefault      bool
	DoesNotAccumulateHours bool
	FingerprintDomains     []string
	FingerprintEmails      []string
	FingerprintKeywords    []string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

// ProjectStore provides PostgreSQL-backed project storage
type ProjectStore struct {
	pool *pgxpool.Pool
}

// NewProjectStore creates a new PostgreSQL project store
func NewProjectStore(pool *pgxpool.Pool) *ProjectStore {
	return &ProjectStore{pool: pool}
}

// Create adds a new project
func (s *ProjectStore) Create(ctx context.Context, userID uuid.UUID, name string, shortCode *string, color string, isBillable, isHiddenByDefault, doesNotAccumulateHours bool) (*Project, error) {
	project := &Project{
		ID:                     uuid.New(),
		UserID:                 userID,
		Name:                   name,
		ShortCode:              shortCode,
		Color:                  color,
		IsBillable:             isBillable,
		IsArchived:             false,
		IsHiddenByDefault:      isHiddenByDefault,
		DoesNotAccumulateHours: doesNotAccumulateHours,
		CreatedAt:              time.Now().UTC(),
		UpdatedAt:              time.Now().UTC(),
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO projects (id, user_id, name, short_code, color, is_billable, is_archived, is_hidden_by_default, does_not_accumulate_hours, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, project.ID, project.UserID, project.Name, project.ShortCode, project.Color,
		project.IsBillable, project.IsArchived, project.IsHiddenByDefault,
		project.DoesNotAccumulateHours, project.CreatedAt, project.UpdatedAt)

	if err != nil {
		return nil, err
	}

	return project, nil
}

// GetByID retrieves a project by ID for a specific user
func (s *ProjectStore) GetByID(ctx context.Context, userID, projectID uuid.UUID) (*Project, error) {
	project := &Project{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, name, short_code, client, color, is_billable, is_archived,
		       is_hidden_by_default, does_not_accumulate_hours,
		       fingerprint_domains, fingerprint_emails, fingerprint_keywords,
		       created_at, updated_at
		FROM projects WHERE id = $1 AND user_id = $2
	`, projectID, userID).Scan(
		&project.ID, &project.UserID, &project.Name, &project.ShortCode, &project.Client, &project.Color,
		&project.IsBillable, &project.IsArchived, &project.IsHiddenByDefault,
		&project.DoesNotAccumulateHours,
		&project.FingerprintDomains, &project.FingerprintEmails, &project.FingerprintKeywords,
		&project.CreatedAt, &project.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}
	return project, nil
}

// List retrieves all projects for a user
func (s *ProjectStore) List(ctx context.Context, userID uuid.UUID, includeArchived bool) ([]*Project, error) {
	query := `
		SELECT id, user_id, name, short_code, client, color, is_billable, is_archived,
		       is_hidden_by_default, does_not_accumulate_hours,
		       fingerprint_domains, fingerprint_emails, fingerprint_keywords,
		       created_at, updated_at
		FROM projects WHERE user_id = $1
	`
	if !includeArchived {
		query += " AND is_archived = false"
	}
	query += " ORDER BY name"

	rows, err := s.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		p := &Project{}
		err := rows.Scan(
			&p.ID, &p.UserID, &p.Name, &p.ShortCode, &p.Client, &p.Color,
			&p.IsBillable, &p.IsArchived, &p.IsHiddenByDefault,
			&p.DoesNotAccumulateHours,
			&p.FingerprintDomains, &p.FingerprintEmails, &p.FingerprintKeywords,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}

	return projects, rows.Err()
}

// Update modifies an existing project
func (s *ProjectStore) Update(ctx context.Context, userID, projectID uuid.UUID, updates map[string]interface{}) (*Project, error) {
	// Build dynamic update query
	updates["updated_at"] = time.Now().UTC()

	setClauses := ""
	args := []interface{}{projectID, userID}
	argNum := 3

	for key, value := range updates {
		if setClauses != "" {
			setClauses += ", "
		}
		setClauses += fmt.Sprintf("%s = $%d", key, argNum)
		args = append(args, value)
		argNum++
	}

	query := "UPDATE projects SET " + setClauses + " WHERE id = $1 AND user_id = $2 RETURNING id, user_id, name, short_code, client, color, is_billable, is_archived, is_hidden_by_default, does_not_accumulate_hours, fingerprint_domains, fingerprint_emails, fingerprint_keywords, created_at, updated_at"

	project := &Project{}
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&project.ID, &project.UserID, &project.Name, &project.ShortCode, &project.Client, &project.Color,
		&project.IsBillable, &project.IsArchived, &project.IsHiddenByDefault,
		&project.DoesNotAccumulateHours,
		&project.FingerprintDomains, &project.FingerprintEmails, &project.FingerprintKeywords,
		&project.CreatedAt, &project.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}

	return project, nil
}

// Delete removes a project
func (s *ProjectStore) Delete(ctx context.Context, userID, projectID uuid.UUID) error {
	// Check if project has time entries
	var hasEntries bool
	err := s.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM time_entries WHERE project_id = $1)",
		projectID,
	).Scan(&hasEntries)
	if err != nil {
		return err
	}
	if hasEntries {
		return ErrProjectHasEntries
	}

	result, err := s.pool.Exec(ctx,
		"DELETE FROM projects WHERE id = $1 AND user_id = $2",
		projectID, userID,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrProjectNotFound
	}

	return nil
}
