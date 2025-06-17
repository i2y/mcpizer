package usecase_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/i2y/mcpizer/internal/domain"
	"github.com/i2y/mcpizer/internal/usecase"

	// mcp-go imports for mocks
	"github.com/mark3labs/mcp-go/mcp"
	mcpServer "github.com/mark3labs/mcp-go/server"
)

// MockToolRepository defined in serve_tools_test.go

// MockSchemaFetcher is a mock implementation of the SchemaFetcher interface.
type MockSchemaFetcher struct {
	mock.Mock
}

func (m *MockSchemaFetcher) Fetch(ctx context.Context, src string) (domain.APISchema, error) {
	args := m.Called(ctx, src)
	return args.Get(0).(domain.APISchema), args.Error(1)
}

func (m *MockSchemaFetcher) FetchWithConfig(ctx context.Context, config usecase.SchemaSourceConfig) (domain.APISchema, error) {
	args := m.Called(ctx, config)
	return args.Get(0).(domain.APISchema), args.Error(1)
}

// MockToolGenerator is a mock implementation of the ToolGenerator interface.
type MockToolGenerator struct {
	mock.Mock
}

func (m *MockToolGenerator) Generate(schema domain.APISchema) ([]domain.Tool, []usecase.InvocationDetails, error) {
	args := m.Called(schema)
	// Handle potential nil slices
	tools := args.Get(0)
	details := args.Get(1)
	var toolsSlice []domain.Tool
	var detailsSlice []usecase.InvocationDetails

	if tools != nil {
		toolsSlice = tools.([]domain.Tool)
	}
	if details != nil {
		detailsSlice = details.([]usecase.InvocationDetails)
	}

	return toolsSlice, detailsSlice, args.Error(2)
}

// MockMCPServer is a mock implementation of the MCPServer.
type MockMCPServer struct {
	mock.Mock
}

func (m *MockMCPServer) AddTool(tool mcp.Tool, handler mcpServer.ToolHandlerFunc) {
	m.Called(tool, handler)
}

// MockToolInvoker is defined elsewhere (e.g., invoke_tool_test.go), remove definition from here.
/*
 type MockToolInvoker struct {
 	mock.Mock
 }

 func (m *MockToolInvoker) Invoke(...) { ... }
*/

