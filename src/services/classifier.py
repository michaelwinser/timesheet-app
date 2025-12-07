"""Classification service for suggesting project assignments.

This module handles rule-based classification of calendar events.
Future versions may include ML-based classification.
"""

from db import get_db


def suggest_classification(event_id: int) -> dict | None:
    """
    Suggest a project classification for an event based on rules.

    Returns:
        dict with project_id and confidence, or None if no suggestion
    """
    db = get_db()

    event = db.execute_one("SELECT * FROM events WHERE id = ?", (event_id,))
    if event is None:
        return None

    # Try to match rules in priority order
    rules = db.execute(
        "SELECT * FROM classification_rules ORDER BY priority DESC"
    )

    for rule in rules:
        if matches_rule(event, rule):
            return {
                "project_id": rule["project_id"],
                "rule_id": rule["id"],
                "confidence": 0.8,  # TODO: calculate actual confidence
            }

    # Try recurrence-based matching
    if event["recurrence_id"]:
        previous_entry = db.execute_one(
            """
            SELECT te.project_id
            FROM time_entries te
            JOIN events e ON te.event_id = e.id
            WHERE e.recurrence_id = ? AND e.id != ?
            ORDER BY e.start_time DESC
            LIMIT 1
            """,
            (event["recurrence_id"], event_id),
        )
        if previous_entry:
            return {
                "project_id": previous_entry["project_id"],
                "rule_id": None,
                "confidence": 0.9,  # High confidence for recurring events
            }

    return None


def matches_rule(event: dict, rule: dict) -> bool:
    """Check if an event matches a classification rule."""
    rule_type = rule["rule_type"]
    rule_value = rule["rule_value"].lower()

    if rule_type == "title_contains":
        title = (event["title"] or "").lower()
        return rule_value in title

    elif rule_type == "attendee":
        import json
        attendees = json.loads(event["attendees"]) if event["attendees"] else []
        return any(rule_value in a.lower() for a in attendees)

    elif rule_type == "color":
        return event["event_color"] == rule_value

    elif rule_type == "recurrence":
        return event["recurrence_id"] == rule_value

    return False


def create_rule_from_classification(event_id: int, project_id: int) -> None:
    """
    Optionally create a rule from a manual classification.

    This is called after manual classification to learn patterns.
    Currently a placeholder for future ML/learning features.
    """
    # TODO: Implement learning from manual classifications
    # Ideas:
    # - If same attendee classified to same project 3+ times, create rule
    # - If same title pattern classified to same project, create rule
    # - Track classification history for ML training
    pass
