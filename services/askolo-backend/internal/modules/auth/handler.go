package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	authcrypto "askolo/backend/internal/platform/crypto"
	"askolo/backend/internal/platform/id"
)

const (
	sessionTTL                                   = 7 * 24 * time.Hour
	emailChallengeTTL                            = 3 * time.Minute
	emailChallengeResendWindow                   = 2 * time.Minute
	emailChallengeMaxAttempts                    = 5
	passwordRecoveryPrimaryEmail                 = "primary_email"
	passwordRecoveryEmail                        = "recovery_email"
	mfaRecoverySupportPurpose                    = "mfa_recovery_support"
	mfaSecurityWindow                            = 15 * time.Minute
	mfaFailureAlertThreshold                     = 20
	mfaReplayAlertThreshold                      = 5
	mfaLockoutAlertThreshold                     = 5
	mfaDecryptionAlertThreshold                  = 3
	mfaRecoveryRequestAlertThreshold             = 20
	mfaRecoveryVerificationFailureAlertThreshold = 5
	mfaRecoveryRateLimitAlertThreshold           = 5
	mfaRecoveryRevocationFailureAlertThreshold   = 1
)

type Handler struct {
	cfg          config.Config
	store        *postgres.Store
	logger       *slog.Logger
	limiter      *rateLimiter
	emailSender  EmailSender
	emailMonitor *EmailDeliveryMonitor
	mfaAlertMu   sync.Mutex
	mfaAlertKey  string
	mfaAlertAt   time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateEntry
}

type rateEntry struct {
	count   int
	resetAt time.Time
}

func NewHandler(cfg config.Config, store *postgres.Store, logger *slog.Logger) *Handler {
	return NewHandlerWithEmailSenderAndMonitor(cfg, store, logger, NewResendEmailSender(cfg), NewEmailDeliveryMonitor())
}

func NewHandlerWithEmailSender(
	cfg config.Config,
	store *postgres.Store,
	logger *slog.Logger,
	emailSender EmailSender,
) *Handler {
	return NewHandlerWithEmailSenderAndMonitor(cfg, store, logger, emailSender, NewEmailDeliveryMonitor())
}

func NewHandlerWithEmailSenderAndMonitor(
	cfg config.Config,
	store *postgres.Store,
	logger *slog.Logger,
	emailSender EmailSender,
	emailMonitor *EmailDeliveryMonitor,
) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if emailSender == nil {
		emailSender = NewResendEmailSender(cfg)
	}
	if emailMonitor == nil {
		emailMonitor = NewEmailDeliveryMonitor()
	}
	return &Handler{
		cfg:          cfg,
		store:        store,
		logger:       logger,
		limiter:      &rateLimiter{entries: make(map[string]rateEntry)},
		emailSender:  emailSender,
		emailMonitor: emailMonitor,
	}
}

type EmailDeliveryReadiness struct {
	Status                 string `json:"status"`
	ResendConfiguration    string `json:"resendConfiguration"`
	ChallengeConfiguration string `json:"challengeConfiguration"`
	EmailDeliverySnapshot
}

// MFASecurityReadiness is a bounded operational signal for MFA abuse and
// authenticator-path failures. It never returns secrets, codes, or identities.
type MFASecurityReadiness struct {
	Status                                   string   `json:"status"`
	Environment                              string   `json:"environment"`
	WindowMinutes                            int      `json:"windowMinutes"`
	FailureEvents                            int64    `json:"failureEvents"`
	ReplayEvents                             int64    `json:"replayEvents"`
	LockoutEvents                            int64    `json:"lockoutEvents"`
	DecryptionFailureEvents                  int64    `json:"decryptionFailureEvents"`
	RecoverySupportRequests                  int64    `json:"recoverySupportRequests"`
	RecoverySupportVerificationFailures      int64    `json:"recoverySupportVerificationFailures"`
	RecoverySupportRateLimited               int64    `json:"recoverySupportRateLimited"`
	RecoverySupportSessionRevocationFailures int64    `json:"recoverySupportSessionRevocationFailures"`
	AffectedUsers                            int64    `json:"affectedUsers"`
	Alert                                    bool     `json:"alert"`
	AlertReasons                             []string `json:"alertReasons,omitempty"`
}

func (h *Handler) EmailDeliveryReadiness() EmailDeliveryReadiness {
	resendStatus := h.cfg.Email.ResendConfigurationStatus()
	challengeStatus := h.cfg.Email.ChallengeConfigurationStatus()
	snapshot := h.emailMonitor.Snapshot()
	status := "unknown"
	switch {
	case resendStatus == "missing" || challengeStatus == "missing":
		status = "not_configured"
	case resendStatus == "invalid" || challengeStatus == "invalid":
		status = "configuration_invalid"
	case snapshot.Attempts == 0:
		status = "unknown"
	case snapshot.LastOutcome == EmailDeliveryHandoff:
		status = "healthy"
	case snapshot.ConsecutiveFailures >= 3:
		status = "persistent_failure"
	default:
		status = "transient_failure"
	}
	return EmailDeliveryReadiness{
		Status:                 status,
		ResendConfiguration:    resendStatus,
		ChallengeConfiguration: challengeStatus,
		EmailDeliverySnapshot:  snapshot,
	}
}

// MFASecurityReadiness reads the current rolling MFA signal. A signal query
// failure is reported as unavailable without affecting dependency readiness.
func (h *Handler) MFASecurityReadiness(ctx context.Context) MFASecurityReadiness {
	signal := MFASecurityReadiness{
		Status:        "unavailable",
		Environment:   h.cfg.Environment,
		WindowMinutes: int(mfaSecurityWindow / time.Minute),
	}
	if h.store == nil {
		return signal
	}
	summary, err := h.store.MFAEventSummary(ctx, time.Now().UTC().Add(-mfaSecurityWindow))
	if err != nil {
		h.logger.Warn("MFA security signal unavailable",
			"environment", h.cfg.Environment,
			"window_minutes", signal.WindowMinutes,
		)
		return signal
	}
	signal.Status = "available"
	signal.FailureEvents = summary.FailureEvents
	signal.ReplayEvents = summary.ReplayEvents
	signal.LockoutEvents = summary.LockoutEvents
	signal.DecryptionFailureEvents = summary.DecryptionFailureEvents
	signal.RecoverySupportRequests = summary.RecoverySupportRequests
	signal.RecoverySupportVerificationFailures = summary.RecoverySupportVerificationFailures
	signal.RecoverySupportRateLimited = summary.RecoverySupportRateLimited
	signal.RecoverySupportSessionRevocationFailures = summary.RecoverySupportSessionRevocationFailures
	signal.AffectedUsers = summary.AffectedUsers
	signal.AlertReasons = mfaAlertReasons(summary)
	signal.Alert = len(signal.AlertReasons) > 0
	if signal.Alert {
		h.logMFAAlert(signal)
	} else {
		h.resetMFAAlert()
	}
	return signal
}

func mfaAlertReasons(summary postgres.MFAEventSummary) []string {
	reasons := make([]string, 0, 8)
	if summary.FailureEvents >= mfaFailureAlertThreshold {
		reasons = append(reasons, "failure_events")
	}
	if summary.ReplayEvents >= mfaReplayAlertThreshold {
		reasons = append(reasons, "replay_events")
	}
	if summary.LockoutEvents >= mfaLockoutAlertThreshold {
		reasons = append(reasons, "lockout_events")
	}
	if summary.DecryptionFailureEvents >= mfaDecryptionAlertThreshold {
		reasons = append(reasons, "decryption_failure_events")
	}
	if summary.RecoverySupportRequests >= mfaRecoveryRequestAlertThreshold {
		reasons = append(reasons, "recovery_support_request_spike")
	}
	if summary.RecoverySupportVerificationFailures >= mfaRecoveryVerificationFailureAlertThreshold {
		reasons = append(reasons, "recovery_support_verification_failures")
	}
	if summary.RecoverySupportRateLimited >= mfaRecoveryRateLimitAlertThreshold {
		reasons = append(reasons, "recovery_support_rate_limited")
	}
	if summary.RecoverySupportSessionRevocationFailures >= mfaRecoveryRevocationFailureAlertThreshold {
		reasons = append(reasons, "recovery_support_session_revocation_failures")
	}
	return reasons
}

