"""Database migration system with tracking.

Runs SQL migration files in order and tracks which have been applied.
"""

import logging
from pathlib import Path

from db import Database

logger = logging.getLogger(__name__)


def init_migration_table(db: Database) -> None:
    """Create the migrations tracking table if it doesn't exist.

    Args:
        db: Database instance
    """
    db.execute_script("""
        CREATE TABLE IF NOT EXISTS applied_migrations (
            id SERIAL PRIMARY KEY,
            filename VARCHAR(255) NOT NULL UNIQUE,
            applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    """)
    logger.debug("Migration tracking table initialized")


def get_applied_migrations(db: Database) -> set[str]:
    """Get set of already-applied migration filenames.

    Args:
        db: Database instance

    Returns:
        Set of migration filenames that have been applied
    """
    try:
        rows = db.execute("SELECT filename FROM applied_migrations")
        return {row['filename'] for row in rows}
    except Exception as e:
        # Table doesn't exist yet (first run)
        logger.debug(f"Could not read applied_migrations: {e}")
        return set()


def mark_migration_applied(db: Database, filename: str) -> None:
    """Mark a migration as applied.

    Args:
        db: Database instance
        filename: Migration filename to mark as applied
    """
    db.execute(
        "INSERT INTO applied_migrations (filename) VALUES (%s)",
        (filename,)
    )
    logger.info(f"Marked migration as applied: {filename}")


def run_migrations(db: Database, migrations_dir: Path) -> dict[str, int]:
    """Run all pending migrations.

    Migrations are SQL files in the migrations_dir that haven't been applied yet.
    They run in alphabetical order (001_xxx.sql, 002_xxx.sql, etc.).

    Args:
        db: Database instance
        migrations_dir: Directory containing migration .sql files

    Returns:
        Dict with 'applied', 'skipped', and 'errors' counts

    Raises:
        RuntimeError: If a migration fails (stops processing)
    """
    # Ensure migration table exists
    init_migration_table(db)

    # Get already-applied migrations
    applied = get_applied_migrations(db)

    # Get all migration files
    migration_files = sorted(migrations_dir.glob("*.sql"))

    if not migration_files:
        logger.info(f"No migration files found in {migrations_dir}")
        return {"applied": 0, "skipped": 0, "errors": 0}

    stats = {"applied": 0, "skipped": 0, "errors": 0}

    for migration_file in migration_files:
        filename = migration_file.name

        if filename in applied:
            logger.debug(f"Skipping already-applied migration: {filename}")
            stats["skipped"] += 1
            continue

        logger.info(f"Applying migration: {filename}")
        try:
            with open(migration_file) as f:
                sql = f.read()

            # Execute migration
            db.execute_script(sql)

            # Mark as applied
            mark_migration_applied(db, filename)

            stats["applied"] += 1
            logger.info(f"Successfully applied: {filename}")

        except Exception as e:
            logger.error(f"Failed to apply migration {filename}: {e}")
            stats["errors"] += 1
            # Stop on first error - don't continue with dependent migrations
            raise RuntimeError(f"Migration failed: {filename}") from e

    logger.info(
        f"Migration summary: {stats['applied']} applied, "
        f"{stats['skipped']} skipped, {stats['errors']} errors"
    )

    return stats
