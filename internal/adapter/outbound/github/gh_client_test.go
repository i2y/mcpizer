package github

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		name          string
		url           string
		expectedOwner string
		expectedRepo  string
		expectedPath  string
		expectedRef   string
		expectError   bool
	}{
		{
			name:          "simple github URL",
			url:           "github://owner/repo/path/to/file.yaml",
			expectedOwner: "owner",
			expectedRepo:  "repo",
			expectedPath:  "path/to/file.yaml",
			expectedRef:   "",
			expectError:   false,
		},
		{
			name:          "github URL with ref",
			url:           "github://owner/repo/path/to/file.yaml@v1.0",
			expectedOwner: "owner",
			expectedRepo:  "repo",
			expectedPath:  "path/to/file.yaml",
			expectedRef:   "v1.0",
			expectError:   false,
		},
		{
			name:          "github URL with branch ref",
			url:           "github://microsoft/api-guidelines/graph/openapi.yaml@main",
			expectedOwner: "microsoft",
			expectedRepo:  "api-guidelines",
			expectedPath:  "graph/openapi.yaml",
			expectedRef:   "main",
			expectError:   false,
		},
		{
			name:        "invalid URL - not github",
			url:         "https://github.com/owner/repo/file.yaml",
			expectError: true,
		},
		{
			name:        "invalid URL - missing path",
			url:         "github://owner/repo",
			expectError: true,
		},
		{
			name:        "invalid URL - missing repo",
			url:         "github://owner",
			expectError: true,
		},
	}

	client := NewGHClient()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, path, ref, err := client.parseGitHubURL(tt.url)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedOwner, owner)
				assert.Equal(t, tt.expectedRepo, repo)
				assert.Equal(t, tt.expectedPath, path)
				assert.Equal(t, tt.expectedRef, ref)
			}
		})
	}
}

func TestIsGitHubURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"github://owner/repo/file.yaml", true},
		{"github://owner/repo/file.yaml@v1.0", true},
		{"https://github.com/owner/repo/file.yaml", false},
		{"http://example.com/api.yaml", false},
		{"file:///local/path/api.yaml", false},
		{"grpc://server:50051", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := IsGitHubURL(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Integration test - requires gh CLI to be installed and authenticated
func TestFetchFile_Integration(t *testing.T) {
	// Skip if gh is not available
	client := NewGHClient()
	if err := client.checkGHCommand(); err != nil {
		t.Skip("Skipping integration test: gh CLI not available or not authenticated")
	}

	// Test with a public repository file
	ctx := context.Background()
	content, err := client.FetchFile(ctx, "github://github/gitignore/Go.gitignore")

	assert.NoError(t, err)
	assert.NotEmpty(t, content)
	assert.Contains(t, string(content), "# Binaries for programs and plugins")
}
