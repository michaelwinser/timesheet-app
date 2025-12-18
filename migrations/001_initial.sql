-- PostgreSQL schema for Timesheet App with multi-user support
-- This schema enforces user data isolation at the database level

-- =============================================================================
-- USERS TABLE - Core of multi-user support
-- =============================================================================
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    last_login_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

COMMENT ON TABLE users IS 'User accounts - one per Google OAuth login';
COMMENT ON COLUMN users.email IS 'User email from Google OAuth (unique identifier)';
COMMENT ON COLUMN users.last_login_at IS 'Timestamp of most recent login (updated on each OAuth callback)';

-- =============================================================================
-- AUTH_TOKENS - OAuth tokens for Google Calendar access (per user)
-- =============================================================================
CREATE TABLE IF NOT EXISTS auth_tokens (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    token_expiry TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- Only one active token per user
    UNIQUE(user_id)
);

CREATE INDEX IF NOT EXISTS idx_auth_tokens_user ON auth_tokens(user_id);

COMMENT ON TABLE auth_tokens IS 'Google OAuth tokens for Calendar API access - one per user';
COMMENT ON COLUMN auth_tokens.user_id IS 'Foreign key to users table';

-- =============================================================================
-- PROJECTS - User-defined project categories for time tracking
-- =============================================================================
CREATE TABLE IF NOT EXISTS projects (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    client VARCHAR(255),
    color VARCHAR(20) DEFAULT '#00aa44',
    is_visible BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- Project names unique per user (different users can have same project name)
    UNIQUE(user_id, name)
);

CREATE INDEX IF NOT EXISTS idx_projects_user ON projects(user_id);
CREATE INDEX IF NOT EXISTS idx_projects_visible ON projects(user_id, is_visible);

COMMENT ON TABLE projects IS 'User-defined projects for classifying calendar events';
COMMENT ON COLUMN projects.is_visible IS 'Whether project is shown in UI (false = hidden/archived)';

-- =============================================================================
-- EVENTS - Calendar events fetched from Google (per user)
-- =============================================================================
CREATE TABLE IF NOT EXISTS events (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    google_event_id VARCHAR(1024) NOT NULL,
    calendar_id VARCHAR(255) NOT NULL,
    title TEXT,
    description TEXT,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    attendees TEXT,  -- JSON array stored as text
    meeting_link TEXT,
    event_color VARCHAR(50),
    recurrence_id VARCHAR(1024),
    is_recurring BOOLEAN DEFAULT false,
    my_response_status VARCHAR(50),  -- accepted, declined, tentative, needsAction
    transparency VARCHAR(50),  -- transparent, opaque
    visibility VARCHAR(50),  -- public, private, confidential
    raw_json TEXT,  -- Full Google Calendar event JSON for debugging
    fetched_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- Google event IDs unique per user (different users can have overlapping event IDs)
    UNIQUE(user_id, google_event_id)
);

CREATE INDEX IF NOT EXISTS idx_events_user ON events(user_id);
CREATE INDEX IF NOT EXISTS idx_events_start_time ON events(user_id, start_time);
CREATE INDEX IF NOT EXISTS idx_events_recurrence ON events(user_id, recurrence_id);

COMMENT ON TABLE events IS 'Google Calendar events synced locally for each user';
COMMENT ON COLUMN events.google_event_id IS 'Unique ID from Google Calendar API';
COMMENT ON COLUMN events.my_response_status IS 'User response status (for filtering out declined meetings)';
COMMENT ON COLUMN events.transparency IS 'transparent = free time, opaque = busy';
COMMENT ON COLUMN events.raw_json IS 'Full event JSON from Google API for debugging';

-- =============================================================================
-- TIME_ENTRIES - Classified time entries (per user)
-- =============================================================================
CREATE TABLE IF NOT EXISTS time_entries (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    hours REAL NOT NULL,
    description TEXT,
    classified_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    classification_source VARCHAR(50),  -- manual, rule, llm
    rule_id INTEGER,  -- Foreign key added below (after classification_rules table exists)

    -- One time entry per event
    UNIQUE(event_id)
);

