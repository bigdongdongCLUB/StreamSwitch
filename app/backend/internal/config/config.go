// Package config 负责加载并解析应用配置。
//
// 配置来源优先级（后者覆盖前者）：
//  1. 内置默认值
//  2. YAML 配置文件（默认 configs/config.yaml，可用 -c 指定）
//  3. 环境变量（前缀 APP_，如 APP_SERVER_PORT 会覆盖 server.port）
package config

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config 是应用的完整配置树。
type Config struct {
	Server   ServerConfig   `yaml:"server"`
	MySQL    MySQLConfig    `yaml:"mysql"`
	Redis    RedisConfig    `yaml:"redis"`
	JWT      JWTConfig      `yaml:"jwt"`
	ZLM      ZLMConfig      `yaml:"zlm"`
	Log      LogConfig      `yaml:"log"`
	HLS      HLSConfig      `yaml:"hls"`
	Auth     AuthConfig     `yaml:"auth"`
	Frontend FrontendConfig `yaml:"frontend"`
	Nginx    NginxConfig    `yaml:"nginx"`
	Studio   StudioConfig   `yaml:"studio"`
}

// StudioConfig 制播管理配置。
type StudioConfig struct {
	// FFmpegPath 完整功能版 ffmpeg 路径（需含 drawtext/overlay 滤镜）
	FFmpegPath string `yaml:"ffmpeg_path"`
	// FFprobePath ffprobe 路径
	FFprobePath string `yaml:"ffprobe_path"`
	// FontFile 字幕使用的字体文件（中文渲染需要）
	FontFile string `yaml:"font_file"`
	// DataDir 制播数据目录（素材/Logo 存放）
	DataDir string `yaml:"data_dir"`
	// PushBase 压制产物回推的 ZLM RTMP 地址（本机回环）
	PushBase string `yaml:"push_base"`
}

// NginxConfig 证书管理相关的 Nginx 部署配置。
type NginxConfig struct {
	// VhostConf 本站点的 Nginx 配置文件路径（宝塔 vhost 目录下）
	VhostConf string `yaml:"vhost_conf"`
	// CertDir 证书存放根目录（宝塔约定 /www/server/panel/vhost/cert）
	CertDir string `yaml:"cert_dir"`
	// ReloadCmd 证书变更后执行的 Nginx 重载命令
	ReloadCmd string `yaml:"reload_cmd"`
	// StreamConf RTMPS 的 Nginx stream 配置文件路径；应用证书时与 VhostConf 同步替换（为空表示未开启 RTMPS）
	StreamConf string `yaml:"stream_conf"`
}

// ServerConfig 后端 HTTP 服务监听配置。
type ServerConfig struct {
	Host            string   `yaml:"host"`             // 监听地址，默认 0.0.0.0
	Port            int      `yaml:"port"`             // 监听端口，默认 8090
	TrustedProxies  []string `yaml:"trusted_proxies"`  // 可信代理IP（取真实客户端IP用）
	Mode            string   `yaml:"mode"`             // gin 运行模式 debug/release
	StaticDir       string   `yaml:"static_dir"`       // 前端静态文件目录
	EnableSwagger   bool     `yaml:"enable_swagger"`   // 是否暴露调试接口
	ReadTimeout     int      `yaml:"read_timeout"`     // 读超时(秒)
	WriteTimeout    int      `yaml:"write_timeout"`    // 写超时(秒)
	ShutdownTimeout int      `yaml:"shutdown_timeout"` // 优雅退出等待(秒)
	PprofAddr       string   `yaml:"pprof_addr"`       // Go pprof 诊断监听地址（如 127.0.0.1:6060），留空不开启
}

// MySQLConfig 业务数据库连接配置。
type MySQLConfig struct {
	Host            string `yaml:"host"`
	Port            int    `yaml:"port"`
	User            string `yaml:"user"`
	Password        string `yaml:"password"`
	Database        string `yaml:"database"`
	Charset         string `yaml:"charset"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime"` // 连接最大存活时间(秒)
	ParseTime       bool   `yaml:"parse_time"`
}

// DSN 拼接 go-sql-driver/mysql 的连接串。
func (m MySQLConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=%t&loc=Local&timeout=10s&readTimeout=30s&writeTimeout=30s",
		m.User, m.Password, m.Host, m.Port, m.Database, m.Charset, m.ParseTime)
}

