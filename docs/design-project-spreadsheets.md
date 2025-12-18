# Project Spreadsheets - Design Doc

## Overview

This document describes the technical design for enhanced project-spreadsheet integration, including explicit UI controls, archival workflow, and improved data formats.

## Database Changes

No schema changes required. Existing columns are sufficient:

```sql
-- Already exists on projects table
sheets_spreadsheet_id VARCHAR(255)  -- Google Sheets ID
sheets_spreadsheet_url TEXT          -- URL to spreadsheet

-- Already exists on invoices table
sheets_spreadsheet_id VARCHAR(255)  -- Spreadsheet where invoice was exported
last_exported_at TIMESTAMP          -- Used for modification detection
```

## API Changes

### New Endpoints

#### POST /api/projects/{project_id}/spreadsheet
Create a new spreadsheet for the project.

**Request**: Empty body

**Response**:
```json
{
  "spreadsheet_id": "1abc...",
  "spreadsheet_url": "https://docs.google.com/spreadsheets/d/1abc...",
  "name": "Alpha Omega - Invoices"
}
```

**Errors**:
- 400: Project already has spreadsheet attached
- 400: Project is not billable
- 404: Project not found

#### POST /api/projects/{project_id}/spreadsheet/archive
Archive (rename and detach) the current spreadsheet.

**Request**:
```json
{
  "archive_name": "Alpha Omega - Invoices 2024"
}
```

**Response**:
```json
{
  "archived_spreadsheet_url": "https://docs.google.com/spreadsheets/d/1abc...",
  "message": "Spreadsheet archived as 'Alpha Omega - Invoices 2024'"
}
```

**Errors**:
- 400: No spreadsheet attached
- 404: Project not found

### Modified Endpoints

#### POST /api/invoices/{invoice_id}/export/sheets
Add optional `force` parameter to bypass modification warning.

**Request** (query param):
```
?force=true
```

**Response** (when modification detected and force=false):
```json
{
  "warning": "spreadsheet_modified",
  "last_exported_at": "2025-01-15T10:30:00Z",
  "spreadsheet_modified_at": "2025-01-16T14:45:00Z",
  "message": "Spreadsheet was modified since last export"
}
```

HTTP status: 409 Conflict

Client should prompt user and retry with `?force=true` if confirmed.

## Service Layer

### sheets.py - New Functions

```python
def create_project_spreadsheet(
    credentials: Credentials,
    project_id: int,
    project_name: str,
    user_id: int
) -> tuple[str, str]:
    """Create a new spreadsheet for a project.

    Returns:
        Tuple of (spreadsheet_id, spreadsheet_url)

    Raises:
        ValueError: If project already has spreadsheet
    """

def archive_project_spreadsheet(
    credentials: Credentials,
    project_id: int,
    archive_name: str,
    user_id: int
) -> str:
    """Rename and detach the project's spreadsheet.

    Returns:
        URL of archived spreadsheet

    Raises:
        ValueError: If no spreadsheet attached
    """

def check_spreadsheet_modified(
    credentials: Credentials,
    spreadsheet_id: str,
    since: datetime
) -> tuple[bool, datetime | None]:
    """Check if spreadsheet was modified after given timestamp.

    Returns:
        Tuple of (was_modified, modified_time)
    """
```

### sheets.py - Modified Functions

#### export_invoice_to_sheets

Update to:
1. Check for modifications before overwriting (unless force=True)
2. Write raw data only (no totals row)
3. Update Summary with direct values (not formulas, per PRD recommendation)

```python
def export_invoice_to_sheets(
    credentials: Credentials,
    invoice: Invoice,
    user_id: int,
    force: bool = False
) -> tuple[str, str] | dict:
    """Export invoice to Google Sheets.

    Args:
        credentials: Google OAuth credentials
        invoice: Invoice to export
        user_id: User ID
        force: If True, skip modification check

    Returns:
        Tuple of (spreadsheet_id, spreadsheet_url) on success
        Dict with warning info if modification detected and force=False
    """
```

## Spreadsheet Structure

### Invoice Worksheet (Updated)

**Name**: Invoice number (e.g., "AOC-2025-001")

