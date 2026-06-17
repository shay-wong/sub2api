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
)

var (
	ErrProjectNotFound        = infraerrors.NotFound("PROJECT_NOT_FOUND", "project not found")
	ErrProjectAccessForbidden = infraerrors.Forbidden("PROJECT_ACCESS_FORBIDDEN", "project access forbidden")
	ErrProjectInvalidInput    = infraerrors.BadRequest("PROJECT_INVALID_INPUT", "project input is invalid")
	ErrProjectInvalidRole     = infraerrors.BadRequest("PROJECT_INVALID_ROLE", "project member role must be admin or user")
	ErrProjectSlugConflict    = infraerrors.Conflict("PROJECT_SLUG_CONFLICT", "project slug already exists")
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
	MoveProjectResources(ctx context.Context, projectID int64, input ProjectResourceMoveInput) (*ProjectResourceMoveResult, error)
}

type ProjectService struct {
	repo                 ProjectRepository
	authCacheInvalidator APIKeyAuthCacheInvalidator
}

type ProjectSummary struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description *string `json:"description,omitempty"`
	Role        string  `json:"role,omitempty"`
	IsOwner     bool    `json:"is_owner"`
}

type ProjectCreateInput struct {
	Name        string
	Slug        string
	Description *string
	OwnerUserID int64
}

type ProjectUpdateInput struct {
	Name        *string
	Description *string
	Status      *string
}

type ProjectMemberInput struct {
	UserID  int64
	Role    string
	IsOwner bool
}

type ProjectResourceMoveInput struct {
	AccountIDs       []int64 `json:"account_ids,omitempty"`
	APIKeyIDs        []int64 `json:"api_key_ids,omitempty"`
	GroupIDs         []int64 `json:"group_ids,omitempty"`
	MoveUsageHistory bool    `json:"move_usage_history"`
}

type ProjectResourceMoveResult struct {
	AccountsMoved               int64    `json:"accounts_moved"`
	APIKeysMoved                int64    `json:"api_keys_moved"`
	GroupsMoved                 int64    `json:"groups_moved"`
	AccountGroupBindingsRemoved int64    `json:"account_group_bindings_removed"`
	APIKeyGroupBindingsCleared  int64    `json:"api_key_group_bindings_cleared"`
	GroupFallbacksCleared       int64    `json:"group_fallbacks_cleared"`
	GroupModelRoutingCleared    int64    `json:"group_model_routing_cleared"`
	ProjectMembersAdded         int64    `json:"project_members_added"`
	UsageLogsMoved              int64    `json:"usage_logs_moved"`
	OpsErrorLogsMoved           int64    `json:"ops_error_logs_moved"`
	InvalidatedUserIDs          []int64  `json:"-"`
	InvalidatedGroupIDs         []int64  `json:"-"`
	InvalidatedAPIKeys          []string `json:"-"`
}

type ProjectMember struct {
	ProjectID int64  `json:"project_id"`
	UserID    int64  `json:"user_id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	IsOwner   bool   `json:"is_owner"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

var projectSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,78}[a-z0-9]$|^[a-z0-9]$`)

func NewProjectService(repo ProjectRepository, authCacheInvalidator ...APIKeyAuthCacheInvalidator) *ProjectService {
	s := &ProjectService{repo: repo}
	if len(authCacheInvalidator) > 0 {
		s.authCacheInvalidator = authCacheInvalidator[0]
	}
	return s
}

func (s *ProjectService) ResolveAdminProject(ctx context.Context, user *User, requestedProjectID int64) (int64, string, error) {
	if user == nil || user.ID <= 0 {
		return 0, "", infraerrors.Unauthorized("UNAUTHORIZED", "authorization required")
	}
	if s == nil || s.repo == nil {
		return 0, "", infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}

	projectID := requestedProjectID
	if projectID <= 0 {
		defaultProjectID, err := s.repo.GetDefaultProjectID(ctx)
		if err != nil {
			return 0, "", err
		}
		projectID = defaultProjectID
	}

	if RoleIsSuperAdmin(user.Role) {
		exists, err := s.repo.ProjectExists(ctx, projectID)
		if err != nil {
			return 0, "", err
		}
		if !exists {
			return 0, "", ErrProjectNotFound
		}
		return projectID, RoleSuperAdmin, nil
	}

	role, ok, err := s.repo.GetProjectRole(ctx, projectID, user.ID)
	if err != nil {
		return 0, "", err
	}
	if !ok {
		return 0, "", ErrProjectAccessForbidden
	}
	if role != ProjectRoleAdmin {
		return 0, "", ErrProjectAccessForbidden
	}
	return projectID, RoleAdmin, nil
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

func (s *ProjectService) MoveProjectResources(ctx context.Context, projectID int64, input ProjectResourceMoveInput) (*ProjectResourceMoveResult, error) {
	if s == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PROJECT_SERVICE_UNAVAILABLE", "project service unavailable")
	}
	if projectID <= 0 {
		return nil, ErrProjectInvalidInput
	}
	input.AccountIDs = normalizeProjectResourceIDs(input.AccountIDs)
	input.APIKeyIDs = normalizeProjectResourceIDs(input.APIKeyIDs)
	input.GroupIDs = normalizeProjectResourceIDs(input.GroupIDs)
	if len(input.AccountIDs) == 0 && len(input.APIKeyIDs) == 0 && len(input.GroupIDs) == 0 {
		return nil, ErrProjectInvalidInput
	}
	exists, err := s.repo.ProjectExists(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrProjectNotFound
	}
	result, err := s.repo.MoveProjectResources(ctx, projectID, input)
	if err != nil {
		return nil, err
	}
	s.invalidateMovedProjectResourceAuthCache(ctx, result)
	return result, nil
}

func (s *ProjectService) invalidateMovedProjectResourceAuthCache(ctx context.Context, result *ProjectResourceMoveResult) {
	if s == nil || s.authCacheInvalidator == nil || result == nil {
		return
	}
	for _, key := range result.InvalidatedAPIKeys {
		s.authCacheInvalidator.InvalidateAuthCacheByKey(ctx, key)
	}
	for _, userID := range result.InvalidatedUserIDs {
		s.authCacheInvalidator.InvalidateAuthCacheByUserID(ctx, userID)
	}
	for _, groupID := range result.InvalidatedGroupIDs {
		s.authCacheInvalidator.InvalidateAuthCacheByGroupID(ctx, groupID)
	}
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

func IsProjectNotFound(err error) bool {
	return errors.Is(err, ErrProjectNotFound)
}

func normalizeProjectSlug(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	var b strings.Builder
	lastDash := false
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == ' ':
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
