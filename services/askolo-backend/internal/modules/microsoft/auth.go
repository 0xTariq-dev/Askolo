package microsoft

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	authmodule "askolo/backend/internal/modules/auth"
	"askolo/backend/internal/platform/authorization"
)

const (
	microsoftAuthCookieName  = "askolo_microsoft_auth"
	microsoftLoginFlow       = "microsoft_auth_login"
	microsoftLinkFlow        = "microsoft_auth_link"
	microsoftLoginCallback   = "/api/auth/microsoft/callback"
	microsoftLinkCallback    = "/api/auth/microsoft/link/callback"
	microsoftSessionTTL      = 7 * 24 * time.Hour
	microsoftMFAChallengeTTL = 5 * time.Minute
	microsoftAuthRateLimit   = 20
	microsoftAuthRateWindow  = 10 * time.Minute
	microsoftAuthMaxBuckets  = 4096
)

var microsoftLoginScopes = []string{"openid", "profile", "email", "User.Read"}

type microsoftAuthState struct {
	Nonce        string `json:"nonce"`
	CodeVerifier string `json:"codeVerifier"`
	ReturnTo     string `json:"returnTo"`
	Flow         string `json:"flow"`
	Intent       string `json:"intent,omitempty"`
	UserID       string `json:"userId,omitempty"`
	IssuedAt     int64  `json:"issuedAt"`
}

type authRateEntry struct {
	count   int
	resetAt time.Time
}

type authRateLimiter struct {
	mu      sync.Mutex
	entries map[string]authRateEntry
}

func newAuthRateLimiter() *authRateLimiter {
	return &authRateLimiter{entries: make(map[string]authRateEntry)}
}

func (l *authRateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	if len(l.entries) >= microsoftAuthMaxBuckets {
		for candidate, entry := range l.entries {
			if !now.Before(entry.resetAt) {
				delete(l.entries, candidate)
			}
		}
	}
	entry, exists := l.entries[key]
	if !exists {
		if len(l.entries) >= microsoftAuthMaxBuckets {
			return false
		}
		l.entries[key] = authRateEntry{count: 1, resetAt: now.Add(microsoftAuthRateWindow)}
		return true
	}
	if !now.Before(entry.resetAt) {
		l.entries[key] = authRateEntry{count: 1, resetAt: now.Add(microsoftAuthRateWindow)}
		return true
	}
	if entry.count >= microsoftAuthRateLimit {
		return false
	}
	entry.count++
	l.entries[key] = entry
	return true
}

func (h *Handler) startAuthLogin(w http.ResponseWriter, r *http.Request) {
	h.startAuth(w, r, microsoftLoginFlow, "", "signin")
}

func (h *Handler) startAuthLink(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r, authorization.ActionProviderWrite)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before linking a Microsoft account.")
		return
	}
	h.startAuth(w, r, microsoftLinkFlow, userID, "link")
}

