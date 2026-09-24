package product

import (
	"encoding/json"
	"reflect"
	"testing"

	"askolo/backend/internal/adapters/postgres"
)

func TestProfileUserResponseUsesClientFieldNames(t *testing.T) {
	user := postgres.User{
		ID:                "user-1",
		Email:             "person@example.test",
		FirstName:         "Ada",
		LastName:          "Lovelace",
		ProfileImageURL:   "https://example.test/avatar.png",
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
		"status":          "active",
		"authProvider":    "google",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profile response = %#v, want %#v", got, want)
	}
}
