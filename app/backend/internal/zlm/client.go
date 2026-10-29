// Package zlm 封装对 ZLMediaKit REST API 的调用。
//
// 约定：
//   - 所有请求都携带 secret 参数（ZLMediaKit 的 api.secret）
//   - 统一解析 ZLM 返回的 {code, msg, data} 结构
//   - ZLM 的 code=0 表示成功，其余为错误
package zlm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"livems/internal/config"
	"livems/internal/pkg/logger"
)

// Client ZLMediaKit API 客户端。
type Client struct {
	baseURL string
	secret  string
	http    *http.Client
	app     string
	vhost   string
}

// New 创建 ZLMediaKit 客户端。
func New(cfg config.ZLMConfig) *Client {
	return &Client{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		secret:  cfg.Secret,
		app:     cfg.App,
		vhost:   cfg.Vhost,
		http: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
	}
}

// response ZLM 通用响应结构。
type response struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// APIError 表示 ZLM 接口返回的业务错误。
type APIError struct {
	Code int
	Msg  string
	Path string
}

// Error 实现 error 接口。
func (e *APIError) Error() string {
	return fmt.Sprintf("ZLMediaKit 接口错误 [%s] code=%d msg=%s", e.Path, e.Code, e.Msg)
}

// do 发起一次 GET 请求并解析统一响应结构。
func (c *Client) do(ctx context.Context, path string, params url.Values, out any) error {
	full := c.baseURL + path
	if len(params) == 0 {
		params = url.Values{}
	}
	// 所有接口都带上 secret
	params.Set("secret", c.secret)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full+"?"+params.Encode(), nil)
	if err != nil {
		return fmt.Errorf("构造 ZLM 请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "livems/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("调用 ZLM 接口 %s 失败: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取 ZLM 响应失败: %w", err)
	}

	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("解析 ZLM 响应失败 %s: %w (body=%s)", path, err, string(body))
	}
	// ZLM 约定：code=0 成功
	if r.Code != 0 {
		return &APIError{Code: r.Code, Msg: r.Msg, Path: path}
	}
	if out != nil && len(r.Data) > 0 && string(r.Data) != "null" {
		if err := json.Unmarshal(r.Data, out); err != nil {
			return fmt.Errorf("解析 ZLM data 失败 %s: %w", path, err)
		}
	}
	return nil
}

// MediaTuple 流的三元组标识。
type MediaTuple struct {
	Schema string `json:"schema"`
	Vhost  string `json:"vhost"`
	App    string `json:"app"`
	Stream string `json:"stream"`
}

