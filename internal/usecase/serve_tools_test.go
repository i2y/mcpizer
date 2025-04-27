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

// MockToolRepository is a mock implementation of the ToolRepository interface.
type MockToolRepository struct {
	mock.Mock
}

func (m *MockToolRepository) Save(ctx context.Context, tools []domain.Tool, details []usecase.InvocationDetails) error {
	args := m.Called(ctx, tools, details)
	return args.Error(0)
}

func (m *MockToolRepository) List(ctx context.Context) ([]domain.Tool, error) {
	args := m.Called(ctx)
	// Need to handle potential nil slice for tools
	result := args.Get(0)
	if result == nil {
		return nil, args.Error(1)
	}
	return result.([]domain.Tool), args.Error(1)
}

func (m *MockToolRepository) FindToolByName(ctx context.Context, name string) (*domain.Tool, error) {
	args := m.Called(ctx, name)
	// Handle potential nil pointer for tool
	result := args.Get(0)
	if result == nil {
		return nil, args.Error(1)
	}
	return result.(*domain.Tool), args.Error(1)
}

func (m *MockToolRepository) FindInvocationDetailsByName(ctx context.Context, name string) (*usecase.InvocationDetails, error) {
	args := m.Called(ctx, name)
	// Handle potential nil pointer for details
	result := args.Get(0)
	if result == nil {
		return nil, args.Error(1)
	}
	return result.(*usecase.InvocationDetails), args.Error(1)
}

func TestServeToolsUseCase_Execute(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})) // Use text handler for tests

	// Prepare mock data
	expectedTools := []domain.Tool{
		{Name: "tool-a", Description: "Tool A"},
		{Name: "tool-b", Description: "Tool B"},
	}
	repoError := errors.New("repository error")

	tests := []struct {
		name          string
		mockSetup     func(*MockToolRepository)
		wantErr       bool
		wantTools     []domain.Tool
		expectErrText string // Optional: check specific error text
	}{
		{
			name: "Success - tools found",
			mockSetup: func(repo *MockToolRepository) {
				repo.On("List", ctx).Return(expectedTools, nil).Once()
			},
			wantErr:   false,
			wantTools: expectedTools,
		},
		{
			name: "Success - no tools found",
			mockSetup: func(repo *MockToolRepository) {
				repo.On("List", ctx).Return([]domain.Tool{}, nil).Once() // Return empty slice
			},
			wantErr:   false,
			wantTools: []domain.Tool{}, // Expect empty slice back
		},
		{
			name: "Failure - repository error",
			mockSetup: func(repo *MockToolRepository) {
				repo.On("List", ctx).Return(nil, repoError).Once()
			},
			wantErr:       true,
			wantTools:     nil,
			expectErrText: "failed to list tools from repository: repository error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockToolRepository)
			tt.mockSetup(mockRepo)

			uc := usecase.NewServeToolsUseCase(mockRepo, logger)
			actualTools, err := uc.Execute(ctx)

			if tt.wantErr {
				assert.Error(err)
				if tt.expectErrText != "" {
					assert.EqualError(err, tt.expectErrText)
				}
				// Check that tools slice is nil on error
				assert.Nil(actualTools)
			} else {
				assert.NoError(err)
				// Use assert.Equal for slice comparison
				assert.Equal(tt.wantTools, actualTools)
			}

			mockRepo.AssertExpectations(t) // Verify mock interactions
		})
	}
} 