package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/notify"
)

type businessEmailDelivery struct {
	recipients        []string
	title, body, html string
}

type businessEmailRecorder struct {
	mu         sync.Mutex
	deliveries []businessEmailDelivery
	fail       bool
}

func (sender *businessEmailRecorder) Send(context.Context, string, string) error {
	return errors.New("must use explicit recipients")
}

func (sender *businessEmailRecorder) SendTo(ctx context.Context, recipients []string, title, body string) error {
	return sender.SendHTMLTo(ctx, recipients, title, body, "")
}

func (sender *businessEmailRecorder) SendHTMLTo(_ context.Context, recipients []string, title, body, html string) error {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	sender.deliveries = append(sender.deliveries, businessEmailDelivery{append([]string(nil), recipients...), title, body, html})
	if sender.fail {
		return errors.New("smtp unavailable")
	}
	return nil
}

func flushBenefitEmails(t testing.TB, service *SubscriptionService) {
	t.Helper()
	if err := service.ProcessBusinessEmails(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestBenefitExtensionEmailsUseEachSubscriptionsMailboxAndCommittedDates(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 2)
	for i, email := range []string{"one@example.com", "two@example.com"} {
		subscription, err := service.Store.GetSubscription(ids[i])
		if err != nil {
			t.Fatal(err)
		}
		subscription.CustomerEmail, subscription.CustomerWechat = email, "shared-wechat"
		if err := service.Store.UpdateSubscription(subscription); err != nil {
			t.Fatal(err)
		}
	}
	recorder := &businessEmailRecorder{}
	service.Notify = notify.Registry{SMTP: recorder}
	input := extensionInput(t, service, ids, "2026-06-20", 7, "email-extension-operation")
	input.Note = "private internal note"
	if n, err := service.RecordCustomerBenefits(input); err != nil || n != 2 {
		t.Fatalf("grant = %d, %v", n, err)
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 2 {
		t.Fatalf("deliveries: %#v", recorder.deliveries)
	}
	seen := map[string]bool{}
	for _, delivery := range recorder.deliveries {
		if len(delivery.recipients) != 1 {
			t.Fatalf("recipients: %#v", delivery.recipients)
		}
		seen[delivery.recipients[0]] = true
		for _, expected := range []string{"赠送天数：7 天", "原到期日期：2026-07-01", "调整后到期日期：2026-07-08"} {
			if !strings.Contains(delivery.body, expected) {
				t.Errorf("missing %q: %s", expected, delivery.body)
			}
		}
		if strings.Contains(delivery.body, input.Note) || !strings.Contains(delivery.html, "福利信息") || strings.Contains(delivery.html, "续费提醒") {
			t.Fatalf("unsafe or mislabeled message: %#v", delivery)
		}
	}
	if !seen["one@example.com"] || !seen["two@example.com"] {
		t.Fatalf("wrong recipients: %#v", seen)
	}
	if _, err := service.RecordCustomerBenefits(input); err == nil {
		t.Fatal("duplicate accepted")
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 2 {
		t.Fatal("duplicate email sent")
	}
}

func TestBenefitDiscountEmailsUsePersistedPriceAndEffectiveDate(t *testing.T) {
	service, ids := extensionTestService(t, "2026-07-01", 1)
	recorder := &businessEmailRecorder{}
	service.Notify = notify.Registry{SMTP: recorder}
	input := RecordCustomerBenefitsInput{SubscriptionIDs: ids, BenefitType: model.CustomerBenefitTypePriceDiscount,
		PriceDiscountYuan: "10", BenefitDate: "2026-07-01", OperationKey: "email-discount-operation"}
	if _, err := service.RecordCustomerBenefits(input); err != nil {
		t.Fatal(err)
	}
	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil || len(benefits) != 1 {
		t.Fatalf("benefits = %#v, %v", benefits, err)
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 1 {
		t.Fatalf("deliveries = %#v", recorder.deliveries)
	}
	body := recorder.deliveries[0].body
	for _, expected := range []string{"原每期价格：¥100.00", "每期优惠：¥10.00", "调整后每期价格：¥90.00", "生效日期：" + benefits[0].PriceEffectiveDueDate} {
		if !strings.Contains(body, expected) {
			t.Errorf("missing %q: %s", expected, body)
		}
	}
	if _, err := service.RecordCustomerBenefits(input); err == nil {
		t.Fatal("duplicate accepted")
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 1 {
		t.Fatal("duplicate email sent")
	}
}

func TestBenefitExtensionRevisionEmailsAndReplay(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	recorder := &businessEmailRecorder{}
	service.Notify = notify.Registry{SMTP: recorder}
	if _, err := service.RecordCustomerBenefits(extensionInput(t, service, ids, "2026-06-20", 7, "email-revision-source")); err != nil {
		t.Fatal(err)
	}
	input := ReviseCustomerBenefitExtensionInput{BenefitID: activeExtensionBenefit(t, service).ID, ExtensionDays: 3,
		Reason: "private correction reason", OperationKey: "email-revision-change"}
	result, err := service.ReviseCustomerBenefitExtension(input)
	if err != nil {
		t.Fatal(err)
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 2 {
		t.Fatalf("deliveries: %#v", recorder.deliveries)
	}
	changed := recorder.deliveries[1]
	if !strings.Contains(changed.title, "修改") || !strings.Contains(changed.body, "调整后到期日期：2026-07-04") || strings.Contains(changed.body, input.Reason) {
		t.Fatalf("change message = %#v", changed)
	}
	if replay, err := service.ReviseCustomerBenefitExtension(input); err != nil || !replay.Replayed {
		t.Fatalf("replay = %#v, %v", replay, err)
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 2 {
		t.Fatal("replay sent another email")
	}
	if _, err := service.ReviseCustomerBenefitExtension(ReviseCustomerBenefitExtensionInput{
		BenefitID: result.ReplacementBenefitID, Reason: "mistake", OperationKey: "email-revision-revoke",
	}); err != nil {
		t.Fatal(err)
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 3 || !strings.Contains(recorder.deliveries[2].title, "撤回") ||
		!strings.Contains(recorder.deliveries[2].body, "调整后到期日期：2026-07-01") {
		t.Fatalf("revoke: %#v", recorder.deliveries)
	}
}

func TestBenefitSMTPFailureOrMissingRecipientDoesNotUndoGrant(t *testing.T) {
	for _, scenario := range []string{"smtp failure", "no recipient", "no sender"} {
		t.Run(scenario, func(t *testing.T) {
			service, ids := extensionTestService(t, "2026-06-20", 1)
			recorder := &businessEmailRecorder{fail: true}
			if scenario != "no sender" {
				service.Notify = notify.Registry{SMTP: recorder}
			}
			if scenario == "no recipient" {
				subscription, _ := service.Store.GetSubscription(ids[0])
				subscription.CustomerEmail = ""
				subscription.CustomerWechat = "wechat-only"
				if err := service.Store.UpdateSubscription(subscription); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := service.RecordCustomerBenefits(extensionInput(t, service, ids, "2026-06-20", 7, "email-failure-grant")); err != nil {
				t.Fatal(err)
			}
			processErr := service.ProcessBusinessEmails(context.Background())
			if scenario == "smtp failure" && processErr == nil {
				t.Fatal("SMTP failure not reported by worker")
			}
			if scenario != "smtp failure" && processErr != nil {
				t.Fatal(processErr)
			}
			subscription, err := service.Store.GetSubscription(ids[0])
			if err != nil || len(subscription.DueExtensions) != 1 || subscription.DueExtensions[0].EffectiveDueDate != "2026-07-08" {
				t.Fatalf("grant not durable: %#v, %v", subscription.DueExtensions, err)
			}
			if scenario != "smtp failure" && len(recorder.deliveries) != 0 {
				t.Fatal("unexpected email")
			}
		})
	}
}

func TestRejectedBenefitSendsNoEmail(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	recorder := &businessEmailRecorder{}
	service.Notify = notify.Registry{SMTP: recorder}
	input := extensionInput(t, service, ids, "2026-06-20", 7, "email-stale-operation")
	subscription, _ := service.Store.GetSubscription(ids[0])
	subscription.Remark = "changed"
	if err := service.Store.UpdateSubscription(subscription); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordCustomerBenefits(input); err == nil {
		t.Fatal("stale operation accepted")
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 0 {
		t.Fatal("emailed uncommitted grant")
	}
}

func TestNextPriceEditSendsDiscountOnceAndNotOnUnrelatedEdit(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	recorder := &businessEmailRecorder{}
	service.Notify = notify.Registry{SMTP: recorder}
	subscription, _ := service.Store.GetSubscription(ids[0])
	input := CreateInput{Name: subscription.Name, PriceYuan: "100", NextPriceYuan: "90", CronExpr: subscription.CronExpr,
		NotifyOffsetsRaw: "3", CustomerEmail: subscription.CustomerEmail, SeatID: subscription.SeatID, BoardedAt: subscription.BoardedAt,
		ExpectedUpdatedAt: subscription.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00")}
	if err := service.Update(ids[0], input); err != nil {
		t.Fatal(err)
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 1 || !strings.Contains(recorder.deliveries[0].body, "调整后每期价格：¥90.00") {
		t.Fatalf("deliveries: %#v", recorder.deliveries)
	}
	subscription, _ = service.Store.GetSubscription(ids[0])
	input.ExpectedUpdatedAt = subscription.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00")
	input.Remark = "unrelated edit"
	if err := service.Update(ids[0], input); err != nil {
		t.Fatal(err)
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 1 {
		t.Fatal("unrelated edit resent benefit email")
	}
}

func TestCustomerCenterManualAndBulkDiscountsEmailAfterCommit(t *testing.T) {
	for _, method := range []string{"manual", "bulk"} {
		t.Run(method, func(t *testing.T) {
			service := openGoalTestService(t)
			ids := createCustomerCareTestSubscriptions(t, service, "pricing@example.com", "", 1)
			seedPaidPricingPeriods(t, service, ids[0], 10000)
			recorder := &businessEmailRecorder{}
			service.Notify = notify.Registry{SMTP: recorder}
			var err error
			if method == "manual" {
				_, err = service.ScheduleManualNextPrices(ManualNextPricesInput{Items: []ManualNextPriceItemInput{{
					SubscriptionID: ids[0], NextPriceYuan: "90",
				}}})
			} else {
				_, err = service.ScheduleBulkNextPrice(BulkNextPriceInput{SubscriptionIDs: ids, NextPriceYuan: "90"})
			}
			if err != nil {
				t.Fatal(err)
			}
			flushBenefitEmails(t, service)
			subscription, err := service.Store.GetSubscription(ids[0])
			if err != nil || subscription.NextPriceCents == nil || *subscription.NextPriceCents != 9000 {
				t.Fatalf("persisted price: %#v, %v", subscription, err)
			}
			if len(recorder.deliveries) != 1 || !strings.Contains(recorder.deliveries[0].body, "生效日期："+subscription.NextPriceEffectiveDueDate) {
				t.Fatalf("deliveries: %#v", recorder.deliveries)
			}
		})
	}
}

func TestBenefitSenderDoesNotFallBackToOperatorRecipients(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	recorder := &extensionRecordingSender{}
	service.Notify = notify.Registry{SMTP: recorder}
	if _, err := service.RecordCustomerBenefits(extensionInput(t, service, ids, "2026-06-20", 7, "email-address-required")); err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessBusinessEmails(context.Background()); err == nil {
		t.Fatal("unsupported sender should fail")
	}
	if recorder.calls != 0 {
		t.Fatal("benefit email sent to default SMTP recipients")
	}
}

func TestBusinessEmailOutboxRetriesAcrossWorkerRestartInSubscriptionOrder(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	recorder := &businessEmailRecorder{fail: true}
	service.Notify = notify.Registry{SMTP: recorder}
	if _, err := service.RecordCustomerBenefits(extensionInput(t, service, ids, "2026-06-20", 7, "outbox-retry-source")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ReviseCustomerBenefitExtension(ReviseCustomerBenefitExtensionInput{
		BenefitID: activeExtensionBenefit(t, service).ID, Reason: "mistake", OperationKey: "outbox-retry-revoke",
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.ProcessBusinessEmails(context.Background()); err == nil {
		t.Fatal("failure not reported")
	}
	if len(recorder.deliveries) != 1 || !strings.Contains(recorder.deliveries[0].title, "赠送") {
		t.Fatal("later event overtook grant")
	}
	if err := service.ProcessBusinessEmails(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(recorder.deliveries) != 1 {
		t.Fatal("retry delay ignored")
	}
	moment := service.now().Add(time.Minute)
	recorder.fail = false
	restarted := &SubscriptionService{Store: service.Store, Notify: notify.Registry{SMTP: recorder}, Clock: func() time.Time { return moment }}
	flushBenefitEmails(t, restarted)
	if len(recorder.deliveries) != 3 || !strings.Contains(recorder.deliveries[1].title, "赠送") || !strings.Contains(recorder.deliveries[2].title, "撤回") {
		t.Fatalf("retry order: %#v", recorder.deliveries)
	}
	flushBenefitEmails(t, restarted)
	if len(recorder.deliveries) != 3 {
		t.Fatal("successful sends retried")
	}
}

func TestBusinessEmailCanceledBatchLeavesEveryMessageForNextTick(t *testing.T) {
	service := openGoalTestService(t)
	recorder := &businessEmailRecorder{}
	service.Notify = notify.Registry{SMTP: recorder}
	messages := make([]db.BusinessEmail, 200)
	for i := range messages {
		messages[i] = db.BusinessEmail{SubscriptionID: int64(i + 1), Recipient: "customer@example.com", Title: "福利", Body: "已赠送延期"}
	}
	if err := service.Store.QueueBusinessEmails(messages, service.now()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := service.ProcessBusinessEmails(ctx); err != nil {
		t.Fatal(err)
	}
	if len(recorder.deliveries) != 0 {
		t.Fatal("canceled batch sent email")
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 200 {
		t.Fatalf("dropped bulk messages: %d", len(recorder.deliveries))
	}
}

func TestMistakenRegistrationDeleteClearsQueuedBenefitBeforeSending(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	recorder := &businessEmailRecorder{}
	service.Notify = notify.Registry{SMTP: recorder}
	if _, err := service.RecordCustomerBenefits(extensionInput(t, service, ids, "2026-06-20", 7, "delete-queued-benefit")); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteMistakenTeamRegistration(ids[0]); err != nil {
		t.Fatal(err)
	}
	flushBenefitEmails(t, service)
	if len(recorder.deliveries) != 0 {
		t.Fatal("email sent after mistaken registration was deleted")
	}
}

func TestRedemptionEmailsShareRenewalRecipientAndRejectDuplicates(t *testing.T) {
	for _, scenario := range []string{"success", "smtp failure", "disabled", "sandbox"} {
		t.Run(scenario, func(t *testing.T) {
			service := openGoalTestService(t)
			recorder := &businessEmailRecorder{fail: scenario == "smtp failure"}
			if scenario != "sandbox" {
				service.Notify = notify.Registry{SMTP: recorder}
			}
			if scenario != "disabled" {
				if err := service.Store.SetSetting(model.SettingRenewalApplicationAlertEmail, "operator@example.com"); err != nil {
					t.Fatal(err)
				}
			}
			codes, err := service.GenerateRedemptionCodes(RedemptionCodeGenerateInput{Count: 1})
			if err != nil {
				t.Fatal(err)
			}
			input := RedemptionSubmitInput{CustomerEmail: "Customer <customer@example.com>", CustomerContact: "wechat-test",
				RedeemCode: codes[0].Code.Code, RequestNote: "please invite"}
			result, err := service.SubmitRedemptionApplication(input)
			if err != nil || result.TrackingToken == "" {
				t.Fatalf("submit = %#v, %v", result, err)
			}
			status, err := service.GetRedemptionStatus(result.TrackingToken)
			if err != nil || status.Status != model.RedemptionStatusPending {
				t.Fatalf("status = %#v, %v", status, err)
			}
			expectedCalls := 1
			if scenario == "disabled" || scenario == "sandbox" {
				expectedCalls = 0
			}
			if len(recorder.deliveries) != expectedCalls {
				t.Fatalf("deliveries: %#v", recorder.deliveries)
			}
			if expectedCalls > 0 {
				delivery := recorder.deliveries[0]
				if len(delivery.recipients) != 1 || delivery.recipients[0] != "operator@example.com" || !strings.Contains(delivery.title, "新的兑换申请") {
					t.Fatalf("alert = %#v", delivery)
				}
				for _, expected := range []string{"customer@example.com", "wechat-test", "please invite", cycle.FormatDateTime(service.now())} {
					if !strings.Contains(delivery.body, expected) {
						t.Errorf("missing %q: %s", expected, delivery.body)
					}
				}
				if strings.Contains(delivery.body, result.TrackingToken) || strings.Contains(delivery.body, input.RedeemCode) {
					t.Fatal("email exposes bearer credentials")
				}
			}
			if _, err := service.SubmitRedemptionApplication(input); err == nil {
				t.Fatal("used code accepted")
			}
			if len(recorder.deliveries) != expectedCalls {
				t.Fatal("duplicate alert")
			}
		})
	}
}
