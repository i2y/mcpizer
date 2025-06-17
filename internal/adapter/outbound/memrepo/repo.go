package memrepo

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/i2y/mcpizer/internal/domain"
	"github.com/i2y/mcpizer/internal/usecase"
)

// InMemoryToolRepository provides an in-memory implementation of the ToolRepository.
// NOTE: This implementation is not persistent and data will be lost on restart.
type InMemoryToolRepository struct {
	mu                sync.RWMutex
	tools             map[string]domain.Tool               // Map tool name to Tool definition
	invocationDetails map[string]usecase.InvocationDetails // Map tool name to Invocation details
	logger            *slog.Logger
}

// NewInMemoryToolRepository creates a new in-memory repository.
func NewInMemoryToolRepository(logger *slog.Logger) *InMemoryToolRepository {
	return &InMemoryToolRepository{
		tools:             make(map[string]domain.Tool),
		invocationDetails: make(map[string]usecase.InvocationDetails),
		logger:            logger.With("component", "mem_repo"),
	}
}

// Save stores the given tools and their corresponding invocation details.
// It assumes tools and details slices correspond by index and have the same length.
func (r *InMemoryToolRepository) Save(ctx context.Context, tools []domain.Tool, details []usecase.InvocationDetails) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(tools) != len(details) {
		msg := fmt.Sprintf("mismatch between number of tools (%d) and invocation details (%d)", len(tools), len(details))
		r.logger.Error("Failed to save tools and details", slog.String("reason", msg))
		return fmt.Errorf("save failed: %s", msg)
	}

	count := 0
	for i, tool := range tools {
		if tool.Name == "" {
			r.logger.Warn("Skipping tool with empty name during save", slog.Int("index", i))
			continue
		}
		r.tools[tool.Name] = tool
		r.invocationDetails[tool.Name] = details[i]
		count++
	}
	r.logger.Info("Saved tools and invocation details", slog.Int("count", count), slog.Int("total_tools", len(r.tools)))
	return nil
}

// List returns all tools currently stored in memory.
func (r *InMemoryToolRepository) List(ctx context.Context) ([]domain.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]domain.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		list = append(list, tool)
	}
	r.logger.Debug("Listed tools from repository", slog.Int("count", len(list)))
	return list, nil
}

// FindToolByName retrieves a tool definition by its name.
func (r *InMemoryToolRepository) FindToolByName(ctx context.Context, name string) (*domain.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	if !ok {
		r.logger.Warn("Tool definition not found", slog.String("tool_name", name))
		return nil, usecase.ErrToolNotFound
	}
	r.logger.Debug("Found tool definition", slog.String("tool_name", name))
	return &tool, nil
}

// FindInvocationDetailsByName retrieves invocation details by tool name.
func (r *InMemoryToolRepository) FindInvocationDetailsByName(ctx context.Context, name string) (*usecase.InvocationDetails, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	details, ok := r.invocationDetails[name]
	if !ok {
		r.logger.Warn("Invocation details not found", slog.String("tool_name", name))
		return nil, usecase.ErrToolNotFound
	}
	r.logger.Debug("Found invocation details", slog.String("tool_name", name))
	return &details, nil
}
