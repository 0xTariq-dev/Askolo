package google

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"askolo/backend/internal/config"
)

func testOAuthHandler() *Handler {
	return NewHandler(config.Config{
		Environment:   "test",
		SessionSecret: "test-session-secret",
		AllowedOAuthHosts: map[string]struct{}{
			"localhost": {},
		},
		Google: config.GoogleOAuthConfig{
			LoginClientID:       "login-client",
			LoginClientSecret:   "login-secret",
			IntegrationClientID: "integration-client",
			IntegrationSecret:   "integration-secret",
			AuthURL:             "https://accounts.google.com/o/oauth2/v2/auth",
		},
	}, nil, slog.Default())
}

func TestOAuthStateIsSignedAndRejectsTampering(t *testing.T) {
	handler := testOAuthHandler()
	original := statePayload{
		Nonce:        "nonce",
		CodeVerifier: "verifier",
		ReturnTo:     "/dashboard",
		Flow:         "login",
		IssuedAt:     time.Now().Unix(),
	}
	encoded, err := handler.encodeState(original)
	if err != nil {
		t.Fatalf("encode state: %v", err)
	}
	decoded, err := handler.decodeState(encoded)
	if err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if decoded.Nonce != original.Nonce || decoded.Flow != original.Flow {
		t.Fatalf("state did not round trip: %#v", decoded)
	}
	if _, err := handler.decodeState(encoded + "tampered"); err == nil {
		t.Fatal("expected tampered state to be rejected")
	}
}

func TestRequestedScopesNeverIncludesRestrictedMailboxScope(t *testing.T) {
	scopes, err := requestedScopes("all")
	if err != nil {
		t.Fatalf("requested scopes: %v", err)
	}
	for _, scope := range scopes {
		if scope == "https://www.googleapis.com/auth/gmail.modify" {
			t.Fatal("release-1 scope list must not include gmail.modify")
		}
	}
	if !hasString(scopes, scopeGmail) || !hasString(scopes, scopeCalendar) {
		t.Fatalf("expected release-1 service scopes, got %v", scopes)
	}
}

func TestOAuthFlowScopesStaySeparate(t *testing.T) {
	loginScopes, err := scopesForFlow("login", "all")
	if err != nil {
		t.Fatalf("login scopes: %v", err)
	}
	wantLoginScopes := []string{scopeOpenID, scopeEmail, scopeProfile}
	if !reflect.DeepEqual(loginScopes, wantLoginScopes) {
		t.Fatalf("expected native login scopes %v, got %v", wantLoginScopes, loginScopes)
	}

	loginScopes, err = scopesForFlow("login", "not-a-valid-integration-scope")
	if err != nil {
		t.Fatalf("login should ignore integration scope selectors: %v", err)
	}
	if !reflect.DeepEqual(loginScopes, wantLoginScopes) {
		t.Fatalf("login scope selector changed native login scopes: %v", loginScopes)
	}

	integrationScopes, err := scopesForFlow("integration", "calendar")
	if err != nil {
		t.Fatalf("calendar integration scopes: %v", err)
	}
	wantIntegrationScopes := []string{scopeOpenID, scopeEmail, scopeProfile, scopeCalendar}
	if !reflect.DeepEqual(integrationScopes, wantIntegrationScopes) {
		t.Fatalf("expected calendar integration scopes %v, got %v", wantIntegrationScopes, integrationScopes)
	}
	if hasString(integrationScopes, "https://www.googleapis.com/auth/gmail.modify") {
		t.Fatal("integration scope list must not include gmail.modify")
	}
}

func TestOAuthFlowsUseDistinctClientsAndCallbacks(t *testing.T) {
	handler := testOAuthHandler()

	for _, flow := range []string{"login", "link"} {
		if got, err := handler.clientID(flow); err != nil || got != "login-client" {
			t.Fatalf("%s should use the login client, got %q, %v", flow, got, err)
		}
		if got, err := handler.clientSecret(flow); err != nil || got != "login-secret" {
			t.Fatalf("%s should use the login secret, got %q, %v", flow, got, err)
		}
	}
	if got, err := handler.clientID("integration"); err != nil || got != "integration-client" {
		t.Fatalf("integration should use the integration client, got %q, %v", got, err)
	}
	if got, err := handler.clientSecret("integration"); err != nil || got != "integration-secret" {
		t.Fatalf("integration should use the integration secret, got %q, %v", got, err)
	}

	if got := callbackPath("login"); got != "/api/auth/google/callback" {
		t.Fatalf("unexpected login callback path %q", got)
	}
	if got := callbackPath("link"); got != "/api/auth/google/link/callback" {
		t.Fatalf("unexpected link callback path %q", got)
	}
	if got := callbackPath("integration"); got != "/api/integrations/google/callback" {
		t.Fatalf("unexpected integration callback path %q", got)
	}
}

func TestSafeReturnToRejectsExternalRedirects(t *testing.T) {
	if got := safeReturnTo("https://evil.example"); got != "/dashboard" {
		t.Fatalf("expected external redirect to be rejected, got %q", got)
	}
	if got := safeReturnTo("//evil.example"); got != "/dashboard" {
		t.Fatalf("expected protocol-relative redirect to be rejected, got %q", got)
	}
	if got := safeReturnTo("/settings?tab=google"); got != "/settings?tab=google" {
		t.Fatalf("expected local redirect to survive, got %q", got)
	}
}

