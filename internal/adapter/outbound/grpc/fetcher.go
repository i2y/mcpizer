package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/i2y/mcpizer/internal/domain"
	"github.com/i2y/mcpizer/internal/usecase"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

// SchemaFetcher implements the usecase.SchemaFetcher interface for gRPC reflection.
type SchemaFetcher struct {
	// Default dialing options can be customized.
	dialOpts []grpc.DialOption
	logger   *slog.Logger
}

// NewSchemaFetcher creates a new gRPC SchemaFetcher.
func NewSchemaFetcher(logger *slog.Logger, opts ...grpc.DialOption) *SchemaFetcher {
	// Default to insecure for local testing/dev; production needs credentials.
	defaultOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// Removed WithBlock() to allow lazy connection
	}
	return &SchemaFetcher{
		dialOpts: append(defaultOpts, opts...),
		logger:   logger.With("component", "grpc_fetcher"),
	}
}

// Fetch connects to a gRPC endpoint, uses the reflection service to list services and methods.
// This delegates to FetchWithMethods for full implementation.
func (f *SchemaFetcher) Fetch(ctx context.Context, src string) (domain.APISchema, error) {
	// Use the enhanced implementation with method discovery
	return f.FetchWithMethods(ctx, src)
}

// FetchLegacy is the old implementation that only fetches service names
func (f *SchemaFetcher) FetchLegacy(ctx context.Context, src string) (domain.APISchema, error) {
	log := f.logger.With(slog.String("source", src))
	log.Info("Fetching gRPC schema via reflection")

	// Parse the source - remove grpc:// prefix if present
	target := src
	if strings.HasPrefix(src, "grpc://") {
		target = strings.TrimPrefix(src, "grpc://")
	}

	// Add a timeout to the context for dialing
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second) // Increased timeout for external services
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, target, f.dialOpts...)
	if err != nil {
		log.Error("Failed to connect to gRPC target", slog.Any("error", err))
		return domain.APISchema{}, fmt.Errorf("failed to connect to gRPC target %s: %w", target, err)
	}
	defer conn.Close() // Ensure connection is closed

	// Create reflection client
	refClient := reflectpb.NewServerReflectionClient(conn)

	// Create a reflection stream
	streamCtx, streamCancel := context.WithTimeout(ctx, 30*time.Second) // Increased timeout for reflection calls
	defer streamCancel()
	stream, err := refClient.ServerReflectionInfo(streamCtx, grpc.WaitForReady(true))
	if err != nil {
		log.Error("Failed to create reflection stream", slog.Any("error", err))
		return domain.APISchema{}, fmt.Errorf("failed to create reflection stream for %s: %w", target, err)
	}

	// Send ListServices request
	log.Debug("Sending ListServices request")
	if err := stream.Send(&reflectpb.ServerReflectionRequest{
		MessageRequest: &reflectpb.ServerReflectionRequest_ListServices{
			ListServices: "*", // List all services
		},
	}); err != nil {
		log.Error("Failed to send ListServices request", slog.Any("error", err))
		return domain.APISchema{}, fmt.Errorf("failed to send ListServices request to %s: %w", target, err)
	}

	// Receive ListServices response
	resp, err := stream.Recv()
	if err != nil {
		log.Error("Failed to receive ListServices response", slog.Any("error", err))
		return domain.APISchema{}, fmt.Errorf("failed to receive ListServices response from %s: %w", target, err)
	}

	serviceResp := resp.GetListServicesResponse()
	if serviceResp == nil {
		log.Error("Invalid ListServices response received", slog.Any("response", resp))
		return domain.APISchema{}, fmt.Errorf("invalid ListServices response received from %s: %v", target, resp)
	}
	log.Debug("Received ListServices response")

	// Store the service names. We would need more reflection calls (FileContainingSymbol)
	// to get method and message details.
	var serviceNames []string
	for _, service := range serviceResp.Service {
		if service != nil {
			// Skip reflection service itself
			if service.Name != "grpc.reflection.v1alpha.ServerReflection" {
				serviceNames = append(serviceNames, service.Name)
			}
		}
	}
	log.Info("Successfully fetched gRPC service names", slog.Int("service_count", len(serviceNames)))

	// TODO: Implement further reflection calls (FileContainingSymbol for each service)
	// to get method definitions and potentially message schemas (FileDescriptorProto).
	// This requires significant additional logic to parse descriptors.

	// For now, ParsedData just holds the list of service names.
	// The generator will need to handle this limited information or this fetcher needs enhancement.
	return domain.APISchema{
		Source:     src,
		Type:       domain.SchemaTypeGRPC,
		RawData:    []byte(strings.Join(serviceNames, "\n")), // Store service names as raw data for now
		ParsedData: serviceNames,                             // Store the list of service names
	}, nil
}

// FetchWithConfig connects to a gRPC endpoint with custom headers.
// Note: gRPC doesn't use HTTP headers in the same way as REST APIs.
// Headers in gRPC are typically metadata attached to individual RPC calls.
// For schema fetching via reflection, custom headers are usually not needed.
func (f *SchemaFetcher) FetchWithConfig(ctx context.Context, config usecase.SchemaSourceConfig) (domain.APISchema, error) {
	log := f.logger.With(slog.String("source", config.URL))
	if len(config.Headers) > 0 {
		log.Warn("gRPC schema fetching does not support custom headers for reflection calls",
			slog.Int("header_count", len(config.Headers)))
	}

	// gRPC reflection doesn't typically require authentication headers
	// If authentication is needed, it should be configured via DialOptions
	// For now, we just delegate to the regular Fetch method
	return f.Fetch(ctx, config.URL)
}