func (h *Handler) logMFAAlert(signal MFASecurityReadiness) {
	key := strings.Join(signal.AlertReasons, ",")
	now := time.Now()
	h.mfaAlertMu.Lock()
	shouldLog := key != h.mfaAlertKey || now.Sub(h.mfaAlertAt) >= mfaSecurityWindow
	if shouldLog {
		h.mfaAlertKey = key
		h.mfaAlertAt = now
	}
	h.mfaAlertMu.Unlock()
	if !shouldLog {
		return
	}
	h.logger.Warn("MFA verification failure spike",
		"alert", true,
		"environment", signal.Environment,
		"window_minutes", signal.WindowMinutes,
		"failure_events", signal.FailureEvents,
		"replay_events", signal.ReplayEvents,
		"lockout_events", signal.LockoutEvents,
		"decryption_failure_events", signal.DecryptionFailureEvents,
		"recovery_support_requests", signal.RecoverySupportRequests,
		"recovery_support_verification_failures", signal.RecoverySupportVerificationFailures,
		"recovery_support_rate_limited", signal.RecoverySupportRateLimited,
		"recovery_support_session_revocation_failures", signal.RecoverySupportSessionRevocationFailures,
		"affected_users", signal.AffectedUsers,
		"alert_reasons", key,
	)
}

func (h *Handler) resetMFAAlert() {
	h.mfaAlertMu.Lock()
	h.mfaAlertKey = ""
	h.mfaAlertAt = time.Time{}
	h.mfaAlertMu.Unlock()
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/user", h.user)
	mux.HandleFunc("GET /api/auth/session", h.user)
	mux.HandleFunc("POST /api/auth/logout", h.logout)
	mux.HandleFunc("POST /api/auth/password/signup", h.passwordSignup)
	mux.HandleFunc("POST /api/auth/password/login", h.passwordLogin)
	mux.HandleFunc("POST /api/auth/password/set", h.passwordSet)
	mux.HandleFunc("POST /api/auth/email/verify", h.verifyEmail)
	mux.HandleFunc("POST /api/auth/email/resend", h.resendEmail)
	mux.HandleFunc("POST /api/auth/recovery/email/enroll", h.enrollRecoveryEmail)
	mux.HandleFunc("POST /api/auth/recovery/email/verify", h.verifyRecoveryEmail)
	mux.HandleFunc("POST /api/auth/password/recovery/request", h.requestPasswordRecovery)
	mux.HandleFunc("POST /api/auth/password/recovery/verify", h.verifyPasswordRecovery)
	mux.HandleFunc("POST /api/auth/password/recovery/reset", h.resetPassword)
	mux.HandleFunc("POST /api/auth/mfa/recovery-support/request", h.requestMFARecoverySupport)
	mux.HandleFunc("POST /api/auth/mfa/recovery-support/verify", h.verifyMFARecoverySupport)
	mux.HandleFunc("GET /api/auth/mfa/status", h.mfaStatus)
	mux.HandleFunc("POST /api/auth/mfa/enroll", h.enrollMFA)
	mux.HandleFunc("POST /api/auth/mfa/confirm", h.confirmMFA)
	mux.HandleFunc("POST /api/auth/mfa/verify", h.verifyMFA)
	mux.HandleFunc("POST /api/auth/mfa/disable", h.disableMFA)
	mux.HandleFunc("POST /api/auth/mfa/recovery-codes/regenerate", h.regenerateRecoveryCodes)
	return mux
}

func (h *Handler) user(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeJSON(w, http.StatusOK, map[string]any{"user": nil})
		return
	}
	if state, err := h.store.SessionMFAState(r.Context(), h.sessionIDFromRequest(r)); err == nil && state.Required && !state.Verified {
		writeJSON(w, http.StatusOK, map[string]any{"user": nil, "mfaRequired": true})
		return
	}
	user, err := h.store.GetUser(r.Context(), userID)
	if err != nil {
		h.writeStoreError(w, "auth user lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user": map[string]any{
			"id":                user.ID,
			"email":             user.Email,
			"firstName":         user.FirstName,
			"lastName":          user.LastName,
			"profileImageUrl":   user.ProfileImageURL,
			"status":            user.Status,
			"emailVerified":     user.EmailVerifiedAt != nil,
			"accountCreatedVia": user.AccountCreatedVia,
			"authProvider":      user.AccountCreatedVia,
		},
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	sessionID := h.sessionIDFromRequest(r)
	if sessionID != "" && h.store != nil {
		if err := h.store.DeleteSession(r.Context(), sessionID); err != nil {
			h.logger.Warn("session deletion failed", "error", err)
		}
	}
	h.clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) passwordLogin(w http.ResponseWriter, r *http.Request) {
	if !h.allow(r, 10, 10*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many authentication attempts.")
		return
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil || !validEmail(input.Email) || len(input.Password) > 256 {
		writeError(w, http.StatusBadRequest, "INVALID_CREDENTIALS", "Email and password are required.")
		return
	}
	user, err := h.store.FindUserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(input.Email)))
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email or password is incorrect.")
			return
		}
		h.writeStoreError(w, "password login lookup failed", err)
		return
	}
	passwordHash, err := h.store.PasswordHash(r.Context(), user.ID)
	if err != nil || !VerifyPassword(passwordHash, input.Password) {
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Email or password is incorrect.")
		return
	}
	if user.Status == "suspended" || user.Status == "deleted" {
		writeError(w, http.StatusForbidden, "ACCOUNT_UNAVAILABLE", "This account is not available.")
		return
	}
	if user.Status == "pending_email_verification" {
		writeError(w, http.StatusForbidden, "EMAIL_VERIFICATION_REQUIRED", "Verify your email before signing in.")
		return
	}
	if user.Status == "pending_provider_onboarding" {
		writeError(w, http.StatusForbidden, "ACCOUNT_SETUP_REQUIRED", "Complete account setup before signing in.")
		return
	}
	mfaEnabled, err := h.store.TOTPEnabled(r.Context(), user.ID)
	if err != nil {
		h.writeStoreError(w, "password login MFA lookup failed", err)
		return
	}
	if mfaEnabled {
		h.createMFAPendingSession(w, r, user.ID)
		return
	}
	h.createSession(w, r, user.ID)
}

func (h *Handler) passwordSignup(w http.ResponseWriter, r *http.Request) {
	if !h.allow(r, 5, 10*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests. Try again later.")
		return
	}
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil || !validEmail(input.Email) {
		writeError(w, http.StatusBadRequest, "INVALID_SIGNUP", "A valid email and password are required.")
		return
	}
	if err := ValidatePassword(input.Password); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PASSWORD", "Password does not meet the security requirements.")
		return
	}
	if err := h.challengeConfiguration(); err != nil {
		h.logger.Error("email verification is unavailable", "operation", "password_signup", "request_id", requestID(r), "error", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PASSWORD", "Password does not meet the security requirements.")
		return
	}
	email := normalizeEmail(input.Email)
	userID, err := h.store.CreatePasswordUser(r.Context(), email, passwordHash)
	if errors.Is(err, postgres.ErrEmailExists) {
		// Keep signup indistinguishable for existing and unknown addresses.
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "verification_required"})
		return
	}
	if err != nil {
		h.writeStoreError(w, "password signup failed", err)
		return
	}
	if err := h.sendChallenge(r, userID, email, "password_signup", "email_verification", "Verify your Askolo email", "Use this code to verify your Askolo email.", false); err != nil {
		h.logChallengeFailure(r, "password_signup", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	h.recordSecurityEvent(r, userID, "password_signup_challenge_sent", map[string]any{"purpose": "email_verification"})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "verification_required"})
}

