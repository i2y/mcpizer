package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"mcp-bridge/internal/domain"
)

// SyncSchemaUseCase orchestrates fetching, generating, and storing tools from a schema source.
type SyncSchemaUseCase struct {
	fetchers   map[domain.SchemaType]SchemaFetcher
	generators map[domain.SchemaType]ToolGenerator
	repository ToolRepository
	logger     *slog.Logger
}

// NewSyncSchemaUseCase creates a new SyncSchemaUseCase.
// It requires maps of fetchers and generators keyed by schema type,
// and a tool repository.
func NewSyncSchemaUseCase(
	fetchers map[domain.SchemaType]SchemaFetcher,
	generators map[domain.SchemaType]ToolGenerator,
	repository ToolRepository,
	logger *slog.Logger,
) *SyncSchemaUseCase {
	return &SyncSchemaUseCase{
		fetchers:   fetchers,
		generators: generators,
		repository: repository,
		logger:     logger.With("usecase", "SyncSchema"),
	}
}

// Execute fetches a schema from the source, determines its type (implicitly or explicitly),
// generates tools using the appropriate generator, and saves them to the repository.
// The 'source' string's format might indicate the type (e.g., prefix `grpc://`),
// or the fetcher implementation could determine it.
func (uc *SyncSchemaUseCase) Execute(ctx context.Context, source string) error {
	log := uc.logger.With(slog.String("source", source))
	log.Info("Starting schema sync")

	// 1. Determine Schema Type and Fetch
	// Simple approach: try fetchers until one succeeds or recognizes the source format.
	// A more robust approach might involve explicit type hints or smarter source parsing.
	var fetchedSchema domain.APISchema
	var fetcher SchemaFetcher
	var err error

	// TODO: Implement a more sophisticated way to select the correct fetcher.
	// For now, let's assume an OpenAPI fetcher is available and try it.
	fetcher, ok := uc.fetchers[domain.SchemaTypeOpenAPI]
	if !ok {
		log.Debug("OpenAPI fetcher not found, trying gRPC fetcher")
		fetcher, ok = uc.fetchers[domain.SchemaTypeGRPC]
		if !ok {
			log.Error("No schema fetcher available for source")
			return fmt.Errorf("no schema fetcher available for source: %s", source)
		}
	}

	fetchedSchema, err = fetcher.Fetch(ctx, source)
	if err != nil {
		log.Error("Failed to fetch schema", slog.Any("error", err))
		return fmt.Errorf("failed to fetch schema from %s: %w", source, err)
	}
	log.Info("Schema fetched successfully", slog.String("schema_type", string(fetchedSchema.Type)))

	// Ensure the fetcher correctly set the schema type
	if fetchedSchema.Type == "" {
		// If not set by fetcher, we might infer it here or return an error
		// For now, assume the fetcher we used corresponds to the type
		fetchedSchema.Type = fetcherType(uc.fetchers, fetcher) // Helper needed
		if fetchedSchema.Type == "" {
			log.Error("Could not determine schema type after fetching")
			return fmt.Errorf("could not determine schema type for source %s", source)
		}
		log.Warn("Fetcher did not set schema type, inferred", slog.String("inferred_type", string(fetchedSchema.Type)))
	}

	// 2. Select Generator
	generator, ok := uc.generators[fetchedSchema.Type]
	if !ok {
		log.Error("No tool generator found for schema type", slog.String("schema_type", string(fetchedSchema.Type)))
		return fmt.Errorf("no tool generator found for schema type: %s", fetchedSchema.Type)
	}

	// 3. Generate Tools and Invocation Details
	log.Info("Generating tools and invocation details", slog.String("schema_type", string(fetchedSchema.Type)))
	tools, details, err := generator.Generate(fetchedSchema)
	if err != nil {
		log.Error("Failed to generate tools/details", slog.Any("error", err))
		return fmt.Errorf("failed to generate tools/details for schema %s: %w", source, err)
	}

	// 4. Save Tools and Details
	log.Info("Saving tools and details to repository", slog.Int("tool_count", len(tools)))
	if err := uc.repository.Save(ctx, tools, details); err != nil {
		log.Error("Failed to save generated tools/details", slog.Any("error", err))
		return fmt.Errorf("failed to save generated tools/details: %w", err)
	}

	log.Info("Successfully synced schema and tools", slog.Int("tool_count", len(tools)))
	return nil
}

// Helper function to determine the type associated with a fetcher instance.
// This is crude; a better approach might involve tagging fetchers/generators directly.
func fetcherType(fetchers map[domain.SchemaType]SchemaFetcher, f SchemaFetcher) domain.SchemaType {
	for t, fetcherInstance := range fetchers {
		if fetcherInstance == f {
			return t
		}
	}
	return "" // Unknown
}