// RedisConfig 缓存/Token 存储配置。
type RedisConfig struct {
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	PoolSize     int    `yaml:"pool_size"`
	DialTimeout  int    `yaml:"dial_timeout"`  // 秒
	ReadTimeout  int    `yaml:"read_timeout"`  // 秒
	WriteTimeout int    `yaml:"write_timeout"` // 秒
	Prefix       string `yaml:"prefix"`        // key 前缀
}

// Addr 返回 host:port。
func (r RedisConfig) Addr() string { return fmt.Sprintf("%s:%d", r.Host, r.Port) }

// JWTConfig 后台登录令牌配置。
type JWTConfig struct {
	Secret     string `yaml:"secret"`      // 签名密钥，生产环境务必修改
	Issuer     string `yaml:"issuer"`      // 签发者
	ExpireHour int    `yaml:"expire_hour"` // 令牌有效期(小时)
}

// ZLMConfig ZLMediaKit 对接配置。
type ZLMConfig struct {
	BaseURL   string `yaml:"base_url"`   // 如 http://127.0.0.1:8080
	Secret    string `yaml:"secret"`     // ZLM api.secret
	App       string `yaml:"app"`        // 默认 app，如 live
	Vhost     string `yaml:"vhost"`      // 默认 vhost
	Timeout   int    `yaml:"timeout"`    // HTTP 调用超时(秒)
	HookPath  string `yaml:"hook_path"`  // 本服务接收 ZLM 回调的路径前缀
	SyncSecs  int    `yaml:"sync_secs"`  // 定期与 ZLM 同步流状态间隔(秒)
	RTMPRTMP  int    `yaml:"rtmp_port"`  // 对外 RTMP 端口
	RTMPSPort int    `yaml:"rtmps_port"` // RTMPS 推流端口（Nginx stream TLS 终止后转发到 RTMP 端口），0=未开启
}

// HLSConfig HLS 加密与代理配置。
type HLSConfig struct {
	// UpstreamURL 是 ZLMediaKit 的 HTTP 根地址，用于回源拉取 m3u8/切片
	UpstreamURL string `yaml:"upstream_url"`
	// ZlmWWWWDir 是 ZLMediaKit 的 HTTP 文件根目录（HLS 切片落盘位置）。
	// 配置后加密代理优先直接读文件（绕过 on_play 鉴权与 HTTP 开销），失败再回退 HTTP 回源。
	// 同机部署时建议配置，如 /www/server/zlm/bin/www
	ZlmWWWDir string `yaml:"zlm_www_dir"`
	// KeyDir 是 HLS 密钥的存放目录
	KeyDir string `yaml:"key_dir"`
	// CacheDir 是加密后 m3u8/切片的缓存目录
	CacheDir string `yaml:"cache_dir"`
	// DefaultExpireSec 是密钥默认有效期(秒)
	DefaultExpireSec int `yaml:"default_expire_sec"`
	// IVStrategy seq/random
	IVStrategy string `yaml:"iv_strategy"`
}

// AuthConfig 播放分发鉴权默认策略。
type AuthConfig struct {
	Enabled        bool   `yaml:"enabled"`         // 是否开启播放鉴权
	TokenExpire    int    `yaml:"token_expire"`    // Token 默认有效期(秒)
	IPBindEnabled  bool   `yaml:"ip_bind_enabled"` // 是否允许开启 IP 绑定
	ParamName      string `yaml:"param_name"`      // Token 参数名，如 token
	AllowEmpty     bool   `yaml:"allow_empty"`     // 是否允许无 Token 播放
	AllowAnonymous bool   `yaml:"allow_anonymous"`
}

// LogConfig 日志配置。
type LogConfig struct {
	Level      string `yaml:"level"`       // debug/info/warn/error
	Format     string `yaml:"format"`      // json/text
	OutputFile string `yaml:"output_file"` // 输出文件，空则输出到 stdout
	MaxSize    int    `yaml:"max_size"`    // 单文件大小(MB)
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"` // 天
}

// FrontendConfig 前端资源配置。
type FrontendConfig struct {
	DistDir string `yaml:"dist_dir"` // 前端构建产物目录
}

var (
	global *Config
	once   sync.Once
)

