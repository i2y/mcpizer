package connect

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/i2y/mcpizer/internal/domain"
	"github.com/i2y/mcpizer/internal/usecase"
)

// SchemaFetcher implements the usecase.SchemaFetcher interface for Connect-RPC.
type SchemaFetcher struct {
	logger *slog.Logger
}

// NewSchemaFetcher creates a new Connect-RPC SchemaFetcher.
func NewSchemaFetcher(logger *slog.Logger) *SchemaFetcher {
	return &SchemaFetcher{
		logger: logger.With("component", "connect_fetcher"),
	}
}

// Fetch attempts to fetch schema for a Connect-RPC endpoint.
// Since Connect-RPC doesn't have a standard discovery mechanism like gRPC reflection,
// this implementation primarily serves as a placeholder that validates the URL format.
func (f *SchemaFetcher) Fetch(ctx context.Context, src string) (domain.APISchema, error) {
	log := f.logger.With(slog.String("source", src))
	log.Info("Fetching Connect-RPC schema")

	// Parse the source - remove connect:// prefix if present
	target := src
	if strings.HasPrefix(src, "connect://") {
		target = strings.TrimPrefix(src, "connect://")
	}

	// For Connect-RPC, we don't have automatic schema discovery like gRPC reflection
	// The schema must be provided via .proto files or configuration
	log.Warn("Connect-RPC does not support automatic schema discovery. Use .proto files or gRPC reflection if available.")

	// Return a minimal schema indicating this is a Connect endpoint
	return domain.APISchema{
		Source:     src,
		Type:       domain.SchemaTypeConnect,
		RawData:    []byte(target), // Store the server URL
		ParsedData: map[string]string{"server": target, "mode": "http"},
	}, nil
}

// FetchWithConfig fetches schema with additional configuration
func (f *SchemaFetcher) FetchWithConfig(ctx context.Context, config usecase.SchemaSourceConfig) (domain.APISchema, error) {
	log := f.logger.With(slog.String("source", config.URL))

	// Determine the mode from config
	mode := "http" // default to HTTP mode
	if config.Mode != "" {
		mode = config.Mode
	}

	// If mode is gRPC, we should delegate to gRPC fetcher
	if mode == "grpc" {
		log.Info("Connect-RPC service with gRPC mode - should use gRPC fetcher")
		return domain.APISchema{}, fmt.Errorf("Connect-RPC with gRPC mode should use gRPC fetcher")
	}

	schema, err := f.Fetch(ctx, config.URL)
	if err != nil {
		return schema, err
	}

	// Update parsed data with mode from config
	if parsedData, ok := schema.ParsedData.(map[string]string); ok {
		parsedData["mode"] = mode
		if config.Server != "" {
			parsedData["server"] = config.Server
		}
	}

	return schema, nil
}
