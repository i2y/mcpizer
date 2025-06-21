package connect

import (
	"fmt"
	"log/slog"

	"github.com/i2y/mcpizer/internal/domain"
	"github.com/i2y/mcpizer/internal/usecase"
)

// Generator implements the usecase.ToolGenerator interface for Connect-RPC.
type Generator struct {
	logger *slog.Logger
}

// NewGenerator creates a new Connect-RPC Generator.
func NewGenerator(logger *slog.Logger) *Generator {
	return &Generator{
		logger: logger.With("component", "connect_generator"),
	}
}

// Generate creates tool definitions from a Connect-RPC schema.
// Since Connect-RPC doesn't provide automatic discovery, this requires
// the schema to be populated from .proto files or other sources.
func (g *Generator) Generate(schema domain.APISchema) ([]domain.Tool, []usecase.InvocationDetails, error) {
	log := g.logger.With(slog.String("source", schema.Source))

	// Connect-RPC schemas should be generated from .proto files
	// This generator mainly serves to create appropriate invocation details
	// for Connect-RPC HTTP mode

	if schema.Type != domain.SchemaTypeConnect && schema.Type != domain.SchemaTypeConnectProto {
		return nil, nil, fmt.Errorf("invalid schema type for Connect generator: %s", schema.Type)
	}

	// Extract server and mode from parsed data
	serverURL := ""
	mode := "http"
	if parsedData, ok := schema.ParsedData.(map[string]string); ok {
		serverURL = parsedData["server"]
		if m, ok := parsedData["mode"]; ok {
			mode = m
		}
	}

	if serverURL == "" {
		return nil, nil, fmt.Errorf("server URL is required for Connect-RPC schemas")
	}

	log.Info("Connect-RPC generator called",
		slog.String("server", serverURL),
		slog.String("mode", mode),
	)

	// For Connect-RPC, we expect the actual tool definitions to come from
	// proto files. This generator is mainly used to create invocation details
	// that specify Connect-RPC HTTP mode.

	// If we have tools from proto generation, we need to update their invocation details
	// This is a placeholder - in practice, this would be integrated with proto generator

	return nil, nil, fmt.Errorf("Connect-RPC requires .proto files for tool generation. Use a .proto file with type: connect")
}
