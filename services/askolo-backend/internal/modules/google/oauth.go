package google

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
)

const (
	stateCookieName = "askolo_google_oauth"
	stateTTL        = 10 * time.Minute
	sessionTTL      = 7 * 24 * time.Hour
)

const (
	scopeOpenID   = "openid"
	scopeEmail    = "email"
	scopeProfile  = "profile"
	scopeGmail    = "https://www.googleapis.com/auth/gmail.send"
	scopeCalendar = "https://www.googleapis.com/auth/calendar"
)

type Handler struct {
	cfg     config.Config
	store   *postgres.Store
	logger  *slog.Logger
	limiter *rateLimiter
	client  *http.Client
}

type statePayload struct {
	Nonce        string `json:"nonce"`
	CodeVerifier string `json:"codeVerifier"`
	ReturnTo     string `json:"returnTo"`
	Flow         string `json:"flow"`
	Intent       string `json:"intent,omitempty"`
	UserID       string `json:"userId,omitempty"`
	IssuedAt     int64  `json:"issuedAt"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

type userInfo struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
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
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/google", h.startLogin)
	mux.HandleFunc("GET /api/auth/google/callback", h.finishLogin)
	mux.HandleFunc("GET /api/auth/google/link", h.startLink)
	mux.HandleFunc("GET /api/auth/google/link/callback", h.finishLink)
	mux.HandleFunc("GET /api/integrations/google", h.startIntegration)
	mux.HandleFunc("GET /api/integrations/google/callback", h.finishIntegration)
	mux.HandleFunc("GET /api/integrations/google/status", h.status)
	mux.HandleFunc("GET /api/integrations/google/accounts", h.status)
	mux.HandleFunc("DELETE /api/integrations/google/{connectionID}", h.disconnect)
	mux.HandleFunc("DELETE /api/integrations/google/{connectionID}/services/{service}", h.disconnectService)
	mux.HandleFunc("PATCH /api/integrations/google/{connectionID}/capabilities/{capability}", h.setCapability)
	mux.HandleFunc("PUT /api/integrations/google/defaults/{service}", h.setDefault)
	mux.HandleFunc("DELETE /api/integrations/google", h.revokeAll)
	mux.HandleFunc("GET /api/integrations/google/calendar/calendars", h.listCalendars)
	mux.HandleFunc("GET /api/integrations/google/calendar/events", h.listCalendarEvents)
	mux.HandleFunc("POST /api/integrations/google/calendar/events", h.createCalendarEvent)
	mux.HandleFunc("PATCH /api/integrations/google/calendar/events/{eventID}", h.updateCalendarEvent)
	mux.HandleFunc("DELETE /api/integrations/google/calendar/events/{eventID}", h.deleteCalendarEvent)
	mux.HandleFunc("POST /api/integrations/google/gmail/send", h.sendGmail)
	return mux
}

func (h *Handler) startLogin(w http.ResponseWriter, r *http.Request) {
	intent := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("intent")))
	if intent != "signup" {
		intent = "signin"
	}
	h.startWithUser(w, r, "login", "", intent)
}

func (h *Handler) startIntegration(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before connecting a Google account.")
		return
	}
	h.startWithUser(w, r, "integration", userID, "")
}

func (h *Handler) startLink(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before linking a Google account.")
		return
	}
	h.startWithUser(w, r, "link", userID, "link")
}

func (h *Handler) startWithUser(w http.ResponseWriter, r *http.Request, flow, userID, intent string) {
	if !h.limiter.allow(clientKey(r), 20, 10*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many Google authorization attempts.")
		return
	}
	baseURL, err := h.callbackBaseURL(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_HOST", "This host is not configured for Google authorization.")
		return
	}
	returnTo := safeReturnTo(r.URL.Query().Get("returnTo"))
	scopes, err := requestedScopes(r.URL.Query().Get("scope"))
	if flow == "login" || flow == "link" {
		scopes = []string{scopeOpenID, scopeEmail, scopeProfile}
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_SCOPE", err.Error())
		return
	}
	if h.store == nil || h.cfg.SessionSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "GOOGLE_NOT_CONFIGURED", "Google authorization is not configured.")
		return
	}
	if flow == "integration" && len(h.cfg.Google.TokenEncryptionKey) != 32 {
		writeError(w, http.StatusServiceUnavailable, "GOOGLE_NOT_CONFIGURED", "Google integration is not configured.")
		return
	}

	nonce, err := randomValue(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STATE_ERROR", "Unable to start Google authorization.")
		return
	}
	verifier, err := randomValue(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STATE_ERROR", "Unable to start Google authorization.")
		return
	}
	payload := statePayload{
		Nonce:        nonce,
		CodeVerifier: verifier,
		ReturnTo:     returnTo,
		Flow:         flow,
		Intent:       intent,
		UserID:       userID,
		IssuedAt:     time.Now().Unix(),
	}
	stateCookie, err := h.encodeState(payload)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "GOOGLE_NOT_CONFIGURED", "Google authorization is not configured.")
		return
	}
	hash := sha256.Sum256([]byte(nonce))
	if err := h.store.InsertOAuthState(r.Context(), hex.EncodeToString(hash[:]), flow, userID, returnTo, time.Now().Add(stateTTL)); err != nil {
		h.logger.Error("failed to persist Google OAuth state", "flow", flow, "error", err)
		writeError(w, http.StatusServiceUnavailable, "GOOGLE_UNAVAILABLE", "Google authorization is temporarily unavailable.")
		return
	}

	clientID, err := h.clientID(flow)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "GOOGLE_NOT_CONFIGURED", "Google authorization is not configured.")
		return
	}
	challenge := base64.RawURLEncoding.EncodeToString(sha256Bytes(verifier))
	params := url.Values{
		"client_id":              {clientID},
		"redirect_uri":           {baseURL + callbackPath(flow)},
		"response_type":          {"code"},
		"scope":                  {strings.Join(scopes, " ")},
		"access_type":            {"offline"},
		"prompt":                 {"consent"},
		"include_granted_scopes": {"true"},
		"state":                  {nonce},
		"code_challenge":         {challenge},
		"code_challenge_method":  {"S256"},
	}
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    stateCookie,
		Path:     "/",
		HttpOnly: true,
		Secure:   baseURL[:5] == "https",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(stateTTL.Seconds()),
	})
	http.Redirect(w, r, h.cfg.Google.AuthURL+"?"+params.Encode(), http.StatusFound)
}

func (h *Handler) finishLogin(w http.ResponseWriter, r *http.Request) {
	h.finish(w, r, "login")
}

func (h *Handler) finishLink(w http.ResponseWriter, r *http.Request) {
	h.finish(w, r, "link")
}

func (h *Handler) finishIntegration(w http.ResponseWriter, r *http.Request) {
	h.finish(w, r, "integration")
}

func (h *Handler) finish(w http.ResponseWriter, r *http.Request, flow string) {
	clearOAuthCookie(w, r)
	if !h.limiter.allow(clientKey(r), 20, 10*time.Minute) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many Google authorization attempts.")
		return
	}
	baseURL, err := h.callbackBaseURL(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_HOST", "This host is not configured for Google authorization.")
		return
	}
	cookie, err := r.Cookie(stateCookieName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_STATE", "Google authorization could not be verified.")
		return
	}
	payload, err := h.decodeState(cookie.Value)
	if err != nil || payload.Flow != flow || payload.IssuedAt < time.Now().Add(-stateTTL).Unix() {
		writeError(w, http.StatusBadRequest, "INVALID_STATE", "Google authorization could not be verified.")
		return
	}
	if state := r.URL.Query().Get("state"); state == "" || state != payload.Nonce {
		writeError(w, http.StatusBadRequest, "INVALID_STATE", "Google authorization could not be verified.")
		return
	}
	hash := sha256.Sum256([]byte(payload.Nonce))
	userID, returnTo, consumed, err := h.store.ConsumeOAuthState(
		r.Context(), hex.EncodeToString(hash[:]), flow,
	)
	if err != nil || !consumed {
		writeError(w, http.StatusBadRequest, "INVALID_STATE", "Google authorization could not be verified.")
		return
	}
	if returnTo == "" {
		returnTo = payload.ReturnTo
	}
	if providerError := r.URL.Query().Get("error"); providerError != "" {
		redirectStatus(w, r, returnTo, "error")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		redirectStatus(w, r, returnTo, "error")
		return
	}
	tokens, err := h.exchangeCode(r.Context(), flow, baseURL+callbackPath(flow), code, payload.CodeVerifier)
	if err != nil {
		h.logger.Warn("Google OAuth token exchange failed", "flow", flow, "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	info, err := h.fetchUserInfo(r.Context(), tokens.AccessToken)
	if err != nil {
		h.logger.Warn("Google user identity lookup failed", "flow", flow, "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	if flow == "login" {
		h.finishLoginUser(w, r, returnTo, info, payload.Intent)
		return
	}
	if flow == "link" {
		h.finishLinkUser(w, r, returnTo, info, payload.UserID)
		return
	}
	if userID == "" {
		userID, _ = h.sessionUserID(r)
	}
	if userID == "" {
		redirectStatus(w, r, returnTo, "error")
		return
	}
	scopes := splitScopes(tokens.Scope)
	capabilities := capabilitiesForScopes(scopes)
	expiresAt := time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second)
	_, err = h.store.UpsertGoogleConnection(
		r.Context(), userID, info.Email, info.Subject, displayName(info),
		info.Picture, scopes, capabilities, tokens.AccessToken, tokens.RefreshToken,
		expiresAt, tokenType(tokens.TokenType), h.cfg.Google.TokenEncryptionKey,
	)
	if err != nil {
		h.logger.Error("Google connection persistence failed", "user_id", userID, "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	redirectStatus(w, r, returnTo, "connected")
}

func (h *Handler) finishLoginUser(w http.ResponseWriter, r *http.Request, returnTo string, info userInfo, intent string) {
	identity, err := h.store.FindProviderIdentity(r.Context(), "google", info.Subject)
	if err == nil {
		if !identity.LoginEnabled {
			redirectStatus(w, r, returnTo, "provider_not_linked")
			return
		}
		h.createLoginSession(w, r, returnTo, identity.UserID)
		return
	}
	if !errors.Is(err, postgres.ErrNotFound) {
		h.logger.Error("Google identity lookup failed", "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	if intent != "signup" {
		redirectStatus(w, r, returnTo, "provider_not_linked")
		return
	}
	if !info.EmailVerified {
		redirectStatus(w, r, returnTo, "error")
		return
	}
	email := strings.ToLower(strings.TrimSpace(info.Email))
	if !validEmail(email) {
		redirectStatus(w, r, returnTo, "error")
		return
	}
	userID, err := h.store.CreateProviderSignupUser(
		r.Context(), "google", info.Subject, email, info.GivenName, info.FamilyName, info.Picture,
	)
	if errors.Is(err, postgres.ErrEmailExists) {
		redirectStatus(w, r, returnTo, "provider_not_linked")
		return
	}
	if err != nil {
		h.logger.Error("Google user persistence failed", "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	h.createLoginSession(w, r, returnTo, userID)
}

func (h *Handler) createLoginSession(w http.ResponseWriter, r *http.Request, returnTo, userID string) {
	sessionID, err := h.store.CreateSession(r.Context(), userID, "google", sessionTTL)
	if err != nil {
		h.logger.Error("Google session persistence failed", "user_id", userID, "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "sid",
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   cookieSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	redirectStatus(w, r, returnTo, "success")
}

func (h *Handler) finishLinkUser(w http.ResponseWriter, r *http.Request, returnTo string, info userInfo, userID string) {
	if userID == "" {
		userID, _ = h.sessionUserID(r)
	}
	if userID == "" {
		redirectStatus(w, r, returnTo, "error")
		return
	}
	if err := h.store.LinkProviderIdentity(
		r.Context(), userID, "google", info.Subject, info.Email, displayName(info), info.Picture,
	); err != nil {
		if errors.Is(err, postgres.ErrOwnership) {
			redirectStatus(w, r, returnTo, "provider_not_linked")
			return
		}
		h.logger.Error("Google identity linking failed", "user_id", userID, "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	redirectStatus(w, r, returnTo, "linked")
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before viewing Google connections.")
		return
	}
	connections, err := h.store.ListGoogleConnections(r.Context(), userID)
	if err != nil {
		h.logger.Error("Google connection status failed", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "GOOGLE_STATUS_FAILED", "Google connection status is unavailable.")
		return
	}
	type accountResponse struct {
		ID           string    `json:"id"`
		Email        string    `json:"email,omitempty"`
		DisplayName  string    `json:"displayName,omitempty"`
		Status       string    `json:"status"`
		Scopes       []string  `json:"scopes"`
		Capabilities []string  `json:"capabilities"`
		UpdatedAt    time.Time `json:"updatedAt"`
	}
	response := make([]accountResponse, 0, len(connections))
	for _, connection := range connections {
		response = append(response, accountResponse{
			ID: connection.ID, Email: connection.Email, DisplayName: connection.DisplayName,
			Status: connection.Status, Scopes: connection.Scopes, Capabilities: connection.Capabilities,
			UpdatedAt: connection.UpdatedAt,
		})
	}
	defaults, err := h.store.ListProviderDefaults(r.Context(), userID)
	if err != nil {
		h.logger.Error("Google defaults lookup failed", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "GOOGLE_STATUS_FAILED", "Google connection status is unavailable.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accounts":  response,
		"connected": len(response) > 0,
		"defaults":  defaults,
	})
}

func (h *Handler) disconnect(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before disconnecting Google.")
		return
	}
	connectionID := r.PathValue("connectionID")
	if len(connectionID) < 16 || len(connectionID) > 128 {
		writeError(w, http.StatusBadRequest, "INVALID_CONNECTION", "Invalid Google connection.")
		return
	}
	credentials, err := h.store.GetGoogleCredentials(r.Context(), userID, connectionID, h.cfg.Google.TokenEncryptionKey)
	if err != nil && !errors.Is(err, postgres.ErrNotFound) {
		h.logger.Error("Google credentials lookup failed", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "GOOGLE_DISCONNECT_FAILED", "Google disconnect failed.")
		return
	}
	if credentials.AccessToken != "" {
		h.revokeToken(r.Context(), credentials.AccessToken)
	}
	if err := h.store.RevokeGoogleConnection(r.Context(), userID, connectionID); err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			writeError(w, http.StatusNotFound, "CONNECTION_NOT_FOUND", "Google connection not found.")
			return
		}
		h.logger.Error("Google connection revoke failed", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "GOOGLE_DISCONNECT_FAILED", "Google disconnect failed.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) disconnectService(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before disconnecting Google.")
		return
	}
	if r.URL.Query().Get("dataAction") == "delete" {
		writeError(w, http.StatusNotImplemented, "IMPORTED_DATA_DELETE_UNAVAILABLE", "Imported-data deletion must be completed before this service can be disconnected.")
		return
	}
	connectionID := r.PathValue("connectionID")
	capability := capabilityForService(r.PathValue("service"))
	if capability == "" {
		writeError(w, http.StatusBadRequest, "INVALID_SERVICE", "Service must be calendar or gmail.")
		return
	}
	connection, err := h.findConnection(r.Context(), userID, connectionID)
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	if err := h.store.UpdateGoogleCapabilities(r.Context(), userID, connectionID, removeString(connection.Capabilities, capability)); err != nil {
		h.writeOperationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) setCapability(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before changing Google capabilities.")
		return
	}
	capability := r.PathValue("capability")
	if capability != "calendar" && capability != "gmail.send" {
		writeError(w, http.StatusBadRequest, "INVALID_CAPABILITY", "Unsupported Google capability.")
		return
	}
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input); err != nil || input.Enabled == nil {
		writeError(w, http.StatusBadRequest, "INVALID_CAPABILITY", "enabled is required.")
		return
	}
	connection, err := h.findConnection(r.Context(), userID, r.PathValue("connectionID"))
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	capabilities := connection.Capabilities
	if *input.Enabled {
		if !hasString(connection.Scopes, scopeForCapability(capability)) {
			writeError(w, http.StatusForbidden, "CAPABILITY_NOT_GRANTED", "Google has not granted this capability.")
			return
		}
		capabilities = appendUnique(capabilities, capability)
	} else {
		capabilities = removeString(capabilities, capability)
	}
	if err := h.store.UpdateGoogleCapabilities(r.Context(), userID, connection.ID, capabilities); err != nil {
		h.writeOperationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"capabilities": capabilities})
}

func (h *Handler) setDefault(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before selecting a default Google account.")
		return
	}
	service := r.PathValue("service")
	if capabilityForService(service) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_SERVICE", "Service must be calendar or gmail.")
		return
	}
	var input struct {
		ConnectionID string `json:"connectionId"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input); err != nil || input.ConnectionID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_CONNECTION", "connectionId is required.")
		return
	}
	connection, err := h.findConnection(r.Context(), userID, input.ConnectionID)
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	if !hasString(connection.Capabilities, capabilityForService(service)) {
		writeError(w, http.StatusForbidden, "CAPABILITY_REQUIRED", "Connect the required Google capability before selecting this account.")
		return
	}
	if err := h.store.SetProviderDefault(r.Context(), userID, service, capabilityForService(service), input.ConnectionID); err != nil {
		h.writeOperationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"service": service, "connectionId": input.ConnectionID})
}

