package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	"askolo/backend/internal/httpapi"
)

const (
	shutdownTimeout                 = 10 * time.Second
	readHeaderTimeout               = 5 * time.Second
	readTimeout                     = 15 * time.Second
	writeTimeout                    = 30 * time.Second
	idleTimeout                     = 60 * time.Second
	emailChallengeCleanupInterval   = 15 * time.Minute
	emailChallengeCleanupQueryLimit = 2 * time.Second
)

type App struct {
	server *http.Server
	logger *slog.Logger
	store  *postgres.Store
}

func New(cfg config.Config, logger *slog.Logger, store *postgres.Store) *App {
	if logger == nil {
		logger = slog.Default()
	}

	return &App{
		logger: logger,
		store:  store,
		server: &http.Server{
			Addr:              cfg.Host + ":" + strconv.Itoa(cfg.Port),
			Handler:           httpapi.New(cfg, logger, store),
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
	}
}

func (a *App) Run(ctx context.Context) error {
	serverErrors := make(chan error, 1)
	cleanupContext, cancelCleanup := context.WithCancel(ctx)
	cleanupDone := make(chan struct{})
	go a.runEmailChallengeCleanup(cleanupContext, cleanupDone)
	defer func() {
		cancelCleanup()
		<-cleanupDone
	}()

	go func() {
		a.logger.Info("askolo backend listening", "addr", a.server.Addr)
		serverErrors <- a.server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		cancelCleanup()
		<-cleanupDone
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := a.server.Shutdown(shutdownContext); err != nil {
			return err
		}
		a.store.Close()
		return nil
	}
}

func (a *App) runEmailChallengeCleanup(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	if a.store == nil {
		return
	}

	cleanup := func() {
		startedAt := time.Now()
		queryContext, cancel := context.WithTimeout(ctx, emailChallengeCleanupQueryLimit)
		deleted, err := a.store.CleanupEmailChallenges(
			queryContext,
			time.Now().UTC(),
			postgres.EmailChallengeCleanupBatchSize,
		)
		cancel()
		if err != nil {
			if ctx.Err() == nil {
				a.logger.Warn("email challenge cleanup failed",
					"operation", "email_challenge_cleanup",
					"error", err,
					"batch_size", postgres.EmailChallengeCleanupBatchSize,
					"retention_hours", int(postgres.EmailChallengeRetention/time.Hour),
				)
			}
			return
		}
		a.logger.Info("email challenge cleanup completed",
			"operation", "email_challenge_cleanup",
			"deleted_count", deleted,
			"batch_size", postgres.EmailChallengeCleanupBatchSize,
			"retention_hours", int(postgres.EmailChallengeRetention/time.Hour),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	}

	cleanup()
	ticker := time.NewTicker(emailChallengeCleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}
