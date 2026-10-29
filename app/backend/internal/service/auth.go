package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"livems/internal/config"
	"livems/internal/database"
	"livems/internal/models"
	"livems/internal/pkg/jwt"
	"livems/internal/pkg/logger"
	"livems/internal/pkg/util"
	"livems/internal/repository"
)

// AuthService 播放分发鉴权服务。
//
// 所有播放格式（RTMP/HTTP-FLV/WS-FLV/HLS）共用同一套鉴权逻辑：
//   - 令牌由 Redis 存储（带 TTL），MySQL 留档用于审计；
//   - 令牌可绑定 IP（支持单 IP 与 CIDR 列表）；
//   - 令牌可限定流与格式。
type AuthService struct {
	cfg       *config.Config
	tokenRepo *repository.TokenRepo
	streamRepo *repository.StreamRepo
	configRepo *repository.ConfigRepo
}

// NewAuthService 创建鉴权服务。
func NewAuthService(cfg *config.Config) *AuthService {
	return &AuthService{
		cfg:        cfg,
		tokenRepo:  repository.NewTokenRepo(),
		streamRepo: repository.NewStreamRepo(),
		configRepo: repository.NewConfigRepo(),
	}
}

// AuthPolicy 是从数据库读取的运行时鉴权策略。
type AuthPolicy struct {
	Enabled        bool   `json:"enabled"`         // 是否开启鉴权
	TokenExpire    int    `json:"token_expire"`    // 默认令牌有效期(秒)
	IPBindEnabled  bool   `json:"ip_bind_enabled"` // 是否允许 IP 绑定
	ParamName      string `json:"param_name"`      // 令牌参数名
	AllowEmpty     bool   `json:"allow_empty"`     // 是否允许无令牌播放
	AllowAnonymous bool   `json:"allow_anonymous"`
}

// Policy 返回当前生效的鉴权策略。
//
// 策略优先从 sys_config 表读取（后台可改，实时生效），
// 读取失败时回退到配置文件中的默认值。
func (s *AuthService) Policy() *AuthPolicy {
	policy := &AuthPolicy{
		Enabled:       s.cfg.Auth.Enabled,
		TokenExpire:   s.cfg.Auth.TokenExpire,
		IPBindEnabled: s.cfg.Auth.IPBindEnabled,
		ParamName:     s.cfg.Auth.ParamName,
		AllowEmpty:    s.cfg.Auth.AllowEmpty,
	}
	raw, err := s.configRepo.Get("auth")
	if err != nil || raw == "" {
		return policy
	}
	var stored AuthPolicy
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return policy
	}
	if stored.ParamName == "" {
		stored.ParamName = policy.ParamName
	}
	if stored.TokenExpire <= 0 {
		stored.TokenExpire = policy.TokenExpire
	}
	return &stored
}

// SavePolicy 保存鉴权策略到数据库（后台配置实时生效，无需重启）。
func (s *AuthService) SavePolicy(p *AuthPolicy) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.configRepo.Set("auth", string(data), "播放鉴权策略")
}

// redisKey 构造播放令牌的 Redis key。
func (s *AuthService) redisKey(token string) string {
	return database.RedisKey(s.cfg.Redis.Prefix, "playtoken", token)
}

// CreateToken 生成一个播放令牌。
//
// 参数：
//   - streamID 为 0 表示不限流
//   - expireSec 为 0 时使用策略中的默认有效期
//   - ipBind 为空表示不绑定 IP
//   - format 为空表示所有格式共用
func (s *AuthService) CreateToken(streamID uint64, expireSec int, ipBind, format string) (*models.PlayToken, error) {
	policy := s.Policy()
	if expireSec <= 0 {
		expireSec = policy.TokenExpire
	}
	if expireSec <= 0 {
		expireSec = 3600
	}
	if ipBind != "" && !policy.IPBindEnabled {
		return nil, fmt.Errorf("当前策略未开启 IP 绑定鉴权")
	}
	// 校验 IP 绑定的合法性
	if ipBind != "" {
		for _, ip := range strings.Split(ipBind, ",") {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			if !util.IsValidIP(ip) && !strings.Contains(ip, "/") {
				return nil, fmt.Errorf("IP 绑定格式不合法: %s", ip)
			}
		}
	}

	token := &models.PlayToken{
		Token:     jwt.RandomToken(24),
		StreamID:  streamID,
		ExpireAt:  time.Now().Add(time.Duration(expireSec) * time.Second).Unix(),
		IPBind:    ipBind,
		Format:    format,
		CreatedAt: time.Now().Unix(),
	}
	// 关联流信息（便于校验时快速判断）
	if streamID > 0 {
		if st, err := s.streamRepo.GetByID(streamID); err == nil && st != nil {
			token.App = st.App
			token.Stream = st.Stream
		}
	}

	// 写入 Redis（带 TTL，自动过期）
	ttl := time.Until(time.Unix(token.ExpireAt, 0))
	if ttl <= 0 {
		ttl = time.Duration(expireSec) * time.Second
	}
	if err := database.RDB.Set(context.Background(), s.redisKey(token.Token),
		token.ExpireAt, ttl).Err(); err != nil {
		return nil, fmt.Errorf("写入播放令牌缓存失败: %w", err)
	}

	// MySQL 留档
	if err := s.tokenRepo.Create(token); err != nil {
		logger.Warn("播放令牌留档失败: %v", err)
	}
	return token, nil
}

