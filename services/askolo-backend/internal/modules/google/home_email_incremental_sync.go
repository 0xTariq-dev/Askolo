package google

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"askolo/backend/internal/adapters/postgres"
)

const homeEmailBaselineKeyVersion int16 = 1
const homeEmailPagesPerRun = 5

type gmailProfile struct {
	HistoryID string `json:"historyId"`
}

type gmailHistoryMessageRef struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
}

type gmailHistoryRecord struct {
	Messages      []gmailHistoryMessageRef `json:"messages"`
	MessagesAdded []struct {
		Message gmailHistoryMessageRef `json:"message"`
	} `json:"messagesAdded"`
	MessagesDeleted []struct {
		Message gmailHistoryMessageRef `json:"message"`
	} `json:"messagesDeleted"`
	LabelsAdded []struct {
		Message gmailHistoryMessageRef `json:"message"`
	} `json:"labelsAdded"`
	LabelsRemoved []struct {
		Message gmailHistoryMessageRef `json:"message"`
	} `json:"labelsRemoved"`
}

type gmailHistoryPage struct {
	HistoryID     string               `json:"historyId"`
	NextPageToken string               `json:"nextPageToken"`
	History       []gmailHistoryRecord `json:"history"`
}

func (h *Handler) syncHomeEmailSummaryIncremental(
	ctx context.Context, target postgres.V2HomeEmailSyncTarget,
) error {
	if target.OwnerEmail == "" || target.LinkedAt.IsZero() {
		return errors.New("connected Gmail account metadata is unavailable")
	}
	_, token, err := h.connectionToken(ctx, target.OwnerUserID, target.ConnectionID, "gmail.read")
	if err != nil {
		return err
	}
	checkpoint, err := h.store.LoadV2HomeEmailSyncCheckpoint(ctx, target)
	if err != nil {
		return err
	}
	keyFingerprint := homeEmailThreadKeyFingerprint(h.cfg.AuthRateLimitHMACSecret)
	if checkpoint.KeyVersion != homeEmailBaselineKeyVersion ||
		checkpoint.KeyFingerprint != keyFingerprint || checkpoint.BaselineHistoryID == "" {
		profile, err := h.getHomeGmailProfile(ctx, token)
		if err != nil {
			return err
		}
		cutoff := checkpoint.CutoffAt
		if cutoff.IsZero() || cutoff.Equal(time.Unix(0, 0).UTC()) {
			cutoff = target.LinkedAt.Add(-30 * 24 * time.Hour)
		}
		started, err := h.store.StartV2HomeEmailBaseline(
			ctx, target, profile.HistoryID, cutoff, homeEmailBaselineKeyVersion,
			keyFingerprint,
		)
		if err != nil {
			return err
		}
		if !started {
			return errors.New("Home Gmail account is no longer eligible")
		}
		checkpoint, err = h.store.LoadV2HomeEmailSyncCheckpoint(ctx, target)
		if err != nil {
			return err
		}
	}

	pages := 0
	for checkpoint.Phase == "inbox" && pages < homeEmailPagesPerRun {
		page, err := h.listHomeSummaryThreadPage(
			ctx, token, checkpoint.CutoffAt, checkpoint.BaselinePageToken,
		)
		if err != nil {
			return err
		}
		states := make([]postgres.V2HomeEmailThreadState, 0, len(page.Threads))
		for _, item := range page.Threads {
			if item.ID == "" || !providerIDPattern.MatchString(item.ID) {
				return errors.New("Gmail returned an invalid thread identifier")
			}
			thread, err := h.fetchHomeSummaryThread(ctx, token, item.ID)
			if err != nil {
				return err
			}
			state, keep, err := h.homeEmailThreadState(target, item.ID, thread, checkpoint.CutoffAt)
			if err != nil {
				return err
			}
			if keep {
				states = append(states, state)
			}
		}
		saved, err := h.store.SaveV2HomeEmailBaselinePage(ctx, target, states, page.NextPageToken)
		if err != nil {
			return err
		}
		if !saved {
			return errors.New("Home Gmail account is no longer eligible")
		}
		checkpoint, err = h.store.LoadV2HomeEmailSyncCheckpoint(ctx, target)
		if err != nil {
			return err
		}
		pages++
	}
	if checkpoint.Phase != "history" && checkpoint.Phase != "complete" {
		return nil
	}
	if checkpoint.HistoryID == "" && checkpoint.Phase == "complete" {
		return errors.New("Home Gmail history cursor is missing")
	}

	startHistoryID := checkpoint.HistoryID
	baselineHistory := checkpoint.Phase == "history"
	if baselineHistory {
		startHistoryID = checkpoint.BaselineHistoryID
	}
	pageToken := checkpoint.HistoryPageToken
	for pages < homeEmailPagesPerRun {
		history, err := h.getHomeGmailHistoryPage(ctx, token, startHistoryID, pageToken)
		if err != nil {
			if !isExpiredHomeGmailHistoryCursor(err) {
				return err
			}
			profile, profileErr := h.getHomeGmailProfile(ctx, token)
			if profileErr != nil {
				return profileErr
			}
			started, startErr := h.store.StartV2HomeEmailBaseline(
				ctx, target, profile.HistoryID, checkpoint.CutoffAt, homeEmailBaselineKeyVersion,
				keyFingerprint,
			)
			if startErr != nil {
				return startErr
			}
			if !started {
				return errors.New("Home Gmail account is no longer eligible")
			}
			return h.syncHomeEmailSummaryIncremental(ctx, target)
		}
		threadIDs := history.threadIDs()
		states := make([]postgres.V2HomeEmailThreadState, 0, len(threadIDs))
		removeTokens := make([]string, 0)
		for _, threadID := range threadIDs {
			if !providerIDPattern.MatchString(threadID) {
				return errors.New("Gmail history contains an invalid thread identifier")
			}
			thread, fetchErr := h.fetchHomeSummaryThread(ctx, token, threadID)
			if fetchErr != nil {
				var provider operationError
				if errors.As(fetchErr, &provider) && provider.status == http.StatusNotFound {
					removeTokens = append(removeTokens, homeEmailThreadToken(
						h.cfg.AuthRateLimitHMACSecret, target.ConnectionID, threadID,
					))
					continue
				}
				return fetchErr
			}
			state, keep, stateErr := h.homeEmailThreadState(target, threadID, thread, checkpoint.CutoffAt)
			if stateErr != nil {
				return stateErr
			}
			if keep {
				states = append(states, state)
			} else {
				removeTokens = append(removeTokens, homeEmailThreadToken(
					h.cfg.AuthRateLimitHMACSecret, target.ConnectionID, threadID,
				))
			}
		}
		saved, err := h.store.SaveV2HomeEmailHistoryPage(
			ctx, target, states, removeTokens, history.NextPageToken, history.HistoryID, baselineHistory,
		)
		if err != nil {
			return err
		}
		if !saved {
			return errors.New("Home Gmail account is no longer eligible")
		}
		pages++
		if history.NextPageToken == "" {
			return nil
		}
		pageToken = history.NextPageToken
	}
	return nil
}

