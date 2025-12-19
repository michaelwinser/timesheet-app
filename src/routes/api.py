"""JSON API routes with multi-user support."""

import logging
from datetime import datetime
from fastapi import APIRouter, HTTPException, Depends, Request
from fastapi.responses import StreamingResponse
from pydantic import BaseModel
import io

from db import get_db
from models import (
    Project, ProjectCreate, ProjectUpdate,
    Event, EventAttendance, TimeEntry, TimeEntryCreate, TimeEntryUpdate, BulkClassifyRequest,
    SyncRequest, SyncResponse,
    InvoiceCreate, InvoiceResponse, InvoiceListResponse, InvoiceLineItemResponse, InvoicePreview,
)
from services.calendar import sync_calendar_events
from services.exporter import export_harvest_csv
from services.classifier import get_event_properties
from routes.auth import get_stored_credentials

router = APIRouter()
logger = logging.getLogger(__name__)


def get_user_id(request: Request) -> int:
    """Dependency that extracts user_id from request state.

    Raises 401 if user not authenticated.
    """
    user_id = getattr(request.state, 'user_id', None)
    if user_id is None:
        raise HTTPException(status_code=401, detail="Not authenticated")
    return user_id


def require_auth(user_id: int = Depends(get_user_id)):
    """Dependency that requires authentication and returns credentials.

    Returns both credentials and user_id.
    """
    credentials = get_stored_credentials(user_id)
    if credentials is None:
        raise HTTPException(status_code=401, detail="Not authenticated")
    return credentials


# --- Projects ---

def _row_to_project(row: dict) -> Project:
    """Convert a database row to a Project model."""
    return Project(
        id=row["id"],
        name=row["name"],
        client=row["client"],
        color=row["color"] or "#00aa44",
        short_code=row.get("short_code"),
        does_not_accumulate_hours=bool(row.get("does_not_accumulate_hours", False)),
        is_billable=bool(row.get("is_billable", False)),
        bill_rate=row.get("bill_rate"),
        is_hidden_by_default=bool(row.get("is_hidden_by_default", False)),
        is_archived=bool(row.get("is_archived", False)),
        sheets_spreadsheet_id=row.get("sheets_spreadsheet_id"),
        sheets_spreadsheet_url=row.get("sheets_spreadsheet_url"),
        created_at=row["created_at"],
    )


@router.get("/projects", response_model=list[Project])
async def list_projects(user_id: int = Depends(get_user_id)):
    """List all projects for current user."""
    db = get_db()
    rows = db.execute(
        "SELECT * FROM projects WHERE user_id = %s ORDER BY name",
        (user_id,)
    )
    return [_row_to_project(row) for row in rows]


@router.post("/projects", response_model=Project)
async def create_project(project: ProjectCreate, user_id: int = Depends(get_user_id)):
    """Create a new project for current user."""
    db = get_db()
    try:
        project_id = db.execute_insert(
            """INSERT INTO projects (
                user_id, name, client, color, short_code,
                does_not_accumulate_hours, is_billable, bill_rate,
                is_hidden_by_default, is_archived
            ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s) RETURNING id""",
            (
                user_id, project.name, project.client, project.color, project.short_code,
                project.does_not_accumulate_hours, project.is_billable, project.bill_rate,
                project.is_hidden_by_default, project.is_archived,
            ),
        )
    except Exception as e:
        if "unique constraint" in str(e).lower():
            raise HTTPException(status_code=400, detail="Project name already exists")
        raise

    row = db.execute_one(
        "SELECT * FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id)
    )
    return _row_to_project(row)


@router.put("/projects/{project_id}", response_model=Project)
async def update_project(
    project_id: int,
    project: ProjectUpdate,
    user_id: int = Depends(get_user_id)
):
    """Update a project (user can only update their own projects)."""
    db = get_db()

    # Verify project belongs to user
    row = db.execute_one(
        "SELECT * FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id)
    )
    if row is None:
        raise HTTPException(status_code=404, detail="Project not found")

    updates = []
    params = []
    if project.name is not None:
        updates.append("name = %s")
        params.append(project.name)
    if project.client is not None:
        updates.append("client = %s")
        params.append(project.client)
    if project.color is not None:
        updates.append("color = %s")
        params.append(project.color)
    if project.short_code is not None:
        updates.append("short_code = %s")
        params.append(project.short_code if project.short_code else None)
    if project.does_not_accumulate_hours is not None:
        updates.append("does_not_accumulate_hours = %s")
        params.append(project.does_not_accumulate_hours)
    if project.is_billable is not None:
        updates.append("is_billable = %s")
        params.append(project.is_billable)
    if project.bill_rate is not None:
        updates.append("bill_rate = %s")
        params.append(project.bill_rate)
    if project.is_hidden_by_default is not None:
        updates.append("is_hidden_by_default = %s")
        params.append(project.is_hidden_by_default)
    if project.is_archived is not None:
        updates.append("is_archived = %s")
        params.append(project.is_archived)

    if updates:
        params.extend([project_id, user_id])
        db.execute(
            f"UPDATE projects SET {', '.join(updates)} WHERE id = %s AND user_id = %s",
            tuple(params),
        )

    row = db.execute_one(
        "SELECT * FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id)
    )
    return _row_to_project(row)


@router.delete("/projects/{project_id}")
async def delete_project(project_id: int, user_id: int = Depends(get_user_id)):
    """Delete a project (user can only delete their own projects)."""
    db = get_db()

    # Verify project belongs to user
    row = db.execute_one(
        "SELECT * FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id)
    )
    if row is None:
        raise HTTPException(status_code=404, detail="Project not found")

    db.execute(
        "DELETE FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id)
    )
    return {"status": "deleted"}


class FingerprintUpdate(BaseModel):
    """Update project fingerprint patterns."""
    domains: list[str] | None = None
    emails: list[str] | None = None
    keywords: list[str] | None = None


