"""MCP Server entry point.

Run with: python -m mcp_server (from src directory)

Requires environment variables:
- DATABASE_URL: PostgreSQL connection string
- TIMESHEET_USER_EMAIL: Email of the user to authenticate as
"""

import logging
import os
import sys

# Add src directory to path for imports when running as module
_src_dir = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if _src_dir not in sys.path:
    sys.path.insert(0, _src_dir)

from db import init_db
from mcp_server.auth import EnvAuthProvider
from mcp_server.server import create_server

# Configure logging to stderr (stdout is used for MCP protocol)
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
    stream=sys.stderr
)
logger = logging.getLogger(__name__)


def main():
    """Main entry point for MCP server."""
    # Get database URL from environment
    database_url = os.environ.get("DATABASE_URL")
    if not database_url:
        logger.error("DATABASE_URL environment variable not set")
        sys.exit(1)

    # Initialize database
    logger.info("Initializing database connection...")
    try:
        db = init_db(database_url)
    except Exception as e:
        logger.error(f"Failed to connect to database: {e}")
        sys.exit(1)

    # Initialize auth provider
    logger.info("Initializing authentication...")
    try:
        auth = EnvAuthProvider(db)
        logger.info(f"Authenticated as: {auth.get_current_user().email}")
    except ValueError as e:
        logger.error(f"Authentication failed: {e}")
        sys.exit(1)

    # Create MCP server
    logger.info("Starting MCP server...")
    mcp = create_server(db, auth)

    # Run with stdio transport
    logger.info("MCP server running on stdio")
    mcp.run(transport="stdio")


if __name__ == "__main__":
    main()
