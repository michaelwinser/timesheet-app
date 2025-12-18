"""MCP Tools for invoice operations."""

from datetime import date
from decimal import Decimal

from .base import BaseTool, ToolResult


class ListInvoicesTool(BaseTool):
    """List invoices with optional filters."""

    name = "list_invoices"
    description = """List invoices for the current user.
    Can filter by project_id and/or status.
    Returns invoice metadata including number, period, hours, and amount."""

    parameters = {
        "type": "object",
        "properties": {
            "project_id": {
                "type": "integer",
                "description": "Filter by project ID"
            },
            "status": {
                "type": "string",
                "enum": ["draft", "finalized", "paid"],
                "description": "Filter by invoice status"
            },
            "limit": {
                "type": "integer",
                "description": "Maximum number of invoices to return (default 50)"
            }
        },
        "required": []
    }

    def execute(
        self,
        project_id: int | None = None,
        status: str | None = None,
        limit: int = 50
    ) -> ToolResult:
        from services.invoice import list_invoices

        invoices, total = list_invoices(
            self.user_id,
            project_id=project_id,
            status=status,
            limit=limit,
            offset=0
        )

        result = [
            {
                "id": inv.id,
                "invoice_number": inv.invoice_number,
                "project_id": inv.project_id,
                "project_name": inv.project_name,
                "client": inv.client,
                "period_start": str(inv.period_start),
                "period_end": str(inv.period_end),
                "invoice_date": str(inv.invoice_date),
                "status": inv.status,
                "total_hours": inv.total_hours,
                "total_amount": float(inv.total_amount),
            }
            for inv in invoices
        ]

        return ToolResult(success=True, data={
            "invoices": result,
            "total": total
        })


class GetInvoiceTool(BaseTool):
    """Get detailed invoice information including line items."""

    name = "get_invoice"
    description = """Get detailed information about a specific invoice.
    Returns full invoice details including all line items."""

    parameters = {
        "type": "object",
        "properties": {
            "invoice_id": {
                "type": "integer",
                "description": "The invoice ID"
            }
        },
        "required": ["invoice_id"]
    }

    def execute(self, invoice_id: int) -> ToolResult:
        from services.invoice import get_invoice

        invoice = get_invoice(self.user_id, invoice_id)

        if not invoice:
            return ToolResult(success=False, error="Invoice not found")

        line_items = [
            {
                "id": item.id,
                "entry_date": str(item.entry_date),
                "description": item.description,
                "hours": item.hours,
                "rate": float(item.rate),
                "amount": float(item.amount),
                "is_orphaned": item.is_orphaned,
            }
            for item in (invoice.line_items or [])
        ]

        return ToolResult(success=True, data={
            "id": invoice.id,
            "invoice_number": invoice.invoice_number,
            "project_id": invoice.project_id,
            "project_name": invoice.project_name,
            "client": invoice.client,
            "period_start": str(invoice.period_start),
            "period_end": str(invoice.period_end),
            "invoice_date": str(invoice.invoice_date),
            "status": invoice.status,
            "total_hours": invoice.total_hours,
            "total_amount": float(invoice.total_amount),
            "line_items": line_items,
        })


class PreviewInvoiceTool(BaseTool):
    """Preview what an invoice would contain without creating it."""

    name = "preview_invoice"
    description = """Preview an invoice for a project and date range.
    Shows unbilled time entries and calculated totals.
    Use this before creating an invoice to verify the data."""

    parameters = {
        "type": "object",
        "properties": {
            "project_id": {
                "type": "integer",
                "description": "The project ID"
            },
            "period_start": {
                "type": "string",
                "description": "Start date in YYYY-MM-DD format"
            },
            "period_end": {
                "type": "string",
                "description": "End date in YYYY-MM-DD format"
            }
        },
        "required": ["project_id", "period_start", "period_end"]
    }

    def execute(
        self,
        project_id: int,
        period_start: str,
        period_end: str
    ) -> ToolResult:
        from services.invoice import get_unbilled_entries, generate_invoice_number

        # Get project details
        project = self.db.execute_one(
            "SELECT id, name, short_code, is_billable, bill_rate FROM projects WHERE id = %s AND user_id = %s",
            (project_id, self.user_id)
        )
        if not project:
            return ToolResult(success=False, error="Project not found")

        if not project["is_billable"]:
            return ToolResult(success=False, error="Project is not billable")

        try:
            start = date.fromisoformat(period_start)
            end = date.fromisoformat(period_end)
        except ValueError:
            return ToolResult(success=False, error="Invalid date format. Use YYYY-MM-DD")

        entries = get_unbilled_entries(self.user_id, project_id, start, end)
        bill_rate = Decimal(str(project["bill_rate"] or 0))
        total_hours = sum(e["hours"] for e in entries)
        total_amount = sum(Decimal(str(e["hours"])) * bill_rate for e in entries)

        return ToolResult(success=True, data={
            "project_id": project_id,
            "project_name": project["name"],
            "invoice_number": generate_invoice_number(
                project_id, project["name"], self.user_id, project.get("short_code")
            ),
            "period_start": period_start,
            "period_end": period_end,
            "unbilled_entries": len(entries),
            "total_hours": total_hours,
            "bill_rate": float(bill_rate),
            "total_amount": float(total_amount),
        })


