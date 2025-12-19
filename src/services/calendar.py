"""Google Calendar integration service."""

import json
from datetime import datetime
from google.oauth2.credentials import Credentials
from googleapiclient.discovery import build

from db import get_db
from services.query_parser import parse_query, ParseError
from services.query_evaluator import QueryEvaluator


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

    # Load query-based rules for auto-classification
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
    parsed_rules = []
    for rule in rules_rows:
        rule = dict(rule)
        if rule.get("query"):
            try:
                rule["_parsed_query"] = parse_query(rule["query"])
                parsed_rules.append(rule)
            except ParseError:
                pass

    # Load project fingerprints for implicit matching
    def parse_jsonb(val):
        if val is None:
            return []
        if isinstance(val, list):
            return val
        if isinstance(val, str):
            return json.loads(val) if val else []
        return []

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
        domains = parse_jsonb(proj.get("fingerprint_domains"))
        emails = parse_jsonb(proj.get("fingerprint_emails"))
        keywords = parse_jsonb(proj.get("fingerprint_keywords"))

        if domains or emails or keywords:
            fp_query = _build_fingerprint_query(domains, emails, keywords)
            if fp_query:
                try:
                    fingerprint_matchers.append({
                        "project_id": proj["id"],
                        "project_name": proj["name"],
                        "_parsed_query": parse_query(fp_query),
                        "target_type": "project",
                    })
                except ParseError:
                    pass

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

        # Auto-classify if not already classified (and not manually classified)
        already_classified = db.execute_one(
            "SELECT id, classification_source FROM time_entries WHERE event_id = %s",
            (event_db_id,),
        )

        # Skip if entry exists - never overwrite existing classifications
        if not already_classified:
            # Build event dict for matching (use DB column names)
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

            # Find matching rule
            matching_rule = None
            for rule in parsed_rules:
                if rule.get("_parsed_query") and rule["_parsed_query"].items:
                    evaluator = QueryEvaluator(event_data)
                    if evaluator.evaluate(rule["_parsed_query"]):
                        matching_rule = rule
                        break

            # Try fingerprint patterns if no explicit rule matched
            if not matching_rule:
                for fp_matcher in fingerprint_matchers:
                    evaluator = QueryEvaluator(event_data)
                    if evaluator.evaluate(fp_matcher["_parsed_query"]):
                        matching_rule = fp_matcher
                        break

            if matching_rule:
                target_type = matching_rule.get("target_type", "project")
                if target_type == "did_not_attend":
                    # Set did_not_attend flag on the event
                    db.execute(
                        "UPDATE events SET did_not_attend = TRUE WHERE id = %s",
                        (event_db_id,),
                    )
                elif target_type == "project" and matching_rule.get("project_id"):
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
                            matching_rule.get("project_id"),
                            hours,
                            event.get("summary", ""),
                            "rule",
                            matching_rule.get("id"),
                        ),
                    )
                    events_classified += 1

    return {
        "events_fetched": len(events),
        "events_new": events_new,
        "events_updated": events_updated,
        "events_classified": events_classified,
    }


def _build_fingerprint_query(domains: list, emails: list, keywords: list) -> str:
    """Build a query string from fingerprint patterns."""
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
