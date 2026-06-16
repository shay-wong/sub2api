package service

import (
	"context"
	"fmt"
	"sort"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	AdminPermissionDashboardRead    = "admin.dashboard.read"
	AdminPermissionOpsRead          = "admin.ops.read"
	AdminPermissionAccountsRead     = "admin.accounts.read"
	AdminPermissionAccountsWrite    = "admin.accounts.write"
	AdminPermissionPermissionsWrite = "admin.permissions.write"
)

var (
	ErrPermissionUserNotFound       = infraerrors.NotFound("PERMISSION_USER_NOT_FOUND", "permission subject not found")
	ErrPermissionInvalidRole        = infraerrors.BadRequest("PERMISSION_INVALID_ROLE", "role must be user or operator")
	ErrPermissionCannotChangeAdmin  = infraerrors.Forbidden("PERMISSION_ADMIN_IMMUTABLE", "admin role cannot be changed here")
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
	repo      OperatorPermissionRepository
	userRepo  UserRepository
	groupRepo GroupRepository
}

func NewPermissionService(repo OperatorPermissionRepository, userRepo UserRepository, groupRepo GroupRepository) *PermissionService {
	return &PermissionService{repo: repo, userRepo: userRepo, groupRepo: groupRepo}
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

func (s *PermissionService) SetOperatorPermissions(ctx context.Context, userID int64, role string, groupIDs []int64, createdBy *int64) (*OperatorPermissionSubject, error) {
	if s.userRepo == nil || s.repo == nil {
		return nil, infraerrors.InternalServer("PERMISSION_SERVICE_UNAVAILABLE", "permission service unavailable")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrPermissionUserNotFound
	}
	if user.Role == RoleAdmin {
		return nil, ErrPermissionCannotChangeAdmin
	}

	switch role {
	case RoleUser, RoleOperator:
	default:
		return nil, ErrPermissionInvalidRole
	}

	groupIDs = normalizeInt64Set(groupIDs)
	if role == RoleOperator {
		if err := s.validateGroupsExist(ctx, groupIDs); err != nil {
			return nil, err
		}
	}

	user.Role = role
	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("update permission subject role: %w", err)
	}

	if role == RoleOperator {
		if err := s.repo.SetOperatorGroupIDs(ctx, userID, groupIDs, createdBy); err != nil {
			return nil, err
		}
	} else if err := s.repo.ClearOperatorGroupIDs(ctx, userID); err != nil {
		return nil, err
	}

	return &OperatorPermissionSubject{
		ID:       user.ID,
		Email:    user.Email,
		Username: user.Username,
		Role:     user.Role,
		Status:   user.Status,
		GroupIDs: groupIDs,
	}, nil
}

func (s *PermissionService) validateGroupsExist(ctx context.Context, groupIDs []int64) error {
	if len(groupIDs) == 0 {
		return nil
	}
	if s.groupRepo == nil {
		return ErrPermissionInvalidGroupScope
	}
	batch, ok := s.groupRepo.(interface {
		ExistsByIDs(context.Context, []int64) (map[int64]bool, error)
	})
	if !ok {
		for _, id := range groupIDs {
			if _, err := s.groupRepo.GetByID(ctx, id); err != nil {
				return ErrPermissionInvalidGroupScope
			}
		}
		return nil
	}
	exists, err := batch.ExistsByIDs(ctx, groupIDs)
	if err != nil {
		return err
	}
	for _, id := range groupIDs {
		if !exists[id] {
			return ErrPermissionInvalidGroupScope
		}
	}
	return nil
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
