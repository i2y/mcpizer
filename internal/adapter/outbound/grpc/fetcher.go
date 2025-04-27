package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"mcp-bridge/internal/domain"

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
		grpc.WithBlock(), // Wait for connection to be established
	}
	return &SchemaFetcher{
		dialOpts: append(defaultOpts, opts...),
		logger:   logger.With("component", "grpc_fetcher"),
	}
}

// Fetch connects to a gRPC endpoint, uses the reflection service to list services,
// and stores the list as ParsedData. It doesn't fetch detailed message schemas yet.
func (f *SchemaFetcher) Fetch(ctx context.Context, src string) (domain.APISchema, error) {
	log := f.logger.With(slog.String("source", src))
	log.Info("Fetching gRPC schema via reflection")

	// Assume src is the gRPC target address (e.g., "localhost:50051")
	target := src

	// Add a timeout to the context for dialing
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second) // Configurable timeout
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
	streamCtx, streamCancel := context.WithTimeout(ctx, 10*time.Second) // Timeout for reflection calls
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
