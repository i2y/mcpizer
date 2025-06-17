package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/i2y/mcpizer/internal/domain"

	// Import mcp-go types
	"github.com/mark3labs/mcp-go/mcp"
	// Remove direct dependency on mcpServer implementation
	// mcpServer "github.com/mark3labs/mcp-go/server"
)

// Interface definitions (like ToolInvoker) should be in interfaces.go

// SyncSchemaUseCase orchestrates fetching, generating, and registering tools with an MCP server.
type SyncSchemaUseCase struct {
	fetchers      map[domain.SchemaType]SchemaFetcher
	generators    map[domain.SchemaType]ToolGenerator
	mcpServer     MCPServerAdapter // Use the interface type
	invoker       ToolInvoker
	logger        *slog.Logger
	schemaSources []SchemaSourceConfig
}

// NewSyncSchemaUseCase creates a new SyncSchemaUseCase.
func NewSyncSchemaUseCase(
	schemaSources []SchemaSourceConfig,
	fetchers map[domain.SchemaType]SchemaFetcher,
	generators map[domain.SchemaType]ToolGenerator,
	mcpSrv MCPServerAdapter, // Use the interface type
	invoker ToolInvoker,
	logger *slog.Logger,
) *SyncSchemaUseCase {
	// Basic validation
	if mcpSrv == nil {
		panic("NewSyncSchemaUseCase requires a non-nil mcpServer adapter")
	}
	if invoker == nil {
		panic("NewSyncSchemaUseCase requires a non-nil invoker")
	}
	return &SyncSchemaUseCase{
		fetchers:      fetchers,
		generators:    generators,
		mcpServer:     mcpSrv,
		invoker:       invoker,
		logger:        logger.With("usecase", "SyncSchema"),
		schemaSources: schemaSources,
	}
}

// SyncAllConfiguredSources fetches schemas from all configured sources,
// generates tools for each, and registers them with the MCP server.
// It returns a joined error if any source fails, but attempts to process all sources.
func (uc *SyncSchemaUseCase) SyncAllConfiguredSources(ctx context.Context) error {
	uc.logger.Info("Starting sync for all configured schema sources.", slog.Int("source_count", len(uc.schemaSources)))

	var syncErrors []error

	for _, source := range uc.schemaSources {
		log := uc.logger.With(slog.String("source", source.URL))
		log.Info("Processing schema source.")

		if err := uc.processSingleSourceAndRegister(ctx, source); err != nil {
			log.Error("Failed to process schema source.", slog.Any("error", err))
			syncErrors = append(syncErrors, fmt.Errorf("source '%s': %w", source.URL, err))
			continue
		}
		log.Info("Successfully processed and registered tools for schema source.")
	}

	if len(syncErrors) > 0 {
		uc.logger.Error("Schema sync completed with errors.", slog.Int("error_count", len(syncErrors)))
		return errors.Join(syncErrors...)
	}

	uc.logger.Info("Successfully synced and registered tools for all configured schema sources.")
	return nil
}

