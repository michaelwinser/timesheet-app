"""Google Sheets export service for invoices."""

from datetime import datetime, timezone
from google.oauth2.credentials import Credentials
from googleapiclient.discovery import build

from db import get_db
from services.invoice import Invoice


def create_project_spreadsheet(
    credentials: Credentials,
    project_id: int,
    project_name: str,
    user_id: int,
) -> tuple[str, str]:
    """Create a new spreadsheet for a project.

    Args:
        credentials: Google OAuth credentials
        project_id: Project ID
        project_name: Project name for spreadsheet title
        user_id: User ID

    Returns:
        Tuple of (spreadsheet_id, spreadsheet_url)

    Raises:
        ValueError: If project already has spreadsheet attached
    """
    db = get_db()

    # Check if project already has a spreadsheet
    project = db.execute_one(
        "SELECT sheets_spreadsheet_id FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id),
    )

    if project and project["sheets_spreadsheet_id"]:
        raise ValueError("Project already has a spreadsheet attached")

    # Create new spreadsheet
    service = build("sheets", "v4", credentials=credentials)

    spreadsheet = {
        "properties": {
            "title": f"{project_name} - Invoices",
        },
        "sheets": [
            {
                "properties": {
                    "title": "Summary",
                    "index": 0,
                }
            }
        ],
    }

    result = service.spreadsheets().create(body=spreadsheet).execute()
    spreadsheet_id = result["spreadsheetId"]
    spreadsheet_url = result["spreadsheetUrl"]

    # Get the actual sheet ID from the response
    summary_sheet_id = result["sheets"][0]["properties"]["sheetId"]

    # Store spreadsheet info in project
    db.execute(
        """
        UPDATE projects
        SET sheets_spreadsheet_id = %s, sheets_spreadsheet_url = %s
        WHERE id = %s AND user_id = %s
        """,
        (spreadsheet_id, spreadsheet_url, project_id, user_id),
    )

    # Initialize Summary sheet with headers
    _init_summary_sheet(service, spreadsheet_id, summary_sheet_id)

    return spreadsheet_id, spreadsheet_url


def get_or_create_spreadsheet(
    credentials: Credentials,
    project_id: int,
    project_name: str,
    user_id: int,
) -> tuple[str, str]:
    """Get existing spreadsheet for project or create a new one.

    Args:
        credentials: Google OAuth credentials
        project_id: Project ID
        project_name: Project name for spreadsheet title
        user_id: User ID

    Returns:
        Tuple of (spreadsheet_id, spreadsheet_url)
    """
    db = get_db()

    # Check if project already has a spreadsheet
    project = db.execute_one(
        "SELECT sheets_spreadsheet_id, sheets_spreadsheet_url FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id),
    )

    if project and project["sheets_spreadsheet_id"]:
        return project["sheets_spreadsheet_id"], project["sheets_spreadsheet_url"]

    # Create new spreadsheet
    return create_project_spreadsheet(credentials, project_id, project_name, user_id)


def archive_project_spreadsheet(
    credentials: Credentials,
    project_id: int,
    archive_name: str,
    user_id: int,
) -> str:
    """Rename and detach the project's spreadsheet.

    Args:
        credentials: Google OAuth credentials
        project_id: Project ID
        archive_name: New name for the archived spreadsheet
        user_id: User ID

    Returns:
        URL of archived spreadsheet

    Raises:
        ValueError: If no spreadsheet attached
    """
    db = get_db()

    # Get current spreadsheet
    project = db.execute_one(
        "SELECT sheets_spreadsheet_id, sheets_spreadsheet_url FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id),
    )

    if not project or not project["sheets_spreadsheet_id"]:
        raise ValueError("No spreadsheet attached to this project")

    spreadsheet_id = project["sheets_spreadsheet_id"]
    spreadsheet_url = project["sheets_spreadsheet_url"]

    # Rename the spreadsheet using Drive API
    drive = build("drive", "v3", credentials=credentials)
    drive.files().update(
        fileId=spreadsheet_id,
        body={"name": archive_name}
    ).execute()

    # Clear spreadsheet from project
    db.execute(
        """
        UPDATE projects
        SET sheets_spreadsheet_id = NULL, sheets_spreadsheet_url = NULL
        WHERE id = %s AND user_id = %s
        """,
        (project_id, user_id),
    )

    return spreadsheet_url


