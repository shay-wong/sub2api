package service

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	ProjectRoleAdmin = "admin"
	ProjectRoleUser  = "user"

	ProjectProfileModeRestricted   = "restricted"
	ProjectProfileModeUnrestricted = "unrestricted"
	ProjectResourceScopeProfile    = "profile"
	ProjectResourceScopeUnlimited  = "unrestricted"

	ProjectResourceTypeUser         = "user"
	ProjectResourceTypeGroup        = "group"
	ProjectResourceTypeAccount      = "account"
	ProjectResourceTypeProxy        = "proxy"
	ProjectResourceTypeSubscription = "subscription"
	ProjectResourceTypeAPIKey       = "api_key"
)

var (
	ErrProjectNotFound               = infraerrors.NotFound("PROJECT_NOT_FOUND", "project not found")
	ErrProjectAccessForbidden        = infraerrors.Forbidden("PROJECT_ACCESS_FORBIDDEN", "project access forbidden")
	ErrProjectInvalidInput           = infraerrors.BadRequest("PROJECT_INVALID_INPUT", "project input is invalid")
	ErrProjectInvalidRole            = infraerrors.BadRequest("PROJECT_INVALID_ROLE", "project member role must be admin or user")
	ErrProjectSlugConflict           = infraerrors.Conflict("PROJECT_SLUG_CONFLICT", "project slug already exists")
	ErrProjectOwnerTransferRequired  = infraerrors.BadRequest("PROJECT_OWNER_TRANSFER_REQUIRED", "project owner must be transferred before removal")
	ErrProjectProfileNotFound        = infraerrors.NotFound("PROJECT_PROFILE_NOT_FOUND", "project profile not found")
	ErrProjectAPIKeyGroupUnavailable = infraerrors.Forbidden("PROJECT_API_KEY_GROUP_UNAVAILABLE", "api key group is not available in target project")
	ErrProjectMemberDisabled         = infraerrors.Forbidden("PROJECT_MEMBER_DISABLED", "api key owner is disabled in this project")
)

type ProjectRepository interface {
	GetDefaultProjectID(ctx context.Context) (int64, error)
	GetProjectRole(ctx context.Context, projectID int64, userID int64) (string, bool, error)
	ProjectExists(ctx context.Context, projectID int64) (bool, error)
	ListActiveProjects(ctx context.Context) ([]ProjectSummary, error)
	ListUserProjects(ctx context.Context, userID int64) ([]ProjectSummary, error)
	CreateProject(ctx context.Context, input ProjectCreateInput) (*ProjectSummary, error)
	UpdateProject(ctx context.Context, projectID int64, input ProjectUpdateInput) (*ProjectSummary, error)
	ListProjectMembers(ctx context.Context, projectID int64) ([]ProjectMember, error)
	SetProjectMember(ctx context.Context, projectID int64, input ProjectMemberInput) (*ProjectMember, error)
	RemoveProjectMember(ctx context.Context, projectID int64, userID int64) error
	ListProjectProfiles(ctx context.Context, projectID int64) ([]ProjectProfile, error)
	CreateProjectProfile(ctx context.Context, projectID int64, input ProjectProfileInput) (*ProjectProfile, error)
	UpdateProjectProfile(ctx context.Context, projectID int64, profileID int64, input ProjectProfileInput) (*ProjectProfile, error)
	DeleteProjectProfile(ctx context.Context, projectID int64, profileID int64) error
	ActivateProjectProfile(ctx context.Context, projectID int64, profileID int64) (*ProjectProfile, error)
	ActivateProjectUnrestrictedScope(ctx context.Context, projectID int64) (*ProjectProfile, error)
	GetProjectProfileBindings(ctx context.Context, projectID int64, profileID int64) (*ProjectProfileBindings, error)
	SetProjectProfileBindings(ctx context.Context, projectID int64, profileID int64, input ProjectProfileBindingInput) (*ProjectProfileBindings, error)
	ValidateProjectProfileBindingScope(ctx context.Context, projectID int64, input ProjectProfileBindingInput) error
	ValidateProjectProfileBindingResources(ctx context.Context, input ProjectProfileBindingInput) error
	SearchProjectBindableResources(ctx context.Context, projectID int64, query string, limit int) (*ProjectResourceSearchResult, error)
}

