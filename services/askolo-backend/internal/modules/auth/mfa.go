package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	totpDigits           = 6
	totpPeriod           = 30 * time.Second
	totpClockSkewSteps   = int64(1)
	mfaChallengeTTL      = 5 * time.Minute
	mfaChallengeAttempts = 5
	recoveryCodeCount    = 4
)

func newTOTPSecret() (string, error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw), nil
}

func totpStep(at time.Time) int64 {
	return at.Unix() / int64(totpPeriod/time.Second)
}

func totpCode(secret string, at time.Time) (string, error) {
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(decoded) == 0 {
		return "", fmt.Errorf("invalid TOTP secret")
	}
	counter := make([]byte, 8)
	binary.BigEndian.PutUint64(counter, uint64(totpStep(at)))
	mac := hmac.New(sha1.New, decoded) // SHA-1 is required by RFC 6238's default profile.
	_, _ = mac.Write(counter)
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	return fmt.Sprintf("%06d", value%1000000), nil
}

func verifyTOTP(secret, code string, at time.Time) (int64, bool) {
	if !validChallengeCode(code) {
		return 0, false
	}
	current := totpStep(at)
	for offset := -totpClockSkewSteps; offset <= totpClockSkewSteps; offset++ {
		stepTime := time.Unix((current+offset)*int64(totpPeriod/time.Second), 0).UTC()
		expected, err := totpCode(secret, stepTime)
		if err != nil {
			return 0, false
		}
		if hmac.Equal([]byte(expected), []byte(code)) {
			return current + offset, true
		}
	}
	return 0, false
}

func newRecoveryCodes() ([]string, []string, error) {
	display := make([]string, 0, recoveryCodeCount)
	hashes := make([]string, 0, recoveryCodeCount)
	for i := 0; i < recoveryCodeCount; i++ {
		raw := make([]byte, 8)
		if _, err := rand.Read(raw); err != nil {
			return nil, nil, err
		}
		code := strings.ToUpper(hex.EncodeToString(raw))
		display = append(display, formatRecoveryCode(code))
		hashes = append(hashes, code)
	}
	return display, hashes, nil
}

func formatRecoveryCode(code string) string {
	code = normalizeRecoveryCode(code)
	if len(code) != 16 {
		return code
	}
	return code[:4] + "-" + code[4:8] + "-" + code[8:12] + "-" + code[12:]
}

func normalizeRecoveryCode(code string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
}

func validRecoveryCode(code string) bool {
	normalized := normalizeRecoveryCode(code)
	if len(normalized) != 16 {
		return false
	}
	for _, char := range normalized {
		if (char < '0' || char > '9') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}

func (h *Handler) hashRecoveryCode(code string) string {
	mac := hmac.New(sha256.New, []byte(h.cfg.Email.ChallengeSecret))
	_, _ = mac.Write([]byte(normalizeRecoveryCode(code)))
	return hex.EncodeToString(mac.Sum(nil))
}
