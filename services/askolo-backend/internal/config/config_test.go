package config

import (
	"bytes"
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

func TestLoadRequiresAndReadsTurnstileSecret(t *testing.T) {
	for key, value := range map[string]string{
		"ASKOLO_ENVIRONMENT":                    "development",
		"ASKOLO_ASSEMBLYAI_VOICE_AGENT_ENABLED": "false",
		"ASKOLO_ASSEMBLYAI_AGENT_IDS":           "",
		"ASKOLO_CANONICAL_ORIGIN":               "",
		"ASKOLO_COOKIE_NAMESPACE":               "askolo_test",
		"ASKOLO_RELEASE_MODE":                   "",
		"ASKOLO_COMMIT_SHA":                     "",
		"ASKOLO_RELEASE_TAG":                    "",
		"ASKO_PARENT_PRODUCTION_TAG":            "",
		"ASKOLO_INTERNAL_TOKEN":                 "",
		"ASKOLO_ADMIN_EMAILS":                   "",
		"ASKOLO_R2_ACCOUNT_ID":                  "",
		"ASKOLO_R2_US_BUCKET":                   "",
		"ASKOLO_R2_EU_BUCKET":                   "",
		"ASKOLO_R2_REGIONAL_ACCESS_KEY_ID":      "",
		"ASKOLO_R2_REGIONAL_SECRET_ACCESS_KEY":  "",
		"SESSION_SECRET":                        "session-secret-for-tests-123456789",
		"AUTH_CHALLENGE_SECRET":                 "",
		"AUTH_RATE_LIMIT_HMAC_SECRET":           "rate-limit-hmac-secret-for-tests-123456789",
		"GOOGLE_TOKEN_ENCRYPTION_KEY":           "",
		"GOOGLE_CLIENT_ID":                      "test-google-client-id",
		"GOOGLE_CLIENT_SECRET":                  "test-google-client-secret",
		"MS_CLIENT_ID":                          "test-microsoft-client-id",
		"MS_CLIENT_SECRET":                      "test-microsoft-client-secret",
		"AUTH_TOTP_ENCRYPTION_KEY":              "",
		"TURNSTILE_SECRET_KEY":                  "",
		"DD_API_KEY":                            "",
		"DD_SITE":                               "",
	} {
		t.Setenv(key, value)
	}

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TURNSTILE_SECRET_KEY") {
		t.Fatalf("Load() error = %v, want a missing Turnstile secret error", err)
	}

	t.Setenv("TURNSTILE_SECRET_KEY", "test-turnstile-secret")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with Turnstile secret: %v", err)
	}
	if cfg.TurnstileSecret != "test-turnstile-secret" {
		t.Fatalf("TurnstileSecret = %q, want configured value", cfg.TurnstileSecret)
	}
	if cfg.Google.LoginClientID != "test-google-client-id" ||
		cfg.Google.LoginClientSecret != "test-google-client-secret" {
		t.Fatal("Google login config did not use the current generic client env names")
	}
	if cfg.Microsoft.ClientID != "test-microsoft-client-id" ||
		cfg.Microsoft.ClientSecret != "test-microsoft-client-secret" {
		t.Fatal("Microsoft integration config did not use the configured MS client env names")
	}
	if len(cfg.TOTPEncryptionKey) != 32 || !bytes.Equal(cfg.TOTPEncryptionKey, cfg.TOTPEncryptionKeyFallback) {
		t.Fatal("without a dedicated TOTP key, configuration should use the session-derived key for current and legacy ciphertext")
	}
	t.Setenv("AUTH_TOTP_ENCRYPTION_KEY", "11234567890123456789012345678901")
	rotated, err := Load()
	if err != nil {
		t.Fatalf("Load() with dedicated TOTP key: %v", err)
	}
	if len(rotated.TOTPEncryptionKey) != 32 ||
		!bytes.Equal(rotated.TOTPEncryptionKeyFallback, cfg.TOTPEncryptionKey) ||
		bytes.Equal(rotated.TOTPEncryptionKey, rotated.TOTPEncryptionKeyFallback) {
		t.Fatal("dedicated TOTP key should be active while retaining the session-derived legacy key")
	}
	if _, ok := cfg.TurnstileAllowedHostnames["localhost"]; !ok {
		t.Fatal("development Turnstile hostname allowlist must include localhost")
	}
}

