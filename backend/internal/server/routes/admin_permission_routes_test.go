package routes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPaymentAdminRoutesRequireAdminOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")

	RegisterPaymentRoutes(
		v1,
		&handler.PaymentHandler{},
		&handler.PaymentWebhookHandler{},
		&adminhandler.PaymentHandler{},
		servermiddleware.JWTAuthMiddleware(func(c *gin.Context) { c.Next() }),
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleOperator)
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/payment/config", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestProjectAdminCanListOwnProjectsForProjectSwitcher(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	projectSvc := service.NewProjectService(&projectRouteRepoStub{
		listUserProjects: []service.ProjectSummary{{ID: 1, Name: "Default", Slug: "default", Role: service.ProjectRoleAdmin}},
	})

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/projects", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestProjectCreateStillRequiresSuperAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/projects", strings.NewReader(`{
		"name":"Escalated",
		"slug":"escalated",
		"profile_mode":"unrestricted"
	}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.False(t, repo.createProjectCalled)
}

func TestProjectAdminCanUpdateScopedAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	adminSvc := &apiKeyRouteAdminServiceStub{}

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{APIKey: adminhandler.NewAdminAPIKeyHandler(adminSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", strings.NewReader(`{"group_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(10), adminSvc.updatedKeyID)
	require.NotNil(t, adminSvc.updatedGroupID)
	require.Equal(t, int64(2), *adminSvc.updatedGroupID)
}

func TestProjectAdminCannotUpdateAPIKeyWithoutAccountPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	adminSvc := &apiKeyRouteAdminServiceStub{}

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{APIKey: adminhandler.NewAdminAPIKeyHandler(adminSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), []string{service.AdminPermissionDashboardRead}))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", strings.NewReader(`{"group_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Zero(t, adminSvc.updatedKeyID)
}

func TestProjectAdminCannotTransferAPIKeyProjectWithAccountPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	adminSvc := &apiKeyRouteAdminServiceStub{}

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{APIKey: adminhandler.NewAdminAPIKeyHandler(adminSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10/project", strings.NewReader(`{"project_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Zero(t, adminSvc.updatedKeyID)
	require.Zero(t, adminSvc.transferProjectID)
}

func TestProjectAdminCannotTransferAPIKeyProjectWithoutAccountPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	adminSvc := &apiKeyRouteAdminServiceStub{}

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{APIKey: adminhandler.NewAdminAPIKeyHandler(adminSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), []string{service.AdminPermissionUsersManage}))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10/project", strings.NewReader(`{"project_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Zero(t, adminSvc.transferProjectID)
}

func TestSuperAdminCanTransferAPIKeyProject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	adminSvc := &apiKeyRouteAdminServiceStub{}

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{APIKey: adminhandler.NewAdminAPIKeyHandler(adminSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 1})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleSuperAdmin)
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10/project", strings.NewReader(`{"project_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(10), adminSvc.updatedKeyID)
	require.Equal(t, int64(2), adminSvc.transferProjectID)
}

func TestUserCannotUpdateAdminAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	adminSvc := &apiKeyRouteAdminServiceStub{}

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{APIKey: adminhandler.NewAdminAPIKeyHandler(adminSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 9})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleUser)
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/api-keys/10", strings.NewReader(`{"group_id":2}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Zero(t, adminSvc.updatedKeyID)
}

func TestSuperAdminCanCreateUnrestrictedProject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleSuperAdmin)
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/projects", strings.NewReader(`{
		"name":"Ops",
		"slug":"ops",
		"profile_mode":"unrestricted",
		"group_ids":[2],
		"account_ids":[3],
		"subscription_ids":[4]
	}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	require.True(t, repo.createProjectCalled)
	require.Equal(t, service.ProjectProfileModeUnrestricted, repo.projectInput.ProfileMode)
	require.Equal(t, []int64{2}, repo.projectInput.Bindings.GroupIDs)
	require.Equal(t, []int64{3}, repo.projectInput.Bindings.AccountIDs)
	require.Equal(t, []int64{4}, repo.projectInput.Bindings.SubscriptionIDs)
}

