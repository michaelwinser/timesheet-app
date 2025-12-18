"""Search-related MCP tools."""

import json
from datetime import datetime

from .base import BaseTool, ToolResult


class SearchEventsTool(BaseTool):
    """Search events by text across title, description, and attendees."""

    name = "search_events"
    description = (
        "Search events by text across title, description, and attendees. "
        "Returns matching events with their classification status."
    )
    parameters = {
        "type": "object",
        "properties": {
            "query": {
                "type": "string",
                "description": "Search text to find in events"
            },
            "start_date": {
                "type": "string",
                "description": "Start date filter (YYYY-MM-DD, optional)"
            },
            "end_date": {
                "type": "string",
                "description": "End date filter (YYYY-MM-DD, optional)"
            },
            "classified": {
                "type": "boolean",
                "description": "Filter by classification status: true=only classified, false=only unclassified, omit=all"
            },
            "limit": {
                "type": "integer",
                "description": "Maximum number of results (default: 100)"
            }
        },
        "required": ["query"]
    }

    def execute(
        self,
        query: str,
        start_date: str | None = None,
        end_date: str | None = None,
        classified: bool | None = None,
        limit: int = 100
    ) -> ToolResult:
        """Execute the search_events tool.

        Args:
            query: Search text
            start_date: Optional start date filter
            end_date: Optional end date filter
            classified: Optional filter by classification status
            limit: Maximum results to return

        Returns:
            ToolResult with matching events
        """
        search_pattern = f"%{query.lower()}%"

        sql = """
            SELECT
                e.id as event_id,
                e.title,
                e.description,
                e.start_time,
                e.end_time,
                e.attendees,
                e.did_not_attend,
                te.id as entry_id,
                te.project_id,
                te.hours,
                p.name as project_name
            FROM events e
            LEFT JOIN time_entries te ON te.event_id = e.id
            LEFT JOIN projects p ON te.project_id = p.id
            WHERE e.user_id = %s
              AND (
                  LOWER(e.title) LIKE %s
                  OR LOWER(COALESCE(e.description, '')) LIKE %s
                  OR LOWER(COALESCE(e.attendees, '')) LIKE %s
              )
        """
        params = [self.user_id, search_pattern, search_pattern, search_pattern]

        if start_date:
            sql += " AND DATE(e.start_time) >= %s"
            params.append(start_date)

        if end_date:
            sql += " AND DATE(e.start_time) <= %s"
            params.append(end_date)

        if classified is not None:
            if classified:
                sql += " AND te.id IS NOT NULL"
            else:
                sql += " AND te.id IS NULL"

        sql += " ORDER BY e.start_time DESC LIMIT %s"
        params.append(limit)

        rows = self.db.execute(sql, tuple(params))

        results = []
        for row in rows:
            # Parse attendees JSON
            attendees = []
            if row["attendees"]:
                try:
                    attendees = json.loads(row["attendees"])
                except (json.JSONDecodeError, TypeError):
                    attendees = []

            # Format timestamps
            start_time = row["start_time"]
            end_time = row["end_time"]
            if isinstance(start_time, datetime):
                start_time = start_time.isoformat()
            if isinstance(end_time, datetime):
                end_time = end_time.isoformat()

            results.append({
                "event_id": row["event_id"],
                "title": row["title"],
                "description": row["description"],
                "start_time": start_time,
                "end_time": end_time,
                "attendees": attendees,
                "did_not_attend": bool(row["did_not_attend"]) if row["did_not_attend"] is not None else False,
                "is_classified": row["entry_id"] is not None,
                "project_id": row["project_id"],
                "project_name": row["project_name"],
                "hours": float(row["hours"]) if row["hours"] else None
            })

        return ToolResult(success=True, data={
            "query": query,
            "total_results": len(results),
            "events": results
        })
