package service

import (
	"context"
	"testing"

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
	require.Equal(t, ProjectProfileModeRestricted, repo.created.ProfileMode)
}

func TestProjectServiceCreateProjectKeepsDefaultProfileBindingsWhenScopeIsUnrestricted(t *testing.T) {
	repo := &projectServiceRepoStub{}
	svc := NewProjectService(repo)

	project, err := svc.CreateProject(context.Background(), ProjectCreateInput{
		Name:        "Demo",
		Slug:        "demo",
		OwnerUserID: 42,
		ProfileMode: " unrestricted ",
		Bindings: ProjectProfileBindingInput{
			GroupIDs:        []int64{7, 6, 7},
			AccountIDs:      []int64{5},
			SubscriptionIDs: []int64{9},
		},
	})

	require.NoError(t, err)
	require.Equal(t, int64(10), project.ID)
	require.Equal(t, ProjectProfileModeUnrestricted, repo.created.ProfileMode)
	require.True(t, repo.validateBindingResourcesCalled)
	require.Equal(t, []int64{6, 7}, repo.created.Bindings.GroupIDs)
	require.Equal(t, []int64{5}, repo.created.Bindings.AccountIDs)
	require.Equal(t, []int64{9}, repo.created.Bindings.SubscriptionIDs)
}

func TestProjectServiceCreateProjectValidatesRestrictedInitialBindings(t *testing.T) {
	repo := &projectServiceRepoStub{}
	svc := NewProjectService(repo)

	project, err := svc.CreateProject(context.Background(), ProjectCreateInput{
		Name:        "Demo",
		Slug:        "demo",
		OwnerUserID: 42,
		ProfileMode: ProjectProfileModeRestricted,
		Bindings: ProjectProfileBindingInput{
			GroupIDs:        []int64{7, -1, 6},
			AccountIDs:      []int64{5, 5},
			SubscriptionIDs: []int64{9, 8, 9},
		},
	})

	require.NoError(t, err)
	require.NotNil(t, project)
	require.True(t, repo.validateBindingResourcesCalled)
	require.Equal(t, []int64{6, 7}, repo.bindingInput.GroupIDs)
	require.Equal(t, []int64{5}, repo.bindingInput.AccountIDs)
	require.Equal(t, []int64{8, 9}, repo.bindingInput.SubscriptionIDs)
	require.Equal(t, repo.bindingInput, repo.created.Bindings)
}

func TestProjectServiceCreateProjectRejectsInvalidInitialBindingsBeforeCreate(t *testing.T) {
	repo := &projectServiceRepoStub{validateBindingResourcesErr: ErrProjectInvalidInput}
	svc := NewProjectService(repo)

	project, err := svc.CreateProject(context.Background(), ProjectCreateInput{
		Name:        "Demo",
		Slug:        "demo",
		OwnerUserID: 42,
		Bindings: ProjectProfileBindingInput{
			GroupIDs: []int64{99},
		},
	})

	require.Nil(t, project)
	require.ErrorIs(t, err, ErrProjectInvalidInput)
	require.True(t, repo.validateBindingResourcesCalled)
	require.Empty(t, repo.created.Name)
}

