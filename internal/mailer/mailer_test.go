package mailer

import (
	"context"
	"strings"
	"testing"
)

func TestSMTPRejectsHeaderInjectionBeforeDial(t *testing.T) {
	smtp := &SMTP{Host: "127.0.0.1", Port: "1", From: "site@example.com"}
	err := smtp.Send(context.Background(), Message{To: "owner@example.com", Subject: "Hello\r\nBcc: evil@example.com", Body: "x"})
	if err == nil || !strings.Contains(err.Error(), "newline") {
		t.Fatalf("error=%v", err)
	}
}
