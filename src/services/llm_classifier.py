"""LLM-based classification service using Claude API.

This module implements few-shot classification of calendar events
using past manual classifications as examples.
"""

from __future__ import annotations

import json
import os
from dataclasses import dataclass
from typing import Any

from db import get_db


@dataclass
class ClassificationSuggestion:
    """A project classification suggestion from the LLM."""
    project_id: int
    project_name: str
    confidence: float  # 0.0 to 1.0
    reasoning: str


def get_classified_examples(db=None, user_id: int = None, limit: int = 50) -> list[dict]:
    """Get classified events to use as few-shot examples.

    Prioritizes manual classifications over rule-based ones.
    Returns full event data with all properties and project assignments.

    Args:
        db: Database connection
        user_id: User ID to filter examples for
        limit: Maximum number of examples to return
    """
    from services.classifier import EventProperties

    if db is None:
        db = get_db()

    # Get manual classifications first, then rule-based
    where_clause = "WHERE e.user_id = %s" if user_id is not None else ""
    params = [user_id, limit] if user_id is not None else [limit]

    rows = db.execute(f"""
        SELECT
            e.*,
            p.name as project_name,
            te.classification_source
        FROM time_entries te
        JOIN events e ON te.event_id = e.id
        JOIN projects p ON te.project_id = p.id
        {where_clause}
        ORDER BY
            CASE te.classification_source
                WHEN 'manual' THEN 0
                ELSE 1
            END,
            te.classified_at DESC
        LIMIT %s
    """, tuple(params))

    examples = []
    for row in rows:
        # Get all properties including computed ones
        props = EventProperties(dict(row))
        examples.append({
            "title": props.get("title") or "Untitled",
            "description": props.get("description") or "",
            "attendees": props.get("attendees") or [],
            "attendee_domains": props.get("attendee_domains") or [],
            "attendee_count": props.get("attendee_count"),
            "has_meeting_link": props.get("has_meeting_link"),
            "weekday": props.get("weekday"),
            "time_block": props.get("time_block"),
            "duration_minutes": props.get("duration_minutes"),
            "is_recurring": props.get("is_recurring"),
            "is_all_day": props.get("is_all_day"),
            "my_response_status": props.get("my_response_status"),
            "transparency": props.get("transparency"),
            "visibility": props.get("visibility"),
            "project": row["project_name"],
            "source": row["classification_source"],
        })

    return examples


def get_classification_rules(db=None, user_id: int = None) -> list[dict]:
    """Get all enabled classification rules with their conditions.

    These represent explicit user-defined classification logic.

    Args:
        db: Database connection
        user_id: User ID to filter rules for
    """
    if db is None:
        db = get_db()

    # Get rules with their conditions
    where_clause = "WHERE cr.user_id = %s AND cr.is_enabled = true" if user_id is not None else "WHERE cr.is_enabled = true"
    params = (user_id,) if user_id is not None else ()

    rows = db.execute(f"""
        SELECT
            cr.id,
            cr.name,
            cr.priority,
            p.name as project_name,
            rc.property_name,
            rc.condition_type,
            rc.condition_value
        FROM classification_rules cr
        JOIN projects p ON cr.project_id = p.id
        LEFT JOIN rule_conditions rc ON cr.id = rc.rule_id
        {where_clause}
        ORDER BY cr.priority DESC, cr.id
    """, params)

    # Group conditions by rule
    rules_dict: dict[int, dict] = {}
    for row in rows:
        rule_id = row["id"]
        if rule_id not in rules_dict:
            rules_dict[rule_id] = {
                "name": row["name"],
                "priority": row["priority"],
                "project": row["project_name"],
                "conditions": [],
            }
        if row["property_name"]:
            rules_dict[rule_id]["conditions"].append({
                "property": row["property_name"],
                "operator": row["condition_type"],
                "value": row["condition_value"],
            })

    return list(rules_dict.values())


def get_available_projects(db=None, user_id: int = None) -> list[dict]:
    """Get all available projects for classification.

    Args:
        db: Database connection
        user_id: User ID to filter projects for
    """
    if db is None:
        db = get_db()

    where_clause = "WHERE user_id = %s AND is_visible = true" if user_id is not None else "WHERE is_visible = true"
    params = (user_id,) if user_id is not None else ()

    rows = db.execute(f"SELECT id, name, client FROM projects {where_clause} ORDER BY name", params)
    return [{"id": row["id"], "name": row["name"], "client": row["client"]} for row in rows]


