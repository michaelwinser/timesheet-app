"""UI routes for server-rendered HTML pages."""

from datetime import datetime, timedelta
from fastapi import APIRouter, Request
from fastapi.responses import HTMLResponse, RedirectResponse
from fastapi.templating import Jinja2Templates
from pathlib import Path

from db import get_db
from routes.auth import get_stored_credentials

router = APIRouter()

templates = Jinja2Templates(directory=Path(__file__).parent.parent / "templates")


@router.get("/", response_class=HTMLResponse)
async def index(request: Request):
    """Redirect to current week."""
    today = datetime.now().date()
    # Find Monday of current week
    monday = today - timedelta(days=today.weekday())
    return RedirectResponse(url=f"/week/{monday.isoformat()}")


@router.get("/week/{date}", response_class=HTMLResponse)
async def week_view(request: Request, date: str):
    """Display week view centered on the given date."""
    credentials = get_stored_credentials()
    if credentials is None:
        return templates.TemplateResponse(
            "login.html",
            {"request": request},
        )

    # Parse date and find Monday of that week
    try:
        target_date = datetime.strptime(date, "%Y-%m-%d").date()
    except ValueError:
        target_date = datetime.now().date()

    monday = target_date - timedelta(days=target_date.weekday())
    sunday = monday + timedelta(days=6)

    # Get events for the week
    db = get_db()
    rows = db.execute(
        """
        SELECT e.*, te.id as entry_id, te.project_id, te.hours, te.description as entry_description,
               te.classified_at, te.classification_source, p.name as project_name, p.color as project_color
        FROM events e
        LEFT JOIN time_entries te ON e.id = te.event_id
        LEFT JOIN projects p ON te.project_id = p.id
        WHERE date(e.start_time) >= ? AND date(e.start_time) <= ?
        ORDER BY e.start_time
        """,
        (monday.isoformat(), sunday.isoformat()),
    )

    # Organize events by day
    import json
    days = []
    for i in range(7):
        day_date = monday + timedelta(days=i)
        day_events = []
        for row in rows:
            event_date = datetime.fromisoformat(row["start_time"]).date()
            if event_date == day_date:
                attendees = json.loads(row["attendees"]) if row["attendees"] else []
                day_events.append({
                    "id": row["id"],
                    "google_event_id": row["google_event_id"],
                    "title": row["title"] or "Untitled",
                    "description": row["description"],
                    "start_time": row["start_time"],
                    "end_time": row["end_time"],
                    "attendees": attendees,
                    "meeting_link": row["meeting_link"],
                    "is_classified": row["entry_id"] is not None,
                    "entry_id": row["entry_id"],
                    "project_id": row["project_id"],
                    "project_name": row["project_name"],
                    "project_color": row["project_color"] or "#00aa44",
                    "hours": row["hours"],
                    "entry_description": row["entry_description"],
                })
        days.append({
            "date": day_date,
            "day_name": day_date.strftime("%a"),
            "day_number": day_date.day,
            "is_today": day_date == datetime.now().date(),
            "events": day_events,
        })

    # Get projects for classification dropdown
    projects = db.execute("SELECT * FROM projects WHERE is_visible = 1 ORDER BY name")
    projects = [dict(row) for row in projects]

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
        },
    )


@router.get("/projects", response_class=HTMLResponse)
async def projects_page(request: Request):
    """Project management page."""
    credentials = get_stored_credentials()
    if credentials is None:
        return RedirectResponse(url="/auth/login")

    db = get_db()
    projects = db.execute("SELECT * FROM projects ORDER BY name")
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
    credentials = get_stored_credentials()
    if credentials is None:
        return RedirectResponse(url="/auth/login")

    db = get_db()

    # Get projects for the dropdown
    projects = db.execute("SELECT * FROM projects ORDER BY name")
    projects = [dict(row) for row in projects]

    # Get rules with their conditions and project info
    from services.classifier import load_rules_with_conditions
    rules = load_rules_with_conditions(db, enabled_only=False)

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