type ProjectService struct {
	repo                 ProjectRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator
}

type ProjectSummary struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Slug        string   `json:"slug"`
	Description *string  `json:"description,omitempty"`
	Role        string   `json:"role,omitempty"`
	IsOwner     bool     `json:"is_owner"`
	Permissions []string `json:"permissions,omitempty"`
}

type ProjectCreateInput struct {
	Name        string
	Slug        string
	Description *string
	OwnerUserID int64
	ProfileMode string
	Bindings    ProjectProfileBindingInput
}

type ProjectUpdateInput struct {
	Name        *string
	Description *string
	Status      *string
}

type ProjectMemberInput struct {
	UserID      int64
	Role        string
	IsOwner     bool
	Status      *string
	Permissions []string
}

type ProjectProfile struct {
	ID          int64   `json:"id"`
	ProjectID   int64   `json:"project_id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Mode        string  `json:"mode"`
	IsActive    bool    `json:"is_active"`
	CreatedAt   string  `json:"created_at,omitempty"`
	UpdatedAt   string  `json:"updated_at,omitempty"`
}

type ProjectProfileInput struct {
	Name        *string
	Description *string
}

type ProjectProfileBindings struct {
	ProfileID       int64                                  `json:"profile_id"`
	GroupIDs        []int64                                `json:"group_ids"`
	AccountIDs      []int64                                `json:"account_ids"`
	ProxyIDs        []int64                                `json:"proxy_ids"`
	SubscriptionIDs []int64                                `json:"subscription_ids"`
	Groups          []ProjectResourceGroupCandidate        `json:"groups,omitempty"`
	Accounts        []ProjectResourceAccountCandidate      `json:"accounts,omitempty"`
	Proxies         []ProjectResourceProxyCandidate        `json:"proxies,omitempty"`
	Subscriptions   []ProjectResourceSubscriptionCandidate `json:"subscriptions,omitempty"`
}

type ProjectProfileBindingInput struct {
	GroupIDs        []int64 `json:"group_ids,omitempty"`
	AccountIDs      []int64 `json:"account_ids,omitempty"`
	ProxyIDs        []int64 `json:"proxy_ids,omitempty"`
	SubscriptionIDs []int64 `json:"subscription_ids,omitempty"`
}

type ProjectResourceSearchResult struct {
	Users         []ProjectResourceUserCandidate         `json:"users"`
	Groups        []ProjectResourceGroupCandidate        `json:"groups"`
	Accounts      []ProjectResourceAccountCandidate      `json:"accounts"`
	Proxies       []ProjectResourceProxyCandidate        `json:"proxies"`
	Subscriptions []ProjectResourceSubscriptionCandidate `json:"subscriptions"`
	APIKeys       []ProjectResourceAPIKeyCandidate       `json:"api_keys"`
}

type ProjectResourceUserCandidate struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Notes    string `json:"notes"`
	Status   string `json:"status"`
}

type ProjectResourceAPIKeyCandidate struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	KeyPrefix string `json:"key_prefix"`
	UserEmail string `json:"user_email"`
	Status    string `json:"status"`
}

type ProjectResourceGroupCandidate struct {
	ID          int64  `json:"id"`
	ProjectID   int64  `json:"project_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Platform    string `json:"platform"`
	Status      string `json:"status"`
}

type ProjectResourceAccountCandidate struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	Notes     string `json:"notes"`
	Platform  string `json:"platform"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	Email     string `json:"email"`
}

type ProjectResourceProxyCandidate struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	Name      string `json:"name"`
	Protocol  string `json:"protocol"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Status    string `json:"status"`
}

