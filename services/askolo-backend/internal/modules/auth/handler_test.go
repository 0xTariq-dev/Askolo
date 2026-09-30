package auth

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"askolo/backend/internal/config"
)

const testAuthRateLimitHMACSecret = "test-auth-rate-limit-hmac-secret-at-least-32-bytes"

func TestAuthRateLimitUsesRemotePeerNotForwardedHeader(t *testing.T) {
	handler := NewHandler(config.Config{AuthRateLimitHMACSecret: testAuthRateLimitHMACSecret}, nil, nil)
	for attempt := 1; attempt <= 2; attempt++ {
		request := httptest.NewRequest("POST", "/api/auth/password/login", nil)
		request.RemoteAddr = "192.0.2.10:1234"
		request.Header.Set("X-Forwarded-For", "198.51.100."+strconv.Itoa(attempt))
		if !handler.allowScoped(request, "password-login", 2, time.Minute) {
			t.Fatalf("attempt %d was unexpectedly rate limited", attempt)
		}
	}

	request := httptest.NewRequest("POST", "/api/auth/password/login", nil)
	request.RemoteAddr = "192.0.2.10:5678"
	request.Header.Set("X-Forwarded-For", "203.0.113.99")
	if handler.allowScoped(request, "password-login", 2, time.Minute) {
		t.Fatal("request with spoofed forwarded address bypassed the remote-peer rate limit")
	}

	differentPeer := httptest.NewRequest("POST", "/api/auth/password/login", nil)
	differentPeer.RemoteAddr = "192.0.2.11:1234"
	if !handler.allowScoped(differentPeer, "password-login", 2, time.Minute) {
		t.Fatal("request from a different remote peer was unexpectedly rate limited")
	}
}

func TestAuthRateLimitTrustedProxyChainUsesActualClientIP(t *testing.T) {
	handler := NewHandler(config.Config{AuthRateLimitHMACSecret: testAuthRateLimitHMACSecret}, nil, nil)
	for attempt := 1; attempt <= 2; attempt++ {
		request := httptest.NewRequest("POST", "/api/auth/password/login", nil)
		request.RemoteAddr = "10.20.0.8:1234"
		request.Header.Set(
			"X-Forwarded-For",
			"203.0.113."+strconv.Itoa(attempt)+", 198.51.100.42, 10.20.0.9",
		)
		if got, want := requestClientIP(request), "198.51.100.42"; got != want {
			t.Fatalf("requestClientIP() = %q, want trusted-chain client %q", got, want)
		}
		if !handler.allowScoped(request, "password-login", 2, time.Minute) {
			t.Fatalf("attempt %d was unexpectedly rate limited", attempt)
		}
	}

	spoofedPrefix := httptest.NewRequest("POST", "/api/auth/password/login", nil)
	spoofedPrefix.RemoteAddr = "10.20.0.8:5678"
	spoofedPrefix.Header.Set("X-Forwarded-For", "192.0.2.250, 198.51.100.42, 10.20.0.9")
	if handler.allowScoped(spoofedPrefix, "password-login", 2, time.Minute) {
		t.Fatal("caller-controlled forwarded prefix bypassed the source rate limit")
	}

	differentClient := httptest.NewRequest("POST", "/api/auth/password/login", nil)
	differentClient.RemoteAddr = "10.20.0.8:5678"
	differentClient.Header.Set("X-Forwarded-For", "203.0.113.10")
	if !handler.allowScoped(differentClient, "password-login", 2, time.Minute) {
		t.Fatal("different forwarded client was unexpectedly rate limited")
	}
}

