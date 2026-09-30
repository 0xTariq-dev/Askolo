package google

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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

func newGoogleOAuthFixture(t *testing.T) *googleOAuthIntegrationFixture {
	t.Helper()
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run Google OAuth integration tests")
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
		CHECK (preferred_locale IS NULL OR preferred_locale IN ('en', 'ar'))
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
`, prefix, prefix, prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix, prefix)
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
		Environment:       "test",
		SessionSecret:     "integration-google-oauth-session-secret",
		SessionCookieName: "askolo_sid",
		AllowedOAuthHosts: map[string]struct{}{"localhost": {}},
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

func serveGoogleLoginCallback(handler *Handler, payload statePayload, cookie *http.Cookie, query url.Values) *httptest.ResponseRecorder {
	query.Set("state", payload.Nonce)
	request := httptest.NewRequest(
		http.MethodGet,
		"http://localhost/api/auth/google/callback?"+query.Encode(),
		nil,
	)
	request.Host = "localhost"
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	handler.Routes().ServeHTTP(response, request)
	return response
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

func TestGoogleCallbackSignupRequiresVerifiedEmailAndCreatesNativeSession(t *testing.T) {
	t.Run("verified email creates session", func(t *testing.T) {
		fixture := newGoogleOAuthFixture(t)
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
