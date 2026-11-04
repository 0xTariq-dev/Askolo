package google

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	"askolo/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

type googleOAuthIntegrationFixture struct {
	store *postgres.Store
	pool  *pgxpool.Pool
}

type controlledGoogleProvider struct {
	mu              sync.Mutex
	tokenStatus     int
	tokenCalls      int
	userInfoCalls   int
	tokenBody       string
	userInfoPayload userInfo
}

const trustedDeviceTestHMACSecret = "integration-google-trusted-device-hmac-secret"

func newGoogleOAuthFixture(t *testing.T) *googleOAuthIntegrationFixture {
	t.Helper()
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run Google OAuth integration tests")
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

	schema := fmt.Sprintf("askolo_google_oauth_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+quoteGoogleIdentifier(schema)); err != nil {
		adminPool.Close()
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), "DROP SCHEMA "+quoteGoogleIdentifier(schema)+" CASCADE")
		adminPool.Close()
	})

	if _, err := adminPool.Exec(ctx, googleOAuthSchemaSQL(schema)); err != nil {
		t.Fatalf("create Google OAuth integration tables: %v", err)
	}
	schemaURL := googleOAuthDatabaseURL(t, baseURL, schema)
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
		t.Fatalf("open Google OAuth store: %v", err)
	}
	t.Cleanup(store.Close)

	return &googleOAuthIntegrationFixture{store: store, pool: pool}
}