type ProjectResourceSubscriptionCandidate struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	GroupID   int64  `json:"group_id"`
	UserEmail string `json:"user_email"`
	GroupName string `json:"group_name"`
	Status    string `json:"status"`
	Notes     string `json:"notes"`
}

type ProjectMember struct {
	ProjectID   int64    `json:"project_id"`
	UserID      int64    `json:"user_id"`
	Email       string   `json:"email"`
	Username    string   `json:"username"`
	Role        string   `json:"role"`
	UserRole    string   `json:"user_role,omitempty"`
	IsOwner     bool     `json:"is_owner"`
	Status      string   `json:"status"`
	UserStatus  string   `json:"user_status,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
}

var projectSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,78}[a-z0-9]$|^[a-z0-9]$`)

func NewProjectService(repo ProjectRepository, authCacheInvalidator ...APIKeyAuthCacheInvalidator) *ProjectService {
	s := &ProjectService{repo: repo}
	if len(authCacheInvalidator) > 0 {
		s.authCacheInvalidator = authCacheInvalidator[0]
	}
	return s
}

func (s *ProjectService) ResolveAdminProject(ctx context.Context, user *User, requestedProjectID int64) (int64, string, []string, error) {
	if user == nil || user.ID <= 0 {
		return 0, "", nil, infraerrors.Unauthorized("UNAUTHORIZED", "authorization required")
	}
	if s == nil || s.repo == nil {
		return 0, "", nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}

	if RoleIsSuperAdmin(user.Role) {
		projectID := requestedProjectID
		if projectID <= 0 {
			defaultProjectID, err := s.repo.GetDefaultProjectID(ctx)
			if err != nil {
				return 0, "", nil, err
			}
			projectID = defaultProjectID
		}
		exists, err := s.repo.ProjectExists(ctx, projectID)
		if err != nil {
			return 0, "", nil, err
		}
		if !exists {
			return 0, "", nil, ErrProjectNotFound
		}
		return projectID, RoleSuperAdmin, DefaultProjectAdminPermissions(), nil
	}
	if RoleIsOperator(user.Role) {
		return 0, "", nil, ErrLegacyOperatorRoleDisabled
	}

	projects, err := s.repo.ListUserProjects(ctx, user.ID)
	if err != nil {
		return 0, "", nil, err
	}
	projectID := requestedProjectID
	if projectID <= 0 {
		for _, project := range projects {
			if project.Role == ProjectRoleAdmin {
				projectID = project.ID
				break
			}
		}
		if projectID <= 0 {
			return 0, "", nil, ErrProjectAccessForbidden
		}
	}

	for _, project := range projects {
		if project.ID != projectID {
			continue
		}
		if project.Role != ProjectRoleAdmin {
			return 0, "", nil, ErrProjectAccessForbidden
		}
		return projectID, RoleAdmin, project.Permissions, nil
	}
	return 0, "", nil, ErrProjectAccessForbidden
}

func (s *ProjectService) ResolveUserProject(ctx context.Context, user *User, requestedProjectID int64) (int64, error) {
	if user == nil || user.ID <= 0 {
		return 0, infraerrors.Unauthorized("UNAUTHORIZED", "authorization required")
	}
	if s == nil || s.repo == nil {
		return 0, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}

	if requestedProjectID > 0 {
		if RoleIsSuperAdmin(user.Role) {
			exists, err := s.repo.ProjectExists(ctx, requestedProjectID)
			if err != nil {
				return 0, err
			}
			if !exists {
				return 0, ErrProjectNotFound
			}
			return requestedProjectID, nil
		}
		_, ok, err := s.repo.GetProjectRole(ctx, requestedProjectID, user.ID)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, ErrProjectAccessForbidden
		}
		return requestedProjectID, nil
	}

	projects, err := s.ListUserProjects(ctx, user)
	if err != nil {
		return 0, err
	}
	if len(projects) == 0 {
		return 0, ErrProjectAccessForbidden
	}
	return projects[0].ID, nil
}

