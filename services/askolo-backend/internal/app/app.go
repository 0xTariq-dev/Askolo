package app

import (
	"context"
	"errors"
	"fmt"
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
	server                    *http.Server
	logger                    *slog.Logger
	store                     *postgres.Store
	cleanupState              *emailChallengeCleanupState
	configuredCleanupInterval time.Duration
	environment               string
	serviceName               string
	shutdownRealtimeSessions  func(context.Context) error
}

func New(cfg config.Config, logger *slog.Logger, store *postgres.Store) *App {
	if logger == nil {
		logger = slog.Default()
	}

	cleanupState := &emailChallengeCleanupState{}
	handler, shutdownRealtimeSessions := httpapi.NewWithShutdown(cfg, logger, store, cleanupState.readiness)
	return &App{
		logger:                    logger,
		store:                     store,
		cleanupState:              cleanupState,
		configuredCleanupInterval: cfg.EmailChallengeCleanupInterval,
		environment:               cfg.Environment,
		serviceName:               cfg.ServiceName,
		shutdownRealtimeSessions:  shutdownRealtimeSessions,
		server: &http.Server{
			Addr:              cfg.Host + ":" + strconv.Itoa(cfg.Port),
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
		},
	}
}

type emailChallengeCleanupState struct {
	mu                           sync.RWMutex
	consecutiveFailures          int
	lastSuccessfulCleanup        time.Time
	persistentFailureOccurrences int
	recoveryEvents               int
	alertActive                  bool
}

func (s *emailChallengeCleanupState) recordFailure() (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consecutiveFailures++
	shouldAlert := s.consecutiveFailures >= httpapi.EmailChallengeCleanupPersistentFailureThreshold &&
		!s.alertActive
	if shouldAlert {
		s.alertActive = true
		s.persistentFailureOccurrences++
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
	if recovered {
		s.recoveryEvents++
	}
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
		Status:                       status,
		ConsecutiveFailures:          s.consecutiveFailures,
		PersistentFailureThreshold:   httpapi.EmailChallengeCleanupPersistentFailureThreshold,
		LastSuccessfulCleanupAt:      lastSuccessfulCleanupAt,
		PersistentFailureOccurrences: s.persistentFailureOccurrences,
		RecoveryEvents:               s.recoveryEvents,
	}
}

func (a *App) Run(ctx context.Context) error {
	if a.store != nil {
		defer a.store.Close()
	}
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
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		shutdownErr := a.shutdown(shutdownContext)
		if errors.Is(err, http.ErrServerClosed) {
			return shutdownErr
		}
		return errors.Join(err, shutdownErr)
	case <-ctx.Done():
		cancelCleanup()
		<-cleanupDone
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return a.shutdown(shutdownContext)
	}
}

func (a *App) shutdown(ctx context.Context) error {
	var shutdownErr error
	if a.shutdownRealtimeSessions != nil {
		if err := a.shutdownRealtimeSessions(ctx); err != nil {
			a.logger.Error("realtime session shutdown did not finish cleanly", "error_type", fmt.Sprintf("%T", err))
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	if a.server != nil {
		if err := a.server.Shutdown(ctx); err != nil {
			_ = a.server.Close()
			shutdownErr = errors.Join(shutdownErr, err)
		}
	}
	return shutdownErr
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
	interval := a.cleanupInterval()
	ticker := time.NewTicker(interval)
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

func (a *App) cleanupInterval() time.Duration {
	if a != nil && a.configuredCleanupInterval > 0 {
		return a.configuredCleanupInterval
	}
	return emailChallengeCleanupInterval
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