func TestProjectAdminCanReadUsageRoutesWithPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	admin.Use(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
		c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
		c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
		c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
		c.Next()
	})

	usage := admin.Group("/usage")
	usageRead := servermiddleware.RequireAdminPermission(service.AdminPermissionUsageRead)
	usage.GET("", usageRead, func(c *gin.Context) { c.Status(http.StatusOK) })
	usage.GET("/stats", usageRead, func(c *gin.Context) { c.Status(http.StatusOK) })
	usage.GET("/search-models", usageRead, func(c *gin.Context) { c.Status(http.StatusOK) })
	usage.GET("/search-accounts", usageRead, func(c *gin.Context) { c.Status(http.StatusOK) })
	usage.GET("/search-groups", usageRead, func(c *gin.Context) { c.Status(http.StatusOK) })
	usage.GET("/cleanup-tasks", servermiddleware.RequireAdminOnly(), func(c *gin.Context) { c.Status(http.StatusOK) })

	readRec := httptest.NewRecorder()
	readReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage", nil)
	router.ServeHTTP(readRec, readReq)
	require.Equal(t, http.StatusOK, readRec.Code)

	statsRec := httptest.NewRecorder()
	statsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage/stats", nil)
	router.ServeHTTP(statsRec, statsReq)
	require.Equal(t, http.StatusOK, statsRec.Code)

	searchModelsRec := httptest.NewRecorder()
	searchModelsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage/search-models", nil)
	router.ServeHTTP(searchModelsRec, searchModelsReq)
	require.Equal(t, http.StatusOK, searchModelsRec.Code)

	searchAccountsRec := httptest.NewRecorder()
	searchAccountsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage/search-accounts?q=claude", nil)
	router.ServeHTTP(searchAccountsRec, searchAccountsReq)
	require.Equal(t, http.StatusOK, searchAccountsRec.Code)

	searchGroupsRec := httptest.NewRecorder()
	searchGroupsReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage/search-groups", nil)
	router.ServeHTTP(searchGroupsRec, searchGroupsReq)
	require.Equal(t, http.StatusOK, searchGroupsRec.Code)

	cleanupRec := httptest.NewRecorder()
	cleanupReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage/cleanup-tasks", nil)
	router.ServeHTTP(cleanupRec, cleanupReq)
	require.Equal(t, http.StatusForbidden, cleanupRec.Code)
}

func TestProjectAdminCannotReadUsageRoutesWithoutPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	admin.Use(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
		c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
		c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
		c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), []string{service.AdminPermissionDashboardRead}))
		c.Next()
	})

	usage := admin.Group("/usage")
	usage.GET("", servermiddleware.RequireAdminPermission(service.AdminPermissionUsageRead), func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestSuperAdminCanAccessUsageRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	admin := router.Group("/api/v1/admin")
	admin.Use(func(c *gin.Context) {
		c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 1})
		c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleSuperAdmin)
		c.Next()
	})

	usage := admin.Group("/usage")
	usage.GET("", servermiddleware.RequireAdminPermission(service.AdminPermissionUsageRead), func(c *gin.Context) { c.Status(http.StatusOK) })
	usage.GET("/cleanup-tasks", servermiddleware.RequireAdminOnly(), func(c *gin.Context) { c.Status(http.StatusOK) })

	readRec := httptest.NewRecorder()
	readReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage", nil)
	router.ServeHTTP(readRec, readReq)
	require.Equal(t, http.StatusOK, readRec.Code)

	cleanupRec := httptest.NewRecorder()
	cleanupReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/usage/cleanup-tasks", nil)
	router.ServeHTTP(cleanupRec, cleanupReq)
	require.Equal(t, http.StatusOK, cleanupRec.Code)
}

