package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	productmodule "askolo/backend/internal/modules/product"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	integrationEmail    = "native-auth@example.com"
	integrationPassword = "correct horse battery staple"
)

var emailCodePattern = regexp.MustCompile(`Your one-time code is: ([0-9]{6})`)

type captureEmailSender struct {
	mu          sync.Mutex
	messages    []EmailMessage
	err         error
	latency     time.Duration
	leakInError bool
}

func (s *captureEmailSender) Send(ctx context.Context, message EmailMessage) error {
	if s.latency > 0 {
		timer := time.NewTimer(s.latency)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	s.mu.Lock()
	s.messages = append(s.messages, message)
	err := s.err
	if err != nil && s.leakInError {
		err = fmt.Errorf("%w: recipient=%s body=%s", err, message.To, message.Body)
	}
	s.mu.Unlock()
	return err
}

func (s *captureEmailSender) snapshot() []EmailMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	messages := make([]EmailMessage, len(s.messages))
	copy(messages, s.messages)
	return messages
}

func (s *captureEmailSender) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.messages)
}

func (s *captureEmailSender) codeForSubject(t *testing.T, subject string) string {
	t.Helper()
	messages := s.snapshot()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Subject != subject {
			continue
		}
		match := emailCodePattern.FindStringSubmatch(messages[i].Body)
		if len(match) == 2 {
			return match[1]
		}
	}
	t.Fatalf("no challenge code captured for subject %q", subject)
	return ""
}

type emailAuthFixture struct {
	store      *postgres.Store
	pool       *pgxpool.Pool
	testPool   *pgxpool.Pool
	schema     string
	baseURL    string
	authConfig config.Config
}

