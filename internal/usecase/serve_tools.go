package usecase

import (
	"context"
	"fmt"
	"log/slog"

	"mcp-bridge/internal/domain"
)

// ServeToolsUseCase provides the functionality to list available tools.
type ServeToolsUseCase struct {
	repository ToolRepository
	logger     *slog.Logger
}

// NewServeToolsUseCase creates a new ServeToolsUseCase.
func NewServeToolsUseCase(repository ToolRepository, logger *slog.Logger) *ServeToolsUseCase {
	return &ServeToolsUseCase{
		repository: repository,
		logger:     logger.With("usecase", "ServeTools"),
	}
}

// Execute retrieves all tools currently stored in the repository.
func (uc *ServeToolsUseCase) Execute(ctx context.Context) ([]domain.Tool, error) {
	uc.logger.Info("Listing tools")
	tools, err := uc.repository.List(ctx)
	if err != nil {
		uc.logger.Error("Failed to list tools from repository", slog.Any("error", err))
		return nil, fmt.Errorf("failed to list tools from repository: %w", err)
	}
	uc.logger.Info("Successfully listed tools", slog.Int("count", len(tools)))
	return tools, nil
}
