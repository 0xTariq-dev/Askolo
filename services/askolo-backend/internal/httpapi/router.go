package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"askolo/backend/internal/config"
	"askolo/backend/internal/platform/apierror"
	"askolo/backend/internal/platform/auth"
	"askolo/backend/internal/platform/httpx"
	"askolo/backend/internal/transport/rest"
	"askolo/backend/internal/transport/webhooks"
	"askolo/backend/internal/transport/websocket"
)

func New(cfg config.Config, logger *slog.Logger) http.Handler {
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
		writeJSON(w, http.StatusOK, map[string]any{
			"service":                cfg.ServiceName,
			"status":                 "ready",
			"internalAuthConfigured": cfg.InternalAuthToken != "",
		})
	})

	restHandler := rest.New(logger, cfg.ServiceName)
	mux.Handle("/internal/rest/", internalAuth.Wrap(http.StripPrefix("/internal/rest", restHandler)))

	websocketHandler := websocket.New(logger, cfg.ServiceName)
	mux.Handle("/internal/ws", internalAuth.Wrap(websocketHandler))
	mux.Handle("/ws", websocketHandler)

	webhookHandler := webhooks.New(logger, cfg.ServiceName)
	mux.Handle("/webhooks/", webhookHandler)
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