func (s *ProjectService) ListUserProjects(ctx context.Context, user *User) ([]ProjectSummary, error) {
	if user == nil || user.ID <= 0 {
		return nil, infraerrors.Unauthorized("UNAUTHORIZED", "authorization required")
	}
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if RoleIsSuperAdmin(user.Role) {
		projects, err := s.repo.ListActiveProjects(ctx)
		if err != nil {
			return nil, err
		}
		for i := range projects {
			projects[i].Role = RoleSuperAdmin
			projects[i].IsOwner = true
			projects[i].Permissions = DefaultProjectAdminPermissions()
		}
		return projects, nil
	}
	return s.repo.ListUserProjects(ctx, user.ID)
}

func (s *ProjectService) ListProjects(ctx context.Context) ([]ProjectSummary, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	return s.repo.ListActiveProjects(ctx)
}

func (s *ProjectService) CreateProject(ctx context.Context, input ProjectCreateInput) (*ProjectSummary, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = normalizeProjectSlug(input.Slug)
	if input.Name == "" || input.Slug == "" || !projectSlugPattern.MatchString(input.Slug) || input.OwnerUserID <= 0 {
		return nil, ErrProjectInvalidInput
	}
	if input.Description != nil {
		desc := strings.TrimSpace(*input.Description)
		input.Description = &desc
	}
	input.ProfileMode = strings.TrimSpace(input.ProfileMode)
	if input.ProfileMode == "" {
		input.ProfileMode = ProjectProfileModeRestricted
	}
	if input.ProfileMode != ProjectProfileModeRestricted && input.ProfileMode != ProjectProfileModeUnrestricted {
		return nil, ErrProjectInvalidInput
	}
	input.Bindings = normalizeProjectProfileBindingInput(input.Bindings)
	if !projectProfileBindingsEmpty(input.Bindings) {
		if err := s.repo.ValidateProjectProfileBindingResources(ctx, input.Bindings); err != nil {
			return nil, err
		}
	}
	project, err := s.repo.CreateProject(ctx, input)
	if err != nil {
		return nil, err
	}
	return project, nil
}

func (s *ProjectService) UpdateProject(ctx context.Context, projectID int64, input ProjectUpdateInput) (*ProjectSummary, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 {
		return nil, ErrProjectInvalidInput
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, ErrProjectInvalidInput
		}
		input.Name = &name
	}
	if input.Description != nil {
		desc := strings.TrimSpace(*input.Description)
		input.Description = &desc
	}
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if status != StatusActive && status != StatusDisabled {
			return nil, ErrProjectInvalidInput
		}
		input.Status = &status
	}
	project, err := s.repo.UpdateProject(ctx, projectID, input)
	if err != nil {
		return nil, err
	}
	return project, nil
}

func (s *ProjectService) ListProjectMembers(ctx context.Context, projectID int64) ([]ProjectMember, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 {
		return nil, ErrProjectInvalidInput
	}
	exists, err := s.repo.ProjectExists(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrProjectNotFound
	}
	return s.repo.ListProjectMembers(ctx, projectID)
}

func (s *ProjectService) SetProjectMember(ctx context.Context, projectID int64, input ProjectMemberInput) (*ProjectMember, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 || input.UserID <= 0 {
		return nil, ErrProjectInvalidInput
	}
	input.Role = strings.TrimSpace(input.Role)
	if input.Role != ProjectRoleAdmin && input.Role != ProjectRoleUser {
		return nil, ErrProjectInvalidRole
	}
	if input.IsOwner {
		input.Role = ProjectRoleAdmin
		input.Permissions = DefaultProjectAdminPermissions()
	}
	input.Permissions = ProjectAdminPermissionsForStorage(input.Role, input.Permissions)
	if input.Status != nil {
		status := strings.TrimSpace(*input.Status)
		if status != StatusActive && status != StatusDisabled {
			return nil, ErrProjectInvalidInput
		}
		input.Status = &status
	}
	if input.IsOwner {
		status := StatusActive
		input.Status = &status
	}
	exists, err := s.repo.ProjectExists(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrProjectNotFound
	}
	return s.repo.SetProjectMember(ctx, projectID, input)
}

