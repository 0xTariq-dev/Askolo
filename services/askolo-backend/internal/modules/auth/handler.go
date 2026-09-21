package auth

import (
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
	"askolo/backend/internal/platform/id"
)

const (
	sessionTTL                 = 7 * 24 * time.Hour
	emailChallengeTTL          = 15 * time.Minute
	emailChallengeResendWindow = 60 * time.Second
	emailChallengeMaxAttempts  = 5
)

type Handler struct {
	cfg         config.Config
	store       *postgres.Store
	logger      *slog.Logger
	limiter     *rateLimiter
	emailSender EmailSender
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
	return NewHandlerWithEmailSender(cfg, store, logger, NewSMTPEmailSender(cfg))
}

func NewHandlerWithEmailSender(
	cfg config.Config,
	store *postgres.Store,
	logger *slog.Logger,
	emailSender EmailSender,
) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if emailSender == nil {
		emailSender = NewSMTPEmailSender(cfg)
	}
	return &Handler{
		cfg:         cfg,
		store:       store,
		logger:      logger,
		limiter:     &rateLimiter{entries: make(map[string]rateEntry)},
		emailSender: emailSender,
	}
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
	mux.HandleFunc("POST /api/auth/password/recovery/reset", h.resetPassword)
	return mux
}

func (h *Handler) user(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeJSON(w, http.StatusOK, map[string]any{"user": nil})
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
	sessionID := sessionIDFromRequest(r)
	if sessionID != "" && h.store != nil {
		if err := h.store.DeleteSession(r.Context(), sessionID); err != nil {
			h.logger.Warn("session deletion failed", "error", err)
		}
	}
	clearSessionCookie(w, r)
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
	if err := h.sendChallenge(r, userID, email, "email_verification", "Verify your Askolo email", "Use this code to verify your Askolo email."); err != nil {
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
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "A verification message was sent recently. Try again later.")
		return
	}
	if err := h.challengeConfiguration(); err != nil {
		h.logChallengeFailure(r, "verification_resend", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	if err := h.sendChallenge(r, user.ID, email, "email_verification", "Verify your Askolo email", "Use this code to verify your Askolo email."); err != nil {
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
	if err := h.sendChallenge(r, userID, email, "recovery_email_enrollment", "Confirm your Askolo recovery email", "Use this code to confirm your recovery email."); err != nil {
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
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &input); err != nil || !validEmail(input.Email) {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "A valid email is required.")
		return
	}
	email := normalizeEmail(input.Email)
	user, err := h.store.FindUserByRecoveryEmail(r.Context(), email)
	if errors.Is(err, postgres.ErrNotFound) {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "recovery_if_available"})
		return
	}
	if err != nil {
		h.writeStoreError(w, "password recovery lookup failed", err)
		return
	}
	if user.Status == "suspended" || user.Status == "deleted" {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "recovery_if_available"})
		return
	}
	if recent, err := h.store.HasRecentEmailChallenge(r.Context(), email, "password_recovery", time.Now().Add(-emailChallengeResendWindow)); err != nil {
		h.writeStoreError(w, "password recovery rate check failed", err)
		return
	} else if recent {
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "A recovery message was sent recently. Try again later.")
		return
	}
	if err := h.challengeConfiguration(); err != nil {
		h.logChallengeFailure(r, "password_recovery", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	if err := h.sendChallenge(r, user.ID, email, "password_recovery", "Reset your Askolo password", "Use this code to reset your Askolo password."); err != nil {
		h.logChallengeFailure(r, "password_recovery", err)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", "Authentication is temporarily unavailable.")
		return
	}
	h.recordSecurityEvent(r, user.ID, "password_recovery_challenge_sent", map[string]any{"purpose": "password_recovery"})
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "recovery_if_available"})
}

func (h *Handler) resetPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
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
	userID, err := h.consumeChallenge(r, input.Email, "password_recovery", input.Code)
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
	clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "password_reset"})
}

func (h *Handler) challengeConfiguration() error {
	if h.store == nil || h.emailSender == nil || strings.TrimSpace(h.cfg.Email.ChallengeSecret) == "" {
		return errors.New("email challenge dependencies are not configured")
	}
	return nil
}

func (h *Handler) sendChallenge(
	r *http.Request,
	userID, email, purpose, subject, instruction string,
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
	if err := h.store.CreateEmailChallenge(
		r.Context(),
		challengeID,
		userID,
		email,
		purpose,
		h.hashChallenge(code),
		time.Now().Add(emailChallengeTTL),
	); err != nil {
		return err
	}
	return h.emailSender.Send(r.Context(), EmailMessage{
		To:      email,
		Subject: subject,
		Body:    fmt.Sprintf("%s\n\nYour one-time code is: %s\n\nThis code expires in 15 minutes. If you did not request this, you can ignore this message.", instruction, code),
	})
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

func (h *Handler) hashChallenge(code string) string {
	mac := hmac.New(sha256.New, []byte(h.cfg.Email.ChallengeSecret))
	_, _ = mac.Write([]byte(code))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

func (h *Handler) recordSecurityEvent(r *http.Request, userID, eventType string, metadata map[string]any) {
	if err := h.store.CreateSecurityEvent(r.Context(), userID, eventType, requestID(r), metadata); err != nil {
		h.logger.Warn("auth security event persistence failed", "operation", eventType, "request_id", requestID(r), "user_id", userID, "error", err)
	}
}

func (h *Handler) logChallengeFailure(r *http.Request, operation string, err error) {
	h.logger.Error("email challenge delivery failed", "operation", operation, "request_id", requestID(r), "error", err)
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
	setSessionCookie(w, r, sessionID, sessionTTL)
	writeJSON(w, http.StatusOK, map[string]string{"status": "authenticated"})
}

func (h *Handler) sessionUserID(r *http.Request) (string, int) {
	if h.store == nil {
		return "", http.StatusServiceUnavailable
	}
	sessionID := sessionIDFromRequest(r)
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

func sessionIDFromRequest(r *http.Request) string {
	if cookie, err := r.Cookie("sid"); err == nil {
		return strings.TrimSpace(cookie.Value)
	}
	authorization := r.Header.Get("Authorization")
	if strings.HasPrefix(authorization, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	}
	return ""
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, sessionID string, ttl time.Duration) {
	secure := true
	host := firstHeader(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
		secure = false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "sid",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	secure := true
	host := firstHeader(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.0.0.1") {
		secure = false
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "sid",
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
