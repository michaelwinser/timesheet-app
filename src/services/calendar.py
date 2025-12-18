"""Google Calendar integration service."""

import json
from datetime import datetime
from google.oauth2.credentials import Credentials
from googleapiclient.discovery import build

from db import get_db
from services.classifier import load_rules_with_conditions, RuleMatcher


def sync_calendar_events(
    credentials: Credentials,
    start_date: str,
    end_date: str,
    user_id: int,
) -> dict:
    """
    Fetch events from Google Calendar and store in database.

    Args:
        credentials: OAuth credentials
        start_date: ISO date string (YYYY-MM-DD)
        end_date: ISO date string (YYYY-MM-DD)
        user_id: User ID to sync events for

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
    events_classified = 0

    # Load rules for auto-classification
    rules = load_rules_with_conditions(db, user_id, enabled_only=True)
    matcher = RuleMatcher(rules)

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

        # Extract attendees and user's response status
        attendees = []
        my_response_status = None
        for attendee in event.get("attendees", []):
            email = attendee.get("email")
            if email:
                attendees.append(email)
            # Check if this is the current user
            if attendee.get("self"):
                my_response_status = attendee.get("responseStatus")

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

        # Transparency (free/busy status) - defaults to opaque if not set
        transparency = event.get("transparency")  # "opaque" or "transparent"

        # Visibility (who can see the event)
        visibility = event.get("visibility")  # "default", "public", "private", "confidential"

        # Check if event already exists
        existing = db.execute_one(
            "SELECT id FROM events WHERE google_event_id = %s AND user_id = %s",
            (google_event_id, user_id),
        )

        if existing:
            # Update existing event
            db.execute(
                """
                UPDATE events SET
                    title = %s,
                    description = %s,
                    start_time = %s,
                    end_time = %s,
                    attendees = %s,
                    meeting_link = %s,
                    event_color = %s,
                    is_recurring = %s,
                    recurrence_id = %s,
                    my_response_status = %s,
                    transparency = %s,
                    visibility = %s,
                    raw_json = %s,
                    fetched_at = CURRENT_TIMESTAMP
                WHERE google_event_id = %s AND user_id = %s
                """,
                (
                    event.get("summary", ""),
                    event.get("description", ""),
                    start_time,
                    end_time,
                    json.dumps(attendees),
                    meeting_link,
                    event_color,
                    is_recurring,
                    recurrence_id,
                    my_response_status,
                    transparency,
                    visibility,
                    json.dumps(event),
                    google_event_id,
                    user_id,
                ),
            )
            events_updated += 1
            event_db_id = existing["id"]
        else:
            # Insert new event
            event_db_id = db.execute_insert(
                """
                INSERT INTO events (
                    user_id, google_event_id, calendar_id, title, description,
                    start_time, end_time, attendees, meeting_link,
                    event_color, is_recurring, recurrence_id, my_response_status,
                    transparency, visibility, raw_json
                ) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
                RETURNING id
                """,
                (
                    user_id,
                    google_event_id,
                    "primary",
                    event.get("summary", ""),
                    event.get("description", ""),
                    start_time,
                    end_time,
                    json.dumps(attendees),
                    meeting_link,
                    event_color,
                    is_recurring,
                    recurrence_id,
                    my_response_status,
                    transparency,
                    visibility,
                    json.dumps(event),
                ),
            )
            events_new += 1

        # Auto-classify if not already classified
        already_classified = db.execute_one(
            "SELECT id FROM time_entries WHERE event_id = %s",
            (event_db_id,),
        )

        if not already_classified:
            # Build event dict for matcher (use DB column names)
            event_data = {
                "id": event_db_id,
                "title": event.get("summary", ""),
                "description": event.get("description", ""),
                "start_time": start_time,
                "end_time": end_time,
                "attendees": json.dumps(attendees),
                "meeting_link": meeting_link,
                "event_color": event_color,
                "is_recurring": is_recurring,
                "recurrence_id": recurrence_id,
                "my_response_status": my_response_status,
                "transparency": transparency,
                "visibility": visibility,
            }

            matching_rule = matcher.match(event_data)
            if matching_rule:
                # Calculate hours from start/end time
                start_dt = datetime.fromisoformat(start_time.replace("Z", "+00:00"))
                end_dt = datetime.fromisoformat(end_time.replace("Z", "+00:00"))
                hours = (end_dt - start_dt).total_seconds() / 3600

                db.execute_insert(
                    """
                    INSERT INTO time_entries (user_id, event_id, project_id, hours, description, classification_source, rule_id)
                    VALUES (%s, %s, %s, %s, %s, %s, %s)
                    RETURNING id
                    """,
                    (
                        user_id,
                        event_db_id,
                        matching_rule.project_id,
                        hours,
                        event.get("summary", ""),
                        "rule",
                        matching_rule.id,
                    ),
                )
                events_classified += 1

    return {
        "events_fetched": len(events),
        "events_new": events_new,
        "events_updated": events_updated,
        "events_classified": events_classified,
    }
