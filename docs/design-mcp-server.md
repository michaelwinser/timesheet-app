# Design Document: Timesheet MCP Server

## 1. Overview

This document details the technical design for an MCP server that exposes timesheet functionality to AI assistants. The server supports two transport modes:

- **stdio**: For Claude Desktop (local, single-user)
- **SSE + OAuth**: For remote access from Claude.ai or other clients (hosted, multi-user)

## 2. Architecture

### 2.1 High-Level Architecture

```
                                    ┌─────────────────────────────────────┐
                                    │         Timesheet MCP Server        │
                                    │                                     │
┌─────────────────┐                 │  ┌─────────────┐  ┌─────────────┐  │
│ Claude Desktop  │───stdio─────────┼─►│  Transport  │  │    Auth     │  │
└─────────────────┘                 │  │   Layer     │  │   Layer     │  │
                                    │  │             │  │             │  │
┌─────────────────┐                 │  │ - stdio     │  │ - env var   │  │
│   Claude.ai     │───SSE/HTTPS────┼─►│ - SSE       │  │ - OAuth     │  │
└─────────────────┘                 │  └──────┬──────┘  └──────┬──────┘  │
                                    │         │                │         │
                                    │         ▼                ▼         │
                                    │  ┌─────────────────────────────┐   │
                                    │  │        Tool Layer           │   │
                                    │  │                             │   │
                                    │  │  - get_time_entries         │   │
                                    │  │  - search_events            │   │
                                    │  │  - bulk_classify            │   │
                                    │  │  - create_rule              │   │
                                    │  │  - ...                      │   │
                                    │  └──────────────┬──────────────┘   │
                                    │                 │                  │
                                    │                 ▼                  │
                                    │  ┌─────────────────────────────┐   │
                                    │  │       Data Layer            │   │
                                    │  │   (reuse existing db.py)    │   │
                                    │  └──────────────┬──────────────┘   │
                                    └─────────────────┼──────────────────┘
                                                      │
                                                      ▼
                                              ┌───────────────┐
                                              │  PostgreSQL   │
                                              └───────────────┘
```

### 2.2 Component Responsibilities

| Component | Responsibility |
|-----------|---------------|
| **Transport Layer** | Handle MCP protocol over stdio or SSE |
| **Auth Layer** | Validate user identity, enforce access control |
| **Tool Layer** | Implement MCP tools, business logic |
| **Data Layer** | Database queries, reuse existing `db.py` |

## 3. Authentication Design

### 3.1 stdio Transport (Claude Desktop)

```
┌─────────────────────────────────────────────────────────────┐
│                    Claude Desktop Config                     │
│  claude_desktop_config.json                                 │
│                                                             │
│  {                                                          │
│    "mcpServers": {                                          │
│      "timesheet": {                                         │
│        "command": "python",                                 │
│        "args": ["-m", "mcp_server"],                        │
│        "cwd": "/path/to/timesheet-app",                     │
│        "env": {                                             │
│          "DATABASE_URL": "postgresql://...",                │
│          "TIMESHEET_USER_EMAIL": "user@example.com"         │
│        }                                                    │
│      }                                                      │
│    }                                                        │
│  }                                                          │
└─────────────────────────────────────────────────────────────┘
```

**Flow:**
1. Claude Desktop spawns MCP server as subprocess
2. Server reads `TIMESHEET_USER_EMAIL` from environment
3. Server looks up user ID from database
4. All queries are filtered by that user ID
5. No per-request authentication needed (trust the environment)

**Security Model:**
- Relies on local machine security
- Only the user who configured the server can access it
- Database credentials are in the config file (same as any local app)

### 3.2 SSE Transport (Remote/OAuth)