CREATE INDEX IF NOT EXISTS idx_time_entries_user ON time_entries(user_id);
CREATE INDEX IF NOT EXISTS idx_time_entries_event ON time_entries(event_id);
CREATE INDEX IF NOT EXISTS idx_time_entries_project ON time_entries(user_id, project_id);

COMMENT ON TABLE time_entries IS 'User classifications of events to projects (the "timesheet")';
COMMENT ON COLUMN time_entries.hours IS 'Duration in hours (calculated from event start/end)';
COMMENT ON COLUMN time_entries.classification_source IS 'How this was classified: manual, rule, or llm';

-- =============================================================================
-- CLASSIFICATION_RULES - Rules engine for automatic event classification
-- =============================================================================
CREATE TABLE IF NOT EXISTS classification_rules (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    priority INTEGER DEFAULT 0,
    is_enabled BOOLEAN DEFAULT true,
    stop_processing BOOLEAN DEFAULT true,  -- If true, stop checking rules after this one matches
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- Rule names unique per user
    UNIQUE(user_id, name)
);

CREATE INDEX IF NOT EXISTS idx_classification_rules_user ON classification_rules(user_id);
CREATE INDEX IF NOT EXISTS idx_classification_rules_priority ON classification_rules(user_id, priority DESC, id);
CREATE INDEX IF NOT EXISTS idx_classification_rules_enabled ON classification_rules(user_id, is_enabled);

COMMENT ON TABLE classification_rules IS 'User-defined rules for automatic event classification';
COMMENT ON COLUMN classification_rules.priority IS 'Higher priority rules evaluated first';
COMMENT ON COLUMN classification_rules.stop_processing IS 'If true and rule matches, skip remaining rules';

-- =============================================================================
-- RULE_CONDITIONS - Individual conditions for classification rules (AND logic)
-- =============================================================================
CREATE TABLE IF NOT EXISTS rule_conditions (
    id SERIAL PRIMARY KEY,
    rule_id INTEGER NOT NULL REFERENCES classification_rules(id) ON DELETE CASCADE,
    property_name VARCHAR(255) NOT NULL,  -- title, description, attendees, etc.
    condition_type VARCHAR(100) NOT NULL,  -- contains, equals, starts_with, etc.
    condition_value TEXT NOT NULL,  -- JSON-encoded value
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_rule_conditions_rule ON rule_conditions(rule_id);

COMMENT ON TABLE rule_conditions IS 'Conditions for rules - multiple conditions per rule (AND logic)';
COMMENT ON COLUMN rule_conditions.property_name IS 'Event property to check (title, description, etc.)';
COMMENT ON COLUMN rule_conditions.condition_type IS 'Type of check (contains, equals, regex, etc.)';
COMMENT ON COLUMN rule_conditions.condition_value IS 'JSON-encoded value to match against';

-- =============================================================================
-- Add foreign key from time_entries to classification_rules
-- (Must be added after classification_rules table exists)
-- =============================================================================
ALTER TABLE time_entries
    DROP CONSTRAINT IF EXISTS fk_time_entries_rule;

ALTER TABLE time_entries
    ADD CONSTRAINT fk_time_entries_rule
    FOREIGN KEY (rule_id) REFERENCES classification_rules(id) ON DELETE SET NULL;

-- =============================================================================
-- CLASSIFICATION_HISTORY - Audit trail of classification decisions
-- =============================================================================
CREATE TABLE IF NOT EXISTS classification_history (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    event_id INTEGER NOT NULL REFERENCES events(id) ON DELETE CASCADE,
    rule_id INTEGER REFERENCES classification_rules(id) ON DELETE SET NULL,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    confidence REAL,  -- For LLM classifications (0.0 to 1.0)
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_classification_history_user ON classification_history(user_id);
CREATE INDEX IF NOT EXISTS idx_classification_history_event ON classification_history(event_id);

COMMENT ON TABLE classification_history IS 'Historical record of classification decisions (for learning and debugging)';
COMMENT ON COLUMN classification_history.confidence IS 'LLM confidence score (0.0 to 1.0) if classified by AI';
