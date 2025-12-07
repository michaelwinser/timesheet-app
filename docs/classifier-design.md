# Technical Design: Rules-Based Classifier

## 1. Overview

The classifier automatically assigns projects to calendar events based on user-defined rules. Rules match event properties (native or computed) against conditions, and when a match occurs, the event is classified to the associated project.

**Scope for Slice 2:**
- Rule data model and storage
- Property system with native and computed properties
- Rule matching algorithm
- Integration with sync and display flows

**Deferred to later slices:**
- Rule learning from manual classifications
- Suggested vs auto-classify confidence thresholds
- Bulk classification UI
- Rule management UI

## 2. Design Goals

1. **Extensible properties** - Adding a new matchable property (native or computed) should require minimal code changes
2. **Extensible conditions** - Adding new condition types (contains, regex, etc.) should be straightforward
3. **Predictable matching** - Clear priority system when multiple rules could match
4. **Debuggable** - Easy to understand why a particular rule matched or didn't match
5. **Performant** - Rule evaluation should be fast enough for real-time use during sync

## 3. Property System

### 3.1 Property Types

Properties are values that can be extracted from an event and used in rule conditions.

| Category | Examples |
|----------|----------|
| Native string | title, description, calendar_id, meeting_link |
| Native list | attendees (list of email addresses) |
| Native temporal | start_time, end_time |
| Computed temporal | weekday, start_hour, duration_minutes |
| Computed boolean | is_recurring, is_all_day, has_attendees |
| Computed string | organizer (extracted from attendees), recurrence_key |

### 3.2 Property Registry

A central registry defines available properties and their metadata:

```python
@dataclass
class PropertyDefinition:
    name: str                    # Unique identifier: "weekday", "title", etc.
    display_name: str            # Human-readable: "Day of Week", "Event Title"
    value_type: PropertyType     # STRING, STRING_LIST, INTEGER, BOOLEAN, DATETIME
    is_computed: bool            # True if derived, False if native
    description: str             # Help text for rule creation UI
```

```python
class PropertyType(Enum):
    STRING = "string"
    STRING_LIST = "string_list"  # For attendees, etc.
    INTEGER = "integer"          # For hours, durations, etc.
    BOOLEAN = "boolean"
    DATETIME = "datetime"
```

### 3.3 Event Property Accessor

The `EventProperties` class wraps a raw event and provides uniform access to all properties:

```python
class EventProperties:
    """Provides access to native and computed event properties."""

    def __init__(self, event: dict):
        self._event = event
        self._cache = {}  # Cache computed properties

    def get(self, property_name: str) -> Any:
        """Get property value by name. Returns None if property doesn't exist."""
        if property_name in self._cache:
            return self._cache[property_name]

        # Try native property first
        if property_name in self._event:
            return self._event[property_name]

        # Try computed property
        method = getattr(self, f"_compute_{property_name}", None)
        if method:
            value = method()
            self._cache[property_name] = value
            return value

        return None

    # --- Computed properties ---

    def _compute_weekday(self) -> str:
        """Day of week: 'monday', 'tuesday', etc."""
        dt = datetime.fromisoformat(self._event["start_time"])
        return dt.strftime("%A").lower()

    def _compute_start_hour(self) -> int:
        """Hour of day (0-23) when event starts."""
        dt = datetime.fromisoformat(self._event["start_time"])
        return dt.hour

    def _compute_duration_minutes(self) -> int:
        """Event duration in minutes."""
        start = datetime.fromisoformat(self._event["start_time"])
        end = datetime.fromisoformat(self._event["end_time"])
        return int((end - start).total_seconds() / 60)

    def _compute_is_recurring(self) -> bool:
        """True if event is part of a recurring series."""
        return bool(self._event.get("is_recurring") or self._event.get("recurrence_id"))

    def _compute_is_all_day(self) -> bool:
        """True if event is an all-day event."""
        # All-day events typically have times at midnight or use date-only format
        start = self._event.get("start_time", "")
        return "T" not in start or start.endswith("T00:00:00")

    def _compute_has_attendees(self) -> bool:
        """True if event has any attendees."""
        attendees = self._event.get("attendees")
        if isinstance(attendees, str):
            import json
            attendees = json.loads(attendees) if attendees else []
        return len(attendees) > 0

    def _compute_attendee_domains(self) -> list[str]:
        """List of unique email domains from attendees."""
        attendees = self._event.get("attendees")
        if isinstance(attendees, str):
            import json
            attendees = json.loads(attendees) if attendees else []
        domains = set()
        for email in attendees:
            if "@" in email:
                domains.add(email.split("@")[1].lower())
        return sorted(domains)

    def _compute_recurrence_key(self) -> str | None:
        """Identifier linking recurring event instances."""
        return self._event.get("recurrence_id")
```

