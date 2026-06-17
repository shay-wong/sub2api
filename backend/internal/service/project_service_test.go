package service

import (
	"context"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestProjectServiceCreateProjectNormalizesInputAndAssignsOwner(t *testing.T) {
	repo := &projectServiceRepoStub{}
	svc := NewProjectService(repo)
	description := "  demo project  "

	project, err := svc.CreateProject(context.Background(), ProjectCreateInput{
		Name:        "  Demo  ",
		Slug:        " Demo_Project ",
		Description: &description,
		OwnerUserID: 42,
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), project.ID)
	require.Equal(t, "Demo", repo.created.Name)
	require.Equal(t, "demo-project", repo.created.Slug)
	require.NotNil(t, repo.created.Description)
	require.Equal(t, "demo project", *repo.created.Description)
	require.Equal(t, int64(42), repo.created.OwnerUserID)
}

func TestProjectServiceSetProjectMemberRejectsLegacyOperatorRole(t *testing.T) {
	repo := &projectServiceRepoStub{projectExists: true}
	svc := NewProjectService(repo)

	member, err := svc.SetProjectMember(context.Background(), 10, ProjectMemberInput{
		UserID: 20,
		Role:   RoleOperator,
	})

	require.Nil(t, member)
	require.ErrorIs(t, err, ErrProjectInvalidRole)
	require.False(t, repo.setMemberCalled)
}

func TestProjectServiceSetProjectMemberAllowsProjectAdmin(t *testing.T) {
	repo := &projectServiceRepoStub{projectExists: true}
	svc := NewProjectService(repo)

	member, err := svc.SetProjectMember(context.Background(), 10, ProjectMemberInput{
		UserID:  20,
		Role:    ProjectRoleAdmin,
		IsOwner: true,
	})

	require.NoError(t, err)
	require.NotNil(t, member)
	require.True(t, repo.setMemberCalled)
	require.Equal(t, ProjectRoleAdmin, repo.memberInput.Role)
	require.True(t, repo.memberInput.IsOwner)
}

func TestProjectServiceMoveProjectResourcesNormalizesInputAndInvalidatesAuthCache(t *testing.T) {
	repo := &projectServiceRepoStub{projectExists: true}
	invalidator := &projectAuthCacheInvalidatorStub{}
	svc := NewProjectService(repo, invalidator)

	result, err := svc.MoveProjectResources(context.Background(), 10, ProjectResourceMoveInput{
		AccountIDs:       []int64{3, 0, 1, 3},
		APIKeyIDs:        []int64{8, 8, 7},
		GroupIDs:         []int64{6, -1, 5, 6},
		MoveUsageHistory: true,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, []int64{1, 3}, repo.moveInput.AccountIDs)
	require.Equal(t, []int64{7, 8}, repo.moveInput.APIKeyIDs)
	require.Equal(t, []int64{5, 6}, repo.moveInput.GroupIDs)
	require.True(t, repo.moveInput.MoveUsageHistory)
	require.Equal(t, []string{"k-a", "k-b"}, invalidator.keys)
	require.Equal(t, []int64{21, 22}, invalidator.userIDs)
	require.Equal(t, []int64{5, 6}, invalidator.groupIDs)
}

func TestProjectServiceMoveProjectResourcesRejectsEmptyResourceSelection(t *testing.T) {
	repo := &projectServiceRepoStub{projectExists: true}
	svc := NewProjectService(repo)

	result, err := svc.MoveProjectResources(context.Background(), 10, ProjectResourceMoveInput{
		AccountIDs: []int64{0, -1},
	})

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrProjectInvalidInput)
	require.Equal(t, 400, infraerrors.Code(err))
	require.False(t, repo.moveResourcesCalled)
}

type projectServiceRepoStub struct {
	created             ProjectCreateInput
	projectExists       bool
	setMemberCalled     bool
	memberInput         ProjectMemberInput
	moveResourcesCalled bool
	moveInput           ProjectResourceMoveInput
}

func (r *projectServiceRepoStub) GetDefaultProjectID(context.Context) (int64, error) {
	return 1, nil
}

func (r *projectServiceRepoStub) GetProjectRole(context.Context, int64, int64) (string, bool, error) {
	return ProjectRoleAdmin, true, nil
}

func (r *projectServiceRepoStub) ProjectExists(context.Context, int64) (bool, error) {
	return r.projectExists, nil
}

func (r *projectServiceRepoStub) ListActiveProjects(context.Context) ([]ProjectSummary, error) {
	return []ProjectSummary{}, nil
}

func (r *projectServiceRepoStub) ListUserProjects(context.Context, int64) ([]ProjectSummary, error) {
	return []ProjectSummary{}, nil
}

func (r *projectServiceRepoStub) CreateProject(_ context.Context, input ProjectCreateInput) (*ProjectSummary, error) {
	r.created = input
	return &ProjectSummary{ID: 10, Name: input.Name, Slug: input.Slug, Description: input.Description}, nil
}

func (r *projectServiceRepoStub) UpdateProject(_ context.Context, projectID int64, input ProjectUpdateInput) (*ProjectSummary, error) {
	name := ""
	if input.Name != nil {
		name = *input.Name
	}
	return &ProjectSummary{ID: projectID, Name: name, Slug: "demo"}, nil
}

func (r *projectServiceRepoStub) ListProjectMembers(context.Context, int64) ([]ProjectMember, error) {
	return []ProjectMember{}, nil
}

func (r *projectServiceRepoStub) SetProjectMember(_ context.Context, projectID int64, input ProjectMemberInput) (*ProjectMember, error) {
	r.setMemberCalled = true
	r.memberInput = input
	return &ProjectMember{ProjectID: projectID, UserID: input.UserID, Role: input.Role, IsOwner: input.IsOwner}, nil
}

func (r *projectServiceRepoStub) RemoveProjectMember(context.Context, int64, int64) error {
	return nil
}

func (r *projectServiceRepoStub) MoveProjectResources(_ context.Context, projectID int64, input ProjectResourceMoveInput) (*ProjectResourceMoveResult, error) {
	r.moveResourcesCalled = true
	r.moveInput = input
	return &ProjectResourceMoveResult{
		AccountsMoved:       int64(len(input.AccountIDs)),
		APIKeysMoved:        int64(len(input.APIKeyIDs)),
		GroupsMoved:         int64(len(input.GroupIDs)),
		InvalidatedAPIKeys:  []string{"k-a", "k-b"},
		InvalidatedUserIDs:  []int64{21, 22},
		InvalidatedGroupIDs: input.GroupIDs,
	}, nil
}

type projectAuthCacheInvalidatorStub struct {
	keys     []string
	userIDs  []int64
	groupIDs []int64
}

func (s *projectAuthCacheInvalidatorStub) InvalidateAuthCacheByKey(_ context.Context, key string) {
	s.keys = append(s.keys, key)
}

func (s *projectAuthCacheInvalidatorStub) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	s.userIDs = append(s.userIDs, userID)
}

func (s *projectAuthCacheInvalidatorStub) InvalidateAuthCacheByGroupID(_ context.Context, groupID int64) {
	s.groupIDs = append(s.groupIDs, groupID)
}
