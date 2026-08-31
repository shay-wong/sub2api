package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type adminResourceActorContextKey struct{}

func WithRestrictedAdminResourceActor(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, adminResourceActorContextKey{}, userID)
}

func RestrictedAdminResourceActorFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(adminResourceActorContextKey{}).(int64)
	return userID, ok && userID > 0
}

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
	ErrPermissionUserNotFound          = infraerrors.NotFound("PERMISSION_USER_NOT_FOUND", "permission subject not found")
	ErrPermissionInvalidRole           = infraerrors.BadRequest("PERMISSION_INVALID_ROLE", "role must be user")
	ErrPermissionCannotChangeAdmin     = infraerrors.Forbidden("PERMISSION_ADMIN_IMMUTABLE", "admin role cannot be changed here")
	ErrLegacyOperatorRoleDisabled      = infraerrors.Forbidden("LEGACY_OPERATOR_ROLE_DISABLED", "legacy operator role is disabled; assign the admin role and explicit admin permissions")
	ErrPermissionInvalidGroupScope     = infraerrors.BadRequest("PERMISSION_INVALID_GROUP_SCOPE", "one or more scoped groups do not exist")
	ErrOperatorScopeRequired           = infraerrors.Forbidden("OPERATOR_SCOPE_REQUIRED", "operator does not have any assigned groups")
	ErrOperatorScopeForbidden          = infraerrors.Forbidden("OPERATOR_SCOPE_FORBIDDEN", "operator cannot access this group scope")
	ErrOperatorAccountScopeRequired    = infraerrors.Forbidden("OPERATOR_ACCOUNT_SCOPE_REQUIRED", "operator-managed accounts must stay in assigned groups")
	ErrOperatorAccountForbidden        = infraerrors.Forbidden("OPERATOR_ACCOUNT_FORBIDDEN", "operator cannot access this account")
	ErrAdminResourceScopeInvalid       = infraerrors.BadRequest("ADMIN_RESOURCE_SCOPE_INVALID", "admin resource scope is invalid")
	ErrAdminResourceScopeUnavailable   = infraerrors.InternalServer("ADMIN_RESOURCE_SCOPE_UNAVAILABLE", "admin resource scope service unavailable")
	ErrAdminGroupScopeForbidden        = infraerrors.Forbidden("ADMIN_GROUP_SCOPE_FORBIDDEN", "admin cannot access this group")
	ErrAdminAccountScopeForbidden      = infraerrors.Forbidden("ADMIN_ACCOUNT_SCOPE_FORBIDDEN", "admin cannot access this account")
	ErrAdminAccountScopeRequired       = infraerrors.Forbidden("ADMIN_ACCOUNT_SCOPE_REQUIRED", "restricted admins must assign managed accounts to an allowed group")
	ErrAdminProxyScopeForbidden        = infraerrors.Forbidden("ADMIN_PROXY_SCOPE_FORBIDDEN", "admin cannot access this proxy")
	ErrAdminSubscriptionScopeForbidden = infraerrors.Forbidden("ADMIN_SUBSCRIPTION_SCOPE_FORBIDDEN", "admin cannot access this subscription")
)

const (
	AdminResourceScopeAll        = "all"
	AdminResourceScopeRestricted = "restricted"

	AdminResourceGroup        = "group"
	AdminResourceAccount      = "account"
	AdminResourceProxy        = "proxy"
	AdminResourceSubscription = "subscription"
)

type AdminResourceScope struct {
	Mode            string  `json:"mode"`
	GroupIDs        []int64 `json:"group_ids"`
	AccountIDs      []int64 `json:"account_ids"`
	ProxyIDs        []int64 `json:"proxy_ids"`
	SubscriptionIDs []int64 `json:"subscription_ids"`
}

func UnrestrictedAdminResourceScope() AdminResourceScope {
	return AdminResourceScope{
		Mode:            AdminResourceScopeAll,
		GroupIDs:        []int64{},
		AccountIDs:      []int64{},
		ProxyIDs:        []int64{},
		SubscriptionIDs: []int64{},
	}
}

