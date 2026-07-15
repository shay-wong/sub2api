//go:build unit

package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type accountRepoStubForBulkUpdate struct {
	accountRepoStub
	bulkUpdateErr        error
	bulkUpdateIDs        []int64
	bindGroupErrByID     map[int64]error
	bindGroupsCalls      []int64
	boundGroupsByID      map[int64][]int64
	getByIDsAccounts     []*Account
	getByIDsErr          error
	getByIDsCalled       bool
	getByIDsIDs          []int64
	getByIDAccounts      map[int64]*Account
	getByIDErrByID       map[int64]error
	getByIDCalled        []int64
	listByGroupData      map[int64][]Account
	listByGroupErr       map[int64]error
	listData             []Account
	listResult           *pagination.PaginationResult
	listErr              error
	listCalled           bool
	listGroupScopeCalled bool
	lastListParams       pagination.PaginationParams
	lastListFilters      struct {
		platform    string
		accountType string
		status      string
		search      string
		groupID     int64
		privacyMode string
	}
	lastGroupScopeFilters struct {
		platform    string
		accountType string
		status      string
		search      string
		groupIDs    []int64
		privacyMode string
	}
}

type proxyRepoStubForAccountProxyScope struct {
	proxyRepoStub
	getErr error
	gotIDs []int64
}

func (s *proxyRepoStubForAccountProxyScope) GetByID(_ context.Context, id int64) (*Proxy, error) {
	s.gotIDs = append(s.gotIDs, id)
	if s.getErr != nil {
		return nil, s.getErr
	}
	return &Proxy{ID: id, Name: "project-proxy", Status: StatusActive}, nil
}

func (s *accountRepoStubForBulkUpdate) BulkUpdate(_ context.Context, ids []int64, _ AccountBulkUpdate) (int64, error) {
	s.bulkUpdateIDs = append([]int64{}, ids...)
	if s.bulkUpdateErr != nil {
		return 0, s.bulkUpdateErr
	}
	return int64(len(ids)), nil
}

func (s *accountRepoStubForBulkUpdate) BindGroups(_ context.Context, accountID int64, groupIDs []int64) error {
	s.bindGroupsCalls = append(s.bindGroupsCalls, accountID)
	if s.boundGroupsByID == nil {
		s.boundGroupsByID = map[int64][]int64{}
	}
	s.boundGroupsByID[accountID] = append([]int64(nil), groupIDs...)
	if err, ok := s.bindGroupErrByID[accountID]; ok {
		return err
	}
	return nil
}

func (s *accountRepoStubForBulkUpdate) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	s.getByIDsCalled = true
	s.getByIDsIDs = append([]int64{}, ids...)
	if s.getByIDsErr != nil {
		return nil, s.getByIDsErr
	}
	return s.getByIDsAccounts, nil
}

func (s *accountRepoStubForBulkUpdate) GetByID(_ context.Context, id int64) (*Account, error) {
	s.getByIDCalled = append(s.getByIDCalled, id)
	if err, ok := s.getByIDErrByID[id]; ok {
		return nil, err
	}
	if account, ok := s.getByIDAccounts[id]; ok {
		return account, nil
	}
	return nil, errors.New("account not found")
}

func (s *accountRepoStubForBulkUpdate) ListByGroup(_ context.Context, groupID int64) ([]Account, error) {
	if err, ok := s.listByGroupErr[groupID]; ok {
		return nil, err
	}
	if rows, ok := s.listByGroupData[groupID]; ok {
		return rows, nil
	}
	return nil, nil
}

func (s *accountRepoStubForBulkUpdate) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error) {
	return nil, nil
}

func (s *accountRepoStubForBulkUpdate) ListWithFilters(_ context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupID int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	s.listCalled = true
	s.lastListParams = params
	s.lastListFilters.platform = platform
	s.lastListFilters.accountType = accountType
	s.lastListFilters.status = status
	s.lastListFilters.search = search
	s.lastListFilters.groupID = groupID
	s.lastListFilters.privacyMode = privacyMode
	if s.listErr != nil {
		return nil, nil, s.listErr
	}
	if s.listResult != nil {
		return s.listData, s.listResult, nil
	}
	return s.listData, &pagination.PaginationResult{Total: int64(len(s.listData))}, nil
}

