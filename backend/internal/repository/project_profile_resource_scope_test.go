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

func TestAccountRepositoryListShadowsByParentBypassesActiveProjectProfileScope(t *testing.T) {
	client := newProjectProfileResourceScopeSQLite(t)
	ctx := context.Background()

	project, err := client.Project.Create().
		SetName("Shadow Scope Project").
		SetSlug("shadow-scope-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	parent, err := client.Account.Create().
		SetProjectID(project.ID).
		SetName("shadow-parent").
		SetPlatform(service.PlatformOpenAI).
		SetType(service.AccountTypeOAuth).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	hiddenShadow, err := client.Account.Create().
		SetProjectID(project.ID).
		SetName("hidden-shadow").
		SetPlatform(service.PlatformOpenAI).
		SetType(service.AccountTypeOAuth).
		SetStatus(service.StatusActive).
		SetParentAccountID(parent.ID).
		SetQuotaDimension("spark").
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
		SetResourceID(parent.ID).
		Save(ctx)
	require.NoError(t, err)

	repo := newAccountRepositoryWithSQL(client, nil, nil)
	projectCtx := service.WithProjectID(ctx, project.ID)
	shadows, err := repo.ListShadowsByParent(projectCtx, parent.ID)
	require.NoError(t, err)
	require.Len(t, shadows, 1, "internal shadow invariants must still see unbound shadows hidden from the active profile")
	require.Equal(t, hiddenShadow.ID, shadows[0].ID)

	_, err = client.ProjectProfile.UpdateOneID(profile.ID).
		SetMode(service.ProjectProfileModeUnrestricted).
		Save(ctx)
	require.NoError(t, err)
	shadows, err = repo.ListShadowsByParent(projectCtx, parent.ID)
	require.NoError(t, err)
	require.Len(t, shadows, 1)
	require.Equal(t, hiddenShadow.ID, shadows[0].ID)
}

func TestAccountRepositoryCreateShadowPreservesParentProjectInScopedContext(t *testing.T) {
	client := newProjectProfileResourceScopeSQLite(t)
	ctx := context.Background()

	home, err := client.Project.Create().
		SetName("Shadow Home Project").
		SetSlug("shadow-home-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	workspace, err := client.Project.Create().
		SetName("Shadow Workspace Project").
		SetSlug("shadow-workspace-project").
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
	parent, err := client.Account.Create().
		SetProjectID(home.ID).
		SetName("foreign-parent").
		SetPlatform(service.PlatformOpenAI).
		SetType(service.AccountTypeOAuth).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectProfileBinding.Create().
		SetProjectProfileID(profile.ID).
		SetResourceType(service.ProjectResourceTypeAccount).
		SetResourceID(parent.ID).
		Save(ctx)
	require.NoError(t, err)

	repo := newAccountRepositoryWithSQL(client, nil, nil)
	parentID := parent.ID
	shadow := &service.Account{
		ProjectID:       parent.ProjectID,
		Name:            "foreign-parent-spark",
		Platform:        service.PlatformOpenAI,
		Type:            service.AccountTypeOAuth,
		Status:          service.StatusActive,
		ParentAccountID: &parentID,
		QuotaDimension:  service.QuotaDimensionSpark,
		Credentials:     map[string]any{"model_mapping": map[string]any{}},
	}
	err = repo.Create(service.WithProjectID(ctx, workspace.ID), shadow)
	require.NoError(t, err)

	persisted, err := client.Account.Get(ctx, shadow.ID)
	require.NoError(t, err)
	require.Equal(t, home.ID, persisted.ProjectID, "shadow created from a visible foreign parent must stay in the parent's home project")

	normal := &service.Account{
		ProjectID:   home.ID,
		Name:        "normal-account",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Credentials: map[string]any{},
	}
	err = repo.Create(service.WithProjectID(ctx, workspace.ID), normal)
	require.NoError(t, err)
	persistedNormal, err := client.Account.Get(ctx, normal.ID)
	require.NoError(t, err)
	require.Equal(t, workspace.ID, persistedNormal.ProjectID, "ordinary scoped creates still belong to the current project")
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

func TestGroupRepositoryListActiveIDsUsesActiveProjectProfileScope(t *testing.T) {
	client := newProjectProfileResourceScopeSQLite(t)
	ctx := context.Background()

	home, err := client.Project.Create().
		SetName("Group ID Home").
		SetSlug("group-id-home").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	workspace, err := client.Project.Create().
		SetName("Group ID Workspace").
		SetSlug("group-id-workspace").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	bound, err := client.Group.Create().
		SetProjectID(home.ID).
		SetName("bound-active-id-group").
		SetStatus(service.StatusActive).
		SetPlatform(service.PlatformAnthropic).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		Save(ctx)
	require.NoError(t, err)
	unbound, err := client.Group.Create().
		SetProjectID(home.ID).
		SetName("unbound-active-id-group").
		SetStatus(service.StatusActive).
		SetPlatform(service.PlatformAnthropic).
		SetSubscriptionType(service.SubscriptionTypeStandard).
		Save(ctx)
	require.NoError(t, err)
	inactive, err := client.Group.Create().
		SetProjectID(home.ID).
		SetName("bound-inactive-id-group").
		SetStatus(service.StatusDisabled).
		SetPlatform(service.PlatformAnthropic).
		SetSubscriptionType(service.SubscriptionTypeStandard).
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
		SetResourceID(bound.ID).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectProfileBinding.Create().
		SetProjectProfileID(profile.ID).
		SetResourceType(service.ProjectResourceTypeGroup).
		SetResourceID(inactive.ID).
		Save(ctx)
	require.NoError(t, err)

	repo := newGroupRepositoryWithSQL(client, nil)
	ids, err := repo.ListActiveIDs(service.WithProjectID(ctx, workspace.ID))
	require.NoError(t, err)
	require.Equal(t, []int64{bound.ID}, ids)
	require.NotContains(t, ids, unbound.ID)
	require.NotContains(t, ids, inactive.ID)
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

func TestUserSubscriptionResetDailyUsageUsesActiveProjectProfileScope(t *testing.T) {
	client := newProjectProfileResourceScopeSQLite(t)
	ctx := context.Background()

	project, err := client.Project.Create().
		SetName("Subscription Reset Scope Project").
		SetSlug("subscription-reset-scope-project").
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
		SetEmail("subscription-reset-scope@test.com").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetProjectID(project.ID).
		SetName("subscription-reset-scope-group").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)

	oldWindowStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newWindowStart := oldWindowStart.Add(24 * time.Hour)
	createSubscription := func(emailSuffix string, usage float64) *dbent.UserSubscription {
		t.Helper()
		sub, err := client.UserSubscription.Create().
			SetUserID(user.ID).
			SetGroupID(group.ID).
			SetStartsAt(time.Now().Add(-1 * time.Hour)).
			SetExpiresAt(time.Now().Add(24 * time.Hour)).
			SetStatus(service.SubscriptionStatusActive).
			SetAssignedAt(time.Now()).
			SetNotes(emailSuffix).
			SetDailyWindowStart(oldWindowStart).
			SetDailyUsageUsd(usage).
			Save(ctx)
		require.NoError(t, err)
		return sub
	}
	bound := createSubscription("bound", 10)
	hidden := createSubscription("hidden", 7)
	_, err = client.ProjectProfileBinding.Create().
		SetProjectProfileID(profile.ID).
		SetResourceType(service.ProjectResourceTypeSubscription).
		SetResourceID(bound.ID).
		Save(ctx)
	require.NoError(t, err)

	repo := NewUserSubscriptionRepository(client)
	projectCtx := service.WithProjectID(ctx, project.ID)
	require.NoError(t, repo.ResetDailyUsage(projectCtx, bound.ID, &oldWindowStart, newWindowStart))
	_, err = client.UserSubscription.UpdateOneID(bound.ID).SetDailyUsageUsd(3).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, repo.ResetDailyUsage(projectCtx, bound.ID, &oldWindowStart, newWindowStart))

	got, err := client.UserSubscription.Get(ctx, bound.ID)
	require.NoError(t, err)
	require.InDelta(t, 3, got.DailyUsageUsd, 1e-6)
	require.WithinDuration(t, newWindowStart, *got.DailyWindowStart, time.Microsecond)

	err = repo.ResetDailyUsage(projectCtx, hidden.ID, &oldWindowStart, newWindowStart)
	require.ErrorIs(t, err, service.ErrSubscriptionNotFound)
	got, err = client.UserSubscription.Get(ctx, hidden.ID)
	require.NoError(t, err)
	require.InDelta(t, 7, got.DailyUsageUsd, 1e-6)
	require.WithinDuration(t, oldWindowStart, *got.DailyWindowStart, time.Microsecond)
}

func TestUserSubscriptionListRelationAttachUsesActiveProjectProfileScope(t *testing.T) {
	client := newProjectProfileResourceScopeSQLite(t)
	ctx := context.Background()

	project, err := client.Project.Create().
		SetName("Subscription Relation Scope Project").
		SetSlug("subscription-relation-scope-project").
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
		SetEmail("subscription-relation-user@test.com").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	assignedBy, err := client.User.Create().
		SetEmail("subscription-relation-assigned@test.com").
		SetPasswordHash("hash").
		SetRole(service.RoleAdmin).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetProjectID(project.ID).
		SetName("subscription-relation-group").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectMember.Create().
		SetProjectID(project.ID).
		SetUserID(user.ID).
		SetRole(service.ProjectRoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	sub, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetAssignedBy(assignedBy.ID).
		SetStartsAt(time.Now().Add(-1 * time.Hour)).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetStatus(service.SubscriptionStatusActive).
		SetAssignedAt(time.Now()).
		SetNotes("").
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectProfileBinding.Create().
		SetProjectProfileID(profile.ID).
		SetResourceType(service.ProjectResourceTypeSubscription).
		SetResourceID(sub.ID).
		Save(ctx)
	require.NoError(t, err)

	repo := NewUserSubscriptionRepository(client)
	projectCtx := service.WithProjectID(ctx, project.ID)
	subs, _, err := repo.List(projectCtx, pagination.PaginationParams{Page: 1, PageSize: 10}, nil, nil, "", "", "", "")
	require.NoError(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, sub.ID, subs[0].ID)
	require.NotNil(t, subs[0].User, "project member user should still be attached")
	require.Equal(t, user.ID, subs[0].User.ID)
	require.Nil(t, subs[0].Group, "unbound group must not be attached via naked ID lookup")
	require.Nil(t, subs[0].AssignedByUser, "non-member assigned-by user must not be attached via naked ID lookup")

	_, err = client.ProjectMember.Create().
		SetProjectID(project.ID).
		SetUserID(assignedBy.ID).
		SetRole(service.ProjectRoleAdmin).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectProfileBinding.Create().
		SetProjectProfileID(profile.ID).
		SetResourceType(service.ProjectResourceTypeGroup).
		SetResourceID(group.ID).
		Save(ctx)
	require.NoError(t, err)
	subs, _, err = repo.List(projectCtx, pagination.PaginationParams{Page: 1, PageSize: 10}, nil, nil, "", "", "", "")
	require.NoError(t, err)
	require.Len(t, subs, 1)
	require.NotNil(t, subs[0].Group)
	require.Equal(t, group.ID, subs[0].Group.ID)
	require.NotNil(t, subs[0].AssignedByUser)
	require.Equal(t, assignedBy.ID, subs[0].AssignedByUser.ID)
}

func TestUserSubscriptionListRelationAttachIncludesSoftDeletedHistory(t *testing.T) {
	client := newProjectProfileResourceScopeSQLite(t)
	ctx := context.Background()

	project, err := client.Project.Create().
		SetName("Subscription Relation History Project").
		SetSlug("subscription-relation-history-project").
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
		SetEmail("subscription-history-user@test.com").
		SetPasswordHash("hash").
		SetRole(service.RoleUser).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	assignedBy, err := client.User.Create().
		SetEmail("subscription-history-assigned@test.com").
		SetPasswordHash("hash").
		SetRole(service.RoleAdmin).
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	group, err := client.Group.Create().
		SetProjectID(project.ID).
		SetName("subscription-history-group").
		SetStatus(service.StatusActive).
		Save(ctx)
	require.NoError(t, err)
	for _, member := range []struct {
		userID int64
		role   string
	}{
		{userID: user.ID, role: service.ProjectRoleUser},
		{userID: assignedBy.ID, role: service.ProjectRoleAdmin},
	} {
		_, err = client.ProjectMember.Create().
			SetProjectID(project.ID).
			SetUserID(member.userID).
			SetRole(member.role).
			SetStatus(service.StatusActive).
			Save(ctx)
		require.NoError(t, err)
	}
	_, err = client.ProjectProfileBinding.Create().
		SetProjectProfileID(profile.ID).
		SetResourceType(service.ProjectResourceTypeGroup).
		SetResourceID(group.ID).
		Save(ctx)
	require.NoError(t, err)
	sub, err := client.UserSubscription.Create().
		SetUserID(user.ID).
		SetGroupID(group.ID).
		SetAssignedBy(assignedBy.ID).
		SetStartsAt(time.Now().Add(-1 * time.Hour)).
		SetExpiresAt(time.Now().Add(24 * time.Hour)).
		SetStatus(service.SubscriptionStatusActive).
		SetAssignedAt(time.Now()).
		SetNotes("").
		Save(ctx)
	require.NoError(t, err)
	require.NoError(t, client.UserSubscription.DeleteOneID(sub.ID).Exec(ctx))
	require.NoError(t, client.User.DeleteOneID(user.ID).Exec(ctx))
	require.NoError(t, client.User.DeleteOneID(assignedBy.ID).Exec(ctx))
	require.NoError(t, client.Group.DeleteOneID(group.ID).Exec(ctx))

	repo := NewUserSubscriptionRepository(client)
	subs, _, err := repo.List(service.WithProjectID(ctx, project.ID), pagination.PaginationParams{Page: 1, PageSize: 10}, nil, nil, service.SubscriptionStatusRevoked, "", "", "")
	require.NoError(t, err)
	require.Len(t, subs, 1)
	require.Equal(t, sub.ID, subs[0].ID)
	require.Equal(t, service.SubscriptionStatusRevoked, subs[0].Status)
	require.NotNil(t, subs[0].User)
	require.Equal(t, user.ID, subs[0].User.ID)
	require.NotNil(t, subs[0].User.DeletedAt)
	require.NotNil(t, subs[0].Group)
	require.Equal(t, group.ID, subs[0].Group.ID)
	require.NotNil(t, subs[0].AssignedByUser)
	require.Equal(t, assignedBy.ID, subs[0].AssignedByUser.ID)
	require.NotNil(t, subs[0].AssignedByUser.DeletedAt)
}
