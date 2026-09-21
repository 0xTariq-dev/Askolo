package authorization

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/platform/apierror"
	policy "askolo/backend/internal/platform/authorization"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9._:@/-]{1,255}$`)
var actionPattern = regexp.MustCompile(`^[a-z][a-z0-9._:-]{0,99}$`)

type Handler struct {
	store  *postgres.Store
	logger *slog.Logger
}

type decisionRequest struct {
	WorkspaceID  string `json:"workspaceId"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	Action       string `json:"action"`
}

func NewHandler(store *postgres.Store, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{store: store, logger: logger}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/authz/decision", h.decide)
	return mux
}

func (h *Handler) decide(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	actorID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, r, status, "UNAUTHORIZED", "Unauthorized.")
		return
	}

	var input decisionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_AUTHORIZATION_REQUEST", "Authorization request is invalid.")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(w, r, http.StatusBadRequest, "INVALID_AUTHORIZATION_REQUEST", "Authorization request is invalid.")
		return
	}
	if input.WorkspaceID != "" && !identifierPattern.MatchString(input.WorkspaceID) {
		writeError(w, r, http.StatusBadRequest, "INVALID_AUTHORIZATION_REQUEST", "Authorization request is invalid.")
		return
	}
	if input.ResourceType != "" && !identifierPattern.MatchString(input.ResourceType) {
		writeError(w, r, http.StatusBadRequest, "INVALID_AUTHORIZATION_REQUEST", "Authorization request is invalid.")
		return
	}
	if input.ResourceID != "" && !identifierPattern.MatchString(input.ResourceID) {
		writeError(w, r, http.StatusBadRequest, "INVALID_AUTHORIZATION_REQUEST", "Authorization request is invalid.")
		return
	}
	if !actionPattern.MatchString(input.Action) {
		writeError(w, r, http.StatusBadRequest, "INVALID_AUTHORIZATION_REQUEST", "Authorization request is invalid.")
		return
	}

	state, stateErr := h.store.SessionMFAState(r.Context(), sessionIDFromRequest(r))
	if stateErr != nil {
		h.logger.Warn("authorization session state lookup failed", "request_id", requestID(r), "error", stateErr)
		writeError(w, r, http.StatusServiceUnavailable, "AUTHORIZATION_UNAVAILABLE", "Authorization is temporarily unavailable.")
		return
	}
	if state.Required && !state.Verified {
		writeError(w, r, http.StatusForbidden, "MFA_REQUIRED", "Complete MFA before accessing the app.")
		return
	}

	workspaceID := strings.TrimSpace(input.WorkspaceID)
	if workspaceID == "" {
		workspaceID = postgres.DefaultWorkspaceID(actorID)
	}
	if workspaceID == postgres.DefaultWorkspaceID(actorID) {
		if err := h.store.EnsurePersonalWorkspace(r.Context(), actorID); err != nil {
			h.logger.Error("personal workspace provisioning failed", "request_id", requestID(r), "actor_id", actorID, "error", err)
			writeError(w, r, http.StatusServiceUnavailable, "AUTHORIZATION_UNAVAILABLE", "Authorization is temporarily unavailable.")
			return
		}
	}

	decision, err := h.store.Authorize(r.Context(), policy.Input{
		ActorUserID:  actorID,
		WorkspaceID:  workspaceID,
		ResourceType: input.ResourceType,
		ResourceID:   input.ResourceID,
		Action:       policy.Action(input.Action),
	})
	duration := time.Since(startedAt).Milliseconds()
	h.logger.Info("authorization decision",
		"request_id", requestID(r),
		"actor_id", actorID,
		"workspace_id", workspaceID,
		"resource_type", input.ResourceType,
		"resource_id", input.ResourceID,
		"action", input.Action,
		"allowed", err == nil && decision.Allowed,
		"reason", decision.Reason,
		"duration_ms", duration,
	)
	if err != nil {
		h.logger.Error("authorization decision failed", "request_id", requestID(r), "actor_id", actorID, "error", err)
		writeError(w, r, http.StatusServiceUnavailable, "AUTHORIZATION_UNAVAILABLE", "Authorization is temporarily unavailable.")
		return
	}
	if !decision.Allowed {
		_ = h.store.CreateSecurityEvent(r.Context(), actorID, "authorization_denied", requestID(r), map[string]any{
			"workspace_id":  workspaceID,
			"resource_type": input.ResourceType,
			"resource_id":   input.ResourceID,
			"action":        input.Action,
			"reason":        decision.Reason,
			"duration_ms":   duration,
		})
		writeError(w, r, http.StatusForbidden, "AUTHORIZATION_DENIED", "You are not allowed to perform this action.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"allowed": true, "workspaceId": workspaceID})
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
		return "", http.StatusServiceUnavailable
	}
	return userID, http.StatusOK
}

func sessionIDFromRequest(r *http.Request) string {
	if cookie, err := r.Cookie("sid"); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	if authorization := r.Header.Get("Authorization"); strings.HasPrefix(authorization, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
	}
	return ""
}

func requestID(r *http.Request) string {
	return r.Header.Get("X-Request-ID")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	apierror.Write(w, r, status, code, message)
}