func (h *Handler) getHomeGmailProfile(ctx context.Context, token string) (gmailProfile, error) {
	var profile gmailProfile
	if err := h.getHomeGmailJSON(
		ctx, "https://gmail.googleapis.com/gmail/v1/users/me/profile", token, &profile,
	); err != nil {
		return gmailProfile{}, err
	}
	if !validGmailHistoryID(profile.HistoryID) {
		return gmailProfile{}, errors.New("Gmail returned no history cursor")
	}
	return profile, nil
}

func (h *Handler) listHomeSummaryThreadPage(
	ctx context.Context, token string, cutoff time.Time, pageToken string,
) (gmailThreadList, error) {
	if cutoff.IsZero() {
		return gmailThreadList{}, errors.New("Home Gmail baseline cutoff is missing")
	}
	if len(pageToken) > 4096 {
		return gmailThreadList{}, errors.New("Gmail returned an invalid page token")
	}
	query := url.Values{}
	query.Set("q", fmt.Sprintf("after:%d {in:inbox is:starred is:important}", cutoff.Unix()))
	query.Set("maxResults", "500")
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	var result gmailThreadList
	endpoint := "https://gmail.googleapis.com/gmail/v1/users/me/threads?" + query.Encode()
	if err := h.getHomeGmailJSON(ctx, endpoint, token, &result); err != nil {
		return gmailThreadList{}, err
	}
	if len(result.Threads) > 500 || len(result.NextPageToken) > 4096 {
		return gmailThreadList{}, errors.New("Gmail returned an oversized baseline page")
	}
	return result, nil
}

