package service

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"livems/internal/config"
	"livems/internal/pkg/logger"
)

// CertInfo 证书信息视图。
type CertInfo struct {
	Name      string   `json:"name"`       // 证书目录名（通常为域名）
	Domains   []string `json:"domains"`    // 覆盖域名（CN + SAN）
	Issuer    string   `json:"issuer"`     // 颁发者
	NotBefore int64    `json:"not_before"` // 生效时间
	NotAfter  int64    `json:"not_after"`  // 到期时间
	DaysLeft  int      `json:"days_left"`  // 剩余天数（负数=已过期）
	InUse     bool     `json:"in_use"`     // 是否被本站点 Nginx 配置引用
	HasKey    bool     `json:"has_key"`    // 私钥文件是否存在
}

// CertService SSL 证书管理服务。
//
// 证书存放遵循宝塔目录约定：<cert_dir>/<name>/fullchain.pem + privkey.pem。
// 应用证书 = 替换站点 conf 中的 ssl_certificate 路径 + nginx -t 校验 + reload；
// 校验失败自动回滚，不会造成站点 HTTPS 中断。
type CertService struct {
	cfg *config.Config
}

// NewCertService 创建证书服务。
func NewCertService(cfg *config.Config) *CertService {
	return &CertService{cfg: cfg}
}