func (h *Handler) startAuth(w http.ResponseWriter, r *http.Request, flow, userID, intent string) {
	if h.store == nil || strings.TrimSpace(h.cfg.SessionSecret) == "" ||
		strings.TrimSpace(h.cfg.Microsoft.ClientID) == "" ||
		strings.TrimSpace(h.cfg.Microsoft.ClientSecret) == "" {
		writeError(w, http.StatusServiceUnavailable, "MICROSOFT_AUTH_NOT_CONFIGURED", "Microsoft sign-in is not configured.")
		return
	}
	if h.authLimiter == nil || !h.authLimiter.allow(h.authRateKey(r)) {
		w.Header().Set("Retry-After", "600")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many Microsoft sign-in attempts.")
		return
	}

	baseURL, err := h.callbackBaseURL(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_HOST", "This host is not configured for Microsoft sign-in.")
		return
	}
	returnTo := safeMicrosoftAuthReturnTo(r.URL.Query().Get("returnTo"), "/sign-in")
	if flow == microsoftLinkFlow {
		returnTo = safeMicrosoftAuthReturnTo(r.URL.Query().Get("returnTo"), "/settings/account/security")
	}
	nonce, err := randomValue(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STATE_ERROR", "Unable to start Microsoft sign-in.")
		return
	}
	verifier, err := randomValue(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STATE_ERROR", "Unable to start Microsoft sign-in.")
		return
	}
	payload := microsoftAuthState{
		Nonce: nonce, CodeVerifier: verifier, ReturnTo: returnTo,
		Flow: flow, Intent: intent, UserID: userID, IssuedAt: time.Now().Unix(),
	}
	cookieValue, err := h.encodeMicrosoftAuthState(payload)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "MICROSOFT_AUTH_NOT_CONFIGURED", "Microsoft sign-in is not configured.")
		return
	}
	nonceHash := sha256.Sum256([]byte(nonce))
	if err := h.store.InsertOAuthState(
		r.Context(), hex.EncodeToString(nonceHash[:]), flow, userID, returnTo, time.Now().Add(stateTTL),
	); err != nil {
		h.logger.Error("Microsoft auth state persistence failed", "flow", flow, "request_id", r.Header.Get("X-Request-ID"), "error", err)
		writeError(w, http.StatusServiceUnavailable, "MICROSOFT_AUTH_UNAVAILABLE", "Microsoft sign-in is temporarily unavailable.")
		return
	}

	callbackPath := microsoftLoginCallback
	if flow == microsoftLinkFlow {
		callbackPath = microsoftLinkCallback
	}
	challenge := base64.RawURLEncoding.EncodeToString(sha256Bytes(verifier))
	params := url.Values{
		"client_id":             {h.cfg.Microsoft.ClientID},
		"redirect_uri":          {baseURL + callbackPath},
		"response_type":         {"code"},
		"response_mode":         {"query"},
		"scope":                 {strings.Join(microsoftLoginScopes, " ")},
		"state":                 {nonce},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"prompt":                {"select_account"},
	}
	http.SetCookie(w, &http.Cookie{
		Name: microsoftAuthCookieName, Value: cookieValue, Path: "/", HttpOnly: true,
		Secure: strings.HasPrefix(baseURL, "https://"), SameSite: http.SameSiteLaxMode,
		MaxAge: int(stateTTL.Seconds()),
	})
	http.Redirect(w, r, authURL+"?"+params.Encode(), http.StatusFound)
}

func (h *Handler) finishAuthLogin(w http.ResponseWriter, r *http.Request) {
	h.finishMicrosoftAuth(w, r, microsoftLoginFlow)
}

func (h *Handler) finishAuthLink(w http.ResponseWriter, r *http.Request) {
	h.finishMicrosoftAuth(w, r, microsoftLinkFlow)
}

