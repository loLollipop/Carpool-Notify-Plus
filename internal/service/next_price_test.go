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

	if err := subscriptionService.SaveCustomerEmailTemplate("普通续费模板：¥{{.AmountDue}}"); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SavePriceIncreaseCustomerEmailTemplate(
		"调价续费模板：原价 ¥{{.PreviousPrice}} / 新价 ¥{{.AmountDue}}",
	); err != nil {
		t.Fatal(err)
	}
	_, previewSubject, previewBody, err := subscriptionService.PreviewCustomerEmail(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(previewSubject, "价格调整通知") ||
		!strings.Contains(previewBody, "调价续费模板") ||
		!strings.Contains(previewBody, "30.00") ||
		!strings.Contains(previewBody, "45.00") ||
		strings.Contains(previewBody, "普通续费模板") {
		t.Fatalf("price-increase preview = subject %q body %q", previewSubject, previewBody)
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
	if recorder.calls != 1 ||
		!strings.Contains(recorder.lastTitle, "价格调整通知") ||
		!strings.Contains(recorder.lastBody, "45.00") ||
		!strings.Contains(recorder.lastBody, "调价续费模板") {
		t.Fatalf("scheduled reminder = calls %d title %q body %q", recorder.calls, recorder.lastTitle, recorder.lastBody)
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
	clock = time.Date(2026, time.July, 15, 9, 0, 0, 0, cycle.Location)
	_, nextSubject, nextBody, err := subscriptionService.PreviewCustomerEmail(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(nextSubject, "拼车续费提醒") ||
		strings.Contains(nextSubject, "价格调整") ||
		nextBody != "普通续费模板：¥45.00" {
		t.Fatalf("post-increase reminder = subject %q body %q", nextSubject, nextBody)
	}
	billsPage, err := subscriptionService.ListBillsPage()
	if err != nil {
		t.Fatal(err)
	}
	pricesByDue := map[string]string{}
	for _, billView := range billsPage.Bills {
		if billView.SubscriptionID == subscriptionID {
			pricesByDue[billView.DueDate] = billView.PriceYuan
		}
	}
	if pricesByDue["2026-07-06"] != "30.00" || pricesByDue["2026-07-13"] != "45.00" {
		t.Fatalf("historical bill views changed after repricing: prices %#v", pricesByDue)
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

func TestManualReminderTargetsScheduledPricePeriod(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.September, 1, 10, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "跨周期调价账号", "车位1")

	input := service.CreateInput{
		Name:             "跨周期调价用户",
		PriceYuan:        "90.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "scheduled@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-08-03",
	}
	subscriptionID, err := subscriptionService.CreateWithInitialBill(input)
	if err != nil {
		t.Fatal(err)
	}
	updatedCount, err := subscriptionService.ScheduleManualNextPrices(service.ManualNextPricesInput{
		Items: []service.ManualNextPriceItemInput{{
			SubscriptionID: subscriptionID,
			NextPriceYuan:  "95.00",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updatedCount != 1 {
		t.Fatalf("updated count = %d, want 1", updatedCount)
	}
	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.NextPriceEffectiveDueDate != "2026-09-02" {
		t.Fatalf("effective due date = %q, want 2026-09-02", subscription.NextPriceEffectiveDueDate)
	}
	if err := subscriptionService.SaveCustomerEmailTemplate(
		"普通续费模板：¥{{.AmountDue}}，到期日 {{.NextDueDate}}",
	); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SavePriceIncreaseCustomerEmailTemplate(
		"调价续费模板：原价 ¥{{.PreviousPrice}} / 新价 ¥{{.AmountDue}}，生效日 {{.NextDueDate}}",
	); err != nil {
		t.Fatal(err)
	}

	_, subject, body, err := subscriptionService.PreviewCustomerEmail(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(subject, "价格调整通知") ||
		!strings.Contains(body, "调价续费模板") ||
		!strings.Contains(body, "90.00") ||
		!strings.Contains(body, "95.00") ||
		!strings.Contains(body, "2026-09-02") ||
		strings.Contains(body, "普通续费模板") ||
		strings.Contains(body, "2026-10-02") {
		t.Fatalf("manual price-increase reminder = subject %q body %q", subject, body)
	}

	rendered, err := subscriptionService.RenderCustomerEmail(subscription)
	if err != nil {
		t.Fatal(err)
	}
	if rendered != body {
		t.Fatalf("rendered manual email differs from preview: rendered %q preview %q", rendered, body)
	}

	periods, err := subscriptionService.ListDuePeriodOptions(subscriptionID, "2026-09-02")
	if err != nil {
		t.Fatal(err)
	}
	var nextPeriod *service.DuePeriodOption
	for index := range periods {
		if periods[index].StartDate == "2026-09-02" {
			nextPeriod = &periods[index]
			break
		}
	}
	if nextPeriod == nil || nextPeriod.PriceYuan != "95.00" || !nextPeriod.PriceChangeApplies {
		t.Fatalf("next billing period did not use scheduled price: %#v", periods)
	}
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-09-02", true); err != nil {
		t.Fatal(err)
	}
	bill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-09-02")
	if err != nil {
		t.Fatal(err)
	}
	if bill.AmountCents != 9500 {
		t.Fatalf("next-period bill amount = %d, want 9500", bill.AmountCents)
	}
}

func TestNormalizeScheduledNextPriceRepairsPreviouslyPostponedPeriod(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.September, 1, 10, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "历史延期调价账号", "车位1")
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "历史延期调价用户",
		PriceYuan:        "90.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "legacy-scheduled@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-08-03",
	})
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	nextPrice := int64(9500)
	subscription.NextPriceCents = &nextPrice
	subscription.NextPriceEffectiveDueDate = "2026-10-02"
	if err := subscriptionService.Store.UpdateSubscription(subscription); err != nil {
		t.Fatal(err)
	}

	repaired, err := subscriptionService.NormalizeScheduledNextPriceEffectiveDates()
	if err != nil {
		t.Fatal(err)
	}
	if repaired != 1 {
		t.Fatalf("repaired = %d, want 1", repaired)
	}
	corrected, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if corrected.NextPriceEffectiveDueDate != "2026-09-02" {
		t.Fatalf("corrected effective date = %q, want 2026-09-02", corrected.NextPriceEffectiveDueDate)
	}
}

func TestScheduledPriceIncreaseSendsAdvanceNotice(t *testing.T) {
	subscriptionService := openTestService(t)
	clock := time.Date(2026, time.July, 2, 9, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return clock }
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "提前告知测试账号", "车位1")

	input := service.CreateInput{
		Name:             "提前告知用户",
		PriceYuan:        "100.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "advance@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-06-01",
	}
	subscriptionID, err := subscriptionService.CreateWithInitialBill(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Store.SetDuePaid(subscriptionID, "2026-07-01", true, 10000); err != nil {
		t.Fatal(err)
	}
	input.NextPriceYuan = "108.00"
	if err := subscriptionService.Update(subscriptionID, input); err != nil {
		t.Fatal(err)
	}
	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.NextPriceEffectiveDueDate != "2026-07-31" {
		t.Fatalf("effective date = %q, want 2026-07-31", subscription.NextPriceEffectiveDueDate)
	}
	if err := subscriptionService.SavePriceIncreaseCustomerEmailTemplate(
		"自定义调价通知：¥{{.PreviousPrice}} → ¥{{.AmountDue}}；当前已支付周期不受影响",
	); err != nil {
		t.Fatal(err)
	}

	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{SMTP: recorder}
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 1 ||
		!strings.Contains(recorder.lastTitle, "价格调整提前告知") ||
		!strings.Contains(recorder.lastBody, "自定义调价通知") ||
		!strings.Contains(recorder.lastBody, "100.00") ||
		!strings.Contains(recorder.lastBody, "108.00") ||
		!strings.Contains(recorder.lastBody, "当前已支付周期不受影响") {
		t.Fatalf("advance notice = calls %d title %q body %q", recorder.calls, recorder.lastTitle, recorder.lastBody)
	}
	logEntry, err := subscriptionService.Store.GetNotificationLog(
		subscriptionID,
		"2026-07-31",
		30,
		model.ChannelSMTP,
		model.NotificationKindPriceIncreaseNotice,
	)
	if err != nil || logEntry.Status != model.NotificationStatusSuccess {
		t.Fatalf("advance notice log = %#v, err = %v", logEntry, err)
	}
}

func TestWeeklyPriceIncreasePlansNoticeFromFutureEffectivePeriod(t *testing.T) {
	subscriptionService := openTestService(t)
	clock := time.Date(2026, time.June, 27, 9, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return clock }
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "周付提前告知账号", "车位1")

	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "周付提前告知用户",
		PriceYuan:        "100.00",
		CronExpr:         "0 0 * * 1",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "weekly-notice@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-06-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	nextPriceCents := int64(10800)
	subscription.NextPriceCents = &nextPriceCents
	subscription.NextPriceEffectiveDueDate = "2026-07-27"
	if err := subscriptionService.Store.UpdateSubscription(subscription); err != nil {
		t.Fatal(err)
	}

	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{SMTP: recorder}
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 1 ||
		!strings.Contains(recorder.lastTitle, "价格调整提前告知") ||
		!strings.Contains(recorder.lastBody, "108.00") {
		t.Fatalf("weekly advance notice = calls %d title %q body %q", recorder.calls, recorder.lastTitle, recorder.lastBody)
	}
	logEntry, err := subscriptionService.Store.GetNotificationLog(
		subscriptionID,
		"2026-07-27",
		30,
		model.ChannelSMTP,
		model.NotificationKindPriceIncreaseNotice,
	)
	if err != nil || logEntry.Status != model.NotificationStatusSuccess {
		t.Fatalf("weekly advance notice log = %#v, err = %v", logEntry, err)
	}
}

func TestScheduledPriceDecreaseUsesRegularCustomerTemplate(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 8, 10, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "降价测试账号", "车位1")
	input := service.CreateInput{
		Name:             "降价用户",
		PriceYuan:        "30.00",
		CronExpr:         "0 0 * * 1",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "decrease@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-01",
	}
	subscriptionID, err := subscriptionService.CreateWithInitialBill(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SaveCustomerEmailTemplate("常规客户模板：¥{{.AmountDue}}"); err != nil {
		t.Fatal(err)
	}
	input.NextPriceYuan = "25.00"
	if err := subscriptionService.Update(subscriptionID, input); err != nil {
		t.Fatal(err)
	}
	_, subject, body, err := subscriptionService.PreviewCustomerEmail(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(subject, "拼车续费提醒") || strings.Contains(subject, "价格调整") || body != "常规客户模板：¥25.00" {
		t.Fatalf("price-decrease preview = subject %q body %q", subject, body)
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

func TestCanceledPriceIncreaseNoticeIsNotRetriedAsRegularEmail(t *testing.T) {
	subscriptionService := openTestService(t)
	clock := time.Date(2026, time.July, 2, 9, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return clock }
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "取消提前告知账号", "车位1")

	input := service.CreateInput{
		Name:             "取消提前告知用户",
		PriceYuan:        "100.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "cancel-notice@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-06-01",
	}
	subscriptionID, err := subscriptionService.CreateWithInitialBill(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Store.SetDuePaid(subscriptionID, "2026-07-01", true, 10000); err != nil {
		t.Fatal(err)
	}
	input.NextPriceYuan = "108.00"
	if err := subscriptionService.Update(subscriptionID, input); err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.Store.UpsertPendingNotification(
		subscriptionID,
		"2026-07-31",
		30,
		model.ChannelSMTP,
		model.NotificationKindPriceIncreaseNotice,
	); err != nil {
		t.Fatal(err)
	}

	input.NextPriceYuan = ""
	if err := subscriptionService.Update(subscriptionID, input); err != nil {
		t.Fatal(err)
	}
	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{SMTP: recorder}
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 0 {
		t.Fatalf("canceled price notice sent %d emails, want 0", recorder.calls)
	}
	logEntry, err := subscriptionService.Store.GetNotificationLog(
		subscriptionID,
		"2026-07-31",
		30,
		model.ChannelSMTP,
		model.NotificationKindPriceIncreaseNotice,
	)
	if err != nil {
		t.Fatal(err)
	}
	if logEntry.Status != model.NotificationStatusCanceled {
		t.Fatalf("canceled price notice status = %q, want %q", logEntry.Status, model.NotificationStatusCanceled)
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
