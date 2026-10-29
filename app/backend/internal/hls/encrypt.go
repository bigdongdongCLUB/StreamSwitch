// Package hls 实现 HLS AES-128 加密播放代理。
//
// 背景：
//   ZLMediaKit 本身不提供 HLS AES-128 加密能力（其 config.ini 的 [hls] 段只有切片、
//   保留、缓冲等参数，没有密钥/IV/EXT-X-KEY 相关配置），因此本包在应用层实现加密：
//
//	1. 从 ZLMediaKit 回源拉取原始 m3u8；
//	2. 逐个拉取 TS 切片，用 AES-128-CBC + PKCS7 加密；
//	3. 重写 m3u8，插入 #EXT-X-KEY:METHOD=AES-128,URI="...",IV=...；
//	4. 对外提供加密后的 m3u8 与切片，以及密钥文件。
//
// 加密规范（RFC 8216 §4.3.2.4）：
//   - 算法固定为 AES-128-CBC，密钥 16 字节；
//   - 采用 PKCS7 填充（TS 包长 188 字节，不是 16 的整数倍，必然需要填充）；
//   - 每个切片在片段边界重新初始化 CBC，IV 取自 EXT-X-KEY 的 IV 属性；
//   - 若 EXT-X-KEY 未指定 IV，则使用该分片的媒体序号（Media Sequence Number）
//     作为 16 字节大端 IV（RFC 8216 §5.2）。
//
// 该实现与 hls.js（video-dev/hls.js）的解密逻辑完全兼容。
package hls

import (
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Encryptor 负责 HLS 切片的 AES-128 加密。
type Encryptor struct {
	// key 是 16 字节 AES-128 密钥
	key []byte
	// block 是 CBC 分组加密器（使用零 IV，实际 IV 在每次加密时指定）
	block cipher.Block
}

// NewEncryptor 使用给定密钥创建加密器。
func NewEncryptor(key []byte) (*Encryptor, error) {
	if len(key) != 16 {
		return nil, fmt.Errorf("AES-128 密钥必须为 16 字节，当前为 %d 字节", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("创建 AES 加密器失败: %w", err)
	}
	return &Encryptor{key: key, block: block}, nil
}

// Encrypt 对一个切片加密，返回密文（含 PKCS7 填充）。
func (e *Encryptor) Encrypt(plain []byte, iv []byte) ([]byte, error) {
	if len(iv) != 16 {
		return nil, fmt.Errorf("IV 必须为 16 字节，当前为 %d 字节", len(iv))
	}
	// PKCS7 填充：填充长度 = 16 - (len % 16)，且至少补 1 字节（即使刚好整除）
	padLen := aes.BlockSize - len(plain)%aes.BlockSize
	padded := make([]byte, len(plain)+padLen)
	copy(padded, plain)
	for i := len(plain); i < len(padded); i++ {
		padded[i] = byte(padLen)
	}

	out := make([]byte, len(padded))
	cipher.NewCBCEncrypter(e.block, iv).CryptBlocks(out, padded)
	return out, nil
}

// IVFromSequence 按 RFC 8216 §5.2 由媒体序号生成 IV。
// 序号以 16 字节大端表示，左侧补零。
func IVFromSequence(seq int64) []byte {
	iv := make([]byte, 16)
	// 使用大端序写入低 8 字节
	for i := 0; i < 8; i++ {
		iv[15-i] = byte(uint64(seq) >> (8 * uint(i)))
	}
	return iv
}

// IVFromHex 从十六进制字符串解析 IV。
func IVFromHex(s string) ([]byte, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "0x")
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("解析 IV 失败: %w", err)
	}
	if len(b) != 16 {
		return nil, fmt.Errorf("IV 必须为 16 字节，当前为 %d 字节", len(b))
	}
	return b, nil
}

// GenerateKey 生成一个随机的 16 字节 AES-128 密钥，返回其十六进制表示。
func GenerateKey() (hexKey string, raw []byte, err error) {
	raw = make([]byte, 16)
	if _, err = rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("生成随机密钥失败: %w", err)
	}
	return hex.EncodeToString(raw), raw, nil
}

// SegmentInfo 描述 m3u8 中的一个切片。
type SegmentInfo struct {
	// URI 是切片在 m3u8 中的原始地址
	URI string
	// Duration 是切片时长（秒）
	Duration float64
	// Sequence 是该切片的媒体序号
	Sequence int64
	// KeyURI 是该切片对应的密钥地址（继承自 EXT-X-KEY）
	KeyURI string
	// IV 是该切片使用的 IV（十六进制），可能为空表示使用序号推导
	IV string
	// IsKeySegment 标记该行是否为密钥声明
	IsKeySegment bool
}

