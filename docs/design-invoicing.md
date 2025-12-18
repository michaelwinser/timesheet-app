# Design Doc: Invoicing

> **Status**: Draft
> **PRD**: `docs/prd-invoicing.md`
> **Target**: Merge into main design doc after implementation complete

## 1. Overview

Add invoicing capability to transform billable time entries into invoice records that can be exported to CSV or Google Sheets. Key design goals:

- Minimal schema additions that integrate cleanly with existing models
- Reuse existing Google OAuth for Sheets access (scope expansion)
- "Living document" model for Google Sheets (one spreadsheet per project)
- Clear audit trail from invoice → line items → time entries

## 2. Data Model

### New Tables

```sql
-- =============================================================================
-- INVOICES - Invoice header records
-- =============================================================================
CREATE TABLE IF NOT EXISTS invoices (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,

    -- Invoice identification
    invoice_number VARCHAR(50) NOT NULL,

    -- Date range covered
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,

    -- Invoice metadata
    invoice_date DATE NOT NULL DEFAULT CURRENT_DATE,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',  -- draft, finalized, paid

    -- Calculated totals (denormalized for quick access)
    total_hours REAL NOT NULL DEFAULT 0,
    total_amount DECIMAL(10, 2) NOT NULL DEFAULT 0,

    -- Google Sheets export tracking
    sheets_spreadsheet_id VARCHAR(255),  -- Inherited from project or created
    sheets_worksheet_id INTEGER,          -- Worksheet (sheet) ID within spreadsheet
    last_exported_at TIMESTAMP,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- Invoice numbers unique per user
    UNIQUE(user_id, invoice_number)
);

CREATE INDEX IF NOT EXISTS idx_invoices_user ON invoices(user_id);
CREATE INDEX IF NOT EXISTS idx_invoices_project ON invoices(user_id, project_id);
CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(user_id, status);
CREATE INDEX IF NOT EXISTS idx_invoices_date ON invoices(user_id, invoice_date DESC);

-- =============================================================================
-- INVOICE_LINE_ITEMS - Individual entries on an invoice
-- =============================================================================
CREATE TABLE IF NOT EXISTS invoice_line_items (
    id SERIAL PRIMARY KEY,
    invoice_id INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    time_entry_id INTEGER REFERENCES time_entries(id) ON DELETE SET NULL,

    -- Snapshot of time entry data at invoice time
    entry_date DATE NOT NULL,
    description TEXT,
    hours REAL NOT NULL,
    rate DECIMAL(10, 2) NOT NULL,  -- Bill rate at time of invoicing
    amount DECIMAL(10, 2) NOT NULL,  -- hours * rate

    -- Track if source entry was deleted
    is_orphaned BOOLEAN DEFAULT FALSE,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_invoice_line_items_invoice ON invoice_line_items(invoice_id);
CREATE INDEX IF NOT EXISTS idx_invoice_line_items_entry ON invoice_line_items(time_entry_id);
```

### Schema Changes to Existing Tables

```sql
-- Add Google Sheets spreadsheet link to projects
ALTER TABLE projects
ADD COLUMN IF NOT EXISTS sheets_spreadsheet_id VARCHAR(255),
ADD COLUMN IF NOT EXISTS sheets_spreadsheet_url TEXT;

COMMENT ON COLUMN projects.sheets_spreadsheet_id IS 'Google Sheets ID for invoice exports';
COMMENT ON COLUMN projects.sheets_spreadsheet_url IS 'URL to the Google Sheets document';

-- Add invoice reference to time_entries for tracking
ALTER TABLE time_entries
ADD COLUMN IF NOT EXISTS invoice_id INTEGER REFERENCES invoices(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_time_entries_invoice ON time_entries(invoice_id);

COMMENT ON COLUMN time_entries.invoice_id IS 'Invoice this entry is included in (NULL = unbilled)';
```

### Entity Relationships

```
users
  └── projects (1:N)
        ├── sheets_spreadsheet_id (optional link to Google Sheets)
        └── invoices (1:N)
              └── invoice_line_items (1:N)
                    └── time_entry (1:1, nullable if orphaned)

time_entries
  └── invoice_id (N:1 to invoices, nullable)
```

### Invoice Number Generation

Format: `{PROJECT}-{YEAR}-{SEQ}` (e.g., ALPHA-2024-001)

Sequential per project per year, using a short prefix derived from project name:

