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

## 9. Docker Deployment & TrueNAS Hosting

### 9.1 Overview

The application is packaged as a Docker container for deployment on TrueNAS Scale using the Custom App feature. This provides:
- Isolated runtime environment
- Easy updates via container image replacement
- Persistent data storage via volumes
- Portable deployment (can run anywhere Docker runs)

**Deployment targets:**
1. **TrueNAS Scale** (primary) - Self-hosted on local infrastructure
2. **Local development** - Docker Compose for testing
3. **Cloud VM** (future) - Any Docker host (DigitalOcean, Linode, etc.)

### 9.2 Dockerfile Design

**Strategy**: Multi-stage build to minimize image size and separate build dependencies from runtime.

```dockerfile
# --- Stage 1: Builder ---
FROM python:3.11-slim as builder

WORKDIR /app

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    && rm -rf /var/lib/apt/lists/*

# Copy only requirements first (layer caching)
COPY requirements.txt .
RUN pip install --user --no-cache-dir -r requirements.txt

# --- Stage 2: Runtime ---
FROM python:3.11-slim

WORKDIR /app

# Copy Python packages from builder
COPY --from=builder /root/.local /root/.local

# Copy application code
COPY src/ ./src/
COPY migrations/ ./migrations/

# Ensure Python packages are in PATH
ENV PATH=/root/.local/bin:$PATH

# Create non-root user for security
RUN useradd -m -u 1000 appuser && \
    chown -R appuser:appuser /app
USER appuser

# Expose port
EXPOSE 8000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health').read()"

# Run migrations on startup, then start server
CMD python src/db.py && uvicorn src.main:app --host 0.0.0.0 --port 8000
```

**Key decisions:**
- **Base image**: `python:3.11-slim` - smaller than full Python, includes essentials
- **Multi-stage**: Separates build tools from runtime (smaller final image)
- **Non-root user**: Security best practice
- **Health check**: TrueNAS can monitor container health
- **Port 8000**: Standard for FastAPI/uvicorn

### 9.3 docker-compose.yaml

For local development and TrueNAS deployment template:

```yaml
version: '3.8'

services:
  timesheet-app:
    build: .
    container_name: timesheet-app
    ports:
      - "8000:8000"
    environment:
      # Google OAuth credentials (REQUIRED)
      - GOOGLE_CLIENT_ID=${GOOGLE_CLIENT_ID}
      - GOOGLE_CLIENT_SECRET=${GOOGLE_CLIENT_SECRET}

      # OAuth redirect URI (adjust for your deployment)
      - OAUTH_REDIRECT_URI=http://localhost:8000/auth/callback

      # Anthropic API for LLM classification (OPTIONAL)
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY:-}

      # Database location
      - DATABASE_PATH=/data/timesheet.db

      # Production settings
      - ENVIRONMENT=production

    volumes:
      # Persistent SQLite database
      - ./data:/data

      # Optional: Mount source for development
      # - ./src:/app/src

    restart: unless-stopped

    # Resource limits (adjust based on usage)
    deploy:
      resources:
        limits:
          cpus: '1.0'
          memory: 512M
        reservations:
          cpus: '0.25'
          memory: 128M

# Named volumes for easier management
volumes:
  data:
    driver: local
```

**Environment variable strategy:**
- **Secrets via .env file** - Not committed to git
- **Override at runtime** - TrueNAS can inject environment variables
- **Defaults where sensible** - `DATABASE_PATH` has a default

### 9.4 Data Persistence

**SQLite Volume Mounting:**

The SQLite database must persist across container restarts. Three options:

1. **Bind mount** (docker-compose development):
   ```yaml
   volumes:
     - ./data:/data
   ```
   Database file lives in `./data/timesheet.db` on host.

2. **Named volume** (TrueNAS Custom App):
   ```yaml
   volumes:
     - timesheet-data:/data
   ```
   TrueNAS manages the volume; accessible via TrueNAS shell.

3. **Host path** (TrueNAS alternative):
   ```yaml
   volumes:
     - /mnt/pool/timesheet:/data
   ```
   Explicit path on TrueNAS pool for backups.

**Recommendation**: Use named volume for simplicity, but configure TrueNAS backups.

**Data directory structure:**
```
/data/
├── timesheet.db          # SQLite database
└── logs/                 # Optional: application logs
```

**Backup strategy:**
- SQLite database can be backed up with `sqlite3 .backup`
- TrueNAS can snapshot the volume
- Export time entries to CSV regularly as additional backup

