package product

import (
	"errors"
	"testing"

	"askolo/backend/internal/adapters/postgres"
)

func TestAIPrivacyPreferencesPersistSeparateConsentsAndRequireAppRedaction(t *testing.T) {
	fixture := openCreditPolicyIntegrationFixture(t)
	const userID = "ai-privacy-preferences-user"
	fixture.createUserAndSession(t, userID, "ai-privacy-preferences@example.test", true)

	ctx := fixture.ctx
	enabled := true
	disabled := false
	appRedaction := "app"

	if _, err := fixture.store.UpdateAIPrivacyPreferences(ctx, userID, postgres.AIPrivacyPreferencesPatch{
		AssistantProcessingConsent: &enabled,
	}); !errors.Is(err, postgres.ErrAssistantRedactionLocationRequired) {
		t.Fatalf("enabling Assistant processing without app redaction error = %v", err)
	}

	if _, err := fixture.store.UpdateAIPrivacyPreferences(ctx, userID, postgres.AIPrivacyPreferencesPatch{
		RedactionLocation: &appRedaction,
	}); err != nil {
		t.Fatalf("select app-side redaction: %v", err)
	}
	preferences, err := fixture.store.UpdateAIPrivacyPreferences(ctx, userID, postgres.AIPrivacyPreferencesPatch{
		AssistantProcessingConsent: &enabled,
	})
	if err != nil {
		t.Fatalf("enable Assistant processing: %v", err)
	}
	if !preferences.AssistantProcessingConsentGiven ||
		preferences.RecordedVoiceInputConsentGiven || preferences.AssemblyAILiveConsentGiven {
		t.Fatalf("consents were not kept separate: %#v", preferences)
	}
	consent, version, err := fixture.store.AssistantProcessingConsent(ctx, userID)
	if err != nil || !consent || version != postgres.AssistantProcessingConsentVersion {
		t.Fatalf("Assistant consent = %v, version %q, err %v", consent, version, err)
	}

	if _, err := fixture.store.UpdateAIPrivacyPreferences(ctx, userID, postgres.AIPrivacyPreferencesPatch{
		RecordedVoiceInputConsent: &enabled,
	}); err != nil {
		t.Fatalf("enable recorded voice input: %v", err)
	}
	if _, err := fixture.store.UpdateAIPrivacyPreferences(ctx, userID, postgres.AIPrivacyPreferencesPatch{
		AssemblyAILiveConsent: &enabled,
	}); err != nil {
		t.Fatalf("enable Live Mode permission: %v", err)
	}
	preferences, err = fixture.store.UpdateAIPrivacyPreferences(ctx, userID, postgres.AIPrivacyPreferencesPatch{
		AssistantProcessingConsent: &disabled,
	})
	if err != nil {
		t.Fatalf("revoke Assistant processing: %v", err)
	}
	if preferences.AssistantProcessingConsentGiven ||
		!preferences.RecordedVoiceInputConsentGiven || !preferences.AssemblyAILiveConsentGiven {
		t.Fatalf("revoking Assistant processing changed unrelated permissions: %#v", preferences)
	}
	consent, _, err = fixture.store.AssistantProcessingConsent(ctx, userID)
	if err != nil || consent {
		t.Fatalf("revoked Assistant consent = %v, err %v", consent, err)
	}
}

func TestVoiceAgentCreditRateRequiresAnActiveHourlyRate(t *testing.T) {
	key := "assemblyai:voice_agent:" + assemblyAIVoiceAgentModel
	for _, test := range []struct {
		name string
		card postgres.USDRateCard
		want bool
	}{
		{name: "no rate", want: false},
		{name: "wrong meter", card: postgres.USDRateCard{Meter: "minute", UsdMicrosPerHour: 100}, want: false},
		{name: "zero rate", card: postgres.USDRateCard{Meter: "hour"}, want: false},
		{name: "configured", card: postgres.USDRateCard{Meter: "hour", UsdMicrosPerHour: 100}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy := postgres.USDPolicy{RateCards: map[string]postgres.USDRateCard{}}
			if test.card.Meter != "" || test.card.UsdMicrosPerHour != 0 {
				policy.RateCards[key] = test.card
			}
			if got := voiceAgentCreditRateConfigured(policy); got != test.want {
				t.Fatalf("voiceAgentCreditRateConfigured() = %v, want %v", got, test.want)
			}
		})
	}
}