// certNamePattern 证书名（目录名）安全校验：域名风格字符，防路径穿越。
var certNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._*-]{0,127}$`)

// List 扫描证书目录并返回全部证书信息。
func (s *CertService) List() ([]CertInfo, error) {
	dir := s.cfg.Nginx.CertDir
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取证书目录失败: %w", err)
	}
	inUse := s.currentCertPaths()
	out := make([]CertInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		fullchain := filepath.Join(dir, name, "fullchain.pem")
		privkey := filepath.Join(dir, name, "privkey.pem")
		info := CertInfo{Name: name}
		if _, err := os.Stat(privkey); err == nil {
			info.HasKey = true
		}
		if _, ok := inUse[fullchain]; ok {
			info.InUse = true
		}
		// 解析证书内容
		if cert, err := parseCertFile(fullchain); err == nil {
			info.Domains = certDomains(cert)
			info.Issuer = cert.Issuer.String()
			info.NotBefore = cert.NotBefore.Unix()
			info.NotAfter = cert.NotAfter.Unix()
			info.DaysLeft = int(time.Until(cert.NotAfter).Hours() / 24)
		}
		out = append(out, info)
	}
	return out, nil
}

// Save 上传/更新证书（PEM 文本）。
//
// apply 为 true 时保存后立即应用到本站点（含 nginx -t 校验与失败回滚）。
func (s *CertService) Save(name, fullchain, privkey string, apply bool) (*CertInfo, error) {
	if !certNamePattern.MatchString(name) || strings.Contains(name, "..") {
		return nil, fmt.Errorf("证书名称只允许字母、数字、点、中划线、下划线")
	}
	fullchain = strings.TrimSpace(fullchain) + "\n"
	privkey = strings.TrimSpace(privkey) + "\n"

	// 校验证书 PEM 可解析
	block, _ := pem.Decode([]byte(fullchain))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("证书内容不是有效的 PEM 格式（需要 fullchain 证书链）")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("证书解析失败: %v", err)
	}
	// 校验证书与私钥匹配
	if _, err := tls.X509KeyPair([]byte(fullchain), []byte(privkey)); err != nil {
		return nil, fmt.Errorf("证书与私钥不匹配: %v", err)
	}

	// 写入文件（目录 0700，fullchain 0644，privkey 0600）
	dir := filepath.Join(s.cfg.Nginx.CertDir, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("创建证书目录失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fullchain.pem"), []byte(fullchain), 0644); err != nil {
		return nil, fmt.Errorf("写入证书失败: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "privkey.pem"), []byte(privkey), 0600); err != nil {
		return nil, fmt.Errorf("写入私钥失败: %w", err)
	}
	logger.Info("证书已保存: %s (覆盖域名: %v)", name, certDomains(cert))

	if apply {
		if err := s.Apply(name); err != nil {
			return nil, err
		}
	}

	info := &CertInfo{
		Name:      name,
		Domains:   certDomains(cert),
		Issuer:    cert.Issuer.String(),
		NotBefore: cert.NotBefore.Unix(),
		NotAfter:  cert.NotAfter.Unix(),
		DaysLeft:  int(time.Until(cert.NotAfter).Hours() / 24),
		InUse:     apply,
		HasKey:    true,
	}
	return info, nil
}

// sslCertLineRe 匹配 conf 中的 ssl_certificate / ssl_certificate_key 行。
var (
	sslCertLineRe    = regexp.MustCompile(`(?m)^(\s*)ssl_certificate\s+\S+;\s*$`)
	sslCertKeyLineRe = regexp.MustCompile(`(?m)^(\s*)ssl_certificate_key\s+\S+;\s*$`)
)

// Apply 将指定证书应用到本站点 Nginx 配置。
//
// 流程：备份 conf -> 替换证书路径 -> nginx -t -> 失败则回滚 -> 成功则 reload。
func (s *CertService) Apply(name string) error {
	if !certNamePattern.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("非法的证书名称")
	}
	dir := filepath.Join(s.cfg.Nginx.CertDir, name)
	fullchain := filepath.Join(dir, "fullchain.pem")
	privkey := filepath.Join(dir, "privkey.pem")
	for _, f := range []string{fullchain, privkey} {
		if _, err := os.Stat(f); err != nil {
			return fmt.Errorf("证书文件不存在: %s", f)
		}
	}

	confPath := s.cfg.Nginx.VhostConf
	raw, err := os.ReadFile(confPath)
	if err != nil {
		return fmt.Errorf("读取 Nginx 配置失败: %w", err)
	}
	content := string(raw)
	if !sslCertLineRe.MatchString(content) || !sslCertKeyLineRe.MatchString(content) {
		return fmt.Errorf("Nginx 配置中未找到 ssl_certificate 配置行")
	}

	// 替换证书路径
	newContent := sslCertLineRe.ReplaceAllString(content, "${1}ssl_certificate    "+fullchain+";")
	newContent = sslCertKeyLineRe.ReplaceAllString(newContent, "${1}ssl_certificate_key    "+privkey+";")

	// 备份原配置
	backup := fmt.Sprintf("%s.bak.%d", confPath, time.Now().Unix())
	if err := os.WriteFile(backup, raw, 0644); err != nil {
		return fmt.Errorf("备份 Nginx 配置失败: %w", err)
	}
	// 写入新配置
	if err := os.WriteFile(confPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("写入 Nginx 配置失败: %w", err)
	}
	// nginx -t 校验，失败自动回滚
	if out, err := exec.Command("nginx", "-t").CombinedOutput(); err != nil {
		_ = os.WriteFile(confPath, raw, 0644)
		_ = exec.Command("nginx", "-t").Run()
		return fmt.Errorf("Nginx 配置校验失败，已自动回滚: %v: %s", err, strings.TrimSpace(string(out)))
	}
	// 重载
	parts := strings.Fields(s.cfg.Nginx.ReloadCmd)
	if len(parts) > 0 {
		if out, err := exec.Command(parts[0], parts[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("Nginx 重载失败: %v: %s", err, strings.TrimSpace(string(out)))
		}
	}
	logger.Info("证书已应用到站点: %s -> %s (备份: %s)", name, fullchain, backup)
	return nil
}

// Delete 删除证书目录（正在被站点引用的证书拒绝删除）。
func (s *CertService) Delete(name string) error {
	if !certNamePattern.MatchString(name) || strings.Contains(name, "..") {
		return fmt.Errorf("非法的证书名称")
	}
	dir := filepath.Join(s.cfg.Nginx.CertDir, name)
	inUse := s.currentCertPaths()
	if _, ok := inUse[filepath.Join(dir, "fullchain.pem")]; ok {
		return fmt.Errorf("该证书正在被站点使用，请先应用其它证书后再删除")
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	logger.Info("证书已删除: %s", name)
	return nil
}

// currentCertPaths 返回站点 conf 当前引用的证书文件路径集合。
func (s *CertService) currentCertPaths() map[string]struct{} {
	out := map[string]struct{}{}
	raw, err := os.ReadFile(s.cfg.Nginx.VhostConf)
	if err != nil {
		return out
	}
	re := regexp.MustCompile(`(?m)ssl_certificate\s+(\S+?);`)
	for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
		out[m[1]] = struct{}{}
	}
	return out
}

// parseCertFile 解析 PEM 证书文件中的第一张证书。
func parseCertFile(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("无效的 PEM 文件")
	}
	return x509.ParseCertificate(block.Bytes)
}

// certDomains 提取证书覆盖的域名（CN + SAN 去重）。
func certDomains(cert *x509.Certificate) []string {
	seen := map[string]bool{}
	out := []string{}
	if cert.Subject.CommonName != "" {
		seen[cert.Subject.CommonName] = true
		out = append(out, cert.Subject.CommonName)
	}
	for _, d := range cert.DNSNames {
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}
