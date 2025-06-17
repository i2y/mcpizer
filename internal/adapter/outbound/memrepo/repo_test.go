package memrepo_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/i2y/mcpizer/internal/adapter/outbound/memrepo"
	"github.com/i2y/mcpizer/internal/domain"
	"github.com/i2y/mcpizer/internal/usecase"
)

func newTestRepo(t *testing.T) *memrepo.InMemoryToolRepository {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return memrepo.NewInMemoryToolRepository(logger)
}

func TestInMemoryToolRepository_SaveAndList(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	ctx := context.Background()
	// repo := newTestRepo(t) // Initial repo state isn't used directly in table tests

	tool1 := domain.Tool{Name: "tool1", Description: "T1"}
	details1 := usecase.InvocationDetails{Type: "http", Host: "host1"}
	tool2 := domain.Tool{Name: "tool2", Description: "T2"}
	details2 := usecase.InvocationDetails{Type: "http", Host: "host2"}

	tests := []struct {
		name        string
		inTools     []domain.Tool
		inDetails   []usecase.InvocationDetails
		wantSaveErr bool
		wantList    []domain.Tool // Expected state after save
	}{
		{
			name:        "Save single tool",
			inTools:     []domain.Tool{tool1},
			inDetails:   []usecase.InvocationDetails{details1},
			wantSaveErr: false,
			wantList:    []domain.Tool{tool1},
		},
		{
			name:        "Save multiple tools",
			inTools:     []domain.Tool{tool1, tool2},
			inDetails:   []usecase.InvocationDetails{details1, details2},
			wantSaveErr: false,
			wantList:    []domain.Tool{tool1, tool2}, // Order might vary in List
		},
		{
			name:        "Save empty list",
			inTools:     []domain.Tool{},
			inDetails:   []usecase.InvocationDetails{},
			wantSaveErr: false,
			wantList:    []domain.Tool{},
		},
		{
			name:        "Save with empty tool name (skipped)",
			inTools:     []domain.Tool{{Name: "", Description: "Empty"}, tool1},
			inDetails:   []usecase.InvocationDetails{{Type: "http"}, details1},
			wantSaveErr: false,
			wantList:    []domain.Tool{tool1}, // Only tool1 should be saved
		},
		{
			name:        "Error on mismatch length",
			inTools:     []domain.Tool{tool1},
			inDetails:   []usecase.InvocationDetails{details1, details2}, // Mismatch
			wantSaveErr: true,
			wantList:    []domain.Tool{}, // Expect state to be unchanged on error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset repo for each test case focusing on save->list
			repo := newTestRepo(t)

			err := repo.Save(ctx, tt.inTools, tt.inDetails)

			if tt.wantSaveErr {
				assert.Error(err)
			} else {
				assert.NoError(err)
			}

			listedTools, listErr := repo.List(ctx)
			require.NoError(listErr) // List should not error here

			// Use ElementsMatch because the order from List is not guaranteed
			assert.ElementsMatch(tt.wantList, listedTools)
		})
	}
}

func TestInMemoryToolRepository_FindByName(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	ctx := context.Background()
	repo := newTestRepo(t)

	tool1 := domain.Tool{Name: "tool1", Description: "T1"}
	details1 := usecase.InvocationDetails{Type: "http", Host: "host1"}
	tool2 := domain.Tool{Name: "tool2", Description: "T2"}
	details2 := usecase.InvocationDetails{Type: "http", Host: "host2"}

	// Pre-populate the repo
	err := repo.Save(ctx, []domain.Tool{tool1, tool2}, []usecase.InvocationDetails{details1, details2})
	require.NoError(err)

	tests := []struct {
		name           string
		inName         string
		wantTool       *domain.Tool
		wantDetails    *usecase.InvocationDetails
		wantFindErr    bool // Indicates if any error is expected for FindToolByName
		wantDetailsErr bool // Indicates if any error is expected for FindInvocationDetailsByName
	}{
		{
			name:        "Find existing tool1",
			inName:      "tool1",
			wantTool:    &tool1,
			wantDetails: &details1,
		},
		{
			name:        "Find existing tool2",
			inName:      "tool2",
			wantTool:    &tool2,
			wantDetails: &details2,
		},
		{
			name:           "Find non-existent tool",
			inName:         "tool3",
			wantTool:       nil,
			wantDetails:    nil,
			wantFindErr:    true, // Expecting ErrToolNotFound
			wantDetailsErr: true, // Expecting ErrToolNotFound
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualTool, err := repo.FindToolByName(ctx, tt.inName)
			if tt.wantFindErr {
				// Check for the specific expected error
				assert.ErrorIs(err, usecase.ErrToolNotFound, "Expected ErrToolNotFound for FindToolByName")
				assert.Nil(actualTool)
			} else {
				assert.NoError(err)
				assert.Equal(tt.wantTool, actualTool)
			}

			actualDetails, err := repo.FindInvocationDetailsByName(ctx, tt.inName)
			if tt.wantDetailsErr {
				// Check for the specific expected error
				assert.ErrorIs(err, usecase.ErrToolNotFound, "Expected ErrToolNotFound for FindInvocationDetailsByName")
				assert.Nil(actualDetails)
			} else {
				assert.NoError(err)
				assert.Equal(tt.wantDetails, actualDetails)
			}
		})
	}
}

func TestInMemoryToolRepository_SaveOverwrite(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	ctx := context.Background()
	repo := newTestRepo(t)

	toolV1 := domain.Tool{Name: "overwrite", Description: "V1"}
	detailsV1 := usecase.InvocationDetails{Type: "http", Host: "v1"}
	toolV2 := domain.Tool{Name: "overwrite", Description: "V2"}
	detailsV2 := usecase.InvocationDetails{Type: "http", Host: "v2"}

	// Save V1
	err := repo.Save(ctx, []domain.Tool{toolV1}, []usecase.InvocationDetails{detailsV1})
	require.NoError(err)

	foundTool, err := repo.FindToolByName(ctx, "overwrite")
	require.NoError(err)
	assert.Equal(&toolV1, foundTool)
	foundDetails, err := repo.FindInvocationDetailsByName(ctx, "overwrite")
	require.NoError(err)
	assert.Equal(&detailsV1, foundDetails)

	// Save V2 (same name)
	err = repo.Save(ctx, []domain.Tool{toolV2}, []usecase.InvocationDetails{detailsV2})
	require.NoError(err)

	// Check if V2 overwrote V1
	foundTool, err = repo.FindToolByName(ctx, "overwrite")
	require.NoError(err)
	assert.Equal(&toolV2, foundTool)
	foundDetails, err = repo.FindInvocationDetailsByName(ctx, "overwrite")
	require.NoError(err)
	assert.Equal(&detailsV2, foundDetails)

	// List should only contain V2
	list, err := repo.List(ctx)
	require.NoError(err)
	assert.Len(list, 1)
	assert.Equal(toolV2, list[0])
}
