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
		WithArgs(2.5, int64(11), service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted, int64(77)).
		WillReturnRows(sqlmock.NewRows([]string{"exhausted"}).AddRow(false))
	mock.ExpectExec("UPDATE api_keys SET").
		WithArgs(2.5, int64(11), int64(77)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("UPDATE accounts SET extra").
		WithArgs(2.5, int64(22), int64(77)).
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
		WithArgs(2.5, int64(11), service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted, int64(77)).
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
