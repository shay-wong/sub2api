package repository

import (
	"context"
	"database/sql"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/ent/projectprofilebinding"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newProjectProfileResourceScopeSQLite(t *testing.T) *dbent.Client {
	t.Helper()

	db, err := sql.Open("sqlite", "file:project_profile_resource_scope?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestProjectScopedAccountPredicateRequiresActiveProfileBinding(t *testing.T) {
	client := newProjectProfileResourceScopeSQLite(t)
	ctx := context.Background()

	project, err := client.Project.Create().
		SetName("Scoped Project").
		SetSlug("scoped-resource-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	bound, err := client.Account.Create().
		SetProjectID(project.ID).
		SetName("bound-account").
		SetPlatform(service.PlatformAnthropic).
		SetType("api_key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	unbound, err := client.Account.Create().
		SetProjectID(project.ID).
		SetName("same-project-unbound-account").
		SetPlatform(service.PlatformAnthropic).
		SetType("api_key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	foreignProject, err := client.Project.Create().
		SetName("Foreign Scoped Project").
		SetSlug("foreign-scoped-resource-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	foreign, err := client.Account.Create().
		SetProjectID(foreignProject.ID).
		SetName("foreign-account").
		SetPlatform(service.PlatformAnthropic).
		SetType("api_key").
		SetStatus(service.StatusActive).
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
		SetResourceID(bound.ID).
		Save(ctx)
	require.NoError(t, err)

	projectCtx := service.WithProjectID(ctx, project.ID)
	visible, err := client.Account.Query().
		Where(projectScopedAccountPredicate(projectCtx)...).
		IDs(ctx)
	require.NoError(t, err)
	require.Equal(t, []int64{bound.ID}, visible)
	require.NotContains(t, visible, unbound.ID)

	_, err = client.ProjectProfile.UpdateOneID(profile.ID).
		SetMode(service.ProjectProfileModeUnrestricted).
		Save(ctx)
	require.NoError(t, err)
	visible, err = client.Account.Query().
		Where(projectScopedAccountPredicate(projectCtx)...).
		IDs(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{bound.ID, unbound.ID, foreign.ID}, visible)
}

func TestAccountRepositoryListUsesActiveProjectProfileScope(t *testing.T) {
	client := newProjectProfileResourceScopeSQLite(t)
	ctx := context.Background()

	project, err := client.Project.Create().
		SetName("Account List Project").
		SetSlug("account-list-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	bound, err := client.Account.Create().
		SetProjectID(project.ID).
		SetName("bound-list-account").
		SetPlatform(service.PlatformAnthropic).
		SetType("api_key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	unbound, err := client.Account.Create().
		SetProjectID(project.ID).
		SetName("unbound-list-account").
		SetPlatform(service.PlatformAnthropic).
		SetType("api_key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	foreignProject, err := client.Project.Create().
		SetName("Foreign Account List Project").
		SetSlug("foreign-account-list-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	foreign, err := client.Account.Create().
		SetProjectID(foreignProject.ID).
		SetName("foreign-list-account").
		SetPlatform(service.PlatformAnthropic).
		SetType("api_key").
		SetStatus(service.StatusActive).
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
		SetResourceID(bound.ID).
		Save(ctx)
	require.NoError(t, err)

	repo := newAccountRepositoryWithSQL(client, nil, nil)
	projectCtx := service.WithProjectID(ctx, project.ID)
	accounts, _, err := repo.ListWithFilters(projectCtx, pagination.PaginationParams{Page: 1, PageSize: 20}, "", "", "", "", 0, "")
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, bound.ID, accounts[0].ID)

	_, err = client.ProjectProfile.UpdateOneID(profile.ID).
		SetMode(service.ProjectProfileModeUnrestricted).
		Save(ctx)
	require.NoError(t, err)
	accounts, _, err = repo.ListWithFilters(projectCtx, pagination.PaginationParams{Page: 1, PageSize: 20}, "", "", "", "", 0, "")
	require.NoError(t, err)
	ids := make([]int64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, account.ID)
	}
	require.ElementsMatch(t, []int64{bound.ID, unbound.ID, foreign.ID}, ids)
}

func TestAccountRepositoryListDoesNotExpandAccountsFromBoundProjectGroups(t *testing.T) {
	client := newProjectProfileResourceScopeSQLite(t)
	ctx := context.Background()

	home, err := client.Project.Create().
		SetName("Grouped Account Home").
		SetSlug("grouped-account-home").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	workspace, err := client.Project.Create().
		SetName("Grouped Account Workspace").
		SetSlug("grouped-account-workspace").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	group, err := client.Group.Create().
		SetProjectID(home.ID).
		SetName("bound-group-with-accounts").
		SetStatus(service.StatusActive).
		SetPlatform(service.PlatformAnthropic).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		Save(ctx)
	require.NoError(t, err)
	boundAccount, err := client.Account.Create().
		SetProjectID(home.ID).
		SetName("directly-bound-visible-account").
		SetPlatform(service.PlatformAnthropic).
		SetType("api_key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	groupedAccount, err := client.Account.Create().
		SetProjectID(home.ID).
		SetName("grouped-hidden-account").
		SetPlatform(service.PlatformAnthropic).
		SetType("api_key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	unboundAccount, err := client.Account.Create().
		SetProjectID(home.ID).
		SetName("grouped-hidden-account").
		SetPlatform(service.PlatformAnthropic).
		SetType("api_key").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.AccountGroup.Create().
		SetAccountID(groupedAccount.ID).
		SetGroupID(group.ID).
		SetPriority(1).
		Save(ctx)
	require.NoError(t, err)

	profile, err := client.ProjectProfile.Create().
		SetProjectID(workspace.ID).
		SetName("Restricted").
		SetMode(service.ProjectProfileModeRestricted).
		SetIsActive(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectProfileBinding.Create().
		SetProjectProfileID(profile.ID).
		SetResourceType(service.ProjectResourceTypeGroup).
		SetResourceID(group.ID).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectProfileBinding.Create().
		SetProjectProfileID(profile.ID).
		SetResourceType(service.ProjectResourceTypeAccount).
		SetResourceID(boundAccount.ID).
		Save(ctx)
	require.NoError(t, err)

	repo := newAccountRepositoryWithSQL(client, nil, nil)
	accounts, _, err := repo.ListWithFilters(service.WithProjectID(ctx, workspace.ID), pagination.PaginationParams{Page: 1, PageSize: 20}, "", "", "", "", 0, "")
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	require.Equal(t, boundAccount.ID, accounts[0].ID)
	require.NotEqual(t, groupedAccount.ID, accounts[0].ID)
	require.NotEqual(t, unboundAccount.ID, accounts[0].ID)
}

func TestAccountRepositoryUpdateKeepsHomeProjectForProfileBoundResource(t *testing.T) {
	client := newProjectProfileResourceScopeSQLite(t)
	ctx := context.Background()

	home, err := client.Project.Create().
		SetName("Account Home Project").
		SetSlug("account-home-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	workspace, err := client.Project.Create().
		SetName("Account Workspace Project").
		SetSlug("account-workspace-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	profile, err := client.ProjectProfile.Create().
		SetProjectID(workspace.ID).
		SetName("Restricted").
		SetMode(service.ProjectProfileModeRestricted).
		SetIsActive(true).
		Save(ctx)
	require.NoError(t, err)
	account, err := client.Account.Create().
		SetProjectID(home.ID).
		SetName("profile-bound-account").
		SetPlatform(service.PlatformAnthropic).
		SetType(service.AccountTypeOAuth).
		SetStatus(service.StatusActive).
		SetCredentials(map[string]any{}).
		SetExtra(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectProfileBinding.Create().
		SetProjectProfileID(profile.ID).
		SetResourceType(service.ProjectResourceTypeAccount).
		SetResourceID(account.ID).
		Save(ctx)
	require.NoError(t, err)

	repo := newAccountRepositoryWithSQL(client, nil, nil)
	input := accountEntityToService(account)
	input.Name = "edited-profile-bound-account"
	err = repo.Update(service.WithProjectID(ctx, workspace.ID), input)
	require.NoError(t, err)

	updated, err := client.Account.Get(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, home.ID, updated.ProjectID)
	require.Equal(t, "edited-profile-bound-account", updated.Name)
}

func TestGroupRepositoryUpdateKeepsHomeProjectForProfileBoundResource(t *testing.T) {
	client := newProjectProfileResourceScopeSQLite(t)
	ctx := context.Background()

	home, err := client.Project.Create().
		SetName("Group Home Project").
		SetSlug("group-home-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	workspace, err := client.Project.Create().
		SetName("Group Workspace Project").
		SetSlug("group-workspace-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	profile, err := client.ProjectProfile.Create().
		SetProjectID(workspace.ID).
		SetName("Restricted").
		SetMode(service.ProjectProfileModeRestricted).
		SetIsActive(true).
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetProjectID(home.ID).
		SetName("profile-bound-group").
		SetStatus(service.StatusActive).
		SetPlatform(service.PlatformAnthropic).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectProfileBinding.Create().
		SetProjectProfileID(profile.ID).
		SetResourceType(service.ProjectResourceTypeGroup).
		SetResourceID(group.ID).
		Save(ctx)
	require.NoError(t, err)

	repo := newGroupRepositoryWithSQL(client, nil)
	input := groupEntityToService(group)
	input.Name = "edited-profile-bound-group"
	err = repo.Update(service.WithProjectID(ctx, workspace.ID), input)
	require.NoError(t, err)

	updated, err := client.Group.Get(ctx, group.ID)
	require.NoError(t, err)
	require.Equal(t, home.ID, updated.ProjectID)
	require.Equal(t, "edited-profile-bound-group", updated.Name)
}

func TestUserSubscriptionCreateBindsRestrictedActiveProjectProfile(t *testing.T) {
	client := newProjectProfileResourceScopeSQLite(t)
	ctx := context.Background()

	project, err := client.Project.Create().
		SetName("Subscription Project").
		SetSlug("subscription-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	profile, err := client.ProjectProfile.Create().
		SetProjectID(project.ID).
		SetName("Restricted").
		SetMode(service.ProjectProfileModeRestricted).
		SetIsActive(true).
		Save(ctx)
	require.NoError(t, err)
	user, err := client.User.Create().
		SetEmail("subscription-bound@test.com").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetProjectID(project.ID).
		SetName("subscription-group").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	repo := NewUserSubscriptionRepository(client)
	sub := &service.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    service.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	err = repo.Create(service.WithProjectID(ctx, project.ID), sub)
	require.NoError(t, err)
	require.NotZero(t, sub.ID)

	exists, err := client.ProjectProfileBinding.Query().
		Where(
			projectprofilebinding.ProjectProfileIDEQ(profile.ID),
			projectprofilebinding.ResourceTypeEQ(service.ProjectResourceTypeSubscription),
			projectprofilebinding.ResourceIDEQ(sub.ID),
		).
		Exist(ctx)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestUserSubscriptionCreateSkipsBindingForUnrestrictedActiveProjectProfile(t *testing.T) {
	client := newProjectProfileResourceScopeSQLite(t)
	ctx := context.Background()

	project, err := client.Project.Create().
		SetName("Unrestricted Subscription Project").
		SetSlug("unrestricted-subscription-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectProfile.Create().
		SetProjectID(project.ID).
		SetName("Unrestricted").
		SetMode(service.ProjectProfileModeUnrestricted).
		SetIsActive(true).
		Save(ctx)
	require.NoError(t, err)
	user, err := client.User.Create().
		SetEmail("subscription-unrestricted@test.com").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetProjectID(project.ID).
		SetName("unrestricted-subscription-group").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	repo := NewUserSubscriptionRepository(client)
	sub := &service.UserSubscription{
		UserID:    user.ID,
		GroupID:   group.ID,
		Status:    service.SubscriptionStatusActive,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	err = repo.Create(service.WithProjectID(ctx, project.ID), sub)
	require.NoError(t, err)
	require.NotZero(t, sub.ID)

	count, err := client.ProjectProfileBinding.Query().
		Where(
			projectprofilebinding.ResourceTypeEQ(service.ProjectResourceTypeSubscription),
			projectprofilebinding.ResourceIDEQ(sub.ID),
		).
		Count(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
}
