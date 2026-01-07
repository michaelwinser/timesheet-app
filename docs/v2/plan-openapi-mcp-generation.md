# Plan: Auto-Generate MCP Server from OpenAPI Spec

## Problem Statement

The current MCP handler (`service/internal/handler/mcp.go`) is manually maintained, duplicating:
- Tool definitions (~300 lines of schema maps)
- Tool handler dispatch (~30 lines)
- Handler implementations (~800 lines)

This manual approach creates maintenance burden and risks drift between the REST API and MCP interface. The issue proposes generating the MCP server directly from the OpenAPI spec.

## Current State

### Existing Code Generation
- **OpenAPI spec**: `docs/v2/api-spec.yaml` (2870 lines, comprehensive)
- **oapi-codegen**: Generates Go types and Chi HTTP handlers → `internal/api/api.gen.go`
- **MCP SDK**: Project uses `github.com/modelcontextprotocol/go-sdk v1.2.0`

### Current MCP Handler Structure
```
MCPHandler
├── initTools()        - Defines 12 tools with JSON schemas
├── initResources()    - Defines 1 resource (query-syntax docs)
├── callTool()         - Dispatch switch statement
├── listProjects()     - Handler implementations
├── getTimeSummary()   - (one per tool)
├── ...
└── ServeHTTP()        - JSON-RPC over HTTP handling
```

### Tool-to-API Mapping
| MCP Tool | REST Endpoint | Notes |
|----------|---------------|-------|
| `list_projects` | `GET /api/projects` | Same |
| `get_time_summary` | `GET /api/time-entries` | Aggregation |
| `list_pending_events` | `GET /api/calendar-events?status=pending` | Filtered |
| `classify_event` | `PUT /api/calendar-events/{id}/classify` | Same |
| `create_time_entry` | `POST /api/time-entries` | Same |
| `search_events` | `GET /api/calendar-events` + query | Custom search |
| `list_rules` | `GET /api/rules` | Same |
| `create_rule` | `POST /api/rules` | Same |
| `preview_rule` | `POST /api/rules/preview` | Same |
| `bulk_classify` | `POST /api/calendar-events/bulk-classify` | Same |
| `apply_rules` | `POST /api/rules/apply` | Same |
| `explain_classification` | N/A | MCP-only (debugging) |

## Research Findings

### jedisct1/openapi-mcp
The primary recommendation from the issue is **archived** (June 29, 2025). While the code remains available, there will be no updates or security fixes.

Key features it had:
- Runtime translation of OpenAPI to MCP
- Go module embedding option
- Support for API key, Bearer, Basic auth

### Alternatives Evaluated

| Option | Pros | Cons |
|--------|------|------|
| **jedisct1/openapi-mcp** | Feature-complete, Go native | Archived, no updates |
| **lyeslabs/mcpgen** | Code generation approach | Generates skeletons, still manual |
| **Custom generator** | Tailored to project needs | Development effort |
| **Extend oapi-codegen** | Reuse existing tooling | MCP not in scope for oapi-codegen |

### MCP Go SDK Capabilities
The official SDK (`modelcontextprotocol/go-sdk`) provides:
- Tool definition via struct tags
- Stdio transport (primary)
- OAuth primitives

It does **not** provide:
- OpenAPI-to-MCP conversion
- HTTP transport (we implemented custom)

## Recommended Approach

Given the constraints, we recommend a **hybrid approach**: build a lightweight code generator tailored to this project that leverages the existing OpenAPI spec and oapi-codegen types.

### Why Not Use openapi-mcp Directly?
1. **Archived**: No security updates or bug fixes
2. **Runtime translation**: Adds latency and complexity
3. **Generic**: Doesn't account for MCP-specific enhancements (resources, instructions)
4. **Naming mismatch**: Would expose `listProjects` vs desired `list_projects`

### Why Build Custom?
1. **Control**: MCP tools can have different semantics than REST endpoints
2. **Resources**: MCP has resources that don't map to REST
3. **Instructions**: MCP server instructions are a first-class concern
4. **Formatting**: Tool responses are formatted text, not JSON objects
5. **Naming**: MCP uses `snake_case`, REST uses `camelCase`

## Design

### Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    docs/v2/api-spec.yaml                         │
│                    (OpenAPI 3.0 source of truth)                │
└─────────────────────────────────────────────────────────────────┘
                              │
            ┌─────────────────┴─────────────────┐
            ▼                                   ▼
