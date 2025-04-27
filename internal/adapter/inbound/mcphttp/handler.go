package mcphttp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"mcp-bridge/internal/domain"
	"mcp-bridge/internal/usecase"
	"mcp-bridge/pkg/shared/mcpjsonrpc"
	// "github.com/gorilla/websocket" // No longer needed
)

// Handlers struct holds dependencies for the HTTP handlers, like use cases.
type Handlers struct {
	serveToolsUseCase *usecase.ServeToolsUseCase
	syncSchemaUseCase *usecase.SyncSchemaUseCase
	invokeToolUseCase *usecase.InvokeToolUseCase // Needed for WebSocket handler
	logger            *slog.Logger
	// Add logger, config, etc. as needed
}

// NewHandlers creates a new Handlers struct.
func NewHandlers(
	serveUC *usecase.ServeToolsUseCase,
	syncUC *usecase.SyncSchemaUseCase,
	invokeUC *usecase.InvokeToolUseCase,
	logger *slog.Logger,
) *Handlers {
	return &Handlers{
		serveToolsUseCase: serveUC,
		syncSchemaUseCase: syncUC,
		invokeToolUseCase: invokeUC,
		logger:            logger.With("component", "mcphttp_handler"), // Add component context
	}
}

// RegisterRoutes sets up the HTTP routes using the standard library mux.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	// MCP Endpoints
	mux.HandleFunc("GET /mcp/v1/tools", h.handleListTools)
	mux.HandleFunc("/mcp", h.handleMCP) // Handles POST (invoke) and GET (listen-only)

	// Admin/Management Endpoints
	mux.HandleFunc("POST /admin/sync", h.handleSyncSchema)
}

