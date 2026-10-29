// Package middleware 提供 Gin 中间件：认证、权限、日志、跨域等。
package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"livems/internal/pkg/jwt"
	"livems/internal/pkg/logger"
	"livems/internal/pkg/response"
	"livems/internal/pkg/util"
)

// ContextKey 上下文键名。
const (
	// ContextUserID 当前登录用户ID
	ContextUserID = "ctx_user_id"
	// ContextUsername 当前登录用户名
	ContextUsername = "ctx_username"
	// ContextIsSuper 当前用户是否为超级管理员
	ContextIsSuper = "ctx_is_super"
	// ContextClientIP 当前请求的真实客户端IP
	ContextClientIP = "ctx_client_ip"
)

// Auth 认证中间件：校验后台登录令牌。
func Auth(jwtMgr *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			response.Unauthorized(c, "未登录或登录已失效")
			c.Abort()
			return
		}
		claims, err := jwtMgr.Parse(token)
		if err != nil {
			response.Unauthorized(c, "登录状态已失效，请重新登录")
			c.Abort()
			return
		}
		c.Set(ContextUserID, claims.UserID)
		c.Set(ContextUsername, claims.Username)
		c.Set(ContextIsSuper, claims.Super)
		c.Next()
	}
}

// extractToken 从请求中提取令牌。
// 优先读取 Authorization 头（Bearer 方案），其次读取 token 查询参数与 X-Token 头。
func extractToken(c *gin.Context) string {
	auth := c.GetHeader("Authorization")
	if auth != "" {
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			return strings.TrimSpace(auth[7:])
		}
		return strings.TrimSpace(auth)
	}
	if t := c.GetHeader("X-Token"); t != "" {
		return strings.TrimSpace(t)
	}
	if t := c.Query("token"); t != "" {
		return strings.TrimSpace(t)
	}
	return ""
}

// PermissionChecker 权限校验接口，由 service.UserService 实现。
type PermissionChecker interface {
	HasPermission(userID uint64, code string) bool
}

// RequirePermission 权限校验中间件：要求当前用户拥有指定权限标识。
//
// 用法：router.GET("/xxx", middleware.RequirePermission(permSvc, "stream:create"), handler)
func RequirePermission(checker PermissionChecker, code string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if isSuper(c) {
			c.Next()
			return
		}
		userID := CurrentUserID(c)
		if userID == 0 {
			response.Unauthorized(c, "未登录")
			c.Abort()
			return
		}
		if !checker.HasPermission(userID, code) {
			response.Forbidden(c, "没有该操作的权限: "+code)
			c.Abort()
			return
		}
		c.Next()
	}
}

// CurrentUserID 从上下文取当前用户ID。
func CurrentUserID(c *gin.Context) uint64 {
	if v, ok := c.Get(ContextUserID); ok {
		if id, ok := v.(uint64); ok {
			return id
		}
	}
	return 0
}

// CurrentUsername 从上下文取当前用户名。
func CurrentUsername(c *gin.Context) string {
	if v, ok := c.Get(ContextUsername); ok {
		if name, ok := v.(string); ok {
			return name
		}
	}
	return ""
}

// isSuper 判断当前用户是否为超级管理员。
func isSuper(c *gin.Context) bool {
	if v, ok := c.Get(ContextIsSuper); ok {
		if s, ok := v.(bool); ok {
			return s
		}
	}
	return false
}

// ClientIP 从请求中提取真实客户端 IP。
//
// 优先读取 X-Forwarded-For（取第一段），其次 X-Real-IP，最后 RemoteAddr。
func ClientIP(c *gin.Context) string {
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		// XFF 可能是 "client, proxy1, proxy2"，取第一个
		parts := strings.Split(xff, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	if rip := c.GetHeader("X-Real-IP"); rip != "" {
		return strings.TrimSpace(rip)
	}
	// RemoteAddr 形如 "1.2.3.4:5678"
	addr := c.Request.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx > 0 {
		addr = addr[:idx]
	}
	return strings.Trim(addr, "[]")
}

// ClientIPMiddleware 把真实客户端 IP 写入上下文。
func ClientIPMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(ContextClientIP, ClientIP(c))
		c.Next()
	}
}

// GetClientIP 从上下文取真实客户端 IP。
func GetClientIP(c *gin.Context) string {
	if v, ok := c.Get(ContextClientIP); ok {
		if ip, ok := v.(string); ok {
			return ip
		}
	}
	return ClientIP(c)
}

// Logger 请求日志中间件。
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		c.Next()

		cost := time.Since(start)
		status := c.Writer.Status()
		ip := GetClientIP(c)
		method := c.Request.Method

		switch {
		case status >= 500:
			logger.Error("%s %s %s %d %v", ip, method, path, status, cost)
		case status >= 400:
			logger.Warn("%s %s %s %d %v", ip, method, path, status, cost)
		default:
			logger.Info("%s %s %s %d %v", ip, method, path, status, cost)
		}
	}
}

// Recovery  panic 恢复中间件。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("请求发生 panic: %v %s %s", r, c.Request.Method, c.Request.URL.Path)
				response.ServerError(c, "服务器内部错误")
				c.Abort()
			}
		}()
		c.Next()
	}
}

// CORS 跨域中间件。
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin == "" {
			origin = "*"
		}
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Content-Length, Accept, Authorization, X-Token, X-Requested-With")
		c.Header("Access-Control-Max-Age", "86400")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// 确保 util 包被引用。
var _ = util.IsValidIP
