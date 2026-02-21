package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"dashi/internal/app"
	"dashi/internal/config"
)

func main() {
	cfg := config.Load()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	logger.Info("starting dashi", "addr", cfg.Addr, "db", cfg.DBPath)

	a, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("init failed", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := a.Run(ctx); err != nil {
		logger.Error("shutdown with error", "err", err)
		os.Exit(1)
	}
}
