package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"livems/internal/config"
	"livems/internal/models"
	"livems/internal/pkg/logger"
	"livems/internal/pkg/util"
	"livems/internal/repository"
	"livems/internal/zlm"
)

// HookService 处理 ZLMediaKit 的回调事件。
//
// 主要处理：
//   - on_publish        推流鉴权（校验流是否存在、是否启用、推流Key是否匹配）
//   - on_play           播放鉴权（校验播放令牌、IP 绑定、格式开关）
//   - on_stream_changed 流上线/下线状态同步
//   - on_flow_report    流量统计
type HookService struct {
	streamRepo   *repository.StreamRepo
	playSvc      *PlayService
	authSvc      *AuthService
	forwardSvc   *ForwardService
	proxyRepo    *repository.ProxyRepo
	forwardRepo  *repository.ForwardRepo
	encryptRepo  *repository.EncryptRepo
	logRepo      *repository.LogRepo
	cfg          *config.Config
}

// NewHookService 创建回调服务。
func NewHookService(cfg *config.Config, playSvc *PlayService, authSvc *AuthService, forwardSvc *ForwardService) *HookService {
	return &HookService{
		streamRepo:  repository.NewStreamRepo(),
		playSvc:     playSvc,
		authSvc:     authSvc,
		forwardSvc:  forwardSvc,
		proxyRepo:   repository.NewProxyRepo(),
		forwardRepo: repository.NewForwardRepo(),
		encryptRepo: repository.NewEncryptRepo(),
		logRepo:     repository.NewLogRepo(),
		cfg:         cfg,
	}
}

// HandlePublish 处理 on_publish 推流事件。
//
// 鉴权规则：
//  1. 该 app/stream 必须在后台存在对应的流记录，否则拒绝（防止随意推流占用资源）；
//  2. 流必须处于启用状态，否则拒绝；
//  3. 若该流配置了推流Key，则 URL 参数中的 key 必须匹配，否则拒绝。
//
// 返回值中的 enable_* 字段用于控制 ZLM 的协议转换：
//  - enable_rtmp 始终为 true（保证 HTTP-FLV/RTMP 可播放）
//  - enable_hls 依据后台的 HLS 格式开关决定
func (s *HookService) HandlePublish(ev *zlm.PublishEvent) zlm.HookReply {
	// 查流记录
	st, err := s.streamRepo.GetByStream(ev.App, ev.Stream)
	if err != nil || st == nil {
		logger.Warn("on_publish 拒绝: 流未在后台登记 app=%s stream=%s ip=%s", ev.App, ev.Stream, ev.IP)
		return zlm.Deny("流未在后台登记，请先在管理后台创建该流")
	}
	// 流被禁用则拒绝
	if st.Status != 1 {
		logger.Warn("on_publish 拒绝: 流已禁用 app=%s stream=%s", ev.App, ev.Stream)
		return zlm.Deny("该流已被禁用")
	}
	// 校验推流Key
	if st.PushKey != "" {
		params := ev.ParseParams()
		key := params["key"]
		if key == "" {
			// 兼容把 key 放在 query 里的其它位置
			key = params["token"]
		}
		if key != st.PushKey {
			logger.Warn("on_publish 拒绝: 推流Key不匹配 app=%s stream=%s ip=%s", ev.App, ev.Stream, ev.IP)
			return zlm.Deny("推流鉴权失败：推流Key不正确")
		}
	}

	// 标记上线
	s.markOnline(st, ev)

	// 控制协议转换开关
	reply := zlm.Allow()
	enableHLS := s.playSvc.FormatEnabled(FormatHLS)
	enableRTMP := s.playSvc.FormatEnabled(FormatRTMP) || s.playSvc.FormatEnabled(FormatHTTPFLV) ||
		s.playSvc.FormatEnabled(FormatWSFLV)
	reply.EnableHLS = &enableHLS
	reply.EnableRTMP = &enableRTMP
	// HTTP-FLV / WS-FLV 依赖 RTMP 转换，HLS 依赖 HLS 转换
	reply.EnableRTSP = &enableRTMP
	reply.EnableTS = &enableRTMP
	reply.EnableFMP4 = &enableRTMP
	logger.Info("on_publish 通过: app=%s stream=%s ip=%s hls=%v rtmp=%v",
		ev.App, ev.Stream, ev.IP, enableHLS, enableRTMP)
	return reply
}

// HandlePlay 处理 on_play 播放事件。
//
// 鉴权规则（所有播放格式共用）：
//  1. 播放格式必须在后台开启；
//  2. 若开启了播放鉴权，则必须携带有效的播放令牌；
//  3. 令牌可绑定 IP，并可限定流与格式。
func (s *HookService) HandlePlay(ev *zlm.PlayEvent) zlm.HookReply {
	// 映射 ZLM 的 schema 到本系统的播放格式
	format := schemaToFormat(ev.Schema, ev.Params)

	// 格式开关校验
	if format != "" && !s.playSvc.FormatEnabled(format) {
		logger.Warn("on_play 拒绝: 播放格式已关闭 format=%s app=%s stream=%s", format, ev.App, ev.Stream)
		return zlm.Deny("该播放格式未开放")
	}

	// 取令牌
	params := ev.ParseParams()
	token := params[s.cfg.Auth.ParamName]
	if token == "" {
		token = params["token"]
	}

	// 校验令牌
	ok, reason := s.authSvc.Verify(token, ev.IP, ev.App, ev.Stream, format)
	if !ok {
		logger.Warn("on_play 拒绝: %s app=%s stream=%s ip=%s schema=%s format=%s params=%s", reason, ev.App, ev.Stream, ev.IP, ev.Schema, format, ev.Params)
		return zlm.Deny(reason)
	}
	return zlm.Allow()
}

