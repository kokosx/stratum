package mailer

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type Message struct {
	To      string
	From    string
	Subject string
	ReplyTo string
	Body    string
}

type Mailer interface {
	Send(context.Context, Message) error
}

type SMTP struct {
	Host, Port, Username, Password, From string
	StartTLS                             bool
	OperationTimeout                     time.Duration
}

func FromEnvironment() *SMTP {
	port := os.Getenv("STRATUM_SMTP_PORT")
	if port == "" {
		port = "587"
	}
	useStartTLS := port == "587"
	if raw, ok := os.LookupEnv("STRATUM_SMTP_STARTTLS"); ok {
		useStartTLS, _ = strconv.ParseBool(raw)
	} else if raw, ok := os.LookupEnv("STRATUM_SMTP_TLS"); ok { // Legacy alias.
		useStartTLS, _ = strconv.ParseBool(raw)
	}
	return &SMTP{Host: os.Getenv("STRATUM_SMTP_HOST"), Port: port, Username: os.Getenv("STRATUM_SMTP_USERNAME"), Password: os.Getenv("STRATUM_SMTP_PASSWORD"), From: os.Getenv("STRATUM_SMTP_FROM"), StartTLS: useStartTLS}
}

func (s *SMTP) Available() bool { return s != nil && s.Host != "" && s.From != "" }

func (s *SMTP) Send(ctx context.Context, m Message) error {
	if !s.Available() {
		return errors.New("SMTP is not configured")
	}
	if m.From == "" {
		m.From = s.From
	}
	for _, value := range []string{m.To, m.From, m.Subject, m.ReplyTo} {
		if strings.ContainsAny(value, "\r\n") {
			return errors.New("mail header contains a newline")
		}
	}
	if _, err := mail.ParseAddress(m.To); err != nil {
		return fmt.Errorf("invalid recipient: %w", err)
	}
	if _, err := mail.ParseAddress(m.From); err != nil {
		return fmt.Errorf("invalid sender: %w", err)
	}
	if m.ReplyTo != "" {
		if _, err := mail.ParseAddress(m.ReplyTo); err != nil {
			return fmt.Errorf("invalid reply-to: %w", err)
		}
	}
	headers := []string{"To: " + m.To, "From: " + m.From, "Subject: " + m.Subject, "MIME-Version: 1.0", "Content-Type: text/plain; charset=UTF-8"}
	if m.ReplyTo != "" {
		headers = append(headers, "Reply-To: "+m.ReplyTo)
	}
	payload := []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + strings.ReplaceAll(m.Body, "\n", "\r\n"))
	address := net.JoinHostPort(s.Host, s.Port)
	deadline := operationDeadline(ctx, time.Now(), s.OperationTimeout)
	dialCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(dialCtx, "tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, s.Host)
	if err != nil {
		return err
	}
	defer client.Close()
	// Port 465 implicit TLS is intentionally unsupported; use STARTTLS instead.
	if s.StartTLS {
		if err := client.StartTLS(&tls.Config{ServerName: s.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if s.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.Username, s.Password, s.Host)); err != nil {
			return err
		}
	}
	if err := client.Mail(m.From); err != nil {
		return err
	}
	if err := client.Rcpt(m.To); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(payload); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func operationDeadline(ctx context.Context, now time.Time, timeout time.Duration) time.Time {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline := now.Add(timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		return contextDeadline
	}
	return deadline
}
