"""Time entry related MCP tools."""

import json
from collections import defaultdict
from datetime import datetime

from .base import BaseTool, ToolResult


class GetTimeEntriesTool(BaseTool):
    """Get time entries with full event details for a date range."""

    name = "get_time_entries"
    description = (
        "Get time entries with full event details for a date range. "
        "Returns both classified entries and optionally unclassified events."
    )
    parameters = {
        "type": "object",
        "properties": {
            "start_date": {
                "type": "string",
                "description": "Start date (YYYY-MM-DD)"
            },
            "end_date": {
                "type": "string",
                "description": "End date (YYYY-MM-DD)"
            },
            "project_id": {
                "type": "integer",
                "description": "Filter by project ID (optional)"
            },
            "include_unclassified": {
                "type": "boolean",
                "description": "Include unclassified events (default: true)"
            }
        },
        "required": ["start_date", "end_date"]
    }

    def execute(
        self,
        start_date: str,
        end_date: str,
        project_id: int | None = None,
        include_unclassified: bool = True
    ) -> ToolResult:
        """Execute the get_time_entries tool.

        Args:
            start_date: Start date in YYYY-MM-DD format
            end_date: End date in YYYY-MM-DD format
            project_id: Optional project ID filter
            include_unclassified: Whether to include unclassified events

        Returns:
            ToolResult with list of entries
        """
        query = """
            SELECT
                e.id as event_id,
                e.title,
                e.description,
                e.start_time,
                e.end_time,
                e.attendees,
                e.meeting_link,
                e.my_response_status,
                e.did_not_attend,
                te.id as entry_id,
                te.hours,
                te.description as entry_description,
                te.project_id,
                te.classification_source,
                te.rule_id,
                p.name as project_name,
                p.color as project_color
            FROM events e
            LEFT JOIN time_entries te ON te.event_id = e.id
            LEFT JOIN projects p ON te.project_id = p.id
            WHERE e.user_id = %s
              AND DATE(e.start_time) >= %s
              AND DATE(e.start_time) <= %s
        """
        params = [self.user_id, start_date, end_date]

        if project_id is not None:
            query += " AND te.project_id = %s"
            params.append(project_id)

        if not include_unclassified:
            query += " AND te.id IS NOT NULL"

        query += " ORDER BY e.start_time"

        rows = self.db.execute(query, tuple(params))

        entries = []
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

            entry = {
                "event": {
                    "id": row["event_id"],
                    "title": row["title"],
                    "description": row["description"],
                    "start_time": start_time,
                    "end_time": end_time,
                    "attendees": attendees,
                    "meeting_link": row["meeting_link"],
                    "my_response_status": row["my_response_status"],
                    "did_not_attend": bool(row["did_not_attend"]) if row["did_not_attend"] is not None else False
                },
                "classification": None
            }

            if row["entry_id"]:
                entry["classification"] = {
                    "entry_id": row["entry_id"],
                    "hours": float(row["hours"]) if row["hours"] else 0.0,
                    "description": row["entry_description"],
                    "project_id": row["project_id"],
                    "project_name": row["project_name"],
                    "project_color": row["project_color"],
                    "source": row["classification_source"],
                    "rule_id": row["rule_id"]
                }

            entries.append(entry)

        return ToolResult(success=True, data=entries)


