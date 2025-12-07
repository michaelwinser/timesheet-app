-- Initial database schema for Timesheet App

-- OAuth tokens for Google Calendar access
CREATE TABLE IF NOT EXISTS auth_tokens (
    id INTEGER PRIMARY KEY,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    token_expiry TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- User-defined projects for classification
CREATE TABLE IF NOT EXISTS projects (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    client TEXT,
    color TEXT DEFAULT '#00aa44',
    is_visible INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Calendar events fetched from Google
CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY,
    google_event_id TEXT NOT NULL UNIQUE,
    calendar_id TEXT NOT NULL,
    title TEXT,
    description TEXT,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    attendees TEXT,
    meeting_link TEXT,
    event_color TEXT,
    recurrence_id TEXT,
    is_recurring INTEGER DEFAULT 0,
    raw_json TEXT,
    fetched_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Classified time entries
CREATE TABLE IF NOT EXISTS time_entries (
    id INTEGER PRIMARY KEY,
    event_id INTEGER NOT NULL REFERENCES events(id),
    project_id INTEGER NOT NULL REFERENCES projects(id),
    hours REAL NOT NULL,
    description TEXT,
    classified_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    classification_source TEXT,
    UNIQUE(event_id)
);

-- Classification rules
CREATE TABLE IF NOT EXISTS classification_rules (
    id INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES projects(id),
    rule_type TEXT NOT NULL,
    rule_value TEXT NOT NULL,
    priority INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Classification history for learning/debugging
CREATE TABLE IF NOT EXISTS classification_history (
    id INTEGER PRIMARY KEY,
    event_id INTEGER NOT NULL REFERENCES events(id),
    rule_id INTEGER REFERENCES classification_rules(id),
    project_id INTEGER NOT NULL REFERENCES projects(id),
    confidence REAL,
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_events_start_time ON events(start_time);
CREATE INDEX IF NOT EXISTS idx_events_google_id ON events(google_event_id);
CREATE INDEX IF NOT EXISTS idx_events_recurrence ON events(recurrence_id);
CREATE INDEX IF NOT EXISTS idx_time_entries_event ON time_entries(event_id);
CREATE INDEX IF NOT EXISTS idx_classification_rules_project ON classification_rules(project_id);
