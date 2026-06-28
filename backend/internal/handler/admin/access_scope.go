package admin

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type adminAccessScope struct {
	Unrestricted  bool
	ProjectScoped bool
	ProjectID     int64
	GroupIDs      []int64
	groupSet      map[int64]struct{}
}

type accountScopedProxyLookup interface {
	GetAccount(context.Context, int64) (*service.Account, error)
	GetProxy(context.Context, int64) (*service.Proxy, error)
}

func resolveAdminAccessScope(c *gin.Context, permissionService *service.PermissionService) (*adminAccessScope, error) {
	role, hasRole := middleware.GetUserRoleFromContext(c)
	if !hasRole && permissionService == nil {
		return &adminAccessScope{Unrestricted: true}, nil
	}
	if service.RoleIsSuperAdmin(role) {
		return &adminAccessScope{Unrestricted: true}, nil
	}
	if role == service.RoleAdmin {
		if projectID, ok := service.ProjectIDFromContext(c.Request.Context()); ok {
			return &adminAccessScope{ProjectScoped: true, ProjectID: projectID}, nil
		}
		return &adminAccessScope{Unrestricted: true}, nil
	}
	if service.RoleIsOperator(role) {
		return nil, service.ErrLegacyOperatorRoleDisabled
	}
	return nil, errors.Forbidden("FORBIDDEN", "admin console access required")
}

func (s *adminAccessScope) isScoped() bool {
	return s != nil && !s.Unrestricted && !s.ProjectScoped
}

func (s *adminAccessScope) containsGroup(id int64) bool {
	if s == nil || s.Unrestricted || s.ProjectScoped {
		return true
	}
	_, ok := s.groupSet[id]
	return ok
}

func (s *adminAccessScope) ensureGroup(id int64) error {
	if s == nil || s.Unrestricted || s.ProjectScoped {
		return nil
	}
	if id <= 0 {
		return service.ErrOperatorScopeForbidden
	}
	if !s.containsGroup(id) {
		return service.ErrOperatorScopeForbidden
	}
	return nil
}

func (s *adminAccessScope) ensureGroups(groupIDs []int64, requireNonEmpty bool) error {
	if s == nil || s.Unrestricted || s.ProjectScoped {
		return nil
	}
	if len(groupIDs) == 0 {
		if requireNonEmpty {
			return service.ErrOperatorAccountScopeRequired
		}
		return nil
	}
	for _, id := range groupIDs {
		if err := s.ensureGroup(id); err != nil {
			return err
		}
	}
	return nil
}

func (s *adminAccessScope) ensureProxyMutation(proxyID *int64) error {
	if s == nil || s.Unrestricted || s.ProjectScoped || proxyID == nil {
		return nil
	}
	return errors.Forbidden("OPERATOR_PROXY_FORBIDDEN", "operator cannot assign account proxy")
}

func (s *adminAccessScope) ensureOAuthProxyUse(c *gin.Context, adminService accountScopedProxyLookup, accountID *int64, proxyID *int64) error {
	if s == nil || s.Unrestricted || proxyID == nil || *proxyID == 0 {
		return nil
	}
	if s.ProjectScoped {
		if adminService == nil {
			return errors.Forbidden("PROXY_NOT_FOUND", "proxy not found")
		}
		_, err := adminService.GetProxy(c.Request.Context(), *proxyID)
		return err
	}
	if accountID == nil || *accountID <= 0 || adminService == nil {
		return errors.Forbidden("OPERATOR_PROXY_FORBIDDEN", "operator cannot assign account proxy")
	}
	account, err := adminService.GetAccount(c.Request.Context(), *accountID)
	if err != nil {
		return err
	}
	if !s.accountVisible(account) {
		return service.ErrOperatorAccountForbidden
	}
	if account.ProxyID == nil || *account.ProxyID != *proxyID {
		return errors.Forbidden("OPERATOR_PROXY_FORBIDDEN", "operator cannot assign account proxy")
	}
	return nil
}

func (s *adminAccessScope) accountVisible(account *service.Account) bool {
	if s == nil || s.Unrestricted {
		return true
	}
	if account == nil {
		return false
	}
	if s.ProjectScoped {
		return true
	}
	for _, id := range account.GroupIDs {
		if s.containsGroup(id) {
			return true
		}
	}
	for _, group := range account.Groups {
		if group != nil && s.containsGroup(group.ID) {
			return true
		}
	}
	for _, accountGroup := range account.AccountGroups {
		if s.containsGroup(accountGroup.GroupID) {
			return true
		}
	}
	return false
}

func (s *adminAccessScope) accountForResponse(account *service.Account) *service.Account {
	if account == nil || s == nil || s.Unrestricted || s.ProjectScoped {
		return account
	}
	out := *account

	if len(account.GroupIDs) > 0 {
		out.GroupIDs = make([]int64, 0, len(account.GroupIDs))
		for _, id := range account.GroupIDs {
			if s.containsGroup(id) {
				out.GroupIDs = append(out.GroupIDs, id)
			}
		}
	}

	if len(account.Groups) > 0 {
		out.Groups = make([]*service.Group, 0, len(account.Groups))
		for _, group := range account.Groups {
			if group != nil && s.containsGroup(group.ID) {
				out.Groups = append(out.Groups, group)
			}
		}
	}

	if len(account.AccountGroups) > 0 {
		out.AccountGroups = make([]service.AccountGroup, 0, len(account.AccountGroups))
		for _, accountGroup := range account.AccountGroups {
			if s.containsGroup(accountGroup.GroupID) {
				out.AccountGroups = append(out.AccountGroups, accountGroup)
			}
		}
	}

	return &out
}

