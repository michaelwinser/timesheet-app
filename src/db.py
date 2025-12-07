"""SQLite database wrapper.

Thin wrapper around sqlite3 providing:
- Connection management
- Simple query interface
- Row factory for dict-like access
"""

import sqlite3
from contextlib import contextmanager
from pathlib import Path
from typing import Any


class Database:
    """SQLite database wrapper."""

    def __init__(self, db_path: Path):
        self.db_path = db_path

    @contextmanager
    def connection(self):
        """Context manager for database connections."""
        conn = sqlite3.connect(self.db_path)
        conn.row_factory = sqlite3.Row  # Access columns by name
        conn.execute("PRAGMA foreign_keys = ON")
        try:
            yield conn
            conn.commit()
        except Exception:
            conn.rollback()
            raise
        finally:
            conn.close()

    def execute(self, sql: str, params: tuple = ()) -> list[sqlite3.Row]:
        """Execute a query and return all rows."""
        with self.connection() as conn:
            cursor = conn.execute(sql, params)
            return cursor.fetchall()

    def execute_one(self, sql: str, params: tuple = ()) -> sqlite3.Row | None:
        """Execute a query and return first row or None."""
        with self.connection() as conn:
            cursor = conn.execute(sql, params)
            return cursor.fetchone()

    def execute_insert(self, sql: str, params: tuple = ()) -> int:
        """Execute an insert and return the last row id."""
        with self.connection() as conn:
            cursor = conn.execute(sql, params)
            return cursor.lastrowid

    def execute_many(self, sql: str, params_list: list[tuple]) -> int:
        """Execute a query with multiple parameter sets."""
        with self.connection() as conn:
            cursor = conn.executemany(sql, params_list)
            return cursor.rowcount

    def execute_script(self, sql: str) -> None:
        """Execute a multi-statement SQL script."""
        with self.connection() as conn:
            conn.executescript(sql)

    def row_to_dict(self, row: sqlite3.Row | None) -> dict[str, Any] | None:
        """Convert a Row to a dict, or return None."""
        if row is None:
            return None
        return dict(row)

    def rows_to_dicts(self, rows: list[sqlite3.Row]) -> list[dict[str, Any]]:
        """Convert a list of Rows to a list of dicts."""
        return [dict(row) for row in rows]


# Database instance - initialized in main.py
db: Database | None = None


def init_db(db_path: Path) -> Database:
    """Initialize the database instance."""
    global db
    db = Database(db_path)
    return db


def get_db() -> Database:
    """Get the database instance. Raises if not initialized."""
    if db is None:
        raise RuntimeError("Database not initialized. Call init_db() first.")
    return db