func newEmailAuthFixture(t *testing.T) *emailAuthFixture {
	t.Helper()
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run native email auth integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	testPool, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	if err := testPool.Ping(ctx); err != nil {
		testPool.Close()
		t.Fatalf("ping integration database: %v", err)
	}

	schema := fmt.Sprintf("askolo_native_auth_%d", time.Now().UnixNano())
	if _, err := testPool.Exec(ctx, `CREATE SCHEMA `+quoteIdentifier(schema)); err != nil {
		testPool.Close()
		t.Fatalf("create integration schema: %v", err)
	}
	cleanupSchema := func() {
		_, _ = testPool.Exec(context.Background(), `DROP SCHEMA `+quoteIdentifier(schema)+` CASCADE`)
		testPool.Close()
	}
	t.Cleanup(cleanupSchema)

	if _, err := testPool.Exec(ctx, integrationSchemaSQL(schema)); err != nil {
		t.Fatalf("create integration tables: %v", err)
	}
	schemaURL := databaseURLWithSearchPath(t, baseURL, schema)
	pool, err := pgxpool.New(ctx, schemaURL)
	if err != nil {
		t.Fatalf("open schema-scoped integration pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping schema-scoped integration pool: %v", err)
	}
	t.Cleanup(pool.Close)

	store, err := postgres.New(ctx, schemaURL)
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	t.Cleanup(store.Close)

	return &emailAuthFixture{
		store:    store,
		pool:     pool,
		testPool: testPool,
		schema:   schema,
		baseURL:  schemaURL,
		authConfig: config.Config{
			Environment:             "test",
			SessionCookieName:       "askolo.sid",
			AuthRateLimitHMACSecret: "integration-only-auth-rate-limit-hmac-secret-at-least-32-bytes",
			TurnstileSecret:         "integration-only-turnstile-secret",
			TurnstileAllowedHostnames: map[string]struct{}{
				"web.askolo.app": {},
			},
			Email: config.EmailConfig{
				ChallengeSecret: "integration-only-challenge-secret",
			},
		},
	}
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func databaseURLWithSearchPath(t *testing.T, databaseURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("options", "-c search_path="+schema)
	// PostgreSQL's libpq-style options value expects a real encoded space,
	// while url.Values.Encode uses '+' for spaces.
	parsed.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	return parsed.String()
}

func integrationSchemaSQL(schema string) string {
	prefix := quoteIdentifier(schema) + "."
	return fmt.Sprintf(`
CREATE TABLE %susers (
	id text PRIMARY KEY,
	email varchar UNIQUE,
	first_name varchar,
	last_name varchar,
	profile_image_url varchar,
	status text DEFAULT 'active' NOT NULL,
	email_verified_at timestamptz,
	account_created_via text,
	created_at timestamptz DEFAULT now() NOT NULL,
	updated_at timestamptz DEFAULT now() NOT NULL,
	preferred_locale varchar(2),
	CONSTRAINT users_preferred_locale_check
		CHECK (
			preferred_locale IS NULL
			OR (preferred_locale COLLATE "C") ~ '^[a-z]{2}$'
		)
);
CREATE TABLE %ssessions (
	sid varchar PRIMARY KEY,
	sess jsonb NOT NULL,
	expire timestamp NOT NULL
);
CREATE TABLE %sauth_passwords (
	user_id text PRIMARY KEY,
	password_hash text NOT NULL,
	hash_version text DEFAULT 'argon2id-v1' NOT NULL,
	created_at timestamptz DEFAULT now() NOT NULL,
	updated_at timestamptz DEFAULT now() NOT NULL
);
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
CREATE TABLE %sauth_recovery_methods (
	id text PRIMARY KEY,
	user_id text NOT NULL,
	kind text NOT NULL,
	address text,
	secret_encrypted text,
	verified_at timestamptz,
	revoked_at timestamptz,
	created_at timestamptz DEFAULT now() NOT NULL,
	updated_at timestamptz DEFAULT now() NOT NULL,
	UNIQUE (user_id, kind, address)
);
CREATE TABLE %sauth_recovery_codes (
	id text PRIMARY KEY,
	user_id text NOT NULL,
	code_hash text NOT NULL,
	used_at timestamptz,
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
CREATE TABLE %sauth_totp (
	user_id text PRIMARY KEY,
	secret_encrypted text NOT NULL,
	enabled_at timestamptz,
	last_used_step integer,
	created_at timestamptz DEFAULT now() NOT NULL,
	updated_at timestamptz DEFAULT now() NOT NULL
);
CREATE TABLE %sauth_mfa_recovery_rate_limits (
bucket_hash text PRIMARY KEY,
window_started_at timestamptz NOT NULL,
request_count integer NOT NULL
);
CREATE TABLE %sauth_trusted_devices (
 id text PRIMARY KEY,
 user_id text NOT NULL,
 credential_hash text UNIQUE NOT NULL,
 fingerprint_hash text NOT NULL,
 created_at timestamptz DEFAULT now() NOT NULL,
 last_used_at timestamptz,
 expires_at timestamptz NOT NULL,
 revoked_at timestamptz
);
CREATE TABLE %sai_credit_accounts (
 user_id text PRIMARY KEY REFERENCES %susers(id) ON DELETE CASCADE,
 granted_credits integer DEFAULT 0 NOT NULL,
 adjustment_credits integer DEFAULT 0 NOT NULL,
 reserved_credits integer DEFAULT 0 NOT NULL,
 spent_credits integer DEFAULT 0 NOT NULL,
 refunded_credits integer DEFAULT 0 NOT NULL,
 granted_usd_micros bigint DEFAULT 0 NOT NULL,
 adjustment_usd_micros bigint DEFAULT 0 NOT NULL,
 reserved_usd_micros bigint DEFAULT 0 NOT NULL,
 spent_usd_micros bigint DEFAULT 0 NOT NULL,
 refunded_usd_micros bigint DEFAULT 0 NOT NULL,
 updated_at timestamptz DEFAULT now() NOT NULL
);
CREATE TABLE %sai_credit_grants (
 id bigserial PRIMARY KEY,
 user_id text NOT NULL REFERENCES %susers(id) ON DELETE CASCADE,
 source_type varchar(40) NOT NULL,
 amount_credits integer NOT NULL,
 entitlement_key varchar(160),
 idempotency_key varchar(200) NOT NULL,
 metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
 created_at timestamptz DEFAULT now() NOT NULL,
 actor_user_id text REFERENCES %susers(id) ON DELETE RESTRICT,
 reason varchar(160),
 currency varchar(8) DEFAULT 'CREDITS' NOT NULL,
 amount_usd_micros bigint DEFAULT 0 NOT NULL,
 UNIQUE (user_id, idempotency_key)
);
`, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix, prefix)
}

func testAuthHandler(fixture *emailAuthFixture, sender EmailSender, logger *slog.Logger) http.Handler {
	return testAuthHandlerWithMonitor(
		fixture.authConfig,
		fixture.store,
		logger,
		sender,
		NewEmailDeliveryMonitor(),
	).Routes()
}

func testAuthHandlerWithMonitor(
	cfg config.Config,
	store *postgres.Store,
	logger *slog.Logger,
	sender EmailSender,
	monitor *EmailDeliveryMonitor,
) *Handler {
	handler := NewHandlerWithEmailSenderAndMonitor(cfg, store, logger, sender, monitor)
	handler.turnstileVerifier = func(_ context.Context, token string) (turnstileSiteverifyResponse, error) {
		action, ok := strings.CutPrefix(token, "test-turnstile:")
		return turnstileSiteverifyResponse{
			Success:  ok,
			Action:   action,
			Hostname: "web.askolo.app",
		}, nil
	}
	return handler
}

func testProductHandler(fixture *emailAuthFixture) http.Handler {
	return productmodule.NewHandler(fixture.authConfig, fixture.store, slog.Default(), fixture.authConfig.SessionCookieName).Routes()
}

func jsonRequest(t *testing.T, handler http.Handler, method, path string, input map[string]string, cookie *http.Cookie, remote string) *httptest.ResponseRecorder {
	t.Helper()
	if action := testTurnstileAction(path); action != "" {
		if input == nil {
			input = make(map[string]string)
		}
		input["turnstileToken"] = "test-turnstile:" + action
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = remote
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func testTurnstileAction(path string) string {
	switch path {
	case "/api/auth/password/signup":
		return "signup"
	case "/api/auth/password/login":
		return "login"
	case "/api/auth/password/recovery/request":
		return "password_recovery"
	case "/api/auth/email/resend":
		return "email_resend"
	case "/api/auth/mfa/recovery/request":
		return "mfa_recovery"
	default:
		return ""
	}
}

func plainRequest(handler http.Handler, method, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func sessionCookie(t *testing.T, response *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "askolo.sid" {
			return cookie
		}
	}
	t.Fatalf("response with status %d did not set a session cookie", response.Code)
	return nil
}

func assertResponseDoesNotContain(t *testing.T, response *httptest.ResponseRecorder, secrets ...string) {
	t.Helper()
	body := response.Body.String()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(body, secret) {
			t.Fatal("response contained a value that must remain private")
		}
	}
}

func createVerifiedUser(t *testing.T, fixture *emailAuthFixture, email, password string) string {
	t.Helper()
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}
	userID, err := fixture.store.CreatePasswordUser(context.Background(), email, hash)
	if err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	if _, err := fixture.pool.Exec(context.Background(), `
		UPDATE users SET status = 'active', email_verified_at = NOW() WHERE id = $1
	`, userID); err != nil {
		t.Fatalf("activate fixture user: %v", err)
	}
	return userID
}

func createChallenge(t *testing.T, fixture *emailAuthFixture, handler *Handler, userID, email, purpose, code string, expiresAt time.Time) {
	t.Helper()
	if err := fixture.store.CreateEmailChallenge(
		context.Background(), fmt.Sprintf("challenge-%d", time.Now().UnixNano()),
		userID, email, purpose, handler.hashChallenge(code), expiresAt,
	); err != nil {
		t.Fatalf("create fixture challenge: %v", err)
	}
}

func securityEventCount(t *testing.T, fixture *emailAuthFixture, userID, eventType string) int {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT count(*)
		FROM auth_security_events
		WHERE user_id = $1 AND event_type = $2
	`, userID, eventType).Scan(&count); err != nil {
		t.Fatalf("count %s security events: %v", eventType, err)
	}
	return count
}

func TestNativeEmailAuthLifecycleAndCleanup(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	sender := &captureEmailSender{}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	authHandler := testAuthHandler(fixture, sender, logger)

	signup := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/signup", map[string]string{
		"email":    integrationEmail,
		"password": integrationPassword,
	}, nil, "192.0.2.10:1000")
	if signup.Code != http.StatusAccepted {
		t.Fatalf("signup status = %d, want accepted", signup.Code)
	}
	if sender.count() != 1 {
		t.Fatalf("fresh signup sent %d messages, want exactly one", sender.count())
	}
	signupCode := sender.codeForSubject(t, "Verify your Askolo email")
	assertResponseDoesNotContain(t, signup, integrationEmail, signupCode, integrationPassword)
	var signupUserID string
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT id FROM users WHERE lower(email) = lower($1)
	`, integrationEmail).Scan(&signupUserID); err != nil {
		t.Fatalf("read pending signup user: %v", err)
	}
	var welcomeGrantCount int
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM ai_credit_grants WHERE user_id=$1 AND idempotency_key='welcome-credit-usd-v1'
	`, signupUserID).Scan(&welcomeGrantCount); err != nil {
		t.Fatalf("count pending signup welcome grants: %v", err)
	}
	if welcomeGrantCount != 0 {
		t.Fatalf("pending password signup has %d welcome grants, want none", welcomeGrantCount)
	}

	verify := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/email/verify", map[string]string{
		"email": integrationEmail,
		"code":  signupCode,
	}, nil, "192.0.2.11:1000")
	if verify.Code != http.StatusOK {
		t.Fatalf("email verification status = %d, want success", verify.Code)
	}
	assertResponseDoesNotContain(t, verify, integrationEmail, signupCode, integrationPassword)
	var welcomeGrantAmount int64
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT amount_usd_micros FROM ai_credit_grants
		WHERE user_id=$1 AND idempotency_key='welcome-credit-usd-v1' AND currency='USD'
	`, signupUserID).Scan(&welcomeGrantAmount); err != nil {
		t.Fatalf("read verified signup welcome grant: %v", err)
	}
	if welcomeGrantAmount != 5_000_000 {
		t.Fatalf("password signup welcome grant = %d USD micros, want 5000000", welcomeGrantAmount)
	}

	primaryRecoveryRequest := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
		"email": integrationEmail,
	}, nil, "192.0.2.121:1000")
	if primaryRecoveryRequest.Code != http.StatusAccepted {
		t.Fatalf("primary email recovery request status = %d, want accepted", primaryRecoveryRequest.Code)
	}
	primaryRecoveryCode := sender.codeForSubject(t, "Reset your Askolo password")
	assertResponseDoesNotContain(t, primaryRecoveryRequest, integrationEmail, primaryRecoveryCode, integrationPassword)

	primaryRecoveryCooldown := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
		"email": integrationEmail,
	}, nil, "192.0.2.122:1000")
	if primaryRecoveryCooldown.Code != http.StatusAccepted ||
		primaryRecoveryCooldown.Body.String() != primaryRecoveryRequest.Body.String() ||
		primaryRecoveryCooldown.Header().Get("Retry-After") != "" {
		t.Fatalf("primary recovery cooldown response = %d, retry-after=%q, body=%s; want the generic recovery response",
			primaryRecoveryCooldown.Code,
			primaryRecoveryCooldown.Header().Get("Retry-After"),
			primaryRecoveryCooldown.Body.String(),
		)
	}

	messagesBeforeDuplicateSignup := sender.count()
	duplicateSignup := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/signup", map[string]string{
		"email": integrationEmail, "password": "a different password",
	}, nil, "192.0.2.111:1000")
	if duplicateSignup.Code != http.StatusAccepted || duplicateSignup.Body.String() != `{"status":"verification_required"}`+"\n" {
		t.Fatalf("duplicate signup status=%d generic-response-matches=%t",
			duplicateSignup.Code,
			duplicateSignup.Body.String() == `{"status":"verification_required"}`+"\n")
	}
	if sender.count() != messagesBeforeDuplicateSignup {
		t.Fatalf("duplicate signup sent a new message (total messages: %d, before request: %d)", sender.count(), messagesBeforeDuplicateSignup)
	}
	assertResponseDoesNotContain(t, duplicateSignup, integrationEmail, "a different password")

	login := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/login", map[string]string{
		"email": integrationEmail, "password": integrationPassword,
	}, nil, "192.0.2.12:1000")
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, want success", login.Code)
	}
	if sender.count() != messagesBeforeDuplicateSignup {
		t.Fatalf("ordinary password sign-in sent an email (messages before=%d, after=%d)",
			messagesBeforeDuplicateSignup, sender.count())
	}
	sessionOne := sessionCookie(t, login)
	loginTwo := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/login", map[string]string{
		"email": integrationEmail, "password": integrationPassword,
	}, nil, "192.0.2.13:1000")
	sessionTwo := sessionCookie(t, loginTwo)

	recoveryEmail := "recovery@example.com"
	enroll := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/recovery/email/enroll", map[string]string{
		"email": recoveryEmail, "currentPassword": integrationPassword,
	}, sessionOne, "192.0.2.14:1000")
	if enroll.Code != http.StatusAccepted {
		t.Fatalf("recovery enrollment status = %d, want accepted", enroll.Code)
	}
	recoveryEmailCode := sender.codeForSubject(t, "Confirm your Askolo recovery email")
	assertResponseDoesNotContain(t, enroll, recoveryEmail, recoveryEmailCode, integrationPassword)

	verifyRecovery := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/recovery/email/verify", map[string]string{
		"email": recoveryEmail, "code": recoveryEmailCode,
	}, sessionOne, "192.0.2.15:1000")
	if verifyRecovery.Code != http.StatusOK {
		t.Fatalf("recovery verification status = %d, want success", verifyRecovery.Code)
	}

	unknownRecovery := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
		"email": "unknown@example.com",
	}, nil, "192.0.2.16:1000")
	if unknownRecovery.Code != http.StatusAccepted || unknownRecovery.Body.String() != `{"status":"recovery_if_available"}`+"\n" {
		t.Fatalf("unknown recovery status=%d generic-response-matches=%t",
			unknownRecovery.Code,
			unknownRecovery.Body.String() == `{"status":"recovery_if_available"}`+"\n")
	}
	assertResponseDoesNotContain(t, unknownRecovery, "unknown@example.com")
	unknownRecoveryRepeat := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
		"email": "unknown@example.com",
	}, nil, "192.0.2.16:2000")
	if unknownRecoveryRepeat.Code != unknownRecovery.Code || unknownRecoveryRepeat.Body.String() != unknownRecovery.Body.String() {
		t.Fatalf("repeated unknown recovery response differs: first=%d %q second=%d %q",
			unknownRecovery.Code, unknownRecovery.Body.String(),
			unknownRecoveryRepeat.Code, unknownRecoveryRepeat.Body.String())
	}

	deliveryCountBeforeDirectRecovery := sender.count()
	directRecoveryAddress := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
		"email": recoveryEmail,
	}, nil, "192.0.2.161:1000")
	if directRecoveryAddress.Code != http.StatusAccepted || directRecoveryAddress.Body.String() != unknownRecovery.Body.String() {
		t.Fatalf("direct recovery address status=%d generic-response-matches=%t",
			directRecoveryAddress.Code,
			directRecoveryAddress.Body.String() == unknownRecovery.Body.String())
	}
	if sender.count() != deliveryCountBeforeDirectRecovery {
		t.Fatalf("direct recovery address unexpectedly sent a message")
	}

	recoveryRequest := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
		"email":  integrationEmail,
		"method": "recovery_email",
	}, nil, "192.0.2.17:1000")
	if recoveryRequest.Code != http.StatusAccepted {
		t.Fatalf("recovery request status = %d, want accepted", recoveryRequest.Code)
	}
	if recoveryRequest.Body.String() != unknownRecovery.Body.String() {
		t.Fatal("known and unknown recovery response bodies differ")
	}
	recoveryCode := sender.codeForSubject(t, "Reset your Askolo password")
	assertResponseDoesNotContain(t, recoveryRequest, recoveryEmail, recoveryCode, integrationPassword)

	recoveryVerify := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/verify", map[string]string{
		"email": integrationEmail, "method": "recovery_email", "code": recoveryCode,
	}, nil, "192.0.2.171:1000")
	if recoveryVerify.Code != http.StatusOK {
		t.Fatalf("recovery code verification status = %d, want success", recoveryVerify.Code)
	}
	assertResponseDoesNotContain(t, recoveryVerify, recoveryEmail, recoveryCode, integrationPassword)

	resetPassword := "new correct horse battery staple"
	reset := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/reset", map[string]string{
		"email": integrationEmail, "method": "recovery_email", "code": recoveryCode, "password": resetPassword,
	}, nil, "192.0.2.18:1000")
	if reset.Code != http.StatusOK {
		t.Fatalf("password reset status = %d, want success", reset.Code)
	}
	assertResponseDoesNotContain(t, reset, recoveryEmail, recoveryCode, resetPassword)

	for name, cookie := range map[string]*http.Cookie{"first": sessionOne, "second": sessionTwo} {
		session := plainRequest(authHandler, http.MethodGet, "/api/auth/session", cookie)
		if session.Code != http.StatusOK || session.Body.String() != `{"user":null}`+"\n" {
			t.Fatalf("%s session after reset status=%d is-null=%t",
				name, session.Code, session.Body.String() == `{"user":null}`+"\n")
		}
	}

	newLogin := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/login", map[string]string{
		"email": integrationEmail, "password": resetPassword,
	}, nil, "192.0.2.19:1000")
	deleteSession := sessionCookie(t, newLogin)
	logout := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/logout", nil, deleteSession, "192.0.2.20:1000")
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d", logout.Code)
	}
	afterLogout := plainRequest(authHandler, http.MethodGet, "/api/auth/session", deleteSession)
	if afterLogout.Body.String() != `{"user":null}`+"\n" {
		t.Fatalf("session after logout is unauthenticated=%t", afterLogout.Body.String() == `{"user":null}`+"\n")
	}

	deleteLogin := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/login", map[string]string{
		"email": integrationEmail, "password": resetPassword,
	}, nil, "192.0.2.21:1000")
	deleteResponse := plainRequest(testProductHandler(fixture), http.MethodDelete, "/api/user/account", sessionCookie(t, deleteLogin))
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("account deletion status = %d, want no content", deleteResponse.Code)
	}
	if _, err := fixture.store.FindUserByEmail(context.Background(), integrationEmail); !errors.Is(err, postgres.ErrNotFound) {
		t.Fatalf("deleted user lookup error = %v, want not found", err)
	}
	var remaining int
	for _, table := range []string{"sessions", "auth_passwords", "auth_email_challenges", "auth_recovery_methods", "auth_security_events", "auth_totp"} {
		if err := fixture.pool.QueryRow(context.Background(), `SELECT count(*) FROM `+table).Scan(&remaining); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if remaining != 0 {
			t.Fatalf("%s retained %d rows after account deletion", table, remaining)
		}
	}
	if strings.Contains(logs.String(), integrationEmail) || strings.Contains(logs.String(), integrationPassword) {
		t.Fatal("auth logs contained a value that must remain private")
	}
}

