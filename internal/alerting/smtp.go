package alerting

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

const (
	TLSModeSTARTTLS = "starttls"
	TLSModeImplicit = "implicit"
	TLSModeNone     = "none"
)

type SMTPConfig struct {
	Host     string
	Port     int
	TLSMode  string
	Username string
	Password string
	From     string
	Timeout  time.Duration
}

type SMTPSender struct {
	host     string
	port     int
	tlsMode  string
	username string
	password string
	from     *mail.Address
	timeout  time.Duration

	dialContext func(context.Context, string, string) (net.Conn, error)
}

func NewSMTPSender(cfg SMTPConfig) (*SMTPSender, error) {
	tlsMode := strings.ToLower(strings.TrimSpace(cfg.TLSMode))
	switch tlsMode {
	case "", TLSModeSTARTTLS:
		tlsMode = TLSModeSTARTTLS
	case TLSModeImplicit, TLSModeNone:
	default:
		return nil, fmt.Errorf("unsupported SMTP TLS mode %q", cfg.TLSMode)
	}
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, fmt.Errorf("SMTP host is required")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("SMTP port must be between 1 and 65535")
	}
	if cfg.Timeout <= 0 {
		return nil, fmt.Errorf("SMTP timeout must be positive")
	}
	username := strings.TrimSpace(cfg.Username)
	password := strings.TrimSpace(cfg.Password)
	if (username == "") != (password == "") {
		return nil, fmt.Errorf("SMTP username and password must be set together")
	}
	from, err := mail.ParseAddress(strings.TrimSpace(cfg.From))
	if err != nil {
		return nil, fmt.Errorf("parse SMTP from address: %w", err)
	}
	return &SMTPSender{
		host:     strings.TrimSpace(cfg.Host),
		port:     cfg.Port,
		tlsMode:  tlsMode,
		username: username,
		password: password,
		from:     from,
		timeout:  cfg.Timeout,
	}, nil
}

func (s *SMTPSender) Send(ctx context.Context, email Email) error {
	recipients, err := parseRecipientAddresses(email.To)
	if err != nil {
		return err
	}
	message, err := s.buildMessage(recipients, email.Subject, email.Body)
	if err != nil {
		return err
	}

	address := net.JoinHostPort(s.host, fmt.Sprintf("%d", s.port))
	conn, err := s.dial(ctx, address)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.SetDeadline(connectionDeadline(ctx, s.timeout)); err != nil {
		return fmt.Errorf("set SMTP deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("create SMTP client: %w", err)
	}
	defer client.Close()

	if s.tlsMode == TLSModeSTARTTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("SMTP server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: s.host}); err != nil {
			return fmt.Errorf("start SMTP TLS: %w", err)
		}
	}

	if s.username != "" {
		auth := smtp.PlainAuth("", s.username, s.password, s.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP auth: %w", err)
		}
	}

	if err := client.Mail(s.from.Address); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %w", err)
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(recipient.Address); err != nil {
			return fmt.Errorf("SMTP RCPT TO %s: %w", recipient.Address, err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write SMTP message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close SMTP message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("SMTP QUIT: %w", err)
	}
	return nil
}

func (s *SMTPSender) dial(ctx context.Context, address string) (net.Conn, error) {
	if s.dialContext != nil {
		conn, err := s.dialContext(ctx, "tcp", address)
		if err != nil {
			return nil, fmt.Errorf("dial SMTP %s: %w", address, err)
		}
		return conn, nil
	}
	dialer := &net.Dialer{Timeout: s.timeout}
	if s.tlsMode == TLSModeImplicit {
		tlsDialer := &tls.Dialer{
			NetDialer: dialer,
			Config: &tls.Config{
				MinVersion: tls.VersionTLS12,
				ServerName: s.host,
			},
		}
		conn, err := tlsDialer.DialContext(ctx, "tcp", address)
		if err != nil {
			return nil, fmt.Errorf("dial SMTPS %s: %w", address, err)
		}
		return conn, nil
	}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("dial SMTP %s: %w", address, err)
	}
	return conn, nil
}

func (s *SMTPSender) buildMessage(recipients []*mail.Address, subject, body string) ([]byte, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil, fmt.Errorf("email subject is required")
	}
	normalizedBody := normalizeBody(body)

	var builder bytes.Buffer
	builder.WriteString("From: ")
	builder.WriteString(s.from.String())
	builder.WriteString("\r\n")
	builder.WriteString("To: ")
	builder.WriteString(joinAddresses(recipients))
	builder.WriteString("\r\n")
	builder.WriteString("Subject: ")
	builder.WriteString(mime.QEncoding.Encode("utf-8", subject))
	builder.WriteString("\r\n")
	builder.WriteString("Date: ")
	builder.WriteString(time.Now().UTC().Format(time.RFC1123Z))
	builder.WriteString("\r\n")
	builder.WriteString("MIME-Version: 1.0\r\n")
	builder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	builder.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	builder.WriteString("\r\n")
	builder.WriteString(normalizedBody)
	return builder.Bytes(), nil
}

func parseRecipientAddresses(values []string) ([]*mail.Address, error) {
	recipients := make([]*mail.Address, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		address, err := mail.ParseAddress(trimmed)
		if err != nil {
			return nil, fmt.Errorf("parse SMTP recipient %q: %w", trimmed, err)
		}
		recipients = append(recipients, address)
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("at least one email recipient is required")
	}
	return recipients, nil
}

func joinAddresses(addresses []*mail.Address) string {
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, address.String())
	}
	return strings.Join(values, ", ")
}

func normalizeBody(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	if !strings.HasSuffix(body, "\r\n") {
		body += "\r\n"
	}
	return body
}

func connectionDeadline(ctx context.Context, timeout time.Duration) time.Time {
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		return ctxDeadline
	}
	return deadline
}
