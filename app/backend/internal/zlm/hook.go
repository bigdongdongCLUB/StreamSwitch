package zlm

import (
	"net/url"
	"strings"
)

// HookEvent 是 ZLMediaKit 回调事件的公共字段。
//
// ZLMediaKit 会向 hook.on_publish / hook.on_play 等地址 POST 一个 JSON，
// 公共字段见：https://docs.zlmediakit.com/guide/media_server/web_hook_api.html
type HookEvent struct {
	MediaServerID string `json:"mediaServerId"` // 媒体服务器ID
	App           string `json:"app"`           // 流应用名
	Stream        string `json:"stream"`        // 流ID
	Vhost         string `json:"vhost"`         // 虚拟主机
	Schema        string `json:"schema"`        // 媒体源类型 rtmp/rtsp/ts/fmp4/hls
	Protocol      string `json:"protocol"`      // 传输协议
	Params        string `json:"params"`        // URL 参数（形如 a=1&token=xxx）
	IP            string `json:"ip"`            // 对端IP
	Port          int    `json:"port"`          // 对端端口
	ID            string `json:"id"`            // TCP连接唯一ID
	OriginType    int    `json:"originType"`    // 来源类型
	OriginTypeStr string `json:"originTypeStr"` // 来源类型描述
	HookIndex     uint64 `json:"hook_index"`    // 回调序号
}

// PublishEvent on_publish 推流事件。
type PublishEvent struct {
	HookEvent
}

// PlayEvent on_play 播放事件。
type PlayEvent struct {
	HookEvent
}

// StreamChangedEvent on_stream_changed 流上线/下线事件。
type StreamChangedEvent struct {
	HookEvent
	Regist bool `json:"regist"` // true=上线 false=下线
}

// FlowReportEvent on_flow_report 流量统计事件。
type FlowReportEvent struct {
	HookEvent
	TotalBytes  int64 `json:"totalBytes"`
	TotalDuration int64 `json:"duration"`
	Player      bool  `json:"player"` // true=播放器 false=推流器
}

// StreamNotFoundEvent on_stream_not_found 流不存在事件。
type StreamNotFoundEvent struct {
	HookEvent
}

// HookReply 是回调的通用回复结构。
//
// code=0 表示允许；非 0 表示拒绝。
// on_publish 还支持通过扩展字段控制转协议开关（如 enable_hls / enable_rtmp）。
type HookReply struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`

	// 以下字段仅 on_publish 使用，用于控制 ZLM 的协议转换行为
	EnableHLS     *bool  `json:"enable_hls,omitempty"`
	EnableHLSFmp4 *bool  `json:"enable_hls_fmp4,omitempty"`
	EnableMP4     *bool  `json:"enable_mp4,omitempty"`
	EnableRTSP    *bool  `json:"enable_rtsp,omitempty"`
	EnableRTMP    *bool  `json:"enable_rtmp,omitempty"`
	EnableTS      *bool  `json:"enable_ts,omitempty"`
	EnableFMP4    *bool  `json:"enable_fmp4,omitempty"`
	HLSDemand     *bool  `json:"hls_demand,omitempty"`
	RTSPDemand    *bool  `json:"rtsp_demand,omitempty"`
	RTMPDemand    *bool  `json:"rtmp_demand,omitempty"`
	TSDemand      *bool  `json:"ts_demand,omitempty"`
	FMP4Demand    *bool  `json:"fmp4_demand,omitempty"`

	// CloseStream 为 true 时，ZLM 会在推流建立后主动断开该推流连接
	CloseStream bool `json:"close_stream,omitempty"`
}

// Allow 返回一个允许通过的回复。
func Allow() HookReply {
	return HookReply{Code: 0, Msg: "success"}
}

// Deny 返回一个拒绝通过的回复。
func Deny(msg string) HookReply {
	return HookReply{Code: -1, Msg: msg}
}

// ParseParams 将 ZLM 回调中的 params 字段解析为键值对。
//
// ZLM 的 params 形如 "token=abc&uid=1"，不含 "?" 前缀。
func (e HookEvent) ParseParams() map[string]string {
	out := map[string]string{}
	p := e.Params
	if p == "" {
		return out
	}
	// 兼容部分客户端把完整 query 带上的情况
	p = strings.TrimPrefix(p, "?")
	for _, kv := range strings.Split(p, "&") {
		if kv == "" {
			continue
		}
		idx := strings.Index(kv, "=")
		if idx < 0 {
			out[kv] = ""
			continue
		}
		k := kv[:idx]
		v := kv[idx+1:]
		// 处理 URL 编码
		if dk, err := url.QueryUnescape(k); err == nil {
			k = dk
		}
		if dv, err := url.QueryUnescape(v); err == nil {
			v = dv
		}
		out[k] = v
	}
	return out
}
