package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	"askolo/backend/internal/httpapi"
	"github.com/jackc/pgx/v5/pgxpool"
)

type cleanupIntegrationFixture struct {
	store     *postgres.Store
	adminPool *pgxpool.Pool
	schema    string
}

func newCleanupIntegrationFixture(t *testing.T) *cleanupIntegrationFixture {
	t.Helper()
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run cleanup readiness integration tests")
	}

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

	schema := fmt.Sprintf("askolo_cleanup_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+quoteCleanupIdentifier(schema)); err != nil {
		adminPool.Close()
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), `DROP SCHEMA `+quoteCleanupIdentifier(schema)+` CASCADE`)
		adminPool.Close()
	})

	schemaPrefix := quoteCleanupIdentifier(schema) + "."
	if _, err := adminPool.Exec(ctx, fmt.Sprintf(`
CREATE TABLE %sauth_email_challenges (
	id text PRIMARY KEY,
	user_id text,
	email text NOT NULL,
	purpose text NOT NULL,
	code_hash text NOT NULL,
	expires_at timestamptz NOT NULL,
	consumed_at timestamptz,
	attempt_count integer DEFAULT 0 NOT NULL,
	created_at timestamptz DEFAULT now() NOT NULL
);
CREATE TABLE %sauth_security_events (
	id text PRIMARY KEY,
	user_id text,
	event_type text NOT NULL,
	provider text,
	request_id text,
	metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
	created_at timestamptz DEFAULT now() NOT NULL
);
`, schemaPrefix, schemaPrefix)); err != nil {
		t.Fatalf("create cleanup integration tables: %v", err)
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("options", "-c search_path="+schema)
	parsed.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	schemaURL := parsed.String()

	store, err := postgres.New(ctx, schemaURL)
	if err != nil {
		t.Fatalf("open cleanup integration store: %v", err)
	}
	t.Cleanup(store.Close)

	return &cleanupIntegrationFixture{
		store:     store,
		adminPool: adminPool,
		schema:    schema,
	}
}

func TestEmailChallengeCleanupReadinessRecoversAfterDatabaseInterruption(t *testing.T) {
	fixture := newCleanupIntegrationFixture(t)
	appConfig := config.Config{
		ServiceName:                   "askolo-backend",
		Environment:                   "test",
		Host:                          "127.0.0.1",
		EmailChallengeCleanupInterval: 100 * time.Millisecond,
		Email: config.EmailConfig{
			ResendAPIKey:    "integration-test-key",
			FromAddress:     "Askolo <no-reply@example.com>",
			ChallengeSecret: "integration-only-challenge-secret",
		},
	}
	application := New(appConfig, slog.Default(), fixture.store)

	ctx, cancel := context.WithCancel(context.Background())
	cleanupDone := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		<-cleanupDone
	})
	go application.runEmailChallengeCleanup(ctx, cleanupDone)

	initial := waitForCleanupReadiness(t, application, "healthy", 0)
	if initial.EmailChallengeCleanup.LastSuccessfulCleanupAt == nil {
		t.Fatal("initial cleanup readiness did not include a successful cleanup timestamp")
	}
	initialSuccess := *initial.EmailChallengeCleanup.LastSuccessfulCleanupAt

	interrupted := false
	restoreChallengeTable := func() {
		if !interrupted {
			return
		}
		restoreContext, restoreCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer restoreCancel()
		if _, err := fixture.adminPool.Exec(
			restoreContext,
			`ALTER TABLE `+quoteCleanupIdentifier(fixture.schema)+`.auth_email_challenges_interrupted RENAME TO auth_email_challenges`,
		); err != nil {
			t.Errorf("restore email challenge table: %v", err)
			return
		}
		interrupted = false
	}
	t.Cleanup(restoreChallengeTable)

	if _, err := fixture.adminPool.Exec(
		ctx,
		`ALTER TABLE `+quoteCleanupIdentifier(fixture.schema)+`.auth_email_challenges RENAME TO auth_email_challenges_interrupted`,
	); err != nil {
		t.Fatalf("induce cleanup query failure: %v", err)
	}
	interrupted = true

	for _, expectedFailures := range []int{1, 2} {
		readiness := waitForCleanupReadiness(t, application, "transient_failure", expectedFailures)
		if readiness.EmailChallengeCleanup.LastSuccessfulCleanupAt == nil ||
			!readiness.EmailChallengeCleanup.LastSuccessfulCleanupAt.Equal(initialSuccess) {
			t.Fatalf("cleanup failure %d lost last successful timestamp: %+v, want %v",
				expectedFailures,
				readiness.EmailChallengeCleanup,
				initialSuccess,
			)
		}
	}

	persistent := waitForCleanupReadiness(
		t,
		application,
		"persistent_failure",
		httpapi.EmailChallengeCleanupPersistentFailureThreshold,
	)
	if persistent.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("persistent cleanup failure readiness HTTP status = %d, want %d",
			persistent.HTTPStatus,
			http.StatusServiceUnavailable,
		)
	}
	if persistent.EmailChallengeCleanup.LastSuccessfulCleanupAt == nil ||
		!persistent.EmailChallengeCleanup.LastSuccessfulCleanupAt.Equal(initialSuccess) {
		t.Fatalf("persistent cleanup failure lost last successful timestamp: %+v, want %v",
			persistent.EmailChallengeCleanup,
			initialSuccess,
		)
	}

	restoreChallengeTable()
	recovered := waitForCleanupReadiness(t, application, "healthy", 0)
	if recovered.EmailChallengeCleanup.LastSuccessfulCleanupAt == nil ||
		recovered.EmailChallengeCleanup.LastSuccessfulCleanupAt.Before(initialSuccess) {
		t.Fatalf("recovered cleanup timestamp = %v, want retained or advanced from %v",
			recovered.EmailChallengeCleanup.LastSuccessfulCleanupAt,
			initialSuccess,
		)
	}
}

type cleanupReadinessHTTPResponse struct {
	HTTPStatus            int
	Body                  string
	EmailChallengeCleanup httpapi.EmailChallengeCleanupReadiness `json:"emailChallengeCleanup"`
}

func waitForCleanupReadiness(
	t *testing.T,
	application *App,
	expectedStatus string,
	expectedFailures int,
) cleanupReadinessHTTPResponse {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last cleanupReadinessHTTPResponse
	for time.Now().Before(deadline) {
		request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		response := httptest.NewRecorder()
		application.server.Handler.ServeHTTP(response, request)
		body := response.Body.String()
		last = cleanupReadinessHTTPResponse{
			HTTPStatus: response.Code,
			Body:       body,
		}
		if json.Unmarshal([]byte(body), &last) == nil &&
			last.EmailChallengeCleanup.Status == expectedStatus &&
			last.EmailChallengeCleanup.ConsecutiveFailures == expectedFailures {
			assertCleanupReadinessResponseIsBounded(t, body)
			return last
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf(
		"timed out waiting for cleanup readiness %q/%d; last response = %d %s",
		expectedStatus,
		expectedFailures,
		last.HTTPStatus,
		last.Body,
	)
	return last
}

func assertCleanupReadinessResponseIsBounded(t *testing.T, body string) {
	t.Helper()
	for _, field := range []string{
		`"challengeId"`,
		`"challenge_id"`,
		`"address"`,
		`"code"`,
		`"token"`,
		`"requestId"`,
		`"request_id"`,
	} {
		if strings.Contains(body, field) {
			t.Fatalf("cleanup readiness response exposed challenge-specific field %q: %s", field, body)
		}
	}
}

func quoteCleanupIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
