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
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// ServiceInfo contains information about a gRPC service and its methods
type ServiceInfo struct {
	Name    string
	Methods []MethodInfo
}

// MethodInfo contains information about a gRPC method
type MethodInfo struct {
	Name             string
	InputType        string
	OutputType       string
	ClientStreaming  bool
	ServerStreaming  bool
	InputDescriptor  *descriptorpb.DescriptorProto
	OutputDescriptor *descriptorpb.DescriptorProto
}

// FetchWithMethods connects to a gRPC endpoint, uses the reflection service to list services and their methods,
// and stores the service descriptors as ParsedData.
func (f *SchemaFetcher) FetchWithMethods(ctx context.Context, src string) (domain.APISchema, error) {
	log := f.logger.With(slog.String("source", src))
	log.Info("Fetching gRPC schema with methods via reflection")

	// Parse the source - remove grpc:// prefix if present
	target := src
	if strings.HasPrefix(src, "grpc://") {
		target = strings.TrimPrefix(src, "grpc://")
	}

	// Add a timeout to the context for dialing
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, target, f.dialOpts...)
	if err != nil {
		log.Error("Failed to connect to gRPC target", slog.Any("error", err))
		return domain.APISchema{}, fmt.Errorf("failed to connect to gRPC target %s: %w", target, err)
	}
	defer conn.Close()

	// Create reflection client
	refClient := reflectpb.NewServerReflectionClient(conn)

	// Create a reflection stream
	streamCtx, streamCancel := context.WithTimeout(ctx, 30*time.Second)
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
			ListServices: "*",
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

	// Collect service descriptors
	var serviceInfos []ServiceInfo
	for _, service := range serviceResp.Service {
		if service != nil && service.Name != "grpc.reflection.v1alpha.ServerReflection" {
			// Get file descriptor for each service
			log.Debug("Fetching file descriptor for service", slog.String("service", service.Name))

			if err := stream.Send(&reflectpb.ServerReflectionRequest{
				MessageRequest: &reflectpb.ServerReflectionRequest_FileContainingSymbol{
					FileContainingSymbol: service.Name,
				},
			}); err != nil {
				log.Error("Failed to send FileContainingSymbol request",
					slog.String("service", service.Name),
					slog.Any("error", err))
				continue
			}

			resp, err := stream.Recv()
			if err != nil {
				log.Error("Failed to receive FileContainingSymbol response",
					slog.String("service", service.Name),
					slog.Any("error", err))
				continue
			}

			fileResp := resp.GetFileDescriptorResponse()
			if fileResp == nil {
				log.Error("Invalid FileDescriptorResponse", slog.String("service", service.Name))
				continue
			}

			// Parse the file descriptors to extract service methods
			serviceInfo, err := f.parseServiceInfo(service.Name, fileResp.FileDescriptorProto)
			if err != nil {
				log.Error("Failed to parse service info",
					slog.String("service", service.Name),
					slog.Any("error", err))
				continue
			}

			serviceInfos = append(serviceInfos, serviceInfo)
			log.Debug("Successfully parsed service info",
				slog.String("service", service.Name),
				slog.Int("method_count", len(serviceInfo.Methods)))
		}
	}

	log.Info("Successfully fetched gRPC service information",
		slog.Int("service_count", len(serviceInfos)))

	return domain.APISchema{
		Source:     src,
		Type:       domain.SchemaTypeGRPC,
		RawData:    []byte{}, // Raw protobuf descriptors could be stored here if needed
		ParsedData: serviceInfos,
	}, nil
}

// parseServiceInfo extracts service and method information from file descriptors
func (f *SchemaFetcher) parseServiceInfo(serviceName string, fileDescriptorProtos [][]byte) (ServiceInfo, error) {
	var serviceInfo ServiceInfo
	serviceInfo.Name = serviceName

	// Keep track of all message types for method resolution
	messageTypes := make(map[string]*descriptorpb.DescriptorProto)

	for _, fdBytes := range fileDescriptorProtos {
		var fd descriptorpb.FileDescriptorProto
		if err := proto.Unmarshal(fdBytes, &fd); err != nil {
			f.logger.Error("Failed to unmarshal FileDescriptorProto", slog.Any("error", err))
			continue
		}

		// Collect all message types
		for _, msgType := range fd.MessageType {
			fullName := fd.GetPackage() + "." + msgType.GetName()
			messageTypes[fullName] = msgType
		}

		// Find the service
		for _, service := range fd.Service {
			fullServiceName := fd.GetPackage() + "." + service.GetName()
			if fullServiceName == serviceName {
				// Extract method information
				for _, method := range service.Method {
					methodInfo := MethodInfo{
						Name:            method.GetName(),
						InputType:       method.GetInputType(),
						OutputType:      method.GetOutputType(),
						ClientStreaming: method.GetClientStreaming(),
						ServerStreaming: method.GetServerStreaming(),
					}

					// Try to find input/output descriptors
					inputTypeName := strings.TrimPrefix(method.GetInputType(), ".")
					outputTypeName := strings.TrimPrefix(method.GetOutputType(), ".")

					if inputDesc, ok := messageTypes[inputTypeName]; ok {
						methodInfo.InputDescriptor = inputDesc
					}
					if outputDesc, ok := messageTypes[outputTypeName]; ok {
						methodInfo.OutputDescriptor = outputDesc
					}

					serviceInfo.Methods = append(serviceInfo.Methods, methodInfo)
				}
				return serviceInfo, nil
			}
		}
	}

	return serviceInfo, fmt.Errorf("service %s not found in file descriptors", serviceName)
}

// FetchWithConfigAndMethods is the enhanced version of FetchWithConfig
func (f *SchemaFetcher) FetchWithConfigAndMethods(ctx context.Context, config usecase.SchemaSourceConfig) (domain.APISchema, error) {
	log := f.logger.With(slog.String("source", config.URL))
	if len(config.Headers) > 0 {
		log.Warn("gRPC schema fetching does not support custom headers for reflection calls",
			slog.Int("header_count", len(config.Headers)))
	}

	// gRPC reflection doesn't typically require authentication headers
	// For now, we just delegate to the regular FetchWithMethods method
	return f.FetchWithMethods(ctx, config.URL)
}
