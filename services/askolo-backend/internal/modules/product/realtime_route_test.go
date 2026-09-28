package product

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"askolo/backend/internal/config"
)

func TestLegacyRealtimeEndpointIsNotMounted(t *testing.T) {
	handler := NewHandler(config.Config{}, nil, slog.Default(), "askolo_session")
	request := httptest.NewRequest(http.MethodGet, "/api/ai/realtime", nil)
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("legacy realtime route status = %d, want %d", response.Code, http.StatusNotFound)
	}
}
