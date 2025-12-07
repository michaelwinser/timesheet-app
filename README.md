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

```bash
# Create .env file
cat > .env << EOF
GOOGLE_CLIENT_ID=your-client-id
GOOGLE_CLIENT_SECRET=your-client-secret
EOF

# Build and run
docker-compose up --build
```

Visit http://localhost:8000

### TrueNAS Deployment

1. Copy files to TrueNAS
2. Create a Custom App with docker-compose.yaml
3. Configure environment variables in TrueNAS UI
4. Update OAUTH_REDIRECT_URI to match your TrueNAS URL

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
