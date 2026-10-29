package hls

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"livems/internal/config"
	"livems/internal/pkg/logger"
)

// Service 是 HLS 加密播放代理服务。
//
// 工作流程：
//  1. 客户端请求 /hls-enc/<app>/<stream>.m3u8
//  2. 服务回源 ZLMediaKit 拉取原始 m3u8
//  3. 解析切片列表，为每个切片计算 IV
//  4. 客户端请求切片 /hls-enc/<app>/<stream>/<seg>.ts 时，服务回源拉取原始切片并加密后返回
//  5. 服务同时重写 m3u8，插入 #EXT-X-KEY 指向本服务的密钥接口
type Service struct {
	cfg      *config.Config
	upstream *Upstream
	cache    *Cache

	// keyMu 保护密钥表的并发访问
	keyMu sync.RWMutex
	// keys 是流标识 -> 密钥的映射
	keys map[string][]byte
	// keyExpire 是流标识 -> 密钥过期时间的映射
	keyExpire map[string]time.Time
}

// NewService 创建 HLS 加密服务。
func NewService(cfg *config.Config) *Service {
	return &Service{
		cfg:       cfg,
		upstream:  NewUpstream(cfg.HLS.UpstreamURL),
		cache:     NewCache(30 * time.Second),
		keys:      make(map[string][]byte),
		keyExpire: make(map[string]time.Time),
	}
}

// SetKey 为某条流设置密钥（并持久化到磁盘）。
//
// expireSec 为 0 表示永不过期；大于 0 时密钥在该时间后自动轮换。
func (s *Service) SetKey(app, stream string, key []byte, expireSec int) error {
	if len(key) != 16 {
		return fmt.Errorf("AES-128 密钥必须为 16 字节")
	}
	name := KeyFileName(app, stream)
	if err := WriteKeyFile(s.cfg.HLS.KeyDir, name, key); err != nil {
		return err
	}
	streamKey := app + "/" + stream
	s.keyMu.Lock()
	s.keys[streamKey] = key
	if expireSec > 0 {
		s.keyExpire[streamKey] = time.Now().Add(time.Duration(expireSec) * time.Second)
	} else {
		delete(s.keyExpire, streamKey)
	}
	s.keyMu.Unlock()
	logger.Info("已设置 HLS 加密密钥: %s", streamKey)
	return nil
}

// GetKey 获取某条流的密钥。
func (s *Service) GetKey(app, stream string) ([]byte, error) {
	streamKey := app + "/" + stream
	s.keyMu.RLock()
	key, ok := s.keys[streamKey]
	s.keyMu.RUnlock()
	if ok {
		return key, nil
	}
	// 从磁盘加载
	raw, err := ReadKeyFile(s.cfg.HLS.KeyDir, KeyFileName(app, stream))
	if err != nil {
		return nil, fmt.Errorf("读取密钥失败: %w", err)
	}
	if len(raw) != 16 {
		return nil, fmt.Errorf("密钥文件内容不合法")
	}
	s.keyMu.Lock()
	s.keys[streamKey] = raw
	s.keyMu.Unlock()
	return raw, nil
}

// KeyExpired 判断某条流的密钥是否已过期。
func (s *Service) KeyExpired(app, stream string) bool {
	streamKey := app + "/" + stream
	s.keyMu.RLock()
	exp, ok := s.keyExpire[streamKey]
	s.keyMu.RUnlock()
	if !ok {
		return false
	}
	return time.Now().After(exp)
}

// fetchSource 拉取原始媒体内容：优先从 ZLM 落盘目录直读，失败回退 HTTP 回源。
// relPath 形如 "live/e2etest/hls.m3u8" 或 "live/e2etest/2026-10-27/11/17-57_16.ts"。
func (s *Service) fetchSource(ctx context.Context, relPath string) ([]byte, error) {
	// 防路径穿越
	if strings.Contains(relPath, "..") {
		return nil, fmt.Errorf("非法的资源路径")
	}
	// 优先：文件系统直读（同机部署时 ZLM 的 HLS 切片直接落盘）
	if dir := s.cfg.HLS.ZlmWWWDir; dir != "" {
		data, err := os.ReadFile(dir + "/" + relPath)
		if err == nil {
			return data, nil
		}
		if !os.IsNotExist(err) {
			logger.Warn("直读 ZLM 落盘文件失败，回退 HTTP 回源: %s: %v", relPath, err)
		}
	}
	// 回退：HTTP 回源（注意：若 ZLM 的 on_play 开启鉴权，回源请求可能被 401 拒绝）
	data, _, err := s.upstream.Fetch(ctx, "/"+relPath)
	return data, err
}

