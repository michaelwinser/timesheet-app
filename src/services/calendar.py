"""Google Calendar integration service."""

import json
from datetime import datetime
from google.oauth2.credentials import Credentials
from googleapiclient.discovery import build

from db import get_db


def sync_calendar_events(
    credentials: Credentials,
    start_date: str,
    end_date: str,
) -> dict:
    """
    Fetch events from Google Calendar and store in database.

    Args:
        credentials: OAuth credentials
        start_date: ISO date string (YYYY-MM-DD)
        end_date: ISO date string (YYYY-MM-DD)

    Returns:
        dict with events_fetched, events_new, events_updated
    """
    # Build the Calendar API client
    service = build("calendar", "v3", credentials=credentials)

    # Convert dates to RFC3339 timestamps
    time_min = f"{start_date}T00:00:00Z"
    time_max = f"{end_date}T23:59:59Z"

    # Fetch events from primary calendar
    events_result = service.events().list(
        calendarId="primary",
        timeMin=time_min,
        timeMax=time_max,
        singleEvents=True,  # Expand recurring events
        orderBy="startTime",
    ).execute()

    events = events_result.get("items", [])

    db = get_db()
    events_new = 0
    events_updated = 0

    for event in events:
        google_event_id = event["id"]

        # Extract start/end times
        start = event.get("start", {})
        end = event.get("end", {})

        # Handle all-day events vs timed events
        if "dateTime" in start:
            start_time = start["dateTime"]
            end_time = end["dateTime"]
        else:
            # All-day event
            start_time = f"{start['date']}T00:00:00"
            end_time = f"{end['date']}T00:00:00"

        # Extract attendees
        attendees = []
        for attendee in event.get("attendees", []):
            email = attendee.get("email")
            if email:
                attendees.append(email)

        # Extract meeting link (Hangouts/Meet)
        meeting_link = None
        if "hangoutLink" in event:
            meeting_link = event["hangoutLink"]
        elif "conferenceData" in event:
            entry_points = event["conferenceData"].get("entryPoints", [])
            for ep in entry_points:
                if ep.get("entryPointType") == "video":
                    meeting_link = ep.get("uri")
                    break

        # Check for recurrence
        is_recurring = "recurringEventId" in event
        recurrence_id = event.get("recurringEventId")

        # Event color
        event_color = event.get("colorId")

        # Check if event already exists
        existing = db.execute_one(
            "SELECT id FROM events WHERE google_event_id = ?",
            (google_event_id,),
        )

        if existing:
            # Update existing event
            db.execute(
                """
                UPDATE events SET
                    title = ?,
                    description = ?,
                    start_time = ?,
                    end_time = ?,
                    attendees = ?,
                    meeting_link = ?,
                    event_color = ?,
                    is_recurring = ?,
                    recurrence_id = ?,
                    raw_json = ?,
                    fetched_at = CURRENT_TIMESTAMP
                WHERE google_event_id = ?
                """,
                (
                    event.get("summary", ""),
                    event.get("description", ""),
                    start_time,
                    end_time,
                    json.dumps(attendees),
                    meeting_link,
                    event_color,
                    1 if is_recurring else 0,
                    recurrence_id,
                    json.dumps(event),
                    google_event_id,
                ),
            )
            events_updated += 1
        else:
            # Insert new event
            db.execute_insert(
                """
                INSERT INTO events (
                    google_event_id, calendar_id, title, description,
                    start_time, end_time, attendees, meeting_link,
                    event_color, is_recurring, recurrence_id, raw_json
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
                """,
                (
                    google_event_id,
                    "primary",
                    event.get("summary", ""),
                    event.get("description", ""),
                    start_time,
                    end_time,
                    json.dumps(attendees),
                    meeting_link,
                    event_color,
                    1 if is_recurring else 0,
                    recurrence_id,
                    json.dumps(event),
                ),
            )
            events_new += 1

    return {
        "events_fetched": len(events),
        "events_new": events_new,
        "events_updated": events_updated,
    }