// handleListTools implements GET /mcp/v1/tools
func (h *Handlers) handleListTools(w http.ResponseWriter, r *http.Request) {
	// Ensure it's a GET request (though handled by ServeMux registration pattern)
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	tools, err := h.serveToolsUseCase.Execute(r.Context())
	if err != nil {
		h.logger.Error("Failed to list tools", slog.Any("error", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// If no tools, return empty list, not error
	if tools == nil {
		tools = []domain.Tool{}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(tools); err != nil {
		h.logger.Error("Failed to encode tools response", slog.Any("error", err))
		// Hard to send error response here as headers/status might be sent
	}
}

// SyncRequest defines the expected JSON body for the /admin/sync endpoint.
type SyncRequest struct {
	Source string `json:"source"`
}

// handleSyncSchema implements POST /admin/sync
func (h *Handlers) handleSyncSchema(w http.ResponseWriter, r *http.Request) {
	// Ensure it's a POST request
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("Failed to decode sync request body", slog.Any("error", err))
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if req.Source == "" {
		h.logger.Warn("Sync request missing source field")
		http.Error(w, "Missing 'source' field in request body", http.StatusBadRequest)
		return
	}

	h.logger.Info("Received sync request", slog.String("source", req.Source))
	if err := h.syncSchemaUseCase.Execute(r.Context(), req.Source); err != nil {
		h.logger.Error("Failed to sync schema", slog.String("source", req.Source), slog.Any("error", err))
		// Determine appropriate status code based on error type?
		http.Error(w, fmt.Sprintf("Failed to sync schema: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted) // Accepted for processing, as sync might take time
	fmt.Fprintf(w, "Sync request accepted for source: %s\n", req.Source)
	h.logger.Info("Sync request accepted", slog.String("source", req.Source))
}

// handleMCP implements the unified /mcp endpoint for Streamable HTTP (MCP Spec 2025-03-26).
func (h *Handlers) handleMCP(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement Origin check
	// TODO: Handle Mcp-Session-Id header if needed

	switch r.Method {
	case http.MethodPost:
		h.handleMCPPost(w, r)
	case http.MethodGet:
		h.handleMCPGet(w, r)
	default:
		h.logger.Warn("Method not allowed for /mcp endpoint", slog.String("method", r.Method))
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleMCPPost handles tool invocation requests.
func (h *Handlers) handleMCPPost(w http.ResponseWriter, r *http.Request) {
	// Add request-specific logging context if possible (e.g., request ID middleware)
	log := h.logger

	var req mcpjsonrpc.Request
	var reqID interface{} // Store the ID for responses
	var responseSent bool = false

	// Helper to send JSON-RPC Error Response (handles both SSE and single JSON)
	sendErrorResponse := func(code int, message string, errData interface{}, id interface{}) {
		if responseSent {
			return
		} // Avoid sending multiple responses for one request
		log.Warn("Sending JSON-RPC error response", slog.Int("code", code), slog.String("message", message), slog.Any("data", errData), slog.Any("req_id", id))
		errResp := mcpjsonrpc.Response{
			Version: "2.0",
			Error: &mcpjsonrpc.Error{
				Code:    code,
				Message: message,
				Data:    errData,
			},
			ID: id, // Use original request ID
		}
		if acceptsSSE(r) {
			sendSSEEvent(w, log, uuid.NewString(), errResp) // Pass logger
		} else {
			w.Header().Set("Content-Type", "application/json")
			httpStatus := http.StatusInternalServerError
			switch code {
			case mcpjsonrpc.CodeParseError, mcpjsonrpc.CodeInvalidRequest, mcpjsonrpc.CodeInvalidParams:
				httpStatus = http.StatusBadRequest
			case mcpjsonrpc.CodeMethodNotFound, mcpjsonrpc.CodeServerErrorToolNotFound:
				httpStatus = http.StatusNotFound
			}
			w.WriteHeader(httpStatus)
			if err := json.NewEncoder(w).Encode(errResp); err != nil {
				log.Error("Failed to encode JSON error response", slog.Any("error", err))
			}
		}
		responseSent = true
	}

	// 1. Parse Request Body as JSON-RPC
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendErrorResponse(mcpjsonrpc.CodeParseError, "Failed to parse JSON request", err.Error(), nil)
		return
	}
	defer r.Body.Close()
	reqID = req.ID                            // Store ID
	log = log.With(slog.Any("req_id", reqID)) // Add request ID to logger context

	log.Debug("Received MCP POST request", slog.Any("request", req)) // Log full request at debug?

	// Basic JSON-RPC Validation
	if req.Version != "2.0" || req.Method == "" {
		sendErrorResponse(mcpjsonrpc.CodeInvalidRequest, "Invalid JSON-RPC request structure", nil, reqID)
		return
	}

	// Check for expected method
	if req.Method != "invokeTool" {
		sendErrorResponse(mcpjsonrpc.CodeMethodNotFound, fmt.Sprintf("Method '%s' not supported", req.Method), nil, reqID)
		return
	}

	// 2. Parse invokeTool parameters
	paramsBytes, _ := json.Marshal(req.Params) // Re-marshal to decode into specific struct
	var invokeParams mcpjsonrpc.InvokeToolParams
	if err := json.Unmarshal(paramsBytes, &invokeParams); err != nil {
		sendErrorResponse(mcpjsonrpc.CodeInvalidParams, "Invalid parameters for invokeTool", err.Error(), reqID)
		return
	}
	if invokeParams.ToolName == "" {
		sendErrorResponse(mcpjsonrpc.CodeInvalidParams, "Missing toolName in parameters", nil, reqID)
		return
	}
	if invokeParams.Parameters == nil {
		invokeParams.Parameters = make(map[string]interface{})
	}

	// 3. Execute Use Case
	log.Info("Invoking tool", slog.String("toolName", invokeParams.ToolName))
	result, err := h.invokeToolUseCase.Execute(r.Context(), invokeParams.ToolName, invokeParams.Parameters)
	if err != nil {
		log.Error("Tool invocation failed", slog.String("toolName", invokeParams.ToolName), slog.Any("error", err))
		sendErrorResponse(mcpjsonrpc.CodeServerErrorToolFailed, "Tool invocation failed", err.Error(), reqID)
		return
	}

	// 4. Send Response (SSE or single JSON)
	log.Info("Tool invocation successful", slog.String("toolName", invokeParams.ToolName))
	successResp := mcpjsonrpc.Response{
		Version: "2.0",
		Result:  result,
		ID:      reqID,
	}

	if acceptsSSE(r) {
		log.Debug("Sending response via SSE")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)                // Send headers before flushing
		streamID := uuid.NewString()                // Generate ID for the stream/response event
		sendSSEEvent(w, log, streamID, successResp) // Pass logger
		// TODO: If use case could stream multiple results, loop here
	} else {
		log.Debug("Sending response via single JSON")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(successResp); err != nil {
			log.Error("Failed to encode JSON success response", slog.Any("error", err))
		}
	}
	responseSent = true
}

// handleMCPGet handles listen-only stream requests (placeholder).
func (h *Handlers) handleMCPGet(w http.ResponseWriter, r *http.Request) {
	log := h.logger.With("handler", "handleMCPGet")
	if !acceptsSSE(r) {
		log.Warn("GET request missing Accept header for SSE")
		http.Error(w, "GET requires Accept: text/event-stream", http.StatusNotAcceptable)
		return
	}

	log.Info("Listen-only SSE client connected")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	// Placeholder: Send a greeting and close, or subscribe to a real notification source.
	sendSSEEvent(w, log, uuid.NewString(), map[string]interface{}{ // Send map as data
		"jsonrpc": "2.0",
		"method":  "serverNotification",
		"params": map[string]string{
			"message": "Listen-only stream connected. No notifications implemented yet.",
		},
	})

	// Keep connection open until client disconnects or context is cancelled
	<-r.Context().Done()
	log.Info("Listen-only SSE client disconnected")
}

// acceptsSSE checks if the client accepts text/event-stream.
func acceptsSSE(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

// sendSSEEvent formats and writes a JSON-RPC response as an SSE event.
func sendSSEEvent(w http.ResponseWriter, logger *slog.Logger, eventID string, data interface{}) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		logger.Error("Streaming unsupported: Flusher interface not available in sendSSEEvent")
		return // Cannot proceed
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		logger.Error("Failed to marshal SSE data", slog.Any("error", err))
		// Attempt to send a simple error message? Careful not to double-encode JSON.
		fmt.Fprintf(w, "id: %s\ndata: {\"jsonrpc\":\"2.0\",\"error\":{\"code\":%d,\"message\":\"internal server error - failed to marshal response\"}}\n\n",
			eventID, mcpjsonrpc.CodeInternalError)
		flusher.Flush()
		return
	}

	logger.Debug("Sending SSE event", slog.String("event_id", eventID), slog.String("data", string(jsonData)))
	fmt.Fprintf(w, "id: %s\ndata: %s\n\n", eventID, string(jsonData))
	flusher.Flush() // Send the event immediately
}