async def classify_event_with_llm(
    event_id: int,
    user_id: int,
    db=None,
) -> ClassificationSuggestion | None:
    """Classify a single event using Claude API.

    Args:
        event_id: ID of the event to classify
        user_id: User ID for filtering
        db: Database connection (uses default if None)

    Returns:
        ClassificationSuggestion with the suggested project, or None if failed
    """
    results = await classify_events_batch([event_id], user_id, db)
    if results and results[0].get("suggestion"):
        s = results[0]["suggestion"]
        return ClassificationSuggestion(
            project_id=s["project_id"],
            project_name=s["project_name"],
            confidence=s["confidence"],
            reasoning=s["reasoning"],
        )
    return None


def build_batch_classification_prompt(
    events: list[dict],
    examples: list[dict],
    projects: list[dict],
    rules: list[dict],
) -> str:
    """Build a prompt to classify multiple events in a single API call."""
    from services.classifier import EventProperties

    # Format rules as explicit classification logic (highest priority)
    rules_text = ""
    if rules:
        rules_text = "\n## Classification Rules (HIGHEST PRIORITY - apply these first)\n"
        rules_text += "These are explicit rules defined by the user. If an event matches a rule, use that classification.\n\n"
        for rule in rules:
            conditions_str = " AND ".join(
                f"{c['property']} {c['operator']} '{c['value']}'"
                for c in rule["conditions"]
            )
            rules_text += f"- **{rule['project']}**: {conditions_str}\n"

    # Format examples as full JSON for richer context
    examples_text = "\n## Past Classifications (use as reference for pattern matching)\n"
    examples_by_project: dict[str, list[dict]] = {}
    for ex in examples:
        if ex["project"] not in examples_by_project:
            examples_by_project[ex["project"]] = []
        examples_by_project[ex["project"]].append(ex)

    for project, exs in examples_by_project.items():
        examples_text += f"\n### {project}:\n"
        for ex in exs[:3]:  # Limit to 3 full examples per project
            ex_summary = {
                "title": ex["title"],
                "attendee_domains": ex.get("attendee_domains", []),
                "attendee_count": ex.get("attendee_count", 0),
                "weekday": ex.get("weekday"),
                "time_block": ex.get("time_block"),
                "is_recurring": ex.get("is_recurring"),
                "my_response_status": ex.get("my_response_status"),
                "transparency": ex.get("transparency"),
                "visibility": ex.get("visibility"),
            }
            examples_text += f"```json\n{json.dumps(ex_summary, indent=2)}\n```\n"

    # Format available projects
    projects_text = ", ".join(p["name"] for p in projects)

    # Build events array with all properties
    events_array = []
    for event in events:
        props = EventProperties(event)
        events_array.append({
            "id": event.get("id"),
            "title": props.get("title") or "Untitled",
            "description": props.get("description") or "",
            "attendees": props.get("attendees") or [],
            "attendee_domains": props.get("attendee_domains") or [],
            "attendee_count": props.get("attendee_count"),
            "has_meeting_link": props.get("has_meeting_link"),
            "weekday": props.get("weekday"),
            "time_block": props.get("time_block"),
            "start_hour": props.get("start_hour"),
            "duration_minutes": props.get("duration_minutes"),
            "is_recurring": props.get("is_recurring"),
            "is_all_day": props.get("is_all_day"),
            "my_response_status": props.get("my_response_status"),
            "transparency": props.get("transparency"),
            "visibility": props.get("visibility"),
        })

    events_text = json.dumps(events_array, indent=2)

    prompt = f"""You are classifying calendar events into projects for timesheet tracking.

{rules_text}
{examples_text}

## Available Projects
{projects_text}

## Events to Classify
```json
{events_text}
```

## Instructions
1. FIRST check if any classification rule matches each event. Rules are explicit user-defined logic and take priority.
2. If no rule matches, use the past classification examples to find similar patterns.
3. Pay special attention to attendee_domains - work domains (like openssf.org, linuxfoundation.org) indicate work meetings, not personal.
4. Meeting title format like "Name / Name" does NOT automatically mean Personal - check attendee domains.
5. IMPORTANT: Check my_response_status - if "declined" or "needsAction", the user likely didn't attend this meeting. Consider classifying as "Junk" or low-priority.
6. Check transparency - if "transparent", the event is marked as "free" time and may not be real work time.

Respond with a JSON array where each element has:
- "id": the event id
- "project": the project name (must be one from the available projects list)
- "confidence": a number from 0.0 to 1.0 indicating how confident you are
- "reasoning": a brief explanation of why you chose this project (mention if a rule matched)

If you cannot determine a good match for an event, use confidence below 0.5.
Respond ONLY with the JSON array, no other text."""

    return prompt


