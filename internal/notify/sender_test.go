package notify

import (
	"context"
	"strings"
	"testing"
)

func TestBuildSMTPMessageEncodesSubjectAndNormalizesHeaders(t *testing.T) {
	message := string(buildSMTPMessage(
		"Carpool Notify <sender@example.com>\r\nBcc: hidden@example.com",
		[]string{"customer@example.com\r\nBcc: hidden@example.com"},
		"拼车续费提醒\r\nBcc: hidden@example.com",
		"line one\r\nline two",
	))
	if strings.Contains(message, "\r\nBcc:") {
		t.Fatalf("message contains an injected header: %q", message)
	}
	if !strings.Contains(message, "Subject: =?UTF-8?q?") {
		t.Fatalf("subject is not RFC 2047 encoded: %q", message)
	}
	if !strings.Contains(message, "line one\r\nline two") || strings.Contains(message, "\r\r\n") {
		t.Fatalf("body line endings were not normalized: %q", message)
	}
}

func TestSMTPSenderRejectsInvalidAddressesBeforeConnecting(t *testing.T) {
	sender := SMTPSender{
		Host:     "smtp.example.com",
		Port:     587,
		Username: "sender@example.com",
		Password: "secret",
		From:     "sender@example.com",
	}
	err := sender.SendTo(
		context.Background(),
		[]string{"customer@example.com\r\nBcc: hidden@example.com"},
		"subject",
		"body",
	)
	if err == nil || !strings.Contains(err.Error(), "recipient address is invalid") {
		t.Fatalf("invalid recipient error = %v", err)
	}
}

func TestSMTPSenderHonorsCanceledContextBeforeConnecting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sender := SMTPSender{
		Host:     "smtp.example.com",
		Port:     587,
		Username: "sender@example.com",
		Password: "secret",
		From:     "sender@example.com",
	}
	if err := sender.SendTo(ctx, []string{"customer@example.com"}, "subject", "body"); err != context.Canceled {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