```python
def generate_invoice_number(project_id: int, project_name: str, db) -> str:
    """Generate next invoice number for project."""
    year = datetime.now().year

    # Generate project prefix (first word, uppercase, max 10 chars)
    prefix = project_name.split()[0].upper()[:10]

    # Get highest sequence number for this project this year
    result = db.execute_one(
        """
        SELECT invoice_number FROM invoices
        WHERE project_id = %s AND invoice_number LIKE %s
        ORDER BY invoice_number DESC LIMIT 1
        """,
        (project_id, f"{prefix}-{year}-%")
    )

    if result:
        # Extract sequence and increment
        current_seq = int(result["invoice_number"].split("-")[-1])
        next_seq = current_seq + 1
    else:
        next_seq = 1

    return f"{prefix}-{year}-{next_seq:03d}"
```

## 3. API Design

### Invoice Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/invoices` | GET | List invoices (with filters) |
| `/api/invoices` | POST | Create new invoice |
| `/api/invoices/{id}` | GET | Get invoice details with line items |
| `/api/invoices/{id}` | PUT | Update invoice (draft only) |
| `/api/invoices/{id}` | DELETE | Delete invoice (draft only) |
| `/api/invoices/{id}/regenerate` | POST | Regenerate line items from time entries |
| `/api/invoices/{id}/finalize` | POST | Change status to finalized |
| `/api/invoices/{id}/mark-paid` | POST | Change status to paid |
| `/api/invoices/{id}/export/csv` | GET | Download as CSV |
| `/api/invoices/{id}/export/sheets` | POST | Export to Google Sheets |

### Request/Response Examples

**Create Invoice:**
```json
POST /api/invoices
{
    "project_id": 42,
    "period_start": "2024-12-01",
    "period_end": "2024-12-31",
    "invoice_date": "2024-12-31"  // optional, defaults to today
}

Response:
{
    "id": 1,
    "invoice_number": "INV-2024-001",
    "project_id": 42,
    "project_name": "Alpha Omega",
    "client": "Linux Foundation",
    "period_start": "2024-12-01",
    "period_end": "2024-12-31",
    "invoice_date": "2024-12-31",
    "status": "draft",
    "total_hours": 42.5,
    "total_amount": 6375.00,
    "line_items_count": 15,
    "created_at": "2024-12-31T10:00:00Z"
}
```

**Get Invoice Details:**
```json
GET /api/invoices/1

Response:
{
    "id": 1,
    "invoice_number": "INV-2024-001",
    "project": {
        "id": 42,
        "name": "Alpha Omega",
        "client": "Linux Foundation",
        "bill_rate": 150.00
    },
    "period_start": "2024-12-01",
    "period_end": "2024-12-31",
    "invoice_date": "2024-12-31",
    "status": "draft",
    "total_hours": 42.5,
    "total_amount": 6375.00,
    "line_items": [
        {
            "id": 1,
            "time_entry_id": 101,
            "entry_date": "2024-12-02",
            "description": "Weekly standup",
            "hours": 0.5,
            "rate": 150.00,
            "amount": 75.00,
            "is_orphaned": false
        },
        // ... more items
    ],
    "sheets_spreadsheet_url": null,
    "last_exported_at": null
}
```

**List Invoices:**
```json
GET /api/invoices?project_id=42&status=draft

Response:
{
    "invoices": [
        {
            "id": 1,
            "invoice_number": "INV-2024-001",
            "project_name": "Alpha Omega",
            "client": "Linux Foundation",
            "period_start": "2024-12-01",
            "period_end": "2024-12-31",
            "invoice_date": "2024-12-31",
            "status": "draft",
            "total_hours": 42.5,
            "total_amount": 6375.00
        }
    ],
    "total": 1
}
```

## 4. Service Layer

### InvoiceService

