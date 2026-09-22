package auth

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
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
	return &resendEmailSender{
		apiKey:   strings.TrimSpace(apiKey),
		from:     strings.TrimSpace(from),
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
		HTML:    brandedEmailHTML(message.Body),
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

func brandedEmailHTML(body string) string {
	escapedBody := html.EscapeString(strings.ReplaceAll(body, "\r\n", "\n"))
	escapedBody = strings.ReplaceAll(escapedBody, "\n", "<br>\n")
	return `<div style="font-family:Arial,sans-serif;line-height:1.6;color:#17202a;max-width:600px">` +
		`<div style="font-weight:700;font-size:20px;margin-bottom:24px">Askolo</div>` +
		`<div>` + escapedBody + `</div>` +
		`</div>`
}

type smtpEmailSender struct {
	host     string
	port     int
	username string
	password string
	from     string
}

func NewSMTPEmailSender(cfg config.Config) EmailSender {
	return &smtpEmailSender{
		host:     cfg.Email.SMTPHost,
		port:     cfg.Email.SMTPPort,
		username: cfg.Email.SMTPUsername,
		password: cfg.Email.SMTPPassword,
		from:     cfg.Email.FromAddress,
	}
}

func (s *smtpEmailSender) Send(ctx context.Context, message EmailMessage) error {
	if s == nil || strings.TrimSpace(s.host) == "" || strings.TrimSpace(s.from) == "" {
		return ErrEmailDeliveryNotConfigured
	}
	if strings.ContainsAny(s.from, "\r\n") || strings.ContainsAny(message.To, "\r\n") {
		return emailDeliveryError(EmailDeliveryConfigurationInvalid, true, errors.New("email address contains invalid header characters"))
	}
	if s.port < 1 || s.port > 65535 {
		return emailDeliveryError(EmailDeliveryConfigurationInvalid, true, errors.New("SMTP port is invalid"))
	}
	select {
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return emailDeliveryError(EmailDeliveryTimeout, true, ctx.Err())
		}
		return emailDeliveryError(EmailDeliveryConnectionFailure, true, ctx.Err())
	default:
	}

	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(s.host, fmt.Sprintf("%d", s.port)))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || isNetTimeout(err) {
			return emailDeliveryError(EmailDeliveryTimeout, true, err)
		}
		return emailDeliveryError(EmailDeliveryConnectionFailure, true, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return emailDeliveryError(EmailDeliveryConnectionFailure, true, err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: s.host}); err != nil {
			if isNetTimeout(err) {
				return emailDeliveryError(EmailDeliveryTimeout, true, err)
			}
			return emailDeliveryError(EmailDeliveryConnectionFailure, true, err)
		}
	} else if s.username != "" {
		return emailDeliveryError(EmailDeliveryConfigurationInvalid, true, errors.New("SMTP server does not support required TLS"))
	}

	if s.username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return emailDeliveryError(EmailDeliveryAuthenticationFailure, true, err)
		}
	}
	if err := client.Mail(s.from); err != nil {
		return emailDeliveryError(EmailDeliveryProviderRejection, true, err)
	}
	if err := client.Rcpt(message.To); err != nil {
		return emailDeliveryError(EmailDeliveryProviderRejection, true, err)
	}
	writer, err := client.Data()
	if err != nil {
		return emailDeliveryError(EmailDeliveryProviderRejection, true, err)
	}
	_, writeErr := io.WriteString(writer, strings.Join([]string{
		"From: " + s.from,
		"To: " + message.To,
		"Subject: " + message.Subject,
		"Content-Type: text/plain; charset=utf-8",
		"",
		message.Body,
		"",
	}, "\r\n"))
	closeErr := writer.Close()
	if writeErr != nil {
		return emailDeliveryError(EmailDeliveryProviderRejection, false, writeErr)
	}
	if closeErr != nil {
		return emailDeliveryError(EmailDeliveryProviderRejection, false, closeErr)
	}
	return nil
}

func isNetTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
