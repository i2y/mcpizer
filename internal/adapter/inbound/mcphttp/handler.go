package mcphttp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/i2y/mcpizer/internal/usecase" // Only need SyncSchemaUseCase
)

// Handlers struct holds dependencies for the HTTP handlers.
type Handlers struct {
	syncSchemaUseCase *usecase.SyncSchemaUseCase
	logger            *slog.Logger
}

// NewHandlers creates a new Handlers struct.
func NewHandlers(
	syncUC *usecase.SyncSchemaUseCase,
	logger *slog.Logger,
) *Handlers {
	return &Handlers{
		syncSchemaUseCase: syncUC,
		logger:            logger.With("component", "mcphttp_handler"),
	}
}

// RegisterAdminRoutes sets up the HTTP routes for admin endpoints.
// Renamed from RegisterRoutes for clarity.
func (h *Handlers) RegisterAdminRoutes(mux *http.ServeMux) {
	// Admin/Management Endpoints
	mux.HandleFunc("POST /admin/sync", h.handleSyncSchema)
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

// handleMCP, handleMCPPost, handleMCPGet, acceptsSSE, sendSSEEvent removed as main MCP handling
// will be done by the mcp-go SSE server directly in main.go.
// handleListTools also removed.
