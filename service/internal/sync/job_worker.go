// Package sync provides calendar synchronization utilities and scheduling.
package sync

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/michaelw/timesheet-app/service/internal/google"
	"github.com/michaelw/timesheet-app/service/internal/store"
	gcal "google.golang.org/api/calendar/v3"
)

// JobWorkerConfig configures the job worker
type JobWorkerConfig struct {
	// PollInterval is how often to poll for new jobs
	PollInterval time.Duration
	// WorkerID identifies this worker instance
	WorkerID string
	// Enabled controls whether the job worker is active
	Enabled bool
	// MaxJobsPerRun limits how many jobs to process in each poll cycle
	MaxJobsPerRun int
}

// DefaultJobWorkerConfig returns the default configuration
func DefaultJobWorkerConfig() JobWorkerConfig {
	return JobWorkerConfig{
		PollInterval:  5 * time.Second,
		WorkerID:      "worker-" + uuid.New().String()[:8],
		Enabled:       true,
		MaxJobsPerRun: 10,
	}
}

// JobWorker processes background sync jobs from the queue
type JobWorker struct {
	config     JobWorkerConfig
	pool       *pgxpool.Pool
	jobStore   *store.SyncJobStore
	calStore   *store.CalendarStore
	connStore  *store.CalendarConnectionStore
	eventStore *store.CalendarEventStore
	googleSvc  google.CalendarClient
	stopCh     chan struct{}
	doneCh     chan struct{}
}

// NewJobWorker creates a new job worker
func NewJobWorker(
	config JobWorkerConfig,
	pool *pgxpool.Pool,
	jobStore *store.SyncJobStore,
	calStore *store.CalendarStore,
	connStore *store.CalendarConnectionStore,
	eventStore *store.CalendarEventStore,
	googleSvc google.CalendarClient,
) *JobWorker {
	return &JobWorker{
		config:     config,
		pool:       pool,
		jobStore:   jobStore,
		calStore:   calStore,
		connStore:  connStore,
		eventStore: eventStore,
		googleSvc:  googleSvc,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
}

// Start begins the job worker loop
func (w *JobWorker) Start(ctx context.Context) {
	if !w.config.Enabled {
		log.Println("Job worker is disabled")
		close(w.doneCh)
		return
	}

	log.Printf("Starting job worker (poll interval: %v, worker ID: %s)", w.config.PollInterval, w.config.WorkerID)

	go func() {
		defer close(w.doneCh)

		// Process jobs immediately on start
		w.processJobs(ctx)

		ticker := time.NewTicker(w.config.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				w.processJobs(ctx)
			case <-w.stopCh:
				log.Println("Job worker stopped")
				return
			case <-ctx.Done():
				log.Println("Job worker context cancelled")
				return
			}
		}
	}()
}

// Stop gracefully stops the job worker
func (w *JobWorker) Stop() {
	close(w.stopCh)
	<-w.doneCh
}

// processJobs processes available jobs
func (w *JobWorker) processJobs(ctx context.Context) {
	for i := 0; i < w.config.MaxJobsPerRun; i++ {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		default:
		}

		// Claim the next job
		job, err := w.jobStore.ClaimNextJob(ctx, w.config.WorkerID)
		if err != nil {
			log.Printf("Job worker: error claiming job: %v", err)
			return
		}

		if job == nil {
			// No more jobs to process
			return
		}

		log.Printf("Job worker: processing job %s (calendar: %s, type: %s, range: %s to %s)",
			job.ID, job.CalendarID, job.JobType,
			job.TargetMinDate.Format("2006-01-02"), job.TargetMaxDate.Format("2006-01-02"))

		// Process the job
		if err := w.processJob(ctx, job); err != nil {
			log.Printf("Job worker: job %s failed: %v", job.ID, err)
			if markErr := w.jobStore.MarkFailed(ctx, job.ID, err.Error()); markErr != nil {
				log.Printf("Job worker: failed to mark job as failed: %v", markErr)
			}
			continue
		}

		// Mark job as completed
		if err := w.jobStore.MarkCompleted(ctx, job.ID); err != nil {
			log.Printf("Job worker: failed to mark job as completed: %v", err)
		}

		log.Printf("Job worker: job %s completed successfully", job.ID)
	}
}

