# MCP Server Implementation Guide

This document describes how the MCP (Model Context Protocol) server is implemented in this project. It serves as a reference for implementing MCP in other Go-based applications.

## Overview

The MCP server exposes application functionality to AI assistants (Claude, etc.) via a standardized protocol. This implementation uses:

- **Transport**: HTTP-based Streamable HTTP (not stdio)
- **Protocol Version**: 2024-11-05
- **Language**: Go
- **Pattern**: Tool definitions generated from OpenAPI spec

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      MCP Handler                            │
│  service/internal/handler/mcp.go                            │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   Auth      │  │  JSON-RPC   │  │   Tool Dispatch     │  │
│  │  Middleware │──│   Router    │──│   (callTool)        │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│                                              │              │
│                         ┌────────────────────┼──────────┐   │
│                         ▼                    ▼          ▼   │
│                   ┌──────────┐        ┌──────────┐ ┌──────┐ │
│                   │  Stores  │        │ Services │ │  ... │ │
│                   └──────────┘        └──────────┘ └──────┘ │
└─────────────────────────────────────────────────────────────┘
                              │
              ┌───────────────┴───────────────┐
              ▼                               ▼
┌─────────────────────────┐    ┌─────────────────────────────┐
│  Generated Tool Defs    │    │   OpenAPI Spec              │
│  service/internal/mcp/  │◄───│   docs/v2/api-spec.yaml     │
│  tools.gen.go           │    │                             │
└─────────────────────────┘    └─────────────────────────────┘
```

## File Structure

```
service/
├── internal/
│   ├── handler/
│   │   ├── mcp.go           # Main MCP handler (HTTP, JSON-RPC, tool dispatch)
│   │   └── mcp_oauth.go     # OAuth endpoints for MCP authentication
│   ├── mcp/
│   │   └── tools.gen.go     # Generated tool definitions from OpenAPI
│   └── store/
│       └── mcp_oauth.go     # Token storage for MCP OAuth flow
├── cmd/
│   └── mcp-codegen/
│       └── main.go          # Code generator: OpenAPI → MCP tool definitions
docs/v2/
└── api-spec.yaml            # OpenAPI spec (source of truth for tools)
```

## Implementation Steps

### 1. Define Tools in OpenAPI Spec

Add an `x-mcp` extension to operations you want to expose as MCP tools:

```yaml
paths:
  /api/projects:
    get:
      operationId: listProjects
      x-mcp:
        tool: list_projects
        description: "List all projects. Use this first to understand available options."
      parameters:
        - name: include_archived
          in: query
          schema:
            type: boolean
            default: false
