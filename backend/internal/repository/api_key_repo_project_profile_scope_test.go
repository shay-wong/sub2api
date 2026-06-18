package repository

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyRepository_IncrementQuotaUsedAndGetStateUsesProjectProfileScope(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	rows := sqlmock.NewRows([]string{"quota_used", "quota", "key", "status"}).
		AddRow(12.5, 20.0, "sk-profile", service.StatusActive)
	mock.ExpectQuery("UPDATE api_keys").
		WithArgs(2.5, service.StatusAPIKeyQuotaExhausted, int64(42)).
		WillReturnRows(rows)

	var capturedSQL string
	repo := newAPIKeyRepositoryWithSQL(nil, captureQuerySQL{db: db, captured: &capturedSQL})

	state, err := repo.IncrementQuotaUsedAndGetState(service.WithProjectID(context.Background(), 7), 42, 2.5)

	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, 12.5, state.QuotaUsed)
	require.Equal(t, 20.0, state.Quota)
	require.Equal(t, "sk-profile", state.Key)
	require.Equal(t, service.StatusActive, state.Status)
	normalized := normalizeSQLWhitespace(capturedSQL)
	require.NotContains(t, normalized, "api_keys.project_id = $4")
	require.Contains(t, normalized, "pp.mode = 'unrestricted'")
	require.Contains(t, normalized, "FROM project_profiles pp")
	require.Contains(t, normalized, "JOIN project_profile_bindings ppb")
	require.Contains(t, normalized, "ppb.resource_type = 'api_key'")
	require.Contains(t, normalized, "ppb.resource_id = api_keys.id")
	require.Contains(t, normalized, "ppb.resource_type = 'user'")
	require.Contains(t, normalized, "ppb.resource_id = api_keys.user_id")
	require.Contains(t, normalized, "ppb.resource_type = 'group'")
	require.Contains(t, normalized, "ppb.resource_id = api_keys.group_id")
	require.NoError(t, mock.ExpectationsWereMet())
}
