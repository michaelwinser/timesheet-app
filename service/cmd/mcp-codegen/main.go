// Command mcp-codegen generates MCP tool definitions from OpenAPI spec
//
// Usage:
//
//	go run ./cmd/mcp-codegen ../docs/v2/api-spec.yaml
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// MCPConfig represents the x-mcp extension at the info level
type MCPConfig struct {
	Name         string        `json:"name"`
	Version      string        `json:"version"`
	Instructions string        `json:"instructions"`
	Resources    []MCPResource `json:"resources"`
	Tools        []MCPTool     `json:"tools"` // MCP-only tools
}

// MCPResource represents an MCP resource definition
type MCPResource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MimeType    string `json:"mimeType"`
}

// MCPTool represents an MCP tool definition
type MCPTool struct {
	Name          string                 `json:"name"`
	Description   string                 `json:"description"`
	InputSchema   map[string]interface{} `json:"inputSchema"`
	CustomHandler bool                   `json:"custom_handler,omitempty"`
}

// MCPOperationExt represents the x-mcp extension on an operation
type MCPOperationExt struct {
	Tool              string              `json:"tool,omitempty"`
	Tools             []MCPToolExt        `json:"tools,omitempty"` // For operations that map to multiple tools
	Description       string              `json:"description,omitempty"`
	CustomHandler     bool                `json:"custom_handler,omitempty"`
	PathParamsAsInput bool                `json:"path_params_as_input,omitempty"`
	PresetParams      map[string]string   `json:"preset_params,omitempty"`
	CustomParams      []CustomParam       `json:"custom_params,omitempty"`
}

// MCPToolExt represents a tool extension when multiple tools map to one operation
type MCPToolExt struct {
	Tool          string            `json:"tool"`
	Description   string            `json:"description"`
	CustomHandler bool              `json:"custom_handler,omitempty"`
	PresetParams  map[string]string `json:"preset_params,omitempty"`
	CustomParams  []CustomParam     `json:"custom_params,omitempty"`
}

// CustomParam represents an MCP-specific parameter not in the REST API
type CustomParam struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Enum        []string `json:"enum,omitempty"`
	Default     any      `json:"default,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <openapi-spec.yaml>\n", os.Args[0])
		os.Exit(1)
	}

	specPath := os.Args[1]
	outputPath := "internal/mcp/tools.gen.go"

	if len(os.Args) > 2 {
		outputPath = os.Args[2]
	}

	// Load OpenAPI spec
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true

	doc, err := loader.LoadFromFile(specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading OpenAPI spec: %v\n", err)
		os.Exit(1)
	}

	// Extract x-mcp config from info
	var mcpConfig MCPConfig
	if ext, ok := doc.Info.Extensions["x-mcp"]; ok {
		data, err := json.Marshal(ext)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling x-mcp config: %v\n", err)
			os.Exit(1)
		}
		if err := json.Unmarshal(data, &mcpConfig); err != nil {
			fmt.Fprintf(os.Stderr, "Error unmarshaling x-mcp config: %v\n", err)
			os.Exit(1)
		}
	}

	// Extract tools from operations
	operationTools := extractOperationTools(doc)

	// Merge all tools (MCP-only + operation-derived)
	allTools := append(operationTools, mcpConfig.Tools...)

	// Sort tools by name for deterministic output
	sort.Slice(allTools, func(i, j int) bool {
		return allTools[i].Name < allTools[j].Name
	})

	// Generate code
	code := generateCode(mcpConfig, allTools)

	// Write output
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputPath, []byte(code), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Generated %s with %d tools and %d resources\n", outputPath, len(allTools), len(mcpConfig.Resources))
}

func extractOperationTools(doc *openapi3.T) []MCPTool {
	var tools []MCPTool

	for path, pathItem := range doc.Paths.Map() {
		for method, op := range pathItem.Operations() {
			if op == nil {
				continue
			}

			ext, ok := op.Extensions["x-mcp"]
			if !ok {
				continue
			}

			var mcpExt MCPOperationExt
			data, err := json.Marshal(ext)
			if err != nil {
				continue
			}
			if err := json.Unmarshal(data, &mcpExt); err != nil {
				continue
			}

			// Handle single tool mapping
			if mcpExt.Tool != "" {
				tool := buildTool(mcpExt.Tool, mcpExt.Description, mcpExt.CustomHandler, path, method, op, mcpExt.CustomParams, mcpExt.PathParamsAsInput)
				tools = append(tools, tool)
			}

			// Handle multiple tool mappings
			for _, toolExt := range mcpExt.Tools {
				tool := buildTool(toolExt.Tool, toolExt.Description, toolExt.CustomHandler, path, method, op, toolExt.CustomParams, false)
				tools = append(tools, tool)
			}
		}
	}

	return tools
}

func buildTool(name, description string, customHandler bool, path, method string, op *openapi3.Operation, customParams []CustomParam, pathParamsAsInput bool) MCPTool {
	inputSchema := buildInputSchema(op, customParams, pathParamsAsInput)

	return MCPTool{
		Name:          name,
		Description:   description,
		InputSchema:   inputSchema,
		CustomHandler: customHandler,
	}
}

func buildInputSchema(op *openapi3.Operation, customParams []CustomParam, pathParamsAsInput bool) map[string]interface{} {
	properties := make(map[string]interface{})
	var required []string

	// Add parameters from operation
	for _, paramRef := range op.Parameters {
		if paramRef == nil || paramRef.Value == nil {
			continue
		}
		param := paramRef.Value

		// Skip path params unless explicitly included
		if param.In == "path" && !pathParamsAsInput {
			continue
		}

		propSchema := schemaToMap(param.Schema)
		if param.Description != "" {
			propSchema["description"] = param.Description
		}

		// For path params, rename to snake_case with _id suffix
		propName := toSnakeCase(param.Name)
		if param.In == "path" && pathParamsAsInput {
			// Rename 'id' to 'event_id' for example
			if propName == "id" {
				propName = "event_id"
			}
		}

		properties[propName] = propSchema

		if param.Required {
			required = append(required, propName)
		}
	}

	// Add request body properties
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		if content, ok := op.RequestBody.Value.Content["application/json"]; ok && content.Schema != nil {
			bodySchema := content.Schema.Value
			if bodySchema != nil && bodySchema.Type != nil && bodySchema.Type.Is("object") {
				for propName, propRef := range bodySchema.Properties {
					if propRef == nil || propRef.Value == nil {
						continue
					}
					propSchema := schemaToMap(propRef)
					snakeName := toSnakeCase(propName)
					properties[snakeName] = propSchema
				}
				for _, req := range bodySchema.Required {
					snakeName := toSnakeCase(req)
					if !contains(required, snakeName) {
						required = append(required, snakeName)
					}
				}
			}
		}
	}

	// Add custom parameters
	for _, cp := range customParams {
		propSchema := map[string]interface{}{
			"type":        cp.Type,
			"description": cp.Description,
		}
		if len(cp.Enum) > 0 {
			propSchema["enum"] = cp.Enum
		}
		if cp.Default != nil {
			propSchema["default"] = cp.Default
		}
		properties[cp.Name] = propSchema
	}

	result := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		sort.Strings(required)
		result["required"] = required
	}

	return result
}

func schemaToMap(schemaRef *openapi3.SchemaRef) map[string]interface{} {
	if schemaRef == nil || schemaRef.Value == nil {
		return map[string]interface{}{"type": "string"}
	}

	schema := schemaRef.Value
	result := make(map[string]interface{})

	// Handle type
	if schema.Type != nil {
		types := schema.Type.Slice()
		if len(types) == 1 {
			result["type"] = types[0]
		} else if len(types) > 1 {
			result["type"] = types
		}
	}

	// Handle description
	if schema.Description != "" {
		result["description"] = schema.Description
	}

	// Handle enum
	if len(schema.Enum) > 0 {
		result["enum"] = schema.Enum
	}

	// Handle default
	if schema.Default != nil {
		result["default"] = schema.Default
	}

	// Handle format (skip uuid as MCP uses plain strings)
	if schema.Format != "" && schema.Format != "uuid" && schema.Format != "date-time" {
		if schema.Format == "date" {
			// Add date format hint to description
			if desc, ok := result["description"].(string); ok && !strings.Contains(desc, "YYYY-MM-DD") {
				result["description"] = desc + " (YYYY-MM-DD)"
			}
		}
	}

	return result
}

func toSnakeCase(s string) string {
	// Simple camelCase to snake_case conversion
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func generateCode(config MCPConfig, tools []MCPTool) string {
	var sb strings.Builder

	sb.WriteString(`// Code generated by mcp-codegen from api-spec.yaml. DO NOT EDIT.