// M3U8 表示解析后的播放列表。
type M3U8 struct {
	// Header 是 m3u8 的头部标签（不含切片行）
	Header []string
	// Segments 是切片列表
	Segments []SegmentInfo
	// MediaSequence 是起始媒体序号
	MediaSequence int64
	// TargetDuration 是目标切片时长
	TargetDuration float64
	// HasEndList 表示是否为点播列表（本系统不使用）
	HasEndList bool
}

// ParseM3U8 解析 m3u8 内容。
//
// 解析要点：
//   - 记录 #EXT-X-MEDIA-SEQUENCE 作为媒体序号起点；
//   - #EXTINF 后面紧跟的 URI 行是一个切片；
//   - #EXT-X-KEY 的 URI/IV 属性作用于其后的所有切片，直到下一个 #EXT-X-KEY。
func ParseM3U8(content string) (*M3U8, error) {
	m := &M3U8{MediaSequence: 0}
	sc := bufio.NewScanner(strings.NewReader(content))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var pendingDuration float64
	var pendingKeyURI, pendingIV string
	var havePendingIV bool
	seq := int64(0)
	started := false

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			// EXTINF 属于切片描述，不计入头部标签（由 Segments 重建输出）
			if !strings.HasPrefix(line, "#EXTINF:") {
				// 记录头部标签
				m.Header = append(m.Header, line)
			}
			switch {
			case strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"):
				if v, err := strconv.ParseInt(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"), 10, 64); err == nil {
					m.MediaSequence = v
					seq = v
				}
			case strings.HasPrefix(line, "#EXT-X-TARGETDURATION:"):
				if v, err := strconv.ParseFloat(strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:"), 64); err == nil {
					m.TargetDuration = v
				}
			case strings.HasPrefix(line, "#EXTINF:"):
				// 形如 #EXTINF:10.0,  后面可能带标题
				val := strings.TrimPrefix(line, "#EXTINF:")
				if idx := strings.Index(val, ","); idx >= 0 {
					val = val[:idx]
				}
				if v, err := strconv.ParseFloat(strings.TrimSpace(val), 64); err == nil {
					pendingDuration = v
				}
			case strings.HasPrefix(line, "#EXT-X-KEY:"):
				attrs := parseKeyAttrs(strings.TrimPrefix(line, "#EXT-X-KEY:"))
				pendingKeyURI = attrs["URI"]
				pendingIV = attrs["IV"]
				havePendingIV = attrs["IV"] != ""
				// METHOD=NONE 表示该切片不加密
				if strings.EqualFold(attrs["METHOD"], "NONE") {
					pendingKeyURI = ""
					pendingIV = ""
					havePendingIV = false
				}
			case strings.HasPrefix(line, "#EXT-X-ENDLIST"):
				m.HasEndList = true
			}
			continue
		}

		// 非注释行 = 切片 URI
		m.Segments = append(m.Segments, SegmentInfo{
			URI:      line,
			Duration: pendingDuration,
			Sequence: seq,
			KeyURI:   pendingKeyURI,
			IV:       pendingIV,
		})
		_ = havePendingIV
		seq++
		pendingDuration = 0
		started = true
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("解析 m3u8 失败: %w", err)
	}
	if !started && len(m.Segments) == 0 {
		return nil, fmt.Errorf("m3u8 中没有找到任何切片")
	}
	return m, nil
}

// parseKeyAttrs 解析 #EXT-X-KEY 的属性列表。
// 形如 METHOD=AES-128,URI="http://x/key",IV=0x1234
func parseKeyAttrs(s string) map[string]string {
	out := map[string]string{}
	for _, part := range splitAttrs(s) {
		idx := strings.Index(part, "=")
		if idx < 0 {
			continue
		}
		k := strings.ToUpper(strings.TrimSpace(part[:idx]))
		v := strings.TrimSpace(part[idx+1:])
		v = strings.Trim(v, `"`)
		out[k] = v
	}
	return out
}

