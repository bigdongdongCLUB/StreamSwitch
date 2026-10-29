// Package server 负责应用启动、路由注册与优雅退出。
package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	_ "net/http/pprof" // 注册到 DefaultServeMux，仅在 pprof_addr 配置时单独监听
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"livems/internal/config"
	"livems/internal/database"
	"livems/internal/handler"
	"livems/internal/middleware"
	"livems/internal/pkg/logger"
	"livems/internal/repository"
	"livems/internal/service"
	"livems/internal/zlm"
)

// Version 服务版本号（由 main 在启动时设置）。
var Version = "dev"

// Server 应用服务器。
type Server struct {
	cfg    *config.Config
	engine *gin.Engine
	http   *http.Server
}

// New 创建应用服务器并注册所有路由。
func New(cfg *config.Config) *Server {
	gin.SetMode(cfg.Server.Mode)
	engine := gin.New()
	engine.Use(
		middleware.Recovery(),
		middleware.Logger(),
		middleware.ClientIPMiddleware(),
		middleware.RequestOrigin(),
		middleware.CORS(),
	)

	// 可信代理（用于正确解析客户端 IP）
	if len(cfg.Server.TrustedProxies) > 0 {
		_ = engine.SetTrustedProxies(cfg.Server.TrustedProxies)
	}

	s := &Server{cfg: cfg, engine: engine}
	s.registerRoutes()
	return s
}