### 9.5 Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GOOGLE_CLIENT_ID` | Yes | - | OAuth 2.0 client ID from Google Cloud Console |
| `GOOGLE_CLIENT_SECRET` | Yes | - | OAuth 2.0 client secret |
| `OAUTH_REDIRECT_URI` | Yes | - | Full URL for OAuth callback (e.g., `https://timesheet.example.com/auth/callback`) |
| `ANTHROPIC_API_KEY` | No | - | For LLM classification features |
| `DATABASE_PATH` | No | `/data/timesheet.db` | SQLite database file location |
| `ENVIRONMENT` | No | `development` | `production` or `development` |
| `LOG_LEVEL` | No | `INFO` | Logging verbosity: `DEBUG`, `INFO`, `WARNING`, `ERROR` |
| `SESSION_SECRET` | Yes | - | Secret key for session encryption (generate random string) |

**Generating secrets:**
```bash
# Session secret (32 bytes, hex-encoded)
python -c "import secrets; print(secrets.token_hex(32))"
```

### 9.6 OAuth Configuration for Docker/TrueNAS

**Critical consideration**: OAuth redirect URI must match exactly.

**For local development:**
```
Redirect URI: http://localhost:8000/auth/callback
Access URL: http://localhost:8000
```

**For TrueNAS with reverse proxy (recommended):**
```
Redirect URI: https://timesheet.yourdomain.com/auth/callback
Access URL: https://timesheet.yourdomain.com
```

**For TrueNAS direct access (IP only):**
```
Redirect URI: http://192.168.1.100:8000/auth/callback
Access URL: http://192.168.1.100:8000
```

**Google Cloud Console setup:**
1. Go to APIs & Services → Credentials
2. Create OAuth 2.0 Client ID (Web application)
3. Add authorized redirect URI(s) from above
4. Note: Cannot use `localhost` or IP addresses for production OAuth

**Workaround for IP-only access**: Use a service like nip.io for DNS:
```
http://timesheet.192.168.1.100.nip.io:8000/auth/callback
```

### 9.7 TrueNAS Scale Deployment

**Prerequisites:**
- TrueNAS Scale 22.x or later
- Docker image built and pushed to registry OR built on TrueNAS
- Google OAuth credentials configured

**Deployment steps:**

1. **Build and push Docker image** (from development machine):
   ```bash
   docker build -t yourusername/timesheet-app:latest .
   docker push yourusername/timesheet-app:latest
   ```

2. **Create Custom App in TrueNAS**:
   - Navigate to Apps → Discover Apps → Custom App
   - Fill in configuration:
     - **Application Name**: `timesheet-app`
     - **Image Repository**: `yourusername/timesheet-app`
     - **Image Tag**: `latest`
     - **Container Port**: `8000`
     - **Node Port**: Choose available port (e.g., `30000`)

3. **Configure Environment Variables**:
   Add each variable from section 9.5 in the TrueNAS UI.

4. **Configure Storage**:
   - Add Host Path Volume:
     - **Host Path**: `/mnt/pool/apps/timesheet/data`
     - **Mount Path**: `/data`
   - TrueNAS will create directory if it doesn't exist

5. **Deploy and verify**:
   - Click Deploy
   - Check logs: Apps → timesheet-app → Logs
   - Access: `http://truenas-ip:30000`

**Alternative: docker-compose on TrueNAS**:

TrueNAS Scale supports docker-compose via SSH:

```bash
# SSH to TrueNAS
ssh admin@truenas-ip

# Create app directory
mkdir -p /mnt/pool/apps/timesheet
cd /mnt/pool/apps/timesheet

# Copy docker-compose.yaml and .env
# ... (upload files)

# Deploy
docker-compose up -d

# View logs
docker-compose logs -f
```

### 9.8 Health Checks and Monitoring

**Health check endpoint** (`src/main.py`):

```python
@app.get("/health")
async def health_check():
    """Health check endpoint for container orchestration."""
    try:
        # Check database connectivity
        db = Database()
        db.execute("SELECT 1")

        return {
            "status": "healthy",
            "timestamp": datetime.utcnow().isoformat(),
            "database": "connected"
        }
    except Exception as e:
        raise HTTPException(status_code=503, detail=f"Unhealthy: {str(e)}")
```

**Monitoring in TrueNAS:**
- View container status in Apps dashboard
- Check logs for errors
- Set up notifications for container failures

**External monitoring** (optional):
- UptimeRobot or similar service pinging `/health`
- Grafana + Prometheus for metrics (future)

### 9.9 Logging

**Log to stdout/stderr** (Docker best practice):

```python
import logging
import sys

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[logging.StreamHandler(sys.stdout)]
)
```

