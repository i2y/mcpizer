//go:build integration
// +build integration

package test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/i2y/mcpizer/internal/adapter/outbound/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConnectRPCIntegration tests against the actual Connect-RPC demo service
func TestConnectRPCIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := slog.Default()
	invoker := connect.NewInvoker(logger)

	testCases := []struct {
		name     string
		sentence string
		// We can't predict exact responses, but we can check they're not empty
		checkResponse func(t *testing.T, response string)
	}{
		{
			name:     "simple greeting",
			sentence: "Hello",
			checkResponse: func(t *testing.T, response string) {
				assert.NotEmpty(t, response)
			},
		},
		{
			name:     "feeling statement",
			sentence: "I feel happy",
			checkResponse: func(t *testing.T, response string) {
				assert.NotEmpty(t, response)
				// ELIZA typically responds with questions about feelings
			},
		},
		{
			name:     "question",
			sentence: "How are you?",
			checkResponse: func(t *testing.T, response string) {
				assert.NotEmpty(t, response)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]interface{}{
				"sentence": tc.sentence,
			}

			// Call the actual Connect-RPC service
			result, err := invoker.InvokeHTTP(
				context.Background(),
				"https://demo.connectrpc.com",
				"/connectrpc.eliza.v1.ElizaService/Say",
				params,
			)

			require.NoError(t, err, "Failed to invoke Connect-RPC service")
			require.NotNil(t, result)

			// Check the response
			resultMap, ok := result.(map[string]interface{})
			require.True(t, ok, "Result should be a map")

			response, ok := resultMap["sentence"].(string)
			require.True(t, ok, "Response should contain a 'sentence' field")

			t.Logf("Input: %q, Response: %q", tc.sentence, response)
			tc.checkResponse(t, response)
		})
	}
}

// TestConnectRPCErrorHandling tests error cases with the demo service
func TestConnectRPCErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	logger := slog.Default()
	invoker := connect.NewInvoker(logger)

	t.Run("invalid method", func(t *testing.T) {
		params := map[string]interface{}{
			"sentence": "test",
		}

		result, err := invoker.InvokeHTTP(
			context.Background(),
			"https://demo.connectrpc.com",
			"/connectrpc.eliza.v1.ElizaService/InvalidMethod",
			params,
		)

		require.Error(t, err)
		assert.Nil(t, result)
		// The error should indicate the method doesn't exist
		t.Logf("Expected error: %v", err)
	})

	t.Run("malformed request", func(t *testing.T) {
		// Send wrong field name
		params := map[string]interface{}{
			"text": "test", // Should be "sentence"
		}

		result, err := invoker.InvokeHTTP(
			context.Background(),
			"https://demo.connectrpc.com",
			"/connectrpc.eliza.v1.ElizaService/Say",
			params,
		)

		// This might not error if the field is optional
		// But the response might be different
		if err != nil {
			t.Logf("Error with malformed request: %v", err)
		} else {
			t.Logf("Result with malformed request: %v", result)
		}
	})
}
