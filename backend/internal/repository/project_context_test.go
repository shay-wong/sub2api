package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestResolveProjectIDForCreatePrefersContextProject(t *testing.T) {
	ctx := service.WithProjectID(context.Background(), 7)

	projectID, err := resolveProjectIDForCreate(ctx, nil, 99)

	require.NoError(t, err)
	require.Equal(t, int64(7), projectID)
}

func TestResolveProjectIDForCreateUsesRequestedWithoutContext(t *testing.T) {
	projectID, err := resolveProjectIDForCreate(context.Background(), nil, 99)

	require.NoError(t, err)
	require.Equal(t, int64(99), projectID)
}

func TestProjectIDForUpdatePrefersContextProject(t *testing.T) {
	ctx := service.WithProjectID(context.Background(), 7)

	projectID, ok := projectIDForUpdate(ctx, 99)

	require.True(t, ok)
	require.Equal(t, int64(7), projectID)
}

func TestProjectIDForUpdateUsesRequestedWithoutContext(t *testing.T) {
	projectID, ok := projectIDForUpdate(context.Background(), 99)

	require.True(t, ok)
	require.Equal(t, int64(99), projectID)
}

func TestProjectIDForUpdateSkipsEmptyProject(t *testing.T) {
	projectID, ok := projectIDForUpdate(context.Background(), 0)

	require.False(t, ok)
	require.Zero(t, projectID)
}
