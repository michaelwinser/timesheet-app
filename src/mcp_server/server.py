"""MCP Server setup and tool registration."""

import json
import logging
from typing import Any

from mcp.server import Server
from mcp.types import Tool, TextContent

from .auth import AuthProvider
from .tools import ALL_TOOLS

logger = logging.getLogger(__name__)


def create_server(db, auth: AuthProvider) -> Server:
    """Create and configure the MCP server with all tools.

    Args:
        db: Database connection object
        auth: Authentication provider

    Returns:
        Configured MCP Server instance
    """
    server = Server("timesheet")

    # Instantiate all tools with db and auth
    tool_instances = {tool.name: tool(db, auth) for tool in ALL_TOOLS}

    @server.list_tools()
    async def list_tools() -> list[Tool]:
        """List all available tools with their schemas."""
        return [
            Tool(
                name=instance.name,
                description=instance.description,
                inputSchema=instance.parameters
            )
            for instance in tool_instances.values()
        ]

    @server.call_tool()
    async def call_tool(name: str, arguments: dict[str, Any]) -> list[TextContent]:
        """Execute a tool by name with given arguments."""
        tool = tool_instances.get(name)
        if not tool:
            logger.warning(f"Unknown tool requested: {name}")
            return [TextContent(
                type="text",
                text=json.dumps({"error": f"Unknown tool: {name}"})
            )]

        try:
            logger.info(f"Executing tool: {name} with args: {arguments}")
            result = tool.execute(**arguments)

            if result.success:
                return [TextContent(
                    type="text",
                    text=json.dumps(result.data, default=str)
                )]
            else:
                return [TextContent(
                    type="text",
                    text=json.dumps({"error": result.error})
                )]

        except Exception as e:
            logger.exception(f"Error executing tool {name}: {e}")
            return [TextContent(
                type="text",
                text=json.dumps({"error": str(e)})
            )]

    return server
