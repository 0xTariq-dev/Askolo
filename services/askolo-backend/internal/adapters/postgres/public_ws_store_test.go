package postgres

import (
	"strings"
	"testing"
	"time"
)

func TestPublicWSMetadataValidation(t *testing.T) {
	validID := "user-123"
	if !validatePublicWSString(validID, true) {
		t.Fatal("expected canonical identifier to be accepted")
	}
	for _, value := range []string{"", " user-123 ", "user';DROP TABLE users;--", strings.Repeat("x", 201)} {
		if validatePublicWSString(value, true) {
			t.Fatalf("accepted invalid identifier %q", value)
		}
	}
	if !publicWSHashPattern.MatchString(strings.Repeat("a", 64)) {
		t.Fatal("expected SHA-256 hash to be accepted")
	}
	for _, hash := range []string{"", strings.Repeat("A", 64), strings.Repeat("a", 63)} {
		if publicWSHashPattern.MatchString(hash) {
			t.Fatalf("accepted invalid event hash %q", hash)
		}
	}
}

func TestPublicWSLeaseBounds(t *testing.T) {
	if err := validatePublicWSLease(time.Second); err != nil {
		t.Fatalf("valid lease rejected: %v", err)
	}
	for _, ttl := range []time.Duration{0, -time.Second, PublicWSMaxLease + time.Second} {
		if err := validatePublicWSLease(ttl); err == nil {
			t.Fatalf("invalid lease %s accepted", ttl)
		}
	}
}
