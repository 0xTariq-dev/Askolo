package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"askolo/backend/internal/adapters/postgres"
	authcrypto "askolo/backend/internal/platform/crypto"
)

const recoveryTestTOTPKey = "01234567890123456789012345678901"

func TestDeviceFingerprintCookieMatchesBrowserClient(t *testing.T) {
	const fingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: "askolo_device_fingerprint", Value: fingerprint})

	if got := deviceFingerprint(request); got != fingerprint {
		t.Fatalf("device fingerprint from browser cookie = %q, want %q", got, fingerprint)
	}
}

// setupMFARecoveryUser deliberately uses the fixture pool for state which is
// not part of the public signup flow. Values here are test-only and contain no
// production credentials.
func setupMFARecoveryUser(t *testing.T, fixture *emailAuthFixture, email, recoveryEmail, password string) string {
	t.Helper()
	ctx := context.Background()
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash test password: %v", err)
	}
	userID, err := fixture.store.CreatePasswordUser(ctx, email, hash)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx, `
		UPDATE users SET status = 'active', email_verified_at = NOW() WHERE id = $1
	`, userID); err != nil {
		t.Fatalf("verify test user: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx, `
		INSERT INTO auth_recovery_methods
			(id, user_id, kind, address, verified_at)
		VALUES ('recovery-method-' || $1, $1, 'email', $2, NOW())
	`, userID, recoveryEmail); err != nil {
		t.Fatalf("insert recovery method: %v", err)
	}
	encrypted, err := authcrypto.Seal([]byte(recoveryTestTOTPKey), []byte("JBSWY3DPEHPK3PXP"))
	if err != nil {
		t.Fatalf("seal test TOTP secret: %v", err)
	}
	if _, err := fixture.pool.Exec(ctx, `
		INSERT INTO auth_totp (user_id, secret_encrypted, enabled_at)
		VALUES ($1, $2, NOW())
	`, userID, encrypted); err != nil {
		t.Fatalf("insert test TOTP: %v", err)
	}
	return userID
}

func TestMFARecoveryVerifiedEmailTransitionIntegration(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	fixture.authConfig.TOTPEncryptionKey = []byte(recoveryTestTOTPKey)
	const email = "mfa-recovery-primary@example.test"
	const recoveryEmail = "mfa-recovery-independent@example.test"
	const password = "recovery-test-password-9!"
	userID := setupMFARecoveryUser(t, fixture, email, recoveryEmail, password)

	// Seed sessions and a device so the successful transition can prove that
	// the store operation is atomic and revokes both kinds of access.
	sessionID, err := fixture.store.CreateSession(context.Background(), userID, "password", time.Hour)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	device, err := fixture.store.CreateTrustedDevice(context.Background(), userID, "credential-hash-recovery", "fingerprint-hash-recovery", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create trusted device: %v", err)
	}
	trustedSession, err := fixture.store.CreateTrustedDeviceSession(context.Background(), userID, "password", time.Hour, device.ID)
	if err != nil {
		t.Fatalf("create trusted session: %v", err)
	}
	sender := &captureEmailSender{}
	handler := testAuthHandler(fixture, sender, slog.Default())

	wrong := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/recovery/request",
		map[string]string{"email": email, "currentPassword": "wrong-password"}, nil, "192.0.2.41:1234")
	if wrong.Code != http.StatusAccepted || sender.count() != 0 {
		t.Fatalf("wrong password unexpectedly changed recovery state: status=%d messages=%d", wrong.Code, sender.count())
	}
	unknown := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/recovery/request",
		map[string]string{"email": "unknown-mfa@example.test", "currentPassword": password}, nil, "192.0.2.42:1234")
	if unknown.Code != http.StatusAccepted || sender.count() != 0 {
		t.Fatalf("unknown account changed recovery state: status=%d messages=%d", unknown.Code, sender.count())
	}
	request := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/recovery/request",
		map[string]string{"email": email, "currentPassword": password}, nil, "192.0.2.43:1234")
	if request.Code != http.StatusAccepted {
		t.Fatalf("recovery request status=%d body=%s", request.Code, request.Body.String())
	}
	messages := sender.snapshot()
	if len(messages) != 1 || messages[0].To != recoveryEmail {
		t.Fatalf("recovery challenge recipient=%q, want %q", messages[0].To, recoveryEmail)
	}
	code := sender.codeForSubject(t, "Verify your Askolo MFA recovery request")
	verify := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/recovery/verify",
		map[string]string{"email": email, "currentPassword": password, "code": code}, nil, "192.0.2.43:1234")
	if verify.Code != http.StatusOK {
		t.Fatalf("recovery verification status=%d body=%s", verify.Code, verify.Body.String())
	}
	for _, cookie := range verify.Result().Cookies() {
		if cookie.Name == fixture.authConfig.SessionCookieName && cookie.Value != "" {
			t.Fatalf("recovery unexpectedly issued a session cookie")
		}
	}
	messages = sender.snapshot()
	if len(messages) != 2 || messages[1].To != email {
		t.Fatalf("completion notification recipient=%q, want %q", messages[len(messages)-1].To, email)
	}
	enabled, err := fixture.store.TOTPEnabled(context.Background(), userID)
	if err != nil {
		t.Fatalf("read recovered MFA state: %v", err)
	}
	if enabled {
		t.Fatal("MFA remained enabled after successful recovery")
	}
	if _, err := fixture.store.SessionUserID(context.Background(), sessionID); !errors.Is(err, postgres.ErrNotFound) {
		t.Fatalf("ordinary session survived recovery: %v", err)
	}
	if _, err := fixture.store.SessionUserID(context.Background(), trustedSession); !errors.Is(err, postgres.ErrNotFound) {
		t.Fatalf("trusted-device session survived recovery: %v", err)
	}
	replay := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/recovery/verify",
		map[string]string{"email": email, "currentPassword": password, "code": code}, nil, "192.0.2.43:1234")
	if replay.Code != http.StatusBadRequest {
		t.Fatalf("replayed recovery code status=%d body=%s", replay.Code, replay.Body.String())
	}
}

func TestTrustedDeviceStoreIntegration(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	userID := setupMFARecoveryUser(t, fixture, "trusted-store@example.test", "trusted-store-recovery@example.test", "trusted-store-password-9!")
	ctx := context.Background()
	active, err := fixture.store.CreateTrustedDevice(ctx, userID, "hashed-credential-a", "hashed-fingerprint-a", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create active device: %v", err)
	}
	var expiredID string
	if err := fixture.pool.QueryRow(ctx, `
		INSERT INTO auth_trusted_devices
			(id, user_id, credential_hash, fingerprint_hash, expires_at)
		VALUES ('expired-trusted-device', $1, 'hashed-credential-expired', 'hashed-fingerprint-expired', NOW() - INTERVAL '1 minute')
		RETURNING id
	`, userID).Scan(&expiredID); err != nil {
		t.Fatalf("insert expired device: %v", err)
	}
	revoked, err := fixture.store.CreateTrustedDevice(ctx, userID, "hashed-credential-revoked", "hashed-fingerprint-revoked", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create revoked device: %v", err)
	}
	if err := fixture.store.RevokeTrustedDevice(ctx, userID, revoked.ID); err != nil {
		t.Fatalf("revoke device: %v", err)
	}
	if _, err := fixture.store.MatchTrustedDevice(ctx, userID, "hashed-credential-a", "wrong-fingerprint"); !errors.Is(err, postgres.ErrTrustedDeviceNotFound) {
		t.Fatalf("wrong fingerprint matched device: %v", err)
	}
	matched, err := fixture.store.MatchTrustedDevice(ctx, userID, "hashed-credential-a", "hashed-fingerprint-a")
	if err != nil || matched.ID != active.ID {
		t.Fatalf("active hashed device did not match: id=%q err=%v", matched.ID, err)
	}
	if _, err := fixture.store.MatchTrustedDevice(ctx, userID, "hashed-credential-expired", "hashed-fingerprint-expired"); !errors.Is(err, postgres.ErrTrustedDeviceNotFound) {
		t.Fatalf("expired device matched: %v", err)
	}
	list, err := fixture.store.ListTrustedDevices(ctx, userID)
	if err != nil || len(list) != 1 || list[0].ID != active.ID {
		t.Fatalf("device list included expired/revoked rows: %#v err=%v", list, err)
	}
	sessionID, err := fixture.store.CreateTrustedDeviceSession(ctx, userID, "password", time.Hour, active.ID)
	if err != nil {
		t.Fatalf("create device session: %v", err)
	}
	if err := fixture.store.RevokeTrustedDevice(ctx, userID, active.ID); err != nil {
		t.Fatalf("revoke active device: %v", err)
	}
	if _, err := fixture.store.SessionUserID(ctx, sessionID); !errors.Is(err, postgres.ErrNotFound) {
		t.Fatalf("revoked device session remained valid: %v", err)
	}
}

func TestTrustedDeviceHashesNeverStoreRawBrowserValuesIntegration(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	userID := setupMFARecoveryUser(t, fixture, "trusted-privacy@example.test", "trusted-privacy-recovery@example.test", "trusted-privacy-password-9!")
	ctx := context.Background()
	rawCredential := strings.Repeat("b", 64)
	rawFingerprint := strings.Repeat("a", 64)
	credentialHash, err := deviceValueHash(fixture.authConfig, userID, "credential", rawCredential)
	if err != nil {
		t.Fatalf("hash test credential: %v", err)
	}
	fingerprintHash, err := deviceValueHash(fixture.authConfig, userID, "fingerprint", rawFingerprint)
	if err != nil {
		t.Fatalf("hash test fingerprint: %v", err)
	}
	device, err := fixture.store.CreateTrustedDevice(ctx, userID, credentialHash, fingerprintHash, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create device with hashed material: %v", err)
	}
	var storedCredentialHash, storedFingerprintHash string
	if err := fixture.pool.QueryRow(ctx, `
		SELECT credential_hash, fingerprint_hash FROM auth_trusted_devices WHERE id = $1
	`, device.ID).Scan(&storedCredentialHash, &storedFingerprintHash); err != nil {
		t.Fatalf("read trusted-device hashes: %v", err)
	}
	if storedCredentialHash != credentialHash || storedFingerprintHash != fingerprintHash ||
		storedCredentialHash == rawCredential || storedFingerprintHash == rawFingerprint ||
		strings.Contains(storedCredentialHash, rawCredential) || strings.Contains(storedFingerprintHash, rawFingerprint) {
		t.Fatal("trusted-device persistence did not retain only keyed hashes")
	}
}

func TestSensitiveTOTPReplayIntegration(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	userID := setupMFARecoveryUser(t, fixture, "totp-replay@example.test", "totp-replay-recovery@example.test", "totp-replay-password-9!")
	ctx := context.Background()
	if err := fixture.store.RecordFreshTOTP(ctx, userID, 987654); err != nil {
		t.Fatalf("record fresh TOTP: %v", err)
	}
	if err := fixture.store.RecordFreshTOTP(ctx, userID, 987654); !errors.Is(err, postgres.ErrMFAReplay) {
		t.Fatalf("replayed sensitive TOTP error=%v", err)
	}
	if err := fixture.store.RecordFreshTOTP(ctx, userID, 987653); !errors.Is(err, postgres.ErrMFAReplay) {
		t.Fatalf("older sensitive TOTP error=%v", err)
	}
}
