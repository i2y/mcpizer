package grpcinvoker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	reflectpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/grpc/status"
)

// Invoker provides dynamic gRPC method invocation capabilities
type Invoker struct {
	logger      *slog.Logger
	dialOptions []grpc.DialOption
}

// NewInvoker creates a new gRPC invoker
func NewInvoker(logger *slog.Logger) *Invoker {
	return &Invoker{
		logger: logger.With("component", "grpc_invoker"),
		dialOptions: []grpc.DialOption{
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		},
	}
}

// InvokeGRPC dynamically invokes a gRPC method
func (i *Invoker) InvokeGRPC(ctx context.Context, target, service, method string, params map[string]interface{}) (interface{}, error) {
	log := i.logger.With(
		slog.String("target", target),
		slog.String("service", service),
		slog.String("method", method),
	)
	log.Info("Invoking gRPC method")

	// Remove grpc:// prefix if present
	if strings.HasPrefix(target, "grpc://") {
		target = strings.TrimPrefix(target, "grpc://")
	}

	// Connect to the gRPC server
	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(dialCtx, target, i.dialOptions...)
	if err != nil {
		log.Error("Failed to connect to gRPC server", slog.Any("error", err))
		return nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}
	defer conn.Close()

	// Create reflection client to get method descriptors
	refClient := grpcreflect.NewClient(ctx, reflectpb.NewServerReflectionClient(conn))
	defer refClient.Reset()

	// Create descriptor source from server reflection
	descSource := grpcurl.DescriptorSourceFromServer(ctx, refClient)

	// Convert params to JSON for grpcurl
	reqJSON, err := json.Marshal(params)
	if err != nil {
		log.Error("Failed to marshal request params", slog.Any("error", err))
		return nil, fmt.Errorf("failed to marshal request params: %w", err)
	}

	// Create request parser
	reqParser, formatter, err := grpcurl.RequestParserAndFormatter(
		grpcurl.FormatJSON,
		descSource,
		bytes.NewReader(reqJSON),
		grpcurl.FormatOptions{},
	)
	if err != nil {
		log.Error("Failed to create request parser", slog.Any("error", err))
		return nil, fmt.Errorf("failed to create request parser: %w", err)
	}

	// Create a buffer to capture formatted responses
	var respBuf bytes.Buffer
	
	// Create event handler that writes formatted responses
	eventHandler := &grpcurl.DefaultEventHandler{
		Out:       &respBuf,
		Formatter: formatter,
	}

	// Construct the full method name
	fullMethod := fmt.Sprintf("%s/%s", service, method)

	// Invoke the RPC
	err = grpcurl.InvokeRPC(
		ctx,
		descSource,
		conn,
		fullMethod,
		nil, // headers
		eventHandler,
		reqParser.Next,
	)

	if err != nil {
		// Check if it's a gRPC status error
		if st, ok := status.FromError(err); ok {
			log.Error("gRPC call failed", 
				slog.String("code", st.Code().String()),
				slog.String("message", st.Message()),
			)
			return nil, fmt.Errorf("gRPC call failed: %s - %s", st.Code(), st.Message())
		}
		log.Error("Failed to invoke RPC", slog.Any("error", err))
		return nil, fmt.Errorf("failed to invoke RPC: %w", err)
	}

	// Parse the response from the buffer
	respJSON := respBuf.String()
	if respJSON == "" {
		log.Warn("Empty response from gRPC call")
		return nil, nil
	}

	var result interface{}
	if err := json.Unmarshal([]byte(respJSON), &result); err != nil {
		log.Error("Failed to parse response JSON", slog.Any("error", err))
		return nil, fmt.Errorf("failed to parse response JSON: %w", err)
	}

	log.Info("Successfully invoked gRPC method", slog.Any("result", result))
	return result, nil
}

// Helper function to build metadata from headers map
func buildMetadata(headers map[string]string) metadata.MD {
	md := metadata.New(nil)
	for k, v := range headers {
		md.Append(k, v)
	}
	return md
}