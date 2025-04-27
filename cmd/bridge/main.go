package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kelseyhightower/envconfig"

	"mcp-bridge/internal/adapter/inbound/mcphttp"
	"mcp-bridge/internal/adapter/outbound/connectinvoker"
	grpcadapter "mcp-bridge/internal/adapter/outbound/grpc"
	"mcp-bridge/internal/adapter/outbound/memrepo"
	"mcp-bridge/internal/adapter/outbound/openapi"
	"mcp-bridge/internal/domain"
	"mcp-bridge/internal/usecase"
	// TODO: Add imports for logging, config, tracing libraries as needed
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	ListenAddr         string        `envconfig:"LISTEN_ADDR" default:":8080"`
	OpenAPIDefaultHost string        `envconfig:"OPENAPI_DEFAULT_HOST"`
	ShutdownTimeout    time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"5s"`
	// TODO: Add fields for schema_sources, auth_token, log level, client timeouts etc.
	// Example for slice: SchemaSources []string `envconfig:"SCHEMA_SOURCES"` (comma-separated)
}

func main() {
	// === Logger Setup (using slog) ===
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)) // Simple JSON logger
	slog.SetDefault(logger)                                 // Set as default for convenience

	// === Configuration (using envconfig) ===
	var cfg Config
	err := envconfig.Process("mcpbridge", &cfg) // Prefix MCPBRIDGE_ is optional
	if err != nil {
		logger.Error("Failed to process configuration", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("Configuration loaded", slog.String("listen_addr", cfg.ListenAddr), slog.String("openapi_default_host", cfg.OpenAPIDefaultHost))

	// === Dependency Injection (Manual) ===
	// In a real app, consider Wire or manual construction in a dedicated function/package.

	// -- Outbound Adapters --

	// Tool Repository
	toolRepo := memrepo.NewInMemoryToolRepository(logger)

	// Schema Fetchers
	// TODO: Configure HttpClient/gRPC options (timeouts, credentials)
	openapiFetcher := openapi.NewSchemaFetcher(nil, logger)
	grpcFetcher := grpcadapter.NewSchemaFetcher(logger) // Pass logger
	fetchers := map[domain.SchemaType]usecase.SchemaFetcher{
		domain.SchemaTypeOpenAPI: openapiFetcher,
		domain.SchemaTypeGRPC:    grpcFetcher,
	}

	// Tool Generators
	openapiGenerator := openapi.NewToolGenerator(cfg.OpenAPIDefaultHost, logger) // Pass logger
	grpcGenerator := grpcadapter.NewToolGenerator(logger)                        // Pass logger
	generators := map[domain.SchemaType]usecase.ToolGenerator{
		domain.SchemaTypeOpenAPI: openapiGenerator,
		domain.SchemaTypeGRPC:    grpcGenerator,
	}

	// Tool Invoker
	// TODO: Configure HttpClient for invoker (timeouts, transport, etc.)
	toolInvoker := connectinvoker.New(nil, logger) // Pass logger

	// -- Use Cases --
	syncUC := usecase.NewSyncSchemaUseCase(fetchers, generators, toolRepo, logger)
	serveUC := usecase.NewServeToolsUseCase(toolRepo, logger)
	invokeUC := usecase.NewInvokeToolUseCase(toolRepo, toolInvoker, logger)

	// -- Inbound Adapters --
	// TODO: Initialize WebSocket adapter when implemented
	httpHandlers := mcphttp.NewHandlers(serveUC, syncUC, invokeUC, logger)

	// === HTTP Server Setup ===
	mux := http.NewServeMux()
	httpHandlers.RegisterRoutes(mux)

	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: mux,
		// TODO: Add timeouts (ReadTimeout, WriteTimeout, IdleTimeout)
	}

	// === Graceful Shutdown ===
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("HTTP server starting", slog.String("address", cfg.ListenAddr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server error", slog.Any("error", err))
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	<-stopChan
	logger.Info("Shutting down server...")

	// Create a context with timeout for shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown error", slog.Any("error", err))
		defer os.Exit(1)
	}

	logger.Info("Server gracefully stopped")
}

// DummyInvoker removed as we now have a real (connect) invoker
