package product

import (
	"encoding/json"
	"reflect"
	"testing"

	"askolo/backend/internal/adapters/postgres"
)

func TestProfileUserResponseUsesClientFieldNames(t *testing.T) {
	preferredLocale := "ar"
	user := postgres.User{
		ID:                "user-1",
		Email:             "person@example.test",
		FirstName:         "Ada",
		LastName:          "Lovelace",
		ProfileImageURL:   "https://example.test/avatar.png",
		PreferredLocale:   &preferredLocale,
		Status:            "active",
		AccountCreatedVia: "google",
	}

	encoded, err := json.Marshal(toProfileUserResponse(user))
	if err != nil {
		t.Fatalf("marshal profile response: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal profile response: %v", err)
	}

	want := map[string]any{
		"id":              "user-1",
		"email":           "person@example.test",
		"firstName":       "Ada",
		"lastName":        "Lovelace",
		"profileImageUrl": "https://example.test/avatar.png",
		"preferredLocale": "ar",
		"status":          "active",
		"authProvider":    "google",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profile response = %#v, want %#v", got, want)
	}
}

func TestPreferredLocaleFormatAndEnabledSetAreSeparate(t *testing.T) {
	for _, test := range []struct {
		value       string
		wellFormed  bool
		isSupported bool
	}{
		{value: "en", wellFormed: true, isSupported: true},
		{value: "ar", wellFormed: true, isSupported: true},
		{value: "fr", wellFormed: true, isSupported: false},
		{value: "zz", wellFormed: true, isSupported: false},
		{value: "", wellFormed: false, isSupported: false},
		{value: "EN", wellFormed: false, isSupported: false},
		{value: "e1", wellFormed: false, isSupported: false},
		{value: "pt-BR", wellFormed: false, isSupported: false},
	} {
		if got := isWellFormedLocaleCode(test.value); got != test.wellFormed {
			t.Errorf("isWellFormedLocaleCode(%q) = %t, want %t", test.value, got, test.wellFormed)
		}
		if got := isSupportedPreferredLocale(test.value); got != test.isSupported {
			t.Errorf("isSupportedPreferredLocale(%q) = %t, want %t", test.value, got, test.isSupported)
		}
	}
}