func TestLoadR2ConfigLoadsRegionalBuckets(t *testing.T) {
	for key, value := range map[string]string{
		"ASKOLO_R2_ACCOUNT_ID":                 "11111111111111111111111111111111",
		"ASKOLO_R2_US_BUCKET":                  "askolo-production-us",
		"ASKOLO_R2_EU_BUCKET":                  "askolo-production-eu",
		"ASKOLO_R2_REGIONAL_ACCESS_KEY_ID":     "regional-access-key",
		"ASKOLO_R2_REGIONAL_SECRET_ACCESS_KEY": "regional-secret-key",
	} {
		t.Setenv(key, value)
	}

	got, err := loadR2Config()
	if err != nil {
		t.Fatalf("loadR2Config(): %v", err)
	}
	if got.US.Bucket != "askolo-production-us" ||
		got.US.Jurisdiction != "us" ||
		got.EU.Bucket != "askolo-production-eu" ||
		got.EU.Jurisdiction != "eu" ||
		got.US.AccessKeyID != "regional-access-key" ||
		got.US.SecretAccessKey != "regional-secret-key" ||
		got.US.AccessKeyID != got.EU.AccessKeyID ||
		got.US.SecretAccessKey != got.EU.SecretAccessKey {
		t.Fatal("regional R2 configuration was not loaded correctly")
	}
}

func TestLoadR2ConfigRejectsIncompleteRegionalBucketsAndCredentials(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
	}{
		{
			name: "missing EU bucket",
			values: map[string]string{
				"ASKOLO_R2_US_BUCKET": "askolo-production-us-files",
			},
		},
		{
			name: "missing regional secret",
			values: map[string]string{
				"ASKOLO_R2_US_BUCKET":                  "askolo-production-us-files",
				"ASKOLO_R2_EU_BUCKET":                  "askolo-production-eu-files",
				"ASKOLO_R2_REGIONAL_ACCESS_KEY_ID":     "regional-access-key",
				"ASKOLO_R2_REGIONAL_SECRET_ACCESS_KEY": "",
			},
		},
		{
			name: "same bucket for both jurisdictions",
			values: map[string]string{
				"ASKOLO_R2_US_BUCKET":                  "askolo-production-us-files",
				"ASKOLO_R2_EU_BUCKET":                  "askolo-production-us-files",
				"ASKOLO_R2_REGIONAL_ACCESS_KEY_ID":     "regional-access-key",
				"ASKOLO_R2_REGIONAL_SECRET_ACCESS_KEY": "regional-secret-key",
			},
		},
	}
	keys := []string{
		"ASKOLO_R2_ACCOUNT_ID",
		"ASKOLO_R2_US_BUCKET",
		"ASKOLO_R2_EU_BUCKET",
		"ASKOLO_R2_REGIONAL_ACCESS_KEY_ID",
		"ASKOLO_R2_REGIONAL_SECRET_ACCESS_KEY",
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := map[string]string{
				"ASKOLO_R2_ACCOUNT_ID": "11111111111111111111111111111111",
			}
			for key, value := range test.values {
				values[key] = value
			}
			for _, key := range keys {
				t.Setenv(key, values[key])
			}
			if _, err := loadR2Config(); err == nil {
				t.Fatal("incomplete regional R2 configuration was accepted")
			}
		})
	}
}

