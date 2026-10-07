package auth

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"askolo/backend/internal/config"
)

const (
	turnstileTestSecret = "unit-test-turnstile-secret"
	turnstileTestToken  = "unit-test-turnstile-token"
)

func TestTurnstileSiteverifyChecksSuccessActionAndExactHostname(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		status     int
		wantOK     bool
		wantStatus int
	}{
		{
			name:       "accepted token",
			response:   `{"success":true,"action":"signup","hostname":"web.askolo.app"}`,
			status:     http.StatusOK,
			wantOK:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid token",
			response:   `{"success":false,"action":"signup","hostname":"web.askolo.app"}`,
			status:     http.StatusOK,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "action mismatch",
			response:   `{"success":true,"action":"login","hostname":"web.askolo.app"}`,
			status:     http.StatusOK,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "hostname mismatch",
			response:   `{"success":true,"action":"signup","hostname":"attacker.example"}`,
			status:     http.StatusOK,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "provider error",
			response:   `{"error":"` + turnstileTestSecret + ` ` + turnstileTestToken + `"}`,
			status:     http.StatusBadGateway,
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "malformed provider response",
			response:   `{not-json`,
			status:     http.StatusOK,
			wantStatus: http.StatusServiceUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("Siteverify method = %s, want POST", r.Method)
				}
				if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
					t.Errorf("Siteverify content type = %q", r.Header.Get("Content-Type"))
				}
				form, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read Siteverify form: %v", err)
					return
				}
				values, err := url.ParseQuery(string(form))
				if err != nil {
					t.Errorf("parse Siteverify form: %v", err)
					return
				}
				if got := values.Get("secret"); got != turnstileTestSecret {
					t.Errorf("Siteverify secret = %q, want configured test secret", got)
				}
				if got := values.Get("response"); got != turnstileTestToken {
					t.Errorf("Siteverify token = %q, want submitted test token", got)
				}
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.response)
			}))
			defer server.Close()

			handler := newTurnstileTestHandler(server, &logs)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/auth/password/signup", nil)
			ok := handler.requireTurnstile(recorder, request, turnstileTestToken, "signup", "password_signup")
			if ok != test.wantOK {
				t.Fatalf("requireTurnstile() = %v, want %v", ok, test.wantOK)
			}
			if recorder.Code != test.wantStatus {
				t.Fatalf("response status = %d, want %d; body=%s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			for _, sensitiveValue := range []string{turnstileTestToken, turnstileTestSecret} {
				if strings.Contains(recorder.Body.String(), sensitiveValue) || strings.Contains(logs.String(), sensitiveValue) {
					t.Fatalf("sensitive Turnstile value leaked in response or logs")
				}
			}
			if !test.wantOK && test.wantStatus == http.StatusForbidden &&
				!strings.Contains(recorder.Body.String(), turnstileFailureMessage) {
				t.Fatalf("invalid challenge response is not generic: %s", recorder.Body.String())
			}
		})
	}
}

