package openapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Common OpenAPI schema paths used by various frameworks
var commonOpenAPIPaths = []string{
	"/openapi.json",            // FastAPI default
	"/docs/openapi.json",       // Alternative FastAPI path
	"/swagger.json",            // Swagger/OpenAPI 2.0
	"/v3/api-docs",             // SpringDoc OpenAPI 3.0
	"/api-docs",                // SpringFox
	"/api/openapi.json",        // Custom API prefix
	"/api/v1/openapi.json",     // Versioned API
	"/api/swagger.json",        // Alternative swagger path
	"/swagger/v1/swagger.json", // .NET default
	"/_spec",                   // Some Node.js frameworks
	"/spec",                    // Alternative spec path
	"/api-spec.json",           // Custom spec name
}

// AutoDiscoverer attempts to find OpenAPI schemas from base URLs
type AutoDiscoverer struct {
	client *http.Client
	logger *slog.Logger
}

// NewAutoDiscoverer creates a new OpenAPI schema auto-discoverer
func NewAutoDiscoverer(client *http.Client, logger *slog.Logger) *AutoDiscoverer {
	return &AutoDiscoverer{
		client: client,
		logger: logger.With("component", "openapi_autodiscoverer"),
	}
}

// DiscoverSchema attempts to find an OpenAPI schema from a base URL
func (d *AutoDiscoverer) DiscoverSchema(ctx context.Context, baseURL string) (string, error) {
	log := d.logger.With(slog.String("base_url", baseURL))
	log.Info("Attempting to auto-discover OpenAPI schema")

	// Parse and validate base URL
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	// Ensure scheme is present
	if parsedURL.Scheme == "" {
		parsedURL.Scheme = "http"
	}

	// Try each common path
	for _, path := range commonOpenAPIPaths {
		schemaURL := parsedURL.String() + path
		log.Debug("Trying OpenAPI path", slog.String("url", schemaURL))

		if found, err := d.checkOpenAPIEndpoint(ctx, schemaURL); found {
			log.Info("Found OpenAPI schema", slog.String("url", schemaURL))
			return schemaURL, nil
		} else if err != nil {
			log.Debug("Failed to check endpoint",
				slog.String("url", schemaURL),
				slog.Any("error", err))
		}
	}

	// Try to find links in the root page (some services expose discovery links)
	if discoveredURL, err := d.checkRootPageForLinks(ctx, parsedURL.String()); discoveredURL != "" {
		log.Info("Found OpenAPI schema via root page discovery", slog.String("url", discoveredURL))
		return discoveredURL, nil
	} else if err != nil {
		log.Debug("Failed to check root page", slog.Any("error", err))
	}

	return "", fmt.Errorf("could not find OpenAPI schema at %s", baseURL)
}

// checkOpenAPIEndpoint checks if a URL returns a valid OpenAPI schema
func (d *AutoDiscoverer) checkOpenAPIEndpoint(ctx context.Context, schemaURL string) (bool, error) {
	// Create request with timeout
	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", schemaURL, nil)
	if err != nil {
		return false, err
	}

	// Some APIs require specific headers
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MCP-Bridge/1.0")

	resp, err := d.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") &&
		!strings.Contains(contentType, "application/vnd.oai.openapi+json") {
		return false, nil
	}

	// We could parse and validate the JSON here, but for discovery
	// purposes, just checking the response is enough
	return true, nil
}

// checkRootPageForLinks checks the root page for OpenAPI discovery links
func (d *AutoDiscoverer) checkRootPageForLinks(ctx context.Context, baseURL string) (string, error) {
	// This is a simplified implementation
	// A full implementation would parse HTML/JSON for discovery links
	// For now, we'll skip this advanced feature
	return "", nil
}

// ResolveSchemaSource takes a source string and returns a resolved OpenAPI URL
// If the source is already a full OpenAPI URL, it returns it as-is
// If it's a base URL, it attempts auto-discovery
func (d *AutoDiscoverer) ResolveSchemaSource(ctx context.Context, source string) (string, error) {
	log := d.logger.With(slog.String("source", source))

	// Check if it's already a schema URL (ends with .json or contains openapi/swagger)
	lowerSource := strings.ToLower(source)
	if strings.HasSuffix(lowerSource, ".json") ||
		strings.Contains(lowerSource, "openapi") ||
		strings.Contains(lowerSource, "swagger") ||
		strings.Contains(lowerSource, "api-docs") {
		log.Debug("Source appears to be a direct schema URL")
		return source, nil
	}

	// Otherwise, attempt auto-discovery
	log.Info("Source appears to be a base URL, attempting auto-discovery")
	discoveredURL, err := d.DiscoverSchema(ctx, source)
	if err != nil {
		// If auto-discovery fails, return the original source
		// This allows manual schema URLs to still work
		log.Warn("Auto-discovery failed, using original source", slog.Any("error", err))
		return source, nil
	}

	return discoveredURL, nil
}

