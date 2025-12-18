"""Export service for generating Harvest-compatible CSV."""

import csv
import io
from datetime import datetime

from db import get_db


def export_harvest_csv(start_date: str, end_date: str, user_id: int) -> str:
    """
    Generate a Harvest-compatible CSV for time entries in date range.

    Harvest CSV format:
    Date, Client, Project, Task, Notes, Hours

    Args:
        start_date: ISO date string (YYYY-MM-DD)
        end_date: ISO date string (YYYY-MM-DD)
        user_id: User ID to export data for

    Returns:
        CSV content as string
    """
    db = get_db()

    rows = db.execute(
        """
        SELECT
            e.start_time,
            p.client,
            p.name as project_name,
            te.description,
            te.hours
        FROM time_entries te
        JOIN events e ON te.event_id = e.id
        JOIN projects p ON te.project_id = p.id
        WHERE e.user_id = %s AND date(e.start_time) >= %s AND date(e.start_time) <= %s
        ORDER BY e.start_time
        """,
        (user_id, start_date, end_date),
    )

    output = io.StringIO()
    writer = csv.writer(output)

    # Harvest header
    writer.writerow(["Date", "Client", "Project", "Task", "Notes", "Hours"])

    for row in rows:
        # Format date as MM/DD/YYYY for Harvest
        start_time = row["start_time"]
        event_date = start_time if isinstance(start_time, datetime) else datetime.fromisoformat(start_time)
        date_str = event_date.strftime("%m/%d/%Y")

        writer.writerow([
            date_str,
            row["client"] or "",
            row["project_name"],
            "",  # Task - not used in our model
            row["description"] or "",
            row["hours"],
        ])

    return output.getvalue()
