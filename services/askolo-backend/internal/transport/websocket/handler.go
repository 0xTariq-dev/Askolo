package websocket

import (
	"log/slog"
	"net/http"

	"askolo/backend/internal/platform/apierror"
)

const maxSessionDurationSeconds = 120

type Handler struct {
	logger     *slog.Logger
	serviceName string
}

func New(logger *slog.Logger, serviceName string) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{logger: logger, serviceName: serviceName}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apierror.Write(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed.")
		return
	}

	// The transport boundary is intentionally present before a WebSocket
	// implementation is selected. The eventual adapter must enforce origin,
	// authentication, frame limits, heartbeats, session duration, and close
	// semantics before handing messages to an application module.
	apierror.Write(w, r, http.StatusNotImplemented, "WEBSOCKET_NOT_CONFIGURED", "WebSocket transport is not configured.")
}