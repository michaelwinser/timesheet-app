"""Project-related MCP tools."""

from .base import BaseTool, ToolResult


class ListProjectsTool(BaseTool):
    """List all projects for the authenticated user."""

    name = "list_projects"
    description = "List all projects with their settings (name, color, billing info, etc.)"
    parameters = {
        "type": "object",
        "properties": {
            "include_archived": {
                "type": "boolean",
                "description": "Include archived projects (default: false)"
            },
            "include_hidden": {
                "type": "boolean",
                "description": "Include hidden projects (default: true)"
            }
        },
        "required": []
    }

    def execute(
        self,
        include_archived: bool = False,
        include_hidden: bool = True
    ) -> ToolResult:
        """Execute the list_projects tool.

        Args:
            include_archived: Whether to include archived projects
            include_hidden: Whether to include hidden projects

        Returns:
            ToolResult with list of projects
        """
        query = """
            SELECT
                id,
                name,
                client,
                color,
                does_not_accumulate_hours,
                is_billable,
                bill_rate,
                is_hidden_by_default,
                is_archived,
                created_at
            FROM projects
            WHERE user_id = %s
        """
        params = [self.user_id]

        if not include_archived:
            query += " AND (is_archived = FALSE OR is_archived IS NULL)"

        if not include_hidden:
            query += " AND (is_hidden_by_default = FALSE OR is_hidden_by_default IS NULL)"

        query += " ORDER BY name"

        rows = self.db.execute(query, tuple(params))

        projects = []
        for row in rows:
            projects.append({
                "id": row["id"],
                "name": row["name"],
                "client": row["client"],
                "color": row["color"] or "#00aa44",
                "does_not_accumulate_hours": bool(row.get("does_not_accumulate_hours", False)),
                "is_billable": bool(row.get("is_billable", False)),
                "bill_rate": row.get("bill_rate"),
                "is_hidden_by_default": bool(row.get("is_hidden_by_default", False)),
                "is_archived": bool(row.get("is_archived", False)),
            })

        return ToolResult(success=True, data=projects)
