package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
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

func New(cfg config.Config, logger *slog.Logger, store *postgres.Store) http.Handler {
	mux := http.NewServeMux()
	internalAuth := auth.NewInternalMiddleware(cfg.InternalAuthToken)
	authHandler := authmodule.NewHandler(cfg, store, logger)

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": cfg.ServiceName,
			"status":  "running",
			"mode":    "companion",
		})
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service":     cfg.ServiceName,
			"status":      "ok",
			"environment": cfg.Environment,
			"release":     cfg.ReleaseTag,
			"commit":      cfg.BuildCommit,
			"origin":      cfg.CanonicalOrigin,
		})
	})
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service":     cfg.ServiceName,
			"status":      "ok",
			"environment": cfg.Environment,
			"release":     cfg.ReleaseTag,
			"commit":      cfg.BuildCommit,
			"origin":      cfg.CanonicalOrigin,
		})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		databaseReachable := false
		authorizationStorageReady := false
		if store != nil {
			pingContext, cancel := context.WithTimeout(r.Context(), 750*time.Millisecond)
			databaseReachable = store.Ping(pingContext) == nil
			if databaseReachable {
				authorizationStorageReady = store.AuthorizationSchemaReady(pingContext)
			}
			cancel()
		}
		emailDeliveryReadiness := authHandler.EmailDeliveryReadiness()
		emailDeliveryConfigured := emailDeliveryReadiness.ResendConfiguration == "configured" &&
			emailDeliveryReadiness.ChallengeConfiguration == "configured"
		status, statusCode := dependencyReadinessStatus(
			databaseReachable,
			authorizationStorageReady,
			emailDeliveryConfigured,
		)
		writeJSON(w, statusCode, map[string]any{
			"environment":               cfg.Environment,
			"release":                   cfg.ReleaseTag,
			"commit":                    cfg.BuildCommit,
			"service":                   cfg.ServiceName,
			"status":                    status,
			"internalAuthConfigured":    cfg.InternalAuthToken != "",
			"databaseReachable":         databaseReachable,
			"authorizationStorageReady": authorizationStorageReady,
			"emailDeliveryConfigured":   emailDeliveryConfigured,
			"emailDelivery":             emailDeliveryReadiness,
		})
	})

	restHandler := rest.New(logger, cfg.ServiceName)
	mux.Handle("/internal/rest/", internalAuth.Wrap(http.StripPrefix("/internal/rest", restHandler)))

	authorizationHandler := authorizationmodule.NewHandler(store, logger, cfg.SessionCookieName)
	mux.Handle("/internal/authz/", internalAuth.Wrap(authorizationHandler.Routes()))

	websocketHandler := websocket.New(logger, cfg.ServiceName, store, cfg.SessionCookieName)
	mux.Handle("/internal/ws", internalAuth.Wrap(websocketHandler))
	mux.Handle("/ws", websocketHandler)

	webhookHandler := webhooks.New(logger, cfg.ServiceName)
	mux.Handle("/webhooks/", webhookHandler)
	apiMux := http.NewServeMux()
	apiMux.Handle("/api/auth/", authHandler.Routes())
	githubRoutes := githuboauth.NewHandler(cfg, store, logger).Routes()
	apiMux.Handle("/api/auth/github", githubRoutes)
	apiMux.Handle("/api/auth/github/", githubRoutes)
	googleRoutes := googleoauth.NewHandler(cfg, store, logger).Routes()
	productRoutes := productmodule.NewHandler(cfg, store, logger, cfg.SessionCookieName).Routes()
	apiMux.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			strings.HasPrefix(r.URL.Path, "/api/user/"):
			productRoutes.ServeHTTP(w, r)
		default:
			googleRoutes.ServeHTTP(w, r)
		}
	}))
	mux.Handle("/api/", apiMux)
	mux.HandleFunc("/", notFound)

	return httpx.Middleware(logger, mux)
}

func dependencyReadinessStatus(
	databaseReachable bool,
	authorizationStorageReady bool,
	emailDeliveryConfigured bool,
) (string, int) {
	// A fresh Autoscale instance has no in-memory delivery history yet, so
	// "unknown" is expected before the first real email attempt. Readiness
	// validates static email configuration here; provider outcomes remain
	// visible in the response without making cold starts depend on them.
	if !databaseReachable || !authorizationStorageReady || !emailDeliveryConfigured {
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
