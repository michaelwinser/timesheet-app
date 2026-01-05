#!/usr/bin/env python3
"""MCP server for the Timesheet application.

Provides AI-friendly tools for analyzing and managing timesheet data.
"""

import json
import os
from collections import defaultdict
from datetime import date, datetime, timedelta

from dotenv import load_dotenv
from mcp.server.fastmcp import FastMCP

from api_client import TimesheetAPI, TimesheetAPIError

load_dotenv()

# Initialize MCP server
mcp = FastMCP(
    "timesheet",
    instructions="""You are an AI assistant helping manage a timesheet application.

The user tracks their time across different projects. Calendar events are synced from
Google Calendar and need to be classified (assigned to projects or marked as skipped).

Key concepts:
- Projects: Named work categories (e.g., "Acme Corp Website", "Internal Meetings")
- Time Entries: Hours logged per project per day
- Calendar Events: Synced from Google Calendar, need classification
- Classification: Assigning an event to a project (billable) or skipping it (non-work)

When helping the user:
1. First list projects to understand available options
2. Look at pending events to see what needs attention
3. Use time summaries to analyze patterns
4. Suggest classifications based on event titles and attendees
""",
)

# Lazy-initialized API client
_api: TimesheetAPI | None = None


def get_api() -> TimesheetAPI:
    """Get or create the API client."""
    global _api
    if _api is None:
        _api = TimesheetAPI()
    return _api


# Helper functions
def format_hours(hours: float) -> str:
    """Format hours as Xh Ym."""
    h = int(hours)
    m = int((hours - h) * 60)
    if m == 0:
        return f"{h}h"
    return f"{h}h {m}m"


def parse_date(date_str: str | None, default_days_ago: int = 0) -> date:
    """Parse a date string or return a default."""
    if date_str:
        return datetime.strptime(date_str, "%Y-%m-%d").date()
    return date.today() - timedelta(days=default_days_ago)


# MCP Tools
@mcp.tool()
def list_projects(include_archived: bool = False) -> str:
    """List all projects in the timesheet system.

    Returns project names, IDs, and metadata needed for classifying events
    or logging time. Use this first to understand available projects.

    Args:
        include_archived: Include archived/inactive projects
    """
    api = get_api()
    projects = api.list_projects(include_archived=include_archived)

    if not projects:
        return "No projects found. Create a project first."

    lines = ["# Projects\n"]
    for p in projects:
        status = ""
        if p.get("is_archived"):
            status = " (archived)"
        elif not p.get("is_billable"):
            status = " (non-billable)"

        lines.append(f"- **{p['name']}**{status}")
        lines.append(f"  - ID: `{p['id']}`")
        if p.get("client"):
            lines.append(f"  - Client: {p['client']}")
        if p.get("short_code"):
            lines.append(f"  - Code: {p['short_code']}")

    return "\n".join(lines)


