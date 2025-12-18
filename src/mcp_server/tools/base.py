"""Base class for MCP tools."""

from dataclasses import dataclass
from typing import Any

from ..auth import AuthProvider


@dataclass
class ToolResult:
    """Result from a tool execution.

    Attributes:
        success: Whether the tool executed successfully
        data: The result data (if successful)
        error: Error message (if not successful)
    """
    success: bool
    data: Any = None
    error: str | None = None


class BaseTool:
    """Base class for all MCP tools.

    Provides common functionality for database access and authentication.
    Subclasses must define:
        - name: Tool name for MCP registration
        - description: Human-readable description
        - parameters: JSON Schema for tool parameters
        - execute(): Method to execute the tool

    Example:
        class MyTool(BaseTool):
            name = "my_tool"
            description = "Does something useful"
            parameters = {
                "type": "object",
                "properties": {
                    "arg1": {"type": "string"}
                },
                "required": ["arg1"]
            }

            def execute(self, arg1: str) -> ToolResult:
                # Implementation
                return ToolResult(success=True, data={"result": arg1})
    """

    # Subclasses must override these
    name: str = ""
    description: str = ""
    parameters: dict = {}

    def __init__(self, db, auth: AuthProvider):
        """Initialize tool with database and auth.

        Args:
            db: Database connection object
            auth: Authentication provider
        """
        self.db = db
        self.auth = auth

    @property
    def user_id(self) -> int:
        """Get the current authenticated user's ID."""
        return self.auth.get_current_user().user_id

    def execute(self, **kwargs) -> ToolResult:
        """Execute the tool.

        Subclasses must override this method.

        Args:
            **kwargs: Tool-specific arguments

        Returns:
            ToolResult with success status and data/error
        """
        raise NotImplementedError("Subclasses must implement execute()")
