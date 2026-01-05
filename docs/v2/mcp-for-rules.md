# MCP Server for Classification Rules

This document outlines approaches for exposing the classification rules system via an MCP (Model Context Protocol) server, enabling LLMs to create and manage rules on behalf of users.

## Background

The classification system uses a Gmail-style query syntax for rules. The challenge is communicating this syntax effectively to an LLM so it can generate valid, useful rules.

### Current Rules Syntax

```
# Basic property matching
title:standup                    # Title contains "standup"
domain:acme.com                  # Attendee from acme.com
email:bob@example.com            # Specific attendee email

# Boolean properties
is-all-day:yes
has-attendees:no
recurring:yes

# Response status
response:declined
response:accepted
response:needsAction

# Time-based
day-of-week:sat
day-of-week:sun
time-of-day:>17:00
time-of-day:<09:00

# Logical operators
title:standup title:daily        # Implicit AND
title:standup OR title:sync      # Explicit OR
(domain:a.com OR domain:b.com)   # Grouping

# Negation
-title:canceled

# Quoted strings for multi-word values
title:"team meeting"
```

### Available Properties

| Property | Type | Description |
|----------|------|-------------|
| `title` | string | Event title (contains, case-insensitive) |
| `description` | string | Event description (contains) |
| `attendees` | string | Attendee name or email (contains) |
| `domain` | string | Attendee email domain (exact match) |
| `email` | string | Attendee email (exact match) |
| `response` | enum | User's response: accepted, declined, needsAction, tentative |
| `recurring` | boolean | yes/no |
| `transparency` | enum | opaque (busy) or transparent (free) |
| `is-all-day` | boolean | yes/no |
| `has-attendees` | boolean | yes/no |
| `day-of-week` | enum | mon, tue, wed, thu, fri, sat, sun |
| `time-of-day` | time | HH:MM with operators: >, >=, <, <=, = |
| `calendar` | string | Calendar name (contains) |
| `text` | string | Searches title, description, and attendees |

---

## MCP Integration Approaches

### Option 1: Structured Tool Parameters (Recommended)

Instead of having the LLM write raw query strings, expose tools with structured parameters that the server assembles into queries.

**Advantages:**
- LLM doesn't need to learn syntax
- Type-safe, validated at schema level
- Harder to produce invalid queries

**Tool Definition:**

```json
{
  "name": "create_classification_rule",
  "description": "Create a rule to automatically classify calendar events to a project based on conditions",
  "inputSchema": {
    "type": "object",
    "required": ["project_id", "conditions"],
    "properties": {
      "project_id": {
        "type": "string",
        "description": "The project ID to classify matching events to"
      },
      "conditions": {
        "type": "array",
        "description": "Conditions that must match for the rule to apply",
        "items": {
          "type": "object",
          "required": ["property", "value"],
          "properties": {
            "property": {
              "type": "string",
              "enum": [
                "title", "description", "attendees", "domain", "email",
                "response", "recurring", "transparency", "is-all-day",
                "has-attendees", "day-of-week", "time-of-day", "calendar", "text"
              ],
              "description": "The event property to match against"
            },
            "value": {
              "type": "string",
              "description": "The value to match. For boolean properties use 'yes'/'no'. For time-of-day use 'HH:MM' with optional operator (e.g., '>17:00')"
            },
            "negated": {
              "type": "boolean",
              "default": false,
              "description": "If true, matches events that DON'T have this property value"
            }
          }
        }
      },
      "logic": {
        "type": "string",
        "enum": ["AND", "OR"],
        "default": "AND",
        "description": "How to combine multiple conditions"
      },
      "weight": {
        "type": "number",
        "default": 1.0,
        "description": "Scoring weight for classification confidence"
      }
    }
  }
}
```

**Example Usage:**

LLM receives: "Create a rule to classify meetings with Acme Corp to the Acme project"

LLM calls:
```json
{
  "name": "create_classification_rule",
  "arguments": {
    "project_id": "proj_acme_123",
    "conditions": [
      { "property": "domain", "value": "acmecorp.com" }
    ]
  }
}
```

Server assembles: `domain:acmecorp.com`

---

### Option 2: Documentation as MCP Resource

Expose the syntax documentation as a fetchable resource that the LLM can read when needed.

**Resource Definition:**

```json
{
  "uri": "timesheet://docs/rules-syntax",
  "name": "Classification Rules Syntax Reference",
  "description": "Complete reference for the Gmail-style query syntax used in classification rules",
  "mimeType": "text/markdown"
}
```

**Advantages:**
- Keeps tool descriptions concise
- LLM can fetch full docs only when needed
- Easy to update documentation independently

**Workflow:**
1. LLM receives request to create a rule
2. LLM fetches `timesheet://docs/rules-syntax` resource
3. LLM reads syntax reference
4. LLM constructs query string using raw query tool

---

