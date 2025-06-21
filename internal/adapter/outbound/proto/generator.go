package proto

import (
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/i2y/mcpizer/internal/domain"
	"github.com/i2y/mcpizer/internal/usecase"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Generator implements the usecase.ToolGenerator interface for .proto files.
type Generator struct {
	logger *slog.Logger
}

// NewGenerator creates a new Proto Generator.
func NewGenerator(logger *slog.Logger) *Generator {
	return &Generator{
		logger: logger.With("component", "proto_generator"),
	}
}

// Generate creates tool definitions from a parsed .proto schema.
func (g *Generator) Generate(schema domain.APISchema) ([]domain.Tool, []usecase.InvocationDetails, error) {
	log := g.logger.With(slog.String("source", schema.Source))
	log.Info("Generating tools from .proto schema")

	// Extract server URL from ParsedData (temporary solution)
	serverURL := ""
	if parsedData, ok := schema.ParsedData.(map[string]string); ok {
		serverURL = parsedData["server"]
	}
	if serverURL == "" {
		return nil, nil, fmt.Errorf("server URL is required for .proto schemas")
	}

	// Parse the .proto content
	parser := protoparse.Parser{
		Accessor: func(filename string) (io.ReadCloser, error) {
			// For now, we only support single file parsing
			if filename == "schema.proto" {
				return io.NopCloser(strings.NewReader(string(schema.RawData))), nil
			}
			return nil, fmt.Errorf("import not supported: %s", filename)
		},
	}

	fileDescs, err := parser.ParseFiles("schema.proto")
	if err != nil {
		log.Error("Failed to parse .proto file", slog.Any("error", err))
		return nil, nil, fmt.Errorf("failed to parse .proto file: %w", err)
	}

	if len(fileDescs) == 0 {
		return nil, nil, fmt.Errorf("no file descriptors found in .proto file")
	}

	fileDesc := fileDescs[0]
	log.Info("Parsed .proto file", slog.String("package", fileDesc.GetPackage()))

	// Generate tools for each service and method
	var tools []domain.Tool
	var invocationDetails []usecase.InvocationDetails

	for _, service := range fileDesc.GetServices() {
		serviceName := service.GetName()
		log.Debug("Processing service", slog.String("service", serviceName))

		for _, method := range service.GetMethods() {
			methodName := method.GetName()
			fullMethodName := fmt.Sprintf("/%s.%s/%s", fileDesc.GetPackage(), serviceName, methodName)

			// Create tool definition
			tool := domain.Tool{
				Name:        fmt.Sprintf("%s_%s", serviceName, methodName),
				Description: g.generateMethodDescription(method),
				InputSchema: g.generateInputSchema(method),
			}

			// Create invocation details
			details := usecase.InvocationDetails{
				Type:       "grpc",
				Server:     serverURL,
				Method:     fullMethodName,
				InputType:  method.GetInputType().GetFullyQualifiedName(),
				OutputType: method.GetOutputType().GetFullyQualifiedName(),
				// Store the file descriptor for later use by the invoker
				FileDescriptor: fileDesc.AsFileDescriptorProto(),
			}

			tools = append(tools, tool)
			invocationDetails = append(invocationDetails, details)

			log.Debug("Generated tool for method",
				slog.String("tool_name", tool.Name),
				slog.String("method", fullMethodName))
		}
	}

	log.Info("Successfully generated tools from .proto", slog.Int("tool_count", len(tools)))
	return tools, invocationDetails, nil
}

// generateMethodDescription creates a description for a gRPC method.
func (g *Generator) generateMethodDescription(method *desc.MethodDescriptor) string {
	// Get comments if available
	comments := method.GetSourceInfo().GetLeadingComments()
	if comments != "" {
		return strings.TrimSpace(comments)
	}

	// Generate default description
	return fmt.Sprintf("Invokes the gRPC method %s", method.GetName())
}

// generateInputSchema creates a JSON schema for the method's input message.
func (g *Generator) generateInputSchema(method *desc.MethodDescriptor) domain.JSONSchemaProps {
	inputType := method.GetInputType()

	// Generate JSON schema from the protobuf message descriptor
	properties := make(map[string]domain.JSONSchemaProps)
	required := []string{}

	for _, field := range inputType.GetFields() {
		fieldName := field.GetJSONName()
		if fieldName == "" {
			fieldName = field.GetName()
		}

		prop := g.fieldToJSONSchema(field)
		properties[fieldName] = prop

		// In proto3, all fields are optional by default
		// Only mark as required if it has specific field options
		if field.IsRequired() {
			required = append(required, fieldName)
		}
	}

	return domain.JSONSchemaProps{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}

// fieldToJSONSchema converts a protobuf field descriptor to JSON schema.
func (g *Generator) fieldToJSONSchema(field *desc.FieldDescriptor) domain.JSONSchemaProps {
	schema := domain.JSONSchemaProps{}

	// Handle repeated fields
	if field.IsRepeated() {
		schema.Type = "array"
		itemSchema := g.scalarTypeToJSONSchema(field.GetType())
		schema.Items = &itemSchema
		return schema
	}

	// Handle maps
	if field.IsMap() {
		schema.Type = "object"
		// For maps, we use additionalProperties in JSON Schema
		// This is a simplified implementation
		return schema
	}

	// Handle message types
	if field.GetType() == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
		msgType := field.GetMessageType()
		return g.messageToJSONSchema(msgType)
	}

	// Handle scalar types
	return g.scalarTypeToJSONSchema(field.GetType())
}

// scalarTypeToJSONSchema converts protobuf scalar types to JSON schema types.
func (g *Generator) scalarTypeToJSONSchema(protoType descriptorpb.FieldDescriptorProto_Type) domain.JSONSchemaProps {
	switch protoType {
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
		descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		return domain.JSONSchemaProps{Type: "number"}

	case descriptorpb.FieldDescriptorProto_TYPE_INT64,
		descriptorpb.FieldDescriptorProto_TYPE_UINT64,
		descriptorpb.FieldDescriptorProto_TYPE_INT32,
		descriptorpb.FieldDescriptorProto_TYPE_UINT32,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED64,
		descriptorpb.FieldDescriptorProto_TYPE_SINT32,
		descriptorpb.FieldDescriptorProto_TYPE_SINT64:
		return domain.JSONSchemaProps{Type: "integer"}

	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return domain.JSONSchemaProps{Type: "boolean"}

	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		return domain.JSONSchemaProps{Type: "string"}

	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return domain.JSONSchemaProps{
			Type:   "string",
			Format: "byte", // Base64 encoded
		}

	case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		// For enums, we could extract the allowed values
		// For now, treat as string
		return domain.JSONSchemaProps{Type: "string"}

	default:
		// Default to string for unknown types
		return domain.JSONSchemaProps{Type: "string"}
	}
}

// messageToJSONSchema converts a protobuf message descriptor to JSON schema.
func (g *Generator) messageToJSONSchema(msg *desc.MessageDescriptor) domain.JSONSchemaProps {
	properties := make(map[string]domain.JSONSchemaProps)
	required := []string{}

	for _, field := range msg.GetFields() {
		fieldName := field.GetJSONName()
		if fieldName == "" {
			fieldName = field.GetName()
		}

		prop := g.fieldToJSONSchema(field)
		properties[fieldName] = prop

		if field.IsRequired() {
			required = append(required, fieldName)
		}
	}

	return domain.JSONSchemaProps{
		Type:       "object",
		Properties: properties,
		Required:   required,
	}
}
