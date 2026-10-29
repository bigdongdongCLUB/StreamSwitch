package service

import (
	"fmt"
	"time"

	"livems/internal/config"
	"livems/internal/hls"
	"livems/internal/models"
	"livems/internal/pkg/logger"
	"livems/internal/repository"
)

// EncryptService HLS 加密配置服务。
type EncryptService struct {
	repo       *repository.EncryptRepo
	streamRepo *repository.StreamRepo
	hlsSvc     *hls.Service
	cfg        *config.Config
}

// NewEncryptService 创建 HLS 加密配置服务。
func NewEncryptService(cfg *config.Config) *EncryptService {
	return &EncryptService{
		repo:       repository.NewEncryptRepo(),
		streamRepo: repository.NewStreamRepo(),
		hlsSvc:     hls.NewService(cfg),
		cfg:        cfg,
	}
}

// HLS 返回底层的 HLS 加密服务（供 HTTP 处理器使用）。
func (s *EncryptService) HLS() *hls.Service { return s.hlsSvc }

// EncryptRequest 配置 HLS 加密的请求参数。
type EncryptRequest struct {
	Enabled      bool   `json:"enabled"`
	KeyExpireSec int    `json:"key_expire_sec"`
	IVStrategy   string `json:"iv_strategy"`
	// RotateKey 为 true 时强制重新生成密钥
	RotateKey bool `json:"rotate_key"`
}

// SetConfig 为某条流配置 HLS 加密。
func (s *EncryptService) SetConfig(streamID uint64, req *EncryptRequest) (*models.HLSEncrypt, error) {
	st, err := s.streamRepo.GetByID(streamID)
	if err != nil {
		return nil, fmt.Errorf("流不存在")
	}
	if req.IVStrategy == "" {
		req.IVStrategy = s.cfg.HLS.IVStrategy
	}
	if req.IVStrategy != "seq" && req.IVStrategy != "random" {
		return nil, fmt.Errorf("IV 策略只支持 seq 或 random")
	}
	if req.KeyExpireSec < 0 {
		return nil, fmt.Errorf("密钥有效期不能为负数")
	}

	existing, _ := s.repo.GetByStreamID(streamID)
	enc := &models.HLSEncrypt{
		StreamID:     streamID,
		Enabled:      0,
		KeyExpireSec: req.KeyExpireSec,
		IVStrategy:   req.IVStrategy,
		UpdatedAt:    time.Now().Unix(),
	}
	if existing != nil {
		enc.ID = existing.ID
		enc.KeyHex = existing.KeyHex
		enc.KeyURI = existing.KeyURI
		enc.ExpireAt = existing.ExpireAt
		enc.LastRotateAt = existing.LastRotateAt
		enc.CreatedAt = existing.CreatedAt
	}
	if req.Enabled {
		enc.Enabled = 1
	}

	// 需要生成或轮换密钥
	if req.Enabled && (enc.KeyHex == "" || req.RotateKey) {
		hexKey, raw, err := hls.GenerateKey()
		if err != nil {
			return nil, err
		}
		enc.KeyHex = hexKey
		enc.LastRotateAt = time.Now().Unix()
		if req.KeyExpireSec > 0 {
			enc.ExpireAt = time.Now().Add(time.Duration(req.KeyExpireSec) * time.Second).Unix()
		} else {
			enc.ExpireAt = 0
		}
		// 写入磁盘并加载到内存
		if err := s.hlsSvc.SetKey(st.App, st.Stream, raw, req.KeyExpireSec); err != nil {
			return nil, err
		}
		logger.Info("已为流 %s/%s 生成 HLS 加密密钥", st.App, st.Stream)
	} else if !req.Enabled {
		enc.ExpireAt = 0
	}

	// 密钥地址
	if enc.KeyURI == "" {
		enc.KeyURI = fmt.Sprintf("/hls-key/%s/%s.key", st.App, st.Stream)
	}

	if err := s.repo.Upsert(enc); err != nil {
		return nil, err
	}
	logger.Info("更新流 %s/%s 的 HLS 加密配置: enabled=%v", st.App, st.Stream, req.Enabled)
	return enc, nil
}

// GetConfig 查询某条流的加密配置。
func (s *EncryptService) GetConfig(streamID uint64) (*models.HLSEncrypt, error) {
	return s.repo.GetByStreamID(streamID)
}

// List 查询全部加密配置。
func (s *EncryptService) List() ([]models.HLSEncrypt, error) {
	return s.repo.List()
}

// Delete 删除某条流的加密配置。
func (s *EncryptService) Delete(streamID uint64) error {
	return s.repo.Delete(streamID)
}

// RotateKey 轮换某条流的密钥。
func (s *EncryptService) RotateKey(streamID uint64) error {
	enc, err := s.repo.GetByStreamID(streamID)
	if err != nil {
		return fmt.Errorf("加密配置不存在")
	}
	_, err = s.SetConfig(streamID, &EncryptRequest{
		Enabled:      enc.Enabled == 1,
		KeyExpireSec: enc.KeyExpireSec,
		IVStrategy:   enc.IVStrategy,
		RotateKey:    true,
	})
	return err
}

// RestoreAll 在应用启动时把数据库中的密钥加载到内存（供加密代理使用）。
func (s *EncryptService) RestoreAll() {
	list, err := s.repo.ListEnabled()
	if err != nil {
		logger.Warn("恢复 HLS 加密配置失败: %v", err)
		return
	}
	for i := range list {
		enc := list[i]
		st, err := s.streamRepo.GetByID(enc.StreamID)
		if err != nil || st == nil {
			continue
		}
		raw, err := hls.HexToKey(enc.KeyHex)
		if err != nil {
			logger.Warn("解析流 %d 的 HLS 密钥失败: %v", enc.StreamID, err)
			continue
		}
		if err := s.hlsSvc.SetKey(st.App, st.Stream, raw, enc.KeyExpireSec); err != nil {
			logger.Warn("恢复流 %s/%s 的 HLS 密钥失败: %v", st.App, st.Stream, err)
		}
	}
	logger.Info("已恢复 %d 条 HLS 加密配置", len(list))
}
