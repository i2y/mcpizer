package connect

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoker_InvokeHTTP(t *testing.T) {
	logger := slog.Default()

	t.Run("successful invocation", func(t *testing.T) {
		// Create test server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request
			assert.Equal(t, "POST", r.Method)
			assert.Equal(t, "/connectrpc.eliza.v1.ElizaService/Say", r.URL.Path)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "1", r.Header.Get("Connect-Protocol-Version"))

			// Parse request body
			var reqBody map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&reqBody)
			require.NoError(t, err)
			assert.Equal(t, "Hello", reqBody["sentence"])

			// Send response
			resp := map[string]interface{}{
				"sentence": "Hello, how are you?",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		// Create invoker
		invoker := NewInvoker(logger)

		// Invoke method
		params := map[string]interface{}{
			"sentence": "Hello",
		}
		result, err := invoker.InvokeHTTP(context.Background(), server.URL, "/connectrpc.eliza.v1.ElizaService/Say", params)

		// Verify result
		require.NoError(t, err)
		require.NotNil(t, result)

		resultMap, ok := result.(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "Hello, how are you?", resultMap["sentence"])
	})

	t.Run("Connect-RPC error response", func(t *testing.T) {
		// Create test server that returns an error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Send Connect-RPC error response
			resp := map[string]interface{}{
				"error": map[string]interface{}{
					"code":    "invalid_argument",
					"message": "sentence cannot be empty",
				},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK) // Connect-RPC returns 200 even for errors
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		// Create invoker
		invoker := NewInvoker(logger)

		// Invoke method
		params := map[string]interface{}{
			"sentence": "",
		}
		result, err := invoker.InvokeHTTP(context.Background(), server.URL, "/connectrpc.eliza.v1.ElizaService/Say", params)

		// Verify error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid_argument")
		assert.Contains(t, err.Error(), "sentence cannot be empty")
		assert.Nil(t, result)
	})

	t.Run("HTTP error", func(t *testing.T) {
		// Create test server that returns HTTP error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
		}))
		defer server.Close()

		// Create invoker
		invoker := NewInvoker(logger)

		// Invoke method
		params := map[string]interface{}{}
		result, err := invoker.InvokeHTTP(context.Background(), server.URL, "/test/Method", params)

		// Verify error
		require.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP error 500")
		assert.Nil(t, result)
	})
}
