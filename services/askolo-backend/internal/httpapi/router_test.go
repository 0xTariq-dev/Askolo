package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	"askolo/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
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

func TestMigrationSchemaReadinessHasIndependentBudget(t *testing.T) {
	observer := &migrationReadinessObserver{}
	ready, err := checkMigrationSchemaReadiness(
		context.Background(),
		observer,
		func(ctx context.Context) (bool, error) {
			timer := time.NewTimer(900 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-timer.C:
				return true, nil
			case <-ctx.Done():
				return false, ctx.Err()
			}
		},
	)
	if err != nil || !ready {
		t.Fatalf("schema readiness result = %t, %v; want ready under its dedicated budget", ready, err)
	}
}

func TestMigrationSchemaReadinessHonorsRequestCancellation(t *testing.T) {
	observer := &migrationReadinessObserver{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ready, err := checkMigrationSchemaReadiness(ctx, observer, func(ctx context.Context) (bool, error) {
		<-ctx.Done()
		return false, ctx.Err()
	})
	if ready || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled schema readiness result = %t, %v; want context cancellation", ready, err)
	}
}

func TestSchemaReadinessModeIsEnvironmentSpecific(t *testing.T) {
	for _, test := range []struct {
		environment string
		want        string
	}{
		{"development", schemaReadinessModeMigrationLedger},
		{"staging", schemaReadinessModeMigrationLedger},
		{"production", schemaReadinessModePublishCheck},
	} {
		if got := schemaReadinessModeForEnvironment(test.environment); got != test.want {
			t.Errorf("mode for %q = %q, want %q", test.environment, got, test.want)
		}
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

func TestMicrosoftIntegrationRoutesReachMicrosoftHandler(t *testing.T) {
	handler := New(testConfig("secret"), slog.Default(), nil)
	request := httptest.NewRequest(http.MethodGet, "/api/integrations/microsoft/status", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable ||
		!strings.Contains(response.Body.String(), "Microsoft accounts") {
		t.Fatalf("Microsoft status route returned %d: %s", response.Code, response.Body.String())
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

func TestReadinessMFAAlertAndRecoveryPayloadsAreAggregateOnly(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run MFA readiness router tests")
	}
	testdb.RequireDisposable(t, baseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	if err := adminPool.Ping(ctx); err != nil {
		adminPool.Close()
		t.Fatalf("ping integration database: %v", err)
	}

	schema := fmt.Sprintf("askolo_mfa_readiness_%d", time.Now().UnixNano())
	quotedSchema := `"` + schema + `"`
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+quotedSchema); err != nil {
		adminPool.Close()
		t.Fatalf("create MFA readiness schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), `DROP SCHEMA `+quotedSchema+` CASCADE`)
		adminPool.Close()
	})
	if _, err := adminPool.Exec(ctx, fmt.Sprintf(`
CREATE TABLE %s.auth_security_events (
	id text PRIMARY KEY,
	user_id text,
	event_type text NOT NULL,
	request_id text,
	metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
	created_at timestamptz DEFAULT now() NOT NULL
)
`, quotedSchema)); err != nil {
		t.Fatalf("create MFA readiness events table: %v", err)
	}

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsedURL.Query()
	query.Set("options", "-c search_path="+schema)
	parsedURL.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	store, err := postgres.New(ctx, parsedURL.String())
	if err != nil {
		t.Fatalf("open MFA readiness store: %v", err)
	}
	t.Cleanup(store.Close)

	const (
		privateUserID = "private-user-id-sentinel"
		privateReqID  = "private-request-id-sentinel"
		privateCode   = "private-code-sentinel"
		privateEmail  = "private-email-sentinel@example.invalid"
		privateSecret = "private-secret-sentinel"
		privateMeta   = "private-raw-event-metadata-sentinel"
	)
	metadata := fmt.Sprintf(
		`{"email":%q,"code":%q,"secret":%q,"rawEventMetadata":%q}`,
		privateEmail,
		privateCode,
		privateSecret,
		privateMeta,
	)
	eventCounts := []struct {
		eventType string
		count     int
	}{
		{"mfa_challenge_failed", 20},
		{"mfa_replay_rejected", 5},
		{"mfa_challenge_locked", 5},
		{"mfa_decryption_failed", 3},
		{"mfa_recovery_support_challenge_sent", 20},
		{"mfa_recovery_support_verification_failed", 5},
		{"mfa_recovery_support_rate_limited", 5},
		{"mfa_recovery_support_session_revocation_failed", 1},
	}
	for _, eventCount := range eventCounts {
		for index := range eventCount.count {
			userID := fmt.Sprintf("%s-%d", privateUserID, index%4)
			id := fmt.Sprintf("%s-%s-%d", privateMeta, eventCount.eventType, index)
			if _, err := adminPool.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s.auth_security_events (id, user_id, event_type, request_id, metadata)
VALUES ($1, $2, $3, $4, $5::jsonb)
`, quotedSchema), id, userID, eventCount.eventType, privateReqID, metadata); err != nil {
				t.Fatalf("insert MFA threshold event: %v", err)
			}
		}
	}

	var logs strings.Builder
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := New(testConfig("internal-secret"), logger, store)

	readiness := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusServiceUnavailable {
			t.Fatalf("readiness status = %d, want 503 with isolated schema; body=%s", response.Code, response.Body.String())
		}
		return response
	}

	alertResponse := readiness()
	alertBody := alertResponse.Body.String()
	for _, field := range []string{
		`"status":"available"`,
		`"environment":"test"`,
		`"windowMinutes":15`,
		`"failureEvents":20`,
		`"replayEvents":5`,
		`"lockoutEvents":5`,
		`"decryptionFailureEvents":3`,
		`"recoverySupportRequests":20`,
		`"recoverySupportVerificationFailures":5`,
		`"recoverySupportRateLimited":5`,
		`"recoverySupportSessionRevocationFailures":1`,
		`"affectedUsers":4`,
		`"alert":true`,
		`"alertReasons":["failure_events","replay_events","lockout_events","decryption_failure_events","recovery_support_request_spike","recovery_support_verification_failures","recovery_support_rate_limited","recovery_support_session_revocation_failures"]`,
	} {
		if !strings.Contains(alertBody, field) {
			t.Fatalf("readiness response missing %q: %s", field, alertBody)
		}
	}
	assertMFAPrivacySentinelsAbsent(t, alertBody, privateUserID, privateReqID, privateCode, privateEmail, privateSecret, privateMeta)
	assertMFAPrivacyFieldsAbsent(t, alertBody)

	if _, err := adminPool.Exec(ctx, `DELETE FROM `+quotedSchema+`.auth_security_events`); err != nil {
		t.Fatalf("clear threshold MFA events: %v", err)
	}
	recoveryResponse := readiness()
	recoveryBody := recoveryResponse.Body.String()
	for _, field := range []string{
		`"status":"available"`,
		`"environment":"test"`,
		`"failureEvents":0`,
		`"recoverySupportRequests":0`,
		`"affectedUsers":0`,
		`"alert":false`,
		`"alertReasons":["failure_events","replay_events","lockout_events","decryption_failure_events","recovery_support_request_spike","recovery_support_verification_failures","recovery_support_rate_limited","recovery_support_session_revocation_failures"]`,
	} {
		if !strings.Contains(recoveryBody, field) {
			t.Fatalf("recovered readiness response missing %q: %s", field, recoveryBody)
		}
	}
	assertMFAPrivacySentinelsAbsent(t, recoveryBody, privateUserID, privateReqID, privateCode, privateEmail, privateSecret, privateMeta)
	assertMFAPrivacyFieldsAbsent(t, recoveryBody)

	logOutput := logs.String()
	var mfaLogs []string
	for _, line := range strings.Split(strings.TrimSpace(logOutput), "\n") {
		if strings.Contains(line, `"msg":"MFA verification failure spike"`) ||
			strings.Contains(line, `"msg":"MFA verification failure spike recovered"`) {
			mfaLogs = append(mfaLogs, line)
		}
	}
	alertCount := strings.Count(strings.Join(mfaLogs, "\n"), `"msg":"MFA verification failure spike"`)
	recoveryCount := strings.Count(strings.Join(mfaLogs, "\n"), `"msg":"MFA verification failure spike recovered"`)
	if alertCount != 1 || recoveryCount != 1 {
		t.Fatalf("MFA alert/recovery log counts = %d/%d, want 1/1: %s", alertCount, recoveryCount, logOutput)
	}
	mfaLogOutput := strings.Join(mfaLogs, "\n")
	for _, field := range []string{
		`"operation":"mfa_security_spike"`,
		`"environment":"test"`,
		`"alert_reasons":"failure_events,replay_events,lockout_events,decryption_failure_events,recovery_support_request_spike,recovery_support_verification_failures,recovery_support_rate_limited,recovery_support_session_revocation_failures"`,
	} {
		if !strings.Contains(mfaLogOutput, field) {
			t.Fatalf("MFA alert log missing stable routing field %q: %s", field, mfaLogOutput)
		}
	}
	if !strings.Contains(mfaLogOutput, `"recovery":true`) {
		t.Fatalf("MFA recovery log missing recovery state: %s", mfaLogOutput)
	}
	assertMFAPrivacySentinelsAbsent(t, mfaLogOutput, privateUserID, privateReqID, privateCode, privateEmail, privateSecret, privateMeta)
	assertMFAPrivacyFieldsAbsent(t, mfaLogOutput)
}

func assertMFAPrivacySentinelsAbsent(t *testing.T, serialized string, sentinels ...string) {
	t.Helper()
	for _, sentinel := range sentinels {
		if strings.Contains(serialized, sentinel) {
			t.Fatalf("serialized MFA payload exposed private event data %q: %s", sentinel, serialized)
		}
	}
}

func assertMFAPrivacyFieldsAbsent(t *testing.T, serialized string) {
	t.Helper()
	for _, field := range []string{
		`"user_id"`,
		`"userId"`,
		`"request_id"`,
		`"requestId"`,
		`"code"`,
		`"email"`,
		`"secret"`,
		`"metadata"`,
		`"rawEventMetadata"`,
	} {
		if strings.Contains(serialized, field) {
			t.Fatalf("serialized MFA payload exposed private field %s: %s", field, serialized)
		}
	}
}

func TestProductionReadinessReportsPublishCompatibilityNotLedgerReadiness(t *testing.T) {
	cfg := testConfig("secret")
	cfg.Environment = "production"
	handler := New(cfg, slog.Default(), nil)
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	body := response.Body.String()
	if !strings.Contains(body, `"schemaReadinessMode":"replit_publish_compatibility"`) ||
		!strings.Contains(body, `"migrationLedgerReady":false`) ||
		!strings.Contains(body, `"managedProductionSchemaCompatible":false`) {
		t.Fatalf("production readiness did not distinguish compatibility and ledger checks: %q", body)
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
			request.Header.Set("Origin", "https://example.com")
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