@mcp.tool()
def get_time_summary(
    start_date: str | None = None,
    end_date: str | None = None,
    group_by: str = "project",
) -> str:
    """Get a summary of time entries grouped by project or date.

    Useful for analyzing time spent, identifying patterns, and reporting.

    Args:
        start_date: Start date (YYYY-MM-DD). Defaults to 7 days ago.
        end_date: End date (YYYY-MM-DD). Defaults to today.
        group_by: How to group results: "project", "date", or "week"
    """
    api = get_api()
    start = parse_date(start_date, default_days_ago=7)
    end = parse_date(end_date, default_days_ago=0)

    entries = api.list_time_entries(start_date=start, end_date=end)

    if not entries:
        return f"No time entries found between {start} and {end}."

    total_hours = sum(e["hours"] for e in entries)

    if group_by == "project":
        by_project: dict[str, float] = defaultdict(float)
        project_names: dict[str, str] = {}
        for e in entries:
            pid = e["project_id"]
            by_project[pid] += e["hours"]
            if "project" in e and e["project"]:
                project_names[pid] = e["project"]["name"]

        lines = [f"# Time Summary ({start} to {end})\n"]
        lines.append(f"**Total: {format_hours(total_hours)}**\n")
        lines.append("## By Project\n")

        for pid, hours in sorted(by_project.items(), key=lambda x: -x[1]):
            name = project_names.get(pid, pid)
            pct = (hours / total_hours * 100) if total_hours > 0 else 0
            lines.append(f"- {name}: {format_hours(hours)} ({pct:.0f}%)")

        return "\n".join(lines)

    elif group_by == "date":
        by_date: dict[str, float] = defaultdict(float)
        for e in entries:
            by_date[e["date"]] += e["hours"]

        lines = [f"# Time Summary ({start} to {end})\n"]
        lines.append(f"**Total: {format_hours(total_hours)}**\n")
        lines.append("## By Date\n")

        for d in sorted(by_date.keys()):
            hours = by_date[d]
            lines.append(f"- {d}: {format_hours(hours)}")

        return "\n".join(lines)

    elif group_by == "week":
        by_week: dict[str, float] = defaultdict(float)
        for e in entries:
            entry_date = datetime.strptime(e["date"], "%Y-%m-%d").date()
            week_start = entry_date - timedelta(days=entry_date.weekday())
            by_week[str(week_start)] += e["hours"]

        lines = [f"# Time Summary ({start} to {end})\n"]
        lines.append(f"**Total: {format_hours(total_hours)}**\n")
        lines.append("## By Week\n")

        for week in sorted(by_week.keys()):
            hours = by_week[week]
            lines.append(f"- Week of {week}: {format_hours(hours)}")

        return "\n".join(lines)

    else:
        return f"Unknown group_by value: {group_by}. Use 'project', 'date', or 'week'."


@mcp.tool()
def list_pending_events(
    start_date: str | None = None,
    end_date: str | None = None,
    limit: int = 20,
) -> str:
    """List calendar events that need classification.

    These are synced events that haven't been assigned to a project yet.
    Review these to decide which project they belong to or skip them.

    Args:
        start_date: Start date (YYYY-MM-DD). Defaults to 30 days ago.
        end_date: End date (YYYY-MM-DD). Defaults to today.
        limit: Maximum number of events to return.
    """
    api = get_api()
    start = parse_date(start_date, default_days_ago=30)
    end = parse_date(end_date, default_days_ago=0)

    events = api.list_calendar_events(
        start_date=start,
        end_date=end,
        classification_status="pending",
    )

    if not events:
        return f"No pending events between {start} and {end}. All caught up!"

    # Sort by start time and limit
    events = sorted(events, key=lambda e: e["start_time"])[:limit]

    lines = [f"# Pending Calendar Events ({len(events)} shown)\n"]

    for e in events:
        start_dt = datetime.fromisoformat(e["start_time"].replace("Z", "+00:00"))
        end_dt = datetime.fromisoformat(e["end_time"].replace("Z", "+00:00"))
        duration = (end_dt - start_dt).total_seconds() / 3600

        lines.append(f"## {e['title']}")
        lines.append(f"- **ID**: `{e['id']}`")
        lines.append(f"- **Date**: {start_dt.strftime('%Y-%m-%d %H:%M')}")
        lines.append(f"- **Duration**: {format_hours(duration)}")

        if e.get("attendees"):
            attendees = e["attendees"][:5]  # Limit displayed attendees
            lines.append(f"- **Attendees**: {', '.join(attendees)}")
            if len(e["attendees"]) > 5:
                lines.append(f"  _(and {len(e['attendees']) - 5} more)_")

        if e.get("calendar_name"):
            lines.append(f"- **Calendar**: {e['calendar_name']}")

        if e.get("description"):
            desc = e["description"][:200]
            if len(e["description"]) > 200:
                desc += "..."
            lines.append(f"- **Description**: {desc}")

        lines.append("")

    if len(events) < len(api.list_calendar_events(start_date=start, end_date=end, classification_status="pending")):
        lines.append(f"_Showing first {limit} events. Use a narrower date range for more._")

    return "\n".join(lines)


