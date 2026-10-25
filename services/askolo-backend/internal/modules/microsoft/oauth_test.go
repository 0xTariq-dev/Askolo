package microsoft

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"askolo/backend/internal/config"
)

func testHandler() *Handler {
	return NewHandler(config.Config{
		Environment:       "test",
		SessionSecret:     "test-session-secret-with-sufficient-entropy",
		AllowedOAuthHosts: map[string]struct{}{"localhost": {}},
		Microsoft: config.MicrosoftOAuthConfig{
			ClientID:           "microsoft-client",
			ClientSecret:       "microsoft-secret",
			TokenEncryptionKey: []byte("01234567890123456789012345678901"),
		},
	}, nil, slog.Default())
}

func TestOAuthStateIsSignedAndRejectsTampering(t *testing.T) {
	handler := testHandler()
	original := statePayload{
		Nonce: "state-nonce", CodeVerifier: "pkce-verifier", ReturnTo: "/settings/account/profile",
		UserID: "user-1", IssuedAt: time.Now().Unix(),
	}
	encoded, err := handler.encodeState(original)
	if err != nil {
		t.Fatalf("encode state: %v", err)
	}
	decoded, err := handler.decodeState(encoded)
	if err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if decoded != original {
		t.Fatalf("state = %#v, want %#v", decoded, original)
	}
	if _, err := handler.decodeState(encoded + "tampered"); err == nil {
		t.Fatal("tampered OAuth state was accepted")
	}
}

func TestCallbackBaseURLUsesCanonicalOriginAndAllowlistedHosts(t *testing.T) {
	handler := testHandler()
	handler.cfg.CanonicalOrigin = "https://app.example.test"
	handler.cfg.AllowedOAuthHosts["app.example.test"] = struct{}{}
	request := httptest.NewRequest(http.MethodGet, "http://attacker.example.test/callback", nil)
	if got, err := handler.callbackBaseURL(request); err != nil || got != "https://app.example.test" {
		t.Fatalf("canonical callback origin = %q, %v", got, err)
	}

	handler.cfg.CanonicalOrigin = ""
	request = httptest.NewRequest(http.MethodGet, "http://localhost/callback", nil)
	request.Header.Set("X-Forwarded-Host", "attacker.example.test")
	if _, err := handler.callbackBaseURL(request); err == nil {
		t.Fatal("unallowlisted forwarded host was accepted")
	}
	request.Header.Set("X-Forwarded-Host", "localhost")
	request.Header.Set("X-Forwarded-Proto", "javascript")
	if _, err := handler.callbackBaseURL(request); err == nil {
		t.Fatal("invalid local callback scheme was accepted")
	}
}

func TestMicrosoftTokenAndProfileRequestsAreBoundedAndAuthenticated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if r.Method != http.MethodPost {
				t.Errorf("token method = %s, want POST", r.Method)
			}
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse token form: %v", err)
			}
			if r.Form.Get("client_id") != "microsoft-client" ||
				r.Form.Get("client_secret") != "microsoft-secret" ||
				r.Form.Get("code_verifier") != "verifier" {
				t.Errorf("token form omitted configured client or PKCE values: %#v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(tokenResponse{
				AccessToken: "access-account-one", RefreshToken: "refresh-account-one",
				ExpiresIn: 3600, Scope: "openid User.Read offline_access", TokenType: "Bearer",
			})
		case "/me":
			if got := r.Header.Get("Authorization"); got != "Bearer access-account-one" {
				t.Errorf("Graph authorization = %q", got)
			}
			_, _ = w.Write([]byte(`{"id":"microsoft-subject","displayName":"Account One","mail":"one@example.com"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	handler := testHandler()
	handler.tokenEndpoint = server.URL + "/token"
	handler.graphEndpoint = server.URL + "/me"
	tokens, err := handler.exchangeCode(t.Context(), "http://localhost/callback", "auth-code", "verifier")
	if err != nil {
		t.Fatalf("exchange authorization code: %v", err)
	}
	if tokens.AccessToken != "access-account-one" || tokens.RefreshToken != "refresh-account-one" {
		t.Fatalf("token response = %#v", tokens)
	}
	profile, err := handler.fetchProfile(t.Context(), tokens.AccessToken)
	if err != nil {
		t.Fatalf("fetch account profile: %v", err)
	}
	if profile.ID != "microsoft-subject" || profile.Mail != "one@example.com" {
		t.Fatalf("Microsoft profile = %#v", profile)
	}
}

func TestRedirectStatusAndReturnPathStayLocal(t *testing.T) {
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	redirectStatus(response, request, "//evil.example", "connected")
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	if location.Host != "" || location.Path != "/profile" || location.Query().Get("microsoft") != "connected" {
		t.Fatalf("redirect location = %q", location.String())
	}
	if got := safeReturnTo(strings.Repeat("x", 2050)); got != "/profile" {
		t.Fatalf("unsafe return path = %q", got)
	}
	for _, value := range []string{"/\\evil.example", "https://evil.example/path", "//evil.example"} {
		if got := safeReturnTo(value); got != "/profile" {
			t.Errorf("unsafe return path %q was retained as %q", value, got)
		}
	}
}
