"""MCP Tools for timesheet operations."""

from .base import BaseTool, ToolResult
from .projects import ListProjectsTool
from .time_entries import GetTimeEntriesTool, GetTimesheetSummaryTool

__all__ = [
    "BaseTool",
    "ToolResult",
    "ListProjectsTool",
    "GetTimeEntriesTool",
    "GetTimesheetSummaryTool",
]

# All available tools - order matters for registration
ALL_TOOLS = [
    ListProjectsTool,
    GetTimeEntriesTool,
    GetTimesheetSummaryTool,
]
