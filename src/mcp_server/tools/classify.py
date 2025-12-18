"""Classification-related MCP tools."""

from datetime import datetime

from .base import BaseTool, ToolResult


class BulkClassifyTool(BaseTool):
    """Classify multiple events to a project at once."""

    name = "bulk_classify"
    description = (
        "Classify multiple events to a project at once. "
        "Skips events that are already classified or marked as did-not-attend."
    )
    parameters = {
        "type": "object",
        "properties": {
            "event_ids": {
                "type": "array",
                "items": {"type": "integer"},
                "description": "List of event IDs to classify"
            },
            "project_id": {
                "type": "integer",
                "description": "Project ID to assign the events to"
            }
        },
        "required": ["event_ids", "project_id"]
    }

    def execute(
        self,
        event_ids: list[int],
        project_id: int
    ) -> ToolResult:
        """Execute the bulk_classify tool.

        Args:
            event_ids: List of event IDs to classify
            project_id: Project to assign

        Returns:
            ToolResult with classification summary
        """
        # Verify project belongs to user
        project = self.db.execute_one(
            "SELECT id, name FROM projects WHERE id = %s AND user_id = %s",
            (project_id, self.user_id)
        )
        if not project:
            return ToolResult(success=False, error="Project not found")

        classified = 0
        skipped_already_classified = 0
        skipped_did_not_attend = 0
        skipped_not_found = 0
        classified_events = []

        for event_id in event_ids:
            # Get event
            event = self.db.execute_one(
                """
                SELECT id, title, start_time, end_time, did_not_attend
                FROM events
                WHERE id = %s AND user_id = %s
                """,
                (event_id, self.user_id)
            )

            if not event:
                skipped_not_found += 1
                continue

            if event["did_not_attend"]:
                skipped_did_not_attend += 1
                continue

            # Check if already classified
            existing = self.db.execute_one(
                "SELECT id FROM time_entries WHERE event_id = %s",
                (event_id,)
            )
            if existing:
                skipped_already_classified += 1
                continue

            # Calculate hours from event duration
            start_time = event["start_time"]
            end_time = event["end_time"]
            if isinstance(start_time, str):
                start_time = datetime.fromisoformat(start_time.replace("Z", "+00:00"))
            if isinstance(end_time, str):
                end_time = datetime.fromisoformat(end_time.replace("Z", "+00:00"))
            hours = (end_time - start_time).total_seconds() / 3600

            # Create time entry
            self.db.execute_insert(
                """
                INSERT INTO time_entries
                (user_id, event_id, project_id, hours, description, classification_source)
                VALUES (%s, %s, %s, %s, %s, 'mcp')
                RETURNING id
                """,
                (self.user_id, event_id, project_id, hours, event["title"])
            )

            classified += 1
            classified_events.append({
                "event_id": event_id,
                "title": event["title"],
                "hours": round(hours, 2)
            })

        return ToolResult(success=True, data={
            "project_id": project_id,
            "project_name": project["name"],
            "classified": classified,
            "skipped": {
                "already_classified": skipped_already_classified,
                "did_not_attend": skipped_did_not_attend,
                "not_found": skipped_not_found
            },
            "classified_events": classified_events
        })
