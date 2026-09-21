package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"strings"
	"time"

	"askolo/backend/internal/config"
)

var ErrEmailDeliveryNotConfigured = errors.New("email delivery is not configured")

type EmailMessage struct {
	To      string
	Subject string
	Body    string
}

type EmailSender interface {
	Send(context.Context, EmailMessage) error
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
		return errors.New("email address contains invalid header characters")
	}
	if s.port < 1 || s.port > 65535 {
		return errors.New("SMTP port is invalid")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(s.host, fmt.Sprintf("%d", s.port)))
	if err != nil {
		return fmt.Errorf("connect to SMTP server: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("create SMTP client: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: s.host}); err != nil {
			return fmt.Errorf("start SMTP TLS: %w", err)
		}
	} else if s.username != "" {
		return errors.New("SMTP server does not support required TLS")
	}

	if s.username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return fmt.Errorf("authenticate with SMTP server: %w", err)
		}
	}
	if err := client.Mail(s.from); err != nil {
		return fmt.Errorf("set SMTP sender: %w", err)
	}
	if err := client.Rcpt(message.To); err != nil {
		return fmt.Errorf("set SMTP recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("open SMTP message: %w", err)
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
		return fmt.Errorf("write SMTP message: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("finish SMTP message: %w", closeErr)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("close SMTP session: %w", err)
	}
	return nil
}
