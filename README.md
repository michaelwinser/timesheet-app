# Timesheet App

Automatic timesheet creation from Google Calendar.

Go API (`service/`) serving a SvelteKit front end (`web/`), backed by PostgreSQL.
Everything runs through Docker Compose.

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Google Cloud project with Calendar API enabled
- OAuth 2.0 credentials (Web application type)

Go 1.24 and Node 20 are only needed to build outside Docker; the image builds
both itself.

### Google Cloud Setup

#### 1. Create or Select a Project

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Click the project dropdown (top left, next to "Google Cloud")
3. Click "New Project" or select an existing one
4. If creating new: enter a name (e.g., "Timesheet App") and click "Create"
5. Wait for the project to be created, then select it

#### 2. Enable the Google Calendar API

1. Go to [APIs & Services > Library](https://console.cloud.google.com/apis/library)
2. Search for "Google Calendar API"
3. Click on "Google Calendar API"
4. Click "Enable"

#### 3. Configure OAuth Consent Screen

1. Go to [APIs & Services > OAuth consent screen](https://console.cloud.google.com/apis/credentials/consent)
2. Select "External" (unless you have a Google Workspace org) and click "Create"
3. Fill in the required fields:
   - App name: "Timesheet App"
   - User support email: your email
   - Developer contact: your email
4. Click "Save and Continue"
5. On "Scopes" page, click "Add or Remove Scopes"
   - Find and select `https://www.googleapis.com/auth/calendar.readonly`
   - Click "Update"
6. Click "Save and Continue"
7. On "Test users" page, click "Add Users"
   - Add your Google email address
   - Click "Add"
8. Click "Save and Continue"
9. Review and click "Back to Dashboard"

#### 4. Create OAuth Credentials

1. Go to [APIs & Services > Credentials](https://console.cloud.google.com/apis/credentials)
2. Click "Create Credentials" > "OAuth client ID"
3. Application type: "Web application"
4. Name: "Timesheet App" (or any name)
5. Under "Authorized redirect URIs", click "Add URI"
   - Enter: `http://localhost:8080/api/auth/google/callback`
   - For a deployed instance, add its URL too, e.g.
     `https://timesheet.yourdomain.com/api/auth/google/callback`
6. Click "Create"
7. A dialog will show your credentials:
   - **Client ID**: Copy this (looks like `xxxx.apps.googleusercontent.com`)
   - **Client Secret**: Copy this (shorter string)
8. Click "OK"

You can always find these again by clicking on the credential name in the Credentials list.

#### 5. Set Environment Variables

The service reads these (defaults in parentheses):

| Variable | Required | Purpose |
| --- | --- | --- |
| `GOOGLE_CLIENT_ID` | yes | OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | yes | OAuth client secret |
| `GOOGLE_REDIRECT_URL` | yes in prod | OAuth callback (`http://localhost:8080/api/auth/google/callback`) |
| `JWT_SECRET` | yes in prod | Signs session tokens (insecure development default) |
| `ENCRYPTION_KEY` | yes | Encrypts stored Google refresh tokens |
| `DATABASE_URL` | yes in prod | PostgreSQL connection string |
| `PORT` | no | Listen port (`8080`) |
| `BASE_URL` | no | Public base URL (`http://localhost:$PORT`) |
| `BACKGROUND_SYNC_ENABLED` | no | Periodic calendar sync (`true`) |

Generate the secrets with `openssl rand -hex 32`.

**Important Notes:**
- The app is in "Testing" mode, so only the test users you added can log in
- To allow anyone to log in, you'd need to publish the app (requires verification)
- For local development, testing mode is fine

### Local Development

```bash
# Generate .env with your Google OAuth credentials and secrets
cat > .env << EOF
GOOGLE_CLIENT_ID=your-client-id
GOOGLE_CLIENT_SECRET=your-client-secret
JWT_SECRET=$(openssl rand -hex 32)
ENCRYPTION_KEY=$(openssl rand -hex 32)
EOF

make up          # start postgres + api
make logs        # follow the api logs
make down        # stop
```

Visit http://localhost:8080

After changing Go or Svelte source, rebuild the image - the front end is
compiled into it, so a restart alone will not pick up web changes:

```bash
make build && make up
```

Run `make help` for the full command list, including the database helpers.

### Backups

```bash
make db-backup                             # dump to ./backups (no downtime)
make db-verify-backup FILE=<dump>          # prove it restores, in a scratch container
make db-restore FILE=<dump>                # replace the database (destructive)
```

Verify every backup you intend to rely on - `db-verify-backup` restores into a
throwaway container and prints row counts, without touching the live database.

The dump contains API key hashes, MCP tokens, and Google refresh tokens
encrypted with `ENCRYPTION_KEY`. Treat it as a credential, and keep a copy of
`ENCRYPTION_KEY` somewhere separate: restore the dump without it and the stored
Google credentials cannot be decrypted, so every calendar connection has to be
re-authorised.

### Docker

#### Building and Pushing to Docker Hub

Images are published to `michaelwinser/timesheet-app`, which is what
`docker-compose.prod.yaml` pulls.

TrueNAS runs on Intel/AMD while an Apple Silicon Mac builds arm64 by default, so
publish multi-architecture images. A single-architecture arm64 push will not run
on the server.

```bash
make login                              # one-time
make build-multiarch                    # build amd64 + arm64 and push :latest
make build-multiarch VERSION=v1.2.3     # publish that version, and move :latest
```

A versioned `build-multiarch` tags both `VERSION` and `latest`, so production
picks up the release while the version tag stays available to roll back to.

`build-multiarch` pushes as part of the buildx run and then prints the platforms
present on the tag, so you can confirm `linux/amd64` made it.

Single-architecture builds, for local testing:

```bash
make image                    # tagged build for PLATFORM (default linux/amd64)
make image-local              # tagged build for this machine
make push                     # push what those built
make build-push               # image, then push
make tag TAG=1.0.5            # retag VERSION as TAG registry-side, no rebuild
```

Note that `make build` is the local Compose build and does not tag or publish
anything.

#### Pulling from Docker Hub

```bash
make pull                     # or: make pull VERSION=v1.2.3
docker pull michaelwinser/timesheet-app:latest
```

### TrueNAS Deployment

Deploy the published image with `docker-compose.prod.yaml`.

1. Publish the image first: `make build-multiarch`
2. Copy `docker-compose.prod.yaml` to the TrueNAS deployment directory as
   `docker-compose.yaml`
3. Create a `.env` alongside it:

   ```
   POSTGRES_PASSWORD=...
   JWT_SECRET=...
   ENCRYPTION_KEY=...
   GOOGLE_CLIENT_ID=...
   GOOGLE_CLIENT_SECRET=...
   GOOGLE_REDIRECT_URL=https://timesheet.yourdomain.com/api/auth/google/callback
   PORT=8000
   POSTGRES_DATA_PATH=./postgres-data
   ```

   `ENCRYPTION_KEY` must match the one the data was written with - changing it
   makes stored Google refresh tokens undecryptable.

   Note that `PORT` means something different here than in the table above: the
   compose file uses it as the *host* port it publishes (`${PORT:-8000}:8080`)
   and always sets the container's own `PORT` to 8080.

4. Create the data directory: `mkdir -p ./postgres-data`
5. Deploy:

   ```bash
   docker compose pull
   docker compose up -d
   ```

To update an existing deployment, publish a new image and repeat step 5;
`pull` picks up the new `:latest` and `up -d` recreates the API container.
Database migrations run automatically on startup.

Note that `docker-compose.prod.yaml` declares both `build:` and `image:`. The
`image:` line is what gets pulled; the `build:` section is only used if you
build on the TrueNAS box itself.

## Usage

1. Click "Sign in with Google" to authenticate
2. Click "Sync" to fetch calendar events
3. Classify events by selecting a project from the dropdown
4. Click "Export CSV" to download Harvest-compatible timesheet

## API Documentation

The OpenAPI spec is the source of truth for the API and is served at
`/api/openapi.yaml`. It lives at `docs/v2/api-spec.yaml`; run `make generate`
after editing it to regenerate the server types.

## Project Structure

```
service/                 # Go API
├── cmd/server/          # Entry point
└── internal/
    ├── api/             # Generated from the OpenAPI spec
    ├── handler/         # HTTP layer
    ├── classification/  # Rule parsing, matching, classification
    ├── store/           # Database access
    ├── sync/            # Google Calendar synchronisation
    ├── timeentry/       # Time entry computation
    └── database/        # Schema migrations
web/                     # SvelteKit front end, built into the image
├── src/lib/components/  # Primitives and widgets
└── src/routes/          # Pages
docs/v2/                 # Design documents and api-spec.yaml
tests/integration/       # CLI integration scenarios
```
