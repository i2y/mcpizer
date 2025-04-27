package domain

// Tool represents a callable function derived from an API schema,
// compliant with the Model Context Protocol (MCP).
// Based on MCP Spec 2025-03-26: https://modelcontextprotocol.io/specification/2025-03-26
type Tool struct {
	// Name follows the pattern "{namespace}-{verb}-{resource}" or "{namespace}-{action}".
	// It MUST be unique within the MCP server.
	Name string `json:"name"`

	// Description provides a natural language explanation of what the tool does.
	// This is crucial for the LLM to understand when to use the tool.
	Description string `json:"description"`

	// InputSchema defines the structure of the data the tool expects.
	// Uses JSON Schema format.
	InputSchema JSONSchemaProps `json:"input_schema"`

	// OutputSchema defines the structure of the data the tool returns upon successful invocation.
	// Optional. If omitted, the output is considered opaque or unstructured.
	// Uses JSON Schema format.
	OutputSchema *JSONSchemaProps `json:"output_schema,omitempty"`

	// TODO: Add fields for invocation details (e.g., HTTP method/path, gRPC service/method)
	// These might live here or in a separate internal mapping structure used by InvokeToolUseCase.
	// Keeping them out of the core MCP definition for now.
	// InvocationTarget InvocationDetails
}

// JSONSchemaProps represents the properties of a JSON schema,
// commonly used for input and output definitions in MCP tools.
// This is a simplified version; a more complete implementation might import
// a dedicated JSON schema library or use map[string]interface{}.
type JSONSchemaProps struct {
	Type       string                     `json:"type"`                 // e.g., "object", "string", "number", "integer", "boolean", "array"
	Properties map[string]JSONSchemaProps `json:"properties,omitempty"` // For type "object"
	Required   []string                   `json:"required,omitempty"`   // For type "object"
	Items      *JSONSchemaProps           `json:"items,omitempty"`      // For type "array"
	Format     string                     `json:"format,omitempty"`     // e.g., "date-time", "email"
	Enum       []interface{}              `json:"enum,omitempty"`       // Possible values
	// Add other JSON Schema fields as needed: description, default, minimum, maximum, etc.
}

// Consider adding helper functions here later, e.g.:
// func (t *Tool) ValidateInput(input map[string]interface{}) error
