package service_test

import (
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"
)

func enableTestRenewalPayment(t *testing.T, subscriptionService *service.SubscriptionService) {
	t.Helper()
	settings := model.DefaultRedeemPageSettings
	settings.PaymentQRCodeDataURL = "data:image/png;base64,iVBORw0KGgo="
	if err := subscriptionService.SaveRedeemPageSettings(settings); err != nil {
		t.Fatal(err)
	}
}

func TestSelfServiceRenewalApprovalIsAtomicAndAdvancesPeriod(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 20, 10, 0, 0, 0, cycle.Location)
	}
	enableTestRenewalPayment(t, subscriptionService)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "续费母号", "车位1")
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "续费客户",
		PriceYuan:        "90",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "renewal@example.com",
		CustomerWechat:   "wx-renewal",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	lookup, err := subscriptionService.LookupRenewalSubscriptions(service.RenewalLookupInput{
		CustomerEmail: "Renewal <RENEWAL@example.com>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(lookup.Subscriptions) != 1 {
		t.Fatalf("subscriptions = %d, want 1", len(lookup.Subscriptions))
	}
	period := lookup.Subscriptions[0]
	if period.SubscriptionID != subscriptionID || period.DueDate != "2026-08-31" || period.AmountYuan != "90.00" || !period.Renewable {
		t.Fatalf("renewal period = %#v", period)
	}

	submitted, err := subscriptionService.SubmitRenewalApplication(service.RenewalSubmitInput{
		CustomerEmail:  lookup.CustomerEmail,
		SubscriptionID: subscriptionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if submitted.TrackingToken == "" || submitted.Status != model.RenewalStatusPending {
		t.Fatalf("submit result = %#v", submitted)
	}
	if _, duplicateErr := subscriptionService.SubmitRenewalApplication(service.RenewalSubmitInput{
		CustomerEmail:  lookup.CustomerEmail,
		SubscriptionID: subscriptionID,
	}); duplicateErr == nil || !strings.Contains(duplicateErr.Error(), "已提交续费审核") {
		t.Fatalf("duplicate submit error = %v", duplicateErr)
	}

	applications, err := subscriptionService.ListRenewalApplicationsView(model.RenewalStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if len(applications) != 1 || applications[0].AmountYuan != "90.00" {
		t.Fatalf("applications = %#v", applications)
	}
	if err := subscriptionService.ApproveRenewalApplication(applications[0].Application.ID, service.RenewalDecisionInput{}); err != nil {
		t.Fatal(err)
	}
	paid, err := subscriptionService.Store.IsDuePaid(subscriptionID, "2026-08-31")
	if err != nil || !paid {
		t.Fatalf("paid = %v, err = %v", paid, err)
	}
	status, err := subscriptionService.GetRenewalStatus(submitted.TrackingToken)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != model.RenewalStatusApproved || status.ProcessedAtLabel == "" {
		t.Fatalf("status = %#v", status)
	}
	if err := subscriptionService.ApproveRenewalApplication(applications[0].Application.ID, service.RenewalDecisionInput{}); err == nil {
		t.Fatal("second approval unexpectedly succeeded")
	}
	lookup, err = subscriptionService.LookupRenewalSubscriptions(service.RenewalLookupInput{CustomerEmail: lookup.CustomerEmail})
	if err != nil {
		t.Fatal(err)
	}
	if got := lookup.Subscriptions[0].DueDate; got != "2026-09-30" {
		t.Fatalf("next due date = %q, want 2026-09-30", got)
	}
}

func TestSelfServiceRenewalRejectsStalePriceThenAllowsResubmission(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 20, 10, 0, 0, 0, cycle.Location)
	}
	enableTestRenewalPayment(t, subscriptionService)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "调价续费母号", "车位1")
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "调价续费客户",
		PriceYuan:        "90",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "repriced-renewal@example.com",
		CustomerWechat:   "wx-repriced-renewal",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := subscriptionService.SubmitRenewalApplication(service.RenewalSubmitInput{
		CustomerEmail:  "repriced-renewal@example.com",
		SubscriptionID: subscriptionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := subscriptionService.ScheduleManualNextPrices(service.ManualNextPricesInput{
		Items: []service.ManualNextPriceItemInput{{
			SubscriptionID: subscriptionID,
			NextPriceYuan:  "95",
		}},
	})
	if err != nil || updated != 1 {
		t.Fatalf("schedule next price = %d, err = %v", updated, err)
	}

	applications, err := subscriptionService.ListRenewalApplicationsView(model.RenewalStatusPending)
	if err != nil || len(applications) != 1 {
		t.Fatalf("pending applications = %#v, err = %v", applications, err)
	}
	if err := subscriptionService.ApproveRenewalApplication(applications[0].Application.ID, service.RenewalDecisionInput{}); err == nil ||
		!strings.Contains(err.Error(), "金额或状态已变化") {
		t.Fatalf("stale approval error = %v", err)
	}
	if _, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-08-31"); err == nil {
		t.Fatal("stale approval unexpectedly created a bill")
	}
	if err := subscriptionService.RejectRenewalApplication(applications[0].Application.ID, service.RenewalDecisionInput{}); err != nil {
		t.Fatal(err)
	}
	firstStatus, err := subscriptionService.GetRenewalStatus(first.TrackingToken)
	if err != nil || firstStatus.Status != model.RenewalStatusRejected {
		t.Fatalf("first status = %#v, err = %v", firstStatus, err)
	}

	lookup, err := subscriptionService.LookupRenewalSubscriptions(service.RenewalLookupInput{
		CustomerEmail: "repriced-renewal@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(lookup.Subscriptions) != 1 || lookup.Subscriptions[0].AmountYuan != "95.00" {
		t.Fatalf("repriced lookup = %#v", lookup)
	}
	second, err := subscriptionService.SubmitRenewalApplication(service.RenewalSubmitInput{
		CustomerEmail:  lookup.CustomerEmail,
		SubscriptionID: subscriptionID,
	})
	if err != nil {
		t.Fatal(err)
	}
	applications, err = subscriptionService.ListRenewalApplicationsView(model.RenewalStatusPending)
	if err != nil || len(applications) != 1 || applications[0].AmountYuan != "95.00" {
		t.Fatalf("resubmitted applications = %#v, err = %v", applications, err)
	}
	if err := subscriptionService.ApproveRenewalApplication(applications[0].Application.ID, service.RenewalDecisionInput{}); err != nil {
		t.Fatal(err)
	}
	secondStatus, err := subscriptionService.GetRenewalStatus(second.TrackingToken)
	if err != nil || secondStatus.Status != model.RenewalStatusApproved {
		t.Fatalf("second status = %#v, err = %v", secondStatus, err)
	}
	bill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-08-31")
	if err != nil || bill.AmountCents != 9500 {
		t.Fatalf("repriced bill = %#v, err = %v", bill, err)
	}
	stored, err := subscriptionService.Store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PricePerPersonCents != 9500 || stored.NextPriceCents != nil || stored.NextPriceEffectiveDueDate != "" {
		t.Fatalf("applied subscription price = %#v", stored)
	}
	changes, err := subscriptionService.Store.ListSubscriptionPriceChanges()
	if err != nil || len(changes) != 1 || changes[0].PreviousPriceCents != 9000 || changes[0].NewPriceCents != 9500 {
		t.Fatalf("price changes = %#v, err = %v", changes, err)
	}
}

func TestSelfServiceRenewalSupportsMultipleSubscriptionsAndRejectsForeignSubscription(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 20, 10, 0, 0, 0, cycle.Location)
	}
	enableTestRenewalPayment(t, subscriptionService)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "多订阅母号", "车位1", "车位2")
	for index, seatID := range seatIDs {
		_, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
			Name:             "多订阅客户",
			PriceYuan:        []string{"88", "96"}[index],
			CronExpr:         "interval:30d",
			NotifyOffsetsRaw: "3",
			CustomerEmail:    "multi-renewal@example.com",
			CustomerWechat:   "wx-multi-renewal",
			SeatID:           seatID,
			BoardedAt:        "2026-08-01",
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	foreignID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "其他客户",
		BusinessType:     model.SubscriptionBusinessPlus,
		PriceYuan:        "68",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "foreign-renewal@example.com",
		CustomerWechat:   "wx-foreign-renewal",
		BoardedAt:        "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	lookup, err := subscriptionService.LookupRenewalSubscriptions(service.RenewalLookupInput{
		CustomerEmail: "Multi User <MULTI-RENEWAL@example.com>",
	})
	if err != nil {
		t.Fatal(err)
	}
	if lookup.CustomerEmail != "MULTI-RENEWAL@example.com" || len(lookup.Subscriptions) != 2 {
		t.Fatalf("multiple subscription lookup = %#v", lookup)
	}
	if _, err := subscriptionService.SubmitRenewalApplication(service.RenewalSubmitInput{
		CustomerEmail:  lookup.CustomerEmail,
		SubscriptionID: foreignID,
	}); err == nil || !strings.Contains(err.Error(), "不属于当前邮箱") {
		t.Fatalf("foreign subscription submit error = %v", err)
	}
}

