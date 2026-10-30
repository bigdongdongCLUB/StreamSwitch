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
	// PushPort 对外推流端口（公网映射后的端口），0 表示使用本机配置的 RTMP / RTMPS 端口
	PushPort int `json:"push_port"`
	// RtspPort 对外 RTSP 端口（展示用，0 表示不开放）
	RtspPort int `json:"rtsp_port"`
	// FollowPlayDomain 推流主机是否跟随默认播放域名（此时 PushHost 仅作为兜底）
	FollowPlayDomain bool `json:"follow_play_domain"`
}

// IngressView 接入配置的展示视图：附带本机实际开放的推流端口（只读，供后台判断 RTMPS 是否可选）。
type IngressView struct {
	IngressConfig
	// RTMPPort 本机 RTMP 端口
	RTMPPort int `json:"rtmp_port"`
	// RTMPSPort 本机 RTMPS 端口，0 表示未开启 RTMPS
	RTMPSPort int `json:"rtmps_port"`
}

// errRTMPSDisabled 未开启 RTMPS 时选择 rtmps 推流协议的错误提示。
var errRTMPSDisabled = fmt.Errorf("本机未开启 RTMPS 推流端口，不能选择 RTMPS：请在服务器执行 sudo streamswitch，进入「端口设置」开启 RTMPS 端口后再保存")

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

// rtmpPort 返回本机 RTMP 端口。
func (s *SettingsService) rtmpPort() int {
	if s.cfg.ZLM.RTMPRTMP > 0 {
		return s.cfg.ZLM.RTMPRTMP
	}
	return 1935
}

// RTMPSEnabled 本机是否开启了 RTMPS 推流端口。
func (s *SettingsService) RTMPSEnabled() bool { return s.cfg.ZLM.RTMPSPort > 0 }

// GetIngressView 返回接入配置及本机推流端口信息。
func (s *SettingsService) GetIngressView() *IngressView {
	return &IngressView{IngressConfig: *s.GetIngress(), RTMPPort: s.rtmpPort(), RTMPSPort: s.cfg.ZLM.RTMPSPort}
}

// GetIngress 读取接入配置；未配置或解析失败时返回基于配置文件的默认值。
//
// 保存过 rtmps 但本机已关闭 RTMPS 端口时回退为 rtmp，避免给出无法连接的推流地址。
func (s *SettingsService) GetIngress() *IngressConfig {
	def := &IngressConfig{
		PushSchema: "rtmp",
		PushHost:   "",
		PushPort:   s.rtmpPort(),
		RtspPort:   0,
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
	if ing.PushSchema == "rtmps" && !s.RTMPSEnabled() {
		ing.PushSchema = "rtmp"
		ing.PushPort = def.PushPort
	}
	if ing.PushPort <= 0 {
		ing.PushPort = s.defaultPushPort(ing.PushSchema)
	}
	return ing
}

// defaultPushPort 返回协议对应的本机推流端口。
func (s *SettingsService) defaultPushPort(schema string) int {
	if schema == "rtmps" {
		return s.cfg.ZLM.RTMPSPort
	}
	return s.rtmpPort()
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
	if ing.PushSchema == "rtmps" && !s.RTMPSEnabled() {
		return errRTMPSDisabled
	}
	ing.PushHost = strings.TrimSpace(ing.PushHost)
	if ing.PushHost != "" {
		// 只接受合法域名或 IPv4（不带协议、端口、路径），端口单独配置
		h, err := normalizeHost("推流主机", ing.PushHost)
		if err != nil {
			return err
		}
		ing.PushHost = h
	}
	if ing.PushPort < 0 || ing.PushPort > 65535 {
		return fmt.Errorf("对外端口需在 0-65535 之间（0 表示使用本机推流端口）")
	}
	if ing.RtspPort < 0 || ing.RtspPort > 65535 {
		return fmt.Errorf("RTSP 端口需在 0-65535 之间（0 表示不开放）")
	}
	if ing.PushPort == 0 {
		ing.PushPort = s.defaultPushPort(ing.PushSchema)
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
