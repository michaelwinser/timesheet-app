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
               te.classified_at, te.classification_source, te.invoice_id,
               p.name as project_name, p.color as project_color,
               inv.invoice_number
        FROM events e
        LEFT JOIN time_entries te ON e.id = te.event_id
        LEFT JOIN projects p ON te.project_id = p.id
        LEFT JOIN invoices inv ON te.invoice_id = inv.id
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
                # Calculate event duration from start/end times
                start_dt = start_time if isinstance(start_time, datetime) else datetime.fromisoformat(start_time)
                end_dt = end_time_val if isinstance(end_time_val, datetime) else datetime.fromisoformat(end_time_val)
                event_duration = (end_dt - start_dt).total_seconds() / 3600  # hours

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
                    "duration": event_duration,  # Always available, even for unclassified
                    "entry_description": row["entry_description"],
                    "did_not_attend": bool(row.get("did_not_attend", False)),
                    "my_response_status": row.get("my_response_status"),
                    "invoice_id": row.get("invoice_id"),
                    "invoice_number": row.get("invoice_number"),
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

    # Filter to non-archived projects for dropdown
    projects = [p for p in all_projects if not p.get("is_archived", False)]

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
            "is_hidden_by_default": p.get("is_hidden_by_default", False),
            "is_archived": p.get("is_archived", False),
            "does_not_accumulate_hours": p.get("does_not_accumulate_hours", False),
        }

        if p.get("is_archived", False):
            # Only include archived projects if they have hours in current view
            if hours > 0:
                archived_projects.append(summary_item)
        elif p.get("is_hidden_by_default", False):
            # Only include hidden projects if they have hours in current view
            if hours > 0:
                hidden_projects.append(summary_item)
        else:
            # Active projects always show (even with 0 hours)
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


@router.get("/projects/{project_id}", response_class=HTMLResponse)
async def project_detail_page(request: Request, project_id: int):
    """Project detail page with fingerprint settings."""
    user_id = get_user_id(request)

    db = get_db()

    # Get the project
    project = db.execute_one(
        "SELECT * FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id)
    )

    if not project:
        return RedirectResponse(url="/projects", status_code=302)

    project = dict(project)

    # Parse fingerprint JSON fields
    import json
    project["fingerprint_domains"] = json.loads(project.get("fingerprint_domains") or "[]")
    project["fingerprint_emails"] = json.loads(project.get("fingerprint_emails") or "[]")
    project["fingerprint_keywords"] = json.loads(project.get("fingerprint_keywords") or "[]")

    # Get project statistics
    stats = db.execute_one(
        """
        SELECT
            COALESCE(SUM(CASE WHEN DATE(e.start_time) >= DATE_TRUNC('week', CURRENT_DATE) THEN te.hours ELSE 0 END), 0) as week_hours,
            COALESCE(SUM(CASE WHEN DATE(e.start_time) >= DATE_TRUNC('month', CURRENT_DATE) THEN te.hours ELSE 0 END), 0) as month_hours,
            COALESCE(SUM(te.hours), 0) as total_hours
        FROM time_entries te
        JOIN events e ON te.event_id = e.id
        WHERE te.project_id = %s AND te.user_id = %s
        """,
        (project_id, user_id)
    )

    # Get custom rules for this project
    rules = db.execute(
        """
        SELECT * FROM classification_rules
        WHERE user_id = %s AND project_id = %s AND is_generated = FALSE
        ORDER BY display_order, priority DESC
        """,
        (user_id, project_id)
    )
    rules = [dict(r) for r in rules]

    return templates.TemplateResponse(
        "project_detail.html",
        {
            "request": request,
            "project": project,
            "stats": dict(stats) if stats else {"week_hours": 0, "month_hours": 0, "total_hours": 0},
            "rules": rules,
        },
    )


@router.get("/rules", response_class=HTMLResponse)
async def rules_page(request: Request):
    """Rule management page with query-based rules grouped by target."""
    user_id = get_user_id(request)

    db = get_db()

    # Get non-archived projects for the dropdown
    projects = db.execute(
        "SELECT * FROM projects WHERE user_id = %s AND is_archived = FALSE ORDER BY name",
        (user_id,)
    )
    projects = [dict(row) for row in projects]

    # Get all rules
    rules = db.execute(
        """
        SELECT cr.*, p.name as project_name, p.color as project_color
        FROM classification_rules cr
        LEFT JOIN projects p ON cr.project_id = p.id
        WHERE cr.user_id = %s
        ORDER BY cr.project_id, cr.display_order, cr.priority DESC
        """,
        (user_id,)
    )
    rules = [dict(r) for r in rules]

    # Group rules by target (project or DNA)
    project_groups = {}  # project_id -> group
    dna_rules = []

    for rule in rules:
        if rule.get("target_type") == "did_not_attend":
            dna_rules.append(rule)
        elif rule.get("project_id"):
            pid = rule["project_id"]
            if pid not in project_groups:
                project_groups[pid] = {
                    "id": pid,
                    "type": "project",
                    "name": rule.get("project_name") or "Unknown Project",
                    "color": rule.get("project_color") or "#00aa44",
                    "rules": [],
                }
            project_groups[pid]["rules"].append(rule)

    # Build rule_groups list: projects first, then DNA
    rule_groups = list(project_groups.values())

    # Sort project groups by name
    rule_groups.sort(key=lambda g: g["name"].lower())

    # Add DNA group if there are DNA rules
    if dna_rules:
        rule_groups.append({
            "id": "dna",
            "type": "dna",
            "name": "Did Not Attend",
            "color": None,
            "rules": dna_rules,
        })

    return templates.TemplateResponse(
        "rules.html",
        {
            "request": request,
            "rule_groups": rule_groups,
            "projects": projects,
        },
    )


@router.get("/invoices", response_class=HTMLResponse)
async def invoices_page(request: Request):
    """Invoice management page."""
    user_id = get_user_id(request)

    db = get_db()

    # Get billable projects for the create invoice dropdown
    billable_projects = db.execute(
        """SELECT * FROM projects
           WHERE user_id = %s AND is_billable = TRUE AND is_archived = FALSE
           ORDER BY name""",
        (user_id,)
    )
    billable_projects = [dict(row) for row in billable_projects]

    return templates.TemplateResponse(
        "invoices.html",
        {
            "request": request,
            "billable_projects": billable_projects,
        },
    )
