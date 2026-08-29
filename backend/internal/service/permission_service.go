package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	AdminPermissionDashboardRead = "admin.dashboard.read"
	AdminPermissionOpsRead       = "admin.ops.read"
	AdminPermissionAccountsWrite = "admin.accounts.write"
	AdminPermissionUsersManage   = "admin.users.manage"
	AdminPermissionGroupsManage  = "admin.groups.manage"
	AdminPermissionProxiesManage = "admin.proxies.manage"
	AdminPermissionSubsManage    = "admin.subscriptions.manage"
	AdminPermissionUsageRead     = "admin.usage.read"
)

var defaultAdminPermissions = []string{
	AdminPermissionDashboardRead,
	AdminPermissionOpsRead,
	AdminPermissionUsersManage,
	AdminPermissionGroupsManage,
	AdminPermissionProxiesManage,
	AdminPermissionSubsManage,
	AdminPermissionAccountsWrite,
	AdminPermissionUsageRead,
}

var (
	ErrPermissionUserNotFound       = infraerrors.NotFound("PERMISSION_USER_NOT_FOUND", "permission subject not found")
	ErrPermissionInvalidRole        = infraerrors.BadRequest("PERMISSION_INVALID_ROLE", "role must be user")
	ErrPermissionCannotChangeAdmin  = infraerrors.Forbidden("PERMISSION_ADMIN_IMMUTABLE", "admin role cannot be changed here")
	ErrLegacyOperatorRoleDisabled   = infraerrors.Forbidden("LEGACY_OPERATOR_ROLE_DISABLED", "legacy operator role is disabled; assign the admin role and explicit admin permissions")
	ErrPermissionInvalidGroupScope  = infraerrors.BadRequest("PERMISSION_INVALID_GROUP_SCOPE", "one or more scoped groups do not exist")
	ErrOperatorScopeRequired        = infraerrors.Forbidden("OPERATOR_SCOPE_REQUIRED", "operator does not have any assigned groups")
	ErrOperatorScopeForbidden       = infraerrors.Forbidden("OPERATOR_SCOPE_FORBIDDEN", "operator cannot access this group scope")
	ErrOperatorAccountScopeRequired = infraerrors.Forbidden("OPERATOR_ACCOUNT_SCOPE_REQUIRED", "operator-managed accounts must stay in assigned groups")
	ErrOperatorAccountForbidden     = infraerrors.Forbidden("OPERATOR_ACCOUNT_FORBIDDEN", "operator cannot access this account")
)

type OperatorPermissionRepository interface {
	ListOperatorPermissionSubjects(ctx context.Context) ([]OperatorPermissionSubject, error)
	GetOperatorGroupIDs(ctx context.Context, userID int64) ([]int64, error)
	GetOperatorScopesByUserIDs(ctx context.Context, userIDs []int64) (map[int64][]int64, error)
	SetOperatorGroupIDs(ctx context.Context, userID int64, groupIDs []int64, createdBy *int64) error
	ClearOperatorGroupIDs(ctx context.Context, userID int64) error
}

type OperatorPermissionSubject struct {
	ID        int64   `json:"id"`
	Email     string  `json:"email"`
	Username  string  `json:"username"`
	Role      string  `json:"role"`
	Status    string  `json:"status"`
	GroupIDs  []int64 `json:"group_ids"`
	CreatedAt string  `json:"created_at,omitempty"`
	UpdatedAt string  `json:"updated_at,omitempty"`
}

type PermissionService struct {
	repo     OperatorPermissionRepository
	userRepo UserRepository
}

func DefaultAdminPermissions() []string {
	return append([]string(nil), defaultAdminPermissions...)
}

func NormalizeAdminPermissions(role string, permissions []string) ([]string, error) {
	if role != RoleAdmin {
		return []string{}, nil
	}
	allowed := map[string]struct{}{}
	for _, permission := range defaultAdminPermissions {
		allowed[permission] = struct{}{}
	}
	out := make([]string, 0, len(defaultAdminPermissions))
	seen := map[string]struct{}{}
	for _, raw := range permissions {
		permission := strings.TrimSpace(raw)
		if permission == "" {
			return nil, infraerrors.BadRequest("INVALID_ADMIN_PERMISSION", "admin permission must not be empty")
		}
		if _, ok := allowed[permission]; !ok {
			return nil, infraerrors.BadRequest("INVALID_ADMIN_PERMISSION", "unknown admin permission: "+permission)
		}
		if _, ok := seen[permission]; ok {
			continue
		}
		seen[permission] = struct{}{}
		out = append(out, permission)
	}
	sort.Strings(out)
	return out, nil
}

func AdminHasPermission(role string, permissions []string, permission string) bool {
	if RoleIsSuperAdmin(role) {
		return true
	}
	for _, item := range permissions {
		if item == permission {
			return true
		}
	}
	return false
}

func NewPermissionService(repo OperatorPermissionRepository, userRepo UserRepository, groupRepo GroupRepository) *PermissionService {
	return &PermissionService{repo: repo, userRepo: userRepo}
}

func (s *PermissionService) ListOperatorPermissions(ctx context.Context) ([]OperatorPermissionSubject, error) {
	if s.repo == nil {
		return []OperatorPermissionSubject{}, nil
	}
	subjects, err := s.repo.ListOperatorPermissionSubjects(ctx)
	if err != nil {
		return nil, err
	}
	return subjects, nil
}

func (s *PermissionService) GetOperatorGroupIDs(ctx context.Context, userID int64) ([]int64, error) {
	if userID <= 0 || s.repo == nil {
		return []int64{}, nil
	}
	groupIDs, err := s.repo.GetOperatorGroupIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	return normalizeInt64Set(groupIDs), nil
}

func (s *PermissionService) SetOperatorPermissions(ctx context.Context, userID int64, role string, _ []int64, createdBy *int64) (*OperatorPermissionSubject, error) {
	if s.userRepo == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PERMISSION_SERVICE_UNAVAILABLE", "permission service unavailable")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrPermissionUserNotFound
	}
	if RoleIsAdmin(user.Role) {
		return nil, ErrPermissionCannotChangeAdmin
	}

	switch role {
	case RoleUser:
	case RoleOperator:
		return nil, ErrLegacyOperatorRoleDisabled
	default:
		return nil, ErrPermissionInvalidRole
	}

	user.Role = role
	if err := s.userRepo.Update(ctx, user, UserUpdateFields{Role: true}); err != nil {
		return nil, fmt.Errorf("update permission subject role: %w", err)
	}

	if err := s.repo.ClearOperatorGroupIDs(ctx, userID); err != nil {
		return nil, err
	}

	return &OperatorPermissionSubject{
		ID:       user.ID,
		Email:    user.Email,
		Username: user.Username,
		Role:     user.Role,
		Status:   user.Status,
		GroupIDs: []int64{},
	}, nil
}

func normalizeInt64Set(values []int64) []int64 {
	if len(values) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(values))
	out := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
