package github

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/i2y/mcpizer/internal/domain"
)

func TestFetcher_Fetch(t *testing.T) {
	// Skip if gh is not available
	client := NewGHClient()
	if err := client.checkGHCommand(); err != nil {
		t.Skip("Skipping integration test: gh CLI not available or not authenticated")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	fetcher := NewFetcher(logger)

	tests := []struct {
		name        string
		source      string
		expectError bool
		checkResult func(t *testing.T, schema domain.APISchema)
	}{
		{
			name:        "valid GitHub OpenAPI URL",
			source:      "github://github/rest-api-description/descriptions/api.github.com/api.github.com.yaml",
			expectError: false,
			checkResult: func(t *testing.T, schema domain.APISchema) {
				assert.Equal(t, domain.SchemaTypeOpenAPI, schema.Type)
				assert.NotEmpty(t, schema.RawData)
				assert.NotNil(t, schema.ParsedData)
				assert.Contains(t, string(schema.RawData), "openapi")
			},
		},
		{
			name:        "invalid URL - not GitHub",
			source:      "https://example.com/api.yaml",
			expectError: true,
		},
		{
			name:        "invalid GitHub URL - file not found",
			source:      "github://owner/repo/nonexistent.yaml",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			schema, err := fetcher.Fetch(ctx, tt.source)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, schema)
				}
			}
		})
	}
}

func TestLoadGitHubConfig(t *testing.T) {
	// Skip if gh is not available
	client := NewGHClient()
	if err := client.checkGHCommand(); err != nil {
		t.Skip("Skipping integration test: gh CLI not available or not authenticated")
	}

	tests := []struct {
		name        string
		url         string
		expectError bool
		checkResult func(t *testing.T, content []byte)
	}{
		{
			name:        "valid GitHub config URL",
			url:         "github://github/gitignore/Go.gitignore",
			expectError: false,
			checkResult: func(t *testing.T, content []byte) {
				assert.NotEmpty(t, content)
				assert.Contains(t, string(content), "# Binaries for programs and plugins")
			},
		},
		{
			name:        "invalid URL - not GitHub",
			url:         "https://example.com/config.yaml",
			expectError: true,
		},
		{
			name:        "invalid GitHub URL",
			url:         "github://invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, err := LoadGitHubConfig(tt.url)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, content)
				}
			}
		})
	}
}
