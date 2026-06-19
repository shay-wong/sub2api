//go:build unit

package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageBillingRepositoryApplyScopesAPIKeyAndAccountByContextProject(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewUsageBillingRepository(nil, db)
	ctx := service.WithProjectID(context.Background(), 77)

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO usage_billing_dedup").
		WithArgs("req-project-scope", int64(11), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	mock.ExpectQuery("SELECT request_fingerprint\\s+FROM usage_billing_dedup_archive").
		WithArgs("req-project-scope", int64(11)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("UPDATE api_keys").
		WithArgs(2.5, int64(11), service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted).
		WillReturnRows(sqlmock.NewRows([]string{"exhausted"}).AddRow(false))
	mock.ExpectExec("UPDATE api_keys SET").
		WithArgs(2.5, int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("UPDATE accounts SET extra").
		WithArgs(2.5, int64(22)).
		WillReturnRows(sqlmock.NewRows([]string{
			"quota_used",
			"quota_limit",
			"quota_daily_used",
			"quota_daily_limit",
			"quota_weekly_used",
			"quota_weekly_limit",
		}).AddRow(2.5, 10.0, 2.5, 0.0, 2.5, 0.0))
	mock.ExpectCommit()

	result, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:           "req-project-scope",
		APIKeyID:            11,
		AccountID:           22,
		AccountType:         service.AccountTypeAPIKey,
		APIKeyQuotaCost:     2.5,
		APIKeyRateLimitCost: 2.5,
		AccountQuotaCost:    2.5,
	})

	require.NoError(t, err)
	require.True(t, result.Applied)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUsageBillingProjectScopeSQLUsesActiveProfileBindings(t *testing.T) {
	ctx := service.WithProjectID(context.Background(), 77)
	apiKeyClause := projectProfileScopeSQL(77, apiKeySQLScopeResources("api_keys"))
	accountClause := projectProfileScopeSQL(77, projectSQLScopeResources{AccountID: "accounts.id"})
	subscriptionClause := projectProfileScopeSQL(77, projectSQLScopeResources{
		SubscriptionID: "us.id",
		UserID:         "us.user_id",
		GroupID:        "us.group_id",
	})
	groupClause := projectUserGroupScopeSQL(77, "u.id", "g.id")

	for name, clause := range map[string]string{
		"account":      accountClause,
		"subscription": subscriptionClause,
		"group":        groupClause,
	} {
		normalized := normalizeSQLWhitespace(clause)
		require.Contains(t, normalized, "pp.mode = 'unrestricted'", name)
		require.Contains(t, normalized, "FROM project_profiles pp", name)
		require.NotContains(t, normalized, ".project_id = $", name)
	}

	normalizedAPIKey := normalizeSQLWhitespace(apiKeyClause)
	require.Contains(t, normalizedAPIKey, "api_keys.project_id = 77")
	require.Contains(t, normalizedAPIKey, "FROM project_members pm")
	require.Contains(t, normalizedAPIKey, "pm.user_id = api_keys.user_id")
	require.NotContains(t, normalizedAPIKey, "project_profile_bindings")

	normalizedAccount := normalizeSQLWhitespace(accountClause)
	require.Contains(t, normalizedAccount, "ppb.resource_type = 'account'")
	require.Contains(t, normalizedAccount, "ppb.resource_id = accounts.id")

	normalizedSubscription := normalizeSQLWhitespace(subscriptionClause)
	require.Contains(t, normalizedSubscription, "ppb.resource_type = 'subscription'")
	require.Contains(t, normalizedSubscription, "ppb.resource_id = us.id")
	require.Contains(t, normalizedSubscription, "ppb.resource_type = 'group'")
	require.Contains(t, normalizedSubscription, "ppb.resource_id = us.group_id")
	require.NotContains(t, normalizedSubscription, "ppb.resource_type = 'user'")

	normalizedGroup := normalizeSQLWhitespace(groupClause)
	require.Contains(t, normalizedGroup, "FROM project_members pm")
	require.Contains(t, normalizedGroup, "pm.user_id = u.id")
	require.Contains(t, normalizedGroup, "ppb.resource_type = 'group'")
	require.Contains(t, normalizedGroup, "ppb.resource_id = g.id")
	require.NotContains(t, normalizedGroup, "ppb.resource_type = 'user'")

	_, ok := service.ProjectIDFromContext(ctx)
	require.True(t, ok)
}

func TestUsageBillingRepositoryApplyReturnsNotFoundWhenContextProjectDoesNotOwnAPIKey(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := NewUsageBillingRepository(nil, db)
	ctx := service.WithProjectID(context.Background(), 77)

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO usage_billing_dedup").
		WithArgs("req-wrong-project", int64(11), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	mock.ExpectQuery("SELECT request_fingerprint\\s+FROM usage_billing_dedup_archive").
		WithArgs("req-wrong-project", int64(11)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("UPDATE api_keys").
		WithArgs(2.5, int64(11), service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	result, err := repo.Apply(ctx, &service.UsageBillingCommand{
		RequestID:       "req-wrong-project",
		APIKeyID:        11,
		APIKeyQuotaCost: 2.5,
	})

	require.Error(t, err)
	require.Nil(t, result)
	require.NoError(t, mock.ExpectationsWereMet())
}