@router.put("/projects/{project_id}/fingerprint")
async def update_project_fingerprint(
    project_id: int,
    fingerprint: FingerprintUpdate,
    user_id: int = Depends(get_user_id)
):
    """Update a project's fingerprint patterns for auto-rule generation."""
    import json

    db = get_db()

    # Verify project belongs to user
    row = db.execute_one(
        "SELECT * FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id)
    )
    if row is None:
        raise HTTPException(status_code=404, detail="Project not found")

    updates = []
    params = []

    if fingerprint.domains is not None:
        updates.append("fingerprint_domains = %s")
        params.append(json.dumps(fingerprint.domains))
    if fingerprint.emails is not None:
        updates.append("fingerprint_emails = %s")
        params.append(json.dumps(fingerprint.emails))
    if fingerprint.keywords is not None:
        updates.append("fingerprint_keywords = %s")
        params.append(json.dumps(fingerprint.keywords))

    if updates:
        params.extend([project_id, user_id])
        db.execute(
            f"UPDATE projects SET {', '.join(updates)} WHERE id = %s AND user_id = %s",
            tuple(params),
        )

    return {"status": "updated"}


@router.post("/projects/{project_id}/spreadsheet")
async def create_project_spreadsheet(
    project_id: int,
    user_id: int = Depends(get_user_id),
    credentials=Depends(require_auth)
):
    """Create a new spreadsheet for a billable project."""
    from services.sheets import create_project_spreadsheet as do_create

    db = get_db()

    # Verify project belongs to user and is billable
    project = db.execute_one(
        "SELECT * FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id)
    )
    if project is None:
        raise HTTPException(status_code=404, detail="Project not found")

    if not project["is_billable"]:
        raise HTTPException(status_code=400, detail="Project is not billable")

    if project["sheets_spreadsheet_id"]:
        raise HTTPException(status_code=400, detail="Project already has a spreadsheet attached")

    try:
        spreadsheet_id, spreadsheet_url = do_create(
            credentials, project_id, project["name"], user_id
        )
        return {
            "spreadsheet_id": spreadsheet_id,
            "spreadsheet_url": spreadsheet_url,
            "name": f"{project['name']} - Invoices"
        }
    except Exception as e:
        logger.exception(f"Failed to create spreadsheet for project {project_id}")
        raise HTTPException(status_code=500, detail=str(e))


class ArchiveSpreadsheetRequest(BaseModel):
    archive_name: str


@router.post("/projects/{project_id}/spreadsheet/archive")
async def archive_project_spreadsheet(
    project_id: int,
    request: ArchiveSpreadsheetRequest,
    user_id: int = Depends(get_user_id),
    credentials=Depends(require_auth)
):
    """Archive (rename and detach) the project's spreadsheet."""
    from services.sheets import archive_project_spreadsheet as do_archive

    db = get_db()

    # Verify project belongs to user
    project = db.execute_one(
        "SELECT * FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id)
    )
    if project is None:
        raise HTTPException(status_code=404, detail="Project not found")

    if not project["sheets_spreadsheet_id"]:
        raise HTTPException(status_code=400, detail="No spreadsheet attached to this project")

    try:
        archived_url = do_archive(credentials, project_id, request.archive_name, user_id)
        return {
            "archived_spreadsheet_url": archived_url,
            "message": f"Spreadsheet archived as '{request.archive_name}'"
        }
    except Exception as e:
        logger.exception(f"Failed to archive spreadsheet for project {project_id}")
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/projects/export")
async def export_projects(user_id: int = Depends(get_user_id)):
    """Export all projects as JSON.

    Returns a JSON object with:
    - version: Export format version
    - exported_at: Timestamp
    - projects: List of project objects (without internal IDs)
    """
    db = get_db()

    rows = db.execute(
        "SELECT * FROM projects WHERE user_id = %s ORDER BY name",
        (user_id,)
    )

    projects = []
    for row in rows:
        projects.append({
            "name": row["name"],
            "client": row["client"],
            "color": row["color"],
            "does_not_accumulate_hours": bool(row.get("does_not_accumulate_hours", False)),
            "is_billable": bool(row.get("is_billable", False)),
            "bill_rate": row.get("bill_rate"),
            "is_hidden_by_default": bool(row.get("is_hidden_by_default", False)),
            "is_archived": bool(row.get("is_archived", False)),
        })

    return {
        "version": 1,
        "exported_at": datetime.now().isoformat(),
        "projects": projects,
    }


@router.post("/projects/import")
async def import_projects(request: dict, user_id: int = Depends(get_user_id)):
    """Import projects from JSON.

    Request body:
    {
        "version": 1,
        "projects": [
            {"name": "Project Name", "color": "#00aa44", ...}
        ],
        "mode": "merge"  // "merge" (default) or "replace"
    }

    In merge mode: Creates new projects, updates existing by name match
    In replace mode: Deletes all existing projects first, then imports

    Returns:
    {
        "imported": 5,
        "updated": 2,
        "skipped": 0
    }
    """
    db = get_db()

    projects = request.get("projects", [])
    mode = request.get("mode", "merge")

    if not projects:
        raise HTTPException(status_code=400, detail="No projects to import")

    if mode not in ("merge", "replace"):
        raise HTTPException(status_code=400, detail="mode must be 'merge' or 'replace'")

    imported = 0
    updated = 0
    skipped = 0

    if mode == "replace":
        # Delete all existing projects (cascade will handle time_entries and rules)
        db.execute("DELETE FROM projects WHERE user_id = %s", (user_id,))

    for proj in projects:
        name = proj.get("name", "").strip()
        if not name:
            skipped += 1
            continue

        # Check if project exists by name
        existing = db.execute_one(
            "SELECT id FROM projects WHERE user_id = %s AND name = %s",
            (user_id, name)
        )

        if existing and mode == "merge":
            # Update existing project
            db.execute(
                """
                UPDATE projects SET
                    client = %s,
                    color = %s,
                    does_not_accumulate_hours = %s,
                    is_billable = %s,
                    bill_rate = %s,
                    is_hidden_by_default = %s,
                    is_archived = %s
                WHERE id = %s AND user_id = %s
                """,
                (
                    proj.get("client"),
                    proj.get("color", "#00aa44"),
                    proj.get("does_not_accumulate_hours", False),
                    proj.get("is_billable", False),
                    proj.get("bill_rate"),
                    proj.get("is_hidden_by_default", False),
                    proj.get("is_archived", False),
                    existing["id"],
                    user_id,
                ),
            )
            updated += 1
        else:
            # Create new project
            db.execute_insert(
                """
                INSERT INTO projects (user_id, name, client, color,
                    does_not_accumulate_hours, is_billable, bill_rate,
                    is_hidden_by_default, is_archived)
                VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
                RETURNING id
                """,
                (
                    user_id,
                    name,
                    proj.get("client"),
                    proj.get("color", "#00aa44"),
                    proj.get("does_not_accumulate_hours", False),
                    proj.get("is_billable", False),
                    proj.get("bill_rate"),
                    proj.get("is_hidden_by_default", False),
                    proj.get("is_archived", False),
                ),
            )
            imported += 1

    return {
        "imported": imported,
        "updated": updated,
        "skipped": skipped,
    }


