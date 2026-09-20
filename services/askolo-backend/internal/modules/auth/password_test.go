package auth

import "testing"

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword(hash, "wrong horse battery staple") {
		t.Fatal("expected wrong password to fail")
	}
}

func TestPasswordValidation(t *testing.T) {
	for _, password := range []string{"short", " leading-password", "trailing-password "} {
		if err := ValidatePassword(password); err == nil {
			t.Fatalf("expected invalid password %q", password)
		}
	}
}