// processJob processes a single sync job
func (w *JobWorker) processJob(ctx context.Context, job *store.SyncJob) error {
	// Get calendar details
	cal, err := w.calStore.GetByID(ctx, job.CalendarID)
	if err != nil {
		return err
	}

	// Get connection for OAuth credentials
	conn, err := w.connStore.GetByIDForSync(ctx, cal.ConnectionID)
	if err != nil {
		return err
	}

	// Check if calendar needs re-auth
	if cal.NeedsReauth {
		return errNeedsReauth
	}

	// Check if we've exceeded failure threshold
	if cal.SyncFailureCount >= 3 {
		return errTooManyFailures
	}

	// Refresh token if needed
	creds := &conn.Credentials
	if time.Now().After(creds.Expiry.Add(-5 * time.Minute)) {
		newCreds, err := w.googleSvc.RefreshToken(ctx, creds)
		if err != nil {
			w.calStore.MarkNeedsReauth(ctx, cal.ID)
			return err
		}
		creds = newCreds
		w.connStore.UpdateCredentials(ctx, conn.ID, *creds)
	}

	// Fetch events from Google
	result, err := w.googleSvc.FetchEvents(ctx, creds, cal.ExternalID, job.TargetMinDate, job.TargetMaxDate)
	if err != nil {
		// Track failure
		if incrementErr := w.calStore.IncrementSyncFailureCount(ctx, cal.ID); incrementErr != nil {
			log.Printf("Job worker: failed to increment failure count: %v", incrementErr)
		}
		return err
	}

	// Use a transaction to atomically update events and water marks
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Track external IDs for orphaning
	externalIDs := make([]string, 0, len(result.Events))

	// Upsert events
	for _, ge := range result.Events {
		if ge.Status == "cancelled" {
			// Mark as orphaned
			if err := w.eventStore.MarkOrphanedByExternalIDAndCalendar(ctx, cal.ID, ge.Id); err != nil {
				log.Printf("Job worker: failed to mark event as orphaned: %v", err)
			}
			continue
		}

		// Skip working location events - these indicate where someone is working
		// (office, home, etc.) rather than actual meetings or work items
		if ge.EventType == "workingLocation" {
			continue
		}

		externalIDs = append(externalIDs, ge.Id)
		event := googleEventToStore(ge, conn.ID, cal.ID, cal.UserID)
		if _, err := w.eventStore.Upsert(ctx, event); err != nil {
			return err
		}
	}

	// Mark events within the synced range as orphaned if not in the result
	if len(externalIDs) > 0 {
		if _, err := w.eventStore.MarkOrphanedInRangeExceptByCalendar(ctx, cal.ID, externalIDs, job.TargetMinDate, job.TargetMaxDate); err != nil {
			return err
		}
	}

	// Expand water marks to include the synced range
	if err := w.calStore.ExpandSyncedWindow(ctx, job.CalendarID, job.TargetMinDate, job.TargetMaxDate); err != nil {
		return err
	}

	// Update sync token if we got a new one
	if result.NextSyncToken != "" {
		if err := w.calStore.UpdateSyncToken(ctx, job.CalendarID, result.NextSyncToken); err != nil {
			return err
		}
	}

	// Update last synced timestamp
	if err := w.calStore.UpdateLastSynced(ctx, job.CalendarID); err != nil {
		return err
	}

	// Reset failure count on success
	if err := w.calStore.ResetSyncFailureCount(ctx, job.CalendarID); err != nil {
		return err
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

// googleEventToStore converts Google Calendar event to store model
func googleEventToStore(ge *gcal.Event, connID, calID uuid.UUID, userID uuid.UUID) *store.CalendarEvent {
	event := &store.CalendarEvent{
		ConnectionID:         connID,
		CalendarID:           &calID,
		UserID:               userID,
		ExternalID:           ge.Id,
		Title:                ge.Summary,
		ClassificationStatus: store.StatusPending,
	}

	if ge.Description != "" {
		event.Description = &ge.Description
	}

	// Parse times - all-day events use Date, timed events use DateTime
	if ge.Start != nil {
		if ge.Start.DateTime != "" {
			event.StartTime, _ = time.Parse(time.RFC3339, ge.Start.DateTime)
		} else if ge.Start.Date != "" {
			event.StartTime, _ = time.Parse("2006-01-02", ge.Start.Date)
			event.IsAllDay = true
		}
	}

	if ge.End != nil {
		if ge.End.DateTime != "" {
			event.EndTime, _ = time.Parse(time.RFC3339, ge.End.DateTime)
		} else if ge.End.Date != "" {
			event.EndTime, _ = time.Parse("2006-01-02", ge.End.Date)
		}
	}

	// Attendees - extract emails and find user's response status
	attendeeSet := make(map[string]bool)
	for _, a := range ge.Attendees {
		event.Attendees = append(event.Attendees, a.Email)
		attendeeSet[a.Email] = true
		if a.Self && a.ResponseStatus != "" {
			event.ResponseStatus = &a.ResponseStatus
		}
	}

	// Add organizer email to attendees if not already present
	// Google Calendar doesn't always include the organizer in the attendees list
	if ge.Organizer != nil && ge.Organizer.Email != "" && !attendeeSet[ge.Organizer.Email] {
		event.Attendees = append(event.Attendees, ge.Organizer.Email)
	}

	event.IsRecurring = ge.RecurringEventId != ""

	if ge.Transparency != "" {
		event.Transparency = &ge.Transparency
	}

	return event
}

// Error types for job failures
type syncError string

func (e syncError) Error() string { return string(e) }

const (
	errNeedsReauth     syncError = "calendar needs re-authentication"
	errTooManyFailures syncError = "too many consecutive sync failures"
)
