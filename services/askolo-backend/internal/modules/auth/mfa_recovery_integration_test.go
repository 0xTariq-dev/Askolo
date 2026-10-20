package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func mfaJSONRequest(
	t *testing.T,
	handler http.Handler,
	method, path string,
	input map[string]any,
	cookies []*http.Cookie,
	headers map[string]string,
	remote string,
) *httptest.ResponseRecorder {
	t.Helper()
	if input != nil {
		if action := testTurnstileAction(path); action != "" {
			input["turnstileToken"] = "test-turnstile:" + action
		}
	}
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("encode MFA request: %v", err)
		}
		body = strings.NewReader(string(raw))
	}
	request := httptest.NewRequest(method, path, body)
	request.RemoteAddr = remote
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func seedRecoveryCodes(t *testing.T, fixture *emailAuthFixture, userID string, hashes []string) {
	t.Helper()
	for index, hash := range hashes {
		if _, err := fixture.pool.Exec(context.Background(), `
			INSERT INTO auth_recovery_codes (id, user_id, code_hash)
			VALUES ($1, $2, $3)
		`, fmt.Sprintf("test-recovery-%s-%d", userID, index), userID, hash); err != nil {
			t.Fatalf("seed recovery code: %v", err)
		}
	}
}

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

func TestPasswordLoginRequiresMFAAndCompletesPendingSessionIntegration(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	rotatedKey := []byte("11234567890123456789012345678901")
	fixture.authConfig.TOTPEncryptionKey = rotatedKey
	fixture.authConfig.TOTPEncryptionKeyFallback = []byte(recoveryTestTOTPKey)
	const email = "mfa-login-primary@example.test"
	const password = "mfa-login-password-9!"
	userID := setupMFARecoveryUser(t, fixture, email, "mfa-login-recovery@example.test", password)
	handler := testAuthHandler(fixture, &captureEmailSender{}, slog.Default())

	login := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/login",
		map[string]string{"email": email, "password": password}, nil, "192.0.2.80:1234")
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), `"status":"mfa_required"`) {
		t.Fatalf("MFA-enabled password login = %d %s, want MFA challenge", login.Code, login.Body.String())
	}
	cookie := sessionCookie(t, login)

	pendingUser := plainRequest(handler, http.MethodGet, "/api/auth/user", cookie)
	if pendingUser.Code != http.StatusOK ||
		!strings.Contains(pendingUser.Body.String(), `"mfaRequired":true`) ||
		!strings.Contains(pendingUser.Body.String(), `"user":null`) {
		t.Fatalf("pending-session user response = %d %s, want MFA-required with no user", pendingUser.Code, pendingUser.Body.String())
	}
	protected := plainRequest(handler, http.MethodGet, "/api/auth/mfa/status", cookie)
	if protected.Code != http.StatusForbidden {
		t.Fatalf("protected MFA status with pending session = %d %s, want forbidden", protected.Code, protected.Body.String())
	}

	code, err := totpCode("JBSWY3DPEHPK3PXP", time.Now().UTC())
	if err != nil {
		t.Fatalf("generate integration TOTP: %v", err)
	}
	verified := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/verify",
		map[string]string{"code": code}, cookie, "192.0.2.80:1234")
	if verified.Code != http.StatusOK || !strings.Contains(verified.Body.String(), `"status":"authenticated"`) {
		t.Fatalf("TOTP challenge verification = %d %s, want authenticated", verified.Code, verified.Body.String())
	}
	reencryptedSecret, err := fixture.store.EncryptedTOTPSecret(context.Background(), userID)
	if err != nil {
		t.Fatalf("read migrated TOTP secret: %v", err)
	}
	plainSecret, err := authcrypto.Open(rotatedKey, reencryptedSecret)
	if err != nil || string(plainSecret) != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("TOTP secret was not re-encrypted with the active key: open error=%v", err)
	}
	for i := range plainSecret {
		plainSecret[i] = 0
	}
	authenticatedUser := plainRequest(handler, http.MethodGet, "/api/auth/user", cookie)
	if authenticatedUser.Code != http.StatusOK ||
		strings.Contains(authenticatedUser.Body.String(), `"mfaRequired":true`) ||
		strings.Contains(authenticatedUser.Body.String(), `"user":null`) {
		t.Fatalf("verified-session user response = %d %s, want authenticated user", authenticatedUser.Code, authenticatedUser.Body.String())
	}

	replayLogin := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/login",
		map[string]string{"email": email, "password": password}, nil, "192.0.2.81:1234")
	if replayLogin.Code != http.StatusOK || !strings.Contains(replayLogin.Body.String(), `"status":"mfa_required"`) {
		t.Fatalf("second password login = %d %s, want a new MFA challenge", replayLogin.Code, replayLogin.Body.String())
	}
	replay := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/verify",
		map[string]string{"code": code}, sessionCookie(t, replayLogin), "192.0.2.81:1234")
	if replay.Code != http.StatusUnauthorized || !strings.Contains(replay.Body.String(), "INVALID_MFA") {
		t.Fatalf("reused TOTP step = %d %s, want replay rejection", replay.Code, replay.Body.String())
	}
}