func googleOAuthSchemaSQL(schema string) string {
	prefix := quoteGoogleIdentifier(schema) + "."
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
CREATE TABLE %sauth_totp (
	user_id text PRIMARY KEY,
	secret_encrypted text NOT NULL,
	enabled_at timestamptz,
	last_used_step integer,
	created_at timestamptz DEFAULT now() NOT NULL,
	updated_at timestamptz DEFAULT now() NOT NULL
);
CREATE TABLE %sauth_trusted_devices (
	id text PRIMARY KEY,
	user_id text NOT NULL,
	credential_hash text NOT NULL,
	fingerprint_hash text NOT NULL,
	created_at timestamptz DEFAULT now() NOT NULL,
	last_used_at timestamptz,
	expires_at timestamptz NOT NULL,
	revoked_at timestamptz
);
CREATE TABLE %sgoogle_oauth_states (
	nonce_hash text PRIMARY KEY,
	flow text NOT NULL,
	user_id text,
	return_to text NOT NULL,
	expires_at timestamptz NOT NULL,
	consumed_at timestamptz
);
CREATE TABLE %sprovider_accounts (
	id text PRIMARY KEY,
	user_id text NOT NULL,
	provider text NOT NULL,
	external_subject text NOT NULL,
	email text,
	display_name text,
	avatar_url text,
	login_enabled boolean DEFAULT true NOT NULL,
	email_verified boolean DEFAULT false NOT NULL,
	linked_at timestamptz,
	created_at timestamptz DEFAULT now() NOT NULL,
	updated_at timestamptz DEFAULT now() NOT NULL,
	UNIQUE (provider, external_subject)
);
CREATE TABLE %sprovider_connections (
	id text PRIMARY KEY,
	user_id text NOT NULL,
	provider_account_id text NOT NULL,
	provider text NOT NULL,
	status text DEFAULT 'active' NOT NULL,
	granted_scopes text[] DEFAULT '{}' NOT NULL,
	capabilities text[] DEFAULT '{}' NOT NULL,
	revoked_at timestamptz,
	created_at timestamptz DEFAULT now() NOT NULL,
	updated_at timestamptz DEFAULT now() NOT NULL,
	UNIQUE (user_id, provider_account_id)
);
CREATE TABLE %sprovider_credentials (
	connection_id text PRIMARY KEY,
	access_token_encrypted text NOT NULL,
	refresh_token_encrypted text NOT NULL,
	expires_at timestamptz NOT NULL,
	token_type text DEFAULT 'Bearer' NOT NULL,
	created_at timestamptz DEFAULT now() NOT NULL,
	updated_at timestamptz DEFAULT now() NOT NULL
);
CREATE TABLE %sprovider_defaults (
	user_id text NOT NULL,
	service text NOT NULL,
	connection_id text NOT NULL,
	updated_at timestamptz DEFAULT now() NOT NULL,
	PRIMARY KEY (user_id, service)
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
  expires_at timestamptz,
 UNIQUE (user_id, idempotency_key)
);
CREATE TABLE %sai_credit_policy_versions (
 version integer PRIMARY KEY,
  signup_grant_usd_micros bigint DEFAULT 0 NOT NULL,
  rollover_cap_usd_micros bigint DEFAULT 0 NOT NULL,
  rollover_expiry_days integer DEFAULT 30 NOT NULL,
  grant_type_policies jsonb NOT NULL DEFAULT '{}'::jsonb
);
INSERT INTO %sai_credit_policy_versions(version,signup_grant_usd_micros)
VALUES (1, 0);
	`, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix, prefix, prefix, prefix)
}

func quoteGoogleIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func googleOAuthDatabaseURL(t *testing.T, databaseURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database URL: %v", err)
	}
	query := parsed.Query()
	query.Set("options", "-c search_path="+schema)
	parsed.RawQuery = strings.ReplaceAll(query.Encode(), "+", "%20")
	return parsed.String()
}

func testGoogleOAuthIntegrationHandler(fixture *googleOAuthIntegrationFixture, tokenURL, userInfoURL string, logger *slog.Logger) *Handler {
	handler := NewHandler(config.Config{
		Environment:             "test",
		SessionSecret:           "integration-google-oauth-session-secret",
		SessionCookieName:       "askolo_sid",
		AuthRateLimitHMACSecret: trustedDeviceTestHMACSecret,
		AllowedOAuthHosts:       map[string]struct{}{"localhost": {}},
		Google: config.GoogleOAuthConfig{
			LoginClientID:     "login-client",
			LoginClientSecret: "login-secret",
			AuthURL:           "https://accounts.google.example/authorize",
			TokenURL:          tokenURL,
			UserInfoURL:       userInfoURL,
		},
	}, fixture.store, logger)
	return handler
}

func newControlledGoogleProvider(t *testing.T, tokenStatus int, info userInfo) (*httptest.Server, *controlledGoogleProvider) {
	t.Helper()
	provider := &controlledGoogleProvider{
		tokenStatus:     tokenStatus,
		tokenBody:       `{"access_token":"controlled-access-token","refresh_token":"controlled-refresh-token","expires_in":3600,"scope":"openid email profile","token_type":"Bearer"}`,
		userInfoPayload: info,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provider.mu.Lock()
		defer provider.mu.Unlock()

		switch r.URL.Path {
		case "/token":
			provider.tokenCalls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(provider.tokenStatus)
			if provider.tokenStatus >= 200 && provider.tokenStatus < 300 {
				_, _ = w.Write([]byte(provider.tokenBody))
			} else {
				_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			}
		case "/userinfo":
			provider.userInfoCalls++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(provider.userInfoPayload)
		default:
			http.NotFound(w, r)
		}
	}))
	return server, provider
}

func loginCallbackState(t *testing.T, fixture *googleOAuthIntegrationFixture, handler *Handler, intent string) (statePayload, *http.Cookie) {
	t.Helper()
	nonce := fmt.Sprintf("integration-nonce-%d", time.Now().UnixNano())
	payload := statePayload{
		Nonce:        nonce,
		CodeVerifier: "integration-code-verifier",
		ReturnTo:     "https://evil.example/redirect",
		Flow:         "login",
		Intent:       intent,
		IssuedAt:     time.Now().Unix(),
	}
	encoded, err := handler.encodeState(payload)
	if err != nil {
		t.Fatalf("encode callback state: %v", err)
	}
	hash := sha256.Sum256([]byte(nonce))
	if err := fixture.store.InsertOAuthState(
		context.Background(), hex.EncodeToString(hash[:]), "login", "", "", time.Now().Add(stateTTL),
	); err != nil {
		t.Fatalf("insert callback state: %v", err)
	}
	return payload, &http.Cookie{Name: stateCookieName, Value: encoded}
}

func serveGoogleLoginCallback(handler *Handler, payload statePayload, cookie *http.Cookie, query url.Values, browserCookies ...*http.Cookie) *httptest.ResponseRecorder {
	query.Set("state", payload.Nonce)
	request := httptest.NewRequest(
		http.MethodGet,
		"http://localhost/api/auth/google/callback?"+query.Encode(),
		nil,
	)
	request.Host = "localhost"
	request.AddCookie(cookie)
	for _, browserCookie := range browserCookies {
		request.AddCookie(browserCookie)
	}
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	return response
}

func trustedDeviceTestHash(userID, kind, value string) string {
	mac := hmac.New(sha256.New, []byte(trustedDeviceTestHMACSecret))
	_, _ = mac.Write([]byte("askolo:trusted-device:v1:" + userID + "\x00" + kind + "\x00" + value))
	return hex.EncodeToString(mac.Sum(nil))
}

func assertSafeGoogleRedirect(t *testing.T, response *httptest.ResponseRecorder, status string) {
	t.Helper()
	if response.Code != http.StatusFound {
		t.Fatalf("callback status = %d, body = %s", response.Code, response.Body.String())
	}
	want := "/dashboard?google=" + status
	if got := response.Header().Get("Location"); got != want {
		t.Fatalf("callback location = %q, want %q", got, want)
	}
	parsed, err := url.Parse(response.Header().Get("Location"))
	if err != nil || parsed.IsAbs() || parsed.Host != "" {
		t.Fatalf("callback escaped same-origin redirect: %q", response.Header().Get("Location"))
	}
}

func assertGoogleCallbackDoesNotLeak(t *testing.T, response *httptest.ResponseRecorder, logs *bytes.Buffer, secrets ...string) {
	t.Helper()
	values := []string{response.Body.String(), response.Header().Get("Location"), logs.String()}
	for _, value := range values {
		for _, secret := range secrets {
			if secret != "" && strings.Contains(value, secret) {
				t.Fatalf("Google callback exposed sensitive value %q", secret)
			}
		}
	}
}

func TestGoogleCallbackSafetyMatrix(t *testing.T) {
	t.Run("denied consent", func(t *testing.T) {
		fixture := newGoogleOAuthFixture(t)
		handler := testGoogleOAuthIntegrationHandler(fixture, "", "", slog.Default())
		payload, cookie := loginCallbackState(t, fixture, handler, "")

		response := serveGoogleLoginCallback(handler, payload, cookie, url.Values{"error": {"access_denied"}})

		assertSafeGoogleRedirect(t, response, "error")
	})

	t.Run("missing code", func(t *testing.T) {
		fixture := newGoogleOAuthFixture(t)
		handler := testGoogleOAuthIntegrationHandler(fixture, "", "", slog.Default())
		payload, cookie := loginCallbackState(t, fixture, handler, "")

		response := serveGoogleLoginCallback(handler, payload, cookie, url.Values{})

		assertSafeGoogleRedirect(t, response, "error")
	})

	t.Run("replayed state", func(t *testing.T) {
		fixture := newGoogleOAuthFixture(t)
		handler := testGoogleOAuthIntegrationHandler(fixture, "", "", slog.Default())
		payload, cookie := loginCallbackState(t, fixture, handler, "")

		first := serveGoogleLoginCallback(handler, payload, cookie, url.Values{"error": {"access_denied"}})
		assertSafeGoogleRedirect(t, first, "error")
		replay := serveGoogleLoginCallback(handler, payload, cookie, url.Values{"error": {"access_denied"}})

		if replay.Code != http.StatusBadRequest || !strings.Contains(replay.Body.String(), `"code":"INVALID_STATE"`) {
			t.Fatalf("replayed callback = %d %q", replay.Code, replay.Body.String())
		}
	})

	t.Run("token exchange failure", func(t *testing.T) {
		fixture := newGoogleOAuthFixture(t)
		provider, calls := newControlledGoogleProvider(t, http.StatusBadRequest, userInfo{})
		defer provider.Close()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		handler := testGoogleOAuthIntegrationHandler(fixture, provider.URL+"/token", provider.URL+"/userinfo", logger)
		payload, cookie := loginCallbackState(t, fixture, handler, "")
		code := "authorization-code-must-not-leak"
		token := "controlled-access-token"

		response := serveGoogleLoginCallback(handler, payload, cookie, url.Values{"code": {code}})

		assertSafeGoogleRedirect(t, response, "error")
		assertGoogleCallbackDoesNotLeak(t, response, &logs, code, token)
		calls.mu.Lock()
		tokenCalls, userInfoCalls := calls.tokenCalls, calls.userInfoCalls
		calls.mu.Unlock()
		if tokenCalls != 1 || userInfoCalls != 0 {
			t.Fatalf("provider calls = token %d, userinfo %d; want 1, 0", tokenCalls, userInfoCalls)
		}
	})
}

func TestGoogleCallbackExistingProviderIdentitySignsInWithoutDuplicate(t *testing.T) {
	fixture := newGoogleOAuthFixture(t)
	info := userInfo{
		Subject:       "google-existing-subject",
		Email:         "existing@example.com",
		EmailVerified: true,
		GivenName:     "Existing",
		FamilyName:    "User",
	}
	provider, _ := newControlledGoogleProvider(t, http.StatusOK, info)
	defer provider.Close()
	handler := testGoogleOAuthIntegrationHandler(fixture, provider.URL+"/token", provider.URL+"/userinfo", slog.Default())
	if _, err := fixture.pool.Exec(context.Background(), `
		INSERT INTO users (id, email, status, email_verified_at, account_created_via)
		VALUES ('existing-user', $1, 'active', NOW(), 'google')
	`, info.Email); err != nil {
		t.Fatalf("seed existing user: %v", err)
	}
	if _, err := fixture.pool.Exec(context.Background(), `
		INSERT INTO provider_accounts
			(id, user_id, provider, external_subject, email, display_name, login_enabled, email_verified, linked_at)
		VALUES ('existing-account', 'existing-user', 'google', $1, $2, 'Existing User', TRUE, TRUE, NOW())
	`, info.Subject, info.Email); err != nil {
		t.Fatalf("seed existing provider identity: %v", err)
	}
	payload, cookie := loginCallbackState(t, fixture, handler, "")

	response := serveGoogleLoginCallback(handler, payload, cookie, url.Values{"code": {"existing-code"}})

	assertSafeGoogleRedirect(t, response, "success")
	session := googleSessionCookie(t, response)
	userID, err := fixture.store.SessionUserID(context.Background(), session.Value)
	if err != nil {
		t.Fatalf("read created session: %v", err)
	}
	if userID != "existing-user" {
		t.Fatalf("session user = %q, want existing-user", userID)
	}
	var accountCount, sessionCount int
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM provider_accounts WHERE provider = 'google' AND external_subject = $1
	`, info.Subject).Scan(&accountCount); err != nil {
		t.Fatalf("count provider identities: %v", err)
	}
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM sessions WHERE sess->>'userId' = 'existing-user'
	`).Scan(&sessionCount); err != nil {
		t.Fatalf("count native sessions: %v", err)
	}
	if accountCount != 1 || sessionCount != 1 {
		t.Fatalf("existing identity counts = accounts %d, sessions %d; want 1, 1", accountCount, sessionCount)
	}
}

func TestGoogleCallbackRequiresMFAForEnabledAccount(t *testing.T) {
	fixture := newGoogleOAuthFixture(t)
	info := userInfo{
		Subject:       "google-mfa-subject",
		Email:         "mfa-existing@example.com",
		EmailVerified: true,
		GivenName:     "MFA",
		FamilyName:    "User",
	}
	provider, _ := newControlledGoogleProvider(t, http.StatusOK, info)
	defer provider.Close()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	handler := testGoogleOAuthIntegrationHandler(fixture, provider.URL+"/token", provider.URL+"/userinfo", logger)
	if _, err := fixture.pool.Exec(context.Background(), `
		INSERT INTO users (id, email, status, email_verified_at, account_created_via)
		VALUES ('google-mfa-user', $1, 'active', NOW(), 'google')
	`, info.Email); err != nil {
		t.Fatalf("seed MFA user: %v", err)
	}
	if _, err := fixture.pool.Exec(context.Background(), `
		INSERT INTO provider_accounts
			(id, user_id, provider, external_subject, email, display_name, login_enabled, email_verified, linked_at)
		VALUES ('google-mfa-account', 'google-mfa-user', 'google', $1, $2, 'MFA User', TRUE, TRUE, NOW())
	`, info.Subject, info.Email); err != nil {
		t.Fatalf("seed MFA provider identity: %v", err)
	}
	if _, err := fixture.pool.Exec(context.Background(), `
		INSERT INTO auth_totp (user_id, secret_encrypted, enabled_at)
		VALUES ('google-mfa-user', 'opaque-test-ciphertext', NOW())
	`); err != nil {
		t.Fatalf("enable test MFA: %v", err)
	}
	payload, cookie := loginCallbackState(t, fixture, handler, "")

	response := serveGoogleLoginCallback(handler, payload, cookie, url.Values{"code": {"google-mfa-code"}})

	assertSafeGoogleRedirect(t, response, "mfa_required")
	session := googleSessionCookie(t, response)
	state, err := fixture.store.SessionMFAState(context.Background(), session.Value)
	if err != nil {
		t.Fatalf("read pending MFA session: %v", err)
	}
	if state.UserID != "google-mfa-user" || !state.Required || state.Verified {
		t.Fatalf("Google callback session MFA state = %+v, want required and unverified", state)
	}
	assertGoogleCallbackDoesNotLeak(t, response, &logs, "google-mfa-code", "opaque-test-ciphertext")
}

func TestGoogleCallbackRecognizesOnlyValidRememberedBrowsers(t *testing.T) {
	tests := []struct {
		name                 string
		expired              bool
		revoked              bool
		requestFingerprint   string
		wantRedirect         string
		wantMFARequired      bool
		wantMFAAuthenticated bool
	}{
		{
			name:                 "valid remembered browser",
			requestFingerprint:   strings.Repeat("1", 64),
			wantRedirect:         "success",
			wantMFAAuthenticated: true,
		},
		{
			name:               "expired remembered browser",
			expired:            true,
			requestFingerprint: strings.Repeat("1", 64),
			wantRedirect:       "mfa_required",
			wantMFARequired:    true,
		},
		{
			name:               "revoked remembered browser",
			revoked:            true,
			requestFingerprint: strings.Repeat("1", 64),
			wantRedirect:       "mfa_required",
			wantMFARequired:    true,
		},
		{
			name:               "fingerprint mismatch",
			requestFingerprint: strings.Repeat("2", 64),
			wantRedirect:       "mfa_required",
			wantMFARequired:    true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newGoogleOAuthFixture(t)
			info := userInfo{
				Subject:       "google-trusted-device-subject",
				Email:         "trusted-device-google@example.com",
				EmailVerified: true,
			}
			provider, _ := newControlledGoogleProvider(t, http.StatusOK, info)
			defer provider.Close()
			handler := testGoogleOAuthIntegrationHandler(fixture, provider.URL+"/token", provider.URL+"/userinfo", slog.Default())
			if _, err := fixture.pool.Exec(context.Background(), `
				INSERT INTO users (id, email, status, email_verified_at, account_created_via)
				VALUES ('google-trusted-device-user', $1, 'active', NOW(), 'google')
			`, info.Email); err != nil {
				t.Fatalf("seed MFA user: %v", err)
			}
			if _, err := fixture.pool.Exec(context.Background(), `
				INSERT INTO provider_accounts
					(id, user_id, provider, external_subject, email, display_name, login_enabled, email_verified, linked_at)
				VALUES ('google-trusted-device-account', 'google-trusted-device-user', 'google', $2, $1,
					'Trusted Device User', TRUE, TRUE, NOW())
			`, info.Email, info.Subject); err != nil {
				t.Fatalf("seed MFA provider identity: %v", err)
			}
			if _, err := fixture.pool.Exec(context.Background(), `
				INSERT INTO auth_totp (user_id, secret_encrypted, enabled_at)
				VALUES ('google-trusted-device-user', 'opaque-test-ciphertext', NOW())
			`); err != nil {
				t.Fatalf("enable MFA: %v", err)
			}

			const userID = "google-trusted-device-user"
			const credential = "google-trusted-browser-credential"
			enrolledFingerprint := strings.Repeat("1", 64)
			_, err := fixture.pool.Exec(context.Background(), `
				INSERT INTO auth_trusted_devices
					(id, user_id, credential_hash, fingerprint_hash, expires_at, revoked_at)
				VALUES ($1, $2, $3, $4,
					CASE WHEN $5 THEN NOW() - INTERVAL '1 minute' ELSE NOW() + INTERVAL '1 hour' END,
					CASE WHEN $6 THEN NOW() END)
			`, "google-trusted-"+strings.ReplaceAll(test.name, " ", "-"), userID,
				trustedDeviceTestHash(userID, "credential", credential),
				trustedDeviceTestHash(userID, "fingerprint", enrolledFingerprint),
				test.expired, test.revoked)
			if err != nil {
				t.Fatalf("seed trusted browser: %v", err)
			}

			payload, stateCookie := loginCallbackState(t, fixture, handler, "")
			response := serveGoogleLoginCallback(handler, payload, stateCookie,
				url.Values{"code": {"google-trusted-device-code"}},
				&http.Cookie{Name: "askolo.trusted_device", Value: credential},
				&http.Cookie{Name: "askolo_device_fingerprint_v2", Value: test.requestFingerprint},
			)
			assertSafeGoogleRedirect(t, response, test.wantRedirect)
			session := googleSessionCookie(t, response)
			state, err := fixture.store.SessionMFAState(context.Background(), session.Value)
			if err != nil {
				t.Fatalf("read callback MFA state: %v", err)
			}
			if state.UserID != userID || state.Required != test.wantMFARequired || state.Verified != test.wantMFAAuthenticated {
				t.Fatalf("Google callback MFA state = %+v, want required=%t verified=%t",
					state, test.wantMFARequired, test.wantMFAAuthenticated)
			}
		})
	}
}

func TestGoogleCallbackSignupExistingEmailRequiresExplicitSignIn(t *testing.T) {
	fixture := newGoogleOAuthFixture(t)
	info := userInfo{
		Subject:       "google-existing-password-subject",
		Email:         "existing-password@example.com",
		EmailVerified: true,
	}
	provider, _ := newControlledGoogleProvider(t, http.StatusOK, info)
	defer provider.Close()
	handler := testGoogleOAuthIntegrationHandler(fixture, provider.URL+"/token", provider.URL+"/userinfo", slog.Default())
	if _, err := fixture.pool.Exec(context.Background(), `
		INSERT INTO users (id, email, status, email_verified_at, account_created_via)
		VALUES ('existing-password-user', $1, 'active', NOW(), 'password')
	`, info.Email); err != nil {
		t.Fatalf("seed existing password account: %v", err)
	}
	payload, cookie := loginCallbackState(t, fixture, handler, "signup")

	response := serveGoogleLoginCallback(handler, payload, cookie, url.Values{"code": {"existing-password-signup-code"}})

	assertSafeGoogleRedirect(t, response, "account_exists")
	var identityCount, sessionCount int
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM provider_accounts WHERE provider = 'google' AND external_subject = $1
	`, info.Subject).Scan(&identityCount); err != nil {
		t.Fatalf("count provider identities: %v", err)
	}
	if err := fixture.pool.QueryRow(context.Background(), `SELECT count(*) FROM sessions`).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if identityCount != 0 || sessionCount != 0 {
		t.Fatalf("signup collision created %d identities and %d sessions; want none", identityCount, sessionCount)
	}
}