func TestProjectAdminCanUpdateRegularProjectMemberStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{
		members: []service.ProjectMember{{ProjectID: 1, UserID: 42, Role: service.ProjectRoleUser, Status: service.StatusActive}},
	}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/projects/1/members/42", strings.NewReader(`{"role":"user","status":"disabled"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.False(t, repo.validateBindingScopeCalled)
	require.True(t, repo.setMemberCalled)
	require.Equal(t, service.ProjectRoleUser, repo.memberInput.Role)
	require.NotNil(t, repo.memberInput.Status)
	require.Equal(t, service.StatusDisabled, *repo.memberInput.Status)
}

func TestProjectAdminCannotChangeProjectMemberRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{
		members: []service.ProjectMember{{ProjectID: 1, UserID: 42, Role: service.ProjectRoleUser, Status: service.StatusActive}},
	}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/projects/1/members/42", strings.NewReader(`{"role":"admin","status":"disabled"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.False(t, repo.validateBindingScopeCalled)
	require.False(t, repo.setMemberCalled)
}

func TestSuperAdminCanUpdateExistingProjectMemberWithoutRebindingScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{
		members: []service.ProjectMember{{ProjectID: 1, UserID: 42, Role: service.ProjectRoleUser, Status: service.StatusActive}},
	}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleSuperAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/projects/1/members/42", strings.NewReader(`{"role":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.False(t, repo.validateBindingScopeCalled)
	require.True(t, repo.setMemberCalled)
	require.Equal(t, service.ProjectRoleAdmin, repo.memberInput.Role)
}

func TestProjectAdminCannotAccessDifferentProjectPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	projectSvc := service.NewProjectService(&projectRouteRepoStub{})

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/projects/2/profiles", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestSuperAdminCannotSetProjectProfileUnrestrictedMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{
		listProfiles: []service.ProjectProfile{
			{ID: 2, ProjectID: 1, Name: "Restricted", Mode: service.ProjectProfileModeRestricted},
		},
	}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleSuperAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/projects/1/profiles/2", strings.NewReader(`{"mode":"unrestricted"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, repo.updateProfileCalled)
}

func TestSuperAdminCannotEditInternalUnrestrictedProjectProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{
		listProfiles: []service.ProjectProfile{
			{
				ID:        2,
				ProjectID: 1,
				Name:      "Unrestricted",
				Mode:      service.ProjectProfileModeUnrestricted,
				IsActive:  false,
			},
		},
	}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleSuperAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/projects/1/profiles/2", strings.NewReader(`{"name":"Renamed"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, repo.updateProfileCalled)
}

func TestProjectAdminCannotCreateProjectProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/projects/1/profiles", strings.NewReader(`{"name":"Open","mode":"unrestricted"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.False(t, repo.createProfileCalled)
}

func TestSuperAdminCannotCreateUnrestrictedProjectProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleSuperAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/projects/1/profiles", strings.NewReader(`{"name":"Open","mode":"unrestricted"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, repo.createProfileCalled)
}

func TestProjectAdminCannotActivateProjectProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{
		listProfiles: []service.ProjectProfile{
			{
				ID:        2,
				ProjectID: 1,
				Name:      "Unrestricted",
				Mode:      service.ProjectProfileModeUnrestricted,
				IsActive:  false,
			},
		},
	}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/projects/1/profiles/2/activate", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.False(t, repo.activateProfileCalled)
}

func TestSuperAdminCannotActivateInternalUnrestrictedProjectProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{
		listProfiles: []service.ProjectProfile{
			{
				ID:        2,
				ProjectID: 1,
				Name:      "Unrestricted",
				Mode:      service.ProjectProfileModeUnrestricted,
				IsActive:  false,
			},
		},
	}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleSuperAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/projects/1/profiles/2/activate", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, repo.activateProfileCalled)
}

func TestSuperAdminCanActivateUnrestrictedProjectScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{
		listProfiles: []service.ProjectProfile{
			{
				ID:        2,
				ProjectID: 1,
				Name:      "Restricted",
				Mode:      service.ProjectProfileModeRestricted,
				IsActive:  true,
			},
		},
	}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleSuperAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/projects/1/resource-scope/unrestricted", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.activateUnrestrictedCalled)
}

func TestProjectAdminCannotDeleteProjectProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{
		listProfiles: []service.ProjectProfile{
			{
				ID:        2,
				ProjectID: 1,
				Name:      "Unrestricted",
				Mode:      service.ProjectProfileModeUnrestricted,
				IsActive:  false,
			},
		},
	}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/projects/1/profiles/2", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.False(t, repo.deleteProfileCalled)
}

func TestProjectAdminCannotSetBindingsForUnrestrictedProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{
		listProfiles: []service.ProjectProfile{
			{
				ID:        2,
				ProjectID: 1,
				Name:      "Unrestricted",
				Mode:      service.ProjectProfileModeUnrestricted,
				IsActive:  false,
			},
		},
	}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/projects/1/profiles/2/bindings", strings.NewReader(`{"group_ids":[7]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.False(t, repo.setBindingsCalled)
	require.False(t, repo.validateBindingScopeCalled)
}

func TestProjectAdminCannotBindResourcesIntoProjectProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{
		listProfiles: []service.ProjectProfile{
			{
				ID:        2,
				ProjectID: 1,
				Name:      "Restricted",
				Mode:      service.ProjectProfileModeRestricted,
				IsActive:  false,
			},
		},
	}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/projects/1/profiles/2/bindings", strings.NewReader(`{"group_ids":[99]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.False(t, repo.setBindingsCalled)
	require.False(t, repo.validateBindingScopeCalled)
}

func TestProjectAdminProfileBindingScopeValidationIsNotReached(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{
		listProfiles: []service.ProjectProfile{
			{
				ID:        2,
				ProjectID: 1,
				Name:      "Restricted",
				Mode:      service.ProjectProfileModeRestricted,
				IsActive:  false,
			},
		},
		validateBindingScopeErr: service.ErrProjectAccessForbidden,
	}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/projects/1/profiles/2/bindings", strings.NewReader(`{"group_ids":[99]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.False(t, repo.validateBindingScopeCalled)
	require.False(t, repo.setBindingsCalled)
}

func TestSuperAdminCanBindGlobalResourcesIntoProjectProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{
		listProfiles: []service.ProjectProfile{
			{
				ID:        2,
				ProjectID: 1,
				Name:      "Restricted",
				Mode:      service.ProjectProfileModeRestricted,
				IsActive:  false,
			},
		},
	}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleSuperAdmin)
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/projects/1/profiles/2/bindings", strings.NewReader(`{"group_ids":[99]}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.setBindingsCalled)
	require.False(t, repo.validateBindingScopeCalled)
}

func TestSuperAdminSearchesScopedCandidatesThroughProjectPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	repo := &projectRouteRepoStub{}
	projectSvc := service.NewProjectService(repo)

	RegisterAdminRoutes(
		v1,
		&handler.Handlers{Admin: &handler.AdminHandlers{Project: adminhandler.NewProjectHandler(projectSvc)}},
		servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
			c.Set(string(servermiddleware.ContextKeyUser), servermiddleware.AuthSubject{UserID: 7})
			c.Set(string(servermiddleware.ContextKeyUserRole), service.RoleSuperAdmin)
			c.Request = c.Request.WithContext(service.WithProjectID(c.Request.Context(), 1))
			c.Request = c.Request.WithContext(service.WithAdminPermissions(c.Request.Context(), service.DefaultProjectAdminPermissions()))
			c.Next()
		}),
		nil,
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/projects/1/resources/search?q=shared&limit=30", nil)
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, repo.searchCalled)
	require.Equal(t, int64(1), repo.searchProjectID)
	require.Equal(t, "shared", repo.searchQuery)
	require.Equal(t, 30, repo.searchLimit)
}