func TestConfirmMFAEnrollmentRevokesAllActiveSessionsIntegration(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	fixture.authConfig.TOTPEncryptionKey = []byte(recoveryTestTOTPKey)
	const email = "mfa-enrollment-primary@example.test"
	const recoveryEmail = "mfa-enrollment-recovery@example.test"
	const password = "mfa-enrollment-password-9!"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("hash test password: %v", err)
	}
	userID, err := fixture.store.CreatePasswordUser(context.Background(), email, hash)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	if _, err := fixture.pool.Exec(context.Background(), `
		UPDATE users SET status = 'active', email_verified_at = NOW() WHERE id = $1
	`, userID); err != nil {
		t.Fatalf("activate test user: %v", err)
	}
	if _, err := fixture.pool.Exec(context.Background(), `
		INSERT INTO auth_recovery_methods
		(id, user_id, kind, address, verified_at)
		VALUES ('recovery-method-' || $1, $1, 'email', $2, NOW())
	`, userID, recoveryEmail); err != nil {
		t.Fatalf("insert recovery method: %v", err)
	}
	currentSession, err := fixture.store.CreateSession(context.Background(), userID, "password", time.Hour)
	if err != nil {
		t.Fatalf("create current session: %v", err)
	}
	otherSession, err := fixture.store.CreateSession(context.Background(), userID, "google", time.Hour)
	if err != nil {
		t.Fatalf("create other-device session: %v", err)
	}
	cookie := &http.Cookie{Name: fixture.authConfig.SessionCookieName, Value: currentSession}
	handler := testAuthHandler(fixture, &captureEmailSender{}, slog.Default())
	enrollment := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/enroll",
		map[string]string{"currentPassword": password}, cookie, "192.0.2.82:1234")
	if enrollment.Code != http.StatusOK {
		t.Fatalf("MFA enrollment = %d %s, want success", enrollment.Code, enrollment.Body.String())
	}
	var enrollmentPayload struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(enrollment.Body.Bytes(), &enrollmentPayload); err != nil || enrollmentPayload.Secret == "" {
		t.Fatalf("decode enrollment secret: err=%v", err)
	}
	storedCiphertext, err := fixture.store.EncryptedTOTPSecret(context.Background(), userID)
	if err != nil {
		t.Fatalf("read encrypted enrollment secret: %v", err)
	}
	if storedCiphertext == enrollmentPayload.Secret {
		t.Fatal("enrollment secret was stored in plaintext")
	}
	storedPlaintext, err := authcrypto.Open(fixture.authConfig.TOTPEncryptionKey, storedCiphertext)
	if err != nil || string(storedPlaintext) != enrollmentPayload.Secret {
		t.Fatalf("stored enrollment secret could not be decrypted: %v", err)
	}
	for i := range storedPlaintext {
		storedPlaintext[i] = 0
	}
	code, err := totpCode(enrollmentPayload.Secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("generate enrollment confirmation code: %v", err)
	}
	confirmed := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/confirm",
		map[string]string{"code": code}, cookie, "192.0.2.82:1234")
	if confirmed.Code != http.StatusOK || !strings.Contains(confirmed.Body.String(), `"status":"enabled"`) {
		t.Fatalf("confirm MFA enrollment = %d %s, want enabled", confirmed.Code, confirmed.Body.String())
	}
	clearedCurrentCookie := false
	for _, resultCookie := range confirmed.Result().Cookies() {
		if resultCookie.Name == fixture.authConfig.SessionCookieName && resultCookie.MaxAge < 0 {
			clearedCurrentCookie = true
		}
	}
	if !clearedCurrentCookie {
		t.Fatal("MFA confirmation did not clear the current session cookie")
	}
	for _, sessionID := range []string{currentSession, otherSession} {
		if _, err := fixture.store.SessionUserID(context.Background(), sessionID); !errors.Is(err, postgres.ErrNotFound) {
			t.Fatalf("session %q remained valid after enabling MFA: %v", sessionID, err)
		}
	}
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

