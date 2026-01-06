package handler

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/google"
	"github.com/michaelw/timesheet-app/service/internal/store"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// InvoiceHandler implements the invoice endpoints
type InvoiceHandler struct {
	invoices  *store.InvoiceStore
	projects  *store.ProjectStore
	sheets    *google.SheetsService
	calendars *store.CalendarConnectionStore
}

// NewInvoiceHandler creates a new invoice handler
func NewInvoiceHandler(invoices *store.InvoiceStore, projects *store.ProjectStore, sheets *google.SheetsService, calendars *store.CalendarConnectionStore) *InvoiceHandler {
	return &InvoiceHandler{
		invoices:  invoices,
		projects:  projects,
		sheets:    sheets,
		calendars: calendars,
	}
}

// ListInvoices returns all invoices for the authenticated user
func (h *InvoiceHandler) ListInvoices(ctx context.Context, req api.ListInvoicesRequestObject) (api.ListInvoicesResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ListInvoices401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	var statusStr *string
	if req.Params.Status != nil {
		s := string(*req.Params.Status)
		statusStr = &s
	}

	invoices, err := h.invoices.List(ctx, userID, req.Params.ProjectId, statusStr)
	if err != nil {
		return nil, err
	}

	result := make([]api.Invoice, len(invoices))
	for i, inv := range invoices {
		result[i] = invoiceToAPI(inv)
	}

	return api.ListInvoices200JSONResponse(result), nil
}

// CreateInvoice creates a new invoice from unbilled entries
func (h *InvoiceHandler) CreateInvoice(ctx context.Context, req api.CreateInvoiceRequestObject) (api.CreateInvoiceResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.CreateInvoice401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	if req.Body == nil {
		return api.CreateInvoice400JSONResponse{
			Code:    "invalid_request",
			Message: "Request body required",
		}, nil
	}

	periodStart := req.Body.PeriodStart.Time
	periodEnd := req.Body.PeriodEnd.Time

	// Default invoice date to today
	invoiceDate := time.Now().UTC()
	if req.Body.InvoiceDate != nil {
		invoiceDate = req.Body.InvoiceDate.Time
	}

	// Verify project exists and belongs to user
	_, err := h.projects.GetByID(ctx, userID, req.Body.ProjectId)
	if err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			return api.CreateInvoice400JSONResponse{
				Code:    "not_found",
				Message: "Project not found",
			}, nil
		}
		return nil, err
	}

	// Create invoice
	invoice, err := h.invoices.Create(ctx, userID, req.Body.ProjectId, periodStart, periodEnd, invoiceDate)
	if err != nil {
		if errors.Is(err, store.ErrNoUnbilledEntries) {
			return api.CreateInvoice400JSONResponse{
				Code:    "no_entries",
				Message: "No unbilled entries found in the specified date range",
			}, nil
		}
		return nil, err
	}

	return api.CreateInvoice201JSONResponse(invoiceToAPI(invoice)), nil
}

// GetInvoice returns a single invoice with line items
func (h *InvoiceHandler) GetInvoice(ctx context.Context, req api.GetInvoiceRequestObject) (api.GetInvoiceResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.GetInvoice401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	invoice, err := h.invoices.GetByID(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrInvoiceNotFound) {
			return api.GetInvoice404JSONResponse{
				Code:    "not_found",
				Message: "Invoice not found",
			}, nil
		}
		return nil, err
	}

	return api.GetInvoice200JSONResponse(invoiceToAPI(invoice)), nil
}

// DeleteInvoice deletes a draft invoice
func (h *InvoiceHandler) DeleteInvoice(ctx context.Context, req api.DeleteInvoiceRequestObject) (api.DeleteInvoiceResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.DeleteInvoice401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	err := h.invoices.Delete(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrInvoiceNotFound) {
			return api.DeleteInvoice404JSONResponse{
				Code:    "not_found",
				Message: "Invoice not found",
			}, nil
		}
		if errors.Is(err, store.ErrInvoiceNotDraft) {
			return api.DeleteInvoice404JSONResponse{
				Code:    "not_draft",
				Message: "Only draft invoices can be deleted",
			}, nil
		}
		return nil, err
	}

	return api.DeleteInvoice204Response{}, nil
}

// UpdateInvoiceStatus updates the status of an invoice
func (h *InvoiceHandler) UpdateInvoiceStatus(ctx context.Context, req api.UpdateInvoiceStatusRequestObject) (api.UpdateInvoiceStatusResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.UpdateInvoiceStatus401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	if req.Body == nil {
		return api.UpdateInvoiceStatus400JSONResponse{
			Code:    "invalid_request",
			Message: "Request body required",
		}, nil
	}

	invoice, err := h.invoices.UpdateStatus(ctx, userID, req.Id, string(req.Body.Status))
	if err != nil {
		if errors.Is(err, store.ErrInvoiceNotFound) {
			return api.UpdateInvoiceStatus404JSONResponse{
				Code:    "not_found",
				Message: "Invoice not found",
			}, nil
		}
		if errors.Is(err, store.ErrInvalidStatusChange) {
			return api.UpdateInvoiceStatus400JSONResponse{
				Code:    "invalid_status",
				Message: "Invalid status value",
			}, nil
		}
		return nil, err
	}

	return api.UpdateInvoiceStatus200JSONResponse(invoiceToAPI(invoice)), nil
}