### Option 3: Examples in Tool Description

Include representative examples directly in the tool description. LLMs often learn better from examples than formal grammar.

**Tool Definition:**

```json
{
  "name": "create_rule",
  "description": "Create a classification rule using Gmail-style query syntax.\n\nSYNTAX EXAMPLES:\n- domain:acme.com (events with acme.com attendees)\n- title:standup (title contains 'standup')\n- title:\"team meeting\" (multi-word, use quotes)\n- response:declined (user declined the event)\n- day-of-week:sat OR day-of-week:sun (weekends)\n- time-of-day:>17:00 (after 5 PM)\n- domain:client.com title:weekly (AND is implicit)\n- (domain:a.com OR domain:b.com) title:sync (grouping)\n- -title:canceled (negation with minus)\n\nPROPERTIES: title, description, attendees, domain, email, response, recurring, is-all-day, has-attendees, day-of-week, time-of-day, calendar, transparency, text\n\nUse preview_rule first to test your rule before saving.",
  "inputSchema": {
    "type": "object",
    "required": ["query", "project_id"],
    "properties": {
      "query": {
        "type": "string",
        "description": "Gmail-style query string"
      },
      "project_id": {
        "type": "string",
        "description": "Project ID to classify matching events to"
      }
    }
  }
}
```

---

## Recommended Hybrid Approach

Combine the approaches for best results:

### 1. Structured Tool for Common Cases

Handles ~80% of rules reliably without syntax knowledge:

- `create_simple_rule` - Single condition rules
- `create_domain_rule` - Rules based on attendee domains
- `create_time_rule` - Rules based on time/day

### 2. Raw Query Tool for Power Users

For complex rules with full syntax access:

- `create_advanced_rule` - Accepts raw query string
- Include good examples in description
- Validate and return helpful errors

### 3. Preview Tool for Validation

Critical for iterative rule development:

```json
{
  "name": "preview_rule",
  "description": "Test a rule query against recent calendar events to see what would match. Always use this before creating a rule to verify it works as expected.",
  "inputSchema": {
    "type": "object",
    "required": ["query"],
    "properties": {
      "query": {
        "type": "string",
        "description": "The rule query to test"
      },
      "limit": {
        "type": "integer",
        "default": 10,
        "description": "Maximum number of matching events to return"
      }
    }
  }
}
```

**Workflow with Preview:**

1. User: "Create a rule for my 1:1s with Sarah"
2. LLM creates query: `attendees:sarah title:1:1`
3. LLM calls `preview_rule` to test
4. LLM sees matching events, confirms with user
5. LLM calls `create_rule` to save

### 4. List Tools for Context

Help the LLM understand available targets:

```json
{
  "name": "list_projects",
  "description": "List all projects that rules can classify events to"
}
```

```json
{
  "name": "list_rules",
  "description": "List existing classification rules to understand current setup"
}
```

---

## Implementation Notes

### Server-Side Query Assembly

For structured tools, the server assembles queries:

```go
func buildQuery(conditions []Condition, logic string) string {
    parts := make([]string, len(conditions))
    for i, c := range conditions {
        part := fmt.Sprintf("%s:%s", c.Property, quoteIfNeeded(c.Value))
        if c.Negated {
            part = "-" + part
        }
        parts[i] = part
    }

    if logic == "OR" {
        return strings.Join(parts, " OR ")
    }
    return strings.Join(parts, " ")  // Implicit AND
}

func quoteIfNeeded(value string) string {
    if strings.Contains(value, " ") {
        return fmt.Sprintf(`"%s"`, value)
    }
    return value
}
```

### Error Messages

Return helpful errors that guide the LLM:

```json
{
  "error": {
    "code": "INVALID_PROPERTY",
    "message": "Unknown property 'subject'. Did you mean 'title'?",
    "valid_properties": ["title", "description", "attendees", ...]
  }
}
```

### Existing API Endpoints

The current API already supports rule preview:

- `POST /api/rules/preview` - Preview rule matches
- `POST /api/rules` - Create rule
- `GET /api/rules` - List rules
- `GET /api/projects` - List projects

These can be wrapped by MCP tools directly.

---

## Future Considerations

### Learning from Corrections

Track when users modify LLM-created rules to improve suggestions:

```json
{
  "name": "get_rule_suggestions",
  "description": "Get suggested rules based on unclassified events and existing patterns"
}
```

### Natural Language to Query

Consider a server-side NL-to-query endpoint that handles the translation, keeping the LLM's job simpler:

```json
{
  "name": "create_rule_from_description",
  "description": "Create a rule from a natural language description",
  "inputSchema": {
    "properties": {
      "description": {
        "type": "string",
        "description": "Natural language description like 'meetings with people from Acme on Fridays'"
      },
      "project_id": { "type": "string" }
    }
  }
}
```

The server would use its own LLM call or heuristics to generate the query, then return it for confirmation.
