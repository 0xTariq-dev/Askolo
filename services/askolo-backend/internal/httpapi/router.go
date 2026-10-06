package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	authmodule "askolo/backend/internal/modules/auth"
	authorizationmodule "askolo/backend/internal/modules/authorization"
	githuboauth "askolo/backend/internal/modules/github"
	googleoauth "askolo/backend/internal/modules/google"
	productmodule "askolo/backend/internal/modules/product"
	"askolo/backend/internal/platform/apierror"
	"askolo/backend/internal/platform/auth"
	"askolo/backend/internal/platform/httpx"
	"askolo/backend/internal/transport/rest"
	"askolo/backend/internal/transport/webhooks"
	"askolo/backend/internal/transport/websocket"
)

const EmailChallengeCleanupPersistentFailureThreshold = 3

const (
	schemaReadinessModeMigrationLedger = "go_migration_ledger"
	schemaReadinessModePublishCheck    = "replit_publish_compatibility"
	readinessProbeTimeout              = 2 * time.Second
	schemaReadinessProbeTimeout        = 4 * time.Second
)

func schemaReadinessModeForEnvironment(environment string) string {
	if strings.EqualFold(strings.TrimSpace(environment), "production") {
		return schemaReadinessModePublishCheck
	}
	return schemaReadinessModeMigrationLedger
}

type EmailChallengeCleanupReadiness struct {
	Status                       string     `json:"status"`
	ConsecutiveFailures          int        `json:"consecutiveFailures"`
	PersistentFailureThreshold   int        `json:"persistentFailureThreshold"`
	LastSuccessfulCleanupAt      *time.Time `json:"lastSuccessfulCleanupAt,omitempty"`
	PersistentFailureOccurrences int        `json:"persistentFailureOccurrences"`
	RecoveryEvents               int        `json:"recoveryEvents"`
}

type EmailChallengeCleanupReadinessProvider func() EmailChallengeCleanupReadiness

type migrationReadinessObserver struct {
	mu          sync.Mutex
	initialized bool
	ready       bool
	reason      string
	checkedAt   time.Time
	cachedReady bool
	cachedErr   error
	inFlight    chan struct{}
}

