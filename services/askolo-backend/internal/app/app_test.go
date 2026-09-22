package app

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"askolo/backend/internal/httpapi"
)

func TestEmailChallengeCleanupStateTracksBoundedReadiness(t *testing.T) {
	state := &emailChallengeCleanupState{}

	readiness := state.readiness()
	if readiness.Status != "unknown" ||
		readiness.ConsecutiveFailures != 0 ||
		readiness.LastSuccessfulCleanupAt != nil ||
		readiness.PersistentFailureThreshold != httpapi.EmailChallengeCleanupPersistentFailureThreshold {
		t.Fatalf("initial cleanup readiness = %+v, want unknown with no last success", readiness)
	}

	consecutiveFailures, shouldAlert := state.recordFailure()
	readiness = state.readiness()
	if consecutiveFailures != 1 || shouldAlert ||
		readiness.Status != "transient_failure" || readiness.ConsecutiveFailures != 1 {
		t.Fatalf("first cleanup failure = %+v, want transient_failure/1", readiness)
	}

	consecutiveFailures, shouldAlert = state.recordFailure()
	readiness = state.readiness()
	if consecutiveFailures != 2 || shouldAlert ||
		readiness.Status != "transient_failure" || readiness.ConsecutiveFailures != 2 {
		t.Fatalf("second cleanup failure = %+v, want transient_failure/2", readiness)
	}

	consecutiveFailures, shouldAlert = state.recordFailure()
	readiness = state.readiness()
	if consecutiveFailures != httpapi.EmailChallengeCleanupPersistentFailureThreshold || !shouldAlert ||
		readiness.Status != "persistent_failure" ||
		readiness.ConsecutiveFailures != httpapi.EmailChallengeCleanupPersistentFailureThreshold {
		t.Fatalf("threshold cleanup failure = %+v, want persistent_failure/%d",
			readiness,
			httpapi.EmailChallengeCleanupPersistentFailureThreshold,
		)
	}

	successfulAt := time.Date(2026, time.September, 22, 12, 34, 56, 123456789, time.FixedZone("test", 3600))
	if !state.recordSuccess(successfulAt) {
		t.Fatal("successful cleanup did not report alert recovery")
	}
	readiness = state.readiness()
	if readiness.Status != "healthy" || readiness.ConsecutiveFailures != 0 {
		t.Fatalf("successful cleanup = %+v, want healthy/0", readiness)
	}
	if readiness.LastSuccessfulCleanupAt == nil || !readiness.LastSuccessfulCleanupAt.Equal(successfulAt.UTC()) {
		t.Fatalf("last successful cleanup = %v, want %v", readiness.LastSuccessfulCleanupAt, successfulAt.UTC())
	}

	consecutiveFailures, shouldAlert = state.recordFailure()
	readiness = state.readiness()
	if consecutiveFailures != 1 || shouldAlert ||
		readiness.Status != "transient_failure" ||
		readiness.ConsecutiveFailures != 1 ||
		readiness.LastSuccessfulCleanupAt == nil ||
		!readiness.LastSuccessfulCleanupAt.Equal(successfulAt.UTC()) {
		t.Fatalf("post-success failure = %+v, want transient failure with last success retained", readiness)
	}

	if state.recordSuccess(successfulAt) {
		t.Fatal("healthy cleanup incorrectly reported alert recovery")
	}
}

func TestEmailChallengeCleanupAlertLogsAreAggregateAndRecoverable(t *testing.T) {
	var logs bytes.Buffer
	app := &App{
		logger:      slog.New(slog.NewTextHandler(&logs, nil)),
		environment: "staging",
		serviceName: "askolo-backend",
	}

	app.logCleanupFailureAlert(httpapi.EmailChallengeCleanupPersistentFailureThreshold)
	app.logCleanupRecovery()

	output := logs.String()
	if strings.Count(output, "Email challenge cleanup failure alert") != 1 {
		t.Fatalf("cleanup failure alert log count = %d, want 1; logs=%q",
			strings.Count(output, "Email challenge cleanup failure alert"), output)
	}
	if strings.Count(output, "Email challenge cleanup recovered") != 1 {
		t.Fatalf("cleanup recovery log count = %d, want 1; logs=%q",
			strings.Count(output, "Email challenge cleanup recovered"), output)
	}
	for _, field := range []string{
		"alert=true",
		"recovery=true",
		"environment=staging",
		"service=askolo-backend",
		"operation=email_challenge_cleanup",
		"failure_threshold=3",
	} {
		if !strings.Contains(output, field) {
			t.Fatalf("cleanup alert logs missing %q: %q", field, output)
		}
	}
	for _, field := range []string{"challenge_id", "address=", "error=", "request_id"} {
		if strings.Contains(output, field) {
			t.Fatalf("cleanup alert logs exposed %q: %q", field, output)
		}
	}
}