func (s *accountRepoStubForBulkUpdate) ListWithGroupScope(_ context.Context, params pagination.PaginationParams, platform, accountType, status, search string, groupIDs []int64, privacyMode string) ([]Account, *pagination.PaginationResult, error) {
	s.listGroupScopeCalled = true
	s.lastListParams = params
	s.lastGroupScopeFilters.platform = platform
	s.lastGroupScopeFilters.accountType = accountType
	s.lastGroupScopeFilters.status = status
	s.lastGroupScopeFilters.search = search
	s.lastGroupScopeFilters.groupIDs = append([]int64(nil), groupIDs...)
	s.lastGroupScopeFilters.privacyMode = privacyMode
	if s.listErr != nil {
		return nil, nil, s.listErr
	}
	if s.listResult != nil {
		return s.listData, s.listResult, nil
	}
	return s.listData, &pagination.PaginationResult{Total: int64(len(s.listData))}, nil
}

// TestAdminService_BulkUpdateAccounts_AllSuccessIDs 验证批量更新成功时返回 success_ids/failed_ids。
func TestAdminService_BulkUpdateAccounts_AllSuccessIDs(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	svc := &adminServiceImpl{accountRepo: repo}

	schedulable := true
	input := &BulkUpdateAccountsInput{
		AccountIDs:  []int64{1, 2, 3},
		Schedulable: &schedulable,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 3, result.Success)
	require.Equal(t, 0, result.Failed)
	require.ElementsMatch(t, []int64{1, 2, 3}, result.SuccessIDs)
	require.Empty(t, result.FailedIDs)
	require.Len(t, result.Results, 3)
}

// TestAdminService_BulkUpdateAccounts_PartialFailureIDs 验证部分失败时 success_ids/failed_ids 正确。
func TestAdminService_BulkUpdateAccounts_PartialFailureIDs(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		bindGroupErrByID: map[int64]error{
			2: errors.New("bind failed"),
		},
	}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 10, Name: "g10"}},
	}

	groupIDs := []int64{10}
	schedulable := false
	input := &BulkUpdateAccountsInput{
		AccountIDs:            []int64{1, 2, 3},
		GroupIDs:              &groupIDs,
		Schedulable:           &schedulable,
		SkipMixedChannelCheck: true,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 2, result.Success)
	require.Equal(t, 1, result.Failed)
	require.ElementsMatch(t, []int64{1, 3}, result.SuccessIDs)
	require.ElementsMatch(t, []int64{2}, result.FailedIDs)
	require.Len(t, result.Results, 3)
}

func TestAdminService_BulkUpdateAccounts_NilGroupRepoReturnsError(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	svc := &adminServiceImpl{accountRepo: repo}

	groupIDs := []int64{10}
	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		GroupIDs:   &groupIDs,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "group repository not configured")
}

func TestAdminServiceCreateAccount_ProjectContextRejectsOutOfScopeGroupsBeforeCreate(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	groupRepo := &groupRepoStubForAdmin{getErr: ErrGroupNotFound}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   groupRepo,
	}

	_, err := svc.CreateAccount(WithProjectID(context.Background(), 7), &CreateAccountInput{
		Name:                  "blocked",
		Platform:              PlatformAnthropic,
		Type:                  AccountTypeOAuth,
		GroupIDs:              []int64{99},
		SkipMixedChannelCheck: true,
	})

	require.ErrorIs(t, err, ErrGroupNotFound)
	require.Empty(t, repo.bulkUpdateIDs)
	require.Empty(t, repo.bindGroupsCalls)
}

