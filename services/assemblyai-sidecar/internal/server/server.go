package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"
)

const requestIDHeader = "X-Request-ID"

type Handler struct {
	logger *slog.Logger
}

func NewHandler(logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	handler := &Handler{logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handler.root)
	mux.HandleFunc("GET /healthz", handler.health)
	mux.HandleFunc("GET /readyz", handler.ready)

	return handler.withRequestLogging(mux)
}

func (h *Handler) root(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "assemblyai-sidecar",
		"status":  "running",
	})
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "assemblyai-sidecar",
		"status":  "ok",
	})
}

func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	// Provider calls and shared-database checks will be added with the
	// authority boundary. The base service has no external dependencies.
	writeJSON(w, http.StatusOK, map[string]string{
		"service": "assemblyai-sidecar",
		"status":  "ready",
	})
}

func (h *Handler) withRequestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(requestIDHeader)
		if requestID == "" {
			requestID = newRequestID()
		}
		w.Header().Set(requestIDHeader, requestID)

		startedAt := time.Now()
		next.ServeHTTP(w, r)
		h.logger.Info(
			"http request completed",
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

func newRequestID() string {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "request-id-unavailable"
	}
	return hex.EncodeToString(bytes[:])
}
