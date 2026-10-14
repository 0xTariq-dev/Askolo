package google

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"askolo/backend/internal/adapters/postgres"
)

type providerCalendarDateTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
	TimeZone string `json:"timeZone"`
}

type providerCalendarEvent struct {
	ID          string                   `json:"id"`
	ETag        string                   `json:"etag"`
	Status      string                   `json:"status"`
	Summary     string                   `json:"summary"`
	Description string                   `json:"description"`
	Location    string                   `json:"location"`
	Start       providerCalendarDateTime `json:"start"`
	End         providerCalendarDateTime `json:"end"`
}

type providerCalendarPage struct {
	Items         []providerCalendarEvent `json:"items"`
	NextPageToken string                  `json:"nextPageToken"`
}

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
		consented, _ := input["providerWriteOptIn"].(bool)
		if !consented {
			writeError(w, http.StatusForbidden, "PROVIDER_WRITE_OPT_IN_REQUIRED", "Explicit Google Calendar write approval is required.")
			return
		}
		delete(input, "calendarId")
		delete(input, "connectionId")
		delete(input, "providerWriteOptIn")
		wrapped, err := json.Marshal(map[string]any{
			"connectionId":       connectionID,
			"calendarId":         calendarID,
			"providerWriteOptIn": true,
			"event":              input,
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

func (h *Handler) v2CalendarSync(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before using Calendar.")
		return
	}
	var input struct {
		From         string `json:"from"`
		To           string `json:"to"`
		ConnectionID string `json:"connectionId"`
		CalendarID   string `json:"calendarId"`
	}
	if err := decodeBody(r, &input, 4096); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_RANGE", "from and to are required.")
		return
	}
	from, fromErr := time.Parse(time.RFC3339, input.From)
	to, toErr := time.Parse(time.RFC3339, input.To)
	if fromErr != nil || toErr != nil || !to.After(from) || to.Sub(from) > 93*24*time.Hour {
		writeError(w, http.StatusBadRequest, "INVALID_RANGE", "Provide a valid time range of no more than 93 days.")
		return
	}
	workspaceID := r.PathValue("workspaceId")
	workspaces, err := h.store.ListV2HomeWorkspaces(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "V2_NOT_READY", "The versioned calendar is temporarily unavailable.")
		return
	}
	workspaceAllowed := false
	for _, workspace := range workspaces {
		if workspace.ID == workspaceID {
			workspaceAllowed = true
			break
		}
	}
	if !workspaceAllowed {
		writeError(w, http.StatusNotFound, "WORKSPACE_NOT_FOUND", "The selected Space is unavailable.")
		return
	}
	connectionID := input.ConnectionID
	if connectionID == "" {
		connectionID, err = h.legacyConnectionID(r, userID, "calendar")
		if err != nil {
			h.writeOperationError(w, err)
			return
		}
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
	calendarID := input.CalendarID
	calendarFound := calendarID == ""
	for _, calendar := range calendars.Items {
		if calendar.ID == calendarID {
			calendarFound = true
		}
		if calendar.Primary && calendarID == "" {
			calendarID = calendar.ID
		}
	}
	if !calendarFound {
		writeError(w, http.StatusNotFound, "CALENDAR_NOT_FOUND", "The selected Google calendar is unavailable.")
		return
	}
	if calendarID == "" && len(calendars.Items) > 0 {
		calendarID = calendars.Items[0].ID
	}
	if calendarID == "" {
		writeError(w, http.StatusNotFound, "NO_CALENDAR", "No calendar found.")
		return
	}
	if len(calendarID) > 1024 || strings.ContainsAny(calendarID, "\r\n\x00") {
		writeError(w, http.StatusBadGateway, "GOOGLE_PROVIDER_FAILED", "Google returned an invalid calendar.")
		return
	}
	events := make([]postgres.V2ProviderCalendarEvent, 0)
	deletedEventIDs := make([]string, 0)
	const maxPages = 10
	pageToken := ""
	for page := 0; page < maxPages; page++ {
		query := url.Values{
			"timeMin": {from.Format(time.RFC3339)}, "timeMax": {to.Format(time.RFC3339)},
			"singleEvents": {"true"}, "showDeleted": {"true"}, "maxResults": {"2500"},
		}
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}
		eventsResponse, requestErr := h.googleRequest(r.Context(), http.MethodGet,
			"https://www.googleapis.com/calendar/v3/calendars/"+url.PathEscape(calendarID)+"/events?"+query.Encode(), token, nil)
		if requestErr != nil {
			h.writeOperationError(w, requestErr)
			return
		}
		var result providerCalendarPage
		decodeErr := json.NewDecoder(io.LimitReader(eventsResponse.Body, 4<<20)).Decode(&result)
		eventsResponse.Body.Close()
		if decodeErr != nil {
			writeError(w, http.StatusBadGateway, "GOOGLE_PROVIDER_FAILED", "Google could not complete the request.")
			return
		}
		for _, item := range result.Items {
			if !providerIDPattern.MatchString(item.ID) {
				continue
			}
			if item.Status == "cancelled" {
				deletedEventIDs = append(deletedEventIDs, item.ID)
				continue
			}
			mapped, valid := mapGoogleCalendarEvent(item)
			if valid {
				mapped.ProviderConnectionID = connectionID
				mapped.ProviderCalendarID = calendarID
				events = append(events, mapped)
			}
		}
		pageToken = result.NextPageToken
		if pageToken == "" {
			break
		}
		if page == maxPages-1 {
			writeError(w, http.StatusUnprocessableEntity, "CALENDAR_RANGE_TOO_LARGE", "Narrow the date range and sync again.")
			return
		}
	}
	if err := h.store.ReconcileV2GoogleCalendarEvents(r.Context(), workspaceID, userID, connectionID, calendarID, events, deletedEventIDs); err != nil {
		writeError(w, http.StatusServiceUnavailable, "CALENDAR_SYNC_FAILED", "Calendar changes could not be saved.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"synced": len(events), "deleted": len(deletedEventIDs), "calendarId": calendarID, "workspaceId": workspaceID,
	})
}

