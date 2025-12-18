"""MCP Tools for timesheet operations."""

from .base import BaseTool, ToolResult
from .classify import BulkClassifyTool
from .projects import ListProjectsTool
from .search import SearchEventsTool
from .time_entries import GetTimeEntriesTool, GetTimesheetSummaryTool
from .invoices import (
    ListInvoicesTool,
    GetInvoiceTool,
    PreviewInvoiceTool,
    CreateInvoiceTool,
    UpdateInvoiceStatusTool,
    GetBillableProjectsTool,
)

__all__ = [
    "BaseTool",
    "ToolResult",
    "ListProjectsTool",
    "GetTimeEntriesTool",
    "GetTimesheetSummaryTool",
    "SearchEventsTool",
    "BulkClassifyTool",
    "ListInvoicesTool",
    "GetInvoiceTool",
    "PreviewInvoiceTool",
    "CreateInvoiceTool",
    "UpdateInvoiceStatusTool",
    "GetBillableProjectsTool",
]

# All available tools - order matters for registration
ALL_TOOLS = [
    ListProjectsTool,
    GetTimeEntriesTool,
    GetTimesheetSummaryTool,
    SearchEventsTool,
    BulkClassifyTool,
    # Invoice tools
    ListInvoicesTool,
    GetInvoiceTool,
    PreviewInvoiceTool,
    CreateInvoiceTool,
    UpdateInvoiceStatusTool,
    GetBillableProjectsTool,
]
