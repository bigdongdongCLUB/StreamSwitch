package handler

import (
	"github.com/gin-gonic/gin"

	"livems/internal/pkg/response"
	"livems/internal/service"
)

// CertHandler SSL 证书管理接口。
type CertHandler struct {
	certSvc *service.CertService
}

// NewCertHandler 创建证书处理器。
func NewCertHandler(certSvc *service.CertService) *CertHandler {
	return &CertHandler{certSvc: certSvc}
}

// List 处理证书列表查询。
//
// GET /api/v1/certs
func (h *CertHandler) List(c *gin.Context) {
	list, err := h.certSvc.List()
	if err != nil {
		response.ServerError(c, "查询失败: "+err.Error())
		return
	}
	response.OK(c, list)
}

// SaveCertRequest 上传证书的请求参数。
type SaveCertRequest struct {
	Name      string `json:"name" binding:"required"`
	Fullchain string `json:"fullchain" binding:"required"`
	Privkey   string `json:"privkey" binding:"required"`
	Apply     bool   `json:"apply"` // 保存后立即应用到站点
}

// Save 处理上传/更新证书。
//
// POST /api/v1/certs
func (h *CertHandler) Save(c *gin.Context) {
	var req SaveCertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	info, err := h.certSvc.Save(req.Name, req.Fullchain, req.Privkey, req.Apply)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	msg := "证书已保存"
	if req.Apply {
		msg = "证书已保存并应用到站点"
	}
	response.OKMsg(c, msg, info)
}

// Apply 处理应用证书到站点。
//
// POST /api/v1/certs/:name/apply
func (h *CertHandler) Apply(c *gin.Context) {
	name := c.Param("name")
	if err := h.certSvc.Apply(name); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "证书已应用并重载 Nginx", nil)
}

// Delete 处理删除证书。
//
// DELETE /api/v1/certs/:name
func (h *CertHandler) Delete(c *gin.Context) {
	name := c.Param("name")
	if err := h.certSvc.Delete(name); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "删除成功", nil)
}