func TestSyncSchemaUseCase_Execute(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Prepare mock data
	sourceURL := "http://example.com/openapi.yaml"
	mockSchema := domain.APISchema{Source: sourceURL, Type: domain.SchemaTypeOpenAPI, ParsedData: "parsed"}
	// Define expected mcp.Tool based on mockTools and convertDomainToolToMCPTool logic
	// Assuming convertDomainToolToMCPTool just copies name and description for now
	mockDomainTool := domain.Tool{Name: "tool-a", Description: "Tool A Desc"}
	mockExpectedMCPTool := mcp.NewTool("tool-a", mcp.WithDescription("Tool A Desc"))
	mockTools := []domain.Tool{mockDomainTool}
	mockDetails := []usecase.InvocationDetails{{Type: "http", HTTPPath: "/path/a"}} // Add some detail for handler test

	fetchErr := errors.New("fetch failed")
	generateErr := errors.New("generate failed")
	// addToolErr is no longer relevant as AddTool doesn't return error

	tests := []struct {
		name string
		// Update mockSetup signature
		mockSetup     func(*MockSchemaFetcher, *MockToolGenerator, *MockMCPServer, *MockToolInvoker)
		inSource      string
		wantErr       bool
		expectErrText string // Optional
	}{
		{
			name: "Success - OpenAPI schema synced",
			// Update mockSetup implementation
			mockSetup: func(fetcher *MockSchemaFetcher, generator *MockToolGenerator, mcpSrv *MockMCPServer, invoker *MockToolInvoker) {
				fetcher.On("Fetch", ctx, sourceURL).Return(mockSchema, nil).Once()
				generator.On("Generate", mockSchema).Return(mockTools, mockDetails, nil).Once()
				// Use mock.Anything for the handler function type matching
				mcpSrv.On("AddTool", mockExpectedMCPTool, mock.Anything).Once()
			},
			inSource: sourceURL,
			wantErr:  false,
		},
		{
			name: "Failure - Fetch error",
			mockSetup: func(fetcher *MockSchemaFetcher, generator *MockToolGenerator, mcpSrv *MockMCPServer, invoker *MockToolInvoker) {
				fetcher.On("Fetch", ctx, sourceURL).Return(domain.APISchema{}, fetchErr).Once()
				// Generate and AddTool should not be called
			},
			inSource: sourceURL,
			wantErr:  true,
			// Expect error wrapped by Execute
			expectErrText: "error executing sync for source http://example.com/openapi.yaml: failed to fetch schema: fetch failed",
		},
		{
			name: "Failure - Generate error",
			mockSetup: func(fetcher *MockSchemaFetcher, generator *MockToolGenerator, mcpSrv *MockMCPServer, invoker *MockToolInvoker) {
				fetcher.On("Fetch", ctx, sourceURL).Return(mockSchema, nil).Once()
				generator.On("Generate", mockSchema).Return(nil, nil, generateErr).Once()
				// AddTool should not be called
			},
			inSource: sourceURL,
			wantErr:  true,
			// Expect error wrapped by Execute
			expectErrText: "error executing sync for source http://example.com/openapi.yaml: failed to generate tools/details: generate failed",
		},
		{
			name: "Failure - No generator for schema type",
			mockSetup: func(fetcher *MockSchemaFetcher, generator *MockToolGenerator, mcpSrv *MockMCPServer, invoker *MockToolInvoker) {
				// Return schema with a type that has no registered generator
				unsupportedSchema := domain.APISchema{Source: sourceURL, Type: "graphql", ParsedData: "graphql data"}
				fetcher.On("Fetch", ctx, sourceURL).Return(unsupportedSchema, nil).Once()
				// Generate and AddTool should not be called
			},
			inSource: sourceURL,
			wantErr:  true,
			// Expect error wrapped by Execute
			expectErrText: "error executing sync for source http://example.com/openapi.yaml: detected schema type (openapi) mismatch with fetched schema type (graphql)",
		},
		// TODO: Add test case for fetcher returning empty schema type and inference working/failing
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockFetcher := new(MockSchemaFetcher)
			mockGenerator := new(MockToolGenerator)
			mockMCPServer := new(MockMCPServer)
			mockInvoker := new(MockToolInvoker)

			// Setup mocks for the specific test case
			fetchersMap := map[domain.SchemaType]usecase.SchemaFetcher{
				domain.SchemaTypeOpenAPI: mockFetcher,
			}
			generatorsMap := map[domain.SchemaType]usecase.ToolGenerator{
				domain.SchemaTypeOpenAPI: mockGenerator,
			}
			if tt.name == "Failure - No generator for schema type" {
				fetchersMap["graphql"] = mockFetcher
			}

			tt.mockSetup(mockFetcher, mockGenerator, mockMCPServer, mockInvoker)

			uc := usecase.NewSyncSchemaUseCase(
				[]usecase.SchemaSourceConfig{},
				fetchersMap,
				generatorsMap,
				mockMCPServer,
				mockInvoker,
				logger,
			)
			// Change back to calling the exported Execute method
			err := uc.Execute(ctx, tt.inSource)

			if tt.wantErr {
				assert.Error(err)
				if tt.expectErrText != "" {
					// Use EqualError now that Execute wraps the error consistently
					assert.EqualError(err, tt.expectErrText)
				}
			} else {
				assert.NoError(err)
			}

			// Verify mock expectations
			mockFetcher.AssertExpectations(t)
			mockGenerator.AssertExpectations(t)
			mockMCPServer.AssertExpectations(t) // Assert AddTool called/not called as expected
		})
	}
}
