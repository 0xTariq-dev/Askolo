package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"askolo/backend/internal/config"
)

func TestMigrationReadinessProbeCoalescesAndCachesChecks(t *testing.T) {
	observer := &migrationReadinessObserver{}
	var calls atomic.Int32
	var enteredOnce sync.Once
	entered := make(chan struct{})
	release := make(chan struct{})
	probe := func(ctx context.Context) (bool, error) {
		calls.Add(1)
		enteredOnce.Do(func() { close(entered) })
		select {
		case <-release:
			return true, nil
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}

	type result struct {
		ready bool
		err   error
	}
	results := make(chan result, 2)
	go func() {
		ready, err := observer.check(context.Background(), probe)
		results <- result{ready: ready, err: err}
	}()
	<-entered
	go func() {
		ready, err := observer.check(context.Background(), probe)
		results <- result{ready: ready, err: err}
	}()
	close(release)

	for range 2 {
		got := <-results
		if got.err != nil || !got.ready {
			t.Fatalf("coalesced readiness result = %t, %v; want ready", got.ready, got.err)
		}
	}
	if ready, err := observer.check(context.Background(), probe); err != nil || !ready {
		t.Fatalf("cached readiness result = %t, %v; want ready", ready, err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("probe ran %d times, want one coalesced and cached check", got)
	}
}

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

func TestCreditRoutesReachProductHandlerAndFailExplicitlyWhenUnavailable(t *testing.T) {
	handler := New(testConfig("secret"), slog.Default(), nil)
	for _, path := range []string{
		"/api/ai/credits",
		"/api/admin/ai-credit-policy",
	} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != http.StatusServiceUnavailable {
				t.Fatalf("expected status 503, got %d with body %q", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), `"code":"AUTHENTICATION_UNAVAILABLE"`) {
				t.Fatalf("expected explicit authentication error, got %q", response.Body.String())
			}
		})
	}
}

func TestCleanupMonitoringDashboardRequiresInternalAuthAndIsAggregateOnly(t *testing.T) {
	cfg := testConfig("secret")
	cleanupReadiness := EmailChallengeCleanupReadiness{
		Status:                       "persistent_failure",
		ConsecutiveFailures:          EmailChallengeCleanupPersistentFailureThreshold,
		PersistentFailureThreshold:   EmailChallengeCleanupPersistentFailureThreshold,
		PersistentFailureOccurrences: 2,
		RecoveryEvents:               1,
	}
	handler := New(cfg, slog.Default(), nil, func() EmailChallengeCleanupReadiness {
		return cleanupReadiness
	})

	unauthorizedRequest := httptest.NewRequest(http.MethodGet, "/internal/monitoring/cleanup", nil)
	unauthorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedResponse, unauthorizedRequest)
	if unauthorizedResponse.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", unauthorizedResponse.Code)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/internal/monitoring/cleanup", nil)
	authorizedRequest.Header.Set("X-Askolo-Internal-Token", "secret")
	authorizedResponse := httptest.NewRecorder()
	handler.ServeHTTP(authorizedResponse, authorizedRequest)
	if authorizedResponse.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d with body %q", authorizedResponse.Code, authorizedResponse.Body.String())
	}

	body := authorizedResponse.Body.String()
	for _, field := range []string{
		`"environment":"test"`,
		`"service":"askolo-backend"`,
		`"operation":"email_challenge_cleanup"`,
		`"persistentFailureOccurrences":2`,
		`"recoveryEvents":1`,
		`"status":"persistent_failure"`,
	} {
		if !strings.Contains(body, field) {
			t.Fatalf("dashboard response missing %q: %s", field, body)
		}
	}
	for _, field := range []string{"challenge_id", "challengeId", "address", "error", "request_id", "requestId"} {
		if strings.Contains(body, field) {
			t.Fatalf("dashboard response exposed %q: %s", field, body)
		}
	}
}

