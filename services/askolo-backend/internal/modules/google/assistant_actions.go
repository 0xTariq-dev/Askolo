package google

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/mail"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"askolo/backend/internal/adapters/postgres"
)

const assistantProviderResponseLimit = 64 * 1024

type assistantGoogleIntent struct {
	Tool         string `json:"tool"`
	To           string `json:"to"`
	Subject      string `json:"subject"`
	Body         string `json:"body"`
	Title        string `json:"title"`
	Start        string `json:"start"`
	End          string `json:"end"`
	ConnectionID string `json:"connectionId"`
	CalendarID   string `json:"calendarId"`
}

func (h *Handler) ResolveAssistantActionTarget(
	ctx context.Context,
	userID string,
	toolName string,
) (string, string, error) {
	if h == nil || h.store == nil || strings.TrimSpace(userID) == "" {
		return "", "", errors.New("Google account is unavailable")
	}
	service, capability := assistantGoogleService(toolName)
	if service == "" {
		return "", "", errors.New("Google action is not supported")
	}
	connections, err := h.store.ListGoogleConnections(ctx, userID)
	if err != nil {
		return "", "", errors.New("Google account status is unavailable")
	}
	defaults, err := h.store.ListProviderDefaults(ctx, userID)
	if err != nil {
		return "", "", errors.New("Google account status is unavailable")
	}
	defaultID := ""
	for _, item := range defaults {
		if item.Service == service {
			defaultID = item.ConnectionID
			break
		}
	}

	var eligible []postgres.ProviderConnection
	for _, connection := range connections {
		if connection.Status == "active" && hasString(connection.Capabilities, capability) {
			eligible = append(eligible, connection)
		}
	}
	if defaultID != "" {
		for _, connection := range eligible {
			if connection.ID == defaultID {
				return connection.ID, connection.Email, nil
			}
		}
		return "", "", errors.New("the selected Google account is unavailable")
	}
	if len(eligible) != 1 {
		return "", "", errors.New("select one Google account before using this action")
	}
	return eligible[0].ID, eligible[0].Email, nil
}

func assistantGoogleService(toolName string) (string, string) {
	switch toolName {
	case "send_gmail":
		return "gmail", "gmail.send"
	case "create_calendar_event":
		return "calendar", "calendar"
	default:
		return "", ""
	}
}

func (h *Handler) ExecuteAssistantAction(
	ctx context.Context,
	userID string,
	runID string,
	toolName string,
	rawIntent json.RawMessage,
) (json.RawMessage, bool, error) {
	if h == nil || h.store == nil || strings.TrimSpace(runID) == "" {
		return nil, false, errors.New("Google action is unavailable")
	}
	var intent assistantGoogleIntent
	if err := json.Unmarshal(rawIntent, &intent); err != nil || intent.Tool != toolName || intent.ConnectionID == "" {
		return nil, false, errors.New("Google action intent is invalid")
	}
	switch toolName {
	case "send_gmail":
		return h.executeAssistantGmail(ctx, userID, intent)
	case "create_calendar_event":
		return h.executeAssistantCalendarEvent(ctx, userID, intent)
	default:
		return nil, false, errors.New("Google action is not supported")
	}
}

func (h *Handler) executeAssistantGmail(
	ctx context.Context,
	userID string,
	intent assistantGoogleIntent,
) (json.RawMessage, bool, error) {
	to := strings.TrimSpace(intent.To)
	subject := strings.TrimSpace(intent.Subject)
	if to == "" || len([]byte(to)) > 254 || strings.ContainsAny(to, "\r\n") ||
		subject == "" || len([]byte(subject)) > 200 || strings.ContainsAny(subject, "\r\n") ||
		len([]byte(intent.Body)) == 0 || len([]byte(intent.Body)) > 4000 ||
		strings.TrimSpace(intent.Body) == "" || !utf8.ValidString(to) ||
		!utf8.ValidString(subject) || !utf8.ValidString(intent.Body) {
		return nil, false, errors.New("email fields are invalid")
	}
	for _, character := range intent.Body {
		if character == 0 || (character < 0x20 && character != '\n' && character != '\r' && character != '\t') {
			return nil, false, errors.New("email body is invalid")
		}
	}
	address, err := mail.ParseAddress(to)
	if err != nil || address.Address != to {
		return nil, false, errors.New("email recipient is invalid")
	}
	_, token, err := h.connectionToken(ctx, userID, intent.ConnectionID, "gmail.send")
	if err != nil {
		return nil, false, err
	}
	raw := "To: " + address.Address + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" + intent.Body
	payload, err := json.Marshal(map[string]string{
		"raw": base64.RawURLEncoding.EncodeToString([]byte(raw)),
	})
	if err != nil {
		return nil, false, errors.New("email request could not be prepared")
	}
	response, err := h.googleRequest(ctx, "POST",
		"https://gmail.googleapis.com/gmail/v1/users/me/messages/send", token, payload)
	if err != nil {
		return nil, true, err
	}
	defer response.Body.Close()
	var result struct {
		ID       string `json:"id"`
		ThreadID string `json:"threadId"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, assistantProviderResponseLimit)).Decode(&result); err != nil ||
		result.ID == "" || len(result.ID) > 256 {
		return nil, true, errors.New("Google could not verify the email result")
	}
	safeResult, err := json.Marshal(map[string]string{"resourceId": result.ID, "threadId": result.ThreadID})
	if err != nil {
		return nil, true, errors.New("Google result could not be prepared")
	}
	return safeResult, true, nil
}

func (h *Handler) executeAssistantCalendarEvent(
	ctx context.Context,
	userID string,
	intent assistantGoogleIntent,
) (json.RawMessage, bool, error) {
	if intent.CalendarID != "primary" || len([]byte(intent.Title)) == 0 ||
		len([]byte(intent.Title)) > 120 || !utf8.ValidString(intent.Title) {
		return nil, false, errors.New("calendar event fields are invalid")
	}
	start, startErr := time.Parse(time.RFC3339, intent.Start)
	end, endErr := time.Parse(time.RFC3339, intent.End)
	if startErr != nil || endErr != nil || !end.After(start) || end.Sub(start) > 72*time.Hour ||
		start.Before(time.Now().Add(-5*time.Minute)) || start.After(time.Now().AddDate(2, 0, 0)) {
		return nil, false, errors.New("calendar event time is invalid")
	}
	_, token, err := h.connectionToken(ctx, userID, intent.ConnectionID, "calendar")
	if err != nil {
		return nil, false, err
	}
	event, err := json.Marshal(map[string]any{
		"summary": intent.Title,
		"start":   map[string]string{"dateTime": start.Format(time.RFC3339)},
		"end":     map[string]string{"dateTime": end.Format(time.RFC3339)},
	})
	if err != nil {
		return nil, false, errors.New("calendar event could not be prepared")
	}
	endpoint := "https://www.googleapis.com/calendar/v3/calendars/" + url.PathEscape(intent.CalendarID) + "/events"
	response, err := h.googleRequest(ctx, "POST", endpoint, token, event)
	if err != nil {
		return nil, true, err
	}
	defer response.Body.Close()
	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, assistantProviderResponseLimit)).Decode(&result); err != nil ||
		result.ID == "" || len(result.ID) > 256 {
		return nil, true, errors.New("Google could not verify the calendar event")
	}
	safeResult, err := json.Marshal(map[string]string{
		"resourceId": result.ID,
		"title":      intent.Title,
		"start":      start.Format(time.RFC3339),
		"end":        end.Format(time.RFC3339),
	})
	if err != nil {
		return nil, true, errors.New("Google result could not be prepared")
	}
	return safeResult, true, nil
}