**View logs:**
```bash
# Docker Compose
docker-compose logs -f

# TrueNAS Custom App
# Use TrueNAS UI: Apps → timesheet-app → Logs

# Direct Docker
docker logs -f timesheet-app
```

**Log rotation**: Docker handles this automatically with default settings.

### 9.10 Security Considerations

1. **Secrets management**:
   - Never commit `.env` file to git
   - Use TrueNAS secrets or environment variable injection
   - Rotate OAuth credentials periodically

2. **File permissions**:
   - SQLite database should be readable only by app user
   - Container runs as non-root user (UID 1000)

3. **Network isolation**:
   - Container only exposes port 8000
   - Use reverse proxy (nginx, Traefik) for HTTPS
   - Firewall rules to restrict access if needed

4. **HTTPS/TLS**:
   - **Do not** expose HTTP publicly
   - Use reverse proxy (TrueNAS nginx, Cloudflare Tunnel, etc.)
   - Let's Encrypt for free certificates

5. **OAuth security**:
   - Validate redirect URIs strictly
   - Use HTTPS for production OAuth
   - Store tokens encrypted in database (future enhancement)

### 9.11 Development vs. Production

**Development configuration**:
```yaml
# docker-compose.dev.yaml
services:
  timesheet-app:
    build: .
    environment:
      - ENVIRONMENT=development
      - LOG_LEVEL=DEBUG
      - OAUTH_REDIRECT_URI=http://localhost:8000/auth/callback
    volumes:
      - ./src:/app/src  # Hot reload
    command: uvicorn src.main:app --host 0.0.0.0 --port 8000 --reload
```

**Production configuration**:
```yaml
# docker-compose.yaml
services:
  timesheet-app:
    image: yourusername/timesheet-app:latest  # Pre-built image
    environment:
      - ENVIRONMENT=production
      - LOG_LEVEL=INFO
    # No source volume mount
    restart: unless-stopped
```

### 9.12 Troubleshooting

**Container won't start:**
- Check logs: `docker logs timesheet-app`
- Verify environment variables are set
- Check database path is writable

**OAuth redirect mismatch:**
- Ensure `OAUTH_REDIRECT_URI` exactly matches Google Cloud Console
- Check for trailing slashes, http vs https
- Verify port numbers match

**Database locked errors:**
- SQLite doesn't support multiple writers well
- Ensure only one container instance is running
- Check file permissions on `/data`

**Port already in use:**
- Change host port in docker-compose: `"8001:8000"`
- Or stop conflicting service

**Slow performance:**
- Check resource limits in docker-compose
- SQLite may need tuning for concurrent access
- Consider migrating to Postgres for multi-user

### 9.13 Future Enhancements

- **CI/CD pipeline**: Automated builds on GitHub push
- **Multi-architecture images**: Support ARM for Raspberry Pi
- **Kubernetes deployment**: Helm chart for k8s clusters
- **Secrets management**: Integration with Vault or sealed secrets
- **Observability**: OpenTelemetry tracing, Prometheus metrics

### 9.14 Multi-User Support (Temporary Solution)

> **Status:** Documented workaround. Not yet implemented.
> **Related Bug:** BUG-034 in BUGS.md

**Problem:**

The application architecture is fundamentally single-user. When multiple users log in via OAuth:
- All users share the same `auth_tokens` table entry
- Second user's login overwrites first user's tokens
- All calendar operations use whichever tokens were stored last
- Users can inadvertently access each other's calendar data

**Root Cause:**

The schema has no `user_id` concept:
```sql
CREATE TABLE auth_tokens (
    id INTEGER PRIMARY KEY,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    -- Missing: user_id, email, etc.
    ...
);
```

**Current Workaround:**

Deploy separate Docker container instances per user. Each user gets:
- Their own container
- Their own database file
- Complete isolation

**Proposed Temporary Solution: Email-Based Database Paths**

Instead of one shared database, create a separate database file per user based on their email address:

```
/data/
├── timesheet-alice_at_example_com.db
├── timesheet-bob_at_example_com.db
└── timesheet-charlie_at_work_net.db
```

**Implementation approach:**

1. **Add session management** to track logged-in user:
   ```python
   # main.py
   from starlette.middleware.sessions import SessionMiddleware

   app.add_middleware(
       SessionMiddleware,
       secret_key=config.SECRET_KEY,
       max_age=7 * 24 * 60 * 60  # 7 days
   )
   ```

2. **Store user email in session after OAuth**:
   ```python
   # routes/auth.py - in auth_callback()

   # After successful OAuth, fetch user profile
   from googleapiclient.discovery import build

   service = build('oauth2', 'v2', credentials=credentials)
   user_info = service.userinfo().get().execute()

   # Store in session
   request.session["user_email"] = user_info["email"]
   request.session["user_name"] = user_info.get("name", "")
   ```