// processSingleSourceAndRegister handles fetching, generating, and registering tools for one source.
func (uc *SyncSchemaUseCase) processSingleSourceAndRegister(ctx context.Context, source SchemaSourceConfig) error {
	log := uc.logger.With(slog.String("source", source.URL))

	schemaType := uc.determineSchemaType(source.URL)
	if schemaType == "" {
		return fmt.Errorf("could not determine schema type from source format")
	}
	log = log.With(slog.String("detected_type", string(schemaType)))

	fetcher, ok := uc.fetchers[schemaType]
	if !ok {
		return fmt.Errorf("no schema fetcher available for type %s", schemaType)
	}
	
	// Use FetchWithConfig if headers are provided
	var fetchedSchema domain.APISchema
	var err error
	if len(source.Headers) > 0 {
		fetchedSchema, err = fetcher.FetchWithConfig(ctx, source)
		if err != nil {
			return fmt.Errorf("failed to fetch schema with headers: %w", err)
		}
	} else {
		fetchedSchema, err = fetcher.Fetch(ctx, source.URL)
		if err != nil {
			return fmt.Errorf("failed to fetch schema: %w", err)
		}
	}
	if fetchedSchema.Type == "" {
		fetchedSchema.Type = schemaType
		log.Warn("Fetcher did not set schema type, using detected type.")
	} else if fetchedSchema.Type != schemaType {
		return fmt.Errorf("detected schema type (%s) mismatch with fetched schema type (%s)", schemaType, fetchedSchema.Type)
	}
	log.Info("Schema fetched successfully.")

	generator, ok := uc.generators[fetchedSchema.Type]
	if !ok {
		return fmt.Errorf("no tool generator found for schema type %s", fetchedSchema.Type)
	}
	log.Info("Generating tools and invocation details.")
	tools, detailsList, err := generator.Generate(fetchedSchema)
	if err != nil {
		return fmt.Errorf("failed to generate tools/details: %w", err)
	}
	log.Info("Generated domain tools and details", slog.Int("count", len(tools)))

	registeredCount := 0
	for i, domainTool := range tools {
		toolName := domainTool.Name
		if i >= len(detailsList) {
			log.Error("Mismatch between tools and details lists", slog.String("toolName", toolName))
			continue
		}
		invocationDetails := detailsList[i]

		mcpTool, err := uc.convertDomainToolToMCPTool(domainTool)
		if err != nil {
			log.Error("Failed to convert domain tool to MCP tool, skipping registration.", slog.String("toolName", toolName), slog.Any("error", err))
			continue
		}

		handlerFunc := uc.createToolHandler(invocationDetails, toolName)

		uc.mcpServer.AddTool(*mcpTool, handlerFunc)
		log.Debug("Registered tool with MCP server", slog.String("toolName", mcpTool.Name))
		registeredCount++
	}

	log.Info("Finished processing source, registered tools.", slog.Int("registered_count", registeredCount))
	return nil
}

