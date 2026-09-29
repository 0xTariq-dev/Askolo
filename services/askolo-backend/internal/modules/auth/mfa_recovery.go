package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	authcrypto "askolo/backend/internal/platform/crypto"
)

var errInvalidFreshTOTP = errors.New("fresh authenticator code is invalid")

func (h *Handler) requestMFARecovery(w http.ResponseWriter, r *http.Request) {
	allowed, err := h.allowMFARecovery(r, "request", mfaRecoveryRequestRateLimit, mfaRecoveryRequestRateWindow)
	if err != nil {
		h.writeStoreError(w, "MFA recovery request rate check failed", err)
		return
	}
	if !allowed {
		h.recordSecurityEvent(r, "", "mfa_recovery_rate_limited", map[string]any{"stage": "request", "reason": "ip"})
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many recovery requests. Try again later.")
		return
	}
	var input struct {
		Email           string `json:"email"`
		CurrentPassword string `json:"currentPassword"`
	}
	if err := decodeJSON(w, r, &input); err != nil ||
		!validEmail(input.Email) || input.CurrentPassword == "" || len(input.CurrentPassword) > 256 {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Enter your account email and password.")
		return
	}
	email := normalizeEmail(input.Email)
	allowed, err = h.allowSharedAuthRateLimit(
		r, "account:mfa-recovery-request", email, mfaRecoveryRequestRateLimit, mfaRecoveryRequestRateWindow,
	)
	if err != nil {
		h.writeStoreError(w, "MFA recovery account rate check failed", err)
		return
	}
	if !allowed {
		h.recordSecurityEvent(r, "", "mfa_recovery_rate_limited", map[string]any{"stage": "request", "reason": "account"})
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many recovery requests. Try again later.")
		return
	}

	accepted := func() {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "mfa_recovery_if_available"})
	}
	user, err := h.store.FindUserByEmail(r.Context(), email)
	if errors.Is(err, postgres.ErrNotFound) {
		accepted()
		return
	}
	if err != nil {
		h.writeStoreError(w, "MFA recovery account lookup failed", err)
		return
	}
	if user.EmailVerifiedAt == nil || user.Status != "active" {
		accepted()
		return
	}
	passwordHash, err := h.store.PasswordHash(r.Context(), user.ID)
	if errors.Is(err, postgres.ErrNotFound) {
		accepted()
		return
	}
	if err != nil {
		h.writeStoreError(w, "MFA recovery reauthentication failed", err)
		return
	}
	if !VerifyPassword(passwordHash, input.CurrentPassword) {
		h.recordSecurityEvent(r, user.ID, "mfa_recovery_request_failed", map[string]any{"reason": "reauthentication"})
		accepted()
		return
	}
	enabled, err := h.store.TOTPEnabled(r.Context(), user.ID)
	if err != nil {
		h.writeStoreError(w, "MFA recovery status lookup failed", err)
		return
	}
	if !enabled {
		accepted()
		return
	}
	recoveryEmail, err := h.store.VerifiedRecoveryEmail(r.Context(), user.ID)
	if errors.Is(err, postgres.ErrRecoveryUnavailable) {
		accepted()
		return
	}
	if err != nil {
		h.writeStoreError(w, "MFA recovery method lookup failed", err)
		return
	}
	if err := h.challengeConfiguration(); err != nil {
		h.logChallengeFailure(r, "mfa_email_recovery", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	if err := h.sendChallenge(
		r, user.ID, recoveryEmail, "mfa_email_recovery", mfaRecoveryEmailPurpose,
		"Verify your Askolo MFA recovery request",
		"This code verifies your independent recovery email. Your password is also required. Completing recovery revokes active sessions and removes MFA; it will not sign you in.",
		true,
	); err != nil {
		if errors.Is(err, postgres.ErrChallengeRecentlySent) {
			setEmailChallengeRetryAfter(w)
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "A recovery message was sent recently. Try again later.")
			return
		}
		h.logChallengeFailure(r, "mfa_email_recovery", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	h.recordSecurityEvent(r, user.ID, "mfa_recovery_email_challenge_sent", map[string]any{"purpose": mfaRecoveryEmailPurpose})
	accepted()
}

func (h *Handler) verifyMFARecovery(w http.ResponseWriter, r *http.Request) {
	allowed, err := h.allowMFARecovery(r, "verification", mfaRecoveryVerificationRateLimit, mfaRecoveryVerificationRateWindow)
	if err != nil {
		h.writeStoreError(w, "MFA recovery verification rate check failed", err)
		return
	}
	if !allowed {
		h.recordSecurityEvent(r, "", "mfa_recovery_rate_limited", map[string]any{"stage": "verification", "reason": "ip"})
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many verification attempts. Try again later.")
		return
	}
	var input struct {
		Email           string `json:"email"`
		CurrentPassword string `json:"currentPassword"`
		Code            string `json:"code"`
	}
	if err := decodeJSON(w, r, &input); err != nil ||
		!validEmail(input.Email) || input.CurrentPassword == "" || len(input.CurrentPassword) > 256 ||
		!validChallengeCode(input.Code) {
		h.recordSecurityEvent(r, "", "mfa_recovery_verification_failed", map[string]any{"reason": "invalid_request"})
		writeError(w, http.StatusBadRequest, "INVALID_RECOVERY", "The recovery request is invalid or expired.")
		return
	}
	email := normalizeEmail(input.Email)
	allowed, err = h.allowSharedAuthRateLimit(
		r, "account:mfa-recovery-verification", email, mfaRecoveryVerificationRateLimit, mfaRecoveryVerificationRateWindow,
	)
	if err != nil {
		h.writeStoreError(w, "MFA recovery verification account rate check failed", err)
		return
	}
	if !allowed {
		h.recordSecurityEvent(r, "", "mfa_recovery_rate_limited", map[string]any{"stage": "verification", "reason": "account"})
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many verification attempts. Try again later.")
		return
	}
	invalid := func(userID string) {
		h.recordSecurityEvent(r, userID, "mfa_recovery_verification_failed", nil)
		writeError(w, http.StatusBadRequest, "INVALID_RECOVERY", "The recovery request is invalid or expired.")
	}
	user, err := h.store.FindUserByEmail(r.Context(), email)
	if errors.Is(err, postgres.ErrNotFound) {
		invalid("")
		return
	}
	if err != nil {
		h.writeStoreError(w, "MFA recovery verification lookup failed", err)
		return
	}
	if user.EmailVerifiedAt == nil || user.Status != "active" {
		invalid("")
		return
	}
	passwordHash, err := h.store.PasswordHash(r.Context(), user.ID)
	if errors.Is(err, postgres.ErrNotFound) {
		invalid(user.ID)
		return
	}
	if err != nil {
		h.writeStoreError(w, "MFA recovery verification reauthentication failed", err)
		return
	}
	if !VerifyPassword(passwordHash, input.CurrentPassword) {
		invalid(user.ID)
		return
	}
	enabled, err := h.store.TOTPEnabled(r.Context(), user.ID)
	if err != nil {
		h.writeStoreError(w, "MFA recovery verification status lookup failed", err)
		return
	}
	if !enabled {
		invalid(user.ID)
		return
	}
	recoveryEmail, err := h.store.VerifiedRecoveryEmail(r.Context(), user.ID)
	if errors.Is(err, postgres.ErrRecoveryUnavailable) {
		invalid(user.ID)
		return
	}
	if err != nil {
		h.writeStoreError(w, "MFA recovery verification method lookup failed", err)
		return
	}
	challengeUserID, err := h.consumeChallenge(r, recoveryEmail, mfaRecoveryEmailPurpose, input.Code)
	if errors.Is(err, postgres.ErrChallengeLocked) {
		h.recordSecurityEvent(r, user.ID, "mfa_recovery_rate_limited", map[string]any{"stage": "verification", "reason": "challenge_locked"})
		writeError(w, http.StatusTooManyRequests, "CHALLENGE_LOCKED", "The recovery request is temporarily locked.")
		return
	}
	if errors.Is(err, postgres.ErrChallengeInvalid) || err == nil && challengeUserID != user.ID {
		invalid(user.ID)
		return
	}
	if err != nil {
		h.writeStoreError(w, "MFA recovery code verification failed", err)
		return
	}
	if err := h.store.DisableMFAForEmailRecovery(r.Context(), user.ID); err != nil {
		h.recordSecurityEvent(r, user.ID, "mfa_recovery_disable_failed", nil)
		h.writeStoreError(w, "MFA recovery state update failed", err)
		return
	}
	if h.emailSender == nil {
		h.recordSecurityEvent(r, user.ID, "mfa_recovery_notification_failed", map[string]any{"reason": "email_unavailable"})
	} else if err := h.emailSender.Send(r.Context(), EmailMessage{
		To:      user.Email,
		Subject: "MFA recovery completed for your Askolo account",
		Body:    "MFA recovery was completed for your Askolo account. All active sessions and trusted devices were revoked, and MFA was removed. If you did not make this change, reset your password and contact support.",
	}); err != nil {
		h.logger.Error("MFA recovery notification delivery failed", "operation", "mfa_email_recovery", "request_id", requestID(r), "user_id", user.ID, "error", err)
		h.recordSecurityEvent(r, user.ID, "mfa_recovery_notification_failed", map[string]any{"reason": "delivery"})
	} else {
		h.recordSecurityEvent(r, user.ID, "mfa_recovery_notification_sent", map[string]any{"channel": "primary_email"})
	}
	h.recordSecurityEvent(r, user.ID, "mfa_recovered_by_verified_email", map[string]any{
		"identity_method":  "password_and_verified_recovery_email",
		"sessions_revoked": true,
		"mfa_changed":      true,
	})
	h.clearSessionCookie(w, r)
	h.clearTrustedDeviceCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "mfa_recovered_sign_in_required"})
}

