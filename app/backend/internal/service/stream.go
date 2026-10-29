package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"livems/internal/config"
	"livems/internal/models"
	"livems/internal/pkg/jwt"
	"livems/internal/pkg/logger"
	"livems/internal/repository"
	"livems/internal/zlm"
)

// StreamService 流管理服务。
type StreamService struct {
	repo      *repository.StreamRepo
	proxyRepo *repository.ProxyRepo
	forwardRepo *repository.ForwardRepo
	encryptRepo *repository.EncryptRepo
	trafficRepo *repository.TrafficRepo
	zlmClient *zlm.Client
	cfg       *config.Config
	settings  *SettingsService
}

// NewStreamService 创建流管理服务。
func NewStreamService(cfg *config.Config, settings *SettingsService) *StreamService {
	return &StreamService{
		repo:        repository.NewStreamRepo(),
		proxyRepo:   repository.NewProxyRepo(),
		forwardRepo: repository.NewForwardRepo(),
		encryptRepo: repository.NewEncryptRepo(),
		trafficRepo: repository.NewTrafficRepo(),
		zlmClient:   zlm.New(cfg.ZLM),
		cfg:         cfg,
		settings:    settings,
	}
}

// CreateStreamRequest 新增流的请求参数。
type CreateStreamRequest struct {
	Name    string `json:"name" binding:"required"`
	App     string `json:"app"`
	Stream  string `json:"stream" binding:"required"`
	Vhost   string `json:"vhost"`
	PushKey string `json:"push_key"`
	Remark  string `json:"remark"`
}

// Create 新增一条流。
func (s *StreamService) Create(req *CreateStreamRequest, userID uint64) (*models.Stream, error) {
	if strings.TrimSpace(req.Stream) == "" {
		return nil, fmt.Errorf("流ID不能为空")
	}
	if req.App == "" {
		req.App = s.cfg.ZLM.App
	}
	if req.Vhost == "" {
		req.Vhost = s.cfg.ZLM.Vhost
	}
	// 流ID只允许安全字符，避免路径穿越与注入
	if err := validateStreamID(req.Stream); err != nil {
		return nil, err
	}
	// 校验唯一性
	if existing, err := s.repo.GetByStream(req.App, req.Stream); err == nil && existing != nil {
		return nil, fmt.Errorf("流已存在: %s/%s", req.App, req.Stream)
	}
	// 自动生成推流Key（用户未指定时）
	if req.PushKey == "" {
		req.PushKey = generatePushKey()
	}

	now := time.Now().Unix()
	st := &models.Stream{
		Name:      req.Name,
		App:       req.App,
		Stream:    req.Stream,
		Vhost:     req.Vhost,
		PushKey:   req.PushKey,
		Status:    1,
		Remark:    req.Remark,
		CreatedBy: userID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.Create(st); err != nil {
		return nil, err
	}
	logger.Info("新增流: %s/%s (操作人ID=%d)", st.App, st.Stream, userID)
	return st, nil
}

// UpdateStreamRequest 更新流的请求参数。
type UpdateStreamRequest struct {
	Name    string `json:"name"`
	PushKey string `json:"push_key"`
	Remark  string `json:"remark"`
	Status  *int   `json:"status"`
}

// Update 更新流信息。
func (s *StreamService) Update(id uint64, req *UpdateStreamRequest) error {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return fmt.Errorf("流不存在")
	}
	if req.Name != "" {
		st.Name = req.Name
	}
	if req.PushKey != "" {
		st.PushKey = req.PushKey
	}
	if req.Remark != "" {
		st.Remark = req.Remark
	}
	if req.Status != nil {
		st.Status = *req.Status
	}
	st.UpdatedAt = time.Now().Unix()
	return s.repo.Update(st)
}

