// Package service 实现业务逻辑层。
package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"livems/internal/config"
	"livems/internal/models"
	"livems/internal/pkg/reqctx"
	"livems/internal/pkg/logger"
	"livems/internal/repository"
)

// PlayService 负责播放地址生成、播放格式开关与播放域名管理。
type PlayService struct {
	domainRepo *repository.DomainRepo
	formatRepo *repository.FormatRepo
	cfg        *config.Config

	// mu 保护下面两个缓存（配置变更无需重启服务，靠缓存失效实现热更新）
	mu           sync.RWMutex
	enabledFmt   map[string]bool
	domains      []models.PlayDomain
	cacheAt      time.Time
	cacheTTL     time.Duration
}

// NewPlayService 创建播放服务。
func NewPlayService(cfg *config.Config) *PlayService {
	return &PlayService{
		domainRepo: repository.NewDomainRepo(),
		formatRepo: repository.NewFormatRepo(),
		cfg:        cfg,
		enabledFmt: map[string]bool{},
		cacheTTL:   5 * time.Second,
	}
}

// Format 支持的播放格式标识。
const (
	FormatRTMP     = "rtmp"
	FormatHTTPFLV  = "http-flv"
	FormatWSFLV    = "ws-flv"
	FormatHLS      = "hls"
)

// AllFormats 返回全部受支持的播放格式。
func AllFormats() []string {
	return []string{FormatRTMP, FormatHTTPFLV, FormatWSFLV, FormatHLS}
}

// refreshCache 从数据库刷新播放配置缓存。
func (s *PlayService) refreshCache() {
	fmts, err := s.formatRepo.EnabledFormats()
	if err != nil {
		logger.Warn("刷新播放格式缓存失败: %v", err)
		return
	}
	domains, err := s.domainRepo.ListEnabled()
	if err != nil {
		logger.Warn("刷新播放域名缓存失败: %v", err)
		return
	}
	s.mu.Lock()
	s.enabledFmt = fmts
	s.domains = domains
	s.cacheAt = time.Now()
	s.mu.Unlock()
}

// ensureCache 确保缓存是最新的（TTL 内直接复用）。
func (s *PlayService) ensureCache() {
	s.mu.RLock()
	fresh := time.Since(s.cacheAt) < s.cacheTTL
	s.mu.RUnlock()
	if !fresh {
		s.refreshCache()
	}
}

// FormatEnabled 判断某种播放格式当前是否开启对外分发。
func (s *PlayService) FormatEnabled(format string) bool {
	s.ensureCache()
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabledFmt[format]
}

// Domains 返回所有启用的播放域名。
func (s *PlayService) Domains() []models.PlayDomain {
	s.ensureCache()
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.PlayDomain, len(s.domains))
	copy(out, s.domains)
	return out
}

// DefaultDomain 返回默认播放域名（没有标记默认时取第一个启用的）。
func (s *PlayService) DefaultDomain() *models.PlayDomain {
	domains := s.Domains()
	for i := range domains {
		if domains[i].IsDefault == 1 {
			return &domains[i]
		}
	}
	if len(domains) > 0 {
		return &domains[0]
	}
	return nil
}

// DomainByID 按ID查询播放域名。
func (s *PlayService) DomainByID(id uint64) (*models.PlayDomain, error) {
	return s.domainRepo.GetByID(id)
}

// ListDomains 查询全部播放域名（含禁用的）。
func (s *PlayService) ListDomains() ([]models.PlayDomain, error) {
	return s.domainRepo.List()
}

// CreateDomain 新增播放域名。
func (s *PlayService) CreateDomain(d *models.PlayDomain) error {
	if err := s.validateDomain(d); err != nil {
		return err
	}
	// 第一个域名自动设为默认
	if d.IsDefault == 1 {
		if err := s.domainRepo.ClearDefault(); err != nil {
			return err
		}
	}
	if err := s.domainRepo.Create(d); err != nil {
		return err
	}
	s.refreshCache()
	return nil
}

// UpdateDomain 更新播放域名。
func (s *PlayService) UpdateDomain(d *models.PlayDomain) error {
	if err := s.validateDomain(d); err != nil {
		return err
	}
	if d.IsDefault == 1 {
		if err := s.domainRepo.ClearDefault(); err != nil {
			return err
		}
	}
	if err := s.domainRepo.Update(d); err != nil {
		return err
	}
	s.refreshCache()
	return nil
}

