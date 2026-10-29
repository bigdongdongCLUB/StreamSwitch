package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"livems/internal/middleware"
	"livems/internal/models"
	"livems/internal/pkg/response"
	"livems/internal/service"
)

// StreamHandler 流管理接口。
type StreamHandler struct {
	streamSvc *service.StreamService
	playSvc   *service.PlayService
	authSvc   *service.AuthService
}

// NewStreamHandler 创建流处理器。
func NewStreamHandler(streamSvc *service.StreamService, playSvc *service.PlayService, authSvc *service.AuthService) *StreamHandler {
	return &StreamHandler{streamSvc: streamSvc, playSvc: playSvc, authSvc: authSvc}
}

// List 处理流列表查询。
//
// GET /api/v1/streams
func (h *StreamHandler) List(c *gin.Context) {
	page, pageSize := parsePaging(c)
	keyword := c.Query("keyword")
	status := parseStatus(c)

	list, total, err := h.streamSvc.List(page, pageSize, keyword, status)
	if err != nil {
		response.ServerError(c, "查询流列表失败: "+err.Error())
		return
	}
	response.OK(c, response.Page{
		List:     list,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// Get 处理流详情查询。
//
// GET /api/v1/streams/:id
func (h *StreamHandler) Get(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	st, err := h.streamSvc.Get(id)
	if err != nil {
		response.NotFound(c, "流不存在")
		return
	}
	response.OK(c, gin.H{
		"stream":      st,
		"publish_url": h.streamSvc.PublishURL(c.Request.Context(), st),
	})
}

// Create 处理新增流。
//
// POST /api/v1/streams
func (h *StreamHandler) Create(c *gin.Context) {
	var req service.CreateStreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	st, err := h.streamSvc.Create(&req, middleware.CurrentUserID(c))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "流创建成功", gin.H{
		"stream":      st,
		"publish_url": h.streamSvc.PublishURL(c.Request.Context(), st),
	})
}

// Update 处理更新流。
//
// PUT /api/v1/streams/:id
func (h *StreamHandler) Update(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	var req service.UpdateStreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	if err := h.streamSvc.Update(id, &req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "更新成功", nil)
}

// Toggle 处理启用/禁用流。
//
// POST /api/v1/streams/:id/toggle
func (h *StreamHandler) Toggle(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	status, err := h.streamSvc.Toggle(id)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	msg := "流已启用"
	if status == 0 {
		msg = "流已禁用"
	}
	response.OKMsg(c, msg, gin.H{"status": status})
}

// Delete 处理删除流。
//
// DELETE /api/v1/streams/:id
func (h *StreamHandler) Delete(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.streamSvc.Delete(id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OKMsg(c, "删除成功", nil)
}

// GenerateURLRequest 生成播放地址的请求参数。
type GenerateURLRequest struct {
	StreamID  uint64 `json:"stream_id" binding:"required"`
	DomainID  uint64 `json:"domain_id"`   // 0 表示使用默认域名
	ExpireSec int    `json:"expire_sec"`  // 令牌有效期，0 表示使用默认
	IPBind    string `json:"ip_bind"`     // IP 绑定，空表示不限制
	Format    string `json:"format"`      // 限定格式，空表示全部格式
	NeedToken bool   `json:"need_token"`  // 是否附带播放令牌
}

// GenerateURL 处理生成多格式播放地址。
//
// POST /api/v1/streams/urls
func (h *StreamHandler) GenerateURL(c *gin.Context) {
	var req GenerateURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "请求参数错误: "+err.Error())
		return
	}
	st, err := h.streamSvc.Get(req.StreamID)
	if err != nil {
		response.NotFound(c, "流不存在")
		return
	}

	// 生成令牌
	token := ""
	var expireAt int64
	if req.NeedToken {
		t, err := h.authSvc.CreateToken(req.StreamID, req.ExpireSec, req.IPBind, req.Format)
		if err != nil {
			response.BadRequest(c, err.Error())
			return
		}
		token = t.Token
		expireAt = t.ExpireAt
	}

	urls, err := h.playSvc.GenerateURLs(c.Request.Context(), st, req.DomainID, token, expireAt)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, urls)
}

// URLs 返回流的多格式播放地址（不生成令牌）。
//
// GET /api/v1/streams/:id/urls
func (h *StreamHandler) URLs(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	st, err := h.streamSvc.Get(id)
	if err != nil {
		response.NotFound(c, "流不存在")
		return
	}
	domainID := uint64(parseInt64(c.Query("domain_id"), 0))
	urls, err := h.playSvc.GenerateURLs(c.Request.Context(), st, domainID, "", 0)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.OK(c, urls)
}

// Sync 处理手动触发与 ZLMediaKit 的状态同步。
//
// POST /api/v1/streams/sync
func (h *StreamHandler) Sync(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := h.streamSvc.SyncFromZLM(ctx); err != nil {
		response.ServerError(c, "同步失败: "+err.Error())
		return
	}
	response.OKMsg(c, "同步完成", nil)
}

// StreamInfo 是流信息的展示视图。
type StreamInfo struct {
	*models.Stream
	PublishURL   string `json:"publish_url"`
	OnlineText   string `json:"online_text"`
	PublishTimeText string `json:"publish_time_text"`
}

// Info 处理流信息查询（含编码、分辨率、码率、上线时间）。
//
// GET /api/v1/streams/:id/info
func (h *StreamHandler) Info(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	st, err := h.streamSvc.Get(id)
	if err != nil {
		response.NotFound(c, "流不存在")
		return
	}
	onlineText := "离线"
	if st.Online == 1 {
		onlineText = "在线"
	}
	response.OK(c, StreamInfo{
		Stream:          st,
		PublishURL:      h.streamSvc.PublishURL(c.Request.Context(), st),
		OnlineText:      onlineText,
		PublishTimeText: formatTime(st.PublishTime),
	})
}

// formatTime 格式化 Unix 时间戳。
func formatTime(ts int64) string {
	if ts <= 0 {
		return "-"
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}

// ensure strconv 被引用。
var _ = strconv.Itoa
