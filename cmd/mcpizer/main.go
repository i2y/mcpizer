package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/i2y/mcpizer/configs"
	"github.com/i2y/mcpizer/internal/adapter/inbound/mcphttp"
	"github.com/i2y/mcpizer/internal/adapter/outbound/grpcinvoker"
	"github.com/i2y/mcpizer/internal/adapter/outbound/httpinvoker"
	"github.com/i2y/mcpizer/internal/adapter/outbound/invoker"
	"github.com/i2y/mcpizer/internal/adapter/outbound/openapi"
	"github.com/i2y/mcpizer/internal/domain"
	"github.com/i2y/mcpizer/internal/usecase"

	// Import outbound adapters needed for syncUC
	grpcadapter "github.com/i2y/mcpizer/internal/adapter/outbound/grpc"

	// "github.com/i2y/mcpizer/internal/adapter/inbound/mcphttp" // Replaced by mcp-go server
	// "github.com/i2y/mcpizer/internal/adapter/outbound/httpinvoker" // Not used here anymore

	// "github.com/i2y/mcpizer/internal/adapter/outbound/memrepo" // Not used here anymore

	// mcp-go imports
	// mcp "github.com/mark3labs/mcp-go/mcp" // Not used directly in main yet
	mcpGoServer "github.com/mark3labs/mcp-go/server"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0" // Use appropriate version
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config holds application configuration loaded from environment variables.
type Config struct {
	ListenAddr         string        `envconfig:"LISTEN_ADDR" default:":8080"`
	OpenAPIDefaultHost string        `envconfig:"OPENAPI_DEFAULT_HOST"`
	ShutdownTimeout    time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"5s"`
	SchemaSources      []string      `envconfig:"SCHEMA_SOURCES"`
	// TODO: Add fields for schema_sources, auth_token, log level, client timeouts etc.
	// Example for slice: SchemaSources []string `envconfig:"SCHEMA_SOURCES"` (comma-separated)
	HTTPClientTimeout time.Duration `envconfig:"HTTP_CLIENT_TIMEOUT" default:"10s"`
}

func main() {
	// === Command Line Flags ===
	var transport string
	flag.StringVar(&transport, "transport", "sse", "Transport mode: sse or stdio")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// === Configuration ===
	cfg, err := configs.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// === Logging ===
	logLevel := cfg.ParsedLogLevel() // Use parsed level from config.
	var logger *slog.Logger

	if transport == "stdio" {
		// In STDIO mode, log to file to avoid interfering with stdio communication
		logFile, err := os.OpenFile("/tmp/mcpizer.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			// Fall back to discard if can't open log file
			logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: logLevel}))
		} else {
			logger = slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: logLevel}))
		}
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))
	}

	slog.SetDefault(logger)
	logger.Info("Logger initialized.", slog.String("level", logLevel.String()), slog.String("transport", transport))

	// === OpenTelemetry Initialization ===
	shutdownOtel, err := initOtelProvider(cfg)
	if err != nil {
		logger.Error("Failed to initialize OpenTelemetry.", slog.Any("error", err))
		os.Exit(1)
	}
	defer func() {
		if err := shutdownOtel(context.Background()); err != nil {
			logger.Error("Failed to shutdown OpenTelemetry TracerProvider.", slog.Any("error", err))
		}
	}()
	logger.Info("OpenTelemetry initialized.")

	// === MCP Server (mark3labs/mcp-go) ===
	mcpSrv := mcpGoServer.NewMCPServer(
		"mcpizer", // Server name
		"0.1.0",   // Server version - TODO: Get from build info?
		// TODO: Add server.WithHooks() if needed later
	)
	logger.Info("MCP server (mark3labs/mcp-go) initialized.")

	// === Dependency Injection ===
	logger.Info("Initializing dependencies...")

	// --- HTTP Client (Needed by Invoker & Fetcher) ---
	httpClient := &http.Client{
		Timeout: cfg.HTTPClientTimeout,
	}
	logger.Debug("HTTP Client configured.", slog.Duration("timeout", cfg.HTTPClientTimeout))

	// --- Schema Fetchers (Outbound - Needed by Sync Use Case) ---
	openapiFetcher := openapi.NewSchemaFetcher(httpClient, logger)
	grpcFetcher := grpcadapter.NewSchemaFetcher(logger)
	fetchers := map[domain.SchemaType]usecase.SchemaFetcher{
		domain.SchemaTypeOpenAPI: openapiFetcher,
		domain.SchemaTypeGRPC:    grpcFetcher,
	}
	logger.Debug("Schema fetchers initialized.")

	// --- Tool Generators (Outbound - Needed by Sync Use Case) ---
	openapiGenerator := openapi.NewToolGenerator(logger)
	grpcGenerator := grpcadapter.NewToolGenerator(logger)
	generators := map[domain.SchemaType]usecase.ToolGenerator{
		domain.SchemaTypeOpenAPI: openapiGenerator,
		domain.SchemaTypeGRPC:    grpcGenerator,
	}
	logger.Debug("Tool generators initialized.")

	// --- Tool Invokers (Outbound - Needed by Sync Use Case Tool Handlers) ---
	httpInv := httpinvoker.New(httpClient, logger)
	grpcInv := grpcinvoker.NewInvoker(logger)
	toolInvoker := invoker.NewRouter(httpInv, grpcInv, logger)
	logger.Debug("Tool invokers initialized (HTTP and gRPC with router).")

	// === Use Case (Admin Sync Only for now) ===
	// Pass real dependencies needed for registration and handlers
	// Convert config SchemaSource to usecase SchemaSourceConfig
	sourceConfigs := make([]usecase.SchemaSourceConfig, len(cfg.SchemaSources))
	for i, source := range cfg.SchemaSources {
		sourceConfigs[i] = usecase.SchemaSourceConfig{
			URL:     source.URL,
			Headers: source.Headers,
		}
	}
	syncUC := usecase.NewSyncSchemaUseCase(
		sourceConfigs,
		fetchers,
		generators,
		mcpSrv,      // Pass the mcp-go server instance
		toolInvoker, // Pass the invoker for handlers
		logger,
	)
	// syncUC := usecase.NewSyncSchemaUseCase(cfg.SchemaSources, nil, nil, nil, logger) // Placeholder dependencies - REMOVED

	// === Initial Schema Sync ===
	// Run initial sync synchronously before starting servers
	logger.Info("Performing initial schema synchronization...")
	if err := syncUC.SyncAllConfiguredSources(context.Background()); err != nil {
		logger.Error("Initial schema sync failed. Server startup continuing, but tools may be missing.", slog.Any("error", err))
		// Decide if you want to exit here based on sync failure
		// os.Exit(1)
	} else {
		logger.Info("Initial schema sync completed successfully.")
	}

	// === Transport Mode Selection ===
	switch transport {
	case "stdio":
		logger.Info("Starting in STDIO mode")

		// Create STDIO server
		stdioServer := mcpGoServer.NewStdioServer(mcpSrv)

		// Run STDIO server (blocking)
		if err := stdioServer.Listen(ctx, os.Stdin, os.Stdout); err != nil {
			logger.Error("STDIO server error", slog.Any("error", err))
			os.Exit(1)
		}

	case "sse":
		logger.Info("Starting in SSE mode")

		// === SSE Server Setup (using mcp-go) ===
		// Assumes the mcp-go server handles CORS, headers etc. internally or via options
		sseServer := mcpGoServer.NewSSEServer(mcpSrv, mcpGoServer.WithBaseURL("http://"+cfg.ListenAddr)) // Use configured listen address
		logger.Info("MCP SSE server initialized.", slog.String("address", cfg.ListenAddr))

		// === Admin HTTP Server Setup ===
		adminMux := http.NewServeMux()
		adminHandlers := mcphttp.NewHandlers(syncUC, logger)
		adminHandlers.RegisterAdminRoutes(adminMux) // Register only admin routes
		adminServer := &http.Server{
			Addr:    ":8081", // Run admin on a different port
			Handler: adminMux,
		}
		go func() {
			logger.Info("Admin HTTP server starting.", slog.String("address", adminServer.Addr))
			if err := adminServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("Admin HTTP server failed to start.", slog.Any("error", err))
				// Optionally stop main context if admin server fails
				// stop()
			}
		}()

		// === MCP SSE Server Startup ===
		go func() {
			logger.Info("MCP SSE server starting.", slog.String("address", cfg.ListenAddr))
			if err := sseServer.Start(cfg.ListenAddr); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("MCP SSE server failed to start.", slog.Any("error", err))
				stop() // Trigger shutdown context if main server fails
			}
		}()

		// Wait for interrupt signal.
		<-ctx.Done()

		// === Server Shutdown ===
		logger.Info("Shutting down servers...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		// Shutdown admin server
		if err := adminServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("Admin HTTP server graceful shutdown failed.", slog.Any("error", err))
		}

		// Shutdown SSE server - Check directly for Shutdown method
		if err := sseServer.Shutdown(shutdownCtx); err != nil {
			// Check if the error indicates the method doesn't exist, or if it's a real shutdown error
			// This requires knowing the error handling of mcp-go v0.23.1
			// For now, log it as potentially failing.
			logger.Error("MCP SSE server graceful shutdown failed (or method not implemented)", slog.Any("error", err))
		}

		logger.Info("Servers shut down gracefully.")

	default:
		logger.Error("Invalid transport mode", slog.String("transport", transport))
		os.Exit(1)
	}
}

// initOtelProvider initializes the OpenTelemetry SDK and sets up the OTLP trace exporter.
// It returns a shutdown function to be called on application exit.
func initOtelProvider(cfg *configs.Config) (func(context.Context) error, error) {
	ctx := context.Background()

	if cfg.OtelExporterOtlpEndpoint == "" {
		slog.Info("OTEL_EXPORTER_OTLP_ENDPOINT not set, OpenTelemetry tracing disabled.")
		return func(context.Context) error { return nil }, nil
	}

	slog.Info("Initializing OTLP exporter.", slog.String("endpoint", cfg.OtelExporterOtlpEndpoint))

	// Setup OTLP gRPC connection options.
	grpcOpts := []grpc.DialOption{}
	if cfg.OtelExporterOtlpInsecure {
		grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		slog.Warn("Using insecure connection for OTLP exporter.") // Log warning for insecure.
	} else {
		// TODO: Add logic to load system CAs or custom TLS config for secure connection.
		slog.Info("Using secure connection for OTLP exporter (assuming system CAs). Adjust if needed.")
		// grpcOpts = append(grpcOpts, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, ""))) // Example
	}
	// TODO: Add other grpc.DialOption if needed (e.g., WithBlock).

	conn, err := grpc.NewClient(cfg.OtelExporterOtlpEndpoint, grpcOpts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection to OTLP endpoint: %w", err)
	}

	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithGRPCConn(conn))
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	// Define application resource attributes.
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("mcpizer"),
		),
	)
	if err != nil {
		_ = traceExporter.Shutdown(ctx)
		_ = conn.Close()
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create and set the TracerProvider.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(r),
	)
	otel.SetTracerProvider(tp)

	// Set the global propagator to W3C Trace Context and Baggage.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	slog.Info("OpenTelemetry TracerProvider configured.")

	// Return the shutdown function for the TracerProvider.
	return func(ctx context.Context) error {
		providerErr := tp.Shutdown(ctx)
		connErr := conn.Close()
		return errors.Join(providerErr, connErr)
	}, nil
}

// DummyInvoker removed as we now have a real (connect) invoker