### 3.4 Adding New Properties

To add a new computed property:

1. Add a `_compute_<property_name>` method to `EventProperties`
2. Register it in the property registry (for UI discovery)
3. No changes needed to rule matching logic

Example - adding "time_block" (morning/afternoon/evening):

```python
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
```

## 4. Condition System

### 4.1 Condition Types

Conditions define how a property value is tested:

| Condition | Applies To | Description |
|-----------|------------|-------------|
| equals | STRING, INTEGER, BOOLEAN | Exact match |
| not_equals | STRING, INTEGER, BOOLEAN | Inverse of equals |
| contains | STRING | Substring match (case-insensitive) |
| starts_with | STRING | Prefix match |
| ends_with | STRING | Suffix match |
| matches | STRING | Regex match |
| in_list | STRING | Value is one of several options |
| list_contains | STRING_LIST | List contains the specified value |
| list_any_match | STRING_LIST | Any list item matches pattern |
| greater_than | INTEGER | Numeric comparison |
| less_than | INTEGER | Numeric comparison |
| between | INTEGER | Value in range (inclusive) |

### 4.2 Condition Evaluator

```python
class ConditionEvaluator:
    """Evaluates conditions against property values."""

    @staticmethod
    def evaluate(condition_type: str, property_value: Any, condition_value: Any) -> bool:
        """
        Evaluate a condition.

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
    def _eval_equals(prop_val: Any, cond_val: Any) -> bool:
        if isinstance(prop_val, str) and isinstance(cond_val, str):
            return prop_val.lower() == cond_val.lower()
        return prop_val == cond_val

    @staticmethod
    def _eval_not_equals(prop_val: Any, cond_val: Any) -> bool:
        return not ConditionEvaluator._eval_equals(prop_val, cond_val)

    @staticmethod
    def _eval_contains(prop_val: str, cond_val: str) -> bool:
        return cond_val.lower() in prop_val.lower()

    @staticmethod
    def _eval_starts_with(prop_val: str, cond_val: str) -> bool:
        return prop_val.lower().startswith(cond_val.lower())

    @staticmethod
    def _eval_ends_with(prop_val: str, cond_val: str) -> bool:
        return prop_val.lower().endswith(cond_val.lower())

    @staticmethod
    def _eval_matches(prop_val: str, cond_val: str) -> bool:
        import re
        return bool(re.search(cond_val, prop_val, re.IGNORECASE))

    @staticmethod
    def _eval_in_list(prop_val: str, cond_val: list[str]) -> bool:
        return prop_val.lower() in [v.lower() for v in cond_val]

    @staticmethod
    def _eval_list_contains(prop_val: list, cond_val: str) -> bool:
        return cond_val.lower() in [v.lower() for v in prop_val]

    @staticmethod
    def _eval_list_any_match(prop_val: list, cond_val: str) -> bool:
        import re
        pattern = re.compile(cond_val, re.IGNORECASE)
        return any(pattern.search(v) for v in prop_val)

    @staticmethod
    def _eval_greater_than(prop_val: int, cond_val: int) -> bool:
        return prop_val > cond_val

    @staticmethod
    def _eval_less_than(prop_val: int, cond_val: int) -> bool:
        return prop_val < cond_val

    @staticmethod
    def _eval_between(prop_val: int, cond_val: tuple[int, int]) -> bool:
        low, high = cond_val
        return low <= prop_val <= high
```

### 4.3 Adding New Conditions

To add a new condition type:

1. Add a `_eval_<condition_type>` method to `ConditionEvaluator`
2. Document which property types it applies to
3. No changes needed to rule matching logic

## 5. Rule Data Model

### 5.1 Database Schema

