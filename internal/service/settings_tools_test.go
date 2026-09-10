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
		settings.WebTeamSecondaryBenefit != defaults.WebTeamSecondaryBenefit ||
		settings.RenewalAnnouncementTitle != defaults.RenewalAnnouncementTitle ||
		settings.RenewalAnnouncementIntro != defaults.RenewalAnnouncementIntro ||
		len(settings.RenewalAnnouncementItems) != len(defaults.RenewalAnnouncementItems) ||
		settings.PaymentTitle != defaults.PaymentTitle ||
		settings.PaymentDescription != defaults.PaymentDescription ||
		settings.PaymentQRCodeDataURL != "" {
		t.Fatalf("legacy redeem settings did not receive benefit defaults: %#v", settings)
	}
}

func TestRedeemPageSettingsUpgradesLegacyRenewalGuidance(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "legacy-redeem-announcement.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.SetSetting(
		model.SettingRedeemPageSettings,
		`{"announcement_title":"加入前请先确认","announcement_intro":"旧说明","announcement_items":["请备份工作空间资料。","长期客户请添加客服微信。","到期后如果没有及时续费，席位可能会被移出空间；移出前未备份的工作空间内容可能无法找回。"],"renewal_announcement_items":["付款金额必须与页面显示的本期应付金额完全一致，否则无法核对续费；付错金额请联系客服。"],"support_title":"客服","support_contact_label":"微信号"}`,
	); err != nil {
		t.Fatal(err)
	}

	settings, err := (&SubscriptionService{Store: store}).GetRedeemPageSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.AnnouncementTitle != "加入前请先确认" {
		t.Fatalf("custom announcement title was overwritten: %q", settings.AnnouncementTitle)
	}
	if len(settings.AnnouncementItems) != 3 ||
		!strings.Contains(settings.AnnouncementItems[2], "自助续费") ||
		!strings.Contains(settings.AnnouncementItems[2], "联系客服") ||
		!strings.Contains(settings.AnnouncementItems[2], "自动移出空间") {
		t.Fatalf("legacy renewal guidance was not upgraded: %#v", settings.AnnouncementItems)
	}
	if len(settings.RenewalAnnouncementItems) != 1 ||
		settings.RenewalAnnouncementItems[0] != model.DefaultRenewalPaymentAmountGuidance {
		t.Fatalf("legacy renewal payment guidance was not upgraded: %#v", settings.RenewalAnnouncementItems)
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
	settings.RenewalAnnouncementTitle = "续费前请核对"
	settings.RenewalAnnouncementItems = []string{"付款备注邮箱", "按页面金额付款"}
	settings.PaymentTitle = "支付宝收款码"
	settings.PaymentQRCodeDataURL = "data:image/png;base64,iVBORw0KGgo="
	if err := service.SaveRedeemPageSettings(settings); err != nil {
		t.Fatal(err)
	}

	stored, err := service.GetRedeemPageSettings()
	if err != nil {
		t.Fatal(err)
	}
	if stored.CodexPlusWeeklyQuotaUSD != 175 || stored.CodexTeamWeeklyQuotaUSD != 230 ||
		stored.WebPrimaryBenefitLabel != "GPT 新模型极高" ||
		stored.WebTeamSecondaryBenefit != "20 次/月" ||
		stored.RenewalAnnouncementTitle != "续费前请核对" ||
		len(stored.RenewalAnnouncementItems) != 2 ||
		stored.PaymentTitle != "支付宝收款码" ||
		stored.PaymentQRCodeDataURL != "data:image/png;base64,iVBORw0KGgo=" {
		t.Fatalf("custom redeem benefits were not persisted: %#v", stored)
	}
}

func TestSettingsPagePersistsAndValidatesRenewalApplicationAlertEmail(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "renewal-alert-settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := &SubscriptionService{Store: store}
	recipient := "operator@example.com"
	if err := service.SaveSettingsPage(
		model.DefaultNotifyTemplate,
		model.DefaultCustomerEmailTemplate,
		model.DefaultPriceIncreaseCustomerEmailTemplate,
		model.DefaultPriceDecreaseCustomerEmailTemplate,
		nil,
		nil,
		nil,
		&recipient,
	); err != nil {
		t.Fatal(err)
	}
	stored, err := service.GetRenewalApplicationAlertEmail()
	if err != nil || stored != recipient {
		t.Fatalf("stored renewal alert recipient = %q, error = %v", stored, err)
	}

	invalid := "operator@example.com, hidden@example.com"
	if err := service.ValidateSettingsPage(
		model.DefaultNotifyTemplate,
		model.DefaultCustomerEmailTemplate,
		model.DefaultPriceIncreaseCustomerEmailTemplate,
		model.DefaultPriceDecreaseCustomerEmailTemplate,
		nil,
		nil,
		&invalid,
	); err == nil || !strings.Contains(err.Error(), "提醒邮箱格式无效") {
		t.Fatalf("invalid renewal alert recipient error = %v", err)
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