// convertDomainToolToMCPTool converts the internal domain.Tool definition
// (including its JSONSchema) into the mcp.Tool format required by the mcp-go library.
func (uc *SyncSchemaUseCase) convertDomainToolToMCPTool(dTool domain.Tool) (*mcp.Tool, error) {
	log := uc.logger.With(slog.String("toolName", dTool.Name))
	log.Debug("Converting domain tool to MCP tool")

	// Start building tool options
	toolOptions := []mcp.ToolOption{
		mcp.WithDescription(dTool.Description),
	}

	// Process InputSchema properties
	if dTool.InputSchema.Type == "object" && dTool.InputSchema.Properties != nil {
		log.Debug("Processing input schema properties", slog.Int("property_count", len(dTool.InputSchema.Properties)))
		requiredMap := make(map[string]bool)
		for _, req := range dTool.InputSchema.Required {
			requiredMap[req] = true
		}

		for name, prop := range dTool.InputSchema.Properties {
			isRequired := requiredMap[name]
			// TODO: Get description from prop.Description if available
			propDescription := ""

			propertyOpts := []mcp.PropertyOption{}
			if propDescription != "" {
				propertyOpts = append(propertyOpts, mcp.Description(propDescription))
			}
			if isRequired {
				propertyOpts = append(propertyOpts, mcp.Required())
			}

			// --- Common options setup complete ---

			// Add parameter based on type using the correct mcp functions
			switch prop.Type {
			case "string":
				// Add Enum option if applicable
				if len(prop.Enum) > 0 {
					enumStrings := make([]string, 0, len(prop.Enum))
					canEnum := true
					for _, enumVal := range prop.Enum {
						if strVal, ok := enumVal.(string); ok {
							enumStrings = append(enumStrings, strVal)
						} else {
							log.Warn("Non-string value found in enum for string property", slog.String("param", name), slog.Any("value", enumVal))
							canEnum = false
							break
						}
					}
					if canEnum {
						propertyOpts = append(propertyOpts, mcp.Enum(enumStrings...))
					}
				}
				toolOptions = append(toolOptions, mcp.WithString(name, propertyOpts...))
				log.Debug("Added string parameter", slog.String("name", name), slog.Bool("required", isRequired))
			case "number":
				toolOptions = append(toolOptions, mcp.WithNumber(name, propertyOpts...))
				log.Debug("Added number parameter", slog.String("name", name), slog.Bool("required", isRequired))
			case "integer":
				toolOptions = append(toolOptions, mcp.WithNumber(name, propertyOpts...))
				log.Debug("Added integer parameter (as number)", slog.String("name", name), slog.Bool("required", isRequired))
			case "boolean":
				toolOptions = append(toolOptions, mcp.WithBoolean(name, propertyOpts...))
				log.Debug("Added boolean parameter", slog.String("name", name), slog.Bool("required", isRequired))
			case "array":
				if prop.Items == nil {
					log.Warn("Array parameter has no items definition, skipping", slog.String("name", name))
					continue
				}
				itemSchemaMap, err := convertDomainSchemaToMap(prop.Items)
				if err != nil {
					log.Error("Failed to convert array item schema, skipping array param", slog.String("name", name), slog.Any("error", err))
					continue
				}
				// Combine base property options with the specific Items option
				arrayPropertyOpts := append([]mcp.PropertyOption{}, propertyOpts...)
				arrayPropertyOpts = append(arrayPropertyOpts, mcp.Items(itemSchemaMap))
				// TODO: Add MinItems, MaxItems if available in prop
				toolOptions = append(toolOptions, mcp.WithArray(name, arrayPropertyOpts...))
				log.Debug("Added array parameter", slog.String("name", name), slog.Bool("required", isRequired))
			case "object":
				objectPropertiesMap, err := convertDomainSchemaPropertiesToMap(prop.Properties)
				if err != nil {
					log.Error("Failed to convert object properties, skipping object param", slog.String("name", name), slog.Any("error", err))
					continue
				}
				// Combine base property options with the specific Properties option
				objectPropertyOpts := append([]mcp.PropertyOption{}, propertyOpts...)
				objectPropertyOpts = append(objectPropertyOpts, mcp.Properties(objectPropertiesMap))
				// Add required fields for the object itself, if any (from prop.Required)
				if len(prop.Required) > 0 {
					// Note: mcp.Required() applies to the top-level param (the object itself).
					// The required fields *within* the object are part of the mcp.Properties schema.
					// We already added the top-level required flag in the initial propertyOpts if needed.
					// We need to ensure the object schema map includes "required": [...] internally.
					// This should be handled by convertDomainSchemaPropertiesToMap/convertDomainSchemaToMap.
				}
				toolOptions = append(toolOptions, mcp.WithObject(name, objectPropertyOpts...))
				log.Debug("Added object parameter", slog.String("name", name), slog.Bool("required", isRequired))
			default:
				log.Warn("Unsupported parameter type in input schema", slog.String("name", name), slog.String("type", prop.Type))
			}
		}
	} else if dTool.InputSchema.Type != "" && dTool.InputSchema.Type != "object" {
		log.Warn("Root input schema is not of type 'object'", slog.String("type", dTool.InputSchema.Type))
	}

	mcpTool := mcp.NewTool(
		dTool.Name,
		toolOptions...,
	)

	return &mcpTool, nil
}