func (h *Handler) finishMicrosoftAuth(w http.ResponseWriter, r *http.Request, flow string) {
	h.clearMicrosoftAuthCookie(w, r)
	returnTo := "/sign-in"
	if flow == microsoftLinkFlow {
		returnTo = "/settings/account/security"
	}
	cookie, err := r.Cookie(microsoftAuthCookieName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_STATE", "Microsoft authorization could not be verified.")
		return
	}
	payload, err := h.decodeMicrosoftAuthState(cookie.Value)
	if err != nil || payload.Flow != flow ||
		payload.IssuedAt < time.Now().Add(-stateTTL).Unix() ||
		payload.IssuedAt > time.Now().Add(time.Minute).Unix() ||
		payload.Nonce != r.URL.Query().Get("state") ||
		(flow == microsoftLoginFlow && payload.UserID != "") ||
		(flow == microsoftLinkFlow && payload.UserID == "") {
		writeError(w, http.StatusBadRequest, "INVALID_STATE", "Microsoft authorization could not be verified.")
		return
	}
	returnTo = payload.ReturnTo
	nonceHash := sha256.Sum256([]byte(payload.Nonce))
	userID, persistedReturnTo, consumed, err := h.store.ConsumeOAuthState(
		r.Context(), hex.EncodeToString(nonceHash[:]), flow,
	)
	if err != nil || !consumed || userID != payload.UserID {
		writeError(w, http.StatusBadRequest, "INVALID_STATE", "Microsoft authorization could not be verified.")
		return
	}
	if persistedReturnTo != "" {
		returnTo = persistedReturnTo
	}
	if r.URL.Query().Get("error") != "" {
		redirectStatus(w, r, returnTo, "error")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		redirectStatus(w, r, returnTo, "error")
		return
	}
	baseURL, err := h.callbackBaseURL(r)
	if err != nil {
		redirectStatus(w, r, returnTo, "error")
		return
	}
	callbackPath := microsoftLoginCallback
	if flow == microsoftLinkFlow {
		callbackPath = microsoftLinkCallback
	}
	tokens, err := h.exchangeCode(r.Context(), baseURL+callbackPath, code, payload.CodeVerifier)
	if err != nil {
		h.logger.Warn("Microsoft auth token exchange failed", "flow", flow, "request_id", r.Header.Get("X-Request-ID"), "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	profile, err := h.fetchProfile(r.Context(), tokens.AccessToken)
	if err != nil || strings.TrimSpace(profile.ID) == "" {
		h.logger.Warn("Microsoft auth profile lookup failed", "flow", flow, "request_id", r.Header.Get("X-Request-ID"))
		redirectStatus(w, r, returnTo, "error")
		return
	}
	email := strings.ToLower(strings.TrimSpace(profile.Mail))
	if !validEmail(email) {
		email = ""
	}
	displayName := strings.TrimSpace(profile.DisplayName)

	if flow == microsoftLinkFlow {
		if err := h.store.LinkProviderIdentity(
			r.Context(), userID, "microsoft", profile.ID, email, displayName, "",
		); err != nil {
			if errors.Is(err, postgres.ErrOwnership) {
				redirectStatus(w, r, returnTo, "provider_not_linked")
				return
			}
			h.logger.Error("Microsoft identity linking failed", "user_id", userID, "request_id", r.Header.Get("X-Request-ID"), "error", err)
			redirectStatus(w, r, returnTo, "error")
			return
		}
		redirectStatus(w, r, returnTo, "linked")
		return
	}

	identity, err := h.store.FindProviderIdentity(r.Context(), "microsoft", profile.ID)
	if errors.Is(err, postgres.ErrNotFound) {
		// Graph's mail and userPrincipalName fields are not proof of a verified
		// mailbox. New accounts must be created through a verified signup flow,
		// then linked from account security settings.
		redirectStatus(w, r, returnTo, "provider_not_linked")
		return
	}
	if err != nil {
		h.logger.Error("Microsoft identity lookup failed", "request_id", r.Header.Get("X-Request-ID"), "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	if !identity.LoginEnabled {
		redirectStatus(w, r, returnTo, "provider_not_linked")
		return
	}
	h.createMicrosoftLoginSession(w, r, returnTo, identity.UserID)
}

func (h *Handler) createMicrosoftLoginSession(w http.ResponseWriter, r *http.Request, returnTo, userID string) {
	mfaEnabled, err := h.store.TOTPEnabled(r.Context(), userID)
	if err != nil {
		h.logger.Error("Microsoft MFA status lookup failed", "user_id", userID, "request_id", r.Header.Get("X-Request-ID"), "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	var sessionID string
	trustedDeviceLogin := false
	if mfaEnabled {
		device, deviceErr := authmodule.TrustedDeviceForRequest(r.Context(), h.store, h.cfg, r, userID)
		if deviceErr == nil {
			sessionID, err = h.store.CreateTrustedDeviceSession(
				r.Context(), userID, "microsoft", microsoftSessionTTL, device.ID,
			)
			if err == nil {
				trustedDeviceLogin = true
				_ = h.store.CreateSecurityEvent(
					r.Context(), userID, "trusted_device_login", r.Header.Get("X-Request-ID"),
					map[string]any{"provider": "microsoft"},
				)
			}
		} else if errors.Is(deviceErr, postgres.ErrTrustedDeviceNotFound) {
			sessionID, err = h.store.CreateMFAPendingSession(
				r.Context(), userID, "microsoft", microsoftSessionTTL, microsoftMFAChallengeTTL,
			)
		} else {
			err = deviceErr
		}
	} else {
		sessionID, err = h.store.CreateSession(r.Context(), userID, "microsoft", microsoftSessionTTL)
	}
	if err != nil {
		h.logger.Error("Microsoft session persistence failed", "user_id", userID, "request_id", r.Header.Get("X-Request-ID"), "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: config.CookieName(h.cfg.SessionCookieName), Value: sessionID, Path: "/",
		HttpOnly: true, Secure: microsoftCookieSecure(r, h.cfg.CanonicalOrigin),
		SameSite: http.SameSiteLaxMode, MaxAge: int(microsoftSessionTTL.Seconds()),
	})
	if mfaEnabled && !trustedDeviceLogin {
		redirectStatus(w, r, returnTo, "mfa_required")
		return
	}
	redirectStatus(w, r, returnTo, "success")
}

func (h *Handler) encodeMicrosoftAuthState(payload microsoftAuthState) (string, error) {
	if strings.TrimSpace(h.cfg.SessionSecret) == "" {
		return "", errors.New("session secret is not configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(h.cfg.SessionSecret))
	_, _ = mac.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (h *Handler) decodeMicrosoftAuthState(value string) (microsoftAuthState, error) {
	if strings.TrimSpace(h.cfg.SessionSecret) == "" {
		return microsoftAuthState{}, errors.New("session secret is not configured")
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 || len(parts[0]) > 4096 {
		return microsoftAuthState{}, errors.New("invalid state")
	}
	mac := hmac.New(sha256.New, []byte(h.cfg.SessionSecret))
	_, _ = mac.Write([]byte(parts[0]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return microsoftAuthState{}, errors.New("invalid state signature")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return microsoftAuthState{}, errors.New("invalid state body")
	}
	var payload microsoftAuthState
	if err := json.Unmarshal(body, &payload); err != nil {
		return microsoftAuthState{}, err
	}
	if payload.Nonce == "" || payload.CodeVerifier == "" ||
		(payload.Flow != microsoftLoginFlow && payload.Flow != microsoftLinkFlow) {
		return microsoftAuthState{}, errors.New("incomplete state")
	}
	return payload, nil
}

func (h *Handler) clearMicrosoftAuthCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: microsoftAuthCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: microsoftCookieSecure(r, h.cfg.CanonicalOrigin),
		SameSite: http.SameSiteLaxMode,
	})
}

func safeMicrosoftAuthReturnTo(value, fallback string) string {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || len(value) == 0 || len(value) > 2048 ||
		!strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") ||
		strings.HasPrefix(value, "/\\") || parsed.IsAbs() || parsed.Host != "" ||
		strings.ContainsAny(value, "\r\n") {
		return fallback
	}
	return value
}

func microsoftCookieSecure(r *http.Request, canonicalOrigin string) bool {
	if strings.TrimSpace(canonicalOrigin) != "" {
		return strings.HasPrefix(strings.TrimSpace(canonicalOrigin), "https://")
	}
	host := firstHeader(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return host != "localhost" && host != "127.0.0.1"
}

func (h *Handler) authRateKey(r *http.Request) string {
	clientIP := microsoftRequestClientIP(r)
	mac := hmac.New(sha256.New, []byte(h.cfg.AuthRateLimitHMACSecret))
	_, _ = mac.Write([]byte("askolo:microsoft-auth-rate-limit:v1:" + clientIP))
	return hex.EncodeToString(mac.Sum(nil))
}

func microsoftRequestClientIP(r *http.Request) string {
	peer := microsoftParseRequestIP(r.RemoteAddr)
	if peer == nil {
		if strings.TrimSpace(r.RemoteAddr) == "" {
			return "unknown"
		}
		return strings.TrimSpace(r.RemoteAddr)
	}
	if !microsoftTrustedProxy(peer) {
		return peer.String()
	}
	forwarded := strings.Join(r.Header.Values("X-Forwarded-For"), ",")
	if len(forwarded) == 0 || len(forwarded) > 1024 {
		return peer.String()
	}
	hops := strings.Split(forwarded, ",")
	if len(hops) > 8 {
		return peer.String()
	}
	for i := len(hops) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(hops[i]))
		if ip == nil {
			continue
		}
		if microsoftTrustedProxy(ip) {
			continue
		}
		return ip.String()
	}
	return peer.String()
}

func microsoftParseRequestIP(address string) net.IP {
	address = strings.TrimSpace(address)
	if host, _, err := net.SplitHostPort(address); err == nil {
		address = host
	}
	return net.ParseIP(strings.Trim(address, "[]"))
}

func microsoftTrustedProxy(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return true
	}
	ipv4 := ip.To4()
	return ipv4 != nil && ipv4[0] == 100 && ipv4[1] >= 64 && ipv4[1] <= 127
}