func (h *Handler) passwordSet(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before setting a password.")
		return
	}
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		Password        string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PASSWORD", "A valid password is required.")
		return
	}
	user, err := h.store.GetUser(r.Context(), userID)
	if err != nil {
		h.writeStoreError(w, "password setup user lookup failed", err)
		return
	}
	if user.Status == "suspended" || user.Status == "deleted" {
		writeError(w, http.StatusForbidden, "ACCOUNT_UNAVAILABLE", "This account is not available.")
		return
	}
	existingHash, err := h.store.PasswordHash(r.Context(), userID)
	if err != nil && !errors.Is(err, postgres.ErrNotFound) {
		h.writeStoreError(w, "password setup lookup failed", err)
		return
	}
	if existingHash != "" && !VerifyPassword(existingHash, input.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "The current password is incorrect.")
		return
	}
	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PASSWORD", "Password does not meet the security requirements.")
		return
	}
	if err := h.store.SetPassword(r.Context(), userID, passwordHash); err != nil {
		h.writeStoreError(w, "password setup failed", err)
		return
	}
	if err := h.store.ActivateProviderUserIfReady(r.Context(), userID); err != nil {
		h.logger.Warn("provider account activation check failed", "operation", "password_setup", "request_id", requestID(r), "user_id", userID, "error", err)
	}
	h.recordSecurityEvent(r, userID, "password_set", nil)
	writeJSON(w, http.StatusNoContent, nil)
}

func (h *Handler) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := decodeJSON(w, r, &input); err != nil || !validEmail(input.Email) || !validChallengeCode(input.Code) {
		writeError(w, http.StatusBadRequest, "INVALID_VERIFICATION", "The verification request is invalid or expired.")
		return
	}
	userID, err := h.consumeChallenge(r, input.Email, "email_verification", input.Code)
	if errors.Is(err, postgres.ErrChallengeLocked) {
		writeError(w, http.StatusTooManyRequests, "CHALLENGE_LOCKED", "The verification request is temporarily locked.")
		return
	}
	if errors.Is(err, postgres.ErrChallengeInvalid) {
		writeError(w, http.StatusBadRequest, "INVALID_VERIFICATION", "The verification request is invalid or expired.")
		return
	}
	if err != nil {
		h.writeStoreError(w, "email verification failed", err)
		return
	}
	if err := h.store.MarkEmailVerified(r.Context(), userID); err != nil {
		h.writeStoreError(w, "email verification state update failed", err)
		return
	}
	h.recordSecurityEvent(r, userID, "email_verified", map[string]any{"purpose": "email_verification"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}

func (h *Handler) resendEmail(w http.ResponseWriter, r *http.Request) {
	if !h.allow(r, 5, 15*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests. Try again later.")
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &input); err != nil || !validEmail(input.Email) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A valid email is required.")
		return
	}
	email := normalizeEmail(input.Email)
	user, err := h.store.FindUserByEmail(r.Context(), email)
	if errors.Is(err, postgres.ErrNotFound) {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "verification_required"})
		return
	}
	if err != nil {
		h.writeStoreError(w, "verification resend lookup failed", err)
		return
	}
	if user.Status != "pending_email_verification" {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "verification_required"})
		return
	}
	if recent, err := h.store.HasRecentEmailChallenge(r.Context(), email, "email_verification", time.Now().Add(-emailChallengeResendWindow)); err != nil {
		h.writeStoreError(w, "verification resend rate check failed", err)
		return
	} else if recent {
		setEmailChallengeRetryAfter(w)
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "A verification message was sent recently. Try again later.")
		return
	}
	if err := h.challengeConfiguration(); err != nil {
		h.logChallengeFailure(r, "verification_resend", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	if err := h.sendChallenge(r, user.ID, email, "verification_resend", "email_verification", "Verify your Askolo email", "Use this code to verify your Askolo email.", true); err != nil {
		if errors.Is(err, postgres.ErrChallengeRecentlySent) {
			setEmailChallengeRetryAfter(w)
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "A verification message was sent recently. Try again later.")
			return
		}
		h.logChallengeFailure(r, "verification_resend", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	h.recordSecurityEvent(r, user.ID, "email_verification_resent", map[string]any{"purpose": "email_verification"})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "verification_required"})
}

func (h *Handler) enrollRecoveryEmail(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before adding a recovery email.")
		return
	}
	if !h.allow(r, 5, 15*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests. Try again later.")
		return
	}
	var input struct {
		Email           string `json:"email"`
		CurrentPassword string `json:"currentPassword"`
	}
	if err := decodeJSON(w, r, &input); err != nil || !validEmail(input.Email) {
		writeError(w, http.StatusBadRequest, "INVALID_RECOVERY_EMAIL", "A valid recovery email is required.")
		return
	}
	user, err := h.store.GetUser(r.Context(), userID)
	if err != nil {
		h.writeStoreError(w, "recovery email user lookup failed", err)
		return
	}
	email := normalizeEmail(input.Email)
	if strings.EqualFold(email, user.Email) {
		writeError(w, http.StatusBadRequest, "INVALID_RECOVERY_EMAIL", "Choose a recovery email different from your sign-in email.")
		return
	}
	passwordHash, err := h.store.PasswordHash(r.Context(), userID)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "REAUTHENTICATION_REQUIRED", "Recent reauthentication is required.")
		return
	}
	if err != nil {
		h.writeStoreError(w, "recovery email reauthentication failed", err)
		return
	}
	if !VerifyPassword(passwordHash, input.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "REAUTHENTICATION_REQUIRED", "Recent reauthentication is required.")
		return
	}
	if recent, err := h.store.HasRecentEmailChallenge(r.Context(), email, "recovery_email_enrollment", time.Now().Add(-emailChallengeResendWindow)); err != nil {
		h.writeStoreError(w, "recovery email rate check failed", err)
		return
	} else if recent {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "A verification message was sent recently. Try again later.")
		return
	}
	if err := h.challengeConfiguration(); err != nil {
		h.logChallengeFailure(r, "recovery_email_enrollment", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	if err := h.sendChallenge(r, userID, email, "recovery_email_enrollment", "recovery_email_enrollment", "Confirm your Askolo recovery email", "Use this code to confirm your recovery email.", true); err != nil {
		if errors.Is(err, postgres.ErrChallengeRecentlySent) {
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "A verification message was sent recently. Try again later.")
			return
		}
		h.logChallengeFailure(r, "recovery_email_enrollment", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	h.recordSecurityEvent(r, userID, "recovery_email_challenge_sent", map[string]any{"purpose": "recovery_email_enrollment"})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "recovery_email_verification_required"})
}

func (h *Handler) verifyRecoveryEmail(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before confirming a recovery email.")
		return
	}
	var input struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := decodeJSON(w, r, &input); err != nil || !validEmail(input.Email) || !validChallengeCode(input.Code) {
		writeError(w, http.StatusBadRequest, "INVALID_RECOVERY_EMAIL", "The recovery email request is invalid or expired.")
		return
	}
	challengeUserID, err := h.consumeChallenge(r, input.Email, "recovery_email_enrollment", input.Code)
	if errors.Is(err, postgres.ErrChallengeLocked) {
		writeError(w, http.StatusTooManyRequests, "CHALLENGE_LOCKED", "The verification request is temporarily locked.")
		return
	}
	if errors.Is(err, postgres.ErrChallengeInvalid) || (err == nil && challengeUserID != userID) {
		writeError(w, http.StatusBadRequest, "INVALID_RECOVERY_EMAIL", "The recovery email request is invalid or expired.")
		return
	}
	if err != nil {
		h.writeStoreError(w, "recovery email verification failed", err)
		return
	}
	if err := h.store.UpsertVerifiedRecoveryEmail(r.Context(), userID, normalizeEmail(input.Email)); err != nil {
		h.writeStoreError(w, "recovery email persistence failed", err)
		return
	}
	if err := h.store.ActivateProviderUserIfReady(r.Context(), userID); err != nil {
		h.logger.Warn("provider account activation check failed", "operation", "recovery_email_verification", "request_id", requestID(r), "user_id", userID, "error", err)
	}
	h.recordSecurityEvent(r, userID, "recovery_email_verified", map[string]any{"purpose": "recovery_email_enrollment"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "recovery_email_verified"})
}

