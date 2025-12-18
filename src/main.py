"""FastAPI application entry point."""

import logging
from pathlib import Path

from fastapi import FastAPI, Request
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates
from fastapi.responses import RedirectResponse
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.middleware.sessions import SessionMiddleware

from config import config
from db import init_db, get_db
from migrations import run_migrations

# Configure logging
logging.basicConfig(
    level=config.LOG_LEVEL,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger(__name__)

# Initialize FastAPI app
app = FastAPI(
    title="Timesheet App",
    description="Automatic timesheet creation from Google Calendar",
    version="0.2.0",  # Bumped for PostgreSQL multi-user support
)


# User Context Middleware - injects user_id from session into request state
class UserContextMiddleware(BaseHTTPMiddleware):
    """Middleware to inject user_id into request state from session."""

    async def dispatch(self, request: Request, call_next):
        # Get user email from session (set by AuthMiddleware after login)
        user_email = request.session.get("user_email")

        if user_email:
            # Look up user_id from database
            db = get_db()
            user = db.execute_one(
                "SELECT id, email, name FROM users WHERE email = %s",
                (user_email,)
            )
            if user:
                # Store user info in request state for use in routes
                request.state.user_id = user['id']
                request.state.user_email = user['email']
                request.state.user_name = user.get('name')
            else:
                # User record doesn't exist - this shouldn't happen
                # but could occur if user deleted after login
                logger.warning(f"User email in session but not in DB: {user_email}")
                request.state.user_id = None
        else:
            request.state.user_id = None

        return await call_next(request)


# Authentication middleware - redirects unauthenticated users to login
class AuthMiddleware(BaseHTTPMiddleware):
    """Middleware to enforce authentication on protected routes."""

    # Routes that don't require authentication
    PUBLIC_ROUTES = {"/login", "/health"}
    PUBLIC_PREFIXES = ("/auth/", "/static/", "/docs", "/redoc", "/openapi.json")

    async def dispatch(self, request: Request, call_next):
        # Check if route is public
        if request.url.path in self.PUBLIC_ROUTES:
            return await call_next(request)

        if any(request.url.path.startswith(prefix) for prefix in self.PUBLIC_PREFIXES):
            return await call_next(request)

        # Check if user is authenticated (has email in session)
        user_email = request.session.get("user_email")
        if not user_email:
            # Redirect to login with next parameter
            next_url = str(request.url.path)
            if request.url.query:
                next_url += f"?{request.url.query}"
            return RedirectResponse(url=f"/login?next={next_url}")

        # User is authenticated, proceed
        return await call_next(request)


# Add middlewares in reverse order (last added runs first)
app.add_middleware(UserContextMiddleware)  # Runs second (after session, before routes)
app.add_middleware(AuthMiddleware)  # Runs third (after session, after user context)

# Add session middleware for user authentication
# Must be added LAST so it runs FIRST (middleware runs in reverse order)
app.add_middleware(
    SessionMiddleware,
    secret_key=config.SECRET_KEY,
    max_age=24 * 60 * 60,  # 24 hours (1 day)
    https_only=config.is_production,  # Require HTTPS in production
)

# Initialize database
logger.info(f"Connecting to database: {config.DATABASE_URL.split('@')[1]}")  # Hide password
db = init_db(config.DATABASE_URL)

# Run migrations on startup (only once, not per-request!)
migrations_dir = Path(__file__).parent.parent / "migrations"
if migrations_dir.exists():
    logger.info("Running database migrations...")
    try:
        stats = run_migrations(db, migrations_dir)
        logger.info(f"Migrations complete: {stats}")
    except Exception as e:
        logger.error(f"Migration failed: {e}")
        raise
else:
    logger.warning(f"Migrations directory not found: {migrations_dir}")

# Mount static files
static_dir = Path(__file__).parent / "static"
app.mount("/static", StaticFiles(directory=static_dir), name="static")

# Templates
templates_dir = Path(__file__).parent / "templates"
templates = Jinja2Templates(directory=templates_dir)

# Import and include routers
from routes.auth import router as auth_router
from routes.api import router as api_router
from routes.ui import router as ui_router

app.include_router(auth_router, prefix="/auth", tags=["auth"])
app.include_router(api_router, prefix="/api", tags=["api"])
app.include_router(ui_router, tags=["ui"])


@app.get("/health")
async def health_check():
    """Health check endpoint for container orchestration.

    Returns:
        200 OK if healthy (database is accessible)
        503 Service Unavailable if unhealthy
    """
    from datetime import datetime
    from fastapi import HTTPException

    try:
        # Check database connectivity
        db.execute("SELECT 1")

        return {
            "status": "healthy",
            "timestamp": datetime.utcnow().isoformat(),
            "database": "connected",
            "version": app.version
        }
    except Exception as e:
        raise HTTPException(
            status_code=503,
            detail=f"Unhealthy: {str(e)}"
        )


@app.on_event("shutdown")
def shutdown_event():
    """Close database connections on shutdown."""
    logger.info("Shutting down, closing database connections...")
    db.close()