┌───────────────────────┐           ┌───────────────────────┐
│     oapi-codegen      │           │    mcp-codegen        │
│   (existing tooling)  │           │   (new, custom)       │
└───────────────────────┘           └───────────────────────┘
            │                                   │
            ▼                                   ▼
┌───────────────────────┐           ┌───────────────────────┐
│  internal/api/        │           │  internal/mcp/        │
│  api.gen.go           │           │  tools.gen.go         │
│  (types, HTTP routes) │           │  (tool definitions)   │
└───────────────────────┘           └───────────────────────┘
```

### MCP Spec Extension

Add MCP-specific metadata to the OpenAPI spec using `x-mcp` extensions:

```yaml
paths:
  /api/projects:
    get:
      operationId: listProjects
      x-mcp:
        tool: list_projects
        description: "List all projects. Use this first to understand available options."
        category: query
      # ... rest of OpenAPI definition
```

Operations without `x-mcp` would not be exposed as MCP tools.

### Generated Code Structure

```go
// internal/mcp/tools.gen.go
// Code generated from api-spec.yaml. DO NOT EDIT.

package mcp

import "github.com/michaelw/timesheet-app/service/internal/api"

// ToolDefinitions returns all MCP tool definitions
func ToolDefinitions() []Tool {
    return []Tool{
        {
            Name:        "list_projects",
            Description: "List all projects. Use this first...",
            InputSchema: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "include_archived": map[string]any{
                        "type":        "boolean",
                        "description": "Include archived/inactive projects",
                        "default":     false,
                    },
                },
            },
        },
        // ... other tools
    }
}

// ToolInputTypes maps tool names to their input struct types
var ToolInputTypes = map[string]reflect.Type{
    "list_projects": reflect.TypeOf(api.ListProjectsParams{}),
    // ...
}
```

### Handler Implementation Options

**Option A: Generated dispatch, manual handlers**
- Generate tool definitions and dispatch logic
- Keep handler implementations manual (they have custom formatting)
- Least disruptive, most flexible

**Option B: Fully generated with templates**
- Generate everything including handlers
- Use templates for response formatting
- More maintenance reduction but less flexibility

**Recommendation**: Option A for initial implementation. The response formatting in MCP is significantly different from REST (markdown text vs JSON objects), making full generation complex.

### Migration Path

1. **Phase 1**: Add `x-mcp` extensions to OpenAPI spec for existing tools
2. **Phase 2**: Build code generator for tool definitions
3. **Phase 3**: Generate dispatch logic, refactor handlers
4. **Phase 4**: Optionally generate handler skeletons for new tools

## Implementation Steps

### Step 1: Create MCP Extension Schema
Define the `x-mcp` extension format:

```yaml
x-mcp:
  tool: string          # MCP tool name (snake_case)
  description: string   # Override operationId description
  category: string      # query | mutation | admin
  exclude: boolean      # true to not expose as MCP tool
  resource: string      # If this operation provides a resource
```

### Step 2: Annotate OpenAPI Spec
Add `x-mcp` extensions to operations that should be MCP tools:

```yaml
/api/projects:
  get:
    operationId: listProjects
    x-mcp:
      tool: list_projects
      description: >-
        List all projects. Use this first to understand available
        options for classification.
```

### Step 3: Build Generator
Create `cmd/mcp-codegen/main.go`:

```go
// Reads api-spec.yaml
// Extracts operations with x-mcp extensions
// Generates internal/mcp/tools.gen.go
```

Generator responsibilities:
- Parse OpenAPI spec (use `kin-openapi` library)
- Extract `x-mcp` annotated operations
- Convert OpenAPI schemas to MCP input schemas
- Generate Go code with proper imports

### Step 4: Refactor MCPHandler
Split current handler:

```
internal/handler/mcp.go (current, 1700 lines)
                │
                ▼
┌───────────────┴───────────────┐
│                               │
▼                               ▼
internal/mcp/              internal/handler/
├── tools.gen.go           └── mcp.go
│   (generated)                (slimmed down)
├── resources.go               ├── ServeHTTP
│   (manual)                   ├── callTool dispatch
└── instructions.go            └── handler implementations
    (manual)