**Content** (raw data only):
```
Row 1: Headers - Date | Description | Hours | Rate | Amount
Row 2+: Line items
```

No metadata header rows, no totals row. Clean tabular data.

**Formatting**:
- Header row: Bold, gray background, frozen
- Amount/Rate columns: Currency format
- Auto-resize columns

### Summary Worksheet (Updated)

**Content**:
```
Row 1: Headers - Invoice Number | Period | Total Hours | Total Amount | Status | Exported
Row 2+: One row per invoice, values updated on each export
```

Values are written directly (not formulas) for simplicity and reliability.

## UI Changes

### Projects Page

Add spreadsheet section to each billable project row or detail view.

**Template changes** (`projects.html`):
- Add spreadsheet info display
- Add "Create Spreadsheet" button (if none)
- Add "Open" link and "Archive" button (if attached)
- Add archive modal

**JavaScript functions**:
```javascript
async function createSpreadsheet(projectId) { ... }
async function archiveSpreadsheet(projectId) { ... }
function showArchiveModal(projectId, currentName) { ... }
function hideArchiveModal() { ... }
```

### Invoices Page

Update export flow to handle modification warning.

**JavaScript changes**:
```javascript
async function exportSheets() {
    const result = await api.post(`/api/invoices/${id}/export/sheets`);

    if (result.warning === 'spreadsheet_modified') {
        if (confirm(`Spreadsheet modified since last export. Overwrite?`)) {
            await api.post(`/api/invoices/${id}/export/sheets?force=true`);
        }
        return;
    }
    // ... success handling
}
```

## Google Sheets API Usage

### Get Spreadsheet Modified Time

```python
from googleapiclient.discovery import build

def get_spreadsheet_modified_time(credentials, spreadsheet_id):
    drive = build("drive", "v3", credentials=credentials)
    file = drive.files().get(
        fileId=spreadsheet_id,
        fields="modifiedTime"
    ).execute()
    return datetime.fromisoformat(file["modifiedTime"].replace("Z", "+00:00"))
```

**Note**: Requires `drive.file` scope (already configured).

### Rename Spreadsheet

```python
def rename_spreadsheet(credentials, spreadsheet_id, new_name):
    drive = build("drive", "v3", credentials=credentials)
    drive.files().update(
        fileId=spreadsheet_id,
        body={"name": new_name}
    ).execute()
```

### Delete Worksheet

```python
def delete_worksheet(credentials, spreadsheet_id, sheet_id):
    sheets = build("sheets", "v4", credentials=credentials)
    sheets.spreadsheets().batchUpdate(
        spreadsheetId=spreadsheet_id,
        body={
            "requests": [{
                "deleteSheet": {"sheetId": sheet_id}
            }]
        }
    ).execute()
```

## Implementation Plan

### Phase 1: Data Format Improvements
1. Update `export_invoice_to_sheets` to write raw data only
2. Update Summary to use direct values
3. Test with new and existing spreadsheets

### Phase 2: Modification Detection
1. Add `check_spreadsheet_modified` function
2. Update export endpoint to return warning
3. Update invoices UI to handle warning

### Phase 3: Projects UI
1. Add spreadsheet section to projects page
2. Implement "Create Spreadsheet" functionality
3. Implement "Archive" with modal

### Phase 4: Invoice Delete Cleanup
1. On invoice delete, also delete worksheet from spreadsheet
2. Handle case where spreadsheet doesn't exist or worksheet missing

## Testing Considerations

1. **New spreadsheet**: Create, verify structure
2. **Export to existing**: Verify worksheet created/updated correctly
3. **Modification detection**: Manually edit sheet, verify warning appears
4. **Archive flow**: Verify rename works, project detached, new export creates fresh sheet
5. **Delete invoice**: Verify worksheet removed
6. **Edge cases**:
   - Project without spreadsheet
   - Spreadsheet deleted from Drive
   - Network errors during Sheets API calls

## Security Considerations

- All operations require authenticated user
- Project ownership verified before spreadsheet operations
- Using `drive.file` scope limits access to app-created files only
- No sensitive data stored in spreadsheets beyond invoice details
