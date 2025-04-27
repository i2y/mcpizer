package mcpjsonrpc

// Based on JSON-RPC 2.0 Specification: https://www.jsonrpc.org/specification

// Request represents a JSON-RPC request object.
type Request struct {
	Version string      `json:"jsonrpc"`          // MUST be "2.0"
	Method  string      `json:"method"`           // Method to be invoked
	Params  interface{} `json:"params,omitempty"` // Parameters (structured value or array)
	ID      interface{} `json:"id,omitempty"`     // Request identifier (string, number, or null)
}

// Response represents a JSON-RPC response object.
type Response struct {
	Version string      `json:"jsonrpc"`          // MUST be "2.0"
	Result  interface{} `json:"result,omitempty"` // Required on success
	Error   *Error      `json:"error,omitempty"`  // Required on error
	ID      interface{} `json:"id"`               // Must match request ID (or null if could not be determined)
}

// Error represents a JSON-RPC error object.
type Error struct {
	Code    int         `json:"code"`           // Error code
	Message string      `json:"message"`        // Error message
	Data    interface{} `json:"data,omitempty"` // Additional data about the error
}

// Error codes (subset, based on JSON-RPC spec and potential application errors)
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
	// -32000 to -32099: Server error (implementation-defined)
	CodeServerErrorToolNotFound = -32000
	CodeServerErrorToolFailed   = -32001
)

// InvokeToolParams defines the structure for the "params" field
// when the method is "invokeTool".
type InvokeToolParams struct {
	ToolName   string                 `json:"toolName"`
	Parameters map[string]interface{} `json:"parameters"`
}