```python
# src/services/invoice.py

class InvoiceService:
    """Service for invoice operations."""

    def __init__(self, db, user_id: int):
        self.db = db
        self.user_id = user_id

    def create_invoice(
        self,
        project_id: int,
        period_start: date,
        period_end: date,
        invoice_date: date | None = None
    ) -> Invoice:
        """Create invoice from unbilled time entries."""

        # Validate project is billable
        project = self._get_project(project_id)
        if not project["is_billable"]:
            raise ValueError("Cannot invoice non-billable project")

        # Get unbilled time entries in range
        entries = self._get_unbilled_entries(project_id, period_start, period_end)
        if not entries:
            raise ValueError("No unbilled entries in date range")

        # Generate invoice number
        invoice_number = generate_invoice_number(self.user_id, self.db)

        # Calculate totals
        bill_rate = project["bill_rate"] or Decimal("0.00")
        total_hours = sum(e["hours"] for e in entries)
        total_amount = sum(Decimal(str(e["hours"])) * bill_rate for e in entries)

        # Create invoice record
        invoice_id = self.db.execute_insert(
            """
            INSERT INTO invoices
            (user_id, project_id, invoice_number, period_start, period_end,
             invoice_date, total_hours, total_amount)
            VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
            RETURNING id
            """,
            (self.user_id, project_id, invoice_number, period_start, period_end,
             invoice_date or date.today(), total_hours, total_amount)
        )

        # Create line items and mark entries as invoiced
        for entry in entries:
            amount = Decimal(str(entry["hours"])) * bill_rate
            self.db.execute_insert(
                """
                INSERT INTO invoice_line_items
                (invoice_id, time_entry_id, entry_date, description, hours, rate, amount)
                VALUES (%s, %s, %s, %s, %s, %s, %s)
                """,
                (invoice_id, entry["id"], entry["entry_date"],
                 entry["description"], entry["hours"], bill_rate, amount)
            )

            # Mark time entry as invoiced
            self.db.execute(
                "UPDATE time_entries SET invoice_id = %s WHERE id = %s",
                (invoice_id, entry["id"])
            )

        return self.get_invoice(invoice_id)

    def regenerate_invoice(self, invoice_id: int) -> Invoice:
        """Regenerate invoice line items from current time entries."""
        invoice = self._get_invoice_for_update(invoice_id)

        if invoice["status"] != "draft":
            raise ValueError("Can only regenerate draft invoices")

        # Clear existing line items and unmark entries
        self.db.execute(
            "UPDATE time_entries SET invoice_id = NULL WHERE invoice_id = %s",
            (invoice_id,)
        )
        self.db.execute(
            "DELETE FROM invoice_line_items WHERE invoice_id = %s",
            (invoice_id,)
        )

        # Recreate from current unbilled entries
        # ... (similar logic to create_invoice)

    def export_csv(self, invoice_id: int) -> str:
        """Generate CSV content for invoice."""
        invoice = self.get_invoice(invoice_id)

        output = io.StringIO()
        writer = csv.writer(output)

        # Header metadata
        writer.writerow(["Invoice", invoice["invoice_number"]])
        writer.writerow(["Project", invoice["project"]["name"]])
        writer.writerow(["Client", invoice["project"]["client"] or ""])
        writer.writerow(["Period", f"{invoice['period_start']} to {invoice['period_end']}"])
        writer.writerow(["Invoice Date", invoice["invoice_date"]])
        writer.writerow([])

        # Line items header
        writer.writerow(["Date", "Description", "Hours", "Rate", "Amount"])

        # Line items
        for item in invoice["line_items"]:
            writer.writerow([
                item["entry_date"],
                item["description"],
                item["hours"],
                f"${item['rate']:.2f}",
                f"${item['amount']:.2f}"
            ])

        # Totals
        writer.writerow([])
        writer.writerow(["", "", invoice["total_hours"], "", f"${invoice['total_amount']:.2f}"])

        return output.getvalue()
```

## 5. Google Sheets Integration

### OAuth Scope Expansion

Current scopes:
- `https://www.googleapis.com/auth/calendar.readonly`

Additional scope needed:
- `https://www.googleapis.com/auth/spreadsheets`

Users will need to re-authorize when first using Sheets export.

### SheetsService

