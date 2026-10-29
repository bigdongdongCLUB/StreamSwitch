// Package errors 定义业务错误类型，支持携带错误码并被 HTTP 层统一转换。
package errors

import (
	"errors"
	"fmt"
)

// Error 业务错误。
type Error struct {
	Code    int    // 业务错误码
	Message string // 错误提示
	Err     error  // 底层错误（可选，用于 errors.Is/As 链）
}

// Error 实现 error 接口。
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap 返回底层错误，支持 errors.Is / errors.As。
func (e *Error) Unwrap() error { return e.Err }

// New 创建一个业务错误。
func New(code int, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Newf 创建一个带格式化的业务错误。
func Newf(code int, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap 包装底层错误并附加业务码与提示。
func Wrap(code int, message string, err error) *Error {
	return &Error{Code: code, Message: message, Err: err}
}

// As 从错误链中提取 *Error。
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// Is 判断错误链中是否存在指定业务码。
func Is(err error, code int) bool {
	e, ok := As(err)
	return ok && e.Code == code
}
