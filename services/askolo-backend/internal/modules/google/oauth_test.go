package google

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"askolo/backend/internal/adapters/postgres"
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

func TestMapGoogleCalendarEvent(t *testing.T) {
	timed, ok := mapGoogleCalendarEvent(providerCalendarEvent{
		ID: "google-event-1", Summary: "Planning", Description: "Weekly review",
		Start: providerCalendarDateTime{DateTime: "2026-10-14T12:30:00+02:00", TimeZone: "Africa/Cairo"},
		End:   providerCalendarDateTime{DateTime: "2026-10-14T13:30:00+02:00", TimeZone: "Africa/Cairo"},
	})
	if !ok || timed.AllDay || timed.Title != "Planning" || timed.TimeZone != "Africa/Cairo" ||
		!timed.StartsAt.Equal(time.Date(2026, 10, 14, 10, 30, 0, 0, time.UTC)) ||
		timed.EndsAt == nil || !timed.EndsAt.Equal(time.Date(2026, 10, 14, 11, 30, 0, 0, time.UTC)) {
		t.Fatalf("mapped timed event = %#v, valid = %t", timed, ok)
	}
	allDay, ok := mapGoogleCalendarEvent(providerCalendarEvent{
		ID: "google-event-2", Summary: "Holiday",
		Start: providerCalendarDateTime{Date: "2026-10-14", TimeZone: "Africa/Cairo"},
		End:   providerCalendarDateTime{Date: "2026-10-15", TimeZone: "Africa/Cairo"},
	})
	if !ok || !allDay.AllDay || allDay.TimeZone != "Africa/Cairo" ||
		allDay.StartsAt.Format("2006-01-02") != "2026-10-14" ||
		allDay.EndsAt == nil || allDay.EndsAt.Format("2006-01-02") != "2026-10-15" {
		t.Fatalf("mapped all-day event = %#v, valid = %t", allDay, ok)
	}
}

func TestLegacyCalendarEventRequiresWriteOptIn(t *testing.T) {
	handler := legacyCalendarEvent(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode adapted event request: %v", err)
		}
		if body["providerWriteOptIn"] != true || body["calendarId"] != "primary" {
			t.Errorf("adapted event request omitted explicit consent or calendar: %#v", body)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	for _, test := range []struct {
		name string
		body string
		want int
	}{
		{name: "missing consent", body: `{"calendarId":"primary","title":"Test"}`, want: http.StatusForbidden},
		{name: "consented", body: `{"calendarId":"primary","providerWriteOptIn":true,"title":"Test"}`, want: http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/google/calendar/events", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			handler(response, req)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.want, response.Body.String())
			}
		})
	}
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
	if !hasString(scopes, scopeGmailRead) || !hasString(scopes, scopeCalendar) {
		t.Fatalf("expected read-only release-1 service scopes, got %v", scopes)
	}
	if hasString(scopes, scopeGmailSend) || hasString(scopes, scopeCalendarWrite) {
		t.Fatalf("release-1 must not request provider write scopes, got %v", scopes)
	}
}

func TestGmailIntegrationRequestsReadOnlyCapability(t *testing.T) {
	scopes, err := requestedScopes("gmail")
	if err != nil {
		t.Fatalf("Gmail integration scopes: %v", err)
	}
	if !hasString(scopes, scopeGmailRead) || hasString(scopes, scopeGmailSend) {
		t.Fatalf("Gmail integration scopes = %v; want only Gmail read scope", scopes)
	}
	capabilities := capabilitiesForScopes(scopes)
	if len(capabilities) != 1 || capabilities[0] != "gmail.read" {
		t.Fatalf("Gmail integration capabilities = %v; want only gmail.read", capabilities)
	}
	if hasString(scopes, "https://www.googleapis.com/auth/gmail.modify") {
		t.Fatal("Gmail integration must not request the broader gmail.modify scope")
	}
}

func TestGoogleConnectionStatusMatchesLegacyClientContract(t *testing.T) {
	connections := []postgres.ProviderConnection{
		{
			Status: "active", Scopes: []string{scopeCalendar},
			Capabilities: []string{"calendar"},
		},
		{
			Status: "active", Scopes: []string{scopeGmailRead},
			Capabilities: []string{"gmail.read"},
		},
		{
			Status: "revoked", Scopes: []string{scopeGmailSend},
			Capabilities: []string{"gmail.send"},
		},
	}
	connected, calendarConnected, gmailConnected, scopes := summarizeGoogleConnections(connections)
	if !connected || !calendarConnected || !gmailConnected {
		t.Fatalf("status = connected:%v calendar:%v gmail:%v; want true, true, true", connected, calendarConnected, gmailConnected)
	}
	if !hasString(scopes, scopeCalendar) || !hasString(scopes, scopeGmailRead) || hasString(scopes, scopeGmailSend) {
		t.Fatalf("status scopes = %v; want active scopes only", scopes)
	}

	connections = append(connections, postgres.ProviderConnection{
		Status: "active", Scopes: []string{scopeGmailSend},
		Capabilities: []string{"gmail.send"},
	})
	connected, calendarConnected, gmailConnected, _ = summarizeGoogleConnections(connections)
	if !connected || !calendarConnected || !gmailConnected {
		t.Fatalf("status after activating Gmail send = connected:%v calendar:%v gmail:%v; want all true",
			connected, calendarConnected, gmailConnected)
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
	if hasString(integrationScopes, scopeCalendarWrite) {
		t.Fatal("calendar integration must not request calendar write scope")
	}
	gmailScopes, err := scopesForFlow("integration", "gmail")
	if err != nil {
		t.Fatalf("gmail integration scopes: %v", err)
	}
	if !hasString(gmailScopes, scopeGmailRead) || hasString(gmailScopes, scopeGmailSend) {
		t.Fatalf("gmail integration scopes must be read-only, got %v", gmailScopes)
	}
}

func TestOnlyIntegrationAuthorizationCarriesGrantedScopes(t *testing.T) {
	for _, flow := range []string{"login", "link"} {
		params := url.Values{}
		addFlowAuthorizationOptions(params, flow)
		if params.Get("prompt") != "select_account" {
			t.Fatalf("%s authorization prompt = %q, want select_account", flow, params.Get("prompt"))
		}
		for _, key := range []string{"access_type", "include_granted_scopes"} {
			if params.Has(key) {
				t.Fatalf("%s authorization unexpectedly set %s=%q", flow, key, params.Get(key))
			}
		}
	}

	params := url.Values{}
	addFlowAuthorizationOptions(params, "integration")
	if params.Get("access_type") != "offline" ||
		params.Get("prompt") != "consent" ||
		params.Get("include_granted_scopes") != "true" {
		t.Fatalf("integration authorization options = %v", params)
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
