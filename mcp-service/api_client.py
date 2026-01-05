"""HTTP client for the Timesheet API."""

import os
from datetime import date, datetime
from typing import Any

import httpx


class TimesheetAPIError(Exception):
    """Error from the Timesheet API."""

    def __init__(self, status_code: int, message: str, details: dict | None = None):
        self.status_code = status_code
        self.message = message
        self.details = details or {}
        super().__init__(f"API Error {status_code}: {message}")


class TimesheetAPI:
    """Client for the Timesheet API."""

    def __init__(self, base_url: str | None = None, api_key: str | None = None):
        self.base_url = (base_url or os.environ.get("TIMESHEET_API_URL", "http://localhost:8080")).rstrip("/")
        self.api_key = api_key or os.environ.get("TIMESHEET_API_KEY")
        if not self.api_key:
            raise ValueError("TIMESHEET_API_KEY environment variable is required")

        self._client = httpx.Client(
            base_url=self.base_url,
            headers={"Authorization": f"Bearer {self.api_key}"},
            timeout=30.0,
        )

    def _request(self, method: str, path: str, **kwargs) -> Any:
        """Make an API request and handle errors."""
        response = self._client.request(method, path, **kwargs)

        if response.status_code >= 400:
            try:
                error_data = response.json()
                raise TimesheetAPIError(
                    response.status_code,
                    error_data.get("message", response.text),
                    error_data.get("details"),
                )
            except (ValueError, KeyError):
                raise TimesheetAPIError(response.status_code, response.text)

        if response.status_code == 204:
            return None

        return response.json()

    # Projects
    def list_projects(self, include_archived: bool = False) -> list[dict]:
        """List all projects."""
        return self._request("GET", "/api/projects", params={"include_archived": include_archived})

    def get_project(self, project_id: str) -> dict:
        """Get a project by ID."""
        return self._request("GET", f"/api/projects/{project_id}")

    def create_project(self, name: str, **kwargs) -> dict:
        """Create a new project."""
        data = {"name": name, **kwargs}
        return self._request("POST", "/api/projects", json=data)

    # Time Entries
    def list_time_entries(
        self,
        start_date: date | str | None = None,
        end_date: date | str | None = None,
        project_id: str | None = None,
    ) -> list[dict]:
        """List time entries with optional filters."""
        params = {}
        if start_date:
            params["start_date"] = str(start_date)
        if end_date:
            params["end_date"] = str(end_date)
        if project_id:
            params["project_id"] = project_id
        return self._request("GET", "/api/time-entries", params=params)

    def create_time_entry(
        self,
        project_id: str,
        date_str: str,
        hours: float,
        description: str | None = None,
    ) -> dict:
        """Create a new time entry."""
        data = {
            "project_id": project_id,
            "date": date_str,
            "hours": hours,
        }
        if description:
            data["description"] = description
        return self._request("POST", "/api/time-entries", json=data)

    def update_time_entry(self, entry_id: str, hours: float | None = None, description: str | None = None) -> dict:
        """Update a time entry."""
        data = {}
        if hours is not None:
            data["hours"] = hours
        if description is not None:
            data["description"] = description
        return self._request("PUT", f"/api/time-entries/{entry_id}", json=data)

    def delete_time_entry(self, entry_id: str) -> None:
        """Delete a time entry."""
        self._request("DELETE", f"/api/time-entries/{entry_id}")

    # Calendar Events
    def list_calendar_events(
        self,
        start_date: date | str | None = None,
        end_date: date | str | None = None,
        classification_status: str | None = None,
        connection_id: str | None = None,
    ) -> list[dict]:
        """List calendar events with optional filters."""
        params = {}
        if start_date:
            params["start_date"] = str(start_date)
        if end_date:
            params["end_date"] = str(end_date)
        if classification_status:
            params["classification_status"] = classification_status
        if connection_id:
            params["connection_id"] = connection_id
        return self._request("GET", "/api/calendar-events", params=params)

    def classify_event(self, event_id: str, project_id: str | None = None, skip: bool = False) -> dict:
        """Classify a calendar event."""
        data = {}
        if project_id:
            data["project_id"] = project_id
        if skip:
            data["skip"] = True
        return self._request("PUT", f"/api/calendar-events/{event_id}/classify", json=data)

    def bulk_classify_events(self, query: str, project_id: str | None = None, skip: bool = False) -> dict:
        """Bulk classify events matching a query."""
        data = {"query": query}
        if project_id:
            data["project_id"] = project_id
        if skip:
            data["skip"] = True
        return self._request("POST", "/api/calendar-events/bulk-classify", json=data)

    # Classification Rules
    def list_rules(self, include_disabled: bool = False) -> list[dict]:
        """List classification rules."""
        return self._request("GET", "/api/rules", params={"include_disabled": include_disabled})

    def create_rule(
        self,
        query: str,
        project_id: str | None = None,
        attended: bool | None = None,
        weight: float = 1.0,
    ) -> dict:
        """Create a classification rule."""
        data = {"query": query, "weight": weight}
        if project_id:
            data["project_id"] = project_id
        if attended is not None:
            data["attended"] = attended
        return self._request("POST", "/api/rules", json=data)

    def preview_rule(
        self,
        query: str,
        project_id: str | None = None,
        start_date: date | str | None = None,
        end_date: date | str | None = None,
    ) -> dict:
        """Preview what events a rule would match."""
        data = {"query": query}
        if project_id:
            data["project_id"] = project_id
        if start_date:
            data["start_date"] = str(start_date)
        if end_date:
            data["end_date"] = str(end_date)
        return self._request("POST", "/api/rules/preview", json=data)

    def apply_rules(
        self,
        start_date: date | str | None = None,
        end_date: date | str | None = None,
        dry_run: bool = False,
    ) -> dict:
        """Apply classification rules to pending events."""
        data = {"dry_run": dry_run}
        if start_date:
            data["start_date"] = str(start_date)
        if end_date:
            data["end_date"] = str(end_date)
        return self._request("POST", "/api/rules/apply", json=data)

    # Calendar Connections
    def list_calendar_connections(self) -> list[dict]:
        """List calendar connections."""
        return self._request("GET", "/api/calendars")

    def sync_calendar(
        self,
        connection_id: str,
        start_date: date | str | None = None,
        end_date: date | str | None = None,
    ) -> dict:
        """Trigger calendar sync."""
        params = {}
        if start_date:
            params["start_date"] = str(start_date)
        if end_date:
            params["end_date"] = str(end_date)
        return self._request("POST", f"/api/calendars/{connection_id}/sync", params=params)

    def close(self):
        """Close the HTTP client."""
        self._client.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()
