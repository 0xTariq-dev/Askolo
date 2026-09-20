package crypto

import "testing"

func TestSealRoundTripAndAuthentication(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	sealed, err := Seal(key, []byte("refresh-token"))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	opened, err := Open(key, sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(opened) != "refresh-token" {
		t.Fatalf("unexpected plaintext %q", opened)
	}
	if _, err := Open([]byte("abcdefghijklmnopqrstuvwxyz123456"), sealed); err == nil {
		t.Fatal("expected wrong key to fail authentication")
	}
}