func (s *ProjectService) RemoveProjectMember(ctx context.Context, projectID int64, userID int64) error {
	if s == nil || s.repo == nil {
		return infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 || userID <= 0 {
		return ErrProjectInvalidInput
	}
	exists, err := s.repo.ProjectExists(ctx, projectID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrProjectNotFound
	}
	return s.repo.RemoveProjectMember(ctx, projectID, userID)
}

func (s *ProjectService) ListProjectProfiles(ctx context.Context, projectID int64) ([]ProjectProfile, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 {
		return nil, ErrProjectInvalidInput
	}
	if err := s.ensureProjectExists(ctx, projectID); err != nil {
		return nil, err
	}
	return s.repo.ListProjectProfiles(ctx, projectID)
}

func (s *ProjectService) CreateProjectProfile(ctx context.Context, projectID int64, input ProjectProfileInput) (*ProjectProfile, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 {
		return nil, ErrProjectInvalidInput
	}
	if err := s.normalizeProjectProfileInput(&input, true); err != nil {
		return nil, err
	}
	if err := s.ensureProjectExists(ctx, projectID); err != nil {
		return nil, err
	}
	return s.repo.CreateProjectProfile(ctx, projectID, input)
}

func (s *ProjectService) UpdateProjectProfile(ctx context.Context, projectID int64, profileID int64, input ProjectProfileInput) (*ProjectProfile, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 || profileID <= 0 {
		return nil, ErrProjectInvalidInput
	}
	if err := s.normalizeProjectProfileInput(&input, false); err != nil {
		return nil, err
	}
	if input.Name == nil && input.Description == nil {
		return nil, ErrProjectInvalidInput
	}
	if err := s.ensureProjectExists(ctx, projectID); err != nil {
		return nil, err
	}
	if err := s.ensureRestrictedProjectProfile(ctx, projectID, profileID); err != nil {
		return nil, err
	}
	profile, err := s.repo.UpdateProjectProfile(ctx, projectID, profileID, input)
	if err != nil {
		return nil, err
	}
	return profile, nil
}

func (s *ProjectService) DeleteProjectProfile(ctx context.Context, projectID int64, profileID int64) error {
	if s == nil || s.repo == nil {
		return infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 || profileID <= 0 {
		return ErrProjectInvalidInput
	}
	if err := s.ensureProjectExists(ctx, projectID); err != nil {
		return err
	}
	if err := s.ensureRestrictedProjectProfile(ctx, projectID, profileID); err != nil {
		return err
	}
	return s.repo.DeleteProjectProfile(ctx, projectID, profileID)
}

func (s *ProjectService) ActivateProjectProfile(ctx context.Context, projectID int64, profileID int64) (*ProjectProfile, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 || profileID <= 0 {
		return nil, ErrProjectInvalidInput
	}
	if err := s.ensureProjectExists(ctx, projectID); err != nil {
		return nil, err
	}
	if err := s.ensureRestrictedProjectProfile(ctx, projectID, profileID); err != nil {
		return nil, err
	}
	affectedBindings, err := s.projectProfileActivationAuthCacheBindings(ctx, projectID, profileID)
	if err != nil {
		return nil, err
	}
	profile, err := s.repo.ActivateProjectProfile(ctx, projectID, profileID)
	if err != nil {
		return nil, err
	}
	s.invalidateProjectProfileAuthCache(ctx, affectedBindings...)
	return profile, nil
}

func (s *ProjectService) ActivateProjectUnrestrictedScope(ctx context.Context, projectID int64) (*ProjectProfile, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 {
		return nil, ErrProjectInvalidInput
	}
	if err := s.ensureProjectExists(ctx, projectID); err != nil {
		return nil, err
	}
	affectedBindings, err := s.activeProjectProfileAuthCacheBindings(ctx, projectID)
	if err != nil {
		return nil, err
	}
	profile, err := s.repo.ActivateProjectUnrestrictedScope(ctx, projectID)
	if err != nil {
		return nil, err
	}
	s.invalidateProjectProfileAuthCache(ctx, affectedBindings...)
	return profile, nil
}

func (s *ProjectService) GetProjectProfileBindings(ctx context.Context, projectID int64, profileID int64) (*ProjectProfileBindings, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 || profileID <= 0 {
		return nil, ErrProjectInvalidInput
	}
	if err := s.ensureProjectExists(ctx, projectID); err != nil {
		return nil, err
	}
	if err := s.ensureRestrictedProjectProfile(ctx, projectID, profileID); err != nil {
		return nil, err
	}
	bindings, err := s.repo.GetProjectProfileBindings(ctx, projectID, profileID)
	if err != nil {
		return nil, err
	}
	return normalizeProjectProfileBindings(bindings), nil
}

func (s *ProjectService) SetProjectProfileBindings(ctx context.Context, projectID int64, profileID int64, input ProjectProfileBindingInput) (*ProjectProfileBindings, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 || profileID <= 0 {
		return nil, ErrProjectInvalidInput
	}
	input = normalizeProjectProfileBindingInput(input)
	if err := s.ensureProjectExists(ctx, projectID); err != nil {
		return nil, err
	}
	if err := s.ensureRestrictedProjectProfile(ctx, projectID, profileID); err != nil {
		return nil, err
	}
	currentBindings, err := s.repo.GetProjectProfileBindings(ctx, projectID, profileID)
	if err != nil {
		return nil, err
	}
	currentBindings = normalizeProjectProfileBindings(currentBindings)
	updatedBindings, err := s.repo.SetProjectProfileBindings(ctx, projectID, profileID, input)
	if err != nil {
		return nil, err
	}
	updatedBindings = normalizeProjectProfileBindings(updatedBindings)
	s.invalidateProjectProfileAuthCache(ctx, currentBindings, updatedBindings)
	return updatedBindings, nil
}

func (s *ProjectService) ValidateProjectProfileBindingScope(ctx context.Context, projectID int64, input ProjectProfileBindingInput) error {
	if s == nil || s.repo == nil {
		return infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 {
		return ErrProjectInvalidInput
	}
	input = normalizeProjectProfileBindingInput(input)
	if err := s.ensureProjectExists(ctx, projectID); err != nil {
		return err
	}
	return s.repo.ValidateProjectProfileBindingScope(ctx, projectID, input)
}

func (s *ProjectService) SearchProjectBindableResources(ctx context.Context, projectID int64, query string, limit int) (*ProjectResourceSearchResult, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 {
		projectID = 0
	}
	query = strings.TrimSpace(query)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if projectID > 0 {
		if err := s.ensureProjectExists(ctx, projectID); err != nil {
			return nil, err
		}
	}
	return s.repo.SearchProjectBindableResources(ctx, projectID, query, limit)
}

func (s *ProjectService) ensureProjectExists(ctx context.Context, projectID int64) error {
	exists, err := s.repo.ProjectExists(ctx, projectID)
	if err != nil {
		return err
	}
	if !exists {
		return ErrProjectNotFound
	}
	return nil
}

func (s *ProjectService) normalizeProjectProfileInput(input *ProjectProfileInput, requireName bool) error {
	if input == nil {
		return ErrProjectInvalidInput
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return ErrProjectInvalidInput
		}
		input.Name = &name
	} else if requireName {
		return ErrProjectInvalidInput
	}
	if input.Description != nil {
		desc := strings.TrimSpace(*input.Description)
		input.Description = &desc
	}
	return nil
}

func (s *ProjectService) ensureRestrictedProjectProfile(ctx context.Context, projectID int64, profileID int64) error {
	profiles, err := s.repo.ListProjectProfiles(ctx, projectID)
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		if profile.ID == profileID {
			if profile.Mode != ProjectProfileModeRestricted {
				return ErrProjectInvalidInput
			}
			return nil
		}
	}
	return ErrProjectProfileNotFound
}

func normalizeProjectResourceIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func normalizeProjectProfileBindingInput(input ProjectProfileBindingInput) ProjectProfileBindingInput {
	input.GroupIDs = normalizeProjectResourceIDs(input.GroupIDs)
	input.AccountIDs = normalizeProjectResourceIDs(input.AccountIDs)
	input.ProxyIDs = normalizeProjectResourceIDs(input.ProxyIDs)
	input.SubscriptionIDs = normalizeProjectResourceIDs(input.SubscriptionIDs)
	return input
}

func projectProfileBindingsEmpty(input ProjectProfileBindingInput) bool {
	return len(input.GroupIDs) == 0 &&
		len(input.AccountIDs) == 0 &&
		len(input.ProxyIDs) == 0 &&
		len(input.SubscriptionIDs) == 0
}

func normalizeProjectProfileBindings(bindings *ProjectProfileBindings) *ProjectProfileBindings {
	if bindings == nil {
		return nil
	}
	if bindings.GroupIDs == nil {
		bindings.GroupIDs = []int64{}
	}
	if bindings.AccountIDs == nil {
		bindings.AccountIDs = []int64{}
	}
	if bindings.ProxyIDs == nil {
		bindings.ProxyIDs = []int64{}
	}
	if bindings.SubscriptionIDs == nil {
		bindings.SubscriptionIDs = []int64{}
	}
	return bindings
}

func (s *ProjectService) projectProfileActivationAuthCacheBindings(ctx context.Context, projectID int64, profileID int64) ([]*ProjectProfileBindings, error) {
	profiles, err := s.repo.ListProjectProfiles(ctx, projectID)
	if err != nil {
		return nil, err
	}
	affected := make([]*ProjectProfileBindings, 0, 2)
	seen := make(map[int64]struct{}, 2)
	for _, profile := range profiles {
		if !profile.IsActive && profile.ID != profileID {
			continue
		}
		if _, ok := seen[profile.ID]; ok {
			continue
		}
		seen[profile.ID] = struct{}{}
		bindings, err := s.repo.GetProjectProfileBindings(ctx, projectID, profile.ID)
		if err != nil {
			return nil, err
		}
		affected = append(affected, normalizeProjectProfileBindings(bindings))
	}
	return affected, nil
}

func (s *ProjectService) activeProjectProfileAuthCacheBindings(ctx context.Context, projectID int64) ([]*ProjectProfileBindings, error) {
	profiles, err := s.repo.ListProjectProfiles(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if !profile.IsActive || profile.Mode != ProjectProfileModeRestricted {
			continue
		}
		bindings, err := s.repo.GetProjectProfileBindings(ctx, projectID, profile.ID)
		if err != nil {
			return nil, err
		}
		return []*ProjectProfileBindings{normalizeProjectProfileBindings(bindings)}, nil
	}
	return nil, nil
}

func (s *ProjectService) invalidateProjectProfileAuthCache(ctx context.Context, bindings ...*ProjectProfileBindings) {
	if s == nil || s.authCacheInvalidator == nil {
		return
	}
	userIDs := map[int64]struct{}{}
	groupIDs := map[int64]struct{}{}
	for _, binding := range bindings {
		if binding == nil {
			continue
		}
		addIDs(groupIDs, binding.GroupIDs)
	}
	for _, id := range sortedIDKeys(userIDs) {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, id)
	}
	for _, id := range sortedIDKeys(groupIDs) {
		s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, id)
	}
}

func addIDs(dst map[int64]struct{}, ids []int64) {
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		dst[id] = struct{}{}
	}
}

func sortedIDKeys(values map[int64]struct{}) []int64 {
	if len(values) == 0 {
		return nil
	}
	out := make([]int64, 0, len(values))
	for id := range values {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func IsProjectNotFound(err error) bool {
	return errors.Is(err, ErrProjectNotFound)
}

func normalizeProjectSlug(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	out := make([]rune, 0, len(slug))
	lastDash := false
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out = append(out, r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ':
			if !lastDash {
				out = append(out, '-')
				lastDash = true
			}
		}
	}
	return strings.Trim(string(out), "-")
}
