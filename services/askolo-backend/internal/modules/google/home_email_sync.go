package google

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"askolo/backend/internal/adapters/postgres"
)

const (
	homeEmailSyncInterval = time.Hour
	homeEmailSyncLimit    = 250
)

type gmailThreadList struct {
	Threads []struct {
		ID string `json:"id"`
	} `json:"threads"`
	NextPageToken string `json:"nextPageToken"`
}

type gmailSummaryThread struct {
	Messages []gmailMessage `json:"messages"`
}

type gmailSummaryMessageList struct {
	Messages []gmailMessage `json:"messages"`
	NextPage string         `json:"nextPageToken"`
}

// RunHomeEmailSummarySync runs a bounded initial sync and then refreshes
// eligible summaries hourly. It is tied to the API process lifecycle.
func (h *Handler) RunHomeEmailSummarySync(ctx context.Context) {
	if h == nil || h.store == nil {
		return
	}
	h.syncHomeEmailSummaries(ctx)
	ticker := time.NewTicker(homeEmailSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.syncHomeEmailSummaries(ctx)
		}
	}
}

func (h *Handler) syncHomeEmailSummaries(ctx context.Context) {
	targets, err := h.store.ListV2HomeEmailSyncTargets(ctx, homeEmailSyncLimit)
	if err != nil {
		if ctx.Err() == nil {
			h.logger.Error("Home Gmail summary target query failed", "operation", "home_gmail_summary_sync")
		}
		return
	}
	startedAt := time.Now()
	synced := 0
	for _, target := range targets {
		if ctx.Err() != nil {
			return
		}
		claimed, err := h.store.ClaimV2HomeEmailSummarySync(ctx, target)
		if err != nil {
			h.logger.Warn("Home Gmail summary claim failed",
				"operation", "home_gmail_summary_sync",
				"error_type", fmt.Sprintf("%T", err),
			)
			continue
		}
		if !claimed {
			continue
		}
		syncCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		err = h.syncHomeEmailSummary(syncCtx, target)
		cancel()
		if err != nil {
			errorCtx, errorCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = h.store.MarkV2HomeEmailSummaryError(errorCtx, target)
			errorCancel()
			h.logger.Warn("Home Gmail summary sync failed",
				"operation", "home_gmail_summary_sync",
				"error_type", fmt.Sprintf("%T", err),
			)
			continue
		}
		synced++
	}
	h.logger.Info("Home Gmail summary sync pass completed",
		"operation", "home_gmail_summary_sync",
		"target_count", len(targets),
		"synced_count", synced,
		"duration_ms", time.Since(startedAt).Milliseconds(),
	)
}

func (h *Handler) syncHomeEmailSummary(ctx context.Context, target postgres.V2HomeEmailSyncTarget) error {
	if target.OwnerEmail == "" {
		return errors.New("connected account email is unavailable")
	}
	_, token, err := h.connectionToken(ctx, target.OwnerUserID, target.ConnectionID, "gmail.read")
	if err != nil {
		return err
	}
	threads, err := h.listHomeInboxThreads(ctx, token)
	if err != nil {
		return err
	}
	inboxMetadata := make([]homeEmailThreadMetadata, 0, len(threads))
	var unread int64
	for _, threadID := range threads {
		thread, err := h.fetchHomeSummaryThread(ctx, token, threadID)
		if err != nil {
			return err
		}
		metadata := homeEmailThreadMetadata{ID: threadID}
		for _, message := range thread.Messages {
			from := ""
			for _, header := range message.Payload.Headers {
				if header.Name == "From" || header.Name == "from" {
					from = header.Value
					break
				}
			}
			metadata.Messages = append(metadata.Messages, homeEmailMessageMetadata{
				ThreadID: message.ThreadID, InternalDate: message.InternalDate, From: from,
			})
			if hasString(message.LabelIDs, "INBOX") && hasString(message.LabelIDs, "UNREAD") {
				unread++
			}
		}
		inboxMetadata = append(inboxMetadata, metadata)
	}
	starred, err := h.listHomeFlaggedMessages(ctx, token, "STARRED")
	if err != nil {
		return err
	}
	important, err := h.listHomeFlaggedMessages(ctx, token, "IMPORTANT")
	if err != nil {
		return err
	}
	flagged := append(starred, important...)
	counts, err := calculateHomeEmailCounts(target.OwnerEmail, unread, inboxMetadata, flagged)
	if err != nil {
		return err
	}
	saved, err := h.store.SaveV2HomeEmailSummary(
		ctx, target, counts.UnreadInboxCount, counts.ActionNeededCount,
	)
	if err != nil {
		return err
	}
	if !saved {
		return errors.New("Home Gmail account is no longer eligible")
	}
	return nil
}