func NormalizeAdminResourceScope(role string, scope AdminResourceScope) (AdminResourceScope, error) {
	if role != RoleAdmin || scope.Mode == "" || scope.Mode == AdminResourceScopeAll {
		return UnrestrictedAdminResourceScope(), nil
	}
	if scope.Mode != AdminResourceScopeRestricted {
		return AdminResourceScope{}, ErrAdminResourceScopeInvalid
	}
	scope.Mode = AdminResourceScopeRestricted
	scope.GroupIDs = normalizeInt64Set(scope.GroupIDs)
	scope.AccountIDs = normalizeInt64Set(scope.AccountIDs)
	scope.ProxyIDs = normalizeInt64Set(scope.ProxyIDs)
	scope.SubscriptionIDs = normalizeInt64Set(scope.SubscriptionIDs)
	return scope, nil
}

type OperatorPermissionRepository interface {
	ListOperatorPermissionSubjects(ctx context.Context) ([]OperatorPermissionSubject, error)
	GetOperatorGroupIDs(ctx context.Context, userID int64) ([]int64, error)
	GetOperatorScopesByUserIDs(ctx context.Context, userIDs []int64) (map[int64][]int64, error)
	SetOperatorGroupIDs(ctx context.Context, userID int64, groupIDs []int64, createdBy *int64) error
	ClearOperatorGroupIDs(ctx context.Context, userID int64) error
}

type AdminResourceScopeRepository interface {
	GetAdminResourceScope(ctx context.Context, userID int64) (AdminResourceScope, error)
	GetAdminResourceScopesByUserIDs(ctx context.Context, userIDs []int64) (map[int64]AdminResourceScope, error)
	UpdateUserAdminAccess(ctx context.Context, userID int64, role string, permissions []string, scope AdminResourceScope, createdBy *int64) error
	BindAdminResource(ctx context.Context, userID int64, resourceType string, resourceID int64, createdBy *int64) error
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
	repo              OperatorPermissionRepository
	resourceScopeRepo AdminResourceScopeRepository
	userRepo          UserRepository
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
	resourceScopeRepo, _ := repo.(AdminResourceScopeRepository)
	return &PermissionService{repo: repo, resourceScopeRepo: resourceScopeRepo, userRepo: userRepo}
}

func (s *PermissionService) GetAdminResourceScope(ctx context.Context, userID int64) (AdminResourceScope, error) {
	if userID <= 0 || s == nil || s.resourceScopeRepo == nil {
		return UnrestrictedAdminResourceScope(), nil
	}
	scope, err := s.resourceScopeRepo.GetAdminResourceScope(ctx, userID)
	if err != nil {
		return AdminResourceScope{}, err
	}
	return NormalizeAdminResourceScope(RoleAdmin, scope)
}

func (s *PermissionService) GetAdminResourceScopesByUserIDs(ctx context.Context, userIDs []int64) (map[int64]AdminResourceScope, error) {
	out := make(map[int64]AdminResourceScope, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	if s == nil || s.resourceScopeRepo == nil {
		for _, userID := range userIDs {
			out[userID] = UnrestrictedAdminResourceScope()
		}
		return out, nil
	}
	loaded, err := s.resourceScopeRepo.GetAdminResourceScopesByUserIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for _, userID := range userIDs {
		scope, ok := loaded[userID]
		if !ok {
			scope = UnrestrictedAdminResourceScope()
		}
		normalized, normalizeErr := NormalizeAdminResourceScope(RoleAdmin, scope)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		out[userID] = normalized
	}
	return out, nil
}

func (s *PermissionService) UpdateUserAdminAccess(ctx context.Context, userID int64, role string, permissions []string, scope AdminResourceScope, createdBy *int64) error {
	if s == nil || s.resourceScopeRepo == nil {
		return ErrAdminResourceScopeUnavailable
	}
	return s.resourceScopeRepo.UpdateUserAdminAccess(ctx, userID, role, permissions, scope, createdBy)
}

func (s *PermissionService) BindAdminResource(ctx context.Context, userID int64, resourceType string, resourceID int64, createdBy *int64) error {
	if userID <= 0 || resourceID <= 0 {
		return ErrAdminResourceScopeInvalid
	}
	switch resourceType {
	case AdminResourceGroup, AdminResourceAccount, AdminResourceProxy, AdminResourceSubscription:
	default:
		return ErrAdminResourceScopeInvalid
	}
	if s == nil || s.resourceScopeRepo == nil {
		return ErrAdminResourceScopeUnavailable
	}
	return s.resourceScopeRepo.BindAdminResource(ctx, userID, resourceType, resourceID, createdBy)
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
