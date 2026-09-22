package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"sync"
	"time"

	"askolo/backend/internal/config"
)

type EmailDeliveryState string

const (
	EmailDeliveryConfigurationMissing  EmailDeliveryState = "configuration_missing"
	EmailDeliveryConfigurationInvalid  EmailDeliveryState = "configuration_invalid"
	EmailDeliveryConnectionFailure     EmailDeliveryState = "connection_failure"
	EmailDeliveryAuthenticationFailure EmailDeliveryState = "authentication_failure"
	EmailDeliveryProviderRejection     EmailDeliveryState = "provider_rejection"
	EmailDeliveryTimeout               EmailDeliveryState = "timeout"
	EmailDeliveryHandoff               EmailDeliveryState = "handoff"
	EmailDeliveryInternalFailure       EmailDeliveryState = "internal_failure"
)

type EmailDeliveryError struct {
	State     EmailDeliveryState
	RetrySafe bool
	Err       error
}

func (e *EmailDeliveryError) Error() string {
	if e == nil {
		return ""
	}
	return "email delivery " + string(e.State)
}

func (e *EmailDeliveryError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

var ErrEmailDeliveryNotConfigured = &EmailDeliveryError{
	State:     EmailDeliveryConfigurationMissing,
	RetrySafe: true,
	Err:       errors.New("email delivery is not configured"),
}

func emailDeliveryError(state EmailDeliveryState, retrySafe bool, err error) error {
	if err == nil {
		return nil
	}
	return &EmailDeliveryError{State: state, RetrySafe: retrySafe, Err: err}
}

func emailDeliveryState(err error) EmailDeliveryState {
	if err == nil {
		return EmailDeliveryHandoff
	}
	var deliveryErr *EmailDeliveryError
	if errors.As(err, &deliveryErr) && deliveryErr != nil {
		return deliveryErr.State
	}
	return EmailDeliveryInternalFailure
}

func normalizeEmailDeliveryError(err error) error {
	if err == nil {
		return nil
	}
	var deliveryErr *EmailDeliveryError
	if errors.As(err, &deliveryErr) && deliveryErr != nil {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) || isNetTimeout(err) {
		return emailDeliveryError(EmailDeliveryTimeout, true, err)
	}
	// EmailSender is an adapter boundary. An adapter error that does not
	// provide a more specific state is treated as a provider rejection and
	// can be retried after the persisted challenge is released.
	return emailDeliveryError(EmailDeliveryProviderRejection, true, err)
}

func emailDeliveryRetrySafe(err error) bool {
	var deliveryErr *EmailDeliveryError
	return errors.As(err, &deliveryErr) && deliveryErr != nil && deliveryErr.RetrySafe
}

type EmailDeliveryMonitor struct {
	mu                  sync.RWMutex
	attempts            uint64
	handoffs            uint64
	failures            map[EmailDeliveryState]uint64
	consecutiveFailures uint64
	lastOutcome         EmailDeliveryState
	lastLatency         time.Duration
}

type EmailDeliverySnapshot struct {
	Attempts            uint64                        `json:"deliveryAttempts"`
	Handoffs            uint64                        `json:"deliveryHandoffs"`
	Failures            map[EmailDeliveryState]uint64 `json:"deliveryFailures"`
	ConsecutiveFailures uint64                        `json:"consecutiveFailures"`
	LastOutcome         EmailDeliveryState            `json:"lastOutcome,omitempty"`
	LastLatencyMS       int64                         `json:"lastLatencyMs"`
}

func NewEmailDeliveryMonitor() *EmailDeliveryMonitor {
	return &EmailDeliveryMonitor{failures: make(map[EmailDeliveryState]uint64)}
}

func (m *EmailDeliveryMonitor) Observe(outcome EmailDeliveryState, latency time.Duration) {
	if m == nil {
		return
	}
	if latency < 0 {
		latency = 0
	}
	const maxObservedLatency = 30 * time.Second
	if latency > maxObservedLatency {
		latency = maxObservedLatency
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.attempts++
	m.lastOutcome = outcome
	m.lastLatency = latency
	if outcome == EmailDeliveryHandoff {
		m.handoffs++
		m.consecutiveFailures = 0
		return
	}
	if m.failures == nil {
		m.failures = make(map[EmailDeliveryState]uint64)
	}
	m.failures[outcome]++
	m.consecutiveFailures++
}

func (m *EmailDeliveryMonitor) Snapshot() EmailDeliverySnapshot {
	snapshot := EmailDeliverySnapshot{Failures: map[EmailDeliveryState]uint64{}}
	if m == nil {
		return snapshot
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	snapshot.Attempts = m.attempts
	snapshot.Handoffs = m.handoffs
	snapshot.ConsecutiveFailures = m.consecutiveFailures
	snapshot.LastOutcome = m.lastOutcome
	snapshot.LastLatencyMS = m.lastLatency.Milliseconds()
	for state, count := range m.failures {
		snapshot.Failures[state] = count
	}
	return snapshot
}

type EmailMessage struct {
	To      string
	Subject string
	Body    string
	Code    string
}

type EmailSender interface {
	Send(context.Context, EmailMessage) error
}

const (
	resendEmailEndpoint       = "https://api.resend.com/emails"
	resendRequestTimeout      = 15 * time.Second
	resendResponseBodyMaxSize = 64 * 1024
)

type resendEmailSender struct {
	apiKey   string
	from     string
	endpoint string
	client   *http.Client
}

type resendEmailRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	HTML    string   `json:"html"`
}

type resendEmailResponse struct {
	ID string `json:"id"`
}

func NewResendEmailSender(cfg config.Config) EmailSender {
	return newResendEmailSender(
		cfg.Email.ResendAPIKey,
		cfg.Email.FromAddress,
		resendEmailEndpoint,
		&http.Client{Timeout: resendRequestTimeout},
	)
}

func newResendEmailSender(apiKey, from, endpoint string, client *http.Client) EmailSender {
	if client == nil {
		client = &http.Client{Timeout: resendRequestTimeout}
	}
	from = askoloFromAddress(from)
	return &resendEmailSender{
		apiKey:   strings.TrimSpace(apiKey),
		from:     from,
		endpoint: endpoint,
		client:   client,
	}
}

func (s *resendEmailSender) Send(ctx context.Context, message EmailMessage) error {
	if s == nil || s.apiKey == "" || s.from == "" {
		return ErrEmailDeliveryNotConfigured
	}
	if strings.ContainsAny(s.apiKey, "\r\n") ||
		!validEmailAddress(s.from, true) || !validEmailAddress(message.To, false) ||
		strings.TrimSpace(message.Subject) == "" || strings.ContainsAny(message.Subject, "\r\n") {
		return emailDeliveryError(
			EmailDeliveryConfigurationInvalid,
			true,
			errors.New("email message contains invalid values"),
		)
	}

	payload, err := json.Marshal(resendEmailRequest{
		From:    s.from,
		To:      []string{strings.TrimSpace(message.To)},
		Subject: message.Subject,
		Text:    message.Body,
		HTML:    brandedEmailHTML(message),
	})
	if err != nil {
		return emailDeliveryError(EmailDeliveryInternalFailure, false, err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, strings.NewReader(string(payload)))
	if err != nil {
		return emailDeliveryError(EmailDeliveryConfigurationInvalid, true, err)
	}
	request.Header.Set("Authorization", "Bearer "+s.apiKey)
	request.Header.Set("Content-Type", "application/json")

	response, err := s.client.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isNetTimeout(err) {
			return emailDeliveryError(EmailDeliveryTimeout, false, err)
		}
		if errors.Is(err, context.Canceled) {
			return emailDeliveryError(EmailDeliveryConnectionFailure, false, err)
		}
		return emailDeliveryError(EmailDeliveryConnectionFailure, false, err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		retrySafe := response.StatusCode < http.StatusInternalServerError
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			return emailDeliveryError(EmailDeliveryAuthenticationFailure, retrySafe, fmt.Errorf("resend returned status %d", response.StatusCode))
		}
		return emailDeliveryError(EmailDeliveryProviderRejection, retrySafe, fmt.Errorf("resend returned status %d", response.StatusCode))
	}

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, resendResponseBodyMaxSize))
	if err != nil {
		return emailDeliveryError(EmailDeliveryInternalFailure, false, err)
	}
	var result resendEmailResponse
	if err := json.Unmarshal(responseBody, &result); err != nil || !validProviderMessageID(result.ID) {
		if err == nil {
			err = errors.New("resend response did not include a valid message id")
		}
		return emailDeliveryError(EmailDeliveryInternalFailure, false, err)
	}
	return nil
}

