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

// TestInvoiceTimeEntryProtection tests the invoice protection lifecycle:
// 1. Creating an invoice sets invoice_id on time entries immediately
// 2. Invoiced entries cannot be updated
// 3. Invoiced entries cannot be deleted
// 4. Deleting an invoice clears invoice_id and entries become editable
func TestInvoiceTimeEntryProtection(t *testing.T) {
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
	billingPeriodStore := store.NewBillingPeriodStore(db.Pool)
	invoiceStore := store.NewInvoiceStore(db.Pool)

	// Create test user
	testEmail := "invoice-test-" + uuid.New().String()[:8] + "@test.com"
	user, err := userStore.Create(ctx, testEmail, "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	defer cleanupUser(t, db.Pool, user.ID)

	// Create test project
	project, err := projectStore.Create(ctx, user.ID, "Test Project", nil, nil, nil, nil, false, false, nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	// Create billing period
	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	billingPeriod, err := billingPeriodStore.Create(ctx, user.ID, project.ID, startDate, nil, 100.00)
	if err != nil {
		t.Fatalf("Failed to create billing period: %v", err)
	}

	// Create time entries
	entryDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	entry1, err := timeEntryStore.Create(ctx, user.ID, project.ID, entryDate, 8.0, nil)
	if err != nil {
		t.Fatalf("Failed to create time entry 1: %v", err)
	}

	entry2, err := timeEntryStore.Create(ctx, user.ID, project.ID, entryDate.AddDate(0, 0, 1), 4.0, nil)
	if err != nil {
		t.Fatalf("Failed to create time entry 2: %v", err)
	}

	// Verify entries are editable before invoicing
	t.Run("entries editable before invoicing", func(t *testing.T) {
		newHours := 7.5
		_, err := timeEntryStore.Update(ctx, user.ID, entry1.ID, &newHours, nil)
		if err != nil {
			t.Errorf("Expected entry to be editable before invoicing, got error: %v", err)
		}
		// Reset hours
		originalHours := 8.0
		_, _ = timeEntryStore.Update(ctx, user.ID, entry1.ID, &originalHours, nil)
	})

	// Create invoice with the time entries
	periodEnd := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
	invoice, err := invoiceStore.Create(ctx, user.ID, project.ID, &billingPeriod.ID, startDate, periodEnd)
	if err != nil {
		t.Fatalf("Failed to create invoice: %v", err)
	}

	// Test 1: Verify invoice_id is set immediately on creation
	t.Run("invoice creation sets invoice_id", func(t *testing.T) {
		// Reload entries from database
		updatedEntry1, err := timeEntryStore.GetByID(ctx, user.ID, entry1.ID)
		if err != nil {
			t.Fatalf("Failed to get entry 1: %v", err)
		}
		if updatedEntry1.InvoiceID == nil {
			t.Errorf("Expected entry 1 to have invoice_id set, got nil")
		}
		if updatedEntry1.InvoiceID != nil && *updatedEntry1.InvoiceID != invoice.ID {
			t.Errorf("Expected entry 1 invoice_id to be %s, got %s", invoice.ID, *updatedEntry1.InvoiceID)
		}

		updatedEntry2, err := timeEntryStore.GetByID(ctx, user.ID, entry2.ID)
		if err != nil {
			t.Fatalf("Failed to get entry 2: %v", err)
		}
		if updatedEntry2.InvoiceID == nil {
			t.Errorf("Expected entry 2 to have invoice_id set, got nil")
		}
	})

	// Test 2: Verify invoiced entries cannot be updated
	t.Run("invoiced entries cannot be updated", func(t *testing.T) {
		newHours := 5.0
		_, err := timeEntryStore.Update(ctx, user.ID, entry1.ID, &newHours, nil)
		if err == nil {
			t.Errorf("Expected error when updating invoiced entry, got nil")
		}
		if err != nil && err != store.ErrTimeEntryInvoiced {
			t.Errorf("Expected ErrTimeEntryInvoiced, got: %v", err)
		}
	})

	// Test 3: Verify invoiced entries cannot be deleted
	t.Run("invoiced entries cannot be deleted", func(t *testing.T) {
		err := timeEntryStore.Delete(ctx, user.ID, entry1.ID)
		if err == nil {
			t.Errorf("Expected error when deleting invoiced entry, got nil")
		}
		if err != nil && err != store.ErrTimeEntryInvoiced {
			t.Errorf("Expected ErrTimeEntryInvoiced, got: %v", err)
		}
	})

	// Test 4: Delete invoice and verify entries become editable
	t.Run("delete invoice clears invoice_id", func(t *testing.T) {
		err := invoiceStore.Delete(ctx, user.ID, invoice.ID)
		if err != nil {
			t.Fatalf("Failed to delete invoice: %v", err)
		}

		// Verify invoice_id is cleared
		updatedEntry1, err := timeEntryStore.GetByID(ctx, user.ID, entry1.ID)
		if err != nil {
			t.Fatalf("Failed to get entry 1 after invoice deletion: %v", err)
		}
		if updatedEntry1.InvoiceID != nil {
			t.Errorf("Expected entry 1 invoice_id to be nil after invoice deletion, got %s", *updatedEntry1.InvoiceID)
		}

		// Verify entry is now editable
		newHours := 6.0
		_, err = timeEntryStore.Update(ctx, user.ID, entry1.ID, &newHours, nil)
		if err != nil {
			t.Errorf("Expected entry to be editable after invoice deletion, got error: %v", err)
		}
	})
}

// cleanupUser removes a test user and all related data
func cleanupUser(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) {
	ctx := context.Background()
	_, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	if err != nil {
		t.Logf("Warning: failed to cleanup test user: %v", err)
	}
}
