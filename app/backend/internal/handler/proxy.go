package handler

import (
	"github.com/gin-gonic/gin"

	"livems/internal/middleware"
	"livems/internal/pkg/response"
	"livems/internal/service"
)

// ProxyHandler 拉流代理接口。
type ProxyHandler struct {
	proxySvc *service.ProxyService
}

// NewProxyHandler 创建拉流代理处理器。
func NewProxyHandler(proxySvc *service.ProxyService) *ProxyHandler {
	return &ProxyHandler{proxySvc: proxySvc}
}

// List 处理拉流代理列表。
//
// GET /api/v1/proxies
func (h *ProxyHandler) List(c *gin.Context) {
	page, pageSize := parsePaging(c)
	list, total, err := h.proxySvc.List(page, pageSize, c.Query("keyword"), parseStatus(c))
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, response.Page{List: list, Total: total, Page: page, PageSize: pageSize})
}

// Get 处理拉流代理详情。
//
// GET /api/v1/proxies/:id
func (h *ProxyHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	p, err := h.proxySvc.Get(id)
	if err != nil {
		response.NotFound(c, "任务不存在")
		return
	}
	response.OK(c, p)
}

// Create 处理新增拉流代理。
//
// POST /api/v1/proxies
func (h *ProxyHandler) Create(c *gin.Context) {
	var req service.CreateProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	p, err := h.proxySvc.Create(&req, middleware.CurrentUserID(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "创建成功", p)
}

// Update 处理更新拉流代理。
//
// PUT /api/v1/proxies/:id
func (h *ProxyHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req service.CreateProxyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	if err := h.proxySvc.Update(id, &req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "更新成功", nil)
}

// Toggle 处理启停拉流代理。
//
// POST /api/v1/proxies/:id/toggle
func (h *ProxyHandler) Toggle(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	status, err := h.proxySvc.Toggle(id)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	msg := "任务已启动"
	if status == 0 {
		msg = "任务已停止"
	}
	response.OKMsg(c, msg, gin.H{"status": status})
}

// Start 处理启动拉流代理。
//
// POST /api/v1/proxies/:id/start
func (h *ProxyHandler) Start(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.proxySvc.Start(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "任务已启动", nil)
}

// Stop 处理停止拉流代理。
//
// POST /api/v1/proxies/:id/stop
func (h *ProxyHandler) Stop(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.proxySvc.Stop(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "任务已停止", nil)
}

// Delete 处理删除拉流代理。
//
// DELETE /api/v1/proxies/:id
func (h *ProxyHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.proxySvc.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "删除成功", nil)
}

// ForwardHandler 转推流接口。
type ForwardHandler struct {
	forwardSvc *service.ForwardService
}

// NewForwardHandler 创建转推流处理器。
func NewForwardHandler(forwardSvc *service.ForwardService) *ForwardHandler {
	return &ForwardHandler{forwardSvc: forwardSvc}
}

// List 处理转推流列表。
//
// GET /api/v1/forwards
func (h *ForwardHandler) List(c *gin.Context) {
	page, pageSize := parsePaging(c)
	list, total, err := h.forwardSvc.List(page, pageSize, c.Query("keyword"), parseStatus(c))
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, response.Page{List: list, Total: total, Page: page, PageSize: pageSize})
}

// Get 处理转推流详情。
//
// GET /api/v1/forwards/:id
func (h *ForwardHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	f, err := h.forwardSvc.Get(id)
	if err != nil {
		response.NotFound(c, "任务不存在")
		return
	}
	response.OK(c, f)
}

// Create 处理新增转推流。
//
// POST /api/v1/forwards
func (h *ForwardHandler) Create(c *gin.Context) {
	var req service.CreateForwardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	f, err := h.forwardSvc.Create(&req, middleware.CurrentUserID(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "创建成功", f)
}

// Update 处理更新转推流。
//
// PUT /api/v1/forwards/:id
func (h *ForwardHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req service.CreateForwardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	if err := h.forwardSvc.Update(id, &req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "更新成功", nil)
}

// Toggle 处理启停转推流。
//
// POST /api/v1/forwards/:id/toggle
func (h *ForwardHandler) Toggle(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	status, err := h.forwardSvc.Toggle(id)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	msg := "任务已启动"
	if status == 0 {
		msg = "任务已停止"
	}
	response.OKMsg(c, msg, gin.H{"status": status})
}

// Delete 处理删除转推流。
//
// DELETE /api/v1/forwards/:id
func (h *ForwardHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.forwardSvc.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "删除成功", nil)
}