// HandleM3U8 处理加密 m3u8 请求。
//
// app/stream 为流标识；keyURL 为写入 EXT-X-KEY 的密钥地址。
func (s *Service) HandleM3U8(ctx context.Context, w http.ResponseWriter, app, stream, keyURL string) error {
	// 拉取原始 m3u8：优先从 ZLM 落盘目录直读（绕过 on_play 鉴权），失败回退 HTTP 回源
	// ZLMediaKit HLS 标准路径：/{app}/{stream}/hls.m3u8
	relPath := fmt.Sprintf("%s/%s/hls.m3u8", app, stream)
	data, err := s.fetchSource(ctx, relPath)
	if err != nil {
		return fmt.Errorf("回源拉取 m3u8 失败: %w", err)
	}
	orig, err := ParseM3U8(string(data))
	if err != nil {
		return err
	}

	// 为每个切片计算 IV（使用媒体序号，RFC 8216 §5.2）
	ivs := make(map[int64]string, len(orig.Segments))
	for _, seg := range orig.Segments {
		iv := IVFromSequence(seg.Sequence)
		ivs[seg.Sequence] = hex.EncodeToString(iv)
	}

	// 切片重写为本服务代理的绝对路径
	segPrefix := fmt.Sprintf("/hls-enc/%s/%s/", app, stream)
	encrypted := buildEncryptedM3U8(orig, keyURL, ivs, segPrefix)

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(encrypted))
	return nil
}

// HandleSegment 处理加密切片请求。
//
// 返回加密后的切片内容。IV 由切片序号推导，与 m3u8 中声明的一致。
func (s *Service) HandleSegment(ctx context.Context, w http.ResponseWriter, app, stream, segName string, seq int64) error {
	// 先查缓存
	cacheKey := fmt.Sprintf("%s/%s/%s", app, stream, segName)
	if data, ok := s.cache.Get(cacheKey); ok {
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return nil
	}

	// 拉取原始切片（与 m3u8 同目录：/{app}/{stream}/{seg}，优先文件直读）
	relPath := fmt.Sprintf("%s/%s/%s", app, stream, segName)
	data, err := s.fetchSource(ctx, relPath)
	if err != nil {
		return fmt.Errorf("回源拉取切片失败: %w", err)
	}

	// 加密
	key, err := s.GetKey(app, stream)
	if err != nil {
		return err
	}
	enc, err := NewEncryptor(key)
	if err != nil {
		return err
	}
	iv := IVFromSequence(seq)
	cipherData, err := enc.Encrypt(data, iv)
	if err != nil {
		return err
	}

	// 缓存加密结果
	s.cache.Set(cacheKey, cipherData, "video/mp2t")

	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(cipherData)
	return nil
}

// HandleKey 处理密钥请求，返回 16 字节原始密钥。
func (s *Service) HandleKey(ctx context.Context, w http.ResponseWriter, app, stream string) error {
	key, err := s.GetKey(app, stream)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(key)
	return nil
}

// Cleanup 定期清理过期缓存。
func (s *Service) Cleanup() {
	s.cache.Cleanup()
}

// EnsureKeyDir 确保密钥目录存在。
func (s *Service) EnsureKeyDir() error {
	return os.MkdirAll(s.cfg.HLS.KeyDir, 0o750)
}

// StreamKeyOf 拼接流的内部标识。
func StreamKeyOf(app, stream string) string { return app + "/" + stream }

// SplitStreamPath 从 "app/stream" 中拆出 app 与 stream。
func SplitStreamPath(path string) (app, stream string, err error) {
	idx := strings.Index(path, "/")
	if idx <= 0 || idx == len(path)-1 {
		return "", "", fmt.Errorf("非法的流路径: %s", path)
	}
	return path[:idx], path[idx+1:], nil
}
