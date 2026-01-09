package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSyncJobNotFound = errors.New("sync job not found")

// SyncJobType defines the type of sync job
type SyncJobType string

const (
	SyncJobTypeExpandWatermarks SyncJobType = "expand_watermarks"
	SyncJobTypeInitialSync      SyncJobType = "initial_sync"
)

// SyncJobStatus defines the status of a sync job
type SyncJobStatus string

const (
	SyncJobStatusPending   SyncJobStatus = "pending"
	SyncJobStatusRunning   SyncJobStatus = "running"
	SyncJobStatusCompleted SyncJobStatus = "completed"
	SyncJobStatusFailed    SyncJobStatus = "failed"
)

// SyncJob represents a background calendar sync job
type SyncJob struct {
	ID            uuid.UUID
	CalendarID    uuid.UUID
	JobType       SyncJobType
	TargetMinDate time.Time
	TargetMaxDate time.Time
	Status        SyncJobStatus
	Priority      int
	CreatedAt     time.Time
	ClaimedAt     *time.Time
	CompletedAt   *time.Time
	ErrorMessage  *string
	ClaimedBy     *string
}

// SyncJobStore provides PostgreSQL-backed sync job storage
type SyncJobStore struct {
	pool *pgxpool.Pool
}

// NewSyncJobStore creates a new store
func NewSyncJobStore(pool *pgxpool.Pool) *SyncJobStore {
	return &SyncJobStore{pool: pool}
}

// Create creates a new sync job
func (s *SyncJobStore) Create(ctx context.Context, job *SyncJob) (*SyncJob, error) {
	now := time.Now().UTC()
	job.ID = uuid.New()
	job.Status = SyncJobStatusPending
	job.CreatedAt = now

	err := s.pool.QueryRow(ctx, `
		INSERT INTO calendar_sync_jobs (
			id, calendar_id, job_type, target_min_date, target_max_date,
			status, priority, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at
	`,
		job.ID, job.CalendarID, job.JobType, job.TargetMinDate, job.TargetMaxDate,
		job.Status, job.Priority, job.CreatedAt,
	).Scan(&job.ID, &job.CreatedAt)

	if err != nil {
		return nil, err
	}

	return job, nil
}

// GetByID retrieves a sync job by ID
func (s *SyncJobStore) GetByID(ctx context.Context, jobID uuid.UUID) (*SyncJob, error) {
	job := &SyncJob{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, calendar_id, job_type, target_min_date, target_max_date,
		       status, priority, created_at, claimed_at, completed_at,
		       error_message, claimed_by
		FROM calendar_sync_jobs
		WHERE id = $1
	`, jobID).Scan(
		&job.ID, &job.CalendarID, &job.JobType, &job.TargetMinDate, &job.TargetMaxDate,
		&job.Status, &job.Priority, &job.CreatedAt, &job.ClaimedAt, &job.CompletedAt,
		&job.ErrorMessage, &job.ClaimedBy,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSyncJobNotFound
		}
		return nil, err
	}

	return job, nil
}

// ListPendingByCalendar returns all pending jobs for a calendar, ordered by priority and creation time
func (s *SyncJobStore) ListPendingByCalendar(ctx context.Context, calendarID uuid.UUID) ([]*SyncJob, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, calendar_id, job_type, target_min_date, target_max_date,
		       status, priority, created_at, claimed_at, completed_at,
		       error_message, claimed_by
		FROM calendar_sync_jobs
		WHERE calendar_id = $1 AND status = 'pending'
		ORDER BY priority DESC, created_at ASC
	`, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSyncJobs(rows)
}

// ClaimNextJob atomically claims the next pending job for processing.
// It uses SELECT FOR UPDATE SKIP LOCKED to safely handle concurrent workers.
// Returns nil if no pending jobs are available.
func (s *SyncJobStore) ClaimNextJob(ctx context.Context, workerID string) (*SyncJob, error) {
	now := time.Now().UTC()

	job := &SyncJob{}
	err := s.pool.QueryRow(ctx, `
		UPDATE calendar_sync_jobs
		SET status = 'running', claimed_at = $2, claimed_by = $3
		WHERE id = (
			SELECT id FROM calendar_sync_jobs
			WHERE status = 'pending'
			ORDER BY priority DESC, created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, calendar_id, job_type, target_min_date, target_max_date,
		          status, priority, created_at, claimed_at, completed_at,
		          error_message, claimed_by
	`, now, now, workerID).Scan(
		&job.ID, &job.CalendarID, &job.JobType, &job.TargetMinDate, &job.TargetMaxDate,
		&job.Status, &job.Priority, &job.CreatedAt, &job.ClaimedAt, &job.CompletedAt,
		&job.ErrorMessage, &job.ClaimedBy,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No pending jobs
		}
		return nil, err
	}

	return job, nil
}