func (h *Handler) requestPasswordRecovery(w http.ResponseWriter, r *http.Request) {
	if !h.allow(r, 5, 15*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests. Try again later.")
		return
	}
	var input struct {
		Email  string `json:"email"`
		Method string `json:"method"`
	}
	if err := decodeJSON(w, r, &input); err != nil || !validEmail(input.Email) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A valid email is required.")
		return
	}
	method := normalizePasswordRecoveryMethod(input.Method)
	if !validPasswordRecoveryMethod(method) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Choose a valid password recovery method.")
		return
	}
	email := normalizeEmail(input.Email)
	user, deliveryEmail, err := h.passwordRecoveryTarget(r.Context(), email, method)
	if errors.Is(err, postgres.ErrNotFound) || errors.Is(err, postgres.ErrRecoveryUnavailable) {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "recovery_if_available"})
		return
	}
	if err != nil {
		h.writeStoreError(w, "password recovery lookup failed", err)
		return
	}
	if user.Status == "suspended" || user.Status == "deleted" || user.EmailVerifiedAt == nil {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "recovery_if_available"})
		return
	}
	if recent, err := h.store.HasRecentEmailChallenge(r.Context(), deliveryEmail, "password_recovery", time.Now().Add(-emailChallengeResendWindow)); err != nil {
		h.writeStoreError(w, "password recovery rate check failed", err)
		return
	} else if recent {
		setEmailChallengeRetryAfter(w)
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "A recovery message was sent recently. Try again later.")
		return
	}
	if err := h.challengeConfiguration(); err != nil {
		h.logChallengeFailure(r, "password_recovery", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	if err := h.sendChallenge(r, user.ID, deliveryEmail, "password_recovery", "password_recovery", "Reset your Askolo password", "Use this code to reset your Askolo password.", true); err != nil {
		if errors.Is(err, postgres.ErrChallengeRecentlySent) {
			setEmailChallengeRetryAfter(w)
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "A recovery message was sent recently. Try again later.")
			return
		}
		h.logChallengeFailure(r, "password_recovery", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	h.recordSecurityEvent(r, user.ID, "password_recovery_challenge_sent", map[string]any{"purpose": "password_recovery"})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "recovery_if_available"})
}