func (h *Handler) listHomeInboxThreads(ctx context.Context, token string) ([]string, error) {
	query := url.Values{}
	query.Add("labelIds", "INBOX")
	query.Set("maxResults", "500")
	var result gmailThreadList
	if err := h.getHomeGmailJSON(ctx,
		"https://gmail.googleapis.com/gmail/v1/users/me/threads?"+query.Encode(), token, &result); err != nil {
		return nil, err
	}
	if result.NextPageToken != "" || len(result.Threads) > homeEmailSyncLimit {
		return nil, errors.New("Gmail inbox exceeds the bounded Home summary sync window")
	}
	ids := make([]string, 0, len(result.Threads))
	for _, thread := range result.Threads {
		if thread.ID == "" {
			return nil, errors.New("Gmail returned an incomplete inbox thread")
		}
		ids = append(ids, thread.ID)
	}
	return ids, nil
}

func (h *Handler) fetchHomeSummaryThread(ctx context.Context, token, threadID string) (gmailSummaryThread, error) {
	if !providerIDPattern.MatchString(threadID) {
		return gmailSummaryThread{}, errors.New("Gmail returned an invalid thread identifier")
	}
	query := url.Values{}
	query.Set("format", "metadata")
	query.Add("metadataHeaders", "From")
	endpoint := "https://gmail.googleapis.com/gmail/v1/users/me/threads/" +
		url.PathEscape(threadID) + "?" + query.Encode()
	var result gmailSummaryThread
	if err := h.getHomeGmailJSON(ctx, endpoint, token, &result); err != nil {
		return gmailSummaryThread{}, err
	}
	return result, nil
}

func (h *Handler) listHomeFlaggedMessages(ctx context.Context, token, label string) ([]homeEmailMessageMetadata, error) {
	query := url.Values{}
	query.Add("labelIds", label)
	query.Set("maxResults", "500")
	var result gmailSummaryMessageList
	endpoint := "https://gmail.googleapis.com/gmail/v1/users/me/messages?" + query.Encode()
	if err := h.getHomeGmailJSON(ctx, endpoint, token, &result); err != nil {
		return nil, err
	}
	if result.NextPage != "" || len(result.Messages) > homeEmailSyncLimit {
		return nil, errors.New("Gmail flagged messages exceed the bounded Home summary sync window")
	}
	messages := make([]homeEmailMessageMetadata, 0, len(result.Messages))
	for _, message := range result.Messages {
		if message.ThreadID == "" {
			return nil, errors.New("Gmail returned a flagged message without a thread")
		}
		messages = append(messages, homeEmailMessageMetadata{
			ThreadID:  message.ThreadID,
			Starred:   label == "STARRED",
			Important: label == "IMPORTANT",
		})
	}
	return messages, nil
}

func (h *Handler) getHomeGmailJSON(ctx context.Context, endpoint, token string, target any) error {
	response, err := h.googleRequest(ctx, http.MethodGet, endpoint, token, nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if err := decodeSharedGmailJSON(response, 4<<20, target); err != nil {
		return fmt.Errorf("read Gmail summary response: %w", err)
	}
	return nil
}
