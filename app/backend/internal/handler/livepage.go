package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"livems/internal/middleware"
	"livems/internal/pkg/response"
	"livems/internal/service"
)

// LivePageHandler 直播分享页接口。
type LivePageHandler struct {
	svc *service.LivePageService
}

// NewLivePageHandler 创建直播分享页处理器。
func NewLivePageHandler(svc *service.LivePageService) *LivePageHandler {
	return &LivePageHandler{svc: svc}
}

// ---------------------------------------------------------------------------
// 管理端（需登录 + livepage 权限）
// ---------------------------------------------------------------------------

// List 处理分页查询。
//
// GET /api/v1/live-pages
func (h *LivePageHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	list, total, err := h.svc.List(page, pageSize, c.Query("keyword"))
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, response.Page{List: list, Total: total, Page: page, PageSize: pageSize})
}

// Create 处理创建。
//
// POST /api/v1/live-pages
func (h *LivePageHandler) Create(c *gin.Context) {
	var req service.LivePageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	p, err := h.svc.Create(&req, middleware.CurrentUserID(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "创建成功", gin.H{"id": p.ID, "code": p.Code})
}

// Update 处理更新。
//
// PUT /api/v1/live-pages/:id
func (h *LivePageHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的ID")
		return
	}
	var req service.LivePageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.Update(id, &req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "更新成功", nil)
}

// Delete 处理删除。
//
// DELETE /api/v1/live-pages/:id
func (h *LivePageHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "无效的ID")
		return
	}
	if err := h.svc.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "删除成功", nil)
}

// ---------------------------------------------------------------------------
// 观众侧（免登录）
// ---------------------------------------------------------------------------

// PublicInfo 获取页面公开信息。
//
// GET /api/v1/p/:code
func (h *LivePageHandler) PublicInfo(c *gin.Context) {
	info, err := h.svc.GetPublicInfo(c.Param("code"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.OK(c, info)
}

// VerifyPassword 校验访问密码，返回观众访问凭证。
//
// POST /api/v1/p/:code/auth
func (h *LivePageHandler) VerifyPassword(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误")
		return
	}
	ticket, err := h.svc.VerifyPassword(c.Param("code"), req.Password)
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	response.OK(c, gin.H{"ticket": ticket})
}

// PlayData 下发播放数据（播放地址内含绑定该流的播放令牌）。
//
// GET /api/v1/p/:code/play?ticket=xxx
func (h *LivePageHandler) PlayData(c *gin.Context) {
	data, err := h.svc.GetPlayData(c.Request.Context(), c.Param("code"), c.Query("ticket"))
	if err != nil {
		response.Forbidden(c, err.Error())
		return
	}
	response.OK(c, data)
}