```

### 2. Generate Tool Definitions

Create a code generator that reads the OpenAPI spec and outputs Go code:

```go
// service/cmd/mcp-codegen/main.go
// Reads api-spec.yaml, finds x-mcp-tool extensions, generates tools.gen.go
```

The generated code provides:
- `GetTools() []Tool` - Returns all tool definitions with JSON schemas
- `GetResources() []Resource` - Returns MCP resources
- `GetServerInfo() ServerInfo` - Returns server metadata and instructions

Run generation:
```bash
make generate  # Runs mcp-codegen as part of code generation
```

### 3. Implement the MCP Handler

The handler struct holds dependencies and implements `http.Handler`:

```go
type MCPHandler struct {
    // Stores for data access
    projects       *store.ProjectStore
    entries        *store.TimeEntryStore
    // ... other stores

    // Services for business logic
    classificationSvc *classification.Service

    // Auth
    apiKeys  *store.APIKeyStore
    mcpOAuth *store.MCPOAuthStore
    jwt      *JWTService

    // Config
    baseURL   string
    tools     []mcpTool
    resources []mcpResource
}
```

### 4. Handle HTTP Methods

```go
func (h *MCPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Authenticate (see Authentication section)
    userID, ok := h.authenticate(r)
    if !ok {
        h.sendOAuthChallenge(w)
        return
    }

    switch r.Method {
    case "GET":
        h.handleSSE(w, r)      // Server-Sent Events for streaming
    case "POST":
        h.handleJSONRPC(w, r, userID)  // JSON-RPC messages
    case "OPTIONS":
        w.Header().Set("Allow", "GET, POST, OPTIONS")
        w.WriteHeader(http.StatusNoContent)
    default:
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
    }
}
```

### 5. Implement JSON-RPC Router

Handle standard MCP methods:

```go
func (h *MCPHandler) handleJSONRPC(w http.ResponseWriter, r *http.Request, userID uuid.UUID) {
    var req struct {
        JSONRPC string          `json:"jsonrpc"`
        ID      any             `json:"id"`
        Method  string          `json:"method"`
        Params  json.RawMessage `json:"params,omitempty"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    switch req.Method {
    case "initialize":
        // Return protocol version, capabilities, server info

    case "tools/list":
        // Return h.tools

    case "tools/call":
        // Parse tool name and arguments, dispatch to callTool()

    case "resources/list":
        // Return h.resources

    case "resources/read":
        // Return resource content by URI
    }
}
```

### 6. Implement Tool Dispatch

Map tool names to handler methods:

```go
func (h *MCPHandler) callTool(ctx context.Context, userID uuid.UUID, name string, args map[string]any) (any, error) {
    switch name {
    case "list_projects":
        return h.listProjects(ctx, userID, args)
    case "search_events":
        return h.searchEvents(ctx, userID, args)
    case "create_rule":
        return h.createRule(ctx, userID, args)
    // ... other tools
    default:
        return nil, fmt.Errorf("unknown tool: %s", name)
    }
}
```

### 7. Implement Tool Handlers

Each tool returns markdown-formatted content for AI consumption:

```go
func (h *MCPHandler) listProjects(ctx context.Context, userID uuid.UUID, args map[string]any) (any, error) {
    includeArchived := false
    if v, ok := args["include_archived"].(bool); ok {
        includeArchived = v
    }

    projects, err := h.projects.List(ctx, userID, includeArchived)
    if err != nil {
        return nil, fmt.Errorf("failed to list projects: %w", err)
    }

    // Format as markdown for AI readability
    var sb strings.Builder
    sb.WriteString("# Projects\n\n")
    for _, p := range projects {
        sb.WriteString(fmt.Sprintf("- **%s**\n", p.Name))
        sb.WriteString(fmt.Sprintf("  - ID: `%s`\n", p.ID))
    }

    // Return MCP content format
    return map[string]any{
        "content": []map[string]any{
            {"type": "text", "text": sb.String()},
        },
    }, nil
}
```

## Authentication

Support multiple authentication methods:

```go
func (h *MCPHandler) authenticate(r *http.Request) (uuid.UUID, bool) {
    // 1. Check context (set by auth middleware for web requests)
    if userID, ok := UserIDFromContext(r.Context()); ok {
        return userID, true
    }

    // 2. Check Bearer token
    authHeader := r.Header.Get("Authorization")
    if !strings.HasPrefix(authHeader, "Bearer ") {
        return uuid.Nil, false
    }
    token := strings.TrimPrefix(authHeader, "Bearer ")

    // 3. MCP OAuth token (mcp_ prefix)
    if strings.HasPrefix(token, "mcp_") {
        if uid, err := h.mcpOAuth.ValidateToken(r.Context(), token); err == nil {
            return uid, true
        }
    }

    // 4. API key (ts_ prefix)
    if strings.HasPrefix(token, "ts_") {
        if uid, err := h.apiKeys.ValidateAndGetUserID(r.Context(), token); err == nil {
            return uid, true
        }
    }

    return uuid.Nil, false
}
```

### OAuth Challenge Response

When unauthenticated, return proper OAuth challenge:

```go
func (h *MCPHandler) sendOAuthChallenge(w http.ResponseWriter) {
    resourceMetadata := fmt.Sprintf("%s/.well-known/oauth-protected-resource", h.baseURL)
    w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s"`, resourceMetadata))
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusUnauthorized)
    json.NewEncoder(w).Encode(map[string]string{
        "error":             "unauthorized",
        "error_description": "Authentication required.",
    })
}
```

## Server Info and Instructions

Provide instructions to guide AI behavior:

```go
func GetServerInfo() ServerInfo {
    return ServerInfo{
        Name:    "timesheet",
        Version: "1.0.0",
        Instructions: `You are an AI assistant helping manage a timesheet application.

IMPORTANT: Before using search_events, create_rule, or preview_rule tools,
first read the timesheet://docs/query-syntax resource to understand the query language.

When helping the user:
1. Read timesheet://docs/query-syntax to learn the search syntax
2. List projects to understand available classification targets
3. Search for pending events to see what needs attention
4. Use preview_rule to test classification patterns
5. Create rules or use bulk_classify to classify events
`,
    }
}
```

## Resources

Expose documentation as MCP resources:

```go
func GetResources() []Resource {
    return []Resource{
        {
            URI:         "timesheet://docs/query-syntax",
            Name:        "Query Syntax Reference",
            Description: "Reference for Gmail-style query syntax",
            MimeType:    "text/markdown",
        },
    }
}
```

Handle resource reads:

```go
case "resources/read":
    switch params.URI {
    case "timesheet://docs/query-syntax":
        result = map[string]any{
            "contents": []map[string]any{
                {
                    "uri":      params.URI,
                    "mimeType": "text/markdown",
                    "text":     h.getQuerySyntaxDoc(),
                },
            },
        }
    }
