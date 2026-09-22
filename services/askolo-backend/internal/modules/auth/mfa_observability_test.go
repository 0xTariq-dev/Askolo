package auth

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

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