func TestTurnstileSiteverifyTimeoutFailsClosed(t *testing.T) {
	var logs bytes.Buffer

	handler := NewHandler(config.Config{
		Environment:             "test",
		AuthRateLimitHMACSecret: "unit-test-auth-rate-limit-hmac-secret-123456789",
		TurnstileSecret:         turnstileTestSecret,
		TurnstileAllowedHostnames: map[string]struct{}{
			"web.askolo.app": {},
		},
	}, nil, slog.New(slog.NewTextHandler(&logs, nil)))
	handler.turnstileClient = &http.Client{
		Timeout:   20 * time.Millisecond,
		Transport: turnstileRequestContextRoundTripper{},
	}
	recorder := httptest.NewRecorder()
	ok := handler.requireTurnstile(
		recorder,
		httptest.NewRequest(http.MethodPost, "/api/auth/password/signup", nil),
		turnstileTestToken,
		"signup",
		"password_signup",
	)
	if ok || recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("timeout result = (%v, %d), want (false, %d)", ok, recorder.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(recorder.Body.String(), turnstileUnavailableMessage) {
		t.Fatalf("timeout response is not generic: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), turnstileTestToken) ||
		strings.Contains(logs.String(), turnstileTestToken) ||
		strings.Contains(logs.String(), turnstileTestSecret) {
		t.Fatal("Turnstile token or secret leaked on timeout")
	}
}

func TestTurnstileRejectsMissingTokenAndMissingSecretBeforeProviderRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = io.WriteString(w, `{"success":true,"action":"signup","hostname":"web.askolo.app"}`)
	}))
	defer server.Close()

	handler := newTurnstileTestHandler(server, io.Discard)
	recorder := httptest.NewRecorder()
	if handler.requireTurnstile(recorder, httptest.NewRequest(http.MethodPost, "/", nil), "", "signup", "password_signup") {
		t.Fatal("missing token was accepted")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("missing token status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	handler.cfg.TurnstileSecret = ""
	recorder = httptest.NewRecorder()
	if handler.requireTurnstile(recorder, httptest.NewRequest(http.MethodPost, "/", nil), turnstileTestToken, "signup", "password_signup") {
		t.Fatal("missing configured secret was accepted")
	}
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing secret status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if requests != 0 {
		t.Fatalf("Siteverify requests = %d, want 0", requests)
	}
}

func TestPasswordSignupHandlerRejectsActionMismatchBeforeAccountCreation(t *testing.T) {
	var logs bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,"action":"login","hostname":"web.askolo.app"}`)
	}))
	defer server.Close()

	handler := newTurnstileTestHandler(server, &logs)
	handler.cfg.Email.ChallengeSecret = "unit-test-challenge-secret"
	body := `{"email":"new-user@example.com","password":"safe-password-123","turnstileToken":"` + turnstileTestToken + `"}`
	request := httptest.NewRequest(http.MethodPost, "/api/auth/password/signup", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("signup status = %d, want %d; body=%s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), turnstileFailureMessage) {
		t.Fatalf("signup rejection is not generic: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), turnstileTestToken) ||
		strings.Contains(recorder.Body.String(), turnstileTestSecret) ||
		strings.Contains(logs.String(), turnstileTestToken) ||
		strings.Contains(logs.String(), turnstileTestSecret) {
		t.Fatal("Turnstile token or secret leaked from handler rejection")
	}
}

func TestPasswordSignupHandlerContinuesAfterAcceptedChallenge(t *testing.T) {
	var logs bytes.Buffer
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,"action":"signup","hostname":"web.askolo.app"}`)
	}))
	defer server.Close()

	handler := newTurnstileTestHandler(server, &logs)
	request := httptest.NewRequest(http.MethodPost, "/api/auth/password/signup", strings.NewReader(
		`{"email":"new-user@example.com","password":"safe-password-123","turnstileToken":"`+turnstileTestToken+`"}`,
	))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("accepted challenge did not continue into signup validation: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), turnstileTestToken) ||
		strings.Contains(recorder.Body.String(), turnstileTestSecret) ||
		strings.Contains(logs.String(), turnstileTestToken) ||
		strings.Contains(logs.String(), turnstileTestSecret) {
		t.Fatal("Turnstile token or secret leaked after accepted challenge")
	}
}

type turnstileRequestContextRoundTripper struct{}

func (turnstileRequestContextRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	<-request.Context().Done()
	return nil, request.Context().Err()
}

func newTurnstileTestHandler(server *httptest.Server, logs io.Writer) *Handler {
	return newTurnstileTestHandlerWithConfig(server, logs, config.Config{
		Environment:               "test",
		AuthRateLimitHMACSecret:   "unit-test-auth-rate-limit-hmac-secret-123456789",
		TurnstileSecret:           turnstileTestSecret,
		TurnstileAllowedHostnames: map[string]struct{}{"web.askolo.app": {}},
	})
}

func newTurnstileTestHandlerWithConfig(server *httptest.Server, logs io.Writer, cfg config.Config) *Handler {
	logger := slog.New(slog.NewTextHandler(logs, nil))
	handler := NewHandler(cfg, nil, logger)
	handler.turnstileClient = server.Client()
	handler.turnstileEndpoint = server.URL
	return handler
}
