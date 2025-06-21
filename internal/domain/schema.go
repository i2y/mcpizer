package domain

// SchemaType defines the type of the source API schema.
type SchemaType string

const (
	SchemaTypeOpenAPI      SchemaType = "openapi"
	SchemaTypeGRPC         SchemaType = "grpc"
	SchemaTypeGitHub       SchemaType = "github"       // GitHub-hosted OpenAPI schemas
	SchemaTypeProto        SchemaType = "proto"        // .proto files
	SchemaTypeConnect      SchemaType = "connect"      // Connect-RPC (HTTP mode)
	SchemaTypeConnectProto SchemaType = "connectproto" // Connect-RPC with .proto file
	// Add other types like GraphQL here if needed later
)

// APISchema represents a fetched API schema before conversion.
// It holds the raw data and metadata about its origin and type.
type APISchema struct {
	// Source indicates the origin of the schema (e.g., URL, file path, gRPC reflection endpoint).
	Source string
	// Type specifies the kind of schema (OpenAPI, gRPC, etc.).
	Type SchemaType
	// RawData holds the unprocessed schema content.
	// For OpenAPI, this could be []byte of JSON/YAML.
	// For gRPC, this might be less relevant if reflection data is processed directly,
	// but could hold service/method names or file descriptors.
	RawData []byte
	// ParsedData holds the schema parsed into a library-specific representation.
	// This avoids reparsing in later stages. The actual type depends on the SchemaType.
	// Example: *openapi3.T for OpenAPI. Use interface{} to keep domain clean,
	// but requires type assertions downstream.
	ParsedData interface{}
}