@mcp.tool()
def classify_event(event_id: str, project_id: str | None = None, skip: bool = False) -> str:
    """Classify a calendar event by assigning it to a project or skipping it.

    This creates a time entry for the event's duration when assigned to a project.
    Use skip=True for events that shouldn't count as work time.

    Args:
        event_id: The calendar event ID to classify
        project_id: Project ID to assign (use list_projects to find IDs)
        skip: Set True to mark as "did not attend" (no time entry created)
    """
    if not project_id and not skip:
        return "Error: Must provide either project_id or skip=True"

    api = get_api()

    try:
        result = api.classify_event(event_id, project_id=project_id, skip=skip)
    except TimesheetAPIError as e:
        return f"Error classifying event: {e.message}"

    event = result["event"]

    if skip:
        return f"Skipped event: **{event['title']}**"

    time_entry = result.get("time_entry")
    if time_entry:
        return (
            f"Classified event: **{event['title']}**\n"
            f"- Project: {time_entry.get('project', {}).get('name', project_id)}\n"
            f"- Hours: {format_hours(time_entry['hours'])}\n"
            f"- Date: {time_entry['date']}"
        )

    return f"Classified event: **{event['title']}** to project {project_id}"


@mcp.tool()
def bulk_classify_events(query: str, project_id: str | None = None, skip: bool = False) -> str:
    """Classify multiple events matching a search query.

    Uses Gmail-style query syntax to match events. Examples:
    - "domain:acme.com" - Events with acme.com attendees
    - "title:standup" - Events with "standup" in title
    - 'title:"weekly sync"' - Exact phrase match
    - "calendar:Work" - Events from Work calendar

    Args:
        query: Gmail-style search query
        project_id: Project ID to assign matching events
        skip: Set True to mark matching events as skipped
    """
    if not project_id and not skip:
        return "Error: Must provide either project_id or skip=True"

    api = get_api()

    try:
        result = api.bulk_classify_events(query, project_id=project_id, skip=skip)
    except TimesheetAPIError as e:
        return f"Error: {e.message}"

    classified = result.get("classified_count", 0)
    skipped = result.get("skipped_count", 0)
    entries = result.get("time_entries_created", 0)

    if classified == 0 and skipped == 0:
        return f"No events matched query: `{query}`"

    lines = [f"# Bulk Classification Results\n"]
    lines.append(f"Query: `{query}`\n")

    if classified > 0:
        lines.append(f"- Events classified: {classified}")
        lines.append(f"- Time entries created: {entries}")
    if skipped > 0:
        lines.append(f"- Events skipped: {skipped}")

    return "\n".join(lines)


@mcp.tool()
def create_time_entry(
    project_id: str,
    date_str: str,
    hours: float,
    description: str | None = None,
) -> str:
    """Create a manual time entry.

    Use this for logging time that wasn't captured by calendar events.

    Args:
        project_id: Project ID (use list_projects to find)
        date_str: Date in YYYY-MM-DD format
        hours: Number of hours (e.g., 1.5 for 1h 30m)
        description: Optional description of work done
    """
    api = get_api()

    try:
        entry = api.create_time_entry(
            project_id=project_id,
            date_str=date_str,
            hours=hours,
            description=description,
        )
    except TimesheetAPIError as e:
        return f"Error creating time entry: {e.message}"

    project_name = entry.get("project", {}).get("name", project_id)
    return (
        f"Created time entry:\n"
        f"- Project: {project_name}\n"
        f"- Date: {entry['date']}\n"
        f"- Hours: {format_hours(entry['hours'])}"
        + (f"\n- Description: {description}" if description else "")
    )


