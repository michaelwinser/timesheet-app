"""Invoice service for generating and managing invoices."""

import csv
import io
from dataclasses import dataclass
from datetime import date, datetime
from decimal import Decimal
from typing import Any

from db import get_db


@dataclass
class InvoiceLineItem:
    """A line item on an invoice."""
    id: int
    time_entry_id: int | None
    entry_date: date
    description: str | None
    hours: float
    rate: Decimal
    amount: Decimal
    is_orphaned: bool


@dataclass
class Invoice:
    """An invoice with its line items."""
    id: int
    user_id: int
    project_id: int
    project_name: str
    client: str | None
    invoice_number: str
    period_start: date
    period_end: date
    invoice_date: date
    status: str
    total_hours: float
    total_amount: Decimal
    sheets_spreadsheet_id: str | None
    sheets_spreadsheet_url: str | None
    sheets_worksheet_id: int | None
    last_exported_at: datetime | None
    created_at: datetime
    line_items: list[InvoiceLineItem] | None = None


def generate_invoice_number(
    project_id: int,
    project_name: str,
    user_id: int,
    short_code: str | None = None
) -> str:
    """Generate next invoice number for project.

    Format: {PREFIX}-{YEAR}-{SEQ}
    Example: ABC-2024-001 (with short_code)
    Example: PROJECTNAME-2024-001 (without short_code)

    Args:
        project_id: Project ID
        project_name: Project name (used for prefix if no short_code)
        user_id: User ID
        short_code: Optional short code (2-3 letters) to use as prefix

    Returns:
        Generated invoice number
    """
    db = get_db()
    year = datetime.now().year

    # Use short_code if provided, otherwise use full project name (uppercase, no spaces)
    if short_code:
        prefix = short_code.upper()
    else:
        prefix = project_name.upper().replace(" ", "")

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


def get_unbilled_entries(
    user_id: int,
    project_id: int,
    period_start: date,
    period_end: date
) -> list[dict]:
    """Get unbilled time entries for a project and date range.

    Args:
        user_id: User ID
        project_id: Project ID
        period_start: Start of period
        period_end: End of period

    Returns:
        List of unbilled time entries
    """
    db = get_db()

    rows = db.execute(
        """
        SELECT
            te.id,
            te.hours,
            te.description,
            e.start_time,
            DATE(e.start_time) as entry_date
        FROM time_entries te
        JOIN events e ON te.event_id = e.id
        WHERE te.user_id = %s
          AND te.project_id = %s
          AND te.invoice_id IS NULL
          AND DATE(e.start_time) >= %s
          AND DATE(e.start_time) <= %s
          AND COALESCE(e.did_not_attend, FALSE) = FALSE
        ORDER BY e.start_time
        """,
        (user_id, project_id, period_start, period_end)
    )

    return [dict(row) for row in rows]


def create_invoice(
    user_id: int,
    project_id: int,
    period_start: date,
    period_end: date,
    invoice_date: date | None = None
) -> Invoice:
    """Create an invoice from unbilled time entries.

    Args:
        user_id: User ID
        project_id: Project ID
        period_start: Start of billing period
        period_end: End of billing period
        invoice_date: Invoice date (defaults to today)

    Returns:
        Created invoice

    Raises:
        ValueError: If project not found, not billable, or no unbilled entries
    """
    db = get_db()

    # Get project details
    project = db.execute_one(
        """
        SELECT id, name, client, short_code, is_billable, bill_rate,
               sheets_spreadsheet_id, sheets_spreadsheet_url
        FROM projects
        WHERE id = %s AND user_id = %s
        """,
        (project_id, user_id)
    )

    if not project:
        raise ValueError("Project not found")

    if not project["is_billable"]:
        raise ValueError("Cannot invoice non-billable project")

    # Get unbilled entries
    entries = get_unbilled_entries(user_id, project_id, period_start, period_end)

    if not entries:
        raise ValueError("No unbilled entries in date range")

    # Generate invoice number
    invoice_number = generate_invoice_number(
        project_id, project["name"], user_id, project.get("short_code")
    )

    # Calculate totals
    bill_rate = Decimal(str(project["bill_rate"] or 0))
    total_hours = sum(e["hours"] for e in entries)
    total_amount = sum(Decimal(str(e["hours"])) * bill_rate for e in entries)

    # Create invoice record
    invoice_id = db.execute_insert(
        """
        INSERT INTO invoices
        (user_id, project_id, invoice_number, period_start, period_end,
         invoice_date, total_hours, total_amount, sheets_spreadsheet_id)
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
        RETURNING id
        """,
        (user_id, project_id, invoice_number, period_start, period_end,
         invoice_date or date.today(), total_hours, total_amount,
         project["sheets_spreadsheet_id"])
    )

    # Create line items and mark entries as invoiced
    for entry in entries:
        entry_date = entry["entry_date"]
        if isinstance(entry_date, str):
            entry_date = datetime.fromisoformat(entry_date).date()

        amount = Decimal(str(entry["hours"])) * bill_rate

        db.execute_insert(
            """
            INSERT INTO invoice_line_items
            (invoice_id, time_entry_id, entry_date, description, hours, rate, amount)
            VALUES (%s, %s, %s, %s, %s, %s, %s)
            """,
            (invoice_id, entry["id"], entry_date,
             entry["description"], entry["hours"], bill_rate, amount)
        )

        # Mark time entry as invoiced
        db.execute(
            "UPDATE time_entries SET invoice_id = %s WHERE id = %s",
            (invoice_id, entry["id"])
        )

    return get_invoice(user_id, invoice_id)


