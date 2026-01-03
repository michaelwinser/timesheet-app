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
	{
		version: 4,
		sql: `
			CREATE TABLE IF NOT EXISTS calendar_connections (
				id UUID PRIMARY KEY,
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				provider TEXT NOT NULL DEFAULT 'google',
				credentials_encrypted BYTEA NOT NULL,
				last_synced_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(user_id, provider)
			);

			CREATE INDEX IF NOT EXISTS idx_calendar_connections_user_id ON calendar_connections(user_id);
		`,
	},
	{
		version: 5,
		sql: `
			DO $$ BEGIN
				CREATE TYPE classification_status AS ENUM ('pending', 'classified', 'skipped');
			EXCEPTION
				WHEN duplicate_object THEN null;
			END $$;

			DO $$ BEGIN
				CREATE TYPE classification_source AS ENUM ('rule', 'fingerprint', 'manual', 'llm');
			EXCEPTION
				WHEN duplicate_object THEN null;
			END $$;

			CREATE TABLE IF NOT EXISTS calendar_events (
				id UUID PRIMARY KEY,
				connection_id UUID NOT NULL REFERENCES calendar_connections(id) ON DELETE CASCADE,
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				external_id TEXT NOT NULL,
				title TEXT NOT NULL,
				description TEXT,
				start_time TIMESTAMPTZ NOT NULL,
				end_time TIMESTAMPTZ NOT NULL,
				attendees JSONB DEFAULT '[]',
				is_recurring BOOLEAN NOT NULL DEFAULT false,
				response_status TEXT,
				transparency TEXT,
				is_orphaned BOOLEAN NOT NULL DEFAULT false,
				is_suppressed BOOLEAN NOT NULL DEFAULT false,
				classification_status classification_status NOT NULL DEFAULT 'pending',
				classification_source classification_source,
				project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(connection_id, external_id)
			);

			CREATE INDEX IF NOT EXISTS idx_calendar_events_user_id ON calendar_events(user_id);
			CREATE INDEX IF NOT EXISTS idx_calendar_events_connection_id ON calendar_events(connection_id);
			CREATE INDEX IF NOT EXISTS idx_calendar_events_start_time ON calendar_events(start_time);
			CREATE INDEX IF NOT EXISTS idx_calendar_events_classification ON calendar_events(classification_status);
		`,
	},
	{
		version: 6,
		sql: `
			ALTER TABLE calendar_connections
			ADD COLUMN IF NOT EXISTS sync_token TEXT;
		`,
	},
	{
		version: 7,
		sql: `
			-- Create calendars table for multi-calendar support
			CREATE TABLE IF NOT EXISTS calendars (
				id UUID PRIMARY KEY,
				connection_id UUID NOT NULL REFERENCES calendar_connections(id) ON DELETE CASCADE,
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				external_id TEXT NOT NULL,
				name TEXT NOT NULL,
				color TEXT,
				is_primary BOOLEAN NOT NULL DEFAULT false,
				is_selected BOOLEAN NOT NULL DEFAULT false,
				sync_token TEXT,
				last_synced_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(connection_id, external_id)
			);

			CREATE INDEX IF NOT EXISTS idx_calendars_connection_id ON calendars(connection_id);
			CREATE INDEX IF NOT EXISTS idx_calendars_user_id ON calendars(user_id);

			-- Add calendar_id to calendar_events (nullable for backward compatibility)
			ALTER TABLE calendar_events
			ADD COLUMN IF NOT EXISTS calendar_id UUID REFERENCES calendars(id) ON DELETE CASCADE;

			CREATE INDEX IF NOT EXISTS idx_calendar_events_calendar_id ON calendar_events(calendar_id);

			-- Migrate existing data: create "primary" calendar for each existing connection
			-- and link existing events to it
			INSERT INTO calendars (id, connection_id, user_id, external_id, name, is_primary, is_selected, sync_token, last_synced_at, created_at, updated_at)
			SELECT
				gen_random_uuid(),
				cc.id,
				cc.user_id,
				'primary',
				'Primary Calendar',
				true,
				true,
				cc.sync_token,
				cc.last_synced_at,
				cc.created_at,
				cc.updated_at
			FROM calendar_connections cc
			WHERE NOT EXISTS (
				SELECT 1 FROM calendars c WHERE c.connection_id = cc.id AND c.external_id = 'primary'
			);

			-- Update existing events to reference the primary calendar
			UPDATE calendar_events ce
			SET calendar_id = c.id
			FROM calendars c
			WHERE ce.connection_id = c.connection_id
			AND c.external_id = 'primary'
			AND ce.calendar_id IS NULL;
		`,
	},
}
