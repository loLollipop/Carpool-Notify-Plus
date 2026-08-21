package notify

import (
	"strings"
	"testing"
)

func TestBuildCustomerEmailHTMLPlacesSummaryBeforeContentAndEscapesInput(t *testing.T) {
	plainText := `您好，您的 ChatGPT Team 拼车服务还有 7 天到期。

客户邮箱：customer@example.com
本期应收：¥90.00
计费周期：月付
到期日期：2026-08-28

请在到期前完成续费。<script>alert(1)</script>`
	htmlBody := BuildCustomerEmailHTML(plainText)
	summaryIndex := strings.Index(htmlBody, "续费信息")
	contentIndex := strings.Index(htmlBody, "您好")
	if summaryIndex < 0 || contentIndex < 0 || summaryIndex >= contentIndex {
		t.Fatalf("summary/content order is wrong: %s", htmlBody)
	}
	if strings.Contains(htmlBody, "<script>") || !strings.Contains(htmlBody, "&lt;script&gt;") {
		t.Fatalf("HTML input was not escaped: %s", htmlBody)
	}
	for _, expected := range []string{"customer@example.com", "¥90.00", "2026-08-28"} {
		if !strings.Contains(htmlBody, expected) {
			t.Fatalf("HTML body missing %q", expected)
		}
	}
}

func TestBuildSMTPHTMLMessageContainsPlainAndHTMLAlternatives(t *testing.T) {
	message, err := buildSMTPHTMLMessage(
		"sender@example.com",
		[]string{"customer@example.com"},
		"拼车续费提醒",
		"plain fallback",
		"<html><body>styled email</body></html>",
	)
	if err != nil {
		t.Fatal(err)
	}
	text := string(message)
	for _, expected := range []string{
		"Content-Type: multipart/alternative",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Type: text/html; charset=UTF-8",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("multipart email missing %q: %s", expected, text)
		}
	}
}
