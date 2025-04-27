package openapi

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"

	"mcp-bridge/internal/domain"

	"github.com/getkin/kin-openapi/openapi3"
)

// SchemaFetcher implements the usecase.SchemaFetcher interface for OpenAPI schemas.
type SchemaFetcher struct {
	httpClient *http.Client
	logger     *slog.Logger
}

// NewSchemaFetcher creates a new OpenAPI SchemaFetcher.
func NewSchemaFetcher(client *http.Client, logger *slog.Logger) *SchemaFetcher {
	if client == nil {
		client = http.DefaultClient
	}
	return &SchemaFetcher{
		httpClient: client,
		logger:     logger.With("component", "openapi_fetcher"),
	}
}

// Fetch loads an OpenAPI schema from a URL or local file path.
func (f *SchemaFetcher) Fetch(ctx context.Context, src string) (domain.APISchema, error) {
	log := f.logger.With(slog.String("source", src))
	log.Info("Fetching OpenAPI schema")

	loader := &openapi3.Loader{Context: ctx, IsExternalRefsAllowed: true}

	var doc *openapi3.T
	var err error
	var rawData []byte

	u, parseErr := url.ParseRequestURI(src)

	if parseErr == nil && (u.Scheme == "http" || u.Scheme == "https") {
		log.Debug("Fetching from URL")
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
		if reqErr != nil {
			log.Error("Failed to create HTTP request", slog.Any("error", reqErr))
			return domain.APISchema{}, fmt.Errorf("failed to create request for %s: %w", src, reqErr)
		}
		resp, httpErr := f.httpClient.Do(req)
		if httpErr != nil {
			log.Error("Failed to fetch schema from URL", slog.Any("error", httpErr))
			return domain.APISchema{}, fmt.Errorf("failed to fetch schema from URL %s: %w", src, httpErr)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Warn("Received non-OK status code from URL", slog.String("status", resp.Status), slog.Int("status_code", resp.StatusCode))
			return domain.APISchema{}, fmt.Errorf("failed to fetch schema from URL %s: status %s", src, resp.Status)
		}

		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			log.Error("Failed to read response body from URL", slog.Any("error", readErr))
			return domain.APISchema{}, fmt.Errorf("failed to read response body from %s: %w", src, readErr)
		}
		rawData = bodyBytes
		doc, err = loader.LoadFromData(rawData)

	} else {
		log.Debug("Assuming local file path")
		fileData, readErr := os.ReadFile(src)
		if readErr != nil {
			// Log specific error based on whether it looked like a URL initially
			if parseErr == nil {
				log.Error("Source looked like URL but fetch failed, and file read also failed", slog.Any("file_read_error", readErr))
				return domain.APISchema{}, fmt.Errorf("source '%s' is a URL but failed to fetch, and is not a local file either: %w", src, readErr)
			} else {
				log.Error("Failed to read schema from file", slog.Any("error", readErr))
				return domain.APISchema{}, fmt.Errorf("failed to read schema from file %s: %w", src, readErr)
			}
		}
		rawData = fileData
		doc, err = loader.LoadFromData(rawData)
	}

	if err != nil {
		log.Error("Failed to parse OpenAPI schema data", slog.Any("error", err))
		return domain.APISchema{}, fmt.Errorf("failed to parse OpenAPI schema from %s: %w", src, err)
	}

	if validateErr := doc.Validate(ctx); validateErr != nil {
		log.Warn("OpenAPI schema validation failed", slog.Any("validation_error", validateErr))
	}

	log.Info("Successfully fetched and parsed OpenAPI schema")
	return domain.APISchema{
		Source:     src,
		Type:       domain.SchemaTypeOpenAPI,
		RawData:    rawData,
		ParsedData: doc,
	}, nil
}
