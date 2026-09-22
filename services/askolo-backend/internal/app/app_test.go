package app

import (
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

	state.recordFailure()
	readiness = state.readiness()
	if readiness.Status != "transient_failure" || readiness.ConsecutiveFailures != 1 {
		t.Fatalf("first cleanup failure = %+v, want transient_failure/1", readiness)
	}

	state.recordFailure()
	readiness = state.readiness()
	if readiness.Status != "transient_failure" || readiness.ConsecutiveFailures != 2 {
		t.Fatalf("second cleanup failure = %+v, want transient_failure/2", readiness)
	}

	state.recordFailure()
	readiness = state.readiness()
	if readiness.Status != "persistent_failure" ||
		readiness.ConsecutiveFailures != httpapi.EmailChallengeCleanupPersistentFailureThreshold {
		t.Fatalf("threshold cleanup failure = %+v, want persistent_failure/%d",
			readiness,
			httpapi.EmailChallengeCleanupPersistentFailureThreshold,
		)
	}

	successfulAt := time.Date(2026, time.September, 22, 12, 34, 56, 123456789, time.FixedZone("test", 3600))
	state.recordSuccess(successfulAt)
	readiness = state.readiness()
	if readiness.Status != "healthy" || readiness.ConsecutiveFailures != 0 {
		t.Fatalf("successful cleanup = %+v, want healthy/0", readiness)
	}
	if readiness.LastSuccessfulCleanupAt == nil || !readiness.LastSuccessfulCleanupAt.Equal(successfulAt.UTC()) {
		t.Fatalf("last successful cleanup = %v, want %v", readiness.LastSuccessfulCleanupAt, successfulAt.UTC())
	}

	state.recordFailure()
	readiness = state.readiness()
	if readiness.Status != "transient_failure" ||
		readiness.ConsecutiveFailures != 1 ||
		readiness.LastSuccessfulCleanupAt == nil ||
		!readiness.LastSuccessfulCleanupAt.Equal(successfulAt.UTC()) {
		t.Fatalf("post-success failure = %+v, want transient failure with last success retained", readiness)
	}
}
