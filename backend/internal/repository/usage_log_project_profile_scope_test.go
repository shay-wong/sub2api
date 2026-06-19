package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newUsageLogProfileScopeSQLite(t *testing.T) (*usageLogRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:usage_log_profile_scope?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return newUsageLogRepositoryWithSQL(client, db), client
}

type usageLogProfileScopeFixture struct {
	ProjectID     int64
	BoundUserID   int64
	OtherUserID   int64
	BoundKeyID    int64
	OtherKeyID    int64
	ForeignUserID int64
	ForeignKeyID  int64
}

func createUsageLogProfileScopeFixture(t *testing.T, ctx context.Context, client *dbent.Client) usageLogProfileScopeFixture {
	t.Helper()

	project, err := client.Project.Create().
		SetName("Scoped Project").
		SetSlug("scoped-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	otherProject, err := client.Project.Create().
		SetName("Other Project").
		SetSlug("other-profile-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	boundUser, err := client.User.Create().
		SetEmail("bound-profile-user@test.com").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	otherUser, err := client.User.Create().
		SetEmail("other-profile-user@test.com").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	boundAccount, err := client.Account.Create().
		SetProjectID(otherProject.ID).
		SetName("bound-profile-account").
		SetPlatform(service.PlatformAnthropic).
		SetType("api_key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	otherAccount, err := client.Account.Create().
		SetProjectID(otherProject.ID).
		SetName("other-profile-account").
		SetPlatform(service.PlatformAnthropic).
		SetType("api_key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	boundKey, err := client.APIKey.Create().
		SetProjectID(project.ID).
		SetUserID(boundUser.ID).
		SetKey("sk-bound-profile").
		SetName("bound-profile-key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	otherKey, err := client.APIKey.Create().
		SetProjectID(project.ID).
		SetUserID(otherUser.ID).
		SetKey("sk-other-profile").
		SetName("other-profile-key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	foreignUser, err := client.User.Create().
		SetEmail("foreign-profile-user@test.com").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	foreignKey, err := client.APIKey.Create().
		SetProjectID(otherProject.ID).
		SetUserID(foreignUser.ID).
		SetKey("sk-foreign-profile").
		SetName("foreign-profile-key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	foreignAccount, err := client.Account.Create().
		SetProjectID(otherProject.ID).
		SetName("foreign-profile-account").
		SetPlatform(service.PlatformAnthropic).
		SetType("api_key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	when := time.Now().UTC()
	_, err = client.ProjectMember.Create().
		SetProjectID(project.ID).
		SetUserID(boundUser.ID).
		SetRole(service.ProjectRoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectMember.Create().
		SetProjectID(project.ID).
		SetUserID(otherUser.ID).
		SetRole(service.ProjectRoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetProjectID(project.ID).
		SetUserID(boundUser.ID).
		SetAPIKeyID(boundKey.ID).
		SetAccountID(boundAccount.ID).
		SetRequestID("req-bound-profile").
		SetModel("gpt-5").
		SetInputTokens(10).
		SetOutputTokens(5).
		SetActualCost(1).
		SetCreatedAt(when).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.UsageLog.Create().
		SetProjectID(project.ID).
		SetUserID(otherUser.ID).
		SetAPIKeyID(otherKey.ID).
		SetAccountID(otherAccount.ID).
		SetRequestID("req-other-profile").
		SetModel("gpt-5").
		SetInputTokens(10).
		SetOutputTokens(5).
		SetActualCost(1).
		SetCreatedAt(when).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.UsageLog.Create().
		SetProjectID(otherProject.ID).
		SetUserID(foreignUser.ID).
		SetAPIKeyID(foreignKey.ID).
		SetAccountID(foreignAccount.ID).
		SetRequestID("req-foreign-profile").
		SetModel("gpt-5").
		SetInputTokens(10).
		SetOutputTokens(5).
		SetActualCost(1).
		SetCreatedAt(when).
		Save(ctx)
	require.NoError(t, err)

	profile, err := client.ProjectProfile.Create().
		SetProjectID(project.ID).
		SetName("Restricted").
		SetMode(service.ProjectProfileModeRestricted).
		SetIsActive(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectProfileBinding.Create().
		SetProjectProfileID(profile.ID).
		SetResourceType(service.ProjectResourceTypeAccount).
		SetResourceID(boundAccount.ID).
		Save(ctx)
	require.NoError(t, err)

	return usageLogProfileScopeFixture{
		ProjectID:     project.ID,
		BoundUserID:   boundUser.ID,
		OtherUserID:   otherUser.ID,
		BoundKeyID:    boundKey.ID,
		OtherKeyID:    otherKey.ID,
		ForeignUserID: foreignUser.ID,
		ForeignKeyID:  foreignKey.ID,
	}
}

func TestUsageLogProjectProfileRestrictedScopesByConfiguredAccount(t *testing.T) {
	repo, client := newUsageLogProfileScopeSQLite(t)
	ctx := context.Background()
	fixture := createUsageLogProfileScopeFixture(t, ctx, client)

	count, err := repo.CountWithFilters(service.WithProjectID(ctx, fixture.ProjectID), usagestats.UsageLogFilters{})
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	count, err = repo.CountWithFilters(service.WithProjectID(ctx, fixture.ProjectID), usagestats.UsageLogFilters{UserID: fixture.BoundUserID})
	require.NoError(t, err)
	require.Equal(t, int64(1), count)

	count, err = repo.CountWithFilters(service.WithProjectID(ctx, fixture.ProjectID), usagestats.UsageLogFilters{UserID: fixture.OtherUserID})
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestUsageLogProjectProfileUnrestrictedAllowsAllResources(t *testing.T) {
	repo, client := newUsageLogProfileScopeSQLite(t)
	ctx := context.Background()
	fixture := createUsageLogProfileScopeFixture(t, ctx, client)

	_, err := client.ProjectProfile.Update().
		SetIsActive(false).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectProfile.Create().
		SetProjectID(fixture.ProjectID).
		SetName("Unrestricted").
		SetMode(service.ProjectProfileModeUnrestricted).
		SetIsActive(true).
		Save(ctx)
	require.NoError(t, err)

	count, err := repo.CountWithFilters(service.WithProjectID(ctx, fixture.ProjectID), usagestats.UsageLogFilters{})
	require.NoError(t, err)
	require.Equal(t, int64(2), count)

	projectAccount, err := client.Account.Create().
		SetProjectID(fixture.ProjectID).
		SetName("unrestricted-project-account").
		SetPlatform(service.PlatformAnthropic).
		SetType("api_key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	projectKey, err := client.APIKey.Create().
		SetProjectID(fixture.ProjectID).
		SetUserID(fixture.BoundUserID).
		SetKey("sk-unrestricted-project").
		SetName("unrestricted-project-key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.UsageLog.Create().
		SetProjectID(fixture.ProjectID).
		SetUserID(fixture.BoundUserID).
		SetAPIKeyID(projectKey.ID).
		SetAccountID(projectAccount.ID).
		SetRequestID("req-unrestricted-project-log").
		SetModel("gpt-5").
		SetInputTokens(10).
		SetOutputTokens(5).
		SetActualCost(1).
		SetCreatedAt(time.Now().UTC()).
		Save(ctx)
	require.NoError(t, err)

	count, err = repo.CountWithFilters(service.WithProjectID(ctx, fixture.ProjectID), usagestats.UsageLogFilters{})
	require.NoError(t, err)
	require.Equal(t, int64(3), count)

	count, err = repo.CountWithFilters(service.WithProjectID(ctx, fixture.ProjectID), usagestats.UsageLogFilters{UserID: fixture.ForeignUserID})
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestUsageLogProjectProfileRestrictedAppliesToUserDashboard(t *testing.T) {
	repo, client := newUsageLogProfileScopeSQLite(t)
	ctx := context.Background()
	fixture := createUsageLogProfileScopeFixture(t, ctx, client)
	projectCtx := service.WithProjectID(ctx, fixture.ProjectID)

	stats, err := repo.GetUserDashboardStats(projectCtx, fixture.BoundUserID)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.TotalAPIKeys)
	require.Equal(t, int64(1), stats.TotalRequests)

	stats, err = repo.GetUserDashboardStats(projectCtx, fixture.OtherUserID)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.TotalAPIKeys)
	require.Equal(t, int64(0), stats.TotalRequests)
}

func TestUsageLogProjectProfileRestrictedAppliesToAPIKeyDashboard(t *testing.T) {
	repo, client := newUsageLogProfileScopeSQLite(t)
	ctx := context.Background()
	fixture := createUsageLogProfileScopeFixture(t, ctx, client)
	projectCtx := service.WithProjectID(ctx, fixture.ProjectID)

	stats, err := repo.GetAPIKeyDashboardStats(projectCtx, fixture.BoundKeyID)
	require.NoError(t, err)
	require.Equal(t, int64(1), stats.TotalRequests)

	stats, err = repo.GetAPIKeyDashboardStats(projectCtx, fixture.OtherKeyID)
	require.NoError(t, err)
	require.Equal(t, int64(0), stats.TotalRequests)
}

func TestUsageLogProjectProfileRestrictedAppliesToDashboardStats(t *testing.T) {
	repo, client := newUsageLogProfileScopeSQLite(t)
	ctx := context.Background()
	fixture := createUsageLogProfileScopeFixture(t, ctx, client)

	stats, err := repo.GetDashboardStats(service.WithProjectID(ctx, fixture.ProjectID))
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.TotalUsers)
	require.Equal(t, int64(2), stats.TotalAPIKeys)
	require.Equal(t, int64(1), stats.TotalAccounts)
	require.Equal(t, int64(1), stats.NormalAccounts)
	require.Equal(t, int64(1), stats.TotalRequests)
}
