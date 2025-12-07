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

    # Anthropic API (for LLM classification features)
    ANTHROPIC_API_KEY: str = os.environ.get("ANTHROPIC_API_KEY", "")

    # Database
    DATABASE_PATH: Path = Path(
        os.environ.get("DATABASE_PATH", "timesheet.db")
    )

    # Application
    SECRET_KEY: str = os.environ.get("SECRET_KEY", "dev-secret-change-in-production")
    ENVIRONMENT: str = os.environ.get("ENVIRONMENT", "development")
    DEBUG: bool = os.environ.get("DEBUG", "true").lower() == "true"
    LOG_LEVEL: str = os.environ.get("LOG_LEVEL", "INFO").upper()

    # Google Calendar API scopes
    GOOGLE_SCOPES: list[str] = [
        "openid",
        "https://www.googleapis.com/auth/userinfo.email",
        "https://www.googleapis.com/auth/userinfo.profile",
        "https://www.googleapis.com/auth/calendar.readonly",
    ]

    @property
    def is_production(self) -> bool:
        """Check if running in production environment."""
        return self.ENVIRONMENT.lower() == "production"

    @property
    def is_development(self) -> bool:
        """Check if running in development environment."""
        return self.ENVIRONMENT.lower() == "development"


config = Config()
