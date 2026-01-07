package timeentry

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/michaelw/timesheet-app/service/internal/store"
)

// mockEventStore implements the event store interface for testing
type mockEventStore struct {
	events []*store.CalendarEvent
}

func (m *mockEventStore) List(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time, status *store.ClassificationStatus, connectionID *uuid.UUID) ([]*store.CalendarEvent, error) {
	var result []*store.CalendarEvent
	for _, e := range m.events {
		if status != nil && e.ClassificationStatus != *status {
			continue
		}
		if startDate != nil && e.StartTime.Before(*startDate) {
			continue
		}
		if endDate != nil && e.StartTime.After(endDate.AddDate(0, 0, 1)) {
			continue
		}
		result = append(result, e)
	}
	return result, nil
}

// mockTimeEntryStore implements the time entry store interface for testing
type mockTimeEntryStore struct {
	entries        []*store.TimeEntry
	upsertedCount  int
	deletedIDs     []uuid.UUID
	updatedCompIDs []uuid.UUID
}

func (m *mockTimeEntryStore) List(ctx context.Context, userID uuid.UUID, startDate, endDate *time.Time, projectID *uuid.UUID) ([]*store.TimeEntry, error) {
	var result []*store.TimeEntry
	for _, e := range m.entries {
		if startDate != nil && e.Date.Before(*startDate) {
			continue
		}
		if endDate != nil && e.Date.After(*endDate) {
			continue
		}
		if projectID != nil && e.ProjectID != *projectID {
			continue
		}
		result = append(result, e)
	}
	return result, nil
}

func (m *mockTimeEntryStore) UpsertFromComputed(ctx context.Context, userID, projectID uuid.UUID, date time.Time, hours float64, title, description string, details []byte, eventIDs []uuid.UUID) (*store.TimeEntry, error) {
	m.upsertedCount++
	// Update existing or add new
	for _, e := range m.entries {
		if e.ProjectID == projectID && e.Date.Equal(date) {
			e.Hours = hours
			e.ComputedHours = &hours
			return e, nil
		}
	}
	entry := &store.TimeEntry{
		ID:            uuid.New(),
		UserID:        userID,
		ProjectID:     projectID,
		Date:          date,
		Hours:         hours,
		ComputedHours: &hours,
	}
	m.entries = append(m.entries, entry)
	return entry, nil
}

func (m *mockTimeEntryStore) UpdateComputed(ctx context.Context, userID uuid.UUID, entryID uuid.UUID, hours float64, title, description string, details []byte, eventIDs []uuid.UUID) error {
	m.updatedCompIDs = append(m.updatedCompIDs, entryID)
	for _, e := range m.entries {
		if e.ID == entryID {
			e.ComputedHours = &hours
			e.IsStale = true
			return nil
		}
	}
	return nil
}

func (m *mockTimeEntryStore) Delete(ctx context.Context, userID, entryID uuid.UUID) error {
	m.deletedIDs = append(m.deletedIDs, entryID)
	// Remove from entries
	for i, e := range m.entries {
		if e.ID == entryID {
			m.entries = append(m.entries[:i], m.entries[i+1:]...)
			return nil
		}
	}
	return nil
}

func TestRecalculateForDate_ReclassifyEvent(t *testing.T) {
	// Test scenario: Event reclassified from Project A to Project B
	// Expected: Project A entry should be deleted, Project B entry should be created

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	projectA := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	projectB := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	eventID := uuid.MustParse("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee")
	entryAID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	// Event is now classified to Project B (was Project A)
	eventStore := &mockEventStore{
		events: []*store.CalendarEvent{
			{
				ID:                   eventID,
				UserID:               userID,
				Title:                "Meeting",
				StartTime:            date.Add(9 * time.Hour),
				EndTime:              date.Add(10 * time.Hour),
				ClassificationStatus: store.StatusClassified,
				ProjectID:            &projectB, // Now classified to B
			},
		},
	}

	// Existing entry for Project A (from previous classification)
	entryStore := &mockTimeEntryStore{
		entries: []*store.TimeEntry{
			{
				ID:        entryAID,
				UserID:    userID,
				ProjectID: projectA,
				Date:      date,
				Hours:     1.0,
				IsPinned:  false,
				IsLocked:  false,
			},
		},
	}

	svc := &Service{
		eventStore:     eventStore,
		timeEntryStore: entryStore,
	}

	err := svc.RecalculateForDate(context.Background(), userID, date)
	if err != nil {
		t.Fatalf("RecalculateForDate() error = %v", err)
	}

	// Verify Project A entry was deleted
	if len(entryStore.deletedIDs) != 1 {
		t.Errorf("Expected 1 deleted entry, got %d", len(entryStore.deletedIDs))
	}
	if len(entryStore.deletedIDs) > 0 && entryStore.deletedIDs[0] != entryAID {
		t.Errorf("Expected entry %s to be deleted, got %s", entryAID, entryStore.deletedIDs[0])
	}

	// Verify Project B entry was created
	if entryStore.upsertedCount != 1 {
		t.Errorf("Expected 1 upserted entry, got %d", entryStore.upsertedCount)
	}

	// Verify final state: only Project B entry exists
	var projectBEntry *store.TimeEntry
	for _, e := range entryStore.entries {
		if e.ProjectID == projectB {
			projectBEntry = e
		}
		if e.ProjectID == projectA {
			t.Errorf("Project A entry should not exist after reclassification")
		}
	}
	if projectBEntry == nil {
		t.Errorf("Project B entry should exist after reclassification")
	}
}

