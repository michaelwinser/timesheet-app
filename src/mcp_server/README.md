# Timesheet MCP Server

An MCP (Model Context Protocol) server that exposes timesheet functionality to AI assistants like Claude Desktop.

## Features

- **list_projects**: List all projects with their settings
- **get_time_entries**: Get time entries with full event details for a date range
- **get_timesheet_summary**: Get a summary of hours by project, day, or week

## Requirements

- Docker with the timesheet-app container running, OR
- Python 3.11+ with PostgreSQL database

## Claude Desktop Configuration

Add the following to your Claude Desktop configuration file:

**macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`
**Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

### Option 1: Docker (Recommended)

Use `docker exec` to run the MCP server inside the already-running container:

```json
{
  "mcpServers": {
    "timesheet": {
      "command": "docker",
      "args": [
        "exec", "-i",
        "-e", "TIMESHEET_USER_EMAIL=your-email@example.com",
        "timesheet-app",
        "python", "-m", "mcp_server"
      ]
    }
  }
}
```

This approach:
- Uses the existing Docker container (must be running)
- Shares the same database connection as the web app
- No additional configuration needed for DATABASE_URL

### Option 2: Local Python

Run directly with a local Python environment:

```json
{
  "mcpServers": {
    "timesheet": {
      "command": "/path/to/timesheet-app/venv/bin/python",
      "args": ["-m", "mcp_server"],
      "cwd": "/path/to/timesheet-app/src",
      "env": {
        "DATABASE_URL": "postgresql://user:password@localhost:5432/timesheet",
        "TIMESHEET_USER_EMAIL": "your-email@example.com"
      }
    }
  }
}
```

### Configuration Notes

- **TIMESHEET_USER_EMAIL**: Your email address (must exist in the database)
- **DATABASE_URL**: Only needed for Option 2 (container already has this)

## Running Manually

For testing, you can run the server directly:

```bash
# Via Docker (container must be running)
docker exec -i -e TIMESHEET_USER_EMAIL="..." timesheet-app python -m mcp_server

# Via local Python
cd /path/to/timesheet-app/src
DATABASE_URL="postgresql://..." TIMESHEET_USER_EMAIL="..." python -m mcp_server
```

## Example Prompts

Once configured, you can ask Claude:

- "Produce a timesheet report for last week"
- "What projects do I have?"
- "How many hours did I work last month?"
- "Show me a breakdown of my time by project for this week"

## Development

### Running with MCP Inspector

```bash
cd src
DATABASE_URL="..." TIMESHEET_USER_EMAIL="..." mcp dev mcp_server
```

### Testing

```bash
cd src
python -c "
from mcp_server.server import create_server
from mcp_server.tools import ALL_TOOLS
print([t.name for t in ALL_TOOLS])
"
```

## Architecture

```
src/mcp_server/
├── __init__.py       # Package initialization
├── __main__.py       # Entry point
├── auth.py           # Authentication providers
├── server.py         # FastMCP server setup
└── tools/
    ├── __init__.py   # Tool registration
    ├── base.py       # Base tool class
    ├── projects.py   # Project tools
    └── time_entries.py  # Time entry tools
```
