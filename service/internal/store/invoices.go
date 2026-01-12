package store

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvoiceNotFound     = errors.New("invoice not found")
	ErrInvoiceNotDraft     = errors.New("invoice is not a draft")
	ErrNoUnbilledEntries   = errors.New("no unbilled entries found in date range")
	ErrInvalidStatusChange = errors.New("invalid status change")
)

// Invoice represents a stored invoice
type Invoice struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	ProjectID        uuid.UUID
	BillingPeriodID  *uuid.UUID
	InvoiceNumber    string
	PeriodStart      time.Time
	PeriodEnd        time.Time
	InvoiceDate      time.Time
	Status           string
	TotalHours       float64
	TotalAmount      float64
	SpreadsheetID    *string
	SpreadsheetURL   *string
	WorksheetID      *int
	CreatedAt        time.Time
	UpdatedAt        time.Time
	// Joined data
	Project   *Project
	LineItems []InvoiceLineItem
}

// InvoiceLineItem represents a line item in an invoice
type InvoiceLineItem struct {
	ID          uuid.UUID
	InvoiceID   uuid.UUID
	TimeEntryID uuid.UUID
	Date        time.Time
	Description string
	Hours       float64
	HourlyRate  float64
	Amount      float64
}

// InvoiceStore provides PostgreSQL-backed invoice storage
type InvoiceStore struct {
	pool           *pgxpool.Pool
	timeEntries    *TimeEntryStore
	billingPeriods *BillingPeriodStore
	projects       *ProjectStore
}

// NewInvoiceStore creates a new PostgreSQL invoice store
func NewInvoiceStore(pool *pgxpool.Pool, timeEntries *TimeEntryStore, billingPeriods *BillingPeriodStore, projects *ProjectStore) *InvoiceStore {
	return &InvoiceStore{
		pool:           pool,
		timeEntries:    timeEntries,
		billingPeriods: billingPeriods,
		projects:       projects,
	}
}

