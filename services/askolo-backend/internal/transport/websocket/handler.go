package websocket

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
	"askolo/backend/internal/platform/apierror"
	policy "askolo/backend/internal/platform/authorization"
)

const maxSessionDurationSeconds = 120

type Handler struct {
	logger            *slog.Logger
	serviceName       string
	store             *postgres.Store
	sessionCookieName string
}

func New(logger *slog.Logger, serviceName string, store *postgres.Store, sessionCookieName string) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{logger: logger, serviceName: serviceName, store: store, sessionCookieName: sessionCookieName}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apierror.Write(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Method not allowed.")
		return
	}
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		apierror.Write(w, r, status, "UNAUTHORIZED", "Unauthorized.")
		return
	}
	workspaceID := strings.TrimSpace(r.Header.Get("X-Askolo-Workspace-ID"))
	if workspaceID == "" {
		workspaceID = postgres.DefaultWorkspaceID(userID)
	}
	if workspaceID == postgres.DefaultWorkspaceID(userID) {
		if err := h.store.EnsurePersonalWorkspace(r.Context(), userID); err != nil {
			apierror.Write(w, r, http.StatusServiceUnavailable, "AUTHORIZATION_UNAVAILABLE", "Authorization is temporarily unavailable.")
			return
		}
	}
	decision, err := h.store.Authorize(r.Context(), policy.Input{
		ActorUserID: userID,
		WorkspaceID: workspaceID,
		Action:      policy.ActionWebSocketConnect,
	})
	if err != nil {
		apierror.Write(w, r, http.StatusServiceUnavailable, "AUTHORIZATION_UNAVAILABLE", "Authorization is temporarily unavailable.")
		return
	}
	if !decision.Allowed {
		_ = h.store.CreateSecurityEvent(r.Context(), userID, "authorization_denied", r.Header.Get("X-Request-ID"), map[string]any{
			"workspace_id": workspaceID,
			"action":       policy.ActionWebSocketConnect,
			"reason":       decision.Reason,
		})
		apierror.Write(w, r, http.StatusForbidden, "AUTHORIZATION_DENIED", "You are not allowed to use this connection.")
		return
	}

	// The transport boundary is intentionally present before a WebSocket
	// implementation is selected. The eventual adapter must enforce origin,
	// authentication, frame limits, heartbeats, session duration, and close
	// semantics before handing messages to an application module.
	apierror.Write(w, r, http.StatusNotImplemented, "WEBSOCKET_NOT_CONFIGURED", "WebSocket transport is not configured.")
}

func (h *Handler) sessionUserID(r *http.Request) (string, int) {
	if h.store == nil {
		return "", http.StatusServiceUnavailable
	}
	sessionID := ""
	if cookie, err := r.Cookie(config.CookieName(h.sessionCookieName)); err == nil {
		sessionID = cookie.Value
	}
	if sessionID == "" && strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		sessionID = strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	}
	if sessionID == "" {
		return "", http.StatusUnauthorized
	}
	userID, err := h.store.SessionUserID(r.Context(), sessionID)
	if errors.Is(err, postgres.ErrNotFound) {
		return "", http.StatusUnauthorized
	}
	if err != nil {
		return "", http.StatusServiceUnavailable
	}
	state, err := h.store.SessionMFAState(r.Context(), sessionID)
	if err != nil {
		return "", http.StatusServiceUnavailable
	}
	if state.Required && !state.Verified {
		return "", http.StatusForbidden
	}
	return userID, http.StatusOK
}