func TestGoogleAuthRoutesPrecedeGenericAuthRoutes(t *testing.T) {
	cfg := testConfig("secret")
	cfg.SessionSecret = "test-session-secret"
	cfg.AllowedOAuthHosts = map[string]struct{}{"localhost": {}}
	cfg.Google.AuthURL = "https://accounts.google.com/o/oauth2/v2/auth"
	handler := New(cfg, slog.Default(), nil)

	for _, test := range []struct {
		name       string
		path       string
		statusCode int
		body       string
	}{
		{
			name:       "signin start",
			path:       "/api/auth/google?intent=signin&returnTo=%2Fdashboard",
			statusCode: http.StatusServiceUnavailable,
			body:       `"code":"GOOGLE_NOT_CONFIGURED"`,
		},
		{
			name:       "signup start",
			path:       "/api/auth/google?intent=signup&returnTo=%2Fsign-up",
			statusCode: http.StatusServiceUnavailable,
			body:       `"code":"GOOGLE_NOT_CONFIGURED"`,
		},
		{
			name:       "callback without state",
			path:       "/api/auth/google/callback?state=missing",
			statusCode: http.StatusBadRequest,
			body:       `"code":"INVALID_STATE"`,
		},
		{
			name:       "google link remains native",
			path:       "/api/auth/google/link",
			statusCode: http.StatusServiceUnavailable,
			body:       `"code":"UNAUTHORIZED"`,
		},
		{
			name:       "integration remains native",
			path:       "/api/integrations/google",
			statusCode: http.StatusServiceUnavailable,
			body:       `"code":"UNAUTHORIZED"`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			request.Host = "localhost"
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.statusCode {
				t.Fatalf("expected status %d, got %d with body %q", test.statusCode, response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), test.body) {
				t.Fatalf("expected body to contain %q, got %q", test.body, response.Body.String())
			}
		})
	}
}

func TestGenericAuthRoutesRemainReachableAlongsideGoogleRoutes(t *testing.T) {
	cfg := testConfig("secret")
	cfg.SessionSecret = "test-session-secret"
	cfg.AllowedOAuthHosts = map[string]struct{}{"localhost": {}}
	handler := New(cfg, slog.Default(), nil)

	request := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	request.Host = "localhost"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"user":null`) {
		t.Fatalf("expected password/session auth route to remain reachable, got %d %q", response.Code, response.Body.String())
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
		!strings.Contains(body, `"migrationSchemaReady":false`) ||
		!strings.Contains(body, `"emailDeliveryConfigured":false`) ||
		!strings.Contains(body, `"emailChallengeCleanup":{"status":"unknown"`) {
		t.Fatalf("expected dependency readiness signals, got %q", body)
	}
}

func TestReadinessDoesNotRequireEmailHandoff(t *testing.T) {
	status, statusCode := dependencyReadinessStatus(true, true, true, true)
	if status != "ready" || statusCode != http.StatusOK {
		t.Fatalf("configured fresh instance = %q/%d, want ready/200", status, statusCode)
	}

	for _, test := range []struct {
		name                      string
		databaseReachable         bool
		authorizationStorageReady bool
		migrationSchemaReady      bool
		emailDeliveryConfigured   bool
	}{
		{
			name:                      "database unavailable",
			databaseReachable:         false,
			authorizationStorageReady: true,
			migrationSchemaReady:      true,
			emailDeliveryConfigured:   true,
		},
		{
			name:                      "authorization schema unavailable",
			databaseReachable:         true,
			authorizationStorageReady: false,
			migrationSchemaReady:      true,
			emailDeliveryConfigured:   true,
		},
		{
			name:                      "migration schema unavailable",
			databaseReachable:         true,
			authorizationStorageReady: true,
			migrationSchemaReady:      false,
			emailDeliveryConfigured:   true,
		},
		{
			name:                      "email configuration unavailable",
			databaseReachable:         true,
			authorizationStorageReady: true,
			migrationSchemaReady:      true,
			emailDeliveryConfigured:   false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, statusCode := dependencyReadinessStatus(
				test.databaseReachable,
				test.authorizationStorageReady,
				test.migrationSchemaReady,
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
	status, statusCode := dependencyReadinessStatus(true, true, true, true, cleanupReadiness)
	if status != "degraded" || statusCode != http.StatusServiceUnavailable {
		t.Fatalf("persistent cleanup failure = %q/%d, want degraded/503", status, statusCode)
	}

	cleanupReadiness.Status = "transient_failure"
	status, statusCode = dependencyReadinessStatus(true, true, true, true, cleanupReadiness)
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