// Toggle 启用/禁用一条流。
//
// 禁用后该流不允许推流（on_publish 钩子会拒绝），并会断开正在进行的推流。
func (s *StreamService) Toggle(id uint64) (int, error) {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return 0, fmt.Errorf("流不存在")
	}
	newStatus := 1
	if st.Status == 1 {
		newStatus = 0
	}
	if err := s.repo.UpdateStatus(id, newStatus); err != nil {
		return 0, err
	}
	// 禁用时主动踢掉当前推流端（之后 on_publish 会因 status=0 拒绝其重连），并立即标记离线
	if newStatus == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.zlmClient.CloseStream(ctx, "", st.Vhost, st.App, st.Stream, true); err != nil {
			logger.Warn("禁用流后断开推流失败 %s/%s: %v", st.App, st.Stream, err)
		} else if st.Online == 1 {
			_ = s.repo.MarkOffline(st.ID, time.Now().Unix())
			logger.Info("流已禁用并断开推流: %s/%s", st.App, st.Stream)
		}
	}
	return newStatus, nil
}

// Delete 删除一条流（同时清理关联的代理/转推/加密配置）。
func (s *StreamService) Delete(id uint64) error {
	st, err := s.repo.GetByID(id)
	if err != nil {
		return fmt.Errorf("流不存在")
	}
	// 断开推流
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.zlmClient.CloseStream(ctx, "", st.Vhost, st.App, st.Stream, true)

	// 清理关联的转推任务
	forwards, _ := s.forwardRepo.ListBySource(st.App, st.Stream)
	for _, f := range forwards {
		if f.ZLMKey != "" {
			_ = s.zlmClient.DelStreamPusherProxy(ctx, f.ZLMKey)
		}
		_ = s.forwardRepo.Delete(f.ID)
	}
	// 清理关联的拉流代理
	proxies, _, _ := s.proxyRepo.List(1, 1000, st.Stream, 1)
	for _, p := range proxies {
		if p.Stream == st.Stream && p.App == st.App {
			if p.ZLMKey != "" {
				_ = s.zlmClient.DelStreamProxy(ctx, p.ZLMKey)
			}
			_ = s.proxyRepo.Delete(p.ID)
		}
	}
	// 清理加密配置
	_ = s.encryptRepo.Delete(id)

	return s.repo.Delete(id)
}

// List 分页查询流列表。
func (s *StreamService) List(page, pageSize int, keyword string, status int) ([]models.Stream, int64, error) {
	return s.repo.List(page, pageSize, keyword, status)
}

// Get 按ID查询流。
func (s *StreamService) Get(id uint64) (*models.Stream, error) {
	return s.repo.GetByID(id)
}

// GetByStream 按 app+stream 查询流。
func (s *StreamService) GetByStream(app, stream string) (*models.Stream, error) {
	return s.repo.GetByStream(app, stream)
}

// PublishURL 返回该流的推流地址。
//
// 对外主机与端口取自「系统设置」中的接入配置（sys_config，实时生效），
// 未配置时回退到配置文件中的 zlm.rtmp_port 与本机回环地址。
func (s *StreamService) PublishURL(st *models.Stream) string {
	base := "rtmp://127.0.0.1"
	if s.settings != nil {
		base = s.settings.PushBaseURL()
	} else if s.cfg.ZLM.RTMPRTMP > 0 {
		base = fmt.Sprintf("rtmp://127.0.0.1:%d", s.cfg.ZLM.RTMPRTMP)
	}
	url := fmt.Sprintf("%s/%s/%s", base, st.App, st.Stream)
	if st.PushKey != "" {
		url += "?key=" + st.PushKey
	}
	return url
}