func TestMFARecoverySupportVerifiesPrimaryEmailAndRevokesSessions(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	sender := &captureEmailSender{}
	authHandler := testAuthHandler(fixture, sender, slog.Default())
	email := "mfa-support@example.com"
	userID := createVerifiedUser(t, fixture, email, "correct horse battery staple")
	if _, err := fixture.pool.Exec(context.Background(), `
		INSERT INTO auth_totp (user_id, secret_encrypted, enabled_at)
		VALUES ($1, 'fixture-secret', NOW())
	`, userID); err != nil {
		t.Fatalf("enable fixture MFA: %v", err)
	}

	request := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/request", map[string]string{
		"email": email,
	}, nil, "192.0.2.80:1000")
	if request.Code != http.StatusAccepted {
		t.Fatalf("MFA recovery support request status = %d, want accepted", request.Code)
	}
	code := sender.codeForSubject(t, "Verify your Askolo MFA recovery request")
	assertResponseDoesNotContain(t, request, email, code)
	if got := securityEventCount(t, fixture, userID, "mfa_recovery_support_challenge_sent"); got != 1 {
		t.Fatalf("MFA recovery support challenge-sent events = %d, want one", got)
	}

	sessionID, err := fixture.store.CreateSession(context.Background(), userID, "password", time.Hour)
	if err != nil {
		t.Fatalf("create session before MFA recovery support verification: %v", err)
	}
	sessionCookie := &http.Cookie{Name: "askolo.sid", Value: sessionID}
	invalidCode := "000001"
	if invalidCode == code {
		invalidCode = "000002"
	}
	invalid := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/verify", map[string]string{
		"email": email,
		"code":  invalidCode,
	}, sessionCookie, "192.0.2.82:1000")
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), `"code":"INVALID_RECOVERY"`) {
		t.Fatalf("invalid MFA recovery support verification status=%d has-safe-error-code=%t",
			invalid.Code, strings.Contains(invalid.Body.String(), `"code":"INVALID_RECOVERY"`))
	}
	if got := securityEventCount(t, fixture, userID, "mfa_recovery_support_verification_failed"); got != 1 {
		t.Fatalf("invalid MFA recovery support events = %d, want one", got)
	}

	verify := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/verify", map[string]string{
		"email": email,
		"code":  code,
	}, sessionCookie, "192.0.2.81:1000")
	if verify.Code != http.StatusOK || !strings.Contains(verify.Body.String(), "mfa_recovery_support_review_required") {
		t.Fatalf("MFA recovery support verification status=%d has-expected-state=%t",
			verify.Code, strings.Contains(verify.Body.String(), "mfa_recovery_support_review_required"))
	}
	if _, err := fixture.store.SessionUserID(context.Background(), sessionID); !errors.Is(err, postgres.ErrNotFound) {
		t.Fatalf("session after MFA recovery support verification error = %v, want session revoked", err)
	}
	if got := securityEventCount(t, fixture, userID, "mfa_recovery_support_verified"); got != 1 {
		t.Fatalf("MFA recovery support verified events = %d, want one", got)
	}
	var sessions int
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM sessions WHERE sess->>'userId' = $1
	`, userID).Scan(&sessions); err != nil {
		t.Fatalf("count sessions after MFA recovery support verification: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("MFA recovery support left %d active sessions, want zero", sessions)
	}
	for _, cookie := range verify.Result().Cookies() {
		if cookie.Name == "askolo.sid" && cookie.Value != "" {
			t.Fatalf("MFA recovery support verification issued a session cookie: %+v", cookie)
		}
	}
}

func TestMFARecoverySupportRejectsInactiveAccounts(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	sender := &captureEmailSender{}
	authHandler := testAuthHandler(fixture, sender, slog.Default())
	expectedVerification := `{"code":"INVALID_RECOVERY","error":"The recovery request is invalid or expired."}` + "\n"

	for _, status := range []string{"suspended", "deleted"} {
		t.Run(status, func(t *testing.T) {
			email := "mfa-" + status + "@example.com"
			userID := createVerifiedUser(t, fixture, email, "correct horse battery staple")
			if _, err := fixture.pool.Exec(context.Background(), `
INSERT INTO auth_totp (user_id, secret_encrypted, enabled_at)
VALUES ($1, 'fixture-secret', NOW())
`, userID); err != nil {
				t.Fatalf("enable fixture MFA: %v", err)
			}
			sessionID, err := fixture.store.CreateSession(context.Background(), userID, "password", time.Hour)
			if err != nil {
				t.Fatalf("create session for inactive account: %v", err)
			}
			if _, err := fixture.pool.Exec(context.Background(), `