```
┌──────────┐                      ┌──────────────┐                    ┌─────────────┐
│          │  1. Connect to MCP   │              │                    │             │
│ Claude   │─────────────────────►│   Timesheet  │                    │  Timesheet  │
│   .ai    │                      │  MCP Server  │                    │   Web App   │
│          │◄─────────────────────│   (hosted)   │                    │   (OAuth)   │
│          │  2. Auth required    │              │                    │             │
│          │                      │              │                    │             │
│          │  3. OAuth redirect   │              │                    │             │
│          │─────────────────────────────────────────────────────────►│             │
│          │                      │              │                    │             │
│          │  4. User logs in     │              │                    │             │
│          │◄─────────────────────────────────────────────────────────│             │
│          │     (auth code)      │              │                    │             │
│          │                      │              │                    │             │
│          │  5. Exchange code    │              │                    │             │
│          │─────────────────────►│              │───────────────────►│             │
│          │                      │              │◄───────────────────│             │
│          │  6. Access token     │              │   (validate)       │             │
│          │◄─────────────────────│              │                    │             │
│          │                      │              │                    │             │
│          │  7. MCP requests     │              │                    │             │
│          │─────────────────────►│  (with token)│                    │             │
│          │                      │              │                    │             │
└──────────┘                      └──────────────┘                    └─────────────┘
```

**OAuth Implementation:**

The MCP server will implement OAuth 2.0 Authorization Code flow:

1. **Authorization Endpoint**: `/oauth/authorize`
   - Redirects to timesheet web app login
   - After login, redirects back with auth code

2. **Token Endpoint**: `/oauth/token`
   - Exchanges auth code for access token
   - Returns JWT containing user_id and email

3. **Token Validation**:
   - Every MCP request includes `Authorization: Bearer <token>`
   - Server validates JWT signature and expiry
   - Extracts user_id for query filtering

**JWT Token Structure:**
```json
{
  "sub": "123",
  "email": "user@example.com",
  "iat": 1702900000,
  "exp": 1702986400
}
```

### 3.3 Auth Layer Implementation

```python
# mcp_server/auth.py

from dataclasses import dataclass
from abc import ABC, abstractmethod

@dataclass
class AuthenticatedUser:
    user_id: int
    email: str

class AuthProvider(ABC):
    @abstractmethod
    def get_current_user(self) -> AuthenticatedUser:
        """Get the authenticated user for the current request."""
        pass

class EnvAuthProvider(AuthProvider):
    """Auth via environment variable (stdio transport)."""

    def __init__(self, db):
        self.db = db
        email = os.environ.get("TIMESHEET_USER_EMAIL")
        if not email:
            raise ValueError("TIMESHEET_USER_EMAIL not set")

        user = db.execute_one(
            "SELECT id, email FROM users WHERE email = %s", (email,)
        )
        if not user:
            raise ValueError(f"User not found: {email}")

        self._user = AuthenticatedUser(user_id=user["id"], email=user["email"])

    def get_current_user(self) -> AuthenticatedUser:
        return self._user

class OAuthProvider(AuthProvider):
    """Auth via OAuth token (SSE transport)."""

    def __init__(self, jwt_secret: str):
        self.jwt_secret = jwt_secret
        self._current_user: AuthenticatedUser | None = None

    def validate_token(self, token: str) -> AuthenticatedUser:
        """Validate JWT and return user."""
        payload = jwt.decode(token, self.jwt_secret, algorithms=["HS256"])
        return AuthenticatedUser(
            user_id=int(payload["sub"]),
            email=payload["email"]
        )

    def set_current_user(self, user: AuthenticatedUser):
        self._current_user = user

    def get_current_user(self) -> AuthenticatedUser:
        if not self._current_user:
            raise ValueError("No authenticated user")
        return self._current_user
```

## 4. Tool Implementations

### 4.1 Tool Structure

Each tool follows this pattern:

```python
# mcp_server/tools/base.py

from dataclasses import dataclass
from typing import Any

@dataclass
class ToolResult:
    success: bool
    data: Any = None
    error: str | None = None

class BaseTool:
    def __init__(self, db, auth: AuthProvider):
        self.db = db
        self.auth = auth

    @property
    def user_id(self) -> int:
        return self.auth.get_current_user().user_id
```

### 4.2 Core Tools

#### get_time_entries