func TestGoogleCallbackSigninWithoutLinkedIdentityReturnsProviderNotLinked(t *testing.T) {
	fixture := newGoogleOAuthFixture(t)
	info := userInfo{
		Subject:       "google-unlinked-signin-subject",
		Email:         "existing-password@example.com",
		EmailVerified: true,
	}
	provider, _ := newControlledGoogleProvider(t, http.StatusOK, info)
	defer provider.Close()
	handler := testGoogleOAuthIntegrationHandler(fixture, provider.URL+"/token", provider.URL+"/userinfo", slog.Default())
	payload, cookie := loginCallbackState(t, fixture, handler, "")

	response := serveGoogleLoginCallback(handler, payload, cookie, url.Values{"code": {"unlinked-signin-code"}})

	assertSafeGoogleRedirect(t, response, "provider_not_linked")
	var identityCount, sessionCount int
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM provider_accounts WHERE provider = 'google' AND external_subject = $1
	`, info.Subject).Scan(&identityCount); err != nil {
		t.Fatalf("count provider identities: %v", err)
	}
	if err := fixture.pool.QueryRow(context.Background(), `SELECT count(*) FROM sessions`).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if identityCount != 0 || sessionCount != 0 {
		t.Fatalf("unlinked signin created %d identities and %d sessions; want none", identityCount, sessionCount)
	}
}

func TestGoogleCallbackSignupRequiresVerifiedEmailAndCreatesNativeSession(t *testing.T) {
	t.Run("verified email creates session", func(t *testing.T) {
		fixture := newGoogleOAuthFixture(t)
		if _, err := fixture.pool.Exec(context.Background(), `
			UPDATE ai_credit_policy_versions
SET signup_grant_usd_micros=5000000,
    grant_type_policies=jsonb_build_object(
      'signup',jsonb_build_object(
        'issuanceCapUsdMicros',5000000,
        'carryoverCapUsdMicros',5000000,
        'expiryDays',30
      )
    )
			WHERE version=(SELECT max(version) FROM ai_credit_policy_versions)
		`); err != nil {
			t.Fatalf("configure test signup grant: %v", err)
		}
		info := userInfo{
			Subject:       "google-signup-subject",
			Email:         "Signup.User@example.com",
			EmailVerified: true,
			GivenName:     "Signup",
			FamilyName:    "User",
		}
		provider, _ := newControlledGoogleProvider(t, http.StatusOK, info)
		defer provider.Close()
		handler := testGoogleOAuthIntegrationHandler(fixture, provider.URL+"/token", provider.URL+"/userinfo", slog.Default())
		payload, cookie := loginCallbackState(t, fixture, handler, "signup")

		response := serveGoogleLoginCallback(handler, payload, cookie, url.Values{"code": {"signup-code"}})

		assertSafeGoogleRedirect(t, response, "success")
		session := googleSessionCookie(t, response)
		userID, err := fixture.store.SessionUserID(context.Background(), session.Value)
		if err != nil {
			t.Fatalf("read signup session: %v", err)
		}
		var email, status, accountCreatedVia string
		var verifiedAt *time.Time
		if err := fixture.pool.QueryRow(context.Background(), `
			SELECT email, status, email_verified_at, account_created_via
			FROM users WHERE id = $1
		`, userID).Scan(&email, &status, &verifiedAt, &accountCreatedVia); err != nil {
			t.Fatalf("read signed-up user: %v", err)
		}
		if email != strings.ToLower(info.Email) || status != "pending_provider_onboarding" ||
			verifiedAt == nil || accountCreatedVia != "google" {
			t.Fatalf("signed-up user = email %q, status %q, verified %v, via %q", email, status, verifiedAt != nil, accountCreatedVia)
		}
		var identityCount int
		if err := fixture.pool.QueryRow(context.Background(), `
			SELECT count(*) FROM provider_accounts WHERE provider = 'google' AND external_subject = $1
		`, info.Subject).Scan(&identityCount); err != nil {
			t.Fatalf("count signup provider identity: %v", err)
		}
		if identityCount != 1 {
			t.Fatalf("signup provider identity count = %d, want 1", identityCount)
		}
		var grantCount int
		var grantAmount int64
		if err := fixture.pool.QueryRow(context.Background(), `
			SELECT count(*), COALESCE(sum(amount_usd_micros), 0)
			FROM ai_credit_grants
			WHERE user_id=$1 AND idempotency_key='welcome-credit-usd-v1' AND currency='USD'
		`, userID).Scan(&grantCount, &grantAmount); err != nil {
			t.Fatalf("read provider signup welcome grant: %v", err)
		}
		if grantCount != 1 || grantAmount != 5_000_000 {
			t.Fatalf("provider signup welcome grant rows=%d amount=%d; want one grant of 5000000 USD micros",
				grantCount, grantAmount)
		}
	})

	t.Run("unverified email is rejected", func(t *testing.T) {
		fixture := newGoogleOAuthFixture(t)
		info := userInfo{
			Subject:       "google-unverified-subject",
			Email:         "unverified@example.com",
			EmailVerified: false,
		}
		provider, _ := newControlledGoogleProvider(t, http.StatusOK, info)
		defer provider.Close()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		handler := testGoogleOAuthIntegrationHandler(fixture, provider.URL+"/token", provider.URL+"/userinfo", logger)
		payload, cookie := loginCallbackState(t, fixture, handler, "signup")
		code := "unverified-signup-code"

		response := serveGoogleLoginCallback(handler, payload, cookie, url.Values{"code": {code}})

		assertSafeGoogleRedirect(t, response, "error")
		assertGoogleCallbackDoesNotLeak(t, response, &logs, code, "controlled-access-token")
		var userCount int
		if err := fixture.pool.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&userCount); err != nil {
			t.Fatalf("count rejected signup users: %v", err)
		}
		if userCount != 0 {
			t.Fatalf("rejected signup created %d users", userCount)
		}
	})
}

func TestConcurrentGoogleFirstSignupConvergesOnOneUser(t *testing.T) {
	fixture := newGoogleOAuthFixture(t)
	ctx := context.Background()
	email := "concurrent-google-signup@example.test"
	subject := "concurrent-google-signup-subject"
	const attempts = 8

	t.Cleanup(func() {
		_, _ = fixture.pool.Exec(ctx, `
			DELETE FROM provider_accounts WHERE provider = 'google' AND external_subject = $1
		`, subject)
		_, _ = fixture.pool.Exec(ctx, `DELETE FROM users WHERE lower(email) = lower($1)`, email)
	})

	start := make(chan struct{})
	results := make(chan struct {
		userID string
		err    error
	}, attempts)
	var workers sync.WaitGroup
	for range attempts {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			userID, err := fixture.store.CreateProviderSignupUser(
				ctx, "google", subject, email, "Concurrent", "Signup", "",
			)
			results <- struct {
				userID string
				err    error
			}{userID: userID, err: err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	var canonicalUserID string
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent signup failed: %v", result.err)
		}
		if canonicalUserID == "" {
			canonicalUserID = result.userID
		} else if result.userID != canonicalUserID {
			t.Fatalf("concurrent signup returned multiple users: %q and %q", canonicalUserID, result.userID)
		}
	}
	var users, identities int
	if err := fixture.pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE lower(email) = lower($1)`, email).Scan(&users); err != nil {
		t.Fatalf("count provisioned users: %v", err)
	}
	if err := fixture.pool.QueryRow(ctx, `
		SELECT count(*) FROM provider_accounts WHERE provider = 'google' AND external_subject = $1
	`, subject).Scan(&identities); err != nil {
		t.Fatalf("count provisioned identities: %v", err)
	}
	if users != 1 || identities != 1 {
		t.Fatalf("created %d users and %d provider identities, want exactly one each", users, identities)
	}
}