3. **Dynamic database path based on session**:
   ```python
   # db.py

   def get_user_db_path(email: str | None) -> Path:
       """Get database path for a specific user."""
       if not email:
           # Fallback for unauthenticated requests
           return Path(os.environ.get("DATABASE_PATH", "/data/timesheet.db"))

       # Sanitize email for use in filename
       safe_email = email.replace("@", "_at_").replace(".", "_")
       return Path(f"/data/timesheet-{safe_email}.db")

   # Modify Database class to accept path per-request
   class Database:
       def __init__(self, db_path: Path | None = None):
           self.db_path = db_path or get_default_path()
   ```

4. **Request-scoped database initialization**:
   ```python
   # main.py or middleware

   from starlette.middleware.base import BaseHTTPMiddleware

   class DatabaseMiddleware(BaseHTTPMiddleware):
       async def dispatch(self, request, call_next):
           email = request.session.get("user_email")
           db_path = get_user_db_path(email)

           # Initialize database for this user
           request.state.db = Database(db_path)

           response = await call_next(request)
           return response
   ```

5. **Update route handlers to use request-scoped DB**:
   ```python
   # routes/api.py

   @router.post("/sync")
   async def sync_events(request: Request):
       db = request.state.db  # User-specific database
       # ... rest of sync logic
   ```

**Benefits:**

- ✅ No schema changes required
- ✅ Complete data isolation between users
- ✅ Each user has independent projects, rules, classifications
- ✅ Works with existing single-user codebase
- ✅ Relatively simple implementation (~100 lines of code)
- ✅ Can support unlimited users in single container

**Limitations:**

- ⚠️ Cannot share projects or rules between users
- ⚠️ Storage grows linearly with number of users
- ⚠️ No centralized user management
- ⚠️ Database file proliferation
- ⚠️ Still requires OAuth per user
- ⚠️ No admin interface to manage users

**Migration Path:**

This solution is a stepping stone, not a permanent architecture:

1. **Short-term** (this solution): Email-based database files
   - Enables multi-user support quickly
   - Isolated data per user
   - Minimal code changes

2. **Medium-term**: Add users table, keep SQLite
   - Add `users` table with email, name, settings
   - Add `user_id` foreign keys to all tables
   - Single shared database with proper isolation
   - Enables features like shared projects

3. **Long-term**: Migrate to Google Cloud Run + Firestore
   - Proper multi-tenant architecture
   - Scalable cloud infrastructure
   - See section 10 for details

**Dependencies:**

```txt
# requirements.txt additions
itsdangerous>=2.1.0  # For session signing (already included via Starlette)
```

**Environment Variables:**

```bash
# .env additions
SESSION_MAX_AGE=604800  # 7 days in seconds (optional)
```

**Testing Multi-User:**

1. Open browser in normal mode, log in as user1@example.com
2. Open browser in incognito mode, log in as user2@example.com
3. Each should see their own calendar and classifications
4. Verify separate database files created:
   ```bash
   docker exec timesheet-app ls -lh /data/
   # Should show:
   # timesheet-user1_at_example_com.db
   # timesheet-user2_at_example_com.db
   ```

**Security Considerations:**

- Session cookies must be HTTP-only and secure (HTTPS in production)
- Database file names contain sanitized emails (no special chars)
- Each user can only access their own database (enforced by session)
- Logout should clear session completely

**Implementation Checklist:**

- [ ] Add SessionMiddleware to main.py
- [ ] Update auth callback to fetch and store user email
- [ ] Implement `get_user_db_path()` function
- [ ] Add DatabaseMiddleware for request-scoped DB
- [ ] Update all route handlers to use `request.state.db`
- [ ] Add logout endpoint that clears session
- [ ] Test with multiple users
- [ ] Update DOCKER.md with multi-user instructions
- [ ] Add migration guide if upgrading from single-user deployment

**Related Issues:**

- **BUG-034**: Documents the original multi-user collision problem
- **Section 10**: Long-term proper multi-user architecture via Cloud Run

## 10. Future Work: Google Cloud Run + Firestore

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

## 11. Open Design Questions

1. **OAuth token refresh**: How do we handle token expiry gracefully? Background refresh, or prompt user when needed?

2. **Event identity**: When re-syncing, how do we match updated events? Google event ID should be stable, but need to verify behavior with recurring events.

3. **Flip card animation**: CSS transitions should suffice, but need to prototype to ensure it feels right.

4. **Mobile responsiveness**: Not in PRD, but week view will need thought for narrow screens. Defer or design now?

## 12. Context for Claude

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