func (o *migrationReadinessObserver) check(
	ctx context.Context,
	probe func(context.Context) (bool, error),
) (bool, error) {
	o.mu.Lock()
	if !o.checkedAt.IsZero() && time.Since(o.checkedAt) < 500*time.Millisecond {
		ready, err := o.cachedReady, o.cachedErr
		o.mu.Unlock()
		return ready, err
	}
	if inFlight := o.inFlight; inFlight != nil {
		o.mu.Unlock()
		select {
		case <-inFlight:
			o.mu.Lock()
			ready, err := o.cachedReady, o.cachedErr
			o.mu.Unlock()
			return ready, err
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}
	inFlight := make(chan struct{})
	o.inFlight = inFlight
	o.mu.Unlock()

	ready, err := probe(ctx)

	o.mu.Lock()
	o.cachedReady = ready
	o.cachedErr = err
	o.checkedAt = time.Now()
	o.inFlight = nil
	close(inFlight)
	o.mu.Unlock()
	return ready, err
}

func (o *migrationReadinessObserver) observe(
	logger *slog.Logger,
	cfg config.Config,
	mode string,
	ready bool,
	checkErr error,
) {
	reason := ""
	if checkErr != nil {
		reason = checkErr.Error()
	} else if !ready {
		reason = "database schema is not ready"
	}

	o.mu.Lock()
	changed := !o.initialized || o.ready != ready || o.reason != reason
	if changed {
		o.initialized = true
		o.ready = ready
		o.reason = reason
	}
	o.mu.Unlock()
	if !changed || logger == nil {
		return
	}
	if ready {
		logger.Info("database schema readiness check passed",
			"environment", cfg.Environment,
			"schema_readiness_mode", mode,
			"service", cfg.ServiceName,
			"operation", "schema_readiness",
			"status", "ready",
		)
		return
	}
	logger.Warn("database schema readiness check blocked",
		"environment", cfg.Environment,
		"schema_readiness_mode", mode,
		"service", cfg.ServiceName,
		"operation", "schema_readiness",
		"status", "not_ready",
		"reason", reason,
	)
}

func checkMigrationSchemaReadiness(
	ctx context.Context,
	observer *migrationReadinessObserver,
	probe func(context.Context) (bool, error),
) (bool, error) {
	probeContext, cancel := context.WithTimeout(ctx, schemaReadinessProbeTimeout)
	defer cancel()
	return observer.check(probeContext, probe)
}

type EmailChallengeCleanupDashboard struct {
	Environment                  string                         `json:"environment"`
	Service                      string                         `json:"service"`
	Operation                    string                         `json:"operation"`
	PersistentFailureOccurrences int                            `json:"persistentFailureOccurrences"`
	RecoveryEvents               int                            `json:"recoveryEvents"`
	Readiness                    EmailChallengeCleanupReadiness `json:"readiness"`
}

func New(
	cfg config.Config,
	logger *slog.Logger,
	store *postgres.Store,
	cleanupReadinessProviders ...EmailChallengeCleanupReadinessProvider,
) http.Handler {
	handler, _ := NewWithShutdown(cfg, logger, store, cleanupReadinessProviders...)
	return handler
}

func NewWithShutdown(
	cfg config.Config,
	logger *slog.Logger,
	store *postgres.Store,
	cleanupReadinessProviders ...EmailChallengeCleanupReadinessProvider,
) (http.Handler, func(context.Context) error) {
	mux := http.NewServeMux()
	internalAuth := auth.NewInternalMiddleware(cfg.InternalAuthToken)
	authHandler := authmodule.NewHandler(cfg, store, logger)
	migrationReadinessObserver := &migrationReadinessObserver{}
	cleanupReadiness := func() EmailChallengeCleanupReadiness {
		return EmailChallengeCleanupReadiness{
			Status:                     "unknown",
			PersistentFailureThreshold: EmailChallengeCleanupPersistentFailureThreshold,
		}
	}
	cleanupDashboard := func() EmailChallengeCleanupDashboard {
		readiness := cleanupReadiness()
		return EmailChallengeCleanupDashboard{
			Environment:                  cfg.Environment,
			Service:                      cfg.ServiceName,
			Operation:                    "email_challenge_cleanup",
			PersistentFailureOccurrences: readiness.PersistentFailureOccurrences,
			RecoveryEvents:               readiness.RecoveryEvents,
			Readiness:                    readiness,
		}
	}
	if len(cleanupReadinessProviders) > 0 && cleanupReadinessProviders[0] != nil {
		cleanupReadiness = cleanupReadinessProviders[0]
	}

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": cfg.ServiceName,
			"status":  "running",
			"mode":    "companion",
		})
	})
	mux.HandleFunc("GET "+PublishedHealthzPath, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service":     cfg.ServiceName,
			"status":      "ok",
			"environment": cfg.Environment,
			"release":     cfg.ReleaseTag,
			"commit":      cfg.BuildCommit,
			"origin":      cfg.CanonicalOrigin,
		})
	})
	mux.HandleFunc("GET "+PublishedAPIPath+"/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service":     cfg.ServiceName,
			"status":      "ok",
			"environment": cfg.Environment,
			"release":     cfg.ReleaseTag,
			"commit":      cfg.BuildCommit,
			"origin":      cfg.CanonicalOrigin,
		})
	})
	mux.HandleFunc("GET "+PublishedReadyzPath, func(w http.ResponseWriter, r *http.Request) {
		databaseReachable := false
		authorizationStorageReady := false
		migrationSchemaReady := false
		migrationLedgerReady := false
		managedProductionSchemaCompatible := false
		schemaReadinessMode := schemaReadinessModeForEnvironment(cfg.Environment)
		schemaReadinessErr := errors.New("database is not configured")
		mfaSecurityReadiness := authmodule.MFASecurityReadiness{
			Status:        "unavailable",
			Environment:   cfg.Environment,
			WindowMinutes: 15,
		}
		if store != nil {
			pingContext, cancelPing := context.WithTimeout(r.Context(), readinessProbeTimeout)
			databaseReachable = store.Ping(pingContext) == nil
			cancelPing()
			if databaseReachable {
				authorizationContext, cancelAuthorization := context.WithTimeout(
					r.Context(),
					readinessProbeTimeout,
				)
				authorizationStorageReady = store.AuthorizationSchemaReady(authorizationContext)
				cancelAuthorization()

				schemaProbe := store.MigrationSchemaReady
				if schemaReadinessMode == schemaReadinessModePublishCheck {
					schemaProbe = store.ProductionSchemaCompatible
				}
				migrationSchemaReady, schemaReadinessErr = checkMigrationSchemaReadiness(
					r.Context(),
					migrationReadinessObserver,
					schemaProbe,
				)
				if schemaReadinessMode == schemaReadinessModePublishCheck {
					managedProductionSchemaCompatible = migrationSchemaReady
				} else {
					migrationLedgerReady = migrationSchemaReady
				}

				mfaContext, cancelMFA := context.WithTimeout(r.Context(), readinessProbeTimeout)
				mfaSecurityReadiness = authHandler.MFASecurityReadiness(mfaContext)
				cancelMFA()
			} else {
				schemaReadinessErr = errors.New("database is not reachable")
			}
		}
		migrationReadinessObserver.observe(
			logger,
			cfg,
			schemaReadinessMode,
			migrationSchemaReady,
			schemaReadinessErr,
		)
		emailDeliveryReadiness := authHandler.EmailDeliveryReadiness()
		emailDeliveryConfigured := emailDeliveryReadiness.ResendConfiguration == "configured" &&
			emailDeliveryReadiness.ChallengeConfiguration == "configured"
		emailChallengeCleanupReadiness := cleanupReadiness()
		status, statusCode := dependencyReadinessStatus(
			databaseReachable,
			authorizationStorageReady,
			migrationSchemaReady,
			emailDeliveryConfigured,
			emailChallengeCleanupReadiness,
		)
		writeJSON(w, statusCode, map[string]any{
			"environment":                       cfg.Environment,
			"release":                           cfg.ReleaseTag,
			"commit":                            cfg.BuildCommit,
			"service":                           cfg.ServiceName,
			"status":                            status,
			"internalAuthConfigured":            cfg.InternalAuthToken != "",
			"databaseReachable":                 databaseReachable,
			"authorizationStorageReady":         authorizationStorageReady,
			"schemaReadinessMode":               schemaReadinessMode,
			"migrationLedgerReady":              migrationLedgerReady,
			"managedProductionSchemaCompatible": managedProductionSchemaCompatible,
			// Kept as the selected schema gate for compatibility with existing probes.
			"migrationSchemaReady":    migrationSchemaReady,
			"emailDeliveryConfigured": emailDeliveryConfigured,
			"emailDelivery":           emailDeliveryReadiness,
			"emailChallengeCleanup":   emailChallengeCleanupReadiness,
			"mfaSecurity":             mfaSecurityReadiness,
		})
	})

	cleanupDashboardHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, cleanupDashboard())
	})
	mux.Handle("/internal/monitoring/cleanup", internalAuth.Wrap(cleanupDashboardHandler))

	restHandler := rest.New(logger, cfg.ServiceName)
	mux.Handle("/internal/rest/", internalAuth.Wrap(http.StripPrefix("/internal/rest", restHandler)))

	authorizationHandler := authorizationmodule.NewHandler(store, logger, cfg.SessionCookieName)
	mux.Handle("/internal/authz/", internalAuth.Wrap(authorizationHandler.Routes()))

	googleHandler := googleoauth.NewHandler(cfg, store, logger)
	productHandler := productmodule.NewHandler(cfg, store, logger, cfg.SessionCookieName)
	productHandler.SetAssistantActionExecutor(googleHandler)
	websocketHandler := websocket.New(
		logger, cfg.ServiceName, store, cfg.SessionCookieName,
		cfg.CanonicalOrigin, cfg.AuthRateLimitHMACSecret, productHandler,
	)
	mux.Handle("/internal/ws", internalAuth.Wrap(websocketHandler))
	mux.Handle(PublishedWebsocketPath, websocketHandler)

	webhookHandler := webhooks.New(logger, cfg.ServiceName)
	mux.Handle(PublishedWebhooksPath+"/", webhookHandler)
	apiMux := http.NewServeMux()
	githubRoutes := githuboauth.NewHandler(cfg, store, logger).Routes()
	googleRoutes := googleHandler.Routes()
	// Google login owns these public paths. Register them before the generic
	// auth subtree so the native OAuth start and callback handlers receive the
	// frontend's requests instead of the password-auth mux returning 404.
	apiMux.Handle("/api/auth/google", googleRoutes)
	apiMux.Handle("/api/auth/google/", googleRoutes)
	apiMux.Handle("/api/auth/", authHandler.Routes())
	apiMux.Handle("/api/auth/github", githubRoutes)
	apiMux.Handle("/api/auth/github/", githubRoutes)
	productRoutes := productHandler.Routes()
	apiMux.Handle(PublishedAPIPath+"/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/habits"),
			strings.HasPrefix(r.URL.Path, "/api/goals"),
			strings.HasPrefix(r.URL.Path, "/api/daily-plans"),
			strings.HasPrefix(r.URL.Path, "/api/events"),
			strings.HasPrefix(r.URL.Path, "/api/chores"),
			strings.HasPrefix(r.URL.Path, "/api/notes"),
			strings.HasPrefix(r.URL.Path, "/api/action-items"),
			strings.HasPrefix(r.URL.Path, "/api/dashboard"),
			strings.HasPrefix(r.URL.Path, "/api/ai/"),
			strings.HasPrefix(r.URL.Path, "/api/admin/ai-credit-"),
			strings.HasPrefix(r.URL.Path, "/api/user/"):
			productRoutes.ServeHTTP(w, r)
		default:
			googleRoutes.ServeHTTP(w, r)
		}
	}))
	mux.Handle(PublishedAPIPath+"/", apiMux)
	mux.HandleFunc("/", notFound)

	return httpx.Middleware(logger, mux), productHandler.ShutdownRealtimeSessions
}

func dependencyReadinessStatus(
	databaseReachable bool,
	authorizationStorageReady bool,
	schemaReady bool,
	emailDeliveryConfigured bool,
	cleanupReadiness ...EmailChallengeCleanupReadiness,
) (string, int) {
	// A fresh Autoscale instance has no in-memory delivery history yet, so
	// "unknown" is expected before the first real email attempt. Readiness
	// validates static email configuration here; provider outcomes remain
	// visible in the response without making cold starts depend on them.
	persistentCleanupFailure := len(cleanupReadiness) > 0 &&
		cleanupReadiness[0].Status == "persistent_failure"
	if !databaseReachable || !authorizationStorageReady || !schemaReady || !emailDeliveryConfigured || persistentCleanupFailure {
		return "degraded", http.StatusServiceUnavailable
	}
	return "ready", http.StatusOK
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

func notFound(w http.ResponseWriter, r *http.Request) {
	apierror.Write(w, r, http.StatusNotFound, "NOT_FOUND", "Route not found.")
}