type projectRouteRepoStub struct {
	listUserProjects           []service.ProjectSummary
	listProfiles               []service.ProjectProfile
	createProfileCalled        bool
	updateProfileCalled        bool
	deleteProfileCalled        bool
	createProjectCalled        bool
	setBindingsCalled          bool
	activateProfile            *service.ProjectProfile
	activateProfileCalled      bool
	activateUnrestrictedCalled bool
	validateBindingScopeCalled bool
	validateBindingScopeErr    error
	scopeInput                 service.ProjectProfileBindingInput
	searchCalled               bool
	searchProjectID            int64
	searchQuery                string
	searchLimit                int
	projectInput               service.ProjectCreateInput
	profileInput               service.ProjectProfileInput
	members                    []service.ProjectMember
	setMemberCalled            bool
	memberInput                service.ProjectMemberInput
}

func (r *projectRouteRepoStub) GetDefaultProjectID(context.Context) (int64, error) {
	return 1, nil
}

func (r *projectRouteRepoStub) GetProjectRole(context.Context, int64, int64) (string, bool, error) {
	return service.ProjectRoleAdmin, true, nil
}

func (r *projectRouteRepoStub) ProjectExists(context.Context, int64) (bool, error) {
	return true, nil
}

func (r *projectRouteRepoStub) ListActiveProjects(context.Context) ([]service.ProjectSummary, error) {
	return r.listUserProjects, nil
}

func (r *projectRouteRepoStub) ListUserProjects(context.Context, int64) ([]service.ProjectSummary, error) {
	return r.listUserProjects, nil
}

func (r *projectRouteRepoStub) CreateProject(_ context.Context, input service.ProjectCreateInput) (*service.ProjectSummary, error) {
	r.createProjectCalled = true
	r.projectInput = input
	return &service.ProjectSummary{ID: 11, Name: input.Name, Slug: input.Slug, Description: input.Description}, nil
}

func (r *projectRouteRepoStub) UpdateProject(context.Context, int64, service.ProjectUpdateInput) (*service.ProjectSummary, error) {
	panic("unexpected UpdateProject")
}

func (r *projectRouteRepoStub) ListProjectMembers(context.Context, int64) ([]service.ProjectMember, error) {
	return append([]service.ProjectMember(nil), r.members...), nil
}

func (r *projectRouteRepoStub) SetProjectMember(_ context.Context, projectID int64, input service.ProjectMemberInput) (*service.ProjectMember, error) {
	r.setMemberCalled = true
	r.memberInput = input
	return &service.ProjectMember{
		ProjectID: projectID,
		UserID:    input.UserID,
		Role:      input.Role,
		Status:    service.StatusActive,
	}, nil
}

func (r *projectRouteRepoStub) RemoveProjectMember(context.Context, int64, int64) error {
	panic("unexpected RemoveProjectMember")
}

func (r *projectRouteRepoStub) ListProjectProfiles(context.Context, int64) ([]service.ProjectProfile, error) {
	return r.listProfiles, nil
}

func (r *projectRouteRepoStub) CreateProjectProfile(_ context.Context, projectID int64, input service.ProjectProfileInput) (*service.ProjectProfile, error) {
	r.createProfileCalled = true
	r.profileInput = input
	name := "profile"
	if input.Name != nil {
		name = *input.Name
	}
	return &service.ProjectProfile{ID: 3, ProjectID: projectID, Name: name, Mode: service.ProjectProfileModeRestricted}, nil
}

func (r *projectRouteRepoStub) UpdateProjectProfile(_ context.Context, projectID int64, profileID int64, input service.ProjectProfileInput) (*service.ProjectProfile, error) {
	r.updateProfileCalled = true
	r.profileInput = input
	name := "profile"
	if input.Name != nil {
		name = *input.Name
	}
	return &service.ProjectProfile{ID: profileID, ProjectID: projectID, Name: name, Mode: service.ProjectProfileModeRestricted}, nil
}

func (r *projectRouteRepoStub) DeleteProjectProfile(context.Context, int64, int64) error {
	r.deleteProfileCalled = true
	return nil
}