func TestRecalculateForDate_ProtectedEntryNotDeleted(t *testing.T) {
	// Test scenario: Event reclassified, but old entry is pinned
	// Expected: Pinned entry should NOT be deleted, but marked stale with computed=0

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	projectA := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	projectB := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	eventID := uuid.MustParse("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee")
	entryAID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	// Event is now classified to Project B
	eventStore := &mockEventStore{
		events: []*store.CalendarEvent{
			{
				ID:                   eventID,
				UserID:               userID,
				Title:                "Meeting",
				StartTime:            date.Add(9 * time.Hour),
				EndTime:              date.Add(10 * time.Hour),
				ClassificationStatus: store.StatusClassified,
				ProjectID:            &projectB,
			},
		},
	}

	// Existing PINNED entry for Project A
	entryStore := &mockTimeEntryStore{
		entries: []*store.TimeEntry{
			{
				ID:        entryAID,
				UserID:    userID,
				ProjectID: projectA,
				Date:      date,
				Hours:     1.0,
				IsPinned:  true, // Protected!
				IsLocked:  false,
			},
		},
	}

	svc := &Service{
		eventStore:     eventStore,
		timeEntryStore: entryStore,
	}

	err := svc.RecalculateForDate(context.Background(), userID, date)
	if err != nil {
		t.Fatalf("RecalculateForDate() error = %v", err)
	}

	// Verify Project A entry was NOT deleted
	if len(entryStore.deletedIDs) != 0 {
		t.Errorf("Expected 0 deleted entries (pinned entry protected), got %d", len(entryStore.deletedIDs))
	}

	// Verify computed values were updated for the pinned entry
	if len(entryStore.updatedCompIDs) != 1 {
		t.Errorf("Expected 1 entry with updated computed values, got %d", len(entryStore.updatedCompIDs))
	}
	if len(entryStore.updatedCompIDs) > 0 && entryStore.updatedCompIDs[0] != entryAID {
		t.Errorf("Expected entry %s to have computed values updated, got %s", entryAID, entryStore.updatedCompIDs[0])
	}
}

func TestRecalculateForDate_LockedEntryNotDeleted(t *testing.T) {
	// Test scenario: Event reclassified, but old entry is locked
	// Expected: Locked entry should NOT be deleted

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	projectA := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	projectB := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	entryAID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	// Event is classified to Project B
	eventStore := &mockEventStore{
		events: []*store.CalendarEvent{
			{
				ID:                   uuid.New(),
				UserID:               userID,
				Title:                "Meeting",
				StartTime:            date.Add(9 * time.Hour),
				EndTime:              date.Add(10 * time.Hour),
				ClassificationStatus: store.StatusClassified,
				ProjectID:            &projectB,
			},
		},
	}

	// Existing LOCKED entry for Project A
	entryStore := &mockTimeEntryStore{
		entries: []*store.TimeEntry{
			{
				ID:        entryAID,
				UserID:    userID,
				ProjectID: projectA,
				Date:      date,
				Hours:     1.0,
				IsPinned:  false,
				IsLocked:  true, // Protected!
			},
		},
	}

	svc := &Service{
		eventStore:     eventStore,
		timeEntryStore: entryStore,
	}

	err := svc.RecalculateForDate(context.Background(), userID, date)
	if err != nil {
		t.Fatalf("RecalculateForDate() error = %v", err)
	}

	// Verify Project A entry was NOT deleted
	if len(entryStore.deletedIDs) != 0 {
		t.Errorf("Expected 0 deleted entries (locked entry protected), got %d", len(entryStore.deletedIDs))
	}
}

