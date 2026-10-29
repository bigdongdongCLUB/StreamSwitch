// Package util 提供通用工具函数。
package util

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Now 返回当前 Unix 时间戳（秒）。
func Now() int64 { return time.Now().Unix() }

// NowMilli 返回当前 Unix 毫秒时间戳。
func NowMilli() int64 { return time.Now().UnixMilli() }

// FormatTime 将 Unix 时间戳格式化为可读时间。
func FormatTime(ts int64) string {
	if ts <= 0 {
		return "-"
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
}

// FormatDuration 将秒数格式化为中文可读时长。
func FormatDuration(sec int64) string {
	if sec <= 0 {
		return "00:00:00"
	}
	d := time.Duration(sec) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// Contains 判断字符串切片中是否包含目标值。
func Contains(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

// Unique 去重并保持原有顺序。
func Unique(list []string) []string {
	seen := make(map[string]struct{}, len(list))
	out := make([]string, 0, len(list))
	for _, v := range list {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// IsValidIP 判断字符串是否为合法的 IPv4/IPv6 地址。
func IsValidIP(s string) bool { return net.ParseIP(s) != nil }

// IsPrivateIP 判断 IP 是否属于内网/回环地址。
func IsPrivateIP(s string) bool {
	ip := net.ParseIP(s)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

// IPInList 判断 IP 是否命中列表（列表中可包含 CIDR 形式，如 192.168.1.0/24）。
func IPInList(ip string, list string) bool {
	list = strings.TrimSpace(list)
	if list == "" {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, item := range strings.Split(list, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if strings.Contains(item, "/") {
			_, ipNet, err := net.ParseCIDR(item)
			if err != nil {
				continue
			}
			if ipNet.Contains(parsed) {
				return true
			}
			continue
		}
		if net.ParseIP(item) != nil && net.ParseIP(item).Equal(parsed) {
			return true
		}
	}
	return false
}

// BuildURL 拼接 URL，自动处理斜杠。
func BuildURL(base, path string) string {
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

// TruncateString 按字节截断字符串（超长时追加省略号）。
func TruncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// ParseInt 解析整数，失败返回默认值。
func ParseInt(s string, def int64) int64 {
	if s == "" {
		return def
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return n
}

// SafeURL 对 URL 做基本合法性校验（允许 rtmp/rtmps/http/https/ws/wss 等）。
func SafeURL(raw string) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "rtmp", "rtmps", "rtsp", "rtsps", "http", "https", "ws", "wss", "srt", "udp", "hls":
		return u.Host != ""
	default:
		return false
	}
}