func (h *Handler) verifyPasswordRecovery(w http.ResponseWriter, r *http.Request) {
	if !h.allow(r, 10, 10*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many verification attempts. Try again later.")
		return
	}
	var input struct {
		Email  string `json:"email"`
		Method string `json:"method"`
		Code   string `json:"code"`
	}
	if err := decodeJSON(w, r, &input); err != nil || !validEmail(input.Email) || !validChallengeCode(input.Code) {
		writeError(w, http.StatusBadRequest, "INVALID_RESET", "The password reset request is invalid or expired.")
		return
	}
	method := normalizePasswordRecoveryMethod(input.Method)
	if !validPasswordRecoveryMethod(method) {
		writeError(w, http.StatusBadRequest, "INVALID_RESET", "The password reset request is invalid or expired.")
		return
	}
	_, deliveryEmail, err := h.passwordRecoveryTarget(r.Context(), input.Email, method)
	if errors.Is(err, postgres.ErrNotFound) || errors.Is(err, postgres.ErrRecoveryUnavailable) {
		writeError(w, http.StatusBadRequest, "INVALID_RESET", "The password reset request is invalid or expired.")
		return
	}
	if err != nil {
		h.writeStoreError(w, "password recovery lookup failed", err)
		return
	}
	userID, err := h.verifyChallenge(r, deliveryEmail, "password_recovery", input.Code)
	if errors.Is(err, postgres.ErrChallengeLocked) {
		writeError(w, http.StatusTooManyRequests, "CHALLENGE_LOCKED", "The recovery request is temporarily locked.")
		return
	}
	if errors.Is(err, postgres.ErrChallengeInvalid) {
		writeError(w, http.StatusBadRequest, "INVALID_RESET", "The password reset request is invalid or expired.")
		return
	}
	if err != nil {
		h.writeStoreError(w, "password recovery code verification failed", err)
		return
	}
	h.recordSecurityEvent(r, userID, "password_recovery_code_verified", map[string]any{"purpose": "password_recovery"})
	writeJSON(w, http.StatusOK, map[string]string{"status": "recovery_code_verified"})
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	if !h.allow(r, 10, 10*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many password reset attempts. Try again later.")
		return
	}
	var input struct {
		Email    string `json:"email"`
		Method   string `json:"method"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil || !validEmail(input.Email) || !validChallengeCode(input.Code) {
		writeError(w, http.StatusBadRequest, "INVALID_RESET", "The password reset request is invalid or expired.")
		return
	}
	if err := ValidatePassword(input.Password); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PASSWORD", "Password does not meet the security requirements.")
		return
	}
	method := normalizePasswordRecoveryMethod(input.Method)
	if !validPasswordRecoveryMethod(method) {
		writeError(w, http.StatusBadRequest, "INVALID_RESET", "The password reset request is invalid or expired.")
		return
	}
	_, deliveryEmail, err := h.passwordRecoveryTarget(r.Context(), input.Email, method)
	if errors.Is(err, postgres.ErrNotFound) || errors.Is(err, postgres.ErrRecoveryUnavailable) {
		writeError(w, http.StatusBadRequest, "INVALID_RESET", "The password reset request is invalid or expired.")
		return
	}
	if err != nil {
		h.writeStoreError(w, "password recovery lookup failed", err)
		return
	}
	userID, err := h.consumeChallenge(r, deliveryEmail, "password_recovery", input.Code)
	if errors.Is(err, postgres.ErrChallengeLocked) {
		writeError(w, http.StatusTooManyRequests, "CHALLENGE_LOCKED", "The recovery request is temporarily locked.")
		return
	}
	if errors.Is(err, postgres.ErrChallengeInvalid) {
		writeError(w, http.StatusBadRequest, "INVALID_RESET", "The password reset request is invalid or expired.")
		return
	}
	if err != nil {
		h.writeStoreError(w, "password recovery challenge failed", err)
		return
	}
	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PASSWORD", "Password does not meet the security requirements.")
		return
	}
	if err := h.store.SetPassword(r.Context(), userID, passwordHash); err != nil {
		h.writeStoreError(w, "password recovery password update failed", err)
		return
	}
	if err := h.store.DeleteUserSessions(r.Context(), userID); err != nil {
		h.logger.Warn("password recovery session revocation failed", "operation", "password_recovery", "request_id", requestID(r), "user_id", userID, "error", err)
	}
	if err := h.store.ActivateProviderUserIfReady(r.Context(), userID); err != nil {
		h.logger.Warn("provider account activation check failed", "operation", "password_recovery", "request_id", requestID(r), "user_id", userID, "error", err)
	}
	h.recordSecurityEvent(r, userID, "password_reset", map[string]any{"purpose": "password_recovery"})
	h.clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "password_reset"})
}

// requestMFARecoverySupport starts a support review without granting access.
// The request is intentionally based only on the account's already-verified
// primary email. Passwords, recovery emails, and unverified addresses are not
// accepted as substitutes for identity verification.
func (h *Handler) requestMFARecoverySupport(w http.ResponseWriter, r *http.Request) {
	if !h.allow(r, 3, 15*time.Minute) {
		h.recordSecurityEvent(r, "", "mfa_recovery_support_rate_limited", map[string]any{
			"stage":  "request",
			"reason": "ip_rate_limit",
		})
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many recovery requests. Try again later.")
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &input); err != nil || !validEmail(input.Email) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A valid account email is required.")
		return
	}

	email := normalizeEmail(input.Email)
	user, err := h.store.FindUserByEmail(r.Context(), email)
	if errors.Is(err, postgres.ErrNotFound) ||
		err == nil && (user.EmailVerifiedAt == nil || user.Status == "suspended" || user.Status == "deleted") {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "mfa_recovery_if_available"})
		return
	}
	if err != nil {
		h.writeStoreError(w, "MFA recovery support lookup failed", err)
		return
	}
	enabled, err := h.store.TOTPEnabled(r.Context(), user.ID)
	if err != nil {
		h.writeStoreError(w, "MFA recovery support MFA lookup failed", err)
		return
	}
	if !enabled {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "mfa_recovery_if_available"})
		return
	}
	if recent, err := h.store.HasRecentEmailChallenge(
		r.Context(), email, mfaRecoverySupportPurpose, time.Now().Add(-emailChallengeResendWindow),
	); err != nil {
		h.writeStoreError(w, "MFA recovery support rate check failed", err)
		return
	} else if recent {
		h.recordSecurityEvent(r, user.ID, "mfa_recovery_support_rate_limited", map[string]any{
			"stage":  "request",
			"reason": "cooldown",
		})
		setEmailChallengeRetryAfter(w)
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "A recovery message was sent recently. Try again later.")
		return
	}
	if err := h.challengeConfiguration(); err != nil {
		h.logChallengeFailure(r, "mfa_recovery_support", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	if err := h.sendChallenge(
		r,
		user.ID,
		email,
		"mfa_recovery_support",
		mfaRecoverySupportPurpose,
		"Verify your Askolo MFA recovery request",
		"This code verifies control of your already-verified primary email. It does not sign you in or disable MFA.",
		true,
	); err != nil {
		if errors.Is(err, postgres.ErrChallengeRecentlySent) {
			h.recordSecurityEvent(r, user.ID, "mfa_recovery_support_rate_limited", map[string]any{
				"stage":  "request",
				"reason": "cooldown",
			})
			setEmailChallengeRetryAfter(w)
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "A recovery message was sent recently. Try again later.")
			return
		}
		h.logChallengeFailure(r, "mfa_recovery_support", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	h.recordSecurityEvent(r, user.ID, "mfa_recovery_support_challenge_sent", map[string]any{
		"purpose": mfaRecoverySupportPurpose,
	})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "mfa_recovery_support_verification_required"})
}

// verifyMFARecoverySupport proves control of the primary email, records the
// request, and revokes sessions. It deliberately stops before any MFA change
// or session creation so support must perform an additional identity review.
func (h *Handler) verifyMFARecoverySupport(w http.ResponseWriter, r *http.Request) {
	if !h.allow(r, 10, 10*time.Minute) {
		h.recordSecurityEvent(r, "", "mfa_recovery_support_rate_limited", map[string]any{
			"stage":  "verification",
			"reason": "ip_rate_limit",
		})
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many verification attempts. Try again later.")
		return
	}
	var input struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := decodeJSON(w, r, &input); err != nil || !validEmail(input.Email) || !validChallengeCode(input.Code) {
		h.recordSecurityEvent(r, "", "mfa_recovery_support_verification_failed", map[string]any{
			"reason": "invalid_request",
		})
		writeError(w, http.StatusBadRequest, "INVALID_RECOVERY", "The recovery request is invalid or expired.")
		return
	}

	email := normalizeEmail(input.Email)
	user, err := h.store.FindUserByEmail(r.Context(), email)
	if errors.Is(err, postgres.ErrNotFound) || err == nil && user.EmailVerifiedAt == nil {
		h.recordSecurityEvent(r, "", "mfa_recovery_support_verification_failed", map[string]any{
			"reason": "unavailable_account",
		})
		writeError(w, http.StatusBadRequest, "INVALID_RECOVERY", "The recovery request is invalid or expired.")
		return
	}
	if err != nil {
		h.writeStoreError(w, "MFA recovery support verification lookup failed", err)
		return
	}
	enabled, err := h.store.TOTPEnabled(r.Context(), user.ID)
	if err != nil {
		h.writeStoreError(w, "MFA recovery support MFA lookup failed", err)
		return
	}
	if !enabled {
		h.recordSecurityEvent(r, user.ID, "mfa_recovery_support_verification_failed", map[string]any{
			"reason": "mfa_not_enabled",
		})
		writeError(w, http.StatusBadRequest, "INVALID_RECOVERY", "The recovery request is invalid or expired.")
		return
	}
	challengeUserID, err := h.consumeChallenge(r, email, mfaRecoverySupportPurpose, input.Code)
	if errors.Is(err, postgres.ErrChallengeLocked) {
		h.recordSecurityEvent(r, user.ID, "mfa_recovery_support_rate_limited", map[string]any{
			"stage":  "verification",
			"reason": "challenge_locked",
		})
		writeError(w, http.StatusTooManyRequests, "CHALLENGE_LOCKED", "The recovery request is temporarily locked.")
		return
	}
	if errors.Is(err, postgres.ErrChallengeInvalid) || (err == nil && challengeUserID != user.ID) {
		h.recordSecurityEvent(r, user.ID, "mfa_recovery_support_verification_failed", nil)
		writeError(w, http.StatusBadRequest, "INVALID_RECOVERY", "The recovery request is invalid or expired.")
		return
	}
	if err != nil {
		h.writeStoreError(w, "MFA recovery support code verification failed", err)
		return
	}
	if err := h.store.DeleteUserSessions(r.Context(), user.ID); err != nil {
		h.logger.Error("MFA recovery support session revocation failed", "operation", "mfa_recovery_support", "request_id", requestID(r), "user_id", user.ID, "error", err)
		h.recordSecurityEvent(r, user.ID, "mfa_recovery_support_session_revocation_failed", nil)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	h.recordSecurityEvent(r, user.ID, "mfa_recovery_support_verified", map[string]any{
		"identity_method":  "verified_primary_email",
		"sessions_revoked": true,
		"mfa_changed":      false,
	})
	h.clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "mfa_recovery_support_review_required"})
}

func normalizePasswordRecoveryMethod(method string) string {
	method = strings.TrimSpace(method)
	if method == "" {
		return passwordRecoveryPrimaryEmail
	}
	return method
}

func validPasswordRecoveryMethod(method string) bool {
	return method == passwordRecoveryPrimaryEmail || method == passwordRecoveryEmail
}

func (h *Handler) passwordRecoveryTarget(ctx context.Context, primaryEmail, method string) (postgres.User, string, error) {
	user, err := h.store.FindUserByEmail(ctx, primaryEmail)
	if err != nil {
		return postgres.User{}, "", err
	}
	deliveryEmail := normalizeEmail(primaryEmail)
	if method == passwordRecoveryEmail {
		deliveryEmail, err = h.store.VerifiedRecoveryEmail(ctx, user.ID)
		if err != nil {
			return postgres.User{}, "", err
		}
	}
	return user, normalizeEmail(deliveryEmail), nil
}

func (h *Handler) mfaStatus(w http.ResponseWriter, r *http.Request) {
	userID, status := h.fullSessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before checking MFA status.")
		return
	}
	enabled, err := h.store.TOTPEnabled(r.Context(), userID)
	if err != nil {
		h.writeStoreError(w, "MFA status lookup failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"enabled": enabled})
}

func (h *Handler) enrollMFA(w http.ResponseWriter, r *http.Request) {
	userID, status := h.fullSessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before enrolling MFA.")
		return
	}
	if !h.allow(r, 5, 15*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many MFA changes. Try again later.")
		return
	}
	var input struct {
		CurrentPassword string `json:"currentPassword"`
	}
	if err := decodeJSON(w, r, &input); err != nil || len(input.CurrentPassword) > 256 {
		writeError(w, http.StatusBadRequest, "REAUTHENTICATION_REQUIRED", "Recent reauthentication is required.")
		return
	}
	if err := h.verifyRecentPassword(r, userID, input.CurrentPassword); err != nil {
		h.writeMFAReauthError(w, r, "MFA enrollment reauthentication failed", err)
		return
	}
	if _, err := h.store.VerifiedRecoveryEmail(r.Context(), userID); err != nil {
		if errors.Is(err, postgres.ErrRecoveryUnavailable) {
			writeError(w, http.StatusForbidden, "RECOVERY_METHOD_REQUIRED", "Set up an independent recovery method before enabling MFA.")
			return
		}
		h.writeStoreError(w, "MFA recovery method lookup failed", err)
		return
	}
	secret, err := newTOTPSecret()
	if err != nil {
		h.writeStoreError(w, "MFA secret generation failed", err)
		return
	}
	if len(h.cfg.TOTPEncryptionKey) != 32 {
		h.logger.Error("MFA encryption is not configured", "operation", "mfa_enrollment", "request_id", requestID(r))
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	encryptedSecret, err := authcrypto.Seal(h.cfg.TOTPEncryptionKey, []byte(secret))
	if err != nil {
		h.writeStoreError(w, "MFA secret encryption failed", err)
		return
	}
	if err := h.store.BeginTOTPEnrollment(r.Context(), userID, encryptedSecret); errors.Is(err, postgres.ErrMFAAlreadyEnabled) {
		writeError(w, http.StatusConflict, "MFA_ALREADY_ENABLED", "MFA is already enabled.")
		return
	} else if err != nil {
		h.writeStoreError(w, "MFA enrollment persistence failed", err)
		return
	}
	user, err := h.store.GetUser(r.Context(), userID)
	if err != nil {
		h.writeStoreError(w, "MFA enrollment user lookup failed", err)
		return
	}
	h.recordSecurityEvent(r, userID, "mfa_enrollment_started", nil)
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "mfa_confirmation_required",
		"secret":  secret,
		"account": user.Email,
	})
}

func (h *Handler) confirmMFA(w http.ResponseWriter, r *http.Request) {
	userID, status := h.fullSessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before confirming MFA.")
		return
	}
	if !h.allow(r, 10, 10*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many MFA attempts. Try again later.")
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &input); err != nil || !validChallengeCode(input.Code) {
		writeError(w, http.StatusBadRequest, "INVALID_MFA", "Enter a valid authenticator code.")
		return
	}
	encryptedSecret, err := h.store.EncryptedTOTPSecret(r.Context(), userID)
	if errors.Is(err, postgres.ErrMFANotEnrolled) {
		writeError(w, http.StatusBadRequest, "MFA_ENROLLMENT_REQUIRED", "Start MFA enrollment before confirming it.")
		return
	}
	if err != nil {
		h.writeStoreError(w, "MFA secret lookup failed", err)
		return
	}
	secretBytes, err := authcrypto.Open(h.cfg.TOTPEncryptionKey, encryptedSecret)
	if err != nil {
		h.logger.Error("MFA secret decryption failed", "operation", "mfa_confirmation", "request_id", requestID(r), "user_id", userID, "error", err)
		h.recordSecurityEvent(r, userID, "mfa_decryption_failed", map[string]any{"operation": "mfa_confirmation"})
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	step, valid := verifyTOTP(string(secretBytes), input.Code, time.Now().UTC())
	if !valid {
		h.recordSecurityEvent(r, userID, "mfa_enrollment_failed", map[string]any{"reason": "invalid_code"})
		writeError(w, http.StatusUnauthorized, "INVALID_MFA", "The authenticator code is incorrect or expired.")
		return
	}
	displayCodes, rawCodes, err := newRecoveryCodes()
	if err != nil {
		h.writeStoreError(w, "MFA recovery code generation failed", err)
		return
	}
	hashes := make([]string, 0, len(rawCodes))
	for _, code := range rawCodes {
		hashes = append(hashes, h.hashRecoveryCode(code))
	}
	if err := h.store.ConfirmTOTPEnrollment(r.Context(), userID, hashes, step); errors.Is(err, postgres.ErrMFAAlreadyEnabled) {
		writeError(w, http.StatusConflict, "MFA_ALREADY_ENABLED", "MFA is already enabled.")
		return
	} else if err != nil {
		h.writeStoreError(w, "MFA enrollment confirmation failed", err)
		return
	}
	h.recordSecurityEvent(r, userID, "mfa_enabled", map[string]any{"recovery_code_count": len(displayCodes)})
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "enabled",
		"recoveryCodes": displayCodes,
	})
}

func (h *Handler) verifyMFA(w http.ResponseWriter, r *http.Request) {
	sessionID := h.sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sign in before completing MFA.")
		return
	}
	state, err := h.store.SessionMFAState(r.Context(), sessionID)
	if errors.Is(err, postgres.ErrMFAChallengeExpired) {
		h.clearSessionCookie(w, r)
		writeError(w, http.StatusUnauthorized, "MFA_CHALLENGE_EXPIRED", "Your MFA challenge expired. Sign in again.")
		return
	}
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Sign in before completing MFA.")
		return
	}
	if err != nil {
		h.writeStoreError(w, "MFA session lookup failed", err)
		return
	}
	if !state.Required || state.Verified {
		writeError(w, http.StatusBadRequest, "MFA_NOT_REQUIRED", "This session does not require MFA.")
		return
	}
	if !h.allow(r, 10, 10*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many MFA attempts. Try again later.")
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_MFA", "Enter your authenticator or recovery code.")
		return
	}
	code := strings.TrimSpace(input.Code)
	if validChallengeCode(code) {
		encryptedSecret, err := h.store.EncryptedTOTPSecret(r.Context(), state.UserID)
		if err != nil {
			h.writeStoreError(w, "MFA challenge secret lookup failed", err)
			return
		}
		secretBytes, err := authcrypto.Open(h.cfg.TOTPEncryptionKey, encryptedSecret)
		if err != nil {
			h.logger.Error("MFA challenge secret decryption failed", "operation", "mfa_challenge", "request_id", requestID(r), "user_id", state.UserID, "error", err)
			h.recordSecurityEvent(r, state.UserID, "mfa_decryption_failed", map[string]any{"operation": "mfa_challenge"})
			writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
			return
		}
		step, valid := verifyTOTP(string(secretBytes), code, time.Now().UTC())
		if !valid {
			if h.recordMFAFailure(r, state.UserID, "invalid_totp") {
				h.writeMFAChallengeLocked(w, r, state.UserID)
				return
			}
			writeError(w, http.StatusUnauthorized, "INVALID_MFA", "The authenticator or recovery code is incorrect.")
			return
		}
		err = h.store.CompleteTOTPChallenge(r.Context(), sessionID, state.UserID, step, mfaChallengeAttempts)
		if errors.Is(err, postgres.ErrMFAReplay) {
			h.recordSecurityEvent(r, state.UserID, "mfa_replay_rejected", map[string]any{"method": "totp"})
			writeError(w, http.StatusUnauthorized, "INVALID_MFA", "That authenticator code has already been used.")
			return
		}
		if errors.Is(err, postgres.ErrMFAChallengeLocked) {
			h.writeMFAChallengeLocked(w, r, state.UserID)
			return
		}
		if err != nil {
			h.writeStoreError(w, "MFA challenge completion failed", err)
			return
		}
		h.recordSecurityEvent(r, state.UserID, "mfa_challenge_succeeded", map[string]any{"method": "totp"})
		writeJSON(w, http.StatusOK, map[string]string{"status": "authenticated"})
		return
	}
	if !validRecoveryCode(code) {
		if h.recordMFAFailure(r, state.UserID, "invalid_code") {
			h.writeMFAChallengeLocked(w, r, state.UserID)
			return
		}
		writeError(w, http.StatusUnauthorized, "INVALID_MFA", "The authenticator or recovery code is incorrect.")
		return
	}
	err = h.store.ConsumeRecoveryCode(r.Context(), sessionID, state.UserID, h.hashRecoveryCode(code), mfaChallengeAttempts)
	if errors.Is(err, postgres.ErrMFAChallengeLocked) {
		h.writeMFAChallengeLocked(w, r, state.UserID)
		return
	}
	if errors.Is(err, postgres.ErrRecoveryCodeInvalid) {
		h.recordSecurityEvent(r, state.UserID, "mfa_recovery_code_failed", nil)
		writeError(w, http.StatusUnauthorized, "INVALID_MFA", "The authenticator or recovery code is incorrect.")
		return
	}
	if err != nil {
		h.writeStoreError(w, "MFA recovery code completion failed", err)
		return
	}
	h.recordSecurityEvent(r, state.UserID, "mfa_recovery_code_used", nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "authenticated"})
}

func (h *Handler) disableMFA(w http.ResponseWriter, r *http.Request) {
	userID, status := h.fullSessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before disabling MFA.")
		return
	}
	if !h.allow(r, 5, 15*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many MFA changes. Try again later.")
		return
	}
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		RecoveryCode    string `json:"recoveryCode"`
	}
	if err := decodeJSON(w, r, &input); err != nil || len(input.CurrentPassword) > 256 || !validRecoveryCode(input.RecoveryCode) {
		writeError(w, http.StatusBadRequest, "REAUTHENTICATION_REQUIRED", "Recent reauthentication and a recovery code are required.")
		return
	}
	if err := h.verifyRecentPassword(r, userID, input.CurrentPassword); err != nil {
		h.writeMFAReauthError(w, r, "MFA disable reauthentication failed", err)
		return
	}
	if !h.hasIndependentRecoveryMethod(w, r, userID) {
		return
	}
	if err := h.store.DisableTOTPWithRecoveryCode(r.Context(), userID, h.hashRecoveryCode(input.RecoveryCode)); errors.Is(err, postgres.ErrRecoveryCodeInvalid) {
		h.recordSecurityEvent(r, userID, "mfa_disable_failed", map[string]any{"reason": "invalid_recovery_code"})
		writeError(w, http.StatusUnauthorized, "INVALID_MFA", "The recovery code is incorrect or already used.")
		return
	} else if errors.Is(err, postgres.ErrMFANotEnrolled) {
		writeError(w, http.StatusBadRequest, "MFA_NOT_ENABLED", "MFA is not enabled.")
		return
	} else if err != nil {
		h.writeStoreError(w, "MFA disable failed", err)
		return
	}
	h.recordSecurityEvent(r, userID, "mfa_disabled", nil)
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

func (h *Handler) regenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	userID, status := h.fullSessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before regenerating recovery codes.")
		return
	}
	if !h.allow(r, 3, 15*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many recovery-code changes. Try again later.")
		return
	}
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		RecoveryCode    string `json:"recoveryCode"`
	}
	if err := decodeJSON(w, r, &input); err != nil || len(input.CurrentPassword) > 256 || !validRecoveryCode(input.RecoveryCode) {
		writeError(w, http.StatusBadRequest, "REAUTHENTICATION_REQUIRED", "Recent reauthentication and a recovery code are required.")
		return
	}
	if err := h.verifyRecentPassword(r, userID, input.CurrentPassword); err != nil {
		h.writeMFAReauthError(w, r, "recovery code regeneration reauthentication failed", err)
		return
	}
	if !h.hasIndependentRecoveryMethod(w, r, userID) {
		return
	}
	displayCodes, rawCodes, err := newRecoveryCodes()
	if err != nil {
		h.writeStoreError(w, "recovery code generation failed", err)
		return
	}
	hashes := make([]string, 0, len(rawCodes))
	for _, code := range rawCodes {
		hashes = append(hashes, h.hashRecoveryCode(code))
	}
	err = h.store.RegenerateRecoveryCodes(r.Context(), userID, h.hashRecoveryCode(input.RecoveryCode), hashes)
	if errors.Is(err, postgres.ErrRecoveryCodeInvalid) {
		h.recordSecurityEvent(r, userID, "recovery_code_regeneration_failed", map[string]any{"reason": "invalid_recovery_code"})
		writeError(w, http.StatusUnauthorized, "INVALID_MFA", "The recovery code is incorrect or already used.")
		return
	}
	if err != nil {
		h.writeStoreError(w, "recovery code regeneration failed", err)
		return
	}
	h.recordSecurityEvent(r, userID, "recovery_codes_regenerated", map[string]any{"recovery_code_count": len(displayCodes)})
	writeJSON(w, http.StatusOK, map[string]any{"status": "regenerated", "recoveryCodes": displayCodes})
}

func (h *Handler) fullSessionUserID(r *http.Request) (string, int) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		return "", status
	}
	state, err := h.store.SessionMFAState(r.Context(), h.sessionIDFromRequest(r))
	if errors.Is(err, postgres.ErrMFAChallengeExpired) {
		return "", http.StatusUnauthorized
	}
	if err != nil {
		h.logger.Error("full session MFA lookup failed", "error", err)
		return "", http.StatusServiceUnavailable
	}
	if state.Required && !state.Verified {
		return "", http.StatusForbidden
	}
	return userID, http.StatusOK
}

func (h *Handler) verifyRecentPassword(r *http.Request, userID, password string) error {
	passwordHash, err := h.store.PasswordHash(r.Context(), userID)
	if err != nil {
		return err
	}
	if !VerifyPassword(passwordHash, password) {
		return errors.New("password reauthentication failed")
	}
	return nil
}

func (h *Handler) hasIndependentRecoveryMethod(w http.ResponseWriter, r *http.Request, userID string) bool {
	if _, err := h.store.VerifiedRecoveryEmail(r.Context(), userID); err != nil {
		if errors.Is(err, postgres.ErrRecoveryUnavailable) {
			writeError(w, http.StatusForbidden, "RECOVERY_METHOD_REQUIRED", "Set up an independent recovery method before changing MFA.")
			return false
		}
		h.writeStoreError(w, "MFA recovery method lookup failed", err)
		return false
	}
	return true
}

func (h *Handler) writeMFAReauthError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	h.logger.Warn(operation, "request_id", requestID(r), "error", err)
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusUnauthorized, "REAUTHENTICATION_REQUIRED", "Recent reauthentication is required.")
		return
	}
	if strings.Contains(err.Error(), "reauthentication failed") {
		writeError(w, http.StatusUnauthorized, "REAUTHENTICATION_REQUIRED", "Recent reauthentication is required.")
		return
	}
	h.writeStoreError(w, operation, err)
}

func (h *Handler) recordMFAFailure(r *http.Request, userID, reason string) bool {
	err := h.store.RecordMFAFailure(r.Context(), h.sessionIDFromRequest(r), userID, mfaChallengeAttempts)
	if errors.Is(err, postgres.ErrMFAChallengeLocked) {
		return true
	}
	h.recordSecurityEvent(r, userID, "mfa_challenge_failed", map[string]any{"reason": reason})
	return false
}

func (h *Handler) writeMFAChallengeLocked(w http.ResponseWriter, r *http.Request, userID string) {
	h.recordSecurityEvent(r, userID, "mfa_challenge_locked", nil)
	if w != nil {
		writeError(w, http.StatusTooManyRequests, "MFA_CHALLENGE_LOCKED", "The MFA challenge is temporarily locked. Sign in again.")
	}
}

func (h *Handler) challengeConfiguration() error {
	if h.store == nil || h.emailSender == nil || strings.TrimSpace(h.cfg.Email.ChallengeSecret) == "" {
		return ErrEmailDeliveryNotConfigured
	}
	return nil
}

func (h *Handler) sendChallenge(
	r *http.Request,
	userID, email, operation, purpose, subject, instruction string,
	preventRecent bool,
) error {
	if err := h.challengeConfiguration(); err != nil {
		return err
	}
	code, err := newChallengeCode()
	if err != nil {
		return err
	}
	challengeID, err := id.New()
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(emailChallengeTTL)
	if preventRecent {
		if err := h.store.CreateEmailChallengeIfAllowed(
			r.Context(), challengeID, userID, email, purpose, h.hashChallenge(code),
			expiresAt, time.Now().Add(-emailChallengeResendWindow),
		); err != nil {
			return err
		}
	} else if err := h.store.CreateEmailChallenge(
		r.Context(), challengeID, userID, email, purpose, h.hashChallenge(code), expiresAt,
	); err != nil {
		return err
	}
	startedAt := time.Now()
	expiryMinutes := int(emailChallengeTTL / time.Minute)
	err = h.emailSender.Send(r.Context(), EmailMessage{
		To:      email,
		Subject: subject,
		Body:    fmt.Sprintf("%s\n\nYour one-time code is: %s\n\nThis code expires in %d minutes. If you did not request this, you can ignore this message.", instruction, code, expiryMinutes),
		Code:    code,
	})
	err = normalizeEmailDeliveryError(err)
	outcome := emailDeliveryState(err)
	latency := time.Since(startedAt)
	h.emailMonitor.Observe(outcome, latency)
	h.logger.Info("email challenge delivery attempt",
		"operation", operation,
		"purpose", purpose,
		"outcome", outcome,
		"duration_ms", boundedEmailDeliveryLatency(latency),
		"count", 1,
	)
	if err == nil {
		return nil
	}
	if emailDeliveryRetrySafe(err) {
		cleanupContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		deleteErr := h.store.DeleteEmailChallenge(cleanupContext, challengeID)
		cancel()
		if deleteErr != nil {
			h.logger.Warn("failed to release email challenge after delivery failure",
				"operation", operation,
				"purpose", purpose,
				"outcome", outcome,
				"request_id", requestID(r),
			)
		}
	}
	return err
}

func (h *Handler) consumeChallenge(r *http.Request, email, purpose, code string) (string, error) {
	if err := h.challengeConfiguration(); err != nil {
		return "", err
	}
	return h.store.ConsumeEmailChallenge(
		r.Context(),
		normalizeEmail(email),
		purpose,
		h.hashChallenge(code),
		emailChallengeMaxAttempts,
	)
}

func (h *Handler) verifyChallenge(r *http.Request, email, purpose, code string) (string, error) {
	if err := h.challengeConfiguration(); err != nil {
		return "", err
	}
	return h.store.VerifyEmailChallenge(
		r.Context(),
		normalizeEmail(email),
		purpose,
		h.hashChallenge(code),
		emailChallengeMaxAttempts,
	)
}

func setEmailChallengeRetryAfter(w http.ResponseWriter) {
	w.Header().Set("Retry-After", fmt.Sprintf("%d", int(emailChallengeResendWindow/time.Second)))
}

func (h *Handler) hashChallenge(code string) string {
	mac := hmac.New(sha256.New, []byte(h.cfg.Email.ChallengeSecret))
	_, _ = mac.Write([]byte(code))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func (h *Handler) recordSecurityEvent(r *http.Request, userID, eventType string, metadata map[string]any) {
	if h.store == nil {
		return
	}
	if err := h.store.CreateSecurityEvent(r.Context(), userID, eventType, requestID(r), metadata); err != nil {
		h.logger.Warn("auth security event persistence failed", "operation", eventType, "request_id", requestID(r), "user_id", userID, "error", err)
	}
}

func (h *Handler) logChallengeFailure(r *http.Request, operation string, err error) {
	// Sender errors may contain recipient addresses or provider-reflected
	// message content. Keep only the bounded category and retry signal.
	h.logger.Error("email challenge delivery failed",
		"operation", operation,
		"request_id", requestID(r),
		"failure_category", emailDeliveryState(err),
		"retry_safe", emailDeliveryRetrySafe(err),
		"count", 1,
	)
}

func boundedEmailDeliveryLatency(latency time.Duration) int64 {
	if latency < 0 {
		return 0
	}
	const maxObservedLatency = 30 * time.Second
	if latency > maxObservedLatency {
		latency = maxObservedLatency
	}
	return latency.Milliseconds()
}

func newChallengeCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}

func validChallengeCode(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func requestID(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("X-Request-ID"))
}

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request, userID string) {
	sessionID, err := h.store.CreateSession(r.Context(), userID, "password", sessionTTL)
	if err != nil {
		h.writeStoreError(w, "session creation failed", err)
		return
	}
	h.setSessionCookie(w, r, sessionID, sessionTTL)
	writeJSON(w, http.StatusOK, map[string]string{"status": "authenticated"})
}

func (h *Handler) createMFAPendingSession(w http.ResponseWriter, r *http.Request, userID string) {
	sessionID, err := h.store.CreateMFAPendingSession(r.Context(), userID, "password", sessionTTL, mfaChallengeTTL)
	if err != nil {
		h.writeStoreError(w, "MFA session creation failed", err)
		return
	}
	h.setSessionCookie(w, r, sessionID, sessionTTL)
	h.recordSecurityEvent(r, userID, "mfa_challenge_started", nil)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "mfa_required",
		"methods": []string{"totp", "recovery_code"},
	})
}

func (h *Handler) sessionUserID(r *http.Request) (string, int) {
	if h.store == nil {
		return "", http.StatusServiceUnavailable
	}
	sessionID := h.sessionIDFromRequest(r)
	if sessionID == "" {
		return "", http.StatusUnauthorized
	}
	userID, err := h.store.SessionUserID(r.Context(), sessionID)
	if errors.Is(err, postgres.ErrNotFound) {
		return "", http.StatusUnauthorized
	}
	if err != nil {
		h.logger.Error("session lookup failed", "error", err)
		return "", http.StatusServiceUnavailable
	}
	return userID, http.StatusOK
}

func (h *Handler) allow(r *http.Request, max int, window time.Duration) bool {
	key := firstHeader(r.Header.Get("X-Forwarded-For"))
	if key == "" {
		key = r.RemoteAddr
	}
	h.limiter.mu.Lock()
	defer h.limiter.mu.Unlock()
	now := time.Now()
	entry, ok := h.limiter.entries[key]
	if !ok || now.After(entry.resetAt) {
		h.limiter.entries[key] = rateEntry{count: 1, resetAt: now.Add(window)}
		return true
	}
	if entry.count >= max {
		return false
	}
	entry.count++
	h.limiter.entries[key] = entry
	return true
}

func (h *Handler) writeStoreError(w http.ResponseWriter, operation string, err error) {
	h.logger.Error(operation, "error", err)
	writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func (h *Handler) sessionIDFromRequest(r *http.Request) string {
	if cookie, err := r.Cookie(config.CookieName(h.cfg.SessionCookieName)); err == nil {
		return strings.TrimSpace(cookie.Value)
	}
	authorization := r.Header.Get("Authorization")
	if strings.HasPrefix(authorization, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	}
	return ""
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, r *http.Request, sessionID string, ttl time.Duration) {
	secure := true
	host := firstHeader(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
		secure = false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     config.CookieName(h.cfg.SessionCookieName),
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	secure := true
	host := firstHeader(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
		secure = false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     config.CookieName(h.cfg.SessionCookieName),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func validEmail(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 320 || strings.ContainsAny(value, " \r\n") {
		return false
	}
	at := strings.LastIndexByte(value, '@')
	return at > 0 && at < len(value)-1 && strings.Contains(value[at+1:], ".")
}

func firstHeader(value string) string {
	if index := strings.IndexByte(value, ','); index >= 0 {
		return strings.TrimSpace(value[:index])
	}
	return strings.TrimSpace(value)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload != nil {
		_ = json.NewEncoder(w).Encode(payload)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
