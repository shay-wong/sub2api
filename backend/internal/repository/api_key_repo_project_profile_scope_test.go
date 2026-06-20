package repository

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyRepository_IncrementQuotaUsedAndGetStateRequiresProjectAndMemberOwner(t *testing.T) {
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
	require.Contains(t, normalized, "api_keys.project_id = 7")
	require.Contains(t, normalized, "FROM project_members pm")
	require.Contains(t, normalized, "pm.project_id = 7")
	require.Contains(t, normalized, "pm.user_id = api_keys.user_id")
	require.NotContains(t, normalized, "project_profile_bindings")
	require.NotContains(t, normalized, "pp.mode = 'unrestricted'")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAPIKeyRepository_UpdateProjectIDRequiresActiveProjectAndTargetMemberRecord(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAPIKeyRepositoryWithSQL(nil, exec)

	err := repo.UpdateProjectID(service.WithProjectID(context.Background(), 7), 42, 9)

	require.NoError(t, err)
	require.Len(t, exec.execQueries, 1)
	require.Equal(t, []any{int64(42), int64(9), service.StatusActive}, exec.execArgs[0])
	normalized := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, normalized, "WITH target_api_key AS")
	require.Contains(t, normalized, "p.status = $3")
	require.Contains(t, normalized, "pm.project_id = $2")
	require.Contains(t, normalized, "pm.user_id = ak.user_id")
	require.NotContains(t, normalized, "pm.status")
	require.Contains(t, normalized, "ak.group_id IS NULL")
	require.Contains(t, normalized, "FROM groups g")
	require.Contains(t, normalized, "g.id = ak.group_id")
	require.Contains(t, normalized, "g.deleted_at IS NULL")
	require.Contains(t, normalized, "pp.project_id = 9")
	require.Contains(t, normalized, "pp.mode = 'unrestricted'")
	require.Contains(t, normalized, "ppb.resource_type = 'group'")
	require.Contains(t, normalized, "ppb.resource_id = g.id")
	require.Contains(t, normalized, "UPDATE usage_logs ul SET project_id = $2 FROM target_api_key tak WHERE ul.api_key_id = tak.id AND ul.project_id IS DISTINCT FROM $2")
	require.Contains(t, normalized, "UPDATE ops_error_logs oel SET project_id = $2 FROM target_api_key tak WHERE oel.api_key_id = tak.id AND oel.project_id IS DISTINCT FROM $2")
	require.Contains(t, normalized, "UPDATE api_keys ak SET project_id = $2")
}

func TestAPIKeyRepository_UpdateProjectIDRejectsMissingTargetMembership(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(0)}
	repo := newAPIKeyRepositoryWithSQL(nil, exec)

	err := repo.UpdateProjectID(context.Background(), 42, 9)

	require.ErrorIs(t, err, service.ErrProjectAccessForbidden)
	require.Len(t, exec.execQueries, 1)
	require.Equal(t, []any{int64(42), int64(9), service.StatusActive}, exec.execArgs[0])
}
