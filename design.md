# Design Doc: Timesheet App

## 1. Overview

A web application that connects to Google Calendar, displays events in a week view, allows users to classify them by project, and exports time entries as Harvest-compatible CSV. The core interaction is a "flip card" UI where calendar events transform into time entries upon classification.

**PRD**: `~/claude/projects/timesheet-app/prd.md`

**Key constraints from PRD:**
- Docker deployment on TrueNAS
- Single-user initially
- SQLite for persistence
- Google Calendar via OAuth 2.0
- Harvest CSV export (API integration later)

## 2. Technology Choices

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Language | Python 3.11+ | Fast prototyping, excellent Google API libraries. Accept potential rewrite later with lessons learned. |
| Web Framework | FastAPI | Lightweight, built-in OpenAPI docs provide API explorer for free, type hints improve readability. |
| Database | SQLite via stdlib `sqlite3` | Simple, portable, no separate server. Single-file database fits single-user constraint. |
| Frontend | Vanilla JS | No framework. Client-side API library for backend communication. |
| Google Calendar | google-api-python-client | Accept the dependency cost for now; OAuth is security-sensitive and not worth implementing ourselves. |
| Templates | Jinja2 (FastAPI default) | Server-rendered HTML for main UI, minimal JS for interactions. |

## 3. Dependencies

| Package | Purpose | Justification | Transitive Deps |
|---------|---------|---------------|-----------------|
| fastapi | Web framework | Core framework, lightweight, gives us OpenAPI docs | Few (starlette, pydantic) |
| uvicorn | ASGI server | Required to run FastAPI | Minimal |
| jinja2 | Templating | Server-rendered HTML | None |
| google-api-python-client | Calendar API | OAuth + Calendar access; security-sensitive, don't DIY | Heavy (~10 deps) |
| google-auth-oauthlib | OAuth flow | Part of Google auth ecosystem | Medium |
| python-multipart | Form parsing | Required for FastAPI form handling | None |

**Accepted but flagged for future removal:**
- Google API client brings significant dependencies. Future version may use direct REST calls with stdlib.

