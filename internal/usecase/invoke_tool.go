package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	// "github.com/i2y/mcpizer/internal/domain" // No longer needed directly here
)

// OpenTelemetry Meter and Tracer
var meter = otel.Meter("mcpizer/usecase")
var tracer = otel.Tracer("mcpizer/usecase")

var (
	// toolInvocationCounter counts tool invocations, labeled by tool name and success status.
	toolInvocationCounter metric.Int64Counter
)

// initMetrics initializes the OpenTelemetry metrics for this package.
// NOTE: This relies on the global MeterProvider being configured elsewhere (e.g., in cmd/main.go).
func initMetrics() {
	var err error
	toolInvocationCounter, err = meter.Int64Counter(
		"mcpizer.tool.invocations",
		metric.WithDescription("Counts the number of tool invocations."),
		metric.WithUnit("{invocation}"),
	)
	if err != nil {
		// Use panic for critical initialization errors. Consider a more robust
		// error handling strategy for production (e.g., log and disable metrics).
		panic(fmt.Sprintf("Failed to create toolInvocationCounter: %v", err))
	}
}

// Call initMetrics on package load.
func init() {
	initMetrics()
}

// InvocationDetails holds the necessary information to call an upstream API corresponding to a tool.
// Supports both HTTP-based calls (including Connect RPC) and native gRPC calls.
type InvocationDetails struct {
	// Type helps the invoker understand the nature of the call (e.g., "http", "connect_http", "grpc").
	Type string `json:"type"`

	// Host is the base URL of the target service (e.g., "http://localhost:8080" or "grpc://localhost:50051").
	Host string `json:"host"`

	// BasePath is the extracted from OpenAPI servers (e.g., "/api/v1").
	BasePath string `json:"base_path,omitempty"`

	// HTTPMethod is the HTTP verb (e.g., "POST", "GET").
	HTTPMethod string `json:"http_method,omitempty"`

	// HTTPPath is the request path (e.g., "/users/{userId}", "/com.example.UserService/GetUser").
	HTTPPath string `json:"http_path,omitempty"`

	// PathParams lists the names of parameters expected to be substituted into the HTTPPath.
	PathParams []string `json:"path_params,omitempty"`

	// QueryParams lists the names of parameters expected to be sent as URL query arguments.
	QueryParams []string `json:"query_params,omitempty"`

	// HeaderParams defines static headers to be included in the request.
	// Dynamic headers (e.g., from tool parameters) might be handled separately by the invoker.
	HeaderParams map[string]string `json:"header_params,omitempty"`

	// BodyParam indicates which single tool input parameter should be marshalled as the HTTP request body.
	// If empty, the request body might be constructed from multiple parameters or be absent.
	BodyParam string `json:"body_param,omitempty"`

	// gRPC specific fields
	// GRPCService is the full service name (e.g., "hello.HelloService")
	GRPCService string `json:"grpc_service,omitempty"`

	// GRPCMethod is the method name (e.g., "SayHello")
	GRPCMethod string `json:"grpc_method,omitempty"`

	// For .proto files: Server is the actual gRPC server endpoint
	Server string `json:"server,omitempty"`

	// For .proto files: Method is the full method path (e.g., "/package.Service/Method")
	Method string `json:"method,omitempty"`

	// For .proto files: Input and Output type names
	InputType  string `json:"input_type,omitempty"`
	OutputType string `json:"output_type,omitempty"`

	// For .proto files: File descriptor for dynamic invocation
	FileDescriptor interface{} `json:"file_descriptor,omitempty"`

	// ContentType indicates the expected Content-Type for the request body (e.g., "application/json").
	// Defaults to application/json if involving a body.
	ContentType string `json:"content_type,omitempty"`

	// TODO: Add authentication details or mechanisms
}

// ToolInvoker defines the contract for executing the actual upstream API call.
// Implementations will handle making HTTP requests (potentially using Connect client).
type ToolInvoker interface {
	Invoke(ctx context.Context, details InvocationDetails, params map[string]interface{}) (interface{}, error)
}

// InvokeToolUseCase handles receiving a tool invocation request and executing it.
type InvokeToolUseCase struct {
	repository ToolRepository
	invoker    ToolInvoker
	logger     *slog.Logger
}

