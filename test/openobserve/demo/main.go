package main

import (
	"os"

	"github.com/icymoss/k8s-deploy-config/test/pkg/logger"
	"github.com/rs/zerolog/log"
)

func main() {
	// 只有这一行配置
	logger.Setup(&logger.Config{
		Env:         os.Getenv("APP_ENV"), // 默认空字符串通常会被视为 local 逻辑(需在Setup微调)或者显式传 local
		ServiceName: "my-go-app",
		LogLevel:    "info",
	})

	// 业务代码完全解耦，根本不知道 OpenObserve 的存在
	log.Info().Msg("Application started")
	log.Warn().Int("latency", 500).Msg("Database slow")
	log.Error().Err(os.ErrNotExist).Msg("File not found")
}