```

## Best Practices

### Tool Design

1. **Return markdown** - Format output for AI readability with headers, lists, code blocks
2. **Include IDs** - Always return entity IDs so the AI can reference them in follow-up calls
3. **Provide context** - Include related information (e.g., project name alongside project ID)
4. **Limit output** - Paginate or limit results to avoid overwhelming context windows

### Error Handling

Return structured JSON-RPC errors:

```go
func (h *MCPHandler) sendJSONRPCError(w http.ResponseWriter, id any, code int, message, data string) {
    json.NewEncoder(w).Encode(map[string]any{
        "jsonrpc": "2.0",
        "id":      id,
        "error": map[string]any{
            "code":    code,
            "message": message,
            "data":    data,
        },
    })
}
```

Standard error codes:
- `-32700` - Parse error
- `-32601` - Method not found
- `-32602` - Invalid params
- `-32000` - Tool execution error

### Security

1. **Always filter by user** - Every query must include `userID` from authentication
2. **Validate inputs** - Check required parameters, parse UUIDs safely
3. **Use HTTPS** - MCP OAuth requires secure transport in production

## Testing

Test MCP endpoints with curl:

```bash
# List tools
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer ts_your_api_key" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'

# Call a tool
curl -X POST http://localhost:8080/mcp \
  -H "Authorization: Bearer ts_your_api_key" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_projects","arguments":{}}}'
```

## Reusable Patterns for Other Projects

1. **OpenAPI `x-mcp` extensions** - Define tools alongside REST endpoints in a single spec
2. **Code generation** - Build a generator with `kin-openapi` to extract extensions and produce tool definitions
3. **Markdown formatting** - Use structured markdown (headers, backtick-quoted IDs) for AI-friendly output
4. **Token prefix convention** - Use prefixes like `mcp_`, `ts_` to distinguish token types
5. **Resource-based documentation** - Expose syntax guides as queryable resources (`timesheet://docs/query-syntax`)
6. **Shared data layer** - Pass same stores/services to both REST and MCP handlers

## References

- [MCP Specification](https://modelcontextprotocol.io/)
- [MCP TypeScript SDK](https://github.com/modelcontextprotocol/typescript-sdk)
- [Streamable HTTP Transport](https://modelcontextprotocol.io/docs/concepts/transports#streamable-http)