func TestGoogleConnectionReauthorizationPreservesScopesAndRefreshToken(t *testing.T) {
	fixture := newGoogleOAuthFixture(t)
	ctx := context.Background()
	userID := "google-reauthorization-test-user"
	subject := "google-reauthorization-test-subject"
	encryptionKey := []byte("12345678901234567890123456789012")
	defer func() {
		_, _ = fixture.pool.Exec(ctx, `
			DELETE FROM provider_credentials
			WHERE connection_id IN (SELECT id FROM provider_connections WHERE user_id = $1)
		`, userID)
		_, _ = fixture.pool.Exec(ctx, `DELETE FROM provider_connections WHERE user_id = $1`, userID)
		_, _ = fixture.pool.Exec(ctx, `
			DELETE FROM provider_accounts WHERE provider = 'google' AND external_subject = $1
		`, subject)
	}()

	firstConnectionID, err := fixture.store.UpsertGoogleConnection(
		ctx, userID, "owner@example.test", subject, "Owner", "",
		[]string{scopeCalendar}, []string{"calendar"},
		"first-access-token", "durable-refresh-token", time.Now().Add(time.Hour), "Bearer", encryptionKey,
	)
	if err != nil {
		t.Fatalf("create initial Google connection: %v", err)
	}
	secondConnectionID, err := fixture.store.UpsertGoogleConnection(
		ctx, userID, "owner@example.test", subject, "Owner", "",
		[]string{scopeGmailRead}, []string{"gmail.read"},
		"second-access-token", "", time.Now().Add(time.Hour), "Bearer", encryptionKey,
	)
	if err != nil {
		t.Fatalf("reauthorize Google connection without a new refresh token: %v", err)
	}
	if secondConnectionID != firstConnectionID {
		t.Fatalf("reauthorization changed connection ID from %q to %q", firstConnectionID, secondConnectionID)
	}

	connections, err := fixture.store.ListGoogleConnections(ctx, userID)
	if err != nil {
		t.Fatalf("list Google connections: %v", err)
	}
	if len(connections) != 1 ||
		!hasString(connections[0].Scopes, scopeCalendar) ||
		!hasString(connections[0].Scopes, scopeGmailRead) ||
		!hasString(connections[0].Capabilities, "calendar") ||
		!hasString(connections[0].Capabilities, "gmail.read") {
		t.Fatalf("reauthorization lost previously granted scopes/capabilities: %+v", connections)
	}

	credentials, err := fixture.store.GetGoogleCredentials(ctx, userID, firstConnectionID, encryptionKey)
	if err != nil {
		t.Fatalf("read reauthorized Google credentials: %v", err)
	}
	if credentials.AccessToken != "second-access-token" || credentials.RefreshToken != "durable-refresh-token" {
		t.Fatalf("reauthorized credentials = access %q refresh %q; expected new access and retained refresh",
			credentials.AccessToken, credentials.RefreshToken)
	}
}

