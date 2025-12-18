"""JSON API routes with multi-user support."""

import logging
from datetime import datetime
from fastapi import APIRouter, HTTPException, Depends, Request
from fastapi.responses import StreamingResponse
from pydantic import BaseModel
import io

from db import get_db
from models import (
    Project, ProjectCreate, ProjectUpdate, ProjectVisibility,
    Event, TimeEntry, TimeEntryCreate, TimeEntryUpdate, BulkClassifyRequest,
    SyncRequest, SyncResponse,
)
from services.calendar import sync_calendar_events
from services.exporter import export_harvest_csv
from services.classifier import (
    load_rules_with_conditions,
    create_rule,
    update_rule,
    delete_rule,
    get_property_definitions,
    ConditionEvaluator,
    test_rules_against_event,
    get_event_properties,
    suggest_classification,
)
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

@router.get("/projects", response_model=list[Project])
async def list_projects(user_id: int = Depends(get_user_id)):
    """List all projects for current user."""
    db = get_db()
    rows = db.execute(
        "SELECT * FROM projects WHERE user_id = %s ORDER BY name",
        (user_id,)
    )
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
async def create_project(project: ProjectCreate, user_id: int = Depends(get_user_id)):
    """Create a new project for current user."""
    db = get_db()
    try:
        project_id = db.execute_insert(
            "INSERT INTO projects (user_id, name, client, color) VALUES (%s, %s, %s, %s) RETURNING id",
            (user_id, project.name, project.client, project.color),
        )
    except Exception as e:
        if "unique constraint" in str(e).lower():
            raise HTTPException(status_code=400, detail="Project name already exists")
        raise

    row = db.execute_one(
        "SELECT * FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id)
    )
    return Project(
        id=row["id"],
        name=row["name"],
        client=row["client"],
        color=row["color"] or "#00aa44",
        is_visible=bool(row["is_visible"]),
        created_at=row["created_at"],
    )


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
    return Project(
        id=row["id"],
        name=row["name"],
        client=row["client"],
        color=row["color"] or "#00aa44",
        is_visible=bool(row["is_visible"]),
        created_at=row["created_at"],
    )


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


@router.put("/projects/{project_id}/visibility", response_model=Project)
async def update_project_visibility(
    project_id: int,
    visibility: ProjectVisibility,
    user_id: int = Depends(get_user_id)
):
    """Toggle project visibility."""
    db = get_db()

    # Verify project belongs to user
    row = db.execute_one(
        "SELECT * FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id)
    )
    if row is None:
        raise HTTPException(status_code=404, detail="Project not found")

    db.execute(
        "UPDATE projects SET is_visible = %s WHERE id = %s AND user_id = %s",
        (visibility.is_visible, project_id, user_id),
    )

    row = db.execute_one(
        "SELECT * FROM projects WHERE id = %s AND user_id = %s",
        (project_id, user_id)
    )
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
            time_entry=time_entry,
        ))

    return events


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
    if entry.hours is not None:
        updates.append("hours = %s")
        params.append(entry.hours)
    if entry.description is not None:
        updates.append("description = %s")
        params.append(entry.description)

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
        start = datetime.fromisoformat(event["start_time"])
        end = datetime.fromisoformat(event["end_time"])
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


# --- Classification Rules ---

@router.get("/rules")
async def list_rules(enabled_only: bool = False, user_id: int = Depends(get_user_id)):
    """List all classification rules with their conditions (user's rules only)."""
    db = get_db()
    rules = load_rules_with_conditions(db, user_id, enabled_only=enabled_only)
    return [r.to_dict() for r in rules]


@router.get("/rules/{rule_id}")
async def get_rule(rule_id: int, user_id: int = Depends(get_user_id)):
    """Get a single rule with conditions."""
    db = get_db()
    rules = load_rules_with_conditions(db, user_id, enabled_only=False)
    for rule in rules:
        if rule.id == rule_id:
            return rule.to_dict()
    raise HTTPException(status_code=404, detail="Rule not found")