func (h *Handler) revokeAll(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before revoking Google.")
		return
	}
	if r.URL.Query().Get("dataAction") == "delete" {
		writeError(w, http.StatusNotImplemented, "IMPORTED_DATA_DELETE_UNAVAILABLE", "Imported-data deletion must be completed before Google can be revoked.")
		return
	}
	connections, err := h.store.ListGoogleConnections(r.Context(), userID)
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	for _, connection := range connections {
		credentials, credentialErr := h.store.GetGoogleCredentials(r.Context(), userID, connection.ID, h.cfg.Google.TokenEncryptionKey)
		if credentialErr == nil {
			h.revokeToken(r.Context(), credentials.AccessToken)
		}
	}
	if err := h.store.RevokeAllGoogleConnections(r.Context(), userID); err != nil {
		h.writeOperationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) findConnection(ctx context.Context, userID, connectionID string) (postgres.ProviderConnection, error) {
	if connectionID == "" || len(connectionID) > 128 {
		return postgres.ProviderConnection{}, operationError{http.StatusBadRequest, "INVALID_CONNECTION", "A target Google account is required."}
	}
	connections, err := h.store.ListGoogleConnections(ctx, userID)
	if err != nil {
		return postgres.ProviderConnection{}, operationError{http.StatusInternalServerError, "GOOGLE_STATUS_FAILED", "Google connection status is unavailable."}
	}
	for _, connection := range connections {
		if connection.ID == connectionID {
			return connection, nil
		}
	}
	return postgres.ProviderConnection{}, operationError{http.StatusNotFound, "CONNECTION_NOT_FOUND", "Google connection not found."}
}