async def classify_events_batch(
    event_ids: list[int],
    user_id: int,
    db=None,
) -> list[dict]:
    """Classify multiple events in a SINGLE Claude API call.

    This is much more efficient than making individual calls per event.
    Returns list of results with event_id, suggestion, and any errors.

    Args:
        event_ids: List of event IDs to classify
        user_id: User ID for filtering
        db: Database connection
    """
    try:
        import anthropic
    except ImportError:
        raise RuntimeError("anthropic package not installed. Run: pip install anthropic")

    api_key = os.environ.get("ANTHROPIC_API_KEY")
    if not api_key:
        raise RuntimeError("ANTHROPIC_API_KEY environment variable not set")

    if db is None:
        db = get_db()

    if not event_ids:
        return []

    # Get all events
    placeholders = ",".join("%s" * len(event_ids))
    events = db.execute(
        f"SELECT * FROM events WHERE id IN ({placeholders}) AND user_id = %s",
        tuple(event_ids) + (user_id,)
    )
    events_list = [dict(e) for e in events]

    if not events_list:
        return []

    # Get examples, projects, and rules
    examples = get_classified_examples(db, user_id, limit=50)
    projects = get_available_projects(db, user_id)
    rules = get_classification_rules(db, user_id)

    if not projects:
        return []

    # Build project lookup
    project_lookup = {p["name"].lower(): p for p in projects}

    # Build single prompt for all events
    prompt = build_batch_classification_prompt(events_list, examples, projects, rules)

    # Call Claude API ONCE for all events
    client = anthropic.Anthropic(api_key=api_key)

    message = client.messages.create(
        model="claude-sonnet-4-20250514",
        max_tokens=4096,  # Larger to accommodate multiple results
        messages=[
            {"role": "user", "content": prompt}
        ]
    )

    # Parse response
    response_text = message.content[0].text.strip()

    # Try to extract JSON array
    try:
        # Handle potential markdown code blocks
        if response_text.startswith("```"):
            lines = response_text.split("\n")
            # Find closing ```
            end_idx = -1
            for i, line in enumerate(lines[1:], 1):
                if line.strip() == "```":
                    end_idx = i
                    break
            if end_idx > 0:
                response_text = "\n".join(lines[1:end_idx])
            else:
                response_text = "\n".join(lines[1:-1])

        classifications = json.loads(response_text)
    except json.JSONDecodeError:
        # Return error for all events
        return [{"event_id": eid, "suggestion": None, "error": "Failed to parse LLM response"} for eid in event_ids]

    # Map results by event id
    results_by_id = {}
    for item in classifications:
        event_id = item.get("id")
        project_name = item.get("project", "").lower()
        project = project_lookup.get(project_name)

        # Try partial matching if exact match fails
        if project is None:
            for name, p in project_lookup.items():
                if name in project_name or project_name in name:
                    project = p
                    break

        if project:
            results_by_id[event_id] = {
                "event_id": event_id,
                "suggestion": {
                    "project_id": project["id"],
                    "project_name": project["name"],
                    "confidence": float(item.get("confidence", 0.5)),
                    "reasoning": item.get("reasoning", ""),
                },
                "error": None,
            }
        else:
            results_by_id[event_id] = {
                "event_id": event_id,
                "suggestion": None,
                "error": f"Unknown project: {item.get('project')}",
            }

    # Build results in original order
    results = []
    for event_id in event_ids:
        if event_id in results_by_id:
            results.append(results_by_id[event_id])
        else:
            results.append({
                "event_id": event_id,
                "suggestion": None,
                "error": "Event not in LLM response",
            })

    return results


