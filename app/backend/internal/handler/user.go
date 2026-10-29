package handler

import (
	"github.com/gin-gonic/gin"

	"livems/internal/pkg/response"
	"livems/internal/repository"
	"livems/internal/service"
)

// UserHandler 用户与角色权限接口。
type UserHandler struct {
	userSvc *service.UserService
}

// NewUserHandler 创建用户处理器。
func NewUserHandler(userSvc *service.UserService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

// List 处理用户列表。
//
// GET /api/v1/users
func (h *UserHandler) List(c *gin.Context) {
	page, pageSize := parsePaging(c)
	list, total, err := h.userSvc.List(page, pageSize, c.Query("keyword"))
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, response.Page{List: list, Total: total, Page: page, PageSize: pageSize})
}

// Get 处理用户详情。
//
// GET /api/v1/users/:id
func (h *UserHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	u, err := h.userSvc.Get(id)
	if err != nil {
		response.NotFound(c, "用户不存在")
		return
	}
	response.OK(c, u)
}

// Create 处理新增用户。
//
// POST /api/v1/users
func (h *UserHandler) Create(c *gin.Context) {
	var req service.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	u, err := h.userSvc.Create(&req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "创建成功", u)
}

// Update 处理更新用户。
//
// PUT /api/v1/users/:id
func (h *UserHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req service.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	if err := h.userSvc.Update(id, &req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "更新成功", nil)
}

// ResetPassword 处理重置密码。
//
// POST /api/v1/users/:id/password
func (h *UserHandler) ResetPassword(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req struct {
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	if err := h.userSvc.ResetPassword(id, req.NewPassword); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "密码已重置", nil)
}

// Delete 处理删除用户。
//
// DELETE /api/v1/users/:id
func (h *UserHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.userSvc.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "删除成功", nil)
}

// ListRoles 处理角色列表。
//
// GET /api/v1/roles
func (h *UserHandler) ListRoles(c *gin.Context) {
	list, err := h.userSvc.ListRoles()
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, list)
}

// ListPermissions 处理权限列表。
//
// GET /api/v1/permissions
func (h *UserHandler) ListPermissions(c *gin.Context) {
	list, err := h.userSvc.ListPermissions()
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, list)
}

// CreateRole 处理新增角色。
//
// POST /api/v1/roles
func (h *UserHandler) CreateRole(c *gin.Context) {
	var req service.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	role, err := h.userSvc.CreateRole(&req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "创建成功", role)
}

// UpdateRole 处理更新角色。
//
// PUT /api/v1/roles/:id
func (h *UserHandler) UpdateRole(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req service.CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	if err := h.userSvc.UpdateRole(id, &req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "更新成功", nil)
}

// DeleteRole 处理删除角色。
//
// DELETE /api/v1/roles/:id
func (h *UserHandler) DeleteRole(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.userSvc.DeleteRole(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "删除成功", nil)
}

// RolePermissions 处理查询角色已分配的权限。
//
// GET /api/v1/roles/:id/permissions
func (h *UserHandler) RolePermissions(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	ids, err := h.userSvc.RolePermissionIDs(id)
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, ids)
}

// LogHandler 日志审计接口。
type LogHandler struct {
	logRepo *repository.LogRepo
}

// NewLogHandler 创建日志处理器。
func NewLogHandler(logRepo *repository.LogRepo) *LogHandler {
	return &LogHandler{logRepo: logRepo}
}

// ListLogs 处理操作日志列表。
//
// GET /api/v1/logs
func (h *LogHandler) ListLogs(c *gin.Context) {
	page, pageSize := parsePaging(c)
	list, total, err := h.logRepo.ListLogs(page, pageSize, c.Query("module"), c.Query("username"))
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, response.Page{List: list, Total: total, Page: page, PageSize: pageSize})
}

// ListTaskErrors 处理任务错误日志列表。
//
// GET /api/v1/task-errors
func (h *LogHandler) ListTaskErrors(c *gin.Context) {
	page, pageSize := parsePaging(c)
	taskID := uint64(parseInt64(c.Query("task_id"), 0))
	list, total, err := h.logRepo.ListTaskErrors(page, pageSize, c.Query("task_type"), taskID)
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, response.Page{List: list, Total: total, Page: page, PageSize: pageSize})
}