def check_spreadsheet_modified(
    credentials: Credentials,
    spreadsheet_id: str,
    since: datetime,
) -> tuple[bool, datetime | None]:
    """Check if spreadsheet was modified after given timestamp.

    Args:
        credentials: Google OAuth credentials
        spreadsheet_id: Google Sheets ID
        since: Timestamp to compare against

    Returns:
        Tuple of (was_modified, modified_time)
    """
    drive = build("drive", "v3", credentials=credentials)

    try:
        file = drive.files().get(
            fileId=spreadsheet_id,
            fields="modifiedTime"
        ).execute()

        modified_str = file["modifiedTime"]
        # Parse ISO format with Z suffix
        modified_time = datetime.fromisoformat(modified_str.replace("Z", "+00:00"))

        # Ensure since is timezone-aware for comparison
        if since.tzinfo is None:
            since = since.replace(tzinfo=timezone.utc)

        was_modified = modified_time > since
        return was_modified, modified_time

    except Exception:
        # If we can't check, assume not modified
        return False, None


def delete_invoice_worksheet(
    credentials: Credentials,
    spreadsheet_id: str,
    invoice_number: str,
) -> bool:
    """Delete an invoice worksheet from the spreadsheet.

    Args:
        credentials: Google OAuth credentials
        spreadsheet_id: Google Sheets ID
        invoice_number: Invoice number (worksheet title)

    Returns:
        True if deleted, False if worksheet not found
    """
    service = build("sheets", "v4", credentials=credentials)

    # Get spreadsheet to find sheet ID
    try:
        spreadsheet = service.spreadsheets().get(spreadsheetId=spreadsheet_id).execute()
    except Exception:
        return False

    sheets = spreadsheet.get("sheets", [])
    sheet_id = None

    for sheet in sheets:
        if sheet["properties"]["title"] == invoice_number:
            sheet_id = sheet["properties"]["sheetId"]
            break

    if sheet_id is None:
        return False

    # Delete the worksheet
    service.spreadsheets().batchUpdate(
        spreadsheetId=spreadsheet_id,
        body={
            "requests": [{
                "deleteSheet": {"sheetId": sheet_id}
            }]
        }
    ).execute()

    return True


def _init_summary_sheet(service, spreadsheet_id: str, sheet_id: int) -> None:
    """Initialize the Summary sheet with headers."""
    headers = [
        ["Invoice Number", "Period", "Total Hours", "Total Amount", "Status", "Exported"],
    ]

    service.spreadsheets().values().update(
        spreadsheetId=spreadsheet_id,
        range="Summary!A1:F1",
        valueInputOption="RAW",
        body={"values": headers},
    ).execute()

    # Format header row
    _format_header_row(service, spreadsheet_id, sheet_id)


def _format_header_row(service, spreadsheet_id: str, sheet_id: int) -> None:
    """Apply bold formatting to header row."""
    requests = [
        {
            "repeatCell": {
                "range": {
                    "sheetId": sheet_id,
                    "startRowIndex": 0,
                    "endRowIndex": 1,
                },
                "cell": {
                    "userEnteredFormat": {
                        "textFormat": {"bold": True},
                        "backgroundColor": {"red": 0.9, "green": 0.9, "blue": 0.9},
                    }
                },
                "fields": "userEnteredFormat(textFormat,backgroundColor)",
            }
        },
        {
            "updateSheetProperties": {
                "properties": {
                    "sheetId": sheet_id,
                    "gridProperties": {"frozenRowCount": 1},
                },
                "fields": "gridProperties.frozenRowCount",
            }
        },
    ]

    service.spreadsheets().batchUpdate(
        spreadsheetId=spreadsheet_id,
        body={"requests": requests},
    ).execute()