# --- Calendar Sync ---

@router.post("/sync", response_model=SyncResponse)
async def sync_events(
    request: SyncRequest,
    user_id: int = Depends(get_user_id),
    credentials=Depends(require_auth)
):
    """Sync events from Google Calendar for current user."""
    result = sync_calendar_events(
        credentials=credentials,
        start_date=request.start_date,
        end_date=request.end_date,
        user_id=user_id,  # Pass user_id to calendar service
    )
    return SyncResponse(**result)


# --- Events ---

@router.get("/events")
async def list_events(
    start_date: str,
    end_date: str,
    user_id: int = Depends(get_user_id)
):
    """List events for a date range (user's events only)."""
    db = get_db()
    rows = db.execute(
        """
        SELECT e.*, te.id as entry_id, te.project_id, te.hours, te.description as entry_description,
               te.classified_at, te.classification_source, p.name as project_name
        FROM events e
        LEFT JOIN time_entries te ON e.id = te.event_id
        LEFT JOIN projects p ON te.project_id = p.id
        WHERE e.user_id = %s AND DATE(e.start_time) >= %s AND DATE(e.start_time) <= %s
        ORDER BY e.start_time
        """,
        (user_id, start_date, end_date),
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
            did_not_attend=bool(row.get("did_not_attend", False)),
            my_response_status=row.get("my_response_status"),
            time_entry=time_entry,
        ))

    return events


@router.put("/events/{event_id}/attendance")
async def update_event_attendance(
    event_id: int,
    attendance: EventAttendance,
    user_id: int = Depends(get_user_id)
):
    """Update the did_not_attend flag for an event."""
    db = get_db()

    # Verify event belongs to user
    row = db.execute_one(
        "SELECT * FROM events WHERE id = %s AND user_id = %s",
        (event_id, user_id)
    )
    if row is None:
        raise HTTPException(status_code=404, detail="Event not found")

    db.execute(
        "UPDATE events SET did_not_attend = %s WHERE id = %s AND user_id = %s",
        (attendance.did_not_attend, event_id, user_id),
    )

    return {"status": "updated", "did_not_attend": attendance.did_not_attend}


# --- Time Entries ---