**Not using:**
- SQLAlchemy (too heavy, we'll write a thin wrapper)
- Any CSS framework (vanilla CSS)
- Any JS framework (vanilla JS)

## 4. Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Docker Container                         │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                      FastAPI App                          │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────────┐   │  │
│  │  │   Routes    │  │  Services   │  │   Data Layer    │   │  │
│  │  │             │  │             │  │                 │   │  │
│  │  │ /api/*      │──│ calendar    │──│ db.py (sqlite3) │   │  │
│  │  │ /auth/*     │  │ classifier  │  │                 │   │  │
│  │  │ /ui/*       │  │ exporter    │  └────────┬────────┘   │  │
│  │  │ /docs       │  │ projects    │           │            │  │
│  │  └─────────────┘  └─────────────┘           │            │  │
│  └─────────────────────────────────────────────│────────────┘  │
│                                                │                │
│  ┌─────────────────────────────────────────────▼────────────┐  │
│  │                    timesheet.db (SQLite)                  │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
          │
          │ OAuth 2.0 + REST
          ▼
┌─────────────────────┐
│  Google Calendar    │
│       API           │
└─────────────────────┘
```

**Components:**

| Component | Responsibility |
|-----------|----------------|
| Routes | HTTP endpoints, request/response handling, template rendering |
| Services | Business logic: calendar sync, classification, export |
| Data Layer | SQLite wrapper, migrations, queries |
| Static Assets | CSS, vanilla JS, client-side API library |

**Data flow:**
1. User authenticates → OAuth tokens stored in DB
2. User requests sync → Service fetches from Google, stores events in DB
3. User views week → Events loaded from DB, rendered as HTML
4. User classifies → JS calls API, DB updated, card flips via JS
5. User exports → Service queries time entries, generates CSV

## 5. Data Model

```sql
-- OAuth tokens for Google Calendar access
CREATE TABLE auth_tokens (
    id INTEGER PRIMARY KEY,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    token_expiry TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- User-defined projects for classification
CREATE TABLE projects (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    client TEXT,  -- Optional, for Harvest export
    color TEXT DEFAULT '#00aa44',  -- Color for time entry card background
    is_visible INTEGER DEFAULT 1,  -- Show/hide toggle
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Calendar events fetched from Google
CREATE TABLE events (
    id INTEGER PRIMARY KEY,
    google_event_id TEXT NOT NULL UNIQUE,
    calendar_id TEXT NOT NULL,
    title TEXT,
    description TEXT,
    start_time TIMESTAMP NOT NULL,
    end_time TIMESTAMP NOT NULL,
    attendees TEXT,  -- JSON array
    meeting_link TEXT,
    event_color TEXT,
    recurrence_id TEXT,  -- Links recurring instances
    is_recurring INTEGER DEFAULT 0,
    raw_json TEXT,  -- Full event data for debugging
    fetched_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Classified time entries (the "flipped" side of events)
CREATE TABLE time_entries (
    id INTEGER PRIMARY KEY,
    event_id INTEGER NOT NULL REFERENCES events(id),
    project_id INTEGER NOT NULL REFERENCES projects(id),
    hours REAL NOT NULL,  -- Can differ from event duration
    description TEXT,
    classified_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    classification_source TEXT,  -- 'manual', 'rule', 'auto'
    UNIQUE(event_id)  -- One time entry per event
);

-- Classification rules for auto/suggested classification
CREATE TABLE classification_rules (
    id INTEGER PRIMARY KEY,
    project_id INTEGER NOT NULL REFERENCES projects(id),
    rule_type TEXT NOT NULL,  -- 'title_contains', 'attendee', 'recurrence', 'color'
    rule_value TEXT NOT NULL,
    priority INTEGER DEFAULT 0,  -- Higher = checked first
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Track which rules matched which events (for learning/debugging)
CREATE TABLE classification_history (
    id INTEGER PRIMARY KEY,
    event_id INTEGER NOT NULL REFERENCES events(id),
    rule_id INTEGER REFERENCES classification_rules(id),
    project_id INTEGER NOT NULL REFERENCES projects(id),
    confidence REAL,  -- 0.0 to 1.0 for future ML
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

**Entity lifecycle:**

- **Events**: Created on sync, updated on re-sync, never deleted (historical record)
- **Time Entries**: Created when user classifies, can be updated (project, hours, description)
- **Projects**: CRUD by user, soft-delete via is_visible
- **Rules**: Created manually or inferred from repeated classifications

## 6. API Design

### Authentication

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/auth/login` | GET | Redirect to Google OAuth |
| `/auth/callback` | GET | OAuth callback, store tokens |
| `/auth/logout` | POST | Clear tokens |
| `/auth/status` | GET | Return auth state |

### Calendar

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/sync` | POST | Fetch events from Google for date range |
| `/api/events` | GET | List events for date range |
| `/api/events/{id}` | GET | Single event with time entry if exists |

### Time Entries

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/entries` | GET | List time entries for date range |
| `/api/entries` | POST | Create/update time entry (classify event) |
| `/api/entries/{id}` | PUT | Update time entry |
| `/api/entries/{id}` | DELETE | Remove classification (unclassify) |
| `/api/entries/bulk` | POST | Classify multiple events |

### Projects

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/projects` | GET | List all projects |
| `/api/projects` | POST | Create project |
| `/api/projects/{id}` | PUT | Update project |
| `/api/projects/{id}` | DELETE | Delete project |
| `/api/projects/{id}/visibility` | PUT | Toggle show/hide |
| `/api/projects/import` | POST | Import from CSV |
| `/api/projects/export` | GET | Export to CSV |

### Export

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/export/harvest` | GET | Generate Harvest-compatible CSV |

### UI Routes

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Redirect to current week |
| `/week/{date}` | GET | Week view centered on date |
| `/projects` | GET | Project management page |

**Navigation:** Header includes "Calendar" link (goes to current week), "Projects" link, and "API" link (opens /docs).

### Built-in API Explorer

- `/docs` - Swagger UI (auto-generated by FastAPI)
- `/redoc` - ReDoc documentation

**Request/Response format:** JSON for all `/api/*` endpoints.

**Authentication:** All `/api/*` endpoints require valid OAuth tokens (checked via middleware). Return 401 if not authenticated.

## 7. User Interface

**Theming:** Dark mode support via CSS custom properties and `prefers-color-scheme` media query. Follows system preference automatically.

### Screen Inventory

| Screen | Purpose |
|--------|---------|
| Week View | Main screen: calendar grid, event cards, classification |
| Project List | CRUD projects, toggle visibility |
| Settings | Calendar selection, preferences |
| Auth | Login/logout flow |

### Week View Layout

```
┌─────────────────────────────────────────────────────────────────┐
│  [◀ Prev] [Today] [Next ▶]  Week of Dec 2-8, 2024  [↻ Refresh] [Export]│
├─────────────────────────────────────────────────────────────────┤
│  [Filter: _______________]                                       │
├─────────────────────────────────────────────────────────────────┤
│   Mon 2    │   Tue 3    │   Wed 4    │   Thu 5    │   Fri 6    │
├────────────┼────────────┼────────────┼────────────┼────────────┤
│ ┌────────┐ │            │ ┌────────┐ │ ┌────────┐ │            │
│ │Meeting │ │            │ │  Call  │ │ │Standup │ │            │
│ │w/ Jane │ │            │ │  Acme  │ │ │        │ │            │
│ │9:00-10 │ │            │ │2:00-3  │ │ │9:00-9:3│ │            │
│ │        │ │            │ │        │ │ │ ✓ Proj │ │            │
│ └────────┘ │            │ └────────┘ │ └────────┘ │            │
│            │            │            │            │            │
│ ┌────────┐ │ ┌────────┐ │            │            │            │
│ │Dev Work│ │ │Workshop│ │            │            │            │
│ │1:00-5  │ │ │All Day │ │            │            │            │
│ │ ✓ Beta │ │ │        │ │            │            │            │
│ └────────┘ │ └────────┘ │            │            │            │
└────────────┴────────────┴────────────┴────────────┴────────────┘
```

### Event Card States

**Unclassified (event side):**
```
┌──────────────────────┐
│ 📅 Meeting with Jane │
│ 9:00 AM - 10:00 AM   │
│ jane@example.com     │
│ 🔗 Meet link         │
│                      │
│ [Classify ▼] [Open↗] │
└──────────────────────┘
```

**Classified (time entry side):**
Visually distinct with project-colored background and white text.
```
┌──────────────────────┐  ← Background color from project.color
│ [Project Alpha ▼]    │  ← Dropdown to reclassify (empty = unclassify)
│ 1.0 hrs [+15m]       │
│ "Meeting with Jane"  │
│                      │
│ [Flip]               │
└──────────────────────┘
```

### Key Interactions

| Interaction | Behavior |
|-------------|----------|
| Page load | Auto-syncs calendar events quietly; reloads page if new events found |
| Click "Today" | Navigate to current week |
| Click "Prev"/"Next" | Navigate weeks; auto-syncs on arrival |
| Click "Refresh" | Force re-sync from Google Calendar, reload page |
| Click "Classify" dropdown | Show project list, select to classify |
| Click "+15m" | Round hours up to next 15 min (or add 15m if already rounded) |
| Click "Flip" | Toggle between event/time entry view |
| Change project dropdown (entry side) | Reclassify to different project; updates background color |
| Select empty option (entry side) | Unclassify (return to event state) |
| Click "Open↗" | Open event in Google Calendar |
| Type in filter | Show only matching events/entries |
| Toggle project visibility | Hide/show entries for that project |

### Project List UI

| Column | Content |
|--------|---------|
| Color | Color picker (updates immediately on change) |
| Name | Display value; becomes text input in edit mode |
| Client | Display value; becomes text input in edit mode |
| Visible | Checkbox toggle (updates immediately) |
| Actions | Edit/Delete buttons; becomes Save/Cancel in edit mode |

**Inline Editing:** Click "Edit" to enter edit mode for a row. Name and Client become editable text inputs. Save commits changes; Cancel reverts to original values.

### Client-Side API Library

```javascript
// api.js - thin wrapper around fetch
const api = {
  async get(path) { ... },
  async post(path, data) { ... },
  async put(path, data) { ... },
  async delete(path) { ... },

  // Domain methods
  async syncEvents(startDate, endDate) { ... },
  async getEvents(startDate, endDate) { ... },
  async classifyEvent(eventId, projectId, hours, description) { ... },
  async unclassify(entryId) { ... },
  async getProjects() { ... },
  async exportHarvest(startDate, endDate) { ... },
};
```

## 8. Implementation Plan

### Vertical Slice 1: Auth + Sync + Display (Working Prototype)
**Goal:** See your calendar events in the app

1. Project scaffolding (FastAPI, Docker, SQLite)
2. Database schema + migration script
3. Google OAuth flow (`/auth/*` routes)
4. Calendar sync service (fetch events, store in DB)
5. Basic week view (server-rendered HTML, no JS yet)
6. Event display (read-only cards)

**Deliverable:** User can log in, sync, and see their week's events.

### Vertical Slice 2: Classification
**Goal:** Transform events into time entries

7. Project CRUD (model, API, simple UI)
8. Classification API (`/api/entries`)
9. Flip card interaction (vanilla JS)
10. Classification dropdown on event cards
11. Hours editing + round-up button
12. Unclassify action

**Deliverable:** User can classify events and see them flip to time entries.

### Vertical Slice 3: Export + Polish
**Goal:** Get data out, refine UX

13. Harvest CSV export
14. Text filter in week view
15. Project visibility toggle
16. Week navigation (prev/next)
17. Sync button in UI

**Deliverable:** User can complete a full weekly timesheet workflow.

### Vertical Slice 4: Smart Classification
**Goal:** Speed up repeat classification

18. Classification rules model
19. Rule matching on sync
20. "Suggested" indicator on event cards
21. Auto-classify high-confidence matches
22. Bulk classification action

**Deliverable:** Returning users see suggested/auto-classifications.

### Vertical Slice 5: Docker + Deployment
**Goal:** Run on TrueNAS

23. Dockerfile (multi-stage, minimal image)
24. docker-compose.yaml
25. Volume for SQLite persistence
26. Environment variable configuration
27. Documentation for TrueNAS setup

**Deliverable:** App runs on TrueNAS.

## 9. Future Work: Google Cloud Run + Firestore

> **Status:** TODO - Not started. This is a large architectural change.

### Overview

Migrate from local Docker/SQLite deployment to Google Cloud Run with Firestore as the backing database. This enables:
- Always-on cloud hosting (no TrueNAS dependency)
- Serverless scaling (pay-per-use)
- Native Google Cloud integration (simplified OAuth, same ecosystem)
- Multi-user support potential

### Key Changes Required

#### Database Migration (SQLite → Firestore)
- Replace `db.py` SQLite wrapper with Firestore client
- Redesign data model for document database:
  - Collections: `users`, `events`, `time_entries`, `projects`, `rules`
  - Denormalize where appropriate for query efficiency
  - Handle atomic operations differently (transactions)
- Migrate existing data (export/import tooling)
- Update all service layer code that uses database

#### Authentication Changes
- Leverage Google Cloud Identity or Firebase Auth
- Store OAuth tokens in Firestore or Secret Manager
- Consider per-user data isolation (multi-tenant)

#### Application Changes
- Add `google-cloud-firestore` dependency
- Environment detection (local vs Cloud Run)
- Handle cold starts gracefully
- Update health check endpoints

#### Deployment Changes
- Create `cloudbuild.yaml` or GitHub Actions workflow
- Configure Cloud Run service
- Set up Firestore database and indexes
- Configure environment variables/secrets in Cloud Run
- Set up custom domain (optional)
- Implement CI/CD pipeline

#### Development Experience
- Local Firestore emulator for development
- Docker Compose option for local Firestore
- Clear documentation for both deployment targets

### Considerations

- **Cost**: Firestore and Cloud Run have free tiers, but need to estimate usage
- **Latency**: Cloud Run cold starts may affect UX
- **Complexity**: Two deployment targets to maintain (or deprecate TrueNAS)
- **Data model**: Document DB requires different thinking than relational

### Implementation Approach

1. Abstract database layer behind interface
2. Implement Firestore backend alongside SQLite
3. Add configuration to switch between backends
4. Test thoroughly with Firestore emulator
5. Deploy to Cloud Run staging environment
6. Migrate data and cut over

---

## 10. Open Design Questions

1. **OAuth token refresh**: How do we handle token expiry gracefully? Background refresh, or prompt user when needed?

2. **Event identity**: When re-syncing, how do we match updated events? Google event ID should be stable, but need to verify behavior with recurring events.

3. **Flip card animation**: CSS transitions should suffice, but need to prototype to ensure it feels right.

4. **Mobile responsiveness**: Not in PRD, but week view will need thought for narrow screens. Defer or design now?

## 10. Context for Claude

### File Structure

```
timesheet-app/
├── prd.md
├── design.md
├── src/
│   ├── main.py              # FastAPI app entry point
│   ├── config.py            # Environment/settings
│   ├── db.py                # SQLite wrapper
│   ├── models.py            # Pydantic models for API
│   ├── routes/
│   │   ├── auth.py          # OAuth routes
│   │   ├── api.py           # JSON API routes
│   │   └── ui.py            # HTML routes
│   ├── services/
│   │   ├── calendar.py      # Google Calendar integration
│   │   ├── classifier.py    # Classification logic
│   │   └── exporter.py      # CSV export
│   ├── templates/
│   │   ├── base.html
│   │   ├── login.html
│   │   ├── week.html
│   │   └── projects.html
│   └── static/
│       ├── css/
│       │   └── style.css
│       └── js/
│           ├── api.js       # Client-side API library
│           └── app.js       # UI interactions
├── tests/
│   ├── test_db.py
│   ├── test_classifier.py
│   └── test_api.py
├── migrations/
│   └── 001_initial.sql
├── Dockerfile
├── docker-compose.yaml
├── requirements.txt
└── README.md
```

### Naming Conventions

- Python: `snake_case` for functions/variables, `PascalCase` for classes
- Files: `snake_case.py`
- Database: `snake_case` for tables and columns
- JavaScript: `camelCase` for functions/variables
- CSS: `kebab-case` for classes

### Patterns to Follow

- **Routes are thin**: Validate input, call service, return response
- **Services contain logic**: Business rules, orchestration
- **DB layer is explicit SQL**: No ORM magic, clear queries
- **Errors return proper HTTP codes**: 400 for bad input, 401 for auth, 404 for not found, 500 for bugs
- **Tests mirror source structure**: `test_<module>.py` for each significant module

### Patterns to Avoid

- No global state except config
- No lazy imports or circular dependencies
- No try/except that swallows errors silently
- No raw SQL in routes (use db layer)
- No business logic in templates

### Running Locally

```bash
cd src
python -m venv venv
source venv/bin/activate
pip install -r requirements.txt
export GOOGLE_CLIENT_ID=xxx
export GOOGLE_CLIENT_SECRET=xxx
uvicorn main:app --reload
```

### Running Tests

```bash
pytest tests/
```

### Security Considerations

- OAuth tokens stored in SQLite; ensure file permissions restrict access
- No credentials in code or docker-compose; use environment variables
- CSRF protection for form submissions
- Input validation on all user-provided data
- Escape user content in templates (Jinja2 does this by default)
