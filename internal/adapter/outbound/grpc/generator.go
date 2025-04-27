package grpc

import (
	"fmt"
	"log/slog"
	"strings"

	"mcp-bridge/internal/domain"
	"mcp-bridge/internal/usecase"
	// Requires libraries for protobuf descriptor parsing if we implement fully.
	// e.g., "google.golang.org/protobuf/reflect/protodesc"
	// e.g., "google.golang.org/protobuf/reflect/protoreflect"
	// e.g., "github.com/jhump/protoreflect/desc"
)

// ToolGenerator implements the usecase.ToolGenerator interface for gRPC schemas.
// NOTE: This is a placeholder implementation. Full implementation requires
// detailed protobuf descriptor parsing and conversion to JSON Schema.
type ToolGenerator struct {
	logger *slog.Logger
}

// NewToolGenerator creates a new gRPC ToolGenerator.
func NewToolGenerator(logger *slog.Logger) *ToolGenerator {
	return &ToolGenerator{
		logger: logger.With("component", "grpc_generator"),
	}
}

// Generate attempts to create MCP Tools and InvocationDetails from gRPC service information.
// The current implementation is basic due to the complexity of Protobuf->JSON Schema conversion
// and relies only on service names provided by the current fetcher.
func (g *ToolGenerator) Generate(schema domain.APISchema) ([]domain.Tool, []usecase.InvocationDetails, error) {
	log := g.logger.With(slog.String("source", schema.Source))
	log.Info("Generating tools from gRPC schema (placeholder)")

	if schema.Type != domain.SchemaTypeGRPC {
		log.Error("Invalid schema type for gRPC generator", slog.String("schema_type", string(schema.Type)))
		return nil, nil, fmt.Errorf("invalid schema type for gRPC generator: %s", schema.Type)
	}

	// Current fetcher provides service names in ParsedData
	serviceNames, ok := schema.ParsedData.([]string)
	if !ok {
		log.Error("Invalid parsed data format for gRPC schema: expected []string")
		return nil, nil, fmt.Errorf("invalid parsed data format for gRPC schema: expected []string of service names")
	}

	var tools []domain.Tool
	var detailsList []usecase.InvocationDetails

	log.Warn("gRPC tool generation is currently a placeholder. Creating dummy tools based on service names only.")

	// Assumed host from the reflection source URL
	host := schema.Source // This is likely just host:port, need to prefix with http:// or https://
	if !strings.Contains(host, "://") {
		host = "http://" + host // Default to http for Connect, might need config
	}
	log.Debug("Using host for generated details", slog.String("host", host))

	for _, serviceName := range serviceNames {
		// Placeholder: Create one dummy tool per service.
		// A real implementation would iterate through methods discovered via enhanced reflection.
		namespace := sanitizeProtoName(serviceName)
		methodName := "invoke-service" // Dummy method name
		toolName := fmt.Sprintf("%s-%s", namespace, methodName)
		log := log.With(slog.String("service", serviceName), slog.String("tool_name", toolName))

		// Create dummy Schemas - real impl needs message descriptor conversion
		inputSchema := domain.JSONSchemaProps{Type: "object" /*, Description: "Placeholder input - specify parameters as JSON object" */}
		outputSchema := &domain.JSONSchemaProps{Type: "object" /*, Description: "Placeholder output" */}

		tool := domain.Tool{
			Name:         toolName,
			Description:  fmt.Sprintf("Placeholder tool for gRPC service: %s. Full method reflection needed.", serviceName),
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
		}
		tools = append(tools, tool)

		// Create placeholder InvocationDetails based on Connect RPC conventions
		details := usecase.InvocationDetails{
			Type:        "connect_http", // Indicate Connect protocol over HTTP
			Host:        host,           // Assumed from reflection source
			HTTPMethod:  "POST",
			HTTPPath:    fmt.Sprintf("/%s/%s", serviceName, methodName), // Connect path convention
			ContentType: "application/json",                             // Assume JSON encoding for dynamic calls
			// PathParams, QueryParams likely empty for standard Connect unary
			// BodyParam typically empty, invoker sends entire params map as body
		}
		detailsList = append(detailsList, details)
		log.Debug("Generated placeholder tool and details")
	}

	log.Info("Finished generating placeholder gRPC tools", slog.Int("count", len(tools)))
	return tools, detailsList, nil
}

// --- Placeholder for complex conversion logic (convertProtoMessageToJSONSchema, etc.) ---

/*
func convertProtoMessageToJSONSchema(msgDesc *desc.MessageDescriptor) (*domain.JSONSchemaProps, error) {
	if msgDesc == nil {
		return nil, fmt.Errorf("nil message descriptor")
	}

	// This function would need to recursively traverse the fields of the message descriptor,
	// map protobuf types (string, int32, bool, enum, nested messages, repeated fields, maps)
	// to their JSON Schema equivalents (type: string, integer, boolean, object, array).
	// It's a significant task.

	// Example sketch:
	props := domain.JSONSchemaProps{
		Type: "object",
		Properties: make(map[string]domain.JSONSchemaProps),
		Required:   []string{}, // Protobuf doesn't have explicit top-level required, proto3 fields are optional
	}

	for _, fieldDesc := range msgDesc.GetFields() {
		fieldName := fieldDesc.GetName()
		fieldSchema, err := convertProtoFieldToJSONSchema(fieldDesc)
		if err != nil {
			return nil, fmt.Errorf("error converting field %s: %w", fieldName, err)
		}
		props.Properties[fieldName] = *fieldSchema
		// Determine required? Maybe based on field presence rules or annotations?
	}

	return &props, nil
}

func convertProtoFieldToJSONSchema(fieldDesc *desc.FieldDescriptor) (*domain.JSONSchemaProps, error) {
	// Handle fieldDesc.GetType(), fieldDesc.IsRepeated(), fieldDesc.IsMap(), fieldDesc.GetEnumType(), fieldDesc.GetMessageType()
	// Map types: TYPE_DOUBLE -> number, TYPE_STRING -> string, TYPE_MESSAGE -> object/array(map), TYPE_ENUM -> string/integer+enum
	// Handle recursion for nested messages.
	return &domain.JSONSchemaProps{Type: "string"}, nil // Basic placeholder
}
*/

// --- Helpers ---

// sanitizeProtoName sanitizes gRPC/Protobuf service/method names for use in tool names.
func sanitizeProtoName(name string) string {
	name = strings.ToLower(name)
	replacer := strings.NewReplacer(".", "-", "_", "-") // Replace proto separators
	name = replacer.Replace(name)
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	name = strings.Trim(name, "-")
	return name
}
