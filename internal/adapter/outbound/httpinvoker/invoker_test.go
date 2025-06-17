package httpinvoker_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/i2y/mcpizer/internal/adapter/outbound/httpinvoker"
	"github.com/i2y/mcpizer/internal/usecase"
)

func newTestInvoker(t *testing.T, handler http.Handler) (*httpinvoker.Invoker, *httptest.Server) {
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close) // Ensure server is closed after test

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	invoker := httpinvoker.New(server.Client(), logger) // Use test server's client
	return invoker, server
}

// Define a type for the error check function
type ErrorCheckFunc func(err error)

func TestInvoker_Invoke(t *testing.T) {
	assert := assert.New(t) // Top-level instance
	require := require.New(t)
	ctx := context.Background()

	// Mock responses
	successRespBody := map[string]interface{}{"message": "ok"}
	successRespBytes, _ := json.Marshal(successRespBody)
	// Error response body for testing
	errorRespBody := map[string]interface{}{
		"error": "thing not found",
	}
	errorRespBodyBytes, _ := json.Marshal(errorRespBody)

	tests := []struct {
		name           string
		mockHandler    func(w http.ResponseWriter, r *http.Request)
		inDetails      usecase.InvocationDetails
		inParams       map[string]interface{}
		wantResult     interface{}
		wantErr        bool
		expectErrCheck ErrorCheckFunc
	}{
		{
			name: "Success - POST with JSON body from params",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				// Use require/assert created at top level
				assert.Equal(http.MethodPost, r.Method)
				assert.Equal("/v1/items", r.URL.Path)
				assert.Equal("application/json", r.Header.Get("Content-Type"))
				bodyBytes, _ := io.ReadAll(r.Body)
				var bodyData map[string]interface{}
				require.NoError(json.Unmarshal(bodyBytes, &bodyData))
				assert.Equal(map[string]interface{}{"p1": "v1", "p2": 123.0}, bodyData)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(successRespBytes)
			},
			inDetails: usecase.InvocationDetails{
				Type:        "http",
				HTTPPath:    "/v1/items",
				HTTPMethod:  http.MethodPost,
				ContentType: "application/json",
			},
			inParams:   map[string]interface{}{"p1": "v1", "p2": 123},
			wantResult: successRespBody,
			wantErr:    false,
		},
		{
			name: "Success - POST with BodyParam",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(http.MethodPost, r.Method)
				assert.Equal("/submit", r.URL.Path)
				assert.Equal("application/json", r.Header.Get("Content-Type"))
				bodyBytes, _ := io.ReadAll(r.Body)
				var bodyData map[string]string
				require.NoError(json.Unmarshal(bodyBytes, &bodyData))
				assert.Equal(map[string]string{"data": "payload"}, bodyData)
				w.WriteHeader(http.StatusAccepted)
			},
			inDetails: usecase.InvocationDetails{
				Type:        "http",
				HTTPPath:    "/submit",
				HTTPMethod:  http.MethodPost,
				ContentType: "application/json",
				BodyParam:   "data",
			},
			inParams:   map[string]interface{}{"data": map[string]string{"data": "payload"}},
			wantResult: "",  // Empty response body returns as empty string
			wantErr:    false,
		},
		{
			name: "Success - GET with query params",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(http.MethodGet, r.Method)
				assert.Equal("/search", r.URL.Path)
				// Assert correct query params
				assert.Equal("val1", r.URL.Query().Get("param1"))
				assert.Equal("123", r.URL.Query().Get("param2"))
				assert.Equal("", r.Header.Get("Content-Type"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(successRespBytes)
			},
			inDetails: usecase.InvocationDetails{
				Type:        "http",
				HTTPPath:    "/search",
				HTTPMethod:  http.MethodGet,
				QueryParams: []string{"param1", "param2"},
			},
			inParams:   map[string]interface{}{"param1": "val1", "param2": 123},
			wantResult: successRespBody,
			wantErr:    false,
		},
		{
			name: "Success - GET with path params",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(http.MethodGet, r.Method)
				// Assert correct path after substitution
				assert.Equal("/users/usr123/details", r.URL.Path)
				assert.Equal("", r.Header.Get("Content-Type"))

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(successRespBytes)
			},
			inDetails: usecase.InvocationDetails{
				Type:       "http",
				HTTPPath:   "/users/{userID}/details",
				HTTPMethod: http.MethodGet,
				PathParams: []string{"userID"},
			},
			// Use correct param key "userID"
			inParams:   map[string]interface{}{"userID": "usr123"},
			wantResult: successRespBody,
			wantErr:    false,
		},
		{
			name: "Success - Missing path parameter (placeholder remains)",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				// Path parameter placeholder remains in URL when not provided
				assert.Equal("/items/{itemID}", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(successRespBytes)
			},
			inDetails: usecase.InvocationDetails{
				Type:       "http",
				HTTPPath:   "/items/{itemID}",
				HTTPMethod: http.MethodGet,
				PathParams: []string{"itemID"},
			},
			inParams:   map[string]interface{}{}, // "itemID" is missing
			wantResult: successRespBody,
			wantErr:    false,
		},
		{
			name: "Failure - HTTP 404 (Generic)",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("Resource not found here"))
			},
			inDetails: usecase.InvocationDetails{
				Type:       "http",
				HTTPPath:   "/notfound",
				HTTPMethod: http.MethodGet,
			},
			inParams:      map[string]interface{}{},
			wantErr:       true,
			expectErrCheck: func(err error) {
				// Use top-level assert instance directly
				assert.Contains(err.Error(), "HTTP 404:")
				assert.Contains(err.Error(), "Resource not found here")
			},
		},
		{
			name: "Failure - HTTP 500 with Error Body",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(errorRespBodyBytes) // Use marshalled map
			},
			inDetails: usecase.InvocationDetails{
				Type:        "http",
				HTTPPath:    "/error",
				HTTPMethod:  http.MethodPost,
				ContentType: "application/json",
			},
			inParams:      map[string]interface{}{},
			wantErr:       true,
		},
		{
			name: "Success - Non-JSON response body returned as string",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("this is not json"))
			},
			inDetails: usecase.InvocationDetails{
				Type:       "http",
				HTTPPath:   "/textresponse",
				HTTPMethod: http.MethodGet,
			},
			inParams:   map[string]interface{}{},
			wantResult: "this is not json",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use require from the main test scope
			invoker, server := newTestInvoker(t, http.HandlerFunc(tt.mockHandler))
			tt.inDetails.Host = server.URL // Set the dynamic host

			actualResult, err := invoker.Invoke(ctx, tt.inDetails, tt.inParams)

			if tt.wantErr {
				require.Error(err) // Use top-level require
				if tt.expectErrCheck != nil {
					tt.expectErrCheck(err)
				}
				assert.Nil(actualResult)
			} else {
				require.NoError(err) // Use top-level require
				assert.Equal(tt.wantResult, actualResult)
			}
		})
	}
}