// splitAttrs 按逗号切分属性，但跳过引号内的逗号。
func splitAttrs(s string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	for _, c := range s {
		switch {
		case c == '"':
			inQuote = !inQuote
			cur.WriteRune(c)
		case c == ',' && !inQuote:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(c)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

// buildEncryptedM3U8 根据切片加密结果重建加密后的 m3u8 内容。
//
// 关键点：
//   - 移除原始的 #EXT-X-KEY 声明（源流通常未加密，无需保留）；
//   - 插入新的 #EXT-X-KEY:METHOD=AES-128,URI="<密钥地址>",IV=0x<iv>；
//   - 保持其它标签原样。
// segPrefix 为切片绝对路径前缀（如 /hls-enc/<app>/<stream>/），
// 原始相对切片 URI 会被重写为该前缀下的绝对路径，避免客户端按 m3u8 相对路径解析时丢失 stream 段。
func buildEncryptedM3U8(orig *M3U8, keyURL string, ivs map[int64]string, segPrefix string) string {
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	if orig.MediaSequence > 0 {
		b.WriteString("#EXT-X-MEDIA-SEQUENCE:" + strconv.FormatInt(orig.MediaSequence, 10) + "\n")
	}
	if orig.TargetDuration > 0 {
		b.WriteString("#EXT-X-TARGETDURATION:" + strconv.FormatInt(int64(orig.TargetDuration+0.999), 10) + "\n")
	}

	// 写入除 EXT-X-KEY 之外的头部标签
	for _, h := range orig.Header {
		if strings.HasPrefix(h, "#EXT-X-KEY:") {
			continue
		}
		if h == "#EXTM3U" {
			continue
		}
		if strings.HasPrefix(h, "#EXT-X-MEDIA-SEQUENCE:") || strings.HasPrefix(h, "#EXT-X-TARGETDURATION:") {
			continue
		}
		if h == "#EXT-X-ENDLIST" {
			continue
		}
		b.WriteString(h + "\n")
	}

	// 逐个切片输出，并在其前插入 EXT-X-KEY
	for _, seg := range orig.Segments {
		iv := ivs[seg.Sequence]
		b.WriteString(fmt.Sprintf(`#EXT-X-KEY:METHOD=AES-128,URI="%s",IV=0x%s`+"\n", keyURL, iv))
		if seg.Duration > 0 {
			b.WriteString("#EXTINF:" + strconv.FormatFloat(seg.Duration, 'f', 3, 64) + ",\n")
		}
		b.WriteString(segPrefix + seg.URI + "\n")
	}
	return b.String()
}

// Upstream 是回源客户端。
type Upstream struct {
	client *http.Client
	base   string
}

// NewUpstream 创建回源客户端。base 为 ZLMediaKit 的 HTTP 根地址。
func NewUpstream(base string) *Upstream {
	return &Upstream{
		client: &http.Client{Timeout: 15 * time.Second},
		base:   strings.TrimRight(base, "/"),
	}
}

// Fetch 拉取一个资源并返回其内容。
func (u *Upstream) Fetch(ctx context.Context, path string) ([]byte, string, error) {
	full := u.base + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, "", fmt.Errorf("构造回源请求失败: %w", err)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("回源拉取失败 %s: %w", full, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("回源返回状态码 %d: %s", resp.StatusCode, full)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("读取回源内容失败: %w", err)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// CacheEntry 缓存条目。
type CacheEntry struct {
	Data      []byte
	ExpiresAt time.Time
	// ContentType 原始内容类型
	ContentType string
}

// Cache 是一个带 TTL 的内存缓存，用于保存加密后的切片。
type Cache struct {
	mu    sync.RWMutex
	items map[string]*CacheEntry
	ttl   time.Duration
}

// NewCache 创建缓存。
func NewCache(ttl time.Duration) *Cache {
	return &Cache{items: make(map[string]*CacheEntry), ttl: ttl}
}

// Get 读取缓存。
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(e.ExpiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return nil, false
	}
	return e.Data, true
}

// Set 写入缓存。
func (c *Cache) Set(key string, data []byte, contentType string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = &CacheEntry{Data: data, ExpiresAt: time.Now().Add(c.ttl), ContentType: contentType}
}

// Cleanup 清理过期条目。
func (c *Cache) Cleanup() {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range c.items {
		if now.After(v.ExpiresAt) {
			delete(c.items, k)
		}
	}
}

// WriteKeyFile 将密钥写入磁盘，供密钥分发接口读取。
func WriteKeyFile(dir, name string, key []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建密钥目录失败: %w", err)
	}
	path := filepath.Join(dir, name)
	// 密钥文件权限设为 0640，避免被任意用户读取
	if err := os.WriteFile(path, key, 0o640); err != nil {
		return fmt.Errorf("写入密钥文件失败: %w", err)
	}
	return nil
}

// ReadKeyFile 读取密钥文件。
func ReadKeyFile(dir, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(dir, name))
}

// KeyFileName 由流标识生成密钥文件名。
func KeyFileName(app, stream string) string {
	safe := strings.NewReplacer("/", "_", "\\", "_", "..", "_", " ", "_").Replace(app + "_" + stream)
	return safe + ".key"
}

// HexToKey 将十六进制字符串转换为 16 字节密钥。
func HexToKey(hexKey string) ([]byte, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(hexKey))
	if err != nil {
		return nil, fmt.Errorf("解析密钥失败: %w", err)
	}
	if len(raw) != 16 {
		return nil, fmt.Errorf("AES-128 密钥必须为 16 字节，当前为 %d 字节", len(raw))
	}
	return raw, nil
}

// encryptSegment 对单个切片执行加密（供测试与复用）。
func encryptSegment(e *Encryptor, data []byte, iv []byte) ([]byte, error) {
	return e.Encrypt(data, iv)
}
