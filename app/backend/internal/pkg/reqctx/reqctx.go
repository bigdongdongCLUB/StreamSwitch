// Package reqctx 在 context 中携带当前请求的访问来源（协议/主机/端口）。
//
// 用途：后台未配置任何播放域名/推流主机时，用浏览器当前访问的地址生成播放与推流地址，
// 使全新部署无需先配置域名即可使用。
package reqctx

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
)

// Origin 请求来源。
type Origin struct {
	Scheme string // http / https
	Host   string // 不含端口
	Port   int    // 显式端口；未知时为协议默认端口
}

type ctxKey struct{}

// With 把来源写入 context。
func With(ctx context.Context, o Origin) context.Context {
	return context.WithValue(ctx, ctxKey{}, o)
}

// From 读取来源。
func From(ctx context.Context) (Origin, bool) {
	if ctx == nil {
		return Origin{}, false
	}
	o, ok := ctx.Value(ctxKey{}).(Origin)
	return o, ok && o.Host != ""
}

// FromRequest 解析请求来源：优先反代传入的 X-Forwarded-Proto / X-Forwarded-Host（含端口），其次 Host 头。
func FromRequest(r *http.Request) Origin {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if p := strings.ToLower(strings.TrimSpace(firstValue(r.Header.Get("X-Forwarded-Proto")))); p == "http" || p == "https" {
		scheme = p
	}
	hostport := firstValue(r.Header.Get("X-Forwarded-Host"))
	if hostport == "" {
		hostport = r.Host
	}
	o := Origin{Scheme: scheme}
	if h, p, err := net.SplitHostPort(hostport); err == nil {
		o.Host = h
		o.Port, _ = strconv.Atoi(p)
	} else {
		o.Host = strings.Trim(hostport, "[]")
	}
	if o.Port == 0 {
		if fp, err := strconv.Atoi(firstValue(r.Header.Get("X-Forwarded-Port"))); err == nil && fp > 0 {
			o.Port = fp
		}
	}
	if o.Port == 0 {
		o.Port = 80
		if scheme == "https" {
			o.Port = 443
		}
	}
	return o
}

func firstValue(v string) string {
	if i := strings.IndexByte(v, ','); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}
