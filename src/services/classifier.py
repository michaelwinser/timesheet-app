"""Classification service for suggesting project assignments.

This module implements rules-based classification of calendar events
using an extensible property and condition system.
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass
from datetime import datetime
from enum import Enum
from typing import Any

from db import get_db


# =============================================================================
# Property System
# =============================================================================


class PropertyType(Enum):
    """Types of property values that can be used in conditions."""
    STRING = "string"
    STRING_LIST = "string_list"
    INTEGER = "integer"
    BOOLEAN = "boolean"
    DATETIME = "datetime"


@dataclass
class PropertyDefinition:
    """Metadata about an available property for rule creation."""
    name: str
    display_name: str
    value_type: PropertyType
    is_computed: bool
    description: str


class EventProperties:
    """Provides uniform access to native and computed event properties.

    This class wraps a raw event dict and provides a consistent interface
    for accessing both native properties (stored in the event) and computed
    properties (derived from event data).

    To add a new computed property, simply add a `_compute_<name>` method.
    """

    def __init__(self, event: dict):
        self._event = event
        self._cache: dict[str, Any] = {}

    def get(self, property_name: str) -> Any:
        """Get property value by name.

        Args:
            property_name: Name of the property to retrieve

        Returns:
            The property value, or None if property doesn't exist
        """
        # Check cache first
        if property_name in self._cache:
            return self._cache[property_name]

        # Try native property
        if property_name in self._event:
            value = self._event[property_name]
            # Handle JSON-encoded fields
            if property_name == "attendees" and isinstance(value, str):
                value = json.loads(value) if value else []
                self._cache[property_name] = value
            return value

        # Try computed property
        method = getattr(self, f"_compute_{property_name}", None)
        if method:
            value = method()
            self._cache[property_name] = value
            return value

        return None

    def get_all(self) -> dict[str, Any]:
        """Get all available properties as a dict (for debugging)."""
        result = {}
        for prop in PROPERTY_REGISTRY:
            result[prop.name] = self.get(prop.name)
        return result

    # =========================================================================
    # Computed Properties
    # =========================================================================

    def _compute_weekday(self) -> str:
        """Day of week: 'monday', 'tuesday', etc."""
        start_time = self._event.get("start_time")
        if not start_time:
            return ""
        dt = self._parse_datetime(start_time)
        return dt.strftime("%A").lower()

    def _compute_start_hour(self) -> int:
        """Hour of day (0-23) when event starts."""
        start_time = self._event.get("start_time")
        if not start_time:
            return 0
        dt = self._parse_datetime(start_time)
        return dt.hour

    def _compute_end_hour(self) -> int:
        """Hour of day (0-23) when event ends."""
        end_time = self._event.get("end_time")
        if not end_time:
            return 0
        dt = self._parse_datetime(end_time)
        return dt.hour

    def _compute_duration_minutes(self) -> int:
        """Event duration in minutes."""
        start_time = self._event.get("start_time")
        end_time = self._event.get("end_time")
        if not start_time or not end_time:
            return 0
        start = self._parse_datetime(start_time)
        end = self._parse_datetime(end_time)
        return int((end - start).total_seconds() / 60)

    def _compute_time_block(self) -> str:
        """Time of day: 'morning', 'afternoon', 'evening', 'night'."""
        hour = self._compute_start_hour()
        if 5 <= hour < 12:
            return "morning"
        elif 12 <= hour < 17:
            return "afternoon"
        elif 17 <= hour < 21:
            return "evening"
        else:
            return "night"

    def _compute_is_recurring(self) -> bool:
        """True if event is part of a recurring series."""
        return bool(
            self._event.get("is_recurring") or
            self._event.get("recurrence_id")
        )

    def _compute_is_all_day(self) -> bool:
        """True if event is an all-day event."""
        start = self._event.get("start_time", "")
        if not start:
            return False
        # All-day events typically have times at midnight
        return isinstance(start, str) and (
            "T" not in start or
            start.endswith("T00:00:00") or
            start.endswith("T00:00:00Z")
        )

    def _compute_has_attendees(self) -> bool:
        """True if event has any attendees."""
        attendees = self.get("attendees")
        return bool(attendees and len(attendees) > 0)

    def _compute_attendee_count(self) -> int:
        """Number of attendees."""
        attendees = self.get("attendees")
        return len(attendees) if attendees else 0

    def _compute_attendee_domains(self) -> list[str]:
        """List of unique email domains from attendees."""
        attendees = self.get("attendees")
        if not attendees:
            return []
        domains = set()
        for email in attendees:
            if isinstance(email, str) and "@" in email:
                domains.add(email.split("@")[1].lower())
        return sorted(domains)

    def _compute_has_meeting_link(self) -> bool:
        """True if event has a video conferencing link."""
        return bool(self._event.get("meeting_link"))

    def _compute_recurrence_key(self) -> str | None:
        """Identifier linking recurring event instances."""
        return self._event.get("recurrence_id")

    def _compute_title_lower(self) -> str:
        """Lowercase title for case-insensitive matching."""
        return (self._event.get("title") or "").lower()

    def _parse_datetime(self, value: str | datetime) -> datetime:
        """Parse a datetime value (string or datetime object)."""
        if isinstance(value, datetime):
            return value
        # Handle ISO format with optional timezone
        if isinstance(value, str):
            # Remove 'Z' suffix and replace with +00:00
            if value.endswith("Z"):
                value = value[:-1] + "+00:00"
            try:
                return datetime.fromisoformat(value)
            except ValueError:
                # Try parsing just the date
                return datetime.strptime(value[:10], "%Y-%m-%d")
        return datetime.now()


# Property registry - defines all available properties for rule creation UI
PROPERTY_REGISTRY: list[PropertyDefinition] = [
    # Native string properties
    PropertyDefinition("title", "Event Title", PropertyType.STRING, False,
                       "The event title/summary"),
    PropertyDefinition("description", "Description", PropertyType.STRING, False,
                       "The event description/notes"),
    PropertyDefinition("calendar_id", "Calendar ID", PropertyType.STRING, False,
                       "The Google Calendar ID"),
    PropertyDefinition("meeting_link", "Meeting Link", PropertyType.STRING, False,
                       "Video conferencing URL if present"),
    PropertyDefinition("event_color", "Event Color", PropertyType.STRING, False,
                       "Color assigned to the event"),
    PropertyDefinition("recurrence_id", "Recurrence ID", PropertyType.STRING, False,
                       "ID linking recurring event instances"),
    PropertyDefinition("my_response_status", "My Response Status", PropertyType.STRING, False,
                       "User's RSVP status: accepted, declined, needsAction, tentative"),
    PropertyDefinition("transparency", "Free/Busy Status", PropertyType.STRING, False,
                       "Event transparency: opaque (busy) or transparent (free)"),
    PropertyDefinition("visibility", "Visibility", PropertyType.STRING, False,
                       "Event visibility: default, public, private, or confidential"),

    # Native list properties
    PropertyDefinition("attendees", "Attendees", PropertyType.STRING_LIST, False,
                       "List of attendee email addresses"),

    # Computed string properties
    PropertyDefinition("weekday", "Day of Week", PropertyType.STRING, True,
                       "Day name: monday, tuesday, etc."),
    PropertyDefinition("time_block", "Time of Day", PropertyType.STRING, True,
                       "Time block: morning, afternoon, evening, night"),
    PropertyDefinition("title_lower", "Title (lowercase)", PropertyType.STRING, True,
                       "Lowercase title for case-insensitive matching"),

    # Computed integer properties
    PropertyDefinition("start_hour", "Start Hour", PropertyType.INTEGER, True,
                       "Hour of day (0-23) when event starts"),
    PropertyDefinition("end_hour", "End Hour", PropertyType.INTEGER, True,
                       "Hour of day (0-23) when event ends"),
    PropertyDefinition("duration_minutes", "Duration (minutes)", PropertyType.INTEGER, True,
                       "Event duration in minutes"),
    PropertyDefinition("attendee_count", "Attendee Count", PropertyType.INTEGER, True,
                       "Number of attendees"),

    # Computed boolean properties
    PropertyDefinition("is_recurring", "Is Recurring", PropertyType.BOOLEAN, True,
                       "True if event is part of a recurring series"),
    PropertyDefinition("is_all_day", "Is All Day", PropertyType.BOOLEAN, True,
                       "True if event is an all-day event"),
    PropertyDefinition("has_attendees", "Has Attendees", PropertyType.BOOLEAN, True,
                       "True if event has any attendees"),
    PropertyDefinition("has_meeting_link", "Has Meeting Link", PropertyType.BOOLEAN, True,
                       "True if event has a video conferencing link"),

    # Computed list properties
    PropertyDefinition("attendee_domains", "Attendee Domains", PropertyType.STRING_LIST, True,
                       "Unique email domains from attendees"),
]


def get_property_definitions() -> list[dict]:
    """Get property definitions as dicts for API response."""
    return [
        {
            "name": p.name,
            "display_name": p.display_name,
            "value_type": p.value_type.value,
            "is_computed": p.is_computed,
            "description": p.description,
        }
        for p in PROPERTY_REGISTRY
    ]


# =============================================================================
# Condition System
# =============================================================================


class ConditionEvaluator:
    """Evaluates conditions against property values.

    Each condition type is implemented as a static `_eval_<type>` method.
    To add a new condition type, simply add a new method following this pattern.
    """

    @staticmethod
    def evaluate(condition_type: str, property_value: Any, condition_value: Any) -> bool:
        """Evaluate a condition.

        Args:
            condition_type: The type of condition (equals, contains, etc.)
            property_value: The actual value from the event
            condition_value: The value to compare against (from the rule)

        Returns:
            True if condition is satisfied, False otherwise
        """
        if property_value is None:
            return False

        evaluator = getattr(ConditionEvaluator, f"_eval_{condition_type}", None)
        if not evaluator:
            raise ValueError(f"Unknown condition type: {condition_type}")

        return evaluator(property_value, condition_value)

    @staticmethod
    def get_available_conditions() -> list[dict]:
        """Get list of available condition types with metadata."""
        return [
            {"type": "equals", "display": "equals",
             "applies_to": ["string", "integer", "boolean"]},
            {"type": "not_equals", "display": "does not equal",
             "applies_to": ["string", "integer", "boolean"]},
            {"type": "contains", "display": "contains",
             "applies_to": ["string"]},
            {"type": "not_contains", "display": "does not contain",
             "applies_to": ["string"]},
            {"type": "starts_with", "display": "starts with",
             "applies_to": ["string"]},
            {"type": "ends_with", "display": "ends with",
             "applies_to": ["string"]},
            {"type": "matches", "display": "matches regex",
             "applies_to": ["string"]},
            {"type": "in_list", "display": "is one of",
             "applies_to": ["string"]},
            {"type": "list_contains", "display": "list contains",
             "applies_to": ["string_list"]},
            {"type": "list_any_match", "display": "any item matches",
             "applies_to": ["string_list"]},
            {"type": "greater_than", "display": "greater than",
             "applies_to": ["integer"]},
            {"type": "less_than", "display": "less than",
             "applies_to": ["integer"]},
            {"type": "between", "display": "between",
             "applies_to": ["integer"]},
        ]

    # =========================================================================
    # Condition Evaluators
    # =========================================================================

    @staticmethod
    def _eval_equals(prop_val: Any, cond_val: Any) -> bool:
        """Exact match (case-insensitive for strings)."""
        if isinstance(prop_val, str) and isinstance(cond_val, str):
            return prop_val.lower() == cond_val.lower()
        if isinstance(prop_val, bool) or isinstance(cond_val, bool):
            # Handle string "true"/"false" for booleans
            if isinstance(cond_val, str):
                cond_val = cond_val.lower() == "true"
            return prop_val == cond_val
        return prop_val == cond_val

    @staticmethod
    def _eval_not_equals(prop_val: Any, cond_val: Any) -> bool:
        """Inverse of equals."""
        return not ConditionEvaluator._eval_equals(prop_val, cond_val)

    @staticmethod
    def _eval_contains(prop_val: str, cond_val: str) -> bool:
        """Substring match (case-insensitive)."""
        if not isinstance(prop_val, str):
            return False
        return cond_val.lower() in prop_val.lower()

    @staticmethod
    def _eval_not_contains(prop_val: str, cond_val: str) -> bool:
        """Inverse of contains."""
        return not ConditionEvaluator._eval_contains(prop_val, cond_val)

    @staticmethod
    def _eval_starts_with(prop_val: str, cond_val: str) -> bool:
        """Prefix match (case-insensitive)."""
        if not isinstance(prop_val, str):
            return False
        return prop_val.lower().startswith(cond_val.lower())

    @staticmethod
    def _eval_ends_with(prop_val: str, cond_val: str) -> bool:
        """Suffix match (case-insensitive)."""
        if not isinstance(prop_val, str):
            return False
        return prop_val.lower().endswith(cond_val.lower())

    @staticmethod
    def _eval_matches(prop_val: str, cond_val: str) -> bool:
        """Regex match (case-insensitive)."""
        if not isinstance(prop_val, str):
            return False
        try:
            return bool(re.search(cond_val, prop_val, re.IGNORECASE))
        except re.error:
            return False

    @staticmethod
    def _eval_in_list(prop_val: str, cond_val: list | str) -> bool:
        """Value is one of several options."""
        if not isinstance(prop_val, str):
            return False
        # Handle JSON-encoded list
        if isinstance(cond_val, str):
            cond_val = json.loads(cond_val)
        return prop_val.lower() in [v.lower() for v in cond_val]

    @staticmethod
    def _eval_list_contains(prop_val: list, cond_val: str) -> bool:
        """List contains the specified value (case-insensitive)."""
        if not isinstance(prop_val, list):
            return False
        cond_lower = cond_val.lower()
        return any(
            cond_lower in str(v).lower()
            for v in prop_val
        )

    @staticmethod
    def _eval_list_any_match(prop_val: list, cond_val: str) -> bool:
        """Any list item matches the regex pattern."""
        if not isinstance(prop_val, list):
            return False
        try:
            pattern = re.compile(cond_val, re.IGNORECASE)
            return any(pattern.search(str(v)) for v in prop_val)
        except re.error:
            return False

    @staticmethod
    def _eval_greater_than(prop_val: int, cond_val: int | str) -> bool:
        """Numeric comparison: greater than."""
        try:
            return int(prop_val) > int(cond_val)
        except (ValueError, TypeError):
            return False

    @staticmethod
    def _eval_less_than(prop_val: int, cond_val: int | str) -> bool:
        """Numeric comparison: less than."""
        try:
            return int(prop_val) < int(cond_val)
        except (ValueError, TypeError):
            return False

    @staticmethod
    def _eval_between(prop_val: int, cond_val: list | str) -> bool:
        """Value in range (inclusive)."""
        try:
            if isinstance(cond_val, str):
                cond_val = json.loads(cond_val)
            low, high = cond_val
            return int(low) <= int(prop_val) <= int(high)
        except (ValueError, TypeError, json.JSONDecodeError):
            return False


# =============================================================================
# Rule Models
# =============================================================================


@dataclass
class RuleCondition:
    """A single condition within a rule."""
    id: int | None
    property_name: str
    condition_type: str
    condition_value: Any  # Parsed from JSON

    @classmethod
    def from_row(cls, row) -> "RuleCondition":
        """Create from database row (sqlite3.Row or dict)."""
        # Convert sqlite3.Row to dict if needed
        if hasattr(row, "keys"):
            row = dict(row)
        value = row["condition_value"]
        # Try to parse JSON
        try:
            value = json.loads(value)
        except (json.JSONDecodeError, TypeError):
            pass  # Keep as string
        return cls(
            id=row.get("id"),
            property_name=row["property_name"],
            condition_type=row["condition_type"],
            condition_value=value,
        )


@dataclass
class ClassificationRule:
    """A classification rule with its conditions."""
    id: int
    name: str
    project_id: int
    project_name: str | None
    priority: int
    is_enabled: bool
    stop_processing: bool
    conditions: list[RuleCondition]

    @classmethod
    def from_row(cls, row, conditions: list[RuleCondition] | None = None) -> "ClassificationRule":
        """Create from database row (sqlite3.Row or dict)."""
        # Convert sqlite3.Row to dict if needed
        if hasattr(row, "keys"):
            row = dict(row)
        return cls(
            id=row["id"],
            name=row["name"],
            project_id=row["project_id"],
            project_name=row.get("project_name"),
            priority=row.get("priority", 0),
            is_enabled=bool(row.get("is_enabled", 1)),
            stop_processing=bool(row.get("stop_processing", 1)),
            conditions=conditions or [],
        )

    def to_dict(self) -> dict:
        """Convert to dict for API response."""
        return {
            "id": self.id,
            "name": self.name,
            "project_id": self.project_id,
            "project_name": self.project_name,
            "priority": self.priority,
            "is_enabled": self.is_enabled,
            "stop_processing": self.stop_processing,
            "conditions": [
                {
                    "id": c.id,
                    "property_name": c.property_name,
                    "condition_type": c.condition_type,
                    "condition_value": c.condition_value,
                }
                for c in self.conditions
            ],
        }


@dataclass
class RuleMatch:
    """Result of matching rules against an event."""
    rule: ClassificationRule
    matched: bool
    condition_results: dict[str, bool]  # property_name -> matched

    def to_dict(self) -> dict:
        """Convert to dict for API response."""
        return {
            "rule": self.rule.to_dict(),
            "matched": self.matched,
            "condition_results": self.condition_results,
        }


# =============================================================================
# Rule Matcher
# =============================================================================


class RuleMatcher:
    """Matches classification rules against events."""

    def __init__(self, rules: list[ClassificationRule]):
        # Sort by priority descending, then by id for stable ordering
        self.rules = sorted(rules, key=lambda r: (-r.priority, r.id))

    def match(self, event: dict) -> ClassificationRule | None:
        """Find the first matching rule for an event.

        Returns the highest-priority rule whose conditions all match,
        or None if no rules match.
        """
        props = EventProperties(event)

        for rule in self.rules:
            if not rule.is_enabled:
                continue

            if self._evaluate_rule(rule, props):
                return rule

        return None

    def match_all(self, event: dict) -> list[RuleMatch]:
        """Evaluate all rules against an event, returning detailed results.

        Useful for debugging and rule management UI.
        """
        props = EventProperties(event)
        results = []

        for rule in self.rules:
            condition_results = {}
            all_matched = True

            for condition in rule.conditions:
                prop_value = props.get(condition.property_name)
                try:
                    matched = ConditionEvaluator.evaluate(
                        condition.condition_type,
                        prop_value,
                        condition.condition_value
                    )
                except ValueError:
                    matched = False
                condition_results[condition.property_name] = matched
                if not matched:
                    all_matched = False

            results.append(RuleMatch(
                rule=rule,
                matched=all_matched and rule.is_enabled,
                condition_results=condition_results
            ))

        return results

    def _evaluate_rule(self, rule: ClassificationRule, props: EventProperties) -> bool:
        """Evaluate all conditions for a rule. All must match (AND logic)."""
        if not rule.conditions:
            return False  # Rules without conditions never match

        for condition in rule.conditions:
            prop_value = props.get(condition.property_name)
            try:
                if not ConditionEvaluator.evaluate(
                    condition.condition_type,
                    prop_value,
                    condition.condition_value
                ):
                    return False
            except ValueError:
                return False
        return True


# =============================================================================
# Database Functions
# =============================================================================


def load_rules_with_conditions(db=None, enabled_only: bool = True) -> list[ClassificationRule]:
    """Load classification rules with their conditions from database.

    Args:
        db: Database connection (uses default if None)
        enabled_only: If True, only load enabled rules

    Returns:
        List of ClassificationRule objects with conditions populated
    """
    if db is None:
        db = get_db()

    # Build query
    where_clause = "WHERE r.is_enabled = 1" if enabled_only else ""

    # Get rules with project names
    rules_query = f"""
        SELECT r.*, p.name as project_name
        FROM classification_rules r
        JOIN projects p ON r.project_id = p.id
        {where_clause}
        ORDER BY r.priority DESC, r.id
    """
    rule_rows = db.execute(rules_query)

    if not rule_rows:
        return []

    # Get all conditions for these rules
    rule_ids = [r["id"] for r in rule_rows]
    placeholders = ",".join("?" * len(rule_ids))
    conditions_query = f"""
        SELECT * FROM rule_conditions
        WHERE rule_id IN ({placeholders})
        ORDER BY rule_id, id
    """
    condition_rows = db.execute(conditions_query, rule_ids)

    # Group conditions by rule_id
    conditions_by_rule: dict[int, list[RuleCondition]] = {}
    for row in condition_rows:
        rule_id = row["rule_id"]
        if rule_id not in conditions_by_rule:
            conditions_by_rule[rule_id] = []
        conditions_by_rule[rule_id].append(RuleCondition.from_row(row))

    # Build rule objects
    rules = []
    for row in rule_rows:
        conditions = conditions_by_rule.get(row["id"], [])
        rules.append(ClassificationRule.from_row(row, conditions))

    return rules


def create_rule(
    db,
    name: str,
    project_id: int,
    conditions: list[dict],
    priority: int = 0,
    is_enabled: bool = True,
    stop_processing: bool = True,
) -> int:
    """Create a new classification rule with conditions.

    Args:
        db: Database connection
        name: Human-readable rule name
        project_id: Project to classify matching events to
        conditions: List of condition dicts with property_name, condition_type, condition_value
        priority: Higher priority rules are evaluated first
        is_enabled: Whether rule is active
        stop_processing: If True, stop evaluating lower-priority rules on match

    Returns:
        ID of the created rule
    """
    # Insert rule
    rule_id = db.execute_insert(
        """
        INSERT INTO classification_rules (name, project_id, priority, is_enabled, stop_processing)
        VALUES (?, ?, ?, ?, ?)
        """,
        (name, project_id, priority, int(is_enabled), int(stop_processing))
    )

    # Insert conditions
    for cond in conditions:
        # Serialize condition_value to JSON if needed
        value = cond["condition_value"]
        if not isinstance(value, str):
            value = json.dumps(value)

        db.execute_insert(
            """
            INSERT INTO rule_conditions (rule_id, property_name, condition_type, condition_value)
            VALUES (?, ?, ?, ?)
            """,
            (rule_id, cond["property_name"], cond["condition_type"], value)
        )

    return rule_id


def update_rule(
    db,
    rule_id: int,
    name: str | None = None,
    project_id: int | None = None,
    priority: int | None = None,
    is_enabled: bool | None = None,
    stop_processing: bool | None = None,
    conditions: list[dict] | None = None,
) -> bool:
    """Update an existing rule.

    Args:
        db: Database connection
        rule_id: ID of rule to update
        Other args: Fields to update (None = don't update)
        conditions: If provided, replaces all existing conditions

    Returns:
        True if rule was found and updated
    """
    # Build update for rule fields
    updates = []
    values = []

    if name is not None:
        updates.append("name = ?")
        values.append(name)
    if project_id is not None:
        updates.append("project_id = ?")
        values.append(project_id)
    if priority is not None:
        updates.append("priority = ?")
        values.append(priority)
    if is_enabled is not None:
        updates.append("is_enabled = ?")
        values.append(int(is_enabled))
    if stop_processing is not None:
        updates.append("stop_processing = ?")
        values.append(int(stop_processing))

    if updates:
        updates.append("updated_at = CURRENT_TIMESTAMP")
        values.append(rule_id)
        db.execute(
            f"UPDATE classification_rules SET {', '.join(updates)} WHERE id = ?",
            values
        )

    # Replace conditions if provided
    if conditions is not None:
        db.execute("DELETE FROM rule_conditions WHERE rule_id = ?", (rule_id,))
        for cond in conditions:
            value = cond["condition_value"]
            if not isinstance(value, str):
                value = json.dumps(value)
            db.execute_insert(
                """
                INSERT INTO rule_conditions (rule_id, property_name, condition_type, condition_value)
                VALUES (?, ?, ?, ?)
                """,
                (rule_id, cond["property_name"], cond["condition_type"], value)
            )

    return True


def delete_rule(db, rule_id: int) -> bool:
    """Delete a rule and its conditions.

    Args:
        db: Database connection
        rule_id: ID of rule to delete

    Returns:
        True if rule was found and deleted
    """
    # Conditions are deleted via ON DELETE CASCADE
    result = db.execute(
        "DELETE FROM classification_rules WHERE id = ?",
        (rule_id,)
    )
    return True


# =============================================================================
# High-Level Functions
# =============================================================================


def suggest_classification(event_id: int) -> dict | None:
    """Suggest a project classification for an event based on rules.

    Returns:
        dict with project_id, rule_id, and confidence, or None if no suggestion
    """
    db = get_db()

    event = db.execute_one("SELECT * FROM events WHERE id = ?", (event_id,))
    if event is None:
        return None

    # Load rules and match
    rules = load_rules_with_conditions(db, enabled_only=True)
    matcher = RuleMatcher(rules)

    matching_rule = matcher.match(dict(event))
    if matching_rule:
        return {
            "project_id": matching_rule.project_id,
            "project_name": matching_rule.project_name,
            "rule_id": matching_rule.id,
            "rule_name": matching_rule.name,
            "confidence": 0.9,  # High confidence for rule match
        }

    # Fall back to recurrence-based matching
    if event["recurrence_id"]:
        previous_entry = db.execute_one(
            """
            SELECT te.project_id, p.name as project_name
            FROM time_entries te
            JOIN events e ON te.event_id = e.id
            JOIN projects p ON te.project_id = p.id
            WHERE e.recurrence_id = ? AND e.id != ?
            ORDER BY e.start_time DESC
            LIMIT 1
            """,
            (event["recurrence_id"], event_id),
        )
        if previous_entry:
            return {
                "project_id": previous_entry["project_id"],
                "project_name": previous_entry["project_name"],
                "rule_id": None,
                "rule_name": None,
                "confidence": 0.85,  # Slightly lower for recurrence match
            }

    return None


def classify_event_by_rules(event: dict, db=None) -> ClassificationRule | None:
    """Attempt to classify an event using rules.

    Args:
        event: Event dict from database
        db: Database connection (uses default if None)

    Returns:
        Matching rule, or None if no rules match
    """
    if db is None:
        db = get_db()

    rules = load_rules_with_conditions(db, enabled_only=True)
    matcher = RuleMatcher(rules)
    return matcher.match(event)


def test_rules_against_event(event_id: int) -> list[dict]:
    """Test all rules against an event for debugging.

    Returns detailed match results for each rule.
    """
    db = get_db()

    event = db.execute_one("SELECT * FROM events WHERE id = ?", (event_id,))
    if event is None:
        return []

    rules = load_rules_with_conditions(db, enabled_only=False)
    matcher = RuleMatcher(rules)
    results = matcher.match_all(dict(event))

    return [r.to_dict() for r in results]


def get_event_properties(event_id: int) -> dict | None:
    """Get all computed properties for an event (for debugging)."""
    db = get_db()

    event = db.execute_one("SELECT * FROM events WHERE id = ?", (event_id,))
    if event is None:
        return None

    props = EventProperties(dict(event))
    return props.get_all()


def create_rule_from_classification(event_id: int, project_id: int) -> None:
    """Placeholder for future learning from manual classifications."""
    # TODO: Implement learning from manual classifications
    pass
