"""MCP Server setup and tool registration."""

import json
import logging
from functools import wraps
from typing import Any, Callable

from mcp.server.fastmcp import FastMCP

from .auth import AuthProvider
from .tools import ALL_TOOLS

logger = logging.getLogger(__name__)


def create_server(db, auth: AuthProvider) -> FastMCP:
    """Create and configure the MCP server with all tools.

    Args:
        db: Database connection object
        auth: Authentication provider

    Returns:
        Configured FastMCP Server instance
    """
    mcp = FastMCP(
        "timesheet",
        instructions=(
            "This MCP server provides access to timesheet data. "
            "Use the available tools to query time entries, projects, and generate reports."
        )
    )

    # Instantiate all tools with db and auth
    tool_instances = {tool.name: tool(db, auth) for tool in ALL_TOOLS}

    # Register each tool with FastMCP
    for tool_cls in ALL_TOOLS:
        tool_instance = tool_instances[tool_cls.name]
        _register_tool(mcp, tool_instance)

    return mcp


def _register_tool(mcp: FastMCP, tool_instance) -> None:
    """Register a tool instance with FastMCP.

    Creates a wrapper function that calls the tool's execute method
    and registers it with the appropriate schema.

    Args:
        mcp: FastMCP server instance
        tool_instance: Instantiated tool object
    """
    # Create a wrapper function for the tool
    async def tool_handler(**kwargs) -> str:
        """Execute the tool and return JSON result."""
        try:
            logger.info(f"Executing tool: {tool_instance.name} with args: {kwargs}")
            result = tool_instance.execute(**kwargs)

            if result.success:
                return json.dumps(result.data, default=str)
            else:
                return json.dumps({"error": result.error})

        except Exception as e:
            logger.exception(f"Error executing tool {tool_instance.name}: {e}")
            return json.dumps({"error": str(e)})

    # Set function metadata for FastMCP
    tool_handler.__name__ = tool_instance.name
    tool_handler.__doc__ = tool_instance.description

    # Add the tool with its schema
    mcp.add_tool(
        tool_handler,
        name=tool_instance.name,
        description=tool_instance.description,
    )
