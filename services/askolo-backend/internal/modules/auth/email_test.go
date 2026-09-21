package auth

import (
	"context"
	"errors"
	"testing"

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