UPDATE users SET status = $2 WHERE id = $1
`, userID, status); err != nil {
				t.Fatalf("mark account %s: %v", status, err)
			}

			request := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/request", map[string]string{
				"email": email,
			}, nil, "192.0.2.120:1000")
			if request.Code != http.StatusAccepted || request.Body.String() != `{"status":"mfa_recovery_if_available"}`+"\n" {
				t.Fatalf("inactive MFA recovery support request status=%d generic-response-matches=%t",
					request.Code, request.Body.String() == `{"status":"mfa_recovery_if_available"}`+"\n")
			}
			if sender.count() != 0 {
				t.Fatalf("inactive MFA recovery support request sent %d messages", sender.count())
			}

			code := "123456"
			createChallenge(t, fixture, NewHandler(fixture.authConfig, fixture.store, slog.Default()), userID, email, mfaRecoverySupportPurpose, code, time.Now().Add(emailChallengeTTL))
			verify := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/verify", map[string]string{
				"email": email,
				"code":  code,
			}, &http.Cookie{Name: fixture.authConfig.SessionCookieName, Value: sessionID}, "192.0.2.121:1000")
			if verify.Code != http.StatusBadRequest || verify.Body.String() != expectedVerification {
				t.Fatalf("inactive MFA recovery support verification status=%d generic-response-matches=%t",
					verify.Code, verify.Body.String() == expectedVerification)
			}
			if got := securityEventCount(t, fixture, userID, "mfa_recovery_support_verified"); got != 0 {
				t.Fatalf("inactive MFA recovery support verified events = %d, want zero", got)
			}
			if got, err := fixture.store.SessionUserID(context.Background(), sessionID); err != nil || got != userID {
				t.Fatalf("session after inactive MFA recovery support verification = user=%q err=%v, want user=%q", got, err, userID)
			}
			for _, cookie := range verify.Result().Cookies() {
				if cookie.Name == fixture.authConfig.SessionCookieName && cookie.Value != "" {
					t.Fatalf("inactive MFA recovery support verification issued a session cookie: %+v", cookie)
				}
			}
		})
	}
}

func TestMFARecoverySupportRejectsEnumerationAndPasswordOnlyPayloads(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	sender := &captureEmailSender{}
	authHandler := testAuthHandler(fixture, sender, slog.Default())
	unverifiedEmail := "mfa-unverified@example.com"
	if _, err := fixture.store.CreatePasswordUser(context.Background(), unverifiedEmail, "fixture-password-hash"); err != nil {
		t.Fatalf("create unverified fixture user: %v", err)
	}

	unknownRequest := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/request", map[string]string{
		"email": "mfa-unknown@example.com",
	}, nil, "192.0.2.90:1000")
	unverifiedRequest := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/request", map[string]string{
		"email": unverifiedEmail,
	}, nil, "192.0.2.91:1000")
	if unknownRequest.Code != http.StatusAccepted || unverifiedRequest.Code != http.StatusAccepted ||
		unknownRequest.Body.String() != `{"status":"mfa_recovery_if_available"}`+"\n" ||
		unverifiedRequest.Body.String() != unknownRequest.Body.String() {
		t.Fatalf("unknown/unverified MFA recovery response mismatch: unknown=%d unverified=%d bodies-match=%t",
			unknownRequest.Code, unverifiedRequest.Code, unknownRequest.Body.String() == unverifiedRequest.Body.String())
	}
	if sender.count() != 0 {
		t.Fatalf("unknown or unverified MFA recovery request sent %d messages", sender.count())
	}

	passwordOnly := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/request", map[string]string{
		"password": integrationPassword,
	}, nil, "192.0.2.92:1000")
	if passwordOnly.Code != http.StatusBadRequest || !strings.Contains(passwordOnly.Body.String(), `"code":"INVALID_REQUEST"`) {
		t.Fatalf("password-only MFA recovery request status=%d has-safe-error-code=%t",
			passwordOnly.Code, strings.Contains(passwordOnly.Body.String(), `"code":"INVALID_REQUEST"`))
	}
	if sender.count() != 0 {
		t.Fatalf("password-only MFA recovery request sent a message")
	}

	unknownVerification := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/verify", map[string]string{
		"email": "mfa-unknown@example.com",
		"code":  "000001",
	}, nil, "192.0.2.93:1000")
	unverifiedVerification := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/verify", map[string]string{
		"email": unverifiedEmail,
		"code":  "000001",
	}, nil, "192.0.2.94:1000")
	if unknownVerification.Code != http.StatusBadRequest ||
		unverifiedVerification.Code != unknownVerification.Code ||
		unverifiedVerification.Body.String() != unknownVerification.Body.String() {
		t.Fatalf("unknown/unverified MFA verification response mismatch: unknown=%d unverified=%d bodies-match=%t",
			unknownVerification.Code, unverifiedVerification.Code,
			unknownVerification.Body.String() == unverifiedVerification.Body.String())
	}
	if sender.count() != 0 {
		t.Fatalf("unknown or unverified MFA verification sent %d messages", sender.count())
	}
}

func TestMFARecoverySupportRateLimitsRequestAndVerification(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	authHandler := testAuthHandler(fixture, &captureEmailSender{}, slog.Default())

	for attempt := 1; attempt <= 3; attempt++ {
		response := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/request", map[string]string{
			"email": "rate-limit-unknown@example.com",
		}, nil, "192.0.2.100:1000")
		if response.Code != http.StatusAccepted {
			t.Fatalf("MFA recovery request attempt %d status = %d, want accepted", attempt, response.Code)
		}
	}
	requestLimited := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/request", map[string]string{
		"email": "rate-limit-unknown@example.com",
	}, nil, "192.0.2.100:1000")
	if requestLimited.Code != http.StatusTooManyRequests ||
		requestLimited.Header().Get("Retry-After") != "60" ||
		!strings.Contains(requestLimited.Body.String(), `"code":"RATE_LIMITED"`) {
		t.Fatalf("MFA recovery request rate limit=%d retry-after=%q has-safe-error-code=%t",
			requestLimited.Code, requestLimited.Header().Get("Retry-After"),
			strings.Contains(requestLimited.Body.String(), `"code":"RATE_LIMITED"`))
	}

	for attempt := 1; attempt <= 10; attempt++ {
		response := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/verify", map[string]string{
			"email": "rate-limit-unknown@example.com",
			"code":  "000001",
		}, nil, "192.0.2.101:1000")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("MFA recovery verification attempt %d status = %d, want bad request", attempt, response.Code)
		}
	}
	verificationLimited := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/verify", map[string]string{
		"email": "rate-limit-unknown@example.com",
		"code":  "000001",
	}, nil, "192.0.2.101:1000")
	if verificationLimited.Code != http.StatusTooManyRequests ||
		verificationLimited.Header().Get("Retry-After") != "60" ||
		!strings.Contains(verificationLimited.Body.String(), `"code":"RATE_LIMITED"`) {
		t.Fatalf("MFA recovery verification rate limit=%d retry-after=%q has-safe-error-code=%t",
			verificationLimited.Code, verificationLimited.Header().Get("Retry-After"),
			strings.Contains(verificationLimited.Body.String(), `"code":"RATE_LIMITED"`))
	}
}

func TestMFARecoverySupportRateLimitsAcrossInstancesConcurrently(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	first := testAuthHandler(fixture, &captureEmailSender{}, slog.Default())
	second := testAuthHandler(fixture, &captureEmailSender{}, slog.Default())
	const requestCount = 12
	results := make(chan int, requestCount)
	var waitGroup sync.WaitGroup

	for i := 0; i < requestCount; i++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			handler := first
			if index%2 == 1 {
				handler = second
			}
			response := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/recovery-support/request", map[string]string{
				"email": "cross-instance-unknown@example.com",
			}, nil, "192.0.2.200:1000")
			results <- response.Code
		}(i)
	}
	waitGroup.Wait()
	close(results)

	accepted, limited := 0, 0
	for status := range results {
		switch status {
		case http.StatusAccepted:
			accepted++
		case http.StatusTooManyRequests:
			limited++
		default:
			t.Fatalf("cross-instance MFA recovery request returned unexpected status %d", status)
		}
	}
	if accepted != mfaRecoveryRequestRateLimit || limited != requestCount-mfaRecoveryRequestRateLimit {
		t.Fatalf("cross-instance request limit accepted=%d limited=%d, want accepted=%d limited=%d",
			accepted, limited, mfaRecoveryRequestRateLimit, requestCount-mfaRecoveryRequestRateLimit)
	}

	const verificationCount = 16
	verificationResults := make(chan int, verificationCount)
	for i := 0; i < verificationCount; i++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			handler := first
			if index%2 == 1 {
				handler = second
			}
			response := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/recovery-support/verify", map[string]string{
				"email": "cross-instance-unknown@example.com",
				"code":  "000001",
			}, nil, "192.0.2.201:1000")
			verificationResults <- response.Code
		}(i)
	}
	waitGroup.Wait()
	close(verificationResults)

	invalid, verificationLimited := 0, 0
	for status := range verificationResults {
		switch status {
		case http.StatusBadRequest:
			invalid++
		case http.StatusTooManyRequests:
			verificationLimited++
		default:
			t.Fatalf("cross-instance MFA recovery verification returned unexpected status %d", status)
		}
	}
	if invalid != mfaRecoveryVerificationRateLimit ||
		verificationLimited != verificationCount-mfaRecoveryVerificationRateLimit {
		t.Fatalf("cross-instance verification limit invalid=%d limited=%d, want invalid=%d limited=%d",
			invalid, verificationLimited, mfaRecoveryVerificationRateLimit, verificationCount-mfaRecoveryVerificationRateLimit)
	}

	var storedColumns string
	if err := fixture.pool.QueryRow(context.Background(), `
SELECT string_agg(column_name, ',' ORDER BY ordinal_position)
FROM information_schema.columns
WHERE table_schema = current_schema()
  AND table_name = 'auth_mfa_recovery_rate_limits'