// registerRoutes 注册所有路由。
func (s *Server) registerRoutes() {
	// ---- 依赖初始化 ----
	playSvc := service.NewPlayService(s.cfg)
	authSvc := service.NewAuthService(s.cfg)
	settingsSvc := service.NewSettingsService(s.cfg, playSvc)
	streamSvc := service.NewStreamService(s.cfg, settingsSvc)
	proxySvc := service.NewProxyService(s.cfg)
	forwardSvc := service.NewForwardService(s.cfg)
	encryptSvc := service.NewEncryptService(s.cfg)
	userSvc := service.NewUserService(s.cfg)
	hookSvc := service.NewHookService(s.cfg, playSvc, authSvc, forwardSvc)
	logRepo := repository.NewLogRepo()

	// ---- 处理器 ----
	authH := handler.NewAuthHandler(userSvc)
	streamH := handler.NewStreamHandler(streamSvc, playSvc, authSvc)
	proxyH := handler.NewProxyHandler(proxySvc)
	forwardH := handler.NewForwardHandler(forwardSvc)
	playH := handler.NewPlayHandler(playSvc, authSvc)
	tokenH := handler.NewTokenHandler(authSvc)
	encryptH := handler.NewEncryptHandler(encryptSvc)
	userH := handler.NewUserHandler(userSvc)
	logH := handler.NewLogHandler(logRepo)
	hookH := handler.NewHookHandler(hookSvc)
	hlsH := handler.NewHLSHandler(encryptSvc, authSvc)
	settingsH := handler.NewSettingsHandler(settingsSvc)
	certH := handler.NewCertHandler(service.NewCertService(s.cfg))
	statsH := handler.NewStatsHandler()
	studioSvc := service.NewStudioService(s.cfg)
	studioH := handler.NewStudioHandler(studioSvc)
	livePageH := handler.NewLivePageHandler(service.NewLivePageService(s.cfg, playSvc, authSvc))

	jwtMgr := userSvc.JWTManager()
	perm := middleware.PermissionChecker(userSvc)

	// ---- ZLMediaKit 回调（无需后台登录鉴权）----
	hookGroup := s.engine.Group(s.cfg.ZLM.HookPath)
	{
		hookGroup.POST("/on_publish", hookH.OnPublish)
		hookGroup.POST("/on_play", hookH.OnPlay)
		hookGroup.POST("/on_stream_changed", hookH.OnStreamChanged)
		hookGroup.POST("/on_flow_report", hookH.OnFlowReport)
		hookGroup.POST("/on_stream_not_found", hookH.OnStreamNotFound)
		hookGroup.POST("/on_stream_none_reader", hookH.OnStreamNoneReader)
		hookGroup.POST("/on_server_started", hookH.OnServerStarted)
		hookGroup.POST("/on_server_exited", hookH.OnServerExited)
	}

	// ---- ZLM 播放分发反代 ----
	// 将 /live/ 下的播放请求（HTTP-FLV / WS-FLV / HLS）反代到 ZLMediaKit 的 HTTP 端口，
	// 使任意访问入口（Nginx 反代或直连本服务 8090）的同源播放/预览都可用。
	// 播放鉴权由 ZLM 的 on_play 钩子统一触发，链路一致。
	{
		zlmUpstream, err := url.Parse(s.cfg.ZLM.BaseURL)
		if err == nil {
			rp := httputil.NewSingleHostReverseProxy(zlmUpstream)
			rp.FlushInterval = 200 * time.Millisecond // FLV 流式响应，低延迟 flush
			s.engine.Any("/live/*path", func(c *gin.Context) {
				rp.ServeHTTP(c.Writer, c.Request)
			})
		}
	}

	// ---- HLS 加密播放代理（由 Nginx 反代到本服务）----
	// 路由说明：
	//   /hls-enc/:app/:stream      播放列表（:stream 以 .m3u8 结尾）
	//   /hls-enc/:app/:stream/*seg 切片（seg 可含日期子目录，如 2026-10-27/11/17-57_16.ts）
	//   /hls-key/:app/:key         AES-128 密钥下发
	{
		s.engine.GET("/hls-enc/:app/:stream", hlsH.Play)
		s.engine.GET("/hls-enc/:app/:stream/*seg", hlsH.Play)
		s.engine.GET("/hls-key/:app/:key", hlsH.Key)
	}

	// ---- 后台 API ----
	api := s.engine.Group("/api/v1")
	{
		// 登录相关（免鉴权，登录行为纳入审计）
		api.POST("/auth/login", middleware.Audit(logRepo), authH.Login)

		// 系统初始化：首次打开后台注册首个超级管理员（免鉴权，仅无用户时可用）
		api.GET("/setup/status", authH.SetupStatus)
		api.POST("/setup", middleware.Audit(logRepo), authH.Setup)
		api.POST("/auth/logout", middleware.Auth(jwtMgr), authH.Logout)

		// 直播分享页（观众侧，免登录）
		api.GET("/p/:code", livePageH.PublicInfo)
		api.POST("/p/:code/auth", livePageH.VerifyPassword)
		api.GET("/p/:code/play", livePageH.PlayData)

		// 需要登录
		authed := api.Group("")
		authed.Use(middleware.Auth(jwtMgr), middleware.Audit(logRepo))
		{
			authed.GET("/auth/profile", authH.Profile)
			authed.POST("/auth/password", authH.ChangePassword)

			// 流管理
			authed.GET("/streams", middleware.RequirePermission(perm, "stream:list"), streamH.List)
			authed.GET("/streams/:id", middleware.RequirePermission(perm, "stream:list"), streamH.Get)
			authed.GET("/streams/:id/info", middleware.RequirePermission(perm, "stream:list"), streamH.Info)
			authed.GET("/streams/:id/urls", middleware.RequirePermission(perm, "stream:list"), streamH.URLs)
			authed.POST("/streams/urls", middleware.RequirePermission(perm, "stream:list"), streamH.GenerateURL)
			authed.POST("/streams/sync", middleware.RequirePermission(perm, "stream:update"), streamH.Sync)
			authed.POST("/streams", middleware.RequirePermission(perm, "stream:create"), streamH.Create)
			authed.PUT("/streams/:id", middleware.RequirePermission(perm, "stream:update"), streamH.Update)
			authed.POST("/streams/:id/toggle", middleware.RequirePermission(perm, "stream:toggle"), streamH.Toggle)
			authed.DELETE("/streams/:id", middleware.RequirePermission(perm, "stream:delete"), streamH.Delete)

			// 拉流代理
			authed.GET("/proxies", middleware.RequirePermission(perm, "proxy:list"), proxyH.List)
			authed.GET("/proxies/:id", middleware.RequirePermission(perm, "proxy:list"), proxyH.Get)
			authed.POST("/proxies", middleware.RequirePermission(perm, "proxy:create"), proxyH.Create)
			authed.PUT("/proxies/:id", middleware.RequirePermission(perm, "proxy:update"), proxyH.Update)
			authed.POST("/proxies/:id/toggle", middleware.RequirePermission(perm, "proxy:toggle"), proxyH.Toggle)
			authed.POST("/proxies/:id/start", middleware.RequirePermission(perm, "proxy:toggle"), proxyH.Start)
			authed.POST("/proxies/:id/stop", middleware.RequirePermission(perm, "proxy:toggle"), proxyH.Stop)
			authed.DELETE("/proxies/:id", middleware.RequirePermission(perm, "proxy:delete"), proxyH.Delete)

			// 转推流
			authed.GET("/forwards", middleware.RequirePermission(perm, "forward:list"), forwardH.List)
			authed.GET("/forwards/:id", middleware.RequirePermission(perm, "forward:list"), forwardH.Get)
			authed.POST("/forwards", middleware.RequirePermission(perm, "forward:create"), forwardH.Create)
			authed.PUT("/forwards/:id", middleware.RequirePermission(perm, "forward:update"), forwardH.Update)
			authed.POST("/forwards/:id/toggle", middleware.RequirePermission(perm, "forward:toggle"), forwardH.Toggle)
			authed.DELETE("/forwards/:id", middleware.RequirePermission(perm, "forward:delete"), forwardH.Delete)

			// 播放域名
			authed.GET("/domains", middleware.RequirePermission(perm, "domain:list"), playH.ListDomains)
			authed.POST("/domains", middleware.RequirePermission(perm, "domain:create"), playH.CreateDomain)
			authed.PUT("/domains/:id", middleware.RequirePermission(perm, "domain:update"), playH.UpdateDomain)
			authed.POST("/domains/:id/default", middleware.RequirePermission(perm, "domain:update"), playH.SetDefaultDomain)
			authed.DELETE("/domains/:id", middleware.RequirePermission(perm, "domain:delete"), playH.DeleteDomain)

			// 播放格式开关
			authed.GET("/formats", middleware.RequirePermission(perm, "format:list"), playH.ListFormats)
			authed.POST("/formats/:format/toggle", middleware.RequirePermission(perm, "format:update"), playH.SetFormatEnabled)

			// 鉴权策略
			authed.GET("/auth-policy", middleware.RequirePermission(perm, "stream:list"), playH.GetPolicy)
			authed.PUT("/auth-policy", middleware.RequirePermission(perm, "stream:update"), playH.SetPolicy)

			// 对外接入设置（推流域名/端口，归入域名设置统一管理）
			authed.GET("/settings/ingress", middleware.RequirePermission(perm, "domain:list"), settingsH.GetIngress)
			authed.PUT("/settings/ingress", middleware.RequirePermission(perm, "domain:update"), settingsH.SaveIngress)

			// SSL 证书管理（域名配置页）
			authed.GET("/certs", middleware.RequirePermission(perm, "domain:list"), certH.List)
			authed.POST("/certs", middleware.RequirePermission(perm, "domain:update"), certH.Save)
			authed.POST("/certs/:name/apply", middleware.RequirePermission(perm, "domain:update"), certH.Apply)
			authed.DELETE("/certs/:name", middleware.RequirePermission(perm, "domain:update"), certH.Delete)

			// 直播分享页管理
			authed.GET("/live-pages", middleware.RequirePermission(perm, "livepage:list"), livePageH.List)
			authed.POST("/live-pages", middleware.RequirePermission(perm, "livepage:create"), livePageH.Create)
			authed.PUT("/live-pages/:id", middleware.RequirePermission(perm, "livepage:update"), livePageH.Update)
			authed.DELETE("/live-pages/:id", middleware.RequirePermission(perm, "livepage:delete"), livePageH.Delete)

			// 仪表盘统计（登录即可）
			authed.GET("/stats/traffic", statsH.TrafficTrend)

			// 制播管理（素材库 + 制播任务）
			studio := authed.Group("/studio")
			studio.GET("/materials", middleware.RequirePermission(perm, "studio:list"), studioH.ListMaterials)
			studio.POST("/materials", middleware.RequirePermission(perm, "studio:create"), studioH.UploadMaterial)
			studio.DELETE("/materials/:id", middleware.RequirePermission(perm, "studio:delete"), studioH.DeleteMaterial)
			studio.GET("/tasks", middleware.RequirePermission(perm, "studio:list"), studioH.ListTasks)
			studio.POST("/tasks", middleware.RequirePermission(perm, "studio:create"), studioH.CreateTask)
			studio.PUT("/tasks/:id", middleware.RequirePermission(perm, "studio:update"), studioH.UpdateTask)
			studio.POST("/tasks/:id/start", middleware.RequirePermission(perm, "studio:toggle"), studioH.StartTask)
			studio.POST("/tasks/:id/stop", middleware.RequirePermission(perm, "studio:toggle"), studioH.StopTask)
			studio.POST("/tasks/:id/toggle", middleware.RequirePermission(perm, "studio:toggle"), studioH.ToggleTask)
			studio.DELETE("/tasks/:id", middleware.RequirePermission(perm, "studio:delete"), studioH.DeleteTask)

			// 播放令牌
			authed.GET("/tokens", middleware.RequirePermission(perm, "token:list"), tokenH.ListTokens)
			authed.POST("/tokens", middleware.RequirePermission(perm, "token:create"), tokenH.CreateToken)
			authed.DELETE("/tokens/:id", middleware.RequirePermission(perm, "token:delete"), tokenH.DeleteToken)

			// HLS 加密
			authed.GET("/encrypts", middleware.RequirePermission(perm, "encrypt:list"), encryptH.List)
			authed.GET("/encrypts/:stream_id", middleware.RequirePermission(perm, "encrypt:list"), encryptH.Get)
			authed.PUT("/encrypts/:stream_id", middleware.RequirePermission(perm, "encrypt:update"), encryptH.Set)
			authed.POST("/encrypts/:stream_id/rotate", middleware.RequirePermission(perm, "encrypt:update"), encryptH.Rotate)
			authed.DELETE("/encrypts/:stream_id", middleware.RequirePermission(perm, "encrypt:update"), encryptH.Delete)

			// 用户与权限
			authed.GET("/users", middleware.RequirePermission(perm, "user:list"), userH.List)
			authed.GET("/users/:id", middleware.RequirePermission(perm, "user:list"), userH.Get)
			authed.POST("/users", middleware.RequirePermission(perm, "user:create"), userH.Create)
			authed.PUT("/users/:id", middleware.RequirePermission(perm, "user:update"), userH.Update)
			authed.POST("/users/:id/password", middleware.RequirePermission(perm, "user:resetpwd"), userH.ResetPassword)
			authed.DELETE("/users/:id", middleware.RequirePermission(perm, "user:delete"), userH.Delete)

			authed.GET("/roles", middleware.RequirePermission(perm, "role:list"), userH.ListRoles)
			authed.POST("/roles", middleware.RequirePermission(perm, "role:create"), userH.CreateRole)
			authed.PUT("/roles/:id", middleware.RequirePermission(perm, "role:update"), userH.UpdateRole)
			authed.DELETE("/roles/:id", middleware.RequirePermission(perm, "role:delete"), userH.DeleteRole)
			authed.GET("/roles/:id/permissions", middleware.RequirePermission(perm, "role:list"), userH.RolePermissions)

			authed.GET("/permissions", middleware.RequirePermission(perm, "role:list"), userH.ListPermissions)

			// 日志审计
			authed.GET("/logs", middleware.RequirePermission(perm, "log:list"), logH.ListLogs)
			authed.GET("/task-errors", middleware.RequirePermission(perm, "log:list"), logH.ListTaskErrors)
		}
	}

	// ---- 健康检查 ----
	s.engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "ok",
			"data": gin.H{
				"time":    time.Now().Format(time.RFC3339),
				"version": Version,
			},
		})
	})

	// ---- 前端静态资源 ----
	s.setupStatic()
}