// generateInvoiceNumber creates an invoice number in format PROJECT-YEAR-SEQ
func (s *InvoiceStore) generateInvoiceNumber(ctx context.Context, tx pgx.Tx, userID, projectID uuid.UUID, invoiceDate time.Time) (string, error) {
	// Get project for short_code or name
	project, err := s.projects.GetByID(ctx, userID, projectID)
	if err != nil {
		return "", err
	}

	// Use short_code if available, otherwise derive from name
	var prefix string
	if project.ShortCode != nil && *project.ShortCode != "" {
		prefix = *project.ShortCode
	} else {
		// Derive from name: "Acme Corp" -> "ACME"
		// Remove non-alphanumeric, take first word, uppercase
		re := regexp.MustCompile(`[^a-zA-Z0-9\s]+`)
		cleaned := re.ReplaceAllString(project.Name, "")
		words := strings.Fields(cleaned)
		if len(words) > 0 {
			prefix = strings.ToUpper(words[0])
		} else {
			prefix = "INV"
		}
	}

	year := invoiceDate.Year()

	// Query for max sequence number for this project and year
	var maxSeq int
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(
			CAST(
				SUBSTRING(invoice_number FROM '[0-9]+$') AS INTEGER
			)
		), 0)
		FROM invoices
		WHERE user_id = $1
		  AND project_id = $2
		  AND EXTRACT(YEAR FROM invoice_date) = $3
	`, userID, projectID, year).Scan(&maxSeq)
	if err != nil {
		return "", err
	}

	nextSeq := maxSeq + 1
	return fmt.Sprintf("%s-%d-%03d", prefix, year, nextSeq), nil
}

// Create generates an invoice from unbilled time entries
func (s *InvoiceStore) Create(ctx context.Context, userID, projectID uuid.UUID, periodStart, periodEnd, invoiceDate time.Time) (*Invoice, error) {
	// Start transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Fetch unbilled time entries in date range
	rows, err := tx.Query(ctx, `
		SELECT id, project_id, date, hours, title, description
		FROM time_entries
		WHERE user_id = $1
		  AND project_id = $2
		  AND date >= $3
		  AND date <= $4
		  AND invoice_id IS NULL
		ORDER BY date ASC
	`, userID, projectID, periodStart, periodEnd)
	if err != nil {
		return nil, err
	}

	var timeEntries []struct {
		ID          uuid.UUID
		ProjectID   uuid.UUID
		Date        time.Time
		Hours       float64
		Title       *string
		Description *string
	}

	for rows.Next() {
		var entry struct {
			ID          uuid.UUID
			ProjectID   uuid.UUID
			Date        time.Time
			Hours       float64
			Title       *string
			Description *string
		}
		if err := rows.Scan(&entry.ID, &entry.ProjectID, &entry.Date, &entry.Hours, &entry.Title, &entry.Description); err != nil {
			rows.Close()
			return nil, err
		}
		timeEntries = append(timeEntries, entry)
	}
	rows.Close()

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(timeEntries) == 0 {
		return nil, ErrNoUnbilledEntries
	}

	// Fetch all billing periods for this project once (instead of N queries)
	billingPeriods, err := s.billingPeriods.ListByProject(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}

	// Helper function to find billing period for a date
	findPeriodForDate := func(date time.Time) *BillingPeriod {
		for _, period := range billingPeriods {
			if !period.StartsOn.After(date) && (period.EndsOn == nil || !period.EndsOn.Before(date)) {
				return period
			}
		}
		return nil
	}

	// Generate invoice number
	invoiceNumber, err := s.generateInvoiceNumber(ctx, tx, userID, projectID, invoiceDate)
	if err != nil {
		return nil, err
	}

	// Create invoice
	invoice := &Invoice{
		ID:            uuid.New(),
		UserID:        userID,
		ProjectID:     projectID,
		InvoiceNumber: invoiceNumber,
		PeriodStart:   periodStart,
		PeriodEnd:     periodEnd,
		InvoiceDate:   invoiceDate,
		Status:        "draft",
		TotalHours:    0,
		TotalAmount:   0,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}

	// Create line items and calculate totals
	var lineItems []InvoiceLineItem
	for _, entry := range timeEntries {
		// Find billing period for this date (in-memory lookup)
		period := findPeriodForDate(entry.Date)
		var hourlyRate float64
		var billingPeriodID *uuid.UUID

		if period == nil {
			// No billing period found - use $0/hr
			hourlyRate = 0
		} else {
			hourlyRate = period.HourlyRate
			billingPeriodID = &period.ID
			// Set invoice's billing_period_id to the first one found
			if invoice.BillingPeriodID == nil {
				invoice.BillingPeriodID = billingPeriodID
			}
		}

		amount := entry.Hours * hourlyRate

		// Build description from title and/or description
		var desc string
		if entry.Title != nil && *entry.Title != "" {
			desc = *entry.Title
			if entry.Description != nil && *entry.Description != "" {
				desc = desc + " - " + *entry.Description
			}
		} else if entry.Description != nil {
			desc = *entry.Description
		} else {
			desc = "Time entry"
		}

		lineItem := InvoiceLineItem{
			ID:          uuid.New(),
			InvoiceID:   invoice.ID,
			TimeEntryID: entry.ID,
			Date:        entry.Date,
			Description: desc,
			Hours:       entry.Hours,
			HourlyRate:  hourlyRate,
			Amount:      amount,
		}
		lineItems = append(lineItems, lineItem)

		invoice.TotalHours += entry.Hours
		invoice.TotalAmount += amount
	}

	// Insert invoice
	_, err = tx.Exec(ctx, `
		INSERT INTO invoices (
			id, user_id, project_id, billing_period_id, invoice_number,
			period_start, period_end, invoice_date, status,
			total_hours, total_amount, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, invoice.ID, invoice.UserID, invoice.ProjectID, invoice.BillingPeriodID,
		invoice.InvoiceNumber, invoice.PeriodStart, invoice.PeriodEnd,
		invoice.InvoiceDate, invoice.Status, invoice.TotalHours,
		invoice.TotalAmount, invoice.CreatedAt, invoice.UpdatedAt)
	if err != nil {
		return nil, err
	}

	// Insert line items
	for _, item := range lineItems {
		_, err = tx.Exec(ctx, `
			INSERT INTO invoice_line_items (
				id, invoice_id, time_entry_id, date, description,
				hours, hourly_rate, amount
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, item.ID, item.InvoiceID, item.TimeEntryID, item.Date,
			item.Description, item.Hours, item.HourlyRate, item.Amount)
		if err != nil {
			return nil, err
		}
	}

	// Set invoice_id on time entries immediately (locks them from editing)
	entryIDs := make([]uuid.UUID, len(timeEntries))
	for i, e := range timeEntries {
		entryIDs[i] = e.ID
	}
	_, err = tx.Exec(ctx, `
		UPDATE time_entries
		SET invoice_id = $1, updated_at = NOW()
		WHERE id = ANY($2)
	`, invoice.ID, entryIDs)
	if err != nil {
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	invoice.LineItems = lineItems

	// Load project data
	project, err := s.projects.GetByID(ctx, userID, projectID)
	if err == nil {
		invoice.Project = project
	}

	return invoice, nil
}

// GetByID retrieves an invoice with line items and project data
func (s *InvoiceStore) GetByID(ctx context.Context, userID, invoiceID uuid.UUID) (*Invoice, error) {
	invoice := &Invoice{Project: &Project{}}
	err := s.pool.QueryRow(ctx, `
		SELECT i.id, i.user_id, i.project_id, i.billing_period_id,
		       i.invoice_number, i.period_start, i.period_end,
		       i.invoice_date, i.status, i.total_hours, i.total_amount,
		       i.spreadsheet_id, i.spreadsheet_url, i.worksheet_id,
		       i.created_at, i.updated_at,
		       p.id, p.user_id, p.name, p.short_code, p.client, p.color,
		       p.is_billable, p.is_archived, p.is_hidden_by_default,
		       p.does_not_accumulate_hours, p.created_at, p.updated_at
		FROM invoices i
		JOIN projects p ON i.project_id = p.id
		WHERE i.id = $1 AND i.user_id = $2
	`, invoiceID, userID).Scan(
		&invoice.ID, &invoice.UserID, &invoice.ProjectID, &invoice.BillingPeriodID,
		&invoice.InvoiceNumber, &invoice.PeriodStart, &invoice.PeriodEnd,
		&invoice.InvoiceDate, &invoice.Status, &invoice.TotalHours, &invoice.TotalAmount,
		&invoice.SpreadsheetID, &invoice.SpreadsheetURL, &invoice.WorksheetID,
		&invoice.CreatedAt, &invoice.UpdatedAt,
		// Project fields
		&invoice.Project.ID, &invoice.Project.UserID, &invoice.Project.Name,
		&invoice.Project.ShortCode, &invoice.Project.Client, &invoice.Project.Color,
		&invoice.Project.IsBillable, &invoice.Project.IsArchived,
		&invoice.Project.IsHiddenByDefault, &invoice.Project.DoesNotAccumulateHours,
		&invoice.Project.CreatedAt, &invoice.Project.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvoiceNotFound
		}
		return nil, err
	}

	// Load line items - JOIN to time_entries for current hours/date/description
	// Amount is recalculated as hours × rate to stay in sync with time entry
	// (for sent/paid invoices, time entries are locked so values won't change)
	rows, err := s.pool.Query(ctx, `
		SELECT ili.id, ili.invoice_id, ili.time_entry_id,
		       te.date,
		       COALESCE(te.title || CASE WHEN te.description IS NOT NULL AND te.description != '' THEN ' - ' || te.description ELSE '' END, te.description, 'Time entry') as description,
		       te.hours, ili.hourly_rate, te.hours * ili.hourly_rate as amount
		FROM invoice_line_items ili
		JOIN time_entries te ON ili.time_entry_id = te.id
		WHERE ili.invoice_id = $1
		ORDER BY te.date ASC
	`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lineItems []InvoiceLineItem
	for rows.Next() {
		var item InvoiceLineItem
		if err := rows.Scan(&item.ID, &item.InvoiceID, &item.TimeEntryID,
			&item.Date, &item.Description, &item.Hours, &item.HourlyRate, &item.Amount); err != nil {
			return nil, err
		}
		lineItems = append(lineItems, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	invoice.LineItems = lineItems
	return invoice, nil
}

// List retrieves all invoices for a user with optional filters
func (s *InvoiceStore) List(ctx context.Context, userID uuid.UUID, projectID *uuid.UUID, status *string) ([]*Invoice, error) {
	query := `
		SELECT i.id, i.user_id, i.project_id, i.billing_period_id,
		       i.invoice_number, i.period_start, i.period_end,
		       i.invoice_date, i.status, i.total_hours, i.total_amount,
		       i.spreadsheet_id, i.spreadsheet_url, i.worksheet_id,
		       i.created_at, i.updated_at,
		       p.id, p.user_id, p.name, p.short_code, p.client, p.color,
		       p.is_billable, p.is_archived, p.is_hidden_by_default,
		       p.does_not_accumulate_hours, p.created_at, p.updated_at
		FROM invoices i
		JOIN projects p ON i.project_id = p.id
		WHERE i.user_id = $1
	`

	args := []interface{}{userID}
	argNum := 2

	if projectID != nil {
		query += fmt.Sprintf(" AND i.project_id = $%d", argNum)
		args = append(args, *projectID)
		argNum++
	}

	if status != nil {
		query += fmt.Sprintf(" AND i.status = $%d", argNum)
		args = append(args, *status)
		argNum++
	}

	query += " ORDER BY i.invoice_date DESC, i.created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invoices []*Invoice
	for rows.Next() {
		invoice := &Invoice{Project: &Project{}}
		if err := rows.Scan(
			&invoice.ID, &invoice.UserID, &invoice.ProjectID, &invoice.BillingPeriodID,
			&invoice.InvoiceNumber, &invoice.PeriodStart, &invoice.PeriodEnd,
			&invoice.InvoiceDate, &invoice.Status, &invoice.TotalHours, &invoice.TotalAmount,
			&invoice.SpreadsheetID, &invoice.SpreadsheetURL, &invoice.WorksheetID,
			&invoice.CreatedAt, &invoice.UpdatedAt,
			// Project fields
			&invoice.Project.ID, &invoice.Project.UserID, &invoice.Project.Name,
			&invoice.Project.ShortCode, &invoice.Project.Client, &invoice.Project.Color,
			&invoice.Project.IsBillable, &invoice.Project.IsArchived,
			&invoice.Project.IsHiddenByDefault, &invoice.Project.DoesNotAccumulateHours,
			&invoice.Project.CreatedAt, &invoice.Project.UpdatedAt,
		); err != nil {
			return nil, err
		}
		invoices = append(invoices, invoice)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return invoices, nil
}

// UpdateStatus updates an invoice status and handles time entry locking
func (s *InvoiceStore) UpdateStatus(ctx context.Context, userID, invoiceID uuid.UUID, newStatus string) (*Invoice, error) {
	// Validate status
	if newStatus != "draft" && newStatus != "sent" && newStatus != "paid" {
		return nil, ErrInvalidStatusChange
	}

	// Start transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Get current invoice status
	var currentStatus string
	err = tx.QueryRow(ctx, `
		SELECT status
		FROM invoices
		WHERE id = $1 AND user_id = $2
	`, invoiceID, userID).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvoiceNotFound
		}
		return nil, err
	}

	// Validate state transition
	if currentStatus == newStatus {
		// No change needed
		return s.GetByID(ctx, userID, invoiceID)
	}

	// Note: Time entries have invoice_id set at invoice creation time and remain
	// locked regardless of invoice status changes. Only deleting the invoice
	// (draft only) will unlock them.

	// Update invoice status
	_, err = tx.Exec(ctx, `
		UPDATE invoices
		SET status = $1, updated_at = NOW()
		WHERE id = $2 AND user_id = $3
	`, newStatus, invoiceID, userID)
	if err != nil {
		return nil, err
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Return updated invoice
	return s.GetByID(ctx, userID, invoiceID)
}

// Delete removes an invoice (only allowed for draft invoices)
func (s *InvoiceStore) Delete(ctx context.Context, userID, invoiceID uuid.UUID) error {
	// Start transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Check status
	var status string
	err = tx.QueryRow(ctx, `
		SELECT status FROM invoices WHERE id = $1 AND user_id = $2
	`, invoiceID, userID).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvoiceNotFound
		}
		return err
	}

	if status != "draft" {
		return ErrInvoiceNotDraft
	}

	// Clear invoice_id on time entries (unlocks them for editing)
	_, err = tx.Exec(ctx, `
		UPDATE time_entries SET invoice_id = NULL, updated_at = NOW()
		WHERE invoice_id = $1
	`, invoiceID)
	if err != nil {
		return err
	}

	// Delete line items
	_, err = tx.Exec(ctx, `
		DELETE FROM invoice_line_items WHERE invoice_id = $1
	`, invoiceID)
	if err != nil {
		return err
	}

	// Delete invoice
	result, err := tx.Exec(ctx, `
		DELETE FROM invoices WHERE id = $1 AND user_id = $2
	`, invoiceID, userID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrInvoiceNotFound
	}

	return tx.Commit(ctx)
}

// UpdateSpreadsheetInfo updates the Sheets export metadata
func (s *InvoiceStore) UpdateSpreadsheetInfo(ctx context.Context, userID, invoiceID uuid.UUID, spreadsheetID, spreadsheetURL string, worksheetID int) error {
	result, err := s.pool.Exec(ctx, `
		UPDATE invoices
		SET spreadsheet_id = $1,
		    spreadsheet_url = $2,
		    worksheet_id = $3,
		    updated_at = NOW()
		WHERE id = $4 AND user_id = $5
	`, spreadsheetID, spreadsheetURL, worksheetID, invoiceID, userID)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return ErrInvoiceNotFound
	}

	return nil
}