```python
# mcp_server/tools/time_entries.py

class GetTimeEntriesTool(BaseTool):
    name = "get_time_entries"
    description = "Get time entries with full event details for a date range"

    parameters = {
        "type": "object",
        "properties": {
            "start_date": {
                "type": "string",
                "description": "Start date (YYYY-MM-DD)"
            },
            "end_date": {
                "type": "string",
                "description": "End date (YYYY-MM-DD)"
            },
            "project_id": {
                "type": "integer",
                "description": "Filter by project (optional)"
            },
            "include_unclassified": {
                "type": "boolean",
                "description": "Include unclassified events (default: true)"
            }
        },
        "required": ["start_date", "end_date"]
    }

    def execute(self, start_date: str, end_date: str,
                project_id: int | None = None,
                include_unclassified: bool = True) -> ToolResult:

        # Query classified entries with event details
        query = """
            SELECT
                e.id as event_id,
                e.title,
                e.description,
                e.start_time,
                e.end_time,
                e.attendees,
                e.meeting_link,
                e.my_response_status,
                e.did_not_attend,
                te.id as entry_id,
                te.hours,
                te.description as entry_description,
                te.project_id,
                te.classification_source,
                te.rule_id,
                p.name as project_name,
                p.color as project_color
            FROM events e
            LEFT JOIN time_entries te ON te.event_id = e.id
            LEFT JOIN projects p ON te.project_id = p.id
            WHERE e.user_id = %s
              AND e.start_time >= %s
              AND e.start_time < %s
        """
        params = [self.user_id, f"{start_date}T00:00:00", f"{end_date}T23:59:59"]

        if project_id:
            query += " AND te.project_id = %s"
            params.append(project_id)

        if not include_unclassified:
            query += " AND te.id IS NOT NULL"

        query += " ORDER BY e.start_time"

        rows = self.db.execute_all(query, tuple(params))

        # Format results
        entries = []
        for row in rows:
            entry = {
                "event": {
                    "id": row["event_id"],
                    "title": row["title"],
                    "description": row["description"],
                    "start_time": row["start_time"].isoformat() if row["start_time"] else None,
                    "end_time": row["end_time"].isoformat() if row["end_time"] else None,
                    "attendees": json.loads(row["attendees"]) if row["attendees"] else [],
                    "meeting_link": row["meeting_link"],
                    "my_response_status": row["my_response_status"],
                    "did_not_attend": row["did_not_attend"]
                },
                "classification": None
            }

            if row["entry_id"]:
                entry["classification"] = {
                    "entry_id": row["entry_id"],
                    "hours": float(row["hours"]),
                    "description": row["entry_description"],
                    "project_id": row["project_id"],
                    "project_name": row["project_name"],
                    "project_color": row["project_color"],
                    "source": row["classification_source"],
                    "rule_id": row["rule_id"]
                }

            entries.append(entry)

        return ToolResult(success=True, data=entries)
```

#### search_events

```python
class SearchEventsTool(BaseTool):
    name = "search_events"
    description = "Search events by text across title, description, and attendees"

    parameters = {
        "type": "object",
        "properties": {
            "query": {
                "type": "string",
                "description": "Search text"
            },
            "start_date": {
                "type": "string",
                "description": "Start date filter (optional)"
            },
            "end_date": {
                "type": "string",
                "description": "End date filter (optional)"
            },
            "classified": {
                "type": "boolean",
                "description": "Filter by classification status (optional)"
            }
        },
        "required": ["query"]
    }

    def execute(self, query: str, start_date: str | None = None,
                end_date: str | None = None,
                classified: bool | None = None) -> ToolResult:

        search_pattern = f"%{query.lower()}%"

        sql = """
            SELECT
                e.id as event_id,
                e.title,
                e.description,
                e.start_time,
                e.end_time,
                e.attendees,
                te.id as entry_id,
                te.project_id,
                p.name as project_name
            FROM events e
            LEFT JOIN time_entries te ON te.event_id = e.id
            LEFT JOIN projects p ON te.project_id = p.id
            WHERE e.user_id = %s
              AND (
                  LOWER(e.title) LIKE %s
                  OR LOWER(e.description) LIKE %s
                  OR LOWER(e.attendees) LIKE %s
              )
        """
        params = [self.user_id, search_pattern, search_pattern, search_pattern]

        if start_date:
            sql += " AND e.start_time >= %s"
            params.append(f"{start_date}T00:00:00")

        if end_date:
            sql += " AND e.start_time < %s"
            params.append(f"{end_date}T23:59:59")

        if classified is not None:
            if classified:
                sql += " AND te.id IS NOT NULL"
            else:
                sql += " AND te.id IS NULL"

        sql += " ORDER BY e.start_time DESC LIMIT 100"

        rows = self.db.execute_all(sql, tuple(params))

        results = [{
            "event_id": row["event_id"],
            "title": row["title"],
            "description": row["description"],
            "start_time": row["start_time"].isoformat() if row["start_time"] else None,
            "end_time": row["end_time"].isoformat() if row["end_time"] else None,
            "attendees": json.loads(row["attendees"]) if row["attendees"] else [],
            "is_classified": row["entry_id"] is not None,
            "project_name": row["project_name"]
        } for row in rows]

        return ToolResult(success=True, data=results)
```