func TestTurnstileAllowedHostnamesAreExactAndEnvironmentScoped(t *testing.T) {
	production := turnstileAllowedHostnames("production")
	if len(production) != 4 {
		t.Fatalf("production hostname allowlist has %d entries, want exactly four", len(production))
	}
	for _, hostname := range []string{
		"dev.askolo.app",
		"web.askolo.app",
		"staging.askolo.app",
		"5b8af2fa-e65c-4295-8e4b-b5f0d0c10891-00-2sb81pwvhiesj.picard.replit.dev",
	} {
		if _, ok := production[hostname]; !ok {
			t.Fatalf("production hostname %q is missing", hostname)
		}
	}
	for _, hostname := range []string{"localhost", "127.0.0.1", "*.askolo.app"} {
		if _, ok := production[hostname]; ok {
			t.Fatalf("production hostname allowlist unexpectedly contains %q", hostname)
		}
	}

	development := turnstileAllowedHostnames("development")
	if len(development) != 6 {
		t.Fatalf("development hostname allowlist has %d entries, want exactly six", len(development))
	}
	for _, hostname := range []string{"localhost", "127.0.0.1"} {
		if _, ok := development[hostname]; !ok {
			t.Fatalf("development hostname allowlist is missing %q", hostname)
		}
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

func TestValidateAssemblyAIVoiceAgentConfigurationRequiresAllVoices(t *testing.T) {
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

	if err := validateAssemblyAIVoiceAgentConfiguration(false, nil); err != nil {
		t.Fatalf("disabled Live Mode should not require agent IDs: %v", err)
	}
	if err := validateAssemblyAIVoiceAgentConfiguration(true, complete); err != nil {
		t.Fatalf("complete agent configuration should allow Live Mode in Production: %v", err)
	}
	if err := validateAssemblyAIVoiceAgentConfiguration(true, map[string]string{"michael": "agent-michael"}); err == nil ||
		!strings.Contains(err.Error(), "all nine stored voice agents") {
		t.Fatalf("incomplete agent configuration error = %v, want a clear all-nine-agents error", err)
	}
	if err := validateAssemblyAIVoiceAgentConfiguration(true, complete); err != nil {
		t.Fatalf("complete Development configuration was rejected: %v", err)
	}
}

func TestLoadProductionLiveModeConfiguration(t *testing.T) {
	const completeAgentIDs = `{"michael":"agent-michael","mary":"agent-mary","paul":"agent-paul","vera":"agent-vera","giovanni":"agent-giovanni","lola":"agent-lola","juergen":"agent-juergen","rafael":"agent-rafael","estelle":"agent-estelle"}`

	t.Run("enabled with all nine agents", func(t *testing.T) {
		setProductionLiveModeConfig(t, "true", completeAgentIDs)

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() with complete Production Live Mode configuration: %v", err)
		}
		if !cfg.AssemblyAIVoiceAgentEnabled {
			t.Fatal("Production Live Mode was not enabled")
		}
		if len(cfg.AssemblyAIVoiceAgentIDs) != len(supportedAssemblyAIVoices) {
			t.Fatalf("configured voice-agent count = %d, want %d", len(cfg.AssemblyAIVoiceAgentIDs), len(supportedAssemblyAIVoices))
		}
	})

	t.Run("disabled without agents", func(t *testing.T) {
		setProductionLiveModeConfig(t, "false", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() with disabled Production Live Mode and no agents: %v", err)
		}
		if cfg.AssemblyAIVoiceAgentEnabled {
			t.Fatal("Production Live Mode should remain disabled")
		}
		if len(cfg.AssemblyAIVoiceAgentIDs) != 0 {
			t.Fatalf("voice-agent IDs = %d, want none", len(cfg.AssemblyAIVoiceAgentIDs))
		}
	})

	t.Run("unset flag defaults off without agents", func(t *testing.T) {
		setProductionLiveModeConfig(t, "", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() with unset Production Live Mode and no agents: %v", err)
		}
		if cfg.AssemblyAIVoiceAgentEnabled {
			t.Fatal("Production Live Mode should default to disabled")
		}
	})

	t.Run("enabled without all agents fails clearly", func(t *testing.T) {
		setProductionLiveModeConfig(t, "true", `{"michael":"agent-michael"}`)

		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "all nine stored voice agents") {
			t.Fatalf("Load() error = %v, want a clear all-nine-agents configuration error", err)
		}
	})
}

func setProductionLiveModeConfig(t *testing.T, enabled, agentIDs string) {
	t.Helper()
	for key, value := range map[string]string{
		"ASKOLO_ENVIRONMENT":                    "production",
		"ASKOLO_ASSEMBLYAI_VOICE_AGENT_ENABLED": enabled,
		"ASKOLO_ASSEMBLYAI_AGENT_IDS":           agentIDs,
		"ASKOLO_CANONICAL_ORIGIN":               "https://production.example",
		"ASKOLO_COOKIE_NAMESPACE":               "askolo_test",
		"ASKOLO_DATABASE_ID":                    "test-production-database",
		"ASKOLO_RELEASE_MODE":                   "normal",
		"ASKOLO_COMMIT_SHA":                     "test-commit",
		"ASKOLO_RELEASE_TAG":                    "test-release",
		"ASKO_PARENT_PRODUCTION_TAG":            "",
		"ASKOLO_INTERNAL_TOKEN":                 "test-internal-token",
		"ASKOLO_ADMIN_EMAILS":                   "",
		"ASKOLO_R2_ACCOUNT_ID":                  "",
		"ASKOLO_R2_US_BUCKET":                   "",
		"ASKOLO_R2_EU_BUCKET":                   "",
		"ASKOLO_R2_REGIONAL_ACCESS_KEY_ID":      "",
		"ASKOLO_R2_REGIONAL_SECRET_ACCESS_KEY":  "",
		"SESSION_SECRET":                        "test-session-secret-for-config-tests",
		"AUTH_CHALLENGE_SECRET":                 "test-challenge-secret",
		"AUTH_RATE_LIMIT_HMAC_SECRET":           "test-rate-limit-hmac-secret-at-least-32",
		"GOOGLE_TOKEN_ENCRYPTION_KEY":           "",
		"AUTH_TOTP_ENCRYPTION_KEY":              "",
		"TURNSTILE_SECRET_KEY":                  "test-turnstile-secret",
		"DD_API_KEY":                            "",
		"DD_SITE":                               "",
		"DATABASE_URL":                          "",
	} {
		t.Setenv(key, value)
	}
}