// SyncFromZLM 从 ZLMediaKit 同步流状态（更新在线状态、编码信息、观看人数等）。
func (s *StreamService) SyncFromZLM(ctx context.Context) error {
	// 读取 ZLM 中所有流
	mediaList, err := s.zlmClient.GetMediaList(ctx, "", "", s.cfg.ZLM.App, "")
	if err != nil {
		return fmt.Errorf("拉取 ZLM 流列表失败: %w", err)
	}
	// 建立 ZLM 流 -> 信息 的索引（mediaList 按 schema 分行，按 app+stream 去重）
	type key struct{ app, stream string }
	zlmIndex := make(map[key]zlm.MediaInfo, len(mediaList))
	for _, m := range mediaList {
		k := key{m.App, m.Stream}
		if exist, ok := zlmIndex[k]; ok {
			// 同一条流的不同 schema 行：观看人数累加，其余字段一致
			exist.ReaderCount += m.ReaderCount
			zlmIndex[k] = exist
		} else {
			zlmIndex[k] = m
		}
	}

	// 顺带采样流量统计（仪表盘趋势图数据源）：
	//   入流量 = Σ 各流接收速率（bytesSpeed）
	//   出流量 = Σ 各流 接收速率×观看数（近似分发带宽）
	{
		var recvBps, sendBps int64
		var readers int
		for _, m := range zlmIndex {
			recvBps += m.BytesSpeed
			sendBps += m.BytesSpeed * int64(m.ReaderCount)
			readers += m.ReaderCount
		}
		_ = s.trafficRepo.Insert(&repository.TrafficStatPoint{
			TS:            time.Now().Unix(),
			RecvBps:       recvBps,
			SendBps:       sendBps,
			OnlineStreams: len(zlmIndex),
			Readers:       readers,
		})
	}

	// 同步数据库中的流
	streams, err := s.repo.All()
	if err != nil {
		return err
	}
	for i := range streams {
		st := &streams[i]
		info, online := zlmIndex[key{st.App, st.Stream}]
		if !online {
			// ZLM 中不存在该流
			if st.Online == 1 {
				s.repo.MarkOffline(st.ID, time.Now().Unix())
			} else if st.ReaderCount != 0 || st.Bitrate != 0 {
				// 历史数据/旧版本遗留：已离线却还挂着观看数或码率，顺手清零
				_ = s.repo.ClearLiveStats(st.ID)
			}
			continue
		}
		// 流在线：更新媒体信息
		updated := models.Stream{}
		for _, t := range info.Tracks {
			if t.IsVideo() {
				updated.VideoCodec = t.CodecIDName
				updated.Width = t.Width
				updated.Height = t.Height
				updated.FPS = int(t.FPS + 0.5)
			} else if t.IsAudio() {
				updated.AudioCodec = t.CodecIDName
				updated.SampleRate = t.SampleRate
			}
		}
		// 码率：ZLM 的 bytesSpeed 是字节/秒，换算为 kbps
		updated.Bitrate = int(info.BytesSpeed * 8 / 1024)
		updated.VideoBitrate = updated.Bitrate
		updated.AudioBitrate = 0
		updated.ReaderCount = info.ReaderCount
		updated.TotalBytes = info.TotalBytes

		if err := s.repo.UpdateMediaInfo(st.ID, updated); err != nil {
			logger.Warn("同步流媒体信息失败 id=%d: %v", st.ID, err)
		}
		if st.Online != 1 {
			s.repo.MarkOnline(st.ID, st.PublishIP, time.Now().Unix())
		}
	}
	return nil
}

// generatePushKey 生成一个随机的推流鉴权 Key。
func generatePushKey() string {
	return jwt.RandomHex(16)
}

// validateStreamID 校验流ID的合法性。
//
// 只允许字母、数字、下划线、连字符，避免路径穿越与命令注入风险。
func validateStreamID(id string) error {
	if len(id) > 128 {
		return fmt.Errorf("流ID长度不能超过128个字符")
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '_' || c == '-' || c == '.':
		default:
			return fmt.Errorf("流ID只能包含字母、数字、下划线、连字符和点")
		}
	}
	// 不允许以点开头（避免隐藏文件与路径穿越）
	if strings.HasPrefix(id, ".") {
		return fmt.Errorf("流ID不能以点开头")
	}
	return nil
}
