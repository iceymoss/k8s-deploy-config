package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/rs/zerolog/pkgerrors"
)

type Config struct {
	Env         string // "local" or "prod"
	ServiceName string // e.g. "payment-service"
	LogLevel    string // "debug", "info", "error"
}

// Setup 初始化日志系统
func Setup(cfg *Config) {
	// 1. 基础设置
	zerolog.TimeFieldFormat = time.RFC3339
	zerolog.ErrorStackMarshaler = pkgerrors.MarshalStack

	// 2. 解析日志级别
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// 3. 关键分支：决定输出格式
	var output io.Writer

	if cfg.Env == "local" {
		// === 本地开发模式 ===
		// 你的需求：本地只需要正常输出到控制台
		// 使用 ConsoleWriter，带颜色，人性化
		output = zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05",
		}
	} else {
		// === 线上生产模式 (ELK/OpenObserve 模式) ===
		// 1. 纯 JSON: 方便 Vector/FluentBit 解析
		// 2. Stdout: 写入标准输出，这是 K8s 日志的标准源
		// 3. 高性能: 无锁，无 HTTP 请求，不阻塞业务
		output = os.Stdout
	}

	// 4. 构建 Logger
	// 自动注入 ServiceName, Environment, Caller(代码行号)
	log.Logger = zerolog.New(output).
		Level(level).
		With().
		Timestamp().
		Caller(). // 生产环境定位 bug 神器
		Str("service", cfg.ServiceName).
		Str("env", cfg.Env).
		Logger()
}
