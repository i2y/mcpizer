package usecase

import (
	"context"

	"mcp-bridge/internal/domain"
)

// SchemaFetcher defines the contract for fetching API schemas from various sources.
// Implementations will handle specific protocols like HTTP(S) for OpenAPI
// or gRPC reflection endpoints.
type SchemaFetcher interface {
	// Fetch retrieves an API schema from the given source identifier (e.g., URL, gRPC target).
	// It returns the domain representation of the schema or an error if fetching fails.
	Fetch(ctx context.Context, src string) (domain.APISchema, error)
}

// ToolGenerator defines the contract for converting a domain APISchema
// into a slice of MCP-compliant Tools and their corresponding InvocationDetails.
// Implementations will exist for each supported schema type (OpenAPI, gRPC).
type ToolGenerator interface {
	// Generate transforms the provided API schema into a list of Tool definitions
	// and the details needed to invoke them.
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