@router.post("/entries", response_model=TimeEntry)
async def create_entry(entry: TimeEntryCreate, user_id: int = Depends(get_user_id)):
    """Create a time entry (classify an event)."""
    db = get_db()

    # Check event exists and belongs to user
    event = db.execute_one(
        "SELECT * FROM events WHERE id = %s AND user_id = %s",
        (entry.event_id, user_id)
    )
    if event is None:
        raise HTTPException(status_code=404, detail="Event not found")

    # Check project exists and belongs to user
    project = db.execute_one(
        "SELECT * FROM projects WHERE id = %s AND user_id = %s",
        (entry.project_id, user_id)
    )
    if project is None:
        raise HTTPException(status_code=404, detail="Project not found")

    # Upsert time entry
    db.execute(
        """
        INSERT INTO time_entries (user_id, event_id, project_id, hours, description, classification_source)
        VALUES (%s, %s, %s, %s, %s, 'manual')
        ON CONFLICT(event_id) DO UPDATE SET
            project_id = EXCLUDED.project_id,
            hours = EXCLUDED.hours,
            description = EXCLUDED.description,
            classification_source = EXCLUDED.classification_source,
            classified_at = CURRENT_TIMESTAMP
        """,
        (user_id, entry.event_id, entry.project_id, entry.hours, entry.description),
    )

    row = db.execute_one(
        """
        SELECT te.*, p.name as project_name
        FROM time_entries te
        JOIN projects p ON te.project_id = p.id
        WHERE te.event_id = %s AND te.user_id = %s
        """,
        (entry.event_id, user_id),
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
async def update_entry(
    entry_id: int,
    entry: TimeEntryUpdate,
    user_id: int = Depends(get_user_id)
):
    """Update a time entry (user can only update their own entries)."""
    db = get_db()

    # Verify entry belongs to user
    row = db.execute_one(
        "SELECT * FROM time_entries WHERE id = %s AND user_id = %s",
        (entry_id, user_id)
    )
    if row is None:
        raise HTTPException(status_code=404, detail="Time entry not found")

    updates = []
    params = []
    mark_manual = False
    if entry.project_id is not None:
        # Verify new project belongs to user
        project = db.execute_one(
            "SELECT * FROM projects WHERE id = %s AND user_id = %s",
            (entry.project_id, user_id)
        )
        if project is None:
            raise HTTPException(status_code=404, detail="Project not found")
        updates.append("project_id = %s")
        params.append(entry.project_id)
        mark_manual = True
    if entry.hours is not None:
        updates.append("hours = %s")
        params.append(entry.hours)
        mark_manual = True
    if entry.description is not None:
        updates.append("description = %s")
        params.append(entry.description)

    # Mark as manual classification when user explicitly changes project or hours
    if mark_manual:
        updates.append("classification_source = 'manual'")

    if updates:
        params.extend([entry_id, user_id])
        db.execute(
            f"UPDATE time_entries SET {', '.join(updates)} WHERE id = %s AND user_id = %s",
            tuple(params),
        )

    row = db.execute_one(
        """
        SELECT te.*, p.name as project_name
        FROM time_entries te
        JOIN projects p ON te.project_id = p.id
        WHERE te.id = %s AND te.user_id = %s
        """,
        (entry_id, user_id),
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
async def delete_entry(entry_id: int, user_id: int = Depends(get_user_id)):
    """Delete a time entry (unclassify event)."""
    db = get_db()

    # Verify entry belongs to user
    row = db.execute_one(
        "SELECT * FROM time_entries WHERE id = %s AND user_id = %s",
        (entry_id, user_id)
    )
    if row is None:
        raise HTTPException(status_code=404, detail="Time entry not found")

    db.execute(
        "DELETE FROM time_entries WHERE id = %s AND user_id = %s",
        (entry_id, user_id)
    )
    return {"status": "deleted"}


@router.post("/entries/bulk")
async def bulk_classify(request: BulkClassifyRequest, user_id: int = Depends(get_user_id)):
    """Classify multiple events at once."""
    db = get_db()

    # Check project exists and belongs to user
    project = db.execute_one(
        "SELECT * FROM projects WHERE id = %s AND user_id = %s",
        (request.project_id, user_id)
    )
    if project is None:
        raise HTTPException(status_code=404, detail="Project not found")

    classified = 0
    for event_id in request.event_ids:
        # Verify event belongs to user
        event = db.execute_one(
            "SELECT * FROM events WHERE id = %s AND user_id = %s",
            (event_id, user_id)
        )
        if event is None:
            continue

        # Calculate hours from event duration
        start = event["start_time"] if isinstance(event["start_time"], datetime) else datetime.fromisoformat(event["start_time"])
        end = event["end_time"] if isinstance(event["end_time"], datetime) else datetime.fromisoformat(event["end_time"])
        hours = (end - start).total_seconds() / 3600

        db.execute(
            """
            INSERT INTO time_entries (user_id, event_id, project_id, hours, description, classification_source)
            VALUES (%s, %s, %s, %s, %s, 'manual')
            ON CONFLICT(event_id) DO UPDATE SET
                project_id = EXCLUDED.project_id,
                hours = EXCLUDED.hours,
                classification_source = EXCLUDED.classification_source,
                classified_at = CURRENT_TIMESTAMP
            """,
            (user_id, event_id, request.project_id, hours, event["title"]),
        )
        classified += 1

    return {"classified": classified}


# --- Export ---

@router.get("/export/harvest")
async def export_harvest(
    start_date: str,
    end_date: str,
    user_id: int = Depends(get_user_id)
):
    """Export time entries as Harvest-compatible CSV."""
    csv_content = export_harvest_csv(start_date, end_date, user_id)

    return StreamingResponse(
        io.StringIO(csv_content),
        media_type="text/csv",
        headers={"Content-Disposition": f"attachment; filename=timesheet_{start_date}_{end_date}.csv"},
    )


# --- Invoices ---

def _invoice_to_response(invoice) -> InvoiceResponse:
    """Convert Invoice dataclass to response model."""
    from services.invoice import Invoice, InvoiceLineItem

    line_items = None
    if invoice.line_items is not None:
        line_items = [
            InvoiceLineItemResponse(
                id=item.id,
                time_entry_id=item.time_entry_id,
                entry_date=str(item.entry_date),
                description=item.description,
                hours=item.hours,
                rate=float(item.rate),
                amount=float(item.amount),
                is_orphaned=item.is_orphaned
            )
            for item in invoice.line_items
        ]

    return InvoiceResponse(
        id=invoice.id,
        project_id=invoice.project_id,
        project_name=invoice.project_name,
        client=invoice.client,
        invoice_number=invoice.invoice_number,
        period_start=str(invoice.period_start),
        period_end=str(invoice.period_end),
        invoice_date=str(invoice.invoice_date),
        status=invoice.status,
        total_hours=invoice.total_hours,
        total_amount=float(invoice.total_amount),
        sheets_spreadsheet_id=invoice.sheets_spreadsheet_id,
        sheets_spreadsheet_url=invoice.sheets_spreadsheet_url,
        last_exported_at=invoice.last_exported_at,
        created_at=invoice.created_at,
        line_items=line_items
    )


@router.get("/invoices", response_model=InvoiceListResponse)
async def list_invoices(
    project_id: int | None = None,
    status: str | None = None,
    limit: int = 100,
    offset: int = 0,
    user_id: int = Depends(get_user_id)
):
    """List invoices with optional filters."""
    from services.invoice import list_invoices as do_list

    invoices, total = do_list(user_id, project_id, status, limit, offset)

    return InvoiceListResponse(
        invoices=[_invoice_to_response(inv) for inv in invoices],
        total=total
    )


@router.post("/invoices", response_model=InvoiceResponse)
async def create_invoice(
    request: InvoiceCreate,
    user_id: int = Depends(get_user_id)
):
    """Create a new invoice from unbilled time entries."""
    from services.invoice import create_invoice as do_create
    from datetime import date

    try:
        period_start = date.fromisoformat(request.period_start)
        period_end = date.fromisoformat(request.period_end)
        invoice_date = date.fromisoformat(request.invoice_date) if request.invoice_date else None

        invoice = do_create(
            user_id=user_id,
            project_id=request.project_id,
            period_start=period_start,
            period_end=period_end,
            invoice_date=invoice_date
        )
        return _invoice_to_response(invoice)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.get("/invoices/preview", response_model=InvoicePreview)
async def preview_invoice(
    project_id: int,
    period_start: str,
    period_end: str,
    user_id: int = Depends(get_user_id)
):
    """Preview what an invoice would contain without creating it."""
    from services.invoice import get_unbilled_entries, generate_invoice_number
    from datetime import date
    from decimal import Decimal

    db = get_db()

    # Get project details
    project = db.execute_one(
        "SELECT id, name, short_code, is_billable, bill_rate FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id)
    )
    if not project:
        raise HTTPException(status_code=404, detail="Project not found")

    if not project["is_billable"]:
        raise HTTPException(status_code=400, detail="Project is not billable")

    start = date.fromisoformat(period_start)
    end = date.fromisoformat(period_end)

    entries = get_unbilled_entries(user_id, project_id, start, end)
    bill_rate = Decimal(str(project["bill_rate"] or 0))
    total_hours = sum(e["hours"] for e in entries)
    total_amount = sum(Decimal(str(e["hours"])) * bill_rate for e in entries)

    return InvoicePreview(
        project_id=project_id,
        project_name=project["name"],
        invoice_number=generate_invoice_number(
            project_id, project["name"], user_id, project.get("short_code")
        ),
        period_start=period_start,
        period_end=period_end,
        unbilled_entries=len(entries),
        total_hours=total_hours,
        bill_rate=float(bill_rate),
        total_amount=float(total_amount)
    )


@router.get("/invoices/{invoice_id}", response_model=InvoiceResponse)
async def get_invoice(
    invoice_id: int,
    user_id: int = Depends(get_user_id)
):
    """Get invoice details with line items."""
    from services.invoice import get_invoice as do_get

    invoice = do_get(user_id, invoice_id)
    if not invoice:
        raise HTTPException(status_code=404, detail="Invoice not found")

    return _invoice_to_response(invoice)


@router.delete("/invoices/{invoice_id}")
async def delete_invoice(
    invoice_id: int,
    user_id: int = Depends(get_user_id),
    credentials=Depends(require_auth)
):
    """Delete a draft invoice and its worksheet from the spreadsheet."""
    from services.invoice import delete_invoice as do_delete
    from services.sheets import delete_invoice_worksheet, remove_invoice_from_summary

    db = get_db()

    # Get invoice info before deleting
    invoice = db.execute_one(
        """
        SELECT i.invoice_number, i.sheets_spreadsheet_id, p.sheets_spreadsheet_id AS project_spreadsheet_id
        FROM invoices i
        JOIN projects p ON i.project_id = p.id
        WHERE i.id = %s AND i.user_id = %s
        """,
        (invoice_id, user_id)
    )

    try:
        # Delete invoice from database
        do_delete(user_id, invoice_id)

        # Clean up spreadsheet if invoice was exported
        if invoice:
            # The invoice might have been exported to a project spreadsheet
            spreadsheet_id = invoice.get("sheets_spreadsheet_id") or invoice.get("project_spreadsheet_id")
            if spreadsheet_id and invoice.get("invoice_number"):
                try:
                    delete_invoice_worksheet(credentials, spreadsheet_id, invoice["invoice_number"])
                    remove_invoice_from_summary(credentials, spreadsheet_id, invoice["invoice_number"])
                except Exception as e:
                    # Log but don't fail - spreadsheet cleanup is best-effort
                    logger.warning(f"Failed to clean up spreadsheet for invoice {invoice_id}: {e}")

        return {"status": "deleted"}
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.post("/invoices/{invoice_id}/regenerate", response_model=InvoiceResponse)
async def regenerate_invoice(
    invoice_id: int,
    user_id: int = Depends(get_user_id)
):
    """Regenerate invoice line items from current unbilled entries."""
    from services.invoice import regenerate_invoice as do_regenerate

    try:
        invoice = do_regenerate(user_id, invoice_id)
        return _invoice_to_response(invoice)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.post("/invoices/{invoice_id}/status")
async def update_invoice_status(
    invoice_id: int,
    request: dict,
    user_id: int = Depends(get_user_id)
):
    """Update invoice status (draft, finalized, paid)."""
    from services.invoice import update_invoice_status as do_update

    status = request.get("status")
    if not status:
        raise HTTPException(status_code=400, detail="status is required")

    try:
        invoice = do_update(user_id, invoice_id, status)
        return _invoice_to_response(invoice)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.get("/invoices/{invoice_id}/export/csv")
async def export_invoice_csv(
    invoice_id: int,
    user_id: int = Depends(get_user_id)
):
    """Export invoice as CSV."""
    from services.invoice import export_invoice_csv as do_export, get_invoice as do_get

    invoice = do_get(user_id, invoice_id, include_line_items=False)
    if not invoice:
        raise HTTPException(status_code=404, detail="Invoice not found")

    try:
        csv_content = do_export(user_id, invoice_id)
        filename = f"{invoice.invoice_number}.csv"

        return StreamingResponse(
            io.StringIO(csv_content),
            media_type="text/csv",
            headers={"Content-Disposition": f"attachment; filename={filename}"},
        )
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))


