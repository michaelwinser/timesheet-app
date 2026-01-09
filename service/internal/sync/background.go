// Package sync provides calendar synchronization utilities and scheduling.
package sync

import (
	"context"
	"log"
	"time"
)

// BackgroundSyncConfig configures the background sync scheduler
type BackgroundSyncConfig struct {
	// Interval between sync runs (default: 24h)
	Interval time.Duration
	// Enabled controls whether background sync is active
	Enabled bool
}

// DefaultBackgroundSyncConfig returns the default configuration
func DefaultBackgroundSyncConfig() BackgroundSyncConfig {
	return BackgroundSyncConfig{
		Interval: 24 * time.Hour,
		Enabled:  true,
	}
}

// BackgroundSyncRunner is the interface for the sync callback
type BackgroundSyncRunner interface {
	RunBackgroundSync(ctx context.Context) error
}

// BackgroundScheduler handles periodic background synchronization
type BackgroundScheduler struct {
	config BackgroundSyncConfig
	runner BackgroundSyncRunner
	stopCh chan struct{}
	doneCh chan struct{}
}

// NewBackgroundScheduler creates a new background sync scheduler
func NewBackgroundScheduler(config BackgroundSyncConfig, runner BackgroundSyncRunner) *BackgroundScheduler {
	return &BackgroundScheduler{
		config: config,
		runner: runner,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Start begins the background sync loop
func (s *BackgroundScheduler) Start(ctx context.Context) {
	if !s.config.Enabled {
		log.Println("Background sync is disabled")
		close(s.doneCh)
		return
	}

	log.Printf("Starting background sync scheduler (interval: %v)", s.config.Interval)

	go func() {
		defer close(s.doneCh)

		// Initial delay to let the server start up
		select {
		case <-time.After(30 * time.Second):
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}

		// Run sync immediately, then on ticker
		s.runSync(ctx)

		ticker := time.NewTicker(s.config.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				s.runSync(ctx)
			case <-s.stopCh:
				log.Println("Background sync scheduler stopped")
				return
			case <-ctx.Done():
				log.Println("Background sync scheduler context cancelled")
				return
			}
		}
	}()
}

// Stop gracefully stops the background sync scheduler
func (s *BackgroundScheduler) Stop() {
	close(s.stopCh)
	<-s.doneCh
}

// runSync performs a single sync run
func (s *BackgroundScheduler) runSync(ctx context.Context) {
	log.Println("Background sync: starting run")

	if err := s.runner.RunBackgroundSync(ctx); err != nil {
		log.Printf("Background sync: run failed: %v", err)
		return
	}

	log.Println("Background sync: run complete")
}
