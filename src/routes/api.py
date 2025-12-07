"""JSON API routes."""

from datetime import datetime
from fastapi import APIRouter, HTTPException, Depends
from fastapi.responses import StreamingResponse
import io

from db import get_db
from models import (
    Project, ProjectCreate, ProjectUpdate, ProjectVisibility,
    Event, TimeEntry, TimeEntryCreate, TimeEntryUpdate, BulkClassifyRequest,
    SyncRequest, SyncResponse,
)
from services.calendar import sync_calendar_events
from services.exporter import export_harvest_csv
from routes.auth import get_stored_credentials

router = APIRouter()


def require_auth():
    """Dependency that requires authentication."""
    credentials = get_stored_credentials()
    if credentials is None:
        raise HTTPException(status_code=401, detail="Not authenticated")
    return credentials


# --- Projects ---

@router.get("/projects", response_model=list[Project])
async def list_projects():
    """List all projects."""
    db = get_db()
    rows = db.execute("SELECT * FROM projects ORDER BY name")
    return [
        Project(
            id=row["id"],
            name=row["name"],
            client=row["client"],
            color=row["color"] or "#00aa44",
            is_visible=bool(row["is_visible"]),
            created_at=row["created_at"],
        )
        for row in rows
    ]


@router.post("/projects", response_model=Project)
async def create_project(project: ProjectCreate):
    """Create a new project."""
    db = get_db()
    try:
        project_id = db.execute_insert(
            "INSERT INTO projects (name, client, color) VALUES (?, ?, ?)",
            (project.name, project.client, project.color),
        )
    except Exception as e:
        if "UNIQUE constraint" in str(e):
            raise HTTPException(status_code=400, detail="Project name already exists")
        raise

    row = db.execute_one("SELECT * FROM projects WHERE id = ?", (project_id,))
    return Project(
        id=row["id"],
        name=row["name"],
        client=row["client"],
        color=row["color"] or "#00aa44",
        is_visible=bool(row["is_visible"]),
        created_at=row["created_at"],
    )


@router.put("/projects/{project_id}", response_model=Project)
async def update_project(project_id: int, project: ProjectUpdate):
    """Update a project."""
    db = get_db()
    row = db.execute_one("SELECT * FROM projects WHERE id = ?", (project_id,))
    if row is None:
        raise HTTPException(status_code=404, detail="Project not found")

    updates = []
    params = []
    if project.name is not None:
        updates.append("name = ?")
        params.append(project.name)
    if project.client is not None:
        updates.append("client = ?")
        params.append(project.client)
    if project.color is not None:
        updates.append("color = ?")
        params.append(project.color)

    if updates:
        params.append(project_id)
        db.execute(
            f"UPDATE projects SET {', '.join(updates)} WHERE id = ?",
            tuple(params),
        )

    row = db.execute_one("SELECT * FROM projects WHERE id = ?", (project_id,))
    return Project(
        id=row["id"],
        name=row["name"],
        client=row["client"],
        color=row["color"] or "#00aa44",
        is_visible=bool(row["is_visible"]),
        created_at=row["created_at"],
    )


@router.delete("/projects/{project_id}")
async def delete_project(project_id: int):
    """Delete a project."""
    db = get_db()
    row = db.execute_one("SELECT * FROM projects WHERE id = ?", (project_id,))
    if row is None:
        raise HTTPException(status_code=404, detail="Project not found")

    db.execute("DELETE FROM projects WHERE id = ?", (project_id,))
    return {"status": "deleted"}


@router.put("/projects/{project_id}/visibility", response_model=Project)
async def update_project_visibility(project_id: int, visibility: ProjectVisibility):
    """Toggle project visibility."""
    db = get_db()
    row = db.execute_one("SELECT * FROM projects WHERE id = ?", (project_id,))
    if row is None:
        raise HTTPException(status_code=404, detail="Project not found")

    db.execute(
        "UPDATE projects SET is_visible = ? WHERE id = ?",
        (1 if visibility.is_visible else 0, project_id),
    )

    row = db.execute_one("SELECT * FROM projects WHERE id = ?", (project_id,))
    return Project(
        id=row["id"],
        name=row["name"],
        client=row["client"],
        color=row["color"] or "#00aa44",
        is_visible=bool(row["is_visible"]),
        created_at=row["created_at"],
    )


# --- Calendar Sync ---

@router.post("/sync", response_model=SyncResponse)
async def sync_events(request: SyncRequest, credentials=Depends(require_auth)):
    """Sync events from Google Calendar."""
    result = sync_calendar_events(
        credentials=credentials,
        start_date=request.start_date,
        end_date=request.end_date,
    )
    return SyncResponse(**result)


# --- Events ---

@router.get("/events")
async def list_events(start_date: str, end_date: str):
    """List events for a date range."""
    db = get_db()
    rows = db.execute(
        """
        SELECT e.*, te.id as entry_id, te.project_id, te.hours, te.description as entry_description,
               te.classified_at, te.classification_source, p.name as project_name
        FROM events e
        LEFT JOIN time_entries te ON e.id = te.event_id
        LEFT JOIN projects p ON te.project_id = p.id
        WHERE date(e.start_time) >= ? AND date(e.start_time) <= ?
        ORDER BY e.start_time
        """,
        (start_date, end_date),
    )

    events = []
    for row in rows:
        import json
        attendees = json.loads(row["attendees"]) if row["attendees"] else []

        time_entry = None
        if row["entry_id"]:
            time_entry = TimeEntry(
                id=row["entry_id"],
                event_id=row["id"],
                project_id=row["project_id"],
                project_name=row["project_name"],
                hours=row["hours"],
                description=row["entry_description"],
                classified_at=row["classified_at"],
                classification_source=row["classification_source"],
            )

        events.append(Event(
            id=row["id"],
            google_event_id=row["google_event_id"],
            calendar_id=row["calendar_id"],
            title=row["title"],
            description=row["description"],
            start_time=row["start_time"],
            end_time=row["end_time"],
            attendees=attendees,
            meeting_link=row["meeting_link"],
            event_color=row["event_color"],
            is_recurring=bool(row["is_recurring"]),
            time_entry=time_entry,
        ))

    return events