func askoloFromAddress(value string) string {
	value = strings.TrimSpace(value)
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address == "" {
		return value
	}
	return "Askolo <" + parsed.Address + ">"
}

func validEmailAddress(value string, allowDisplayName bool) bool {
	value = strings.TrimSpace(value)
	if value == "" || strings.ContainsAny(value, "\r\n") {
		return false
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address == "" {
		return false
	}
	return allowDisplayName || parsed.Address == value
}

func validProviderMessageID(value string) bool {
	return value != "" && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\r\n")
}

func brandedEmailHTML(message EmailMessage) string {
	body := strings.ReplaceAll(message.Body, "\r\n", "\n")
	if message.Code != "" {
		body = strings.Replace(body, "Your one-time code is: "+message.Code, "", 1)
	}
	body = strings.Replace(body, " If you did not request this, you can ignore this message.", "", 1)

	codeBlock := ""
	if message.Code != "" {
		codeBlock = `<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="margin:28px 0">` +
			`<tr><td align="center" style="background:#eef5ff;border:1px solid #cfe0ff;border-radius:14px;padding:20px">` +
			`<div style="font-family:Arial,sans-serif;font-size:11px;font-weight:700;letter-spacing:1.8px;color:#54709a;text-transform:uppercase">Your verification code</div>` +
			`<div style="font-family:Arial,sans-serif;font-size:36px;font-weight:700;letter-spacing:10px;line-height:1.2;color:#173f7a;margin:10px 0 0 10px">` +
			html.EscapeString(message.Code) +
			`</div></td></tr></table>`
	}

	return `<!doctype html><html lang="en"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>` + html.EscapeString(message.Subject) + `</title></head>` +
		`<body style="margin:0;padding:0;background:#f4f7fb">` +
		`<div style="display:none;max-height:0;overflow:hidden;opacity:0;color:transparent">` +
		`Your Askolo verification code</div>` +
		`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#f4f7fb">` +
		`<tr><td align="center" style="padding:32px 16px">` +
		`<table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:560px;background:#fff;border:1px solid #e3eaf3;border-radius:20px">` +
		`<tr><td style="padding:40px 36px 32px">` +
		`<div style="font-family:Arial,sans-serif;font-size:18px;font-weight:800;letter-spacing:2.5px;color:#2367d1">ASKOLO</div>` +
		`<div style="font-family:Arial,sans-serif;font-size:11px;font-weight:700;letter-spacing:1.6px;color:#7b8da5;text-transform:uppercase;margin-top:28px">Secure account access</div>` +
		`<h1 style="font-family:Arial,sans-serif;font-size:28px;line-height:1.2;color:#17202a;margin:10px 0 20px">` +
		html.EscapeString(message.Subject) +
		`</h1>` +
		emailBodyHTML(body) +
		codeBlock +
		`<p style="font-family:Arial,sans-serif;font-size:13px;line-height:1.6;color:#718096;margin:28px 0 0">` +
		`If you did not request this email, you can safely ignore it.</p>` +
		`</td></tr><tr><td style="border-top:1px solid #edf1f6;padding:20px 36px">` +
		`<p style="font-family:Arial,sans-serif;font-size:12px;line-height:1.5;color:#9aa8ba;margin:0">` +
		`Askolo · Secure access for your account</p>` +
		`</td></tr></table></td></tr></table></body></html>`
}

func emailBodyHTML(body string) string {
	var markup strings.Builder
	for _, paragraph := range strings.Split(strings.TrimSpace(body), "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		escaped := html.EscapeString(paragraph)
		escaped = strings.ReplaceAll(escaped, "\n", "<br>\n")
		markup.WriteString(`<p style="font-family:Arial,sans-serif;font-size:16px;line-height:1.6;color:#3c4a5c;margin:0 0 16px">`)
		markup.WriteString(escaped)
		markup.WriteString(`</p>`)
	}
	return markup.String()
}

func isNetTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