func (h *Handler) verifyFreshTOTP(r *http.Request, userID, code string) error {
	if !validChallengeCode(code) {
		return errInvalidFreshTOTP
	}
	encryptedSecret, err := h.store.EncryptedTOTPSecret(r.Context(), userID)
	if err != nil {
		return err
	}
	secretBytes, err := authcrypto.Open(h.cfg.TOTPEncryptionKey, encryptedSecret)
	if err != nil {
		h.logger.Error("MFA secret decryption failed", "operation", "sensitive_action", "request_id", requestID(r), "user_id", userID, "error", err)
		h.recordSecurityEvent(r, userID, "mfa_decryption_failed", map[string]any{"operation": "sensitive_action"})
		return err
	}
	step, valid := verifyTOTP(string(secretBytes), code, time.Now().UTC())
	for i := range secretBytes {
		secretBytes[i] = 0
	}
	if !valid {
		return errInvalidFreshTOTP
	}
	return h.store.RecordFreshTOTP(r.Context(), userID, step)
}

func (h *Handler) writeFreshTOTPFailure(w http.ResponseWriter, r *http.Request, userID, operation string, err error) {
	if errors.Is(err, errInvalidFreshTOTP) || errors.Is(err, postgres.ErrMFAReplay) || errors.Is(err, postgres.ErrMFANotEnrolled) {
		h.recordSecurityEvent(r, userID, "mfa_fresh_totp_failed", map[string]any{"operation": operation})
		writeError(w, http.StatusUnauthorized, "REAUTHENTICATION_REQUIRED", "Enter a current authenticator code.")
		return
	}
	h.writeStoreError(w, "fresh MFA verification failed", err)
}