func TestRecoveryCodeChallengeIsSingleUseIntegration(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	fixture.authConfig.TOTPEncryptionKey = []byte(recoveryTestTOTPKey)
	const email = "mfa-recovery-code-once@example.test"
	const password = "recovery-once-password-9!"
	userID := setupMFARecoveryUser(t, fixture, email, "mfa-recovery-code-once-alt@example.test", password)
	auth := testAuthHandlerWithMonitor(fixture.authConfig, fixture.store, slog.Default(), &captureEmailSender{}, NewEmailDeliveryMonitor())
	handler := auth.Routes()
	displayCodes, rawCodes, err := newRecoveryCodes()
	if err != nil {
		t.Fatalf("generate recovery codes: %v", err)
	}
	seedRecoveryCodes(t, fixture, userID, []string{auth.hashRecoveryCode(rawCodes[0])})

	login := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/login",
		map[string]string{"email": email, "password": password}, nil, "192.0.2.110:1234")
	if login.Code != http.StatusOK || !strings.Contains(login.Body.String(), `"status":"mfa_required"`) {
		t.Fatalf("first login = %d %s, want MFA challenge", login.Code, login.Body.String())
	}
	firstVerification := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/verify",
		map[string]string{"code": displayCodes[0]}, sessionCookie(t, login), "192.0.2.110:1234")
	if firstVerification.Code != http.StatusOK {
		t.Fatalf("first recovery-code use = %d %s, want success", firstVerification.Code, firstVerification.Body.String())
	}

	relogin := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/login",
		map[string]string{"email": email, "password": password}, nil, "192.0.2.111:1234")
	if relogin.Code != http.StatusOK || !strings.Contains(relogin.Body.String(), `"status":"mfa_required"`) {
		t.Fatalf("second login = %d %s, want MFA challenge", relogin.Code, relogin.Body.String())
	}
	replay := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/verify",
		map[string]string{"code": displayCodes[0]}, sessionCookie(t, relogin), "192.0.2.111:1234")
	if replay.Code != http.StatusUnauthorized || !strings.Contains(replay.Body.String(), "INVALID_MFA") {
		t.Fatalf("replayed recovery code = %d %s, want rejection", replay.Code, replay.Body.String())
	}
}

