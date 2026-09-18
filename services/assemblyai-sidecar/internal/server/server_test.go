package server

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthEndpoint(t *testing.T) {
	handler := NewHandler(slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil)))
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"status":"ok"`) {
		t.Fatalf("expected healthy JSON response, got %q", response.Body.String())
	}
	if response.Header().Get(requestIDHeader) == "" {
		t.Fatal("expected a request ID response header")
	}
}

func TestRequestIDIsPreserved(t *testing.T) {
	handler := NewHandler(slog.Default())
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	request.Header.Set(requestIDHeader, "test-request-id")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if got := response.Header().Get(requestIDHeader); got != "test-request-id" {
		t.Fatalf("expected request ID to be preserved, got %q", got)
	}
}
