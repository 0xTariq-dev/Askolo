package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/app"
	"askolo/backend/internal/config"
	"askolo/backend/internal/observability/datadogmetrics"
	"askolo/backend/internal/observability/datadogmfa"
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

	store, err := postgres.NewForEnvironment(ctx, cfg.DatabaseURL, cfg.Environment)
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	datadogHandler, shutdownDatadog, err := datadogmfa.New(logger.Handler(), datadogmfa.Config{
		APIKey:      cfg.Datadog.APIKey,
		Site:        cfg.Datadog.Site,
		Environment: cfg.Environment,
	})
	if err != nil {
		logger.Error("Datadog MFA telemetry setup failed", "error_type", "invalid_configuration")
		os.Exit(1)
	}
	requestMetrics, shutdownRequestMetrics, err := datadogmetrics.New(datadogmetrics.Config{
		APIKey:      cfg.Datadog.APIKey,
		Site:        cfg.Datadog.Site,
		Environment: cfg.Environment,
		Service:     cfg.ServiceName,
		ReadDatabasePoolStats: func() datadogmetrics.DatabasePoolSnapshot {
			stats := store.PoolStats()
			return datadogmetrics.DatabasePoolSnapshot{
				TotalConnections:    float64(stats.TotalConnections),
				AcquiredConnections: float64(stats.AcquiredConnections),
				IdleConnections:     float64(stats.IdleConnections),
				MaxConnections:      float64(stats.MaxConnections),
				AcquireCount:        float64(stats.AcquireCount),
				AcquireDurationMS:   float64(stats.AcquireDuration) / float64(time.Millisecond),
			}
		},
	}, logger)
	if err != nil {
		logger.Error("Datadog HTTP metrics setup failed", "error_type", "invalid_configuration")
		os.Exit(1)
	}
	logger = slog.New(datadogHandler)
	slog.SetDefault(logger)
	if cfg.Datadog.APIKey == "" {
		logger.Warn("Datadog telemetry disabled",
			"environment", cfg.Environment,
			"operation", "application_observability",
			"reason", "missing_api_key",
		)
	}

	backend := app.New(cfg, logger, store, app.Observability{
		MFAObserver:         datadogHandler,
		HTTPRequestObserver: requestMetrics,
	})
	runErr := backend.Run(ctx)
	if runErr != nil {
		logger.Error("backend stopped unexpectedly", "error", runErr)
	}
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := shutdownDatadog(shutdownContext); err != nil {
		logger.Warn("Datadog MFA telemetry shutdown did not drain",
			"environment", cfg.Environment,
			"operation", "mfa_security_spike",
		)
	}
	if err := shutdownRequestMetrics(shutdownContext); err != nil {
		logger.Warn("Datadog HTTP metrics shutdown did not drain",
			"environment", cfg.Environment,
			"operation", "http_request_metrics",
		)
	}
	if runErr != nil {
		os.Exit(1)
	}
}
