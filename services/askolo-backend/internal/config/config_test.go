package config

import "testing"

func TestResendConfigurationStatus(t *testing.T) {
	tests := []struct {
		name string
		cfg  EmailConfig
		want string
	}{
		{
			name: "missing",
			want: "missing",
		},
		{
			name: "configured",
			cfg: EmailConfig{
				ResendAPIKey: "unit-test-key",
				FromAddress:  "Askolo <noreply@example.com>",
			},
			want: "configured",
		},
		{
			name: "invalid sender",
			cfg: EmailConfig{
				ResendAPIKey: "unit-test-key",
				FromAddress:  "not-an-email",
			},
			want: "invalid",
		},
		{
			name: "invalid header value",
			cfg: EmailConfig{
				ResendAPIKey: "unit-test-key\r\nX-Injected: true",
				FromAddress:  "noreply@example.com",
			},
			want: "invalid",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.cfg.ResendConfigurationStatus(); got != test.want {
				t.Fatalf("ResendConfigurationStatus() = %q, want %q", got, test.want)
			}
		})
	}
}
