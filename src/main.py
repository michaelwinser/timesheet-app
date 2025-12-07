"""FastAPI application entry point."""

from pathlib import Path

from fastapi import FastAPI
from fastapi.staticfiles import StaticFiles
from fastapi.templating import Jinja2Templates

from config import config
from db import init_db

# Initialize FastAPI app
app = FastAPI(
    title="Timesheet App",
    description="Automatic timesheet creation from Google Calendar",
    version="0.1.0",
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
    """Health check endpoint."""
    return {"status": "ok"}
