package connect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Invoker implements HTTP-based invocation for Connect-RPC services
type Invoker struct {
	logger     *slog.Logger
	httpClient *http.Client
}

// NewInvoker creates a new Connect-RPC HTTP invoker
func NewInvoker(logger *slog.Logger) *Invoker {
	return &Invoker{
		logger: logger.With("component", "connect_invoker"),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// InvokeHTTP invokes a Connect-RPC method using HTTP/JSON
func (i *Invoker) InvokeHTTP(ctx context.Context, server, fullMethod string, params map[string]interface{}) (interface{}, error) {
	log := i.logger.With(
		slog.String("server", server),
		slog.String("method", fullMethod),
	)
	log.Info("Invoking Connect-RPC method via HTTP")

	// Ensure server URL has proper scheme
	if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
		server = "https://" + server
	}

	// Remove trailing slash from server
	server = strings.TrimSuffix(server, "/")

	// Construct the full URL
	// Connect-RPC uses the pattern: https://server/package.Service/Method
	url := fmt.Sprintf("%s%s", server, fullMethod)

	// Marshal request body
	reqBody, err := json.Marshal(params)
	if err != nil {
		log.Error("Failed to marshal request", slog.Any("error", err))
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		log.Error("Failed to create request", slog.Any("error", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set Connect-RPC headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// Connect protocol version header (optional but recommended)
	req.Header.Set("Connect-Protocol-Version", "1")

	// Send request
	resp, err := i.httpClient.Do(req)
	if err != nil {
		log.Error("Failed to send request", slog.Any("error", err))
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("Failed to read response", slog.Any("error", err))
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for Connect-RPC errors
	// Connect-RPC returns 200 OK even for errors, with error details in the response
	if resp.StatusCode != http.StatusOK {
		log.Error("HTTP error",
			slog.Int("status", resp.StatusCode),
			slog.String("body", string(respBody)),
		)
		return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Error("Failed to unmarshal response", slog.Any("error", err))
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Check for Connect-RPC error in response
	if errObj, ok := result["error"].(map[string]interface{}); ok {
		code := errObj["code"]
		message := errObj["message"]
		log.Error("Connect-RPC error",
			slog.Any("code", code),
			slog.Any("message", message),
		)
		return nil, fmt.Errorf("Connect-RPC error %v: %v", code, message)
	}

	log.Info("Successfully invoked Connect-RPC method", slog.Any("result", result))
	return result, nil
}

// InvokeStreaming handles streaming Connect-RPC calls (future implementation)
// Connect-RPC streaming uses different content types:
// - application/connect+json for JSON streaming
// - application/connect+proto for binary streaming
func (i *Invoker) InvokeStreaming(ctx context.Context, server, fullMethod string, params map[string]interface{}) (interface{}, error) {
	return nil, fmt.Errorf("streaming not yet implemented for Connect-RPC")
}