// SetDefaultDomain 设置默认域名。
func (s *PlayService) SetDefaultDomain(id uint64) error {
	if err := s.domainRepo.SetDefault(id); err != nil {
		return err
	}
	s.refreshCache()
	return nil
}

// DeleteDomain 删除播放域名。
func (s *PlayService) DeleteDomain(id uint64) error {
	if err := s.domainRepo.Delete(id); err != nil {
		return err
	}
	s.refreshCache()
	return nil
}

// validateDomain 校验播放域名参数。
func (s *PlayService) validateDomain(d *models.PlayDomain) error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("域名名称不能为空")
	}
	if strings.TrimSpace(d.Host) == "" {
		return fmt.Errorf("域名主机名不能为空")
	}
	if d.Schema == "" {
		d.Schema = "http"
	}
	if d.Schema != "http" && d.Schema != "https" {
		return fmt.Errorf("域名协议只支持 http 或 https")
	}
	if d.Port <= 0 {
		if d.Schema == "https" {
			d.Port = 443
		} else {
			d.Port = 80
		}
	}
	if d.Status == 0 {
		d.Status = 1
	}
	return nil
}

// ListFormats 查询全部播放格式配置。
func (s *PlayService) ListFormats() ([]models.PlayFormat, error) {
	return s.formatRepo.List()
}

// SetFormatEnabled 开启/关闭某种播放格式的对外分发。
func (s *PlayService) SetFormatEnabled(format string, enabled bool) error {
	if !isValidFormat(format) {
		return fmt.Errorf("不支持的播放格式: %s", format)
	}
	val := 0
	if enabled {
		val = 1
	}
	if err := s.formatRepo.UpdateEnabled(format, val); err != nil {
		return err
	}
	s.refreshCache()
	return nil
}

// UpdateFormat 更新播放格式配置。
func (s *PlayService) UpdateFormat(f *models.PlayFormat) error {
	if !isValidFormat(f.Format) {
		return fmt.Errorf("不支持的播放格式: %s", f.Format)
	}
	if err := s.formatRepo.Update(f); err != nil {
		return err
	}
	s.refreshCache()
	return nil
}

// isValidFormat 判断格式标识是否受支持。
func isValidFormat(format string) bool {
	for _, f := range AllFormats() {
		if f == format {
			return true
		}
	}
	return false
}

// PlayURLs 一个流的多格式播放地址集合。
type PlayURLs struct {
	StreamID   uint64            `json:"stream_id"`
	App        string            `json:"app"`
	Stream     string            `json:"stream"`
	Domain     string            `json:"domain"`
	Protocol   string            `json:"protocol"`   // 使用的协议 http/https
	URLs       map[string]string `json:"urls"`        // 各格式播放地址
	Disabled   []string          `json:"disabled"`    // 被关闭的格式
	Encrypted  bool              `json:"encrypted"`   // HLS 是否加密
	KeyURL     string            `json:"key_url"`     // HLS 密钥地址（加密时）
	Token      string            `json:"token"`       // 附带的播放令牌
	ExpiresAt  int64             `json:"expires_at"`  // 令牌过期时间
	GeneratedAt int64            `json:"generated_at"`
}