def get_invoice(user_id: int, invoice_id: int, include_line_items: bool = True) -> Invoice | None:
    """Get an invoice by ID.

    Args:
        user_id: User ID
        invoice_id: Invoice ID
        include_line_items: Whether to include line items

    Returns:
        Invoice or None if not found
    """
    db = get_db()

    row = db.execute_one(
        """
        SELECT
            i.id,
            i.user_id,
            i.project_id,
            p.name as project_name,
            p.client,
            i.invoice_number,
            i.period_start,
            i.period_end,
            i.invoice_date,
            i.status,
            i.total_hours,
            i.total_amount,
            i.sheets_spreadsheet_id,
            p.sheets_spreadsheet_url,
            i.sheets_worksheet_id,
            i.last_exported_at,
            i.created_at
        FROM invoices i
        JOIN projects p ON i.project_id = p.id
        WHERE i.id = %s AND i.user_id = %s
        """,
        (invoice_id, user_id)
    )

    if not row:
        return None

    invoice = Invoice(
        id=row["id"],
        user_id=row["user_id"],
        project_id=row["project_id"],
        project_name=row["project_name"],
        client=row["client"],
        invoice_number=row["invoice_number"],
        period_start=row["period_start"],
        period_end=row["period_end"],
        invoice_date=row["invoice_date"],
        status=row["status"],
        total_hours=row["total_hours"],
        total_amount=Decimal(str(row["total_amount"])),
        sheets_spreadsheet_id=row["sheets_spreadsheet_id"],
        sheets_spreadsheet_url=row["sheets_spreadsheet_url"],
        sheets_worksheet_id=row["sheets_worksheet_id"],
        last_exported_at=row["last_exported_at"],
        created_at=row["created_at"],
        line_items=None
    )

    if include_line_items:
        invoice.line_items = get_invoice_line_items(invoice_id)

    return invoice


def get_invoice_line_items(invoice_id: int) -> list[InvoiceLineItem]:
    """Get line items for an invoice.

    Args:
        invoice_id: Invoice ID

    Returns:
        List of line items
    """
    db = get_db()

    rows = db.execute(
        """
        SELECT id, time_entry_id, entry_date, description, hours, rate, amount, is_orphaned
        FROM invoice_line_items
        WHERE invoice_id = %s
        ORDER BY entry_date, id
        """,
        (invoice_id,)
    )

    return [
        InvoiceLineItem(
            id=row["id"],
            time_entry_id=row["time_entry_id"],
            entry_date=row["entry_date"],
            description=row["description"],
            hours=row["hours"],
            rate=Decimal(str(row["rate"])),
            amount=Decimal(str(row["amount"])),
            is_orphaned=row["is_orphaned"]
        )
        for row in rows
    ]


