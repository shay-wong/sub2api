package admin

import (
	"context"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type ProjectHandler struct {
	projectService *service.ProjectService
}

func NewProjectHandler(projectService *service.ProjectService) *ProjectHandler {
	return &ProjectHandler{projectService: projectService}
}

type createProjectRequest struct {
	Name            string  `json:"name" binding:"required"`
	Slug            string  `json:"slug" binding:"required"`
	Description     *string `json:"description"`
	ProfileMode     string  `json:"profile_mode"`
	GroupIDs        []int64 `json:"group_ids"`
	AccountIDs      []int64 `json:"account_ids"`
	SubscriptionIDs []int64 `json:"subscription_ids"`
}

type updateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
}

type setProjectMemberRequest struct {
	Role        string   `json:"role" binding:"required"`
	IsOwner     bool     `json:"is_owner"`
	Status      *string  `json:"status"`
	Permissions []string `json:"permissions"`
}

type projectProfileRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Mode        *string `json:"mode"`
}

type setProjectProfileBindingsRequest struct {
	GroupIDs        []int64 `json:"group_ids"`
	AccountIDs      []int64 `json:"account_ids"`
	SubscriptionIDs []int64 `json:"subscription_ids"`
}

func (h *ProjectHandler) List(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Authorization required")
		return
	}
	role, _ := middleware.GetUserRoleFromContext(c)
	user := &service.User{ID: subject.UserID, Role: role}
	projects, err := h.projectService.ListUserProjects(c.Request.Context(), user)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, projects)
}

func (h *ProjectHandler) Create(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	var req createProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.Unauthorized(c, "Authorization required")
		return
	}
	project, err := h.projectService.CreateProject(c.Request.Context(), service.ProjectCreateInput{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		OwnerUserID: subject.UserID,
		ProfileMode: req.ProfileMode,
		Bindings: service.ProjectProfileBindingInput{
			GroupIDs:        req.GroupIDs,
			AccountIDs:      req.AccountIDs,
			SubscriptionIDs: req.SubscriptionIDs,
		},
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, project)
}

