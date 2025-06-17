package invoker

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/i2y/mcpizer/internal/adapter/outbound/httpinvoker"
	"github.com/i2y/mcpizer/internal/adapter/outbound/grpcinvoker"
	"github.com/i2y/mcpizer/internal/usecase"
)

// Router implements usecase.ToolInvoker and routes invocations based on the Type field
type Router struct {
	httpInvoker *httpinvoker.Invoker
	grpcInvoker    *grpcinvoker.Invoker
	logger         *slog.Logger
}

// NewRouter creates a new invoker router
func NewRouter(httpInv *httpinvoker.Invoker, grpcInv *grpcinvoker.Invoker, logger *slog.Logger) *Router {
	return &Router{
		httpInvoker: httpInv,
		grpcInvoker:    grpcInv,
		logger:         logger.With("component", "invoker_router"),
	}
}

// Invoke routes the invocation to the appropriate invoker based on the details.Type
func (r *Router) Invoke(ctx context.Context, details usecase.InvocationDetails, params map[string]interface{}) (interface{}, error) {
	log := r.logger.With(slog.String("type", details.Type))
	
	switch details.Type {
	case "grpc":
		log.Info("Routing to gRPC invoker")
		return r.grpcInvoker.InvokeGRPC(ctx, details.Host, details.GRPCService, details.GRPCMethod, params)
		
	case "http", "":
		log.Info("Routing to HTTP invoker")
		return r.httpInvoker.Invoke(ctx, details, params)
		
	default:
		log.Error("Unknown invocation type", slog.String("type", details.Type))
		return nil, fmt.Errorf("unknown invocation type: %s", details.Type)
	}
}