`).Scan(&storedColumns); err != nil {
		t.Fatalf("inspect MFA recovery rate-limit state: %v", err)
	}
	if storedColumns != "bucket_hash,window_started_at,request_count" {
		t.Fatalf("MFA recovery rate-limit state exposed unexpected columns: %q", storedColumns)
	}
}

func TestMFARecoveryRateLimitsAcrossProcesses(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	handler := testAuthHandler(fixture, &captureEmailSender{}, slog.Default())
	const address = "mfa-process-limits@example.com"
	const remote = "192.0.2.250:1000"

	for attempt := 0; attempt < mfaRecoveryRequestRateLimit; attempt++ {
		response := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/recovery/request", map[string]string{
			"email": address, "currentPassword": "invalid-password",
		}, nil, remote)
		if response.Code != http.StatusAccepted {
			t.Fatalf("request limit setup attempt %d status=%d", attempt+1, response.Code)
		}
	}
	runMFARecoveryRateLimitProcess(t, fixture.baseURL, "request", address, remote)

	for attempt := 0; attempt < mfaRecoveryVerificationRateLimit; attempt++ {
		response := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/recovery/verify", map[string]string{
			"email": address, "currentPassword": "invalid-password", "code": "000001",
		}, nil, remote)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("verification limit setup attempt %d status=%d", attempt+1, response.Code)
		}
	}
	runMFARecoveryRateLimitProcess(t, fixture.baseURL, "verification", address, remote)
}

func TestMFARecoveryRateLimitProcessHelper(t *testing.T) {
	if os.Getenv("ASKOLO_MFA_RECOVERY_PROCESS_HELPER") != "1" {
		t.Skip("subprocess helper")
	}
	databaseURL := os.Getenv("ASKOLO_MFA_RECOVERY_DATABASE_URL")
	mode := os.Getenv("ASKOLO_MFA_RECOVERY_PROCESS_MODE")
	address := os.Getenv("ASKOLO_MFA_RECOVERY_PROCESS_EMAIL")
	remote := os.Getenv("ASKOLO_MFA_RECOVERY_PROCESS_REMOTE")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := postgres.New(ctx, databaseURL)
	if err != nil {
		t.Fatal("subprocess could not open shared test database")
	}
	defer store.Close()
	fixture := &emailAuthFixture{
		store: store,
		authConfig: config.Config{
			Environment:             "test",
			SessionCookieName:       "askolo.sid",
			AuthRateLimitHMACSecret: "integration-only-auth-rate-limit-hmac-secret-at-least-32-bytes",
			TurnstileSecret:         "integration-only-turnstile-secret",
			TurnstileAllowedHostnames: map[string]struct{}{
				"web.askolo.app": {},
			},
			Email: config.EmailConfig{
				ChallengeSecret: "integration-only-challenge-secret",
			},
		},
	}
	handler := testAuthHandler(fixture, &captureEmailSender{}, slog.Default())
	var response *httptest.ResponseRecorder
	if mode == "request" {
		response = jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/recovery/request", map[string]string{
			"email": address, "currentPassword": "invalid-password",
		}, nil, remote)
	} else {
		response = jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/recovery/verify", map[string]string{
			"email": address, "currentPassword": "invalid-password", "code": "000001",
		}, nil, remote)
	}
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("shared recovery throttle did not apply in subprocess: status=%d", response.Code)
	}
}

func runMFARecoveryRateLimitProcess(t *testing.T, databaseURL, mode, address, remote string) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestMFARecoveryRateLimitProcessHelper$")
	command.Env = append(os.Environ(),
		"ASKOLO_MFA_RECOVERY_PROCESS_HELPER=1",
		"ASKOLO_MFA_RECOVERY_DATABASE_URL="+databaseURL,
		"ASKOLO_MFA_RECOVERY_PROCESS_MODE="+mode,
		"ASKOLO_MFA_RECOVERY_PROCESS_EMAIL="+address,
		"ASKOLO_MFA_RECOVERY_PROCESS_REMOTE="+remote,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("separate-process MFA %s throttle check failed: %s", mode, strings.TrimSpace(string(output)))
	}
}

func TestPasswordLoginAccountLimitIsSharedAcrossInstances(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	first := testAuthHandler(fixture, &captureEmailSender{}, slog.Default())
	second := testAuthHandler(fixture, &captureEmailSender{}, slog.Default())
	email := "shared-limit-unknown@example.com"

	for attempt := 1; attempt <= passwordLoginAccountRateLimit+1; attempt++ {
		handler := first
		if attempt%2 == 0 {
			handler = second
		}
		response := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/login", map[string]string{
			"email":    email,
			"password": "incorrect-password",
		}, nil, fmt.Sprintf("198.51.100.%d:1000", attempt))

		want := http.StatusUnauthorized
		if attempt > passwordLoginAccountRateLimit {
			want = http.StatusTooManyRequests
		}
		if response.Code != want {
			t.Fatalf("login attempt %d returned %d, want %d", attempt, response.Code, want)
		}
	}
}

func TestPasswordRecoveryIsNotBlockedByLoginAttempts(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	handler := testAuthHandler(fixture, &captureEmailSender{}, slog.Default())
	email := "mixed-flow-unknown@example.com"
	remoteAddress := "198.51.100.84:1000"

	for attempt := 1; attempt <= 5; attempt++ {
		response := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/login", map[string]string{
			"email":    email,
			"password": "incorrect-password",
		}, nil, remoteAddress)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("login attempt %d returned %d, want %d", attempt, response.Code, http.StatusUnauthorized)
		}
	}

	recovery := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
		"email":  email,
		"method": passwordRecoveryPrimaryEmail,
	}, nil, remoteAddress)
	if recovery.Code != http.StatusAccepted ||
		recovery.Body.String() != `{"status":"recovery_if_available"}`+"\n" {
		t.Fatalf("recovery after login attempts returned status=%d body=%q",
			recovery.Code, recovery.Body.String())
	}
}

func TestPasswordRecoveryAccountLimitKeepsGenericResponse(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	first := testAuthHandler(fixture, &captureEmailSender{}, slog.Default())
	second := testAuthHandler(fixture, &captureEmailSender{}, slog.Default())
	email := "shared-recovery-unknown@example.com"

	for attempt := 1; attempt <= passwordRecoveryAccountRateLimit+1; attempt++ {
		handler := first
		if attempt%2 == 0 {
			handler = second
		}
		response := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
			"email":  email,
			"method": passwordRecoveryPrimaryEmail,
		}, nil, fmt.Sprintf("198.51.100.%d:1000", attempt+10))
		if response.Code != http.StatusAccepted ||
			response.Body.String() != `{"status":"recovery_if_available"}`+"\n" {
			t.Fatalf("recovery attempt %d exposed a different response: status=%d body=%q",
				attempt, response.Code, response.Body.String())
		}
	}
}

func TestAuthRateLimitBucketsExpireAfterRetention(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	ctx := context.Background()
	const expiredBucket = "expired-auth-rate-limit-bucket"
	if _, err := fixture.pool.Exec(ctx, `
INSERT INTO auth_mfa_recovery_rate_limits (bucket_hash, window_started_at, request_count)
VALUES ($1, NOW() - INTERVAL '25 hours', 1)
`, expiredBucket); err != nil {
		t.Fatalf("insert expired rate-limit bucket: %v", err)
	}

	handler := NewHandler(fixture.authConfig, fixture.store, slog.Default())
	freshBucket, err := handler.authRateLimitBucketHash("account:test", "fresh@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if allowed, err := fixture.store.AllowAuthRateLimitBucket(ctx, freshBucket, 1, time.Minute); err != nil || !allowed {
		t.Fatalf("consume fresh bucket: allowed=%v err=%v", allowed, err)
	}
	var exists bool
	if err := fixture.pool.QueryRow(ctx, `