func TestAdminServiceCreateAccount_ProjectContextRejectsOutOfScopeProxyBeforeCreate(t *testing.T) {
	proxyID := int64(99)
	repo := &accountRepoStubForBulkUpdate{}
	proxyRepo := &proxyRepoStubForAccountProxyScope{getErr: ErrProxyNotFound}
	svc := &adminServiceImpl{
		accountRepo: repo,
		proxyRepo:   proxyRepo,
	}

	_, err := svc.CreateAccount(WithProjectID(context.Background(), 7), &CreateAccountInput{
		Name:                  "blocked",
		Platform:              PlatformAnthropic,
		Type:                  AccountTypeOAuth,
		ProxyID:               &proxyID,
		SkipMixedChannelCheck: true,
		SkipDefaultGroupBind:  true,
	})

	require.ErrorIs(t, err, ErrProxyNotFound)
	require.Equal(t, []int64{99}, proxyRepo.gotIDs)
	require.Empty(t, repo.bulkUpdateIDs)
	require.Empty(t, repo.bindGroupsCalls)
}

func TestAdminServiceBulkUpdateAccounts_ProjectContextRejectsOutOfScopeProxyBeforeWrite(t *testing.T) {
	proxyID := int64(99)
	repo := &accountRepoStubForBulkUpdate{}
	proxyRepo := &proxyRepoStubForAccountProxyScope{getErr: ErrProxyNotFound}
	svc := &adminServiceImpl{
		accountRepo: repo,
		proxyRepo:   proxyRepo,
	}

	result, err := svc.BulkUpdateAccounts(WithProjectID(context.Background(), 7), &BulkUpdateAccountsInput{
		AccountIDs: []int64{1, 2},
		ProxyID:    &proxyID,
	})

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrProxyNotFound)
	require.Equal(t, []int64{99}, proxyRepo.gotIDs)
	require.Empty(t, repo.bulkUpdateIDs)
}

func TestAdminServiceBulkUpdateAccounts_ProjectContextRequiresVisibleExplicitAccounts(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		getByIDsAccounts: []*Account{
			{ID: 1, Platform: PlatformAnthropic},
		},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	schedulable := true
	result, err := svc.BulkUpdateAccounts(WithProjectID(context.Background(), 7), &BulkUpdateAccountsInput{
		AccountIDs:  []int64{1, 2},
		Schedulable: &schedulable,
	})

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrAccountNotFound)
	require.True(t, repo.getByIDsCalled)
	require.Equal(t, []int64{1, 2}, repo.getByIDsIDs)
	require.Empty(t, repo.bulkUpdateIDs)
	require.Empty(t, repo.bindGroupsCalls)
}

func TestAdminServiceCreateAccount_ProjectAdminRejectsCustomGrokOAuthBaseURL(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{}
	svc := &adminServiceImpl{accountRepo: repo}
	ctx := WithAdminRole(WithProjectID(context.Background(), 7), RoleAdmin)

	_, err := svc.CreateAccount(ctx, &CreateAccountInput{
		Name:                 "blocked",
		Platform:             PlatformGrok,
		Type:                 AccountTypeOAuth,
		Credentials:          map[string]any{"base_url": "https://attacker.example/v1"},
		SkipDefaultGroupBind: true,
	})

	require.ErrorIs(t, err, ErrProjectAdminGrokOAuthCustomBaseURL)
}

func TestAdminServiceUpdateAccount_ProjectAdminRejectsCustomGrokOAuthBaseURL(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{getByIDAccounts: map[int64]*Account{
		1: {ID: 1, Platform: PlatformGrok, Type: AccountTypeOAuth, Credentials: map[string]any{"base_url": "https://api.x.ai/v1"}},
	}}
	svc := &adminServiceImpl{accountRepo: repo}
	ctx := WithAdminRole(WithProjectID(context.Background(), 7), RoleAdmin)

	_, err := svc.UpdateAccount(ctx, 1, &UpdateAccountInput{
		Credentials: map[string]any{"base_url": "https://attacker.example/v1"},
	})

	require.ErrorIs(t, err, ErrProjectAdminGrokOAuthCustomBaseURL)
}