// ResolveSchemaSourceWithHeaders takes a source string and returns a resolved OpenAPI URL with custom headers
func (d *AutoDiscoverer) ResolveSchemaSourceWithHeaders(ctx context.Context, source string, headers map[string]string) (string, error) {
	log := d.logger.With(slog.String("source", source))

	// Check if it's already a schema URL (ends with .json or contains openapi/swagger)
	lowerSource := strings.ToLower(source)
	if strings.HasSuffix(lowerSource, ".json") ||
		strings.Contains(lowerSource, "openapi") ||
		strings.Contains(lowerSource, "swagger") ||
		strings.Contains(lowerSource, "api-docs") {
		log.Debug("Source appears to be a direct schema URL")
		return source, nil
	}

	// Otherwise, attempt auto-discovery with headers
	log.Info("Source appears to be a base URL, attempting auto-discovery with headers")
	discoveredURL, err := d.DiscoverSchemaWithHeaders(ctx, source, headers)
	if err != nil {
		// If auto-discovery fails, return the original source
		// This allows manual schema URLs to still work
		log.Warn("Auto-discovery failed, using original source", slog.Any("error", err))
		return source, nil
	}

	return discoveredURL, nil
}

// DiscoverSchemaWithHeaders attempts to find an OpenAPI schema from a base URL with custom headers
func (d *AutoDiscoverer) DiscoverSchemaWithHeaders(ctx context.Context, baseURL string, headers map[string]string) (string, error) {
	log := d.logger.With(slog.String("base_url", baseURL))
	log.Info("Attempting to auto-discover OpenAPI schema with headers")

	// Parse and validate base URL
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	// Ensure we have a scheme
	if parsedURL.Scheme == "" {
		return "", fmt.Errorf("base URL must include scheme (http:// or https://)")
	}

	// Try common OpenAPI paths
	for _, path := range commonOpenAPIPaths {
		testURL := strings.TrimRight(baseURL, "/") + path
		log.Debug("Testing OpenAPI path", slog.String("url", testURL))

		if valid, err := d.isValidOpenAPIWithHeaders(ctx, testURL, headers); err != nil {
			log.Debug("Error checking path", slog.String("url", testURL), slog.Any("error", err))
			continue
		} else if valid {
			log.Info("Found OpenAPI schema", slog.String("url", testURL))
			return testURL, nil
		}
	}

	// Try to find discovery links on the root page
	if discoveredURL, err := d.checkRootPageForLinksWithHeaders(ctx, baseURL, headers); err == nil && discoveredURL != "" {
		return discoveredURL, nil
	}

	return "", fmt.Errorf("no OpenAPI schema found at base URL: %s", baseURL)
}

// isValidOpenAPIWithHeaders checks if a URL returns a valid OpenAPI response with custom headers
func (d *AutoDiscoverer) isValidOpenAPIWithHeaders(ctx context.Context, testURL string, headers map[string]string) (bool, error) {
	// Create a timeout context for the probe
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
	if err != nil {
		return false, err
	}

	// Set standard headers
	req.Header.Set("Accept", "application/json, application/vnd.oai.openapi+json")
	req.Header.Set("User-Agent", "MCPizer/1.0")

	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") &&
		!strings.Contains(contentType, "application/vnd.oai.openapi+json") {
		return false, nil
	}

	// We could parse and validate the JSON here, but for discovery
	// purposes, just checking the response is enough
	return true, nil
}

// checkRootPageForLinksWithHeaders checks the root page for OpenAPI discovery links with custom headers
func (d *AutoDiscoverer) checkRootPageForLinksWithHeaders(ctx context.Context, baseURL string, headers map[string]string) (string, error) {
	// This is a simplified implementation
	// A full implementation would parse HTML/JSON for discovery links
	// For now, we'll skip this advanced feature
	return "", nil
}
