package usecase_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"mcp-bridge/internal/domain"
	"mcp-bridge/internal/usecase"
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

func TestSyncSchemaUseCase_Execute(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Prepare mock data
	sourceURL := "http://example.com/openapi.yaml"
	mockSchema := domain.APISchema{Source: sourceURL, Type: domain.SchemaTypeOpenAPI, ParsedData: "parsed"}
	mockTools := []domain.Tool{{Name: "tool-a"}}
	mockDetails := []usecase.InvocationDetails{{Type: "http"}}

	fetchErr := errors.New("fetch failed")
	generateErr := errors.New("generate failed")
	saveErr := errors.New("save failed")

	tests := []struct {
		name          string
		mockSetup     func(*MockSchemaFetcher, *MockToolGenerator, *MockToolRepository)
		inSource      string
		wantErr       bool
		expectErrText string // Optional
	}{
		{
			name: "Success - OpenAPI schema synced",
			mockSetup: func(fetcher *MockSchemaFetcher, generator *MockToolGenerator, repo *MockToolRepository) {
				fetcher.On("Fetch", ctx, sourceURL).Return(mockSchema, nil).Once()
				generator.On("Generate", mockSchema).Return(mockTools, mockDetails, nil).Once()
				repo.On("Save", ctx, mockTools, mockDetails).Return(nil).Once()
			},
			inSource: sourceURL,
			wantErr:  false,
		},
		{
			name: "Failure - Fetch error",
			mockSetup: func(fetcher *MockSchemaFetcher, generator *MockToolGenerator, repo *MockToolRepository) {
				fetcher.On("Fetch", ctx, sourceURL).Return(domain.APISchema{}, fetchErr).Once()
				// Generate and Save should not be called
			},
			inSource:      sourceURL,
			wantErr:       true,
			expectErrText: "failed to fetch schema from http://example.com/openapi.yaml: fetch failed",
		},
		{
			name: "Failure - Generate error",
			mockSetup: func(fetcher *MockSchemaFetcher, generator *MockToolGenerator, repo *MockToolRepository) {
				fetcher.On("Fetch", ctx, sourceURL).Return(mockSchema, nil).Once()
				generator.On("Generate", mockSchema).Return(nil, nil, generateErr).Once()
				// Save should not be called
			},
			inSource:      sourceURL,
			wantErr:       true,
			expectErrText: "failed to generate tools/details for schema http://example.com/openapi.yaml: generate failed",
		},
		{
			name: "Failure - Save error",
			mockSetup: func(fetcher *MockSchemaFetcher, generator *MockToolGenerator, repo *MockToolRepository) {
				fetcher.On("Fetch", ctx, sourceURL).Return(mockSchema, nil).Once()
				generator.On("Generate", mockSchema).Return(mockTools, mockDetails, nil).Once()
				repo.On("Save", ctx, mockTools, mockDetails).Return(saveErr).Once()
			},
			inSource:      sourceURL,
			wantErr:       true,
			expectErrText: "failed to save generated tools/details: save failed",
		},
		{
			name: "Failure - No generator for schema type",
			mockSetup: func(fetcher *MockSchemaFetcher, generator *MockToolGenerator, repo *MockToolRepository) {
				// Return schema with a type that has no registered generator
				unsupportedSchema := domain.APISchema{Source: sourceURL, Type: "graphql", ParsedData: "graphql data"}
				fetcher.On("Fetch", ctx, sourceURL).Return(unsupportedSchema, nil).Once()
			},
			inSource:      sourceURL,
			wantErr:       true,
			expectErrText: "no tool generator found for schema type: graphql",
		},
		// TODO: Add test case for fetcher returning empty schema type and inference working/failing
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockFetcher := new(MockSchemaFetcher)
			mockGenerator := new(MockToolGenerator)
			mockRepo := new(MockToolRepository)

			// Setup mocks for the specific test case
			// Use maps for fetchers/generators passed to the use case constructor
			fetchersMap := map[domain.SchemaType]usecase.SchemaFetcher{
				domain.SchemaTypeOpenAPI: mockFetcher, // Only register the one we expect to be called
				// domain.SchemaTypeGRPC:    mockFetcher, // Or register both if needed for type inference test
			}
			generatorsMap := map[domain.SchemaType]usecase.ToolGenerator{
				domain.SchemaTypeOpenAPI: mockGenerator,
				// domain.SchemaTypeGRPC: mockGenerator,
			}
			// Add mocks for specific types needed by the test case
			if tt.name == "Failure - No generator for schema type" {
				fetchersMap["graphql"] = mockFetcher // Need to register the fetcher returning this type
			}

			tt.mockSetup(mockFetcher, mockGenerator, mockRepo)

			uc := usecase.NewSyncSchemaUseCase(fetchersMap, generatorsMap, mockRepo, logger)
			err := uc.Execute(ctx, tt.inSource)

			if tt.wantErr {
				assert.Error(err)
				if tt.expectErrText != "" {
					assert.EqualError(err, tt.expectErrText)
				}
			} else {
				assert.NoError(err)
			}

			// Verify mock expectations for all involved mocks
			mockFetcher.AssertExpectations(t)
			mockGenerator.AssertExpectations(t)
			mockRepo.AssertExpectations(t)
		})
	}
}
