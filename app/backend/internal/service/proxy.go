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

// ProxyService 拉流代理服务。
//
// 拉流代理用于把远端源（rtmp / http-flv / hls）自动拉到本地 ZLMediaKit，
// 由 ZLMediaKit 负责断线重连（通过 retry_count 参数）。
// 这里的重试计数是「任务级」的：ZLM 内部会不断重试同一个代理，
// 当 ZLM 判定失败并移除代理后，本服务按 retry_interval 重新拉起。
type ProxyService struct {
	repo      *repository.ProxyRepo
	logRepo   *repository.LogRepo
	zlmClient *zlm.Client
	cfg       *config.Config
}

// NewProxyService 创建拉流代理服务。
func NewProxyService(cfg *config.Config) *ProxyService {
	return &ProxyService{
		repo:      repository.NewProxyRepo(),
		logRepo:   repository.NewLogRepo(),
		zlmClient: zlm.New(cfg.ZLM),
		cfg:       cfg,
	}
}

// CreateProxyRequest 新增拉流代理的请求参数。
type CreateProxyRequest struct {
	Name          string `json:"name" binding:"required"`
	SrcURL        string `json:"src_url" binding:"required"`
	App           string `json:"app"`
	Stream        string `json:"stream" binding:"required"`
	Vhost         string `json:"vhost"`
	Status        int    `json:"status"`
	RetryCount    int    `json:"retry_count"`
	RetryInterval int    `json:"retry_interval"`
	TimeoutSec    int    `json:"timeout_sec"`
	EnableRTSP    int    `json:"enable_rtsp"`
	EnableRTMP    int    `json:"enable_rtmp"`
	EnableTS      int    `json:"enable_ts"`
	EnableFMP4    int    `json:"enable_fmp4"`
	EnableHLS     int    `json:"enable_hls"`
}