#### bulk_classify

```python
class BulkClassifyTool(BaseTool):
    name = "bulk_classify"
    description = "Classify multiple events to a project at once"

    parameters = {
        "type": "object",
        "properties": {
            "event_ids": {
                "type": "array",
                "items": {"type": "integer"},
                "description": "Event IDs to classify"
            },
            "project_id": {
                "type": "integer",
                "description": "Project to assign"
            }
        },
        "required": ["event_ids", "project_id"]
    }

    def execute(self, event_ids: list[int], project_id: int) -> ToolResult:
        # Verify project belongs to user
        project = self.db.execute_one(
            "SELECT id FROM projects WHERE id = %s AND user_id = %s",
            (project_id, self.user_id)
        )
        if not project:
            return ToolResult(success=False, error="Project not found")

        classified = 0
        skipped = 0
        errors = []

        for event_id in event_ids:
            # Get event
            event = self.db.execute_one("""
                SELECT id, title, start_time, end_time, did_not_attend
                FROM events
                WHERE id = %s AND user_id = %s
            """, (event_id, self.user_id))

            if not event:
                errors.append(f"Event {event_id} not found")
                continue

            if event["did_not_attend"]:
                skipped += 1
                continue

            # Check if already classified
            existing = self.db.execute_one(
                "SELECT id FROM time_entries WHERE event_id = %s",
                (event_id,)
            )
            if existing:
                skipped += 1
                continue

            # Calculate hours
            start = event["start_time"]
            end = event["end_time"]
            hours = (end - start).total_seconds() / 3600

            # Create entry
            self.db.execute_insert("""
                INSERT INTO time_entries
                (user_id, event_id, project_id, hours, description, classification_source)
                VALUES (%s, %s, %s, %s, %s, 'manual')
                RETURNING id
            """, (self.user_id, event_id, project_id, hours, event["title"]))

            classified += 1

        return ToolResult(success=True, data={
            "classified": classified,
            "skipped": skipped,
            "errors": errors
        })
```

#### create_rule

```python
class CreateRuleTool(BaseTool):
    name = "create_rule"
    description = "Create a classification rule with conditions"

    parameters = {
        "type": "object",
        "properties": {
            "name": {
                "type": "string",
                "description": "Rule name"
            },
            "project_id": {
                "type": "integer",
                "description": "Target project ID"
            },
            "conditions": {
                "type": "array",
                "items": {
                    "type": "object",
                    "properties": {
                        "property_name": {"type": "string"},
                        "condition_type": {"type": "string"},
                        "condition_value": {}
                    }
                },
                "description": "Array of conditions (AND logic)"
            },
            "priority": {
                "type": "integer",
                "description": "Priority (higher = first, default 50)"
            }
        },
        "required": ["name", "project_id", "conditions"]
    }

    def execute(self, name: str, project_id: int,
                conditions: list[dict], priority: int = 50) -> ToolResult:

        # Verify project
        project = self.db.execute_one(
            "SELECT id, name FROM projects WHERE id = %s AND user_id = %s",
            (project_id, self.user_id)
        )
        if not project:
            return ToolResult(success=False, error="Project not found")

        # Create rule
        rule_id = self.db.execute_insert("""
            INSERT INTO rules (user_id, name, target_type, project_id, priority, is_enabled)
            VALUES (%s, %s, 'project', %s, %s, true)
            RETURNING id
        """, (self.user_id, name, project_id, priority))

        # Create conditions
        for cond in conditions:
            value = cond["condition_value"]
            if isinstance(value, list):
                value = json.dumps(value)

            self.db.execute_insert("""
                INSERT INTO rule_conditions
                (rule_id, property_name, condition_type, condition_value)
                VALUES (%s, %s, %s, %s)
                RETURNING id
            """, (rule_id, cond["property_name"], cond["condition_type"], value))

        return ToolResult(success=True, data={
            "rule_id": rule_id,
            "name": name,
            "project": project["name"],
            "conditions": len(conditions)
        })
```

