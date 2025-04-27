package usecase

import (
	"context"
	"fmt"
	"log/slog"
	// "mcp-bridge/internal/domain" // No longer needed directly here
)

// InvocationDetails holds the necessary information to call an upstream API corresponding to a tool.
// Tailored for HTTP-based calls, including those following Connect RPC patterns.
type InvocationDetails struct {
	// Type helps the invoker understand the nature of the call (e.g., "http", "connect_http").
	// For now, primarily indicates it's an HTTP-based call.
	Type string `json:"type"`

	// Host is the base URL of the target service (e.g., "http://localhost:8080").
	Host string `json:"host"`

	// HTTPMethod is the HTTP verb (e.g., "POST", "GET").
	HTTPMethod string `json:"http_method"`

	// HTTPPath is the request path (e.g., "/users/{userId}", "/com.example.UserService/GetUser").
	HTTPPath string `json:"http_path"`

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

	// Connect specific details (optional, often derivable from path/headers for standard Connect)
	// ConnectService string `json:"connect_service,omitempty"` // e.g., "com.example.UserService"
	// ConnectMethod string `json:"connect_method,omitempty"`  // e.g., "GetUser"
	// ConnectProtocol string `json:"connect_protocol,omitempty"` // "connect", "grpc", "grpcweb"

	// ContentType indicates the expected Content-Type for the request body (e.g., "application/json").
	// Defaults to application/json if involving a body.
	ContentType string `json:"content_type,omitempty"`

	// TODO: Add authentication details or mechanisms
}

// ToolInvoker defines the contract for executing the actual upstream API call.
// Implementations will handle making HTTP requests (potentially using Connect client).
type ToolInvoker interface {
	Invoke(ctx context.Context, details InvocationDetails, params map[string]interface{}) (map[string]interface{}, error)
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
func (uc *InvokeToolUseCase) Execute(ctx context.Context, toolName string, params map[string]interface{}) (map[string]interface{}, error) {
	log := uc.logger.With(slog.String("tool_name", toolName))
	log.Info("Executing tool invocation")

	// 1. Find Tool Definition (Optional - for validation)
	_, err := uc.repository.FindToolByName(ctx, toolName)
	if err != nil {
		log.Warn("Tool definition not found", slog.Any("error", err))
		return nil, fmt.Errorf("tool '%s' definition not found: %w", toolName, err)
	}

	// 2. Find Invocation Details
	invocationDetails, err := uc.repository.FindInvocationDetailsByName(ctx, toolName)
	if err != nil {
		log.Error("Invocation details not found", slog.Any("error", err))
		return nil, fmt.Errorf("tool '%s' invocation details not found: %w", toolName, err)
	}
	log.Debug("Found invocation details", slog.Any("details", invocationDetails)) // Be careful logging sensitive details

	// 3. Validate Parameters against tool.InputSchema (Optional but recommended)
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
		log.Error("Failed to invoke upstream tool", slog.Any("error", err))
		return nil, fmt.Errorf("failed to invoke tool %s: %w", toolName, err)
	}

	// 5. Validate Output against tool.OutputSchema (Optional)
	// log.Debug("Validating output result")

	// 6. Return result
	log.Info("Tool invocation successful")
	log.Debug("Invocation result", slog.Any("result", result)) // Be careful logging sensitive results
	return result, nil
}

// Placeholder function for input validation (could live in domain or a helper package)
// func validateInput(schema domain.JSONSchemaProps, params map[string]interface{}) error { ... }
