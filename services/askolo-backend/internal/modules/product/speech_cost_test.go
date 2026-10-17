package product

import (
	"testing"

	"askolo/backend/internal/adapters/postgres"
)

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