class GetTimesheetSummaryTool(BaseTool):
    """Get a summary of hours by project for a date range."""

    name = "get_timesheet_summary"
    description = (
        "Get a summary of hours worked by project for a date range. "
        "Useful for creating timesheet reports."
    )
    parameters = {
        "type": "object",
        "properties": {
            "start_date": {
                "type": "string",
                "description": "Start date (YYYY-MM-DD)"
            },
            "end_date": {
                "type": "string",
                "description": "End date (YYYY-MM-DD)"
            },
            "group_by": {
                "type": "string",
                "enum": ["project", "day", "week"],
                "description": "How to group hours (default: project)"
            }
        },
        "required": ["start_date", "end_date"]
    }

    def execute(
        self,
        start_date: str,
        end_date: str,
        group_by: str = "project"
    ) -> ToolResult:
        """Execute the get_timesheet_summary tool.

        Args:
            start_date: Start date in YYYY-MM-DD format
            end_date: End date in YYYY-MM-DD format
            group_by: How to group the results (project, day, or week)

        Returns:
            ToolResult with summary data
        """
        # Get all classified entries in range
        query = """
            SELECT
                e.start_time,
                te.hours,
                te.project_id,
                p.name as project_name,
                p.client,
                p.is_billable,
                p.bill_rate,
                p.does_not_accumulate_hours
            FROM time_entries te
            JOIN events e ON te.event_id = e.id
            JOIN projects p ON te.project_id = p.id
            WHERE te.user_id = %s
              AND DATE(e.start_time) >= %s
              AND DATE(e.start_time) <= %s
            ORDER BY e.start_time
        """
        rows = self.db.execute(query, (self.user_id, start_date, end_date))

        # Also get unclassified events count
        unclassified_query = """
            SELECT COUNT(*) as count
            FROM events e
            LEFT JOIN time_entries te ON te.event_id = e.id
            WHERE e.user_id = %s
              AND DATE(e.start_time) >= %s
              AND DATE(e.start_time) <= %s
              AND te.id IS NULL
              AND (e.did_not_attend = FALSE OR e.did_not_attend IS NULL)
        """
        unclassified_result = self.db.execute_one(
            unclassified_query, (self.user_id, start_date, end_date)
        )
        unclassified_count = unclassified_result["count"] if unclassified_result else 0

        if group_by == "project":
            return self._group_by_project(rows, unclassified_count)
        elif group_by == "day":
            return self._group_by_day(rows, unclassified_count)
        elif group_by == "week":
            return self._group_by_week(rows, unclassified_count)
        else:
            return ToolResult(
                success=False,
                error=f"Invalid group_by value: {group_by}. Must be project, day, or week."
            )

    def _group_by_project(self, rows: list, unclassified_count: int) -> ToolResult:
        """Group hours by project."""
        projects = defaultdict(lambda: {
            "hours": 0.0,
            "billable_hours": 0.0,
            "billable_amount": 0.0,
            "client": None,
            "entry_count": 0
        })

        total_hours = 0.0
        total_billable_hours = 0.0
        total_billable_amount = 0.0

        for row in rows:
            hours = float(row["hours"]) if row["hours"] else 0.0
            project_name = row["project_name"]

            # Skip non-accumulating projects in totals
            if not row["does_not_accumulate_hours"]:
                total_hours += hours

            projects[project_name]["hours"] += hours
            projects[project_name]["client"] = row["client"]
            projects[project_name]["entry_count"] += 1

            if row["is_billable"]:
                projects[project_name]["billable_hours"] += hours
                if not row["does_not_accumulate_hours"]:
                    total_billable_hours += hours

                if row["bill_rate"]:
                    amount = hours * float(row["bill_rate"])
                    projects[project_name]["billable_amount"] += amount
                    if not row["does_not_accumulate_hours"]:
                        total_billable_amount += amount

        summary = {
            "total_hours": round(total_hours, 2),
            "total_billable_hours": round(total_billable_hours, 2),
            "total_billable_amount": round(total_billable_amount, 2),
            "unclassified_events": unclassified_count,
            "by_project": [
                {
                    "project": name,
                    "client": data["client"],
                    "hours": round(data["hours"], 2),
                    "billable_hours": round(data["billable_hours"], 2),
                    "billable_amount": round(data["billable_amount"], 2),
                    "entry_count": data["entry_count"]
                }
                for name, data in sorted(projects.items())
            ]
        }

        return ToolResult(success=True, data=summary)

    def _group_by_day(self, rows: list, unclassified_count: int) -> ToolResult:
        """Group hours by day."""
        days = defaultdict(lambda: defaultdict(float))
        total_hours = 0.0

        for row in rows:
            hours = float(row["hours"]) if row["hours"] else 0.0
            start_time = row["start_time"]
            if isinstance(start_time, str):
                start_time = datetime.fromisoformat(start_time.replace("Z", "+00:00"))
            day = start_time.strftime("%Y-%m-%d")
            project = row["project_name"]

            if not row["does_not_accumulate_hours"]:
                total_hours += hours

            days[day][project] += hours

        summary = {
            "total_hours": round(total_hours, 2),
            "unclassified_events": unclassified_count,
            "by_day": [
                {
                    "date": day,
                    "total_hours": round(sum(projects.values()), 2),
                    "by_project": {
                        proj: round(hrs, 2) for proj, hrs in sorted(projects.items())
                    }
                }
                for day, projects in sorted(days.items())
            ]
        }

        return ToolResult(success=True, data=summary)

    def _group_by_week(self, rows: list, unclassified_count: int) -> ToolResult:
        """Group hours by week."""
        weeks = defaultdict(lambda: defaultdict(float))
        total_hours = 0.0

        for row in rows:
            hours = float(row["hours"]) if row["hours"] else 0.0
            start_time = row["start_time"]
            if isinstance(start_time, str):
                start_time = datetime.fromisoformat(start_time.replace("Z", "+00:00"))

            # Get ISO week (year, week number)
            iso_cal = start_time.isocalendar()
            week_key = f"{iso_cal[0]}-W{iso_cal[1]:02d}"
            project = row["project_name"]

            if not row["does_not_accumulate_hours"]:
                total_hours += hours

            weeks[week_key][project] += hours

        summary = {
            "total_hours": round(total_hours, 2),
            "unclassified_events": unclassified_count,
            "by_week": [
                {
                    "week": week,
                    "total_hours": round(sum(projects.values()), 2),
                    "by_project": {
                        proj: round(hrs, 2) for proj, hrs in sorted(projects.items())
                    }
                }
                for week, projects in sorted(weeks.items())
            ]
        }

        return ToolResult(success=True, data=summary)
