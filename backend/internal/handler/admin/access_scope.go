package admin

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type adminAccessScope struct {
	UserID                     int64
	Unrestricted               bool
	ProtectManagedAccountState bool
	GroupIDs                   []int64
	AccountIDs                 []int64
	ProxyIDs                   []int64
	SubscriptionIDs            []int64
	groupSet                   map[int64]struct{}
	accountSet                 map[int64]struct{}
	proxySet                   map[int64]struct{}
	subscriptionSet            map[int64]struct{}
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
		subject, hasSubject := middleware.GetAuthSubjectFromContext(c)
		if permissionService == nil || !hasSubject || subject.UserID <= 0 {
			return &adminAccessScope{Unrestricted: true, ProtectManagedAccountState: true}, nil
		}
		resourceScope, err := permissionService.GetAdminResourceScope(c.Request.Context(), subject.UserID)
		if err != nil {
			return nil, err
		}
		if resourceScope.Mode == service.AdminResourceScopeAll {
			return &adminAccessScope{UserID: subject.UserID, Unrestricted: true, ProtectManagedAccountState: true}, nil
		}
		c.Request = c.Request.WithContext(service.WithRestrictedAdminResourceActor(c.Request.Context(), subject.UserID))
		return &adminAccessScope{
			UserID:                     subject.UserID,
			ProtectManagedAccountState: true,
			GroupIDs:                   resourceScope.GroupIDs,
			AccountIDs:                 resourceScope.AccountIDs,
			ProxyIDs:                   resourceScope.ProxyIDs,
			SubscriptionIDs:            resourceScope.SubscriptionIDs,
			groupSet:                   int64Set(resourceScope.GroupIDs),
			accountSet:                 int64Set(resourceScope.AccountIDs),
			proxySet:                   int64Set(resourceScope.ProxyIDs),
			subscriptionSet:            int64Set(resourceScope.SubscriptionIDs),
		}, nil
	}
	if service.RoleIsOperator(role) {
		return nil, service.ErrLegacyOperatorRoleDisabled
	}
	return nil, errors.Forbidden("FORBIDDEN", "admin console access required")
}

func int64Set(ids []int64) map[int64]struct{} {
	out := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id > 0 {
			out[id] = struct{}{}
		}
	}
	return out
}

func (s *adminAccessScope) isScoped() bool {
	return s != nil && !s.Unrestricted
}

func (s *adminAccessScope) containsGroup(id int64) bool {
	if s == nil || s.Unrestricted {
		return true
	}
	_, ok := s.groupSet[id]
	return ok
}

func (s *adminAccessScope) ensureGroup(id int64) error {
	if s == nil || s.Unrestricted {
		return nil
	}
	if id <= 0 {
		return service.ErrAdminGroupScopeForbidden
	}
	if !s.containsGroup(id) {
		return service.ErrAdminGroupScopeForbidden
	}
	return nil
}

func (s *adminAccessScope) ensureGroups(groupIDs []int64, requireNonEmpty bool) error {
	if s == nil || s.Unrestricted {
		return nil
	}
	if len(groupIDs) == 0 {
		if requireNonEmpty {
			return service.ErrAdminAccountScopeRequired
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
	if s == nil || s.Unrestricted || proxyID == nil {
		return nil
	}
	if *proxyID == 0 || s.containsProxy(*proxyID) {
		return nil
	}
	return service.ErrAdminProxyScopeForbidden
}

func (s *adminAccessScope) ensureOAuthProxyUse(c *gin.Context, adminService accountScopedProxyLookup, accountID *int64, proxyID *int64) error {
	if s == nil || s.Unrestricted || proxyID == nil || *proxyID == 0 {
		return nil
	}
	return s.ensureProxyMutation(proxyID)
}

func (s *adminAccessScope) accountVisible(account *service.Account) bool {
	if s == nil || s.Unrestricted {
		return true
	}
	if account == nil {
		return false
	}
	_, ok := s.accountSet[account.ID]
	return ok
}

func (s *adminAccessScope) containsProxy(id int64) bool {
	if s == nil || s.Unrestricted {
		return true
	}
	_, ok := s.proxySet[id]
	return ok
}

func (s *adminAccessScope) ensureProxy(id int64) error {
	if id <= 0 || !s.containsProxy(id) {
		return service.ErrAdminProxyScopeForbidden
	}
	return nil
}

func (s *adminAccessScope) containsSubscription(id int64) bool {
	if s == nil || s.Unrestricted {
		return true
	}
	_, ok := s.subscriptionSet[id]
	return ok
}

func (s *adminAccessScope) ensureSubscription(id int64) error {
	if id <= 0 || !s.containsSubscription(id) {
		return service.ErrAdminSubscriptionScopeForbidden
	}
	return nil
}

func (s *adminAccessScope) bindCreatedResource(ctx context.Context, permissionService *service.PermissionService, resourceType string, resourceID int64) error {
	if s == nil || s.Unrestricted || permissionService == nil {
		return nil
	}
	actorID := s.UserID
	return permissionService.BindAdminResource(ctx, s.UserID, resourceType, resourceID, &actorID)
}

func (s *adminAccessScope) accountForResponse(account *service.Account) *service.Account {
	if account == nil || s == nil || !s.ProtectManagedAccountState {
		return account
	}
	out := *account
	out.Extra = copyAccountExtraWithoutUpstreamBillingProbe(account.Extra)

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

func copyAccountExtraWithoutUpstreamBillingProbe(extra map[string]any) map[string]any {
	if extra == nil {
		return nil
	}
	out := make(map[string]any, len(extra))
	for key, value := range extra {
		if key == service.UpstreamBillingProbeEnabledExtraKey ||
			key == service.UpstreamBillingRateSyncEnabledExtraKey ||
			key == service.UpstreamBillingProbeExtraKey {
			continue
		}
		out[key] = value
	}
	return out
}

func (s *adminAccessScope) ensureUpstreamBillingProbeMutation(extra map[string]any, settings ...*bool) error {
	if s == nil || !s.ProtectManagedAccountState {
		return nil
	}
	for _, setting := range settings {
		if setting != nil {
			return errors.Forbidden("UPSTREAM_BILLING_PROBE_FORBIDDEN", "upstream billing probe settings require super admin access")
		}
	}
	for _, key := range []string{
		service.UpstreamBillingProbeEnabledExtraKey,
		service.UpstreamBillingRateSyncEnabledExtraKey,
		service.UpstreamBillingProbeExtraKey,
	} {
		if _, ok := extra[key]; ok {
			return errors.Forbidden("UPSTREAM_BILLING_PROBE_FORBIDDEN", "upstream billing probe settings require super admin access")
		}
	}
	return nil
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
		return service.ErrAdminGroupScopeForbidden
	}
	return s.ensureGroup(*detail.GroupID)
}

func (s *adminAccessScope) ensureOpsAlertEventVisible(event *service.OpsAlertEvent) error {
	if s == nil || !s.isScoped() {
		return nil
	}
	if event == nil || event.Dimensions == nil {
		return service.ErrAdminGroupScopeForbidden
	}
	groupID, ok := eventDimensionGroupID(event.Dimensions["group_id"])
	if !ok || groupID <= 0 {
		return service.ErrAdminGroupScopeForbidden
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
