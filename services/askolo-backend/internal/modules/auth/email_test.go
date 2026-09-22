package auth

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"askolo/backend/internal/config"
)

func TestNewChallengeCodeIsSixDigits(t *testing.T) {
	code, err := newChallengeCode()
	if err != nil {
		t.Fatalf("newChallengeCode() error = %v", err)
	}
	if !validChallengeCode(code) {
		t.Fatalf("generated code %q is not a valid challenge code", code)
	}
}

func TestChallengeHashIsStableAndSecretBound(t *testing.T) {
	first := NewHandler(config.Config{
		Email: config.EmailConfig{ChallengeSecret: "first-secret"},
	}, nil, nil)
	second := NewHandler(config.Config{
		Email: config.EmailConfig{ChallengeSecret: "second-secret"},
	}, nil, nil)

	if first.hashChallenge("123456") != first.hashChallenge("123456") {
		t.Fatal("challenge hash is not stable")
	}
	if first.hashChallenge("123456") == first.hashChallenge("654321") {
		t.Fatal("different challenge values share a hash")
	}
	if first.hashChallenge("123456") == second.hashChallenge("123456") {
		t.Fatal("challenge hash is not bound to its secret")
	}
}

func TestChallengeCodeValidation(t *testing.T) {
	for _, test := range []struct {
		value string
		valid bool
	}{
		{"123456", true},
		{"000001", true},
		{"12345", false},
		{"1234567", false},
		{"12345a", false},
		{"１２３４５６", false},
	} {
		if got := validChallengeCode(test.value); got != test.valid {
			t.Errorf("validChallengeCode(%q) = %v, want %v", test.value, got, test.valid)
		}
	}
}

func TestSMTPEmailSenderRequiresConfiguration(t *testing.T) {
	sender := NewSMTPEmailSender(config.Config{})
	err := sender.Send(context.Background(), EmailMessage{
		To:      "user@example.com",
		Subject: "Test",
		Body:    "Test",
	})
	if !errors.Is(err, ErrEmailDeliveryNotConfigured) {
		t.Fatalf("Send() error = %v, want ErrEmailDeliveryNotConfigured", err)
	}
}

func TestEmailDeliveryMonitorBoundsLatencyAndTracksFailures(t *testing.T) {
	monitor := NewEmailDeliveryMonitor()
	monitor.Observe(EmailDeliveryProviderRejection, 45*time.Second)
	monitor.Observe(EmailDeliveryProviderRejection, 2*time.Second)
	monitor.Observe(EmailDeliveryProviderRejection, time.Second)

	snapshot := monitor.Snapshot()
	if snapshot.Attempts != 3 || snapshot.Handoffs != 0 {
		t.Fatalf("snapshot counts = %+v, want three attempts and no handoffs", snapshot)
	}
	if snapshot.ConsecutiveFailures != 3 {
		t.Fatalf("consecutive failures = %d, want 3", snapshot.ConsecutiveFailures)
	}
	if snapshot.LastLatencyMS != 1000 {
		t.Fatalf("last latency = %dms, want 1000ms", snapshot.LastLatencyMS)
	}
	if snapshot.Failures[EmailDeliveryProviderRejection] != 3 {
		t.Fatalf("provider rejection count = %d, want 3", snapshot.Failures[EmailDeliveryProviderRejection])
	}

	monitor.Observe(EmailDeliveryHandoff, 31*time.Second)
	snapshot = monitor.Snapshot()
	if snapshot.Handoffs != 1 || snapshot.ConsecutiveFailures != 0 {
		t.Fatalf("successful snapshot = %+v, want one handoff and reset failures", snapshot)
	}
	if snapshot.LastLatencyMS != 30000 {
		t.Fatalf("bounded handoff latency = %dms, want 30000ms", snapshot.LastLatencyMS)
	}
}

func TestEmailDeliveryReadinessDistinguishesConfigurationAndProviderState(t *testing.T) {
	cfg := config.Config{
		Email: config.EmailConfig{
			SMTPHost:        "smtp.example.com",
			SMTPPort:        587,
			SMTPUsername:    "mailer",
			SMTPPassword:    "secret",
			FromAddress:     "no-reply@example.com",
			ChallengeSecret: "challenge-secret",
		},
	}
	monitor := NewEmailDeliveryMonitor()
	handler := NewHandlerWithEmailSenderAndMonitor(cfg, nil, nil, nil, monitor)

	readiness := handler.EmailDeliveryReadiness()
	if readiness.Status != "unknown" || readiness.SMTPConfiguration != "configured" ||
		readiness.ChallengeConfiguration != "configured" {
		t.Fatalf("initial readiness = %+v, want configured but unknown", readiness)
	}

	monitor.Observe(EmailDeliveryConnectionFailure, time.Second)
	readiness = handler.EmailDeliveryReadiness()
	if readiness.Status != "transient_failure" {
		t.Fatalf("one failure readiness = %+v, want transient_failure", readiness)
	}
	monitor.Observe(EmailDeliveryConnectionFailure, time.Second)
	monitor.Observe(EmailDeliveryConnectionFailure, time.Second)
	readiness = handler.EmailDeliveryReadiness()
	if readiness.Status != "persistent_failure" {
		t.Fatalf("three failures readiness = %+v, want persistent_failure", readiness)
	}

	missing := NewHandler(config.Config{}, nil, nil).EmailDeliveryReadiness()
	if missing.Status != "not_configured" || missing.SMTPConfiguration != "missing" ||
		missing.ChallengeConfiguration != "missing" {
		t.Fatalf("missing readiness = %+v, want not_configured", missing)
	}
}

func TestNormalizeEmailDeliveryErrorDoesNotExposeAdapterDetails(t *testing.T) {
	err := normalizeEmailDeliveryError(fmt.Errorf("recipient=user@example.com body=secret: %w", errors.New("provider failed")))
	if emailDeliveryState(err) != EmailDeliveryProviderRejection || !emailDeliveryRetrySafe(err) {
		t.Fatalf("normalized error = %v, want retry-safe provider rejection", err)
	}
	if err.Error() != "email delivery provider_rejection" {
		t.Fatalf("normalized error text = %q, want privacy-safe category", err.Error())
	}
}
