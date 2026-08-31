package repository

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateUserAdminAccessRejectsMissingResourcesBeforeUserUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := &operatorPermissionRepository{sql: db}
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT\\s+\\(SELECT COUNT\\(\\*\\) FROM groups").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"groups", "accounts", "proxies", "subscriptions"}).AddRow(1, 0, 1, 1))
	mock.ExpectRollback()

	err = repo.UpdateUserAdminAccess(context.Background(), 42, service.RoleAdmin, []string{service.AdminPermissionAccountsWrite}, service.AdminResourceScope{
		Mode:            service.AdminResourceScopeRestricted,
		GroupIDs:        []int64{10},
		AccountIDs:      []int64{20},
		ProxyIDs:        []int64{30},
		SubscriptionIDs: []int64{40},
	}, nil)

	require.ErrorIs(t, err, service.ErrAdminResourceScopeInvalid)
	require.NoError(t, mock.ExpectationsWereMet())
}
