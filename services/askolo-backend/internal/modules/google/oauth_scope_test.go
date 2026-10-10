package google

import "testing"

func TestGmailReadScopeDoesNotGrantSendCapability(t *testing.T) {
	scopes, err := requestedScopes("gmail.read")
	if err != nil {
		t.Fatal(err)
	}
	if !hasString(scopes, scopeGmailRead) {
		t.Fatal("gmail.read must request the Gmail read-only scope")
	}
	if hasString(scopes, scopeGmailSend) {
		t.Fatal("gmail.read must not request the Gmail send scope")
	}
	capabilities := capabilitiesForScopes(scopes)
	if len(capabilities) != 1 || capabilities[0] != "gmail.read" {
		t.Fatalf("capabilitiesForScopes() = %v; want only gmail.read", capabilities)
	}
	if got := scopeForCapability("gmail.read"); got != scopeGmailRead {
		t.Fatalf("scopeForCapability(gmail.read) = %q; want %q", got, scopeGmailRead)
	}
}

func TestGrantedScopesFallbackToRequestedScopesWhenTokenOmitsScope(t *testing.T) {
	requested := []string{scopeOpenID, scopeEmail, scopeProfile, scopeGmailRead}

	got := grantedScopesFromToken("", requested)
	if len(got) != len(requested) {
		t.Fatalf("granted scopes = %v, want requested scopes %v", got, requested)
	}
	for _, scope := range requested {
		if !hasString(got, scope) {
			t.Fatalf("granted scopes %v do not include requested %q", got, scope)
		}
	}
	if capabilities := capabilitiesForScopes(got); len(capabilities) != 1 || capabilities[0] != "gmail.read" {
		t.Fatalf("capabilities from omitted token scope = %v, want gmail.read", capabilities)
	}
}

func TestGrantedScopesTrustNonemptyTokenScope(t *testing.T) {
	got := grantedScopesFromToken("openid email profile", []string{scopeGmailRead})
	if hasString(got, scopeGmailRead) {
		t.Fatalf("fallback added an unreported scope to nonempty provider response: %v", got)
	}
}
