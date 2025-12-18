"""MCP Tools for timesheet operations."""

from .base import BaseTool, ToolResult
from .classify import BulkClassifyTool
from .projects import ListProjectsTool
from .search import SearchEventsTool
from .time_entries import GetTimeEntriesTool, GetTimesheetSummaryTool

__all__ = [
    "BaseTool",
    "ToolResult",
    "ListProjectsTool",
    "GetTimeEntriesTool",
    "GetTimesheetSummaryTool",
    "SearchEventsTool",
    "BulkClassifyTool",
]

# All available tools - order matters for registration
ALL_TOOLS = [
    ListProjectsTool,
    GetTimeEntriesTool,
    GetTimesheetSummaryTool,
    SearchEventsTool,
    BulkClassifyTool,
]
