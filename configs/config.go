package configs

import (
	"fmt"
	"log/slog"
	"os" // Added for file reading
	"strings"
	"time"

	"github.com/i2y/mcpizer/internal/adapter/outbound/github"
	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v3" // Added YAML parser
)

// SchemaSource represents a single schema source with optional headers
type SchemaSource struct {
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Server  string            `yaml:"server,omitempty"` // For .proto files, the gRPC server endpoint
	Type    string            `yaml:"type,omitempty"`   // Schema type override (e.g., "connect" for Connect-RPC)
	Mode    string            `yaml:"mode,omitempty"`   // Invocation mode (e.g., "http" or "grpc" for Connect-RPC)
}

// FileConfig defines the structure loaded from the YAML configuration file.
type FileConfig struct {
	SchemaSources []interface{} `yaml:"schema_sources"`
	// Add other file-configurable fields here, e.g.:
	// DefaultOpenAPIHost string `yaml:"default_openapi_host"`
}

// Config holds the final application configuration, merged from file and environment variables.
// Fields are loaded from environment variables with the prefix "MCPIZER_", potentially overriding file settings.
type Config struct {
	// Config File Path (Loaded first from env)
	ConfigFilePath string `envconfig:"CONFIG_FILE" default:"configs/mcpizer.yaml"`

	// File-loaded fields (merged)
	SchemaSources []SchemaSource // Loaded from FileConfig

	// Environment-overridable fields
	ListenAddr               string        `envconfig:"LISTEN_ADDR" default:":8080"`
	HTTPClientTimeout        time.Duration `envconfig:"HTTP_CLIENT_TIMEOUT" default:"30s"`
	ShutdownTimeout          time.Duration `envconfig:"SHUTDOWN_TIMEOUT" default:"5s"`
	ServerReadTimeout        time.Duration `envconfig:"SERVER_READ_TIMEOUT" default:"5s"`
	ServerWriteTimeout       time.Duration `envconfig:"SERVER_WRITE_TIMEOUT" default:"10s"`
	ServerIdleTimeout        time.Duration `envconfig:"SERVER_IDLE_TIMEOUT" default:"120s"`
	OtelExporterOtlpEndpoint string        `envconfig:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	OtelExporterOtlpInsecure bool          `envconfig:"OTEL_EXPORTER_OTLP_INSECURE" default:"true"`
	LogLevel                 string        `envconfig:"LOG_LEVEL" default:"info"`

	// TODO: Add fields for SchemaSources, AuthToken etc.
}

// ParsedLogLevel returns the slog.Level based on the configured LogLevel string.
func (c *Config) ParsedLogLevel() slog.Level {
	switch strings.ToLower(c.LogLevel) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info":
		fallthrough
	default:
		return slog.LevelInfo
	}
}

// Load loads configuration first from environment variables (to get file path),
// then from the specified YAML file, and finally merges/overrides with environment variables again.
func Load() (*Config, error) {
	// 1. Load initial config from Env (primarily to get ConfigFilePath)
	var initialCfg Config
	if err := envconfig.Process("mcpizer", &initialCfg); err != nil {
		return nil, fmt.Errorf("failed to process initial environment variables: %w", err)
	}

	// 2. Load config from YAML file if path is specified
	fileCfg := FileConfig{} // Defaults for file-based settings
	if initialCfg.ConfigFilePath != "" {
		var yamlFile []byte
		var err error

		// Check if the config path is a GitHub URL
		if strings.HasPrefix(initialCfg.ConfigFilePath, "github://") {
			// Use the github package's helper function
			yamlFile, err = github.LoadGitHubConfig(initialCfg.ConfigFilePath)
			if err != nil {
				return nil, fmt.Errorf("failed to load config from GitHub '%s': %w", initialCfg.ConfigFilePath, err)
			}
			slog.Info("Loaded configuration from GitHub.", "url", initialCfg.ConfigFilePath)
		} else {
			// Regular file path
			yamlFile, err = os.ReadFile(initialCfg.ConfigFilePath)
			if err != nil {
				// Allow file not existing if path is default and it's just not created?
				// Or maybe error strictly. Let's error strictly for now.
				return nil, fmt.Errorf("failed to read config file '%s': %w", initialCfg.ConfigFilePath, err)
			}
			slog.Info("Loaded configuration from file.", "path", initialCfg.ConfigFilePath)
		}

		err = yaml.Unmarshal(yamlFile, &fileCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal config file '%s': %w", initialCfg.ConfigFilePath, err)
		}
	} else {
		slog.Info("No config file path specified (MCPIZER_CONFIG_FILE), using defaults/env vars only.")
	}

	// 3. Create final config, starting with file values, then process Env vars again for overrides.
	finalCfg := initialCfg // Start with initial env vars (like file path itself)

	// Parse SchemaSources - support both string and object formats
	finalCfg.SchemaSources = make([]SchemaSource, 0, len(fileCfg.SchemaSources))
	for _, source := range fileCfg.SchemaSources {
		switch v := source.(type) {
		case string:
			// Simple string format
			finalCfg.SchemaSources = append(finalCfg.SchemaSources, SchemaSource{URL: v})
		case map[string]interface{}:
			// Object format with headers
			ss := SchemaSource{}
			if url, ok := v["url"].(string); ok {
				ss.URL = url
			}
			if headers, ok := v["headers"].(map[string]interface{}); ok {
				ss.Headers = make(map[string]string)
				for k, val := range headers {
					if strVal, ok := val.(string); ok {
						ss.Headers[k] = strVal
					}
				}
			}
			if server, ok := v["server"].(string); ok {
				ss.Server = server
			}
			if typ, ok := v["type"].(string); ok {
				ss.Type = typ
			}
			if mode, ok := v["mode"].(string); ok {
				ss.Mode = mode
			}
			if ss.URL != "" {
				// Validate that .proto files have a server specified
				if strings.HasSuffix(ss.URL, ".proto") && ss.Server == "" {
					slog.Warn("Proto file source missing server field, skipping", "url", ss.URL)
					continue
				}
				finalCfg.SchemaSources = append(finalCfg.SchemaSources, ss)
			}
		default:
			slog.Warn("Ignoring invalid schema source format", "source", source)
		}
	}
	// Potentially apply other fileCfg fields to finalCfg here

	// Process environment variables AGAIN to allow overrides over file settings.
	if err := envconfig.Process("mcpizer", &finalCfg); err != nil {
		return nil, fmt.Errorf("failed to process overriding environment variables: %w", err)
	}

	return &finalCfg, nil
}