package mcp

// ServerInfo contains MCP server metadata
type ServerInfo struct {
	Name         string
	Version      string
	Instructions string
}

// GetServerInfo returns the MCP server metadata
func GetServerInfo() ServerInfo {
	return ServerInfo{
		Name:         `)
	sb.WriteString(fmt.Sprintf("%q", config.Name))
	sb.WriteString(`,
		Version:      `)
	sb.WriteString(fmt.Sprintf("%q", config.Version))
	sb.WriteString(`,
		Instructions: `)
	sb.WriteString(fmt.Sprintf("%q", config.Instructions))
	sb.WriteString(`,
	}
}

// Resource represents an MCP resource
type Resource struct {
	URI         string
	Name        string
	Description string
	MimeType    string
}

// GetResources returns all MCP resources
func GetResources() []Resource {
	return []Resource{
`)

	for _, r := range config.Resources {
		sb.WriteString(fmt.Sprintf(`		{
			URI:         %q,
			Name:        %q,
			Description: %q,
			MimeType:    %q,
		},
`, r.URI, r.Name, r.Description, r.MimeType))
	}

	sb.WriteString(`	}
}

// Tool represents an MCP tool definition
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// GetTools returns all MCP tool definitions
func GetTools() []Tool {
	return []Tool{
`)

	for _, t := range tools {
		inputSchemaJSON, _ := json.MarshalIndent(t.InputSchema, "\t\t\t", "\t")
		// Clean up the JSON formatting for Go
		schemaStr := string(inputSchemaJSON)
		schemaStr = strings.ReplaceAll(schemaStr, `"`, "`+\"`\"+`")
		schemaStr = "parseSchema(`" + strings.ReplaceAll(string(inputSchemaJSON), "`", "` + \"`\" + `") + "`)"

		sb.WriteString(fmt.Sprintf(`		{
			Name:        %q,
			Description: %q,
			InputSchema: %s,
		},
`, t.Name, t.Description, schemaStr))
	}

	sb.WriteString(`	}
}

// parseSchema parses a JSON schema string into a map
func parseSchema(s string) map[string]any {
	var result map[string]any
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		panic("invalid schema: " + err.Error())
	}
	return result
}

// ToolNames returns a list of all tool names
func ToolNames() []string {
	tools := GetTools()
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}
`)

	// Add import for json
	code := sb.String()
	code = strings.Replace(code, "package mcp\n", "package mcp\n\nimport \"encoding/json\"\n", 1)

	return code
}
