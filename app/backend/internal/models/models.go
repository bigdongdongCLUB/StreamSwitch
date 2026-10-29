// Package models 定义与数据库表一一对应的实体模型。
package models

// Stream 推流管理实体，对应表 stream。
type Stream struct {
	ID            uint64 `json:"id"`
	Name          string `json:"name"`
	App           string `json:"app"`
	Stream        string `json:"stream"`
	Vhost         string `json:"vhost"`
	PushKey       string `json:"push_key"`
	Status        int    `json:"status"`
	Remark        string `json:"remark"`
	Online        int    `json:"online"`
	PublishIP     string `json:"publish_ip"`
	PublishTime   int64  `json:"publish_time"`
	OfflineTime   int64  `json:"offline_time"`
	VideoCodec    string `json:"video_codec"`
	AudioCodec    string `json:"audio_codec"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	FPS           int    `json:"fps"`
	Bitrate       int    `json:"bitrate"`
	VideoBitrate  int    `json:"video_bitrate"`
	AudioBitrate  int    `json:"audio_bitrate"`
	SampleRate    int    `json:"sample_rate"`
	ReaderCount   int    `json:"reader_count"`
	TotalReader   int    `json:"total_reader"`
	TotalBytes    int64  `json:"total_bytes"`
	CreatedBy     uint64 `json:"created_by"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// StreamProxy 拉流代理任务实体，对应表 stream_proxy。
type StreamProxy struct {
	ID             uint64 `json:"id"`
	Name           string `json:"name"`
	SrcURL         string `json:"src_url"`
	App            string `json:"app"`
	Stream         string `json:"stream"`
	Vhost          string `json:"vhost"`
	Status         int    `json:"status"`
	RetryCount     int    `json:"retry_count"`
	RetryInterval  int    `json:"retry_interval"`
	TimeoutSec     int    `json:"timeout_sec"`
	EnableRTSP     int    `json:"enable_rtsp"`
	EnableRTMP     int    `json:"enable_rtmp"`
	EnableTS       int    `json:"enable_ts"`
	EnableFMP4     int    `json:"enable_fmp4"`
	EnableHLS      int    `json:"enable_hls"`
	ZLMKey         string `json:"zlm_key"`
	RunStatus      string `json:"run_status"`
	LastError      string `json:"last_error"`
	RetryTimes     int    `json:"retry_times"`
	LastActiveAt   int64  `json:"last_active_at"`
	CreatedBy      uint64 `json:"created_by"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

// StreamForward 转推流任务实体，对应表 stream_forward。
type StreamForward struct {
	ID            uint64 `json:"id"`
	Name          string `json:"name"`
	SrcApp        string `json:"src_app"`
	SrcStream     string `json:"src_stream"`
	SrcVhost      string `json:"src_vhost"`
	DstURL        string `json:"dst_url"`
	Schema        string `json:"schema"`
	Status        int    `json:"status"`
	RunStatus     string `json:"run_status"`
	RetryCount    int    `json:"retry_count"`
	TimeoutSec    int    `json:"timeout_sec"`
	ZLMKey        string `json:"zlm_key"`
	RetryTimes    int    `json:"retry_times"`
	LastError     string `json:"last_error"`
	LastActiveAt  int64  `json:"last_active_at"`
	CreatedBy     uint64 `json:"created_by"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// PlayDomain 播放域名实体，对应表 play_domain。
type PlayDomain struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	Schema    string `json:"schema"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	IsDefault int    `json:"is_default"`
	Status    int    `json:"status"`
	Sort      int    `json:"sort"`
	Remark    string `json:"remark"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// PlayFormat 播放格式开关实体，对应表 play_format。
type PlayFormat struct {
	ID         uint64 `json:"id"`
	Format     string `json:"format"`
	Name       string `json:"name"`
	Enabled    int    `json:"enabled"`
	PathPrefix string `json:"path_prefix"`
	Sort       int    `json:"sort"`
	Remark     string `json:"remark"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

// HLSEncrypt HLS 加密配置实体，对应表 hls_encrypt。
type HLSEncrypt struct {
	ID           uint64 `json:"id"`
	StreamID     uint64 `json:"stream_id"`
	Enabled      int    `json:"enabled"`
	KeyHex       string `json:"key_hex"`
	KeyURI       string `json:"key_uri"`
	KeyExpireSec int    `json:"key_expire_sec"`
	IVStrategy   string `json:"iv_strategy"`
	ExpireAt     int64  `json:"expire_at"`
	LastRotateAt int64  `json:"last_rotate_at"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
}

// PlayToken 播放令牌实体，对应表 play_token。
type PlayToken struct {
	ID        uint64 `json:"id"`
	Token     string `json:"token"`
	StreamID  uint64 `json:"stream_id"`
	App       string `json:"app"`
	Stream    string `json:"stream"`
	ExpireAt  int64  `json:"expire_at"`
	IPBind    string `json:"ip_bind"`
	Format    string `json:"format"`
	CreatedBy uint64 `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
}

// User 管理员用户实体，对应表 sys_user。
type User struct {
	ID           uint64 `json:"id"`
	Username     string `json:"username"`
	Password     string `json:"-"`
	Nickname     string `json:"nickname"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Avatar       string `json:"avatar"`
	Status       int    `json:"status"`
	IsSuper      int    `json:"is_super"`
	LastLoginAt  int64  `json:"last_login_at"`
	LastLoginIP  string `json:"last_login_ip"`
	Remark       string `json:"remark"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
	Roles        []Role `json:"roles,omitempty"`
	Permissions  []string `json:"permissions,omitempty"`
}

// Role 角色实体，对应表 sys_role。
type Role struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	Remark    string `json:"remark"`
	Status    int    `json:"status"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// Permission 权限实体，对应表 sys_permission。
type Permission struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	Code      string `json:"code"`
	Type      int    `json:"type"`
	ParentID  uint64 `json:"parent_id"`
	Sort      int    `json:"sort"`
	Remark    string `json:"remark"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// SysLog 操作日志实体，对应表 sys_log。
type SysLog struct {
	ID        uint64 `json:"id"`
	UserID    uint64 `json:"user_id"`
	Username  string `json:"username"`
	Module    string `json:"module"`
	Action    string `json:"action"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	IP        string `json:"ip"`
	Params    string `json:"params"`
	Result    string `json:"result"`
	Status    int    `json:"status"`
	ErrorMsg  string `json:"error_msg"`
	Duration  int    `json:"duration"`
	CreatedAt int64  `json:"created_at"`
}

// TaskErrorLog 任务错误日志实体，对应表 task_error_log。
type TaskErrorLog struct {
	ID        uint64 `json:"id"`
	TaskType  string `json:"task_type"`
	TaskID    uint64 `json:"task_id"`
	TaskName  string `json:"task_name"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	CreatedAt int64  `json:"created_at"`
}

// SysConfig 全局配置实体，对应表 sys_config。
type SysConfig struct {
	ID        uint64 `json:"id"`
	ConfigKey string `json:"config_key"`
	ConfigVal string `json:"config_val"`
	Remark    string `json:"remark"`
	UpdatedAt int64  `json:"updated_at"`
}

// LivePage 直播分享页实体，对应表 live_page。
// 观众免登录通过 /p/<code> 访问观看直播，可选访问密码。
type LivePage struct {
	ID           uint64 `json:"id"`
	Name         string `json:"name"`
	Code         string `json:"code"`          // 访问码（URL 标识）
	StreamID     uint64 `json:"stream_id"`     // 关联流
	Password     string `json:"password"`      // 访问密码（明文存储，管理端可见，便于告知观众；观众侧接口不下发）
	Announcement string `json:"announcement"`  // 页面公告
	Status       int    `json:"status"`
	ViewCount    int64  `json:"view_count"`
	CreatedBy    uint64 `json:"created_by"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`

	// HasPasswordView 管理端列表展示的密码状态（非数据库字段，查询后填充）
	HasPasswordView bool `json:"has_password" gorm:"-"`
}

// HasPassword 是否设置了访问密码。
func (p *LivePage) HasPassword() bool { return p.Password != "" }

// StudioMaterial 制播素材库实体，对应表 studio_material。
type StudioMaterial struct {
	ID        uint64 `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"` // video=视频素材 logo=Logo图片
	FilePath  string `json:"file_path"`
	Size      int64  `json:"size"`
	Duration  int    `json:"duration"` // 视频时长（秒）
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	CreatedBy uint64 `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
}

// StudioTask 制播任务实体，对应表 studio_task。
type StudioTask struct {
	ID              uint64 `json:"id"`
	Name            string `json:"name"`
	Mode            string `json:"mode"` // loop=视频轮播 relay=拉流压制
	MaterialID      uint64 `json:"material_id"`
	SrcURL          string `json:"src_url"`
	App             string `json:"app"`
	Stream          string `json:"stream"`
	CaptionEnabled  int    `json:"caption_enabled"`
	CaptionText     string `json:"caption_text"`
	CaptionSize     int    `json:"caption_size"`
	CaptionColor    string `json:"caption_color"`
	CaptionPosition string `json:"caption_position"`
	CaptionSpeed    int    `json:"caption_speed"`
	LogoMaterialID  uint64 `json:"logo_material_id"`
	LogoPosition    string `json:"logo_position"` // tl/tr/bl/br 四角
	LogoOpacity     int    `json:"logo_opacity"`
	LogoWidthPct    int    `json:"logo_width_pct"`
	VBitrate        int    `json:"vbitrate"`
	QualityMode     string `json:"quality_mode"` // bitrate=固定码率 crf=画质优先
	CRF             int    `json:"crf"`          // CRF 质量因子(0-51，越小越清晰)
	Status          int    `json:"status"`
	RunStatus       string `json:"run_status"`
	PID             int    `json:"pid"`
	LastError       string `json:"last_error"`
	RetryCount      int    `json:"retry_count"`
	RetryTimes      int    `json:"retry_times"`
	LastActiveAt    int64  `json:"last_active_at"`
	CreatedBy       uint64 `json:"created_by"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}
