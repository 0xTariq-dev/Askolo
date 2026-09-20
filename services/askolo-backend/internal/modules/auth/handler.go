package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
)

const sessionTTL = 7 * 24 * time.Hour

type Handler struct {
	cfg     config.Config
	store   *postgres.Store
	logger  *slog.Logger
	limiter *rateLimiter
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
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		cfg:     cfg,
		store:   store,
		logger:  logger,
		limiter: &rateLimiter{entries: make(map[string]rateEntry)},
	}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/user", h.user)
	mux.HandleFunc("GET /api/auth/session", h.user)
	mux.HandleFunc("POST /api/auth/logout", h.logout)
	mux.HandleFunc("POST /api/auth/password/login", h.passwordLogin)
	mux.HandleFunc("POST /api/auth/password/set", h.passwordSet)
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
	h.createSession(w, r, user.ID)
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
	writeJSON(w, http.StatusNoContent, nil)
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