def list_invoices(
    user_id: int,
    project_id: int | None = None,
    status: str | None = None,
    limit: int = 100,
    offset: int = 0
) -> tuple[list[Invoice], int]:
    """List invoices with optional filters.

    Args:
        user_id: User ID
        project_id: Optional project filter
        status: Optional status filter
        limit: Max results
        offset: Offset for pagination

    Returns:
        Tuple of (invoices, total_count)
    """
    db = get_db()

    # Build query
    where_clauses = ["i.user_id = %s"]
    params: list[Any] = [user_id]

    if project_id:
        where_clauses.append("i.project_id = %s")
        params.append(project_id)

    if status:
        where_clauses.append("i.status = %s")
        params.append(status)

    where_sql = " AND ".join(where_clauses)

    # Get total count
    count_result = db.execute_one(
        f"SELECT COUNT(*) as count FROM invoices i WHERE {where_sql}",
        tuple(params)
    )
    total = count_result["count"] if count_result else 0

    # Get invoices
    rows = db.execute(
        f"""
        SELECT
            i.id,
            i.user_id,
            i.project_id,
            p.name as project_name,
            p.client,
            i.invoice_number,
            i.period_start,
            i.period_end,
            i.invoice_date,
            i.status,
            i.total_hours,
            i.total_amount,
            i.sheets_spreadsheet_id,
            p.sheets_spreadsheet_url,
            i.sheets_worksheet_id,
            i.last_exported_at,
            i.created_at
        FROM invoices i
        JOIN projects p ON i.project_id = p.id
        WHERE {where_sql}
        ORDER BY i.invoice_date DESC, i.id DESC
        LIMIT %s OFFSET %s
        """,
        tuple(params) + (limit, offset)
    )

    invoices = [
        Invoice(
            id=row["id"],
            user_id=row["user_id"],
            project_id=row["project_id"],
            project_name=row["project_name"],
            client=row["client"],
            invoice_number=row["invoice_number"],
            period_start=row["period_start"],
            period_end=row["period_end"],
            invoice_date=row["invoice_date"],
            status=row["status"],
            total_hours=row["total_hours"],
            total_amount=Decimal(str(row["total_amount"])),
            sheets_spreadsheet_id=row["sheets_spreadsheet_id"],
            sheets_spreadsheet_url=row["sheets_spreadsheet_url"],
            sheets_worksheet_id=row["sheets_worksheet_id"],
            last_exported_at=row["last_exported_at"],
            created_at=row["created_at"],
            line_items=None
        )
        for row in rows
    ]

    return invoices, total


def update_invoice_status(user_id: int, invoice_id: int, status: str) -> Invoice:
    """Update invoice status.

    Args:
        user_id: User ID
        invoice_id: Invoice ID
        status: New status (draft, finalized, paid)

    Returns:
        Updated invoice

    Raises:
        ValueError: If invoice not found or invalid status
    """
    if status not in ("draft", "finalized", "paid"):
        raise ValueError(f"Invalid status: {status}")

    db = get_db()

    # Verify invoice exists and belongs to user
    invoice = get_invoice(user_id, invoice_id, include_line_items=False)
    if not invoice:
        raise ValueError("Invoice not found")

    db.execute(
        "UPDATE invoices SET status = %s, updated_at = NOW() WHERE id = %s",
        (status, invoice_id)
    )

    return get_invoice(user_id, invoice_id)