func TestRecoveryCodeRegenerationReplacesSetIntegration(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	fixture.authConfig.TOTPEncryptionKey = []byte(recoveryTestTOTPKey)
	const email = "mfa-recovery-code-rotate@example.test"
	const password = "recovery-rotate-password-9!"
	userID := setupMFARecoveryUser(t, fixture, email, "mfa-recovery-code-rotate-alt@example.test", password)
	auth := testAuthHandlerWithMonitor(fixture.authConfig, fixture.store, slog.Default(), &captureEmailSender{}, NewEmailDeliveryMonitor())
	handler := auth.Routes()
	oldDisplay, oldRaw, err := newRecoveryCodes()
	if err != nil {
		t.Fatalf("generate original recovery codes: %v", err)
	}
	oldHashes := make([]string, 0, len(oldRaw))
	for _, code := range oldRaw {
		oldHashes = append(oldHashes, auth.hashRecoveryCode(code))
	}
	seedRecoveryCodes(t, fixture, userID, oldHashes)

	sessionID, err := fixture.store.CreateSession(context.Background(), userID, "password", time.Hour)
	if err != nil {
		t.Fatalf("create authenticated session: %v", err)
	}
	totpCodeValue, err := totpCode("JBSWY3DPEHPK3PXP", time.Now().UTC())
	if err != nil {
		t.Fatalf("generate current TOTP: %v", err)
	}
	regenerated := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/recovery-codes/regenerate",
		map[string]string{
			"currentPassword": password,
			"recoveryCode":    oldDisplay[0],
			"totpCode":        totpCodeValue,
		},
		&http.Cookie{Name: "askolo.sid", Value: sessionID}, "192.0.2.112:1234")
	if regenerated.Code != http.StatusOK {
		t.Fatalf("recovery code regeneration = %d %s, want success", regenerated.Code, regenerated.Body.String())
	}
	var payload struct {
		RecoveryCodes []string `json:"recoveryCodes"`
	}
	if err := json.Unmarshal(regenerated.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode regenerated codes: %v", err)
	}
	if len(payload.RecoveryCodes) != recoveryCodeCount {
		t.Fatalf("regenerated %d codes, want exactly %d", len(payload.RecoveryCodes), recoveryCodeCount)
	}
	var activeCodes int
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT count(*) FROM auth_recovery_codes WHERE user_id = $1 AND used_at IS NULL
	`, userID).Scan(&activeCodes); err != nil {
		t.Fatalf("count active recovery codes: %v", err)
	}
	if activeCodes != recoveryCodeCount {
		t.Fatalf("stored %d active codes, want exactly %d", activeCodes, recoveryCodeCount)
	}
	for _, hash := range oldHashes {
		var count int
		if err := fixture.pool.QueryRow(context.Background(), `
			SELECT count(*) FROM auth_recovery_codes WHERE user_id = $1 AND code_hash = $2
		`, userID, hash).Scan(&count); err != nil {
			t.Fatalf("check replaced recovery code: %v", err)
		}
		if count != 0 {
			t.Fatal("a previous recovery code remained usable after regeneration")
		}
	}

	login := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/login",
		map[string]string{"email": email, "password": password}, nil, "192.0.2.113:1234")
	pending := sessionCookie(t, login)
	oldCodeAttempt := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/verify",
		map[string]string{"code": oldDisplay[1]}, pending, "192.0.2.113:1234")
	if oldCodeAttempt.Code != http.StatusUnauthorized {
		t.Fatalf("old recovery code after regeneration = %d %s, want rejection", oldCodeAttempt.Code, oldCodeAttempt.Body.String())
	}
	newCodeAttempt := jsonRequest(t, handler, http.MethodPost, "/api/auth/mfa/verify",
		map[string]string{"code": payload.RecoveryCodes[0]}, pending, "192.0.2.113:1234")
	if newCodeAttempt.Code != http.StatusOK {
		t.Fatalf("new recovery code = %d %s, want success", newCodeAttempt.Code, newCodeAttempt.Body.String())
	}
}

func TestTrustedDeviceLoginRequiresMatchingFingerprintAndFreshTotpForRevocationIntegration(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	fixture.authConfig.TOTPEncryptionKey = []byte(recoveryTestTOTPKey)
	const email = "trusted-device-flow@example.test"
	const password = "trusted-device-flow-password-9!"
	const fingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	userID := setupMFARecoveryUser(t, fixture, email, "trusted-device-flow-alt@example.test", password)
	auth := testAuthHandlerWithMonitor(fixture.authConfig, fixture.store, slog.Default(), &captureEmailSender{}, NewEmailDeliveryMonitor())
	handler := auth.Routes()
	primaryLogin := jsonRequest(t, handler, http.MethodPost, "/api/auth/password/login",
		map[string]string{"email": email, "password": password}, nil, "192.0.2.114:1234")
	if primaryLogin.Code != http.StatusOK || !strings.Contains(primaryLogin.Body.String(), `"status":"mfa_required"`) {
		t.Fatalf("primary login = %d %s, want MFA challenge", primaryLogin.Code, primaryLogin.Body.String())
	}
	pendingSession := sessionCookie(t, primaryLogin)
	code, err := totpCode("JBSWY3DPEHPK3PXP", time.Now().UTC())
	if err != nil {
		t.Fatalf("generate TOTP: %v", err)
	}
	verified := mfaJSONRequest(t, handler, http.MethodPost, "/api/auth/mfa/verify",
		map[string]any{"code": code, "trustDevice": true},
		[]*http.Cookie{pendingSession},
		map[string]string{"X-Askolo-Device-Fingerprint": fingerprint},
		"192.0.2.114:1234")
	if verified.Code != http.StatusOK || !strings.Contains(verified.Body.String(), `"trustedDevice":true`) {
		t.Fatalf("trusted-device enrollment = %d %s, want trusted success", verified.Code, verified.Body.String())
	}
	var trustedCookie *http.Cookie
	for _, cookie := range verified.Result().Cookies() {
		if strings.Contains(cookie.Name, "trusted_device") {
			trustedCookie = cookie
			break
		}
	}
	if trustedCookie == nil || !trustedCookie.HttpOnly || !trustedCookie.Secure {
		t.Fatalf("trusted-device cookie missing or insecure: %#v", trustedCookie)
	}
	deviceCookies := []*http.Cookie{trustedCookie}
	listed := mfaJSONRequest(t, handler, http.MethodGet, "/api/auth/mfa/trusted-devices", nil,
		append(deviceCookies, pendingSession),
		map[string]string{"X-Askolo-Device-Fingerprint": fingerprint},
		"192.0.2.114:1234")
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"current":true`) {
		t.Fatalf("trusted-device list = %d %s, want current device", listed.Code, listed.Body.String())
	}

	trustedLogin := mfaJSONRequest(t, handler, http.MethodPost, "/api/auth/password/login",
		map[string]any{"email": email, "password": password}, deviceCookies,
		map[string]string{"X-Askolo-Device-Fingerprint": fingerprint},
		"192.0.2.115:1234")
	if trustedLogin.Code != http.StatusOK || !strings.Contains(trustedLogin.Body.String(), `"status":"authenticated"`) ||
		strings.Contains(trustedLogin.Body.String(), `"status":"mfa_required"`) {
		t.Fatalf("trusted-device login = %d %s, want authenticated without MFA", trustedLogin.Code, trustedLogin.Body.String())
	}
	authenticatedSession := sessionCookie(t, trustedLogin)

	wrongFingerprintLogin := mfaJSONRequest(t, handler, http.MethodPost, "/api/auth/password/login",
		map[string]any{"email": email, "password": password}, deviceCookies,
		map[string]string{"X-Askolo-Device-Fingerprint": strings.Repeat("f", 64)},
		"192.0.2.116:1234")
	if wrongFingerprintLogin.Code != http.StatusOK ||
		!strings.Contains(wrongFingerprintLogin.Body.String(), `"status":"mfa_required"`) {
		t.Fatalf("trusted device with a different fingerprint = %d %s, want MFA challenge", wrongFingerprintLogin.Code, wrongFingerprintLogin.Body.String())
	}

	var deviceID string
	if err := fixture.pool.QueryRow(context.Background(), `
		SELECT id FROM auth_trusted_devices WHERE user_id = $1 AND revoked_at IS NULL
	`, userID).Scan(&deviceID); err != nil {
		t.Fatalf("read trusted device id: %v", err)
	}
	reusedCode := jsonRequest(t, handler, http.MethodPost,
		"/api/auth/mfa/trusted-devices/"+deviceID+"/revoke",
		map[string]string{"totpCode": code}, authenticatedSession, "192.0.2.115:1234")
	if reusedCode.Code != http.StatusUnauthorized {
		t.Fatalf("revocation with reused TOTP = %d %s, want fresh-code rejection", reusedCode.Code, reusedCode.Body.String())
	}
	devices, err := fixture.store.ListTrustedDevices(context.Background(), userID)
	if err != nil || len(devices) != 1 {
		t.Fatalf("device remained listed after rejected stale TOTP: %d err=%v", len(devices), err)
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
