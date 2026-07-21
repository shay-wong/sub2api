//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestGetErrorLogByID_APIKeyPrefixAndUpstreamStatus(t *testing.T) {
	ctx := context.Background()
	_, _ = integrationDB.ExecContext(ctx, "TRUNCATE ops_error_logs RESTART IDENTITY CASCADE")
	repo := NewOpsRepository(integrationDB).(*opsRepository)
	projectID := mustDefaultProjectID(t, integrationEntClient)

	// ── Case 1: 带 deleted_key_owner 信息的记录 ──────────────────────────────
	owner := mustCreateUser(t, integrationEntClient, &service.User{
		Email: "deleted-key-owner-" + time.Now().Format("150405.000000000") + "@example.com",
	})

	var insertedID int64
	err := integrationDB.QueryRowContext(ctx, `
		INSERT INTO ops_error_logs (
			project_id, error_phase, error_type, severity, status_code, created_at,
			attempted_key_prefix, deleted_key_owner_user_id, deleted_key_name
		) VALUES (
			$1, 'auth', 'INVALID_API_KEY', 'error', 401, NOW(),
			'sk-test-abc', $2, 'my-deleted-key'
		) RETURNING id`,
		projectID,
		owner.ID,
	).Scan(&insertedID)
	require.NoError(t, err)
	require.Positive(t, insertedID)

	detail, err := repo.GetErrorLogByID(ctx, insertedID)
	require.NoError(t, err)
	require.NotNil(t, detail)

	require.Equal(t, "sk-test-abc", detail.AttemptedKeyPrefix)
	require.NotNil(t, detail.DeletedKeyOwnerUserID)
	require.Equal(t, owner.ID, *detail.DeletedKeyOwnerUserID)
	require.Equal(t, owner.Email, detail.DeletedKeyOwnerEmail)
	require.Equal(t, "my-deleted-key", detail.DeletedKeyName)

	// ── Case 2: 新列全为 NULL 的普通错误记录 ──────────────────────────────────
	var plainID int64
	err = integrationDB.QueryRowContext(ctx, `
		INSERT INTO ops_error_logs (
			project_id, error_phase, error_type, severity, status_code, created_at
		) VALUES (
			$1, 'upstream', 'upstream_error', 'error', 500, NOW()
		) RETURNING id`,
		projectID,
	).Scan(&plainID)
	require.NoError(t, err)
	require.Positive(t, plainID)

	plain, err := repo.GetErrorLogByID(ctx, plainID)
	require.NoError(t, err)
	require.NotNil(t, plain)
	require.Empty(t, plain.AttemptedKeyPrefix)
	require.Nil(t, plain.DeletedKeyOwnerUserID)
	require.Empty(t, plain.DeletedKeyOwnerEmail)
	require.Empty(t, plain.DeletedKeyName)
	require.Empty(t, plain.APIKeyPrefix)

	validID, err := repo.InsertErrorLog(ctx, &service.OpsInsertErrorLogInput{
		ErrorPhase:   "request",
		ErrorType:    "api_error",
		Severity:     "error",
		StatusCode:   402,
		CreatedAt:    time.Now(),
		APIKeyPrefix: "sk-valid",
	})
	require.NoError(t, err)

	valid, err := repo.GetErrorLogByID(ctx, validID)
	require.NoError(t, err)
	require.Equal(t, "sk-valid", valid.APIKeyPrefix)

	zero := 0
	credentialFailureID, err := repo.InsertErrorLog(ctx, &service.OpsInsertErrorLogInput{
		ErrorPhase:         "account_auth",
		ErrorType:          "upstream_error",
		Severity:           "error",
		StatusCode:         503,
		UpstreamStatusCode: &zero,
		CreatedAt:          time.Now(),
	})
	require.NoError(t, err)

	credentialFailure, err := repo.GetErrorLogByID(ctx, credentialFailureID)
	require.NoError(t, err)
	require.NotNil(t, credentialFailure.UpstreamStatusCode)
	require.Zero(t, *credentialFailure.UpstreamStatusCode)
}
