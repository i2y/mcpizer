package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"
)

// GHClient wraps the gh CLI command for GitHub operations
type GHClient struct{}

// NewGHClient creates a new GitHub client
func NewGHClient() *GHClient {
	return &GHClient{}
}

// parseGitHubURL parses a github:// URL into its components
// Format: github://owner/repo/path/to/file[@ref]
func (c *GHClient) parseGitHubURL(githubURL string) (owner, repo, path, ref string, err error) {
	if !strings.HasPrefix(githubURL, "github://") {
		return "", "", "", "", fmt.Errorf("invalid GitHub URL format: %s", githubURL)
	}

	// Remove the github:// prefix
	urlPath := strings.TrimPrefix(githubURL, "github://")

	// Check for ref (@branch or @tag)
	parts := strings.Split(urlPath, "@")
	if len(parts) == 2 {
		urlPath = parts[0]
		ref = parts[1]
	}

	// Split the path
	pathParts := strings.SplitN(urlPath, "/", 3)
	if len(pathParts) < 3 {
		return "", "", "", "", fmt.Errorf("invalid GitHub URL format: expected github://owner/repo/path/to/file")
	}

	owner = pathParts[0]
	repo = pathParts[1]
	path = pathParts[2]

	return owner, repo, path, ref, nil
}

// FetchFile retrieves a file from GitHub using the gh CLI
func (c *GHClient) FetchFile(ctx context.Context, githubURL string) ([]byte, error) {
	owner, repo, path, ref, err := c.parseGitHubURL(githubURL)
	if err != nil {
		return nil, err
	}

	// Check if gh command is available
	if err := c.checkGHCommand(); err != nil {
		return nil, err
	}

	// Build the API path
	apiPath := fmt.Sprintf("repos/%s/%s/contents/%s", owner, repo, path)
	if ref != "" {
		apiPath += "?ref=" + ref
	}

	// Execute gh api command
	cmd := exec.CommandContext(ctx, "gh", "api", apiPath, "--jq", ".content")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("gh command failed: %s", stderr.String())
		}
		return nil, fmt.Errorf("gh command failed: %w", err)
	}

	// The content is base64 encoded, decode it
	encodedContent := strings.TrimSpace(stdout.String())
	if encodedContent == "" {
		return nil, fmt.Errorf("empty response from GitHub")
	}

	content, err := base64.StdEncoding.DecodeString(encodedContent)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 content: %w", err)
	}

	return content, nil
}

// FetchFileRaw retrieves a file from GitHub using the raw content endpoint
func (c *GHClient) FetchFileRaw(ctx context.Context, githubURL string) ([]byte, error) {
	owner, repo, path, ref, err := c.parseGitHubURL(githubURL)
	if err != nil {
		return nil, err
	}

	// Check if gh command is available
	if err := c.checkGHCommand(); err != nil {
		return nil, err
	}

	// For raw content, we need to get the download URL first
	apiPath := fmt.Sprintf("repos/%s/%s/contents/%s", owner, repo, path)
	if ref != "" {
		apiPath += "?ref=" + ref
	}

	// Get the download URL
	cmd := exec.CommandContext(ctx, "gh", "api", apiPath, "--jq", ".download_url")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("gh command failed: %s", stderr.String())
		}
		return nil, fmt.Errorf("gh command failed: %w", err)
	}

	downloadURL := strings.TrimSpace(stdout.String())
	if downloadURL == "" || downloadURL == "null" {
		return nil, fmt.Errorf("no download URL found")
	}

	// Fetch the raw content using curl (gh doesn't have direct raw download)
	curlCmd := exec.CommandContext(ctx, "curl", "-s", "-L", downloadURL)

	stdout.Reset()
	stderr.Reset()
	curlCmd.Stdout = &stdout
	curlCmd.Stderr = &stderr

	if err := curlCmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("curl command failed: %s", stderr.String())
		}
		return nil, fmt.Errorf("curl command failed: %w", err)
	}

	return stdout.Bytes(), nil
}

// checkGHCommand verifies that the gh CLI is installed and authenticated
func (c *GHClient) checkGHCommand() error {
	cmd := exec.Command("gh", "auth", "status")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "not found") || strings.Contains(err.Error(), "executable file not found") {
			return fmt.Errorf("gh CLI is not installed. Please install it from https://cli.github.com/")
		}
		if strings.Contains(stderr.String(), "not logged in") {
			return fmt.Errorf("gh CLI is not authenticated. Please run 'gh auth login' first")
		}
		return fmt.Errorf("gh auth check failed: %s", stderr.String())
	}

	return nil
}

// IsGitHubURL checks if a URL is a GitHub URL
func IsGitHubURL(url string) bool {
	return strings.HasPrefix(url, "github://")
}