// setupStatic 挂载前端构建产物（单页应用）。
func (s *Server) setupStatic() {
	distDir := s.cfg.Frontend.DistDir
	if distDir == "" {
		return
	}
	abs, err := filepath.Abs(distDir)
	if err != nil {
		return
	}
	if _, err := os.Stat(abs); err != nil {
		logger.Warn("前端目录不存在，跳过静态资源挂载: %s", abs)
		return
	}
	logger.Info("挂载前端静态资源: %s", abs)
	// 静态资源缓存策略：
	//   /lib 第三方库（内容稳定）     -> 长缓存 7 天
	//   /js /css 等业务代码           -> no-cache（每次协商验证，改动即时生效）
	//   index.html / SPA 回退         -> no-cache
	s.engine.Use(func(c *gin.Context) {
		p := c.Request.URL.Path
		switch {
		case strings.HasPrefix(p, "/lib/"):
			c.Header("Cache-Control", "public, max-age=604800")
		case strings.HasPrefix(p, "/js/") || strings.HasPrefix(p, "/css/") ||
			strings.HasPrefix(p, "/assets/") || strings.HasPrefix(p, "/img/") ||
			strings.HasPrefix(p, "/images/") || strings.HasPrefix(p, "/fonts/") ||
			strings.HasPrefix(p, "/static/"):
			c.Header("Cache-Control", "no-cache")
		}
		c.Next()
	})
	// 静态资源目录：自动挂载 dist 下存在的常见资源目录
	for _, dir := range []string{"assets", "js", "css", "lib", "img", "images", "fonts", "static"} {
		p := filepath.Join(abs, dir)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			s.engine.Static("/"+dir, p)
		}
	}
	// favicon 存在才注册，避免 404 噪音
	if _, err := os.Stat(filepath.Join(abs, "favicon.ico")); err == nil {
		s.engine.StaticFile("/favicon.ico", filepath.Join(abs, "favicon.ico"))
	}

	// SPA 回退：非 API / 非 Hook 路径一律返回 index.html
	indexPath := filepath.Join(abs, "index.html")
	s.engine.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		// API 与回调路径不回落
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, s.cfg.ZLM.HookPath) ||
			strings.HasPrefix(path, "/hls-enc/") || strings.HasPrefix(path, "/hls-key/") {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "接口不存在"})
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.File(indexPath)
	})
}

