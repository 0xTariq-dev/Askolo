package httpapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"askolo/backend/internal/config"
)

func testConfig(token string) config.Config {
	return config.Config{
		ServiceName:       "askolo-backend",
		Environment:       "test",
		Host:              "127.0.0.1",
		Port:              8090,
		InternalAuthToken: token,
	}
}

func TestHealthAndRequestID(t *testing.T) {
	handler := New(testConfig("secret"), slog.Default(), nil)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if response.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected a request ID")
	}
	if !strings.Contains(response.Body.String(), `"status":"ok"`) {
		t.Fatalf("expected health response, got %q", response.Body.String())
	}
}

func TestReadinessReportsMissingDependencies(t *testing.T) {
	handler := New(testConfig("secret"), slog.Default(), nil)
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"status":"degraded"`) ||
		!strings.Contains(body, `"databaseReachable":false`) ||
		!strings.Contains(body, `"emailDeliveryConfigured":false`) {
		t.Fatalf("expected dependency readiness signals, got %q", body)
	}
}

func TestInternalRestRequiresAndAcceptsServiceAuth(t *testing.T) {
	handler := New(testConfig("secret"), slog.Default(), nil)

	unauthorizedRequest := httptest.NewRequest(http.MethodGet, "/internal/rest/v1/status", nil)
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorizedRequest)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", unauthorizedResponse.Code)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/internal/rest/v1/status", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer secret")
	authorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(authorizedResponse, authorizedRequest)
	if authorizedResponse.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", authorizedResponse.Code)
	}
}

func TestProtocolShellsRequireAuthorizationBeforeConfigurationCheck(t *testing.T) {
	handler := New(testConfig("secret"), slog.Default(), nil)

	for _, path := range []string{"/ws", "/webhooks/example"} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		if path == "/ws" {
			request = httptest.NewRequest(http.MethodGet, path, nil)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		expected := http.StatusNotImplemented
		if path == "/ws" {
			expected = http.StatusServiceUnavailable
		}
		if response.Code != expected {
			t.Fatalf("expected %s to return %d, got %d", path, expected, response.Code)
		}
	}
}

func TestUnknownRoutesUseJSONErrorContract(t *testing.T) {
	handler := New(testConfig("secret"), slog.Default(), nil)
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"code":"NOT_FOUND"`) {
		t.Fatalf("expected JSON error contract, got %q", response.Body.String())
	}
}