func (h *Handler) getHomeGmailHistoryPage(
	ctx context.Context, token, startHistoryID, pageToken string,
) (gmailHistoryPage, error) {
	if !validGmailHistoryID(startHistoryID) || len(pageToken) > 4096 {
		return gmailHistoryPage{}, errors.New("invalid Gmail history checkpoint")
	}
	query := url.Values{}
	query.Set("startHistoryId", startHistoryID)
	query.Set("maxResults", "500")
	query.Set("historyTypes", "messageAdded")
	query.Add("historyTypes", "messageDeleted")
	query.Add("historyTypes", "labelAdded")
	query.Add("historyTypes", "labelRemoved")
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	var result gmailHistoryPage
	endpoint := "https://gmail.googleapis.com/gmail/v1/users/me/history?" + query.Encode()
	if err := h.getHomeGmailJSON(ctx, endpoint, token, &result); err != nil {
		return gmailHistoryPage{}, err
	}
	if !validGmailHistoryID(result.HistoryID) || len(result.NextPageToken) > 4096 {
		return gmailHistoryPage{}, errors.New("Gmail returned an incomplete history page")
	}
	return result, nil
}

func validGmailHistoryID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, char := range id {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func (page gmailHistoryPage) threadIDs() []string {
	set := make(map[string]struct{})
	add := func(message gmailHistoryMessageRef) {
		if message.ThreadID != "" {
			set[message.ThreadID] = struct{}{}
		}
	}
	for _, record := range page.History {
		for _, message := range record.Messages {
			add(message)
		}
		for _, change := range record.MessagesAdded {
			add(change.Message)
		}
		for _, change := range record.MessagesDeleted {
			add(change.Message)
		}
		for _, change := range record.LabelsAdded {
			add(change.Message)
		}
		for _, change := range record.LabelsRemoved {
			add(change.Message)
		}
	}
	result := make([]string, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	return result
}

func (h *Handler) homeEmailThreadState(
	target postgres.V2HomeEmailSyncTarget, threadID string, thread gmailSummaryThread, cutoff time.Time,
) (postgres.V2HomeEmailThreadState, bool, error) {
	owner, err := normalizeMailbox(target.OwnerEmail)
	if err != nil {
		return postgres.V2HomeEmailThreadState{}, false, errors.New("connected account email is invalid")
	}
	var state postgres.V2HomeEmailThreadState
	state.Token = homeEmailThreadToken(h.cfg.AuthRateLimitHMACSecret, target.ConnectionID, threadID)
	latestMillis := int64(-1)
	for _, message := range thread.Messages {
		millis, err := strconv.ParseInt(message.InternalDate, 10, 64)
		if err != nil || millis < 0 {
			return postgres.V2HomeEmailThreadState{}, false, errors.New("Gmail returned an invalid message date")
		}
		date := time.UnixMilli(millis).UTC()
		if date.Before(cutoff) {
			continue
		}
		if hasString(message.LabelIDs, "INBOX") {
			state.InInboxWindow = true
			if hasString(message.LabelIDs, "UNREAD") {
				state.UnreadMessageCount++
			}
		}
		if hasString(message.LabelIDs, "STARRED") || hasString(message.LabelIDs, "IMPORTANT") {
			state.FlaggedInWindow = true
		}
		if millis >= latestMillis {
			from := ""
			for _, header := range message.Payload.Headers {
				if strings.EqualFold(header.Name, "From") {
					from = header.Value
					break
				}
			}
			sender, err := normalizeMailbox(from)
			if err != nil {
				return postgres.V2HomeEmailThreadState{}, false, errors.New("Gmail returned an invalid sender")
			}
			state.LatestSenderIsOwner = sender == owner
			state.LastActivityAt = date
			latestMillis = millis
		}
	}
	if latestMillis < 0 {
		return state, false, nil
	}
	return state, state.InInboxWindow || state.FlaggedInWindow, nil
}

func isExpiredHomeGmailHistoryCursor(err error) bool {
	var provider operationError
	return errors.As(err, &provider) && provider.code == "GOOGLE_HISTORY_CURSOR_EXPIRED"
}
