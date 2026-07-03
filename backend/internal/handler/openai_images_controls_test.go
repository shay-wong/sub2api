package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIImagesPermissionAccountRepo struct {
	service.AccountRepository
	account service.Account
}

func (r openAIImagesPermissionAccountRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	if r.account.ID != id {
		return nil, service.ErrNoAvailableAccounts
	}
	account := r.account
	return &account, nil
}

func (r openAIImagesPermissionAccountRepo) ListSchedulableByGroupIDAndPlatform(_ context.Context, _ int64, platform string) ([]service.Account, error) {
	if r.account.Platform != platform {
		return nil, nil
	}
	return []service.Account{r.account}, nil
}

func (r openAIImagesPermissionAccountRepo) ListSchedulableByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return r.ListSchedulableByGroupIDAndPlatform(context.Background(), 0, platform)
}

func (r openAIImagesPermissionAccountRepo) ListSchedulableUngroupedByPlatform(_ context.Context, platform string) ([]service.Account, error) {
	return r.ListSchedulableByGroupIDAndPlatform(context.Background(), 0, platform)
}

type openAIImagesPermissionGroupRepo struct {
	service.GroupRepository
	group *service.Group
}

func (r openAIImagesPermissionGroupRepo) GetByID(_ context.Context, _ int64) (*service.Group, error) {
	return r.group, nil
}

func TestOpenAIGatewayHandlerImages_SelectedGroupRejectsImagePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	groupID := int64(111)
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.Scheduling.DbFallbackEnabled = true
	accountRepo := openAIImagesPermissionAccountRepo{account: service.Account{
		ID:          1,
		Name:        "image-account",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeOAuth,
		Status:      service.StatusActive,
		Schedulable: true,
		Concurrency: 0,
		Credentials: map[string]any{"access_token": "token"},
	}}
	selectedGroup := &service.Group{
		ID:                   groupID,
		Platform:             service.PlatformOpenAI,
		Status:               service.StatusActive,
		Hydrated:             true,
		AllowImageGeneration: false,
	}
	gatewayService := service.NewOpenAIGatewayService(
		accountRepo,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		cfg,
		service.NewSchedulerSnapshotService(nil, nil, accountRepo, openAIImagesPermissionGroupRepo{group: selectedGroup}, cfg),
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
		nil,
	)
	billingService := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, nil, cfg, nil)
	t.Cleanup(billingService.Stop)
	handler := NewOpenAIGatewayHandler(
		gatewayService,
		service.NewConcurrencyService(nil),
		billingService,
		service.NewAPIKeyService(nil, nil, nil, nil, nil, nil, cfg),
		nil,
		nil,
		nil,
		nil,
		cfg,
	)

	body := []byte(`{"model":"gpt-image-2","prompt":"draw","size":"1024x1024"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
		ID:      222,
		GroupID: &groupID,
		Group: &service.Group{
			ID:                   groupID,
			AllowImageGeneration: true,
		},
		User: &service.User{ID: 333},
	})
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 333, Concurrency: 0})

	handler.Images(c)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "permission_error", gjson.GetBytes(rec.Body.Bytes(), "error.type").String())
	require.Contains(t, rec.Body.String(), service.ImageGenerationPermissionMessage())
}
