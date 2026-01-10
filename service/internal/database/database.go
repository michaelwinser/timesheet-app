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

// Consolidated schema as of 2026-01-07
// Previous migrations 1-19 have been collapsed into this single initial schema.
var migrations = []migration{
	{
		version: 1,
		sql: `
			-- =============================================================================
			-- ENUMS
			-- =============================================================================

			CREATE TYPE classification_status AS ENUM ('pending', 'classified');
			CREATE TYPE classification_source AS ENUM ('rule', 'fingerprint', 'manual', 'llm');
			CREATE TYPE invoice_status AS ENUM ('draft', 'sent', 'paid');

			-- =============================================================================
			-- USERS
			-- =============================================================================

			CREATE TABLE users (
				id UUID PRIMARY KEY,
				email TEXT NOT NULL UNIQUE,
				name TEXT NOT NULL,
				password_hash TEXT NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE INDEX idx_users_email ON users(email);

			-- =============================================================================
			-- PROJECTS
			-- =============================================================================

			CREATE TABLE projects (
				id UUID PRIMARY KEY,
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				name TEXT NOT NULL,
				short_code TEXT,
				client TEXT,
				color TEXT NOT NULL DEFAULT '#6B7280',
				is_billable BOOLEAN NOT NULL DEFAULT true,
				is_archived BOOLEAN NOT NULL DEFAULT false,
				is_hidden_by_default BOOLEAN NOT NULL DEFAULT false,
				does_not_accumulate_hours BOOLEAN NOT NULL DEFAULT false,
				-- Fingerprint fields for auto-classification
				fingerprint_domains TEXT[] DEFAULT '{}',
				fingerprint_emails TEXT[] DEFAULT '{}',
				fingerprint_keywords TEXT[] DEFAULT '{}',
				-- Google Sheets integration
				sheets_spreadsheet_id TEXT,
				sheets_spreadsheet_url TEXT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE INDEX idx_projects_user_id ON projects(user_id);

			-- =============================================================================
			-- TIME ENTRIES
			-- =============================================================================

			CREATE TABLE time_entries (
				id UUID PRIMARY KEY,
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				project_id UUID NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
				date DATE NOT NULL,
				hours DECIMAL(5,2) NOT NULL DEFAULT 0,
				title TEXT,
				description TEXT,
				source TEXT NOT NULL DEFAULT 'manual',
				invoice_id UUID,
				has_user_edits BOOLEAN NOT NULL DEFAULT false,
				-- Protection model
				is_pinned BOOLEAN NOT NULL DEFAULT false,
				is_locked BOOLEAN NOT NULL DEFAULT false,
				is_stale BOOLEAN NOT NULL DEFAULT false,
				-- Computed fields from calendar events
				computed_hours DECIMAL(5,2),
				computed_title TEXT,
				computed_description TEXT,
				calculation_details JSONB,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(user_id, project_id, date)
			);

			CREATE INDEX idx_time_entries_user_id ON time_entries(user_id);
			CREATE INDEX idx_time_entries_project_id ON time_entries(project_id);
			CREATE INDEX idx_time_entries_date ON time_entries(date);
			CREATE INDEX idx_time_entries_is_stale ON time_entries(is_stale) WHERE is_stale = true;
			CREATE INDEX idx_time_entries_invoice_id ON time_entries(invoice_id) WHERE invoice_id IS NOT NULL;

			-- =============================================================================
			-- CALENDAR CONNECTIONS
			-- =============================================================================

			CREATE TABLE calendar_connections (
				id UUID PRIMARY KEY,
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				provider TEXT NOT NULL DEFAULT 'google',
				credentials_encrypted BYTEA NOT NULL,
				sync_token TEXT,
				last_synced_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(user_id, provider)
			);

			CREATE INDEX idx_calendar_connections_user_id ON calendar_connections(user_id);

			-- =============================================================================
			-- CALENDARS
			-- =============================================================================

			CREATE TABLE calendars (
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
				min_synced_date DATE,
				max_synced_date DATE,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(connection_id, external_id)
			);

			CREATE INDEX idx_calendars_connection_id ON calendars(connection_id);
			CREATE INDEX idx_calendars_user_id ON calendars(user_id);

			-- =============================================================================
			-- CLASSIFICATION RULES
			-- =============================================================================

			CREATE TABLE classification_rules (
				id UUID PRIMARY KEY,
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				query TEXT NOT NULL,
				project_id UUID REFERENCES projects(id) ON DELETE CASCADE,
				attended BOOLEAN,
				weight FLOAT NOT NULL DEFAULT 1.0,
				is_enabled BOOLEAN NOT NULL DEFAULT true,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				-- Must target either project or attendance, not both
				CONSTRAINT rule_has_target CHECK (
					(project_id IS NOT NULL AND attended IS NULL) OR
					(project_id IS NULL AND attended IS NOT NULL)
				)
			);

			CREATE INDEX idx_classification_rules_user_id ON classification_rules(user_id);
			CREATE INDEX idx_classification_rules_project_id ON classification_rules(project_id);

			-- =============================================================================
			-- CALENDAR EVENTS
			-- =============================================================================

			CREATE TABLE calendar_events (
				id UUID PRIMARY KEY,
				connection_id UUID NOT NULL REFERENCES calendar_connections(id) ON DELETE CASCADE,
				calendar_id UUID REFERENCES calendars(id) ON DELETE CASCADE,
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
				is_locked BOOLEAN NOT NULL DEFAULT false,
				is_skipped BOOLEAN NOT NULL DEFAULT false,
				classification_status classification_status NOT NULL DEFAULT 'pending',
				classification_source classification_source,
				classification_confidence FLOAT,
				classification_rule_id UUID REFERENCES classification_rules(id) ON DELETE SET NULL,
				needs_review BOOLEAN NOT NULL DEFAULT false,
				project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(connection_id, external_id)
			);

			CREATE INDEX idx_calendar_events_user_id ON calendar_events(user_id);
			CREATE INDEX idx_calendar_events_connection_id ON calendar_events(connection_id);
			CREATE INDEX idx_calendar_events_calendar_id ON calendar_events(calendar_id);
			CREATE INDEX idx_calendar_events_start_time ON calendar_events(start_time);
			CREATE INDEX idx_calendar_events_classification ON calendar_events(classification_status);
			CREATE INDEX idx_calendar_events_needs_review ON calendar_events(needs_review) WHERE needs_review = true;
			CREATE INDEX idx_calendar_events_is_skipped ON calendar_events(is_skipped) WHERE is_skipped = true;

			-- =============================================================================
			-- CLASSIFICATION OVERRIDES
			-- =============================================================================

			CREATE TABLE classification_overrides (
				id UUID PRIMARY KEY,
				event_id UUID NOT NULL REFERENCES calendar_events(id) ON DELETE CASCADE,
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				from_project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
				to_project_id UUID REFERENCES projects(id) ON DELETE SET NULL,
				from_source TEXT,
				reason TEXT,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			);

			CREATE INDEX idx_classification_overrides_event_id ON classification_overrides(event_id);
			CREATE INDEX idx_classification_overrides_user_id ON classification_overrides(user_id);

			-- =============================================================================
			-- TIME ENTRY EVENTS (junction table)
			-- =============================================================================

			CREATE TABLE time_entry_events (
				time_entry_id UUID NOT NULL REFERENCES time_entries(id) ON DELETE CASCADE,
				calendar_event_id UUID NOT NULL REFERENCES calendar_events(id) ON DELETE CASCADE,
				PRIMARY KEY (time_entry_id, calendar_event_id)
			);

			CREATE INDEX idx_time_entry_events_entry_id ON time_entry_events(time_entry_id);
			CREATE INDEX idx_time_entry_events_event_id ON time_entry_events(calendar_event_id);

			-- =============================================================================
			-- API KEYS
			-- =============================================================================

			CREATE TABLE api_keys (
				id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				name VARCHAR(255) NOT NULL,
				key_hash VARCHAR(64) NOT NULL,
				key_prefix VARCHAR(12) NOT NULL,
				last_used_at TIMESTAMPTZ,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(user_id, name)
			);

			CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);
			CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);

			-- =============================================================================
			-- MCP OAUTH
			-- =============================================================================

			CREATE TABLE mcp_oauth_sessions (
				id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				state TEXT NOT NULL UNIQUE,
				code_challenge TEXT NOT NULL,
				code_challenge_method TEXT NOT NULL,
				redirect_uri TEXT NOT NULL,
				auth_code TEXT,
				auth_code_expires_at TIMESTAMPTZ,
				user_id UUID REFERENCES users(id) ON DELETE CASCADE,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				expires_at TIMESTAMPTZ NOT NULL
			);

			CREATE INDEX idx_mcp_oauth_sessions_state ON mcp_oauth_sessions(state);
			CREATE INDEX idx_mcp_oauth_sessions_auth_code ON mcp_oauth_sessions(auth_code) WHERE auth_code IS NOT NULL;

			CREATE TABLE mcp_access_tokens (
				id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				token_hash VARCHAR(64) NOT NULL,
				token_prefix VARCHAR(12) NOT NULL,
				expires_at TIMESTAMPTZ NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				last_used_at TIMESTAMPTZ
			);

			CREATE INDEX idx_mcp_access_tokens_token_hash ON mcp_access_tokens(token_hash);
			CREATE INDEX idx_mcp_access_tokens_user_id ON mcp_access_tokens(user_id);

			-- =============================================================================
			-- BILLING PERIODS
			-- =============================================================================

			CREATE TABLE billing_periods (
				id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
				starts_on DATE NOT NULL,
				ends_on DATE,
				hourly_rate DECIMAL(10,2) NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				CONSTRAINT billing_period_dates_valid CHECK (ends_on IS NULL OR ends_on >= starts_on)
			);

			CREATE INDEX idx_billing_periods_user_id ON billing_periods(user_id);
			CREATE INDEX idx_billing_periods_project_id ON billing_periods(project_id);
			CREATE INDEX idx_billing_periods_dates ON billing_periods(project_id, starts_on, ends_on);

			-- Trigger to prevent overlapping billing periods
			CREATE OR REPLACE FUNCTION check_billing_period_overlap()
			RETURNS TRIGGER AS $$
			BEGIN
				IF EXISTS (
					SELECT 1 FROM billing_periods
					WHERE project_id = NEW.project_id
					AND id != COALESCE(NEW.id, '00000000-0000-0000-0000-000000000000'::uuid)
					AND (
						(NEW.starts_on, COALESCE(NEW.ends_on, '9999-12-31'::date)) OVERLAPS
						(starts_on, COALESCE(ends_on, '9999-12-31'::date))
					)
				) THEN
					RAISE EXCEPTION 'Billing periods for project % cannot overlap', NEW.project_id;
				END IF;
				RETURN NEW;
			END;
			$$ LANGUAGE plpgsql;

			CREATE TRIGGER billing_period_overlap_check
			BEFORE INSERT OR UPDATE ON billing_periods
			FOR EACH ROW EXECUTE FUNCTION check_billing_period_overlap();

			-- =============================================================================
			-- INVOICES
			-- =============================================================================

			CREATE TABLE invoices (
				id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				project_id UUID NOT NULL REFERENCES projects(id) ON DELETE RESTRICT,
				billing_period_id UUID REFERENCES billing_periods(id) ON DELETE SET NULL,
				invoice_number TEXT NOT NULL,
				period_start DATE NOT NULL,
				period_end DATE NOT NULL,
				invoice_date DATE NOT NULL,
				status invoice_status NOT NULL DEFAULT 'draft',
				total_hours DECIMAL(10,2) NOT NULL DEFAULT 0,
				total_amount DECIMAL(10,2) NOT NULL DEFAULT 0,
				spreadsheet_id TEXT,
				spreadsheet_url TEXT,
				worksheet_id INTEGER,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(user_id, invoice_number)
			);

			CREATE INDEX idx_invoices_user_id ON invoices(user_id);
			CREATE INDEX idx_invoices_project_id ON invoices(project_id);
			CREATE INDEX idx_invoices_status ON invoices(status);
			CREATE INDEX idx_invoices_number ON invoices(invoice_number);

			-- =============================================================================
			-- INVOICE LINE ITEMS
			-- =============================================================================

			CREATE TABLE invoice_line_items (
				id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
				time_entry_id UUID NOT NULL REFERENCES time_entries(id) ON DELETE RESTRICT,
				date DATE NOT NULL,
				description TEXT,
				hours DECIMAL(10,2) NOT NULL,
				hourly_rate DECIMAL(10,2) NOT NULL,
				amount DECIMAL(10,2) NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				UNIQUE(invoice_id, time_entry_id)
			);

			CREATE INDEX idx_invoice_line_items_invoice_id ON invoice_line_items(invoice_id);
			CREATE INDEX idx_invoice_line_items_time_entry_id ON invoice_line_items(time_entry_id);
		`,
	},
	{
		version: 2,
		sql: `
			-- =============================================================================
			-- CALENDAR SYNC V2: Add columns for background sync and auth tracking
			-- =============================================================================

			-- Track consecutive sync failures (stop retrying after 3)
			ALTER TABLE calendars ADD COLUMN sync_failure_count INT NOT NULL DEFAULT 0;

			-- Track if calendar needs re-authentication (OAuth token revoked)
			ALTER TABLE calendars ADD COLUMN needs_reauth BOOLEAN NOT NULL DEFAULT FALSE;

			-- Index for background sync job: find stale calendars that aren't broken
			CREATE INDEX idx_calendars_background_sync
				ON calendars (last_synced_at)
				WHERE needs_reauth = FALSE AND sync_failure_count < 3;
		`,
	},
	{
		version: 3,
		sql: `
			-- =============================================================================
			-- CALENDAR SYNC JOBS: Background job queue for watermark expansion
			-- =============================================================================

			CREATE TABLE calendar_sync_jobs (
				id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
				calendar_id UUID NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
				job_type TEXT NOT NULL,
				target_min_date DATE NOT NULL,
				target_max_date DATE NOT NULL,
				status TEXT NOT NULL DEFAULT 'pending',
				priority INT NOT NULL DEFAULT 0,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				claimed_at TIMESTAMPTZ,
				completed_at TIMESTAMPTZ,
				error_message TEXT,
				claimed_by TEXT,
				CONSTRAINT valid_date_range CHECK (target_min_date <= target_max_date)
			);

			CREATE INDEX idx_sync_jobs_pending ON calendar_sync_jobs (calendar_id, priority DESC, created_at ASC)
				WHERE status = 'pending';
			CREATE INDEX idx_sync_jobs_calendar ON calendar_sync_jobs (calendar_id, status);
			CREATE INDEX idx_sync_jobs_completed ON calendar_sync_jobs (completed_at)
				WHERE status IN ('completed', 'failed');
		`,
	},
	{
		version: 4,
		sql: `
			-- =============================================================================
			-- EPHEMERAL TIME ENTRIES: Support for computed-on-demand time entries
			-- =============================================================================

			-- Snapshot of computed_hours at materialization time (for staleness detection)
			ALTER TABLE time_entries ADD COLUMN snapshot_computed_hours DECIMAL(5,2);

			-- Flag for user-suppressed entries (prevents re-computation from recreating)
			ALTER TABLE time_entries ADD COLUMN is_suppressed BOOLEAN NOT NULL DEFAULT FALSE;

			-- Backfill snapshot for existing materialized entries
			UPDATE time_entries
			SET snapshot_computed_hours = computed_hours
			WHERE has_user_edits = true
			  AND snapshot_computed_hours IS NULL
			  AND computed_hours IS NOT NULL;

			UPDATE time_entries
			SET snapshot_computed_hours = computed_hours
			WHERE invoice_id IS NOT NULL
			  AND snapshot_computed_hours IS NULL
			  AND computed_hours IS NOT NULL;
		`,
	},
	{
		version: 5,
		sql: `
			-- =============================================================================
			-- ALL-DAY EVENT FLAG: Proper timezone handling for calendar events
			-- =============================================================================

			ALTER TABLE calendar_events ADD COLUMN is_all_day BOOLEAN NOT NULL DEFAULT FALSE;

			-- Backfill: events spanning 24+ hours with midnight boundaries are likely all-day
			UPDATE calendar_events
			SET is_all_day = true
			WHERE is_all_day = false
			  AND EXTRACT(HOUR FROM start_time AT TIME ZONE 'UTC') = 0
			  AND EXTRACT(MINUTE FROM start_time AT TIME ZONE 'UTC') = 0
			  AND EXTRACT(HOUR FROM end_time AT TIME ZONE 'UTC') = 0
			  AND EXTRACT(MINUTE FROM end_time AT TIME ZONE 'UTC') = 0
			  AND end_time - start_time >= INTERVAL '24 hours';
		`,
	},
	{
		version: 6,
		sql: `
			-- =============================================================================
			-- UNIQUE SHORT CODES: Enforce unique project short codes per user
			-- =============================================================================

			-- Partial unique index: only applies to non-NULL short codes
			CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_user_short_code_unique
			ON projects (user_id, short_code)
			WHERE short_code IS NOT NULL;
		`,
	},
}