// HandleStreamChanged 处理 on_stream_changed 流上下线事件。
func (s *HookService) HandleStreamChanged(ev *zlm.StreamChangedEvent) {
	st, err := s.streamRepo.GetByStream(ev.App, ev.Stream)
	if err != nil || st == nil {
		return
	}
	now := time.Now().Unix()
	if ev.Regist {
		// 上线
		s.streamRepo.MarkOnline(st.ID, ev.IP, now)
		logger.Info("流上线: %s/%s ip=%s", ev.App, ev.Stream, ev.IP)
		// 拉流代理任务若对应此流，更新活跃时间
		s.proxyRepo.UpdateRunStatus(st.ID, "running", "")
		// 联动：源上线时自动拉起等待中的转推任务（含制播输出流）
		if s.forwardSvc != nil {
			s.forwardSvc.StartBySource(ev.App, ev.Stream)
		}
		return
	}
	// 下线
	s.streamRepo.MarkOffline(st.ID, now)
	logger.Info("流下线: %s/%s", ev.App, ev.Stream)
}

// HandleFlowReport 处理 on_flow_report 流量统计事件。
func (s *HookService) HandleFlowReport(ev *zlm.FlowReportEvent) {
	if !ev.Player {
		return
	}
	st, err := s.streamRepo.GetByStream(ev.App, ev.Stream)
	if err != nil || st == nil {
		return
	}
	// 累加观看人次
	s.streamRepo.IncReader(st.ID)
}

// HandleStreamNotFound 处理 on_stream_not_found 事件。
func (s *HookService) HandleStreamNotFound(ev *zlm.StreamNotFoundEvent) {
	logger.Warn("请求的流不存在: app=%s stream=%s ip=%s", ev.App, ev.Stream, ev.IP)
}

// markOnline 标记流上线并记录推流端信息。
func (s *HookService) markOnline(st *models.Stream, ev *zlm.PublishEvent) {
	now := time.Now().Unix()
	s.streamRepo.MarkOnline(st.ID, ev.IP, now)
	// 若该流有对应的拉流代理任务，更新其运行状态
	if list, _, err := s.proxyRepo.List(1, 50, st.Stream, 1); err == nil {
		for _, p := range list {
			if p.Stream == st.Stream && p.App == st.App {
				s.proxyRepo.UpdateRunStatus(p.ID, "running", "")
			}
		}
	}
}

// schemaToFormat 将 ZLM 的 schema/协议映射为本系统的播放格式标识。
//
// 说明：
//   - HLS 播放时 ZLM 的 schema 为 "hls"
//   - HTTP-FLV / WS-FLV 播放时 schema 为 "http"（或 "fmp4"）
//   - RTMP 播放时 schema 为 "rtmp"
//   - 通过 URL 后缀可以进一步区分 flv 与其它格式
func schemaToFormat(schema, params string) string {
	switch strings.ToLower(schema) {
	case "hls", "hls.fmp4":
		return FormatHLS
	case "rtmp", "rtsp":
		return FormatRTMP
	case "http", "https", "ws", "wss", "fmp4", "ts":
		// 通过 URL 参数/路径中的 .flv 判断
		if strings.Contains(params, ".flv") {
			return FormatHTTPFLV
		}
		if strings.Contains(params, ".m3u8") {
			return FormatHLS
		}
		return FormatHTTPFLV
	default:
		return ""
	}
}

// SyncStreams 定期与 ZLM 同步流状态。
func (s *HookService) SyncStreams(ctx context.Context) error {
	list, err := s.streamRepo.All()
	if err != nil {
		return err
	}
	for i := range list {
		st := &list[i]
		if st.Online == 1 {
			// 检查是否真的还在线
			online, err := s.isOnline(ctx, st)
			if err != nil {
				continue
			}
			if !online {
				s.streamRepo.MarkOffline(st.ID, time.Now().Unix())
				logger.Info("同步发现流已离线: %s/%s", st.App, st.Stream)
			}
		}
	}
	return nil
}

// isOnline 检查流是否在 ZLM 中在线。
func (s *HookService) isOnline(ctx context.Context, st *models.Stream) (bool, error) {
	client := zlm.New(s.cfg.ZLM)
	for _, schema := range []string{"rtmp", "rtsp", "ts", "fmp4", "hls"} {
		online, err := client.IsMediaOnline(ctx, schema, st.Vhost, st.App, st.Stream)
		if err != nil {
			return false, err
		}
		if online {
			return true, nil
		}
	}
	return false, nil
}

// ensure util 包被引用。
var _ = util.IsValidIP

// ensure fmt 包被引用。
var _ = fmt.Sprintf
