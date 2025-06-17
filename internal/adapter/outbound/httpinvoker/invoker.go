package httpinvoker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/i2y/mcpizer/internal/usecase"

)

// Invoker implements the usecase.ToolInvoker interface using standard net/http.
type Invoker struct {
	client *http.Client
	logger *slog.Logger
}

// New creates a new HTTP Invoker.
func New(client *http.Client, logger *slog.Logger) *Invoker {
	if client == nil {
		client = http.DefaultClient
	}
	return &Invoker{
		client: client,
		logger: logger.With("component", "http_invoker"),
	}
}

// Invoke executes the upstream HTTP call based on InvocationDetails and parameters.
func (i *Invoker) Invoke(ctx context.Context, details usecase.InvocationDetails, params map[string]interface{}) (interface{}, error) {
	log := i.logger.With(
		slog.String("method", details.HTTPMethod),
		slog.String("path", details.HTTPPath),
		slog.String("host", details.Host),
	)

	// --- 1. Construct URL with Path Parameters --- //
	baseURL, err := url.Parse(details.Host)
	if err != nil {
		log.Error("Failed to parse host URL", slog.Any("error", err))
		return nil, fmt.Errorf("invalid host URL %s: %w", details.Host, err)
	}
	fullPath := path.Join(details.BasePath, details.HTTPPath)

	processedPath := fullPath
	remainingParams := make(map[string]interface{})
	for k, v := range params {
		placeholder := "{" + k + "}"
		if strings.Contains(processedPath, placeholder) {
			processedPath = strings.ReplaceAll(processedPath, placeholder, fmt.Sprintf("%v", v))
		} else {
			remainingParams[k] = v // Keep params not used in path
		}
	}
	baseURL.Path = processedPath
	finalURL := baseURL.String() // Base URL without query params yet
	log.Debug("Constructed base URL without query params", slog.String("url", finalURL))

	// --- 2. Separate Query Parameters --- //
	query := url.Values{}
	bodyCandidateParams := make(map[string]interface{})
	queryParamsSet := make(map[string]struct{})
	for _, qpName := range details.QueryParams {
		queryParamsSet[qpName] = struct{}{}
	}

	for k, v := range remainingParams {
		if _, isQueryParam := queryParamsSet[k]; isQueryParam {
			// TODO: Handle different types for query params (arrays?)
			query.Add(k, fmt.Sprintf("%v", v))
		} else {
			// Parameters not in path or query are candidates for the body
			bodyCandidateParams[k] = v
		}
	}

	// --- 3. Construct Request Body (only for methods that allow it) --- //
	var requestBody io.Reader
	bodyAllowed := details.HTTPMethod == "POST" || details.HTTPMethod == "PUT" || details.HTTPMethod == "PATCH" // Add other methods if needed

	if bodyAllowed {
		bodyParams := make(map[string]interface{})
		if details.BodyParam == "" {
			// Complex body: Use all body candidate params
			bodyParams = bodyCandidateParams
		} else if bodyVal, ok := bodyCandidateParams[details.BodyParam]; ok {
			// Simple body: A single parameter represents the body.
			// Remove it from bodyCandidates so it's not logged as unused if it's the only one.
			delete(bodyCandidateParams, details.BodyParam)

			if details.ContentType == "application/json" {
				jsonData, err := json.Marshal(bodyVal)
				if err != nil {
					log.Error("Failed to marshal simple request body parameter", slog.String("bodyParam", details.BodyParam), slog.Any("error", err))
					return nil, fmt.Errorf("failed to marshal request body param %s: %w", details.BodyParam, err)
				}
				requestBody = bytes.NewBuffer(jsonData)
			} else {
				// Handle non-JSON simple bodies (e.g., plain text)
				requestBody = strings.NewReader(fmt.Sprintf("%v", bodyVal))
			}
		} else {
			// BodyParam specified but not found in remaining params - treat as empty body?
			log.Warn("Specified BodyParam not found in non-path/query parameters", slog.String("bodyParam", details.BodyParam))
			requestBody = nil
		}

		// Marshal complex body if not handled as simple body
		if requestBody == nil && len(bodyParams) > 0 {
			if details.ContentType == "application/json" {
				jsonData, err := json.Marshal(bodyParams)
				if err != nil {
					log.Error("Failed to marshal complex request body", slog.Any("error", err))
					return nil, fmt.Errorf("failed to marshal request body: %w", err)
				}
				requestBody = bytes.NewBuffer(jsonData)
				log.Debug("Prepared request body from multiple params", slog.Any("params", bodyParams), slog.Int("size", len(jsonData)))
			} else {
				log.Warn("Unsupported complex request body content type", slog.String("contentType", details.ContentType))
				return nil, fmt.Errorf("cannot handle complex body for Content-Type: %s", details.ContentType)
			}
		}
	} else if len(bodyCandidateParams) > 0 {
		// Method doesn't allow body, but there were body candidates left.
		log.Warn("Parameters remain but HTTP method does not support body",
			slog.String("method", details.HTTPMethod),
			slog.Any("remaining_params", bodyCandidateParams))
	}

	// --- 4. Create HTTP Request --- //
	req, err := http.NewRequestWithContext(ctx, details.HTTPMethod, finalURL, requestBody)
	if err != nil {
		log.Error("Failed to create HTTP request", slog.Any("error", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add query parameters to the request URL
	if len(query) > 0 {
		req.URL.RawQuery = query.Encode()
		finalURL = req.URL.String() // Update finalURL for logging
		log.Debug("Added query parameters", slog.String("query", query.Encode()))
	}

	log = log.With(slog.String("url", finalURL)) // Add final URL to subsequent logs

	// Set Content-Type header if there was a request body prepared
	if requestBody != nil && details.ContentType != "" {
		req.Header.Set("Content-Type", details.ContentType)
	}
	
	// Add headers from HeaderParams
	for key, value := range details.HeaderParams {
		req.Header.Set(key, value)
		log.Debug("Added header", slog.String("key", key), slog.String("value", value))
	}

	// --- 5. Execute Request --- //
	log.Debug("Executing HTTP request", slog.Any("headers", req.Header))
	resp, err := i.client.Do(req)
	if err != nil {
		log.Error("HTTP request failed", slog.Any("error", err))
		// Could map to more specific error types if needed
		return nil, fmt.Errorf("request execution failed: %w", err)
	}
	defer resp.Body.Close()

	log = log.With(slog.Int("status_code", resp.StatusCode), slog.String("status", resp.Status))
	log.Debug("Received HTTP response")

	// --- 6. Process Response --- //
	respBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("Failed to read response body", slog.Any("error", err))
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		// Successful response
		var resultData interface{}
		// Attempt to decode JSON if content type indicates it
		if strings.Contains(resp.Header.Get("Content-Type"), "application/json") && len(respBodyBytes) > 0 {
			err := json.Unmarshal(respBodyBytes, &resultData)
			if err != nil {
				log.Warn("Failed to unmarshal JSON response, returning raw body as string", slog.Any("error", err))
				resultData = string(respBodyBytes) // Fallback to string
			} else {
				log.Debug("Successfully unmarshalled JSON response")
			}
		} else {
			// Non-JSON or empty response, return body as string
			resultData = string(respBodyBytes)
			log.Debug("Returning non-JSON response body as string")
		}
		return resultData, nil
	} else {
		// Non-success status code
		log.Warn("Received non-success status code")
		respBodyStr := string(respBodyBytes)
		log.Warn("Returning generic HTTP error", slog.String("response_body", respBodyStr))

		// Return error with status code and response body
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, respBodyStr)
	}
}
