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
	if !strings.Contains(logs.String(), "operation=mfa_security_spike") {
		t.Fatalf("alert log did not expose the stable operation label: %q", logs.String())
	}
}

func TestMFARecoveryLogIsBoundedAndEmittedOnce(t *testing.T) {
	var logs bytes.Buffer
	handler := NewHandler(
		config.Config{Environment: "production"},
		nil,
		slog.New(slog.NewJSONHandler(&logs, nil)),
	)
	alert := MFASecurityReadiness{
		Environment:   "production",
		WindowMinutes: 15,
		FailureEvents: 20,
		AffectedUsers: 4,
		AlertReasons:  []string{"failure_events"},
		Alert:         true,
	}
	handler.logMFAAlert(alert)
	handler.logMFAAlert(alert)
	recovered := alert
	recovered.Alert = false
	recovered.AlertReasons = []string{"failure_events"}
	recovered.FailureEvents = 0
	recovered.AffectedUsers = 0
	if !handler.resetMFAAlert() {
		t.Fatal("expected an active alert to reset")
	}
	handler.logMFARecovery(recovered)
	if handler.resetMFAAlert() {
		t.Fatal("recovery reset should be idempotent")
	}
	if got := strings.Count(logs.String(), "MFA verification failure spike recovered"); got != 1 {
		t.Fatalf("recovery log count = %d, want 1; logs=%q", got, logs.String())
	}
	recoveryLog := strings.Split(logs.String(), "\n")[1]
	if strings.Contains(recoveryLog, "user_id") || strings.Contains(recoveryLog, "request_id") ||
		!strings.Contains(recoveryLog, `"alert_reasons":"failure_events"`) {
		t.Fatalf("recovery log exposed an individual or alert-only field: %q", recoveryLog)
	}
}

