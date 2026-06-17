//go:build unit

package repository

import (
	"context"
	"database/sql"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	dbuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestEnsureSimpleModeAdminConcurrencyUpgradesSuperAdmins(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:simple_mode_admin_concurrency?mode=memory&cache=shared&_fk=1")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	superAdmin := createConcurrencyTestUser(t, ctx, client, "root@example.com", service.RoleSuperAdmin, simpleModeLegacyAdminConcurrency)
	legacyAdmin := createConcurrencyTestUser(t, ctx, client, "admin@example.com", service.RoleAdmin, simpleModeLegacyAdminConcurrency)
	ordinaryUser := createConcurrencyTestUser(t, ctx, client, "user@example.com", service.RoleUser, simpleModeLegacyAdminConcurrency)
	customSuperAdmin := createConcurrencyTestUser(t, ctx, client, "custom-root@example.com", service.RoleSuperAdmin, 12)

	require.NoError(t, ensureSimpleModeAdminConcurrency(ctx, client))

	assertConcurrency(t, ctx, client, superAdmin.ID, simpleModeTargetAdminConcurrency)
	assertConcurrency(t, ctx, client, legacyAdmin.ID, simpleModeTargetAdminConcurrency)
	assertConcurrency(t, ctx, client, ordinaryUser.ID, simpleModeLegacyAdminConcurrency)
	assertConcurrency(t, ctx, client, customSuperAdmin.ID, 12)

	require.NoError(t, ensureSimpleModeAdminConcurrency(ctx, client))
	assertConcurrency(t, ctx, client, superAdmin.ID, simpleModeTargetAdminConcurrency)
}

func createConcurrencyTestUser(t *testing.T, ctx context.Context, client *dbent.Client, email, role string, concurrency int) *dbent.User {
	t.Helper()
	user, err := client.User.Create().
		SetEmail(email).
		SetPasswordHash("hash").
		SetRole(role).
		SetStatus(service.StatusActive).
		SetConcurrency(concurrency).
		Save(ctx)
	require.NoError(t, err)
	return user
}

func assertConcurrency(t *testing.T, ctx context.Context, client *dbent.Client, userID int64, want int) {
	t.Helper()
	got, err := client.User.Query().Where(dbuser.IDEQ(userID)).Only(ctx)
	require.NoError(t, err)
	require.Equal(t, want, got.Concurrency)
}