func (h *Handler) sessionUserID(r *http.Request) (string, int) {
	if h.store == nil {
		return "", http.StatusServiceUnavailable
	}
	cookie, err := r.Cookie("sid")
	if err != nil || cookie.Value == "" {
		return "", http.StatusUnauthorized
	}
	userID, err := h.store.SessionUserID(r.Context(), cookie.Value)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return "", http.StatusUnauthorized
		}
		h.logger.Error("session lookup failed", "error", err)
		return "", http.StatusServiceUnavailable
	}
	return userID, http.StatusOK
}

func (h *Handler) clientID(flow string) (string, error) {
	if (flow == "login" || flow == "link") && h.cfg.Google.LoginClientID != "" {
		return h.cfg.Google.LoginClientID, nil
	}
	if flow == "integration" && h.cfg.Google.IntegrationClientID != "" {
		return h.cfg.Google.IntegrationClientID, nil
	}
	return "", errors.New("Google OAuth client is not configured")
}

func (h *Handler) clientSecret(flow string) (string, error) {
	if (flow == "login" || flow == "link") && h.cfg.Google.LoginClientSecret != "" {
		return h.cfg.Google.LoginClientSecret, nil
	}
	if flow == "integration" && h.cfg.Google.IntegrationSecret != "" {
		return h.cfg.Google.IntegrationSecret, nil
	}
	return "", errors.New("Google OAuth client secret is not configured")
}

