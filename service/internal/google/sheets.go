package google

import (
	"context"
	"fmt"
	"time"

	"github.com/michaelw/timesheet-app/service/internal/store"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// SheetsService handles Google Sheets API interactions
type SheetsService struct {
	config *oauth2.Config
}

// NewSheetsService creates a new Google Sheets service
func NewSheetsService(clientID, clientSecret, redirectURL string) *SheetsService {
	return &SheetsService{
		config: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{drive.DriveFileScope},
			Endpoint:     google.Endpoint,
		},
	}
}

// InvoiceData contains the data needed to export an invoice
type InvoiceData struct {
	InvoiceNumber string
	ProjectName   string
	Client        string
	PeriodStart   time.Time
	PeriodEnd     time.Time
	InvoiceDate   time.Time
	Status        string
	TotalHours    float64
	TotalAmount   float64
	LineItems     []InvoiceLineItemData
}

// InvoiceLineItemData contains line item data for export
type InvoiceLineItemData struct {
	Date        time.Time
	Description string
	Hours       float64
	HourlyRate  float64
	Amount      float64
}

// TokenFromConnection converts a calendar connection to an OAuth token
func (s *SheetsService) TokenFromConnection(conn *store.CalendarConnection) *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  conn.Credentials.AccessToken,
		RefreshToken: conn.Credentials.RefreshToken,
		TokenType:    conn.Credentials.TokenType,
		Expiry:       conn.Credentials.Expiry,
	}
}

// CreateSpreadsheet creates a new Google Sheets spreadsheet
func (s *SheetsService) CreateSpreadsheet(ctx context.Context, token *oauth2.Token, title string) (string, string, error) {
	srv, err := s.getService(ctx, token)
	if err != nil {
		return "", "", err
	}

	spreadsheet := &sheets.Spreadsheet{
		Properties: &sheets.SpreadsheetProperties{
			Title: title,
		},
	}

	result, err := srv.Spreadsheets.Create(spreadsheet).Do()
	if err != nil {
		return "", "", err
	}

	return result.SpreadsheetId, result.SpreadsheetUrl, nil
}

// CreateInvoiceWorksheet creates a new worksheet in a spreadsheet with invoice data
func (s *SheetsService) CreateInvoiceWorksheet(ctx context.Context, token *oauth2.Token, spreadsheetID, worksheetTitle string, data InvoiceData) (int, error) {
	srv, err := s.getService(ctx, token)
	if err != nil {
		return 0, err
	}

	// Create a new sheet
	addSheetReq := &sheets.Request{
		AddSheet: &sheets.AddSheetRequest{
			Properties: &sheets.SheetProperties{
				Title: worksheetTitle,
			},
		},
	}

	batchReq := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{addSheetReq},
	}

	resp, err := srv.Spreadsheets.BatchUpdate(spreadsheetID, batchReq).Do()
	if err != nil {
		return 0, err
	}

	sheetID := int(resp.Replies[0].AddSheet.Properties.SheetId)

	// Write invoice data to the sheet
	err = s.writeInvoiceData(ctx, srv, spreadsheetID, worksheetTitle, data)
	if err != nil {
		return 0, err
	}

	return sheetID, nil
}

// UpdateInvoiceWorksheet updates an existing worksheet with invoice data
func (s *SheetsService) UpdateInvoiceWorksheet(ctx context.Context, token *oauth2.Token, spreadsheetID, worksheetTitle string, data InvoiceData) (int, error) {
	srv, err := s.getService(ctx, token)
	if err != nil {
		return 0, err
	}

	// Get the sheet to find its ID
	spreadsheet, err := srv.Spreadsheets.Get(spreadsheetID).Do()
	if err != nil {
		return 0, err
	}

	var sheetID int64
	found := false
	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == worksheetTitle {
			sheetID = sheet.Properties.SheetId
			found = true
			break
		}
	}

	if !found {
		return 0, fmt.Errorf("worksheet %s not found", worksheetTitle)
	}

	// Clear existing data
	clearReq := &sheets.ClearValuesRequest{}
	_, err = srv.Spreadsheets.Values.Clear(spreadsheetID, worksheetTitle, clearReq).Do()
	if err != nil {
		return 0, err
	}

	// Write updated invoice data
	err = s.writeInvoiceData(ctx, srv, spreadsheetID, worksheetTitle, data)
	if err != nil {
		return 0, err
	}

	return int(sheetID), nil
}