@router.post("/invoices/{invoice_id}/export/sheets")
async def export_invoice_sheets(
    invoice_id: int,
    force: bool = False,
    user_id: int = Depends(get_user_id),
    credentials=Depends(require_auth)
):
    """Export invoice to Google Sheets.

    Creates or updates a worksheet within the project's spreadsheet.
    Returns the spreadsheet URL.

    If the spreadsheet was modified since last export and force=False,
    returns 409 Conflict with modification details.
    """
    from services.invoice import get_invoice as do_get
    from services.sheets import export_invoice_to_sheets

    invoice = do_get(user_id, invoice_id, include_line_items=True)
    if not invoice:
        raise HTTPException(status_code=404, detail="Invoice not found")

    try:
        result = export_invoice_to_sheets(
            credentials=credentials,
            invoice=invoice,
            user_id=user_id,
            force=force
        )

        # Check if result is a warning dict
        if isinstance(result, dict) and result.get("warning") == "spreadsheet_modified":
            raise HTTPException(status_code=409, detail=result)

        spreadsheet_id, spreadsheet_url = result
        return {
            "spreadsheet_id": spreadsheet_id,
            "spreadsheet_url": spreadsheet_url,
            "invoice_number": invoice.invoice_number,
        }
    except HTTPException:
        raise
    except Exception as e:
        logger.exception(f"Failed to export invoice {invoice_id} to Sheets")
        raise HTTPException(status_code=500, detail=f"Failed to export to Sheets: {str(e)}")


