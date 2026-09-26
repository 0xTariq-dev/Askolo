package config

import "testing"

func TestParseAdminEmailsFailClosedAndNormalizes(t *testing.T) {
	if got, err := parseAdminEmails(""); err != nil || len(got) != 0 {
		t.Fatal("empty admin allowlist must deny all")
	}
	got, err := parseAdminEmails(" Admin@example.com,admin@example.com ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("normalized admin allowlist length = %d, want 1", len(got))
	}
	if _, ok := got["admin@example.com"]; !ok {
		t.Fatal("normalized admin email missing")
	}
}

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
