package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"carpool-notify/internal/config"
	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
	"carpool-notify/internal/notify"
)

func TestRenewalSuccessEmailTracksOnlyNewCommittedBills(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	recorder := &businessEmailRecorder{fail: true}
	service.Notify = notify.Registry{SMTP: recorder}
	subscription, err := service.Store.GetSubscription(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	subscription.NextPriceCents = ptrBusinessPrice(8800)
	subscription.NextPriceEffectiveDueDate = "2026-07-01"
	subscription.Remark = "private internal note"
	if err := service.Store.UpdateSubscription(subscription); err != nil {
		t.Fatal(err)
	}
	if err := service.SetDuePaid(ids[0], "2026-07-01", true); err != nil {
		t.Fatal(err)
	}
	pending, err := service.Store.PendingBusinessEmails(service.now(), 8)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %#v, %v", pending, err)
	}
	for _, expected := range []string{"续费金额：¥88.00", "本期开始：2026-07-01", "本期结束：2026-07-31", "下次到期日期：2026-07-31", "无需重复付款"} {
		if !strings.Contains(pending[0].Body, expected) {
			t.Fatalf("missing %q: %s", expected, pending[0].Body)
		}
	}
	if strings.Contains(pending[0].Body, subscription.Remark) {
		t.Fatal("private note leaked")
	}
	if err := service.SetDuePaid(ids[0], "2026-07-01", true); err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessBusinessEmails(context.Background()); err == nil {
		t.Fatal("SMTP failure not reported")
	}
	if len(recorder.deliveries) != 1 || recorder.deliveries[0].recipients[0] != subscription.CustomerEmail || !strings.Contains(recorder.deliveries[0].html, "续费成功") || strings.Contains(recorder.deliveries[0].html, "福利信息") {
		t.Fatalf("delivery: %#v", recorder.deliveries)
	}
	if _, err := service.Store.GetBillByOccurrence(ids[0], "2026-07-01"); err != nil {
		t.Fatal("SMTP failure rolled back bill", err)
	}
	recorder.fail = false
	moment := service.now().Add(time.Minute)
	service.Clock = func() time.Time { return moment }
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 2 {
		t.Fatal("durable retry missing or duplicate bill queued")
	}
	if err := service.SetDuePaid(ids[0], "2026-07-01", false); err != nil {
		t.Fatal(err)
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 2 {
		t.Fatal("cancel paid sent email")
	}
}
func ptrBusinessPrice(value int64) *int64 { return &value }

func TestInvalidBusinessTemplatesRejectSaveAndRollBackBusinessMutation(t *testing.T) {
	for _, body := range []string{"", "{{", "{{.PrivateNote}}", `{{if false}}{{.Unknown}}{{end}}`, `{{$root := .}}{{if false}}{{$root.Unknown}}{{end}}`, `{{index .ExtensionDays 0}}`, `{{if eq .BenefitType "extension_revoke"}}{{index .ExtensionDays 0}}{{end}}`} {
		if err := validateBusinessEmailTemplate(body); err == nil {
			t.Fatalf("accepted invalid template %q", body)
		}
	}
	service, ids := extensionTestService(t, "2026-06-20", 1)
	service.Notify = notify.Registry{SMTP: &businessEmailRecorder{}}
	bad := "{{.Unknown}}"
	if err := service.Store.SetSetting(model.SettingRenewalSuccessCustomerEmailTemplate, bad); err != nil {
		t.Fatal(err)
	}
	if err := service.SetDuePaid(ids[0], "2026-07-01", true); err == nil {
		t.Fatal("bad renewal template accepted")
	}
	if _, err := service.Store.GetBillByOccurrence(ids[0], "2026-07-01"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("failed renewal created bill", err)
	}
	if err := service.Store.SetSetting(model.SettingCustomerBenefitEmailTemplate, bad); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordCustomerBenefits(extensionInput(t, service, ids, "2026-06-20", 7, "invalid-template-operation")); err == nil {
		t.Fatal("bad benefit template accepted")
	}
	subscription, err := service.Store.GetSubscription(ids[0])
	if err != nil || len(subscription.DueExtensions) != 0 {
		t.Fatalf("benefit escaped rollback: %#v, %v", subscription.DueExtensions, err)
	}
	pending, err := service.Store.PendingBusinessEmails(service.now(), 8)
	if err != nil || len(pending) != 0 {
		t.Fatalf("uncommitted email: %#v, %v", pending, err)
	}
}

func TestBusinessEmailTemplatesShareSavedRenderingAcrossPreviewsTestsAndExport(t *testing.T) {
	service, _ := extensionTestService(t, "2026-06-20", 1)
	recorder := &businessEmailRecorder{}
	service.Notify = notify.Registry{SMTP: recorder}
	service.Config = config.Config{SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPUsername: "sender@example.com", SMTPPassword: "secret", SMTPFrom: "sender@example.com"}
	renewal := "saved renewal {{.CustomerEmail}} {{.AmountPaid}} {{.PeriodEnd}}"
	benefit := `saved benefit {{.CustomerEmail}} {{.BenefitType}} {{.ExtensionDays}} {{.NextDueDate}} {{.NewPrice}}`
	if err := service.SaveSettingsPage(model.DefaultNotifyTemplate, model.DefaultCustomerEmailTemplate, model.DefaultPriceIncreaseCustomerEmailTemplate, model.DefaultPriceDecreaseCustomerEmailTemplate, nil, nil, nil, nil, BusinessEmailTemplateSettings{RenewalSuccess: &renewal, CustomerBenefit: &benefit}); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveSettingsPage(model.DefaultNotifyTemplate, model.DefaultCustomerEmailTemplate, model.DefaultPriceIncreaseCustomerEmailTemplate, model.DefaultPriceDecreaseCustomerEmailTemplate, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"customer_renewal_success", "customer_benefit_extension", "customer_benefit_discount", "customer_benefit_revision", "customer_benefit_revoke"} {
		body := benefit
		if kind == "customer_renewal_success" {
			body = renewal
		}
		preview, sample, err := service.PreviewTemplate(kind, body)
		if err != nil || !strings.Contains(preview, "虚拟示例") || !strings.Contains(sample, "customer@example.com") || strings.Contains(preview, "extension@example.com") {
			t.Fatalf("preview %s = %q %q %v", kind, preview, sample, err)
		}
		if err := service.SendTestCustomerEmail(context.Background(), "test@example.com", kind); err != nil {
			t.Fatal(err)
		}
		delivery := recorder.deliveries[len(recorder.deliveries)-1]
		expected, err := renderBusinessEmailTemplate(body, sampleBusinessEmailData(kind, "test@example.com", "unused"))
		if err != nil || !strings.Contains(delivery.body, expected) || !strings.Contains(delivery.body, "虚拟示例") || delivery.recipients[0] != "test@example.com" {
			t.Fatalf("test %s = %#v, %v", kind, delivery, err)
		}
		label := "福利信息"
		if kind == "customer_renewal_success" {
			label = "续费成功"
		}
		if !strings.Contains(delivery.html, label) {
			t.Fatalf("HTML missing %s", label)
		}
	}
	exported, err := service.Export()
	if err != nil || exported.RenewalSuccessCustomerEmailTemplate != renewal || exported.CustomerBenefitEmailTemplate != benefit {
		t.Fatalf("exported templates = %#v, %v", exported, err)
	}
}

func TestRenewalSuccessEmailDisabledForSandboxAndPlus(t *testing.T) {
	for _, scenario := range []string{"sandbox", "plus", "resale", "empty recipient"} {
		t.Run(scenario, func(t *testing.T) {
			service, ids := extensionTestService(t, "2026-06-20", 1)
			subscription, err := service.Store.GetSubscription(ids[0])
			if err != nil {
				t.Fatal(err)
			}
			if scenario != "sandbox" {
				service.Notify = notify.Registry{SMTP: &businessEmailRecorder{}}
			}
			if scenario == "plus" {
				subscription.BusinessType = model.SubscriptionBusinessPlus
			}
			if scenario == "resale" {
				subscription.IsResale = true
			}
			if scenario == "empty recipient" {
				subscription.CustomerEmail = ""
			}
			batch, err := service.renewalSuccessEmailBatch(subscription, "2026-07-01")
			if err != nil || batch.Build != nil || batch.BuildChecked != nil {
				t.Fatalf("unexpected notification: %#v, %v", batch, err)
			}
		})
	}
}

func TestBillCorrectionsCancelStalePendingRenewalConfirmations(t *testing.T) {
	for _, scenario := range []string{"before first send", "before retry", "note only", "subscription price"} {
		t.Run(scenario, func(t *testing.T) {
			service, ids := extensionTestService(t, "2026-06-20", 1)
			recorder := &businessEmailRecorder{fail: scenario == "before retry"}
			service.Notify = notify.Registry{SMTP: recorder}
			if err := service.SetDuePaid(ids[0], "2026-07-01", true); err != nil {
				t.Fatal(err)
			}
			attempts := 0
			if recorder.fail {
				if err := service.ProcessBusinessEmails(context.Background()); err == nil {
					t.Fatal("expected SMTP failure")
				}
				attempts = 1
			}
			bill, err := service.Store.GetBillByOccurrence(ids[0], "2026-07-01")
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "subscription price" {
				subscription, err := service.Store.GetSubscription(ids[0])
				if err != nil {
					t.Fatal(err)
				}
				subscription.PricePerPersonCents = 7800
				if err := service.Store.UpdateSubscriptionAndSyncBill(subscription, bill.DueDate, 7800, bill.CostCents); err != nil {
					t.Fatal(err)
				}
			} else {
				amount := "78.00"
				if scenario == "note only" {
					amount = cycle.FormatCents(bill.AmountCents)
				}
				if err := service.UpdateBill(bill.ID, BillEditInput{AmountYuan: amount, Note: "correction"}); err != nil {
					t.Fatal(err)
				}
			}
			moment := service.now().Add(time.Hour)
			service.Clock = func() time.Time { return moment }
			recorder.fail = false
			flushBenefitEmails(t, service)
			want := attempts
			if scenario == "note only" {
				want++
			}
			if len(recorder.deliveries) != want {
				t.Fatalf("deliveries after correction = %d, want %d", len(recorder.deliveries), want)
			}
			if _, err := service.Store.GetBill(bill.ID); err != nil {
				t.Fatal("correction removed bill", err)
			}
		})
	}
}

func TestCancelledPaymentsDoNotSendPendingRenewalConfirmations(t *testing.T) {
	for _, scenario := range []string{"unmark", "delete bill", "cancel retry"} {
		t.Run(scenario, func(t *testing.T) {
			service, ids := extensionTestService(t, "2026-06-20", 1)
			recorder := &businessEmailRecorder{fail: scenario == "cancel retry"}
			service.Notify = notify.Registry{SMTP: recorder}
			if err := service.SetDuePaid(ids[0], "2026-07-01", true); err != nil {
				t.Fatal(err)
			}
			attempts := 0
			if recorder.fail {
				if err := service.ProcessBusinessEmails(context.Background()); err == nil {
					t.Fatal("expected SMTP failure")
				}
				attempts = 1
			}
			if scenario == "delete bill" {
				bill, err := service.Store.GetBillByOccurrence(ids[0], "2026-07-01")
				if err != nil {
					t.Fatal(err)
				}
				if err := service.DeleteBill(bill.ID); err != nil {
					t.Fatal(err)
				}
			} else if err := service.SetDuePaid(ids[0], "2026-07-01", false); err != nil {
				t.Fatal(err)
			}
			moment := service.now().Add(time.Hour)
			service.Clock = func() time.Time { return moment }
			recorder.fail = false
			flushBenefitEmails(t, service)
			if len(recorder.deliveries) != attempts {
				t.Fatal("cancelled payment confirmation was delivered")
			}
			// Re-recording payment creates a new bill and one new confirmation.
			if err := service.SetDuePaid(ids[0], "2026-07-01", true); err != nil {
				t.Fatal(err)
			}
			flushBenefitEmails(t, service)
			if len(recorder.deliveries) != attempts+1 {
				t.Fatalf("new payment confirmation missing: %#v", recorder.deliveries)
			}
		})
	}
}

type blockedBusinessEmailSender struct {
	*businessEmailRecorder
	started chan struct{}
	release chan struct{}
}

func (sender *blockedBusinessEmailSender) SendHTMLTo(ctx context.Context, recipients []string, title, body, html string) error {
	close(sender.started)
	select {
	case <-sender.release:
		return sender.businessEmailRecorder.SendHTMLTo(ctx, recipients, title, body, html)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestPaymentCancellationWaitsForInFlightDeliveryThenCancelsRetry(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	recorder := &businessEmailRecorder{fail: true}
	sender := &blockedBusinessEmailSender{businessEmailRecorder: recorder, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(sender.release) })
	t.Cleanup(release)
	service.Notify = notify.Registry{SMTP: sender}
	if err := service.SetDuePaid(ids[0], "2026-07-01", true); err != nil {
		t.Fatal(err)
	}
	workerDone := make(chan error, 1)
	go func() { workerDone <- service.ProcessBusinessEmails(context.Background()) }()
	select {
	case <-sender.started:
	case <-time.After(5 * time.Second):
		t.Fatal("delivery did not start")
	}
	cancelStarted, cancelDone := make(chan struct{}), make(chan error, 1)
	go func() {
		close(cancelStarted)
		cancelDone <- service.SetDuePaid(ids[0], "2026-07-01", false)
	}()
	<-cancelStarted
	select {
	case err := <-cancelDone:
		t.Fatalf("cancellation overtook in-flight delivery: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	release()
	if err := <-workerDone; err == nil {
		t.Fatal("expected delivery failure")
	}
	if err := <-cancelDone; err != nil {
		t.Fatal(err)
	}
	moment := service.now().Add(time.Hour)
	service.Clock = func() time.Time { return moment }
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 1 {
		t.Fatal("cancelled payment was retried")
	}
}

func TestBillCorrectionWaitsForInFlightDeliveryThenCancelsRetry(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	recorder := &businessEmailRecorder{fail: true}
	sender := &blockedBusinessEmailSender{businessEmailRecorder: recorder, started: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(sender.release) })
	t.Cleanup(release)
	service.Notify = notify.Registry{SMTP: sender}
	if err := service.SetDuePaid(ids[0], "2026-07-01", true); err != nil {
		t.Fatal(err)
	}
	bill, err := service.Store.GetBillByOccurrence(ids[0], "2026-07-01")
	if err != nil {
		t.Fatal(err)
	}
	workerDone := make(chan error, 1)
	go func() { workerDone <- service.ProcessBusinessEmails(context.Background()) }()
	select {
	case <-sender.started:
	case <-time.After(5 * time.Second):
		t.Fatal("delivery did not start")
	}
	editDone := make(chan error, 1)
	go func() { editDone <- service.UpdateBill(bill.ID, BillEditInput{AmountYuan: "78.00"}) }()
	select {
	case err := <-editDone:
		t.Fatalf("bill correction overtook in-flight delivery: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	release()
	if err := <-workerDone; err == nil {
		t.Fatal("expected delivery failure")
	}
	if err := <-editDone; err != nil {
		t.Fatal(err)
	}
	moment := service.now().Add(time.Hour)
	service.Clock = func() time.Time { return moment }
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 1 {
		t.Fatal("stale confirmation was retried after correction")
	}
}

func TestResaleCommissionIsNotSentAsCustomerPayment(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	recorder := &businessEmailRecorder{}
	service.Notify = notify.Registry{SMTP: recorder}
	subscription, err := service.Store.GetSubscription(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	subscription.IsResale, subscription.AgencyFeeCents = true, 1234
	if err := service.Store.UpdateSubscription(subscription); err != nil {
		t.Fatal(err)
	}
	if err := service.SetDuePaid(ids[0], "2026-07-01", true); err != nil {
		t.Fatal(err)
	}
	bill, err := service.Store.GetBillByOccurrence(ids[0], "2026-07-01")
	if err != nil || bill.AmountCents != 1234 {
		t.Fatalf("commission bill changed: %#v, %v", bill, err)
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 0 {
		t.Fatal("internal commission leaked into customer email")
	}
}

func TestCommittedBenefitEmailKeepsConfiguredTemplateSnapshot(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	recorder := &businessEmailRecorder{}
	service.Notify = notify.Registry{SMTP: recorder}
	body := "saved gift {{.CustomerEmail}} {{.BenefitType}} {{.ExtensionDays}} {{.PreviousDueDate}} {{.NextDueDate}} {{.OperationTime}}"
	if err := service.Store.SetSetting(model.SettingCustomerBenefitEmailTemplate, body); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordCustomerBenefits(extensionInput(t, service, ids, "2026-06-20", 7, "custom-template-snapshot")); err != nil {
		t.Fatal(err)
	}
	if err := service.Store.SetSetting(model.SettingCustomerBenefitEmailTemplate, "replacement template"); err != nil {
		t.Fatal(err)
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 1 {
		t.Fatalf("deliveries = %#v", recorder.deliveries)
	}
	delivered := recorder.deliveries[0].body
	for _, expected := range []string{"saved gift", "extension@example.com", "extension 7 2026-07-01 2026-07-08"} {
		if !strings.Contains(delivered, expected) {
			t.Fatalf("missing %q in %s", expected, delivered)
		}
	}
	if strings.Contains(delivered, "replacement template") {
		t.Fatal("template edit changed committed event")
	}
}
