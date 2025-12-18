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

    # Database - PostgreSQL connection string
    DATABASE_URL: str = os.environ.get(
        "DATABASE_URL",
        "postgresql://timesheet:changeMe123!@localhost:5432/timesheet"
    )

    # Application
    SECRET_KEY: str = os.environ.get("SECRET_KEY", "dev-secret-change-in-production")
    ENVIRONMENT: str = os.environ.get("ENVIRONMENT", "development")
    DEBUG: bool = os.environ.get("DEBUG", "true").lower() == "true"
    LOG_LEVEL: str = os.environ.get("LOG_LEVEL", "INFO").upper()

    # Google API scopes (Calendar + Sheets)
    # Note: drive.file only grants access to files created by this app
    GOOGLE_SCOPES: list[str] = [
        "openid",
        "https://www.googleapis.com/auth/userinfo.email",
        "https://www.googleapis.com/auth/userinfo.profile",
        "https://www.googleapis.com/auth/calendar.readonly",
        "https://www.googleapis.com/auth/drive.file",
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
