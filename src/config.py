"""Application configuration from environment variables."""

import os
from pathlib import Path


class Config:
    """Configuration loaded from environment variables."""

    # Google OAuth
    GOOGLE_CLIENT_ID: str = os.environ.get("GOOGLE_CLIENT_ID", "")
    GOOGLE_CLIENT_SECRET: str = os.environ.get("GOOGLE_CLIENT_SECRET", "")

    # OAuth redirect URI - must match Google Cloud Console configuration
    OAUTH_REDIRECT_URI: str = os.environ.get(
        "OAUTH_REDIRECT_URI",
        "http://localhost:8000/auth/callback"
    )

    # Database
    DATABASE_PATH: Path = Path(
        os.environ.get("DATABASE_PATH", "timesheet.db")
    )

    # Application
    SECRET_KEY: str = os.environ.get("SECRET_KEY", "dev-secret-change-in-production")
    DEBUG: bool = os.environ.get("DEBUG", "true").lower() == "true"

    # Google Calendar API scopes
    GOOGLE_SCOPES: list[str] = [
        "https://www.googleapis.com/auth/calendar.readonly",
    ]


config = Config()
