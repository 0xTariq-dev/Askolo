package postgres

import (
	"encoding/json"
	"testing"
)

func TestValidateUSDUsageEvidenceAcceptsTranscriptionAndRealtimeMetering(t *testing.T) {
	tests := []struct {
		name     string
		evidence USDEvidence
	}{
		{
			name: "recorded transcription",
			evidence: USDEvidence{
				UsageUnit: "milliseconds", UsageUnits: 1200, DurationMs: 1200,
				ProviderRequestId: "provider-request-id",
			},
		},
		{
			name: "realtime meter source",
			evidence: USDEvidence{
				UsageUnit: "milliseconds", UsageUnits: 1200, DurationMs: 1200,
				Payload: json.RawMessage(`{"meterSource":"server_elapsed"}`),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateUSDUsageEvidence(test.evidence); err != nil {
				t.Fatalf("expected valid usage evidence, got %v", err)
			}
		})
	}
}

func TestValidateUSDUsageEvidenceRejectsNonMeteringPayload(t *testing.T) {
	evidence := USDEvidence{
		UsageUnit: "milliseconds", UsageUnits: 1200, DurationMs: 1200,
		Payload: json.RawMessage(`{"transcript":"must not be stored"}`),
	}
	if err := validateUSDUsageEvidence(evidence); err == nil {
		t.Fatal("expected non-metering payload to be rejected")
	}
}
