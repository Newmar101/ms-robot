package logutil

import (
	"log"
	"os"
	"strings"
)

// 日志级别：数值越小越详细。CurrentLevel 为最低输出级别，>= 该级别的日志会输出。
const (
	LevelDebug = iota // 0：详细调试（设备输出、流程等）
	LevelInfo         // 1：一般信息（监听、连接、启动等）
	LevelWarn         // 2：警告
	LevelError        // 3：仅错误
)

// CurrentLevel 当前最低输出级别，由环境变量 LOG_LEVEL 设置（debug|info|warn|error），默认 info
var CurrentLevel int

func init() {
	CurrentLevel = LevelInfo
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		CurrentLevel = LevelDebug
	case "warn", "warning":
		CurrentLevel = LevelWarn
	case "error":
		CurrentLevel = LevelError
	default:
		CurrentLevel = LevelInfo
	}
}

// DebugEnabled 当前是否允许 debug 级别日志（LOG_LEVEL=debug 时 true）。昂贵日志可先判此再调 Debugf 避免构造参数。
func DebugEnabled() bool {
	return CurrentLevel == LevelDebug
}

// Debugf 仅当级别为 debug 时输出（流程、设备 stdout/stderr 等）
func Debugf(format string, args ...interface{}) {
	if LevelDebug < CurrentLevel {
		return
	}
	log.Printf(format, args...)
}

// DebugLazy 仅当 debug 开启时才执行 fn 并输出，避免在关闭 debug 时做昂贵字符串构造。用法：DebugLazy(func() string { return fmt.Sprintf(...) })
func DebugLazy(fn func() string) {
	if LevelDebug < CurrentLevel {
		return
	}
	log.Print(fn())
}

// Infof 当级别为 debug 或 info 时输出（监听、连接、启动等）
func Infof(format string, args ...interface{}) {
	if LevelInfo < CurrentLevel {
		return
	}
	log.Printf(format, args...)
}

// Warnf 当级别为 debug / info / warn 时输出
func Warnf(format string, args ...interface{}) {
	if LevelWarn < CurrentLevel {
		return
	}
	log.Printf(format, args...)
}

// Errorf 当级别为 debug / info / warn / error 时输出（即始终输出，除非将来加更高档）
func Errorf(format string, args ...interface{}) {
	if LevelError < CurrentLevel {
		return
	}
	log.Printf(format, args...)
}
