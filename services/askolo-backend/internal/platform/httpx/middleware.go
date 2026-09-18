package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"

	"askolo/backend/internal/platform/apierror"
)

const requestIDHeader = "X-Request-ID"

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

func Middleware(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := requestIDFrom(r)
		r.Header.Set(requestIDHeader, requestID)
		w.Header().Set(requestIDHeader, requestID)

		writer := &responseWriter{ResponseWriter: w}
		startedAt := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("unhandled HTTP panic", "request_id", requestID, "error", recovered)
				apierror.Write(writer, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error.")
			}
			status := writer.status
			if status == 0 {
				status = http.StatusOK
			}
			logger.Info(
				"http request completed",
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"duration_ms", time.Since(startedAt).Milliseconds(),
			)
		}()

		next.ServeHTTP(writer, r)
	})
}

func requestIDFrom(r *http.Request) string {
	existing := r.Header.Get(requestIDHeader)
	if len(existing) > 0 && len(existing) <= 128 {
		return existing
	}

	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "request-id-unavailable"
	}
	return hex.EncodeToString(bytes[:])
}