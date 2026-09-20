package google

import (
	"log/slog"
	"net/http/httptest"
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
			AuthURL: "https://accounts.google.com/o/oauth2/v2/auth",
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
