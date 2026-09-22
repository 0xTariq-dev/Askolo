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
	adminPool  *pgxpool.Pool
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
	adminPool, err := pgxpool.New(ctx, baseURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	if err := adminPool.Ping(ctx); err != nil {
		adminPool.Close()
		t.Fatalf("ping integration database: %v", err)
	}

	schema := fmt.Sprintf("askolo_native_auth_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, `CREATE SCHEMA `+quoteIdentifier(schema)); err != nil {
		adminPool.Close()
		t.Fatalf("create integration schema: %v", err)
	}
	cleanupSchema := func() {
		_, _ = adminPool.Exec(context.Background(), `DROP SCHEMA `+quoteIdentifier(schema)+` CASCADE`)
		adminPool.Close()
	}
	t.Cleanup(cleanupSchema)

	if _, err := adminPool.Exec(ctx, integrationSchemaSQL(schema)); err != nil {
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
		store:     store,
		pool:      pool,
		adminPool: adminPool,
		schema:    schema,
		baseURL:   schemaURL,
		authConfig: config.Config{
			Environment:       "test",
			SessionCookieName: "askolo.sid",
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
	updated_at timestamptz DEFAULT now() NOT NULL
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
`, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix)
}

func testAuthHandler(fixture *emailAuthFixture, sender EmailSender, logger *slog.Logger) http.Handler {
	return NewHandlerWithEmailSender(fixture.authConfig, fixture.store, logger, sender).Routes()
}

func testProductHandler(fixture *emailAuthFixture) http.Handler {
	return productmodule.NewHandler(fixture.authConfig, fixture.store, slog.Default(), fixture.authConfig.SessionCookieName).Routes()
}

func jsonRequest(t *testing.T, handler http.Handler, method, path string, input map[string]string, cookie *http.Cookie, remote string) *httptest.ResponseRecorder {
	t.Helper()
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
	t.Fatalf("response did not set a session cookie: %s", response.Body.String())
	return nil
}

func assertResponseDoesNotContain(t *testing.T, response *httptest.ResponseRecorder, secrets ...string) {
	t.Helper()
	body := response.Body.String()
	for _, secret := range secrets {
		if secret != "" && strings.Contains(body, secret) {
			t.Fatalf("response exposed sensitive value %q: %s", secret, body)
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
		t.Fatalf("signup status = %d, body = %s", signup.Code, signup.Body.String())
	}
	signupCode := sender.codeForSubject(t, "Verify your Askolo email")
	assertResponseDoesNotContain(t, signup, integrationEmail, signupCode, integrationPassword)

	verify := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/email/verify", map[string]string{
		"email": integrationEmail,
		"code":  signupCode,
	}, nil, "192.0.2.11:1000")
	if verify.Code != http.StatusOK {
		t.Fatalf("email verification status = %d, body = %s", verify.Code, verify.Body.String())
	}
	assertResponseDoesNotContain(t, verify, integrationEmail, signupCode, integrationPassword)

	primaryRecoveryRequest := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
		"email": integrationEmail,
	}, nil, "192.0.2.121:1000")
	if primaryRecoveryRequest.Code != http.StatusAccepted {
		t.Fatalf("primary email recovery request status = %d, body = %s", primaryRecoveryRequest.Code, primaryRecoveryRequest.Body.String())
	}
	primaryRecoveryCode := sender.codeForSubject(t, "Reset your Askolo password")
	assertResponseDoesNotContain(t, primaryRecoveryRequest, integrationEmail, primaryRecoveryCode, integrationPassword)

	primaryRecoveryCooldown := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
		"email": integrationEmail,
	}, nil, "192.0.2.122:1000")
	if primaryRecoveryCooldown.Code != http.StatusTooManyRequests || primaryRecoveryCooldown.Header().Get("Retry-After") != "120" {
		t.Fatalf("primary recovery cooldown = %d, retry-after=%q, body=%s",
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
		t.Fatalf("duplicate signup response = %d %q", duplicateSignup.Code, duplicateSignup.Body.String())
	}
	if sender.count() != messagesBeforeDuplicateSignup {
		t.Fatalf("duplicate signup sent a new message (total messages: %d, before request: %d)", sender.count(), messagesBeforeDuplicateSignup)
	}
	assertResponseDoesNotContain(t, duplicateSignup, integrationEmail, "a different password")

	login := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/login", map[string]string{
		"email": integrationEmail, "password": integrationPassword,
	}, nil, "192.0.2.12:1000")
	if login.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", login.Code, login.Body.String())
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
		t.Fatalf("recovery enrollment status = %d, body = %s", enroll.Code, enroll.Body.String())
	}
	recoveryEmailCode := sender.codeForSubject(t, "Confirm your Askolo recovery email")
	assertResponseDoesNotContain(t, enroll, recoveryEmail, recoveryEmailCode, integrationPassword)

	verifyRecovery := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/recovery/email/verify", map[string]string{
		"email": recoveryEmail, "code": recoveryEmailCode,
	}, sessionOne, "192.0.2.15:1000")
	if verifyRecovery.Code != http.StatusOK {
		t.Fatalf("recovery verification status = %d, body = %s", verifyRecovery.Code, verifyRecovery.Body.String())
	}

	unknownRecovery := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
		"email": "unknown@example.com",
	}, nil, "192.0.2.16:1000")
	if unknownRecovery.Code != http.StatusAccepted || unknownRecovery.Body.String() != `{"status":"recovery_if_available"}`+"\n" {
		t.Fatalf("unknown recovery response = %d %q", unknownRecovery.Code, unknownRecovery.Body.String())
	}
	assertResponseDoesNotContain(t, unknownRecovery, "unknown@example.com")

	deliveryCountBeforeDirectRecovery := sender.count()
	directRecoveryAddress := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
		"email": recoveryEmail,
	}, nil, "192.0.2.161:1000")
	if directRecoveryAddress.Code != http.StatusAccepted || directRecoveryAddress.Body.String() != unknownRecovery.Body.String() {
		t.Fatalf("direct recovery address response = %d %q, want generic accepted response", directRecoveryAddress.Code, directRecoveryAddress.Body.String())
	}
	if sender.count() != deliveryCountBeforeDirectRecovery {
		t.Fatalf("direct recovery address unexpectedly sent a message")
	}

	recoveryRequest := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/request", map[string]string{
		"email":  integrationEmail,
		"method": "recovery_email",
	}, nil, "192.0.2.17:1000")
	if recoveryRequest.Code != http.StatusAccepted {
		t.Fatalf("recovery request status = %d, body = %s", recoveryRequest.Code, recoveryRequest.Body.String())
	}
	if recoveryRequest.Body.String() != unknownRecovery.Body.String() {
		t.Fatalf("known and unknown recovery responses differ: known=%q unknown=%q", recoveryRequest.Body.String(), unknownRecovery.Body.String())
	}
	recoveryCode := sender.codeForSubject(t, "Reset your Askolo password")
	assertResponseDoesNotContain(t, recoveryRequest, recoveryEmail, recoveryCode, integrationPassword)

	recoveryVerify := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/verify", map[string]string{
		"email": integrationEmail, "method": "recovery_email", "code": recoveryCode,
	}, nil, "192.0.2.171:1000")
	if recoveryVerify.Code != http.StatusOK {
		t.Fatalf("recovery code verification status = %d, body = %s", recoveryVerify.Code, recoveryVerify.Body.String())
	}
	assertResponseDoesNotContain(t, recoveryVerify, recoveryEmail, recoveryCode, integrationPassword)

	resetPassword := "new correct horse battery staple"
	reset := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/recovery/reset", map[string]string{
		"email": integrationEmail, "method": "recovery_email", "code": recoveryCode, "password": resetPassword,
	}, nil, "192.0.2.18:1000")
	if reset.Code != http.StatusOK {
		t.Fatalf("password reset status = %d, body = %s", reset.Code, reset.Body.String())
	}
	assertResponseDoesNotContain(t, reset, recoveryEmail, recoveryCode, resetPassword)

	for name, cookie := range map[string]*http.Cookie{"first": sessionOne, "second": sessionTwo} {
		session := plainRequest(authHandler, http.MethodGet, "/api/auth/session", cookie)
		if session.Code != http.StatusOK || session.Body.String() != `{"user":null}`+"\n" {
			t.Fatalf("%s session after reset = %d %q", name, session.Code, session.Body.String())
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
		t.Fatalf("session remained active after logout: %s", afterLogout.Body.String())
	}

	deleteLogin := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/password/login", map[string]string{
		"email": integrationEmail, "password": resetPassword,
	}, nil, "192.0.2.21:1000")
	deleteResponse := plainRequest(testProductHandler(fixture), http.MethodDelete, "/api/user/account", sessionCookie(t, deleteLogin))
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("account deletion status = %d, body = %s", deleteResponse.Code, deleteResponse.Body.String())
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
		t.Fatalf("auth logs exposed signup credentials or email: %s", logs.String())
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
		t.Fatalf("MFA recovery support request status = %d, body = %s", request.Code, request.Body.String())
	}
	code := sender.codeForSubject(t, "Verify your Askolo MFA recovery request")
	assertResponseDoesNotContain(t, request, email, code)

	sessionID, err := fixture.store.CreateSession(context.Background(), userID, "password", time.Hour)
	if err != nil {
		t.Fatalf("create session before MFA recovery support verification: %v", err)
	}
	sessionCookie := &http.Cookie{Name: "askolo.sid", Value: sessionID}
	verify := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/mfa/recovery-support/verify", map[string]string{
		"email": email,
		"code":  code,
	}, sessionCookie, "192.0.2.81:1000")
	if verify.Code != http.StatusOK || !strings.Contains(verify.Body.String(), "mfa_recovery_support_review_required") {
		t.Fatalf("MFA recovery support verification = %d, body = %s", verify.Code, verify.Body.String())
	}
	if _, err := fixture.store.SessionUserID(context.Background(), sessionID); !errors.Is(err, postgres.ErrNotFound) {
		t.Fatalf("session after MFA recovery support verification error = %v, want session revoked", err)
	}
	for _, cookie := range verify.Result().Cookies() {
		if cookie.Name == "askolo.sid" && cookie.Value != "" {
			t.Fatalf("MFA recovery support verification issued a session cookie: %+v", cookie)
		}
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
			t.Fatalf("invalid attempt %d status = %d, body = %s", attempt+1, response.Code, response.Body.String())
		}
	}
	locked := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/email/verify", map[string]string{
		"email": integrationEmail, "code": wrongCode,
	}, nil, "192.0.2.40:2000")
	if locked.Code != http.StatusTooManyRequests || !strings.Contains(locked.Body.String(), `"code":"CHALLENGE_LOCKED"`) {
		t.Fatalf("locked challenge response = %d %s", locked.Code, locked.Body.String())
	}

	expiredEmail := "expired@example.com"
	expiredUser := createVerifiedUser(t, fixture, expiredEmail, integrationPassword)
	createChallenge(t, fixture, NewHandler(fixture.authConfig, fixture.store, slog.Default()), expiredUser, expiredEmail, "email_verification", "123456", time.Now().Add(-time.Minute))
	expired := jsonRequest(t, authHandler, http.MethodPost, "/api/auth/email/verify", map[string]string{
		"email": expiredEmail, "code": "123456",
	}, nil, "192.0.2.41:2000")
	if expired.Code != http.StatusBadRequest {
		t.Fatalf("expired challenge status = %d, body = %s", expired.Code, expired.Body.String())
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
		} else if status != http.StatusTooManyRequests {
			t.Fatalf("concurrent recovery request returned unexpected status %d", status)
		}
	}
	if accepted != 1 || concurrentSender.count() != 1 {
		t.Fatalf("concurrent recovery sends accepted=%d deliveries=%d, want one each", accepted, concurrentSender.count())
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
		t.Fatalf("delivery failure response = %d %s", failure.Code, failure.Body.String())
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
		t.Fatalf("delivery failure logs exposed sensitive values: %s", failureLogs.String())
	}
	if !strings.Contains(failureLogs.String(), "failure_category=provider_rejection") {
		t.Fatalf("delivery failure logs omitted safe failure category: %s", failureLogs.String())
	}

	retrySender := &captureEmailSender{}
	retryHandler := testAuthHandler(fixture, retrySender, slog.Default())
	retry := jsonRequest(t, retryHandler, http.MethodPost, "/api/auth/email/resend", map[string]string{
		"email": failureEmail,
	}, nil, "192.0.2.51:5000")
	if retry.Code != http.StatusAccepted {
		t.Fatalf("delivery retry status = %d, body = %s", retry.Code, retry.Body.String())
	}
	if retrySender.count() != 1 {
		t.Fatalf("delivery retry sent %d messages, want one", retrySender.count())
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

func timePtr(value time.Time) *time.Time {
	return &value
}