SELECT EXISTS (SELECT 1 FROM auth_mfa_recovery_rate_limits WHERE bucket_hash = $1)
`, expiredBucket).Scan(&exists); err != nil {
		t.Fatalf("check expired rate-limit bucket: %v", err)
	}
	if exists {
		t.Fatal("expired rate-limit bucket was retained past the cleanup run")
	}
}

func TestNativeEmailAuthAbuseFailureAndConcurrencyGuarantees(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	sender := &captureEmailSender{}
	authHandler := testAuthHandler(fixture, sender, slog.Default())
	userID := createVerifiedUser(t, fixture, integrationEmail, integrationPassword)

	wrongCode := "000001"
	createChallenge(t, fixture, NewHandler(fixture.authConfig, fixture.store, slog.Default()), userID, integrationEmail, "email_verification", "123456", time.Now().Add(emailChallengeTTL))
	for attempt := 0; attempt < emailChallengeMaxAttempts; attempt++ {
		response := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/email/verify", map[string]string{
			"email": integrationEmail, "code": wrongCode,
		}, nil, fmt.Sprintf("192.0.2.%d:2000", 30+attempt))
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid attempt %d status = %d, want bad request", attempt+1, response.Code)
		}
	}
	locked := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/email/verify", map[string]string{
		"email": integrationEmail, "code": wrongCode,
	}, nil, "192.0.2.40:2000")
	if locked.Code != http.StatusTooManyRequests || !strings.Contains(locked.Body.String(), `"code":"CHALLENGE_LOCKED"`) {
		t.Fatalf("locked challenge response status=%d has-safe-error-code=%t",
			locked.Code, strings.Contains(locked.Body.String(), `"code":"CHALLENGE_LOCKED"`))
	}

	expiredEmail := "expired@example.com"
	expiredUser := createVerifiedUser(t, fixture, expiredEmail, integrationPassword)
	createChallenge(t, fixture, NewHandler(fixture.authConfig, fixture.store, slog.Default()), expiredUser, expiredEmail, "email_verification", "123456", time.Now().Add(-time.Minute))
	expired := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/email/verify", map[string]string{
		"email": expiredEmail, "code": "123456",
	}, nil, "192.0.2.41:2000")
	if expired.Code != http.StatusBadRequest {
		t.Fatalf("expired challenge status = %d, want bad request", expired.Code)
	}
	assertResponseDoesNotContain(t, expired, expiredEmail, "123456", integrationPassword)

	concurrentEmail := "concurrent@example.com"
	concurrentUser := createVerifiedUser(t, fixture, concurrentEmail, integrationPassword)
	createChallenge(t, fixture, NewHandler(fixture.authConfig, fixture.store, slog.Default()), concurrentUser, concurrentEmail, "email_verification", "654321", time.Now().Add(emailChallengeTTL))
	const consumers = 12
	results := make(chan int, consumers)
	var waitGroup sync.WaitGroup
	for i := 0; i < consumers; i++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			response := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/email/verify", map[string]string{
				"email": concurrentEmail, "code": "654321",
			}, nil, fmt.Sprintf("198.51.100.%d:3000", index+1))
			results <- response.Code
		}(i)
	}
	waitGroup.Wait()
	close(results)
	successes := 0
	for status := range results {
		if status == http.StatusOK {
			successes++
		} else if status != http.StatusBadRequest {
			t.Fatalf("concurrent challenge consumption returned unexpected status %d", status)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent challenge consumption succeeded %d times, want exactly once", successes)
	}

	concurrentRecoveryEmail := "recovery-concurrent@example.com"
	concurrentRecoveryUser := createVerifiedUser(t, fixture, concurrentRecoveryEmail, integrationPassword)
	if _, err := fixture.pool.Exec(context.Background(), `
		INSERT INTO auth_recovery_methods (id, user_id, kind, address, verified_at)
		VALUES ('recovery-method-concurrent', $1, 'email', $2, NOW())
	`, concurrentRecoveryUser, concurrentRecoveryEmail); err != nil {
		t.Fatalf("create concurrent recovery method: %v", err)
	}
	concurrentSender := &captureEmailSender{latency: 50 * time.Millisecond}
	concurrentRecoveryHandler := testAuthHandler(fixture, concurrentSender, slog.Default())
	const recoveryRequests = 8
	recoveryResults := make(chan int, recoveryRequests)
	for i := 0; i < recoveryRequests; i++ {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			response := jsonRequest(t, concurrentRecoveryHandler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
				"email": concurrentRecoveryEmail,
			}, nil, fmt.Sprintf("203.0.113.%d:4000", index+1))
			recoveryResults <- response.Code
		}(i)
	}
	waitGroup.Wait()
	close(recoveryResults)
	accepted := 0
	for status := range recoveryResults {
		if status == http.StatusAccepted {
			accepted++
		} else {
			t.Fatalf("concurrent recovery request returned unexpected status %d", status)
		}
	}
	if accepted != recoveryRequests || concurrentSender.count() != 1 {
		t.Fatalf("concurrent recovery responses accepted=%d deliveries=%d, want %d generic responses and one delivery",
			accepted, concurrentSender.count(), recoveryRequests)
	}

	failureEmail := "delivery-failure@example.com"
	failureSender := &captureEmailSender{
		err:         errors.New("capture-only delivery failure"),
		leakInError: true,
	}
	var failureLogs bytes.Buffer
	failureHandler := testAuthHandler(fixture, failureSender, slog.New(slog.NewTextHandler(&failureLogs, nil)))
	failure := jsonRequest(t, failureHandler, http.MethodPost, "/api/auth/password/signup", map[string]string{
		"email": failureEmail, "password": integrationPassword,
	}, nil, "192.0.2.50:5000")
	if failure.Code != http.StatusServiceUnavailable || !strings.Contains(failure.Body.String(), `"code":"AUTH_UNAVAILABLE"`) {
		t.Fatalf("delivery failure status=%d has-safe-error-code=%t",
			failure.Code, strings.Contains(failure.Body.String(), `"code":"AUTH_UNAVAILABLE"`))
	}
	assertResponseDoesNotContain(t, failure, failureEmail, integrationPassword)
	var persisted int
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM auth_email_challenges WHERE email = $1 AND purpose = 'email_verification'
	`, failureEmail).Scan(&persisted); err != nil {
		t.Fatalf("count persisted delivery-failure challenge: %v", err)
	}
	if persisted != 0 {
		t.Fatalf("delivery failure persisted %d challenges, want zero", persisted)
	}
	failureCode := failureSender.codeForSubject(t, "Verify your Askolo email")
	if strings.Contains(failureLogs.String(), failureEmail) ||
		strings.Contains(failureLogs.String(), integrationPassword) ||
		strings.Contains(failureLogs.String(), failureCode) {
		t.Fatal("delivery failure logs contained a value that must remain private")
	}
	if !strings.Contains(failureLogs.String(), "failure_category=provider_rejection") {
		t.Fatal("delivery failure logs omitted the safe failure category")
	}

	retrySender := &captureEmailSender{}
	retryHandler := testAuthHandler(fixture, retrySender, slog.Default())
	retry := jsonRequest(t, retryHandler, http.MethodPost, "/api/auth/email/resend", map[string]string{
		"email": failureEmail,
	}, nil, "192.0.2.51:5000")
	if retry.Code != http.StatusAccepted {
		t.Fatalf("delivery retry status = %d, want accepted", retry.Code)
	}
	if retrySender.count() != 1 {
		t.Fatalf("delivery retry sent %d messages, want one", retrySender.count())
	}
}

func TestNativeEmailSignupAndExplicitVerificationResendInvokeSenderOnce(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	sender := &captureEmailSender{}
	monitor := NewEmailDeliveryMonitor()
	handler := testAuthHandlerWithMonitor(
		fixture.authConfig,
		fixture.store,
		slog.Default(),
		sender,
		monitor,
	).Routes()

	signup := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/signup", map[string]string{
		"email":    integrationEmail,
		"password": integrationPassword,
	}, nil, "192.0.2.60:6000")
	if signup.Code != http.StatusAccepted || sender.count() != 1 {
		t.Fatalf("fresh signup status=%d sender calls=%d; want accepted and exactly one send",
			signup.Code, sender.count())
	}

	afterSignup := monitor.Snapshot()
	if afterSignup.Attempts != 1 || afterSignup.Handoffs != 1 {
		t.Fatalf("fresh signup delivery snapshot attempts=%d handoffs=%d; want 1 and 1",
			afterSignup.Attempts, afterSignup.Handoffs)
	}
	signupCode := sender.codeForSubject(t, "Verify your Askolo email")
	assertResponseDoesNotContain(t, signup, integrationEmail, signupCode, integrationPassword)

	duplicate := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/signup", map[string]string{
		"email": integrationEmail, "password": "a different password",
	}, nil, "192.0.2.61:6000")
	if duplicate.Code != http.StatusAccepted ||
		duplicate.Body.String() != `{"status":"verification_required"}`+"\n" ||
		sender.count() != 1 {
		t.Fatalf("duplicate signup status=%d sender calls=%d; want generic accepted response and no additional send",
			duplicate.Code, sender.count())
	}
	afterDuplicate := monitor.Snapshot()
	if afterDuplicate.Attempts != 1 || afterDuplicate.Handoffs != 1 {
		t.Fatalf("duplicate signup changed delivery attempts to %d handoffs to %d; want both unchanged at 1",
			afterDuplicate.Attempts, afterDuplicate.Handoffs)
	}
	assertResponseDoesNotContain(t, duplicate, integrationEmail, "a different password")

	var userID string
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT id FROM users WHERE email = $1
	`, integrationEmail).Scan(&userID); err != nil {
		t.Fatalf("lookup fresh signup user: %v", err)
	}
	if result, err := fixture.pool.Exec(context.Background(), `
		UPDATE auth_email_challenges
		SET created_at = NOW() - INTERVAL '3 minutes'
		WHERE user_id = $1 AND purpose = 'email_verification' AND consumed_at IS NULL
	`, userID); err != nil {
		t.Fatalf("age verification challenge for resend test: %v", err)
	} else if result.RowsAffected() != 1 {
		t.Fatalf("aged %d active verification challenges, want exactly one", result.RowsAffected())
	}

	resend := jsonRequest(t, handler, http.MethodPost, "/api/auth/email/resend", map[string]string{
		"email": integrationEmail,
	}, nil, "192.0.2.62:6000")
	if resend.Code != http.StatusAccepted || sender.count() != 2 {
		t.Fatalf("explicit resend status=%d sender calls=%d; want accepted and exactly one additional send",
			resend.Code, sender.count())
	}
	afterResend := monitor.Snapshot()
	if afterResend.Attempts != 2 || afterResend.Handoffs != 2 {
		t.Fatalf("explicit resend delivery snapshot attempts=%d handoffs=%d; want 2 and 2",
			afterResend.Attempts, afterResend.Handoffs)
	}
	resendCode := sender.codeForSubject(t, "Verify your Askolo email")
	assertResponseDoesNotContain(t, resend, integrationEmail, resendCode, integrationPassword)

	verify := jsonRequest(t, handler, http.MethodPost, "/api/auth/email/verify", map[string]string{
		"email": integrationEmail,
		"code":  resendCode,
	}, nil, "192.0.2.63:6000")
	if verify.Code != http.StatusOK {
		t.Fatalf("verification after resend status=%d, want success", verify.Code)
	}
	assertResponseDoesNotContain(t, verify, integrationEmail, resendCode, integrationPassword)
}

func TestEmailVerificationResendSupportsActiveUnverifiedLegacyUser(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	const email = "active-unverified@example.com"
	passwordHash, err := HashPassword(integrationPassword)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}
	userID, err := fixture.store.CreatePasswordUser(context.Background(), email, passwordHash)
	if err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	if _, err := fixture.pool.Exec(context.Background(), `
		UPDATE users
		SET status = 'active', email_verified_at = NULL, account_created_via = ''
		WHERE id = $1
	`, userID); err != nil {
		t.Fatalf("prepare active unverified user: %v", err)
	}
	if _, err := fixture.pool.Exec(context.Background(), `
		DELETE FROM auth_passwords WHERE user_id = $1
	`, userID); err != nil {
		t.Fatalf("remove fixture password identity: %v", err)
	}

	sender := &captureEmailSender{}
	handler := testAuthHandler(fixture, sender, slog.Default())
	resend := jsonRequest(t, handler, http.MethodPost, "/api/auth/email/resend", map[string]string{
		"email": email,
	}, nil, "192.0.2.64:6000")
	if resend.Code != http.StatusAccepted || sender.count() != 1 {
		t.Fatalf("active unverified resend status=%d sender calls=%d; want accepted and one send",
			resend.Code, sender.count())
	}
	code := sender.codeForSubject(t, "Verify your Askolo email")
	assertResponseDoesNotContain(t, resend, email, code, integrationPassword)

	verify := jsonRequest(t, handler, http.MethodPost, "/api/auth/email/verify", map[string]string{
		"email": email,
		"code":  code,
	}, nil, "192.0.2.65:6000")
	if verify.Code != http.StatusOK {
		t.Fatalf("active unverified account verification status=%d, want success", verify.Code)
	}
	assertResponseDoesNotContain(t, verify, email, code, integrationPassword)

	recoveryRequest := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
		"email": email, "method": "primary_email",
	}, nil, "192.0.2.67:6000")
	if recoveryRequest.Code != http.StatusAccepted || sender.count() != 2 {
		t.Fatalf("initial-password recovery status=%d sender calls=%d; want accepted and one recovery email",
			recoveryRequest.Code, sender.count())
	}
	recoveryCode := sender.codeForSubject(t, "Reset your Askolo password")
	recoveryVerify := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/recovery/verify", map[string]string{
		"email": email, "method": "primary_email", "code": recoveryCode,
	}, nil, "192.0.2.68:6000")
	if recoveryVerify.Code != http.StatusOK {
		t.Fatalf("initial-password recovery verification status=%d, want success", recoveryVerify.Code)
	}
	resetPassword := "new active account password"
	reset := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/recovery/reset", map[string]string{
		"email": email, "method": "primary_email", "code": recoveryCode, "password": resetPassword,
	}, nil, "192.0.2.69:6000")
	if reset.Code != http.StatusOK {
		t.Fatalf("initial-password setup status=%d, want success", reset.Code)
	}
	assertResponseDoesNotContain(t, recoveryRequest, email, recoveryCode, resetPassword)
	assertResponseDoesNotContain(t, recoveryVerify, email, recoveryCode, resetPassword)
	assertResponseDoesNotContain(t, reset, email, recoveryCode, resetPassword)
	login := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/login", map[string]string{
		"email": email, "password": resetPassword,
	}, nil, "192.0.2.70:6000")
	if login.Code != http.StatusOK || login.Header().Get("Set-Cookie") == "" {
		t.Fatalf("login after initial password setup status=%d has-session-cookie=%t; want authenticated",
			login.Code, login.Header().Get("Set-Cookie") != "")
	}
	assertResponseDoesNotContain(t, login, email, resetPassword)

	var status, accountCreatedVia string
	var emailVerified, hasPassword bool
	var welcomeGrants int
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT u.status, (u.email_verified_at IS NOT NULL), COALESCE(u.account_created_via, ''),
		       EXISTS (SELECT 1 FROM auth_passwords p WHERE p.user_id = u.id),
		       (SELECT count(*) FROM ai_credit_grants g
		        WHERE g.user_id = u.id AND g.idempotency_key = 'welcome-credit-usd-v1')
		FROM users u
		WHERE u.id = $1
	`, userID).Scan(&status, &emailVerified, &accountCreatedVia, &hasPassword, &welcomeGrants); err != nil {
		t.Fatalf("read verified legacy account state: %v", err)
	}
	if status != "active" || !emailVerified || accountCreatedVia != "" || !hasPassword {
		t.Fatalf("verified legacy account state = status:%q verified:%t source:%q password:%t",
			status, emailVerified, accountCreatedVia, hasPassword)
	}
	if welcomeGrants != 0 {
		t.Fatalf("active legacy verification granted signup welcome credit %d times, want zero", welcomeGrants)
	}

	verifiedResend := jsonRequest(t, handler, http.MethodPost, "/api/auth/email/resend", map[string]string{
		"email": email,
	}, nil, "192.0.2.66:6000")
	if verifiedResend.Code != http.StatusAccepted || sender.count() != 2 {
		t.Fatalf("verified account resend status=%d sender calls=%d; want accepted and no additional send",
			verifiedResend.Code, sender.count())
	}
	assertResponseDoesNotContain(t, verifiedResend, email, code, recoveryCode, resetPassword)
}