```python
# src/services/sheets.py

from googleapiclient.discovery import build
from google.oauth2.credentials import Credentials

class SheetsService:
    """Service for Google Sheets operations."""

    def __init__(self, credentials: Credentials):
        self.service = build('sheets', 'v4', credentials=credentials)

    def create_spreadsheet(self, title: str) -> tuple[str, str]:
        """Create new spreadsheet, return (spreadsheet_id, url)."""
        spreadsheet = self.service.spreadsheets().create(
            body={"properties": {"title": title}}
        ).execute()

        return (
            spreadsheet["spreadsheetId"],
            spreadsheet["spreadsheetUrl"]
        )

    def add_invoice_worksheet(
        self,
        spreadsheet_id: str,
        invoice: dict
    ) -> int:
        """Add worksheet for invoice, return worksheet ID."""

        # Create new sheet
        request = {
            "addSheet": {
                "properties": {
                    "title": invoice["invoice_number"]
                }
            }
        }
        response = self.service.spreadsheets().batchUpdate(
            spreadsheetId=spreadsheet_id,
            body={"requests": [request]}
        ).execute()

        sheet_id = response["replies"][0]["addSheet"]["properties"]["sheetId"]

        # Populate with invoice data
        self._write_invoice_data(spreadsheet_id, invoice["invoice_number"], invoice)

        return sheet_id

    def update_invoice_worksheet(
        self,
        spreadsheet_id: str,
        worksheet_name: str,
        invoice: dict
    ):
        """Update existing worksheet with invoice data."""
        # Clear existing content
        self.service.spreadsheets().values().clear(
            spreadsheetId=spreadsheet_id,
            range=f"'{worksheet_name}'!A:F"
        ).execute()

        # Write updated data
        self._write_invoice_data(spreadsheet_id, worksheet_name, invoice)

    def _write_invoice_data(
        self,
        spreadsheet_id: str,
        worksheet_name: str,
        invoice: dict
    ):
        """Write invoice data to worksheet."""
        rows = [
            ["Invoice:", invoice["invoice_number"]],
            ["Project:", invoice["project"]["name"]],
            ["Client:", invoice["project"]["client"] or ""],
            ["Period:", f"{invoice['period_start']} to {invoice['period_end']}"],
            ["Invoice Date:", str(invoice["invoice_date"])],
            [],
            ["Date", "Description", "Hours", "Rate", "Amount"],
        ]

        for item in invoice["line_items"]:
            rows.append([
                str(item["entry_date"]),
                item["description"] or "",
                item["hours"],
                item["rate"],
                item["amount"]
            ])

        rows.append([])
        rows.append(["", "TOTAL", invoice["total_hours"], "", invoice["total_amount"]])

        self.service.spreadsheets().values().update(
            spreadsheetId=spreadsheet_id,
            range=f"'{worksheet_name}'!A1",
            valueInputOption="USER_ENTERED",
            body={"values": rows}
        ).execute()
```

### Export Flow

```python
def export_to_sheets(self, invoice_id: int) -> str:
    """Export invoice to Google Sheets, return spreadsheet URL."""
    invoice = self.get_invoice(invoice_id)
    project = invoice["project"]

    # Get user's OAuth credentials
    credentials = self._get_user_credentials()
    sheets = SheetsService(credentials)

    # Check if project has linked spreadsheet
    if project["sheets_spreadsheet_id"]:
        spreadsheet_id = project["sheets_spreadsheet_id"]

        # Check if worksheet exists for this invoice
        if invoice["sheets_worksheet_id"]:
            # Update existing worksheet
            sheets.update_invoice_worksheet(
                spreadsheet_id,
                invoice["invoice_number"],
                invoice
            )
        else:
            # Add new worksheet
            worksheet_id = sheets.add_invoice_worksheet(spreadsheet_id, invoice)
            self._update_invoice_sheets_ids(invoice_id, spreadsheet_id, worksheet_id)
    else:
        # Create new spreadsheet for project
        title = f"{project['name']} - Invoices"
        spreadsheet_id, spreadsheet_url = sheets.create_spreadsheet(title)

        # Link to project
        self._update_project_spreadsheet(project["id"], spreadsheet_id, spreadsheet_url)

        # Add invoice worksheet
        worksheet_id = sheets.add_invoice_worksheet(spreadsheet_id, invoice)
        self._update_invoice_sheets_ids(invoice_id, spreadsheet_id, worksheet_id)

    # Update last exported timestamp
    self.db.execute(
        "UPDATE invoices SET last_exported_at = NOW() WHERE id = %s",
        (invoice_id,)
    )

    return project["sheets_spreadsheet_url"] or \
           f"https://docs.google.com/spreadsheets/d/{spreadsheet_id}"
```

## 6. UI Design

### Navigation

Add "Invoices" link to main navigation header (after "Rules").

### Invoice List Page (`/invoices`)