@router.post("/rules")
async def create_classification_rule(request: dict, user_id: int = Depends(get_user_id)):
    """Create a new classification rule.

    Request body:
    {
        "name": "Rule name",
        "project_id": 1,
        "priority": 100,
        "is_enabled": true,
        "stop_processing": true,
        "conditions": [
            {"property_name": "title", "condition_type": "contains", "condition_value": "standup"},
            {"property_name": "weekday", "condition_type": "in_list", "condition_value": ["monday", "friday"]}
        ]
    }
    """
    db = get_db()

    # Validate required fields
    if "name" not in request:
        raise HTTPException(status_code=400, detail="Rule name is required")
    if "project_id" not in request:
        raise HTTPException(status_code=400, detail="Project ID is required")
    if "conditions" not in request or not request["conditions"]:
        raise HTTPException(status_code=400, detail="At least one condition is required")

    # Check project exists and belongs to user
    project = db.execute_one(
        "SELECT * FROM projects WHERE id = %s AND user_id = %s",
        (request["project_id"], user_id)
    )
    if project is None:
        raise HTTPException(status_code=404, detail="Project not found")

    rule_id = create_rule(
        db=db,
        user_id=user_id,
        name=request["name"],
        project_id=request["project_id"],
        conditions=request["conditions"],
        priority=request.get("priority", 0),
        is_enabled=request.get("is_enabled", True),
        stop_processing=request.get("stop_processing", True),
    )

    # Return the created rule
    rules = load_rules_with_conditions(db, user_id, enabled_only=False)
    for rule in rules:
        if rule.id == rule_id:
            return rule.to_dict()

    return {"id": rule_id}


@router.put("/rules/{rule_id}")
async def update_classification_rule(
    rule_id: int,
    request: dict,
    user_id: int = Depends(get_user_id)
):
    """Update a classification rule.

    Request body (all fields optional):
    {
        "name": "New name",
        "project_id": 2,
        "priority": 50,
        "is_enabled": false,
        "stop_processing": true,
        "conditions": [...]  // If provided, replaces all conditions
    }
    """
    db = get_db()

    # Check rule exists and belongs to user
    existing = db.execute_one(
        "SELECT * FROM classification_rules WHERE id = %s AND user_id = %s",
        (rule_id, user_id)
    )
    if existing is None:
        raise HTTPException(status_code=404, detail="Rule not found")

    # Check project exists if changing and belongs to user
    if "project_id" in request:
        project = db.execute_one(
            "SELECT * FROM projects WHERE id = %s AND user_id = %s",
            (request["project_id"], user_id)
        )
        if project is None:
            raise HTTPException(status_code=404, detail="Project not found")

    update_rule(
        db=db,
        rule_id=rule_id,
        name=request.get("name"),
        project_id=request.get("project_id"),
        priority=request.get("priority"),
        is_enabled=request.get("is_enabled"),
        stop_processing=request.get("stop_processing"),
        conditions=request.get("conditions"),
    )

    # Return the updated rule
    rules = load_rules_with_conditions(db, user_id, enabled_only=False)
    for rule in rules:
        if rule.id == rule_id:
            return rule.to_dict()

    raise HTTPException(status_code=404, detail="Rule not found")


@router.delete("/rules/{rule_id}")
async def delete_classification_rule(rule_id: int, user_id: int = Depends(get_user_id)):
    """Delete a classification rule."""
    db = get_db()

    # Verify rule belongs to user
    existing = db.execute_one(
        "SELECT * FROM classification_rules WHERE id = %s AND user_id = %s",
        (rule_id, user_id)
    )
    if existing is None:
        raise HTTPException(status_code=404, detail="Rule not found")

    delete_rule(db, rule_id)
    return {"status": "deleted"}


@router.post("/rules/{rule_id}/test")
async def test_rule(rule_id: int, request: dict, user_id: int = Depends(get_user_id)):
    """Test a rule against an event.

    Request body:
    {"event_id": 123}
    """
    event_id = request.get("event_id")
    if not event_id:
        raise HTTPException(status_code=400, detail="event_id is required")

    results = test_rules_against_event(event_id, user_id)

    # Find the specific rule in results
    for result in results:
        if result["rule"]["id"] == rule_id:
            return result

    raise HTTPException(status_code=404, detail="Rule not found")