func TestMFAAlertReasonChangeEmitsRecoveryForPreviousGroup(t *testing.T) {
	var logs bytes.Buffer
	handler := NewHandler(
		config.Config{Environment: "staging"},
		nil,
		slog.New(slog.NewJSONHandler(&logs, nil)),
	)
	first := MFASecurityReadiness{
		Environment:   "staging",
		WindowMinutes: 15,
		FailureEvents: 20,
		Alert:         true,
		AlertReasons:  []string{"failure_events"},
	}
	handler.logMFAAlert(first)

	second := first
	second.ReplayEvents = 5
	second.AlertReasons = []string{"failure_events", "replay_events"}
	handler.logMFAAlert(second)

	lines := strings.Split(strings.TrimSpace(logs.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("log entries = %d, want initial alert, previous-group recovery, and new alert: %q", len(lines), logs.String())
	}
	if !strings.Contains(lines[1], `"recovery":true`) ||
		!strings.Contains(lines[1], `"alert_reasons":"failure_events"`) {
		t.Fatalf("reason-set change did not recover the old group: %s", lines[1])
	}
	if !strings.Contains(lines[2], `"alert_reasons":"failure_events,replay_events"`) {
		t.Fatalf("new alert did not contain its fixed reason set: %s", lines[2])
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

func TestMFAEventSummaryStaysFastWithHistoricalSecurityEvents(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	ctx := context.Background()
	const historicalEventCount = 500_000
	const latencyBudget = 500 * time.Millisecond
	now := time.Now().UTC()
	since := now.Add(-15 * time.Minute)

	_, err := fixture.pool.Exec(ctx, `
INSERT INTO auth_security_events
  (id, user_id, event_type, request_id, metadata, created_at)
SELECT
  'historical-security-event-' || n,
  'historical-user-' || (n % 25000),
  CASE n % 10
    WHEN 0 THEN 'mfa_challenge_failed'
    WHEN 1 THEN 'mfa_replay_rejected'
    WHEN 2 THEN 'mfa_challenge_locked'
    WHEN 3 THEN 'mfa_decryption_failed'
    WHEN 4 THEN 'login_failed'
    WHEN 5 THEN 'password_reset_requested'
    WHEN 6 THEN 'login_failed'
    WHEN 7 THEN 'password_reset_requested'
    WHEN 8 THEN 'login_failed'
    ELSE 'password_reset_requested'
  END,
  'historical-request-' || n,
  '{"source":"historical-scale-check"}'::jsonb,
  $1::timestamptz - interval '16 minutes' - ((n % 94608000) * interval '1 second')
FROM generate_series(1, $2::integer) AS n
`, now, historicalEventCount)
	if err != nil {
		t.Fatalf("insert historical auth security events: %v", err)
	}

	// Keep recent events separate from historical rows so the expected rolling
	// summary is exact while the planner still sees a large, realistic history.
	for _, event := range []struct {
		id        string
		userID    string
		eventType string
	}{
		{id: "current-mfa-failure-1", userID: "current-user-1", eventType: "mfa_challenge_failed"},
		{id: "current-mfa-failure-2", userID: "current-user-2", eventType: "mfa_recovery_code_failed"},
		{id: "current-mfa-replay", userID: "current-user-1", eventType: "mfa_replay_rejected"},
	} {
		if _, err := fixture.pool.Exec(ctx, `
INSERT INTO auth_security_events
  (id, user_id, event_type, request_id, metadata, created_at)
VALUES ($1, $2, $3, $4, $5::jsonb, $6)
`, event.id, event.userID, event.eventType, "current-request-"+event.id,
			`{"marker":"private-current-event-metadata"}`, now); err != nil {
			t.Fatalf("insert recent MFA event: %v", err)
		}
	}
	if _, err := fixture.pool.Exec(ctx, `ANALYZE auth_security_events`); err != nil {
		t.Fatalf("analyze historical auth security events: %v", err)
	}

	var plan string
	err = fixture.pool.QueryRow(ctx, `
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)
SELECT COUNT(*)
FROM auth_security_events
WHERE created_at >= $1
  AND event_type IN (
    'mfa_challenge_failed',
    'mfa_recovery_code_failed',
    'mfa_enrollment_failed',
    'mfa_disable_failed',
    'recovery_code_regeneration_failed',
    'mfa_replay_rejected',
    'mfa_challenge_locked',
    'mfa_decryption_failed',
    'mfa_recovery_support_challenge_sent',
    'mfa_recovery_support_verification_failed',
    'mfa_recovery_support_rate_limited',
    'mfa_recovery_support_session_revocation_failed'
  )
`, since).Scan(&plan)
	if err != nil {
		t.Fatalf("explain rolling MFA event filter: %v", err)
	}
	if !strings.Contains(plan, "auth_security_events_type_created_idx") {
		t.Fatal("rolling MFA event filter did not use the event-type/time index")
	}

	// Warm the connection and query path before measuring steady-state reads.
	if _, err := fixture.store.MFAEventSummary(ctx, since); err != nil {
		t.Fatalf("warm rolling MFA event summary: %v", err)
	}
	for i := 0; i < 5; i++ {
		started := time.Now()
		summary, err := fixture.store.MFAEventSummary(ctx, since)
		elapsed := time.Since(started)
		if err != nil {
			t.Fatalf("read rolling MFA event summary: %v", err)
		}
		if elapsed > latencyBudget {
			t.Fatalf("rolling MFA summary took %s with %d historical events; budget is %s",
				elapsed.Round(time.Millisecond), historicalEventCount, latencyBudget)
		}
		if summary.FailureEvents != 2 || summary.ReplayEvents != 1 || summary.AffectedUsers != 2 {
			t.Fatalf("rolling MFA summary counts = %+v, want two failures, one replay, and two users", summary)
		}
	}
}

func TestMFAEventSummaryAggregatesRecoverySupportTraffic(t *testing.T) {
	fixture := newEmailAuthFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	since := now.Add(-15 * time.Minute)

	currentEvents := []struct {
		userID    string
		eventType string
		requestID string
		metadata  map[string]any
	}{
		{
			userID:    "recovery-user-1",
			eventType: "mfa_recovery_support_challenge_sent",
			requestID: "recovery-request-1",
			metadata:  map[string]any{"marker": "request-metadata"},
		},
		{
			userID:    "recovery-user-1",
			eventType: "mfa_recovery_support_challenge_sent",
			requestID: "recovery-request-2",
			metadata:  map[string]any{"marker": "request-metadata-2"},
		},
		{
			userID:    "recovery-user-1",
			eventType: "mfa_recovery_support_verification_failed",
			requestID: "recovery-verification-1",
			metadata:  map[string]any{"submittedCode": "123456"},
		},
		{
			userID:    "recovery-user-2",
			eventType: "mfa_recovery_support_verification_failed",
			requestID: "recovery-verification-2",
			metadata:  map[string]any{"submittedCode": "654321"},
		},
		{
			userID:    "recovery-user-2",
			eventType: "mfa_recovery_support_rate_limited",
			requestID: "recovery-rate-limit-1",
			metadata:  map[string]any{"clientKey": "hashed-client-key"},
		},
		{
			userID:    "recovery-user-3",
			eventType: "mfa_recovery_support_session_revocation_failed",
			requestID: "recovery-revocation-1",
			metadata:  map[string]any{"sessionID": "session-secret"},
		},
		{
			userID:    "recovery-user-4",
			eventType: "login_failed",
			requestID: "unrelated-auth-event",
			metadata:  map[string]any{"marker": "unrelated-event"},
		},
	}
	for _, event := range currentEvents {
		if err := fixture.store.CreateSecurityEvent(
			ctx,
			event.userID,
			event.eventType,
			event.requestID,
			event.metadata,
		); err != nil {
			t.Fatalf("insert current %s event: %v", event.eventType, err)
		}
	}

	staleEvents := []struct {
		id        string
		userID    string
		eventType string
	}{
		{
			id:        "stale-recovery-request",
			userID:    "stale-recovery-user-1",
			eventType: "mfa_recovery_support_challenge_sent",
		},
		{
			id:        "stale-recovery-verification",
			userID:    "stale-recovery-user-2",
			eventType: "mfa_recovery_support_verification_failed",
		},
		{
			id:        "stale-recovery-rate-limit",
			userID:    "stale-recovery-user-3",
			eventType: "mfa_recovery_support_rate_limited",
		},
		{
			id:        "stale-recovery-revocation",
			userID:    "stale-recovery-user-4",
			eventType: "mfa_recovery_support_session_revocation_failed",
		},
	}
	for _, event := range staleEvents {
		if _, err := fixture.pool.Exec(ctx, `
			INSERT INTO auth_security_events
				(id, user_id, event_type, request_id, metadata, created_at)
			VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		`, event.id, event.userID, event.eventType, "stale-recovery-request",
			`{"marker":"stale-recovery-metadata"}`, now.Add(-16*time.Minute)); err != nil {
			t.Fatalf("insert stale %s event: %v", event.eventType, err)
		}
	}

	summary, err := fixture.store.MFAEventSummary(ctx, since)
	if err != nil {
		t.Fatalf("read recovery support MFA event summary: %v", err)
	}
	want := postgres.MFAEventSummary{
		RecoverySupportRequests:                  2,
		RecoverySupportVerificationFailures:      2,
		RecoverySupportRateLimited:               1,
		RecoverySupportSessionRevocationFailures: 1,
		AffectedUsers:                            3,
	}
	if summary != want {
		t.Fatalf("recovery support MFA event summary = %+v, want %+v", summary, want)
	}

	encoded, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal recovery support MFA event summary: %v", err)
	}
	encodedSummary := string(encoded)
	for _, secret := range []string{
		"recovery-user-1",
		"recovery-request-1",
		"request-metadata",
		"123456",
		"stale-recovery-metadata",
	} {
		if strings.Contains(encodedSummary, secret) {
			t.Fatalf("recovery support MFA event summary exposed event metadata or identifier %q: %s", secret, encodedSummary)
		}
	}
}
