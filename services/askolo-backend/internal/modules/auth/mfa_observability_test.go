package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"askolo/backend/internal/adapters/postgres"
	"askolo/backend/internal/config"
)

func TestMFAAlertReasonsUseBoundedThresholds(t *testing.T) {
	tests := []struct {
		name    string
		summary postgres.MFAEventSummary
		want    string
	}{
		{
			name:    "below thresholds",
			summary: postgres.MFAEventSummary{FailureEvents: 19, ReplayEvents: 4, LockoutEvents: 4, DecryptionFailureEvents: 2},
		},
		{
			name:    "failure threshold",
			summary: postgres.MFAEventSummary{FailureEvents: 20},
			want:    "failure_events",
		},
		{
			name: "all thresholds",
			summary: postgres.MFAEventSummary{
				FailureEvents:                            20,
				ReplayEvents:                             5,
				LockoutEvents:                            5,
				DecryptionFailureEvents:                  3,
				RecoverySupportRequests:                  20,
				RecoverySupportVerificationFailures:      5,
				RecoverySupportRateLimited:               5,
				RecoverySupportSessionRevocationFailures: 1,
			},
			want: "failure_events,replay_events,lockout_events,decryption_failure_events,recovery_support_request_spike,recovery_support_verification_failures,recovery_support_rate_limited,recovery_support_session_revocation_failures",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reasons := strings.Join(mfaAlertReasons(test.summary), ",")
			if reasons != test.want {
				t.Fatalf("alert reasons = %q, want %q", reasons, test.want)
			}
		})
	}
}

func TestMFAAlertLogIsDeduplicatedWithoutUserIdentifiers(t *testing.T) {
	var logs bytes.Buffer
	handler := NewHandler(
		config.Config{Environment: "staging"},
		nil,
		slog.New(slog.NewTextHandler(&logs, nil)),
	)
	signal := MFASecurityReadiness{
		Environment:   "staging",
		WindowMinutes: 15,
		FailureEvents: 20,
		AffectedUsers: 4,
		AlertReasons:  []string{"failure_events"},
		Alert:         true,
	}

	handler.logMFAAlert(signal)
	handler.logMFAAlert(signal)

	if got := strings.Count(logs.String(), "MFA verification failure spike"); got != 1 {
		t.Fatalf("alert log count = %d, want 1; logs=%q", got, logs.String())
	}
	if strings.Contains(logs.String(), "user_id") || strings.Contains(logs.String(), "request_id") {
		t.Fatalf("alert log exposed an individual identifier: %q", logs.String())
	}
}

func TestMFASecurityReadinessReportsUnavailableWithoutStore(t *testing.T) {
	signal := NewHandler(config.Config{Environment: "development"}, nil, nil).
		MFASecurityReadiness(t.Context())

	if signal.Status != "unavailable" || signal.Environment != "development" || signal.Alert {
		t.Fatalf("unavailable MFA signal = %+v", signal)
	}
	if signal.WindowMinutes != 15 {
		t.Fatalf("window minutes = %d, want 15", signal.WindowMinutes)
	}
}