func TestAuthRateLimitBucketHashIsStableAndPurposeSeparated(t *testing.T) {
	handler := NewHandler(config.Config{AuthRateLimitHMACSecret: testAuthRateLimitHMACSecret}, nil, nil)
	email := normalizeEmail("  Person@Example.com ")

	first, err := handler.authRateLimitBucketHash("account:password-login", email)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := handler.authRateLimitBucketHash("account:password-login", "person@example.com")
	if err != nil {
		t.Fatal(err)
	}
	otherPurpose, err := handler.authRateLimitBucketHash("account:password-recovery", email)
	if err != nil {
		t.Fatal(err)
	}
	if first != repeated {
		t.Fatal("normalized account produced different HMAC buckets")
	}
	if first == otherPurpose {
		t.Fatal("different rate-limit purposes reused the same HMAC bucket")
	}
	if strings.Contains(first, "person@example.com") {
		t.Fatal("HMAC bucket contains the submitted account address")
	}
}

func TestAuthRateLimitScopesAreIndependent(t *testing.T) {
	handler := NewHandler(config.Config{AuthRateLimitHMACSecret: testAuthRateLimitHMACSecret}, nil, nil)
	request := httptest.NewRequest("POST", "/api/auth/password/login", nil)
	request.RemoteAddr = "192.0.2.20:1234"

	for attempt := 1; attempt <= 5; attempt++ {
		if !handler.allowScoped(request, "password-login", 5, time.Minute) {
			t.Fatalf("login attempt %d was unexpectedly rate limited", attempt)
		}
	}
	if handler.allowScoped(request, "password-login", 5, time.Minute) {
		t.Fatal("login scope admitted a request after its limit")
	}
	if !handler.allowScoped(request, "password-recovery-request", 5, time.Minute) {
		t.Fatal("login attempts incorrectly exhausted the password-recovery scope")
	}
}

func TestAuthRateLimiterCleansExpiredEntriesAndCapsMap(t *testing.T) {
	now := time.Now()
	limiter := &rateLimiter{
		entries: map[string]rateEntry{
			"expired": {count: 1, resetAt: now.Add(-time.Second)},
			"active":  {count: 1, resetAt: now.Add(time.Minute)},
		},
		cleanupAt: now.Add(-time.Second),
	}
	handler := &Handler{
		cfg:     config.Config{AuthRateLimitHMACSecret: testAuthRateLimitHMACSecret},
		limiter: limiter,
	}
	request := httptest.NewRequest("POST", "/api/auth/password/login", nil)
	request.RemoteAddr = "192.0.2.12:1234"
	if !handler.allowScoped(request, "password-login", 2, time.Minute) {
		t.Fatal("request was unexpectedly rate limited after expired entries were cleaned")
	}
	if _, ok := limiter.entries["expired"]; ok {
		t.Fatal("expired rate-limit entry was not removed")
	}

	fullEntries := make(map[string]rateEntry, authRateLimiterMaxEntries)
	for i := 0; i < authRateLimiterMaxEntries; i++ {
		fullEntries["peer-"+strconv.Itoa(i)] = rateEntry{count: 1, resetAt: now.Add(time.Minute)}
	}
	limiter = &rateLimiter{entries: fullEntries, cleanupAt: now.Add(time.Hour)}
	handler.limiter = limiter
	request = httptest.NewRequest("POST", "/api/auth/password/login", nil)
	request.RemoteAddr = "192.0.2.13:1234"
	if handler.allowScoped(request, "password-login", 2, time.Minute) {
		t.Fatal("new peer was admitted after the rate-limit map reached its cap")
	}
	if len(limiter.entries) != authRateLimiterMaxEntries {
		t.Fatalf("rate-limit map size = %d, want cap %d", len(limiter.entries), authRateLimiterMaxEntries)
	}
}

func TestRequestClientIPNormalizesRemoteAddress(t *testing.T) {
	request := httptest.NewRequest("POST", "/", nil)
	request.RemoteAddr = "[2001:db8::1]:54321"
	request.Header.Set("X-Forwarded-For", "198.51.100.42")

	if got, want := requestClientIP(request), "2001:db8::1"; got != want {
		t.Fatalf("requestClientIP() = %q, want %q", got, want)
	}
}
