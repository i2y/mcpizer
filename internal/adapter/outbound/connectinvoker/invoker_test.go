package connectinvoker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mcp-bridge/internal/adapter/outbound/connectinvoker"
	"mcp-bridge/internal/usecase"
)

func newTestInvoker(t *testing.T, handler http.Handler) (*connectinvoker.Invoker, *httptest.Server) {
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close) // Ensure server is closed after test

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	invoker := connectinvoker.New(server.Client(), logger) // Use test server's client
	return invoker, server
}

// Define a type for the error check function
type ErrorCheckFunc func(t *testing.T, err error)

func TestInvoker_Invoke(t *testing.T) {
	// Create assert/require instances once for the main test function
	assert := assert.New(t)
	require := require.New(t)
	ctx := context.Background()

	// Mock responses
	successRespBody := map[string]interface{}{"message": "ok"}
	successRespBytes, _ := json.Marshal(successRespBody)
	connectErrBodyBytes, _ := json.Marshal(connect.NewError(connect.CodeNotFound, fmt.Errorf("thing not found")))

	tests := []struct {
		name           string
		mockHandler    func(w http.ResponseWriter, r *http.Request)
		inDetails      usecase.InvocationDetails
		inParams       map[string]interface{}
		wantResult     map[string]interface{}
		wantErr        bool
		expectErrCode  connect.Code
		expectErrCheck ErrorCheckFunc // Use the defined type
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
				Type:       "http",
				HTTPPath:   "/v1/items",
				HTTPMethod: http.MethodPost,
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
				assert.Equal("application/custom", r.Header.Get("Content-Type"))
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
				ContentType: "application/custom",
				BodyParam:   "data",
			},
			inParams:   map[string]interface{}{"data": map[string]string{"data": "payload"}},
			wantResult: nil, // Expect nil body for 202 Accepted
			wantErr:    false,
		},
		{
			name: "Success - GET with query params",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(http.MethodGet, r.Method)
				assert.Equal("/search", r.URL.Path)
				assert.Equal("val1", r.URL.Query().Get("q1"))
				assert.Equal("123", r.URL.Query().Get("q2"))
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
			inParams:   map[string]interface{}{"id": "usr123"},
			wantResult: successRespBody,
			wantErr:    false,
		},
		{
			name:        "Failure - Missing path parameter",
			mockHandler: func(w http.ResponseWriter, r *http.Request) { t.Fail() }, // Should not be called
			inDetails: usecase.InvocationDetails{
				Type:       "http",
				HTTPPath:   "/items/{itemID}",
				HTTPMethod: http.MethodGet,
				PathParams: []string{"itemID"},
			},
			inParams: map[string]interface{}{}, // "id" is missing
			wantErr:  true,
			expectErrCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "missing required path parameter: itemID")
			},
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
			inParams: map[string]interface{}{},
			wantErr:  true,
			expectErrCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "request failed with status 404 Not Found")
				assert.Contains(t, err.Error(), "Resource not found here")
			},
		},
		{
			name: "Failure - HTTP 500 with Connect Error Body",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				w.Write(connectErrBodyBytes)
			},
			inDetails: usecase.InvocationDetails{
				Type:       "http",
				HTTPPath:   "/error",
				HTTPMethod: http.MethodPost,
			},
			inParams:      map[string]interface{}{},
			wantErr:       true,
			expectErrCode: connect.CodeNotFound,
		},
		{
			name: "Failure - Non-JSON response body",
			mockHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("this is not json"))
			},
			inDetails: usecase.InvocationDetails{
				Type:       "http",
				HTTPPath:   "/invalidjson",
				HTTPMethod: http.MethodGet,
			},
			inParams: map[string]interface{}{},
			wantErr:  true,
			expectErrCheck: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "failed to unmarshal JSON response")
				assert.Contains(t, err.Error(), "this is not json")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use assert/require from the main test function
			invoker, server := newTestInvoker(t, http.HandlerFunc(tt.mockHandler))
			tt.inDetails.Host = server.URL // Set the dynamic host

			actualResult, err := invoker.Invoke(ctx, tt.inDetails, tt.inParams)

			if tt.wantErr {
				assert.Error(err)
				if tt.expectErrCode != connect.CodeUnknown {
					assert.Equal(tt.expectErrCode, connect.CodeOf(err))
				}
				if tt.expectErrCheck != nil {
					tt.expectErrCheck(t, err) // Pass testing.T
				}
				assert.Nil(t, actualResult)
			} else {
				assert.NoError(err)
				assert.Equal(tt.wantResult, actualResult)
			}
		})
	}
}
