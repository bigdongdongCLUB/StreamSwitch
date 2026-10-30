package service

import (
	"strings"
	"testing"
)

func TestNormalizeHost(t *testing.T) {
	ok := map[string]string{
		"awskr.jldup.com":     "awskr.jldup.com",
		"  Live.Example.COM ": "live.example.com",
		"live.example.com.":   "live.example.com",
		"localhost":           "localhost",
		"a-b.example.com":     "a-b.example.com",
		"52.79.173.246":       "52.79.173.246",
		"xn--fiq228c.com":     "xn--fiq228c.com",
	}
	for in, want := range ok {
		got, err := normalizeHost("主机", in)
		if err != nil || got != want {
			t.Errorf("normalizeHost(%q) = %q, %v；期望 %q", in, got, err, want)
		}
	}

	bad := map[string]string{
		"awskr,jldup.com":                "逗号",
		"awskr，jldup.com":                "逗号",
		"awskr。jldup.com":                "逗号",
		"rtmp://live.example.com":        "协议",
		"live.example.com/live":          "路径",
		"live.example.com:1935":          "端口",
		"live_push.example.com":          "下划线",
		"live example.com":               "空格",
		"live..example.com":              "连续的点",
		".example.com":                   "连续的点",
		"-live.example.com":              "中划线",
		"live-.example.com":              "中划线",
		"::1":                            "IPv6",
		"999.1.1.1":                      "IPv4",
		"1.2.3":                          "IPv4",
		strings.Repeat("a", 64) + ".com": "63",
	}
	for in, hint := range bad {
		_, err := normalizeHost("主机", in)
		if err == nil {
			t.Errorf("normalizeHost(%q) 应报错", in)
			continue
		}
		if !strings.Contains(err.Error(), hint) {
			t.Errorf("normalizeHost(%q) 报错 %q 未包含提示 %q", in, err, hint)
		}
	}
	if _, err := normalizeHost("主机", "  "); err == nil {
		t.Error("空主机名应报错")
	}
}