def export_invoice_to_sheets(
    credentials: Credentials,
    invoice: Invoice,
    user_id: int,
    force: bool = False,
) -> tuple[str, str] | dict:
    """Export an invoice to Google Sheets.

    Creates or updates a worksheet for the invoice within the project's spreadsheet.

    Args:
        credentials: Google OAuth credentials
        invoice: Invoice to export
        user_id: User ID
        force: If True, skip modification check

    Returns:
        Tuple of (spreadsheet_id, spreadsheet_url) on success
        Dict with warning info if modification detected and force=False
    """
    db = get_db()
    service = build("sheets", "v4", credentials=credentials)

    # Get or create spreadsheet for project
    spreadsheet_id, spreadsheet_url = get_or_create_spreadsheet(
        credentials, invoice.project_id, invoice.project_name, user_id
    )

    # Check if worksheet for this invoice already exists
    spreadsheet = service.spreadsheets().get(spreadsheetId=spreadsheet_id).execute()
    sheets = spreadsheet.get("sheets", [])

    sheet_title = invoice.invoice_number
    existing_sheet_id = None

    for sheet in sheets:
        if sheet["properties"]["title"] == sheet_title:
            existing_sheet_id = sheet["properties"]["sheetId"]
            break

    # Check for modifications if updating existing sheet and not forcing
    if existing_sheet_id is not None and not force:
        # Get last export time from invoice
        inv_record = db.execute_one(
            "SELECT last_exported_at FROM invoices WHERE id = %s",
            (invoice.id,)
        )
        if inv_record and inv_record["last_exported_at"]:
            was_modified, modified_time = check_spreadsheet_modified(
                credentials, spreadsheet_id, inv_record["last_exported_at"]
            )
            if was_modified:
                return {
                    "warning": "spreadsheet_modified",
                    "last_exported_at": inv_record["last_exported_at"].isoformat(),
                    "spreadsheet_modified_at": modified_time.isoformat() if modified_time else None,
                    "message": "Spreadsheet was modified since last export"
                }

    if existing_sheet_id is None:
        # Create new worksheet
        add_sheet_request = {
            "addSheet": {
                "properties": {
                    "title": sheet_title,
                    "index": 1,  # After Summary
                }
            }
        }

        response = service.spreadsheets().batchUpdate(
            spreadsheetId=spreadsheet_id,
            body={"requests": [add_sheet_request]},
        ).execute()

        existing_sheet_id = response["replies"][0]["addSheet"]["properties"]["sheetId"]

    # Prepare invoice data - RAW DATA ONLY (no metadata, no totals)
    invoice_data = [
        ["Date", "Description", "Hours", "Rate", "Amount"],
    ]

    # Add line items
    for item in invoice.line_items:
        invoice_data.append([
            str(item.entry_date),
            item.description or "-",
            float(item.hours),
            float(item.rate),
            float(item.amount),
        ])

    # Clear existing data and write new data
    service.spreadsheets().values().clear(
        spreadsheetId=spreadsheet_id,
        range=f"'{sheet_title}'!A:Z",
    ).execute()

    service.spreadsheets().values().update(
        spreadsheetId=spreadsheet_id,
        range=f"'{sheet_title}'!A1",
        valueInputOption="RAW",
        body={"values": invoice_data},
    ).execute()

    # Format the worksheet
    _format_invoice_sheet(service, spreadsheet_id, existing_sheet_id, len(invoice_data))

    # Update Summary sheet
    _update_summary_sheet(service, spreadsheet_id, invoice)

    # Update invoice with export info
    db.execute(
        """
        UPDATE invoices
        SET sheets_spreadsheet_id = %s, last_exported_at = CURRENT_TIMESTAMP
        WHERE id = %s
        """,
        (spreadsheet_id, invoice.id),
    )

    return spreadsheet_id, spreadsheet_url