func (r *projectRouteRepoStub) ActivateProjectProfile(context.Context, int64, int64) (*service.ProjectProfile, error) {
	r.activateProfileCalled = true
	if r.activateProfile != nil {
		return r.activateProfile, nil
	}
	return &service.ProjectProfile{ID: 2, ProjectID: 1, Name: "Restricted", Mode: service.ProjectProfileModeRestricted, IsActive: true}, nil
}

func (r *projectRouteRepoStub) ActivateProjectUnrestrictedScope(context.Context, int64) (*service.ProjectProfile, error) {
	r.activateUnrestrictedCalled = true
	return &service.ProjectProfile{ID: 99, ProjectID: 1, Name: "Unrestricted", Mode: service.ProjectProfileModeUnrestricted, IsActive: true}, nil
}

func (r *projectRouteRepoStub) GetProjectProfileBindings(context.Context, int64, int64) (*service.ProjectProfileBindings, error) {
	return &service.ProjectProfileBindings{ProfileID: 2}, nil
}

func (r *projectRouteRepoStub) SetProjectProfileBindings(_ context.Context, _ int64, profileID int64, _ service.ProjectProfileBindingInput) (*service.ProjectProfileBindings, error) {
	r.setBindingsCalled = true
	return &service.ProjectProfileBindings{ProfileID: profileID}, nil
}

func (r *projectRouteRepoStub) ValidateProjectProfileBindingScope(_ context.Context, _ int64, input service.ProjectProfileBindingInput) error {
	r.validateBindingScopeCalled = true
	r.scopeInput = input
	return r.validateBindingScopeErr
}

func (r *projectRouteRepoStub) ValidateProjectProfileBindingResources(context.Context, service.ProjectProfileBindingInput) error {
	return nil
}

func (r *projectRouteRepoStub) SearchProjectBindableResources(_ context.Context, projectID int64, query string, limit int) (*service.ProjectResourceSearchResult, error) {
	r.searchCalled = true
	r.searchProjectID = projectID
	r.searchQuery = query
	r.searchLimit = limit
	return &service.ProjectResourceSearchResult{
		Users:         []service.ProjectResourceUserCandidate{},
		Groups:        []service.ProjectResourceGroupCandidate{},
		Accounts:      []service.ProjectResourceAccountCandidate{},
		Subscriptions: []service.ProjectResourceSubscriptionCandidate{},
		APIKeys:       []service.ProjectResourceAPIKeyCandidate{},
	}, nil
}

type apiKeyRouteAdminServiceStub struct {
	service.AdminService
	updatedKeyID      int64
	updatedGroupID    *int64
	transferProjectID int64
}

func (s *apiKeyRouteAdminServiceStub) AdminUpdateAPIKeyGroupID(_ context.Context, keyID int64, groupID *int64) (*service.AdminUpdateAPIKeyGroupIDResult, error) {
	s.updatedKeyID = keyID
	if groupID != nil {
		gid := *groupID
		s.updatedGroupID = &gid
	}
	key := &service.APIKey{ID: keyID, UserID: 7, Key: "sk-test", Name: "test", Status: service.StatusAPIKeyActive, GroupID: s.updatedGroupID}
	return &service.AdminUpdateAPIKeyGroupIDResult{APIKey: key}, nil
}

func (s *apiKeyRouteAdminServiceStub) AdminTransferAPIKeyProject(_ context.Context, keyID int64, projectID int64) (*service.APIKey, error) {
	s.updatedKeyID = keyID
	s.transferProjectID = projectID
	key := &service.APIKey{ID: keyID, UserID: 7, ProjectID: projectID, Key: "sk-test", Name: "test", Status: service.StatusAPIKeyActive}
	return key, nil
}

func (s *apiKeyRouteAdminServiceStub) AdminResetAPIKeyRateLimitUsage(_ context.Context, keyID int64) (*service.APIKey, error) {
	return &service.APIKey{ID: keyID, UserID: 7, Key: "sk-test", Name: "test", Status: service.StatusAPIKeyActive}, nil
}
