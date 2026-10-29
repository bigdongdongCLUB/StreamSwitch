// Package jwt 实现基于 HMAC-SHA256 的后台登录令牌签发与校验。
//
// 使用标准库 crypto/hmac + encoding/base64 + encoding/json 自实现，
// 避免引入额外依赖，同时保证令牌格式自解释、可跨进程校验。
package jwt

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Claims 令牌载荷。
type Claims struct {
	UserID   uint64 `json:"uid"`             // 用户ID
	Username string `json:"uname"`           // 用户名
	Super    bool   `json:"super,omitempty"` // 是否超管
	IssuedAt int64  `json:"iat"`             // 签发时间(秒)
	Expires  int64  `json:"exp"`             // 过期时间(秒)
	Issuer   string `json:"iss,omitempty"`   // 签发者
}

// Manager 令牌管理器。
type Manager struct {
	secret     []byte
	issuer     string
	expireHour int
}

// New 创建令牌管理器。
func New(secret, issuer string, expireHour int) *Manager {
	if expireHour <= 0 {
		expireHour = 24
	}
	return &Manager{secret: []byte(secret), issuer: issuer, expireHour: expireHour}
}

// Generate 签发令牌，返回令牌串与过期时间戳。
func (m *Manager) Generate(userID uint64, username string, super bool) (string, int64) {
	now := time.Now()
	exp := now.Add(time.Duration(m.expireHour) * time.Hour)
	claims := Claims{
		UserID:   userID,
		Username: username,
		Super:    super,
		IssuedAt: now.Unix(),
		Expires:  exp.Unix(),
		Issuer:   m.issuer,
	}
	return m.encode(claims), exp.Unix()
}

// encode 将载荷序列化为 header.payload.signature 三段式令牌。
func (m *Manager) encode(c Claims) string {
	header := base64URL([]byte(`{"alg":"HS256","typ":"JWT"}`))
	body, _ := json.Marshal(c)
	payload := base64URL(body)
	sig := base64URL(m.sign(header + "." + payload))
	return header + "." + payload + "." + sig
}

// sign 计算 HMAC-SHA256 签名。
func (m *Manager) sign(data string) []byte {
	h := hmac.New(sha256.New, m.secret)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// Parse 解析并校验令牌，失败时返回错误。
func (m *Manager) Parse(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("令牌格式错误")
	}
	// 校验签名，防止篡改
	expected := m.sign(parts[0] + "." + parts[1])
	actual, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("令牌签名解析失败")
	}
	if !hmac.Equal(expected, actual) {
		return nil, fmt.Errorf("令牌签名无效")
	}
	// 解析载荷
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("令牌载荷解析失败")
	}
	var c Claims
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("令牌载荷格式错误")
	}
	if c.Expires > 0 && time.Now().Unix() > c.Expires {
		return nil, fmt.Errorf("令牌已过期")
	}
	return &c, nil
}

// base64URL 编码为无填充的 URL 安全 base64。
func base64URL(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// RandomHex 生成指定字节数的随机十六进制字符串。
func RandomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		// 极端情况下使用时间戳兜底（rand.Read 极少失败）
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return hex.EncodeToString(buf)
}

// RandomToken 生成指定字节数的 URL 安全随机字符串，用于播放 Token。
func RandomToken(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
