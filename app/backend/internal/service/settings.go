package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"livems/internal/config"
	"livems/internal/pkg/reqctx"
	"livems/internal/repository"
)

// IngressConfig 对外接入配置（推流/播放地址的对外主机与端口）。
//
// 存储于 sys_config 表（config_key=ingress），后台修改后实时生效，无需重启。
// 适用于内网服务器 + 公网端口映射的部署形态：这里配置的端口是**公网映射后**的端口。
type IngressConfig struct {
	// PushSchema 推流协议：rtmp / rtmps
	PushSchema string `json:"push_schema"`
	// PushHost 推流主机（域名或 IP），如 live.example.com
	PushHost string `json:"push_host"`
	// PushPort 对外 RTMP 端口（公网映射后的端口），rtmp 默认 1935、rtmps 默认 443
	PushPort int `json:"push_port"`
	// RtspPort 对外 RTSP 端口（展示用，0 表示不开放）
	RtspPort int `json:"rtsp_port"`
	// FollowPlayDomain 推流主机是否跟随默认播放域名（此时 PushHost 仅作为兜底）
	FollowPlayDomain bool `json:"follow_play_domain"`
}

// ingressConfigKey 是 sys_config 中的键名。
const ingressConfigKey = "ingress"

// SettingsService 系统设置服务。
type SettingsService struct {
	cfg        *config.Config
	configRepo *repository.ConfigRepo
	playSvc    *PlayService
}

// NewSettingsService 创建系统设置服务。
func NewSettingsService(cfg *config.Config, playSvc *PlayService) *SettingsService {
	return &SettingsService{
		cfg:        cfg,
		configRepo: repository.NewConfigRepo(),
		playSvc:    playSvc,
	}
}

// GetIngress 读取接入配置；未配置或解析失败时返回基于配置文件的默认值。
func (s *SettingsService) GetIngress() *IngressConfig {
	def := &IngressConfig{
		PushSchema: "rtmp",
		PushHost:   "",
		PushPort:   s.cfg.ZLM.RTMPRTMP,
		RtspPort:   0,
	}
	if def.PushPort <= 0 {
		def.PushPort = 1935
	}
	raw, err := s.configRepo.Get(ingressConfigKey)
	if err != nil || strings.TrimSpace(raw) == "" {
		return def
	}
	ing := &IngressConfig{}
	if err := json.Unmarshal([]byte(raw), ing); err != nil {
		return def
	}
	if ing.PushSchema == "" {
		ing.PushSchema = "rtmp"
	}
	if ing.PushPort <= 0 {
		ing.PushPort = def.PushPort
	}
	return ing
}

// SaveIngress 校验并保存接入配置。
func (s *SettingsService) SaveIngress(ing *IngressConfig) error {
	ing.PushSchema = strings.ToLower(strings.TrimSpace(ing.PushSchema))
	if ing.PushSchema == "" {
		ing.PushSchema = "rtmp"
	}
	if ing.PushSchema != "rtmp" && ing.PushSchema != "rtmps" {
		return fmt.Errorf("推流协议只支持 rtmp 或 rtmps")
	}
	ing.PushHost = strings.TrimSpace(ing.PushHost)
	if ing.PushHost != "" {
		// 不允许带协议头、路径与端口（端口单独配置）
		if strings.Contains(ing.PushHost, "://") || strings.Contains(ing.PushHost, "/") || strings.Contains(ing.PushHost, ":") {
			return fmt.Errorf("推流主机只需填写域名或 IP，不要带协议、端口或路径")
		}
	}
	if ing.PushPort < 0 || ing.PushPort > 65535 {
		return fmt.Errorf("对外端口需在 0-65535 之间（0 表示使用协议默认端口）")
	}
	if ing.RtspPort < 0 || ing.RtspPort > 65535 {
		return fmt.Errorf("RTSP 端口需在 0-65535 之间（0 表示不开放）")
	}
	data, err := json.Marshal(ing)
	if err != nil {
		return err
	}
	return s.configRepo.Set(ingressConfigKey, string(data), "对外接入配置（推流/播放地址的对外主机与端口）")
}

// resolvePushHost 解析最终生效的推流主机。
func (s *SettingsService) resolvePushHost(ctx context.Context, ing *IngressConfig) string {
	if ing.FollowPlayDomain {
		if d := s.playSvc.DefaultDomain(); d != nil {
			return d.Host
		}
	}
	if ing.PushHost != "" {
		return ing.PushHost
	}
	// 兜底：默认播放域名 → 浏览器当前访问的主机 → 本机
	if d := s.playSvc.DefaultDomain(); d != nil {
		return d.Host
	}
	if o, ok := reqctx.From(ctx); ok {
		return o.Host
	}
	return "127.0.0.1"
}

// PushBaseURL 返回推流地址基础部分（含协议、主机、端口），如 rtmp://live.example.com:1935。
//
// 默认端口省略：rtmp=1935、rtmps=443。
func (s *SettingsService) PushBaseURL(ctx context.Context) string {
	ing := s.GetIngress()
	host := s.resolvePushHost(ctx, ing)
	port := ing.PushPort
	defaultPort := 1935
	if ing.PushSchema == "rtmps" {
		defaultPort = 443
	}
	if port > 0 && port != defaultPort {
		return fmt.Sprintf("%s://%s:%d", ing.PushSchema, host, port)
	}
	return fmt.Sprintf("%s://%s", ing.PushSchema, host)
}
