package config

import (
	"strings"
	"testing"
)

func TestLoadDatadogConfig(t *testing.T) {
	t.Setenv("DD_API_KEY", "test-api-key")
	t.Setenv("DD_SITE", "us5.datadoghq.com")

	got, err := loadDatadogConfig()
	if err != nil {
		t.Fatalf("loadDatadogConfig() = error %v", err)
	}
	if got.APIKey != "test-api-key" || got.Site != "us5.datadoghq.com" {
		t.Fatalf("Datadog config = %+v, want configured key and site", got)
	}
}

func TestLoadDatadogConfigRequiresSiteForAPIKey(t *testing.T) {
	t.Setenv("DD_API_KEY", "test-api-key")
	t.Setenv("DD_SITE", "")

	if _, err := loadDatadogConfig(); err == nil || !strings.Contains(err.Error(), "DD_SITE is required") {
		t.Fatalf("loadDatadogConfig() error = %v, want required-site error", err)
	}
}

func TestLoadDatadogConfigRejectsUnsupportedSiteAndWhitespaceKey(t *testing.T) {
	t.Setenv("DD_API_KEY", "test-api-key")
	t.Setenv("DD_SITE", "attacker.example")
	if _, err := loadDatadogConfig(); err == nil || !strings.Contains(err.Error(), "supported Datadog site") {
		t.Fatalf("loadDatadogConfig() error = %v, want supported-site error", err)
	}

	t.Setenv("DD_API_KEY", " test-api-key ")
	t.Setenv("DD_SITE", "us5.datadoghq.com")
	if _, err := loadDatadogConfig(); err == nil || !strings.Contains(err.Error(), "must not contain whitespace") {
		t.Fatalf("loadDatadogConfig() error = %v, want whitespace-key error", err)
	}
}

func TestLoadDatadogConfigAllowsTelemetryToBeDisabled(t *testing.T) {
	t.Setenv("DD_API_KEY", "")
	t.Setenv("DD_SITE", "")

	got, err := loadDatadogConfig()
	if err != nil {
		t.Fatalf("loadDatadogConfig() = error %v", err)
	}
	if got.APIKey != "" || got.Site != "" {
		t.Fatalf("Datadog config = %+v, want disabled configuration", got)
	}
}
