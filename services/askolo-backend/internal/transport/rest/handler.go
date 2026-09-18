package rest

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"askolo/backend/internal/platform/apierror"
)

func New(logger *slog.Logger, serviceName string) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"service":   serviceName,
			"transport": "rest",
			"status":    "available",
		})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		apierror.Write(w, r, http.StatusNotFound, "REST_ROUTE_NOT_FOUND", "REST route not found.")
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("failed to write REST response", "error", err)
	}
}