//go:build unit

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminAuthJWTValidatesTokenVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpireHour: 1}}
	authService := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)

	admin := &service.User{
		ID:           1,
		Email:        "admin@example.com",
		Role:         service.RoleAdmin,
		Status:       service.StatusActive,
		TokenVersion: 2,
		Concurrency:  1,
	}

	userRepo := &stubUserRepo{
		getByID: func(ctx context.Context, id int64) (*service.User, error) {
			if id != admin.ID {
				return nil, service.ErrUserNotFound
			}
			clone := *admin
			return &clone, nil
		},
	}
	userService := service.NewUserService(userRepo, nil, nil, nil)

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAdminAuthMiddleware(authService, userService, nil, nil)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	t.Run("token_version_mismatch_rejected", func(t *testing.T) {
		token, err := authService.GenerateToken(&service.User{
			ID:           admin.ID,
			Email:        admin.Email,
			Role:         admin.Role,
			TokenVersion: admin.TokenVersion - 1,
		})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Contains(t, w.Body.String(), "TOKEN_REVOKED")
	})

	t.Run("token_version_match_allows", func(t *testing.T) {
		token, err := authService.GenerateToken(&service.User{
			ID:           admin.ID,
			Email:        admin.Email,
			Role:         admin.Role,
			TokenVersion: admin.TokenVersion,
		})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("websocket_token_version_mismatch_rejected", func(t *testing.T) {
		token, err := authService.GenerateToken(&service.User{
			ID:           admin.ID,
			Email:        admin.Email,
			Role:         admin.Role,
			TokenVersion: admin.TokenVersion - 1,
		})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Sec-WebSocket-Protocol", "sub2api-admin, jwt."+token)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Contains(t, w.Body.String(), "TOKEN_REVOKED")
	})

	t.Run("websocket_token_version_match_allows", func(t *testing.T) {
		token, err := authService.GenerateToken(&service.User{
			ID:           admin.ID,
			Email:        admin.Email,
			Role:         admin.Role,
			TokenVersion: admin.TokenVersion,
		})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/t", nil)
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Sec-WebSocket-Protocol", "sub2api-admin, jwt."+token)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestAdminAuthJWTUsesProjectIDQueryForWebSocket(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpireHour: 1}}
	authService := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)

	projectMember := &service.User{
		ID:           7,
		Email:        "project-admin@example.com",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		TokenVersion: 1,
		Concurrency:  1,
	}

	userRepo := &stubUserRepo{
		getByID: func(ctx context.Context, id int64) (*service.User, error) {
			if id != projectMember.ID {
				return nil, service.ErrUserNotFound
			}
			clone := *projectMember
			return &clone, nil
		},
	}
	userService := service.NewUserService(userRepo, nil, nil, nil)
	projectService := service.NewProjectService(&stubProjectRepo{
		projects: []service.ProjectSummary{
			{ID: 42, Name: "Admin Project", Slug: "admin-project", Role: service.ProjectRoleAdmin},
		},
		roles: map[int64]map[int64]string{
			42: {projectMember.ID: service.ProjectRoleAdmin},
		},
		exists: map[int64]bool{42: true},
	})

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAdminAuthMiddleware(authService, userService, nil, projectService)))
	router.GET("/t", func(c *gin.Context) {
		projectID, ok := service.ProjectIDFromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, int64(42), projectID)
		role, ok := GetUserRoleFromContext(c)
		require.True(t, ok)
		require.Equal(t, service.RoleAdmin, role)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	token, err := authService.GenerateToken(&service.User{
		ID:           projectMember.ID,
		Email:        projectMember.Email,
		Role:         projectMember.Role,
		TokenVersion: projectMember.TokenVersion,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t?project_id=42", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Protocol", "sub2api-admin, jwt."+token)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAdminAuthJWTDefaultsToFirstProjectAdminMembership(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpireHour: 1}}
	authService := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)

	projectMember := &service.User{
		ID:           7,
		Email:        "project-admin@example.com",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		TokenVersion: 1,
		Concurrency:  1,
	}

	userRepo := &stubUserRepo{
		getByID: func(ctx context.Context, id int64) (*service.User, error) {
			if id != projectMember.ID {
				return nil, service.ErrUserNotFound
			}
			clone := *projectMember
			return &clone, nil
		},
	}
	userService := service.NewUserService(userRepo, nil, nil, nil)
	projectService := service.NewProjectService(&stubProjectRepo{
		projects: []service.ProjectSummary{
			{ID: 10, Name: "Member Only", Slug: "member-only", Role: service.ProjectRoleUser},
			{ID: 42, Name: "Admin Project", Slug: "admin-project", Role: service.ProjectRoleAdmin},
		},
		roles: map[int64]map[int64]string{
			10: {projectMember.ID: service.ProjectRoleUser},
			42: {projectMember.ID: service.ProjectRoleAdmin},
		},
		exists: map[int64]bool{10: true, 42: true},
	})

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAdminAuthMiddleware(authService, userService, nil, projectService)))
	router.GET("/t", func(c *gin.Context) {
		projectID, ok := service.ProjectIDFromContext(c.Request.Context())
		require.True(t, ok)
		require.Equal(t, int64(42), projectID)
		role, ok := GetUserRoleFromContext(c)
		require.True(t, ok)
		require.Equal(t, service.RoleAdmin, role)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	token, err := authService.GenerateToken(&service.User{
		ID:           projectMember.ID,
		Email:        projectMember.Email,
		Role:         projectMember.Role,
		TokenVersion: projectMember.TokenVersion,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAdminAuthJWTRejectsLegacyOperatorWithoutProjectAdminMembership(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpireHour: 1}}
	authService := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)

	operator := &service.User{
		ID:           7,
		Email:        "operator@example.com",
		Role:         service.RoleOperator,
		Status:       service.StatusActive,
		TokenVersion: 1,
		Concurrency:  1,
	}

	userRepo := &stubUserRepo{
		getByID: func(ctx context.Context, id int64) (*service.User, error) {
			if id != operator.ID {
				return nil, service.ErrUserNotFound
			}
			clone := *operator
			return &clone, nil
		},
	}
	userService := service.NewUserService(userRepo, nil, nil, nil)
	projectService := service.NewProjectService(&stubProjectRepo{
		projects: []service.ProjectSummary{
			{ID: 42, Name: "Member Project", Slug: "member-project", Role: service.ProjectRoleUser},
		},
		roles: map[int64]map[int64]string{
			42: {operator.ID: service.ProjectRoleUser},
		},
		exists: map[int64]bool{42: true},
	})

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAdminAuthMiddleware(authService, userService, nil, projectService)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	token, err := authService.GenerateToken(&service.User{
		ID:           operator.ID,
		Email:        operator.Email,
		Role:         operator.Role,
		TokenVersion: operator.TokenVersion,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "LEGACY_OPERATOR_ROLE_DISABLED")
}

func TestAdminAuthJWTRejectsLegacyOperatorWithProjectAdminMembership(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpireHour: 1}}
	authService := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)

	operator := &service.User{
		ID:           7,
		Email:        "operator@example.com",
		Role:         service.RoleOperator,
		Status:       service.StatusActive,
		TokenVersion: 1,
		Concurrency:  1,
	}

	userRepo := &stubUserRepo{
		getByID: func(ctx context.Context, id int64) (*service.User, error) {
			if id != operator.ID {
				return nil, service.ErrUserNotFound
			}
			clone := *operator
			return &clone, nil
		},
	}
	userService := service.NewUserService(userRepo, nil, nil, nil)
	projectService := service.NewProjectService(&stubProjectRepo{
		projects: []service.ProjectSummary{
			{ID: 42, Name: "Admin Project", Slug: "admin-project", Role: service.ProjectRoleAdmin},
		},
		roles: map[int64]map[int64]string{
			42: {operator.ID: service.ProjectRoleAdmin},
		},
		exists: map[int64]bool{42: true},
	})

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAdminAuthMiddleware(authService, userService, nil, projectService)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	token, err := authService.GenerateToken(&service.User{
		ID:           operator.ID,
		Email:        operator.Email,
		Role:         operator.Role,
		TokenVersion: operator.TokenVersion,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Project-ID", "42")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "LEGACY_OPERATOR_ROLE_DISABLED")
}

func TestAdminAuthJWTRejectsDisabledProjectMember(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{JWT: config.JWTConfig{Secret: "test-secret", ExpireHour: 1}}
	authService := service.NewAuthService(nil, nil, nil, nil, cfg, nil, nil, nil, nil, nil, nil, nil, nil)

	projectMember := &service.User{
		ID:           8,
		Email:        "disabled-project-admin@example.com",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		TokenVersion: 1,
		Concurrency:  1,
	}

	userRepo := &stubUserRepo{
		getByID: func(ctx context.Context, id int64) (*service.User, error) {
			if id != projectMember.ID {
				return nil, service.ErrUserNotFound
			}
			clone := *projectMember
			return &clone, nil
		},
	}
	userService := service.NewUserService(userRepo, nil, nil, nil)
	projectService := service.NewProjectService(&stubProjectRepo{
		exists: map[int64]bool{42: true},
	})

	router := gin.New()
	router.Use(gin.HandlerFunc(NewAdminAuthMiddleware(authService, userService, nil, projectService)))
	router.GET("/t", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	token, err := authService.GenerateToken(&service.User{
		ID:           projectMember.ID,
		Email:        projectMember.Email,
		Role:         projectMember.Role,
		TokenVersion: projectMember.TokenVersion,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Project-ID", "42")
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "PROJECT_ACCESS_FORBIDDEN")
}

type stubProjectRepo struct {
	defaultID int64
	exists    map[int64]bool
	roles     map[int64]map[int64]string
	projects  []service.ProjectSummary
}

func (s *stubProjectRepo) GetDefaultProjectID(context.Context) (int64, error) {
	if s.defaultID > 0 {
		return s.defaultID, nil
	}
	return 1, nil
}

func (s *stubProjectRepo) ProjectExists(_ context.Context, projectID int64) (bool, error) {
	if s.exists == nil {
		return true, nil
	}
	return s.exists[projectID], nil
}

func (s *stubProjectRepo) GetProjectRole(_ context.Context, projectID int64, userID int64) (string, bool, error) {
	if s.roles == nil || s.roles[projectID] == nil {
		return "", false, nil
	}
	role, ok := s.roles[projectID][userID]
	return role, ok, nil
}

func (s *stubProjectRepo) ListActiveProjects(context.Context) ([]service.ProjectSummary, error) {
	return append([]service.ProjectSummary(nil), s.projects...), nil
}

func (s *stubProjectRepo) ListUserProjects(context.Context, int64) ([]service.ProjectSummary, error) {
	return append([]service.ProjectSummary(nil), s.projects...), nil
}

func (s *stubProjectRepo) CreateProject(context.Context, service.ProjectCreateInput) (*service.ProjectSummary, error) {
	panic("unexpected CreateProject call")
}

func (s *stubProjectRepo) UpdateProject(context.Context, int64, service.ProjectUpdateInput) (*service.ProjectSummary, error) {
	panic("unexpected UpdateProject call")
}

func (s *stubProjectRepo) ListProjectMembers(context.Context, int64) ([]service.ProjectMember, error) {
	panic("unexpected ListProjectMembers call")
}

func (s *stubProjectRepo) SetProjectMember(context.Context, int64, service.ProjectMemberInput) (*service.ProjectMember, error) {
	panic("unexpected SetProjectMember call")
}

func (s *stubProjectRepo) RemoveProjectMember(context.Context, int64, int64) error {
	panic("unexpected RemoveProjectMember call")
}

func (s *stubProjectRepo) ListProjectProfiles(context.Context, int64) ([]service.ProjectProfile, error) {
	panic("unexpected ListProjectProfiles call")
}

func (s *stubProjectRepo) CreateProjectProfile(context.Context, int64, service.ProjectProfileInput) (*service.ProjectProfile, error) {
	panic("unexpected CreateProjectProfile call")
}

func (s *stubProjectRepo) UpdateProjectProfile(context.Context, int64, int64, service.ProjectProfileInput) (*service.ProjectProfile, error) {
	panic("unexpected UpdateProjectProfile call")
}

func (s *stubProjectRepo) DeleteProjectProfile(context.Context, int64, int64) error {
	panic("unexpected DeleteProjectProfile call")
}

func (s *stubProjectRepo) ActivateProjectProfile(context.Context, int64, int64) (*service.ProjectProfile, error) {
	panic("unexpected ActivateProjectProfile call")
}

func (s *stubProjectRepo) ActivateProjectUnrestrictedScope(context.Context, int64) (*service.ProjectProfile, error) {
	panic("unexpected ActivateProjectUnrestrictedScope call")
}

func (s *stubProjectRepo) GetProjectProfileBindings(context.Context, int64, int64) (*service.ProjectProfileBindings, error) {
	panic("unexpected GetProjectProfileBindings call")
}

func (s *stubProjectRepo) SetProjectProfileBindings(context.Context, int64, int64, service.ProjectProfileBindingInput) (*service.ProjectProfileBindings, error) {
	panic("unexpected SetProjectProfileBindings call")
}

func (s *stubProjectRepo) ValidateProjectProfileBindingScope(context.Context, int64, service.ProjectProfileBindingInput) error {
	panic("unexpected ValidateProjectProfileBindingScope call")
}

func (s *stubProjectRepo) ValidateProjectProfileBindingResources(context.Context, service.ProjectProfileBindingInput) error {
	panic("unexpected ValidateProjectProfileBindingResources call")
}

func (s *stubProjectRepo) SearchProjectBindableResources(context.Context, int64, string, int) (*service.ProjectResourceSearchResult, error) {
	panic("unexpected SearchProjectBindableResources call")
}

type stubUserRepo struct {
	getByID func(ctx context.Context, id int64) (*service.User, error)
}

func (s *stubUserRepo) Create(ctx context.Context, user *service.User) error {
	panic("unexpected Create call")
}

func (s *stubUserRepo) GetByID(ctx context.Context, id int64) (*service.User, error) {
	if s.getByID == nil {
		panic("GetByID not stubbed")
	}
	return s.getByID(ctx, id)
}

func (s *stubUserRepo) GetByEmail(ctx context.Context, email string) (*service.User, error) {
	panic("unexpected GetByEmail call")
}

func (s *stubUserRepo) GetFirstAdmin(ctx context.Context) (*service.User, error) {
	panic("unexpected GetFirstAdmin call")
}

func (s *stubUserRepo) Update(ctx context.Context, user *service.User) error {
	panic("unexpected Update call")
}

func (s *stubUserRepo) Delete(ctx context.Context, id int64) error {
	panic("unexpected Delete call")
}

func (s *stubUserRepo) GetUserAvatar(ctx context.Context, userID int64) (*service.UserAvatar, error) {
	return nil, nil
}

func (s *stubUserRepo) UpsertUserAvatar(ctx context.Context, userID int64, input service.UpsertUserAvatarInput) (*service.UserAvatar, error) {
	panic("unexpected UpsertUserAvatar call")
}

func (s *stubUserRepo) DeleteUserAvatar(ctx context.Context, userID int64) error {
	panic("unexpected DeleteUserAvatar call")
}

func (s *stubUserRepo) List(ctx context.Context, params pagination.PaginationParams) ([]service.User, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (s *stubUserRepo) ListWithFilters(ctx context.Context, params pagination.PaginationParams, filters service.UserListFilters) ([]service.User, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (s *stubUserRepo) GetLatestUsedAtByUserIDs(ctx context.Context, userIDs []int64) (map[int64]*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserIDs call")
}

func (s *stubUserRepo) GetLatestUsedAtByUserID(ctx context.Context, userID int64) (*time.Time, error) {
	panic("unexpected GetLatestUsedAtByUserID call")
}

func (s *stubUserRepo) UpdateUserLastActiveAt(ctx context.Context, userID int64, activeAt time.Time) error {
	panic("unexpected UpdateUserLastActiveAt call")
}

func (s *stubUserRepo) UpdateBalance(ctx context.Context, id int64, amount float64) error {
	panic("unexpected UpdateBalance call")
}

func (s *stubUserRepo) DeductBalance(ctx context.Context, id int64, amount float64) error {
	panic("unexpected DeductBalance call")
}

func (s *stubUserRepo) UpdateConcurrency(ctx context.Context, id int64, amount int) error {
	panic("unexpected UpdateConcurrency call")
}

func (s *stubUserRepo) BatchSetConcurrency(context.Context, []int64, int) (int, error) { return 0, nil }
func (s *stubUserRepo) BatchAddConcurrency(context.Context, []int64, int) (int, error) { return 0, nil }

func (s *stubUserRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	panic("unexpected ExistsByEmail call")
}

func (s *stubUserRepo) RemoveGroupFromAllowedGroups(ctx context.Context, groupID int64) (int64, error) {
	panic("unexpected RemoveGroupFromAllowedGroups call")
}

func (s *stubUserRepo) RemoveGroupFromUserAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	panic("unexpected RemoveGroupFromUserAllowedGroups call")
}

func (s *stubUserRepo) AddGroupToAllowedGroups(ctx context.Context, userID int64, groupID int64) error {
	panic("unexpected AddGroupToAllowedGroups call")
}

func (s *stubUserRepo) ListUserAuthIdentities(ctx context.Context, userID int64) ([]service.UserAuthIdentityRecord, error) {
	panic("unexpected ListUserAuthIdentities call")
}

func (s *stubUserRepo) UnbindUserAuthProvider(context.Context, int64, string) error {
	panic("unexpected UnbindUserAuthProvider call")
}

func (s *stubUserRepo) UpdateTotpSecret(ctx context.Context, userID int64, encryptedSecret *string) error {
	panic("unexpected UpdateTotpSecret call")
}

func (s *stubUserRepo) EnableTotp(ctx context.Context, userID int64) error {
	panic("unexpected EnableTotp call")
}

func (s *stubUserRepo) DisableTotp(ctx context.Context, userID int64) error {
	panic("unexpected DisableTotp call")
}

func (s *stubUserRepo) GetByIDIncludeDeleted(ctx context.Context, id int64) (*service.User, error) {
	panic("unexpected GetByIDIncludeDeleted call")
}