# --- Time Entries ---

@router.post("/entries", response_model=TimeEntry)
async def create_entry(entry: TimeEntryCreate):
    """Create a time entry (classify an event)."""
    db = get_db()

    # Check event exists
    event = db.execute_one("SELECT * FROM events WHERE id = ?", (entry.event_id,))
    if event is None:
        raise HTTPException(status_code=404, detail="Event not found")

    # Check project exists
    project = db.execute_one("SELECT * FROM projects WHERE id = ?", (entry.project_id,))
    if project is None:
        raise HTTPException(status_code=404, detail="Project not found")

    # Upsert time entry
    db.execute(
        """
        INSERT INTO time_entries (event_id, project_id, hours, description, classification_source)
        VALUES (?, ?, ?, ?, 'manual')
        ON CONFLICT(event_id) DO UPDATE SET
            project_id = excluded.project_id,
            hours = excluded.hours,
            description = excluded.description,
            classification_source = excluded.classification_source,
            classified_at = CURRENT_TIMESTAMP
        """,
        (entry.event_id, entry.project_id, entry.hours, entry.description),
    )

    row = db.execute_one(
        """
        SELECT te.*, p.name as project_name
        FROM time_entries te
        JOIN projects p ON te.project_id = p.id
        WHERE te.event_id = ?
        """,
        (entry.event_id,),
    )

    return TimeEntry(
        id=row["id"],
        event_id=row["event_id"],
        project_id=row["project_id"],
        project_name=row["project_name"],
        hours=row["hours"],
        description=row["description"],
        classified_at=row["classified_at"],
        classification_source=row["classification_source"],
    )


@router.put("/entries/{entry_id}", response_model=TimeEntry)
async def update_entry(entry_id: int, entry: TimeEntryUpdate):
    """Update a time entry."""
    db = get_db()

    row = db.execute_one("SELECT * FROM time_entries WHERE id = ?", (entry_id,))
    if row is None:
        raise HTTPException(status_code=404, detail="Time entry not found")

    updates = []
    params = []
    if entry.project_id is not None:
        updates.append("project_id = ?")
        params.append(entry.project_id)
    if entry.hours is not None:
        updates.append("hours = ?")
        params.append(entry.hours)
    if entry.description is not None:
        updates.append("description = ?")
        params.append(entry.description)

    if updates:
        params.append(entry_id)
        db.execute(
            f"UPDATE time_entries SET {', '.join(updates)} WHERE id = ?",
            tuple(params),
        )

    row = db.execute_one(
        """
        SELECT te.*, p.name as project_name
        FROM time_entries te
        JOIN projects p ON te.project_id = p.id
        WHERE te.id = ?
        """,
        (entry_id,),
    )

    return TimeEntry(
        id=row["id"],
        event_id=row["event_id"],
        project_id=row["project_id"],
        project_name=row["project_name"],
        hours=row["hours"],
        description=row["description"],
        classified_at=row["classified_at"],
        classification_source=row["classification_source"],
    )


@router.delete("/entries/{entry_id}")
async def delete_entry(entry_id: int):
    """Delete a time entry (unclassify event)."""
    db = get_db()

    row = db.execute_one("SELECT * FROM time_entries WHERE id = ?", (entry_id,))
    if row is None:
        raise HTTPException(status_code=404, detail="Time entry not found")

    db.execute("DELETE FROM time_entries WHERE id = ?", (entry_id,))
    return {"status": "deleted"}


@router.post("/entries/bulk")
async def bulk_classify(request: BulkClassifyRequest):
    """Classify multiple events at once."""
    db = get_db()

    # Check project exists
    project = db.execute_one("SELECT * FROM projects WHERE id = ?", (request.project_id,))
    if project is None:
        raise HTTPException(status_code=404, detail="Project not found")

    classified = 0
    for event_id in request.event_ids:
        event = db.execute_one("SELECT * FROM events WHERE id = ?", (event_id,))
        if event is None:
            continue

        # Calculate hours from event duration
        start = datetime.fromisoformat(event["start_time"])
        end = datetime.fromisoformat(event["end_time"])
        hours = (end - start).total_seconds() / 3600

        db.execute(
            """
            INSERT INTO time_entries (event_id, project_id, hours, description, classification_source)
            VALUES (?, ?, ?, ?, 'manual')
            ON CONFLICT(event_id) DO UPDATE SET
                project_id = excluded.project_id,
                hours = excluded.hours,
                classification_source = excluded.classification_source,
                classified_at = CURRENT_TIMESTAMP
            """,
            (event_id, request.project_id, hours, event["title"]),
        )
        classified += 1

    return {"classified": classified}


# --- Export ---

@router.get("/export/harvest")
async def export_harvest(start_date: str, end_date: str):
    """Export time entries as Harvest-compatible CSV."""
    csv_content = export_harvest_csv(start_date, end_date)

    return StreamingResponse(
        io.StringIO(csv_content),
        media_type="text/csv",
        headers={"Content-Disposition": f"attachment; filename=timesheet_{start_date}_{end_date}.csv"},
    )
