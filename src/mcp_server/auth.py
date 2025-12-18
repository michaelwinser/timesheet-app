"""Authentication providers for MCP server.

Supports two modes:
- EnvAuthProvider: For stdio transport (Claude Desktop), user email from env var
- OAuthProvider: For SSE transport (remote), user from JWT token
"""

import os
from abc import ABC, abstractmethod
from dataclasses import dataclass


@dataclass
class AuthenticatedUser:
    """Represents an authenticated user."""
    user_id: int
    email: str


class AuthProvider(ABC):
    """Abstract base class for authentication providers."""

    @abstractmethod
    def get_current_user(self) -> AuthenticatedUser:
        """Get the authenticated user for the current request."""
        pass


class EnvAuthProvider(AuthProvider):
    """Auth via environment variable (stdio transport).

    Reads TIMESHEET_USER_EMAIL from environment and looks up the user
    in the database at initialization time. All subsequent requests
    use this cached user.
    """

    def __init__(self, db):
        """Initialize with database connection.

        Args:
            db: Database connection object with execute_one method

        Raises:
            ValueError: If TIMESHEET_USER_EMAIL not set or user not found
        """
        self.db = db

        email = os.environ.get("TIMESHEET_USER_EMAIL")
        if not email:
            raise ValueError(
                "TIMESHEET_USER_EMAIL environment variable not set. "
                "This is required for MCP server authentication."
            )

        user = db.execute_one(
            "SELECT id, email FROM users WHERE email = %s",
            (email,)
        )
        if not user:
            raise ValueError(
                f"User not found: {email}. "
                "Please ensure this user exists in the database."
            )

        self._user = AuthenticatedUser(
            user_id=user["id"],
            email=user["email"]
        )

    def get_current_user(self) -> AuthenticatedUser:
        """Get the authenticated user."""
        return self._user


class OAuthProvider(AuthProvider):
    """Auth via OAuth token (SSE transport).

    Validates JWT tokens and extracts user information.
    Token must be set via set_current_user() before get_current_user() is called.

    Note: Full implementation deferred to Phase 6.
    """

    def __init__(self, jwt_secret: str):
        """Initialize with JWT secret for token validation.

        Args:
            jwt_secret: Secret key for JWT signature validation
        """
        self.jwt_secret = jwt_secret
        self._current_user: AuthenticatedUser | None = None

    def validate_token(self, token: str) -> AuthenticatedUser:
        """Validate JWT and return user.

        Args:
            token: JWT access token

        Returns:
            AuthenticatedUser extracted from token

        Raises:
            jwt.InvalidTokenError: If token is invalid or expired
        """
        # Deferred to Phase 6
        import jwt
        payload = jwt.decode(token, self.jwt_secret, algorithms=["HS256"])
        return AuthenticatedUser(
            user_id=int(payload["sub"]),
            email=payload["email"]
        )

    def set_current_user(self, user: AuthenticatedUser):
        """Set the current user for this request.

        Args:
            user: Authenticated user from token validation
        """
        self._current_user = user

    def get_current_user(self) -> AuthenticatedUser:
        """Get the authenticated user.

        Raises:
            ValueError: If no user has been set
        """
        if not self._current_user:
            raise ValueError("No authenticated user - call set_current_user() first")
        return self._current_user
