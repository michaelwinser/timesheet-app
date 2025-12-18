"""PostgreSQL database wrapper with connection pooling.

Thin wrapper around psycopg2 providing:
- Connection pool management
- Simple query interface
- Dict-like row access via RealDictCursor
"""

import logging
from contextlib import contextmanager
from typing import Any

import psycopg2
from psycopg2 import pool
from psycopg2.extras import RealDictCursor

logger = logging.getLogger(__name__)


class Database:
    """PostgreSQL database wrapper with connection pooling."""

    def __init__(self, database_url: str, min_conn: int = 1, max_conn: int = 10):
        """Initialize connection pool.

        Args:
            database_url: PostgreSQL connection string (postgresql://user:pass@host:port/db)
            min_conn: Minimum number of connections in pool
            max_conn: Maximum number of connections in pool
        """
        self.database_url = database_url
        try:
            self.pool = psycopg2.pool.ThreadedConnectionPool(
                min_conn,
                max_conn,
                database_url,
                cursor_factory=RealDictCursor  # Return dicts instead of tuples
            )
            logger.info(f"PostgreSQL connection pool created (min={min_conn}, max={max_conn})")
        except Exception as e:
            logger.error(f"Failed to create connection pool: {e}")
            raise

    @contextmanager
    def connection(self):
        """Context manager for database connections from pool."""
        conn = self.pool.getconn()
        try:
            yield conn
            conn.commit()
        except Exception:
            conn.rollback()
            raise
        finally:
            self.pool.putconn(conn)

    def execute(self, sql: str, params: tuple = ()) -> list[dict]:
        """Execute a query and return all rows as dicts.

        Args:
            sql: SQL query with %s placeholders
            params: Tuple of parameters

        Returns:
            List of dict rows
        """
        with self.connection() as conn:
            with conn.cursor() as cursor:
                cursor.execute(sql, params)
                # Handle queries that don't return rows (INSERT, UPDATE, DELETE)
                if cursor.description is None:
                    return []
                return cursor.fetchall()

    def execute_one(self, sql: str, params: tuple = ()) -> dict | None:
        """Execute a query and return first row or None.

        Args:
            sql: SQL query with %s placeholders
            params: Tuple of parameters

        Returns:
            Dict row or None
        """
        with self.connection() as conn:
            with conn.cursor() as cursor:
                cursor.execute(sql, params)
                if cursor.description is None:
                    return None
                return cursor.fetchone()

    def execute_insert(self, sql: str, params: tuple = ()) -> int:
        """Execute an insert and return the inserted row id.

        IMPORTANT: SQL must include RETURNING id clause for PostgreSQL.

        Args:
            sql: INSERT query with RETURNING id clause
            params: Tuple of parameters

        Returns:
            The inserted row's id, or number of affected rows if no RETURNING clause

        Example:
            db.execute_insert(
                "INSERT INTO users (email, name) VALUES (%s, %s) RETURNING id",
                ("user@example.com", "User Name")
            )
        """
        with self.connection() as conn:
            with conn.cursor() as cursor:
                cursor.execute(sql, params)
                # If query has RETURNING id, fetch it
                if cursor.description and 'id' in [desc[0] for desc in cursor.description]:
                    result = cursor.fetchone()
                    return result['id'] if result else 0
                # Otherwise return affected row count
                return cursor.rowcount

    def execute_many(self, sql: str, params_list: list[tuple]) -> int:
        """Execute a query with multiple parameter sets.

        Args:
            sql: SQL query with %s placeholders
            params_list: List of parameter tuples

        Returns:
            Number of rows affected
        """
        with self.connection() as conn:
            with conn.cursor() as cursor:
                cursor.executemany(sql, params_list)
                return cursor.rowcount

    def execute_script(self, sql: str) -> None:
        """Execute a multi-statement SQL script.

        Args:
            sql: SQL script (multiple statements separated by semicolons)
        """
        with self.connection() as conn:
            with conn.cursor() as cursor:
                cursor.execute(sql)

    def row_to_dict(self, row: dict | None) -> dict[str, Any] | None:
        """Convert a Row to a dict, or return None.

        Note: With RealDictCursor, rows are already dicts.
        This method kept for API compatibility with SQLite version.

        Args:
            row: Dict row from database

        Returns:
            The same dict or None
        """
        return row

    def rows_to_dicts(self, rows: list[dict]) -> list[dict[str, Any]]:
        """Convert a list of Rows to a list of dicts.

        Note: With RealDictCursor, rows are already dicts.
        This method kept for API compatibility with SQLite version.

        Args:
            rows: List of dict rows from database

        Returns:
            The same list of dicts
        """
        return rows

    def close(self):
        """Close all connections in the pool."""
        if self.pool:
            self.pool.closeall()
            logger.info("PostgreSQL connection pool closed")


# Database instance - initialized in main.py
db: Database | None = None


def init_db(database_url: str) -> Database:
    """Initialize the database instance.

    Args:
        database_url: PostgreSQL connection string

    Returns:
        Database instance

    Example:
        db = init_db("postgresql://user:pass@localhost:5432/timesheet")
    """
    global db
    db = Database(database_url)
    return db


def get_db() -> Database:
    """Get the database instance. Raises if not initialized.

    Returns:
        Database instance

    Raises:
        RuntimeError: If database not initialized
    """
    if db is None:
        raise RuntimeError("Database not initialized. Call init_db() first.")
    return db
