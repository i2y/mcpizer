package test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMCPizerConnectRPC tests the full flow through MCPizer with Connect-RPC
func TestMCPizerConnectRPC(t *testing.T) {
	t.Skip("This test requires a running MCPizer server. Run manually with: go test -run TestMCPizerConnectRPC ./test")

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Start MCPizer server
	serverURL := "http://localhost:8080"

	// Wait for server to be ready
	_, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Wait for server to start
	for i := 0; i < 10; i++ {
		resp, err := http.Get(serverURL + "/health")
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			break
		}
		if i == 9 {
			t.Skip("MCPizer server not running. Start it with: ./mcpizer -config=test/test-connect-config.yaml")
		}
		time.Sleep(time.Second)
	}

	t.Run("list tools", func(t *testing.T) {
		// Create MCP request to list tools
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/list",
			"id":      1,
		}

		respBody := sendSSERequest(t, serverURL+"/sse", request)

		// Parse response
		var response map[string]interface{}
		err := json.Unmarshal([]byte(respBody), &response)
		require.NoError(t, err)

		// Check we have the ElizaService_Say tool
		result, ok := response["result"].(map[string]interface{})
		require.True(t, ok)

		tools, ok := result["tools"].([]interface{})
		require.True(t, ok)

		// Find the Eliza Say tool
		var elizaTool map[string]interface{}
		for _, tool := range tools {
			t := tool.(map[string]interface{})
			if t["name"] == "ElizaService_Say" {
				elizaTool = t
				break
			}
		}

		require.NotNil(t, elizaTool, "ElizaService_Say tool not found")
		t.Logf("Found tool: %v", elizaTool)
	})

	t.Run("call ElizaService_Say", func(t *testing.T) {
		// Create MCP request to call the tool
		request := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"id":      2,
			"params": map[string]interface{}{
				"name": "ElizaService_Say",
				"arguments": map[string]interface{}{
					"sentence": "Hello, I am testing MCPizer",
				},
			},
		}

		respBody := sendSSERequest(t, serverURL+"/sse", request)

		// Parse response
		var response map[string]interface{}
		err := json.Unmarshal([]byte(respBody), &response)
		require.NoError(t, err)

		// Check the result
		result, ok := response["result"].(map[string]interface{})
		require.True(t, ok)

		content, ok := result["content"].([]interface{})
		require.True(t, ok)
		require.Len(t, content, 1)

		firstContent := content[0].(map[string]interface{})
		text, ok := firstContent["text"].(string)
		require.True(t, ok)

		// The response should contain the Eliza reply
		assert.Contains(t, text, "sentence")
		t.Logf("Eliza response: %s", text)
	})
}

// sendSSERequest sends a request to the SSE endpoint and returns the response
func sendSSERequest(t *testing.T, url string, request interface{}) string {
	reqBody, err := json.Marshal(request)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Read SSE response
	reader := bufio.NewReader(resp.Body)
	var responseData string

	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		if line == "\n" {
			continue
		}

		if len(line) > 6 && line[:6] == "data: " {
			responseData = line[6:]
			break
		}
	}

	return responseData
}
