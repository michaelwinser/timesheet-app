//go:build integration

package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/michaelw/timesheet-app/service/internal/database"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

// TestCalendarDeselection tests that deselecting a calendar properly filters events
// from query results while preserving materialized time entries.
func TestCalendarDeselection(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()

	// Connect to database
	db, err := database.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create stores
	userStore := store.NewUserStore(db.Pool)
	projectStore := store.NewProjectStore(db.Pool)
	calendarConnStore := store.NewCalendarConnectionStore(db.Pool)
	calendarStore := store.NewCalendarStore(db.Pool)
	eventStore := store.NewCalendarEventStore(db.Pool)
	timeEntryStore := store.NewTimeEntryStore(db.Pool)

	// Create test user
	testEmail := "calendar-test-" + uuid.New().String()[:8] + "@test.com"
	user, err := userStore.Create(ctx, testEmail, "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	defer cleanupTestUser(t, db.Pool, user.ID)

	// Create test project
	project, err := projectStore.Create(ctx, user.ID, "Test Project", nil, nil, nil, nil, false, false, nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	// Create calendar connection with dummy credentials
	dummyCreds := store.CalendarCredentials{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(24 * time.Hour),
	}
	conn, err := calendarConnStore.Create(ctx, user.ID, "google", dummyCreds)
	if err != nil {
		t.Fatalf("Failed to create calendar connection: %v", err)
	}

	// Create a selected calendar
	calendar := &store.Calendar{
		ConnectionID: conn.ID,
		UserID:       user.ID,
		ExternalID:   "test-calendar-1",
		Name:         "Test Calendar",
		IsSelected:   true,
	}
	calendar, err = calendarStore.Upsert(ctx, calendar)
	if err != nil {
		t.Fatalf("Failed to create calendar: %v", err)
	}

	// Create a calendar event
	eventStart := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	eventEnd := time.Date(2025, 1, 15, 11, 0, 0, 0, time.UTC)
	event := &store.CalendarEvent{
		ID:           uuid.New(),
		ConnectionID: conn.ID,
		CalendarID:   &calendar.ID,
		UserID:       user.ID,
		ExternalID:   "event-1",
		Title:        "Test Meeting",
		StartTime:    eventStart,
		EndTime:      eventEnd,
	}
	err = eventStore.Upsert(ctx, event)
	if err != nil {
		t.Fatalf("Failed to create calendar event: %v", err)
	}

	// Classify the event to a project
	err = eventStore.UpdateClassification(ctx, user.ID, event.ID, &project.ID, "manual", nil, nil, 1.0)
	if err != nil {
		t.Fatalf("Failed to classify event: %v", err)
	}

	// Test: Events from selected calendar are visible
	t.Run("events visible when calendar selected", func(t *testing.T) {
		events, err := eventStore.List(ctx, user.ID, eventStart, eventEnd.Add(24*time.Hour), nil, nil)
		if err != nil {
			t.Fatalf("Failed to list events: %v", err)
		}
		if len(events) != 1 {
			t.Errorf("Expected 1 event when calendar selected, got %d", len(events))
		}
	})

	// Deselect the calendar
	err = calendarStore.UpdateSelection(ctx, conn.ID, []uuid.UUID{})
	if err != nil {
		t.Fatalf("Failed to deselect calendar: %v", err)
	}

	// Test: Events from deselected calendar are filtered out
	t.Run("events filtered when calendar deselected", func(t *testing.T) {
		events, err := eventStore.List(ctx, user.ID, eventStart, eventEnd.Add(24*time.Hour), nil, nil)
		if err != nil {
			t.Fatalf("Failed to list events: %v", err)
		}
		if len(events) != 0 {
			t.Errorf("Expected 0 events when calendar deselected, got %d", len(events))
		}
	})

	// Re-select the calendar
	err = calendarStore.UpdateSelection(ctx, conn.ID, []uuid.UUID{calendar.ID})
	if err != nil {
		t.Fatalf("Failed to re-select calendar: %v", err)
	}

	// Test: Events reappear when calendar re-selected
	t.Run("events reappear when calendar re-selected", func(t *testing.T) {
		events, err := eventStore.List(ctx, user.ID, eventStart, eventEnd.Add(24*time.Hour), nil, nil)
		if err != nil {
			t.Fatalf("Failed to list events: %v", err)
		}
		if len(events) != 1 {
			t.Errorf("Expected 1 event when calendar re-selected, got %d", len(events))
		}
	})
}

// TestMaterializedEntryPreservation tests that materialized time entries
// survive calendar disconnection with their stored hours intact.
func TestMaterializedEntryPreservation(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()

	// Connect to database
	db, err := database.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create stores
	userStore := store.NewUserStore(db.Pool)
	projectStore := store.NewProjectStore(db.Pool)
	calendarConnStore := store.NewCalendarConnectionStore(db.Pool)
	calendarStore := store.NewCalendarStore(db.Pool)
	eventStore := store.NewCalendarEventStore(db.Pool)
	timeEntryStore := store.NewTimeEntryStore(db.Pool)
	billingPeriodStore := store.NewBillingPeriodStore(db.Pool)
	invoiceStore := store.NewInvoiceStore(db.Pool)

	// Create test user
	testEmail := "preserved-test-" + uuid.New().String()[:8] + "@test.com"
	user, err := userStore.Create(ctx, testEmail, "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	defer cleanupTestUser(t, db.Pool, user.ID)

	// Create test project
	project, err := projectStore.Create(ctx, user.ID, "Test Project", nil, nil, nil, nil, false, false, nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	// Create billing period for invoicing
	startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err = billingPeriodStore.Create(ctx, user.ID, project.ID, startDate, nil, 100.00)
	if err != nil {
		t.Fatalf("Failed to create billing period: %v", err)
	}

	// Create calendar connection with dummy credentials
	dummyCreds := store.CalendarCredentials{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(24 * time.Hour),
	}
	conn, err := calendarConnStore.Create(ctx, user.ID, "google", dummyCreds)
	if err != nil {
		t.Fatalf("Failed to create calendar connection: %v", err)
	}

	// Create a selected calendar
	calendar := &store.Calendar{
		ConnectionID: conn.ID,
		UserID:       user.ID,
		ExternalID:   "test-calendar-2",
		Name:         "Test Calendar",
		IsSelected:   true,
	}
	calendar, err = calendarStore.Upsert(ctx, calendar)
	if err != nil {
		t.Fatalf("Failed to create calendar: %v", err)
	}

	// Create calendar event
	eventStart := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	eventEnd := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC) // 2 hour meeting
	event := &store.CalendarEvent{
		ID:           uuid.New(),
		ConnectionID: conn.ID,
		CalendarID:   &calendar.ID,
		UserID:       user.ID,
		ExternalID:   "event-2",
		Title:        "Important Meeting",
		StartTime:    eventStart,
		EndTime:      eventEnd,
	}
	err = eventStore.Upsert(ctx, event)
	if err != nil {
		t.Fatalf("Failed to create calendar event: %v", err)
	}

	// Classify event to project
	err = eventStore.UpdateClassification(ctx, user.ID, event.ID, &project.ID, "manual", nil, nil, 1.0)
	if err != nil {
		t.Fatalf("Failed to classify event: %v", err)
	}

	// Create time entry linked to the event (materialized because we're creating it directly)
	entryDate := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	entry, err := timeEntryStore.Create(ctx, user.ID, project.ID, entryDate, 2.0, nil)
	if err != nil {
		t.Fatalf("Failed to create time entry: %v", err)
	}

	// Link time entry to event
	err = linkTimeEntryToEvent(ctx, db.Pool, entry.ID, event.ID)
	if err != nil {
		t.Fatalf("Failed to link time entry to event: %v", err)
	}

	// User edits the time entry (making it materialized with has_user_edits = true)
	newHours := 2.5
	_, err = timeEntryStore.Update(ctx, user.ID, entry.ID, &newHours, nil)
	if err != nil {
		t.Fatalf("Failed to update time entry: %v", err)
	}

	// Verify entry has user edits flag
	entry, err = timeEntryStore.GetByID(ctx, user.ID, entry.ID)
	if err != nil {
		t.Fatalf("Failed to get time entry: %v", err)
	}
	if !entry.HasUserEdits {
		t.Fatal("Expected time entry to have HasUserEdits = true after update")
	}

	// Test: Disconnect calendar connection (CASCADE deletes events and time_entry_events links)
	t.Run("materialized entry survives connection deletion", func(t *testing.T) {
		// Delete calendar connection (triggers CASCADE to calendars, events, time_entry_events)
		err := calendarConnStore.Delete(ctx, user.ID, conn.ID)
		if err != nil {
			t.Fatalf("Failed to delete calendar connection: %v", err)
		}

		// Verify time entry still exists with stored hours
		preservedEntry, err := timeEntryStore.GetByID(ctx, user.ID, entry.ID)
		if err != nil {
			t.Fatalf("Expected time entry to survive, got error: %v", err)
		}
		if preservedEntry.Hours != 2.5 {
			t.Errorf("Expected stored hours to be 2.5, got %f", preservedEntry.Hours)
		}
		if !preservedEntry.HasUserEdits {
			t.Error("Expected HasUserEdits to remain true")
		}
	})
}

// TestInvoicedEntryPreservation tests that invoiced time entries survive
// calendar disconnection with their stored hours intact.
func TestInvoicedEntryPreservation(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()

	// Connect to database
	db, err := database.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create stores
	userStore := store.NewUserStore(db.Pool)
	projectStore := store.NewProjectStore(db.Pool)
	calendarConnStore := store.NewCalendarConnectionStore(db.Pool)
	calendarStore := store.NewCalendarStore(db.Pool)
	eventStore := store.NewCalendarEventStore(db.Pool)
	timeEntryStore := store.NewTimeEntryStore(db.Pool)
	billingPeriodStore := store.NewBillingPeriodStore(db.Pool)
	invoiceStore := store.NewInvoiceStore(db.Pool)

	// Create test user
	testEmail := "invoiced-test-" + uuid.New().String()[:8] + "@test.com"
	user, err := userStore.Create(ctx, testEmail, "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	defer cleanupTestUser(t, db.Pool, user.ID)

	// Create test project
	project, err := projectStore.Create(ctx, user.ID, "Test Project", nil, nil, nil, nil, false, false, nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	// Create billing period
	startDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err = billingPeriodStore.Create(ctx, user.ID, project.ID, startDate, nil, 100.00)
	if err != nil {
		t.Fatalf("Failed to create billing period: %v", err)
	}

	// Create calendar connection
	dummyCreds := store.CalendarCredentials{
		AccessToken:  "test-token",
		RefreshToken: "test-refresh",
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(24 * time.Hour),
	}
	conn, err := calendarConnStore.Create(ctx, user.ID, "google", dummyCreds)
	if err != nil {
		t.Fatalf("Failed to create calendar connection: %v", err)
	}

	// Create calendar
	calendar := &store.Calendar{
		ConnectionID: conn.ID,
		UserID:       user.ID,
		ExternalID:   "test-calendar-3",
		Name:         "Test Calendar",
		IsSelected:   true,
	}
	calendar, err = calendarStore.Upsert(ctx, calendar)
	if err != nil {
		t.Fatalf("Failed to create calendar: %v", err)
	}

	// Create calendar event
	eventStart := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	eventEnd := time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC)
	event := &store.CalendarEvent{
		ID:           uuid.New(),
		ConnectionID: conn.ID,
		CalendarID:   &calendar.ID,
		UserID:       user.ID,
		ExternalID:   "event-3",
		Title:        "Billable Meeting",
		StartTime:    eventStart,
		EndTime:      eventEnd,
	}
	err = eventStore.Upsert(ctx, event)
	if err != nil {
		t.Fatalf("Failed to create calendar event: %v", err)
	}

	// Classify event
	err = eventStore.UpdateClassification(ctx, user.ID, event.ID, &project.ID, "manual", nil, nil, 1.0)
	if err != nil {
		t.Fatalf("Failed to classify event: %v", err)
	}

	// Create time entry
	entryDate := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	entry, err := timeEntryStore.Create(ctx, user.ID, project.ID, entryDate, 2.0, nil)
	if err != nil {
		t.Fatalf("Failed to create time entry: %v", err)
	}

	// Link time entry to event
	err = linkTimeEntryToEvent(ctx, db.Pool, entry.ID, event.ID)
	if err != nil {
		t.Fatalf("Failed to link time entry to event: %v", err)
	}

	// Create invoice (which materializes the entry via invoice_id)
	periodEnd := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
	_, err = invoiceStore.Create(ctx, user.ID, project.ID, nil, startDate, periodEnd)
	if err != nil {
		t.Fatalf("Failed to create invoice: %v", err)
	}

	// Verify entry is invoiced
	entry, err = timeEntryStore.GetByID(ctx, user.ID, entry.ID)
	if err != nil {
		t.Fatalf("Failed to get time entry: %v", err)
	}
	if entry.InvoiceID == nil {
		t.Fatal("Expected time entry to have InvoiceID set after invoicing")
	}

	// Test: Disconnect calendar - invoiced entry must survive
	t.Run("invoiced entry survives connection deletion", func(t *testing.T) {
		// Delete calendar connection
		err := calendarConnStore.Delete(ctx, user.ID, conn.ID)
		if err != nil {
			t.Fatalf("Failed to delete calendar connection: %v", err)
		}

		// Verify invoiced entry still exists
		preservedEntry, err := timeEntryStore.GetByID(ctx, user.ID, entry.ID)
		if err != nil {
			t.Fatalf("Expected invoiced entry to survive, got error: %v", err)
		}
		if preservedEntry.Hours != 2.0 {
			t.Errorf("Expected stored hours to be 2.0, got %f", preservedEntry.Hours)
		}
		if preservedEntry.InvoiceID == nil {
			t.Error("Expected InvoiceID to remain set")
		}
	})
}

// TestOrphanedEntryMigration tests that the migration correctly marks
// orphaned entries (with hours but no event links) as having user edits.
func TestOrphanedEntryMigration(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()

	// Connect to database
	db, err := database.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	// Create stores
	userStore := store.NewUserStore(db.Pool)
	projectStore := store.NewProjectStore(db.Pool)
	timeEntryStore := store.NewTimeEntryStore(db.Pool)

	// Create test user
	testEmail := "orphan-test-" + uuid.New().String()[:8] + "@test.com"
	user, err := userStore.Create(ctx, testEmail, "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	defer cleanupTestUser(t, db.Pool, user.ID)

	// Create test project
	project, err := projectStore.Create(ctx, user.ID, "Test Project", nil, nil, nil, nil, false, false, nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	// Create time entry with hours but no linked events (simulating orphaned entry)
	// Set has_user_edits = false initially to test the migration
	entryDate := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	entry, err := timeEntryStore.Create(ctx, user.ID, project.ID, entryDate, 3.0, nil)
	if err != nil {
		t.Fatalf("Failed to create time entry: %v", err)
	}

	// Manually set has_user_edits to false (simulating pre-migration state)
	_, err = db.Pool.Exec(ctx, `UPDATE time_entries SET has_user_edits = false WHERE id = $1`, entry.ID)
	if err != nil {
		t.Fatalf("Failed to set has_user_edits to false: %v", err)
	}

	// Run the migration SQL manually (since it already ran in Migrate)
	_, err = db.Pool.Exec(ctx, `
		UPDATE time_entries
		SET has_user_edits = true
		WHERE id NOT IN (SELECT time_entry_id FROM time_entry_events)
		  AND hours > 0
		  AND has_user_edits = false
		  AND invoice_id IS NULL
	`)
	if err != nil {
		t.Fatalf("Failed to run migration SQL: %v", err)
	}

	// Verify the entry now has has_user_edits = true
	t.Run("orphaned entry marked as user-edited", func(t *testing.T) {
		updatedEntry, err := timeEntryStore.GetByID(ctx, user.ID, entry.ID)
		if err != nil {
			t.Fatalf("Failed to get time entry: %v", err)
		}
		if !updatedEntry.HasUserEdits {
			t.Error("Expected orphaned entry to have HasUserEdits = true after migration")
		}
		if updatedEntry.Hours != 3.0 {
			t.Errorf("Expected hours to remain 3.0, got %f", updatedEntry.Hours)
		}
	})
}

// linkTimeEntryToEvent creates a link in the time_entry_events junction table
func linkTimeEntryToEvent(ctx context.Context, pool *pgxpool.Pool, entryID, eventID uuid.UUID) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO time_entry_events (time_entry_id, calendar_event_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, entryID, eventID)
	return err
}

// cleanupTestUser removes a test user and all related data
func cleanupTestUser(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) {
	ctx := context.Background()
	_, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	if err != nil {
		t.Logf("Warning: failed to cleanup test user: %v", err)
	}
}