func TestRecalculateForDate_InvoicedEntryNotDeleted(t *testing.T) {
	// Test scenario: Event reclassified, but old entry is invoiced
	// Expected: Invoiced entry should NOT be deleted

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	projectA := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	projectB := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	invoiceID := uuid.MustParse("99999999-9999-4999-a999-999999999999")
	entryAID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

	// Event is classified to Project B
	eventStore := &mockEventStore{
		events: []*store.CalendarEvent{
			{
				ID:                   uuid.New(),
				UserID:               userID,
				Title:                "Meeting",
				StartTime:            date.Add(9 * time.Hour),
				EndTime:              date.Add(10 * time.Hour),
				ClassificationStatus: store.StatusClassified,
				ProjectID:            &projectB,
			},
		},
	}

	// Existing INVOICED entry for Project A
	entryStore := &mockTimeEntryStore{
		entries: []*store.TimeEntry{
			{
				ID:        entryAID,
				UserID:    userID,
				ProjectID: projectA,
				Date:      date,
				Hours:     1.0,
				IsPinned:  false,
				IsLocked:  false,
				InvoiceID: &invoiceID, // Protected!
			},
		},
	}

	svc := &Service{
		eventStore:     eventStore,
		timeEntryStore: entryStore,
	}

	err := svc.RecalculateForDate(context.Background(), userID, date)
	if err != nil {
		t.Fatalf("RecalculateForDate() error = %v", err)
	}

	// Verify Project A entry was NOT deleted
	if len(entryStore.deletedIDs) != 0 {
		t.Errorf("Expected 0 deleted entries (invoiced entry protected), got %d", len(entryStore.deletedIDs))
	}
}

func TestRecalculateForDate_MultipleProjects(t *testing.T) {
	// Test scenario: Events for multiple projects, one project loses all events
	// Expected: Entry without events deleted, others updated

	date := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	projectA := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	projectB := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	projectC := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")
	entryAID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	entryBID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	// Events for Project B and C only (A has no events now)
	eventStore := &mockEventStore{
		events: []*store.CalendarEvent{
			{
				ID:                   uuid.New(),
				UserID:               userID,
				Title:                "Meeting B",
				StartTime:            date.Add(9 * time.Hour),
				EndTime:              date.Add(10 * time.Hour),
				ClassificationStatus: store.StatusClassified,
				ProjectID:            &projectB,
			},
			{
				ID:                   uuid.New(),
				UserID:               userID,
				Title:                "Meeting C",
				StartTime:            date.Add(11 * time.Hour),
				EndTime:              date.Add(12 * time.Hour),
				ClassificationStatus: store.StatusClassified,
				ProjectID:            &projectC,
			},
		},
	}

	// Existing entries for A and B (A will be orphaned)
	entryStore := &mockTimeEntryStore{
		entries: []*store.TimeEntry{
			{
				ID:        entryAID,
				UserID:    userID,
				ProjectID: projectA,
				Date:      date,
				Hours:     1.0,
			},
			{
				ID:        entryBID,
				UserID:    userID,
				ProjectID: projectB,
				Date:      date,
				Hours:     0.5, // Will be updated to 1.0
			},
		},
	}

	svc := &Service{
		eventStore:     eventStore,
		timeEntryStore: entryStore,
	}

	err := svc.RecalculateForDate(context.Background(), userID, date)
	if err != nil {
		t.Fatalf("RecalculateForDate() error = %v", err)
	}

	// Verify Project A entry was deleted
	if len(entryStore.deletedIDs) != 1 {
		t.Errorf("Expected 1 deleted entry, got %d", len(entryStore.deletedIDs))
	}
	if len(entryStore.deletedIDs) > 0 && entryStore.deletedIDs[0] != entryAID {
		t.Errorf("Expected entry %s to be deleted, got %s", entryAID, entryStore.deletedIDs[0])
	}

	// Verify Project B updated and Project C created (2 upserts)
	if entryStore.upsertedCount != 2 {
		t.Errorf("Expected 2 upserted entries (B updated, C created), got %d", entryStore.upsertedCount)
	}
}