// Default 返回内置默认配置。
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            8090,
			TrustedProxies:  []string{"127.0.0.1", "::1"},
			Mode:            "release",
			StaticDir:       "",
			EnableSwagger:   false,
			ReadTimeout:     30,
			WriteTimeout:    30,
			ShutdownTimeout: 10,
		},
		MySQL: MySQLConfig{
			Host:            "127.0.0.1",
			Port:            3306,
			User:            "root",
			Password:        "",
			Database:        "streamswitch",
			Charset:         "utf8mb4",
			MaxOpenConns:    50,
			MaxIdleConns:    10,
			ConnMaxLifetime: 3600,
			ParseTime:       true,
		},
		Redis: RedisConfig{
			Host:         "127.0.0.1",
			Port:         6379,
			Password:     "",
			DB:           0,
			PoolSize:     20,
			DialTimeout:  5,
			ReadTimeout:  3,
			WriteTimeout: 3,
			Prefix:       "livems:",
		},
		JWT: JWTConfig{
			Secret:     "CHANGE_ME_livems_jwt_secret_2026",
			Issuer:     "livems",
			ExpireHour: 24,
		},
		ZLM: ZLMConfig{
			BaseURL:   "http://127.0.0.1:8080",
			Secret:    "035c73f7-bb6b-4889-a715-d9eb2d1925cc",
			App:       "live",
			Vhost:     "__defaultVhost__",
			Timeout:   10,
			HookPath:  "/index/hook",
			SyncSecs:  30,
			RTMPRTMP:  1935,
			RTMPSPort: 0,
		},
		Log: LogConfig{
			Level:      "info",
			Format:     "text",
			OutputFile: "",
			MaxSize:    100,
			MaxBackups: 7,
			MaxAge:     30,
		},
		HLS: HLSConfig{
			UpstreamURL:     "http://127.0.0.1:8080",
			KeyDir:          "./data/hls_key",
			CacheDir:        "./data/hls_cache",
			DefaultExpireSec: 0,
			IVStrategy:      "seq",
		},
		Auth: AuthConfig{
			Enabled:        true,
			TokenExpire:    3600,
			IPBindEnabled:  false,
			ParamName:      "token",
			AllowEmpty:     false,
			AllowAnonymous: false,
		},
		Frontend: FrontendConfig{
			DistDir: "../frontend/dist",
		},
		Nginx: NginxConfig{
			VhostConf: "/www/server/panel/vhost/nginx/streamswitch.conf",
			CertDir:   "/www/server/panel/vhost/cert",
			ReloadCmd: "nginx -s reload",
		},
		Studio: StudioConfig{
			FFmpegPath:  "./bin/tools/ffmpeg",
			FFprobePath: "./bin/tools/ffprobe",
			FontFile:    "./assets/fonts/NotoSansSC-Regular.otf",
			DataDir:     "./data/studio",
			PushBase:    "rtmp://127.0.0.1:1935",
		},
	}
}

