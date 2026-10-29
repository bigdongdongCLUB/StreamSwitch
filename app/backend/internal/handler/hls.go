package handler

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"livems/internal/pkg/logger"
	"livems/internal/pkg/response"
	"livems/internal/service"
)

// HLSHandler 处理加密 HLS 播放代理的请求。
//
// 路由：
//
//	GET /hls-enc/:app/:stream     加密播放列表（:stream 以 .m3u8 结尾）
//	GET /hls-enc/:app/:stream/*seg 加密切片（/*seg 形如 /0.ts）
//	GET /hls-key/:app/:key        AES-128 密钥（:key 形如 xxx.key）
//
// 鉴权说明：
//   播放令牌由 ZLMediaKit 的 on_play 钩子统一校验（所有播放格式共用同一套逻辑），
//   本服务不再重复校验，避免双重解析带来的性能损耗。
//   密钥接口只在该流已开启加密时才返回内容。
type HLSHandler struct {
	encryptSvc *service.EncryptService
	authSvc    *service.AuthService
}

// NewHLSHandler 创建 HLS 加密代理处理器。
func NewHLSHandler(encryptSvc *service.EncryptService, authSvc *service.AuthService) *HLSHandler {
	return &HLSHandler{encryptSvc: encryptSvc, authSvc: authSvc}
}

// Play 处理加密 HLS 播放请求。
//
// 根据路径后缀自动区分播放列表(.m3u8)与切片(.ts)。
//
// GET /hls-enc/:app/:stream
func (h *HLSHandler) Play(c *gin.Context) {
	app := c.Param("app")
	stream := c.Param("stream")

	// 播放列表：/hls-enc/<app>/<stream>.m3u8
	if strings.HasSuffix(stream, ".m3u8") {
		h.M3U8(c, app, strings.TrimSuffix(stream, ".m3u8"))
		return
	}
	// 切片：/hls-enc/<app>/<stream>/<seg>
	if seg := c.Param("seg"); seg != "" {
		h.Segment(c, app, stream, strings.TrimPrefix(seg, "/"))
		return
	}
	response.BadRequest(c, "非法的播放请求路径")
}

// M3U8 处理加密 m3u8 请求。
func (h *HLSHandler) M3U8(c *gin.Context, app, stream string) {
	if app == "" || stream == "" {
		response.BadRequest(c, "非法的流路径")
		return
	}
	// 构造密钥地址（使用当前请求的 Host，保证与播放域名一致）
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	keyURL := scheme + "://" + c.Request.Host + "/hls-key/" + app + "/" + stream + ".key"

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if err := h.encryptSvc.HLS().HandleM3U8(ctx, c.Writer, app, stream, keyURL); err != nil {
		logger.Warn("处理加密 m3u8 失败 app=%s stream=%s: %v", app, stream, err)
		response.ServerError(c, "获取播放列表失败: "+err.Error())
	}
}

// Segment 处理加密切片请求。
func (h *HLSHandler) Segment(c *gin.Context, app, stream, file string) {
	if app == "" || stream == "" || file == "" {
		response.BadRequest(c, "非法的切片请求路径")
		return
	}
	// 从切片文件名中解析媒体序号，用于推导 IV
	seq := parseSegmentSequence(file)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	if err := h.encryptSvc.HLS().HandleSegment(ctx, c.Writer, app, stream, file, seq); err != nil {
		logger.Warn("处理加密切片失败 app=%s stream=%s file=%s: %v", app, stream, file, err)
		response.ServerError(c, "获取切片失败: "+err.Error())
	}
}

// Key 处理密钥请求。
//
// GET /hls-key/:app/:key
func (h *HLSHandler) Key(c *gin.Context) {
	app := c.Param("app")
	key := c.Param("key")
	stream := strings.TrimSuffix(key, ".key")
	if app == "" || stream == "" {
		response.BadRequest(c, "非法的密钥请求路径")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := h.encryptSvc.HLS().HandleKey(ctx, c.Writer, app, stream); err != nil {
		response.NotFound(c, "密钥不存在")
	}
}

// parseSegmentSequence 从切片文件名解析媒体序号。
//
// 支持的命名：
//   - "0.ts"      -> 0
//   - "abc_12.ts" -> 12
//   - "12.ts"     -> 12
//
// 解析失败时返回 0。
func parseSegmentSequence(file string) int64 {
	name := file
	if idx := strings.LastIndex(name, "."); idx > 0 {
		name = name[:idx]
	}
	// 取最后一个下划线之后的部分
	part := name
	if idx := strings.LastIndex(name, "_"); idx >= 0 {
		part = name[idx+1:]
	}
	n, err := strconv.ParseInt(part, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
