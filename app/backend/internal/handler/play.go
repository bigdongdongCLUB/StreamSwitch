package handler

import (
	"github.com/gin-gonic/gin"

	"livems/internal/middleware"
	"livems/internal/models"
	"livems/internal/pkg/response"
	"livems/internal/service"
)

// PlayHandler 播放域名与格式开关接口。
type PlayHandler struct {
	playSvc *service.PlayService
	authSvc *service.AuthService
}

// NewPlayHandler 创建播放配置处理器。
func NewPlayHandler(playSvc *service.PlayService, authSvc *service.AuthService) *PlayHandler {
	return &PlayHandler{playSvc: playSvc, authSvc: authSvc}
}

// DomainRequest 播放域名的请求参数。
type DomainRequest struct {
	Name      string `json:"name" binding:"required"`
	Schema    string `json:"schema"`
	Host      string `json:"host" binding:"required"`
	Port      int    `json:"port"`
	IsDefault int    `json:"is_default"`
	Status    int    `json:"status"`
	Sort      int    `json:"sort"`
	Remark    string `json:"remark"`
}

// ListDomains 处理播放域名列表。
//
// GET /api/v1/domains
func (h *PlayHandler) ListDomains(c *gin.Context) {
	list, err := h.playSvc.ListDomains()
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, list)
}

// CreateDomain 处理新增播放域名。
//
// POST /api/v1/domains
func (h *PlayHandler) CreateDomain(c *gin.Context) {
	var req DomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	d := &models.PlayDomain{
		Name:      req.Name,
		Schema:    req.Schema,
		Host:      req.Host,
		Port:      req.Port,
		IsDefault: req.IsDefault,
		Status:    req.Status,
		Sort:      req.Sort,
		Remark:    req.Remark,
	}
	if err := h.playSvc.CreateDomain(d); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "创建成功", d)
}

// UpdateDomain 处理更新播放域名。
//
// PUT /api/v1/domains/:id
func (h *PlayHandler) UpdateDomain(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req DomainRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	d := &models.PlayDomain{
		ID:        id,
		Name:      req.Name,
		Schema:    req.Schema,
		Host:      req.Host,
		Port:      req.Port,
		IsDefault: req.IsDefault,
		Status:    req.Status,
		Sort:      req.Sort,
		Remark:    req.Remark,
	}
	if err := h.playSvc.UpdateDomain(d); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "更新成功", nil)
}

// SetDefaultDomain 处理设置默认播放域名。
//
// POST /api/v1/domains/:id/default
func (h *PlayHandler) SetDefaultDomain(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.playSvc.SetDefaultDomain(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "已设为默认域名", nil)
}

// DeleteDomain 处理删除播放域名。
//
// DELETE /api/v1/domains/:id
func (h *PlayHandler) DeleteDomain(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.playSvc.DeleteDomain(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "删除成功", nil)
}

// ListFormats 处理播放格式开关列表。
//
// GET /api/v1/formats
func (h *PlayHandler) ListFormats(c *gin.Context) {
	list, err := h.playSvc.ListFormats()
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, list)
}

// SetFormatEnabled 处理开启/关闭某个播放格式。
//
// POST /api/v1/formats/:format/toggle
func (h *PlayHandler) SetFormatEnabled(c *gin.Context) {
	format := c.Param("format")
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	if err := h.playSvc.SetFormatEnabled(format, req.Enabled); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	msg := "格式已开启"
	if !req.Enabled {
		msg = "格式已关闭"
	}
	response.OKMsg(c, msg, gin.H{"format": format, "enabled": req.Enabled})
}

// GetPolicy 处理获取鉴权策略。
//
// GET /api/v1/auth-policy
func (h *PlayHandler) GetPolicy(c *gin.Context) {
	response.OK(c, h.authSvc.Policy())
}

// SetPolicy 处理更新鉴权策略（实时生效，无需重启）。
//
// PUT /api/v1/auth-policy
func (h *PlayHandler) SetPolicy(c *gin.Context) {
	var req service.AuthPolicy
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	if req.ParamName == "" {
		req.ParamName = "token"
	}
	if err := h.authSvc.SavePolicy(&req); err != nil {
		response.ServerError(c, "保存失败: "+err.Error())
		return
	}
	response.OKMsg(c, "策略已保存并实时生效", h.authSvc.Policy())
}

// TokenHandler 播放令牌接口。
type TokenHandler struct {
	authSvc *service.AuthService
}

// NewTokenHandler 创建令牌处理器。
func NewTokenHandler(authSvc *service.AuthService) *TokenHandler {
	return &TokenHandler{authSvc: authSvc}
}

// CreateTokenRequest 生成令牌的请求参数。
type CreateTokenRequest struct {
	StreamID  uint64 `json:"stream_id"`
	ExpireSec int    `json:"expire_sec"`
	IPBind    string `json:"ip_bind"`
	Format    string `json:"format"`
}

// CreateToken 处理生成播放令牌。
//
// POST /api/v1/tokens
func (h *TokenHandler) CreateToken(c *gin.Context) {
	var req CreateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	t, err := h.authSvc.CreateToken(req.StreamID, req.ExpireSec, req.IPBind, req.Format)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "令牌生成成功", t)
}

// ListTokens 处理令牌列表。
//
// GET /api/v1/tokens
func (h *TokenHandler) ListTokens(c *gin.Context) {
	page, pageSize := parsePaging(c)
	list, total, err := h.authSvc.List(page, pageSize)
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, response.Page{List: list, Total: total, Page: page, PageSize: pageSize})
}

// DeleteToken 处理删除令牌。
//
// DELETE /api/v1/tokens/:id
func (h *TokenHandler) DeleteToken(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.authSvc.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "删除成功", nil)
}

// EncryptHandler HLS 加密配置接口。
type EncryptHandler struct {
	encryptSvc *service.EncryptService
}

// NewEncryptHandler 创建加密配置处理器。
func NewEncryptHandler(encryptSvc *service.EncryptService) *EncryptHandler {
	return &EncryptHandler{encryptSvc: encryptSvc}
}

// List 处理加密配置列表。
//
// GET /api/v1/encrypts
func (h *EncryptHandler) List(c *gin.Context) {
	list, err := h.encryptSvc.List()
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, list)
}

// Get 处理查询单个流的加密配置。
//
// GET /api/v1/encrypts/:stream_id
func (h *EncryptHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "stream_id")
	if !ok {
		return
	}
	e, err := h.encryptSvc.GetConfig(id)
	if err != nil {
		response.NotFound(c, "加密配置不存在")
		return
	}
	response.OK(c, e)
}

// Set 处理设置 HLS 加密配置。
//
// PUT /api/v1/encrypts/:stream_id
func (h *EncryptHandler) Set(c *gin.Context) {
	id, ok := parseID(c, "stream_id")
	if !ok {
		return
	}
	var req service.EncryptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	e, err := h.encryptSvc.SetConfig(id, &req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "加密配置已保存", e)
}

// Rotate 处理轮换密钥。
//
// POST /api/v1/encrypts/:stream_id/rotate
func (h *EncryptHandler) Rotate(c *gin.Context) {
	id, ok := parseID(c, "stream_id")
	if !ok {
		return
	}
	if err := h.encryptSvc.RotateKey(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "密钥已轮换", nil)
}

// Delete 处理删除加密配置。
//
// DELETE /api/v1/encrypts/:stream_id
func (h *EncryptHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "stream_id")
	if !ok {
		return
	}
	if err := h.encryptSvc.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "删除成功", nil)
}

// 确保 middleware 包被引用。
var _ = middleware.CurrentUserID
