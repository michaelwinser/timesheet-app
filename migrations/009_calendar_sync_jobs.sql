-- Calendar sync job queue for background water mark expansion
-- Jobs are created when users navigate outside current water marks,
-- and processed by a background worker to fill gaps.

CREATE TABLE calendar_sync_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- The calendar this job is for
    calendar_id UUID NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,

    -- Job type: 'expand_watermarks' for filling gaps, 'initial_sync' for first sync
    job_type TEXT NOT NULL,

    -- Target date range to sync
    target_min_date DATE NOT NULL,
    target_max_date DATE NOT NULL,

    -- Job status: pending, running, completed, failed
    status TEXT NOT NULL DEFAULT 'pending',

    -- Priority: higher values processed first (0 = normal, 10 = high/user-initiated)
    priority INT NOT NULL DEFAULT 0,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at TIMESTAMPTZ,           -- When a worker claimed this job
    completed_at TIMESTAMPTZ,         -- When the job finished (success or failure)

    -- Error details if job failed
    error_message TEXT,

    -- Worker identifier (for debugging which instance processed the job)
    claimed_by TEXT,

    -- Constraint: target_min_date must be <= target_max_date
    CONSTRAINT valid_date_range CHECK (target_min_date <= target_max_date)
);

-- Index for finding pending jobs (worker query)
-- Uses partial index to only include pending jobs
CREATE INDEX idx_sync_jobs_pending ON calendar_sync_jobs (calendar_id, priority DESC, created_at ASC)
    WHERE status = 'pending';

-- Index for finding jobs by calendar (for coalescing check)
CREATE INDEX idx_sync_jobs_calendar ON calendar_sync_jobs (calendar_id, status);

-- Index for cleanup of old completed/failed jobs
CREATE INDEX idx_sync_jobs_completed ON calendar_sync_jobs (completed_at)
    WHERE status IN ('completed', 'failed');

-- Comment on table
COMMENT ON TABLE calendar_sync_jobs IS
    'Queue for background calendar sync jobs. Created when user navigates outside water marks.';
COMMENT ON COLUMN calendar_sync_jobs.job_type IS
    'Type of sync: expand_watermarks (fill gap), initial_sync (first calendar sync)';
COMMENT ON COLUMN calendar_sync_jobs.priority IS
    'Job priority: 0=normal (background), 10=high (user-initiated navigation)';
