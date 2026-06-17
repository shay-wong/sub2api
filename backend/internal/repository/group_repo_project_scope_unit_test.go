package repository

import (
	"context"
	"database/sql"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	dbgroup "github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func newGroupRepoSQLite(t *testing.T) (*groupRepository, *dbent.Client) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:group_repo_project_scope?mode=memory&cache=shared")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() { _ = client.Close() })

	return newGroupRepositoryWithSQL(client, db), client
}

func mustCreateGroupRepoProject(t *testing.T, ctx context.Context, client *dbent.Client, name, slug string) int64 {
	t.Helper()
	project, err := client.Project.Create().
		SetName(name).
		SetSlug(slug).
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	return project.ID
}

func mustCreateGroupRepoGroup(t *testing.T, ctx context.Context, repo *groupRepository, projectID int64, name string) *service.Group {
	t.Helper()
	g := &service.Group{
		ProjectID:        projectID,
		Name:             name,
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1,
		Status:           service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	require.NoError(t, repo.Create(ctx, g))
	return g
}

func TestGroupRepositoryUpdateRequiresContextProject(t *testing.T) {
	repo, client := newGroupRepoSQLite(t)
	ctx := context.Background()
	projectA := mustCreateGroupRepoProject(t, ctx, client, "Project A", "project-a")
	projectB := mustCreateGroupRepoProject(t, ctx, client, "Project B", "project-b")
	groupB := mustCreateGroupRepoGroup(t, ctx, repo, projectB, "group-b")

	groupB.Name = "stolen"
	err := repo.Update(service.WithProjectID(ctx, projectA), groupB)

	require.ErrorIs(t, err, service.ErrGroupNotFound)
	got, err := client.Group.Query().Where(dbgroup.IDEQ(groupB.ID)).Only(ctx)
	require.NoError(t, err)
	require.Equal(t, projectB, got.ProjectID)
	require.Equal(t, "group-b", got.Name)
}

func TestGroupRepositoryDeleteRequiresContextProject(t *testing.T) {
	repo, client := newGroupRepoSQLite(t)
	ctx := context.Background()
	projectA := mustCreateGroupRepoProject(t, ctx, client, "Project A", "project-a")
	projectB := mustCreateGroupRepoProject(t, ctx, client, "Project B", "project-b")
	groupB := mustCreateGroupRepoGroup(t, ctx, repo, projectB, "group-b")

	err := repo.Delete(service.WithProjectID(ctx, projectA), groupB.ID)

	require.ErrorIs(t, err, service.ErrGroupNotFound)
	exists, err := client.Group.Query().Where(dbgroup.IDEQ(groupB.ID)).Exist(ctx)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestGroupRepositoryExistsByIDsRequiresContextProject(t *testing.T) {
	repo, client := newGroupRepoSQLite(t)
	ctx := context.Background()
	projectA := mustCreateGroupRepoProject(t, ctx, client, "Project A", "project-a")
	projectB := mustCreateGroupRepoProject(t, ctx, client, "Project B", "project-b")
	groupA := mustCreateGroupRepoGroup(t, ctx, repo, projectA, "group-a")
	groupB := mustCreateGroupRepoGroup(t, ctx, repo, projectB, "group-b")

	exists, err := repo.ExistsByIDs(service.WithProjectID(ctx, projectA), []int64{groupA.ID, groupB.ID})

	require.NoError(t, err)
	require.True(t, exists[groupA.ID])
	require.False(t, exists[groupB.ID])
}