class CreateInvoiceTool(BaseTool):
    """Create a new invoice from unbilled time entries."""

    name = "create_invoice"
    description = """Create an invoice for a project's unbilled time entries.
    Automatically captures all unbilled entries in the date range.
    Returns the created invoice with line items."""

    parameters = {
        "type": "object",
        "properties": {
            "project_id": {
                "type": "integer",
                "description": "The project ID"
            },
            "period_start": {
                "type": "string",
                "description": "Start date in YYYY-MM-DD format"
            },
            "period_end": {
                "type": "string",
                "description": "End date in YYYY-MM-DD format"
            },
            "invoice_date": {
                "type": "string",
                "description": "Invoice date in YYYY-MM-DD format (defaults to today)"
            }
        },
        "required": ["project_id", "period_start", "period_end"]
    }

    def execute(
        self,
        project_id: int,
        period_start: str,
        period_end: str,
        invoice_date: str | None = None
    ) -> ToolResult:
        from services.invoice import create_invoice

        try:
            start = date.fromisoformat(period_start)
            end = date.fromisoformat(period_end)
            inv_date = date.fromisoformat(invoice_date) if invoice_date else None

            invoice = create_invoice(
                self.user_id,
                project_id=project_id,
                period_start=start,
                period_end=end,
                invoice_date=inv_date
            )

            return ToolResult(success=True, data={
                "id": invoice.id,
                "invoice_number": invoice.invoice_number,
                "project_name": invoice.project_name,
                "period_start": str(invoice.period_start),
                "period_end": str(invoice.period_end),
                "total_hours": invoice.total_hours,
                "total_amount": float(invoice.total_amount),
                "line_items_count": len(invoice.line_items or []),
                "message": f"Created invoice {invoice.invoice_number} with {len(invoice.line_items or [])} line items"
            })
        except ValueError as e:
            return ToolResult(success=False, error=str(e))


class UpdateInvoiceStatusTool(BaseTool):
    """Update an invoice's status."""

    name = "update_invoice_status"
    description = """Update the status of an invoice.
    Status can be: draft, finalized, or paid.
    Only draft invoices can be deleted or regenerated."""

    parameters = {
        "type": "object",
        "properties": {
            "invoice_id": {
                "type": "integer",
                "description": "The invoice ID"
            },
            "status": {
                "type": "string",
                "enum": ["draft", "finalized", "paid"],
                "description": "The new status"
            }
        },
        "required": ["invoice_id", "status"]
    }

    def execute(self, invoice_id: int, status: str) -> ToolResult:
        from services.invoice import update_invoice_status

        try:
            invoice = update_invoice_status(self.user_id, invoice_id, status)
            return ToolResult(success=True, data={
                "invoice_number": invoice.invoice_number,
                "status": invoice.status,
                "message": f"Invoice {invoice.invoice_number} status updated to {status}"
            })
        except ValueError as e:
            return ToolResult(success=False, error=str(e))


class GetBillableProjectsTool(BaseTool):
    """List projects that are configured for billing."""

    name = "get_billable_projects"
    description = """Get a list of projects that are marked as billable.
    Use this to find which projects can have invoices created."""

    parameters = {
        "type": "object",
        "properties": {},
        "required": []
    }

    def execute(self) -> ToolResult:
        rows = self.db.execute(
            """
            SELECT id, name, client, bill_rate
            FROM projects
            WHERE user_id = %s AND is_billable = TRUE AND is_archived = FALSE
            ORDER BY name
            """,
            (self.user_id,)
        )

        projects = [
            {
                "id": row["id"],
                "name": row["name"],
                "client": row["client"],
                "bill_rate": float(row["bill_rate"]) if row["bill_rate"] else None,
            }
            for row in rows
        ]

        return ToolResult(success=True, data={"projects": projects})
