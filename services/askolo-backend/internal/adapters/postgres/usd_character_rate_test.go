package postgres

import "testing"

func TestValidateCharacterMeterRateCard(t *testing.T) {
	card := USDRateCard{
		Provider:                      "azure",
		Mode:                          "assistant_speech",
		Model:                         "azure-neural-tts",
		Meter:                         "characters",
		UsdMicrosPerMillionCharacters: 200_000,
	}
	if err := validateRateCard(card); err != nil {
		t.Fatalf("valid character rate card rejected: %v", err)
	}
	card.UsdMicrosPerMillionCharacters = 0
	if err := validateRateCard(card); err == nil {
		t.Fatal("character rate card without a configured rate was accepted")
	}
}
