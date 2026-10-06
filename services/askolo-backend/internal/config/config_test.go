package config

import (
	"strings"
	"testing"
)

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

func TestParseAssemblyAIVoiceAgentIDsRequiresSupportedVoiceKeysAndSafeIDs(t *testing.T) {
	encoded := `{"michael":"agent-michael","mary":"agent-mary","paul":"agent-paul","vera":"agent-vera","giovanni":"agent-giovanni","lola":"agent-lola","juergen":"agent-juergen","rafael":"agent-rafael","estelle":"agent-estelle"}`
	got, err := parseAssemblyAIVoiceAgentIDs(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 9 || got["michael"] != "agent-michael" || got["estelle"] != "agent-estelle" {
		t.Fatalf("supported voice IDs were not retained: %#v", got)
	}

	for name, raw := range map[string]string{
		"unknown voice":      `{"unknown":"agent-123"}`,
		"unsafe agent id":    `{"michael":"agent id"}`,
		"null object":        `null`,
		"malformed json":     `{"michael":`,
		"oversized json":     strings.Repeat("x", 8193),
		"wrong value type":   `{"michael":123}`,
		"empty agent id":     `{"michael":""}`,
		"leading whitespace": `{"michael":" agent-123"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseAssemblyAIVoiceAgentIDs(raw); err == nil {
				t.Fatal("invalid voice-agent configuration was accepted")
			}
		})
	}
}

func TestValidateAssemblyAIVoiceAgentConfigurationRequiresDevelopmentAndAllVoices(t *testing.T) {
	complete := make(map[string]string, len(supportedAssemblyAIVoices))
	for _, voice := range supportedAssemblyAIVoices {
		complete[voice] = "agent-" + voice
	}

	enabled, disabled := normalizeAssemblyAIVoiceAgentEnabled(
		"development",
		true,
		map[string]string{"michael": "agent-michael"},
	)
	if enabled || !disabled {
		t.Fatal("Development Live Mode must fail closed when stored agents are incomplete")
	}
	enabled, disabled = normalizeAssemblyAIVoiceAgentEnabled("development", true, complete)
	if !enabled || disabled {
		t.Fatal("complete Development agent configuration should preserve the enabled flag")
	}
	enabled, disabled = normalizeAssemblyAIVoiceAgentEnabled("development", false, nil)
	if enabled || disabled {
		t.Fatal("explicitly disabled Live Mode should remain disabled")
	}

	if err := validateAssemblyAIVoiceAgentConfiguration("production", false, nil); err != nil {
		t.Fatalf("disabled Live Mode should not require agent IDs: %v", err)
	}
	if err := validateAssemblyAIVoiceAgentConfiguration("production", true, complete); err == nil {
		t.Fatal("Live Mode was allowed outside Development")
	}
	if err := validateAssemblyAIVoiceAgentConfiguration("development", true, map[string]string{"michael": "agent-michael"}); err == nil {
		t.Fatal("Live Mode was allowed with an incomplete set of stored agents")
	}
	if err := validateAssemblyAIVoiceAgentConfiguration("development", true, complete); err != nil {
		t.Fatalf("complete Development configuration was rejected: %v", err)
	}
}
