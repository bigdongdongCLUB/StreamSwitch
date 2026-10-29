package handler

import (
	"github.com/gin-gonic/gin"

	"livems/internal/pkg/logger"
	"livems/internal/pkg/response"
	"livems/internal/service"
	"livems/internal/zlm"
)

// HookHandler 处理 ZLMediaKit 的回调事件。
//
// ZLMediaKit 会向以下地址发送 POST 请求（需在 ZLM 的 config.ini 中配置）：
//
//	POST /index/hook/on_publish        推流鉴权
//	POST /index/hook/on_play           播放鉴权
//	POST /index/hook/on_stream_changed 流上下线
//	POST /index/hook/on_flow_report    流量统计
//	POST /index/hook/on_stream_not_found 流不存在
//
// 所有回调统一返回 ZLMediaKit 约定的格式：{"code":0,"msg":"success"}
// code 非 0 表示拒绝该操作。
type HookHandler struct {
	hookSvc *service.HookService
}

// NewHookHandler 创建回调处理器。
func NewHookHandler(hookSvc *service.HookService) *HookHandler {
	return &HookHandler{hookSvc: hookSvc}
}

// OnPublish 处理推流事件。
//
// POST /index/hook/on_publish
func (h *HookHandler) OnPublish(c *gin.Context) {
	var ev zlm.PublishEvent
	if err := c.ShouldBindJSON(&ev); err != nil {
		logger.Warn("on_publish 解析请求体失败: %v", err)
		// 解析失败时拒绝推流，避免绕过鉴权
		c.JSON(200, zlm.Deny("请求格式错误"))
		return
	}
	reply := h.hookSvc.HandlePublish(&ev)
	c.JSON(200, reply)
}

// OnPlay 处理播放事件。
//
// POST /index/hook/on_play
func (h *HookHandler) OnPlay(c *gin.Context) {
	var ev zlm.PlayEvent
	if err := c.ShouldBindJSON(&ev); err != nil {
		logger.Warn("on_play 解析请求体失败: %v", err)
		c.JSON(200, zlm.Deny("请求格式错误"))
		return
	}
	reply := h.hookSvc.HandlePlay(&ev)
	c.JSON(200, reply)
}

// OnStreamChanged 处理流上下线事件。
//
// POST /index/hook/on_stream_changed
func (h *HookHandler) OnStreamChanged(c *gin.Context) {
	var ev zlm.StreamChangedEvent
	if err := c.ShouldBindJSON(&ev); err != nil {
		logger.Warn("on_stream_changed 解析请求体失败: %v", err)
		c.JSON(200, response.Body{Code: 0, Message: "success"})
		return
	}
	h.hookSvc.HandleStreamChanged(&ev)
	c.JSON(200, response.Body{Code: 0, Message: "success"})
}

// OnFlowReport 处理流量统计事件。
//
// POST /index/hook/on_flow_report
func (h *HookHandler) OnFlowReport(c *gin.Context) {
	var ev zlm.FlowReportEvent
	if err := c.ShouldBindJSON(&ev); err != nil {
		c.JSON(200, response.Body{Code: 0, Message: "success"})
		return
	}
	h.hookSvc.HandleFlowReport(&ev)
	c.JSON(200, response.Body{Code: 0, Message: "success"})
}

// OnStreamNotFound 处理流不存在事件。
//
// POST /index/hook/on_stream_not_found
func (h *HookHandler) OnStreamNotFound(c *gin.Context) {
	var ev zlm.StreamNotFoundEvent
	if err := c.ShouldBindJSON(&ev); err != nil {
		c.JSON(200, response.Body{Code: 0, Message: "success"})
		return
	}
	h.hookSvc.HandleStreamNotFound(&ev)
	c.JSON(200, response.Body{Code: 0, Message: "success"})
}

// OnStreamNoneReader 处理无人观看事件。
//
// POST /index/hook/on_stream_none_reader
func (h *HookHandler) OnStreamNoneReader(c *gin.Context) {
	c.JSON(200, response.Body{Code: 0, Message: "success"})
}

// OnServerStarted 处理 ZLM 启动通知。
//
// POST /index/hook/on_server_started
func (h *HookHandler) OnServerStarted(c *gin.Context) {
	logger.Info("收到 ZLMediaKit 启动通知")
	c.JSON(200, response.Body{Code: 0, Message: "success"})
}

// OnServerExited 处理 ZLM 退出通知。
//
// POST /index/hook/on_server_exited
func (h *HookHandler) OnServerExited(c *gin.Context) {
	logger.Warn("收到 ZLMediaKit 退出通知")
	c.JSON(200, response.Body{Code: 0, Message: "success"})
}