// writeInvoiceData writes invoice data to a worksheet with formatting
func (s *SheetsService) writeInvoiceData(ctx context.Context, srv *sheets.Service, spreadsheetID, worksheetTitle string, data InvoiceData) error {
	// Build rows - just column headers and line items (no metadata or totals)
	var values [][]interface{}

	// Column headers
	values = append(values, []interface{}{"Date", "Description", "Hours", "Rate", "Amount"})

	// Line items
	for _, item := range data.LineItems {
		values = append(values, []interface{}{
			item.Date.Format("2006-01-02"),
			item.Description,
			item.Hours,
			item.HourlyRate,
			item.Amount,
		})
	}

	// Write values
	valueRange := &sheets.ValueRange{
		Values: values,
	}

	_, err := srv.Spreadsheets.Values.Update(
		spreadsheetID,
		fmt.Sprintf("%s!A1", worksheetTitle),
		valueRange,
	).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return err
	}

	// Apply formatting
	err = s.formatInvoiceSheet(ctx, srv, spreadsheetID, worksheetTitle, len(data.LineItems))
	if err != nil {
		return err
	}

	return nil
}

// formatInvoiceSheet applies formatting to the invoice worksheet
func (s *SheetsService) formatInvoiceSheet(ctx context.Context, srv *sheets.Service, spreadsheetID, worksheetTitle string, lineItemCount int) error {
	// Get the sheet ID
	spreadsheet, err := srv.Spreadsheets.Get(spreadsheetID).Do()
	if err != nil {
		return err
	}

	var sheetID int64
	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == worksheetTitle {
			sheetID = sheet.Properties.SheetId
			break
		}
	}

	// Column headers are at row 0, line items start at row 1
	requests := []*sheets.Request{
		// Bold the column headers (row 0)
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:       sheetID,
					StartRowIndex: 0,
					EndRowIndex:   1,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{
							Bold: true,
						},
					},
				},
				Fields: "userEnteredFormat.textFormat.bold",
			},
		},
		// Format Rate and Amount columns as currency (rows 1 onwards)
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    1,
					EndRowIndex:      int64(1 + lineItemCount),
					StartColumnIndex: 3, // Rate column
					EndColumnIndex:   5, // Amount column (inclusive)
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						NumberFormat: &sheets.NumberFormat{
							Type:    "CURRENCY",
							Pattern: "$#,##0.00",
						},
					},
				},
				Fields: "userEnteredFormat.numberFormat",
			},
		},
		// Auto-resize columns
		{
			AutoResizeDimensions: &sheets.AutoResizeDimensionsRequest{
				Dimensions: &sheets.DimensionRange{
					SheetId:    sheetID,
					Dimension:  "COLUMNS",
					StartIndex: 0,
					EndIndex:   5,
				},
			},
		},
	}

	batchReq := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}

	_, err = srv.Spreadsheets.BatchUpdate(spreadsheetID, batchReq).Do()
	return err
}

// InvoiceSummaryData contains summary data for an invoice in the Invoices sheet
type InvoiceSummaryData struct {
	InvoiceNumber string
	PeriodStart   time.Time
	PeriodEnd     time.Time
	InvoiceDate   time.Time
	Status        string
	TotalHours    float64
	TotalAmount   float64
}