// GenerateURLs 为指定的流生成多格式播放地址。
//
// domainID 为 0 时使用默认域名；token 为空表示不附带鉴权令牌。
// 被后台关闭的格式不会出现在返回结果中，而是列入 Disabled。
func (s *PlayService) GenerateURLs(ctx context.Context, st *models.Stream, domainID uint64, token string, expireAt int64) (*PlayURLs, error) {
	// 选择域名
	var domain *models.PlayDomain
	if domainID > 0 {
		d, err := s.domainRepo.GetByID(domainID)
		if err != nil {
			return nil, fmt.Errorf("播放域名不存在: %w", err)
		}
		if d.Status != 1 {
			return nil, fmt.Errorf("播放域名已禁用: %s", d.Host)
		}
		domain = d
	} else {
		domain = s.DefaultDomain()
	}
	if domain == nil {
		// 未配置任何播放域名：使用浏览器当前访问的地址（全新部署开箱即用）
		if o, ok := reqctx.From(ctx); ok {
			domain = &models.PlayDomain{Name: "当前访问地址", Schema: o.Scheme, Host: o.Host, Port: o.Port, Status: 1}
		} else {
			return nil, fmt.Errorf("没有可用的播放域名，请先在后台「域名配置」中添加")
		}
	}

	// 组装基础 URL
	base := domainBaseURL(domain)
	rtmpHost := domain.Host
	rtmpPort := s.cfg.ZLM.RTMPRTMP
	if rtmpPort > 0 && rtmpPort != 1935 {
		rtmpHost = fmt.Sprintf("%s:%d", domain.Host, rtmpPort)
	}

	result := &PlayURLs{
		StreamID:    st.ID,
		App:         st.App,
		Stream:      st.Stream,
		Domain:      domain.Host,
		Protocol:    domain.Schema,
		URLs:        map[string]string{},
		Disabled:    []string{},
		Token:       token,
		ExpiresAt:   expireAt,
		GeneratedAt: time.Now().Unix(),
	}

	// 查询该流的 HLS 加密配置
	encrypted := false
	keyURL := ""
	if enc, err := s.encryptRepo().GetByStreamID(st.ID); err == nil && enc != nil && enc.Enabled == 1 {
		encrypted = true
		keyURL = enc.KeyURI
	}
	result.Encrypted = encrypted
	result.KeyURL = keyURL

	// 各格式的地址模板
	// 说明：RTMP 直连 ZLM 的 1935 端口；HTTP-FLV/WS-FLV/HLS 经由 Nginx 反代到 ZLM 的 HTTP 端口。
	// ZLMediaKit 地址规范：FLV 为 /{app}/{stream}.live.flv，HLS 为 /{app}/{stream}/hls.m3u8。
	templates := map[string]func() string{
		FormatRTMP: func() string {
			// RTMP 播放同样需要令牌（ZLM 会把 query 传给 on_play 校验）
			return fmt.Sprintf("rtmp://%s/%s/%s%s", rtmpHost, st.App, st.Stream, tokenQuery(token))
		},
		FormatHTTPFLV: func() string {
			return fmt.Sprintf("%s/%s/%s.live.flv%s", base, st.App, st.Stream, tokenQuery(token))
		},
		FormatWSFLV: func() string {
			wsScheme := "ws"
			if domain.Schema == "https" {
				wsScheme = "wss"
			}
			return fmt.Sprintf("%s://%s/%s/%s.live.flv%s", wsScheme, domainHostPort(domain), st.App, st.Stream, tokenQuery(token))
		},
		FormatHLS: func() string {
			if encrypted {
				// 加密流经由本系统的加密代理分发（m3u8 重写 + 切片加密）
				return fmt.Sprintf("%s/hls-enc/%s/%s.m3u8%s", base, st.App, st.Stream, tokenQuery(token))
			}
			return fmt.Sprintf("%s/%s/%s/hls.m3u8%s", base, st.App, st.Stream, tokenQuery(token))
		},
	}

	for _, format := range AllFormats() {
		if !s.FormatEnabled(format) {
			result.Disabled = append(result.Disabled, format)
			continue
		}
		result.URLs[format] = templates[format]()
	}
	return result, nil
}

// encryptRepo 返回加密配置仓储（延迟初始化，避免循环依赖）。
func (s *PlayService) encryptRepo() *repository.EncryptRepo {
	return repository.NewEncryptRepo()
}

// domainBaseURL 拼接域名的基础 URL（含协议与端口）。
func domainBaseURL(d *models.PlayDomain) string {
	return fmt.Sprintf("%s://%s", d.Schema, domainHostPort(d))
}

// domainHostPort 拼接域名的 host[:port]（默认端口省略）。
func domainHostPort(d *models.PlayDomain) string {
	port := d.Port
	if port == 0 {
		port = 80
	}
	// 80/443 是默认端口时不带端口号
	if (d.Schema == "http" && port == 80) || (d.Schema == "https" && port == 443) {
		return d.Host
	}
	return fmt.Sprintf("%s:%d", d.Host, port)
}

// tokenQuery 生成 token 查询串（无令牌时返回空串）。
func tokenQuery(token string) string {
	if token == "" {
		return ""
	}
	return "?token=" + token
}
