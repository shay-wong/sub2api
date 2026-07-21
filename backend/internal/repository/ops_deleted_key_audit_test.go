package repository

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestLookupDeletedKeyAuditQueriesByDigest(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	const rawKey = "sk-deleted-secret"
	wantDigest := fmt.Sprintf("%x", sha256.Sum256([]byte(rawKey)))
	mock.ExpectQuery("FROM deleted_api_key_audits").
		WithArgs(wantDigest).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "key_name"}).AddRow(int64(42), "deleted key"))

	repo := &opsRepository{db: db}
	result, err := repo.LookupDeletedKeyAudit(context.Background(), rawKey)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int64(42), result.UserID)
	require.Equal(t, "deleted key", result.KeyName)
	require.NoError(t, mock.ExpectationsWereMet())
}