func TestProjectServiceCreateProjectRejectsInvalidProfileMode(t *testing.T) {
	repo := &projectServiceRepoStub{}
	svc := NewProjectService(repo)

	project, err := svc.CreateProject(context.Background(), ProjectCreateInput{
		Name:        "Demo",
		Slug:        "demo",
		OwnerUserID: 42,
		ProfileMode: "legacy",
	})

	require.Nil(t, project)
	require.ErrorIs(t, err, ErrProjectInvalidInput)
	require.Empty(t, repo.created.Name)
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

func TestProjectServiceResolveAdminProjectDefaultsToFirstAdminMembership(t *testing.T) {
	repo := &projectServiceRepoStub{
		projectRoles: map[int64]string{
			10: ProjectRoleUser,
			20: ProjectRoleAdmin,
			30: ProjectRoleAdmin,
		},
		userProjects: []ProjectSummary{
			{ID: 10, Role: ProjectRoleUser},
			{ID: 20, Role: ProjectRoleAdmin},
			{ID: 30, Role: ProjectRoleAdmin},
		},
	}
	svc := NewProjectService(repo)

	projectID, role, permissions, err := svc.ResolveAdminProject(context.Background(), &User{ID: 7, Role: RoleUser}, 0)

	require.NoError(t, err)
	require.Equal(t, int64(20), projectID)
	require.Equal(t, RoleAdmin, role)
	require.Empty(t, permissions)
	require.False(t, repo.defaultProjectCalled)
	require.Equal(t, []int64{7}, repo.listUserProjectsCalls)
}

func TestProjectServiceResolveAdminProjectReturnsProjectPermissions(t *testing.T) {
	repo := &projectServiceRepoStub{
		userProjects: []ProjectSummary{
			{
				ID:          20,
				Role:        ProjectRoleAdmin,
				Permissions: []string{AdminPermissionDashboardRead, AdminPermissionUsersManage},
			},
		},
	}
	svc := NewProjectService(repo)

	projectID, role, permissions, err := svc.ResolveAdminProject(context.Background(), &User{ID: 7, Role: RoleUser}, 20)

	require.NoError(t, err)
	require.Equal(t, int64(20), projectID)
	require.Equal(t, RoleAdmin, role)
	require.ElementsMatch(t, []string{AdminPermissionDashboardRead, AdminPermissionUsersManage}, permissions)
}

func TestProjectServiceResolveAdminProjectRejectsUsersWithoutAdminMembership(t *testing.T) {
	repo := &projectServiceRepoStub{
		userProjects: []ProjectSummary{{ID: 10, Role: ProjectRoleUser}},
		projectRoles: map[int64]string{
			10: ProjectRoleUser,
		},
	}
	svc := NewProjectService(repo)

	projectID, role, permissions, err := svc.ResolveAdminProject(context.Background(), &User{ID: 7, Role: RoleUser}, 0)

	require.Zero(t, projectID)
	require.Empty(t, role)
	require.Nil(t, permissions)
	require.ErrorIs(t, err, ErrProjectAccessForbidden)
	require.False(t, repo.defaultProjectCalled)
}

func TestProjectServiceResolveAdminProjectRejectsLegacyOperatorRole(t *testing.T) {
	repo := &projectServiceRepoStub{
		projectExists: true,
		projectRoles: map[int64]string{
			10: ProjectRoleAdmin,
		},
	}
	svc := NewProjectService(repo)

	projectID, role, permissions, err := svc.ResolveAdminProject(context.Background(), &User{ID: 7, Role: RoleOperator}, 10)

	require.Zero(t, projectID)
	require.Empty(t, role)
	require.Nil(t, permissions)
	require.ErrorIs(t, err, ErrLegacyOperatorRoleDisabled)
	require.False(t, repo.defaultProjectCalled)
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
	require.ElementsMatch(t, DefaultProjectAdminPermissions(), repo.memberInput.Permissions)
}

func TestProjectServiceSetProjectMemberPromotesOwnerToAdmin(t *testing.T) {
	repo := &projectServiceRepoStub{projectExists: true}
	svc := NewProjectService(repo)

	member, err := svc.SetProjectMember(context.Background(), 10, ProjectMemberInput{
		UserID:  20,
		Role:    ProjectRoleUser,
		IsOwner: true,
	})

	require.NoError(t, err)
	require.NotNil(t, member)
	require.True(t, repo.setMemberCalled)
	require.Equal(t, ProjectRoleAdmin, repo.memberInput.Role)
	require.True(t, repo.memberInput.IsOwner)
	require.ElementsMatch(t, DefaultProjectAdminPermissions(), repo.memberInput.Permissions)
	require.NotNil(t, repo.memberInput.Status)
	require.Equal(t, StatusActive, *repo.memberInput.Status)
}

func TestProjectServiceSetProjectMemberNormalizesAdminPermissions(t *testing.T) {
	repo := &projectServiceRepoStub{projectExists: true}
	svc := NewProjectService(repo)

	member, err := svc.SetProjectMember(context.Background(), 10, ProjectMemberInput{
		UserID: 20,
		Role:   ProjectRoleAdmin,
		Permissions: []string{
			AdminPermissionUsersManage,
			"unknown",
			AdminPermissionDashboardRead,
			AdminPermissionUsersManage,
		},
	})

	require.NoError(t, err)
	require.NotNil(t, member)
	require.Equal(t, []string{AdminPermissionDashboardRead, AdminPermissionUsersManage}, repo.memberInput.Permissions)
}

func TestProjectServiceSetProjectMemberStoresExplicitEmptyAdminPermissions(t *testing.T) {
	repo := &projectServiceRepoStub{projectExists: true}
	svc := NewProjectService(repo)

	member, err := svc.SetProjectMember(context.Background(), 10, ProjectMemberInput{
		UserID:      20,
		Role:        ProjectRoleAdmin,
		Permissions: []string{},
	})

	require.NoError(t, err)
	require.NotNil(t, member)
	require.Equal(t, []string{"__none__"}, repo.memberInput.Permissions)
}

func TestProjectServiceCreateProjectProfileDefaultsRestrictedMode(t *testing.T) {
	repo := &projectServiceRepoStub{projectExists: true}
	svc := NewProjectService(repo)
	name := "  Production  "

	profile, err := svc.CreateProjectProfile(context.Background(), 10, ProjectProfileInput{Name: &name})

	require.NoError(t, err)
	require.NotNil(t, profile)
	require.True(t, repo.createProfileCalled)
	require.Equal(t, "Production", *repo.profileInput.Name)
}

func TestProjectServiceUpdateProjectProfileRejectsEmptyPatch(t *testing.T) {
	repo := &projectServiceRepoStub{projectExists: true}
	svc := NewProjectService(repo)

	profile, err := svc.UpdateProjectProfile(context.Background(), 10, 20, ProjectProfileInput{})

	require.ErrorIs(t, err, ErrProjectInvalidInput)
	require.Nil(t, profile)
	require.False(t, repo.updateProfileCalled)
}

func TestProjectServiceSetProjectProfileBindingsNormalizesResourceIDs(t *testing.T) {
	repo := &projectServiceRepoStub{
		projectExists: true,
		profiles:      []ProjectProfile{{ID: 20, ProjectID: 10, Mode: ProjectProfileModeRestricted}},
	}
	svc := NewProjectService(repo)

	bindings, err := svc.SetProjectProfileBindings(context.Background(), 10, 20, ProjectProfileBindingInput{
		GroupIDs:        []int64{7, -1, 6},
		AccountIDs:      []int64{5, 5},
		SubscriptionIDs: []int64{9, 8, 9},
	})

	require.NoError(t, err)
	require.NotNil(t, bindings)
	require.Equal(t, []int64{6, 7}, repo.bindingInput.GroupIDs)
	require.Equal(t, []int64{5}, repo.bindingInput.AccountIDs)
	require.Equal(t, []int64{8, 9}, repo.bindingInput.SubscriptionIDs)
}

func TestProjectServiceGetProjectProfileBindingsReturnsEmptySlices(t *testing.T) {
	repo := &projectServiceRepoStub{
		projectExists: true,
		profiles:      []ProjectProfile{{ID: 20, ProjectID: 10, Mode: ProjectProfileModeRestricted}},
	}
	svc := NewProjectService(repo)

	bindings, err := svc.GetProjectProfileBindings(context.Background(), 10, 20)

	require.NoError(t, err)
	require.NotNil(t, bindings)
	require.Equal(t, int64(20), bindings.ProfileID)
	require.NotNil(t, bindings.GroupIDs)
	require.Empty(t, bindings.GroupIDs)
	require.NotNil(t, bindings.AccountIDs)
	require.Empty(t, bindings.AccountIDs)
	require.NotNil(t, bindings.SubscriptionIDs)
	require.Empty(t, bindings.SubscriptionIDs)
}

func TestProjectServiceSetProjectProfileBindingsReturnsEmptySlices(t *testing.T) {
	repo := &projectServiceRepoStub{
		projectExists: true,
		profiles:      []ProjectProfile{{ID: 20, ProjectID: 10, Mode: ProjectProfileModeRestricted}},
	}
	svc := NewProjectService(repo)

	bindings, err := svc.SetProjectProfileBindings(context.Background(), 10, 20, ProjectProfileBindingInput{})

	require.NoError(t, err)
	require.NotNil(t, bindings)
	require.Equal(t, int64(20), bindings.ProfileID)
	require.NotNil(t, bindings.GroupIDs)
	require.Empty(t, bindings.GroupIDs)
	require.NotNil(t, bindings.AccountIDs)
	require.Empty(t, bindings.AccountIDs)
	require.NotNil(t, bindings.SubscriptionIDs)
	require.Empty(t, bindings.SubscriptionIDs)
}

func TestProjectServiceSetProjectProfileBindingsInvalidatesAuthCache(t *testing.T) {
	repo := &projectServiceRepoStub{
		projectExists: true,
		profiles:      []ProjectProfile{{ID: 20, ProjectID: 10, Mode: ProjectProfileModeRestricted}},
		existingBindings: &ProjectProfileBindings{
			ProfileID: 20,
			GroupIDs:  []int64{7, 10},
		},
	}
	invalidator := &projectAuthCacheInvalidatorStub{}
	svc := NewProjectService(repo, invalidator)

	_, err := svc.SetProjectProfileBindings(context.Background(), 10, 20, ProjectProfileBindingInput{
		GroupIDs: []int64{7, 6},
	})

	require.NoError(t, err)
	require.ElementsMatch(t, []int64{6, 7, 10}, invalidator.groupIDs)
}

func TestProjectServiceActivateProjectProfileInvalidatesPreviousAndNextBindings(t *testing.T) {
	repo := &projectServiceRepoStub{
		projectExists: true,
		profiles: []ProjectProfile{
			{ID: 20, ProjectID: 10, Mode: ProjectProfileModeRestricted, IsActive: true},
			{ID: 30, ProjectID: 10, Mode: ProjectProfileModeRestricted, IsActive: false},
		},
		bindingsByProfileID: map[int64]*ProjectProfileBindings{
			20: {ProfileID: 20, GroupIDs: []int64{6}},
			30: {ProfileID: 30, GroupIDs: []int64{7}},
		},
	}
	invalidator := &projectAuthCacheInvalidatorStub{}
	svc := NewProjectService(repo, invalidator)

	_, err := svc.ActivateProjectProfile(context.Background(), 10, 30)

	require.NoError(t, err)
	require.ElementsMatch(t, []int64{6, 7}, invalidator.groupIDs)
}

func TestProjectServiceActivateProjectUnrestrictedScopeInvalidatesActiveBindings(t *testing.T) {
	repo := &projectServiceRepoStub{
		projectExists: true,
		profiles: []ProjectProfile{
			{ID: 20, ProjectID: 10, Mode: ProjectProfileModeRestricted, IsActive: true},
		},
		bindingsByProfileID: map[int64]*ProjectProfileBindings{
			20: {ProfileID: 20, GroupIDs: []int64{6}},
		},
	}
	invalidator := &projectAuthCacheInvalidatorStub{}
	svc := NewProjectService(repo, invalidator)

	profile, err := svc.ActivateProjectUnrestrictedScope(context.Background(), 10)

	require.NoError(t, err)
	require.NotNil(t, profile)
	require.True(t, repo.activateUnrestrictedCalled)
	require.ElementsMatch(t, []int64{6}, invalidator.groupIDs)
}

func TestProjectServiceActivateProjectUnrestrictedScopeSkipsInternalScopeBindings(t *testing.T) {
	repo := &projectServiceRepoStub{
		projectExists: true,
		profiles: []ProjectProfile{
			{ID: 99, ProjectID: 10, Mode: ProjectProfileModeUnrestricted, IsActive: true},
			{ID: 20, ProjectID: 10, Mode: ProjectProfileModeRestricted, IsActive: false},
		},
	}
	invalidator := &projectAuthCacheInvalidatorStub{}
	svc := NewProjectService(repo, invalidator)

	profile, err := svc.ActivateProjectUnrestrictedScope(context.Background(), 10)

	require.NoError(t, err)
	require.NotNil(t, profile)
	require.True(t, repo.activateUnrestrictedCalled)
	require.Empty(t, invalidator.groupIDs)
}

func TestProjectServiceValidateProjectProfileBindingScopeNormalizesResourceIDs(t *testing.T) {
	repo := &projectServiceRepoStub{projectExists: true}
	svc := NewProjectService(repo)

	err := svc.ValidateProjectProfileBindingScope(context.Background(), 10, ProjectProfileBindingInput{
		GroupIDs:   []int64{7, -1, 6},
		AccountIDs: []int64{5, 5},
	})

	require.NoError(t, err)
	require.True(t, repo.validateBindingScopeCalled)
	require.Equal(t, []int64{6, 7}, repo.bindingInput.GroupIDs)
	require.Equal(t, []int64{5}, repo.bindingInput.AccountIDs)
}

type projectServiceRepoStub struct {
	created                        ProjectCreateInput
	projectExists                  bool
	defaultProjectCalled           bool
	userProjects                   []ProjectSummary
	listUserProjectsCalls          []int64
	projectRoles                   map[int64]string
	setMemberCalled                bool
	memberInput                    ProjectMemberInput
	createProfileCalled            bool
	updateProfileCalled            bool
	activateUnrestrictedCalled     bool
	validateBindingScopeCalled     bool
	validateBindingResourcesCalled bool
	validateBindingResourcesErr    error
	profileInput                   ProjectProfileInput
	bindingInput                   ProjectProfileBindingInput
	profiles                       []ProjectProfile
	existingBindings               *ProjectProfileBindings
	bindingsByProfileID            map[int64]*ProjectProfileBindings
}

func (r *projectServiceRepoStub) GetDefaultProjectID(context.Context) (int64, error) {
	r.defaultProjectCalled = true
	return 1, nil
}

func (r *projectServiceRepoStub) GetProjectRole(_ context.Context, projectID int64, _ int64) (string, bool, error) {
	if r.projectRoles != nil {
		role, ok := r.projectRoles[projectID]
		return role, ok, nil
	}
	return ProjectRoleAdmin, true, nil
}

func (r *projectServiceRepoStub) ProjectExists(context.Context, int64) (bool, error) {
	return r.projectExists, nil
}

func (r *projectServiceRepoStub) ListActiveProjects(context.Context) ([]ProjectSummary, error) {
	return []ProjectSummary{}, nil
}

func (r *projectServiceRepoStub) ListUserProjects(_ context.Context, userID int64) ([]ProjectSummary, error) {
	r.listUserProjectsCalls = append(r.listUserProjectsCalls, userID)
	return append([]ProjectSummary(nil), r.userProjects...), nil
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

func (r *projectServiceRepoStub) ListProjectProfiles(context.Context, int64) ([]ProjectProfile, error) {
	if r.profiles != nil {
		return append([]ProjectProfile(nil), r.profiles...), nil
	}
	return []ProjectProfile{}, nil
}

func (r *projectServiceRepoStub) CreateProjectProfile(_ context.Context, projectID int64, input ProjectProfileInput) (*ProjectProfile, error) {
	r.createProfileCalled = true
	r.profileInput = input
	return &ProjectProfile{ID: 20, ProjectID: projectID, Name: *input.Name, Mode: ProjectProfileModeRestricted}, nil
}

func (r *projectServiceRepoStub) UpdateProjectProfile(_ context.Context, projectID int64, profileID int64, input ProjectProfileInput) (*ProjectProfile, error) {
	r.updateProfileCalled = true
	r.profileInput = input
	return &ProjectProfile{ID: profileID, ProjectID: projectID, Name: "profile", Mode: ProjectProfileModeRestricted}, nil
}

func (r *projectServiceRepoStub) DeleteProjectProfile(context.Context, int64, int64) error {
	return nil
}

func (r *projectServiceRepoStub) ActivateProjectProfile(_ context.Context, projectID int64, profileID int64) (*ProjectProfile, error) {
	return &ProjectProfile{ID: profileID, ProjectID: projectID, Name: "profile", Mode: ProjectProfileModeRestricted, IsActive: true}, nil
}

func (r *projectServiceRepoStub) ActivateProjectUnrestrictedScope(_ context.Context, projectID int64) (*ProjectProfile, error) {
	r.activateUnrestrictedCalled = true
	return &ProjectProfile{ID: 99, ProjectID: projectID, Name: "Unrestricted", Mode: ProjectProfileModeUnrestricted, IsActive: true}, nil
}

func (r *projectServiceRepoStub) GetProjectProfileBindings(_ context.Context, _ int64, profileID int64) (*ProjectProfileBindings, error) {
	if r.bindingsByProfileID != nil {
		if bindings, ok := r.bindingsByProfileID[profileID]; ok {
			return cloneTestProjectProfileBindings(bindings), nil
		}
	}
	if r.existingBindings != nil {
		return cloneTestProjectProfileBindings(r.existingBindings), nil
	}
	return &ProjectProfileBindings{ProfileID: profileID}, nil
}

func (r *projectServiceRepoStub) SetProjectProfileBindings(_ context.Context, _ int64, profileID int64, input ProjectProfileBindingInput) (*ProjectProfileBindings, error) {
	r.bindingInput = input
	return &ProjectProfileBindings{
		ProfileID:       profileID,
		GroupIDs:        input.GroupIDs,
		AccountIDs:      input.AccountIDs,
		SubscriptionIDs: input.SubscriptionIDs,
	}, nil
}

func (r *projectServiceRepoStub) ValidateProjectProfileBindingScope(_ context.Context, _ int64, input ProjectProfileBindingInput) error {
	r.validateBindingScopeCalled = true
	r.bindingInput = input
	return nil
}

func (r *projectServiceRepoStub) ValidateProjectProfileBindingResources(_ context.Context, input ProjectProfileBindingInput) error {
	r.validateBindingResourcesCalled = true
	r.bindingInput = input
	return r.validateBindingResourcesErr
}

func (r *projectServiceRepoStub) SearchProjectBindableResources(context.Context, int64, string, int) (*ProjectResourceSearchResult, error) {
	return &ProjectResourceSearchResult{}, nil
}

type projectAuthCacheInvalidatorStub struct {
	keys      []string
	userIDs   []int64
	groupIDs  []int64
	apiKeyIDs []int64
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

func (s *projectAuthCacheInvalidatorStub) InvalidateAuthCacheByAPIKeyID(_ context.Context, apiKeyID int64) {
	s.apiKeyIDs = append(s.apiKeyIDs, apiKeyID)
}

func cloneTestProjectProfileBindings(value *ProjectProfileBindings) *ProjectProfileBindings {
	if value == nil {
		return nil
	}
	return &ProjectProfileBindings{
		ProfileID:       value.ProfileID,
		GroupIDs:        append([]int64(nil), value.GroupIDs...),
		AccountIDs:      append([]int64(nil), value.AccountIDs...),
		SubscriptionIDs: append([]int64(nil), value.SubscriptionIDs...),
	}
}
