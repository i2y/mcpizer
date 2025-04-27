package connectinvoker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"mcp-bridge/internal/usecase"

	"connectrpc.com/connect"
)

// Invoker implements the usecase.ToolInvoker interface using connect-go.
// It makes dynamic calls assuming JSON encoding.
type Invoker struct {
	httpClient *http.Client
	logger     *slog.Logger
	// We might need specific connect clients per host/service later,
	// but start with a generic HTTP client approach.
}

// New creates a new ConnectInvoker.
func New(httpClient *http.Client, logger *slog.Logger) *Invoker {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Invoker{
		httpClient: httpClient,
		logger:     logger.With("component", "connect_invoker"),
	}
}

// Invoke executes the upstream call based on InvocationDetails.
func (inv *Invoker) Invoke(ctx context.Context, details usecase.InvocationDetails, params map[string]interface{}) (map[string]interface{}, error) {
	log := inv.logger.With(slog.String("method", details.HTTPMethod), slog.String("path", details.HTTPPath), slog.String("host", details.Host))

	// 1. Construct URL
	// Replace path parameters
	replacedPath := details.HTTPPath
	for _, paramName := range details.PathParams {
		paramValue, ok := params[paramName]
		if !ok {
			log.Warn("Missing required path parameter", slog.String("param_name", paramName))
			return nil, fmt.Errorf("missing required path parameter: %s", paramName)
		}
		// TODO: Handle non-string path params appropriately (URL encoding?)
		replacedPath = strings.Replace(replacedPath, fmt.Sprintf("{%s}", paramName), fmt.Sprintf("%v", paramValue), 1)
	}

	// Combine host and path
	baseURL := strings.TrimSuffix(details.Host, "/")
	fullURL := baseURL + replacedPath

	// Add query parameters
	queryParams := url.Values{}
	for _, paramName := range details.QueryParams {
		if paramValue, ok := params[paramName]; ok {
			// TODO: Handle complex query param types (arrays, objects?)
			queryParams.Add(paramName, fmt.Sprintf("%v", paramValue))
		}
	}
	if len(queryParams) > 0 {
		fullURL += "?" + queryParams.Encode()
	}
	log = log.With(slog.String("url", fullURL))

	// 2. Prepare Request Body
	var requestBody io.Reader
	bodyParams := make(map[string]interface{})

	// Collect parameters not used in path or query (potential body/header params)
	usedParams := make(map[string]struct{})
	for _, p := range details.PathParams {
		usedParams[p] = struct{}{}
	}
	for _, p := range details.QueryParams {
		usedParams[p] = struct{}{}
	}

	for name, value := range params {
		if _, used := usedParams[name]; !used {
			bodyParams[name] = value
		}
	}

	if details.BodyParam != "" {
		// Single parameter maps to the entire body
		bodyValue, ok := params[details.BodyParam]
		if !ok {
			// Allow optional body param?
			log.Warn("Specified BodyParam not found in input params", slog.String("body_param", details.BodyParam))
			requestBody = nil // Explicitly nil body
		} else {
			jsonData, err := json.Marshal(bodyValue)
			if err != nil {
				log.Error("Failed to marshal body parameter", slog.String("body_param", details.BodyParam), slog.Any("error", err))
				return nil, fmt.Errorf("failed to marshal body param '%s': %w", details.BodyParam, err)
			}
			requestBody = bytes.NewBuffer(jsonData)
			log.Debug("Prepared request body from single param", slog.String("body_param", details.BodyParam), slog.Int("size", len(jsonData)))
		}
	} else if len(bodyParams) > 0 {
		// Multiple remaining parameters form the body object
		jsonData, err := json.Marshal(bodyParams)
		if err != nil {
			log.Error("Failed to marshal request body parameters", slog.Any("error", err))
			return nil, fmt.Errorf("failed to marshal request body parameters: %w", err)
		}
		requestBody = bytes.NewBuffer(jsonData)
		log.Debug("Prepared request body from multiple params", slog.Any("params", bodyParams), slog.Int("size", len(jsonData)))
	}

	// 3. Create HTTP Request
	req, err := http.NewRequestWithContext(ctx, details.HTTPMethod, fullURL, requestBody)
	if err != nil {
		log.Error("Failed to create http request", slog.Any("error", err))
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	// 4. Set Headers
	// Set Content-Type if body is present
	if requestBody != nil && details.ContentType != "" {
		req.Header.Set("Content-Type", details.ContentType)
	} else if requestBody != nil {
		req.Header.Set("Content-Type", "application/json") // Default if unspecified
	}
	// Add static headers from InvocationDetails
	for key, value := range details.HeaderParams {
		req.Header.Set(key, value)
	}
	// TODO: Add dynamic headers from params if needed?
	// TODO: Add Connect specific headers? (connect-protocol-version, etc.)
	// Typically handled by connect-go client, but check if needed for generic HTTP client.
	log.Debug("Executing HTTP request", slog.Any("headers", req.Header)) // Log headers at debug

	// 5. Execute Request
	// Use the configured HTTP client
	resp, err := inv.httpClient.Do(req)
	if err != nil {
		log.Error("HTTP request failed", slog.Any("error", err))
		return nil, fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()
	log.Debug("Received HTTP response", slog.String("status", resp.Status), slog.Int("status_code", resp.StatusCode))

	// 6. Process Response Body
	responseBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("Failed to read response body", slog.Any("error", err))
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for non-success status codes
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Warn("Received non-success status code", slog.Int("status_code", resp.StatusCode))
		// Try to parse error details (e.g., Connect error format)
		connectErr := &connect.Error{}
		if json.Unmarshal(responseBodyBytes, connectErr) == nil && connectErr.Code() != connect.CodeUnknown {
			log.Warn("Parsed Connect error from response", slog.String("connect_code", connectErr.Code().String()), slog.String("connect_message", connectErr.Message()))
			return nil, connectErr
		}
		// Return generic HTTP error
		log.Warn("Returning generic HTTP error", slog.String("response_body", string(responseBodyBytes)))
		return nil, fmt.Errorf("request failed with status %s: %s", resp.Status, string(responseBodyBytes))
	}

	// Handle empty response body for non-200 success codes (e.g., 204 No Content)
	if len(responseBodyBytes) == 0 {
		log.Debug("Received empty response body for success status code")
		// Represent empty success as an empty map or nil?
		// Empty map seems safer for JSON expectations.
		return make(map[string]interface{}), nil
	}

	// Attempt to unmarshal JSON response
	var result map[string]interface{}
	if err := json.Unmarshal(responseBodyBytes, &result); err != nil {
		log.Error("Failed to unmarshal JSON response", slog.Any("error", err), slog.String("response_body", string(responseBodyBytes)))
		// If response wasn't JSON, return as raw string under a key?
		// Or return error? Returning error seems safer.
		return nil, fmt.Errorf("failed to unmarshal JSON response (status %s): %w. Body: %s", resp.Status, err, string(responseBodyBytes))
	}

	log.Debug("Successfully unmarshalled JSON response")
	return result, nil
}
