package usecase

import (
	"context"
	"errors"

	"github.com/i2y/mcpizer/internal/domain"
	// Import mcp types needed for the adapter interface
	"github.com/mark3labs/mcp-go/mcp"
	// Import server type for the handler function
	mcpGoServer "github.com/mark3labs/mcp-go/server"
)

// Standard errors returned by use cases and adapters.
var (
	ErrToolNotFound = errors.New("tool not found")
	// TODO: Define other standard errors like ErrInvocationFailed, ErrSchemaFetchFailed etc.
)

// --- Schema Source Related ---

// SchemaSourceConfig represents a schema source with optional configuration
type SchemaSourceConfig struct {
	URL     string
	Headers map[string]string
}

// SchemaFetcher defines the interface for fetching API schemas from various sources.
type SchemaFetcher interface {
	Fetch(ctx context.Context, source string) (domain.APISchema, error)
	FetchWithConfig(ctx context.Context, config SchemaSourceConfig) (domain.APISchema, error)
}

// ToolGenerator defines the interface for generating Tools and InvocationDetails
// from a fetched APISchema.
type ToolGenerator interface {
	Generate(schema domain.APISchema) ([]domain.Tool, []InvocationDetails, error)
}

// ToolRepository defines the contract for storing and retrieving generated Tools
// and their InvocationDetails.
// Implementations could range from in-memory stores to persistent databases.
type ToolRepository interface {
	// Save stores a list of tools and their associated invocation details.
	// It should ensure that the length of tools and details match and correspond by index,
	// or handle potential mismatches appropriately.
	Save(ctx context.Context, tools []domain.Tool, details []InvocationDetails) error

	// List retrieves all currently stored tools.
	List(ctx context.Context) ([]domain.Tool, error)

	// FindToolByName retrieves a specific tool definition by its unique name.
	FindToolByName(ctx context.Context, name string) (*domain.Tool, error)

	// FindInvocationDetailsByName retrieves the invocation details for a specific tool by name.
	FindInvocationDetailsByName(ctx context.Context, name string) (*InvocationDetails, error)
}

// --- MCP Server Abstraction ---

// MCPServerAdapter defines the interface required by the SyncSchemaUseCase
// to interact with the underlying MCP server (like mcp-go).
// This avoids direct dependency on a specific server implementation in the use case.
type MCPServerAdapter interface {
	// AddTool registers a tool and its handler with the server.
	// The handlerFunc signature must match the expected signature of the specific
	// MCP server library being adapted.
	// Use the specific type from the mcp-go/server package.
	AddTool(tool mcp.Tool, handlerFunc mcpGoServer.ToolHandlerFunc)
	// TODO: Add other methods if SyncSchemaUseCase needs them (e.g., RemoveTool)
}

// --- Tool Invocation Related ---

// InvocationDetails is defined in invoke_tool.go

// ToolInvoker is defined in invoke_tool.go
/*
type ToolInvoker interface {
	Invoke(ctx context.Context, details InvocationDetails, params map[string]interface{}) (interface{}, error)
}
*/