def regenerate_invoice(user_id: int, invoice_id: int) -> Invoice:
    """Regenerate invoice line items from current unbilled entries.

    Only allowed for draft invoices. Uses the original rate stored on line items.

    Args:
        user_id: User ID
        invoice_id: Invoice ID

    Returns:
        Updated invoice

    Raises:
        ValueError: If invoice not found or not in draft status
    """
    db = get_db()

    invoice = get_invoice(user_id, invoice_id)
    if not invoice:
        raise ValueError("Invoice not found")

    if invoice.status != "draft":
        raise ValueError("Can only regenerate draft invoices")

    # Get the rate from existing line items (snapshot from creation)
    if invoice.line_items:
        bill_rate = invoice.line_items[0].rate
    else:
        # Fallback to project rate if no line items
        project = db.execute_one(
            "SELECT bill_rate FROM projects WHERE id = %s",
            (invoice.project_id,)
        )
        bill_rate = Decimal(str(project["bill_rate"] or 0))

    # Clear existing line items and unmark entries
    db.execute(
        "UPDATE time_entries SET invoice_id = NULL WHERE invoice_id = %s",
        (invoice_id,)
    )
    db.execute(
        "DELETE FROM invoice_line_items WHERE invoice_id = %s",
        (invoice_id,)
    )

    # Get entries for the period (now unbilled again)
    entries = get_unbilled_entries(
        user_id, invoice.project_id, invoice.period_start, invoice.period_end
    )

    if not entries:
        # No entries - update totals to zero
        db.execute(
            """
            UPDATE invoices
            SET total_hours = 0, total_amount = 0, updated_at = NOW()
            WHERE id = %s
            """,
            (invoice_id,)
        )
        return get_invoice(user_id, invoice_id)

    # Calculate new totals
    total_hours = sum(e["hours"] for e in entries)
    total_amount = sum(Decimal(str(e["hours"])) * bill_rate for e in entries)

    # Create new line items
    for entry in entries:
        entry_date = entry["entry_date"]
        if isinstance(entry_date, str):
            entry_date = datetime.fromisoformat(entry_date).date()

        amount = Decimal(str(entry["hours"])) * bill_rate

        db.execute_insert(
            """
            INSERT INTO invoice_line_items
            (invoice_id, time_entry_id, entry_date, description, hours, rate, amount)
            VALUES (%s, %s, %s, %s, %s, %s, %s)
            """,
            (invoice_id, entry["id"], entry_date,
             entry["description"], entry["hours"], bill_rate, amount)
        )

        db.execute(
            "UPDATE time_entries SET invoice_id = %s WHERE id = %s",
            (invoice_id, entry["id"])
        )

    # Update invoice totals
    db.execute(
        """
        UPDATE invoices
        SET total_hours = %s, total_amount = %s, updated_at = NOW()
        WHERE id = %s
        """,
        (total_hours, total_amount, invoice_id)
    )

    return get_invoice(user_id, invoice_id)


def delete_invoice(user_id: int, invoice_id: int) -> bool:
    """Delete a draft invoice.

    Args:
        user_id: User ID
        invoice_id: Invoice ID

    Returns:
        True if deleted

    Raises:
        ValueError: If invoice not found or not in draft status
    """
    db = get_db()

    invoice = get_invoice(user_id, invoice_id, include_line_items=False)
    if not invoice:
        raise ValueError("Invoice not found")

    if invoice.status != "draft":
        raise ValueError("Can only delete draft invoices")

    # Unmark time entries
    db.execute(
        "UPDATE time_entries SET invoice_id = NULL WHERE invoice_id = %s",
        (invoice_id,)
    )

    # Delete invoice (cascade deletes line items)
    db.execute(
        "DELETE FROM invoices WHERE id = %s",
        (invoice_id,)
    )

    return True


def export_invoice_csv(user_id: int, invoice_id: int) -> str:
    """Export invoice as CSV.

    Args:
        user_id: User ID
        invoice_id: Invoice ID

    Returns:
        CSV content as string

    Raises:
        ValueError: If invoice not found
    """
    invoice = get_invoice(user_id, invoice_id)
    if not invoice:
        raise ValueError("Invoice not found")

    output = io.StringIO()
    writer = csv.writer(output)

    # Header metadata
    writer.writerow(["Invoice", invoice.invoice_number])
    writer.writerow(["Project", invoice.project_name])
    writer.writerow(["Client", invoice.client or ""])
    writer.writerow(["Period", f"{invoice.period_start} to {invoice.period_end}"])
    writer.writerow(["Invoice Date", str(invoice.invoice_date)])
    writer.writerow([])

    # Line items header
    writer.writerow(["Date", "Description", "Hours", "Rate", "Amount"])

    # Line items
    for item in invoice.line_items or []:
        writer.writerow([
            str(item.entry_date),
            item.description or "",
            item.hours,
            f"${item.rate:.2f}",
            f"${item.amount:.2f}"
        ])

    # Totals
    writer.writerow([])
    writer.writerow(["", "TOTAL", invoice.total_hours, "", f"${invoice.total_amount:.2f}"])

    return output.getvalue()
