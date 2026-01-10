-- Migration: All-Day Event Flag
--
-- Adds is_all_day column to calendar_events to properly distinguish
-- all-day events from timed events. Google Calendar provides this
-- information via Start.Date vs Start.DateTime, but we weren't storing it.
--
-- This fixes timezone display bugs where timed events starting at midnight UTC
-- were incorrectly treated as all-day events.

-- =============================================================================
-- CALENDAR_EVENTS - Add is_all_day flag
-- =============================================================================

ALTER TABLE calendar_events ADD COLUMN IF NOT EXISTS is_all_day BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN calendar_events.is_all_day IS
    'True if this is an all-day event (no specific start/end times). Determined by Google Calendar API using Date vs DateTime fields.';

-- =============================================================================
-- Backfill is_all_day for existing events
-- =============================================================================

-- Heuristic: events spanning 24+ hours with midnight start times are likely all-day.
-- This is imperfect but better than nothing for existing data.
-- New syncs will set the flag correctly based on the Google Calendar API response.
UPDATE calendar_events
SET is_all_day = true
WHERE is_all_day = false
  AND EXTRACT(HOUR FROM start_time AT TIME ZONE 'UTC') = 0
  AND EXTRACT(MINUTE FROM start_time AT TIME ZONE 'UTC') = 0
  AND EXTRACT(HOUR FROM end_time AT TIME ZONE 'UTC') = 0
  AND EXTRACT(MINUTE FROM end_time AT TIME ZONE 'UTC') = 0
  AND end_time - start_time >= INTERVAL '24 hours';
