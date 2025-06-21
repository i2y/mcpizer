package proto

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/i2y/mcpizer/internal/domain"
	"github.com/i2y/mcpizer/internal/usecase"
)

// SchemaFetcher implements the usecase.SchemaFetcher interface for .proto files.
type SchemaFetcher struct {
	httpClient *http.Client
	logger     *slog.Logger
}

// NewSchemaFetcher creates a new Proto SchemaFetcher.
func NewSchemaFetcher(httpClient *http.Client, logger *slog.Logger) *SchemaFetcher {
	return &SchemaFetcher{
		httpClient: httpClient,
		logger:     logger.With("component", "proto_fetcher"),
	}
}

// Fetch retrieves a .proto file from the given URL.
func (f *SchemaFetcher) Fetch(ctx context.Context, src string) (domain.APISchema, error) {
	log := f.logger.With(slog.String("source", src))
	log.Info("Fetching .proto schema")

	// Validate that the URL ends with .proto
	if !strings.HasSuffix(src, ".proto") {
		return domain.APISchema{}, fmt.Errorf("source must be a .proto file, got: %s", src)
	}

	var data []byte
	var err error

	// Parse URL to check scheme
	parsedURL, err := url.Parse(src)
	if err != nil {
		log.Error("Failed to parse URL", slog.Any("error", err))
		return domain.APISchema{}, fmt.Errorf("failed to parse URL: %w", err)
	}

	// Handle file:// URLs
	if parsedURL.Scheme == "file" {
		filePath := parsedURL.Path
		log.Debug("Reading local file", slog.String("path", filePath))

		data, err = os.ReadFile(filePath)
		if err != nil {
			log.Error("Failed to read local file", slog.Any("error", err))
			return domain.APISchema{}, fmt.Errorf("failed to read local file: %w", err)
		}
	} else {
		// Handle HTTP/HTTPS URLs
		// Create HTTP request with context
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
		if err != nil {
			log.Error("Failed to create HTTP request", slog.Any("error", err))
			return domain.APISchema{}, fmt.Errorf("failed to create request: %w", err)
		}

		// Execute request
		resp, err := f.httpClient.Do(req)
		if err != nil {
			log.Error("Failed to fetch .proto file", slog.Any("error", err))
			return domain.APISchema{}, fmt.Errorf("failed to fetch .proto file: %w", err)
		}
		defer resp.Body.Close()

		// Check status code
		if resp.StatusCode != http.StatusOK {
			log.Error("HTTP request failed", slog.Int("status_code", resp.StatusCode))
			return domain.APISchema{}, fmt.Errorf("HTTP request failed with status %d", resp.StatusCode)
		}

		// Read response body
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			log.Error("Failed to read response body", slog.Any("error", err))
			return domain.APISchema{}, fmt.Errorf("failed to read response body: %w", err)
		}
	}

	log.Info("Successfully fetched .proto file", slog.Int("size", len(data)))

	// Return the schema with raw proto content
	// The ParsedData will be populated by the generator after parsing
	return domain.APISchema{
		Source:     src,
		Type:       domain.SchemaTypeProto,
		RawData:    data,
		ParsedData: nil, // Will be parsed by the generator
	}, nil
}

// FetchWithConfig fetches a .proto file with custom headers.
func (f *SchemaFetcher) FetchWithConfig(ctx context.Context, config usecase.SchemaSourceConfig) (domain.APISchema, error) {
	log := f.logger.With(slog.String("source", config.URL))
	log.Info("Fetching .proto schema with config", slog.Int("header_count", len(config.Headers)))

	// Validate that the URL ends with .proto
	if !strings.HasSuffix(config.URL, ".proto") {
		return domain.APISchema{}, fmt.Errorf("source must be a .proto file, got: %s", config.URL)
	}

	var data []byte
	var err error

	// Parse URL to check scheme
	parsedURL, err := url.Parse(config.URL)
	if err != nil {
		log.Error("Failed to parse URL", slog.Any("error", err))
		return domain.APISchema{}, fmt.Errorf("failed to parse URL: %w", err)
	}

	// Handle file:// URLs
	if parsedURL.Scheme == "file" {
		filePath := parsedURL.Path
		log.Debug("Reading local file", slog.String("path", filePath))

		data, err = os.ReadFile(filePath)
		if err != nil {
			log.Error("Failed to read local file", slog.Any("error", err))
			return domain.APISchema{}, fmt.Errorf("failed to read local file: %w", err)
		}
	} else {
		// Handle HTTP/HTTPS URLs
		// Create HTTP request with context
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.URL, nil)
		if err != nil {
			log.Error("Failed to create HTTP request", slog.Any("error", err))
			return domain.APISchema{}, fmt.Errorf("failed to create request: %w", err)
		}

		// Add custom headers
		for key, value := range config.Headers {
			req.Header.Set(key, value)
		}

		// Execute request
		resp, err := f.httpClient.Do(req)
		if err != nil {
			log.Error("Failed to fetch .proto file", slog.Any("error", err))
			return domain.APISchema{}, fmt.Errorf("failed to fetch .proto file: %w", err)
		}
		defer resp.Body.Close()

		// Check status code
		if resp.StatusCode != http.StatusOK {
			log.Error("HTTP request failed", slog.Int("status_code", resp.StatusCode))
			// Read error body if available
			errorBody, _ := io.ReadAll(resp.Body)
			if len(errorBody) > 0 {
				log.Error("Error response body", slog.String("body", string(errorBody)))
			}
			return domain.APISchema{}, fmt.Errorf("HTTP request failed with status %d", resp.StatusCode)
		}

		// Read response body
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			log.Error("Failed to read response body", slog.Any("error", err))
			return domain.APISchema{}, fmt.Errorf("failed to read response body: %w", err)
		}
	}

	log.Info("Successfully fetched .proto file with config", slog.Int("size", len(data)))

	// Return the schema with raw proto content
	// Note: For .proto files, we need the server field from config
	// Store the server in ParsedData for now (will be properly structured later)
	parsedData := map[string]string{}
	if config.Server != "" {
		parsedData["server"] = config.Server
	}
	if config.Mode != "" {
		parsedData["mode"] = config.Mode
	}

	// Determine schema type based on configuration
	schemaType := domain.SchemaTypeProto
	if config.Type == "connect" {
		schemaType = domain.SchemaTypeConnectProto
	}

	return domain.APISchema{
		Source:     config.URL,
		Type:       schemaType,
		RawData:    data,
		ParsedData: parsedData,
	}, nil
}