func (s *adminAccessScope) dashboardGroupIDs(groupID int64) ([]int64, error) {
	if s == nil || !s.isScoped() {
		if groupID > 0 {
			return []int64{groupID}, nil
		}
		return nil, nil
	}
	if groupID > 0 {
		if err := s.ensureGroup(groupID); err != nil {
			return nil, err
		}
		return []int64{groupID}, nil
	}
	return s.GroupIDs, nil
}

func (s *adminAccessScope) applyOpsDashboardScope(filter *service.OpsDashboardFilter) error {
	if s == nil || !s.isScoped() || filter == nil {
		return nil
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		if err := s.ensureGroup(*filter.GroupID); err != nil {
			return err
		}
		filter.GroupIDs = nil
		filter.GroupScopeEmpty = false
		return nil
	}
	if len(s.GroupIDs) == 0 {
		filter.GroupIDs = []int64{}
		filter.GroupScopeEmpty = true
		return nil
	}
	filter.GroupIDs = append([]int64(nil), s.GroupIDs...)
	filter.GroupScopeEmpty = false
	return nil
}

func (s *adminAccessScope) applyOpsOpenAITokenStatsScope(filter *service.OpsOpenAITokenStatsFilter) error {
	if s == nil || !s.isScoped() || filter == nil {
		return nil
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		if err := s.ensureGroup(*filter.GroupID); err != nil {
			return err
		}
		filter.GroupIDs = nil
		filter.GroupScopeEmpty = false
		return nil
	}
	if len(s.GroupIDs) == 0 {
		filter.GroupIDs = []int64{}
		filter.GroupScopeEmpty = true
		return nil
	}
	filter.GroupIDs = append([]int64(nil), s.GroupIDs...)
	filter.GroupScopeEmpty = false
	return nil
}

func (s *adminAccessScope) applyOpsErrorLogScope(filter *service.OpsErrorLogFilter) error {
	if s == nil || !s.isScoped() || filter == nil {
		return nil
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		if err := s.ensureGroup(*filter.GroupID); err != nil {
			return err
		}
		filter.GroupIDs = nil
		filter.GroupScopeEmpty = false
		return nil
	}
	if len(s.GroupIDs) == 0 {
		filter.GroupIDs = []int64{}
		filter.GroupScopeEmpty = true
		return nil
	}
	filter.GroupIDs = append([]int64(nil), s.GroupIDs...)
	filter.GroupScopeEmpty = false
	return nil
}

func (s *adminAccessScope) applyOpsRequestDetailScope(filter *service.OpsRequestDetailFilter) error {
	if s == nil || !s.isScoped() || filter == nil {
		return nil
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		if err := s.ensureGroup(*filter.GroupID); err != nil {
			return err
		}
		filter.GroupIDs = nil
		filter.GroupScopeEmpty = false
		return nil
	}
	if len(s.GroupIDs) == 0 {
		filter.GroupIDs = []int64{}
		filter.GroupScopeEmpty = true
		return nil
	}
	filter.GroupIDs = append([]int64(nil), s.GroupIDs...)
	filter.GroupScopeEmpty = false
	return nil
}

func (s *adminAccessScope) applyOpsAlertEventScope(filter *service.OpsAlertEventFilter) error {
	if s == nil || !s.isScoped() || filter == nil {
		return nil
	}
	if filter.GroupID != nil && *filter.GroupID > 0 {
		if err := s.ensureGroup(*filter.GroupID); err != nil {
			return err
		}
		filter.GroupIDs = nil
		filter.GroupScopeEmpty = false
		return nil
	}
	if len(s.GroupIDs) == 0 {
		filter.GroupIDs = []int64{}
		filter.GroupScopeEmpty = true
		return nil
	}
	filter.GroupIDs = append([]int64(nil), s.GroupIDs...)
	filter.GroupScopeEmpty = false
	return nil
}

func (s *adminAccessScope) ensureOpsErrorLogVisible(detail *service.OpsErrorLogDetail) error {
	if s == nil || !s.isScoped() {
		return nil
	}
	if detail == nil || detail.GroupID == nil || *detail.GroupID <= 0 {
		return service.ErrOperatorScopeForbidden
	}
	return s.ensureGroup(*detail.GroupID)
}

func (s *adminAccessScope) ensureOpsAlertEventVisible(event *service.OpsAlertEvent) error {
	if s == nil || !s.isScoped() {
		return nil
	}
	if event == nil || event.Dimensions == nil {
		return service.ErrOperatorScopeForbidden
	}
	groupID, ok := eventDimensionGroupID(event.Dimensions["group_id"])
	if !ok || groupID <= 0 {
		return service.ErrOperatorScopeForbidden
	}
	return s.ensureGroup(groupID)
}

func eventDimensionGroupID(v any) (int64, bool) {
	switch x := v.(type) {
	case int64:
		return x, true
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case float64:
		i := int64(x)
		return i, x == float64(i)
	case float32:
		i := int64(x)
		return i, x == float32(i)
	case string:
		if x == "" {
			return 0, false
		}
		var id int64
		for _, ch := range x {
			if ch < '0' || ch > '9' {
				return 0, false
			}
			id = id*10 + int64(ch-'0')
		}
		return id, true
	default:
		return 0, false
	}
}
