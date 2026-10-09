package google

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"askolo/backend/internal/modules/product"
)

const (
	sharedGmailPageSize        = 20
	sharedGmailListBodyLimit   = 512 * 1024
	sharedGmailMetaBodyLimit   = 64 * 1024
	sharedGmailDetailBodyLimit = 2 * 1024 * 1024
	sharedGmailTextLimit       = 1024 * 1024
)

var (
	sharedGmailMessageIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,200}$`)
	htmlScriptBlockPattern      = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`)
	htmlStyleBlockPattern       = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style\s*>`)
	htmlLineBreakPattern        = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|li|h[1-6])\s*>`)
	htmlTagPattern              = regexp.MustCompile(`(?s)<[^>]+>`)
)

func (h *Handler) ListSharedGmailMessages(
	ctx context.Context,
	ownerUserID, connectionID, pageToken string,
) (product.SharedEmailPage, error) {
	_, token, err := h.connectionToken(ctx, ownerUserID, connectionID, "gmail.read")
	if err != nil {
		return product.SharedEmailPage{}, err
	}
	query := url.Values{}
	query.Set("maxResults", strconv.Itoa(sharedGmailPageSize))
	query.Add("labelIds", "INBOX")
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	endpoint := "https://gmail.googleapis.com/gmail/v1/users/me/messages?" + query.Encode()
	response, err := h.googleRequest(ctx, http.MethodGet, endpoint, token, nil)
	if err != nil {
		return product.SharedEmailPage{}, err
	}
	defer response.Body.Close()
	var listed struct {
		NextPageToken string `json:"nextPageToken"`
		Messages      []struct {
			ID string `json:"id"`
		} `json:"messages"`
	}
	if err := decodeSharedGmailJSON(response, sharedGmailListBodyLimit, &listed); err != nil {
		return product.SharedEmailPage{}, err
	}
	if len(listed.Messages) > sharedGmailPageSize || len(listed.NextPageToken) > 1024 {
		return product.SharedEmailPage{}, errors.New("Gmail returned an invalid page")
	}

	messageIDs := make([]string, len(listed.Messages))
	for i, item := range listed.Messages {
		if !sharedGmailMessageIDPattern.MatchString(item.ID) {
			return product.SharedEmailPage{}, errors.New("Gmail returned an invalid message identifier")
		}
		messageIDs[i] = item.ID
	}
	messages, err := h.fetchSharedGmailMetadata(ctx, token, messageIDs)
	if err != nil {
		return product.SharedEmailPage{}, err
	}
	return product.SharedEmailPage{Messages: messages, NextPageToken: listed.NextPageToken}, nil
}

func (h *Handler) fetchSharedGmailMetadata(
	ctx context.Context,
	token string,
	messageIDs []string,
) ([]product.SharedEmailMessage, error) {
	messages := make([]product.SharedEmailMessage, len(messageIDs))
	if len(messageIDs) == 0 {
		return messages, nil
	}
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	semaphore := make(chan struct{}, 5)
	var wait sync.WaitGroup
	var once sync.Once
	var firstErr error
	for i, messageID := range messageIDs {
		wait.Add(1)
		go func(index int, id string) {
			defer wait.Done()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-requestCtx.Done():
				return
			}
			message, fetchErr := h.fetchSharedGmailMessage(requestCtx, token, id, false)
			if fetchErr != nil {
				once.Do(func() {
					firstErr = fetchErr
					cancel()
				})
				return
			}
			messages[index] = toSharedEmailMessage(message, false)
		}(i, messageID)
	}
	wait.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return messages, nil
}

func (h *Handler) GetSharedGmailMessage(
	ctx context.Context,
	ownerUserID, connectionID, messageID string,
) (product.SharedEmailMessage, error) {
	if !sharedGmailMessageIDPattern.MatchString(messageID) {
		return product.SharedEmailMessage{}, errors.New("invalid Gmail message identifier")
	}
	_, token, err := h.connectionToken(ctx, ownerUserID, connectionID, "gmail.read")
	if err != nil {
		return product.SharedEmailMessage{}, err
	}
	message, err := h.fetchSharedGmailMessage(ctx, token, messageID, true)
	if err != nil {
		return product.SharedEmailMessage{}, err
	}
	if message.ID != messageID {
		return product.SharedEmailMessage{}, errors.New("Gmail returned a different message identifier")
	}
	result := toSharedEmailMessage(message, true)
	if len(result.BodyText) > sharedGmailTextLimit {
		return product.SharedEmailMessage{}, errors.New("Gmail message text exceeds the supported limit")
	}
	return result, nil
}

