// Command api 启动公开 HTTP API 并管理其进程生命周期。
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"exchange-lab/internal/app"
	"exchange-lab/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.LoadOS()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, cfg, logger); err != nil {
		logger.Error("api stopped", "error", err)
		os.Exit(1)
	}
}