func TestMFAEventSummaryAggregatesConcurrentTraffic(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	since := now.Add(-15 * time.Minute)

	staleEvents := []struct {
		id        string
		eventType string
	}{
		{id: "stale-mfa-failure", eventType: "mfa_challenge_failed"},
		{id: "stale-mfa-replay", eventType: "mfa_replay_rejected"},
		{id: "stale-mfa-lockout", eventType: "mfa_challenge_locked"},
		{id: "stale-mfa-decryption", eventType: "mfa_decryption_failed"},
		{id: "stale-auth-event", eventType: "login_failed"},
	}
	for _, event := range staleEvents {
		if _, err := fixture.pool.Exec(ctx, `
			INSERT INTO auth_security_events
				(id, user_id, event_type, request_id, metadata, created_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		`, event.id, "stale-mfa-user", event.eventType, "stale-mfa-request",
			`{"marker":"stale-event-metadata"}`, now.Add(-16*time.Minute)); err != nil {
			t.Fatalf("insert stale %s event: %v", event.eventType, err)
		}
	}

	failureTypes := []string{
		"mfa_challenge_failed",
		"mfa_recovery_code_failed",
		"mfa_enrollment_failed",
		"mfa_disable_failed",
		"recovery_code_regeneration_failed",
	}
	events := make([]struct {
		eventType string
		userID    string
	}, 0, 41)
	for i := 0; i < 20; i++ {
		events = append(events, struct {
			eventType string
			userID    string
		}{
			eventType: failureTypes[i%len(failureTypes)],
			userID:    fmt.Sprintf("mfa-user-%d", i%4),
		})
	}
	for i := 0; i < 5; i++ {
		events = append(events, struct {
			eventType string
			userID    string
		}{eventType: "mfa_replay_rejected", userID: fmt.Sprintf("mfa-user-%d", i%4)})
		events = append(events, struct {
			eventType string
			userID    string
		}{eventType: "mfa_challenge_locked", userID: fmt.Sprintf("mfa-user-%d", i%4)})
	}
	for i := 0; i < 3; i++ {
		events = append(events, struct {
			eventType string
			userID    string
		}{eventType: "mfa_decryption_failed", userID: fmt.Sprintf("mfa-user-%d", i%4)})
	}
	for i := 0; i < 8; i++ {
		events = append(events, struct {
			eventType string
			userID    string
		}{eventType: []string{"login_failed", "password_reset_requested"}[i%2], userID: fmt.Sprintf("auth-user-%d", i)})
	}

	start := make(chan struct{})
	writeErrors := make(chan error, len(events))
	var writers sync.WaitGroup
	for i, event := range events {
		writers.Add(1)
		go func(index int, eventType, userID string) {
			defer writers.Done()
			<-start
			writeErrors <- fixture.store.CreateSecurityEvent(
				ctx,
				userID,
				eventType,
				fmt.Sprintf("mfa-request-%d", index),
				map[string]any{
					"marker":        "event-metadata-must-not-escape",
					"submittedCode": "123456",
				},
			)
		}(i, event.eventType, event.userID)
	}

	readErrors := make(chan error, 12)
	var readers sync.WaitGroup
	for i := 0; i < 12; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			<-start
			_, err := fixture.store.MFAEventSummary(ctx, since)
			readErrors <- err
		}()
	}
	close(start)
	writers.Wait()
	readers.Wait()
	close(writeErrors)
	close(readErrors)
	for err := range writeErrors {
		if err != nil {
			t.Fatalf("concurrent security event insert: %v", err)
		}
	}
	for err := range readErrors {
		if err != nil {
			t.Fatalf("concurrent MFA summary read: %v", err)
		}
	}

	summary, err := fixture.store.MFAEventSummary(ctx, since)
	if err != nil {
		t.Fatalf("read MFA event summary: %v", err)
	}
	want := postgres.MFAEventSummary{
		FailureEvents:           20,
		ReplayEvents:            5,
		LockoutEvents:           5,
		DecryptionFailureEvents: 3,
		AffectedUsers:           4,
	}
	if summary != want {
		t.Fatalf("MFA event summary = %+v, want %+v", summary, want)
	}

	handler := NewHandler(config.Config{Environment: "test"}, fixture.store, slog.Default())
	signal := handler.MFASecurityReadiness(ctx)
	if signal.Status != "available" ||
		signal.FailureEvents != 20 ||
		signal.ReplayEvents != 5 ||
		signal.LockoutEvents != 5 ||
		signal.DecryptionFailureEvents != 3 ||
		signal.AffectedUsers != 4 {
		t.Fatalf("MFA security signal = %+v, want all four thresholds and four affected users", signal)
	}
	if got := strings.Join(signal.AlertReasons, ","); got != "failure_events,replay_events,lockout_events,decryption_failure_events" {
		t.Fatalf("MFA alert reasons = %q, want all four threshold alerts", got)
	}

	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal MFA event summary: %v", err)
	}
	var values map[string]any
	if err := json.Unmarshal(encoded, &values); err != nil {
		t.Fatalf("decode MFA event summary: %v", err)
	}
	wantFields := map[string]int64{
		"FailureEvents":                            20,
		"ReplayEvents":                             5,
		"LockoutEvents":                            5,
		"DecryptionFailureEvents":                  3,
		"RecoverySupportRequests":                  0,
		"RecoverySupportVerificationFailures":      0,
		"RecoverySupportRateLimited":               0,
		"RecoverySupportSessionRevocationFailures": 0,
		"AffectedUsers":                            4,
	}
	if len(values) != len(wantFields) {
		t.Fatalf("MFA event summary exposed %d fields, want %d: %s", len(values), len(wantFields), encoded)
	}
	for field, expected := range wantFields {
		value, ok := values[field].(float64)
		if !ok || int64(value) != expected {
			t.Fatalf("MFA event summary field %q = %#v, want numeric %d", field, values[field], expected)
		}
	}
	for field := range values {
		if _, ok := wantFields[field]; !ok {
			t.Fatalf("MFA event summary exposed unexpected field %q", field)
		}
	}
	encodedSummary := string(encoded)
	for _, secret := range []string{
		"stale-mfa-user",
		"stale-mfa-request",
		"stale-event-metadata",
		"mfa-user-0",
		"mfa-request-0",
		"event-metadata-must-not-escape",
		"123456",
	} {
		if strings.Contains(encodedSummary, secret) {
			t.Fatalf("MFA event summary exposed event metadata or identifier %q: %s", secret, encodedSummary)
		}
	}
}