// Verify 校验一个播放令牌是否有效。
//
// 参数：
//   - token 为令牌值（可能为空）
//   - clientIP 为客户端 IP
//   - app/stream 为当前播放的流
//   - format 为当前播放格式
//
// 返回 true 表示允许播放。
func (s *AuthService) Verify(token, clientIP, app, stream, format string) (bool, string) {
	policy := s.Policy()

	// 鉴权整体关闭时直接放行
	if !policy.Enabled {
		return true, ""
	}

	// 未携带令牌
	if token == "" {
		if policy.AllowEmpty {
			return true, ""
		}
		return false, "缺少播放令牌"
	}

	// 从 Redis 校验（存在即有效，TTL 自动管理过期）
	expireAt, err := database.RDB.Get(context.Background(), s.redisKey(token)).Int64()
	if err == redis.Nil {
		// Redis 中不存在，回落到 MySQL 校验（兼容 Redis 重启等场景）
		return s.verifyFromDB(token, clientIP, app, stream, format)
	}
	if err != nil {
		logger.Warn("校验播放令牌缓存异常: %v", err)
		return s.verifyFromDB(token, clientIP, app, stream, format)
	}
	if expireAt > 0 && time.Now().Unix() > expireAt {
		return false, "播放令牌已过期"
	}

	// 取留档信息做细粒度校验
	rec, err := s.tokenRepo.GetByToken(token)
	if err != nil {
		// 无留档记录时，只要 Redis 有效即放行
		return true, ""
	}
	return s.checkRecord(rec, clientIP, app, stream, format)
}

// verifyFromDB 从 MySQL 校验令牌（Redis 不可用时的兜底）。
func (s *AuthService) verifyFromDB(token, clientIP, app, stream, format string) (bool, string) {
	rec, err := s.tokenRepo.GetByToken(token)
	if err != nil {
		return false, "播放令牌无效"
	}
	if rec.ExpireAt > 0 && time.Now().Unix() > rec.ExpireAt {
		return false, "播放令牌已过期"
	}
	return s.checkRecord(rec, clientIP, app, stream, format)
}

// checkRecord 校验令牌记录的细粒度约束。
func (s *AuthService) checkRecord(rec *models.PlayToken, clientIP, app, stream, format string) (bool, string) {
	// 限定流的令牌只能播放该流
	if rec.StreamID > 0 {
		if rec.App != "" && rec.App != app {
			return false, "播放令牌与当前流不匹配"
		}
		if rec.Stream != "" && rec.Stream != stream {
			return false, "播放令牌与当前流不匹配"
		}
	}
	// 限定格式的令牌只能播放该格式
	if rec.Format != "" && format != "" && rec.Format != format {
		return false, "播放令牌与当前播放格式不匹配"
	}
	// IP 绑定校验
	if rec.IPBind != "" {
		if clientIP == "" {
			return false, "播放令牌已绑定 IP，无法识别客户端地址"
		}
		if !util.IPInList(clientIP, rec.IPBind) {
			return false, "播放令牌与客户端 IP 不匹配"
		}
	}
	return true, ""
}

// Revoke 吊销一个令牌。
func (s *AuthService) Revoke(token string) error {
	if err := database.RDB.Del(context.Background(), s.redisKey(token)).Err(); err != nil {
		return err
	}
	return nil
}

// Delete 删除令牌（同时清除缓存）。
func (s *AuthService) Delete(id uint64) error {
	rec, err := s.tokenRepo.GetByID(id)
	if err == nil && rec != nil {
		_ = s.Revoke(rec.Token)
	}
	return s.tokenRepo.Delete(id)
}

// List 分页查询令牌。
func (s *AuthService) List(page, pageSize int) ([]models.PlayToken, int64, error) {
	return s.tokenRepo.List(page, pageSize)
}

// Cleanup 清理过期的令牌留档。
func (s *AuthService) Cleanup() {
	if n, err := s.tokenRepo.DeleteExpired(); err == nil && n > 0 {
		logger.Info("清理过期播放令牌留档 %d 条", n)
	}
}
