package invoker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/i2y/mcpizer/internal/adapter/outbound/connect"
	"github.com/i2y/mcpizer/internal/adapter/outbound/grpcinvoker"
	"github.com/i2y/mcpizer/internal/adapter/outbound/httpinvoker"
	"github.com/i2y/mcpizer/internal/usecase"
)

// Router implements usecase.ToolInvoker and routes invocations based on the Type field
type Router struct {
	httpInvoker    *httpinvoker.Invoker
	grpcInvoker    *grpcinvoker.Invoker
	connectInvoker *connect.Invoker
	logger         *slog.Logger
}

// NewRouter creates a new invoker router
func NewRouter(httpInv *httpinvoker.Invoker, grpcInv *grpcinvoker.Invoker, connectInv *connect.Invoker, logger *slog.Logger) *Router {
	return &Router{
		httpInvoker:    httpInv,
		grpcInvoker:    grpcInv,
		connectInvoker: connectInv,
		logger:         logger.With("component", "invoker_router"),
	}
}

// Invoke routes the invocation to the appropriate invoker based on the details.Type
func (r *Router) Invoke(ctx context.Context, details usecase.InvocationDetails, params map[string]interface{}) (interface{}, error) {
	log := r.logger.With(slog.String("type", details.Type))

	switch details.Type {
	case "grpc":
		log.Info("Routing to gRPC invoker")
		// Use Server field if available (for .proto files), otherwise fall back to Host
		target := details.Host
		if details.Server != "" {
			target = details.Server
		}
		// Use Method field if available (for .proto files), otherwise use GRPCService/GRPCMethod
		if details.Method != "" {
			// Method already contains the full path like /package.Service/Method
			// Extract service and method parts
			parts := strings.Split(details.Method, "/")
			if len(parts) >= 3 {
				// parts[0] is empty, parts[1] is package.Service, parts[2] is Method
				// parts[1] contains the full service name like "package.Service"
				method := parts[2]
				return r.grpcInvoker.InvokeGRPC(ctx, target, parts[1], method, params)
			}
		}
		return r.grpcInvoker.InvokeGRPC(ctx, target, details.GRPCService, details.GRPCMethod, params)

	case "connect":
		log.Info("Routing to Connect-RPC invoker")
		// Use Server field for the Connect-RPC server URL
		server := details.Host
		if details.Server != "" {
			server = details.Server
		}
		// Method contains the full path like /package.Service/Method
		return r.connectInvoker.InvokeHTTP(ctx, server, details.Method, params)

	case "http", "":
		log.Info("Routing to HTTP invoker")
		return r.httpInvoker.Invoke(ctx, details, params)

	default:
		log.Error("Unknown invocation type", slog.String("type", details.Type))
		return nil, fmt.Errorf("unknown invocation type: %s", details.Type)
	}
}
