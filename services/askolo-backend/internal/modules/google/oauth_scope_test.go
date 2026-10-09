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
