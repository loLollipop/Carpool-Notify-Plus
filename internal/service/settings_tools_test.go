package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"carpool-notify/internal/config"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/notify"
)

type testEmailSender struct {
	calls          int
	lastRecipients []string
	lastTitle      string
	lastBody       string
}

func (sender *testEmailSender) Send(_ context.Context, title string, body string) error {
	sender.calls++
	sender.lastTitle = title
	sender.lastBody = body
	return nil
}

func (sender *testEmailSender) SendTo(
	_ context.Context,
	recipients []string,
	title string,
	body string,
) error {
	sender.calls++
	sender.lastRecipients = append([]string(nil), recipients...)
	sender.lastTitle = title
	sender.lastBody = body
	return nil
}

func TestRedeemPageSettingsBackfillBenefitDefaults(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "redeem-settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.SetSetting(
		model.SettingRedeemPageSettings,
		`{"announcement_title":"旧公告","announcement_intro":"旧说明","announcement_items":["旧条目"],"support_title":"客服","support_contact_label":"微信号"}`,
	); err != nil {
		t.Fatal(err)
	}

	service := &SubscriptionService{Store: store}
	settings, err := service.GetRedeemPageSettings()
	if err != nil {
		t.Fatal(err)
	}
	defaults := model.DefaultRedeemPageSettings
	if settings.CodexPlusWeeklyQuotaUSD != defaults.CodexPlusWeeklyQuotaUSD ||
		settings.CodexTeamWeeklyQuotaUSD != defaults.CodexTeamWeeklyQuotaUSD ||
		settings.WebPrimaryBenefitLabel != defaults.WebPrimaryBenefitLabel ||
		settings.WebTeamSecondaryBenefit != defaults.WebTeamSecondaryBenefit {
		t.Fatalf("legacy redeem settings did not receive benefit defaults: %#v", settings)
	}
}

func TestRedeemPageSettingsPersistCustomBenefits(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "custom-redeem-settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := &SubscriptionService{Store: store}
	settings := model.DefaultRedeemPageSettings
	settings.CodexPlusWeeklyQuotaUSD = 175
	settings.CodexTeamWeeklyQuotaUSD = 230
	settings.WebPrimaryBenefitLabel = "GPT 新模型极高"
	settings.WebTeamSecondaryBenefit = "20 次/月"
	if err := service.SaveRedeemPageSettings(settings); err != nil {
		t.Fatal(err)
	}

	stored, err := service.GetRedeemPageSettings()
	if err != nil {
		t.Fatal(err)
	}
	if stored.CodexPlusWeeklyQuotaUSD != 175 || stored.CodexTeamWeeklyQuotaUSD != 230 ||
		stored.WebPrimaryBenefitLabel != "GPT 新模型极高" ||
		stored.WebTeamSecondaryBenefit != "20 次/月" {
		t.Fatalf("custom redeem benefits were not persisted: %#v", stored)
	}
}

func TestSendTestCustomerEmailUsesSelectedStoredTemplate(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "email-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	recorder := &testEmailSender{}
	service := &SubscriptionService{
		Store: store,
		Config: config.Config{
			SMTPHost:     "smtp.example.com",
			SMTPPort:     587,
			SMTPUsername: "sender@example.com",
			SMTPPassword: "secret",
			SMTPFrom:     "sender@example.com",
		},
		Notify: notify.Registry{SMTP: recorder},
	}
	if err := service.SaveCustomerEmailTemplate("常规模板 {{.CustomerEmail}} ¥{{.AmountDue}}"); err != nil {
		t.Fatal(err)
	}
	if err := service.SavePriceIncreaseCustomerEmailTemplate(
		"调价模板 {{.CustomerEmail}} ¥{{.PreviousPrice}} -> ¥{{.AmountDue}}",
	); err != nil {
		t.Fatal(err)
	}
	if err := service.SavePriceDecreaseCustomerEmailTemplate(
		"优惠模板 {{.CustomerEmail}} ¥{{.PreviousPrice}} -> ¥{{.AmountDue}}",
	); err != nil {
		t.Fatal(err)
	}

	for _, testCase := range []struct {
		kind         string
		title        string
		bodyContains []string
	}{
		{
			kind:         "customer",
			title:        "[测试] 拼车续费提醒",
			bodyContains: []string{"常规模板", "deliverability@example.com", "88.00"},
		},
		{
			kind:         "customer_price_increase",
			title:        "[测试] 拼车续费价格调整通知",
			bodyContains: []string{"调价模板", "88.00", "98.00"},
		},
		{
			kind:         "customer_price_decrease",
			title:        "[测试] 拼车续费优惠通知",
			bodyContains: []string{"优惠模板", "88.00", "78.00"},
		},
	} {
		t.Run(testCase.kind, func(t *testing.T) {
			if err := service.SendTestCustomerEmail(
				context.Background(),
				"deliverability@example.com",
				testCase.kind,
			); err != nil {
				t.Fatal(err)
			}
			if len(recorder.lastRecipients) != 1 || recorder.lastRecipients[0] != "deliverability@example.com" {
				t.Fatalf("test email recipients = %#v", recorder.lastRecipients)
			}
			if !strings.Contains(recorder.lastTitle, testCase.title) {
				t.Fatalf("test email title = %q", recorder.lastTitle)
			}
			for _, expected := range testCase.bodyContains {
				if !strings.Contains(recorder.lastBody, expected) {
					t.Fatalf("test email body = %q, missing %q", recorder.lastBody, expected)
				}
			}
		})
	}
}

func TestSendTestCustomerEmailRejectsInvalidInputBeforeSending(t *testing.T) {
	recorder := &testEmailSender{}
	service := &SubscriptionService{Notify: notify.Registry{SMTP: recorder}}
	for _, input := range []struct {
		recipient string
		kind      string
	}{
		{recipient: "not-an-email", kind: "customer"},
		{recipient: "test@example.com", kind: "unknown"},
	} {
		if err := service.SendTestCustomerEmail(
			context.Background(),
			input.recipient,
			input.kind,
		); err == nil {
			t.Fatalf("SendTestCustomerEmail(%q, %q) unexpectedly succeeded", input.recipient, input.kind)
		}
	}
	if recorder.calls != 0 {
		t.Fatalf("invalid email test sent %d messages", recorder.calls)
	}
}
