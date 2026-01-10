-- Migration: Unique Project Short Codes
--
-- Adds a unique constraint on (user_id, short_code) to prevent duplicate
-- short codes within a user's projects. Uses a partial index to allow
-- multiple projects with NULL short codes.

-- =============================================================================
-- PROJECTS - Add unique constraint on short_code per user
-- =============================================================================

-- Create a partial unique index that only applies to non-NULL short codes
-- This allows multiple projects to have NULL short_code while enforcing
-- uniqueness for projects that have a short_code set.
CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_user_short_code_unique
ON projects (user_id, short_code)
WHERE short_code IS NOT NULL;

COMMENT ON INDEX idx_projects_user_short_code_unique IS
    'Ensures short_code is unique per user when set. NULL short codes are allowed for multiple projects.';
