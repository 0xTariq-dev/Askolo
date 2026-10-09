package google

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestGmailTextBodyPrefersPlainTextAlternative(t *testing.T) {
	plain := "This is a plain email body."
	markup := "<html><body><b>HTML-only text</b></body></html>"
	message := gmailMessage{Payload: gmailPayload{
		MIMEType: "multipart/alternative",
		Parts: []gmailPayload{
			{
				MIMEType: "text/html",
				Body: struct {
					Data string `json:"data"`
				}{Data: base64.RawURLEncoding.EncodeToString([]byte(markup))},
			},
			{
				MIMEType: "text/plain",
				Body: struct {
					Data string `json:"data"`
				}{Data: base64.RawURLEncoding.EncodeToString([]byte(plain))},
			},
		},
	}}
	if got := gmailTextBody(message.Payload); got != plain {
		t.Fatalf("gmailTextBody() = %q; want %q", got, plain)
	}
}

func TestGmailTextBodyStripsMarkupFromHTMLFallback(t *testing.T) {
	markup := `<html><head><style>.hidden{display:none}</style></head><body><script>bad()</script><p>Read &amp; review</p><div>Next item</div></body></html>`
	payload := gmailPayload{
		MIMEType: "text/html",
		Body: struct {
			Data string `json:"data"`
		}{Data: base64.RawURLEncoding.EncodeToString([]byte(markup))},
	}
	got := gmailTextBody(payload)
	if strings.Contains(got, "<") || strings.Contains(got, "bad()") {
		t.Fatalf("HTML fallback was not reduced to text: %q", got)
	}
	if !strings.Contains(got, "Read & review") || !strings.Contains(got, "Next item") {
		t.Fatalf("HTML fallback lost readable text: %q", got)
	}
}

func TestSharedEmailMetadataDoesNotIncludeBody(t *testing.T) {
	message := gmailMessage{
		ID:           "message-id",
		InternalDate: "1791590400000",
		Payload: gmailPayload{
			Headers: []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			}{{Name: "Subject", Value: "Hello"}},
			MIMEType: "text/plain",
			Body: struct {
				Data string `json:"data"`
			}{Data: base64.RawURLEncoding.EncodeToString([]byte("private content"))},
		},
	}
	got := toSharedEmailMessage(message, false)
	if got.Subject != "Hello" || got.BodyText != "" {
		t.Fatalf("metadata projection = %+v; body must be omitted", got)
	}
}
