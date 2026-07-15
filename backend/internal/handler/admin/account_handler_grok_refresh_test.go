//go:build unit

package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokRefreshAdminService struct {
	*stubAdminService
	updatedCredentials map[string]any
	getAccountErr      error
}

func (s *grokRefreshAdminService) GetAccount(ctx context.Context, id int64) (*service.Account, error) {
	if s.getAccountErr != nil {
		return nil, s.getAccountErr
	}
	return s.stubAdminService.GetAccount(ctx, id)
}

func TestRefreshGrokRejectsNonGrokAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := &grokRefreshAdminService{stubAdminService: newStubAdminService()}
	adminSvc.getAccountResult = &service.Account{
		ID: 99, Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive,
	}
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	var refreshCalls int
	handler.SetGrokOAuthAccountRefresher(func(context.Context, *service.Account) (*service.Account, error) {
		refreshCalls++
		return nil, nil
	})
	router := gin.New()
	router.POST("/api/v1/admin/grok/accounts/:id/refresh", handler.RefreshGrok)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/grok/accounts/99/refresh", nil)

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Zero(t, refreshCalls)
}

func TestRefreshGrokPreservesInternalGetAccountErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adminSvc := &grokRefreshAdminService{
		stubAdminService: newStubAdminService(),
		getAccountErr:    errors.New("database unavailable"),
	}
	handler := NewAccountHandler(adminSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	var refreshCalls int
	handler.SetGrokOAuthAccountRefresher(func(context.Context, *service.Account) (*service.Account, error) {
		refreshCalls++
		return nil, nil
	})
	router := gin.New()
	router.POST("/api/v1/admin/grok/accounts/:id/refresh", handler.RefreshGrok)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/grok/accounts/99/refresh", nil)

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Zero(t, refreshCalls)
}

func (s *grokRefreshAdminService) UpdateAccount(_ context.Context, id int64, input *service.UpdateAccountInput) (*service.Account, error) {
	s.updatedCredentials = input.Credentials
	return &service.Account{
		ID:          id,
		Platform:    service.PlatformGrok,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Credentials: input.Credentials,
	}, nil
}

func TestRefreshSingleAccountRoutesGrokThroughSharedRefreshAuthority(t *testing.T) {
	t.Parallel()

	adminSvc := &grokRefreshAdminService{stubAdminService: newStubAdminService()}
	handler := NewAccountHandler(
		adminSvc,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	account := &service.Account{
		ID:       4227,
		Platform: service.PlatformGrok,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "old-access",
			"refresh_token":      "old-refresh",
			"base_url":           "https://example.invalid/v1",
			"subscription_tier":  "SUPER_GROK",
			"entitlement_status": "ACTIVE",
		},
	}
	var calls int
	var refreshedAccount *service.Account
	handler.SetGrokOAuthAccountRefresher(func(_ context.Context, got *service.Account) (*service.Account, error) {
		calls++
		refreshedAccount = got
		updated := *got
		updated.Credentials = map[string]any{
			"access_token":       "new-access",
			"refresh_token":      "new-refresh",
			"base_url":           "https://example.invalid/v1",
			"subscription_tier":  "SUPER_GROK",
			"entitlement_status": "ACTIVE",
		}
		return &updated, nil
	})

	updated, warning, err := handler.refreshSingleAccount(context.Background(), account)
	require.NoError(t, err)
	require.Empty(t, warning)
	require.Equal(t, 1, calls)
	require.Same(t, account, refreshedAccount)
	require.Nil(t, adminSvc.updatedCredentials, "the handler must not persist through the stale generic update path")
	require.Equal(t, "new-access", updated.GetGrokAccessToken())
	require.Equal(t, "new-refresh", updated.GetGrokRefreshToken())
	require.Equal(t, "https://example.invalid/v1", updated.GetCredential("base_url"))
}