// Create 新增拉流代理任务。
func (s *ProxyService) Create(req *CreateProxyRequest, userID uint64) (*models.StreamProxy, error) {
	if err := validateSrcURL(req.SrcURL); err != nil {
		return nil, err
	}
	if err := validateStreamID(req.Stream); err != nil {
		return nil, err
	}
	if req.App == "" {
		req.App = s.cfg.ZLM.App
	}
	if req.Vhost == "" {
		req.Vhost = s.cfg.ZLM.Vhost
	}
	// 默认参数
	if req.Status == 0 {
		req.Status = 1
	}
	if req.RetryCount == 0 {
		req.RetryCount = -1
	}
	if req.RetryInterval <= 0 {
		req.RetryInterval = 10
	}
	if req.TimeoutSec <= 0 {
		req.TimeoutSec = 10
	}
	// 转协议开关默认值
	if req.EnableRTSP == 0 && req.EnableRTMP == 0 && req.EnableHLS == 0 {
		req.EnableRTSP = 1
		req.EnableRTMP = 1
		req.EnableHLS = 1
	}

	now := time.Now().Unix()
	p := &models.StreamProxy{
		Name:          req.Name,
		SrcURL:        req.SrcURL,
		App:           req.App,
		Stream:        req.Stream,
		Vhost:         req.Vhost,
		Status:        req.Status,
		RetryCount:    req.RetryCount,
		RetryInterval: req.RetryInterval,
		TimeoutSec:    req.TimeoutSec,
		EnableRTSP:    req.EnableRTSP,
		EnableRTMP:    req.EnableRTMP,
		EnableTS:      req.EnableTS,
		EnableFMP4:    req.EnableFMP4,
		EnableHLS:     req.EnableHLS,
		RunStatus:     "idle",
		CreatedBy:     userID,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.repo.Create(p); err != nil {
		return nil, err
	}
	logger.Info("新增拉流代理任务: %s -> %s/%s", p.SrcURL, p.App, p.Stream)
	// 立即尝试启动
	if p.Status == 1 {
		if err := s.Start(p.ID); err != nil {
			logger.Warn("拉流代理任务启动失败 id=%d: %v", p.ID, err)
		}
	}
	return p, nil
}

// Start 启动一个拉流代理任务（向 ZLM 注册代理）。
func (s *ProxyService) Start(id uint64) error {
	p, err := s.repo.GetByID(id)
	if err != nil {
		return fmt.Errorf("拉流代理任务不存在")
	}
	if p.Status != 1 {
		return fmt.Errorf("任务已禁用，无法启动")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	key, err := s.zlmClient.AddStreamProxy(ctx, zlm.ProxyAddReq{
		Vhost:        p.Vhost,
		App:          p.App,
		Stream:       p.Stream,
		URL:          p.SrcURL,
		RetryCount:   p.RetryCount,
		TimeoutSec:   p.TimeoutSec,
		EnableRTSP:   p.EnableRTSP == 1,
		EnableRTMP:   p.EnableRTMP == 1,
		EnableTS:     p.EnableTS == 1,
		EnableFMP4:   p.EnableFMP4 == 1,
		EnableHLS:    p.EnableHLS == 1,
	})
	if err != nil {
		s.repo.UpdateRunStatus(id, "failed", util.TruncateString(err.Error(), 1000))
		s.logRepo.CreateTaskError("proxy", id, p.Name, "error", err.Error())
		return fmt.Errorf("启动拉流代理失败: %w", err)
	}
	s.repo.UpdateZLMKey(id, key)
	s.repo.UpdateRunStatus(id, "running", "")
	logger.Info("拉流代理任务已启动: id=%d %s/%s key=%s", id, p.App, p.Stream, key)
	return nil
}

// Stop 停止一个拉流代理任务。
func (s *ProxyService) Stop(id uint64) error {
	p, err := s.repo.GetByID(id)
	if err != nil {
		return fmt.Errorf("拉流代理任务不存在")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if p.ZLMKey != "" {
		if err := s.zlmClient.DelStreamProxy(ctx, p.ZLMKey); err != nil {
			logger.Warn("关闭拉流代理失败 id=%d: %v", id, err)
		}
	}
	s.repo.UpdateZLMKey(id, "")
	s.repo.UpdateRunStatus(id, "stopped", "")
	logger.Info("拉流代理任务已停止: id=%d", id)
	return nil
}

// Toggle 启用/禁用并相应启停任务。
func (s *ProxyService) Toggle(id uint64) (int, error) {
	p, err := s.repo.GetByID(id)
	if err != nil {
		return 0, fmt.Errorf("拉流代理任务不存在")
	}
	newStatus := 1
	if p.Status == 1 {
		newStatus = 0
		_ = s.Stop(id)
	}
	if err := s.repo.UpdateStatus(id, newStatus); err != nil {
		return 0, err
	}
	if newStatus == 1 {
		if err := s.Start(id); err != nil {
			return 1, err
		}
	}
	return newStatus, nil
}

// Update 更新拉流代理任务。
func (s *ProxyService) Update(id uint64, req *CreateProxyRequest) error {
	p, err := s.repo.GetByID(id)
	if err != nil {
		return fmt.Errorf("拉流代理任务不存在")
	}
	if err := validateSrcURL(req.SrcURL); err != nil {
		return err
	}
	if err := validateStreamID(req.Stream); err != nil {
		return err
	}
	// 记录是否需要重启
	needRestart := p.SrcURL != req.SrcURL || p.Stream != req.Stream || p.App != req.App ||
		p.EnableRTSP != req.EnableRTSP || p.EnableRTMP != req.EnableRTMP ||
		p.EnableHLS != req.EnableHLS || p.EnableTS != req.EnableTS || p.EnableFMP4 != req.EnableFMP4

	if req.Name != "" {
		p.Name = req.Name
	}
	p.SrcURL = req.SrcURL
	if req.App != "" {
		p.App = req.App
	}
	p.Stream = req.Stream
	if req.Vhost != "" {
		p.Vhost = req.Vhost
	}
	if req.Status > 0 {
		p.Status = req.Status
	}
	if req.RetryCount != 0 {
		p.RetryCount = req.RetryCount
	}
	if req.RetryInterval > 0 {
		p.RetryInterval = req.RetryInterval
	}
	if req.TimeoutSec > 0 {
		p.TimeoutSec = req.TimeoutSec
	}
	p.EnableRTSP = req.EnableRTSP
	p.EnableRTMP = req.EnableRTMP
	p.EnableTS = req.EnableTS
	p.EnableFMP4 = req.EnableFMP4
	p.EnableHLS = req.EnableHLS

	if err := s.repo.Update(p); err != nil {
		return err
	}
	// 关键参数变更后重启任务
	if needRestart && p.Status == 1 {
		_ = s.Stop(id)
		return s.Start(id)
	}
	return nil
}

// Delete 删除拉流代理任务。
func (s *ProxyService) Delete(id uint64) error {
	_ = s.Stop(id)
	return s.repo.Delete(id)
}

// List 分页查询拉流代理任务。
func (s *ProxyService) List(page, pageSize int, keyword string, status int) ([]models.StreamProxy, int64, error) {
	return s.repo.List(page, pageSize, keyword, status)
}

// Get 按ID查询拉流代理任务。
func (s *ProxyService) Get(id uint64) (*models.StreamProxy, error) {
	return s.repo.GetByID(id)
}

// RestoreAll 在应用启动后恢复所有启用的拉流代理任务。
func (s *ProxyService) RestoreAll() {
	list, err := s.repo.ListEnabled()
	if err != nil {
		logger.Warn("恢复拉流代理任务失败: %v", err)
		return
	}
	for i := range list {
		p := list[i]
		if err := s.Start(p.ID); err != nil {
			logger.Warn("恢复拉流代理任务失败 id=%d: %v", p.ID, err)
		}
	}
}

// validateSrcURL 校验远端源地址的合法性与协议。
func validateSrcURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("远端源地址不能为空")
	}
	if len(raw) > 1024 {
		return fmt.Errorf("远端源地址过长")
	}
	lowered := strings.ToLower(raw)
	// 只允许拉流协议
	switch {
	case strings.HasPrefix(lowered, "rtmp://"),
		strings.HasPrefix(lowered, "rtmps://"),
		strings.HasPrefix(lowered, "rtsp://"),
		strings.HasPrefix(lowered, "rtsps://"),
		strings.HasPrefix(lowered, "http://"),
		strings.HasPrefix(lowered, "https://"),
		strings.HasPrefix(lowered, "srt://"):
		return nil
	default:
		return fmt.Errorf("不支持的源地址协议，仅支持 rtmp/rtmps/rtsp/rtsps/http/https/srt")
	}
}
