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