// Track 流的轨道（音视频）信息。
type Track struct {
	CodecID     int     `json:"codec_id"`
	CodecIDName string  `json:"codec_id_name"`
	CodecType   int     `json:"codec_type"` // 0=video 1=audio
	Ready       bool    `json:"ready"`
	Frames      int64   `json:"frames"`
	Duration    float64 `json:"duration"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	FPS         float64 `json:"fps"` // ZLM 返回浮点帧率（如 25.0），入库时取整
	GopSize     int     `json:"gop_size"`
	SampleRate  int     `json:"sample_rate"`
	Channels    int     `json:"channels"`
	SampleBit   int     `json:"sample_bit"`
}

// MediaInfo 一条流在 ZLMediaKit 中的实时信息。
type MediaInfo struct {
	MediaTuple
	CreateStamp     int64   `json:"createStamp"`
	AliveSecond     int64   `json:"aliveSecond"`
	BytesSpeed      int64   `json:"bytesSpeed"`
	TotalBytes      int64   `json:"totalBytes"`
	ReaderCount     int     `json:"readerCount"`
	TotalReaderCount int    `json:"totalReaderCount"`
	OriginType      int     `json:"originType"`
	OriginTypeStr   string  `json:"originTypeStr"`
	OriginURL       string  `json:"originUrl"`
	IsRecordingMP4  bool    `json:"isRecordingMP4"`
	IsRecordingHLS  bool    `json:"isRecordingHLS"`
	Tracks          []Track `json:"tracks"`
}

// IsVideo 判断是否为视频轨道。
func (t Track) IsVideo() bool { return t.CodecType == 0 }

// IsAudio 判断是否为音频轨道。
func (t Track) IsAudio() bool { return t.CodecType == 1 }

// GetMediaList 获取当前 ZLM 中所有（或筛选后的）流列表。
// schema/vhost/app/stream 均为可选筛选条件。
func (c *Client) GetMediaList(ctx context.Context, schema, vhost, app, stream string) ([]MediaInfo, error) {
	params := url.Values{}
	if schema != "" {
		params.Set("schema", schema)
	}
	if vhost != "" {
		params.Set("vhost", vhost)
	}
	if app != "" {
		params.Set("app", app)
	}
	if stream != "" {
		params.Set("stream", stream)
	}
	var list []MediaInfo
	if err := c.do(ctx, "/index/api/getMediaList", params, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// IsMediaOnline 判断指定流是否在线。
func (c *Client) IsMediaOnline(ctx context.Context, schema, vhost, app, stream string) (bool, error) {
	params := url.Values{
		"schema": {schema},
		"vhost":  {vhost},
		"app":    {app},
		"stream": {stream},
	}
	var out struct {
		Online bool `json:"online"`
	}
	if err := c.do(ctx, "/index/api/isMediaOnline", params, &out); err != nil {
		return false, err
	}
	return out.Online, nil
}

// CloseStream 主动关闭一条流（踢掉推流端）。
//
// 使用 close_streams（按条件批量关闭）而不是 close_stream：后者要求 schema 必填，
// 而业务上禁用/删除一条流需要把它在 rtmp/hls/ts/fmp4 等所有协议下的实例一并踢掉。
// schema 为空时不传该参数，表示匹配全部协议。
func (c *Client) CloseStream(ctx context.Context, schema, vhost, app, stream string, force bool) error {
	params := url.Values{
		"vhost":  {vhost},
		"app":    {app},
		"stream": {stream},
		"force":  {strconv.FormatBool(force)},
	}
	if schema != "" {
		params.Set("schema", schema)
	}
	return c.do(ctx, "/index/api/close_streams", params, nil)
}

// ProxyAddReq 新增拉流代理的请求参数。
type ProxyAddReq struct {
	Vhost          string
	App            string
	Stream         string
	URL            string // 远端源地址
	RetryCount     int    // 失败重试次数，-1 表示无限重试
	TimeoutSec     int
	EnableRTSP     bool
	EnableRTMP     bool
	EnableTS       bool
	EnableFMP4     bool
	EnableHLS      bool
}

// AddStreamProxy 创建拉流代理任务（从远端源拉流并推到本地）。
// 返回 ZLM 生成的任务 key。
func (c *Client) AddStreamProxy(ctx context.Context, req ProxyAddReq) (string, error) {
	params := url.Values{
		"vhost":         {req.Vhost},
		"app":           {req.App},
		"stream":        {req.Stream},
		"url":           {req.URL},
		"enable_rtsp":   {boolToInt(req.EnableRTSP)},
		"enable_rtmp":   {boolToInt(req.EnableRTMP)},
		"enable_ts":     {boolToInt(req.EnableTS)},
		"enable_fmp4":   {boolToInt(req.EnableFMP4)},
		"enable_hls":    {boolToInt(req.EnableHLS)},
		"retry_count":   {strconv.Itoa(req.RetryCount)},
		"timeout_sec":   {strconv.Itoa(req.TimeoutSec)},
	}
	var out struct {
		Key string `json:"key"`
	}
	if err := c.do(ctx, "/index/api/addStreamProxy", params, &out); err != nil {
		return "", err
	}
	return out.Key, nil
}

// DelStreamProxy 关闭拉流代理任务。
func (c *Client) DelStreamProxy(ctx context.Context, key string) error {
	params := url.Values{"key": {key}}
	return c.do(ctx, "/index/api/delStreamProxy", params, nil)
}

// ListStreamProxy 列出 ZLM 中当前的拉流代理任务。
func (c *Client) ListStreamProxy(ctx context.Context, key string) ([]map[string]any, error) {
	params := url.Values{}
	if key != "" {
		params.Set("key", key)
	}
	var list []map[string]any
	if err := c.do(ctx, "/index/api/listStreamProxy", params, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// ForwardAddReq 新增转推流任务的请求参数。
type ForwardAddReq struct {
	Schema      string // rtmp / rtmps
	Vhost       string
	App         string
	Stream      string
	DstURL      string // 目标 RTMP 地址
	RetryCount  int
	TimeoutSec  int
}

// AddStreamPusherProxy 创建转推流任务（把本地流推到远端 RTMP）。
func (c *Client) AddStreamPusherProxy(ctx context.Context, req ForwardAddReq) (string, error) {
	params := url.Values{
		"schema":       {req.Schema},
		"vhost":        {req.Vhost},
		"app":          {req.App},
		"stream":       {req.Stream},
		"dst_url":      {req.DstURL},
		"retry_count":  {strconv.Itoa(req.RetryCount)},
		"timeout_sec":  {strconv.Itoa(req.TimeoutSec)},
	}
	var out struct {
		Key string `json:"key"`
	}
	if err := c.do(ctx, "/index/api/addStreamPusherProxy", params, &out); err != nil {
		return "", err
	}
	return out.Key, nil
}

// DelStreamPusherProxy 关闭转推流任务。
func (c *Client) DelStreamPusherProxy(ctx context.Context, key string) error {
	params := url.Values{"key": {key}}
	return c.do(ctx, "/index/api/delStreamPusherProxy", params, nil)
}

// ListStreamPusherProxy 列出 ZLM 中当前的转推流任务。
func (c *Client) ListStreamPusherProxy(ctx context.Context, key string) ([]map[string]any, error) {
	params := url.Values{}
	if key != "" {
		params.Set("key", key)
	}
	var list []map[string]any
	if err := c.do(ctx, "/index/api/listStreamPusherProxy", params, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// GetServerConfig 获取 ZLM 的完整配置（用于同步参数与排查问题）。
func (c *Client) GetServerConfig(ctx context.Context) (map[string]string, error) {
	out := map[string]string{}
	if err := c.do(ctx, "/index/api/getServerConfig", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetServerInfo 获取 ZLM 的版本与运行信息。
func (c *Client) GetServerInfo(ctx context.Context) (map[string]any, error) {
	out := map[string]any{}
	if err := c.do(ctx, "/index/api/getServerInfo", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Health 探测 ZLM 是否存活。
func (c *Client) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return c.do(ctx, "/index/api/getServerConfig", nil, nil)
}

// boolToInt 将布尔转为 ZLM 使用的 0/1 字符串。
func boolToInt(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// SyncStreams 将 ZLM 的实际流状态同步到数据库（返回统计信息）。
// 用于应用重启后恢复流状态，或定时纠正漂移。
func (c *Client) SyncStreams(ctx context.Context) (total, online int, err error) {
	list, err := c.GetMediaList(ctx, "", "", c.app, "")
	if err != nil {
		return 0, 0, err
	}
	for i := range list {
		total++
		if list[i].AliveSecond > 0 {
			online++
		}
	}
	logger.Debug("ZLMediaKit 同步流列表: 总数=%d 在线=%d", total, online)
	return total, online, nil
}
