package grpc

import (
	"fmt"
	"hash/fnv"
	"log/slog"
	"strings"

	"github.com/i2y/mcpizer/internal/domain"
	"github.com/i2y/mcpizer/internal/usecase"
	"google.golang.org/protobuf/types/descriptorpb"
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

// Generate creates MCP Tools and InvocationDetails from gRPC service information.
// Now supports full method discovery from ServiceInfo structures.
func (g *ToolGenerator) Generate(schema domain.APISchema) ([]domain.Tool, []usecase.InvocationDetails, error) {
	log := g.logger.With(slog.String("source", schema.Source))
	log.Info("Generating tools from gRPC schema")

	if schema.Type != domain.SchemaTypeGRPC {
		log.Error("Invalid schema type for gRPC generator", slog.String("schema_type", string(schema.Type)))
		return nil, nil, fmt.Errorf("invalid schema type for gRPC generator: %s", schema.Type)
	}

	// Try to parse as ServiceInfo array first (new format)
	serviceInfos, ok := schema.ParsedData.([]ServiceInfo)
	if ok {
		return g.generateFromServiceInfos(schema.Source, serviceInfos)
	}

	// Fall back to legacy string array format
	serviceNames, ok := schema.ParsedData.([]string)
	if ok {
		log.Warn("Using legacy service name format, full method discovery not available")
		return g.generateFromServiceNamesLegacy(schema.Source, serviceNames)
	}

	log.Error("Invalid parsed data format for gRPC schema")
	return nil, nil, fmt.Errorf("invalid parsed data format for gRPC schema: expected []ServiceInfo or []string")
}

// generateFromServiceInfos generates tools from full ServiceInfo structures with method details
func (g *ToolGenerator) generateFromServiceInfos(source string, serviceInfos []ServiceInfo) ([]domain.Tool, []usecase.InvocationDetails, error) {
	var tools []domain.Tool
	var detailsList []usecase.InvocationDetails

	log := g.logger.With(slog.String("source", source))
	log.Info("Generating tools from service infos", slog.Int("service_count", len(serviceInfos)))

	for _, serviceInfo := range serviceInfos {
		for _, method := range serviceInfo.Methods {
			// Skip streaming methods for now (require more complex handling)
			if method.ClientStreaming || method.ServerStreaming {
				log.Warn("Skipping streaming method", 
					slog.String("service", serviceInfo.Name),
					slog.String("method", method.Name))
				continue
			}

			// Generate tool name - keep it simple and short
			// Use only the last part of the service name
			parts := strings.Split(serviceInfo.Name, ".")
			servicePart := parts[len(parts)-1]
			if len(servicePart) > 20 {
				servicePart = servicePart[:20]
			}
			
			methodPart := method.Name
			if len(methodPart) > 20 {
				methodPart = methodPart[:20]
			}
			
			// Create tool name
			toolName := fmt.Sprintf("%s-%s", strings.ToLower(servicePart), strings.ToLower(methodPart))
			
			// Final safety check - ensure it's under 50 chars (well below 64 limit)
			if len(toolName) > 50 {
				h := fnv.New32a()
				h.Write([]byte(serviceInfo.Name + "." + method.Name))
				hash := fmt.Sprintf("%x", h.Sum32()&0xFFFF)
				// Keep first 40 chars and add 5-char hash
				toolName = toolName[:40] + "-" + hash
			}
			
			log.Debug("Generated tool name",
				slog.String("service", serviceInfo.Name),
				slog.String("method", method.Name),
				slog.String("tool_name", toolName),
				slog.Int("length", len(toolName)))

			// Create JSON Schema from protobuf descriptors
			inputSchema := convertProtoToJSONSchema(method.InputDescriptor, method.InputType)
			outputSchema := convertProtoToJSONSchema(method.OutputDescriptor, method.OutputType)
			outputSchemaPtr := &outputSchema

			tool := domain.Tool{
				Name:         toolName,
				Description:  fmt.Sprintf("Calls %s.%s gRPC method", serviceInfo.Name, method.Name),
				InputSchema:  inputSchema,
				OutputSchema: outputSchemaPtr,
			}
			tools = append(tools, tool)

			// Create InvocationDetails for native gRPC
			details := usecase.InvocationDetails{
				Type:        "grpc",
				Host:        source,
				GRPCService: serviceInfo.Name,
				GRPCMethod:  method.Name,
			}
			detailsList = append(detailsList, details)

			log.Debug("Generated tool for gRPC method",
				slog.String("service", serviceInfo.Name),
				slog.String("method", method.Name),
				slog.String("tool_name", toolName))
		}
	}

	log.Info("Finished generating gRPC tools", slog.Int("count", len(tools)))
	return tools, detailsList, nil
}

// generateFromServiceNamesLegacy is the old implementation for backward compatibility
func (g *ToolGenerator) generateFromServiceNamesLegacy(source string, serviceNames []string) ([]domain.Tool, []usecase.InvocationDetails, error) {
	var tools []domain.Tool
	var detailsList []usecase.InvocationDetails

	log := g.logger.With(slog.String("source", source))
	log.Warn("gRPC tool generation is currently a placeholder. Creating dummy tools based on service names only.")

	
	for _, serviceName := range serviceNames {
		// Placeholder: Create one dummy tool per service
		namespace := sanitizeProtoName(serviceName)
		methodName := "invoke"
		toolName := fmt.Sprintf("%s-%s", namespace, methodName)
		
		// Limit tool name length
		if len(toolName) > 64 {
			h := fnv.New32a()
			h.Write([]byte(serviceName))
			toolName = toolName[:60] + fmt.Sprintf("-%x", h.Sum32()&0xFFFF)
		}

		// Create dummy Schemas
		inputSchema := domain.JSONSchemaProps{Type: "object"}
		outputSchema := &domain.JSONSchemaProps{Type: "object"}

		tool := domain.Tool{
			Name:         toolName,
			Description:  fmt.Sprintf("Placeholder tool for gRPC service: %s. Full method reflection needed.", serviceName),
			InputSchema:  inputSchema,
			OutputSchema: outputSchema,
		}
		tools = append(tools, tool)

		// Create InvocationDetails for native gRPC
		details := usecase.InvocationDetails{
			Type:        "grpc",
			Host:        source,
			GRPCService: serviceName,
			GRPCMethod:  "Invoke", // Placeholder method name
		}
		detailsList = append(detailsList, details)
	}

	log.Info("Finished generating placeholder gRPC tools", slog.Int("count", len(tools)))
	return tools, detailsList, nil
}

// convertProtoToJSONSchema converts a protobuf descriptor to JSON Schema
func convertProtoToJSONSchema(descriptor *descriptorpb.DescriptorProto, typeName string) domain.JSONSchemaProps {
	// If no descriptor available, return a basic object schema
	if descriptor == nil {
		return domain.JSONSchemaProps{
			Type: "object",
			// TODO: Add description field to JSONSchemaProps if needed
		}
	}

	// Create properties map for the message fields
	properties := make(map[string]domain.JSONSchemaProps)
	var required []string

	for _, field := range descriptor.Field {
		fieldName := field.GetName()
		fieldSchema := protoFieldToJSONSchema(field)
		
		properties[fieldName] = fieldSchema
		
		// In proto3, all fields are optional by default
		// Only add to required if it has specific annotations (future enhancement)
	}

	return domain.JSONSchemaProps{
		Type:       "object",
		Properties: properties,
		Required:   required,
		// TODO: Add description field to JSONSchemaProps if needed
	}
}

// protoFieldToJSONSchema converts a protobuf field to JSON Schema
func protoFieldToJSONSchema(field *descriptorpb.FieldDescriptorProto) domain.JSONSchemaProps {
	var schema domain.JSONSchemaProps

	// Handle repeated fields
	if field.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		schema.Type = "array"
		itemSchema := protoTypeToJSONSchema(field.GetType())
		schema.Items = &itemSchema
		return schema
	}

	// Handle singular fields
	return protoTypeToJSONSchema(field.GetType())
}

