-- Migration: Project settings and event attendance tracking
--
-- Adds:
-- 1. New project settings (billing, visibility, hour tracking)
-- 2. Did-not-attend flag on events
-- 3. Rule target type to support did-not-attend rules

-- =============================================================================
-- EVENTS - Add did_not_attend flag
-- =============================================================================
ALTER TABLE events ADD COLUMN IF NOT EXISTS did_not_attend BOOLEAN DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_events_did_not_attend ON events(user_id, did_not_attend);

COMMENT ON COLUMN events.did_not_attend IS 'If true, user did not attend this event (excludes from time tracking)';

-- =============================================================================
-- PROJECTS - Add new settings columns
-- =============================================================================

-- Does not accumulate hours: entries in this project excluded from totals/exports
ALTER TABLE projects ADD COLUMN IF NOT EXISTS does_not_accumulate_hours BOOLEAN DEFAULT FALSE;

-- Billing settings
ALTER TABLE projects ADD COLUMN IF NOT EXISTS is_billable BOOLEAN DEFAULT FALSE;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS bill_rate DECIMAL(10, 2);

-- Visibility settings
ALTER TABLE projects ADD COLUMN IF NOT EXISTS is_hidden_by_default BOOLEAN DEFAULT FALSE;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS is_archived BOOLEAN DEFAULT FALSE;

-- Add index for filtering by visibility states
CREATE INDEX IF NOT EXISTS idx_projects_visibility ON projects(user_id, is_archived, is_hidden_by_default);

COMMENT ON COLUMN projects.does_not_accumulate_hours IS 'If true, time entries excluded from hour totals and exports (e.g., Noise projects)';
COMMENT ON COLUMN projects.is_billable IS 'Whether time on this project is billable';
COMMENT ON COLUMN projects.bill_rate IS 'Hourly billing rate (when is_billable is true)';
COMMENT ON COLUMN projects.is_hidden_by_default IS 'If true, entries hidden in UI by default (shown in collapsed Hidden section)';
COMMENT ON COLUMN projects.is_archived IS 'If true, project hidden from all UI; entries show in Archived warning section';

-- =============================================================================
-- CLASSIFICATION_RULES - Add target_type for did-not-attend rules
-- =============================================================================

-- Add target_type column: 'project' (default) or 'did_not_attend'
ALTER TABLE classification_rules ADD COLUMN IF NOT EXISTS target_type VARCHAR(20) DEFAULT 'project';

-- Make project_id nullable (did_not_attend rules don't need a project)
ALTER TABLE classification_rules ALTER COLUMN project_id DROP NOT NULL;

-- Add check constraint: project rules need project_id, did_not_attend rules don't
-- Note: PostgreSQL doesn't support ADD CONSTRAINT IF NOT EXISTS, so we drop first
ALTER TABLE classification_rules DROP CONSTRAINT IF EXISTS check_rule_target;
ALTER TABLE classification_rules ADD CONSTRAINT check_rule_target CHECK (
    (target_type = 'project' AND project_id IS NOT NULL) OR
    (target_type = 'did_not_attend' AND project_id IS NULL)
);

COMMENT ON COLUMN classification_rules.target_type IS 'Rule target: project (classify to project) or did_not_attend (set attendance flag)';
