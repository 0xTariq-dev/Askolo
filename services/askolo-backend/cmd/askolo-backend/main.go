package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/app"
	"askolo/backend/internal/config"
	"askolo/backend/internal/observability/datadogmetrics"
	"askolo/backend/internal/observability/datadogmfa"
)

func main() {
	logger := setupLogger()
	initSentry(logger)

	cfg := loadConfig(logger)
	ctx := setupSignalContext()

	store := initDatabase(ctx, cfg, logger)
	mfaObserver, shutdownDatadog := initMFAObserver(cfg, logger)
	requestMetrics, shutdownRequestMetrics := initRequestMetrics(cfg, store, logger)

	logger = slog.New(mfaObserver)
	slog.SetDefault(logger)
	warnIfDatadogDisabled(cfg, logger)

	backend := createBackend(backendParams{
	cfg:            cfg,
	logger:         logger,
	store:          store,
	mfaObserver:    mfaObserver,
	requestMetrics: requestMetrics,
})
	runErr := backend.Run(ctx)

	shutdown(shutdownDatadog, shutdownRequestMetrics, runErr, logger)
}

func setupLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

func initSentry(logger *slog.Logger) {
	err := sentry.Init(sentry.ClientOptions{
		Dsn:              "https://39e40f7b28335c4692d45b4538eed4e7@o4512485582766080.ingest.us.sentry.io/4512485663768576",
		TracesSampleRate: 1.0,
	})
	if err != nil {
		logger.Error("sentry.Init failed", "error", err)
		os.Exit(1)
	}
	// Flush is deferred in main to ensure it runs after backend.Run
}

func loadConfig(logger *slog.Logger) config.Config {
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid backend configuration", "error", err)
		os.Exit(1)
	}
	return cfg
}

func setupSignalContext() context.Context {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	// stop is called via defer in main
	_ = stop
	return ctx
}

func initDatabase(ctx context.Context, cfg config.Config, logger *slog.Logger) *postgres.Store {
	store, err := postgres.NewForEnvironment(ctx, cfg.DatabaseURL, cfg.Environment)
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	return store
}

func initMFAObserver(cfg config.Config, logger *slog.Logger) (*datadogmfa.Handler, func(context.Context) error) {
	handler, shutdown, err := datadogmfa.New(logger.Handler(), datadogmfa.Config{
		APIKey:      cfg.Datadog.APIKey,
		Site:        cfg.Datadog.Site,
		Environment: cfg.Environment,
	})
	if err != nil {
		logger.Error("Datadog MFA telemetry setup failed", "error_type", "invalid_configuration")
		os.Exit(1)
	}
	return handler, shutdown
}

func initRequestMetrics(cfg config.Config, store *postgres.Store, logger *slog.Logger) (*datadogmetrics.Handler, func(context.Context) error) {
	metrics, shutdown, err := datadogmetrics.New(datadogmetrics.Config{
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
	return metrics, shutdown
}

func warnIfDatadogDisabled(cfg config.Config, logger *slog.Logger) {
	if cfg.Datadog.APIKey == "" {
		logger.Warn("Datadog telemetry disabled",
			"environment", cfg.Environment,
			"operation", "application_observability",
			"reason", "missing_api_key",
		)
	}
}

type backendParams struct {
	cfg            config.Config
	logger         *slog.Logger
	store          *postgres.Store
	mfaObserver    *datadogmfa.Handler
	requestMetrics *datadogmetrics.Handler
}

func createBackend(p backendParams) *app.App {
	return app.New(p.cfg, p.logger, p.store, app.Observability{
		MFAObserver:         p.mfaObserver,
		HTTPRequestObserver: p.requestMetrics,
	})
}

func shutdown(shutdownDatadog, shutdownRequestMetrics func(context.Context) error, runErr error, logger *slog.Logger) {
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()

	if err := shutdownDatadog(shutdownContext); err != nil {
		logger.Warn("Datadog MFA telemetry shutdown did not drain",
			"environment", "unknown",
			"operation", "mfa_security_spike",
		)
	}
	if err := shutdownRequestMetrics(shutdownContext); err != nil {
		logger.Warn("Datadog HTTP metrics shutdown did not drain",
			"environment", "unknown",
			"operation", "http_request_metrics",
		)
	}
	if runErr != nil {
		logger.Error("backend stopped unexpectedly", "error", runErr)
		os.Exit(1)
	}
}