// Run 启动 HTTP 服务并阻塞，直到收到退出信号。
func (s *Server) Run() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Host, s.cfg.Server.Port)
	// 注意：read_timeout 只作用于请求头（ReadHeaderTimeout），不限制请求体读取时长，
	// 否则大文件上传（素材可达 4GB）会在 30 秒后被切断。
	// write_timeout 仍作为普通接口的整体超时；上传接口在 handler 内自行延长（见 StudioHandler.UploadMaterial）。
	s.http = &http.Server{
		Addr:              addr,
		Handler:           s.engine,
		ReadHeaderTimeout: time.Duration(s.cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(s.cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// 启动后台协程
	s.startBackgroundJobs()

	// 可选：pprof 诊断端口（仅建议绑定 127.0.0.1，用于排查 CPU/内存问题）
	if s.cfg.Server.PprofAddr != "" {
		go func() {
			logger.Info("pprof 诊断接口监听: http://%s/debug/pprof/", s.cfg.Server.PprofAddr)
			if err := http.ListenAndServe(s.cfg.Server.PprofAddr, nil); err != nil {
				logger.Warn("pprof 监听失败: %v", err)
			}
		}()
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("流媒体管理系统已启动，监听 %s", addr)
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case sig := <-quit:
		logger.Info("收到信号 %v，开始优雅退出", sig)
	}

	return s.Shutdown()
}

// startBackgroundJobs 启动后台周期任务。
func (s *Server) startBackgroundJobs() {
	// 定期与 ZLMediaKit 同步流状态
	syncSecs := s.cfg.ZLM.SyncSecs
	if syncSecs <= 0 {
		syncSecs = 30
	}
	go func() {
		ticker := time.NewTicker(time.Duration(syncSecs) * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			playSvc := service.NewPlayService(s.cfg)
			streamSvc := service.NewStreamService(s.cfg, service.NewSettingsService(s.cfg, playSvc))
			if err := streamSvc.SyncFromZLM(ctx); err != nil {
				logger.Warn("同步 ZLMediaKit 流状态失败: %v", err)
			}
			// 转推任务兜底：源在线但未运行的任务自动拉起
			forwardSvc := service.NewForwardService(s.cfg)
			forwardSvc.EnsureRunning(ctx)
			cancel()
		}
	}()

	// 定期清理过期的令牌留档与 HLS 缓存
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			authSvc := service.NewAuthService(s.cfg)
			authSvc.Cleanup()
		}
	}()
}

