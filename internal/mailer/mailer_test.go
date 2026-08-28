package mailer

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSMTPRejectsEveryInjectedHeaderBeforeDial(t *testing.T) {
	for name, mutate := range map[string]func(*Message){
		"to":       func(m *Message) { m.To += "\r\nBcc: evil@example.com" },
		"from":     func(m *Message) { m.From += "\r\nBcc: evil@example.com" },
		"subject":  func(m *Message) { m.Subject += "\r\nBcc: evil@example.com" },
		"reply-to": func(m *Message) { m.ReplyTo += "\r\nBcc: evil@example.com" },
	} {
		t.Run(name, func(t *testing.T) {
			smtp := &SMTP{Host: "127.0.0.1", Port: "1", From: "site@example.com"}
			message := Message{To: "owner@example.com", From: "site@example.com", Subject: "Hello", ReplyTo: "reply@example.com", Body: "x"}
			mutate(&message)
			err := smtp.Send(context.Background(), message)
			if err == nil || !strings.Contains(err.Error(), "newline") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestOperationDeadlineUsesDefaultAndEarlierContext(t *testing.T) {
	now := time.Unix(1000, 0)
	if got := operationDeadline(context.Background(), now, 0); !got.Equal(now.Add(10 * time.Second)) {
		t.Fatalf("default deadline=%v", got)
	}
	ctx, cancel := context.WithDeadline(context.Background(), now.Add(3*time.Second))
	defer cancel()
	if got := operationDeadline(ctx, now, 20*time.Second); !got.Equal(now.Add(3 * time.Second)) {
		t.Fatalf("context deadline=%v", got)
	}
}

func TestEnvironmentSTARTTLSSemantics(t *testing.T) {
	for _, key := range []string{"STRATUM_SMTP_PORT", "STRATUM_SMTP_STARTTLS", "STRATUM_SMTP_TLS"} {
		value, present := os.LookupEnv(key)
		t.Cleanup(func() {
			if present {
				_ = os.Setenv(key, value)
			} else {
				_ = os.Unsetenv(key)
			}
		})
		_ = os.Unsetenv(key)
	}
	_ = os.Setenv("STRATUM_SMTP_PORT", "587")
	if !FromEnvironment().StartTLS {
		t.Fatal("port 587 should default to STARTTLS")
	}
	_ = os.Setenv("STRATUM_SMTP_TLS", "false")
	if FromEnvironment().StartTLS {
		t.Fatal("legacy TLS=false should disable STARTTLS")
	}
	_ = os.Setenv("STRATUM_SMTP_STARTTLS", "true")
	if !FromEnvironment().StartTLS {
		t.Fatal("STARTTLS should take precedence over the legacy alias")
	}
	_ = os.Setenv("STRATUM_SMTP_STARTTLS", "false")
	if FromEnvironment().StartTLS {
		t.Fatal("STARTTLS=false ignored")
	}
}