func TestPasswordRecoveryVerifiesActiveUnverifiedEmailAndSetsFirstPassword(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	const email = "active-unverified-first-password@example.com"
	passwordHash, err := HashPassword(integrationPassword)
	if err != nil {
		t.Fatalf("hash fixture password: %v", err)
	}
	userID, err := fixture.store.CreatePasswordUser(context.Background(), email, passwordHash)
	if err != nil {
		t.Fatalf("create fixture user: %v", err)
	}
	if _, err := fixture.pool.Exec(context.Background(), `
		UPDATE users
		SET status = 'active', email_verified_at = NULL, account_created_via = ''
		WHERE id = $1
	`, userID); err != nil {
		t.Fatalf("prepare active unverified user: %v", err)
	}
	if _, err := fixture.pool.Exec(context.Background(), `
		DELETE FROM auth_passwords WHERE user_id = $1
	`, userID); err != nil {
		t.Fatalf("remove fixture password identity: %v", err)
	}

	sender := &captureEmailSender{}
	handler := testAuthHandler(fixture, sender, slog.Default())
	recoveryRequest := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
		"email": email, "method": passwordRecoveryPrimaryEmail,
	}, nil, "192.0.2.81:6000")
	if recoveryRequest.Code != http.StatusAccepted ||
		recoveryRequest.Body.String() != `{"status":"recovery_if_available"}`+"\n" ||
		sender.count() != 1 {
		t.Fatalf("unverified passwordless recovery status=%d body=%q sender calls=%d",
			recoveryRequest.Code, recoveryRequest.Body.String(), sender.count())
	}
	code := sender.codeForSubject(t, "Reset your Askolo password")
	if messages := sender.snapshot(); len(messages) != 1 || messages[0].To != email {
		t.Fatalf("recovery message was not sent to the primary email")
	}
	assertResponseDoesNotContain(t, recoveryRequest, email, code, integrationPassword)

	wrongCode := "000000"
	if wrongCode == code {
		wrongCode = "000001"
	}
	invalidVerify := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/recovery/verify", map[string]string{
		"email": email, "method": passwordRecoveryPrimaryEmail, "code": wrongCode,
	}, nil, "192.0.2.82:6000")
	if invalidVerify.Code != http.StatusBadRequest ||
		!strings.Contains(invalidVerify.Body.String(), `"code":"INVALID_RESET"`) {
		t.Fatalf("invalid recovery code status=%d body=%q", invalidVerify.Code, invalidVerify.Body.String())
	}

	recoveryVerify := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/recovery/verify", map[string]string{
		"email": email, "method": passwordRecoveryPrimaryEmail, "code": code,
	}, nil, "192.0.2.83:6000")
	if recoveryVerify.Code != http.StatusOK {
		t.Fatalf("recovery code verification status=%d, want success", recoveryVerify.Code)
	}
	var verifiedBeforeReset bool
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT email_verified_at IS NOT NULL FROM users WHERE id = $1
	`, userID).Scan(&verifiedBeforeReset); err != nil {
		t.Fatalf("check verification before password reset: %v", err)
	}
	if verifiedBeforeReset {
		t.Fatal("non-consuming recovery verification confirmed the email before reset")
	}

	resetPassword := "new active account password"
	reset := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/recovery/reset", map[string]string{
		"email": email, "method": passwordRecoveryPrimaryEmail, "code": code, "password": resetPassword,
	}, nil, "192.0.2.84:6000")
	if reset.Code != http.StatusOK {
		t.Fatalf("first-password setup status=%d body=%q", reset.Code, reset.Body.String())
	}
	assertResponseDoesNotContain(t, recoveryVerify, email, code, resetPassword)
	assertResponseDoesNotContain(t, reset, email, code, resetPassword)

	var status, accountCreatedVia string
	var emailVerified, hasPassword bool
	var welcomeGrants int
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT u.status, (u.email_verified_at IS NOT NULL), COALESCE(u.account_created_via, ''),
		       EXISTS (SELECT 1 FROM auth_passwords p WHERE p.user_id = u.id),
		       (SELECT count(*) FROM ai_credit_grants g
		        WHERE g.user_id = u.id AND g.idempotency_key = 'welcome-credit-usd-v1')
		FROM users u
		WHERE u.id = $1
	`, userID).Scan(&status, &emailVerified, &accountCreatedVia, &hasPassword, &welcomeGrants); err != nil {
		t.Fatalf("read recovered account state: %v", err)
	}
	if status != "active" || !emailVerified || accountCreatedVia != "" || !hasPassword {
		t.Fatalf("recovered account state = status:%q verified:%t source:%q password:%t",
			status, emailVerified, accountCreatedVia, hasPassword)
	}
	if welcomeGrants != 0 {
		t.Fatalf("password recovery granted signup welcome credit %d times, want zero", welcomeGrants)
	}

	login := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/login", map[string]string{
		"email": email, "password": resetPassword,
	}, nil, "192.0.2.85:6000")
	if login.Code != http.StatusOK || login.Header().Get("Set-Cookie") == "" {
		t.Fatalf("login after initial password setup status=%d has-session-cookie=%t; want authenticated",
			login.Code, login.Header().Get("Set-Cookie") != "")
	}
	assertResponseDoesNotContain(t, login, email, resetPassword)

	reusedCode := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/recovery/reset", map[string]string{
		"email": email, "method": passwordRecoveryPrimaryEmail, "code": code, "password": "another active account password",
	}, nil, "192.0.2.86:6000")
	if reusedCode.Code != http.StatusBadRequest {
		t.Fatalf("consumed recovery code reuse status=%d, want invalid request", reusedCode.Code)
	}
}

