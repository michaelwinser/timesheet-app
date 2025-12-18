"""UI routes for server-rendered HTML pages with multi-user support."""

from datetime import datetime, timedelta
from fastapi import APIRouter, Request
from fastapi.responses import HTMLResponse, RedirectResponse
from fastapi.templating import Jinja2Templates
from pathlib import Path

from db import get_db

router = APIRouter()


def get_user_id(request: Request) -> int | None:
    """Get user_id from request state (set by UserContextMiddleware)."""
    return getattr(request.state, 'user_id', None)

templates = Jinja2Templates(directory=Path(__file__).parent.parent / "templates")


@router.get("/login", response_class=HTMLResponse)
async def login_page(request: Request, next: str = None):
    """Login page with 'Login with Google' button."""
    return templates.TemplateResponse(
        "login.html",
        {
            "request": request,
            "next": next,
        },
    )


@router.get("/", response_class=HTMLResponse)
async def index(request: Request):
    """Show current week (same as clicking Today button)."""
    today = datetime.now().date()
    # Find Monday of current week
    monday = today - timedelta(days=today.weekday())
    # Forward to week view with current week's Monday
    return await week_view(request, monday.isoformat())


@router.get("/week/{date}", response_class=HTMLResponse)
async def week_view(request: Request, date: str):
    """Display week view centered on the given date."""
    # Authentication is enforced by middleware, no need to check here
    user_id = get_user_id(request)

    # Parse date and find Monday of that week
    try:
        target_date = datetime.strptime(date, "%Y-%m-%d").date()
    except ValueError:
        target_date = datetime.now().date()

    monday = target_date - timedelta(days=target_date.weekday())
    sunday = monday + timedelta(days=6)

    # Get events for the week (filtered by user_id)
    db = get_db()
    rows = db.execute(
        """
        SELECT e.*, te.id as entry_id, te.project_id, te.hours, te.description as entry_description,
               te.classified_at, te.classification_source, p.name as project_name, p.color as project_color
        FROM events e
        LEFT JOIN time_entries te ON e.id = te.event_id
        LEFT JOIN projects p ON te.project_id = p.id
        WHERE e.user_id = %s AND DATE(e.start_time) >= %s AND DATE(e.start_time) <= %s
        ORDER BY e.start_time
        """,
        (user_id, monday.isoformat(), sunday.isoformat()),
    )

    # Organize events by day
    import json
    days = []
    for i in range(7):
        day_date = monday + timedelta(days=i)
        day_events = []
        for row in rows:
            start_time = row["start_time"]
            event_date = start_time.date() if isinstance(start_time, datetime) else datetime.fromisoformat(start_time).date()
            if event_date == day_date:
                attendees = json.loads(row["attendees"]) if row["attendees"] else []
                # Convert datetime objects to ISO strings for template
                start_str = start_time.isoformat() if isinstance(start_time, datetime) else start_time
                end_time_val = row["end_time"]
                end_str = end_time_val.isoformat() if isinstance(end_time_val, datetime) else end_time_val
                day_events.append({
                    "id": row["id"],
                    "google_event_id": row["google_event_id"],
                    "title": row["title"] or "Untitled",
                    "description": row["description"],
                    "start_time": start_str,
                    "end_time": end_str,
                    "attendees": attendees,
                    "meeting_link": row["meeting_link"],
                    "is_classified": row["entry_id"] is not None,
                    "entry_id": row["entry_id"],
                    "project_id": row["project_id"],
                    "project_name": row["project_name"],
                    "project_color": row["project_color"] or "#00aa44",
                    "hours": row["hours"],
                    "entry_description": row["entry_description"],
                    "did_not_attend": bool(row.get("did_not_attend", False)),
                    "my_response_status": row.get("my_response_status"),
                })
        days.append({
            "date": day_date,
            "day_name": day_date.strftime("%a"),
            "day_number": day_date.day,
            "is_today": day_date == datetime.now().date(),
            "events": day_events,
        })

    # Get all projects (visible and hidden) for summary
    all_projects = db.execute(
        "SELECT * FROM projects WHERE user_id = %s ORDER BY name",
        (user_id,)
    )
    all_projects = [dict(row) for row in all_projects]

    # Build project lookup for quick access
    project_lookup = {p["id"]: p for p in all_projects}

    # Filter to non-archived, visible projects for dropdown
    projects = [p for p in all_projects if p["is_visible"] and not p.get("is_archived", False)]

    # Calculate project hours summary for this week
    # Exclude did_not_attend events and does_not_accumulate_hours projects from totals
    project_hours = {}
    total_hours = 0.0
    for day in days:
        for event in day["events"]:
            if event["is_classified"] and event["hours"] and not event.get("did_not_attend", False):
                pid = event["project_id"]
                project = project_lookup.get(pid, {})
                project_hours[pid] = project_hours.get(pid, 0.0) + event["hours"]
                # Only add to total if project accumulates hours
                if not project.get("does_not_accumulate_hours", False):
                    total_hours += event["hours"]

    # Build project summary lists (grouped by type)
    regular_projects = []
    hidden_projects = []
    archived_projects = []

    for p in all_projects:
        hours = project_hours.get(p["id"], 0.0)
        summary_item = {
            "id": p["id"],
            "name": p["name"],
            "color": p["color"] or "#00aa44",
            "hours": hours,
            "is_visible": p["is_visible"],
            "is_hidden_by_default": p.get("is_hidden_by_default", False),
            "is_archived": p.get("is_archived", False),
            "does_not_accumulate_hours": p.get("does_not_accumulate_hours", False),
        }

        if p.get("is_archived", False):
            # Only include archived projects if they have hours in current view
            if hours > 0:
                archived_projects.append(summary_item)
        elif p.get("is_hidden_by_default", False):
            hidden_projects.append(summary_item)
        else:
            regular_projects.append(summary_item)

    # Sort each group by hours descending, then by name
    for group in [regular_projects, hidden_projects, archived_projects]:
        group.sort(key=lambda x: (-x["hours"], x["name"]))

    # Legacy project_summary for compatibility (all non-archived projects)
    project_summary = regular_projects + hidden_projects

    # Calculate prev/next week dates
    prev_week = (monday - timedelta(days=7)).isoformat()
    next_week = (monday + timedelta(days=7)).isoformat()

    return templates.TemplateResponse(
        "week.html",
        {
            "request": request,
            "week_start": monday,
            "week_end": sunday,
            "days": days,
            "projects": projects,
            "prev_week": prev_week,
            "next_week": next_week,
            "project_summary": project_summary,
            "regular_projects": regular_projects,
            "hidden_projects": hidden_projects,
            "archived_projects": archived_projects,
            "total_hours": total_hours,
        },
    )