def _format_invoice_sheet(
    service, spreadsheet_id: str, sheet_id: int, row_count: int
) -> None:
    """Apply formatting to invoice worksheet."""
    requests = [
        # Bold header row (row 1, 0-indexed row 0)
        {
            "repeatCell": {
                "range": {
                    "sheetId": sheet_id,
                    "startRowIndex": 0,
                    "endRowIndex": 1,
                },
                "cell": {
                    "userEnteredFormat": {
                        "textFormat": {"bold": True},
                        "backgroundColor": {"red": 0.9, "green": 0.9, "blue": 0.9},
                    }
                },
                "fields": "userEnteredFormat(textFormat,backgroundColor)",
            }
        },
        # Freeze header row
        {
            "updateSheetProperties": {
                "properties": {
                    "sheetId": sheet_id,
                    "gridProperties": {"frozenRowCount": 1},
                },
                "fields": "gridProperties.frozenRowCount",
            }
        },
        # Currency format for Amount column (E)
        {
            "repeatCell": {
                "range": {
                    "sheetId": sheet_id,
                    "startRowIndex": 1,
                    "endRowIndex": row_count,
                    "startColumnIndex": 4,
                    "endColumnIndex": 5,
                },
                "cell": {
                    "userEnteredFormat": {
                        "numberFormat": {
                            "type": "CURRENCY",
                            "pattern": "$#,##0.00",
                        }
                    }
                },
                "fields": "userEnteredFormat(numberFormat)",
            }
        },
        # Currency format for Rate column (D)
        {
            "repeatCell": {
                "range": {
                    "sheetId": sheet_id,
                    "startRowIndex": 1,
                    "endRowIndex": row_count,
                    "startColumnIndex": 3,
                    "endColumnIndex": 4,
                },
                "cell": {
                    "userEnteredFormat": {
                        "numberFormat": {
                            "type": "CURRENCY",
                            "pattern": "$#,##0.00",
                        }
                    }
                },
                "fields": "userEnteredFormat(numberFormat)",
            }
        },
        # Auto-resize columns
        {
            "autoResizeDimensions": {
                "dimensions": {
                    "sheetId": sheet_id,
                    "dimension": "COLUMNS",
                    "startIndex": 0,
                    "endIndex": 5,
                }
            }
        },
    ]

    service.spreadsheets().batchUpdate(
        spreadsheetId=spreadsheet_id,
        body={"requests": requests},
    ).execute()


def _update_summary_sheet(service, spreadsheet_id: str, invoice: Invoice) -> None:
    """Update Summary sheet with invoice info."""
    # Get current Summary data
    result = service.spreadsheets().values().get(
        spreadsheetId=spreadsheet_id,
        range="Summary!A:F",
    ).execute()

    values = result.get("values", [])

    # Find row for this invoice or add new row
    row_index = None
    for i, row in enumerate(values):
        if row and row[0] == invoice.invoice_number:
            row_index = i
            break

    now = datetime.now().strftime("%Y-%m-%d %H:%M")
    invoice_row = [
        invoice.invoice_number,
        f"{invoice.period_start} - {invoice.period_end}",
        float(invoice.total_hours),
        float(invoice.total_amount),
        invoice.status.upper(),
        now,
    ]

    if row_index is not None:
        # Update existing row
        range_name = f"Summary!A{row_index + 1}:F{row_index + 1}"
    else:
        # Append new row
        row_index = len(values)
        range_name = f"Summary!A{row_index + 1}:F{row_index + 1}"

    service.spreadsheets().values().update(
        spreadsheetId=spreadsheet_id,
        range=range_name,
        valueInputOption="RAW",
        body={"values": [invoice_row]},
    ).execute()


def remove_invoice_from_summary(
    credentials: Credentials,
    spreadsheet_id: str,
    invoice_number: str,
) -> bool:
    """Remove an invoice row from the Summary sheet.

    Args:
        credentials: Google OAuth credentials
        spreadsheet_id: Google Sheets ID
        invoice_number: Invoice number to remove

    Returns:
        True if removed, False if not found
    """
    service = build("sheets", "v4", credentials=credentials)

    # Get current Summary data
    result = service.spreadsheets().values().get(
        spreadsheetId=spreadsheet_id,
        range="Summary!A:F",
    ).execute()

    values = result.get("values", [])

    # Find row for this invoice
    row_index = None
    for i, row in enumerate(values):
        if row and row[0] == invoice_number:
            row_index = i
            break

    if row_index is None or row_index == 0:  # Don't delete header
        return False

    # Get Summary sheet ID
    spreadsheet = service.spreadsheets().get(spreadsheetId=spreadsheet_id).execute()
    summary_sheet_id = None
    for sheet in spreadsheet.get("sheets", []):
        if sheet["properties"]["title"] == "Summary":
            summary_sheet_id = sheet["properties"]["sheetId"]
            break

    if summary_sheet_id is None:
        return False

    # Delete the row
    service.spreadsheets().batchUpdate(
        spreadsheetId=spreadsheet_id,
        body={
            "requests": [{
                "deleteDimension": {
                    "range": {
                        "sheetId": summary_sheet_id,
                        "dimension": "ROWS",
                        "startIndex": row_index,
                        "endIndex": row_index + 1,
                    }
                }
            }]
        }
    ).execute()

    return True