// ExportInvoiceCSV generates and returns a CSV export of an invoice
func (h *InvoiceHandler) ExportInvoiceCSV(ctx context.Context, req api.ExportInvoiceCSVRequestObject) (api.ExportInvoiceCSVResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ExportInvoiceCSV401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	invoice, err := h.invoices.GetByID(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrInvoiceNotFound) {
			return api.ExportInvoiceCSV404JSONResponse{
				Code:    "not_found",
				Message: "Invoice not found",
			}, nil
		}
		return nil, err
	}

	// Generate CSV
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Write header rows
	w.Write([]string{"Invoice Number:", invoice.InvoiceNumber})
	if invoice.Project != nil {
		w.Write([]string{"Project:", invoice.Project.Name})
		if invoice.Project.Client != nil && *invoice.Project.Client != "" {
			w.Write([]string{"Client:", *invoice.Project.Client})
		}
	}
	w.Write([]string{"Period:", fmt.Sprintf("%s to %s", invoice.PeriodStart.Format("2006-01-02"), invoice.PeriodEnd.Format("2006-01-02"))})
	w.Write([]string{"Invoice Date:", invoice.InvoiceDate.Format("2006-01-02")})
	w.Write([]string{"Status:", invoice.Status})
	w.Write([]string{}) // Empty row

	// Write column headers
	w.Write([]string{"Date", "Description", "Hours", "Rate", "Amount"})

	// Write line items
	for _, item := range invoice.LineItems {
		w.Write([]string{
			item.Date.Format("2006-01-02"),
			item.Description,
			fmt.Sprintf("%.2f", item.Hours),
			fmt.Sprintf("%.2f", item.HourlyRate),
			fmt.Sprintf("%.2f", item.Amount),
		})
	}

	// Write totals row
	w.Write([]string{}) // Empty row
	w.Write([]string{
		"Total",
		"",
		fmt.Sprintf("%.2f", invoice.TotalHours),
		"",
		fmt.Sprintf("%.2f", invoice.TotalAmount),
	})

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}

	// Return as CSV response
	return api.ExportInvoiceCSV200TextcsvResponse{
		Body: io.NopCloser(&buf),
	}, nil
}

