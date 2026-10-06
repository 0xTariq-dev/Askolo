package product

import (
	"strings"
	"testing"
)

func TestRedactAssistantSensitiveValues(t *testing.T) {
	input := strings.Join([]string{
		"Name: Maya Nasser. Client: Project Atlas.",
		"Email: maya@example.test. Phone: +1 (415) 555-2671.",
		"Card: 4111 1111 1111 1111. IBAN: DE89 3704 0044 0532 0130 00.",
		"SSN: 123-45-6789. Passport number: P12345678.",
		"Password: hunter2. DOB: 1990-01-02. Home address: 123 Main Street, Cairo.",
		"Meet Client Delta on 2026-10-06 at 09:30 for 2 hours. Launch Project Atlas.",
	}, " ")

	filtered := redactAssistantSensitiveValues(input)

	for _, preserved := range []string{
		"Maya Nasser",
		"Project Atlas",
		"Client Delta",
		"2026-10-06",
		"09:30",
		"2 hours",
	} {
		if !strings.Contains(filtered, preserved) {
			t.Errorf("redaction removed non-sensitive content %q from %q", preserved, filtered)
		}
	}

	for _, sensitive := range []string{
		"maya@example.test",
		"+1 (415) 555-2671",
		"4111 1111 1111 1111",
		"DE89 3704 0044 0532 0130 00",
		"123-45-6789",
		"P12345678",
		"hunter2",
		"1990-01-02",
		"123 Main Street",
		"Cairo",
	} {
		if strings.Contains(filtered, sensitive) {
			t.Errorf("sensitive content %q was not redacted from %q", sensitive, filtered)
		}
	}
}