// NewInvokeToolUseCase creates a new InvokeToolUseCase.
func NewInvokeToolUseCase(repo ToolRepository, invoker ToolInvoker, logger *slog.Logger) *InvokeToolUseCase {
	return &InvokeToolUseCase{
		repository: repo,
		invoker:    invoker,
		logger:     logger.With("usecase", "InvokeTool"),
	}
}

// Execute finds the tool, finds invocation details, validates parameters (optional),
// and uses the ToolInvoker to call the upstream API.
func (uc *InvokeToolUseCase) Execute(ctx context.Context, toolName string, params map[string]interface{}) (interface{}, error) {
	// Instrument: Start trace span
	ctx, span := tracer.Start(ctx, "InvokeToolUseCase.Execute", trace.WithAttributes(
		attribute.String("tool.name", toolName),
	))
	defer span.End()

	// Instrument: Record invocation start and defer counter update
	invocationSuccess := false
	defer func() {
		// Record final status on span
		span.SetAttributes(attribute.Bool("success", invocationSuccess))
		toolInvocationCounter.Add(ctx, 1, metric.WithAttributes(
			attribute.String("tool.name", toolName),
			attribute.Bool("success", invocationSuccess),
		))
	}()

	log := uc.logger.With(slog.String("tool_name", toolName))
	log.Info("Executing tool invocation")

	// 1. Find Tool Definition (Optional - for validation)
	// Assign to _ for now, as tool is only used in optional validation below.
	_, err := uc.repository.FindToolByName(ctx, toolName)
	if err != nil {
		if errors.Is(err, ErrToolNotFound) {
			log.Warn("Tool definition not found", slog.Any("error", err))
			span.RecordError(err) // Record specific error on span
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		// Log other unexpected repository errors
		log.Error("Failed to find tool definition", slog.Any("error", err))
		span.RecordError(err) // Record unexpected error on span
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to retrieve definition for tool '%s': %w", toolName, err)
	}

	// 2. Find Invocation Details
	invocationDetails, err := uc.repository.FindInvocationDetailsByName(ctx, toolName)
	if err != nil {
		if errors.Is(err, ErrToolNotFound) {
			// This case should be rare if FindToolByName succeeded, but handle defensively
			log.Warn("Invocation details not found for known tool", slog.Any("error", err))
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		// Log other unexpected repository errors
		log.Error("Failed to find invocation details", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to retrieve invocation details for tool '%s': %w", toolName, err)
	}
	log.Debug("Found invocation details", slog.Any("details", invocationDetails)) // Be careful logging sensitive details

	// 3. Validate Parameters against tool.InputSchema (Optional but recommended)
	// Potential place to use tool.InputSchema loaded earlier
	// log.Debug("Validating input parameters")
	// err = validateInput(tool.InputSchema, params)
	// if err != nil {
	// 	 log.Warn("Invalid input parameters", slog.Any("error", err), slog.Any("params", params))
	// 	 return nil, fmt.Errorf("invalid input parameters for tool %s: %w", toolName, err)
	// }

	// 4. Invoke the upstream service
	log.Info("Invoking upstream service")
	result, err := uc.invoker.Invoke(ctx, *invocationDetails, params)
	if err != nil {
		// TODO: Consider mapping specific invoker errors (e.g., connect.CodeNotFound)
		// to use case errors like ErrUpstreamNotFound or ErrInvocationFailed.
		log.Error("Failed to invoke upstream tool", slog.Any("error", err))
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("failed to invoke tool %s: %w", toolName, err)
	}

	// 5. Validate Output against tool.OutputSchema (Optional)
	// Potential place to use tool.OutputSchema loaded earlier
	// log.Debug("Validating output result")

	// 6. Return result
	log.Info("Tool invocation successful")
	log.Debug("Invocation result", slog.Any("result", result)) // Be careful logging sensitive results
	invocationSuccess = true                                   // Mark success before returning
	span.SetStatus(codes.Ok, "Success")                        // Set span status to OK for success
	return result, nil
}

// Placeholder function for input validation (could live in domain or a helper package)
// func validateInput(schema domain.JSONSchemaProps, params map[string]interface{}) error { ... }
