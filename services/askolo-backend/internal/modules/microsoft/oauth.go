package microsoft

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
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	"askolo/backend/internal/platform/authorization"
)

const (
	stateCookieName   = "askolo_microsoft_oauth"
	stateFlow         = "microsoft_integration"
	callbackPath      = "/api/integrations/microsoft/callback"
	stateTTL          = 10 * time.Minute
	authURL           = "https://login.microsoftonline.com/common/oauth2/v2.0/authorize"
	microsoftTokenURL = "https://login.microsoftonline.com/common/oauth2/v2.0/token"
	microsoftGraphURL = "https://graph.microsoft.com/v1.0/me?$select=id,displayName,mail,userPrincipalName"
)

var requestedScopes = []string{"openid", "profile", "email", "offline_access", "User.Read"}

type Handler struct {
	cfg           config.Config
	store         *postgres.Store
	logger        *slog.Logger
	client        *http.Client
	tokenEndpoint string
	graphEndpoint string
}

type statePayload struct {
	Nonce        string `json:"nonce"`
	CodeVerifier string `json:"codeVerifier"`
	ReturnTo     string `json:"returnTo"`
	UserID       string `json:"userId"`
	IssuedAt     int64  `json:"issuedAt"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

type graphProfile struct {
	ID                string `json:"id"`
	DisplayName       string `json:"displayName"`
	Mail              string `json:"mail"`
	UserPrincipalName string `json:"userPrincipalName"`
}

func NewHandler(cfg config.Config, store *postgres.Store, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		cfg: cfg, store: store, logger: logger,
		client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		tokenEndpoint: microsoftTokenURL, graphEndpoint: microsoftGraphURL,
	}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/integrations/microsoft", h.start)
	mux.HandleFunc("GET /api/integrations/microsoft/callback", h.callback)
	mux.HandleFunc("GET /api/integrations/microsoft/status", h.status)
	mux.HandleFunc("GET /api/integrations/microsoft/accounts", h.status)
	mux.HandleFunc("DELETE /api/integrations/microsoft/{connectionID}", h.disconnect)
	return mux
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r, authorization.ActionProviderWrite)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before connecting a Microsoft account.")
		return
	}
	if h.store == nil || h.cfg.SessionSecret == "" ||
		h.cfg.Microsoft.ClientID == "" || h.cfg.Microsoft.ClientSecret == "" ||
		len(h.cfg.Microsoft.TokenEncryptionKey) != 32 {
		writeError(w, http.StatusServiceUnavailable, "MICROSOFT_NOT_CONFIGURED", "Microsoft authorization is not configured.")
		return
	}
	baseURL, err := h.callbackBaseURL(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_HOST", "This host is not configured for Microsoft authorization.")
		return
	}
	nonce, err := randomValue(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STATE_ERROR", "Unable to start Microsoft authorization.")
		return
	}
	verifier, err := randomValue(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STATE_ERROR", "Unable to start Microsoft authorization.")
		return
	}
	payload := statePayload{
		Nonce: nonce, CodeVerifier: verifier, ReturnTo: safeReturnTo(r.URL.Query().Get("returnTo")),
		UserID: userID, IssuedAt: time.Now().Unix(),
	}
	cookieValue, err := h.encodeState(payload)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "MICROSOFT_NOT_CONFIGURED", "Microsoft authorization is not configured.")
		return
	}
	nonceHash := sha256.Sum256([]byte(nonce))
	if err := h.store.InsertOAuthState(
		r.Context(), hex.EncodeToString(nonceHash[:]), stateFlow, userID, payload.ReturnTo, time.Now().Add(stateTTL),
	); err != nil {
		h.logger.Error("Microsoft OAuth state persistence failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "MICROSOFT_UNAVAILABLE", "Microsoft authorization is temporarily unavailable.")
		return
	}

	challenge := base64.RawURLEncoding.EncodeToString(sha256Bytes(verifier))
	params := url.Values{
		"client_id":             {h.cfg.Microsoft.ClientID},
		"redirect_uri":          {baseURL + callbackPath},
		"response_type":         {"code"},
		"response_mode":         {"query"},
		"scope":                 {strings.Join(requestedScopes, " ")},
		"state":                 {nonce},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"prompt":                {"select_account"},
	}
	http.SetCookie(w, &http.Cookie{
		Name: stateCookieName, Value: cookieValue, Path: "/", HttpOnly: true,
		Secure: strings.HasPrefix(baseURL, "https://"), SameSite: http.SameSiteLaxMode,
		MaxAge: int(stateTTL.Seconds()),
	})
	http.Redirect(w, r, authURL+"?"+params.Encode(), http.StatusFound)
}

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	h.clearOAuthCookie(w, r)
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "MICROSOFT_NOT_CONFIGURED", "Microsoft authorization is not configured.")
		return
	}
	baseURL, err := h.callbackBaseURL(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_HOST", "This host is not configured for Microsoft authorization.")
		return
	}
	cookie, err := r.Cookie(stateCookieName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_STATE", "Microsoft authorization could not be verified.")
		return
	}
	payload, err := h.decodeState(cookie.Value)
	now := time.Now()
	if err != nil || payload.IssuedAt < now.Add(-stateTTL).Unix() ||
		payload.IssuedAt > now.Add(time.Minute).Unix() ||
		payload.UserID == "" || payload.Nonce != r.URL.Query().Get("state") {
		writeError(w, http.StatusBadRequest, "INVALID_STATE", "Microsoft authorization could not be verified.")
		return
	}
	nonceHash := sha256.Sum256([]byte(payload.Nonce))
	userID, returnTo, consumed, err := h.store.ConsumeOAuthState(
		r.Context(), hex.EncodeToString(nonceHash[:]), stateFlow,
	)
	if err != nil || !consumed || userID == "" || userID != payload.UserID {
		writeError(w, http.StatusBadRequest, "INVALID_STATE", "Microsoft authorization could not be verified.")
		return
	}
	if returnTo == "" {
		returnTo = payload.ReturnTo
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
	tokens, err := h.exchangeCode(r.Context(), baseURL+callbackPath, code, payload.CodeVerifier)
	if err != nil {
		h.logger.Warn("Microsoft OAuth token exchange failed", "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	profile, err := h.fetchProfile(r.Context(), tokens.AccessToken)
	if err != nil {
		h.logger.Warn("Microsoft account profile lookup failed", "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	email := strings.ToLower(strings.TrimSpace(profile.Mail))
	if email == "" {
		email = strings.ToLower(strings.TrimSpace(profile.UserPrincipalName))
	}
	scopes := splitScopes(tokens.Scope)
	if len(scopes) == 0 {
		scopes = append([]string(nil), requestedScopes...)
	}
	capabilities := []string{}
	if hasString(scopes, "User.Read") {
		capabilities = append(capabilities, "profile.read")
	}
	_, err = h.store.UpsertProviderConnection(
		r.Context(), "microsoft", userID, email, profile.ID, profile.DisplayName, "",
		scopes, capabilities, tokens.AccessToken, tokens.RefreshToken,
		time.Now().Add(time.Duration(tokens.ExpiresIn)*time.Second), tokens.TokenType,
		h.cfg.Microsoft.TokenEncryptionKey,
	)
	if err != nil {
		h.logger.Error("Microsoft connection persistence failed", "user_id", userID, "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	redirectStatus(w, r, returnTo, "connected")
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r, authorization.ActionProviderRead)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before viewing Microsoft accounts.")
		return
	}
	connections, err := h.store.ListProviderConnections(r.Context(), "microsoft", userID)
	if err != nil {
		h.logger.Error("Microsoft connection status failed", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "MICROSOFT_STATUS_FAILED", "Microsoft connection status is unavailable.")
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
	accounts := make([]accountResponse, 0, len(connections))
	connected := false
	for _, connection := range connections {
		accounts = append(accounts, accountResponse{
			ID: connection.ID, Email: connection.Email, DisplayName: connection.DisplayName,
			Status: connection.Status, Scopes: connection.Scopes,
			Capabilities: connection.Capabilities, UpdatedAt: connection.UpdatedAt,
		})
		connected = connected || connection.Status == "active"
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts, "connected": connected})
}

func (h *Handler) disconnect(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r, authorization.ActionProviderWrite)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before disconnecting a Microsoft account.")
		return
	}
	connectionID := strings.TrimSpace(r.PathValue("connectionID"))
	if len(connectionID) < 16 || len(connectionID) > 128 {
		writeError(w, http.StatusBadRequest, "INVALID_CONNECTION", "Invalid Microsoft connection.")
		return
	}
	if err := h.store.RevokeProviderConnection(r.Context(), "microsoft", userID, connectionID); err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			writeError(w, http.StatusNotFound, "CONNECTION_NOT_FOUND", "Microsoft connection not found.")
			return
		}
		h.logger.Error("Microsoft connection disconnect failed", "user_id", userID, "error", err)
		writeError(w, http.StatusInternalServerError, "MICROSOFT_DISCONNECT_FAILED", "Microsoft disconnect failed.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) sessionUserID(r *http.Request, action authorization.Action) (string, int) {
	if h.store == nil {
		return "", http.StatusServiceUnavailable
	}
	cookie, err := r.Cookie(config.CookieName(h.cfg.SessionCookieName))
	if err != nil || cookie.Value == "" {
		return "", http.StatusUnauthorized
	}
	userID, err := h.store.SessionUserID(r.Context(), cookie.Value)
	if err != nil {
		if errors.Is(err, postgres.ErrNotFound) {
			return "", http.StatusUnauthorized
		}
		h.logger.Error("Microsoft connection session lookup failed", "error", err)
		return "", http.StatusServiceUnavailable
	}
	mfaState, err := h.store.SessionMFAState(r.Context(), cookie.Value)
	if err != nil {
		return "", http.StatusServiceUnavailable
	}
	if mfaState.Required && !mfaState.Verified {
		return "", http.StatusForbidden
	}

	workspaceID := strings.TrimSpace(r.Header.Get("X-Askolo-Workspace-ID"))
	if workspaceID == "" {
		workspaceID = postgres.DefaultWorkspaceID(userID)
	}
	if workspaceID == postgres.DefaultWorkspaceID(userID) {
		if err := h.store.EnsurePersonalWorkspace(r.Context(), userID); err != nil {
			h.logger.Error("Microsoft authorization workspace setup failed", "error", err)
			return "", http.StatusServiceUnavailable
		}
	}
	decision, err := h.store.Authorize(r.Context(), authorization.Input{
		ActorUserID: userID, WorkspaceID: workspaceID, Action: action,
	})
	if err != nil {
		h.logger.Error("Microsoft provider authorization failed", "error", err)
		return "", http.StatusServiceUnavailable
	}
	if !decision.Allowed {
		return "", http.StatusForbidden
	}
	return userID, http.StatusOK
}

func (h *Handler) callbackBaseURL(r *http.Request) (string, error) {
	if canonicalOrigin := strings.TrimRight(strings.TrimSpace(h.cfg.CanonicalOrigin), "/"); canonicalOrigin != "" {
		parsed, err := url.Parse(canonicalOrigin)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
			parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", errors.New("canonical origin is invalid")
		}
		if _, allowed := h.cfg.AllowedOAuthHosts[parsed.Hostname()]; !allowed {
			return "", errors.New("canonical origin host is not allowed")
		}
		return canonicalOrigin, nil
	}
	host := strings.TrimSpace(firstHeader(r.Header.Get("X-Forwarded-Host")))
	if host == "" {
		host = strings.TrimSpace(r.Host)
	}
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}
	if _, allowed := h.cfg.AllowedOAuthHosts[host]; !allowed {
		return "", errors.New("host is not allowed")
	}
	scheme := "https"
	if host == "localhost" || host == "127.0.0.1" {
		scheme = strings.TrimSpace(firstHeader(r.Header.Get("X-Forwarded-Proto")))
		if scheme == "" {
			scheme = "http"
		}
		if scheme != "http" && scheme != "https" {
			return "", errors.New("local callback scheme is invalid")
		}
	}
	return scheme + "://" + host, nil
}

func (h *Handler) clearOAuthCookie(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil
	if canonicalOrigin := strings.TrimSpace(h.cfg.CanonicalOrigin); canonicalOrigin != "" {
		secure = strings.HasPrefix(canonicalOrigin, "https://")
	} else {
		host := strings.TrimSpace(firstHeader(r.Header.Get("X-Forwarded-Host")))
		if host == "" {
			host = strings.TrimSpace(r.Host)
		}
		if strings.Contains(host, ":") {
			host = strings.Split(host, ":")[0]
		}
		secure = host != "localhost" && host != "127.0.0.1"
	}
	http.SetCookie(w, &http.Cookie{
		Name: stateCookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
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
	if payload.Nonce == "" || payload.CodeVerifier == "" || payload.UserID == "" {
		return statePayload{}, errors.New("incomplete state")
	}
	return payload, nil
}

func (h *Handler) exchangeCode(ctx context.Context, redirectURI, code, verifier string) (tokenResponse, error) {
	body := url.Values{
		"grant_type": {"authorization_code"}, "code": {code},
		"client_id": {h.cfg.Microsoft.ClientID}, "client_secret": {h.cfg.Microsoft.ClientSecret},
		"redirect_uri": {redirectURI}, "code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.tokenEndpoint, strings.NewReader(body.Encode()))
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
		return tokenResponse{}, fmt.Errorf("Microsoft token endpoint returned %d", response.StatusCode)
	}
	var tokens tokenResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&tokens); err != nil {
		return tokenResponse{}, err
	}
	if tokens.AccessToken == "" || tokens.ExpiresIn <= 0 {
		return tokenResponse{}, errors.New("Microsoft returned an incomplete access token")
	}
	return tokens, nil
}

func (h *Handler) fetchProfile(ctx context.Context, accessToken string) (graphProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.graphEndpoint, nil)
	if err != nil {
		return graphProfile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := h.client.Do(req)
	if err != nil {
		return graphProfile{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return graphProfile{}, fmt.Errorf("Microsoft Graph profile endpoint returned %d", response.StatusCode)
	}
	var profile graphProfile
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&profile); err != nil {
		return graphProfile{}, err
	}
	email := profile.Mail
	if strings.TrimSpace(email) == "" {
		email = profile.UserPrincipalName
	}
	if strings.TrimSpace(profile.ID) == "" ||
		(strings.TrimSpace(email) != "" && !validEmail(email)) {
		return graphProfile{}, errors.New("Microsoft did not return a usable account identity")
	}
	return profile, nil
}

func redirectStatus(w http.ResponseWriter, r *http.Request, returnTo, status string) {
	returnTo = safeReturnTo(returnTo)
	target, err := url.Parse(returnTo)
	if err != nil || !strings.HasPrefix(target.Path, "/") || strings.HasPrefix(target.Path, "//") {
		target = &url.URL{Path: "/profile"}
	}
	query := target.Query()
	query.Set("microsoft", status)
	target.RawQuery = query.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
}

func safeReturnTo(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 2048 || strings.Contains(value, `\`) {
		return "/profile"
	}
	target, err := url.Parse(value)
	if err != nil || target.Scheme != "" || target.Host != "" ||
		!strings.HasPrefix(target.Path, "/") || strings.HasPrefix(target.Path, "//") {
		return "/profile"
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

func splitScopes(value string) []string {
	seen := make(map[string]struct{})
	var scopes []string
	for _, scope := range strings.Fields(value) {
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		scopes = append(scopes, scope)
	}
	return scopes
}

func hasString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"code": code, "error": message})
}
