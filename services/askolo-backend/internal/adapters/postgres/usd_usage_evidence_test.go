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
		{
			name: "Azure usage awaiting admin review",
			evidence: USDEvidence{
				UsageUnit: "characters", UsageUnits: 87,
				Payload: json.RawMessage(`{"meterSource":"azure_text_characters","reviewStatus":"pending"}`),
			},
		},
		{
			name: "billable output unavailable",
			evidence: USDEvidence{
				UsageUnit: "characters", UsageUnits: 87,
				Payload: json.RawMessage(`{"meterSource":"azure_text_characters","outputStatus":"unavailable"}`),
			},
		},
		{
			name: "estimated token usage",
			evidence: USDEvidence{
				UsageUnit: "tokens", InputTokens: 10, OutputTokens: 2,
				Payload: json.RawMessage(`{"usageStatus":"estimated","cachedInputTokens":0}`),
			},
		},
		{
			name: "provider-reconciled token usage",
			evidence: USDEvidence{
				UsageUnit: "tokens", InputTokens: 20, OutputTokens: 5,
				Payload: json.RawMessage(`{"usageStatus":"measured","provenance":"provider_reconciliation","cachedInputTokens":4}`),
			},
		},
		{
			name: "provider-reconciled voice usage",
			evidence: USDEvidence{
				UsageUnit: "milliseconds", UsageUnits: 10_000, DurationMs: 10_000,
				Payload: json.RawMessage(`{"usageStatus":"measured","provenance":"provider_reconciliation","durationMs":10000}`),
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

func TestValidateUSDUsageEvidenceRejectsUnknownUsageStatus(t *testing.T) {
	evidence := USDEvidence{
		UsageUnit: "tokens", InputTokens: 1, OutputTokens: 1,
		Payload: json.RawMessage(`{"usageStatus":"trusted"}`),
	}
	if err := validateUSDUsageEvidence(evidence); err == nil {
		t.Fatal("expected unknown usage status to be rejected")
	}
}

func TestValidateUSDUsageEvidenceRejectsUnknownReviewMetadata(t *testing.T) {
	evidence := USDEvidence{
		UsageUnit: "characters", UsageUnits: 87,
		Payload: json.RawMessage(`{"reviewStatus":"approved"}`),
	}
	if err := validateUSDUsageEvidence(evidence); err == nil {
		t.Fatal("expected unknown review status to be rejected")
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