// protoTypeToJSONSchema maps protobuf types to JSON Schema types
func protoTypeToJSONSchema(protoType descriptorpb.FieldDescriptorProto_Type) domain.JSONSchemaProps {
	switch protoType {
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
		descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		return domain.JSONSchemaProps{Type: "number"}
		
	case descriptorpb.FieldDescriptorProto_TYPE_INT64,
		descriptorpb.FieldDescriptorProto_TYPE_UINT64,
		descriptorpb.FieldDescriptorProto_TYPE_INT32,
		descriptorpb.FieldDescriptorProto_TYPE_UINT32,
		descriptorpb.FieldDescriptorProto_TYPE_SINT32,
		descriptorpb.FieldDescriptorProto_TYPE_SINT64,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		return domain.JSONSchemaProps{Type: "integer"}
		
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return domain.JSONSchemaProps{Type: "boolean"}
		
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		return domain.JSONSchemaProps{Type: "string"}
		
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return domain.JSONSchemaProps{
			Type:   "string",
			Format: "byte", // Base64 encoded bytes
		}
		
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE:
		// Nested messages - would need more context to properly convert
		return domain.JSONSchemaProps{Type: "object"}
		
	case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		// Enums - would need enum descriptor for values
		return domain.JSONSchemaProps{Type: "string"}
		
	default:
		return domain.JSONSchemaProps{Type: "string"}
	}
}

// --- Helpers ---

// sanitizeProtoName sanitizes gRPC/Protobuf service/method names for use in tool names.
func sanitizeProtoName(name string) string {
	// For very long service names, just use the last component
	parts := strings.Split(name, ".")
	if len(parts) > 2 {
		// Take only the last 2 parts for namespace
		name = strings.Join(parts[len(parts)-2:], ".")
	}
	
	name = strings.ToLower(name)
	replacer := strings.NewReplacer(".", "-", "_", "-") // Replace proto separators
	name = replacer.Replace(name)
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	name = strings.Trim(name, "-")
	
	// Further limit length
	if len(name) > 30 {
		name = name[:30]
	}
	
	return name
}