// Shutdown 优雅退出。
func (s *Server) Shutdown() error {
	timeout := time.Duration(s.cfg.Server.ShutdownTimeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// 先停止所有制播 ffmpeg 进程，避免僵尸进程占用推流
	service.NewStudioService(s.cfg).StopAllTasks()

	if s.http != nil {
		if err := s.http.Shutdown(ctx); err != nil {
			logger.Error("HTTP 服务关闭异常: %v", err)
		}
	}
	database.Close()
	logger.Info("服务已退出")
	return nil
}

// RestoreTasks 在启动后恢复代理与转推任务，并加载加密配置。
func RestoreTasks(cfg *config.Config) {
	// 确保播放格式数据完整
	formatRepo := repository.NewFormatRepo()
	if err := formatRepo.EnsureFormats(); err != nil {
		logger.Warn("初始化播放格式失败: %v", err)
	}
	// 恢复 HLS 加密密钥
	encryptSvc := service.NewEncryptService(cfg)
	encryptSvc.RestoreAll()
	// 恢复拉流代理任务
	proxySvc := service.NewProxyService(cfg)
	proxySvc.RestoreAll()
	// 恢复转推流任务
	forwardSvc := service.NewForwardService(cfg)
	forwardSvc.RestoreAll()
	// 检查 ZLMediaKit 连通性
	zlmClient := zlm.New(cfg.ZLM)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := zlmClient.Health(ctx); err != nil {
		logger.Warn("ZLMediaKit 暂不可用（%s）: %v", cfg.ZLM.BaseURL, err)
	} else {
		logger.Info("ZLMediaKit 连接正常: %s", cfg.ZLM.BaseURL)
	}
	// 恢复制播任务（ZLM 就绪后再拉起 ffmpeg 回推）
	studioSvc := service.NewStudioService(cfg)
	studioSvc.RestoreTasks()
}
