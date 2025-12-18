"""Pydantic models for API request/response validation."""

from datetime import datetime
from pydantic import BaseModel


# --- Auth ---

class AuthStatus(BaseModel):
    """Authentication status response."""
    authenticated: bool
    email: str | None = None


# --- Projects ---

class ProjectCreate(BaseModel):
    """Request to create a project."""
    name: str
    client: str | None = None
    color: str = "#00aa44"
    does_not_accumulate_hours: bool = False
    is_billable: bool = False
    bill_rate: float | None = None
    is_hidden_by_default: bool = False
    is_archived: bool = False


class ProjectUpdate(BaseModel):
    """Request to update a project."""
    name: str | None = None
    client: str | None = None
    color: str | None = None
    does_not_accumulate_hours: bool | None = None
    is_billable: bool | None = None
    bill_rate: float | None = None
    is_hidden_by_default: bool | None = None
    is_archived: bool | None = None


class ProjectVisibility(BaseModel):
    """Request to update project visibility."""
    is_visible: bool


class Project(BaseModel):
    """Project response."""
    id: int
    name: str
    client: str | None
    color: str
    is_visible: bool
    does_not_accumulate_hours: bool = False
    is_billable: bool = False
    bill_rate: float | None = None
    is_hidden_by_default: bool = False
    is_archived: bool = False
    created_at: datetime


# --- Events ---

class Event(BaseModel):
    """Calendar event response."""
    id: int
    google_event_id: str
    calendar_id: str
    title: str | None
    description: str | None
    start_time: datetime
    end_time: datetime
    attendees: list[str]
    meeting_link: str | None
    event_color: str | None
    is_recurring: bool
    did_not_attend: bool = False
    my_response_status: str | None = None
    # Populated if event has been classified
    time_entry: "TimeEntry | None" = None


class EventAttendance(BaseModel):
    """Request to update event attendance flag."""
    did_not_attend: bool


# --- Time Entries ---

class TimeEntryCreate(BaseModel):
    """Request to create/update a time entry (classify an event)."""
    event_id: int
    project_id: int
    hours: float
    description: str | None = None


class TimeEntryUpdate(BaseModel):
    """Request to update a time entry."""
    project_id: int | None = None
    hours: float | None = None
    description: str | None = None


class TimeEntry(BaseModel):
    """Time entry response."""
    id: int
    event_id: int
    project_id: int
    project_name: str
    hours: float
    description: str | None
    classified_at: datetime
    classification_source: str


class BulkClassifyRequest(BaseModel):
    """Request to classify multiple events."""
    event_ids: list[int]
    project_id: int


# --- Sync ---

class SyncRequest(BaseModel):
    """Request to sync calendar events."""
    start_date: str  # ISO date: YYYY-MM-DD
    end_date: str    # ISO date: YYYY-MM-DD


class SyncResponse(BaseModel):
    """Response from sync operation."""
    events_fetched: int
    events_new: int
    events_updated: int


# --- Export ---

class ExportRequest(BaseModel):
    """Request to export time entries."""
    start_date: str  # ISO date: YYYY-MM-DD
    end_date: str    # ISO date: YYYY-MM-DD


# Resolve forward references
Event.model_rebuild()
