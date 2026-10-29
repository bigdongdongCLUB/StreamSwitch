// Package handler 实现 HTTP 接口层。
package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"livems/internal/middleware"
	"livems/internal/pkg/response"
	"livems/internal/service"
)

// AuthHandler 认证相关接口。
type AuthHandler struct {
	userSvc *service.UserService
}

// NewAuthHandler 创建认证处理器。
func NewAuthHandler(userSvc *service.UserService) *AuthHandler {
	return &AuthHandler{userSvc: userSvc}
}

// LoginRequest 登录请求。
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录响应。
type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	User      any    `json:"user"`
}

// Login 处理管理员登录。
//
// POST /api/v1/auth/login
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	ip := middleware.GetClientIP(c)
	if need, _ := h.userSvc.NeedSetup(); need {
		response.Fail(c, response.CodeFail, "系统尚未初始化，请先注册超级管理员")
		return
	}
	token, expireAt, user, err := h.userSvc.Login(req.Username, req.Password, ip)
	if err != nil {
		response.Fail(c, response.CodeUnauthorized, err.Error())
		return
	}
	// 返回用户信息（含权限）
	full, _ := h.userSvc.Get(user.ID)
	response.OK(c, LoginResponse{
		Token:     token,
		ExpiresAt: expireAt,
		User:      full,
	})
}

// SetupStatus 返回系统是否需要初始化（注册首个超级管理员）。
//
// GET /api/v1/setup/status
func (h *AuthHandler) SetupStatus(c *gin.Context) {
	need, err := h.userSvc.NeedSetup()
	if err != nil {
		response.ServerError(c, "查询初始化状态失败: "+err.Error())
		return
	}
	response.OK(c, gin.H{"need_setup": need})
}

// SetupRequest 注册首个超级管理员的请求。
type SetupRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Nickname string `json:"nickname"`
}

// Setup 注册首个超级管理员并直接登录（仅系统没有任何用户时可用）。
//
// POST /api/v1/setup
func (h *AuthHandler) Setup(c *gin.Context) {
	var req SetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请填写用户名和密码")
		return
	}
	token, expireAt, user, err := h.userSvc.SetupFirstAdmin(req.Username, req.Password, req.Nickname, middleware.GetClientIP(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	full, _ := h.userSvc.Get(user.ID)
	response.OKMsg(c, "初始化完成，已登录", LoginResponse{Token: token, ExpiresAt: expireAt, User: full})
}

// Profile 处理获取当前登录用户信息。
//
// GET /api/v1/auth/profile
func (h *AuthHandler) Profile(c *gin.Context) {
	userID := middleware.CurrentUserID(c)
	if userID == 0 {
		response.Unauthorized(c, "未登录")
		return
	}
	u, err := h.userSvc.Get(userID)
	if err != nil {
		response.NotFound(c, "用户不存在")
		return
	}
	response.OK(c, u)
}

// ChangePasswordRequest 修改密码请求。
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// ChangePassword 处理修改当前用户密码。
//
// POST /api/v1/auth/password
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	userID := middleware.CurrentUserID(c)
	// 校验旧密码
	if _, _, _, err := h.userSvc.Login(middleware.CurrentUsername(c), req.OldPassword, ""); err != nil {
		response.Fail(c, response.CodeUnauthorized, "旧密码错误")
		return
	}
	if err := h.userSvc.ResetPassword(userID, req.NewPassword); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "密码修改成功", nil)
}

// Logout 处理退出登录。
//
// POST /api/v1/auth/logout
//
// 说明：令牌为无状态 JWT，服务端不维护会话；
// 客户端丢弃本地令牌即可。保留该接口便于前端统一调用。
func (h *AuthHandler) Logout(c *gin.Context) {
	response.OKMsg(c, "已退出登录", nil)
}

// parseID 解析路径中的 ID 参数。
func parseID(c *gin.Context, name string) (uint64, bool) {
	v := c.Param(name)
	id, err := strconv.ParseUint(v, 10, 64)
	if err != nil || id == 0 {
		response.BadRequest(c, "无效的 ID 参数")
		return 0, false
	}
	return id, true
}

// parsePaging 解析分页参数。
func parsePaging(c *gin.Context) (page, pageSize int) {
	page = int(parseInt64(c.Query("page"), 1))
	pageSize = int(parseInt64(c.Query("page_size"), 20))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return page, pageSize
}

// parseInt64 解析 int64 查询参数。
func parseInt64(s string, def int64) int64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}

// parseStatus 解析状态过滤参数（-1 表示全部）。
func parseStatus(c *gin.Context) int {
	s := c.Query("status")
	if s == "" {
		return -1
	}
	return int(parseInt64(s, -1))
}

// trimStrings 去除字符串切片中每项的首尾空白并丢弃空项。
func trimStrings(list []string) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