func TestEmailChallengeCleanupRetainsRecentAndActiveRecords(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	oldTerminalAt := now.Add(-postgres.EmailChallengeRetention - time.Hour)
	recentTerminalAt := now.Add(-time.Hour)

	type challengeFixture struct {
		id           string
		purpose      string
		expiresAt    time.Time
		consumedAt   *time.Time
		shouldDelete bool
	}
	oldConsumedExpiry := now.Add(time.Hour)
	challenges := []challengeFixture{
		{id: "cleanup-expired-verification", purpose: "email_verification", expiresAt: oldTerminalAt, shouldDelete: true},
		{id: "cleanup-expired-recovery-enrollment", purpose: "recovery_email_enrollment", expiresAt: oldTerminalAt, shouldDelete: true},
		{id: "cleanup-expired-password-recovery", purpose: "password_recovery", expiresAt: oldTerminalAt, shouldDelete: true},
		{id: "cleanup-consumed-verification", purpose: "email_verification", expiresAt: oldConsumedExpiry, consumedAt: timePtr(oldTerminalAt), shouldDelete: true},
		{id: "cleanup-consumed-recovery-enrollment", purpose: "recovery_email_enrollment", expiresAt: oldConsumedExpiry, consumedAt: timePtr(oldTerminalAt), shouldDelete: true},
		{id: "cleanup-consumed-password-recovery", purpose: "password_recovery", expiresAt: oldConsumedExpiry, consumedAt: timePtr(oldTerminalAt), shouldDelete: true},
		{id: "cleanup-recent-expired", purpose: "email_verification", expiresAt: recentTerminalAt},
		{id: "cleanup-recent-consumed", purpose: "password_recovery", expiresAt: oldConsumedExpiry, consumedAt: timePtr(recentTerminalAt)},
		{id: "cleanup-active", purpose: "recovery_email_enrollment", expiresAt: now.Add(time.Hour)},
		{id: "cleanup-unrelated-purpose", purpose: "unrelated_auth_purpose", expiresAt: oldTerminalAt},
	}
	for _, challenge := range challenges {
		if _, err := fixture.pool.Exec(ctx, `
			INSERT INTO auth_email_challenges (id, email, purpose, code_hash, expires_at, consumed_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, challenge.id, challenge.id+"@example.com", challenge.purpose, "hash-"+challenge.id, challenge.expiresAt, challenge.consumedAt); err != nil {
			t.Fatalf("insert %s: %v", challenge.id, err)
		}
	}

	deleted, err := fixture.store.CleanupEmailChallenges(ctx, now, 2)
	if err != nil {
		t.Fatalf("bounded cleanup: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("bounded cleanup deleted %d rows, want 2", deleted)
	}

	for {
		deleted, err = fixture.store.CleanupEmailChallenges(ctx, now, postgres.EmailChallengeCleanupBatchSize)
		if err != nil {
			t.Fatalf("drain cleanup: %v", err)
		}
		if deleted == 0 {
			break
		}
	}

	for _, challenge := range challenges {
		var count int
		if err := fixture.pool.QueryRow(ctx, `SELECT count(*) FROM auth_email_challenges WHERE id = $1`, challenge.id).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", challenge.id, err)
		}
		if challenge.shouldDelete && count != 0 {
			t.Errorf("terminal challenge %s remains", challenge.id)
		}
		if !challenge.shouldDelete && count != 1 {
			t.Errorf("protected challenge %s was deleted", challenge.id)
		}
	}
}

func TestEmailChallengeCleanupSkipsLockedTerminalRecord(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	lockedTerminalAt := now.Add(-postgres.EmailChallengeRetention - 2*time.Hour)
	otherTerminalAt := now.Add(-postgres.EmailChallengeRetention - time.Hour)
	lockedID := "cleanup-locked-terminal"
	otherID := "cleanup-available-terminal"

	for _, challenge := range []struct {
		id        string
		expiresAt time.Time
	}{
		{id: lockedID, expiresAt: lockedTerminalAt},
		{id: otherID, expiresAt: otherTerminalAt},
	} {
		if _, err := fixture.pool.Exec(ctx, `
INSERT INTO auth_email_challenges (id, email, purpose, code_hash, expires_at)
VALUES ($1, $2, 'email_verification', $3, $4)
`, challenge.id, challenge.id+"@example.com", "hash-"+challenge.id, challenge.expiresAt); err != nil {
			t.Fatalf("insert %s: %v", challenge.id, err)
		}
	}

	verificationTx, err := fixture.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin verification transaction: %v", err)
	}
	defer verificationTx.Rollback(ctx)

	var selectedID string
	if err := verificationTx.QueryRow(ctx, `
SELECT id
FROM auth_email_challenges
WHERE id = $1
FOR UPDATE
`, lockedID).Scan(&selectedID); err != nil {
		t.Fatalf("lock terminal challenge: %v", err)
	}
	if selectedID != lockedID {
		t.Fatalf("locked challenge = %q, want %q", selectedID, lockedID)
	}

	cleanupCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	deleted, err := fixture.store.CleanupEmailChallenges(cleanupCtx, now, 1)
	if err != nil {
		t.Fatalf("cleanup while verification holds lock: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("cleanup deleted %d rows while locked row was skipped, want 1", deleted)
	}

	var exists bool
	if err := fixture.pool.QueryRow(ctx, `
SELECT EXISTS (
SELECT 1
FROM auth_email_challenges
WHERE id = $1
)
`, lockedID).Scan(&exists); err != nil {
		t.Fatalf("check locked challenge after cleanup: %v", err)
	}
	if !exists {
		t.Fatal("cleanup deleted the challenge held by the active verification transaction")
	}
	if err := fixture.pool.QueryRow(ctx, `
SELECT EXISTS (
SELECT 1
FROM auth_email_challenges
WHERE id = $1
)
`, otherID).Scan(&exists); err != nil {
		t.Fatalf("check available challenge after cleanup: %v", err)
	}
	if exists {
		t.Fatal("cleanup did not delete the available terminal challenge")
	}

	if err := verificationTx.Commit(ctx); err != nil {
		t.Fatalf("commit verification transaction: %v", err)
	}

	deleted, err = fixture.store.CleanupEmailChallenges(ctx, now, 1)
	if err != nil {
		t.Fatalf("cleanup after verification completes: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("cleanup deleted %d rows after lock release, want 1", deleted)
	}
	if err := fixture.pool.QueryRow(ctx, `
SELECT EXISTS (
SELECT 1
FROM auth_email_challenges
WHERE id = $1
)
`, lockedID).Scan(&exists); err != nil {
		t.Fatalf("check locked challenge after later cleanup: %v", err)
	}
	if exists {
		t.Fatal("later cleanup did not delete the previously locked terminal challenge")
	}
}

func TestRecoveryChallengeCleanupSkipsLockedTerminalRecords(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	for _, challengePurpose := range []struct {
		name    string
		purpose string
	}{
		{name: "recovery-email-enrollment", purpose: "recovery_email_enrollment"},
		{name: "password-recovery", purpose: "password_recovery"},
	} {
		t.Run(challengePurpose.name, func(t *testing.T) {
			lockedTerminalAt := now.Add(-postgres.EmailChallengeRetention - 2*time.Hour)
			otherTerminalAt := now.Add(-postgres.EmailChallengeRetention - time.Hour)
			lockedID := "cleanup-locked-" + challengePurpose.name
			otherID := "cleanup-available-" + challengePurpose.name

			for _, challenge := range []struct {
				id        string
				expiresAt time.Time
			}{
				{id: lockedID, expiresAt: lockedTerminalAt},
				{id: otherID, expiresAt: otherTerminalAt},
			} {
				if _, err := fixture.pool.Exec(ctx, `
INSERT INTO auth_email_challenges (id, email, purpose, code_hash, expires_at)
VALUES ($1, $2, $3, $4, $5)
`, challenge.id, challenge.id+"@example.com", challengePurpose.purpose, "hash-"+challenge.id, challenge.expiresAt); err != nil {
					t.Fatalf("insert %s: %v", challenge.id, err)
				}
			}

			verificationTx, err := fixture.pool.Begin(ctx)
			if err != nil {
				t.Fatalf("begin verification transaction: %v", err)
			}
			defer verificationTx.Rollback(ctx)

			var selectedID string
			if err := verificationTx.QueryRow(ctx, `
SELECT id
FROM auth_email_challenges
WHERE id = $1
FOR UPDATE
`, lockedID).Scan(&selectedID); err != nil {
				t.Fatalf("lock terminal challenge: %v", err)
			}
			if selectedID != lockedID {
				t.Fatalf("locked challenge = %q, want %q", selectedID, lockedID)
			}

			cleanupCtx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			deleted, err := fixture.store.CleanupEmailChallenges(cleanupCtx, now, 1)
			if err != nil {
				t.Fatalf("cleanup while verification holds lock: %v", err)
			}
			if deleted != 1 {
				t.Fatalf("cleanup deleted %d rows while locked row was skipped, want 1", deleted)
			}

			var exists bool
			if err := fixture.pool.QueryRow(ctx, `
SELECT EXISTS (
SELECT 1
FROM auth_email_challenges
WHERE id = $1
)
`, lockedID).Scan(&exists); err != nil {
				t.Fatalf("check locked challenge after cleanup: %v", err)
			}
			if !exists {
				t.Fatal("cleanup deleted the challenge held by the active verification transaction")
			}
			if err := fixture.pool.QueryRow(ctx, `
SELECT EXISTS (
SELECT 1
FROM auth_email_challenges
WHERE id = $1
)
`, otherID).Scan(&exists); err != nil {
				t.Fatalf("check available challenge after cleanup: %v", err)
			}
			if exists {
				t.Fatal("cleanup did not delete the available terminal challenge")
			}

			if err := verificationTx.Commit(ctx); err != nil {
				t.Fatalf("commit verification transaction: %v", err)
			}

			deleted, err = fixture.store.CleanupEmailChallenges(ctx, now, 1)
			if err != nil {
				t.Fatalf("cleanup after verification completes: %v", err)
			}
			if deleted != 1 {
				t.Fatalf("cleanup deleted %d rows after lock release, want 1", deleted)
			}
			if err := fixture.pool.QueryRow(ctx, `
SELECT EXISTS (
SELECT 1
FROM auth_email_challenges
WHERE id = $1
)
`, lockedID).Scan(&exists); err != nil {
				t.Fatalf("check locked challenge after later cleanup: %v", err)
			}
			if exists {
				t.Fatal("later cleanup did not delete the previously locked terminal challenge")
			}
		})
	}
}

func timePtr(value time.Time) *time.Time {
	return &value
}
