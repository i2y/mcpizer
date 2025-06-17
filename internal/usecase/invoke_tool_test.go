package usecase_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/i2y/mcpizer/internal/domain" // Needed for FindToolByName return type
	"github.com/i2y/mcpizer/internal/usecase"
)

// MockToolRepository defined in serve_tools_test.go

// MockToolInvoker is a mock implementation of the ToolInvoker interface.
type MockToolInvoker struct {
	mock.Mock
}

func (m *MockToolInvoker) Invoke(ctx context.Context, details usecase.InvocationDetails, params map[string]interface{}) (interface{}, error) {
	args := m.Called(ctx, details, params)
	// Return the first argument directly as interface{} and the error
	return args.Get(0), args.Error(1)
}

func TestInvokeToolUseCase_Execute(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Prepare mock data
	toolName := "test-tool"
	inputParams := map[string]interface{}{"param1": "value1"}
	mockTool := &domain.Tool{Name: toolName, Description: "Test tool"} // Found tool
	mockDetails := &usecase.InvocationDetails{Type: "http", Host: "example.com", HTTPMethod: "POST", HTTPPath: "/test"}
	expectedResult := map[string]interface{}{"success": true}
	invokerErr := errors.New("invocation failed error")

	tests := []struct {
		name          string
		mockSetup     func(*MockToolRepository, *MockToolInvoker)
		inToolName    string
		inParams      map[string]interface{}
		wantErr       bool
		wantResult    interface{}
		expectErrText string // Optional: check specific error text
	}{
		{
			name: "Success - tool invoked",
			mockSetup: func(repo *MockToolRepository, invoker *MockToolInvoker) {
				repo.On("FindToolByName", mock.Anything, toolName).Return(mockTool, nil).Once()
				repo.On("FindInvocationDetailsByName", mock.Anything, toolName).Return(mockDetails, nil).Once()
				invoker.On("Invoke", mock.Anything, *mockDetails, inputParams).Return(expectedResult, nil).Once()
			},
			inToolName: toolName,
			inParams:   inputParams,
			wantErr:    false,
			wantResult: expectedResult,
		},
		{
			name: "Failure - tool definition not found",
			mockSetup: func(repo *MockToolRepository, invoker *MockToolInvoker) {
				repo.On("FindToolByName", mock.Anything, toolName).Return(nil, usecase.ErrToolNotFound).Once()
				// FindInvocationDetailsByName and Invoke should not be called
			},
			inToolName:    toolName,
			inParams:      inputParams,
			wantErr:       true,
			expectErrText: usecase.ErrToolNotFound.Error(),
		},
		{
			name: "Failure - invocation details not found",
			mockSetup: func(repo *MockToolRepository, invoker *MockToolInvoker) {
				repo.On("FindToolByName", mock.Anything, toolName).Return(mockTool, nil).Once()
				repo.On("FindInvocationDetailsByName", mock.Anything, toolName).Return(nil, usecase.ErrToolNotFound).Once()
				// Invoke should not be called
			},
			inToolName:    toolName,
			inParams:      inputParams,
			wantErr:       true,
			expectErrText: usecase.ErrToolNotFound.Error(),
		},
		{
			name: "Failure - invoker error",
			mockSetup: func(repo *MockToolRepository, invoker *MockToolInvoker) {
				repo.On("FindToolByName", mock.Anything, toolName).Return(mockTool, nil).Once()
				repo.On("FindInvocationDetailsByName", mock.Anything, toolName).Return(mockDetails, nil).Once()
				invoker.On("Invoke", mock.Anything, *mockDetails, inputParams).Return(nil, invokerErr).Once()
			},
			inToolName:    toolName,
			inParams:      inputParams,
			wantErr:       true,
			expectErrText: "failed to invoke tool test-tool: invocation failed error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockToolRepository)
			mockInvoker := new(MockToolInvoker)
			tt.mockSetup(mockRepo, mockInvoker)

			uc := usecase.NewInvokeToolUseCase(mockRepo, mockInvoker, logger)
			actualResult, err := uc.Execute(ctx, tt.inToolName, tt.inParams)

			if tt.wantErr {
				assert.Error(err)
				if tt.expectErrText != "" {
					assert.EqualError(err, tt.expectErrText)
				}
				assert.Nil(actualResult)
			} else {
				assert.NoError(err)
				assert.Equal(tt.wantResult, actualResult)
			}

			mockRepo.AssertExpectations(t)
			mockInvoker.AssertExpectations(t)
		})
	}
}