func TestGoogleConnectionsKeepMultipleAccountsAndCredentialsIsolated(t *testing.T) {
	fixture := newGoogleOAuthFixture(t)
	ctx := context.Background()
	userID := "google-multiple-accounts-user"
	encryptionKey := []byte("12345678901234567890123456789012")

	firstID, err := fixture.store.UpsertGoogleConnection(
		ctx, userID, "calendar@example.test", "google-calendar-subject", "Calendar", "",
		[]string{scopeCalendar}, []string{"calendar"},
		"calendar-access-token", "calendar-refresh-token", time.Now().Add(time.Hour), "Bearer", encryptionKey,
	)
	if err != nil {
		t.Fatalf("create first Google account: %v", err)
	}
	secondID, err := fixture.store.UpsertGoogleConnection(
		ctx, userID, "mail@example.test", "google-mail-subject", "Mail", "",
		[]string{scopeGmailRead}, []string{"gmail.read"},
		"mail-access-token", "mail-refresh-token", time.Now().Add(time.Hour), "Bearer", encryptionKey,
	)
	if err != nil {
		t.Fatalf("create second Google account: %v", err)
	}
	if firstID == secondID {
		t.Fatal("different Google identities reused the same connection")
	}

	// The shared connection model can contain other providers, but Google
	// operations must not surface or use those rows.
	if _, err := fixture.pool.Exec(ctx, `
		INSERT INTO provider_accounts (id, user_id, provider, external_subject, email)
		VALUES ('microsoft-account', $1, 'microsoft', 'microsoft-subject', 'work@example.test')
	`, userID); err != nil {
		t.Fatalf("insert other-provider account: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx, `
		INSERT INTO provider_connections (id, user_id, provider_account_id, provider)
		VALUES ('microsoft-connection', $1, 'microsoft-account', 'microsoft')
	`, userID); err != nil {
		t.Fatalf("insert other-provider connection: %v", err)
	}

	connections, err := fixture.store.ListGoogleConnections(ctx, userID)
	if err != nil {
		t.Fatalf("list Google accounts: %v", err)
	}
	if len(connections) != 2 {
		t.Fatalf("Google account count = %d, want 2", len(connections))
	}
	for _, test := range []struct {
		connectionID, wantAccess, wantRefresh string
	}{
		{firstID, "calendar-access-token", "calendar-refresh-token"},
		{secondID, "mail-access-token", "mail-refresh-token"},
	} {
		credentials, err := fixture.store.GetGoogleCredentials(ctx, userID, test.connectionID, encryptionKey)
		if err != nil {
			t.Fatalf("get credentials for %s: %v", test.connectionID, err)
		}
		if credentials.AccessToken != test.wantAccess || credentials.RefreshToken != test.wantRefresh {
			t.Fatalf("credentials for %s crossed accounts", test.connectionID)
		}
	}

	if err := fixture.store.RevokeGoogleConnection(ctx, userID, firstID); err != nil {
		t.Fatalf("revoke first Google account: %v", err)
	}
	if _, err := fixture.store.GetGoogleCredentials(ctx, userID, firstID, encryptionKey); !errors.Is(err, postgres.ErrNotFound) {
		t.Fatalf("revoked account credentials error = %v, want not found", err)
	}
	remaining, err := fixture.store.GetGoogleCredentials(ctx, userID, secondID, encryptionKey)
	if err != nil || remaining.AccessToken != "mail-access-token" {
		t.Fatalf("second Google account after first revoke = %+v, err %v", remaining, err)
	}
}

type googleAccountProviderCall struct {
	method        string
	path          string
	authorization string
}

type googleAccountProviderTransport struct {
	calls []googleAccountProviderCall
}

func (transport *googleAccountProviderTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.calls = append(transport.calls, googleAccountProviderCall{
		method:        request.Method,
		path:          request.URL.Path,
		authorization: request.Header.Get("Authorization"),
	})
	if request.Method != http.MethodPost || request.URL.Host != "gmail.googleapis.com" ||
		request.URL.Path != "/gmail/v1/users/me/messages/send" {
		return nil, fmt.Errorf("unexpected Google provider request: %s %s", request.Method, request.URL)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"mock-sent-message","threadId":"mock-thread"}`)),
		Request:    request,
	}, nil
}