# --- Properties and Conditions (metadata) ---

@router.get("/properties")
async def list_properties():
    """List available properties for rule conditions."""
    return get_property_definitions()


@router.get("/conditions")
async def list_conditions():
    """List available condition types."""
    return ConditionEvaluator.get_available_conditions()


# --- Event Analysis (debugging) ---

@router.get("/events/{event_id}/properties")
async def get_event_props(event_id: int, user_id: int = Depends(get_user_id)):
    """Get all computed properties for an event."""
    props = get_event_properties(event_id, user_id)
    if props is None:
        raise HTTPException(status_code=404, detail="Event not found")
    return props


@router.get("/events/{event_id}/test-rules")
async def test_event_rules(event_id: int, user_id: int = Depends(get_user_id)):
    """Test all rules against an event."""
    results = test_rules_against_event(event_id, user_id)
    if not results:
        # Check if event exists and belongs to user
        db = get_db()
        event = db.execute_one(
            "SELECT * FROM events WHERE id = %s AND user_id = %s",
            (event_id, user_id)
        )
        if event is None:
            raise HTTPException(status_code=404, detail="Event not found")
    return results


@router.get("/events/{event_id}/suggest")
async def get_suggestion(event_id: int, user_id: int = Depends(get_user_id)):
    """Get classification suggestion for an event."""
    suggestion = suggest_classification(event_id, user_id)
    if suggestion is None:
        return {"suggestion": None}
    return {"suggestion": suggestion}


@router.post("/rules/apply")
async def apply_rules_to_events(request: dict = None, user_id: int = Depends(get_user_id)):
    """Apply rules to unclassified events.

    Request body (optional):
    {
        "start_date": "2025-01-01",  // Optional: filter by date range
        "end_date": "2025-12-31",
        "dry_run": false  // If true, only return what would be classified
    }
    """
    from services.classifier import RuleMatcher, EventProperties
    import json

    db = get_db()
    request = request or {}

    # Build query for unclassified events (user's events only)
    query = """
        SELECT e.* FROM events e
        LEFT JOIN time_entries te ON e.id = te.event_id
        WHERE e.user_id = %s AND te.id IS NULL
    """
    params = [user_id]

    if request.get("start_date"):
        query += " AND DATE(e.start_time) >= %s"
        params.append(request["start_date"])
    if request.get("end_date"):
        query += " AND DATE(e.start_time) <= %s"
        params.append(request["end_date"])

    query += " ORDER BY e.start_time"

    events = db.execute(query, tuple(params))

    # Load rules for this user only
    rules = load_rules_with_conditions(db, user_id, enabled_only=True)
    matcher = RuleMatcher(rules)

    dry_run = request.get("dry_run", False)
    classified = []
    matched = []

    for event in events:
        event_dict = dict(event)
        matching_rule = matcher.match(event_dict)

        if matching_rule:
            match_info = {
                "event_id": event["id"],
                "event_title": event["title"],
                "rule_id": matching_rule.id,
                "rule_name": matching_rule.name,
                "project_id": matching_rule.project_id,
                "project_name": matching_rule.project_name,
            }

            if dry_run:
                matched.append(match_info)
            else:
                # Calculate hours from event duration
                start = datetime.fromisoformat(event["start_time"].replace("Z", "+00:00"))
                end = datetime.fromisoformat(event["end_time"].replace("Z", "+00:00"))
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
                        matching_rule.project_id,
                        hours,
                        event["title"],
                        "rule",
                        matching_rule.id,
                    ),
                )
                classified.append(match_info)

    if dry_run:
        return {
            "dry_run": True,
            "would_classify": len(matched),
            "matches": matched,
        }

    return {
        "classified": len(classified),
        "entries": classified,
    }


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
