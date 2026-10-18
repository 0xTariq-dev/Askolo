package product

import (
	"testing"

	"askolo/backend/internal/adapters/postgres"
)

func TestSelectedRateCardRequiresTheConfiguredProviderAccount(t *testing.T) {
	card := postgres.USDRateCard{
		Provider: "openai", ProviderAccountID: "acct_primary", Mode: "assistant",
		Model: "gpt-5.6-luna", Meter: "tokens",
		InputUsdMicrosPerMillion: 100, OutputUsdMicrosPerMillion: 200,
	}
	policy := postgres.USDPolicy{
		ProviderAccountIDs: map[string]string{"openai": "acct_primary"},
		RateCards:          map[string]postgres.USDRateCard{"openai:assistant:gpt-5.6-luna": card},
	}
	if got, ok := selectedRateCard(policy, "openai:assistant:gpt-5.6-luna"); !ok || got.ProviderAccountID != "acct_primary" {
		t.Fatalf("selectedRateCard() = %#v, %v", got, ok)
	}
	policy.ProviderAccountIDs["openai"] = "acct_secondary"
	if _, ok := selectedRateCard(policy, "openai:assistant:gpt-5.6-luna"); ok {
		t.Fatal("rate card for a different configured account was selected")
	}
}

func TestAssistantSpeechUSDMicroCostRoundsUpPerCharacter(t *testing.T) {
	card := postgres.USDRateCard{
		Meter:                         "characters",
		UsdMicrosPerMillionCharacters: 200_000,
	}
	got, err := assistantSpeechUSDMicroCost(card, 10)
	if err != nil {
		t.Fatalf("assistantSpeechUSDMicroCost: %v", err)
	}
	if got != 2 {
		t.Fatalf("cost = %d micro-USD, want 2", got)
	}
}

func TestAssistantSpeechUSDMicroCostRejectsInvalidMeterAndUsage(t *testing.T) {
	card := postgres.USDRateCard{
		Meter:                         "hour",
		UsdMicrosPerMillionCharacters: 200_000,
	}
	if _, err := assistantSpeechUSDMicroCost(card, 10); err == nil {
		t.Fatal("accepted non-character rate card")
	}
	card.Meter = "characters"
	if _, err := assistantSpeechUSDMicroCost(card, 0); err == nil {
		t.Fatal("accepted zero character usage")
	}
}
