# PRD: Invoicing

> **Status**: Draft
> **Target**: Merge into main PRD after implementation complete

## 1. Problem Statement

**What problem are we solving?**
After classifying time entries, users need to generate invoices for billable projects. Currently, this requires manually exporting time data and reformatting it into invoices, duplicating effort and introducing potential for errors.

**Who experiences this problem?**
Consultants and freelancers who bill clients based on tracked time and need to produce regular invoices from their timesheet data.

**What's the impact of not solving it?**
- Manual effort to transform time entries into invoice format
- Risk of missing billable entries or calculation errors
- No audit trail linking invoices to source time entries
- Difficulty tracking which time periods have been invoiced

**Why now?**
The timesheet app already has billable projects with bill rates and accurately tracked time entries. Invoicing is the natural next step in the workflow.

## 2. Goals & Success Criteria

**What does success look like?**
- User can generate an invoice for a project's billable time entries in a date range with one click
- Invoice data is stored and can be re-exported or updated
- Seamless export to CSV or Google Sheets for client delivery
- Clear tracking of which time entries have been invoiced

**How will we measure it?**
- Time to generate an invoice (target: under 30 seconds)
- Accuracy: invoice totals match sum of included time entries (100%)
- Export success rate to Google Sheets (target: >95%)

**Primary outcomes we're optimizing for:**
1. Speed of invoice generation from existing time data
2. Accuracy and auditability of invoice amounts
3. Flexible export options (CSV, Google Sheets)

## 3. Non-Goals & Constraints

**What are we explicitly NOT doing?**
- PDF invoice generation (data export is sufficient)
- Payment processing or collection
- Accounts receivable / aging reports
- Tax calculation (user handles externally)
- Email/sending invoices directly to clients
- Multi-currency support (single currency assumed)

**Constraints:**
- Must integrate with existing project and time entry models
- Google Sheets export requires existing OAuth scope expansion
- Invoice numbers must be unique and sequential per user

## 4. Solution Overview

**High-level description:**
Add invoicing capability that:
1. Generates invoices from billable time entries for a project and date range
2. Stores invoice records with line items in the database
3. Tracks which time entries are included in each invoice
4. Exports invoice data as CSV or to Google Sheets
5. Supports a "living document" model for Google Sheets (one spreadsheet per project, one worksheet per invoice)

**Key user flows:**

1. **Create Invoice**: User selects a project, date range, and clicks "Generate Invoice". System creates invoice from unbilled time entries.

2. **Review Invoice**: User views invoice details showing line items (date, description, hours, rate, amount), totals, and status.

3. **Export to CSV**: User downloads invoice as CSV file for import into accounting software or client delivery.

4. **Export to Google Sheets**: User exports to Google Sheets. If project has an associated spreadsheet, adds a new worksheet; otherwise creates new spreadsheet and links it to the project.

5. **Update Invoice**: User can regenerate an invoice to include additional time entries or correct details. Previous export to Sheets is updated in place.

**Core concepts and terminology:**
- **Invoice**: A record of billable time for a project over a date range
- **Invoice Line Item**: Individual time entry included in an invoice (date, description, hours, rate, amount)
- **Invoice Status**: Draft, Finalized, Paid (user-managed)
- **Project Spreadsheet**: Optional Google Sheets document linked to a project for invoice exports

## 5. Detailed Requirements

### Functional Requirements

**Invoice Generation:**
- Generate invoice for a single project and date range
- Include only billable time entries (project.is_billable = true)
- Exclude time entries already included in another invoice (unless regenerating)
- Calculate line item amounts: hours × project.bill_rate
- Calculate invoice total: sum of line item amounts
- Auto-generate sequential invoice number (e.g., INV-2024-001)
- Store invoice date (defaults to generation date, editable)

**Invoice Data Model:**
- Invoice header: number, project, date range, invoice date, status, total hours, total amount
- Invoice line items: reference to time_entry, date, description, hours, rate, amount
- Link to Google Sheets spreadsheet ID and worksheet ID (if exported)

**Invoice Status:**
- Draft: Can be modified, regenerated, or deleted
- Finalized: Locked for editing (can still export)
- Paid: Marked as paid by user (informational only)

**Invoice Modification:**
- Regenerate: Re-query time entries for date range, update line items
- Add entries: Expand date range or add specific entries
- Remove entries: Remove line items (entries become unbilled again)
- Only allowed for Draft status invoices

**CSV Export:**
- Columns: Date, Description, Hours, Rate, Amount
- Include header row with invoice metadata (number, project, client, date range, total)
- Download as file: `{project_name}-{invoice_number}.csv`

**Google Sheets Export:**
- If project has no linked spreadsheet:
  - Create new Google Sheets document named "{Project Name} - Invoices"
  - Store spreadsheet ID on project record
- Add new worksheet named with invoice number (e.g., "INV-2024-001")
- Worksheet contains:
  - Header section: Invoice number, project, client, date range, invoice date
  - Line items: Date, Description, Hours, Rate, Amount
  - Footer: Total hours, Total amount
- Re-export updates existing worksheet (preserves any client annotations on other cells)

**Invoice List View:**
- Show all invoices with: number, project, date range, total, status
- Filter by project, status, date range
- Sort by date, number, amount
- Quick actions: View, Export CSV, Export Sheets, Mark Paid

**Invoice Detail View:**
- Show invoice header information
- List all line items with totals
- Export buttons (CSV, Google Sheets)
- Status change buttons (Finalize, Mark Paid)
- Regenerate button (Draft only)

**UI Integration:**
- New "Invoices" link in navigation header
- Invoice generation accessible from:
  - Invoices page: "New Invoice" button
  - Project page: "Generate Invoice" action
- Visual indicator on time entries showing if they're included in an invoice

### Non-Functional Requirements

- Invoice generation should complete in under 2 seconds for typical months
- Google Sheets export should complete in under 5 seconds
- Invoice numbers must be unique per user (enforced by database)

### Edge Cases

- **No billable entries in range**: Show message, don't create empty invoice
- **All entries already invoiced**: Show message indicating no new entries
- **Project bill rate is null**: Use $0.00 rate (show warning)
- **Bill rate changed mid-period**: Use rate at time of invoice generation (stored on line item)
- **Time entry deleted after invoicing**: Keep line item, mark as orphaned
- **Project archived**: Can still view/export existing invoices, can't create new ones

## 6. Resolved Questions

1. **Invoice number format**: `{PROJECT}-{YEAR}-{SEQ}` (e.g., ALPHA-2024-001). Project prefix derived from project name, sequence per project per year.

2. **Partial invoicing**: Date range only. All unbilled entries in the range are included. Simpler UX, avoids tedious entry selection.

3. **Bill rate changes**: No recalculation. Line items snapshot the rate at invoice creation time. Regenerating uses the snapshotted rate, not current project rate.

4. **Google Sheets formatting**: Basic formatting - bold headers, currency format for amounts, appropriate column widths. No borders or alternating row colors.

5. **Undo finalize**: Yes, users can revert a finalized invoice back to draft for corrections.

## 7. Future Considerations

- Invoice templates with customizable header/footer text
- Recurring invoice generation (auto-generate monthly)
- Invoice reminders and follow-up tracking
- Integration with accounting software (QuickBooks, Xero)
- PDF generation for formal client delivery
- Multi-currency support with exchange rates
