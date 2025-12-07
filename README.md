# Timesheet App

Automatic timesheet creation from Google Calendar.

## Quick Start

### Prerequisites

- Python 3.11+
- Google Cloud project with Calendar API enabled
- OAuth 2.0 credentials (Web application type)

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
   - Enter: `http://localhost:8000/auth/callback`
6. Click "Create"
7. A dialog will show your credentials:
   - **Client ID**: Copy this (looks like `xxxx.apps.googleusercontent.com`)
   - **Client Secret**: Copy this (shorter string)
8. Click "OK"

You can always find these again by clicking on the credential name in the Credentials list.

#### 5. Set Environment Variables

```bash
export GOOGLE_CLIENT_ID="your-client-id-here"
export GOOGLE_CLIENT_SECRET="your-client-secret-here"
```

**Important Notes:**
- The app is in "Testing" mode, so only the test users you added can log in
- To allow anyone to log in, you'd need to publish the app (requires verification)
- For local development, testing mode is fine

### Local Development

```bash
cd src

# Create virtual environment
python -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate

# Install dependencies
pip install -r ../requirements.txt

# Set environment variables
export GOOGLE_CLIENT_ID=your-client-id
export GOOGLE_CLIENT_SECRET=your-client-secret

# Run the app
uvicorn main:app --reload
```

Visit http://localhost:8000

### Docker

#### Local Development

```bash
# Create .env file
cat > .env << EOF
GOOGLE_CLIENT_ID=your-client-id
GOOGLE_CLIENT_SECRET=your-client-secret
SECRET_KEY=$(python3 -c "import secrets; print(secrets.token_hex(32))")
EOF

# Build and run with docker-compose
docker-compose up --build

# Or use the Makefile
make build
make run
```

Visit http://localhost:8000

#### Building and Pushing to DockerHub

The project is configured to push images to `michaelwinser/timesheet-app` on DockerHub.

**Important for Apple Silicon Users:**
If you're developing on Apple Silicon (M1/M2/M3 Mac) and deploying to TrueNAS on Intel/AMD, you need to build multi-architecture images:

```bash
# Login to DockerHub (one-time setup)
make login

# Build for both architectures and push (RECOMMENDED)
make build-multiarch VERSION=v1.0.0
```

This builds a single image that works on both ARM (Apple Silicon) and AMD64 (Intel/AMD) architectures.

**Alternative: Single-platform builds**

```bash
# Build for specific platform (e.g., for TrueNAS Intel/AMD)
make build VERSION=v1.0.0 PLATFORM=linux/amd64
make push VERSION=v1.0.0

# Build for local testing on Apple Silicon (faster)
make build-local

# Tag an existing image with a new version
make tag TAG=v1.0.0
```

**Available Makefile Commands:**
- `make help` - Show all available commands
- `make build` - Build Docker image locally
- `make push` - Push Docker image to DockerHub
- `make build-push` - Build and push in one command
- `make login` - Login to DockerHub
- `make run` - Start container with docker-compose
- `make stop` - Stop container
- `make logs` - View container logs
- `make test` - Run health check
- `make rebuild` - Rebuild without cache and restart

#### Pulling from DockerHub

To use the pre-built image instead of building locally:

```bash
# Pull the latest image
docker pull michaelwinser/timesheet-app:latest

# Or pull a specific version
docker pull michaelwinser/timesheet-app:v1.0.0

# Run with docker-compose (will use pre-built image)
docker-compose up
```

### TrueNAS Deployment

**Option 1: Use Pre-built Image from DockerHub (Recommended)**

1. Use `docker-compose.prod.yaml` as your deployment configuration
2. The image `michaelwinser/timesheet-app:latest` will be pulled automatically
3. Configure environment variables in TrueNAS Custom App UI:
   - `GOOGLE_CLIENT_ID`
   - `GOOGLE_CLIENT_SECRET`
   - `SECRET_KEY` (generate with: `python -c "import secrets; print(secrets.token_hex(32))"`)
   - `OAUTH_REDIRECT_URI` (e.g., `https://timesheet.yourdomain.com/auth/callback`)
4. Set data volume path: `/mnt/pool/apps/timesheet/data:/data`
5. Deploy and access via your TrueNAS IP or domain

**Option 2: Build Locally on TrueNAS**

1. Copy repository files to TrueNAS
2. Uncomment the `build: .` line in `docker-compose.prod.yaml`
3. Comment out the `image:` line
4. Follow Option 1 steps for configuration

## Usage

1. Click "Sign in with Google" to authenticate
2. Click "Sync" to fetch calendar events
3. Classify events by selecting a project from the dropdown
4. Click "Export CSV" to download Harvest-compatible timesheet

## API Documentation

Visit `/docs` for interactive Swagger UI.

## Project Structure

```
src/
├── main.py              # FastAPI entry point
├── config.py            # Configuration
├── db.py                # SQLite wrapper
├── models.py            # Pydantic models
├── routes/
│   ├── auth.py          # OAuth routes
│   ├── api.py           # JSON API
│   └── ui.py            # HTML pages
├── services/
│   ├── calendar.py      # Google Calendar
│   ├── classifier.py    # Classification logic
│   └── exporter.py      # CSV export
├── templates/           # Jinja2 templates
└── static/              # CSS, JS
```