func TestPendingRenewalAppearsInOperationsOverview(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 20, 10, 0, 0, 0, cycle.Location)
	}
	enableTestRenewalPayment(t, subscriptionService)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "待办续费母号", "车位1")
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "待办续费客户",
		PriceYuan:        "90",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "operations-renewal@example.com",
		CustomerWechat:   "wx-operations-renewal",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.SubmitRenewalApplication(service.RenewalSubmitInput{
		CustomerEmail:  "operations-renewal@example.com",
		SubscriptionID: subscriptionID,
	}); err != nil {
		t.Fatal(err)
	}

	overview, err := subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Work.PendingRenewalCount != 1 {
		t.Fatalf("pending renewal count = %d, want 1", overview.Work.PendingRenewalCount)
	}
	var found bool
	for _, task := range overview.Tasks {
		if task.Kind == "renewal_review" {
			found = task.SubscriptionID == subscriptionID &&
				task.RenewalApplicationID > 0 &&
				task.Route == "/redemptions?section=renewals&renewal="+strings.TrimPrefix(task.ID, "renewal:")
			break
		}
	}
	if !found {
		t.Fatalf("renewal task missing or malformed: %#v", overview.Tasks)
	}
}

func TestSelfServiceRenewalRejectsOneMonthPlusRental(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 20, 10, 0, 0, 0, cycle.Location)
	}
	enableTestRenewalPayment(t, subscriptionService)
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:           "单月短租客户",
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68",
		CronExpr:       cycle.OneMonthRentalExpression,
		CustomerEmail:  "short@example.com",
		CustomerWechat: "wx-short",
		BoardedAt:      "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	lookup, err := subscriptionService.LookupRenewalSubscriptions(service.RenewalLookupInput{CustomerEmail: "short@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(lookup.Subscriptions) != 1 || lookup.Subscriptions[0].Renewable {
		t.Fatalf("one-month lookup = %#v", lookup)
	}
	_, err = subscriptionService.SubmitRenewalApplication(service.RenewalSubmitInput{
		CustomerEmail:  "short@example.com",
		SubscriptionID: subscriptionID,
	})
	if err == nil || !strings.Contains(err.Error(), "单月短租") {
		t.Fatalf("submit error = %v", err)
	}
}
