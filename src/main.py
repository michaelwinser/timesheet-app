"""FastAPI application entry point."""

from pathlib import Path

from fastapi import FastAPI, Request
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates
from fastapi.responses import RedirectResponse
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.middleware.sessions import SessionMiddleware

from config import config
from db import init_db

# Initialize FastAPI app
app = FastAPI(
    title="Timesheet App",
    description="Automatic timesheet creation from Google Calendar",
    version="0.1.0",
)

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


app.add_middleware(AuthMiddleware)

# Add session middleware for user authentication
# Must be added AFTER AuthMiddleware so it runs BEFORE (middleware runs in reverse order)
app.add_middleware(
    SessionMiddleware,
    secret_key=config.SECRET_KEY,
    max_age=24 * 60 * 60,  # 24 hours (1 day)
    https_only=config.is_production,  # Require HTTPS in production
)

# Initialize database
db = init_db(config.DATABASE_PATH)

# Run migrations on startup
migrations_dir = Path(__file__).parent.parent / "migrations"
if migrations_dir.exists():
    for migration_file in sorted(migrations_dir.glob("*.sql")):
        with open(migration_file) as f:
            db.execute_script(f.read())

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