```sql
-- Classification rules
CREATE TABLE classification_rules (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,                    -- User-friendly name
    project_id INTEGER NOT NULL REFERENCES projects(id),
    priority INTEGER DEFAULT 0,            -- Higher = evaluated first
    is_enabled INTEGER DEFAULT 1,          -- Can disable without deleting
    stop_processing INTEGER DEFAULT 1,     -- If true, don't evaluate lower-priority rules
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Rule conditions (a rule can have multiple conditions, all must match)
CREATE TABLE rule_conditions (
    id INTEGER PRIMARY KEY,
    rule_id INTEGER NOT NULL REFERENCES classification_rules(id) ON DELETE CASCADE,
    property_name TEXT NOT NULL,           -- e.g., "title", "weekday", "attendees"
    condition_type TEXT NOT NULL,          -- e.g., "contains", "equals", "list_contains"
    condition_value TEXT NOT NULL,         -- JSON-encoded value
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Index for efficient rule lookup
CREATE INDEX idx_rules_priority ON classification_rules(priority DESC, id);
CREATE INDEX idx_conditions_rule ON rule_conditions(rule_id);
```

### 5.2 Rule Structure

A rule consists of:
- **Metadata**: name, project, priority, enabled flag
- **Conditions**: One or more conditions that ALL must match (AND logic)
- **Action**: Classify to the associated project

For OR logic, create multiple rules with the same project.

Example rules:

```
Rule: "Daily Standup"
  Priority: 100
  Project: "Internal Meetings"
  Conditions:
    - title contains "standup"
    - weekday in_list ["monday", "tuesday", "wednesday", "thursday", "friday"]

Rule: "Client Acme Meetings"
  Priority: 90
  Project: "Acme Corp"
  Conditions:
    - attendees list_any_match "@acme\\.com$"

Rule: "Friday Afternoon Off"
  Priority: 50
  Project: "Personal"
  Conditions:
    - weekday equals "friday"
    - start_hour greater_than 14
```

### 5.3 Pydantic Models

```python
from pydantic import BaseModel
from typing import Any

class RuleCondition(BaseModel):
    property_name: str
    condition_type: str
    condition_value: Any  # Parsed from JSON

class ClassificationRule(BaseModel):
    id: int
    name: str
    project_id: int
    project_name: str | None = None  # Joined from projects table
    priority: int
    is_enabled: bool
    stop_processing: bool
    conditions: list[RuleCondition]

class RuleMatch(BaseModel):
    """Result of matching rules against an event."""
    rule: ClassificationRule
    matched: bool
    condition_results: dict[str, bool]  # property_name -> matched
```

## 6. Rule Matching Algorithm

### 6.1 Matcher Class

```python
class RuleMatcher:
    """Matches classification rules against events."""

    def __init__(self, rules: list[ClassificationRule]):
        # Sort by priority descending, then by id for stable ordering
        self.rules = sorted(rules, key=lambda r: (-r.priority, r.id))

    def match(self, event: dict) -> ClassificationRule | None:
        """
        Find the first matching rule for an event.

        Returns the highest-priority rule whose conditions all match,
        or None if no rules match.
        """
        props = EventProperties(event)

        for rule in self.rules:
            if not rule.is_enabled:
                continue

            if self._evaluate_rule(rule, props):
                return rule

            # If this rule has stop_processing and matched, we'd have returned
            # If it didn't match but has stop_processing, continue to next rule

        return None

    def match_all(self, event: dict) -> list[RuleMatch]:
        """
        Evaluate all rules against an event, returning detailed results.
        Useful for debugging and rule management UI.
        """
        props = EventProperties(event)
        results = []

        for rule in self.rules:
            condition_results = {}
            all_matched = True

            for condition in rule.conditions:
                prop_value = props.get(condition.property_name)
                matched = ConditionEvaluator.evaluate(
                    condition.condition_type,
                    prop_value,
                    condition.condition_value
                )
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
        for condition in rule.conditions:
            prop_value = props.get(condition.property_name)
            if not ConditionEvaluator.evaluate(
                condition.condition_type,
                prop_value,
                condition.condition_value
            ):
                return False
        return True
```

### 6.2 Integration Points

**On Sync (services/calendar.py):**
```python
def sync_calendar_events(...):
    # ... fetch events from Google ...

    # Load enabled rules
    rules = load_enabled_rules(db)
    matcher = RuleMatcher(rules)

    for event in events:
        # Store event in DB
        event_id = store_event(db, event)

        # Check if already classified
        if is_classified(db, event_id):
            continue

        # Try to auto-classify
        matching_rule = matcher.match(event)
        if matching_rule:
            create_time_entry(
                db,
                event_id=event_id,
                project_id=matching_rule.project_id,
                hours=calculate_hours(event),
                description=event["title"],
                classification_source="rule",
                rule_id=matching_rule.id
            )
```

