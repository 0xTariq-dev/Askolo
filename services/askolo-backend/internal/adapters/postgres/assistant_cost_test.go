package postgres

import "testing"

func TestAssistantUSDMicroCostAccountsForCachedInput(t *testing.T) {
	card := USDRateCard{
		Meter:                          "tokens",
		InputUsdMicrosPerMillion:       200_000,
		CachedInputUsdMicrosPerMillion: 20_000,
		OutputUsdMicrosPerMillion:      1_200_000,
	}

	got, err := assistantUSDMicroCost(card, 2_000_000, 1_000_000, 1_000_000)
	if err != nil {
		t.Fatalf("assistantUSDMicroCost: %v", err)
	}
	if got != 1_420_000 {
		t.Fatalf("cost = %d micro-USD, want 1420000", got)
	}
}

func TestAssistantUSDMicroCostFallsBackWhenCacheRateIsUnset(t *testing.T) {
	card := USDRateCard{
		Meter:                     "tokens",
		InputUsdMicrosPerMillion:  200_000,
		OutputUsdMicrosPerMillion: 1_200_000,
	}

	got, err := assistantUSDMicroCost(card, 1_000_000, 500_000, 0)
	if err != nil {
		t.Fatalf("assistantUSDMicroCost: %v", err)
	}
	if got != 200_000 {
		t.Fatalf("cost = %d micro-USD, want 200000", got)
	}
}

func TestAssistantUSDMicroCostRejectsImpossibleCachedUsage(t *testing.T) {
	card := USDRateCard{
		Meter:                          "tokens",
		InputUsdMicrosPerMillion:       200_000,
		CachedInputUsdMicrosPerMillion: 20_000,
		OutputUsdMicrosPerMillion:      1_200_000,
	}
	if _, err := assistantUSDMicroCost(card, 3, 4, 1); err == nil {
		t.Fatal("accepted cached token count above total input tokens")
	}
}
