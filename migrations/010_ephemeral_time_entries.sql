-- Migration: Ephemeral Time Entries
--
-- Supports the ephemeral time entry model where entries are computed on demand
-- and only persisted when the user interacts with them or they are invoiced.
--
-- Adds:
-- 1. snapshot_computed_hours - captures computed hours at materialization time
-- 2. is_suppressed - marks entries the user has explicitly suppressed
--
-- See docs/v2/design-ephemeral-time-entries.md for full design.

-- =============================================================================
-- TIME_ENTRIES - Add ephemeral model support fields
-- =============================================================================

-- Snapshot of computed_hours at the time the user set hours.
-- Used for staleness detection: entry is stale if computed_hours has drifted
-- from this snapshot since the user made their edit.
-- NULL for ephemeral (non-materialized) entries.
ALTER TABLE time_entries ADD COLUMN IF NOT EXISTS snapshot_computed_hours DECIMAL(5,2);

COMMENT ON COLUMN time_entries.snapshot_computed_hours IS
    'Computed hours at materialization time. Used to detect staleness when computed values drift.';

-- Flag indicating the user has explicitly suppressed this entry.
-- Suppressed entries show 0 hours but are tracked to prevent re-computation
-- from recreating them.
ALTER TABLE time_entries ADD COLUMN IF NOT EXISTS is_suppressed BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN time_entries.is_suppressed IS
    'True if user explicitly suppressed this entry. Prevents re-computation from recreating it.';

-- =============================================================================
-- Backfill snapshot_computed_hours for existing materialized entries
-- =============================================================================

-- For entries that have user edits, set snapshot to current computed_hours.
-- This establishes a baseline so existing entries don't immediately appear stale.
UPDATE time_entries
SET snapshot_computed_hours = computed_hours
WHERE has_user_edits = true
  AND snapshot_computed_hours IS NULL
  AND computed_hours IS NOT NULL;

-- For invoiced entries without user edits, also capture the snapshot.
-- These are effectively materialized by the invoicing process.
UPDATE time_entries
SET snapshot_computed_hours = computed_hours
WHERE invoice_id IS NOT NULL
  AND snapshot_computed_hours IS NULL
  AND computed_hours IS NOT NULL;
