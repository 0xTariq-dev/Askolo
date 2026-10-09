package product

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"askolo/backend/internal/adapters/postgres"
)

var (
	homeProviderConnectionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	homeMessageIDPattern            = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)
)

func (h *Handler) v2ListHomeProviderConnections(w http.ResponseWriter, r *http.Request, userID, _ string) {
	workspaceID := r.PathValue("workspaceId")
	connections, err := h.store.ListV2ProviderConnections(r.Context(), workspaceID, userID)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "list Home provider connections", err)
		return
	}
	if connections == nil {
		connections = []postgres.V2HomeProviderConnection{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": connections})
}

func (h *Handler) v2AssociateHomeProviderConnection(w http.ResponseWriter, r *http.Request, userID, _ string) {
	var input struct {
		ConnectionID string `json:"connectionId"`
	}
	if err := decodeHomeBody(w, r, &input, 4096); err != nil || !homeProviderConnectionIDPattern.MatchString(input.ConnectionID) {
		writeError(w, http.StatusBadRequest, "INVALID_CONNECTION", "A valid provider connection is required.")
		return
	}
	err := h.store.AssociateV2ProviderConnection(
		r.Context(), r.PathValue("workspaceId"), userID, input.ConnectionID,
	)
	if errors.Is(err, postgres.ErrOwnership) {
		writeError(w, http.StatusConflict, "CONNECTION_ALREADY_ASSOCIATED", "This provider connection is already associated with a Space.")
		return
	}
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "associate Home provider connection", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"associated": true})
}

func (h *Handler) v2RemoveHomeProviderConnection(w http.ResponseWriter, r *http.Request, userID, role string) {
	connectionID := r.PathValue("connectionId")
	if !homeProviderConnectionIDPattern.MatchString(connectionID) {
		writeError(w, http.StatusBadRequest, "INVALID_CONNECTION", "A valid provider connection is required.")
		return
	}
	err := h.store.RemoveV2ProviderConnection(
		r.Context(), r.PathValue("workspaceId"), userID, role, connectionID,
	)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "remove Home provider connection", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) v2SetHomeProviderAccess(w http.ResponseWriter, r *http.Request, userID, role string) {
	var input struct {
		Access string `json:"access"`
	}
	if err := decodeHomeBody(w, r, &input, 4096); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PROVIDER_ACCESS", "Choose owner-only or Space-shared access.")
		return
	}
	connectionID := r.PathValue("connectionId")
	if !homeProviderConnectionIDPattern.MatchString(connectionID) {
		writeError(w, http.StatusBadRequest, "INVALID_CONNECTION", "A valid provider connection is required.")
		return
	}
	err := h.store.SetV2ProviderAccess(
		r.Context(), r.PathValue("workspaceId"), userID, role, connectionID, input.Access,
	)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "update Home provider access", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"access": input.Access})
}

func (h *Handler) v2SetHomeEmailReadConsent(w http.ResponseWriter, r *http.Request, userID, _ string) {
	var input struct {
		Consent bool `json:"consent"`
	}
	if err := decodeHomeBody(w, r, &input, 4096); err != nil || !input.Consent {
		writeError(w, http.StatusBadRequest, "INVALID_CONSENT", "Explicit message-read consent must be true.")
		return
	}
	connectionID := r.PathValue("connectionId")
	if !homeProviderConnectionIDPattern.MatchString(connectionID) {
		writeError(w, http.StatusBadRequest, "INVALID_CONNECTION", "A valid provider connection is required.")
		return
	}
	err := h.store.SetV2EmailReadConsent(
		r.Context(), r.PathValue("workspaceId"), userID, connectionID,
	)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "record email-read consent", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"consent": true, "policyVersion": postgres.V2HomeEmailConsentPolicyVersion,
	})
}

