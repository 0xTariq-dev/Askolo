package postgres

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseSessionMFAStateTreatsLegacySessionsAsVerified(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"userId":   "user-1",
		"provider": "password",
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := parseSessionMFAState(payload)
	if err != nil {
		t.Fatalf("parseSessionMFAState() error = %v", err)
	}
	if state.UserID != "user-1" || !state.Verified || state.Required {
		t.Fatalf("legacy state = %+v, want verified non-MFA state", state)
	}
}

func TestParseSessionMFAStateReadsPendingChallenge(t *testing.T) {
	expiresAt := time.Now().UTC().Add(5 * time.Minute).Truncate(time.Second)
	payload, err := json.Marshal(map[string]any{
		"userId":                "user-1",
		"mfaRequired":           true,
		"mfaVerified":           false,
		"mfaAttempts":           2,
		"mfaChallengeExpiresAt": expiresAt.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := parseSessionMFAState(payload)
	if err != nil {
		t.Fatalf("parseSessionMFAState() error = %v", err)
	}
	if state.UserID != "user-1" || !state.Required || state.Verified || state.Attempts != 2 {
		t.Fatalf("pending state = %+v", state)
	}
	if !state.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("pending expiry = %v, want %v", state.ExpiresAt, expiresAt)
	}
}

func TestSessionWithMFACompletedPreservesSessionIdentity(t *testing.T) {
	payload, err := json.Marshal(map[string]any{
		"userId":                "user-1",
		"provider":              "google",
		"mfaRequired":           true,
		"mfaVerified":           false,
		"mfaAttempts":           1,
		"mfaChallengeExpiresAt": time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := sessionWithMFACompleted(payload)
	if err != nil {
		t.Fatalf("sessionWithMFACompleted() error = %v", err)
	}
	state, err := parseSessionMFAState(completed)
	if err != nil {
		t.Fatalf("parseSessionMFAState() error = %v", err)
	}
	if state.UserID != "user-1" || !state.Verified || state.Required || state.Attempts != 0 {
		t.Fatalf("completed state = %+v", state)
	}
}
