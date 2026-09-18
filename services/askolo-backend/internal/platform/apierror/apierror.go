package apierror

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type Response struct {
	Error     string `json:"error"`
	Code      string `json:"code"`
	RequestID string `json:"requestId,omitempty"`
}

func Write(w http.ResponseWriter, r *http.Request, status int, code string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(Response{
		Error:     message,
		Code:      code,
		RequestID: r.Header.Get("X-Request-ID"),
	}); err != nil {
		slog.Error("failed to write API error response", "error", err)
	}
}