func (h *Handler) listTrustedDevices(w http.ResponseWriter, r *http.Request) {
	userID, status := h.fullSessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before viewing trusted devices.")
		return
	}
	devices, err := h.store.ListTrustedDevices(r.Context(), userID)
	if err != nil {
		h.writeStoreError(w, "trusted-device lookup failed", err)
		return
	}
	currentID := ""
	if device, err := TrustedDeviceForRequest(r.Context(), h.store, h.cfg, r, userID); err == nil {
		currentID = device.ID
	} else if !errors.Is(err, postgres.ErrTrustedDeviceNotFound) {
		h.writeStoreError(w, "current trusted-device lookup failed", err)
		return
	}
	result := make([]map[string]any, 0, len(devices))
	for _, device := range devices {
		result = append(result, map[string]any{
			"id": device.ID, "createdAt": device.CreatedAt, "lastUsedAt": device.LastUsedAt,
			"expiresAt": device.ExpiresAt, "current": device.ID == currentID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": result})
}

func (h *Handler) revokeTrustedDevice(w http.ResponseWriter, r *http.Request) {
	userID, status := h.fullSessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before revoking a trusted device.")
		return
	}
	if !h.allow(r, 5, 15*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many MFA changes. Try again later.")
		return
	}
	var input struct {
		TOTPCode string `json:"totpCode"`
	}
	if err := decodeJSON(w, r, &input); err != nil || !validChallengeCode(input.TOTPCode) {
		writeError(w, http.StatusBadRequest, "REAUTHENTICATION_REQUIRED", "A current authenticator code is required.")
		return
	}
	if err := h.verifyFreshTOTP(r, userID, input.TOTPCode); err != nil {
		h.recordSecurityEvent(r, userID, "trusted_device_revoke_failed", map[string]any{"reason": "fresh_totp_required"})
		writeError(w, http.StatusUnauthorized, "REAUTHENTICATION_REQUIRED", "Enter a current authenticator code.")
		return
	}
	deviceID := strings.TrimSpace(r.PathValue("deviceId"))
	if deviceID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A trusted device is required.")
		return
	}
	currentDeviceID := ""
	if current, err := TrustedDeviceForRequest(r.Context(), h.store, h.cfg, r, userID); err == nil {
		currentDeviceID = current.ID
	} else if !errors.Is(err, postgres.ErrTrustedDeviceNotFound) {
		h.writeStoreError(w, "current trusted-device lookup failed", err)
		return
	}
	if err := h.store.RevokeTrustedDevice(r.Context(), userID, deviceID); errors.Is(err, postgres.ErrTrustedDeviceNotFound) {
		writeError(w, http.StatusNotFound, "DEVICE_NOT_FOUND", "This trusted device is no longer available.")
		return
	} else if err != nil {
		h.writeStoreError(w, "trusted-device revocation failed", err)
		return
	}
	h.recordSecurityEvent(r, userID, "trusted_device_revoked", map[string]any{"device_id": deviceID})
	if currentDeviceID == deviceID {
		h.clearTrustedDeviceCookie(w, r)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "revoked"})
}

