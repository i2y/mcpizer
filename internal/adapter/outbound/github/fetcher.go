package github

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/i2y/mcpizer/internal/domain"
	"github.com/i2y/mcpizer/internal/usecase"
)

// Fetcher fetches OpenAPI schemas from GitHub repositories
type Fetcher struct {
	ghClient *GHClient
	logger   *slog.Logger
}

// NewFetcher creates a new GitHub schema fetcher
func NewFetcher(logger *slog.Logger) *Fetcher {
	return &Fetcher{
		ghClient: NewGHClient(),
		logger:   logger.With("component", "github_fetcher"),
	}
}

// Fetch retrieves a schema from a GitHub repository
func (f *Fetcher) Fetch(ctx context.Context, source string) (domain.APISchema, error) {
	log := f.logger.With(slog.String("source", source))

	if !IsGitHubURL(source) {
		return domain.APISchema{}, fmt.Errorf("not a GitHub URL: %s", source)
	}

	// Check if it's a .proto file (handle @ref suffix)
	sourcePath := source
	if idx := strings.Index(source, "@"); idx != -1 {
		sourcePath = source[:idx]
	}
	if strings.HasSuffix(sourcePath, ".proto") {
		log.Info("Fetching .proto file from GitHub")
		
		// Fetch the file content from GitHub
		content, err := f.ghClient.FetchFileRaw(ctx, source)
		if err != nil {
			log.Error("Failed to fetch .proto file from GitHub", slog.Any("error", err))
			return domain.APISchema{}, fmt.Errorf("failed to fetch .proto file from GitHub: %w", err)
		}
		
		log.Info("Successfully fetched .proto file from GitHub", slog.Int("size", len(content)))
		return domain.APISchema{
			Source:     source,
			Type:       domain.SchemaTypeProto,
			RawData:    content,
			ParsedData: nil, // Will be parsed by the proto generator
		}, nil
	}

	log.Info("Fetching OpenAPI schema from GitHub")

	// Fetch the file content from GitHub
	content, err := f.ghClient.FetchFileRaw(ctx, source)
	if err != nil {
		log.Error("Failed to fetch file from GitHub", slog.Any("error", err))
		return domain.APISchema{}, fmt.Errorf("failed to fetch file from GitHub: %w", err)
	}

	// Parse the OpenAPI content
	loader := &openapi3.Loader{Context: ctx, IsExternalRefsAllowed: true}
	doc, parseErr := loader.LoadFromData(content)
	if parseErr != nil {
		log.Error("Failed to parse OpenAPI schema data", slog.Any("error", parseErr))
		return domain.APISchema{}, fmt.Errorf("failed to parse OpenAPI schema from %s: %w", source, parseErr)
	}

	// Validate the schema
	if validateErr := doc.Validate(ctx); validateErr != nil {
		log.Warn("OpenAPI schema validation failed", slog.Any("validation_error", validateErr))
	}

	log.Info("Successfully fetched and parsed OpenAPI schema from GitHub")
	return domain.APISchema{
		Source:     source,
		Type:       domain.SchemaTypeOpenAPI,
		RawData:    content,
		ParsedData: doc,
	}, nil
}

// FetchWithConfig retrieves a schema with additional configuration
func (f *Fetcher) FetchWithConfig(ctx context.Context, config usecase.SchemaSourceConfig) (domain.APISchema, error) {
	// Fetch the schema
	schema, err := f.Fetch(ctx, config.URL)
	if err != nil {
		return schema, err
	}
	
	// If it's a .proto file and has a server, store it in ParsedData
	if schema.Type == domain.SchemaTypeProto && config.Server != "" {
		parsedData := map[string]string{"server": config.Server}
		schema.ParsedData = parsedData
	}
	
	return schema, nil
}

// LoadGitHubConfig loads a configuration file from GitHub
func LoadGitHubConfig(githubURL string) ([]byte, error) {
	if !IsGitHubURL(githubURL) {
		return nil, fmt.Errorf("not a GitHub URL: %s", githubURL)
	}

	client := NewGHClient()
	ctx := context.Background()

	content, err := client.FetchFileRaw(ctx, githubURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config from GitHub: %w", err)
	}

	return content, nil
}

// LoadConfigFromGitHubOrFile loads configuration from either a GitHub URL or local file
func LoadConfigFromGitHubOrFile(path string) (io.Reader, error) {
	if IsGitHubURL(path) {
		content, err := LoadGitHubConfig(path)
		if err != nil {
			return nil, err
		}
		return strings.NewReader(string(content)), nil
	}

	// Regular file path
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	return file, nil
}
