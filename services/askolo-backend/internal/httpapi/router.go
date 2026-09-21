package httpapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	authmodule "askolo/backend/internal/modules/auth"
	githuboauth "askolo/backend/internal/modules/github"
	googleoauth "askolo/backend/internal/modules/google"
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

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": cfg.ServiceName,
			"status":  "running",
			"mode":    "companion",
		})
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service": cfg.ServiceName,
			"status":  "ok",
		})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		databaseReachable := false
		if store != nil {
			pingContext, cancel := context.WithTimeout(r.Context(), 750*time.Millisecond)
			databaseReachable = store.Ping(pingContext) == nil
			cancel()
		}
		emailDeliveryConfigured := cfg.Email.SMTPHost != "" &&
			cfg.Email.FromAddress != "" &&
			cfg.Email.ChallengeSecret != ""
		status := "ready"
		statusCode := http.StatusOK
		if !databaseReachable {
			status = "degraded"
			statusCode = http.StatusServiceUnavailable
		}
		writeJSON(w, statusCode, map[string]any{
			"service":                 cfg.ServiceName,
			"status":                  status,
			"internalAuthConfigured":  cfg.InternalAuthToken != "",
			"databaseReachable":       databaseReachable,
			"emailDeliveryConfigured": emailDeliveryConfigured,
		})
	})

	restHandler := rest.New(logger, cfg.ServiceName)
	mux.Handle("/internal/rest/", internalAuth.Wrap(http.StripPrefix("/internal/rest", restHandler)))

	websocketHandler := websocket.New(logger, cfg.ServiceName)
	mux.Handle("/internal/ws", internalAuth.Wrap(websocketHandler))
	mux.Handle("/ws", websocketHandler)

	webhookHandler := webhooks.New(logger, cfg.ServiceName)
	mux.Handle("/webhooks/", webhookHandler)
	apiMux := http.NewServeMux()
	apiMux.Handle("/api/auth/", authmodule.NewHandler(cfg, store, logger).Routes())
	githubRoutes := githuboauth.NewHandler(cfg, store, logger).Routes()
	apiMux.Handle("/api/auth/github", githubRoutes)
	apiMux.Handle("/api/auth/github/", githubRoutes)
	apiMux.Handle("/api/", googleoauth.NewHandler(cfg, store, logger).Routes())
	mux.Handle("/api/", apiMux)
	mux.HandleFunc("/", notFound)

	return httpx.Middleware(logger, mux)
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