func (h *Handler) v2UpdateProviderCalendarEvent(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before using Calendar.")
		return
	}
	if !h.v2WorkspaceMember(w, r, userID) {
		return
	}
	h.updateCalendarEvent(w, r)
}

func (h *Handler) v2DeleteProviderCalendarEvent(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeError(w, status, "UNAUTHORIZED", "Sign in before using Calendar.")
		return
	}
	if !h.v2WorkspaceMember(w, r, userID) {
		return
	}
	var input struct {
		ConnectionID       string `json:"connectionId"`
		CalendarID         string `json:"calendarId"`
		ProviderWriteOptIn bool   `json:"providerWriteOptIn"`
	}
	if err := decodeBody(r, &input, 4096); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_EVENT", "The calendar event request is invalid.")
		return
	}
	query := r.URL.Query()
	query.Set("connectionId", input.ConnectionID)
	query.Set("calendarId", input.CalendarID)
	if input.ProviderWriteOptIn {
		query.Set("providerWriteOptIn", "true")
	}
	r.URL.RawQuery = query.Encode()
	h.deleteCalendarEvent(w, r)
}

func (h *Handler) v2WorkspaceMember(w http.ResponseWriter, r *http.Request, userID string) bool {
	workspaces, err := h.store.ListV2HomeWorkspaces(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "V2_NOT_READY", "The versioned calendar is temporarily unavailable.")
		return false
	}
	for _, workspace := range workspaces {
		if workspace.ID == r.PathValue("workspaceId") {
			return true
		}
	}
	writeError(w, http.StatusNotFound, "WORKSPACE_NOT_FOUND", "The selected Space is unavailable.")
	return false
}

func mapGoogleCalendarEvent(item providerCalendarEvent) (postgres.V2ProviderCalendarEvent, bool) {
	title := strings.TrimSpace(item.Summary)
	if title == "" {
		title = "Untitled event"
	}
	if len(title) > 240 {
		title = title[:240]
	}
	zone := item.Start.TimeZone
	if _, err := time.LoadLocation(zone); err != nil {
		zone = "UTC"
	}
	event := postgres.V2ProviderCalendarEvent{
		ProviderEventID: item.ID, ProviderETag: item.ETag, Title: title, TimeZone: zone,
	}
	if value := strings.TrimSpace(item.Description); value != "" {
		if len(value) > 4000 {
			value = value[:4000]
		}
		event.Description = &value
	}
	if value := strings.TrimSpace(item.Location); value != "" {
		if len(value) > 500 {
			value = value[:500]
		}
		event.Location = &value
	}
	if item.Start.Date != "" {
		location, _ := time.LoadLocation(zone)
		startDate, startErr := time.Parse("2006-01-02", item.Start.Date)
		endDate, endErr := time.Parse("2006-01-02", item.End.Date)
		if startErr != nil || endErr != nil {
			return postgres.V2ProviderCalendarEvent{}, false
		}
		event.StartsAt = time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, location)
		end := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 0, 0, 0, 0, location)
		event.EndsAt, event.AllDay = &end, true
	} else {
		start, startErr := time.Parse(time.RFC3339Nano, item.Start.DateTime)
		end, endErr := time.Parse(time.RFC3339Nano, item.End.DateTime)
		if startErr != nil || endErr != nil || end.Before(start) {
			return postgres.V2ProviderCalendarEvent{}, false
		}
		event.StartsAt, event.EndsAt = start, &end
	}
	return event, true
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
	MIMEType string `json:"mimeType"`
	Filename string `json:"filename"`
	Headers  []struct {
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
	connectionID, err := h.legacyConnectionID(r, userID, "gmail.read")
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	_, token, err := h.connectionToken(r.Context(), userID, connectionID, "gmail.read")
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
	connectionID, err := h.legacyConnectionID(r, userID, "gmail.read")
	if err != nil {
		h.writeOperationError(w, err)
		return
	}
	_, token, err := h.connectionToken(r.Context(), userID, connectionID, "gmail.read")
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
