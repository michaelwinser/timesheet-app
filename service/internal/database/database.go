package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps the PostgreSQL connection pool
type DB struct {
	Pool *pgxpool.Pool
}

// New creates a new database connection pool
func New(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Close closes the database connection pool
func (db *DB) Close() {
	db.Pool.Close()
}

// Migrate runs database migrations
func (db *DB) Migrate(ctx context.Context) error {
	// Create migrations table if not exists
	_, err := db.Pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Run migrations
	for _, m := range migrations {
		if err := db.runMigration(ctx, m); err != nil {
			return err
		}
	}

	return nil
}

func (db *DB) runMigration(ctx context.Context, m migration) error {
	// Check if already applied
	var exists bool
	err := db.Pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)",
		m.version,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check migration %d: %w", m.version, err)
	}

	if exists {
		return nil
	}

	// Run migration
	_, err = db.Pool.Exec(ctx, m.sql)
	if err != nil {
		return fmt.Errorf("failed to run migration %d: %w", m.version, err)
	}

	// Record migration
	_, err = db.Pool.Exec(ctx,
		"INSERT INTO schema_migrations (version) VALUES ($1)",
		m.version,
	)
	if err != nil {
		return fmt.Errorf("failed to record migration %d: %w", m.version, err)
	}

	return nil
}

type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		sql: `
			CREATE TABLE IF NOT EXISTS users (
				id UUID PRIMARY KEY,
				email TEXT NOT NULL UNIQUE,
				name TEXT NOT NULL,
				password_hash TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
		`,
	},
	{
		version: 2,
		sql: `
			CREATE TABLE IF NOT EXISTS projects (
				id UUID PRIMARY KEY,
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				name TEXT NOT NULL,
				short_code TEXT,
				color TEXT NOT NULL DEFAULT '#6B7280',
				is_billable BOOLEAN NOT NULL DEFAULT true,
				is_archived BOOLEAN NOT NULL DEFAULT false,
				is_hidden_by_default BOOLEAN NOT NULL DEFAULT false,
				does_not_accumulate_hours BOOLEAN NOT NULL DEFAULT false,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE INDEX IF NOT EXISTS idx_projects_user_id ON projects(user_id);
		`,
	},
	{
		version: 3,
		sql: `
			CREATE TABLE IF NOT EXISTS time_entries (
				id UUID PRIMARY KEY,
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				project_id UUID NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
				date DATE NOT NULL,
				hours DECIMAL(5,2) NOT NULL DEFAULT 0,
				description TEXT,
				source TEXT NOT NULL DEFAULT 'manual',
				invoice_id UUID,
				has_user_edits BOOLEAN NOT NULL DEFAULT false,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(user_id, project_id, date)
			);

			CREATE INDEX IF NOT EXISTS idx_time_entries_user_id ON time_entries(user_id);
			CREATE INDEX IF NOT EXISTS idx_time_entries_project_id ON time_entries(project_id);
			CREATE INDEX IF NOT EXISTS idx_time_entries_date ON time_entries(date);
		`,
	},
}
