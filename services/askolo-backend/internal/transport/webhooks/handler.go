package webhooks

import (
	"log/slog"
	"net/http"
	"strings"

	"askolo/backend/internal/platform/apierror"
)

const maxWebhookBodyBytes = 1 << 20

type Handler struct {
	logger      *slog.Logger
	serviceName string
}

func New(logger *slog.Logger, serviceName string) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{logger: logger, serviceName: serviceName}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apierror.Write(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed.")
		return
	}

	source := strings.Trim(strings.TrimPrefix(r.URL.Path, "/webhooks/"), "/")
	if source == "" || strings.Contains(source, "/") {
		apierror.Write(w, r, http.StatusBadRequest, "WEBHOOK_SOURCE_REQUIRED", "Webhook source is required.")
		return
	}

	// Do not read or log webhook bodies until a source-specific verifier and
	// durable receiver are installed. Keep the cap here so future receivers
	// cannot accidentally accept unbounded payloads.
	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes)
	apierror.Write(w, r, http.StatusNotImplemented, "WEBHOOK_NOT_CONFIGURED", "Webhook transport is not configured.")
}
