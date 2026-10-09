package product

import "testing"

func TestV2AgendaCursorIsBoundToItsScope(t *testing.T) {
	const signingSecret = "test-auth-rate-limit-hmac-secret-32-bytes"
	scope := v2AgendaScope(
		"user-1",
		"space-1",
		"member",
		"2026-10-01",
		"2026-11-11",
		"Africa/Cairo",
	)
	want := v2AgendaCursor{
		Version:             1,
		Scope:               scope,
		TaskOffset:          100,
		TaskItemOffset:      500,
		PlanEntryOffset:     500,
		CalendarEventOffset: 1000,
		PlanEntriesDone:     true,
	}
	token, err := encodeV2AgendaCursor(want, signingSecret)
	if err != nil {
		t.Fatalf("encode agenda cursor: %v", err)
	}
	got, err := decodeV2AgendaCursor(token, scope, signingSecret)
	if err != nil {
		t.Fatalf("decode agenda cursor: %v", err)
	}
	if got != want {
		t.Fatalf("decoded cursor = %#v, want %#v", got, want)
	}
	if _, err := decodeV2AgendaCursor(token, v2AgendaScope(
		"user-2",
		"space-1",
		"member",
		"2026-10-01",
		"2026-11-11",
		"Africa/Cairo",
	), signingSecret); err == nil {
		t.Fatal("cursor was accepted for a different user")
	}
	if _, err := decodeV2AgendaCursor(token, v2AgendaScope(
		"user-1",
		"space-1",
		"member",
		"2026-10-01",
		"2026-11-11",
		"UTC",
	), signingSecret); err == nil {
		t.Fatal("cursor was accepted for a different time zone")
	}
	if _, err := decodeV2AgendaCursor(token, scope, "another-test-signing-key-with-32-bytes"); err == nil {
		t.Fatal("cursor was accepted with a different signing key")
	}
	tamperedToken := token[:len(token)-1] + "A"
	if tamperedToken == token {
		tamperedToken = token[:len(token)-1] + "B"
	}
	if _, err := decodeV2AgendaCursor(tamperedToken, scope, signingSecret); err == nil {
		t.Fatal("tampered cursor was accepted")
	}
}

func TestValidV2TimeZone(t *testing.T) {
	for _, zone := range []string{"UTC", "Africa/Cairo", "America/Los_Angeles"} {
		if !validV2TimeZone(zone) {
			t.Errorf("validV2TimeZone(%q) = false", zone)
		}
	}
	for _, zone := range []string{"", "Local", " UTC", "Not/A_Time_Zone"} {
		if validV2TimeZone(zone) {
			t.Errorf("validV2TimeZone(%q) = true", zone)
		}
	}
}
