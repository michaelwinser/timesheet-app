-- Migration: Remove deprecated is_visible column from projects
--
-- The is_visible column was the original "archived" concept, but was superseded
-- by is_archived in migration 002. This migration removes the redundant column.
--
-- Before running: Ensure any projects with is_visible=false are either:
--   - Set to is_archived=true, OR
--   - Manually updated as needed

-- =============================================================================
-- PROJECTS - Remove is_visible column
-- =============================================================================

-- Drop the index first
DROP INDEX IF EXISTS idx_projects_visible;

-- Drop the column
ALTER TABLE projects DROP COLUMN IF EXISTS is_visible;

-- Update table comment to reflect current visibility model
COMMENT ON TABLE projects IS 'User-defined projects for classifying calendar events. Visibility controlled by is_archived and is_hidden_by_default.';
