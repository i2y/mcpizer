package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/grpcreflect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"github.com/example/calculator/gen/calculator/v1/calculatorv1connect"
	"github.com/example/calculator/internal/service"
)

func main() {
	// Create calculator service
	calcService := &service.CalculatorService{}

	// Create a new mux
	mux := http.NewServeMux()

	// Create Connect handler path
	path, handler := calculatorv1connect.NewCalculatorServiceHandler(calcService)
	mux.Handle(path, handler)

	// Create reflection service
	reflector := grpcreflect.NewStaticReflector(
		"calculator.v1.CalculatorService",
	)

	// Add reflection handlers for both Connect and gRPC protocols
	mux.Handle(grpcreflect.NewHandlerV1(reflector))
	mux.Handle(grpcreflect.NewHandlerV1Alpha(reflector))

	// Add a health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK\n")
	})

	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "50051"
	}

	addr := fmt.Sprintf(":%s", port)

	// Create server with h2c for gRPC support without TLS
	server := &http.Server{
		Addr:    addr,
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}

	// Handle graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start server in a goroutine
	go func() {
		log.Printf("Calculator service starting on %s", addr)
		log.Printf("Reflection enabled for gRPC and Connect clients")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal
	<-ctx.Done()

	// Graceful shutdown
	log.Println("Shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}