### 4.3 Complete Tool List

| Tool | Phase | Priority |
|------|-------|----------|
| `list_projects` | 1 | High |
| `get_time_entries` | 1 | High |
| `get_timesheet_summary` | 1 | High |
| `search_events` | 2 | High |
| `bulk_classify` | 2 | High |
| `list_rules` | 3 | Medium |
| `create_rule` | 3 | Medium |
| `apply_rules` | 3 | Medium |
| `classify_event` | 4 | Medium |
| `update_entry` | 4 | Low |
| `unclassify_entry` | 4 | Low |
| `set_did_not_attend` | 4 | Low |
| `sync_calendar` | 5 | Low |

## 5. Server Implementation

### 5.1 Entry Point

```python
# mcp_server/__main__.py

import asyncio
import os
import sys

from mcp.server import Server
from mcp.server.stdio import stdio_server
from mcp.server.sse import sse_server

from .auth import EnvAuthProvider, OAuthProvider
from .tools import register_all_tools
from db import get_db

def create_server(transport: str = "stdio") -> Server:
    """Create and configure MCP server."""

    server = Server("timesheet")
    db = get_db()

    # Set up auth based on transport
    if transport == "stdio":
        auth = EnvAuthProvider(db)
    else:
        jwt_secret = os.environ["JWT_SECRET"]
        auth = OAuthProvider(jwt_secret)

    # Register all tools
    register_all_tools(server, db, auth)

    return server

async def main():
    transport = os.environ.get("MCP_TRANSPORT", "stdio")
    server = create_server(transport)

    if transport == "stdio":
        async with stdio_server() as (read_stream, write_stream):
            await server.run(read_stream, write_stream)
    else:
        # SSE transport with OAuth
        port = int(os.environ.get("MCP_PORT", 8001))
        await sse_server(server, port=port)

if __name__ == "__main__":
    asyncio.run(main())
```

### 5.2 Tool Registration

```python
# mcp_server/tools/__init__.py

from mcp.server import Server
from mcp.types import Tool

from .time_entries import GetTimeEntriesTool, GetTimesheetSummaryTool
from .search import SearchEventsTool
from .classify import BulkClassifyTool, ClassifyEventTool, UnclassifyEntryTool
from .rules import ListRulesTool, CreateRuleTool, ApplyRulesTool
from .projects import ListProjectsTool

ALL_TOOLS = [
    ListProjectsTool,
    GetTimeEntriesTool,
    GetTimesheetSummaryTool,
    SearchEventsTool,
    BulkClassifyTool,
    ClassifyEventTool,
    UnclassifyEntryTool,
    ListRulesTool,
    CreateRuleTool,
    ApplyRulesTool,
]

def register_all_tools(server: Server, db, auth):
    """Register all tools with the MCP server."""

    tool_instances = {tool.name: tool(db, auth) for tool in ALL_TOOLS}

    @server.list_tools()
    async def list_tools():
        return [
            Tool(
                name=tool.name,
                description=tool.description,
                inputSchema=tool.parameters
            )
            for tool in tool_instances.values()
        ]

    @server.call_tool()
    async def call_tool(name: str, arguments: dict):
        tool = tool_instances.get(name)
        if not tool:
            return {"error": f"Unknown tool: {name}"}

        result = tool.execute(**arguments)

        if result.success:
            return result.data
        else:
            return {"error": result.error}
```

### 5.3 SSE Server with OAuth

