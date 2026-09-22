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

func TestPublishedAPIHealthAlias(t *testing.T) {
	handler := New(testConfig("secret"), slog.Default(), nil)
	request := httptest.NewRequest(http.MethodGet, "/api/healthz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
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
		!strings.Contains(body, `"emailDeliveryConfigured":false`) ||
		!strings.Contains(body, `"emailChallengeCleanup":{"status":"unknown"`) {
		t.Fatalf("expected dependency readiness signals, got %q", body)
	}
}

func TestReadinessDoesNotRequireEmailHandoff(t *testing.T) {
	status, statusCode := dependencyReadinessStatus(true, true, true)
	if status != "ready" || statusCode != http.StatusOK {
		t.Fatalf("configured fresh instance = %q/%d, want ready/200", status, statusCode)
	}

	for _, test := range []struct {
		name                      string
		databaseReachable         bool
		authorizationStorageReady bool
		emailDeliveryConfigured   bool
	}{
		{
			name:                      "database unavailable",
			databaseReachable:         false,
			authorizationStorageReady: true,
			emailDeliveryConfigured:   true,
		},
		{
			name:                      "authorization schema unavailable",
			databaseReachable:         true,
			authorizationStorageReady: false,
			emailDeliveryConfigured:   true,
		},
		{
			name:                      "email configuration unavailable",
			databaseReachable:         true,
			authorizationStorageReady: true,
			emailDeliveryConfigured:   false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, statusCode := dependencyReadinessStatus(
				test.databaseReachable,
				test.authorizationStorageReady,
				test.emailDeliveryConfigured,
			)
			if status != "degraded" || statusCode != http.StatusServiceUnavailable {
				t.Fatalf("dependency failure = %q/%d, want degraded/503", status, statusCode)
			}
		})
	}
}

func TestReadinessReportsPersistentEmailChallengeCleanupFailure(t *testing.T) {
	cleanupReadiness := EmailChallengeCleanupReadiness{
		Status:                     "persistent_failure",
		ConsecutiveFailures:        EmailChallengeCleanupPersistentFailureThreshold,
		PersistentFailureThreshold: EmailChallengeCleanupPersistentFailureThreshold,
	}
	status, statusCode := dependencyReadinessStatus(true, true, true, cleanupReadiness)
	if status != "degraded" || statusCode != http.StatusServiceUnavailable {
		t.Fatalf("persistent cleanup failure = %q/%d, want degraded/503", status, statusCode)
	}

	cleanupReadiness.Status = "transient_failure"
	status, statusCode = dependencyReadinessStatus(true, true, true, cleanupReadiness)
	if status != "ready" || statusCode != http.StatusOK {
		t.Fatalf("transient cleanup failure = %q/%d, want ready/200", status, statusCode)
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