# --- Classification Rules ---

@router.get("/rules")
async def list_rules(enabled_only: bool = False, user_id: int = Depends(get_user_id)):
    """List all classification rules (user's rules only)."""
    db = get_db()
    query = "SELECT * FROM classification_rules WHERE user_id = %s"
    params = [user_id]
    if enabled_only:
        query += " AND is_enabled = TRUE"
    query += " ORDER BY priority DESC, display_order"
    rules = db.execute(query, tuple(params))
    return [dict(r) for r in rules]


@router.get("/rules/{rule_id}")
async def get_rule(rule_id: int, user_id: int = Depends(get_user_id)):
    """Get a single rule with query and conditions."""
    db = get_db()

    # Get rule directly from database
    row = db.execute_one(
        "SELECT * FROM classification_rules WHERE id = %s AND user_id = %s",
        (rule_id, user_id)
    )
    if row is None:
        raise HTTPException(status_code=404, detail="Rule not found")

    return dict(row)


@router.post("/rules")
async def create_classification_rule(request: dict, user_id: int = Depends(get_user_id)):
    """Create a new classification rule.

    Request body:
    {
        "query": "domain:example.com title:standup",
        "target_type": "project",  // or "did_not_attend"
        "project_id": 1,  // required for project rules
    }
    """
    from services.query_parser import parse_query, ParseError

    db = get_db()

    # Require query
    if "query" not in request:
        raise HTTPException(status_code=400, detail="Query is required")

    query = request["query"].strip()
    if not query:
        raise HTTPException(status_code=400, detail="Query cannot be empty")

    # Validate query syntax
    try:
        parse_query(query)
    except ParseError as e:
        raise HTTPException(status_code=400, detail=f"Invalid query: {str(e)}")

    target_type = request.get("target_type", "project")

    # Validate target_type
    if target_type not in ("project", "did_not_attend"):
        raise HTTPException(status_code=400, detail="target_type must be 'project' or 'did_not_attend'")

    # For project rules, validate project_id
    project_id = None
    if target_type == "project":
        if "project_id" not in request:
            raise HTTPException(status_code=400, detail="Project ID is required for project rules")
        project_id = request["project_id"]
        project = db.execute_one(
            "SELECT * FROM projects WHERE id = %s AND user_id = %s",
            (project_id, user_id)
        )
        if project is None:
            raise HTTPException(status_code=404, detail="Project not found")

    # Generate a name from the query if not provided
    name = request.get("name", query[:50])

    # Insert the rule
    rule_id = db.execute_insert(
        """
        INSERT INTO classification_rules (
            user_id, name, project_id, target_type, query,
            priority, is_enabled, stop_processing, is_generated
        ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
        RETURNING id
        """,
        (
            user_id, name, project_id, target_type, query,
            request.get("priority", 0),
            request.get("is_enabled", True),
            request.get("stop_processing", True),
            request.get("is_generated", False),
        )
    )

    # Return the created rule
    row = db.execute_one(
        "SELECT * FROM classification_rules WHERE id = %s AND user_id = %s",
        (rule_id, user_id)
    )
    return dict(row)


@router.put("/rules/{rule_id}")
async def update_classification_rule(
    rule_id: int,
    request: dict,
    user_id: int = Depends(get_user_id)
):
    """Update a classification rule.

    Request body (all fields optional):
    {
        "query": "domain:example.com",
        "is_enabled": true,
        "name": "New name",
    }
    """
    from services.query_parser import parse_query, ParseError

    db = get_db()

    # Check rule exists and belongs to user
    existing = db.execute_one(
        "SELECT * FROM classification_rules WHERE id = %s AND user_id = %s",
        (rule_id, user_id)
    )
    if existing is None:
        raise HTTPException(status_code=404, detail="Rule not found")

    updates = []
    params = []

    if "query" in request:
        query = request["query"].strip()
        if not query:
            raise HTTPException(status_code=400, detail="Query cannot be empty")

        # Validate query syntax
        try:
            parse_query(query)
        except ParseError as e:
            raise HTTPException(status_code=400, detail=f"Invalid query: {str(e)}")

        updates.append("query = %s")
        params.append(query)

    if "is_enabled" in request:
        updates.append("is_enabled = %s")
        params.append(request["is_enabled"])

    if "name" in request:
        updates.append("name = %s")
        params.append(request["name"])

    if not updates:
        raise HTTPException(status_code=400, detail="No fields to update")

    params.extend([rule_id, user_id])
    db.execute(
        f"UPDATE classification_rules SET {', '.join(updates)} WHERE id = %s AND user_id = %s",
        tuple(params)
    )

    # Return updated rule
    row = db.execute_one(
        "SELECT * FROM classification_rules WHERE id = %s AND user_id = %s",
        (rule_id, user_id)
    )
    return dict(row)


@router.delete("/rules/{rule_id}")
async def delete_classification_rule(rule_id: int, user_id: int = Depends(get_user_id)):
    """Delete a classification rule."""
    db = get_db()

    # Verify rule belongs to user and delete
    result = db.execute(
        "DELETE FROM classification_rules WHERE id = %s AND user_id = %s",
        (rule_id, user_id)
    )
    if result == 0:
        raise HTTPException(status_code=404, detail="Rule not found")

    return {"status": "deleted"}


# --- Event Analysis ---

@router.get("/events/{event_id}/properties")
async def get_event_props(event_id: int, user_id: int = Depends(get_user_id)):
    """Get all computed properties for an event (used by create rule modal)."""
    props = get_event_properties(event_id, user_id)
    if props is None:
        raise HTTPException(status_code=404, detail="Event not found")
    return props


class RulePreviewRequest(BaseModel):
    """Preview which events match a query."""
    query: str
    limit: int = 100