// ExportInvoiceSheets exports an invoice to Google Sheets
func (h *InvoiceHandler) ExportInvoiceSheets(ctx context.Context, req api.ExportInvoiceSheetsRequestObject) (api.ExportInvoiceSheetsResponseObject, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return api.ExportInvoiceSheets401JSONResponse{
			Code:    "unauthorized",
			Message: "Authentication required",
		}, nil
	}

	// Get invoice
	invoice, err := h.invoices.GetByID(ctx, userID, req.Id)
	if err != nil {
		if errors.Is(err, store.ErrInvoiceNotFound) {
			return api.ExportInvoiceSheets404JSONResponse{
				Code:    "not_found",
				Message: "Invoice not found",
			}, nil
		}
		return nil, err
	}

	// Get OAuth credentials from calendar connection
	// First get list to find connection ID, then get full connection with credentials
	conns, err := h.calendars.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(conns) == 0 {
		return api.ExportInvoiceSheets401JSONResponse{
			Code:    "no_connection",
			Message: "No Google Calendar connection found. Please connect your calendar first.",
		}, nil
	}
	// Get full connection with credentials (List doesn't include credentials for security)
	conn, err := h.calendars.GetByID(ctx, userID, conns[0].ID)
	if err != nil {
		return nil, err
	}

	// Create OAuth token
	token := h.sheets.TokenFromConnection(conn)

	// Prepare invoice data
	invoiceData := google.InvoiceData{
		InvoiceNumber: invoice.InvoiceNumber,
		ProjectName:   invoice.Project.Name,
		PeriodStart:   invoice.PeriodStart,
		PeriodEnd:     invoice.PeriodEnd,
		InvoiceDate:   invoice.InvoiceDate,
		Status:        invoice.Status,
		TotalHours:    invoice.TotalHours,
		TotalAmount:   invoice.TotalAmount,
	}
	if invoice.Project.Client != nil {
		invoiceData.Client = *invoice.Project.Client
	}
	for _, item := range invoice.LineItems {
		invoiceData.LineItems = append(invoiceData.LineItems, google.InvoiceLineItemData{
			Date:        item.Date,
			Description: item.Description,
			Hours:       item.Hours,
			HourlyRate:  item.HourlyRate,
			Amount:      item.Amount,
		})
	}

	// Check if project already has a spreadsheet
	project, err := h.projects.GetByID(ctx, userID, invoice.ProjectID)
	if err != nil {
		return nil, err
	}

	var spreadsheetID string
	var spreadsheetURL string

	if project.SheetsSpreadsheetID == nil || *project.SheetsSpreadsheetID == "" {
		// Create new spreadsheet
		title := fmt.Sprintf("%s - Invoices", project.Name)
		spreadsheetID, spreadsheetURL, err = h.sheets.CreateSpreadsheet(ctx, token, title)
		if err != nil {
			return api.ExportInvoiceSheets404JSONResponse{
				Code:    "sheets_error",
				Message: fmt.Sprintf("Failed to create spreadsheet: %s", err.Error()),
			}, nil
		}

		// Update project with spreadsheet info
		updates := map[string]interface{}{
			"sheets_spreadsheet_id":  spreadsheetID,
			"sheets_spreadsheet_url": spreadsheetURL,
		}
		_, err = h.projects.Update(ctx, userID, project.ID, updates)
		if err != nil {
			return nil, err
		}
	} else {
		spreadsheetID = *project.SheetsSpreadsheetID
		if project.SheetsSpreadsheetURL != nil {
			spreadsheetURL = *project.SheetsSpreadsheetURL
		}
	}

	// Check if invoice already has a worksheet (re-export)
	worksheetTitle := invoice.InvoiceNumber
	var worksheetID int

	if invoice.WorksheetID != nil {
		// Update existing worksheet
		worksheetID, err = h.sheets.UpdateInvoiceWorksheet(ctx, token, spreadsheetID, worksheetTitle, invoiceData)
		if err != nil {
			return api.ExportInvoiceSheets404JSONResponse{
				Code:    "sheets_error",
				Message: fmt.Sprintf("Failed to update worksheet: %s", err.Error()),
			}, nil
		}
	} else {
		// Create new worksheet
		worksheetID, err = h.sheets.CreateInvoiceWorksheet(ctx, token, spreadsheetID, worksheetTitle, invoiceData)
		if err != nil {
			return api.ExportInvoiceSheets404JSONResponse{
				Code:    "sheets_error",
				Message: fmt.Sprintf("Failed to create worksheet: %s", err.Error()),
			}, nil
		}
	}

	// Update invoice with spreadsheet info
	err = h.invoices.UpdateSpreadsheetInfo(ctx, userID, invoice.ID, spreadsheetID, spreadsheetURL, worksheetID)
	if err != nil {
		return nil, err
	}

	return api.ExportInvoiceSheets200JSONResponse{
		SpreadsheetId:  &spreadsheetID,
		SpreadsheetUrl: &spreadsheetURL,
		WorksheetId:    &worksheetID,
	}, nil
}

// invoiceToAPI converts a store Invoice to an API Invoice
func invoiceToAPI(inv *store.Invoice) api.Invoice {
	invoice := api.Invoice{
		Id:            inv.ID,
		UserId:        inv.UserID,
		ProjectId:     inv.ProjectID,
		InvoiceNumber: inv.InvoiceNumber,
		PeriodStart:   openapi_types.Date{Time: inv.PeriodStart},
		PeriodEnd:     openapi_types.Date{Time: inv.PeriodEnd},
		InvoiceDate:   openapi_types.Date{Time: inv.InvoiceDate},
		Status:        api.InvoiceStatus(inv.Status),
		TotalHours:    float32(inv.TotalHours),
		TotalAmount:   float32(inv.TotalAmount),
		CreatedAt:     inv.CreatedAt,
	}

	if inv.BillingPeriodID != nil {
		invoice.BillingPeriodId = inv.BillingPeriodID
	}

	if inv.Project != nil {
		project := projectToAPI(inv.Project)
		invoice.Project = &project
	}

	if len(inv.LineItems) > 0 {
		lineItems := make([]api.InvoiceLineItem, len(inv.LineItems))
		for i, item := range inv.LineItems {
			lineItems[i] = api.InvoiceLineItem{
				Id:          item.ID,
				InvoiceId:   item.InvoiceID,
				TimeEntryId: item.TimeEntryID,
				Date:        openapi_types.Date{Time: item.Date},
				Description: item.Description,
				Hours:       float32(item.Hours),
				HourlyRate:  float32(item.HourlyRate),
				Amount:      float32(item.Amount),
			}
		}
		invoice.LineItems = &lineItems
	}

	if inv.SpreadsheetID != nil {
		invoice.SpreadsheetId = inv.SpreadsheetID
	}
	if inv.SpreadsheetURL != nil {
		invoice.SpreadsheetUrl = inv.SpreadsheetURL
	}
	if inv.WorksheetID != nil {
		invoice.WorksheetId = inv.WorksheetID
	}

	updatedAt := inv.UpdatedAt
	invoice.UpdatedAt = &updatedAt

	return invoice
}