// convertDomainSchemaToMap converts domain.JSONSchemaProps to map[string]any for JSON Schema representation.
func convertDomainSchemaToMap(schema *domain.JSONSchemaProps) (map[string]any, error) {
	if schema == nil {
		return map[string]any{}, nil // Represent nil schema as empty object
	}

	schemaMap := make(map[string]any)

	if schema.Type != "" {
		schemaMap["type"] = schema.Type
	}
	if schema.Format != "" {
		schemaMap["format"] = schema.Format
	}
	if len(schema.Enum) > 0 {
		schemaMap["enum"] = schema.Enum
	}
	// TODO: Add description, default, validation constraints etc.
	// if schema.Description != "" {
	// 	schemaMap["description"] = schema.Description
	// }

	switch schema.Type {
	case "object":
		propertiesMap, err := convertDomainSchemaPropertiesToMap(schema.Properties)
		if err != nil {
			return nil, fmt.Errorf("error converting object properties: %w", err)
		}
		if len(propertiesMap) > 0 {
			schemaMap["properties"] = propertiesMap
		}
		if len(schema.Required) > 0 {
			schemaMap["required"] = schema.Required
		}
	case "array":
		if schema.Items != nil {
			itemSchemaMap, err := convertDomainSchemaToMap(schema.Items)
			if err != nil {
				return nil, fmt.Errorf("error converting array items: %w", err)
			}
			schemaMap["items"] = itemSchemaMap
		} else {
			// Array without items defaults to items allowing any type
			schemaMap["items"] = map[string]any{}
		}
		// TODO: Add minItems, maxItems if available
	}

	return schemaMap, nil
}

// convertDomainSchemaPropertiesToMap converts a map of domain schemas to a map for JSON Schema properties.
func convertDomainSchemaPropertiesToMap(props map[string]domain.JSONSchemaProps) (map[string]any, error) {
	propertiesMap := make(map[string]any)
	for name, prop := range props {
		propMap, err := convertDomainSchemaToMap(&prop)
		if err != nil {
			return nil, fmt.Errorf("error converting property '%s': %w", name, err)
		}
		propertiesMap[name] = propMap
	}
	return propertiesMap, nil
}

// createToolHandler creates a handler function specific to a tool, capturing
// its invocation details and the shared invoker.
// Return type should match mcpServer.ToolHandlerFunc from the adapter interface
// Need to import mcpServer alias locally or fully qualify
func (uc *SyncSchemaUseCase) createToolHandler(details InvocationDetails, toolName string) func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) { // Use imported mcp types
	invoker := uc.invoker
	log := uc.logger.With(slog.String("toolName", toolName))

	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.Info("Executing MCP tool handler")
		params := request.GetArguments()
		log.Debug("Handler received parameters", slog.Any("params", params))

		resultData, invokeErr := invoker.Invoke(ctx, details, params)
		if invokeErr != nil {
			log.Error("Tool handler failed during invocation", slog.Any("error", invokeErr))
			return nil, invokeErr
		}

		log.Info("Tool handler invocation successful")
		resultText := fmt.Sprintf("%+v", resultData)
		mcpResult := mcp.NewToolResultText(resultText) // Use imported mcp type
		log.Warn("Tool result formatting is a placeholder (text).", slog.Any("resultData", resultData))

		return mcpResult, nil
	}
}

// determineSchemaType guesses the schema type based on the source string prefix.
func (uc *SyncSchemaUseCase) determineSchemaType(source string) domain.SchemaType {
	if strings.HasPrefix(source, "grpc://") {
		return domain.SchemaTypeGRPC
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") || !strings.Contains(source, "://") {
		return domain.SchemaTypeOpenAPI
	}
	return ""
}

// Execute method now uses the interface implicitly via processSingleSourceAndRegister
func (uc *SyncSchemaUseCase) Execute(ctx context.Context, source string) error {
	log := uc.logger.With(slog.String("source", source))
	log.Info("Starting single schema sync via Execute method.")

	// Create a SchemaSourceConfig from the string source
	sourceConfig := SchemaSourceConfig{URL: source}
	
	// Wrap the error from processSingleSourceAndRegister to match expected test output
	if err := uc.processSingleSourceAndRegister(ctx, sourceConfig); err != nil {
		log.Error("Failed to process schema source via Execute.", slog.Any("error", err))
		// Wrap the error here to provide context expected by tests
		return fmt.Errorf("error executing sync for source %s: %w", source, err)
	}

	log.Info("Successfully synced schema and registered tools via Execute.")
	return nil
}