func (h *Handler) v2RevokeHomeEmailReadConsent(w http.ResponseWriter, r *http.Request, userID, _ string) {
	connectionID := r.PathValue("connectionId")
	if !homeProviderConnectionIDPattern.MatchString(connectionID) {
		writeError(w, http.StatusBadRequest, "INVALID_CONNECTION", "A valid provider connection is required.")
		return
	}
	err := h.store.RevokeV2EmailReadConsent(
		r.Context(), r.PathValue("workspaceId"), userID, connectionID,
	)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "revoke email-read consent", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) v2ListSharedEmailMessages(w http.ResponseWriter, r *http.Request, userID, _ string) {
	workspaceID := r.PathValue("workspaceId")
	connectionID := r.PathValue("connectionId")
	if !homeProviderConnectionIDPattern.MatchString(connectionID) {
		writeError(w, http.StatusBadRequest, "INVALID_CONNECTION", "A valid provider connection is required.")
		return
	}
	if !v2QueryValid(r, "pageToken") {
		writeError(w, http.StatusBadRequest, "INVALID_QUERY", "The request contains an unsupported or repeated query parameter.")
		return
	}
	pageToken := r.URL.Query().Get("pageToken")
	if len(pageToken) > 1024 || strings.ContainsAny(pageToken, "\r\n\x00") {
		writeError(w, http.StatusBadRequest, "INVALID_PAGE_TOKEN", "The message page token is invalid.")
		return
	}
	ownerUserID, err := h.store.V2SharedEmailOwner(r.Context(), workspaceID, userID, connectionID)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "authorize shared email list", err)
		return
	}
	if h.sharedEmailReader == nil {
		writeError(w, http.StatusServiceUnavailable, "EMAIL_PROVIDER_UNAVAILABLE", "Email reading is temporarily unavailable.")
		return
	}
	page, err := h.sharedEmailReader.ListSharedGmailMessages(r.Context(), ownerUserID, connectionID, pageToken)
	if err != nil {
		h.logHomeProviderError(r, userID, "read shared email list", err)
		writeError(w, http.StatusBadGateway, "EMAIL_PROVIDER_FAILED", "The email provider could not complete the request.")
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) v2GetSharedEmailMessage(w http.ResponseWriter, r *http.Request, userID, _ string) {
	workspaceID := r.PathValue("workspaceId")
	connectionID := r.PathValue("connectionId")
	messageID := r.PathValue("messageId")
	if !homeProviderConnectionIDPattern.MatchString(connectionID) ||
		!homeMessageIDPattern.MatchString(messageID) {
		writeError(w, http.StatusBadRequest, "INVALID_MESSAGE", "A valid message is required.")
		return
	}
	ownerUserID, err := h.store.V2SharedEmailOwner(r.Context(), workspaceID, userID, connectionID)
	if err != nil {
		v2HandleStoreError(w, r, h, userID, "authorize shared email detail", err)
		return
	}
	if h.sharedEmailReader == nil {
		writeError(w, http.StatusServiceUnavailable, "EMAIL_PROVIDER_UNAVAILABLE", "Email reading is temporarily unavailable.")
		return
	}
	message, err := h.sharedEmailReader.GetSharedGmailMessage(
		r.Context(), ownerUserID, connectionID, messageID,
	)
	if err != nil {
		h.logHomeProviderError(r, userID, "read shared email detail", err)
		writeError(w, http.StatusBadGateway, "EMAIL_PROVIDER_FAILED", "The email provider could not complete the request.")
		return
	}
	if message.ID != messageID {
		h.logHomeProviderError(r, userID, "validate shared email detail", errors.New("provider returned a different message ID"))
		writeError(w, http.StatusBadGateway, "EMAIL_PROVIDER_FAILED", "The email provider could not complete the request.")
		return
	}
	writeJSON(w, http.StatusOK, message)
}

func (h *Handler) logHomeProviderError(r *http.Request, userID, operation string, err error) {
	if h.logger == nil {
		return
	}
	h.logger.Error("shared email provider request failed",
		"operation", operation,
		"request_id", r.Header.Get("X-Request-ID"),
		"user_id", userID,
		"workspace_id", r.PathValue("workspaceId"),
		"error_kind", fmt.Sprintf("%T", err),
	)
}