```python
# mcp_server/sse.py

from aiohttp import web
import jwt

async def handle_oauth_authorize(request):
    """Redirect to timesheet web app for login."""
    redirect_uri = request.query.get("redirect_uri")
    state = request.query.get("state")

    # Redirect to web app login
    login_url = f"{WEBAPP_URL}/auth/login?redirect_uri={redirect_uri}&state={state}"
    raise web.HTTPFound(login_url)

async def handle_oauth_token(request):
    """Exchange auth code for access token."""
    data = await request.json()
    code = data.get("code")

    # Validate code with web app
    # ... (implementation depends on how web app generates codes)

    # Generate JWT
    token = jwt.encode({
        "sub": str(user_id),
        "email": user_email,
        "exp": datetime.utcnow() + timedelta(days=1)
    }, JWT_SECRET, algorithm="HS256")

    return web.json_response({"access_token": token})

async def handle_mcp_sse(request):
    """Handle MCP over SSE with OAuth."""
    auth_header = request.headers.get("Authorization")
    if not auth_header or not auth_header.startswith("Bearer "):
        raise web.HTTPUnauthorized()

    token = auth_header[7:]
    try:
        user = auth.validate_token(token)
        auth.set_current_user(user)
    except jwt.InvalidTokenError:
        raise web.HTTPUnauthorized()

    # Handle SSE connection
    # ... (standard MCP SSE handling)
```

## 6. File Structure

```
timesheet-app/
├── src/
│   ├── mcp_server/
│   │   ├── __init__.py
│   │   ├── __main__.py          # Entry point
│   │   ├── server.py            # Server setup
│   │   ├── auth.py              # Auth providers
│   │   ├── sse.py               # SSE transport + OAuth
│   │   ├── tools/
│   │   │   ├── __init__.py      # Tool registration
│   │   │   ├── base.py          # Base tool class
│   │   │   ├── time_entries.py  # get_time_entries, get_timesheet_summary
│   │   │   ├── search.py        # search_events
│   │   │   ├── classify.py      # bulk_classify, classify_event, etc.
│   │   │   ├── rules.py         # list_rules, create_rule, apply_rules
│   │   │   └── projects.py      # list_projects
│   │   └── resources.py         # MCP resources
│   ├── db.py                    # Existing - reuse
│   ├── services/
│   │   └── classifier.py        # Existing - reuse for rules
│   └── ...
├── docs/
│   ├── prd-mcp-server.md
│   └── design-mcp-server.md     # This document
└── ...
```

## 7. Detailed Implementation Plan

### Phase 1: Foundation + Reporting (Week 1)

**Goal:** Basic MCP server that can answer "What did I work on last week?"

| Task | Est. Hours | Dependencies |
|------|------------|--------------|
| 1.1 Set up mcp_server package structure | 1 | - |
| 1.2 Implement EnvAuthProvider | 1 | 1.1 |
| 1.3 Implement base tool class | 1 | 1.1 |
| 1.4 Implement `list_projects` tool | 1 | 1.3 |
| 1.5 Implement `get_time_entries` tool | 3 | 1.3 |
| 1.6 Implement `get_timesheet_summary` tool | 2 | 1.3 |
| 1.7 Set up stdio server entry point | 2 | 1.2-1.6 |
| 1.8 Test with Claude Desktop | 2 | 1.7 |
| 1.9 Write Claude Desktop config docs | 1 | 1.8 |

**Deliverable:** Working MCP server for reporting use case.

**Test Prompt:** "Produce a timesheet report for last week"

### Phase 2: Search + Bulk Operations (Week 2)

**Goal:** Find events and classify them in bulk.

| Task | Est. Hours | Dependencies |
|------|------------|--------------|
| 2.1 Implement `search_events` tool | 2 | Phase 1 |
| 2.2 Implement `bulk_classify` tool | 3 | Phase 1 |
| 2.3 Add search result pagination | 1 | 2.1 |
| 2.4 Test search + classify flow | 2 | 2.1-2.2 |

**Deliverable:** Ability to search and bulk-classify events.

**Test Prompt:** "Find all events with 'Scovetta' and assign them to Alpha-Omega"

### Phase 3: Rule Management (Week 3)

**Goal:** Create rules from natural language and apply them.

