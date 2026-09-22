package google

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// legacyCalendarEvent adapts the old local-event request shape to the native
// provider operation without retaining an API-server compatibility proxy.
func legacyCalendarEvent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			next(w, r)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64*1024))
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_EVENT", "The calendar event is invalid.")
			return
		}
		var input map[string]any
		if err := json.Unmarshal(body, &input); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_EVENT", "The calendar event is invalid.")
			return
		}
		calendarID, _ := input["calendarId"].(string)
		connectionID, _ := input["connectionId"].(string)
		delete(input, "calendarId")
		delete(input, "connectionId")
		wrapped, err := json.Marshal(map[string]any{
			"connectionId": connectionID,
			"calendarId":   calendarID,
			"event":        input,
		})
		if err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_EVENT", "The calendar event is invalid.")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(wrapped))
		r.ContentLength = int64(len(wrapped))
		next(w, r)
	}
}

func (h *Handler) legacyCalendarSync(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before using Calendar.")
		return
	}
	var input struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decodeBody(r, &input, 4096); err != nil || input.From == "" || input.To == "" {
		writeError(w, http.StatusBadRequest, "INVALID_RANGE", "from and to are required.")
		return
	}
	connectionID, err := h.legacyConnectionID(r, userID, "calendar")
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	_, token, err := h.connectionToken(r.Context(), userID, connectionID, "calendar")
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	calendarsResponse, err := h.googleRequest(r.Context(), http.MethodGet, "https://www.googleapis.com/calendar/v3/users/me/calendarList", token, nil)
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	var calendars struct {
		Items []struct {
			ID      string `json:"id"`
			Primary bool   `json:"primary"`
		} `json:"items"`
	}
	if err := json.NewDecoder(calendarsResponse.Body).Decode(&calendars); err != nil {
		calendarsResponse.Body.Close()
		writeError(w, http.StatusBadGateway, "GOOGLE_PROVIDER_FAILED", "Google could not complete the request.")
		return
	}
	calendarsResponse.Body.Close()
	calendarID := ""
	for _, calendar := range calendars.Items {
		if calendar.Primary {
			calendarID = calendar.ID
			break
		}
	}
	if calendarID == "" && len(calendars.Items) > 0 {
		calendarID = calendars.Items[0].ID
	}
	if calendarID == "" {
		writeError(w, http.StatusNotFound, "NO_CALENDAR", "No calendar found.")
		return
	}
	query := url.Values{"timeMin": {input.From}, "timeMax": {input.To}, "singleEvents": {"true"}}
	eventsResponse, err := h.googleRequest(r.Context(), http.MethodGet,
		"https://www.googleapis.com/calendar/v3/calendars/"+url.PathEscape(calendarID)+"/events?"+query.Encode(), token, nil)
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	var events struct {
		Items []json.RawMessage `json:"items"`
	}
	err = json.NewDecoder(eventsResponse.Body).Decode(&events)
	eventsResponse.Body.Close()
	if err != nil {
		writeError(w, http.StatusBadGateway, "GOOGLE_PROVIDER_FAILED", "Google could not complete the request.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"synced": len(events.Items), "calendarId": calendarID})
}

func (h *Handler) legacyConnectionID(r *http.Request, userID, capability string) (string, error) {
	if value := strings.TrimSpace(r.URL.Query().Get("connectionId")); value != "" {
		return value, nil
	}
	connections, err := h.store.ListGoogleConnections(r.Context(), userID)
	if err != nil {
		return "", operationError{http.StatusInternalServerError, "GOOGLE_STATUS_FAILED", "Google connection status is unavailable."}
	}
	for _, connection := range connections {
		if connection.Status == "active" && hasString(connection.Capabilities, capability) {
			return connection.ID, nil
		}
	}
	return "", operationError{http.StatusNotFound, "CONNECTION_NOT_FOUND", "Google connection not found."}
}

type gmailPayload struct {
	Headers []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"headers"`
	Body struct {
		Data string `json:"data"`
	} `json:"body"`
	Parts []gmailPayload `json:"parts"`
}

type gmailMessage struct {
	ID           string       `json:"id"`
	ThreadID     string       `json:"threadId"`
	Snippet      string       `json:"snippet"`
	InternalDate string       `json:"internalDate"`
	LabelIDs     []string     `json:"labelIds"`
	Payload      gmailPayload `json:"payload"`
}

func (h *Handler) legacyGmailMessages(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before using Gmail.")
		return
	}
	connectionID, err := h.legacyConnectionID(r, userID, "gmail.send")
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	_, token, err := h.connectionToken(r.Context(), userID, connectionID, "gmail.send")
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	listResponse, err := h.googleRequest(r.Context(), http.MethodGet, "https://gmail.googleapis.com/gmail/v1/users/me/messages?maxResults=20", token, nil)
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	var listed struct {
		Messages []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	err = json.NewDecoder(listResponse.Body).Decode(&listed)
	listResponse.Body.Close()
	if err != nil {
		writeError(w, http.StatusBadGateway, "GOOGLE_PROVIDER_FAILED", "Google could not complete the request.")
		return
	}
	messages := make([]map[string]any, 0, len(listed.Messages))
	for _, item := range listed.Messages {
		message, fetchErr := h.fetchGmailMessage(r, token, item.ID)
		if fetchErr != nil {
			continue
		}
		messages = append(messages, map[string]any{
			"id": message.ID, "threadId": message.ThreadID, "subject": gmailHeader(message, "Subject"),
			"from": gmailHeader(message, "From"), "snippet": message.Snippet, "body": gmailBody(message),
			"internalDate": message.InternalDate, "priority": "normal", "labelIds": message.LabelIDs,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
}

func (h *Handler) legacyGmailDraft(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before using Gmail.")
		return
	}
	var input struct {
		MessageID string `json:"messageId"`
		Tone      string `json:"tone"`
	}
	if err := decodeBody(r, &input, 4096); err != nil || input.MessageID == "" {
		writeError(w, http.StatusBadRequest, "INVALID_MESSAGE", "messageId is required.")
		return
	}
	connectionID, err := h.legacyConnectionID(r, userID, "gmail.send")
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	_, token, err := h.connectionToken(r.Context(), userID, connectionID, "gmail.send")
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	message, err := h.fetchGmailMessage(r, token, input.MessageID)
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	draft := "Thank you for your message. I’ll follow up shortly."
	if strings.EqualFold(input.Tone, "friendly") {
		draft = "Thanks for reaching out! I’ll get back to you shortly."
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"to": gmailHeader(message, "From"), "subject": gmailHeader(message, "Subject"), "draft": draft, "messageId": input.MessageID,
	})
}

func (h *Handler) fetchGmailMessage(r *http.Request, token, id string) (gmailMessage, error) {
	response, err := h.googleRequest(r.Context(), http.MethodGet,
		"https://gmail.googleapis.com/gmail/v1/users/me/messages/"+url.PathEscape(id)+"?format=full", token, nil)
	if err != nil {
		return gmailMessage{}, err
	}
	defer response.Body.Close()
	var message gmailMessage
	if err := json.NewDecoder(response.Body).Decode(&message); err != nil {
		return gmailMessage{}, operationError{http.StatusBadGateway, "GOOGLE_PROVIDER_FAILED", "Google could not complete the request."}
	}
	return message, nil
}

func gmailHeader(message gmailMessage, name string) string {
	for _, header := range message.Payload.Headers {
		if strings.EqualFold(header.Name, name) {
			return header.Value
		}
	}
	return ""
}

func gmailBody(message gmailMessage) string {
	if message.Payload.Body.Data != "" {
		if decoded, err := base64.RawURLEncoding.DecodeString(message.Payload.Body.Data); err == nil {
			return string(decoded)
		}
	}
	for _, part := range message.Payload.Parts {
		if text := gmailBody(gmailMessage{Payload: part}); text != "" {
			return text
		}
	}
	return ""
}
