package websocket

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAllowedOriginRequiresExactCanonicalOrigin(t *testing.T) {
	handler := New(nil, "askolo-backend", nil, "askolo_session", "https://assistant.example", strings.Repeat("x", 32), nil)
	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		{name: "exact", origin: "https://assistant.example", want: true},
		{name: "different host", origin: "https://assistant.example.evil.test"},
		{name: "different scheme", origin: "http://assistant.example"},
		{name: "path", origin: "https://assistant.example/"},
		{name: "userinfo", origin: "https://user@assistant.example"},
		{name: "suffix port", origin: "https://assistant.example:444"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "https://assistant.example/ws", nil)
			request.Header.Set("Origin", test.origin)
			_, allowed := handler.allowedOrigin(request)
			if allowed != test.want {
				t.Fatalf("allowedOrigin(%q) = %v, want %v", test.origin, allowed, test.want)
			}
		})
	}
}

func TestAllowedOriginDevelopmentFallbackDoesNotTrustForwardedHost(t *testing.T) {
	handler := New(nil, "askolo-backend", nil, "askolo_session", "", strings.Repeat("x", 32), nil)
	tests := []struct {
		name   string
		host   string
		origin string
		xff    string
		want   bool
	}{
		{name: "same public host", host: "assistant.example", origin: "https://assistant.example", want: true},
		{name: "local development", host: "localhost:5173", origin: "http://localhost:5173", want: true},
		{name: "cross host", host: "assistant.example", origin: "https://other.example"},
		{name: "untrusted forwarded host", host: "assistant.example", origin: "https://other.example", xff: "other.example"},
		{name: "plain http public host", host: "assistant.example", origin: "http://assistant.example"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "https://"+test.host+"/ws", nil)
			request.Host = test.host
			request.Header.Set("Origin", test.origin)
			if test.xff != "" {
				request.Header.Set("X-Forwarded-Host", test.xff)
			}
			_, allowed := handler.allowedOrigin(request)
			if allowed != test.want {
				t.Fatalf("allowedOrigin(%q, host %q) = %v, want %v", test.origin, test.host, allowed, test.want)
			}
		})
	}
}

func TestDecodeClientEnvelopeRequiresVersionedStrictJSON(t *testing.T) {
	tests := []struct {
		name  string
		value string
		valid bool
	}{
		{name: "valid start", value: `{"version":1,"type":"session.start","sequence":1,"payload":{}}`, valid: true},
		{name: "unknown top-level field", value: `{"version":1,"type":"session.start","sequence":1,"owner":"someone"}`},
		{name: "wrong version", value: `{"version":2,"type":"session.start","sequence":1}`},
		{name: "missing sequence", value: `{"version":1,"type":"session.start"}`},
		{name: "non-canonical correlation", value: `{"version":1,"type":"session.start","sequence":1,"correlationId":" id "}`},
		{name: "trailing JSON", value: `{"version":1,"type":"session.start","sequence":1} {}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, valid := decodeClientEnvelope([]byte(test.value))
			if valid != test.valid {
				t.Fatalf("decodeClientEnvelope valid = %v, want %v", valid, test.valid)
			}
		})
	}
}