func (h *Handler) callbackBaseURL(r *http.Request) (string, error) {
	host := strings.TrimSpace(firstHeader(r.Header.Get("X-Forwarded-Host")))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}
	if _, ok := h.cfg.AllowedOAuthHosts[host]; !ok {
		return "", errors.New("host is not allowed")
	}
	protocol := "https"
	if host == "localhost" || host == "127.0.0.1" {
		protocol = strings.TrimSpace(firstHeader(r.Header.Get("X-Forwarded-Proto")))
		if protocol == "" {
			protocol = "http"
		}
	}
	return protocol + "://" + host, nil
}

func (h *Handler) encodeState(payload statePayload) (string, error) {
	if h.cfg.SessionSecret == "" {
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

func (h *Handler) decodeState(value string) (statePayload, error) {
	if h.cfg.SessionSecret == "" {
		return statePayload{}, errors.New("session secret is not configured")
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 || len(parts[0]) > 4096 {
		return statePayload{}, errors.New("invalid state")
	}
	mac := hmac.New(sha256.New, []byte(h.cfg.SessionSecret))
	_, _ = mac.Write([]byte(parts[0]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return statePayload{}, errors.New("invalid state signature")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return statePayload{}, errors.New("invalid state body")
	}
	var payload statePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return statePayload{}, err
	}
	if payload.Nonce == "" || payload.CodeVerifier == "" || payload.Flow == "" {
		return statePayload{}, errors.New("incomplete state")
	}
	return payload, nil
}

func (h *Handler) exchangeCode(ctx context.Context, flow, redirectURI, code, verifier string) (tokenResponse, error) {
	secret, err := h.clientSecret(flow)
	if err != nil {
		return tokenResponse{}, err
	}
	clientID, err := h.clientID(flow)
	if err != nil {
		return tokenResponse{}, err
	}
	body := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"client_secret": {secret},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.Google.TokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := h.client.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return tokenResponse{}, fmt.Errorf("Google token endpoint returned %d", response.StatusCode)
	}
	var tokens tokenResponse
	if err := json.NewDecoder(response.Body).Decode(&tokens); err != nil {
		return tokenResponse{}, err
	}
	if tokens.AccessToken == "" {
		return tokenResponse{}, errors.New("Google returned no access token")
	}
	return tokens, nil
}

func (h *Handler) fetchUserInfo(ctx context.Context, accessToken string) (userInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.cfg.Google.UserInfoURL, nil)
	if err != nil {
		return userInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := h.client.Do(req)
	if err != nil {
		return userInfo{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return userInfo{}, fmt.Errorf("Google userinfo endpoint returned %d", response.StatusCode)
	}
	var info userInfo
	if err := json.NewDecoder(response.Body).Decode(&info); err != nil {
		return userInfo{}, err
	}
	if info.Subject == "" || !validEmail(info.Email) || !info.EmailVerified {
		return userInfo{}, errors.New("Google did not return a verified email identity")
	}
	return info, nil
}

func (h *Handler) revokeToken(ctx context.Context, token string) {
	if token == "" || h.cfg.Google.RevokeURL == "" {
		return
	}
	body := url.Values{"token": {token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.Google.RevokeURL, strings.NewReader(body.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := h.client.Do(req)
	if err == nil {
		response.Body.Close()
	}
}

func callbackPath(flow string) string {
	if flow == "login" {
		return "/api/auth/google/callback"
	}
	if flow == "link" {
		return "/api/auth/google/link/callback"
	}
	return "/api/integrations/google/callback"
}

func requestedScopes(value string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all":
		return []string{scopeOpenID, scopeEmail, scopeProfile, scopeCalendar, scopeGmail}, nil
	case "calendar":
		return []string{scopeOpenID, scopeEmail, scopeProfile, scopeCalendar}, nil
	case "gmail":
		return []string{scopeOpenID, scopeEmail, scopeProfile, scopeGmail}, nil
	default:
		return nil, errors.New("scope must be calendar, gmail, or all")
	}
}

func capabilitiesForScopes(scopes []string) []string {
	set := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		set[scope] = struct{}{}
	}
	var capabilities []string
	if _, ok := set[scopeCalendar]; ok {
		capabilities = append(capabilities, "calendar")
	}
	if _, ok := set[scopeGmail]; ok {
		capabilities = append(capabilities, "gmail.send")
	}
	return capabilities
}

func capabilityForService(service string) string {
	switch service {
	case "calendar":
		return "calendar"
	case "gmail":
		return "gmail.send"
	default:
		return ""
	}
}

func scopeForCapability(capability string) string {
	switch capability {
	case "calendar":
		return scopeCalendar
	case "gmail.send":
		return scopeGmail
	default:
		return ""
	}
}

func appendUnique(values []string, target string) []string {
	if hasString(values, target) {
		return values
	}
	return append(values, target)
}

func removeString(values []string, target string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			filtered = append(filtered, value)
		}
	}
	return filtered
}

func splitScopes(value string) []string {
	seen := make(map[string]struct{})
	var scopes []string
	for _, scope := range strings.Fields(value) {
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		scopes = append(scopes, scope)
	}
	return scopes
}

func displayName(info userInfo) string {
	if strings.TrimSpace(info.Name) != "" {
		return strings.TrimSpace(info.Name)
	}
	return strings.TrimSpace(strings.Join([]string{info.GivenName, info.FamilyName}, " "))
}

func tokenType(value string) string {
	if strings.TrimSpace(value) == "" {
		return "Bearer"
	}
	return value
}

func safeReturnTo(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 2048 || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return "/dashboard"
	}
	return value
}

func validEmail(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 320 || strings.ContainsAny(value, " \r\n") {
		return false
	}
	at := strings.LastIndexByte(value, '@')
	return at > 0 && at < len(value)-1 && strings.Contains(value[at+1:], ".")
}

func randomValue(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func sha256Bytes(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func firstHeader(value string) string {
	if index := strings.IndexByte(value, ','); index >= 0 {
		return strings.TrimSpace(value[:index])
	}
	return strings.TrimSpace(value)
}

func clientKey(r *http.Request) string {
	value := firstHeader(r.Header.Get("X-Forwarded-For"))
	if value == "" {
		value = r.RemoteAddr
	}
	if value == "" {
		return "unknown"
	}
	return value
}

func (l *rateLimiter) allow(key string, max int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	entry, ok := l.entries[key]
	if !ok || now.After(entry.resetAt) {
		l.entries[key] = rateEntry{count: 1, resetAt: now.Add(window)}
		return true
	}
	if entry.count >= max {
		return false
	}
	entry.count++
	l.entries[key] = entry
	return true
}

func clearOAuthCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: stateCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: cookieSecure(r), SameSite: http.SameSiteLaxMode})
}

func cookieSecure(r *http.Request) bool {
	host := firstHeader(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	return !strings.HasPrefix(host, "localhost") && !strings.HasPrefix(host, "127.0.0.1")
}

func redirectStatus(w http.ResponseWriter, r *http.Request, returnTo, status string) {
	returnTo = safeReturnTo(returnTo)
	target, err := url.Parse(returnTo)
	if err != nil || !strings.HasPrefix(target.Path, "/") || strings.HasPrefix(target.Path, "//") {
		target = &url.URL{Path: "/dashboard"}
	}
	query := target.Query()
	query.Set("google", status)
	target.RawQuery = query.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