func TestAdminServiceBulkUpdateAccounts_ProjectAdminRejectsCustomGrokOAuthBaseURLBeforeWrite(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{getByIDsAccounts: []*Account{
		{ID: 1, Platform: PlatformGrok, Type: AccountTypeOAuth, Credentials: map[string]any{"base_url": "https://api.x.ai/v1"}},
	}}
	svc := &adminServiceImpl{accountRepo: repo}
	ctx := WithAdminRole(WithProjectID(context.Background(), 7), RoleAdmin)

	result, err := svc.BulkUpdateAccounts(ctx, &BulkUpdateAccountsInput{
		AccountIDs:  []int64{1},
		Credentials: map[string]any{"base_url": "https://attacker.example/v1"},
	})

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrProjectAdminGrokOAuthCustomBaseURL)
	require.Empty(t, repo.bulkUpdateIDs)
}

func TestProjectAdminGrokOAuthBaseURLPolicyAllowsOfficialHosts(t *testing.T) {
	ctx := WithAdminRole(WithProjectID(context.Background(), 7), RoleAdmin)
	require.NoError(t, ensureProjectAdminGrokOAuthBaseURL(ctx, PlatformGrok, AccountTypeOAuth, "https://api.x.ai/v1"))
	require.NoError(t, ensureProjectAdminGrokOAuthBaseURL(ctx, PlatformGrok, AccountTypeOAuth, "https://cli-chat-proxy.grok.com/v1"))
}

// TestAdminService_BulkUpdateAccounts_MixedChannelPreCheckBlocksOnExistingConflict verifies
// that the global pre-check detects a conflict with existing group members and returns an
// error before any DB write is performed.
func TestAdminService_BulkUpdateAccounts_MixedChannelPreCheckBlocksOnExistingConflict(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		getByIDsAccounts: []*Account{
			{ID: 1, Platform: PlatformAntigravity},
		},
		// Group 10 already contains an Anthropic account.
		listByGroupData: map[int64][]Account{
			10: {{ID: 99, Platform: PlatformAnthropic}},
		},
	}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 10, Name: "target-group"}},
	}

	groupIDs := []int64{10}
	input := &BulkUpdateAccountsInput{
		AccountIDs: []int64{1},
		GroupIDs:   &groupIDs,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "mixed channel")
	// No BindGroups should have been called since the check runs before any write.
	require.Empty(t, repo.bindGroupsCalls)
}

func TestAdminServiceBulkUpdateAccounts_ResolvesIDsFromFilters(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		listData: []Account{
			{ID: 7},
			{ID: 11},
		},
		listResult: &pagination.PaginationResult{Total: 2},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	schedulable := true
	input := &BulkUpdateAccountsInput{
		Schedulable: &schedulable,
	}

	filtersField := reflect.ValueOf(input).Elem().FieldByName("Filters")
	require.True(t, filtersField.IsValid(), "BulkUpdateAccountsInput should expose Filters for filter-target bulk update")
	require.Equal(t, reflect.Ptr, filtersField.Kind(), "BulkUpdateAccountsInput.Filters should be a pointer field")

	filtersValue := reflect.New(filtersField.Type().Elem())
	filtersValue.Elem().FieldByName("Platform").SetString(PlatformOpenAI)
	filtersValue.Elem().FieldByName("Type").SetString(AccountTypeOAuth)
	filtersValue.Elem().FieldByName("Status").SetString(StatusActive)
	filtersValue.Elem().FieldByName("Group").SetString("12")
	filtersValue.Elem().FieldByName("PrivacyMode").SetString(PrivacyModeCFBlocked)
	filtersValue.Elem().FieldByName("Search").SetString("bulk-target")
	filtersField.Set(filtersValue)

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.NoError(t, err)
	require.True(t, repo.listCalled, "expected filter-target bulk update to resolve matching IDs via account list filters")
	require.Equal(t, PlatformOpenAI, repo.lastListFilters.platform)
	require.Equal(t, AccountTypeOAuth, repo.lastListFilters.accountType)
	require.Equal(t, StatusActive, repo.lastListFilters.status)
	require.Equal(t, "bulk-target", repo.lastListFilters.search)
	require.Equal(t, int64(12), repo.lastListFilters.groupID)
	require.Equal(t, PrivacyModeCFBlocked, repo.lastListFilters.privacyMode)
	require.Equal(t, []int64{7, 11}, repo.bulkUpdateIDs)
	require.Equal(t, 2, result.Success)
	require.Equal(t, 0, result.Failed)
	require.Equal(t, []int64{7, 11}, result.SuccessIDs)
}