// ClaimNextJobForCalendar atomically claims the next pending job for a specific calendar.
// This is useful when we want to coalesce jobs for a specific calendar.
func (s *SyncJobStore) ClaimNextJobForCalendar(ctx context.Context, calendarID uuid.UUID, workerID string) (*SyncJob, error) {
	now := time.Now().UTC()

	job := &SyncJob{}
	err := s.pool.QueryRow(ctx, `
		UPDATE calendar_sync_jobs
		SET status = 'running', claimed_at = $3, claimed_by = $4
		WHERE id = (
			SELECT id FROM calendar_sync_jobs
			WHERE calendar_id = $1 AND status = 'pending'
			ORDER BY priority DESC, created_at ASC
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, calendar_id, job_type, target_min_date, target_max_date,
		          status, priority, created_at, claimed_at, completed_at,
		          error_message, claimed_by
	`, calendarID, now, now, workerID).Scan(
		&job.ID, &job.CalendarID, &job.JobType, &job.TargetMinDate, &job.TargetMaxDate,
		&job.Status, &job.Priority, &job.CreatedAt, &job.ClaimedAt, &job.CompletedAt,
		&job.ErrorMessage, &job.ClaimedBy,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No pending jobs for this calendar
		}
		return nil, err
	}

	return job, nil
}

// MarkCompleted marks a job as successfully completed
func (s *SyncJobStore) MarkCompleted(ctx context.Context, jobID uuid.UUID) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		UPDATE calendar_sync_jobs
		SET status = 'completed', completed_at = $2
		WHERE id = $1
	`, jobID, now)
	return err
}

// MarkFailed marks a job as failed with an error message
func (s *SyncJobStore) MarkFailed(ctx context.Context, jobID uuid.UUID, errorMessage string) error {
	now := time.Now().UTC()
	_, err := s.pool.Exec(ctx, `
		UPDATE calendar_sync_jobs
		SET status = 'failed', completed_at = $2, error_message = $3
		WHERE id = $1
	`, jobID, now, errorMessage)
	return err
}

// CoalescePendingJobs finds all pending jobs for a calendar and returns a coalesced date range.
// This doesn't modify the jobs - caller should delete them after processing.
func (s *SyncJobStore) CoalescePendingJobs(ctx context.Context, calendarID uuid.UUID) (minDate, maxDate time.Time, jobIDs []uuid.UUID, err error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, target_min_date, target_max_date
		FROM calendar_sync_jobs
		WHERE calendar_id = $1 AND status = 'pending'
		FOR UPDATE
	`, calendarID)
	if err != nil {
		return time.Time{}, time.Time{}, nil, err
	}
	defer rows.Close()

	var initialized bool
	for rows.Next() {
		var id uuid.UUID
		var jobMin, jobMax time.Time
		if err := rows.Scan(&id, &jobMin, &jobMax); err != nil {
			return time.Time{}, time.Time{}, nil, err
		}

		jobIDs = append(jobIDs, id)

		if !initialized {
			minDate = jobMin
			maxDate = jobMax
			initialized = true
		} else {
			if jobMin.Before(minDate) {
				minDate = jobMin
			}
			if jobMax.After(maxDate) {
				maxDate = jobMax
			}
		}
	}

	if err := rows.Err(); err != nil {
		return time.Time{}, time.Time{}, nil, err
	}

	return minDate, maxDate, jobIDs, nil
}

// DeleteJobs deletes multiple jobs by ID
func (s *SyncJobStore) DeleteJobs(ctx context.Context, jobIDs []uuid.UUID) error {
	if len(jobIDs) == 0 {
		return nil
	}

	_, err := s.pool.Exec(ctx, `
		DELETE FROM calendar_sync_jobs
		WHERE id = ANY($1)
	`, jobIDs)
	return err
}

// DeleteOldCompletedJobs removes completed/failed jobs older than the given duration
func (s *SyncJobStore) DeleteOldCompletedJobs(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)

	result, err := s.pool.Exec(ctx, `
		DELETE FROM calendar_sync_jobs
		WHERE status IN ('completed', 'failed')
		  AND completed_at < $1
	`, cutoff)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected(), nil
}

// CountPendingByCalendar returns the count of pending jobs for a calendar
func (s *SyncJobStore) CountPendingByCalendar(ctx context.Context, calendarID uuid.UUID) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM calendar_sync_jobs
		WHERE calendar_id = $1 AND status = 'pending'
	`, calendarID).Scan(&count)
	return count, err
}

// Helper function to scan sync jobs from rows
func scanSyncJobs(rows pgx.Rows) ([]*SyncJob, error) {
	var jobs []*SyncJob
	for rows.Next() {
		job := &SyncJob{}
		err := rows.Scan(
			&job.ID, &job.CalendarID, &job.JobType, &job.TargetMinDate, &job.TargetMaxDate,
			&job.Status, &job.Priority, &job.CreatedAt, &job.ClaimedAt, &job.CompletedAt,
			&job.ErrorMessage, &job.ClaimedBy,
		)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}
