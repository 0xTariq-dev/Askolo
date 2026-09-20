package google

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"askolo/backend/internal/adapters/postgres"
)

var providerIDPattern = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,200}$`)

type operationError struct {
	status  int
	code    string
	message string
}

func (e operationError) Error() string {
	return e.message
}

func (h *Handler) listCalendars(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before using Calendar.")
		return
	}
	connection, token, err := h.connectionToken(r.Context(), userID, r.URL.Query().Get("connectionId"), "calendar")
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	response, err := h.googleRequest(r.Context(), http.MethodGet, "https://www.googleapis.com/calendar/v3/users/me/calendarList", token, nil)
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	_ = connection
	writeGoogleResponse(w, response)
}

func (h *Handler) listCalendarEvents(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before using Calendar.")
		return
	}
	calendarID := r.URL.Query().Get("calendarId")
	if !providerIDPattern.MatchString(calendarID) {
		writeError(w, http.StatusBadRequest, "INVALID_CALENDAR", "A valid calendarId is required.")
		return
	}
	_, token, err := h.connectionToken(r.Context(), userID, r.URL.Query().Get("connectionId"), "calendar")
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	query := url.Values{}
	for _, name := range []string{"timeMin", "timeMax", "pageToken", "singleEvents", "orderBy"} {
		if value := r.URL.Query().Get(name); value != "" {
			if len(value) > 200 {
				writeError(w, http.StatusBadRequest, "INVALID_QUERY", "Calendar query value is too long.")
				return
			}
			query.Set(name, value)
		}
	}
	path := "https://www.googleapis.com/calendar/v3/calendars/" + url.PathEscape(calendarID) + "/events"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	response, err := h.googleRequest(r.Context(), http.MethodGet, path, token, nil)
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	writeGoogleResponse(w, response)
}

func (h *Handler) createCalendarEvent(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before using Calendar.")
		return
	}
	var input struct {
		ConnectionID string          `json:"connectionId"`
		CalendarID   string          `json:"calendarId"`
		Event        json.RawMessage `json:"event"`
	}
	if err := decodeBody(r, &input, 64*1024); err != nil || !providerIDPattern.MatchString(input.CalendarID) || len(input.Event) == 0 || string(input.Event) == "null" {
		writeError(w, http.StatusBadRequest, "INVALID_EVENT", "connectionId, calendarId, and event are required.")
		return
	}
	_, token, err := h.connectionToken(r.Context(), userID, input.ConnectionID, "calendar")
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	response, err := h.googleRequest(r.Context(), http.MethodPost,
		"https://www.googleapis.com/calendar/v3/calendars/"+url.PathEscape(input.CalendarID)+"/events",
		token, input.Event)
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	writeGoogleResponseStatus(w, response, http.StatusCreated)
}

func (h *Handler) updateCalendarEvent(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before using Calendar.")
		return
	}
	eventID := r.PathValue("eventID")
	if !providerIDPattern.MatchString(eventID) {
		writeError(w, http.StatusBadRequest, "INVALID_EVENT", "A valid eventId is required.")
		return
	}
	var input struct {
		ConnectionID string          `json:"connectionId"`
		CalendarID   string          `json:"calendarId"`
		Event        json.RawMessage `json:"event"`
	}
	if err := decodeBody(r, &input, 64*1024); err != nil || !providerIDPattern.MatchString(input.CalendarID) || len(input.Event) == 0 || string(input.Event) == "null" {
		writeError(w, http.StatusBadRequest, "INVALID_EVENT", "connectionId, calendarId, and event are required.")
		return
	}
	_, token, err := h.connectionToken(r.Context(), userID, input.ConnectionID, "calendar")
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	response, err := h.googleRequest(r.Context(), http.MethodPatch,
		"https://www.googleapis.com/calendar/v3/calendars/"+url.PathEscape(input.CalendarID)+"/events/"+url.PathEscape(eventID),
		token, input.Event)
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	writeGoogleResponse(w, response)
}

func (h *Handler) deleteCalendarEvent(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before using Calendar.")
		return
	}
	eventID := r.PathValue("eventID")
	calendarID := r.URL.Query().Get("calendarId")
	if !providerIDPattern.MatchString(eventID) || !providerIDPattern.MatchString(calendarID) {
		writeError(w, http.StatusBadRequest, "INVALID_EVENT", "calendarId and eventId are required.")
		return
	}
	_, token, err := h.connectionToken(r.Context(), userID, r.URL.Query().Get("connectionId"), "calendar")
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	response, err := h.googleRequest(r.Context(), http.MethodDelete,
		"https://www.googleapis.com/calendar/v3/calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(eventID),
		token, nil)
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	response.Body.Close()
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) sendGmail(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before sending email.")
		return
	}
	var input struct {
		ConnectionID string `json:"connectionId"`
		To           string `json:"to"`
		Subject      string `json:"subject"`
		Body         string `json:"body"`
		ThreadID     string `json:"threadId"`
		Confirmed    bool   `json:"confirmed"`
	}
	if err := decodeBody(r, &input, 1024*1024); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_EMAIL", "The email request is invalid.")
		return
	}
	if !input.Confirmed {
		writeError(w, http.StatusBadRequest, "CONFIRMATION_REQUIRED", "Confirm the target account and message before sending.")
		return
	}
	if !validHeaderValue(input.To, 1000) || !validHeaderValue(input.Subject, 998) || len(input.Body) == 0 || len(input.Body) > 900_000 {
		writeError(w, http.StatusBadRequest, "INVALID_EMAIL", "The email fields are invalid.")
		return
	}
	address, err := mail.ParseAddress(input.To)
	if err != nil || strings.TrimSpace(address.Address) == "" {
		writeError(w, http.StatusBadRequest, "INVALID_EMAIL", "A valid recipient is required.")
		return
	}
	_, token, err := h.connectionToken(r.Context(), userID, input.ConnectionID, "gmail.send")
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	raw := "To: " + input.To + "\r\n" +
		"Subject: " + input.Subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" + input.Body
	payload := map[string]string{"raw": base64.RawURLEncoding.EncodeToString([]byte(raw))}
	if input.ThreadID != "" {
		if !providerIDPattern.MatchString(input.ThreadID) {
			writeError(w, http.StatusBadRequest, "INVALID_EMAIL", "The threadId is invalid.")
			return
		}
		payload["threadId"] = input.ThreadID
	}
	body, _ := json.Marshal(payload)
	response, err := h.googleRequest(r.Context(), http.MethodPost, "https://gmail.googleapis.com/gmail/v1/users/me/messages/send", token, body)
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	writeGoogleResponse(w, response)
}

func (h *Handler) connectionToken(ctx context.Context, userID, connectionID, capability string) (postgres.ProviderConnection, string, error) {
	if connectionID == "" || len(connectionID) > 128 {
		return postgres.ProviderConnection{}, "", operationError{http.StatusBadRequest, "INVALID_CONNECTION", "A target Google account is required."}
	}
	connections, err := h.store.ListGoogleConnections(ctx, userID)
	if err != nil {
		return postgres.ProviderConnection{}, "", operationError{http.StatusInternalServerError, "GOOGLE_STATUS_FAILED", "Google connection status is unavailable."}
	}
	var selected postgres.ProviderConnection
	for _, candidate := range connections {
		if candidate.ID == connectionID {
			selected = candidate
			break
		}
	}
	if selected.ID == "" {
		return postgres.ProviderConnection{}, "", operationError{http.StatusNotFound, "CONNECTION_NOT_FOUND", "Google connection not found."}
	}
	if selected.Status != "active" {
		return selected, "", operationError{http.StatusConflict, "CONNECTION_DISCONNECTED", "Reconnect this Google account before using it."}
	}
	if !hasString(selected.Capabilities, capability) {
		return selected, "", operationError{http.StatusForbidden, "CAPABILITY_REQUIRED", "Connect the required Google capability before using this action."}
	}
	credentials, err := h.store.GetGoogleCredentials(ctx, userID, connectionID, h.cfg.Google.TokenEncryptionKey)
	if err != nil {
		return selected, "", operationError{http.StatusUnauthorized, "GOOGLE_AUTHORIZATION_EXPIRED", "Reconnect this Google account before using it."}
	}
	if time.Until(credentials.ExpiresAt) > time.Minute {
		return selected, credentials.AccessToken, nil
	}
	if credentials.RefreshToken == "" {
		return selected, "", operationError{http.StatusUnauthorized, "GOOGLE_AUTHORIZATION_EXPIRED", "Reconnect this Google account before using it."}
	}
	refreshed, err := h.refreshAccessToken(ctx, credentials.RefreshToken)
	if err != nil {
		return selected, "", operationError{http.StatusUnauthorized, "GOOGLE_AUTHORIZATION_EXPIRED", "Reconnect this Google account before using it."}
	}
	expiresAt := time.Now().Add(time.Duration(refreshed.ExpiresIn) * time.Second)
	if err := h.store.UpdateGoogleAccessToken(ctx, userID, connectionID, refreshed.AccessToken, expiresAt, h.cfg.Google.TokenEncryptionKey); err != nil {
		return selected, "", operationError{http.StatusInternalServerError, "GOOGLE_TOKEN_UPDATE_FAILED", "Google authorization could not be refreshed."}
	}
	return selected, refreshed.AccessToken, nil
}

func (h *Handler) refreshAccessToken(ctx context.Context, refreshToken string) (tokenResponse, error) {
	if h.cfg.Google.IntegrationClientID == "" || h.cfg.Google.IntegrationSecret == "" {
		return tokenResponse{}, errors.New("Google integration is not configured")
	}
	body := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {h.cfg.Google.IntegrationClientID},
		"client_secret": {h.cfg.Google.IntegrationSecret},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.Google.TokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := h.client.Do(request)
	if err != nil {
		return tokenResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return tokenResponse{}, fmt.Errorf("Google token refresh returned %d", response.StatusCode)
	}
	var refreshed tokenResponse
	if err := json.NewDecoder(response.Body).Decode(&refreshed); err != nil {
		return tokenResponse{}, err
	}
	if refreshed.AccessToken == "" {
		return tokenResponse{}, errors.New("Google token refresh returned no access token")
	}
	if refreshed.ExpiresIn <= 0 {
		refreshed.ExpiresIn = 3600
	}
	return refreshed, nil
}

func (h *Handler) googleRequest(ctx context.Context, method, endpoint, accessToken string, body []byte) (*http.Response, error) {
	requestContext, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(requestContext, method, endpoint, reader)
	if err != nil {
		return nil, operationError{http.StatusBadRequest, "INVALID_PROVIDER_REQUEST", "The Google request is invalid."}
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := h.client.Do(request)
	if err != nil {
		return nil, operationError{http.StatusBadGateway, "GOOGLE_PROVIDER_UNAVAILABLE", "Google is temporarily unavailable."}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		if response.StatusCode == http.StatusUnauthorized {
			return nil, operationError{http.StatusUnauthorized, "GOOGLE_AUTHORIZATION_EXPIRED", "Reconnect this Google account before using it."}
		}
		return nil, operationError{http.StatusBadGateway, "GOOGLE_PROVIDER_FAILED", "Google could not complete the request."}
	}
	return response, nil
}

func (h *Handler) writeOperationError(w http.ResponseWriter, err error) {
	var operation operationError
	if errors.As(err, &operation) {
		writeError(w, operation.status, operation.code, operation.message)
		return
	}
	writeError(w, http.StatusInternalServerError, "GOOGLE_OPERATION_FAILED", "Google could not complete the request.")
}

func writeGoogleResponse(w http.ResponseWriter, response *http.Response) {
	writeGoogleResponseStatus(w, response, http.StatusOK)
}

func writeGoogleResponseStatus(w http.ResponseWriter, response *http.Response, status int) {
	defer response.Body.Close()
	w.Header().Set("Content-Type", mime.TypeByExtension(".json"))
	w.WriteHeader(status)
	_, _ = io.CopyN(w, response.Body, 2*1024*1024)
}

func decodeBody(r *http.Request, target any, maxBytes int64) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil || int64(len(body)) > maxBytes {
		return errors.New("request body is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON value")
	}
	return nil
}

func validHeaderValue(value string, max int) bool {
	return len(value) > 0 && len(value) <= max && !strings.ContainsAny(value, "\r\n")
}

func hasString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
