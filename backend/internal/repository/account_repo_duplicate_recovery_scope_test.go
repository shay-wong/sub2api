package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/ent/projectprofile"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestFindDuplicateByOperationIDSurvivesProfileSwitchWithoutCrossingProject(t *testing.T) {
	client := newProjectProfileResourceScopeSQLite(t)
	ctx := context.Background()
	project, err := client.Project.Create().
		SetName("Duplicate Recovery Project").
		SetSlug("duplicate-recovery-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	foreignProject, err := client.Project.Create().
		SetName("Foreign Duplicate Recovery Project").
		SetSlug("foreign-duplicate-recovery-project").
		SetProfiles(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)

	const operationID = "duplicate-operation-profile-switch"
	duplicate, err := client.Account.Create().
		SetProjectID(project.ID).
		SetName("committed-copy").
		SetPlatform(service.PlatformAnthropic).
		SetType(service.AccountTypeAPIKey).
		SetStatus(service.StatusActive).
		SetExtra(map[string]any{"duplicate_operation_id": operationID}).
		Save(ctx)
	require.NoError(t, err)
	foreignDuplicate, err := client.Account.Create().
		SetProjectID(foreignProject.ID).
		SetName("foreign-copy").
		SetPlatform(service.PlatformAnthropic).
		SetType(service.AccountTypeAPIKey).
		SetStatus(service.StatusActive).
		SetExtra(map[string]any{"duplicate_operation_id": operationID}).
		Save(ctx)
	require.NoError(t, err)

	oldProfile, err := client.ProjectProfile.Create().
		SetProjectID(project.ID).
		SetName("Old Restricted Profile").
		SetMode(service.ProjectProfileModeRestricted).
		SetIsActive(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectProfileBinding.Create().
		SetProjectProfileID(oldProfile.ID).
		SetResourceType(service.ProjectResourceTypeAccount).
		SetResourceID(duplicate.ID).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ProjectProfile.UpdateOneID(oldProfile.ID).SetIsActive(false).Save(ctx)
	require.NoError(t, err)
	_, err = client.ProjectProfile.Create().
		SetProjectID(project.ID).
		SetName("New Restricted Profile").
		SetMode(service.ProjectProfileModeRestricted).
		SetIsActive(true).
		Save(ctx)
	require.NoError(t, err)

	repo := newAccountRepositoryWithSQL(client, nil, nil)
	projectCtx := service.WithProjectID(ctx, project.ID)
	visible, err := repo.FindByExtraField(projectCtx, "duplicate_operation_id", operationID)
	require.NoError(t, err)
	require.Empty(t, visible, "the committed copy is intentionally hidden by the new active profile")

	recovered, err := repo.FindDuplicateByOperationID(projectCtx, operationID)
	require.NoError(t, err)
	require.NotNil(t, recovered)
	require.Equal(t, duplicate.ID, recovered.ID)

	foreignRecovered, err := repo.FindDuplicateByOperationID(service.WithProjectID(ctx, foreignProject.ID), operationID)
	require.NoError(t, err)
	require.NotNil(t, foreignRecovered)
	require.Equal(t, foreignDuplicate.ID, foreignRecovered.ID)

	activeProfiles, err := client.ProjectProfile.Query().
		Where(projectprofile.ProjectIDEQ(project.ID), projectprofile.IsActiveEQ(true)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, activeProfiles)
}
