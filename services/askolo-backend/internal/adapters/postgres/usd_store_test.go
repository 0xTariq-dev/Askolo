package postgres

import "testing"

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