func (h *Handler) fetchSharedGmailMessage(
	ctx context.Context,
	token, messageID string,
	includeBody bool,
) (gmailMessage, error) {
	if !sharedGmailMessageIDPattern.MatchString(messageID) {
		return gmailMessage{}, errors.New("invalid Gmail message identifier")
	}
	query := url.Values{}
	if includeBody {
		query.Set("format", "full")
	} else {
		query.Set("format", "metadata")
		query.Add("metadataHeaders", "Subject")
		query.Add("metadataHeaders", "From")
		query.Add("metadataHeaders", "Date")
	}
	endpoint := "https://gmail.googleapis.com/gmail/v1/users/me/messages/" +
		url.PathEscape(messageID) + "?" + query.Encode()
	response, err := h.googleRequest(ctx, http.MethodGet, endpoint, token, nil)
	if err != nil {
		return gmailMessage{}, err
	}
	defer response.Body.Close()
	limit := int64(sharedGmailMetaBodyLimit)
	if includeBody {
		limit = sharedGmailDetailBodyLimit
	}
	var message gmailMessage
	if err := decodeSharedGmailJSON(response, limit, &message); err != nil {
		return gmailMessage{}, err
	}
	return message, nil
}

func decodeSharedGmailJSON(response *http.Response, limit int64, target any) error {
	if response == nil || response.Body == nil {
		return errors.New("Gmail returned an empty response")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return errors.New("Gmail response could not be read")
	}
	if int64(len(body)) > limit {
		return errors.New("Gmail response exceeds the supported limit")
	}
	if err := json.Unmarshal(body, target); err != nil {
		return errors.New("Gmail returned an invalid response")
	}
	return nil
}

func toSharedEmailMessage(message gmailMessage, includeBody bool) product.SharedEmailMessage {
	result := product.SharedEmailMessage{
		ID: message.ID, ThreadID: message.ThreadID,
		Subject: gmailHeader(message, "Subject"), From: gmailHeader(message, "From"),
		Snippet: message.Snippet, LabelIDs: append([]string(nil), message.LabelIDs...),
	}
	if millis, err := strconv.ParseInt(message.InternalDate, 10, 64); err == nil && millis > 0 {
		result.Date = time.UnixMilli(millis).UTC().Format(time.RFC3339)
	} else {
		result.Date = gmailHeader(message, "Date")
	}
	if includeBody {
		result.BodyText = gmailTextBody(message.Payload)
	}
	return result
}

func gmailTextBody(payload gmailPayload) string {
	var plainText, htmlText string
	var visit func(gmailPayload)
	visit = func(current gmailPayload) {
		if current.Filename != "" {
			return
		}
		if current.Body.Data != "" {
			decoded, err := base64.RawURLEncoding.DecodeString(current.Body.Data)
			if err == nil {
				switch strings.ToLower(current.MIMEType) {
				case "text/plain":
					if plainText == "" {
						plainText = string(decoded)
					}
				case "text/html":
					if htmlText == "" {
						htmlText = string(decoded)
					}
				}
			}
		}
		for _, part := range current.Parts {
			visit(part)
		}
	}
	visit(payload)
	if plainText != "" {
		return plainText
	}
	return htmlMailToText(htmlText)
}

func htmlMailToText(value string) string {
	value = htmlScriptBlockPattern.ReplaceAllString(value, "")
	value = htmlStyleBlockPattern.ReplaceAllString(value, "")
	value = htmlLineBreakPattern.ReplaceAllString(value, "\n")
	value = htmlTagPattern.ReplaceAllString(value, "")
	return strings.TrimSpace(html.UnescapeString(value))
}
