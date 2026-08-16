package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
	"carpool-notify/internal/notify"
	"carpool-notify/internal/service"
)

func TestScheduledNextPriceProtectsCurrentPeriodAndAppliesOnRenewal(t *testing.T) {
	subscriptionService := openTestService(t)
	clock := time.Date(2026, time.July, 8, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return clock }
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "调价测试账号", "车位1")

	input := service.CreateInput{
		Name:             "低价老用户",
		PriceYuan:        "30.00",
		CronExpr:         "0 0 * * 1",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "customer@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-01",
	}
	subscriptionID, err := subscriptionService.CreateWithInitialBill(input)
	if err != nil {
		t.Fatal(err)
	}
	initialBill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-06")
	if err != nil {
		t.Fatal(err)
	}
	if initialBill.AmountCents != 3000 {
		t.Fatalf("initial bill = %d, want 3000", initialBill.AmountCents)
	}

	input.NextPriceYuan = "45.00"
	if err := subscriptionService.Update(subscriptionID, input); err != nil {
		t.Fatal(err)
	}
	input.Remark = "保留已安排调价"
	if err := subscriptionService.Update(subscriptionID, input); err != nil {
		t.Fatal(err)
	}
	updated, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PricePerPersonCents != 3000 || updated.NextPriceCents == nil || *updated.NextPriceCents != 4500 {
		t.Fatalf("prices after scheduling = current %d next %v", updated.PricePerPersonCents, updated.NextPriceCents)
	}
	if updated.NextPriceEffectiveDueDate != "2026-07-13" {
		t.Fatalf("effective due date = %q, want 2026-07-13", updated.NextPriceEffectiveDueDate)
	}
	unchangedInitialBill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-06")
	if err != nil {
		t.Fatal(err)
	}
	if unchangedInitialBill.AmountCents != 3000 {
		t.Fatalf("historical bill changed to %d, want 3000", unchangedInitialBill.AmountCents)
	}

	_, _, previewBody, err := subscriptionService.PreviewCustomerEmail(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(previewBody, "45.00") {
		t.Fatalf("preview does not contain scheduled price: %q", previewBody)
	}

	periods, err := subscriptionService.ListDuePeriodOptions(subscriptionID, "2026-07-13")
	if err != nil {
		t.Fatal(err)
	}
	foundScheduledPeriod := false
	for _, period := range periods {
		if period.StartDate == "2026-07-13" {
			foundScheduledPeriod = period.PriceYuan == "45.00" && period.PriceChangeApplies
		}
	}
	if !foundScheduledPeriod {
		t.Fatalf("due periods do not expose scheduled price: %#v", periods)
	}

	month, err := subscriptionService.CalendarMonth(time.Date(2026, time.July, 1, 0, 0, 0, 0, cycle.Location))
	if err != nil {
		t.Fatal(err)
	}
	foundCalendarPrice := false
	for _, occurrence := range month.Occurrences {
		if occurrence.SubscriptionID == subscriptionID && occurrence.DueDate == "2026-07-13" {
			foundCalendarPrice = occurrence.PriceYuan == "45.00"
		}
	}
	if !foundCalendarPrice {
		t.Fatal("calendar did not use the scheduled renewal price")
	}

	clock = time.Date(2026, time.July, 10, 9, 0, 0, 0, cycle.Location)
	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{SMTP: recorder}
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 1 || !strings.Contains(recorder.lastBody, "45.00") {
		t.Fatalf("scheduled reminder = calls %d body %q", recorder.calls, recorder.lastBody)
	}

	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-13", true); err != nil {
		t.Fatal(err)
	}
	renewalBill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-13")
	if err != nil {
		t.Fatal(err)
	}
	if renewalBill.AmountCents != 4500 {
		t.Fatalf("renewal bill = %d, want 4500", renewalBill.AmountCents)
	}
	applied, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if applied.PricePerPersonCents != 4500 || applied.NextPriceCents != nil || applied.NextPriceEffectiveDueDate != "" {
		t.Fatalf("applied price state = current %d next %v effective %q", applied.PricePerPersonCents, applied.NextPriceCents, applied.NextPriceEffectiveDueDate)
	}
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-13", false); err != nil {
		t.Fatal(err)
	}
	afterUnmark, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if afterUnmark.PricePerPersonCents != 4500 {
		t.Fatalf("unmark rolled price back to %d", afterUnmark.PricePerPersonCents)
	}
}

func TestScheduledNextPriceCanBeCancelled(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 8, 10, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "取消调价账号", "车位1")
	input := service.CreateInput{
		Name:             "取消调价用户",
		PriceYuan:        "30.00",
		CronExpr:         "0 0 * * 1",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "cancel@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-01",
	}
	subscriptionID, err := subscriptionService.CreateWithInitialBill(input)
	if err != nil {
		t.Fatal(err)
	}
	input.NextPriceYuan = "45.00"
	if err := subscriptionService.Update(subscriptionID, input); err != nil {
		t.Fatal(err)
	}
	input.NextPriceYuan = ""
	if err := subscriptionService.Update(subscriptionID, input); err != nil {
		t.Fatal(err)
	}
	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.NextPriceCents != nil || subscription.NextPriceEffectiveDueDate != "" {
		t.Fatalf("cancelled next price still present: %#v", subscription)
	}
	_, _, previewBody, err := subscriptionService.PreviewCustomerEmail(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(previewBody, "30.00") || strings.Contains(previewBody, "45.00") {
		t.Fatalf("cancelled preview uses wrong price: %q", previewBody)
	}
}

func TestPlusNextPriceKeepsCostSnapshot(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 15, 10, 0, 0, 0, cycle.Location)
	}
	input := service.CreateInput{
		Name:           "Plus 调价用户",
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CostYuan:       "20.00",
		CronExpr:       "interval:30d",
		CustomerEmail:  "plus@example.com",
		CustomerWechat: "plus-customer",
		BoardedAt:      "2026-07-01",
	}
	subscriptionID, err := subscriptionService.CreateWithInitialBill(input)
	if err != nil {
		t.Fatal(err)
	}
	input.NextPriceYuan = "78.00"
	if err := subscriptionService.Update(subscriptionID, input); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-31", true); err != nil {
		t.Fatal(err)
	}
	bill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-31")
	if err != nil {
		t.Fatal(err)
	}
	if bill.AmountCents != 7800 || bill.CostCents != 2000 {
		t.Fatalf("Plus renewal snapshot = amount %d cost %d, want 7800/2000", bill.AmountCents, bill.CostCents)
	}
}
