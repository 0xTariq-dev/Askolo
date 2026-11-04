package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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
	"askolo/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

const githubTrustedDeviceTestHMACSecret = "integration-github-trusted-device-hmac-secret"

func githubTrustedDeviceTestHash(userID, kind, value string) string {
	mac := hmac.New(sha256.New, []byte(githubTrustedDeviceTestHMACSecret))
	_, _ = mac.Write([]byte("askolo:trusted-device:v1:" + userID + "\x00" + kind + "\x00" + value))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestGitHubLoginRequiresMFAForEnabledAccount(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_URL"))
	if baseURL == "" {
		t.Skip("set ASKOLO_TEST_DATABASE_URL to run GitHub MFA integration tests")
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

	schema := fmt.Sprintf("askolo_github_mfa_%d", time.Now().UnixNano())
	quotedSchema := `"` + strings.ReplaceAll(schema, `"`, `""`) + `"`
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		adminPool.Close()
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminPool.Exec(context.Background(), "DROP SCHEMA "+quotedSchema+" CASCADE")
		adminPool.Close()
	})
	if _, err := adminPool.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE %s.sessions (
			sid varchar PRIMARY KEY,
			sess jsonb NOT NULL,
			expire timestamp NOT NULL
		);
		CREATE TABLE %s.auth_totp (
			user_id text PRIMARY KEY,
			secret_encrypted text NOT NULL,
			enabled_at timestamptz,
			last_used_step integer,
			created_at timestamptz DEFAULT now() NOT NULL,
			updated_at timestamptz DEFAULT now() NOT NULL
		);
		CREATE TABLE %s.auth_trusted_devices (
			id text PRIMARY KEY,
			user_id text NOT NULL,
			credential_hash text NOT NULL,
			fingerprint_hash text NOT NULL,
			created_at timestamptz DEFAULT now() NOT NULL,
			last_used_at timestamptz,
			expires_at timestamptz NOT NULL,
			revoked_at timestamptz
		);
		CREATE TABLE %s.google_oauth_states (
			nonce_hash text PRIMARY KEY,
			flow text NOT NULL,
			user_id text,
			return_to text NOT NULL,
			expires_at timestamptz NOT NULL,
			consumed_at timestamptz
		);
		CREATE TABLE %s.provider_accounts (
			id text PRIMARY KEY,
			user_id text NOT NULL,
			provider text NOT NULL,
			external_subject text NOT NULL,
			email text,
			login_enabled boolean DEFAULT true NOT NULL,
			email_verified boolean DEFAULT true NOT NULL
		);
	`, quotedSchema, quotedSchema, quotedSchema, quotedSchema, quotedSchema)); err != nil {
		t.Fatalf("create GitHub MFA test tables: %v", err)
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
		t.Fatalf("open GitHub MFA store: %v", err)
	}
	t.Cleanup(store.Close)
	if _, err := adminPool.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.auth_totp (user_id, secret_encrypted, enabled_at)
		VALUES ('github-mfa-user', 'opaque-test-ciphertext', NOW())
		;
		INSERT INTO %s.provider_accounts
			(id, user_id, provider, external_subject, email, login_enabled, email_verified)
		VALUES ('github-mfa-account', 'github-mfa-user', 'github', '12345', 'github-mfa@example.test', TRUE, TRUE)
	`, quotedSchema, quotedSchema)); err != nil {
		t.Fatalf("seed GitHub MFA account: %v", err)
	}

	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/token":
			_, _ = fmt.Fprint(w, `{"access_token":"controlled-github-token"}`)
		case "/user":
			_, _ = fmt.Fprint(w, `{"id":12345,"login":"mfa-user","name":"MFA User"}`)
		case "/emails":
			_, _ = fmt.Fprint(w, `[{"email":"github-mfa@example.test","primary":true,"verified":true}]`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	handler := NewHandler(config.Config{
		SessionSecret:           "github-mfa-integration-session-secret",
		SessionCookieName:       "askolo_sid",
		AuthRateLimitHMACSecret: githubTrustedDeviceTestHMACSecret,
		AllowedOAuthHosts:       map[string]struct{}{"localhost": {}},
		GitHub: config.GitHubOAuthConfig{
			ClientID:     "github-client",
			ClientSecret: "github-secret",
			AuthURL:      provider.URL + "/authorize",
			TokenURL:     provider.URL + "/token",
			UserURL:      provider.URL + "/user",
			EmailsURL:    provider.URL + "/emails",
		},
	}, store, slog.Default())
	tests := []struct {
		name               string
		expired            bool
		revoked            bool
		requestFingerprint string
		wantRedirect       string
		wantRequired       bool
		wantVerified       bool
	}{
		{
			name:               "valid remembered browser",
			requestFingerprint: strings.Repeat("1", 64),
			wantRedirect:       "success",
			wantVerified:       true,
		},
		{
			name:               "expired remembered browser",
			expired:            true,
			requestFingerprint: strings.Repeat("1", 64),
			wantRedirect:       "mfa_required",
			wantRequired:       true,
		},
		{
			name:               "revoked remembered browser",
			revoked:            true,
			requestFingerprint: strings.Repeat("1", 64),
			wantRedirect:       "mfa_required",
			wantRequired:       true,
		},
		{
			name:               "fingerprint mismatch",
			requestFingerprint: strings.Repeat("2", 64),
			wantRedirect:       "mfa_required",
			wantRequired:       true,
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const userID = "github-mfa-user"
			credential := fmt.Sprintf("github-trusted-browser-credential-%d", index)
			enrolledFingerprint := strings.Repeat("1", 64)
			if _, err := adminPool.Exec(ctx, fmt.Sprintf(`
				INSERT INTO %s.auth_trusted_devices
					(id, user_id, credential_hash, fingerprint_hash, expires_at, revoked_at)
				VALUES ($1, $2, $3, $4,
					CASE WHEN $5 THEN NOW() - INTERVAL '1 minute' ELSE NOW() + INTERVAL '1 hour' END,
					CASE WHEN $6 THEN NOW() END)
			`, quotedSchema), fmt.Sprintf("github-trusted-device-%d", index), userID,
				githubTrustedDeviceTestHash(userID, "credential", credential),
				githubTrustedDeviceTestHash(userID, "fingerprint", enrolledFingerprint),
				test.expired, test.revoked); err != nil {
				t.Fatalf("seed trusted browser: %v", err)
			}

			payload := statePayload{
				Nonce: fmt.Sprintf("github-mfa-nonce-%d", index), ReturnTo: "/dashboard",
				Flow: "login", Intent: "signin", IssuedAt: time.Now().Unix(),
			}
			encodedState, err := handler.encodeState(payload)
			if err != nil {
				t.Fatalf("encode GitHub state: %v", err)
			}
			nonceHash := sha256.Sum256([]byte(payload.Nonce))
			if err := store.InsertOAuthState(
				ctx, hex.EncodeToString(nonceHash[:]), "github_login", "", payload.ReturnTo, time.Now().Add(stateTTL),
			); err != nil {
				t.Fatalf("insert GitHub OAuth state: %v", err)
			}
			request := httptest.NewRequest(http.MethodGet,
				"http://localhost/api/auth/github/callback?state="+payload.Nonce+"&code=github-mfa-code", nil)
			request.Host = "localhost"
			request.AddCookie(&http.Cookie{Name: stateCookieName, Value: encodedState})
			request.AddCookie(&http.Cookie{Name: "askolo.trusted_device", Value: credential})
			request.AddCookie(&http.Cookie{Name: "askolo_device_fingerprint", Value: test.requestFingerprint})
			response := httptest.NewRecorder()
			handler.Routes().ServeHTTP(response, request)

			if response.Code != http.StatusFound || response.Header().Get("Location") != "/dashboard?github="+test.wantRedirect {
				t.Fatalf("GitHub login response = %d %q, want %s redirect", response.Code, response.Header().Get("Location"), test.wantRedirect)
			}
			var sessionCookie *http.Cookie
			for _, cookie := range response.Result().Cookies() {
				if cookie.Name == "askolo_sid" && cookie.Value != "" {
					sessionCookie = cookie
					break
				}
			}
			if sessionCookie == nil || sessionCookie.Value == "" {
				t.Fatal("GitHub MFA login did not issue a session cookie")
			}
			state, err := store.SessionMFAState(ctx, sessionCookie.Value)
			if err != nil {
				t.Fatalf("read callback MFA state: %v", err)
			}
			if state.UserID != userID || state.Required != test.wantRequired || state.Verified != test.wantVerified {
				t.Fatalf("GitHub callback MFA state = %+v, want required=%t verified=%t",
					state, test.wantRequired, test.wantVerified)
			}
		})
	}
}
