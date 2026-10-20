package postgres

import "testing"

func TestCeilVoiceCost(t *testing.T) {
	tests := []struct {
		name     string
		rate     int64
		duration int64
		want     int64
		ok       bool
	}{
		{name: "rounds fractional micro dollar up", rate: 1, duration: 1, want: 1, ok: true},
		{name: "exact hour rate", rate: 3600000, duration: 3600000, want: 3600000, ok: true},
		{name: "zero duration", rate: 10, duration: 0, want: 0, ok: true},
		{name: "overflow rejected", rate: 9223372036854775807, duration: 2, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := ceilVoiceCost(test.rate, test.duration)
			if ok != test.ok || got != test.want {
				t.Fatalf("ceilVoiceCost(%d,%d) = %d,%v; want %d,%v",
					test.rate, test.duration, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestValidateUSDPolicySignupGrantConfiguration(t *testing.T) {
	base := USDPolicy{
		Version:            1,
		RateCards:          map[string]USDRateCard{},
		RolloverExpiryDays: 30,
		ChangeReason:       "initial policy",
	}
	for _, test := range []struct {
		name    string
		amount  int64
		wantErr bool
	}{
		{name: "unconfigured grant remains disabled", amount: 0},
		{name: "operator configured grant", amount: 5 * USDMicroUnitsPerDollar},
		{name: "negative grant rejected", amount: -1, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			policy := base
			policy.SignupGrantUsdMicros = test.amount
			err := validateUSDPolicy(policy)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateUSDPolicy() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestValidateUSDPolicyBindsRateCardsToConfiguredProviderAccounts(t *testing.T) {
	policy := USDPolicy{
		Version: 1, ChangeReason: "test",
		RolloverExpiryDays: 30,
		ProviderAccountIDs: map[string]string{"openai": "acct_primary"},
		RateCards: map[string]USDRateCard{
			"openai:assistant:gpt-5.6-luna": {
				Provider: "openai", ProviderAccountID: "acct_primary", Mode: "assistant",
				Model: "gpt-5.6-luna", Meter: "tokens",
				InputUsdMicrosPerMillion: 100, OutputUsdMicrosPerMillion: 200,
			},
		},
	}
	if err := validateUSDPolicy(policy); err != nil {
		t.Fatalf("valid account-bound policy rejected: %v", err)
	}
	card := policy.RateCards["openai:assistant:gpt-5.6-luna"]
	card.ProviderAccountID = "acct_other"
	policy.RateCards["openai:assistant:gpt-5.6-luna"] = card
	if err := validateUSDPolicy(policy); err == nil {
		t.Fatal("rate card for a different provider account was accepted")
	}
}