// Load 从 YAML 文件加载配置（文件不存在则使用默认值），随后应用环境变量覆盖。
// 传入空路径时仅使用默认值 + 环境变量。
func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		// 允许配置文件不存在（首次部署时用默认值 + 环境变量即可跑起来）
		if _, err := os.Stat(path); err == nil {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("读取配置文件失败 %s: %w", path, err)
			}
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("解析配置文件失败 %s: %w", path, err)
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("访问配置文件失败 %s: %w", path, err)
		}
	}

	applyEnv(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// applyEnv 使用 APP_ 前缀的环境变量覆盖配置，键名用下划线表示层级。
// 例如 APP_SERVER_PORT=9000 覆盖 server.port；APP_MYSQL_PASSWORD=xxx 覆盖 mysql.password。
func applyEnv(cfg *Config) {
	setStr(&cfg.Server.Host, "APP_SERVER_HOST")
	setInt(&cfg.Server.Port, "APP_SERVER_PORT")
	setStr(&cfg.Server.Mode, "APP_SERVER_MODE")
	setStr(&cfg.Server.StaticDir, "APP_SERVER_STATIC_DIR")
	setBool(&cfg.Server.EnableSwagger, "APP_SERVER_ENABLE_SWAGGER")

	setStr(&cfg.MySQL.Host, "APP_MYSQL_HOST")
	setInt(&cfg.MySQL.Port, "APP_MYSQL_PORT")
	setStr(&cfg.MySQL.User, "APP_MYSQL_USER")
	setStr(&cfg.MySQL.Password, "APP_MYSQL_PASSWORD")
	setStr(&cfg.MySQL.Database, "APP_MYSQL_DATABASE")

	setStr(&cfg.Redis.Host, "APP_REDIS_HOST")
	setInt(&cfg.Redis.Port, "APP_REDIS_PORT")
	setStr(&cfg.Redis.Password, "APP_REDIS_PASSWORD")
	setInt(&cfg.Redis.DB, "APP_REDIS_DB")
	setStr(&cfg.Redis.Prefix, "APP_REDIS_PREFIX")

	setStr(&cfg.JWT.Secret, "APP_JWT_SECRET")
	setStr(&cfg.JWT.Issuer, "APP_JWT_ISSUER")
	setInt(&cfg.JWT.ExpireHour, "APP_JWT_EXPIRE_HOUR")

	setStr(&cfg.ZLM.BaseURL, "APP_ZLM_BASE_URL")
	setStr(&cfg.ZLM.Secret, "APP_ZLM_SECRET")
	setStr(&cfg.ZLM.App, "APP_ZLM_APP")
	setStr(&cfg.ZLM.Vhost, "APP_ZLM_VHOST")
	setInt(&cfg.ZLM.RTMPRTMP, "APP_ZLM_RTMP_PORT")

	setStr(&cfg.HLS.UpstreamURL, "APP_HLS_UPSTREAM_URL")
	setStr(&cfg.HLS.ZlmWWWDir, "APP_HLS_ZLM_WWW_DIR")
	setStr(&cfg.Nginx.VhostConf, "APP_NGINX_VHOST_CONF")
	setStr(&cfg.Nginx.CertDir, "APP_NGINX_CERT_DIR")
	setStr(&cfg.Nginx.ReloadCmd, "APP_NGINX_RELOAD_CMD")
	setStr(&cfg.Studio.FFmpegPath, "APP_STUDIO_FFMPEG_PATH")
	setStr(&cfg.Studio.FFprobePath, "APP_STUDIO_FFPROBE_PATH")
	setStr(&cfg.Studio.FontFile, "APP_STUDIO_FONT_FILE")
	setStr(&cfg.Studio.DataDir, "APP_STUDIO_DATA_DIR")
	setStr(&cfg.Studio.PushBase, "APP_STUDIO_PUSH_BASE")
	setStr(&cfg.HLS.KeyDir, "APP_HLS_KEY_DIR")
	setStr(&cfg.HLS.CacheDir, "APP_HLS_CACHE_DIR")
	setInt(&cfg.HLS.DefaultExpireSec, "APP_HLS_DEFAULT_EXPIRE_SEC")

	setStr(&cfg.Log.Level, "APP_LOG_LEVEL")
	setStr(&cfg.Log.Format, "APP_LOG_FORMAT")
	setStr(&cfg.Log.OutputFile, "APP_LOG_OUTPUT_FILE")
}

func setStr(dst *string, env string) {
	if v, ok := os.LookupEnv(env); ok && v != "" {
		*dst = v
	}
}

func setInt(dst *int, env string) {
	if v, ok := os.LookupEnv(env); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*dst = n
		}
	}
}

func setBool(dst *bool, env string) {
	if v, ok := os.LookupEnv(env); ok && v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			*dst = b
		}
	}
}

// Validate 校验配置的必填项。
func (c *Config) Validate() error {
	if c.MySQL.Database == "" {
		return fmt.Errorf("配置错误: mysql.database 不能为空")
	}
	if c.MySQL.Host == "" {
		return fmt.Errorf("配置错误: mysql.host 不能为空")
	}
	if c.ZLM.BaseURL == "" {
		return fmt.Errorf("配置错误: zlm.base_url 不能为空")
	}
	if c.JWT.Secret == "" {
		return fmt.Errorf("配置错误: jwt.secret 不能为空")
	}
	if c.ZLM.App == "" {
		c.ZLM.App = "live"
	}
	if c.ZLM.Vhost == "" {
		c.ZLM.Vhost = "__defaultVhost__"
	}
	if c.Redis.Prefix == "" {
		c.Redis.Prefix = "livems:"
	}
	if c.Auth.ParamName == "" {
		c.Auth.ParamName = "token"
	}
	return nil
}

// Get 返回全局配置单例。
func Get() *Config {
	once.Do(func() {
		if global == nil {
			global = Default()
		}
	})
	return global
}

// Set 设置全局配置单例（供启动流程使用）。
func Set(c *Config) {
	global = c
	once.Do(func() {})
}