```
┌─────────────────────────────────────────────────────────────────┐
│  Invoices                                    [+ New Invoice]    │
├─────────────────────────────────────────────────────────────────┤
│  Filters: [Project ▼] [Status ▼] [Date Range: ___ to ___]      │
├─────────────────────────────────────────────────────────────────┤
│  Invoice      │ Project      │ Period        │ Amount  │ Status │
├───────────────┼──────────────┼───────────────┼─────────┼────────┤
│  INV-2024-003 │ Alpha Omega  │ Dec 1-31      │ $6,375  │ Draft  │
│  INV-2024-002 │ Beta Project │ Dec 1-31      │ $2,100  │ Paid   │
│  INV-2024-001 │ Alpha Omega  │ Nov 1-30      │ $5,250  │ Paid   │
└───────────────┴──────────────┴───────────────┴─────────┴────────┘
```

### New Invoice Modal

```
┌─────────────────────────────────────────────┐
│  Create Invoice                         [X] │
├─────────────────────────────────────────────┤
│                                             │
│  Project:  [Alpha Omega          ▼]         │
│                                             │
│  Period:   [Dec 1, 2024] to [Dec 31, 2024]  │
│                                             │
│  Invoice Date: [Dec 31, 2024]               │
│                                             │
│  Preview:                                   │
│    15 unbilled entries                      │
│    42.5 hours @ $150.00/hr                  │
│    Total: $6,375.00                         │
│                                             │
│            [Cancel]  [Create Invoice]       │
└─────────────────────────────────────────────┘
```

### Invoice Detail Page (`/invoices/{id}`)

```
┌─────────────────────────────────────────────────────────────────┐
│  INV-2024-003                                                   │
│  Alpha Omega • Linux Foundation                                 │
│  Period: Dec 1-31, 2024 • Invoice Date: Dec 31, 2024           │
├─────────────────────────────────────────────────────────────────┤
│  Status: [Draft ▼]     [Regenerate] [Export CSV] [Export Sheets]│
├─────────────────────────────────────────────────────────────────┤
│  Date       │ Description              │ Hours │  Rate  │ Amount│
├─────────────┼──────────────────────────┼───────┼────────┼───────┤
│  Dec 2      │ Weekly standup           │  0.50 │ $150   │ $75   │
│  Dec 2      │ Code review session      │  2.00 │ $150   │ $300  │
│  Dec 3      │ Feature implementation   │  4.00 │ $150   │ $600  │
│  ...        │ ...                      │  ...  │ ...    │ ...   │
├─────────────┼──────────────────────────┼───────┼────────┼───────┤
│             │                    TOTAL │ 42.50 │        │$6,375 │
└─────────────┴──────────────────────────┴───────┴────────┴───────┘
```

### Time Entry Invoice Indicator

On classified time entries in week view, show small invoice icon if entry is part of an invoice:

```
┌──────────────────────┐
│ [Project Alpha ▼] 📋 │  ← 📋 indicates "invoiced"
│ 1.0 hrs [+15m]       │
│ "Meeting with Jane"  │
│ [Flip]               │
└──────────────────────┘
```

Clicking the icon shows tooltip: "Included in INV-2024-003"

## 7. Implementation Plan

### Phase 1: Core Invoice Model
1. Add database migrations for invoices and invoice_line_items tables
2. Add invoice_id column to time_entries
3. Implement InvoiceService with create, get, list methods
4. Add API endpoints for CRUD operations
5. Basic invoice list and detail pages

### Phase 2: CSV Export
6. Implement CSV export in InvoiceService
7. Add export endpoint and download functionality
8. Add export button to invoice detail page

### Phase 3: Invoice Management
9. Implement regenerate functionality
10. Implement status changes (finalize, mark paid)
11. Add "New Invoice" modal with preview
12. Add invoice indicator to time entries

### Phase 4: Google Sheets Integration
13. Add Sheets OAuth scope to authorization flow
14. Implement SheetsService
15. Add sheets columns to projects and invoices tables
16. Implement export to Sheets endpoint
17. Add Sheets export button and link display

### Phase 5: Polish
18. Add filters to invoice list
19. Add invoice generation from project page
20. Handle edge cases (orphaned entries, archived projects)
21. Add MCP tools for invoice operations

## 8. MCP Integration

Add invoice tools to the MCP server:

| Tool | Description |
|------|-------------|
| `list_invoices` | List invoices with optional filters |
| `get_invoice` | Get invoice details with line items |
| `create_invoice` | Create invoice for project and date range |
| `get_unbilled_entries` | Preview unbilled entries for a project/period |

Example MCP prompts:
- "Create an invoice for Alpha Omega for December"
- "Show me all unpaid invoices"
- "What time entries haven't been invoiced yet?"
