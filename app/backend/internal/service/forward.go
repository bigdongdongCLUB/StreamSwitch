package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"livems/internal/config"
	"livems/internal/models"
	"livems/internal/pkg/logger"
	"livems/internal/repository"
	"livems/internal/zlm"
)

// ForwardService 转推流服务。
//
// 转推流把一路本地源流推送到多个 RTMP 目标地址。
// 每个转推任务独立启停，各自记录错误日志。
type ForwardService struct {
	repo       *repository.ForwardRepo
	streamRepo *repository.StreamRepo
	logRepo    *repository.LogRepo
	zlmClient  *zlm.Client
	cfg        *config.Config
}

// NewForwardService 创建转推流服务。
func NewForwardService(cfg *config.Config) *ForwardService {
	return &ForwardService{
		repo:       repository.NewForwardRepo(),
		streamRepo: repository.NewStreamRepo(),
		logRepo:    repository.NewLogRepo(),
		zlmClient:  zlm.New(cfg.ZLM),
		cfg:        cfg,
	}
}

// CreateForwardRequest 新增转推流任务的请求参数。
type CreateForwardRequest struct {
	Name       string `json:"name" binding:"required"`
	SrcApp     string `json:"src_app"`
	SrcStream  string `json:"src_stream" binding:"required"`
	SrcVhost   string `json:"src_vhost"`
	DstURL     string `json:"dst_url" binding:"required"`
	Schema     string `json:"schema"`
	Status     int    `json:"status"`
	RetryCount int    `json:"retry_count"`
	TimeoutSec int    `json:"timeout_sec"`
}

