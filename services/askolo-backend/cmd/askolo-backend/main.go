package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"askolo/backend/internal/app"
	"askolo/backend/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid backend configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	backend := app.New(cfg, logger)
	if err := backend.Run(ctx); err != nil {
		logger.Error("backend stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}