| Task | Est. Hours | Dependencies |
|------|------------|--------------|
| 3.1 Implement `list_rules` tool | 1 | Phase 1 |
| 3.2 Implement `create_rule` tool | 3 | Phase 1 |
| 3.3 Implement `apply_rules` tool with dry_run | 3 | 3.2 |
| 3.4 Test rule creation from NL | 2 | 3.1-3.3 |
| 3.5 Test rule inference workflow | 2 | 3.1-3.3 |

**Deliverable:** Full rule management via MCP.

**Test Prompts:**
- "Create a rule for VEX in title OR Munawar as attendee → Alpha-Omega"
- "Look at last week's entries and propose rules"

### Phase 4: Single Entry Operations (Week 4)

**Goal:** Individual entry management for edge cases.

| Task | Est. Hours | Dependencies |
|------|------------|--------------|
| 4.1 Implement `classify_event` tool | 1 | Phase 1 |
| 4.2 Implement `update_entry` tool | 1 | Phase 1 |
| 4.3 Implement `unclassify_entry` tool | 1 | Phase 1 |
| 4.4 Implement `set_did_not_attend` tool | 1 | Phase 1 |
| 4.5 Integration testing | 2 | 4.1-4.4 |

**Deliverable:** Complete CRUD for time entries.

### Phase 5: Resources & Prompts (Week 5)

**Goal:** Improve UX with pre-loaded context and workflow prompts.

| Task | Est. Hours | Dependencies |
|------|------------|--------------|
| 5.1 Implement `timesheet://projects` resource | 1 | Phase 1 |
| 5.2 Implement `timesheet://rules` resource | 1 | Phase 3 |
| 5.3 Implement MCP prompts | 2 | Phase 1-3 |
| 5.4 Test prompt workflows | 2 | 5.3 |

**Deliverable:** Polished UX with prompts.

### Phase 6: SSE + OAuth (Week 6-7)

**Goal:** Remote access without local installation.

| Task | Est. Hours | Dependencies |
|------|------------|--------------|
| 6.1 Implement OAuthProvider | 3 | Phase 1 |
| 6.2 Add OAuth endpoints to web app | 4 | 6.1 |
| 6.3 Implement SSE transport | 4 | 6.1 |
| 6.4 JWT token generation/validation | 2 | 6.1-6.2 |
| 6.5 Integration testing | 4 | 6.1-6.4 |
| 6.6 Deploy hosted MCP server | 2 | 6.5 |
| 6.7 Documentation for remote setup | 2 | 6.6 |

**Deliverable:** Hosted MCP server with OAuth.

## 8. Testing Strategy

### 8.1 Unit Tests

Each tool should have unit tests:

```python
# tests/mcp_server/test_time_entries.py

def test_get_time_entries_filters_by_user():
    """Ensure entries are filtered by authenticated user."""
    ...

def test_get_time_entries_date_range():
    """Test date filtering works correctly."""
    ...

def test_get_time_entries_includes_unclassified():
    """Test include_unclassified parameter."""
    ...
```

### 8.2 Integration Tests

Test full tool execution with database:

```python
def test_search_and_bulk_classify_flow():
    """Test the search → bulk classify workflow."""
    # Create test events
    # Search for them
    # Bulk classify
    # Verify classifications
    ...
```

### 8.3 End-to-End Tests

Test with actual Claude Desktop:

1. Configure MCP server in Claude Desktop
2. Run test prompts manually
3. Verify correct tool calls and results

## 9. Security Considerations

1. **User Isolation**: All queries MUST filter by user_id
2. **Input Validation**: Validate all tool parameters
3. **SQL Injection**: Use parameterized queries (already done via db.py)
4. **Token Expiry**: OAuth tokens expire after 24 hours
5. **HTTPS Only**: SSE transport must use HTTPS in production
6. **Rate Limiting**: Consider rate limits for SSE transport

## 10. Open Questions (Resolved)

| Question | Resolution |
|----------|------------|
| Multi-user support? | One user per connection (env var or OAuth token) |
| OAuth for sync? | Phase 6 - reuse web app OAuth flow |
| Rate limiting? | Defer until SSE transport is needed |

## 11. References

- [MCP Python SDK](https://github.com/modelcontextprotocol/python-sdk)
- [MCP Specification](https://modelcontextprotocol.io/)
- [OAuth 2.0 RFC](https://datatracker.ietf.org/doc/html/rfc6749)