@router.get("/projects", response_class=HTMLResponse)
async def projects_page(request: Request):
    """Project management page."""
    # Authentication is enforced by middleware, no need to check here
    user_id = get_user_id(request)

    db = get_db()
    projects = db.execute(
        "SELECT * FROM projects WHERE user_id = %s ORDER BY name",
        (user_id,)
    )
    projects = [dict(row) for row in projects]

    return templates.TemplateResponse(
        "projects.html",
        {
            "request": request,
            "projects": projects,
        },
    )


@router.get("/rules", response_class=HTMLResponse)
async def rules_page(request: Request):
    """Rule management page."""
    # Authentication is enforced by middleware, no need to check here
    user_id = get_user_id(request)

    db = get_db()

    # Get projects for the dropdown
    projects = db.execute(
        "SELECT * FROM projects WHERE user_id = %s ORDER BY name",
        (user_id,)
    )
    projects = [dict(row) for row in projects]

    # Get rules with their conditions and project info
    from services.classifier import load_rules_with_conditions
    rules = load_rules_with_conditions(db, user_id, enabled_only=False)

    # Add project color to rules
    project_colors = {p["id"]: p["color"] for p in projects}
    rules_data = []
    for rule in rules:
        rule_dict = rule.to_dict()
        rule_dict["project_color"] = project_colors.get(rule.project_id, "#00aa44")
        rules_data.append(rule_dict)

    return templates.TemplateResponse(
        "rules.html",
        {
            "request": request,
            "rules": rules_data,
            "projects": projects,
        },
    )
