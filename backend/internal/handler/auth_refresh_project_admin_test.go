//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAuthHandlerRefreshTokenAllowsProjectAdminInBackendMode(t *testing.T) {
	gin.SetMode(gin.TestMode)

	user := &service.User{
		ID:           42,
		Email:        "project-admin@example.com",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		TokenVersion: 3,
	}
	repo := &userHandlerRepoStub{user: user}
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:                   "test-secret",
			ExpireHour:               1,
			AccessTokenExpireMinutes: 60,
			RefreshTokenExpireDays:   7,
		},
	}
	settingSvc := service.NewSettingService(&oauthPendingFlowSettingRepoStub{
		values: map[string]string{
			service.SettingKeyBackendModeEnabled: "true",
		},
	}, cfg)
	authSvc := service.NewAuthService(nil, repo, nil, newAuthRefreshMemoryTokenCache(), cfg, settingSvc, nil, nil, nil, nil, nil, nil, nil)
	projectSvc := service.NewProjectService(&authRefreshProjectRepoStub{
		projects: []service.ProjectSummary{
			{ID: 10, Name: "Default", Slug: "default", Role: service.ProjectRoleAdmin, IsOwner: true},
		},
	})

	initialPair, err := authSvc.GenerateTokenPair(context.Background(), user, "")
	require.NoError(t, err)

	h := &AuthHandler{
		authService:    authSvc,
		userService:    service.NewUserService(repo, nil, nil, nil),
		settingSvc:     settingSvc,
		projectService: projectSvc,
	}

	body, err := json.Marshal(RefreshTokenRequest{RefreshToken: initialPair.RefreshToken})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	h.RefreshToken(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp struct {
		Code int `json:"code"`
		Data struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.NotEmpty(t, resp.Data.AccessToken)
	require.NotEmpty(t, resp.Data.RefreshToken)
}

type authRefreshMemoryTokenCache struct {
	tokens map[string]*service.RefreshTokenData
}

func newAuthRefreshMemoryTokenCache() *authRefreshMemoryTokenCache {
	return &authRefreshMemoryTokenCache{tokens: make(map[string]*service.RefreshTokenData)}
}

func (c *authRefreshMemoryTokenCache) StoreRefreshToken(_ context.Context, tokenHash string, data *service.RefreshTokenData, _ time.Duration) error {
	clone := *data
	c.tokens[tokenHash] = &clone
	return nil
}

func (c *authRefreshMemoryTokenCache) GetRefreshToken(_ context.Context, tokenHash string) (*service.RefreshTokenData, error) {
	data, ok := c.tokens[tokenHash]
	if !ok {
		return nil, service.ErrRefreshTokenNotFound
	}
	clone := *data
	return &clone, nil
}

func (c *authRefreshMemoryTokenCache) DeleteRefreshToken(_ context.Context, tokenHash string) error {
	delete(c.tokens, tokenHash)
	return nil
}

func (c *authRefreshMemoryTokenCache) DeleteUserRefreshTokens(context.Context, int64) error {
	c.tokens = map[string]*service.RefreshTokenData{}
	return nil
}

func (c *authRefreshMemoryTokenCache) DeleteTokenFamily(context.Context, string) error {
	c.tokens = map[string]*service.RefreshTokenData{}
	return nil
}

func (c *authRefreshMemoryTokenCache) AddToUserTokenSet(context.Context, int64, string, time.Duration) error {
	return nil
}

func (c *authRefreshMemoryTokenCache) AddToFamilyTokenSet(context.Context, string, string, time.Duration) error {
	return nil
}

func (c *authRefreshMemoryTokenCache) GetUserTokenHashes(context.Context, int64) ([]string, error) {
	return nil, nil
}

func (c *authRefreshMemoryTokenCache) GetFamilyTokenHashes(context.Context, string) ([]string, error) {
	return nil, nil
}

func (c *authRefreshMemoryTokenCache) IsTokenInFamily(context.Context, string, string) (bool, error) {
	return false, nil
}

type authRefreshProjectRepoStub struct {
	projects []service.ProjectSummary
}

func (r *authRefreshProjectRepoStub) GetDefaultProjectID(context.Context) (int64, error) {
	return 10, nil
}

func (r *authRefreshProjectRepoStub) GetProjectRole(context.Context, int64, int64) (string, bool, error) {
	return service.ProjectRoleAdmin, true, nil
}

func (r *authRefreshProjectRepoStub) ProjectExists(context.Context, int64) (bool, error) {
	return true, nil
}

func (r *authRefreshProjectRepoStub) ListActiveProjects(context.Context) ([]service.ProjectSummary, error) {
	return append([]service.ProjectSummary(nil), r.projects...), nil
}

func (r *authRefreshProjectRepoStub) ListUserProjects(context.Context, int64) ([]service.ProjectSummary, error) {
	return append([]service.ProjectSummary(nil), r.projects...), nil
}

func (r *authRefreshProjectRepoStub) CreateProject(context.Context, service.ProjectCreateInput) (*service.ProjectSummary, error) {
	panic("unexpected CreateProject call")
}

func (r *authRefreshProjectRepoStub) UpdateProject(context.Context, int64, service.ProjectUpdateInput) (*service.ProjectSummary, error) {
	panic("unexpected UpdateProject call")
}

func (r *authRefreshProjectRepoStub) ListProjectMembers(context.Context, int64) ([]service.ProjectMember, error) {
	panic("unexpected ListProjectMembers call")
}

func (r *authRefreshProjectRepoStub) SetProjectMember(context.Context, int64, service.ProjectMemberInput) (*service.ProjectMember, error) {
	panic("unexpected SetProjectMember call")
}

func (r *authRefreshProjectRepoStub) RemoveProjectMember(context.Context, int64, int64) error {
	panic("unexpected RemoveProjectMember call")
}

func (r *authRefreshProjectRepoStub) ListProjectProfiles(context.Context, int64) ([]service.ProjectProfile, error) {
	panic("unexpected ListProjectProfiles call")
}

func (r *authRefreshProjectRepoStub) CreateProjectProfile(context.Context, int64, service.ProjectProfileInput) (*service.ProjectProfile, error) {
	panic("unexpected CreateProjectProfile call")
}

func (r *authRefreshProjectRepoStub) UpdateProjectProfile(context.Context, int64, int64, service.ProjectProfileInput) (*service.ProjectProfile, error) {
	panic("unexpected UpdateProjectProfile call")
}

func (r *authRefreshProjectRepoStub) DeleteProjectProfile(context.Context, int64, int64) error {
	panic("unexpected DeleteProjectProfile call")
}

func (r *authRefreshProjectRepoStub) ActivateProjectProfile(context.Context, int64, int64) (*service.ProjectProfile, error) {
	panic("unexpected ActivateProjectProfile call")
}

func (r *authRefreshProjectRepoStub) ActivateProjectUnrestrictedScope(context.Context, int64) (*service.ProjectProfile, error) {
	panic("unexpected ActivateProjectUnrestrictedScope call")
}

func (r *authRefreshProjectRepoStub) GetProjectProfileBindings(context.Context, int64, int64) (*service.ProjectProfileBindings, error) {
	panic("unexpected GetProjectProfileBindings call")
}

func (r *authRefreshProjectRepoStub) SetProjectProfileBindings(context.Context, int64, int64, service.ProjectProfileBindingInput) (*service.ProjectProfileBindings, error) {
	panic("unexpected SetProjectProfileBindings call")
}

func (r *authRefreshProjectRepoStub) ValidateProjectProfileBindingScope(context.Context, int64, service.ProjectProfileBindingInput) error {
	panic("unexpected ValidateProjectProfileBindingScope call")
}

func (r *authRefreshProjectRepoStub) ValidateProjectProfileBindingResources(context.Context, service.ProjectProfileBindingInput) error {
	panic("unexpected ValidateProjectProfileBindingResources call")
}

func (r *authRefreshProjectRepoStub) SearchProjectBindableResources(context.Context, int64, string, int) (*service.ProjectResourceSearchResult, error) {
	panic("unexpected SearchProjectBindableResources call")
}