func TestAdminServiceBulkUpdateAccounts_ResolvesFilterTargetsWithinGroupScope(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		listData: []Account{
			{ID: 7},
			{ID: 11},
		},
		listResult: &pagination.PaginationResult{Total: 2},
	}
	svc := &adminServiceImpl{accountRepo: repo}

	schedulable := true
	input := &BulkUpdateAccountsInput{
		Filters: &BulkUpdateAccountFilters{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Search:      "bulk-target",
			PrivacyMode: PrivacyModeCFBlocked,
		},
		GroupScopeIDs: []int64{10, 20},
		Schedulable:   &schedulable,
	}

	result, err := svc.BulkUpdateAccounts(context.Background(), input)
	require.NoError(t, err)
	require.True(t, repo.listGroupScopeCalled)
	require.False(t, repo.listCalled)
	require.Equal(t, PlatformOpenAI, repo.lastGroupScopeFilters.platform)
	require.Equal(t, AccountTypeOAuth, repo.lastGroupScopeFilters.accountType)
	require.Equal(t, StatusActive, repo.lastGroupScopeFilters.status)
	require.Equal(t, "bulk-target", repo.lastGroupScopeFilters.search)
	require.Equal(t, []int64{10, 20}, repo.lastGroupScopeFilters.groupIDs)
	require.Equal(t, PrivacyModeCFBlocked, repo.lastGroupScopeFilters.privacyMode)
	require.Equal(t, []int64{7, 11}, repo.bulkUpdateIDs)
	require.Equal(t, 2, result.Success)
	require.Equal(t, []int64{7, 11}, result.SuccessIDs)
}

func TestAdminServiceBulkUpdateAccounts_WithGroupScopePreservesOutOfScopeGroupsPerAccount(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		getByIDsAccounts: []*Account{
			{ID: 1, Platform: PlatformAnthropic, GroupIDs: []int64{10, 30}},
			{ID: 2, Platform: PlatformAnthropic, AccountGroups: []AccountGroup{{GroupID: 20}, {GroupID: 40}}},
		},
	}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 10, Name: "group"}},
	}

	nextVisibleGroups := []int64{10}
	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs:            []int64{1, 2},
		GroupIDs:              &nextVisibleGroups,
		GroupScopeIDs:         []int64{10, 20},
		SkipMixedChannelCheck: true,
	})

	require.NoError(t, err)
	require.Equal(t, 2, result.Success)
	require.Equal(t, []int64{10, 30}, repo.boundGroupsByID[1])
	require.Equal(t, []int64{10, 40}, repo.boundGroupsByID[2])
}

func TestAdminServiceBulkUpdateAccounts_WithGroupScopeRequiresLoadedAccounts(t *testing.T) {
	repo := &accountRepoStubForBulkUpdate{
		getByIDsAccounts: []*Account{
			{ID: 1, Platform: PlatformAnthropic, GroupIDs: []int64{10, 30}},
		},
	}
	svc := &adminServiceImpl{
		accountRepo: repo,
		groupRepo:   &groupRepoStubForAdmin{getByID: &Group{ID: 10, Name: "group"}},
	}

	nextVisibleGroups := []int64{10}
	result, err := svc.BulkUpdateAccounts(context.Background(), &BulkUpdateAccountsInput{
		AccountIDs:            []int64{1, 2},
		GroupIDs:              &nextVisibleGroups,
		GroupScopeIDs:         []int64{10},
		SkipMixedChannelCheck: true,
	})

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrAccountNotFound)
	require.Empty(t, repo.bindGroupsCalls)
}
