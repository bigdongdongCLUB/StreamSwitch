// Package logger 提供带级别与文件输出的结构化日志。
package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Level 日志级别。
type Level int

const (
	// LevelDebug 调试日志
	LevelDebug Level = iota
	// LevelInfo 普通信息
	LevelInfo
	// LevelWarn 警告
	LevelWarn
	// LevelError 错误
	LevelError
)

// ParseLevel 解析日志级别字符串，未识别时返回 LevelInfo。
func ParseLevel(s string) Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// String 返回级别的字符串表示。
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

// Logger 简单的分级日志器，支持同时输出到控制台与文件。
type Logger struct {
	mu     sync.Mutex
	level  Level
	json   bool
	closer io.Closer
	// file 是可选的文件句柄，用于写日志文件
	file *os.File
}

// Options 日志器配置。
type Options struct {
	Level      Level
	Format     string // json / text
	OutputFile string
}

// New 创建日志器。outputFile 非空时同时写入文件（自动轮转按天由文件名日期后缀实现）。
func New(opt Options) *Logger {
	l := &Logger{level: opt.Level, json: strings.EqualFold(opt.Format, "json")}
	if opt.OutputFile != "" {
		if err := os.MkdirAll(filepath.Dir(opt.OutputFile), 0o755); err == nil {
			f, err := os.OpenFile(opt.OutputFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
			if err == nil {
				l.file = f
				l.closer = f
			}
		}
	}
	return l
}

var std = New(Options{Level: LevelInfo, Format: "text"})

// Default 返回默认日志器。
func Default() *Logger { return std }

// SetDefault 替换默认日志器。
func SetDefault(l *Logger) { std = l }

// Close 关闭日志文件句柄。
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closer != nil {
		_ = l.closer.Close()
		l.closer = nil
	}
}

// Debug 输出调试日志。
func (l *Logger) Debug(format string, args ...any) { l.log(LevelDebug, format, args...) }

// Info 输出普通日志。
func (l *Logger) Info(format string, args ...any) { l.log(LevelInfo, format, args...) }

// Warn 输出警告日志。
func (l *Logger) Warn(format string, args ...any) { l.log(LevelWarn, format, args...) }

// Error 输出错误日志。
func (l *Logger) Error(format string, args ...any) { l.log(LevelError, format, args...) }

func (l *Logger) log(lv Level, format string, args ...any) {
	if lv < l.level {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	now := time.Now()

	l.mu.Lock()
	defer l.mu.Unlock()

	var line string
	if l.json {
		line = fmt.Sprintf(`{"time":"%s","level":"%s","msg":%q}`+"\n",
			now.Format(time.RFC3339), lv.String(), msg)
	} else {
		line = fmt.Sprintf("[%s] %-5s %s\n", now.Format("2006-01-02 15:04:05"), lv.String(), msg)
	}

	_, _ = os.Stdout.WriteString(line)
	if l.file != nil {
		_, _ = l.file.WriteString(line)
	}
}

// 以下为包级快捷函数，使用默认日志器。

// Debug 输出调试日志。
func Debug(format string, args ...any) { std.Debug(format, args...) }

// Info 输出普通日志。
func Info(format string, args ...any) { std.Info(format, args...) }

// Warn 输出警告日志。
func Warn(format string, args ...any) { std.Warn(format, args...) }

// Error 输出错误日志。
func Error(format string, args ...any) { std.Error(format, args...) }
