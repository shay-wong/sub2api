package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type openAITokenRefreshFailoverCache struct {
	deletedKeys []string
}

func (c *openAITokenRefreshFailoverCache) GetAccessToken(context.Context, string) (string, error) {
	return "", nil
}

func (c *openAITokenRefreshFailoverCache) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
}

func (c *openAITokenRefreshFailoverCache) DeleteAccessToken(_ context.Context, cacheKey string) error {
	c.deletedKeys = append(c.deletedKeys, cacheKey)
	return nil
}

func (c *openAITokenRefreshFailoverCache) AcquireRefreshLock(context.Context, string, time.Duration) (bool, error) {
	return true, nil
}

func (c *openAITokenRefreshFailoverCache) ReleaseRefreshLock(context.Context, string) error {
	return nil
}

type openAITokenRefreshFailoverExecutor struct {
	err error
}

func (e *openAITokenRefreshFailoverExecutor) CanRefresh(account *Account) bool {
	return account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeOAuth
}

func (e *openAITokenRefreshFailoverExecutor) NeedsRefresh(*Account, time.Duration) bool {
	return true
}

func (e *openAITokenRefreshFailoverExecutor) Refresh(context.Context, *Account) (map[string]any, error) {
	return nil, e.err
}

func (e *openAITokenRefreshFailoverExecutor) CacheKey(account *Account) string {
	return OpenAITokenCacheKey(account)
}

type openAITokenRefreshFailoverRepo struct {
	stubOpenAIAccountRepo
	account      *Account
	setErrorID   int64
	setErrorMsg  string
	setErrorCall int
}

func (r *openAITokenRefreshFailoverRepo) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func (r *openAITokenRefreshFailoverRepo) SetError(_ context.Context, id int64, errorMsg string) error {
	r.setErrorID = id
	r.setErrorMsg = errorMsg
	r.setErrorCall++
	return nil
}

type openAITokenRefreshFailoverBlocker struct {
	accountIDs []int64
	reasons    []string
}

func (b *openAITokenRefreshFailoverBlocker) BlockAccountScheduling(account *Account, _ time.Time, reason string) {
	if account != nil {
		b.accountIDs = append(b.accountIDs, account.ID)
	}
	b.reasons = append(b.reasons, reason)
}

func (b *openAITokenRefreshFailoverBlocker) ClearAccountSchedulingBlock(int64) {}

func TestOpenAITokenProvider_PermanentRefreshFailureDisablesAccount(t *testing.T) {
	expiresAt := time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:       4509,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "expired-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiresAt,
		},
	}
	repo := &openAITokenRefreshFailoverRepo{account: account}
	cache := &openAITokenRefreshFailoverCache{}
	provider := NewOpenAITokenProvider(repo, cache, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &openAITokenRefreshFailoverExecutor{
		err: errors.New(`OPENAI_OAUTH_TOKEN_REFRESH_FAILED: token refresh failed: status 400, body: {"error":{"message":"Your session has ended. Please log in again.","type":"invalid_request_error","code":"app_session_terminated"}}`),
	})
	blocker := &openAITokenRefreshFailoverBlocker{}
	provider.SetAccountRuntimeBlocker(blocker)

	token, err := provider.GetAccessToken(context.Background(), account)

	require.Error(t, err)
	require.Empty(t, token)
	var permanentErr *OpenAITokenPermanentRefreshError
	require.ErrorAs(t, err, &permanentErr)
	require.Equal(t, 1, repo.setErrorCall)
	require.Equal(t, account.ID, repo.setErrorID)
	require.Contains(t, repo.setErrorMsg, "app_session_terminated")
	require.Equal(t, []int64{account.ID}, blocker.accountIDs)
	require.Equal(t, []string{"token_refresh_non_retryable"}, blocker.reasons)
	require.Equal(t, []string{OpenAITokenCacheKey(account)}, cache.deletedKeys)
}

func TestOpenAIGatewayService_GetAccessTokenPermanentRefreshFailureReturnsFailover(t *testing.T) {
	expiresAt := time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:       4510,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":  "expired-access-token",
			"refresh_token": "refresh-token",
			"expires_at":    expiresAt,
		},
	}
	repo := &openAITokenRefreshFailoverRepo{account: account}
	cache := &openAITokenRefreshFailoverCache{}
	provider := NewOpenAITokenProvider(repo, cache, nil)
	provider.SetRefreshAPI(NewOAuthRefreshAPI(repo, cache), &openAITokenRefreshFailoverExecutor{
		err: errors.New(`OPENAI_OAUTH_TOKEN_REFRESH_FAILED: token refresh failed: status 400, body: {"error":{"code":"app_session_terminated"}}`),
	})
	svc := &OpenAIGatewayService{openAITokenProvider: provider}

	token, tokenType, err := svc.GetAccessToken(context.Background(), account)

	require.Error(t, err)
	require.Empty(t, token)
	require.Empty(t, tokenType)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "app_session_terminated")
	require.Equal(t, 1, repo.setErrorCall)
}

func TestOpenAIGatewayService_GetAccessTokenMissingRefreshTokenReturnsFailover(t *testing.T) {
	expiresAt := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	account := &Account{
		ID:       4511,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "expired-access-token",
			"expires_at":   expiresAt,
		},
	}
	repo := &openAITokenRefreshFailoverRepo{account: account}
	cache := &openAITokenRefreshFailoverCache{}
	provider := NewOpenAITokenProvider(repo, cache, nil)
	svc := &OpenAIGatewayService{openAITokenProvider: provider}

	token, tokenType, err := svc.GetAccessToken(context.Background(), account)

	require.Error(t, err)
	require.Empty(t, token)
	require.Empty(t, tokenType)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Contains(t, string(failoverErr.ResponseBody), "refresh_token is missing")
	require.Equal(t, 1, repo.setErrorCall)
	require.Contains(t, repo.setErrorMsg, "refresh_token is missing")
	require.Equal(t, []string{OpenAITokenCacheKey(account)}, cache.deletedKeys)
}
