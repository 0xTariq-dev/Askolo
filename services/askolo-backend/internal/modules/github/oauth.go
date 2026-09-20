package github

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
	"strconv"
	"strings"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
)

const (
	stateCookieName = "askolo_github_oauth"
	stateTTL        = 10 * time.Minute
	sessionTTL      = 7 * 24 * time.Hour
)

type Handler struct {
	cfg    config.Config
	store  *postgres.Store
	logger *slog.Logger
	client *http.Client
}

type statePayload struct {
	Nonce    string `json:"nonce"`
	ReturnTo string `json:"returnTo"`
	Flow     string `json:"flow"`
	Intent   string `json:"intent,omitempty"`
	UserID   string `json:"userId,omitempty"`
	IssuedAt int64  `json:"issuedAt"`
}

type githubUser struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

type githubEmail struct {
	Email    string `json:"email"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
}

func NewHandler(cfg config.Config, store *postgres.Store, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		cfg:    cfg,
		store:  store,
		logger: logger,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/auth/github", h.startLogin)
	mux.HandleFunc("GET /api/auth/github/callback", h.finishLogin)
	mux.HandleFunc("GET /api/auth/github/link", h.startLink)
	mux.HandleFunc("GET /api/auth/github/link/callback", h.finishLink)
	return mux
}

func (h *Handler) startLogin(w http.ResponseWriter, r *http.Request) {
	intent := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("intent")))
	if intent != "signup" {
		intent = "signin"
	}
	h.start(w, r, "login", "", intent)
}

func (h *Handler) startLink(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before linking a GitHub account.")
		return
	}
	h.start(w, r, "link", userID, "link")
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request, flow, userID, intent string) {
	if h.store == nil || h.cfg.SessionSecret == "" ||
		h.cfg.GitHub.ClientID == "" || h.cfg.GitHub.ClientSecret == "" {
		writeError(w, http.StatusServiceUnavailable, "GITHUB_NOT_CONFIGURED", "GitHub authorization is not configured.")
		return
	}
	returnTo := safeReturnTo(r.URL.Query().Get("returnTo"))
	nonce, err := randomValue(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STATE_ERROR", "Unable to start GitHub authorization.")
		return
	}
	payload := statePayload{
		Nonce: nonce, ReturnTo: returnTo, Flow: flow, Intent: intent,
		UserID: userID, IssuedAt: time.Now().Unix(),
	}
	encoded, err := h.encodeState(payload)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "GITHUB_NOT_CONFIGURED", "GitHub authorization is not configured.")
		return
	}
	hash := sha256.Sum256([]byte(nonce))
	if err := h.store.InsertOAuthState(
		r.Context(), hex.EncodeToString(hash[:]), "github_"+flow, userID, returnTo, time.Now().Add(stateTTL),
	); err != nil {
		h.logger.Error("failed to persist GitHub OAuth state", "flow", flow, "error", err)
		writeError(w, http.StatusServiceUnavailable, "GITHUB_UNAVAILABLE", "GitHub authorization is temporarily unavailable.")
		return
	}
	baseURL, err := h.callbackBaseURL(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_HOST", "This host is not configured for GitHub authorization.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: stateCookieName, Value: encoded, Path: "/", HttpOnly: true,
		Secure: strings.HasPrefix(baseURL, "https://"), SameSite: http.SameSiteLaxMode,
		MaxAge: int(stateTTL.Seconds()),
	})
	params := url.Values{
		"client_id":    {h.cfg.GitHub.ClientID},
		"redirect_uri": {baseURL + callbackPath(flow)},
		"scope":        {"read:user user:email"},
		"state":        {nonce},
		"allow_signup": {"false"},
	}
	http.Redirect(w, r, h.cfg.GitHub.AuthURL+"?"+params.Encode(), http.StatusFound)
}

func (h *Handler) finishLogin(w http.ResponseWriter, r *http.Request) {
	h.finish(w, r, "login")
}

func (h *Handler) finishLink(w http.ResponseWriter, r *http.Request) {
	h.finish(w, r, "link")
}

func (h *Handler) finish(w http.ResponseWriter, r *http.Request, flow string) {
	clearCookie(w, r)
	cookie, err := r.Cookie(stateCookieName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_STATE", "GitHub authorization could not be verified.")
		return
	}
	payload, err := h.decodeState(cookie.Value)
	if err != nil || payload.Flow != flow || payload.IssuedAt < time.Now().Add(-stateTTL).Unix() ||
		r.URL.Query().Get("state") != payload.Nonce {
		writeError(w, http.StatusBadRequest, "INVALID_STATE", "GitHub authorization could not be verified.")
		return
	}
	hash := sha256.Sum256([]byte(payload.Nonce))
	userID, returnTo, consumed, err := h.store.ConsumeOAuthState(
		r.Context(), hex.EncodeToString(hash[:]), "github_"+flow,
	)
	if err != nil || !consumed {
		writeError(w, http.StatusBadRequest, "INVALID_STATE", "GitHub authorization could not be verified.")
		return
	}
	if returnTo == "" {
		returnTo = payload.ReturnTo
	}
	if r.URL.Query().Get("error") != "" {
		redirectStatus(w, r, returnTo, "error")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		redirectStatus(w, r, returnTo, "error")
		return
	}
	info, err := h.fetchIdentity(r.Context(), code, r, flow)
	if err != nil {
		h.logger.Warn("GitHub identity lookup failed", "flow", flow, "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	if flow == "link" {
		if userID == "" {
			userID, _ = h.sessionUserID(r)
		}
		if userID == "" {
			redirectStatus(w, r, returnTo, "error")
			return
		}
		if err := h.store.LinkProviderIdentity(
			r.Context(), userID, "github", info.subject, info.email, info.displayName, info.avatarURL,
		); err != nil {
			if errors.Is(err, postgres.ErrOwnership) {
				redirectStatus(w, r, returnTo, "provider_not_linked")
				return
			}
			h.logger.Error("GitHub identity linking failed", "user_id", userID, "error", err)
			redirectStatus(w, r, returnTo, "error")
			return
		}
		redirectStatus(w, r, returnTo, "linked")
		return
	}

	identity, err := h.store.FindProviderIdentity(r.Context(), "github", info.subject)
	if err == nil {
		if !identity.LoginEnabled {
			redirectStatus(w, r, returnTo, "provider_not_linked")
			return
		}
		h.createSession(w, r, returnTo, identity.UserID)
		return
	}
	if !errors.Is(err, postgres.ErrNotFound) {
		h.logger.Error("GitHub identity lookup failed", "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	if payload.Intent != "signup" || !info.emailVerified {
		redirectStatus(w, r, returnTo, "provider_not_linked")
		return
	}
	createdUserID, err := h.store.CreateProviderSignupUser(
		r.Context(), "github", info.subject, info.email, info.firstName, info.lastName, info.avatarURL,
	)
	if errors.Is(err, postgres.ErrEmailExists) {
		redirectStatus(w, r, returnTo, "provider_not_linked")
		return
	}
	if err != nil {
		h.logger.Error("GitHub user persistence failed", "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	h.createSession(w, r, returnTo, createdUserID)
}

type identity struct {
	subject       string
	email         string
	emailVerified bool
	firstName     string
	lastName      string
	displayName   string
	avatarURL     string
}

func (h *Handler) fetchIdentity(ctx context.Context, code string, r *http.Request, flow string) (identity, error) {
	baseURL, err := h.callbackBaseURL(r)
	if err != nil {
		return identity{}, err
	}
	body := url.Values{
		"client_id": {h.cfg.GitHub.ClientID}, "client_secret": {h.cfg.GitHub.ClientSecret},
		"code": {code}, "redirect_uri": {baseURL + callbackPath(flow)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.GitHub.TokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return identity{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := h.client.Do(req)
	if err != nil {
		return identity{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return identity{}, fmt.Errorf("GitHub token endpoint returned %d", response.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil || token.AccessToken == "" {
		return identity{}, errors.New("GitHub returned no access token")
	}
	user, err := githubRequest[githubUser](ctx, h.client, h.cfg.GitHub.UserURL, token.AccessToken)
	if err != nil {
		return identity{}, err
	}
	emails, err := githubRequest[[]githubEmail](ctx, h.client, h.cfg.GitHub.EmailsURL, token.AccessToken)
	if err != nil {
		return identity{}, err
	}
	var selected githubEmail
	for _, candidate := range emails {
		if candidate.Primary && candidate.Verified {
			selected = candidate
			break
		}
	}
	if selected.Email == "" {
		for _, candidate := range emails {
			if candidate.Verified {
				selected = candidate
				break
			}
		}
	}
	if selected.Email == "" || !validEmail(selected.Email) {
		return identity{}, errors.New("GitHub did not return a verified email identity")
	}
	firstName, lastName := splitName(user.Name, user.Login)
	return identity{
		subject: strconv.FormatInt(user.ID, 10), email: strings.ToLower(strings.TrimSpace(selected.Email)),
		emailVerified: true, firstName: firstName, lastName: lastName,
		displayName: strings.TrimSpace(user.Name), avatarURL: user.AvatarURL,
	}, nil
}

func githubRequest[T any](ctx context.Context, client *http.Client, endpoint, accessToken string) (T, error) {
	var result T
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return result, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, fmt.Errorf("GitHub API returned %d", response.StatusCode)
	}
	return result, json.NewDecoder(response.Body).Decode(&result)
}

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request, returnTo, userID string) {
	sessionID, err := h.store.CreateSession(r.Context(), userID, "github", sessionTTL)
	if err != nil {
		h.logger.Error("GitHub session persistence failed", "user_id", userID, "error", err)
		redirectStatus(w, r, returnTo, "error")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "sid", Value: sessionID, Path: "/", HttpOnly: true,
		Secure: !strings.HasPrefix(r.Host, "localhost"), SameSite: http.SameSiteLaxMode,
		MaxAge: int(sessionTTL.Seconds()),
	})
	redirectStatus(w, r, returnTo, "success")
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
	if errors.Is(err, postgres.ErrNotFound) {
		return "", http.StatusUnauthorized
	}
	if err != nil {
		return "", http.StatusServiceUnavailable
	}
	return userID, http.StatusOK
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
	if err := json.Unmarshal(body, &payload); err != nil || payload.Nonce == "" || payload.Flow == "" {
		return statePayload{}, errors.New("incomplete state")
	}
	return payload, nil
}

func callbackPath(flow string) string {
	if flow == "link" {
		return "/api/auth/github/link/callback"
	}
	return "/api/auth/github/callback"
}

func splitName(name, login string) (string, string) {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) == 0 {
		return login, ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func randomValue(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func clearCookie(w http.ResponseWriter, r *http.Request) {
	host := firstHeader(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	secure := !strings.HasPrefix(host, "localhost") && !strings.HasPrefix(host, "127.0.0.1")
	http.SetCookie(w, &http.Cookie{Name: stateCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode})
}

func redirectStatus(w http.ResponseWriter, r *http.Request, returnTo, status string) {
	target, err := url.Parse(safeReturnTo(returnTo))
	if err != nil {
		target = &url.URL{Path: "/dashboard"}
	}
	query := target.Query()
	query.Set("github", status)
	target.RawQuery = query.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound)
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

func firstHeader(value string) string {
	if index := strings.IndexByte(value, ','); index >= 0 {
		return strings.TrimSpace(value[:index])
	}
	return strings.TrimSpace(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code, "error": message})
}
