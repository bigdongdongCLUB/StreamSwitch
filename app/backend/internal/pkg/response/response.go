// Package response 统一封装 HTTP JSON 响应格式。
package response

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"livems/internal/pkg/logger"
)

// 业务状态码。
const (
	// CodeSuccess 请求成功
	CodeSuccess = 0
	// CodeFail 请求失败（通用）
	CodeFail = 1
	// CodeUnauthorized 未登录或令牌失效
	CodeUnauthorized = 401
	// CodeForbidden 无权限
	CodeForbidden = 403
	// CodeNotFound 资源不存在
	CodeNotFound = 404
	// CodeParamError 参数错误
	CodeParamError = 400
	// CodeServerError 服务端内部错误
	CodeServerError = 500
)

// Body 统一响应体结构。
type Body struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Page 分页数据结构。
type Page struct {
	List     any   `json:"list"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

// JSON 输出统一格式的 JSON 响应。
//
// 失败响应的 message 若是数据库/网络驱动的英文原始报错（如 "sql: expected 21 arguments, got 23"），
// 会被转换为中文提示返回给前端，原始文本写入日志供排查。
func JSON(c *gin.Context, status int, code int, message string, data any) {
	if code != CodeSuccess {
		message = Humanize(c, message)
	}
	c.JSON(status, Body{Code: code, Message: message, Data: data})
}

// rawErrorRules 底层英文报错 → 中文提示。按顺序匹配，命中即止。
var rawErrorRules = []struct {
	re  *regexp.Regexp
	msg string
}{
	{regexp.MustCompile(`sql: expected \d+ arguments, got \d+`), "服务端 SQL 语句与参数数量不匹配（程序缺陷，请联系开发者修复）"},
	{regexp.MustCompile(`sql: no rows in result set`), "记录不存在或已被删除"},
	{regexp.MustCompile(`Error 1062[^']*'([^']*)'`), "已存在相同的记录（%s），请勿重复添加"},
	{regexp.MustCompile(`Error 1406[^']*column '([^']*)'`), "字段「%s」内容过长，超出数据库允许的长度"},
	{regexp.MustCompile(`Error 1366[^']*column '([^']*)'`), "字段「%s」的值格式不正确"},
	{regexp.MustCompile(`Error 1048[^']*Column '([^']*)'`), "字段「%s」不能为空"},
	{regexp.MustCompile(`Error 1054[^']*column '([^']*)'`), "数据库缺少字段「%s」，请检查是否已执行最新的建表/升级脚本"},
	{regexp.MustCompile(`Error 1146[^']*Table '([^']*)'`), "数据库缺少表「%s」，请检查是否已导入 schema.sql"},
	{regexp.MustCompile(`Error 1451|Error 1452`), "该记录被其它数据引用，无法完成操作"},
	{regexp.MustCompile(`Error 1213|Deadlock found`), "数据库操作冲突，请稍后重试"},
	{regexp.MustCompile(`Error 1040|Too many connections`), "数据库连接数已满，请稍后重试"},
	{regexp.MustCompile(`Error 1045|Access denied for user`), "数据库账号或密码错误，请检查后端配置"},
	{regexp.MustCompile(`driver: bad connection|invalid connection|connection refused.*3306|dial tcp.*3306`), "数据库连接失败，请确认 MySQL 服务正在运行"},
	{regexp.MustCompile(`connection refused.*6379|dial tcp.*6379|redis:`), "Redis 连接失败，请确认 Redis 服务正在运行"},
	{regexp.MustCompile(`connection refused.*8080|dial tcp.*8080|ZLM|zlm`), "无法连接 ZLMediaKit，请确认流媒体服务正在运行"},
	{regexp.MustCompile(`context deadline exceeded|i/o timeout|Client\.Timeout exceeded`), "请求超时，请稍后重试"},
	{regexp.MustCompile(`context canceled`), "请求已被取消"},
	{regexp.MustCompile(`no such file or directory`), "文件或目录不存在"},
	{regexp.MustCompile(`permission denied`), "没有文件访问权限，请检查服务运行账号的权限"},
	{regexp.MustCompile(`no space left on device`), "磁盘空间不足"},
	{regexp.MustCompile(`executable file not found|exec: "`), "所需外部程序（如 ffmpeg/nginx）未找到，请检查后端配置中的路径"},
	{regexp.MustCompile(`invalid character|unexpected end of JSON|cannot unmarshal`), "请求数据格式不正确"},
	{regexp.MustCompile(`EOF`), "连接意外中断，请重试"},
}

// Humanize 把 message 中夹带的英文底层报错替换为中文提示。
//
// 中文/已友好化的提示原样返回。命中规则时保留调用方自己加的中文前缀（如 "保存失败: "），
// 并把原始报错以 WARN 级别记录到日志，便于排查。
func Humanize(c *gin.Context, message string) string {
	for _, r := range rawErrorRules {
		loc := r.re.FindStringSubmatchIndex(message)
		if loc == nil {
			continue
		}
		friendly := r.msg
		if len(loc) >= 4 && loc[2] >= 0 && strings.Contains(friendly, "%s") {
			friendly = fmt.Sprintf(friendly, message[loc[2]:loc[3]])
		}
		// 保留原始报错前面调用方给出的中文上下文（以中文冒号/英文冒号分隔的前缀）
		prefix := ""
		if i := strings.LastIndexAny(message[:loc[0]], ":："); i >= 0 {
			prefix = strings.TrimSpace(message[:i])
			if prefix != "" {
				prefix += "："
			}
		}
		path := ""
		if c != nil && c.Request != nil {
			path = c.Request.Method + " " + c.Request.URL.Path
		}
		logger.Warn("接口报错已转为中文提示 [%s] 原始: %s", path, message)
		return prefix + friendly
	}
	return message
}

// OK 返回成功响应（HTTP 200）。
func OK(c *gin.Context, data any) {
	JSON(c, http.StatusOK, CodeSuccess, "success", data)
}

// OKMsg 返回带自定义提示的成功响应。
func OKMsg(c *gin.Context, message string, data any) {
	JSON(c, http.StatusOK, CodeSuccess, message, data)
}

// Fail 返回业务失败响应（HTTP 200，body.code 非 0）。
func Fail(c *gin.Context, code int, message string) {
	JSON(c, http.StatusOK, code, message, nil)
}

// Error 返回错误响应（HTTP 4xx/5xx）。
func Error(c *gin.Context, status int, message string) {
	JSON(c, status, status, message, nil)
}

// BadRequest 参数错误。
func BadRequest(c *gin.Context, message string) {
	JSON(c, http.StatusBadRequest, CodeParamError, message, nil)
}

// Unauthorized 未认证。
func Unauthorized(c *gin.Context, message string) {
	JSON(c, http.StatusUnauthorized, CodeUnauthorized, message, nil)
}

// Forbidden 无权限。
func Forbidden(c *gin.Context, message string) {
	JSON(c, http.StatusForbidden, CodeForbidden, message, nil)
}

// NotFound 资源不存在。
func NotFound(c *gin.Context, message string) {
	JSON(c, http.StatusNotFound, CodeNotFound, message, nil)
}

// ServerError 服务端错误。
func ServerError(c *gin.Context, message string) {
	JSON(c, http.StatusInternalServerError, CodeServerError, message, nil)
}
