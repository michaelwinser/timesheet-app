# MCP Server Usage Guide

The timesheet application includes a built-in MCP (Model Context Protocol) server that enables AI assistants like Claude to analyze and manage your timesheet data.

## Quick Start

### Using with Claude Code

1. Ensure the timesheet server is running
2. From the project directory, run Claude Code:
   ```bash
   claude
   ```
3. When Claude connects to the MCP server, you'll be prompted to authenticate
4. Log in with your timesheet credentials in the browser window that opens
5. Once authenticated, Claude can access your timesheet data

The project includes a `.mcp.json` file that configures Claude Code automatically.

### Authentication Flow

The MCP server uses OAuth 2.1 for authentication:

1. Claude Code connects to the MCP endpoint
2. Server returns an authentication challenge
3. Claude Code opens a browser for you to log in
4. You authenticate with your timesheet credentials
5. Claude receives an access token valid for 24 hours

No API keys or manual configuration required.

## Available Tools

| Tool | Description |
|------|-------------|
| `list_projects` | List all projects (active and optionally archived) |
| `get_time_summary` | Aggregate time entries by project or date |
| `list_pending_events` | Show calendar events awaiting classification |
| `classify_event` | Assign an event to a project or skip it |
| `create_time_entry` | Log time manually |

## Example Interactions

### View Time Summary

> "How did I spend my time last week?"

Claude will call `get_time_summary` and show you a breakdown by project.

### Classify Events

> "Show me pending calendar events and help me classify them"

Claude will:
1. Call `list_projects` to understand available options
2. Call `list_pending_events` to see unclassified events
3. Ask which events should go to which projects
4. Call `classify_event` for each one

### Manual Time Entry

> "Log 2 hours on the Acme project for today"

Claude will:
1. Call `list_projects` to find the Acme project ID
2. Call `create_time_entry` with the project, date, and hours

## Remote Access

For accessing the MCP server remotely:

1. Set the timesheet URL:
   ```bash
   export TIMESHEET_URL=https://timesheet.example.com
   ```

2. Run Claude Code - authentication will work the same way via browser

## Alternative: API Key Authentication

If you prefer not to use OAuth (e.g., for automation), you can use API keys:

1. Go to **Settings** > **API Keys** in the timesheet app
2. Create a new API key
3. Configure Claude Code with the key:
   ```bash
   export TIMESHEET_API_KEY=ts_your_key_here
   ```

Update `.mcp.json` to include the Authorization header:
```json
{
  "mcpServers": {
    "timesheet": {
      "type": "http",
      "url": "http://localhost:8080/mcp",
      "headers": {
        "Authorization": "Bearer ${TIMESHEET_API_KEY}"
      }
    }
  }
}
```

## Security Notes

- **OAuth tokens expire in 24 hours**: You'll need to re-authenticate periodically
- **API keys don't expire**: They act as you until revoked
- **Revoke compromised keys**: Delete from Settings if a key is exposed
- **Use HTTPS for remote**: Always use HTTPS when accessing over the network

## MCP Protocol Details

The endpoint implements OAuth 2.1 with PKCE:

- **GET /.well-known/oauth-authorization-server**: OAuth metadata
- **GET /.well-known/oauth-protected-resource**: Resource metadata
- **GET /mcp/authorize**: Start OAuth flow
- **POST /mcp/token**: Exchange auth code for token
- **POST /mcp**: JSON-RPC requests
- **GET /mcp**: Server-Sent Events

### Testing with curl

```bash
# Get OAuth metadata
curl http://localhost:8080/.well-known/oauth-authorization-server

# List tools (requires authentication)
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer mcp_your_token" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/list"
  }'
```

## Troubleshooting

### "Authentication required" error

The OAuth flow hasn't completed. Check that:
- The browser window opened for authentication
- You successfully logged in
- The redirect completed

### Browser doesn't open

If the browser window doesn't open automatically, copy the URL from the terminal and open it manually.

### Token expired

After 24 hours, you'll need to re-authenticate. Claude Code will prompt you automatically.

### Connection refused

The timesheet server isn't running. Start it with:
```bash
cd service
go run ./cmd/server
```
