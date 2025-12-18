"""Authentication routes for Google OAuth with multi-user support."""

import logging
from fastapi import APIRouter, Request, HTTPException
from fastapi.responses import RedirectResponse
from google_auth_oauthlib.flow import Flow
from google.oauth2.credentials import Credentials

from config import config
from db import get_db
from models import AuthStatus

router = APIRouter()
logger = logging.getLogger(__name__)


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


def get_or_create_user(email: str, name: str = None) -> dict:
    """Get existing user or create new one.

    Args:
        email: User email from Google OAuth
        name: User display name from Google OAuth

    Returns:
        User dict with id, email, name
    """
    db = get_db()

    # Try to find existing user
    user = db.execute_one(
        "SELECT id, email, name FROM users WHERE email = %s",
        (email,)
    )

    if user:
        # Update last login time
        db.execute(
            "UPDATE users SET last_login_at = CURRENT_TIMESTAMP WHERE id = %s",
            (user['id'],)
        )
        logger.info(f"User logged in: {email} (id={user['id']})")
        return user

    # Create new user
    user_id = db.execute_insert(
        """
        INSERT INTO users (email, name, last_login_at)
        VALUES (%s, %s, CURRENT_TIMESTAMP)
        RETURNING id
        """,
        (email, name)
    )

    logger.info(f"New user created: {email} (id={user_id})")

    return {
        'id': user_id,
        'email': email,
        'name': name
    }


def get_stored_credentials(user_id: int) -> Credentials | None:
    """Get stored OAuth credentials from database for a specific user.

    Args:
        user_id: User ID to get credentials for

    Returns:
        Credentials object or None if not found
    """
    db = get_db()
    row = db.execute_one(
        """
        SELECT access_token, refresh_token, token_expiry
        FROM auth_tokens
        WHERE user_id = %s
        ORDER BY id DESC
        LIMIT 1
        """,
        (user_id,)
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


def store_credentials(user_id: int, credentials: Credentials) -> None:
    """Store OAuth credentials in database for a specific user.

    Uses PostgreSQL's ON CONFLICT to upsert (update existing or insert new).

    Args:
        user_id: User ID to store credentials for
        credentials: OAuth credentials from Google
    """
    db = get_db()

    # Upsert - replace existing credentials for this user
    db.execute(
        """
        INSERT INTO auth_tokens (user_id, access_token, refresh_token, token_expiry)
        VALUES (%s, %s, %s, %s)
        ON CONFLICT (user_id) DO UPDATE SET
            access_token = EXCLUDED.access_token,
            refresh_token = EXCLUDED.refresh_token,
            token_expiry = EXCLUDED.token_expiry,
            created_at = CURRENT_TIMESTAMP
        """,
        (
            user_id,
            credentials.token,
            credentials.refresh_token,
            credentials.expiry.isoformat() if credentials.expiry else None,
        ),
    )
    logger.debug(f"Stored credentials for user_id={user_id}")


@router.get("/login")
async def login(request: Request, next: str = None):
    """Redirect to Google OAuth consent screen."""
    if not config.GOOGLE_CLIENT_ID or not config.GOOGLE_CLIENT_SECRET:
        raise HTTPException(
            status_code=500,
            detail="Google OAuth not configured. Set GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET.",
        )

    # Store the next parameter in session to redirect after OAuth
    if next:
        request.session["next_url"] = next

    flow = get_oauth_flow()
    auth_url, _ = flow.authorization_url(
        access_type="offline",
        include_granted_scopes="true",
        prompt="consent",
    )
    return RedirectResponse(url=auth_url)


@router.get("/callback")
async def callback(request: Request, code: str = None, error: str = None):
    """Handle OAuth callback from Google."""
    if error:
        raise HTTPException(status_code=400, detail=f"OAuth error: {error}")

    if not code:
        raise HTTPException(status_code=400, detail="Missing authorization code")

    flow = get_oauth_flow()
    flow.fetch_token(code=code)
    credentials = flow.credentials

    # Get user email from ID token
    import google.auth.transport.requests
    from google.oauth2 import id_token

    # Verify and decode the ID token to get user info
    id_info = id_token.verify_oauth2_token(
        credentials.id_token,
        google.auth.transport.requests.Request(),
        config.GOOGLE_CLIENT_ID
    )

    # Get or create user record
    user = get_or_create_user(
        email=id_info["email"],
        name=id_info.get("name", "")
    )

    # Store user info in session
    request.session["user_email"] = user['email']
    request.session["user_name"] = user.get('name', '')

    # Store credentials in database (linked to user)
    store_credentials(user['id'], credentials)

    # Redirect to next_url from session, or home page if not set
    next_url = request.session.pop("next_url", "/")
    return RedirectResponse(url=next_url)


@router.post("/logout")
async def logout(request: Request):
    """Clear stored OAuth tokens and session."""
    # Get user_id from request state (set by UserContextMiddleware)
    user_id = getattr(request.state, 'user_id', None)

    if user_id:
        # Clear OAuth tokens from database for this user only
        db = get_db()
        db.execute("DELETE FROM auth_tokens WHERE user_id = %s", (user_id,))
        logger.info(f"User logged out: user_id={user_id}")

    # Clear session
    request.session.clear()

    # Redirect to login page
    return RedirectResponse(url="/login", status_code=303)


@router.get("/status", response_model=AuthStatus)
async def status(request: Request):
    """Check authentication status."""
    user_id = getattr(request.state, 'user_id', None)
    user_email = request.session.get("user_email")

    if not user_id:
        return AuthStatus(authenticated=False, email=None)

    credentials = get_stored_credentials(user_id)

    if credentials is None:
        return AuthStatus(authenticated=False, email=None)

    return AuthStatus(authenticated=True, email=user_email)
