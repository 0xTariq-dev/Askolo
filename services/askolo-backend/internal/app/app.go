package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
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
	server       *http.Server
	logger       *slog.Logger
	store        *postgres.Store
	cleanupState *emailChallengeCleanupState
	environment  string
	serviceName  string
}

func New(cfg config.Config, logger *slog.Logger, store *postgres.Store) *App {
	if logger == nil {
		logger = slog.Default()
	}

	cleanupState := &emailChallengeCleanupState{}
	return &App{
		logger:       logger,
		store:        store,
		cleanupState: cleanupState,
		environment:  cfg.Environment,
		serviceName:  cfg.ServiceName,
		server: &http.Server{
			Addr:              cfg.Host + ":" + strconv.Itoa(cfg.Port),
			Handler:           httpapi.New(cfg, logger, store, cleanupState.readiness),
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
	}
}

type emailChallengeCleanupState struct {
	mu                    sync.RWMutex
	consecutiveFailures   int
	lastSuccessfulCleanup time.Time
	alertActive           bool
}

func (s *emailChallengeCleanupState) recordFailure() (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consecutiveFailures++
	shouldAlert := s.consecutiveFailures >= httpapi.EmailChallengeCleanupPersistentFailureThreshold &&
		!s.alertActive
	if shouldAlert {
		s.alertActive = true
	}
	return s.consecutiveFailures, shouldAlert
}

func (s *emailChallengeCleanupState) recordSuccess(at time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	recovered := s.alertActive
	s.consecutiveFailures = 0
	s.lastSuccessfulCleanup = at.UTC()
	s.alertActive = false
	return recovered
}

func (s *emailChallengeCleanupState) readiness() httpapi.EmailChallengeCleanupReadiness {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := "unknown"
	switch {
	case s.consecutiveFailures >= httpapi.EmailChallengeCleanupPersistentFailureThreshold:
		status = "persistent_failure"
	case s.consecutiveFailures > 0:
		status = "transient_failure"
	case !s.lastSuccessfulCleanup.IsZero():
		status = "healthy"
	}

	var lastSuccessfulCleanupAt *time.Time
	if !s.lastSuccessfulCleanup.IsZero() {
		timestamp := s.lastSuccessfulCleanup
		lastSuccessfulCleanupAt = &timestamp
	}
	return httpapi.EmailChallengeCleanupReadiness{
		Status:                     status,
		ConsecutiveFailures:        s.consecutiveFailures,
		PersistentFailureThreshold: httpapi.EmailChallengeCleanupPersistentFailureThreshold,
		LastSuccessfulCleanupAt:    lastSuccessfulCleanupAt,
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
				consecutiveFailures, shouldAlert := a.cleanupState.recordFailure()
				a.logger.Warn("email challenge cleanup failed",
					"operation", "email_challenge_cleanup",
					"batch_size", postgres.EmailChallengeCleanupBatchSize,
					"retention_hours", int(postgres.EmailChallengeRetention/time.Hour),
					"consecutive_failures", consecutiveFailures,
				)
				if shouldAlert {
					a.logCleanupFailureAlert(consecutiveFailures)
				}
			}
			return
		}
		recovered := a.cleanupState.recordSuccess(time.Now().UTC())
		if recovered {
			a.logCleanupRecovery()
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

func (a *App) logCleanupFailureAlert(consecutiveFailures int) {
	a.logger.Warn("Email challenge cleanup failure alert",
		"alert", true,
		"environment", a.environment,
		"service", a.serviceName,
		"operation", "email_challenge_cleanup",
		"status", "persistent_failure",
		"consecutive_failures", consecutiveFailures,
		"failure_threshold", httpapi.EmailChallengeCleanupPersistentFailureThreshold,
	)
}

func (a *App) logCleanupRecovery() {
	a.logger.Info("Email challenge cleanup recovered",
		"alert", false,
		"recovery", true,
		"environment", a.environment,
		"service", a.serviceName,
		"operation", "email_challenge_cleanup",
		"status", "healthy",
		"failure_threshold", httpapi.EmailChallengeCleanupPersistentFailureThreshold,
	)
}
