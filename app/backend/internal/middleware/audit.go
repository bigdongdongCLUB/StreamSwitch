package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"livems/internal/models"
	"livems/internal/pkg/logger"
	"livems/internal/pkg/response"
)

// auditBodyLimit 审计日志最多截取的请求体字节数（非上传请求）。
const auditBodyLimit = 64 << 10

// AuditRecorder 操作日志写入器接口。
type AuditRecorder interface {
	CreateLog(l *models.SysLog) error
}

// Audit 操作日志审计中间件。
//
// 记录每一次写操作（POST/PUT/DELETE/PATCH）的：
//   - 操作人、模块、动作、请求路径、IP、请求参数、结果、耗时。
//
// 为了避免记录敏感信息（密码等），请求参数中的敏感字段会被脱敏。
func Audit(recorder AuditRecorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 只审计写操作
		method := c.Request.Method
		switch method {
		case "POST", "PUT", "DELETE", "PATCH":
		default:
			c.Next()
			return
		}

		start := time.Now()
		// 读取并缓存请求体，供后续解析。
		// 文件上传（multipart）不读取正文：素材可达数 GB，整体读入内存会拖慢请求甚至 OOM，
		// 只记录大小；其余 JSON 请求也只截取前 auditBodyLimit 字节，剩余部分原样透传给处理器。
		var bodyBytes []byte
		if c.Request.Body != nil {
			if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
				bodyBytes = []byte(fmt.Sprintf(`{"_body":"multipart/form-data","_size":%d}`, c.Request.ContentLength))
			} else {
				bodyBytes, _ = io.ReadAll(io.LimitReader(c.Request.Body, auditBodyLimit))
				c.Request.Body = struct {
					io.Reader
					io.Closer
				}{io.MultiReader(bytes.NewReader(bodyBytes), c.Request.Body), c.Request.Body}
			}
		}

		c.Next()
		cost := time.Since(start)

	// 解析模块与动作
	module, action := parsePathAction(c.Request.URL.Path, method)

		entry := &models.SysLog{
			UserID:   CurrentUserID(c),
			Username: CurrentUsername(c),
			Module:   module,
			Action:   action,
			Method:   method,
			Path:     c.Request.URL.Path,
			IP:       GetClientIP(c),
			Params:   maskSensitiveParams(string(bodyBytes)),
			Status:   1,
			Duration: int(cost.Milliseconds()),
			CreatedAt: time.Now().Unix(),
		}
		if entry.UserID == 0 {
			entry.Username = "anonymous"
		}
		// 记录失败信息
		if len(c.Errors) > 0 {
			entry.Status = 0
			entry.ErrorMsg = utilTruncate(c.Errors.String(), 1000)
		} else if c.Writer.Status() >= 400 {
			entry.Status = 0
		}

		if err := recorder.CreateLog(entry); err != nil {
			logger.Warn("写入操作日志失败: %v", err)
		}
	}
}

// parsePathAction 从请求路径与方法中解析模块与动作。
//
// 例：
//
//	POST   /api/v1/streams          => streams/create
//	PUT    /api/v1/streams/1        => streams/update
//	DELETE /api/v1/streams/1        => streams/delete
//	POST   /api/v1/streams/1/toggle => streams/toggle
//	POST   /api/v1/auth/login       => auth/login
func parsePathAction(path, method string) (module, action string) {
	// 去掉前缀
	p := strings.TrimPrefix(path, "/api/v1/")
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return "unknown", "unknown"
	}
	module = parts[0]
	switch {
	case len(parts) == 1:
		// 集合资源：POST=create，其余按方法名
		if method == "POST" {
			action = "create"
		} else {
			action = strings.ToLower(method)
		}
	case len(parts) == 2:
		if isNumeric(parts[1]) {
			// 单资源：PUT=update DELETE=delete，其余按方法名
			switch method {
			case "PUT", "PATCH":
				action = "update"
			case "DELETE":
				action = "delete"
			default:
				action = strings.ToLower(method)
			}
		} else {
			// 子动作：auth/login、streams/sync、streams/urls
			action = parts[1]
		}
	default:
		// 多级路径取最后一段：streams/1/toggle => toggle
		action = parts[len(parts)-1]
	}
	return module, action
}

// isNumeric 判断字符串是否为数字。
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// maskSensitiveParams 对请求参数中的敏感字段脱敏。
func maskSensitiveParams(raw string) string {
	if raw == "" {
		return ""
	}
	// 尝试按 JSON 解析并脱敏
	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err == nil {
		maskMap(data)
		out, err := json.Marshal(data)
		if err == nil {
			return utilTruncate(string(out), 2000)
		}
	}
	// 非 JSON，按 key=value 形式处理
	if len(raw) > 2000 {
		return utilTruncate(raw, 2000)
	}
	return raw
}

// maskMap 递归脱敏 map 中的敏感字段。
func maskMap(m map[string]any) {
	sensitive := map[string]bool{
		"password": true, "old_password": true, "new_password": true,
		"push_key": true, "key": true, "secret": true, "token": true,
	}
	for k, v := range m {
		if sensitive[strings.ToLower(k)] {
			m[k] = "******"
			continue
		}
		if sub, ok := v.(map[string]any); ok {
			maskMap(sub)
		}
	}
}

// utilTruncate 按字节截断字符串。
func utilTruncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// 确保 response 包被引用（供其它中间件复用）。
var _ = response.OK