@mcp.tool()
def search_events(
    query: str,
    start_date: str | None = None,
    end_date: str | None = None,
    include_classified: bool = False,
) -> str:
    """Search calendar events using query syntax.

    Useful for finding events to classify in bulk or analyze patterns.

    Query syntax (Gmail-style):
    - title:word - Match word in title
    - title:"exact phrase" - Match exact phrase
    - domain:example.com - Attendee email domain
    - attendee:email@example.com - Specific attendee
    - calendar:CalendarName - Specific calendar
    - Combine with spaces: title:meeting domain:acme.com

    Args:
        query: Search query
        start_date: Start date (YYYY-MM-DD). Defaults to 30 days ago.
        end_date: End date (YYYY-MM-DD). Defaults to today.
        include_classified: Include already-classified events
    """
    api = get_api()
    start = parse_date(start_date, default_days_ago=30)
    end = parse_date(end_date, default_days_ago=0)

    try:
        result = api.preview_rule(query, start_date=start, end_date=end)
    except TimesheetAPIError as e:
        return f"Error searching: {e.message}"

    matches = result.get("matches", [])
    stats = result.get("stats", {})

    if not matches:
        return f"No events matched query: `{query}`"

    lines = [f"# Search Results for `{query}`\n"]
    lines.append(f"Found {stats.get('total_matches', len(matches))} matching events\n")

    for m in matches[:20]:
        start_dt = datetime.fromisoformat(m["start_time"].replace("Z", "+00:00"))
        lines.append(f"- **{m['title']}** ({start_dt.strftime('%Y-%m-%d')})")
        lines.append(f"  - ID: `{m['event_id']}`")

    if len(matches) > 20:
        lines.append(f"\n_Showing first 20 of {len(matches)} matches_")

    return "\n".join(lines)


@mcp.tool()
def list_rules() -> str:
    """List all classification rules.

    Rules automatically classify new calendar events based on patterns.
    """
    api = get_api()
    rules = api.list_rules(include_disabled=True)

    if not rules:
        return "No classification rules defined."

    lines = ["# Classification Rules\n"]

    for r in rules:
        status = "" if r.get("is_enabled") else " (disabled)"
        action = f"-> {r.get('project_name', 'Unknown')}" if r.get("project_id") else "-> Skip"
        if r.get("attended") is False:
            action = "-> Mark as not attended"

        lines.append(f"- `{r['query']}`{status}")
        lines.append(f"  - Action: {action}")
        lines.append(f"  - Weight: {r.get('weight', 1.0)}")
        lines.append(f"  - ID: `{r['id']}`")
        lines.append("")

    return "\n".join(lines)


@mcp.tool()
def sync_calendar(connection_id: str | None = None) -> str:
    """Trigger a calendar sync to fetch new events.

    Syncs events from connected Google Calendar accounts.

    Args:
        connection_id: Specific connection to sync (syncs all if not provided)
    """
    api = get_api()

    if connection_id:
        try:
            result = api.sync_calendar(connection_id)
            return (
                f"Calendar sync complete:\n"
                f"- Events created: {result.get('events_created', 0)}\n"
                f"- Events updated: {result.get('events_updated', 0)}\n"
                f"- Events orphaned: {result.get('events_orphaned', 0)}"
            )
        except TimesheetAPIError as e:
            return f"Error syncing calendar: {e.message}"

    # Sync all connections
    connections = api.list_calendar_connections()
    if not connections:
        return "No calendar connections found. Connect a Google Calendar first."

    results = []
    for conn in connections:
        try:
            result = api.sync_calendar(conn["id"])
            results.append(
                f"- {conn.get('provider', 'Unknown')}: "
                f"{result.get('events_created', 0)} new, "
                f"{result.get('events_updated', 0)} updated"
            )
        except TimesheetAPIError as e:
            results.append(f"- {conn.get('provider', 'Unknown')}: Error - {e.message}")

    return "# Calendar Sync Results\n\n" + "\n".join(results)


# Entry point
if __name__ == "__main__":
    transport = os.environ.get("MCP_TRANSPORT", "stdio")

    if transport == "http":
        host = os.environ.get("MCP_HOST", "127.0.0.1")
        port = int(os.environ.get("MCP_PORT", "3001"))
        mcp.run(transport="sse", host=host, port=port)
    else:
        mcp.run(transport="stdio")