// UpdateInvoicesSummary creates or updates the "Invoices" summary sheet
func (s *SheetsService) UpdateInvoicesSummary(ctx context.Context, token *oauth2.Token, spreadsheetID string, invoices []InvoiceSummaryData) error {
	srv, err := s.getService(ctx, token)
	if err != nil {
		return err
	}

	const summarySheetName = "Invoices"

	// Get the spreadsheet to check if the summary sheet exists
	spreadsheet, err := srv.Spreadsheets.Get(spreadsheetID).Do()
	if err != nil {
		return err
	}

	var sheetID int64
	sheetExists := false
	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == summarySheetName {
			sheetID = sheet.Properties.SheetId
			sheetExists = true
			break
		}
	}

	// Create the sheet if it doesn't exist
	if !sheetExists {
		addSheetReq := &sheets.Request{
			AddSheet: &sheets.AddSheetRequest{
				Properties: &sheets.SheetProperties{
					Title: summarySheetName,
					Index: 0, // Put it first
				},
			},
		}

		batchReq := &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{addSheetReq},
		}

		resp, err := srv.Spreadsheets.BatchUpdate(spreadsheetID, batchReq).Do()
		if err != nil {
			return err
		}

		sheetID = resp.Replies[0].AddSheet.Properties.SheetId
	} else {
		// Clear existing data
		clearReq := &sheets.ClearValuesRequest{}
		_, err = srv.Spreadsheets.Values.Clear(spreadsheetID, summarySheetName, clearReq).Do()
		if err != nil {
			return err
		}
	}

	// Build rows
	var values [][]interface{}

	// Column headers
	values = append(values, []interface{}{"Invoice", "Period Start", "Period End", "Invoice Date", "Status", "Hours", "Amount"})

	// Invoice rows
	for _, inv := range invoices {
		values = append(values, []interface{}{
			inv.InvoiceNumber,
			inv.PeriodStart.Format("2006-01-02"),
			inv.PeriodEnd.Format("2006-01-02"),
			inv.InvoiceDate.Format("2006-01-02"),
			inv.Status,
			inv.TotalHours,
			inv.TotalAmount,
		})
	}

	// Write values
	valueRange := &sheets.ValueRange{
		Values: values,
	}

	_, err = srv.Spreadsheets.Values.Update(
		spreadsheetID,
		fmt.Sprintf("%s!A1", summarySheetName),
		valueRange,
	).ValueInputOption("USER_ENTERED").Do()
	if err != nil {
		return err
	}

	// Apply formatting
	requests := []*sheets.Request{
		// Bold the column headers
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:       sheetID,
					StartRowIndex: 0,
					EndRowIndex:   1,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{
							Bold: true,
						},
					},
				},
				Fields: "userEnteredFormat.textFormat.bold",
			},
		},
		// Format Amount column as currency
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    1,
					EndRowIndex:      int64(1 + len(invoices)),
					StartColumnIndex: 6, // Amount column
					EndColumnIndex:   7,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						NumberFormat: &sheets.NumberFormat{
							Type:    "CURRENCY",
							Pattern: "$#,##0.00",
						},
					},
				},
				Fields: "userEnteredFormat.numberFormat",
			},
		},
		// Auto-resize columns
		{
			AutoResizeDimensions: &sheets.AutoResizeDimensionsRequest{
				Dimensions: &sheets.DimensionRange{
					SheetId:    sheetID,
					Dimension:  "COLUMNS",
					StartIndex: 0,
					EndIndex:   7,
				},
			},
		},
	}

	batchReq := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}

	_, err = srv.Spreadsheets.BatchUpdate(spreadsheetID, batchReq).Do()
	return err
}

// getService creates a Sheets service client with the given token
func (s *SheetsService) getService(ctx context.Context, token *oauth2.Token) (*sheets.Service, error) {
	client := s.config.Client(ctx, token)
	return sheets.NewService(ctx, option.WithHTTPClient(client))
}
