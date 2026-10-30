package handler

import (
	"github.com/gin-gonic/gin"

	"livems/internal/pkg/response"
	"livems/internal/service"
)

// SettingsHandler 系统设置接口。
type SettingsHandler struct {
	settingsSvc *service.SettingsService
}

// NewSettingsHandler 创建系统设置处理器。
func NewSettingsHandler(settingsSvc *service.SettingsService) *SettingsHandler {
	return &SettingsHandler{settingsSvc: settingsSvc}
}

// GetIngress 处理读取对外接入配置。
//
// GET /api/v1/settings/ingress
func (h *SettingsHandler) GetIngress(c *gin.Context) {
	response.OK(c, h.settingsSvc.GetIngressView())
}

// SaveIngress 处理保存对外接入配置（实时生效，无需重启）。
//
// PUT /api/v1/settings/ingress
func (h *SettingsHandler) SaveIngress(c *gin.Context) {
	var req service.IngressConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	if err := h.settingsSvc.SaveIngress(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "接入配置已保存并实时生效", h.settingsSvc.GetIngressView())
}