// Create 新增转推流任务。
func (s *ForwardService) Create(req *CreateForwardRequest, userID uint64) (*models.StreamForward, error) {
	if err := validateDstURL(req.DstURL); err != nil {
		return nil, err
	}
	if err := validateStreamID(req.SrcStream); err != nil {
		return nil, err
	}
	if req.SrcApp == "" {
		req.SrcApp = s.cfg.ZLM.App
	}
	if req.SrcVhost == "" {
		req.SrcVhost = s.cfg.ZLM.Vhost
	}
	if req.Schema == "" {
		req.Schema = "rtmp"
	}
	if req.Schema != "rtmp" && req.Schema != "rtmps" {
		return nil, fmt.Errorf("转推协议只支持 rtmp 或 rtmps")
	}
	if req.Status == 0 {
		req.Status = 1
	}
	if req.RetryCount == 0 {
		req.RetryCount = -1
	}
	if req.TimeoutSec <= 0 {
		req.TimeoutSec = 10
	}

	now := time.Now().Unix()
	f := &models.StreamForward{
		Name:       req.Name,
		SrcApp:     req.SrcApp,
		SrcStream:  req.SrcStream,
		SrcVhost:   req.SrcVhost,
		DstURL:     req.DstURL,
		Schema:     req.Schema,
		Status:     req.Status,
		RunStatus:  "idle",
		RetryCount: req.RetryCount,
		TimeoutSec: req.TimeoutSec,
		CreatedBy:  userID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.repo.Create(f); err != nil {
		return nil, err
	}
	logger.Info("新增转推流任务: %s/%s -> %s", f.SrcApp, f.SrcStream, f.DstURL)
	if f.Status == 1 {
		if err := s.Start(f.ID); err != nil {
			logger.Warn("转推流任务启动失败 id=%d: %v", f.ID, err)
		}
	}
	return f, nil
}

// Toggle 启用/禁用转推流任务。
func (s *ForwardService) Toggle(id uint64) (int, error) {
	f, err := s.repo.GetByID(id)
	if err != nil {
		return 0, fmt.Errorf("转推流任务不存在")
	}
	newStatus := 1
	if f.Status == 1 {
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

// Update 更新转推流任务。
func (s *ForwardService) Update(id uint64, req *CreateForwardRequest) error {
	f, err := s.repo.GetByID(id)
	if err != nil {
		return fmt.Errorf("转推流任务不存在")
	}
	if err := validateDstURL(req.DstURL); err != nil {
		return err
	}
	needRestart := f.DstURL != req.DstURL || f.SrcStream != req.SrcStream || f.SrcApp != req.SrcApp

	if req.Name != "" {
		f.Name = req.Name
	}
	if req.SrcApp != "" {
		f.SrcApp = req.SrcApp
	}
	f.SrcStream = req.SrcStream
	if req.SrcVhost != "" {
		f.SrcVhost = req.SrcVhost
	}
	f.DstURL = req.DstURL
	if req.Schema != "" {
		f.Schema = req.Schema
	}
	if req.Status > 0 {
		f.Status = req.Status
	}
	if req.RetryCount != 0 {
		f.RetryCount = req.RetryCount
	}
	if req.TimeoutSec > 0 {
		f.TimeoutSec = req.TimeoutSec
	}

	if err := s.repo.Update(f); err != nil {
		return err
	}
	if needRestart && f.Status == 1 {
		_ = s.Stop(id)
		return s.Start(id)
	}
	return nil
}

// Delete 删除转推流任务。
func (s *ForwardService) Delete(id uint64) error {
	_ = s.Stop(id)
	return s.repo.Delete(id)
}

// List 分页查询转推流任务。
func (s *ForwardService) List(page, pageSize int, keyword string, status int) ([]models.StreamForward, int64, error) {
	return s.repo.List(page, pageSize, keyword, status)
}

// Get 按ID查询转推流任务。
func (s *ForwardService) Get(id uint64) (*models.StreamForward, error) {
	return s.repo.GetByID(id)
}

// StartBySource 源流上线联动：启动所有以该流为源、已启用但未运行的转推任务。
//
// ZLM 的 addStreamPusherProxy 要求源流在线才能创建；源离线期间启动会失败，
// 由本方法在源上线时（on_stream_changed 钩子）自动补拉起。
func (s *ForwardService) StartBySource(app, stream string) {
	list, err := s.repo.ListBySource(app, stream)
	if err != nil {
		return
	}
	for i := range list {
		f := &list[i]
		if f.Status != 1 || f.RunStatus == "running" {
			continue
		}
		if err := s.Start(f.ID); err != nil {
			logger.Warn("源上线联动启动转推失败 id=%d: %v", f.ID, err)
		} else {
			logger.Info("源流上线，联动启动转推: id=%d %s/%s -> %s", f.ID, f.SrcApp, f.SrcStream, f.DstURL)
		}
	}
}

// EnsureRunning 周期兜底：启用中但未运行的任务，若源流当前在线则自动拉起。
//
// 覆盖场景：服务重启恢复时源流恰好离线导致失败，之后源上线但无新的 regist 事件触发联动。
func (s *ForwardService) EnsureRunning(ctx context.Context) {
	list, err := s.repo.ListEnabled()
	if err != nil {
		return
	}
	// 过滤出需要拉起的任务
	pending := make([]*models.StreamForward, 0, len(list))
	for i := range list {
		if list[i].RunStatus != "running" {
			pending = append(pending, &list[i])
		}
	}
	if len(pending) == 0 {
		return
	}
	// 查 ZLM 在线流
	mediaList, err := s.zlmClient.GetMediaList(ctx, "", "", s.cfg.ZLM.App, "")
	if err != nil {
		return
	}
	online := make(map[string]bool, len(mediaList))
	for _, m := range mediaList {
		online[m.App+"/"+m.Stream] = true
	}
	for _, f := range pending {
		if !online[f.SrcApp+"/"+f.SrcStream] {
			continue
		}
		if err := s.Start(f.ID); err != nil {
			logger.Warn("兜底拉起转推任务失败 id=%d: %v", f.ID, err)
		} else {
			logger.Info("源流在线，兜底拉起转推任务: id=%d %s/%s -> %s", f.ID, f.SrcApp, f.SrcStream, f.DstURL)
		}
	}
}

// RestoreAll 在应用启动后恢复所有启用的转推流任务。
func (s *ForwardService) RestoreAll() {
	list, err := s.repo.ListEnabled()
	if err != nil {
		logger.Warn("恢复转推流任务失败: %v", err)
		return
	}
	for i := range list {
		f := list[i]
		if err := s.Start(f.ID); err != nil {
			logger.Warn("恢复转推流任务失败 id=%d: %v", f.ID, err)
		}
	}
}

// validateDstURL 校验转推目标地址（必须是 RTMP/RTMPS）。
func validateDstURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("转推目标地址不能为空")
	}
	if len(raw) > 1024 {
		return fmt.Errorf("转推目标地址过长")
	}
	lowered := strings.ToLower(raw)
	if !strings.HasPrefix(lowered, "rtmp://") && !strings.HasPrefix(lowered, "rtmps://") {
		return fmt.Errorf("转推目标地址必须是 rtmp:// 或 rtmps:// 协议")
	}
	return nil
}