func (h *ProjectHandler) Update(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	var req updateProjectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	project, err := h.projectService.UpdateProject(c.Request.Context(), projectID, service.ProjectUpdateInput{
		Name:        req.Name,
		Description: req.Description,
		Status:      req.Status,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, project)
}

func (h *ProjectHandler) ListMembers(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	if !authorizeProjectPath(c, projectID) {
		return
	}
	members, err := h.projectService.ListProjectMembers(c.Request.Context(), projectID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, members)
}

func (h *ProjectHandler) SetMember(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	if !authorizeProjectPath(c, projectID) {
		return
	}
	userID, ok := parseUserIDParam(c)
	if !ok {
		return
	}
	var req setProjectMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !service.RoleIsSuperAdmin(currentAdminRole(c)) {
		member, allowed, err := h.canProjectAdminUpdateMemberStatus(c.Request.Context(), projectID, userID, req)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if !allowed {
			response.Forbidden(c, "Project access forbidden")
			return
		}
		response.Success(c, member)
		return
	}
	member, err := h.projectService.SetProjectMember(c.Request.Context(), projectID, service.ProjectMemberInput{
		UserID:      userID,
		Role:        req.Role,
		IsOwner:     req.IsOwner,
		Status:      req.Status,
		Permissions: req.Permissions,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, member)
}

func (h *ProjectHandler) ListProfiles(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	if !authorizeProjectPath(c, projectID) {
		return
	}
	profiles, err := h.projectService.ListProjectProfiles(c.Request.Context(), projectID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, profiles)
}

func (h *ProjectHandler) CreateProfile(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	if !authorizeProjectPath(c, projectID) {
		return
	}
	var req projectProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Mode != nil {
		response.BadRequest(c, "Profile mode cannot be set on an application profile")
		return
	}
	profile, err := h.projectService.CreateProjectProfile(c.Request.Context(), projectID, service.ProjectProfileInput{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, profile)
}

func (h *ProjectHandler) UpdateProfile(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	if !authorizeProjectPath(c, projectID) {
		return
	}
	profileID, ok := parseProfileIDParam(c)
	if !ok {
		return
	}
	var req projectProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Mode != nil {
		response.BadRequest(c, "Profile mode cannot be set on an application profile")
		return
	}
	profile, err := h.projectService.UpdateProjectProfile(c.Request.Context(), projectID, profileID, service.ProjectProfileInput{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, profile)
}

func (h *ProjectHandler) DeleteProfile(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	if !authorizeProjectPath(c, projectID) {
		return
	}
	profileID, ok := parseProfileIDParam(c)
	if !ok {
		return
	}
	if err := h.projectService.DeleteProjectProfile(c.Request.Context(), projectID, profileID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "success"})
}

func (h *ProjectHandler) ActivateProfile(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	if !authorizeProjectPath(c, projectID) {
		return
	}
	profileID, ok := parseProfileIDParam(c)
	if !ok {
		return
	}
	profile, err := h.projectService.ActivateProjectProfile(c.Request.Context(), projectID, profileID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, profile)
}

func (h *ProjectHandler) ActivateUnrestrictedScope(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	if !authorizeProjectPath(c, projectID) {
		return
	}
	profile, err := h.projectService.ActivateProjectUnrestrictedScope(c.Request.Context(), projectID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, profile)
}

func (h *ProjectHandler) GetProfileBindings(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	if !authorizeProjectPath(c, projectID) {
		return
	}
	profileID, ok := parseProfileIDParam(c)
	if !ok {
		return
	}
	bindings, err := h.projectService.GetProjectProfileBindings(c.Request.Context(), projectID, profileID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, bindings)
}

func (h *ProjectHandler) SetProfileBindings(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	if !authorizeProjectPath(c, projectID) {
		return
	}
	profileID, ok := parseProfileIDParam(c)
	if !ok {
		return
	}
	var req setProjectProfileBindingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if !service.RoleIsSuperAdmin(currentAdminRole(c)) {
		mode, err := h.projectProfileMode(c, projectID, profileID)
		if err != nil {
			response.ErrorFrom(c, err)
			return
		}
		if mode == service.ProjectProfileModeUnrestricted {
			response.Forbidden(c, "Project access forbidden")
			return
		}
		if err := h.projectService.ValidateProjectProfileBindingScope(c.Request.Context(), projectID, service.ProjectProfileBindingInput{
			GroupIDs:        req.GroupIDs,
			AccountIDs:      req.AccountIDs,
			SubscriptionIDs: req.SubscriptionIDs,
		}); err != nil {
			response.ErrorFrom(c, err)
			return
		}
	}
	bindings, err := h.projectService.SetProjectProfileBindings(c.Request.Context(), projectID, profileID, service.ProjectProfileBindingInput{
		GroupIDs:        req.GroupIDs,
		AccountIDs:      req.AccountIDs,
		SubscriptionIDs: req.SubscriptionIDs,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, bindings)
}

func (h *ProjectHandler) SearchBindableResources(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	if !authorizeProjectPath(c, projectID) {
		return
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	result, err := h.projectService.SearchProjectBindableResources(c.Request.Context(), projectID, c.Query("q"), limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ProjectHandler) SearchGlobalBindableResources(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	role, _ := middleware.GetUserRoleFromContext(c)
	if !service.RoleIsSuperAdmin(role) {
		response.Forbidden(c, "Project access forbidden")
		return
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	result, err := h.projectService.SearchProjectBindableResources(c.Request.Context(), 0, c.Query("q"), limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func (h *ProjectHandler) RemoveMember(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	if !authorizeProjectPath(c, projectID) {
		return
	}
	userID, ok := parseUserIDParam(c)
	if !ok {
		return
	}
	if err := h.projectService.RemoveProjectMember(c.Request.Context(), projectID, userID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "success"})
}

func parseProjectIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid project ID")
		return 0, false
	}
	return id, true
}

func parseUserIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("user_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid user ID")
		return 0, false
	}
	return id, true
}

func (h *ProjectHandler) canProjectAdminUpdateMemberStatus(ctx context.Context, projectID int64, userID int64, req setProjectMemberRequest) (*service.ProjectMember, bool, error) {
	if req.Status == nil || req.IsOwner || len(req.Permissions) > 0 {
		return nil, false, nil
	}
	if !adminContextHasPermission(ctx, service.AdminPermissionUsersManage) {
		return nil, false, nil
	}
	members, err := h.projectService.ListProjectMembers(ctx, projectID)
	if err != nil {
		return nil, false, err
	}
	for _, member := range members {
		if member.UserID != userID {
			continue
		}
		if member.IsOwner || member.Role != service.ProjectRoleUser || req.Role != service.ProjectRoleUser {
			return nil, false, nil
		}
		updated, err := h.projectService.SetProjectMember(ctx, projectID, service.ProjectMemberInput{
			UserID: userID,
			Role:   service.ProjectRoleUser,
			Status: req.Status,
		})
		if err != nil {
			return nil, false, err
		}
		return updated, true, nil
	}
	return nil, false, nil
}

func adminContextHasPermission(ctx context.Context, permission string) bool {
	permissions, ok := service.AdminPermissionsFromContext(ctx)
	if !ok {
		return false
	}
	for _, item := range permissions {
		if item == permission {
			return true
		}
	}
	return false
}

func parseProfileIDParam(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("profile_id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid profile ID")
		return 0, false
	}
	return id, true
}

func authorizeProjectPath(c *gin.Context, projectID int64) bool {
	if service.RoleIsSuperAdmin(currentAdminRole(c)) {
		return true
	}
	currentProjectID, ok := service.ProjectIDFromContext(c.Request.Context())
	if ok && currentProjectID == projectID {
		return true
	}
	response.Forbidden(c, "Project access forbidden")
	return false
}

func currentAdminRole(c *gin.Context) string {
	role, _ := middleware.GetUserRoleFromContext(c)
	return role
}

func (h *ProjectHandler) projectProfileMode(c *gin.Context, projectID int64, profileID int64) (string, error) {
	profiles, err := h.projectService.ListProjectProfiles(c.Request.Context(), projectID)
	if err != nil {
		return "", err
	}
	for _, profile := range profiles {
		if profile.ID == profileID {
			return profile.Mode, nil
		}
	}
	return "", service.ErrProjectProfileNotFound
}