async def infer_rules_from_classifications(user_id: int, db=None) -> dict:
    """Ask the LLM to infer classification rules from past classifications.

    This does NOT use the existing rules - it tries to discover patterns
    from the classified examples alone.

    Args:
        user_id: User ID for filtering
        db: Database connection
    """
    try:
        import anthropic
    except ImportError:
        raise RuntimeError("anthropic package not installed. Run: pip install anthropic")

    api_key = os.environ.get("ANTHROPIC_API_KEY")
    if not api_key:
        raise RuntimeError("ANTHROPIC_API_KEY environment variable not set")

    if db is None:
        db = get_db()

    # Get classified examples (without rules)
    examples = get_classified_examples(db, user_id, limit=100)  # Get more for pattern detection
    projects = get_available_projects(db, user_id)

    if not examples:
        return {"rules": [], "error": "No classified examples found"}

    # Available condition types (matching what the rules engine supports)
    condition_types_text = """- equals: exact match (string, integer, boolean)
- not_equals: does not equal (string, integer, boolean)
- contains: substring match (string)
- not_contains: does not contain substring (string)
- starts_with: starts with prefix (string)
- ends_with: ends with suffix (string)
- greater_than: greater than value (integer)
- less_than: less than value (integer)
- list_contains: list contains exact value (list)
- list_any_match: list contains any of comma-separated values (list)
- domain_match: attendee domains contain this domain (list)"""

    # Build prompt for rule inference
    examples_text = json.dumps(examples, indent=2)
    projects_text = ", ".join(p["name"] for p in projects)

    prompt = f"""You are analyzing classified calendar events to discover patterns and generate classification rules.

## Classified Events
These events have been manually classified by the user:
```json
{examples_text}
```

## Available Projects
{projects_text}

## Available Event Properties
- title (string): The event title
- description (string): The event description
- attendees (list): List of attendee email addresses
- attendee_domains (list): List of unique domains from attendee emails
- attendee_count (number): Number of attendees
- has_meeting_link (boolean): Whether the event has a video call link
- weekday (string): Day of week (monday, tuesday, etc.)
- time_block (string): Time of day (morning, afternoon, evening)
- start_hour (number): Hour the event starts (0-23)
- duration_minutes (number): Duration in minutes
- is_recurring (boolean): Whether this is a recurring event
- is_all_day (boolean): Whether this is an all-day event

## Available Condition Types
{condition_types_text}

## Task
Analyze the classified events and infer classification rules that could automate future classifications.
Look for patterns like:
- Specific attendee email addresses that always map to a project
- Attendee domains that indicate a project (e.g., @linuxfoundation.org → Alpha-Omega)
- Title patterns (contains, starts_with, equals)
- Time-based patterns (certain days/times → certain projects)

Generate rules that are:
1. Specific enough to avoid false positives
2. General enough to be useful for future events
3. Based on clear patterns you observe in the data

Respond with a JSON object containing:
{{
  "rules": [
    {{
      "name": "descriptive rule name",
      "project": "project name",
      "priority": 50-100 (higher = more specific/important),
      "conditions": [
        {{
          "property": "property_name",
          "operator": "condition_type",
          "value": "the value to match"
        }}
      ],
      "reasoning": "why you created this rule",
      "example_matches": ["list of example event titles this would match"]
    }}
  ],
  "observations": "general observations about classification patterns"
}}

Focus on high-confidence rules. It's better to have fewer accurate rules than many questionable ones.
Respond ONLY with the JSON object, no other text."""

    # Call Claude API
    client = anthropic.Anthropic(api_key=api_key)

    message = client.messages.create(
        model="claude-sonnet-4-20250514",
        max_tokens=4096,
        messages=[
            {"role": "user", "content": prompt}
        ]
    )

    # Parse response
    response_text = message.content[0].text.strip()

    try:
        # Handle potential markdown code blocks
        if response_text.startswith("```"):
            lines = response_text.split("\n")
            end_idx = -1
            for i, line in enumerate(lines[1:], 1):
                if line.strip() == "```":
                    end_idx = i
                    break
            if end_idx > 0:
                response_text = "\n".join(lines[1:end_idx])
            else:
                response_text = "\n".join(lines[1:-1])

        result = json.loads(response_text)
        return result
    except json.JSONDecodeError as e:
        return {"rules": [], "error": f"Failed to parse LLM response: {e}", "raw": response_text}


def preview_classification_prompt(event_ids: int | list[int], user_id: int, db=None) -> dict | None:
    """Preview what prompt would be sent to Claude for event(s).

    Useful for debugging and understanding the classification context.
    Accepts a single event_id or a list of event_ids.

    Args:
        event_ids: Single event ID or list of event IDs
        user_id: User ID for filtering
        db: Database connection
    """
    if db is None:
        db = get_db()

    # Normalize to list
    if isinstance(event_ids, int):
        event_ids = [event_ids]

    # Get all events
    placeholders = ",".join("%s" * len(event_ids))
    events = db.execute(
        f"SELECT * FROM events WHERE id IN ({placeholders}) AND user_id = %s",
        tuple(event_ids) + (user_id,)
    )
    events_list = [dict(e) for e in events]

    if not events_list:
        return None

    examples = get_classified_examples(db, user_id, limit=50)
    projects = get_available_projects(db, user_id)
    rules = get_classification_rules(db, user_id)

    prompt = build_batch_classification_prompt(events_list, examples, projects, rules)

    return {
        "event_ids": event_ids,
        "event_count": len(events_list),
        "example_count": len(examples),
        "project_count": len(projects),
        "rule_count": len(rules),
        "prompt": prompt,
        "prompt_length": len(prompt),
    }