@router.post("/rules/preview")
async def preview_rule_matches(
    request: RulePreviewRequest,
    user_id: int = Depends(get_user_id)
):
    """Preview which events match a query string.

    Returns list of events that would match the given query.
    Used for live preview in rule creation UI.
    """
    from services.query_parser import parse_query, ParseError
    from services.query_evaluator import QueryEvaluator
    import json

    db = get_db()

    # Validate and parse query
    try:
        ast = parse_query(request.query)
    except ParseError as e:
        raise HTTPException(status_code=400, detail=f"Invalid query: {str(e)}")

    if not ast.items:
        return {"events": [], "total": 0}

    # Get recent events for matching (last 90 days by default)
    events = db.execute(
        """
        SELECT * FROM events
        WHERE user_id = %s
          AND start_time >= CURRENT_DATE - INTERVAL '90 days'
        ORDER BY start_time DESC
        """,
        (user_id,)
    )

    # Match events against query
    matching = []
    for event in events:
        event_dict = dict(event)
        evaluator = QueryEvaluator(event_dict)
        if evaluator.evaluate(ast):
            matching.append({
                "id": event_dict["id"],
                "title": event_dict.get("title"),
                "start_time": event_dict.get("start_time").isoformat() if event_dict.get("start_time") else None,
                "attendees": json.loads(event_dict.get("attendees") or "[]"),
            })
            if len(matching) >= request.limit:
                break

    return {
        "events": matching,
        "total": len(matching),
        "query_parsed": str(ast)
    }


@router.post("/rules/apply")
async def apply_rules_to_events(request: dict = None, user_id: int = Depends(get_user_id)):
    """Apply rules to events.

    Handles both project classification rules and did_not_attend rules:
    - Project rules: Create time entries for unclassified events
    - Did-not-attend rules: Set did_not_attend flag on matching events

    Supports both query-based (v2) and conditions-based (legacy) rules.

    Request body (optional):
    {
        "start_date": "2025-01-01",  // Optional: filter by date range
        "end_date": "2025-12-31",
        "dry_run": false  // If true, only return what would be affected
    }
    """
    from services.query_parser import parse_query, ParseError
    from services.query_evaluator import QueryEvaluator
    import json

    db = get_db()
    request = request or {}

    # Build query for events that need processing:
    # - No time entry (for project rules), OR
    # - did_not_attend is FALSE (for did_not_attend rules)
    events_query = """
        SELECT e.* FROM events e
        LEFT JOIN time_entries te ON e.id = te.event_id
        WHERE e.user_id = %s
          AND (te.id IS NULL OR e.did_not_attend = FALSE)
    """
    params = [user_id]

    if request.get("start_date"):
        events_query += " AND DATE(e.start_time) >= %s"
        params.append(request["start_date"])
    if request.get("end_date"):
        events_query += " AND DATE(e.start_time) <= %s"
        params.append(request["end_date"])

    events_query += " ORDER BY e.start_time"

    events = db.execute(events_query, tuple(params))

    # Load all enabled rules
    rules_rows = db.execute(
        """
        SELECT cr.*, p.name as project_name
        FROM classification_rules cr
        LEFT JOIN projects p ON cr.project_id = p.id
        WHERE cr.user_id = %s AND cr.is_enabled = TRUE
        ORDER BY cr.priority DESC, cr.display_order
        """,
        (user_id,)
    )
    rules = [dict(r) for r in rules_rows]

    # Prepare parsed queries for v2 rules
    parsed_rules = []
    for rule in rules:
        if rule.get("query"):
            try:
                rule["_parsed_query"] = parse_query(rule["query"])
            except ParseError:
                rule["_parsed_query"] = None
        else:
            rule["_parsed_query"] = None
        parsed_rules.append(rule)

    # Load project fingerprints for implicit matching
    projects = db.execute(
        """
        SELECT id, name, fingerprint_domains, fingerprint_emails, fingerprint_keywords
        FROM projects
        WHERE user_id = %s AND is_archived = FALSE
        """,
        (user_id,)
    )
    fingerprint_matchers = []
    for proj in projects:
        domains = _parse_jsonb_list(proj.get("fingerprint_domains"))
        emails = _parse_jsonb_list(proj.get("fingerprint_emails"))
        keywords = _parse_jsonb_list(proj.get("fingerprint_keywords"))

        if domains or emails or keywords:
            # Build query from fingerprint
            fp_query = _build_fingerprint_query_for_apply(domains, emails, keywords)
            if fp_query:
                try:
                    parsed = parse_query(fp_query)
                    fingerprint_matchers.append({
                        "project_id": proj["id"],
                        "project_name": proj["name"],
                        "_parsed_query": parsed,
                        "name": f"Fingerprint: {proj['name']}",
                        "target_type": "project",
                    })
                except ParseError:
                    pass

    dry_run = request.get("dry_run", False)
    classified = []
    attendance_updated = []
    matched = []

    for event in events:
        event_dict = dict(event)
        matching_rule = None

        # First try query-based rules
        for rule in parsed_rules:
            if rule.get("_parsed_query") and rule["_parsed_query"].items:
                evaluator = QueryEvaluator(event_dict)
                if evaluator.evaluate(rule["_parsed_query"]):
                    matching_rule = rule
                    break

        # Next try fingerprint patterns
        if not matching_rule:
            for fp_matcher in fingerprint_matchers:
                evaluator = QueryEvaluator(event_dict)
                if evaluator.evaluate(fp_matcher["_parsed_query"]):
                    matching_rule = fp_matcher
                    break

        if matching_rule:
            match_info = {
                "event_id": event["id"],
                "event_title": event["title"],
                "rule_id": matching_rule.get("id"),
                "rule_name": matching_rule.get("name") or matching_rule.get("query", "")[:50],
                "target_type": matching_rule.get("target_type", "project"),
                "project_id": matching_rule.get("project_id"),
                "project_name": matching_rule.get("project_name"),
            }

            if dry_run:
                matched.append(match_info)
            else:
                target_type = matching_rule.get("target_type", "project")
                if target_type == "did_not_attend":
                    # Set did_not_attend flag on event
                    if not event.get("did_not_attend"):  # Only update if not already set
                        db.execute(
                            "UPDATE events SET did_not_attend = TRUE WHERE id = %s AND user_id = %s",
                            (event["id"], user_id)
                        )
                        attendance_updated.append(match_info)
                elif target_type == "project":
                    project_id = matching_rule.get("project_id")
                    # Skip project rules without a valid project_id
                    if project_id is None:
                        continue
                    # Only classify if event doesn't have a time entry yet
                    has_entry = db.execute_one(
                        "SELECT id FROM time_entries WHERE event_id = %s",
                        (event["id"],)
                    )
                    if not has_entry:
                        # Calculate hours from event duration
                        start_time = event["start_time"]
                        end_time = event["end_time"]
                        start = start_time if isinstance(start_time, datetime) else datetime.fromisoformat(start_time.replace("Z", "+00:00"))
                        end = end_time if isinstance(end_time, datetime) else datetime.fromisoformat(end_time.replace("Z", "+00:00"))
                        hours = (end - start).total_seconds() / 3600

                        db.execute_insert(
                            """
                            INSERT INTO time_entries (user_id, event_id, project_id, hours, description, classification_source, rule_id)
                            VALUES (%s, %s, %s, %s, %s, %s, %s)
                            RETURNING id
                            """,
                            (
                                user_id,
                                event["id"],
                                project_id,
                                hours,
                                event["title"],
                                "rule",
                                matching_rule.get("id"),
                            ),
                        )
                        classified.append(match_info)

    if dry_run:
        return {
            "dry_run": True,
            "would_classify": len([m for m in matched if m["target_type"] == "project"]),
            "would_mark_did_not_attend": len([m for m in matched if m["target_type"] == "did_not_attend"]),
            "matches": matched,
        }

    return {
        "classified": len(classified),
        "attendance_updated": len(attendance_updated),
        "entries": classified,
        "did_not_attend_events": attendance_updated,
    }


