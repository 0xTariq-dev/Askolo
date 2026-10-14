package google

import (
	"strconv"
	"testing"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
)

func TestHomeEmailThreadStateContainsOnlyOpaqueWindowMetadata(t *testing.T) {
	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	handler := &Handler{cfg: config.Config{AuthRateLimitHMACSecret: "test-auth-hmac-secret-for-home-email-state-32-bytes"}}
	target := postgres.V2HomeEmailSyncTarget{
		ConnectionID: "google-connection-1",
		OwnerEmail:   "Owner@example.com",
	}
	thread := gmailSummaryThread{Messages: []gmailMessage{
		{
			ID: "older", ThreadID: "raw-thread-id", InternalDate: strconv.FormatInt(cutoff.Add(-time.Hour).UnixMilli(), 10),
			LabelIDs: []string{"INBOX", "UNREAD"},
			Payload: gmailPayload{Headers: []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			}{{Name: "From", Value: "other@example.com"}}},
		},
		{
			ID: "latest", ThreadID: "raw-thread-id", InternalDate: strconv.FormatInt(cutoff.Add(time.Hour).UnixMilli(), 10),
			LabelIDs: []string{"INBOX", "UNREAD", "STARRED"},
			Payload: gmailPayload{Headers: []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			}{{Name: "From", Value: "Owner@example.com"}}},
		},
	}}

	state, keep, err := handler.homeEmailThreadState(target, "raw-thread-id", thread, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if !keep || state.UnreadMessageCount != 1 || !state.InInboxWindow ||
		!state.LatestSenderIsOwner || !state.FlaggedInWindow {
		t.Fatalf("thread state = %+v, keep=%v", state, keep)
	}
	if state.Token == "" || state.Token == "raw-thread-id" || len(state.Token) != 64 {
		t.Fatalf("thread token is not an opaque SHA-256 HMAC: %q", state.Token)
	}
	if state.LastActivityAt.Before(cutoff) {
		t.Fatalf("last activity %s precedes cutoff %s", state.LastActivityAt, cutoff)
	}
}
