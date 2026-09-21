package auth

import (
	"testing"
	"time"

	"askolo/backend/internal/config"
)

func TestTOTPMatchesRFC6238SHA1Vectors(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	tests := []struct {
		at   time.Time
		code string
	}{
		{time.Unix(59, 0).UTC(), "287082"},
		{time.Unix(1111111109, 0).UTC(), "081804"},
		{time.Unix(1111111111, 0).UTC(), "050471"},
		{time.Unix(1234567890, 0).UTC(), "005924"},
	}
	for _, test := range tests {
		got, err := totpCode(secret, test.at)
		if err != nil {
			t.Fatalf("totpCode() error = %v", err)
		}
		if got != test.code {
			t.Fatalf("totpCode(%v) = %s, want %s", test.at, got, test.code)
		}
		step, valid := verifyTOTP(secret, test.code, test.at)
		if !valid || step != totpStep(test.at) {
			t.Fatalf("verifyTOTP(%v) = (%d, %t), want (%d, true)", test.at, step, valid, totpStep(test.at))
		}
	}
}

func TestVerifyTOTPAllowsOneClockStepButRejectsBadInput(t *testing.T) {
	at := time.Unix(1_700_000_000, 0).UTC()
	code, err := totpCode("JBSWY3DPEHPK3PXP", at.Add(-totpPeriod))
	if err != nil {
		t.Fatalf("totpCode() error = %v", err)
	}
	step, valid := verifyTOTP("JBSWY3DPEHPK3PXP", code, at)
	if !valid || step != totpStep(at)-1 {
		t.Fatalf("verifyTOTP() = (%d, %t), want previous step", step, valid)
	}
	if _, valid := verifyTOTP("JBSWY3DPEHPK3PXP", "12345", at); valid {
		t.Fatal("verifyTOTP accepted a short code")
	}
}

func TestRecoveryCodesAreUniqueAndNormalized(t *testing.T) {
	display, raw, err := newRecoveryCodes()
	if err != nil {
		t.Fatalf("newRecoveryCodes() error = %v", err)
	}
	if len(display) != recoveryCodeCount || len(raw) != recoveryCodeCount {
		t.Fatalf("newRecoveryCodes() returned %d display and %d raw codes", len(display), len(raw))
	}
	seen := map[string]bool{}
	for i, code := range raw {
		if seen[code] {
			t.Fatalf("recovery code %q was duplicated", code)
		}
		seen[code] = true
		if !validRecoveryCode(code) || formatRecoveryCode(code) != display[i] {
			t.Fatalf("recovery code %q has an invalid display form %q", code, display[i])
		}
		if !validRecoveryCode(display[i]) {
			t.Fatalf("formatted recovery code %q was not accepted", display[i])
		}
	}
	if normalizeRecoveryCode(" abcd-ef12 3456-7890 ") != "ABCDEF1234567890" {
		t.Fatal("recovery code normalization did not remove separators")
	}
}

func TestRecoveryCodeHashDoesNotExposeCode(t *testing.T) {
	handler := NewHandler(config.Config{
		Email: config.EmailConfig{ChallengeSecret: "test-secret"},
	}, nil, nil)
	code := "ABCD-EF12-3456-7890"
	hash := handler.hashRecoveryCode(code)
	if hash == normalizeRecoveryCode(code) || len(hash) != 64 {
		t.Fatalf("recovery code hash = %q, expected a 32-byte digest", hash)
	}
	if hash != handler.hashRecoveryCode("abcd ef12 3456 7890") {
		t.Fatal("recovery-code hashing did not normalize equivalent input")
	}
}