func TestGoogleActionAfterSiblingAccountRevocationUsesOnlySurvivor(t *testing.T) {
	fixture := newGoogleOAuthFixture(t)
	ctx := context.Background()
	userID := "google-revocation-isolation-user"
	encryptionKey := []byte("12345678901234567890123456789012")

	revokedCalendarID, err := fixture.store.UpsertGoogleConnection(
		ctx, userID, "calendar@example.test", "google-revoked-calendar-subject", "Calendar", "",
		[]string{scopeCalendar}, []string{"calendar"},
		"revoked-calendar-access-token", "revoked-calendar-refresh-token",
		time.Now().Add(time.Hour), "Bearer", encryptionKey,
	)
	if err != nil {
		t.Fatalf("create Calendar identity: %v", err)
	}
	survivingGmailID, err := fixture.store.UpsertGoogleConnection(
		ctx, userID, "mail@example.test", "google-surviving-mail-subject", "Mail", "",
		[]string{scopeGmailSend}, []string{"gmail.send"},
		"surviving-mail-access-token", "surviving-mail-refresh-token",
		time.Now().Add(time.Hour), "Bearer", encryptionKey,
	)
	if err != nil {
		t.Fatalf("create Gmail identity: %v", err)
	}
	if revokedCalendarID == survivingGmailID {
		t.Fatal("different Google identities reused the same connection")
	}

	handler := testGoogleOAuthIntegrationHandler(fixture, "", "", slog.Default())
	handler.cfg.Google.TokenEncryptionKey = encryptionKey
	provider := &googleAccountProviderTransport{}
	handler.client = &http.Client{Transport: provider}

	if err := fixture.store.RevokeGoogleConnection(ctx, userID, revokedCalendarID); err != nil {
		t.Fatalf("revoke Calendar identity: %v", err)
	}

	connectionID, email, err := handler.ResolveAssistantActionTarget(ctx, userID, "send_gmail")
	if err != nil {
		t.Fatalf("resolve surviving Gmail action target: %v", err)
	}
	if connectionID != survivingGmailID || email != "mail@example.test" {
		t.Fatalf("resolved Gmail target = %q (%q), want surviving identity %q",
			connectionID, email, survivingGmailID)
	}

	gmailIntent, err := json.Marshal(assistantGoogleIntent{
		Tool:         "send_gmail",
		To:           "recipient@example.test",
		Subject:      "Account isolation check",
		Body:         "Deterministic provider-call test.",
		ConnectionID: connectionID,
	})
	if err != nil {
		t.Fatalf("encode Gmail action: %v", err)
	}
	result, executed, err := handler.ExecuteAssistantAction(
		ctx, userID, "google-revocation-gmail-run", "send_gmail", gmailIntent,
	)
	if err != nil || !executed {
		t.Fatalf("surviving Gmail action executed=%t err=%v", executed, err)
	}
	var safeResult map[string]string
	if err := json.Unmarshal(result, &safeResult); err != nil {
		t.Fatalf("decode Gmail action result: %v", err)
	}
	if safeResult["resourceId"] != "mock-sent-message" {
		t.Fatalf("Gmail action result = %s, want mock-sent-message", result)
	}

	calendarIntent, err := json.Marshal(assistantGoogleIntent{
		Tool:         "create_calendar_event",
		Title:        "Must not reach revoked Google identity",
		Start:        time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		End:          time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339),
		ConnectionID: revokedCalendarID,
		CalendarID:   "primary",
	})
	if err != nil {
		t.Fatalf("encode Calendar action: %v", err)
	}
	_, executed, err = handler.ExecuteAssistantAction(
		ctx, userID, "google-revocation-calendar-run", "create_calendar_event", calendarIntent,
	)
	var operation operationError
	if executed || !errors.As(err, &operation) ||
		operation.code != "CONNECTION_DISCONNECTED" || operation.status != http.StatusConflict {
		t.Fatalf("revoked Calendar action executed=%t error=%v; want safe CONNECTION_DISCONNECTED conflict", executed, err)
	}

	if len(provider.calls) != 1 {
		t.Fatalf("Google provider calls = %+v; want exactly one successful surviving-account request", provider.calls)
	}
	call := provider.calls[0]
	if call.method != http.MethodPost || call.path != "/gmail/v1/users/me/messages/send" ||
		call.authorization != "Bearer surviving-mail-access-token" {
		t.Fatalf("provider call = %+v; want Gmail send using only the surviving identity", call)
	}
}

func googleSessionCookie(t *testing.T, response *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == "askolo_sid" {
			return cookie
		}
	}
	t.Fatalf("Google callback did not set a native session cookie")
	return nil
}
