"""Authentication routes for Google OAuth."""

from fastapi import APIRouter, Request, HTTPException
from fastapi.responses import RedirectResponse
from google_auth_oauthlib.flow import Flow
from google.oauth2.credentials import Credentials

from config import config
from db import get_db
from models import AuthStatus

router = APIRouter()


def get_oauth_flow() -> Flow:
    """Create OAuth flow for Google Calendar."""
    client_config = {
        "web": {
            "client_id": config.GOOGLE_CLIENT_ID,
            "client_secret": config.GOOGLE_CLIENT_SECRET,
            "auth_uri": "https://accounts.google.com/o/oauth2/auth",
            "token_uri": "https://oauth2.googleapis.com/token",
            "redirect_uris": [config.OAUTH_REDIRECT_URI],
        }
    }
    flow = Flow.from_client_config(
        client_config,
        scopes=config.GOOGLE_SCOPES,
        redirect_uri=config.OAUTH_REDIRECT_URI,
    )
    return flow


def get_stored_credentials() -> Credentials | None:
    """Get stored OAuth credentials from database."""
    db = get_db()
    row = db.execute_one(
        "SELECT access_token, refresh_token, token_expiry FROM auth_tokens ORDER BY id DESC LIMIT 1"
    )
    if row is None:
        return None

    return Credentials(
        token=row["access_token"],
        refresh_token=row["refresh_token"],
        token_uri="https://oauth2.googleapis.com/token",
        client_id=config.GOOGLE_CLIENT_ID,
        client_secret=config.GOOGLE_CLIENT_SECRET,
    )


def store_credentials(credentials: Credentials) -> None:
    """Store OAuth credentials in database."""
    db = get_db()
    db.execute(
        """
        INSERT INTO auth_tokens (access_token, refresh_token, token_expiry)
        VALUES (?, ?, ?)
        """,
        (
            credentials.token,
            credentials.refresh_token,
            credentials.expiry.isoformat() if credentials.expiry else None,
        ),
    )


@router.get("/login")
async def login():
    """Redirect to Google OAuth consent screen."""
    if not config.GOOGLE_CLIENT_ID or not config.GOOGLE_CLIENT_SECRET:
        raise HTTPException(
            status_code=500,
            detail="Google OAuth not configured. Set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET.",
        )

    flow = get_oauth_flow()
    auth_url, _ = flow.authorization_url(
        access_type="offline",
        include_granted_scopes="true",
        prompt="consent",
    )
    return RedirectResponse(url=auth_url)


@router.get("/callback")
async def callback(code: str = None, error: str = None):
    """Handle OAuth callback from Google."""
    if error:
        raise HTTPException(status_code=400, detail=f"OAuth error: {error}")

    if not code:
        raise HTTPException(status_code=400, detail="Missing authorization code")

    flow = get_oauth_flow()
    flow.fetch_token(code=code)
    credentials = flow.credentials

    store_credentials(credentials)

    return RedirectResponse(url="/")


@router.post("/logout")
async def logout():
    """Clear stored OAuth tokens."""
    db = get_db()
    db.execute("DELETE FROM auth_tokens")
    return {"status": "logged_out"}


@router.get("/status", response_model=AuthStatus)
async def status():
    """Check authentication status."""
    credentials = get_stored_credentials()
    if credentials is None:
        return AuthStatus(authenticated=False)

    # Could fetch user info here if we add email scope
    return AuthStatus(authenticated=True)
