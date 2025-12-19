"""
Query evaluator for Rules v2.

Evaluates parsed queries against calendar events.
"""

import json
import re
from datetime import datetime
from typing import Any

from services.query_parser import AndGroup, OrGroup, Term, parse_query


class QueryEvaluator:
    """Evaluates queries against calendar events."""

    # Day of week mapping
    DAY_NAMES = {
        'mon': 0, 'monday': 0,
        'tue': 1, 'tuesday': 1,
        'wed': 2, 'wednesday': 2,
        'thu': 3, 'thursday': 3,
        'fri': 4, 'friday': 4,
        'sat': 5, 'saturday': 5,
        'sun': 6, 'sunday': 6,
    }

    def __init__(self, event: dict):
        """Initialize with an event dict.

        Event should have keys matching database columns:
        - title, description, start_time, end_time
        - attendees (JSON string or list)
        - is_recurring, recurrence_id
        - my_response_status
        - transparency, visibility
        - event_color
        """
        self.event = event
        self._attendees_cache = None
        self._domains_cache = None

    @property
    def attendees(self) -> list[str]:
        """Get attendees as a list of email addresses."""
        if self._attendees_cache is not None:
            return self._attendees_cache

        attendees = self.event.get('attendees', [])
        if isinstance(attendees, str):
            try:
                attendees = json.loads(attendees)
            except (json.JSONDecodeError, TypeError):
                attendees = []

        self._attendees_cache = attendees or []
        return self._attendees_cache

    @property
    def domains(self) -> set[str]:
        """Extract unique domains from attendee email addresses."""
        if self._domains_cache is not None:
            return self._domains_cache

        domains = set()
        for email in self.attendees:
            if '@' in email:
                domain = email.split('@')[1].lower()
                domains.add(domain)

        self._domains_cache = domains
        return self._domains_cache

    def evaluate(self, ast: AndGroup) -> bool:
        """Evaluate the query AST against the event.

        Returns True if the event matches the query.
        """
        if not ast.items:
            return False

        # AND: all items must match
        return all(self._evaluate_item(item) for item in ast.items)

    def _evaluate_item(self, item: Term | OrGroup) -> bool:
        """Evaluate a single item (term or group)."""
        if isinstance(item, OrGroup):
            # OR: any term must match
            return any(self._evaluate_item(t) for t in item.terms)
        elif isinstance(item, Term):
            return self._evaluate_term(item)
        return False

    def _normalize_for_match(self, text: str) -> str:
        """Normalize text for matching by stripping quotes."""
        # Strip both straight and curly quotes
        return text.replace('"', '').replace("'", '').replace('"', '').replace('"', '').replace(''', '').replace(''', '')

    def _evaluate_term(self, term: Term) -> bool:
        """Evaluate a single term against the event."""
        prop = term.property
        value = self._normalize_for_match(term.value.lower())

        # String properties (contains matching, case-insensitive)
        if prop == 'title':
            title = self._normalize_for_match((self.event.get('title') or '').lower())
            return value in title

        if prop == 'description':
            desc = self._normalize_for_match((self.event.get('description') or '').lower())
            return value in desc

        # Attendee matching
        if prop == 'attendees':
            # Match against name or email (contains)
            for email in self.attendees:
                if value in self._normalize_for_match(email.lower()):
                    return True
            return False

        if prop == 'domain':
            # Exact domain match
            return value in self.domains

        if prop == 'email':
            # Exact email match (case-insensitive)
            return value in [e.lower() for e in self.attendees]

        # Response status
        if prop == 'response':
            response = (self.event.get('my_response_status') or '').lower()
            # Support common aliases
            if value in ('needsaction', 'needs-action', 'needs_action', 'pending'):
                return response == 'needsaction'
            return response == value

        # Boolean properties
        if prop == 'recurring':
            is_recurring = self.event.get('is_recurring', False)
            return self._match_boolean(value, is_recurring)

        if prop == 'is-all-day':
            # Check if start_time has no time component (ends with T00:00:00)
            start = self.event.get('start_time', '')
            is_all_day = 'T00:00:00' in start and self.event.get('end_time', '').endswith('T00:00:00')
            return self._match_boolean(value, is_all_day)

        if prop == 'has-attendees':
            has_attendees = len(self.attendees) > 0
            return self._match_boolean(value, has_attendees)

        # Transparency (free/busy)
        if prop == 'transparency':
            transparency = (self.event.get('transparency') or 'opaque').lower()
            # Support 'free' as alias for 'transparent', 'busy' for 'opaque'
            if value == 'free':
                return transparency == 'transparent'
            if value == 'busy':
                return transparency == 'opaque'
            return transparency == value

        # Visibility
        if prop == 'visibility':
            visibility = (self.event.get('visibility') or 'default').lower()
            return visibility == value

        # Day of week
        if prop == 'day-of-week':
            start = self.event.get('start_time', '')
            if not start:
                return False
            try:
                dt = datetime.fromisoformat(start.replace('Z', '+00:00'))
                event_day = dt.weekday()  # 0 = Monday
                target_day = self.DAY_NAMES.get(value.lower())
                if target_day is not None:
                    return event_day == target_day
            except ValueError:
                return False
            return False

        # Time of day (supports >, <, >= , <= operators)
        if prop == 'time-of-day':
            start = self.event.get('start_time', '')
            if not start:
                return False
            try:
                dt = datetime.fromisoformat(start.replace('Z', '+00:00'))
                event_time = dt.hour * 60 + dt.minute  # minutes since midnight

                # Parse operator and time
                match = re.match(r'([<>]=?)?(\d{1,2}):(\d{2})', value)
                if match:
                    op = match.group(1) or '='
                    hours = int(match.group(2))
                    minutes = int(match.group(3))
                    target_time = hours * 60 + minutes

                    if op == '>':
                        return event_time > target_time
                    elif op == '>=':
                        return event_time >= target_time
                    elif op == '<':
                        return event_time < target_time
                    elif op == '<=':
                        return event_time <= target_time
                    else:
                        return event_time == target_time
            except ValueError:
                return False
            return False

        # Calendar color
        if prop == 'color':
            color = self.event.get('event_color') or ''
            return str(color).lower() == value

        # Recurrence ID (for LLM use)
        if prop == 'recurrence-id':
            recurrence_id = self.event.get('recurrence_id') or ''
            return recurrence_id.lower() == value

        # Unknown property - no match
        return False

    def _match_boolean(self, value: str, actual: bool) -> bool:
        """Match a boolean value."""
        true_values = ('yes', 'true', '1', 'on')
        false_values = ('no', 'false', '0', 'off')

        if value in true_values:
            return actual is True
        elif value in false_values:
            return actual is False
        return False


def evaluate_query(query: str, event: dict) -> bool:
    """Evaluate a query string against an event.

    Args:
        query: Query string like 'domain:foo.com title:"meeting"'
        event: Event dict with database column names

    Returns:
        True if event matches the query
    """
    ast = parse_query(query)
    evaluator = QueryEvaluator(event)
    return evaluator.evaluate(ast)


def find_matching_events(query: str, events: list[dict]) -> list[dict]:
    """Find all events matching a query.

    Args:
        query: Query string
        events: List of event dicts

    Returns:
        List of events that match the query
    """
    if not query or not query.strip():
        return []

    ast = parse_query(query)
    results = []

    for event in events:
        evaluator = QueryEvaluator(event)
        if evaluator.evaluate(ast):
            results.append(event)

    return results