**On Display (routes/ui.py):**
```python
def week_view(...):
    # ... load events ...

    # For unclassified events, show suggested classification
    rules = load_enabled_rules(db)
    matcher = RuleMatcher(rules)

    for event in events:
        if not event["is_classified"]:
            matching_rule = matcher.match(event)
            if matching_rule:
                event["suggested_project_id"] = matching_rule.project_id
                event["suggested_project_name"] = matching_rule.project_name
                event["suggested_rule_name"] = matching_rule.name
```

## 7. Classification Source Tracking

Track how each time entry was classified for debugging and learning:

```sql
-- Add to time_entries table
ALTER TABLE time_entries ADD COLUMN rule_id INTEGER REFERENCES classification_rules(id);
```

Classification sources:
- `manual` - User selected project from dropdown
- `rule` - Auto-classified by a rule (rule_id populated)
- `bulk` - Classified via bulk action
- `suggested` - User accepted a suggestion (future)

## 8. API Endpoints (Future)

For rule management UI (not in Slice 2 scope):

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/rules` | GET | List all rules |
| `/api/rules` | POST | Create rule |
| `/api/rules/{id}` | GET | Get rule with conditions |
| `/api/rules/{id}` | PUT | Update rule |
| `/api/rules/{id}` | DELETE | Delete rule |
| `/api/rules/{id}/test` | POST | Test rule against sample events |
| `/api/properties` | GET | List available properties for rules |

## 9. Implementation Plan

### Phase 1: Core Infrastructure
1. Create `EventProperties` class with initial computed properties
2. Create `ConditionEvaluator` with basic condition types
3. Create `RuleMatcher` class
4. Add `rule_conditions` table and update `classification_rules`
5. Add `rule_id` column to `time_entries`

### Phase 2: Integration
6. Integrate matcher into calendar sync service
7. Add suggested classification to week view
8. Update UI to show suggestion indicator on event cards

### Phase 3: Seed Rules (Manual)
9. Add a few test rules via direct DB inserts or simple script
10. Test end-to-end: sync → auto-classify → display

## 10. Open Questions

1. **Condition value serialization**: JSON is flexible but requires careful parsing. Should we use a more structured format?

2. **Rule validation**: How do we validate that a condition type is compatible with a property type? Enforce at creation time?

3. **Performance**: For many rules, should we index/optimize? Current design evaluates all rules sequentially.

4. **Suggestion UI**: How should suggested classifications appear? Subtle indicator, or pre-filled dropdown?

5. **Manual override tracking**: If user manually classifies after auto-classification, should we track this as a signal for rule refinement?

6. **Classification timing and decoupling**: The current implementation tightly couples classification with calendar sync (classifier is invoked inline during sync). This raises several design questions:

   **When should classification run?**
   - Option A: During sync (current implementation) - simple, but couples sync and classify
   - Option B: As a separate batch operation triggered after sync completes
   - Option C: Only on explicit user request (manual "Classify" button)
   - Option D: Hybrid - auto-classify new events on sync, but provide separate reclassify for existing events

   **When should re-classification run?**
   - After rule changes (automatically, or on explicit request?)
   - On manual "Reclassify" button click for current week
   - On bulk selection of specific events (when bulk operations are implemented)

   **What should reclassification do with existing classifications?**
   - Option A: Only classify unclassified events (never overwrite)
   - Option B: Overwrite all classifications (except manual?)
   - Option C: Overwrite only rule-based classifications, preserve manual
   - Option D: User-selectable mode via checkbox/option

   **Preferred direction**: Decouple sync from classification. Sync should focus on fetching and storing events. Classification should be a separate concern with explicit backend batch operations and user-controllable UI triggers (e.g., "Reclassify Week" button). Avoid per-event frontend calls to classification APIs.

## 11. Future Considerations

- **Rule learning**: Analyze manual classifications to suggest new rules
- **Confidence scoring**: Based on rule specificity, historical accuracy
- **Rule conflicts**: Warn when rules might conflict
- **Rule templates**: Pre-built rules for common patterns
- **Import/export**: Share rules between instances