func TestCallbackBaseURLOnlyAllowsConfiguredHosts(t *testing.T) {
	handler := testOAuthHandler()
	request := httptest.NewRequest("GET", "http://localhost/api/auth/google", nil)
	request.Host = "localhost"
	base, err := handler.callbackBaseURL(request)
	if err != nil || base != "http://localhost" {
		t.Fatalf("expected localhost callback URL, got %q, %v", base, err)
	}
	request.Host = "attacker.example"
	if _, err := handler.callbackBaseURL(request); err == nil {
		t.Fatal("expected unconfigured host to be rejected")
	}
}

func TestCallbackBaseURLUsesCanonicalOriginForStaging(t *testing.T) {
	handler := testOAuthHandler()
	handler.cfg.Environment = "staging"
	handler.cfg.CanonicalOrigin = "https://staging.askolo.example"
	handler.cfg.AllowedOAuthHosts = map[string]struct{}{
		"staging.askolo.example": {},
	}
	request := httptest.NewRequest("GET", "https://proxy.example/api/auth/google", nil)
	request.Host = "proxy.example"
	request.Header.Set("X-Forwarded-Host", "proxy.example")
	request.Header.Set("X-Forwarded-Proto", "https")

	base, err := handler.callbackBaseURL(request)
	if err != nil {
		t.Fatalf("expected canonical staging origin, got error: %v", err)
	}
	if base != "https://staging.askolo.example" {
		t.Fatalf("expected canonical origin, got %q", base)
	}
}

func TestCallbackBaseURLRejectsCanonicalOriginOutsideAllowedHosts(t *testing.T) {
	handler := testOAuthHandler()
	handler.cfg.CanonicalOrigin = "https://staging.askolo.example"
	handler.cfg.AllowedOAuthHosts = map[string]struct{}{"localhost": {}}
	request := httptest.NewRequest("GET", "https://staging.askolo.example/api/auth/google", nil)
	request.Host = "staging.askolo.example"

	if _, err := handler.callbackBaseURL(request); err == nil {
		t.Fatal("expected canonical origin outside allowed hosts to be rejected")
	}
}

func TestGoogleCallbackRejectsExpiredState(t *testing.T) {
	handler := testOAuthHandler()
	encoded, err := handler.encodeState(statePayload{
		Nonce:        "expired-nonce",
		CodeVerifier: "verifier",
		ReturnTo:     "/dashboard",
		Flow:         "login",
		IssuedAt:     time.Now().Add(-stateTTL - time.Second).Unix(),
	})
	if err != nil {
		t.Fatalf("encode expired state: %v", err)
	}

	request := httptest.NewRequest(
		http.MethodGet,
		"http://localhost/api/auth/google/callback?state=expired-nonce",
		nil,
	)
	request.Host = "localhost"
	request.AddCookie(&http.Cookie{Name: stateCookieName, Value: encoded})
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected expired callback to return 400, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"code":"INVALID_STATE"`) {
		t.Fatalf("expected generic invalid-state response, got %q", response.Body.String())
	}
	if !strings.Contains(response.Header().Get("Set-Cookie"), "Max-Age=0") {
		t.Fatalf("expected expired state cookie to be cleared, got %q", response.Header().Get("Set-Cookie"))
	}
}

func TestGoogleCallbackRejectsTamperedState(t *testing.T) {
	handler := testOAuthHandler()
	encoded, err := handler.encodeState(statePayload{
		Nonce:        "tampered-nonce",
		CodeVerifier: "verifier",
		ReturnTo:     "/dashboard",
		Flow:         "login",
		IssuedAt:     time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("encode state: %v", err)
	}

	request := httptest.NewRequest(
		http.MethodGet,
		"http://localhost/api/auth/google/callback?state=tampered-nonce",
		nil,
	)
	request.Host = "localhost"
	request.AddCookie(&http.Cookie{Name: stateCookieName, Value: encoded + "tampered"})
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), `"code":"INVALID_STATE"`) {
		t.Fatalf("expected tampered callback to return generic 400, got %d %q", response.Code, response.Body.String())
	}
}

func TestGoogleCallbackRejectsStateFromDifferentFlow(t *testing.T) {
	handler := testOAuthHandler()
	encoded, err := handler.encodeState(statePayload{
		Nonce:        "integration-nonce",
		CodeVerifier: "verifier",
		ReturnTo:     "/dashboard",
		Flow:         "integration",
		IssuedAt:     time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("encode state: %v", err)
	}

	request := httptest.NewRequest(
		http.MethodGet,
		"http://localhost/api/auth/google/callback?state=integration-nonce&code=unused",
		nil,
	)
	request.Host = "localhost"
	request.AddCookie(&http.Cookie{Name: stateCookieName, Value: encoded})
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest ||
		!strings.Contains(response.Body.String(), `"code":"INVALID_STATE"`) {
		t.Fatalf("expected cross-flow state to be rejected, got %d %q", response.Code, response.Body.String())
	}
}

func TestGoogleFailureRedirectUsesSafeSameOriginPath(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://localhost/api/auth/google/callback", nil)
	request.Host = "localhost"
	response := httptest.NewRecorder()

	redirectStatus(response, request, "https://evil.example/account", "error")

	if response.Code != http.StatusFound {
		t.Fatalf("expected failure redirect, got %d", response.Code)
	}
	if location := response.Header().Get("Location"); location != "/dashboard?google=error" {
		t.Fatalf("expected safe dashboard redirect, got %q", location)
	}
}