def _parse_jsonb_list(val) -> list:
    """Parse a JSONB list field that may be a string or already-parsed list."""
    if val is None:
        return []
    if isinstance(val, list):
        return val
    if isinstance(val, str):
        import json
        return json.loads(val) if val else []
    return []


def _build_fingerprint_query_for_apply(domains: list, emails: list, keywords: list) -> str:
    """Build a query string from fingerprint patterns for rule application.

    Creates an OR query that matches any of the patterns.
    """
    parts = []

    for d in domains:
        parts.append(f"domain:{d}")

    for e in emails:
        parts.append(f"email:{e}")

    for k in keywords:
        parts.append(f'title:"{k}"' if " " in k else f"title:{k}")

    if not parts:
        return ""

    if len(parts) == 1:
        return parts[0]

    return "(" + " OR ".join(parts) + ")"


# =============================================================================
# LLM Classification Endpoints (Experimental)
# =============================================================================


@router.get("/llm/examples")
async def get_llm_examples(user_id: int = Depends(get_user_id)):
    """Get the examples that would be used for LLM classification."""
    from services.llm_classifier import get_classified_examples, get_available_projects

    db = get_db()
    examples = get_classified_examples(db, user_id, limit=50)
    projects = get_available_projects(db, user_id)

    # Group examples by project
    by_project: dict[str, list[str]] = {}
    for ex in examples:
        if ex["project"] not in by_project:
            by_project[ex["project"]] = []
        by_project[ex["project"]].append(ex["title"])

    return {
        "total_examples": len(examples),
        "manual_count": sum(1 for e in examples if e["source"] == "manual"),
        "rule_count": sum(1 for e in examples if e["source"] == "rule"),
        "projects": [p["name"] for p in projects],
        "examples_by_project": by_project,
    }


@router.get("/llm/preview/{event_id}")
async def preview_llm_classification(event_id: int, user_id: int = Depends(get_user_id)):
    """Preview what prompt would be sent to Claude for an event."""
    from services.llm_classifier import preview_classification_prompt

    db = get_db()
    result = preview_classification_prompt(event_id, user_id, db)
    if result is None:
        raise HTTPException(status_code=404, detail="Event not found")

    return result


@router.post("/llm/classify/{event_id}")
async def classify_event_with_llm(event_id: int, user_id: int = Depends(get_user_id)):
    """Classify a single event using Claude API.

    Requires ANTHROPIC_API_KEY environment variable to be set.
    """
    from services.llm_classifier import classify_event_with_llm as do_classify

    db = get_db()
    try:
        suggestion = await do_classify(event_id, user_id, db)
    except RuntimeError as e:
        raise HTTPException(status_code=500, detail=str(e))

    if suggestion is None:
        raise HTTPException(status_code=404, detail="Event not found or classification failed")

    return {
        "event_id": event_id,
        "project_id": suggestion.project_id,
        "project_name": suggestion.project_name,
        "confidence": suggestion.confidence,
        "reasoning": suggestion.reasoning,
    }


class LLMClassifyBatchRequest(BaseModel):
    event_ids: list[int]


@router.post("/llm/classify")
async def classify_events_batch_with_llm(
    request: LLMClassifyBatchRequest,
    user_id: int = Depends(get_user_id)
):
    """Classify multiple events using Claude API.

    Requires ANTHROPIC_API_KEY environment variable to be set.
    """
    from services.llm_classifier import classify_events_batch

    db = get_db()
    try:
        results = await classify_events_batch(request.event_ids, user_id, db)
    except RuntimeError as e:
        raise HTTPException(status_code=500, detail=str(e))

    return {
        "results": results,
        "total": len(results),
        "successful": sum(1 for r in results if r["suggestion"] is not None),
    }


@router.post("/llm/infer-rules")
async def infer_rules_with_llm(user_id: int = Depends(get_user_id)):
    """Ask the LLM to infer classification rules from past classifications.

    This analyzes the classified examples (without seeing existing rules)
    and suggests rules that could automate future classifications.
    """
    from services.llm_classifier import infer_rules_from_classifications

    db = get_db()
    try:
        result = await infer_rules_from_classifications(user_id, db)
    except RuntimeError as e:
        raise HTTPException(status_code=500, detail=str(e))

    return result
