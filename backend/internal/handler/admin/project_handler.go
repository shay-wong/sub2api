package admin

import (
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
	Name        string  `json:"name" binding:"required"`
	Slug        string  `json:"slug" binding:"required"`
	Description *string `json:"description"`
}

type updateProjectRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
}

type setProjectMemberRequest struct {
	Role    string `json:"role" binding:"required"`
	IsOwner bool   `json:"is_owner"`
}

type moveProjectResourcesRequest struct {
	AccountIDs       []int64 `json:"account_ids"`
	APIKeyIDs        []int64 `json:"api_key_ids"`
	GroupIDs         []int64 `json:"group_ids"`
	MoveUsageHistory *bool   `json:"move_usage_history"`
}

func (h *ProjectHandler) List(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projects, err := h.projectService.ListProjects(c.Request.Context())
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
	userID, ok := parseUserIDParam(c)
	if !ok {
		return
	}
	var req setProjectMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	member, err := h.projectService.SetProjectMember(c.Request.Context(), projectID, service.ProjectMemberInput{
		UserID:  userID,
		Role:    req.Role,
		IsOwner: req.IsOwner,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, member)
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

func (h *ProjectHandler) MoveResources(c *gin.Context) {
	if h.projectService == nil {
		response.InternalError(c, "Project service unavailable")
		return
	}
	projectID, ok := parseProjectIDParam(c)
	if !ok {
		return
	}
	var req moveProjectResourcesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	moveUsageHistory := true
	if req.MoveUsageHistory != nil {
		moveUsageHistory = *req.MoveUsageHistory
	}
	result, err := h.projectService.MoveProjectResources(c.Request.Context(), projectID, service.ProjectResourceMoveInput{
		AccountIDs:       req.AccountIDs,
		APIKeyIDs:        req.APIKeyIDs,
		GroupIDs:         req.GroupIDs,
		MoveUsageHistory: moveUsageHistory,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
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