```

### Step 5: Update Build Process
Add to Makefile:

```makefile
generate: generate-api generate-mcp

generate-api:
	oapi-codegen -config oapi-codegen.yaml ../docs/v2/api-spec.yaml

generate-mcp:
	go run ./cmd/mcp-codegen ../docs/v2/api-spec.yaml
```

### Step 6: Add CI Check
Ensure generated code stays in sync:

```yaml
- name: Check generated code
  run: |
    make generate
    git diff --exit-code internal/mcp/tools.gen.go
```

## MCP-Only Features

Some MCP features don't map to REST endpoints:

### Resources
```yaml
# Add to api-spec.yaml
x-mcp-resources:
  - uri: "timesheet://docs/query-syntax"
    name: "Query Syntax Reference"
    description: "Complete reference for Gmail-style query syntax"
    mimeType: "text/markdown"
```

### Instructions
```yaml
# Add to api-spec.yaml
x-mcp-instructions: |
  You are an AI assistant helping manage a timesheet application.

  The user tracks their time across different projects...
```

### MCP-Only Tools
For tools like `explain_classification` that have no REST equivalent:

```yaml
x-mcp-tools:
  - name: explain_classification
    description: "Explain how an event was classified..."
    inputSchema:
      type: object
      required: [event_id]
      properties:
        event_id:
          type: string
          description: "The calendar event ID"
```

## Schema Translation

### OpenAPI to MCP Schema Mapping

| OpenAPI | MCP InputSchema |
|---------|-----------------|
| `type: string` | `type: string` |
| `type: integer` | `type: integer` |
| `type: number` | `type: number` |
| `type: boolean` | `type: boolean` |
| `type: array, items: {$ref}` | `type: array, items: {...}` |
| `format: uuid` | `type: string` (no format) |
| `format: date` | `type: string, description: "(YYYY-MM-DD)"` |
| `$ref: '#/components/schemas/X'` | Inline expansion |

### Parameter Source Mapping

| OpenAPI | MCP |
|---------|-----|
| `in: query` | Property in inputSchema |
| `in: path` | Property in inputSchema (required) |
| `requestBody` | Properties merged into inputSchema |

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Generator bugs | High | Comprehensive tests, manual review |
| Schema drift | Medium | CI check for generated code freshness |
| Loss of flexibility | Medium | Keep handlers manual, only generate definitions |
| Complex nested schemas | Low | Support common cases, manual fallback for edge cases |

## Success Criteria

1. **Tool definitions generated**: 12 tool schemas generated from OpenAPI spec
2. **No functionality loss**: All existing MCP features continue to work
3. **Single source of truth**: Changes to API spec automatically update MCP tools
4. **Build integration**: `make generate` produces both API and MCP code
5. **CI enforcement**: PRs fail if generated code is stale

## Alternatives Considered

### Alternative 1: Use openapi-mcp Despite Archive Status
**Rejected**: Security risk, no bug fixes, would require forking to maintain.

### Alternative 2: Pure Runtime Translation
**Rejected**: Adds latency, harder to debug, less control over tool semantics.

### Alternative 3: Manual Maintenance with Linting
**Rejected**: Doesn't solve the fundamental duplication problem.

### Alternative 4: Extend oapi-codegen
**Rejected**: Out of scope for oapi-codegen project, would require maintaining a fork.

## Timeline Estimate

| Phase | Effort |
|-------|--------|
| Step 1-2: Extension schema + annotations | Small |
| Step 3: Build generator | Medium |
| Step 4: Refactor handler | Medium |
| Step 5-6: Build + CI integration | Small |
| Testing and validation | Medium |

## Open Questions

1. **Response formatting**: Should the generator produce response formatters, or keep those manual?
2. **Error handling**: How should OpenAPI error responses map to MCP tool errors?
3. **Pagination**: REST endpoints may paginate; MCP tools typically don't. Handle in generator or handler?
4. **Authentication**: MCP OAuth is handled separately; should generator be aware of security schemes?

## References

- Issue #48: [Change MCP server to be automatically generated from the OpenAPI spec](https://github.com/michaelw/timesheet-app/issues/48)
- [jedisct1/openapi-mcp](https://github.com/jedisct1/openapi-mcp) (archived)
- [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk)
- [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)
- [kin-openapi](https://github.com/getkin/kin-openapi) - OpenAPI 3 parser for Go