func TrustedDeviceForRequest(
	ctx context.Context, store *postgres.Store, cfg config.Config, r *http.Request, userID string,
) (postgres.TrustedDevice, error) {
	credential, err := r.Cookie(config.CookieName(trustedDeviceCookieBaseName))
	if err != nil || strings.TrimSpace(credential.Value) == "" {
		return postgres.TrustedDevice{}, postgres.ErrTrustedDeviceNotFound
	}
	fingerprint := deviceFingerprint(r)
	if fingerprint == "" {
		return postgres.TrustedDevice{}, postgres.ErrTrustedDeviceNotFound
	}
	credentialHash, err := deviceValueHash(cfg, userID, "credential", credential.Value)
	if err != nil {
		return postgres.TrustedDevice{}, err
	}
	fingerprintHash, err := deviceValueHash(cfg, userID, "fingerprint", fingerprint)
	if err != nil {
		return postgres.TrustedDevice{}, err
	}
	return store.MatchTrustedDevice(ctx, userID, credentialHash, fingerprintHash)
}

func deviceValueHash(cfg config.Config, userID, kind, value string) (string, error) {
	secret := strings.TrimSpace(cfg.AuthRateLimitHMACSecret)
	if len([]byte(secret)) < authRateLimitHMACSecretMinBytes {
		return "", errors.New("trusted-device HMAC secret is not configured")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("askolo:trusted-device:v1:" + userID + "\x00" + kind + "\x00" + value))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func deviceFingerprint(r *http.Request) string {
	fingerprint := strings.TrimSpace(r.Header.Get("X-Askolo-Device-Fingerprint"))
	if fingerprint == "" {
		if cookie, err := r.Cookie(deviceFingerprintCookieName); err == nil {
			fingerprint = strings.TrimSpace(cookie.Value)
		}
	}
	if len(fingerprint) != 64 {
		return ""
	}
	if _, err := hex.DecodeString(fingerprint); err != nil {
		return ""
	}
	return strings.ToLower(fingerprint)
}

func (h *Handler) issueTrustedDevice(w http.ResponseWriter, r *http.Request, userID string) error {
	fingerprint := deviceFingerprint(r)
	if fingerprint == "" {
		return errors.New("a stable browser fingerprint is required")
	}
	credentialBytes := make([]byte, 32)
	if _, err := rand.Read(credentialBytes); err != nil {
		return err
	}
	credential := hex.EncodeToString(credentialBytes)
	for i := range credentialBytes {
		credentialBytes[i] = 0
	}
	credentialHash, err := deviceValueHash(h.cfg, userID, "credential", credential)
	if err != nil {
		return err
	}
	fingerprintHash, err := deviceValueHash(h.cfg, userID, "fingerprint", fingerprint)
	if err != nil {
		return err
	}
	device, err := h.store.CreateTrustedDevice(r.Context(), userID, credentialHash, fingerprintHash, time.Now().Add(trustedDeviceTTL))
	if err != nil {
		return err
	}
	sessionID := h.sessionIDFromRequest(r)
	if err := h.store.AttachTrustedDeviceToSession(r.Context(), sessionID, userID, device.ID); err != nil {
		_ = h.store.RevokeTrustedDevice(r.Context(), userID, device.ID)
		return err
	}
	h.setTrustedDeviceCookie(w, r, credential)
	h.recordSecurityEvent(r, userID, "trusted_device_added", map[string]any{"expires_at": device.ExpiresAt})
	return nil
}

func (h *Handler) setTrustedDeviceCookie(w http.ResponseWriter, r *http.Request, credential string) {
	http.SetCookie(w, &http.Cookie{
		Name: config.CookieName(trustedDeviceCookieBaseName), Value: credential, Path: "/",
		HttpOnly: true, Secure: secureCookieRequest(r), SameSite: http.SameSiteLaxMode,
		MaxAge: int(trustedDeviceTTL.Seconds()),
	})
}

func (h *Handler) clearTrustedDeviceCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: config.CookieName(trustedDeviceCookieBaseName), Value: "", Path: "/",
		HttpOnly: true, Secure: secureCookieRequest(r), SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

func secureCookieRequest(r *http.Request) bool {
	host := firstHeader(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return !strings.HasPrefix(host, "localhost") && !strings.HasPrefix(host, "127.0.0.1")
}
