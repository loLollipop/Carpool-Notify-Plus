package service_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
	"carpool-notify/internal/notify"
	"carpool-notify/internal/service"
)

func TestPlusRentalCreateEnforcesManualWechatRenewal(t *testing.T) {
	subscriptionService := openTestService(t)

	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "Plus customer",
		BusinessType:     model.SubscriptionBusinessPlus,
		PriceYuan:        "68.00",
		CostYuan:         "20.50",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "14,7,3",
		CustomerEmail:    "rented-plus@example.com",
		CustomerWechat:   "wx-plus-customer",
		IsResale:         true,
		AgencyFeeYuan:    "not-an-amount",
		BoardedAt:        "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.BusinessType != model.SubscriptionBusinessPlus {
		t.Fatalf("business type = %q, want plus", subscription.BusinessType)
	}
	if subscription.CustomerWechat != "wx-plus-customer" {
		t.Fatalf("customer WeChat = %q", subscription.CustomerWechat)
	}
	if subscription.SeatID != 0 || subscription.AccountID != 0 || subscription.AccountName != "" || subscription.SeatName != "" {
		t.Fatalf("Plus rental retained Team placement: seat=%d account=%d/%q seat_name=%q", subscription.SeatID, subscription.AccountID, subscription.AccountName, subscription.SeatName)
	}
	if subscription.CostCents != 2050 {
		t.Fatalf("cost cents = %d, want 2050", subscription.CostCents)
	}
	if subscription.CustomerEmail != "rented-plus@example.com" {
		t.Fatalf("rented account email = %q", subscription.CustomerEmail)
	}
	if len(subscription.NotifyOffsets) != 0 {
		t.Fatalf("notify offsets = %#v, want empty", subscription.NotifyOffsets)
	}
	if subscription.IsResale || subscription.AgencyFeeCents != 0 {
		t.Fatalf("resale fields were not cleared: resale=%v fee=%d", subscription.IsResale, subscription.AgencyFeeCents)
	}
}

func TestPlusRentalRequiresCustomerWechat(t *testing.T) {
	subscriptionService := openTestService(t)

	_, err := subscriptionService.Create(service.CreateInput{
		Name:          "Plus customer",
		BusinessType:  model.SubscriptionBusinessPlus,
		PriceYuan:     "68.00",
		CronExpr:      "interval:30d",
		CustomerEmail: "rented-plus@example.com",
		BoardedAt:     "2026-07-01",
	})
	if err == nil || !strings.Contains(err.Error(), "客户微信") {
		t.Fatalf("create error = %v, want customer WeChat validation", err)
	}
}

func TestPlusRentalRequiresRentedAccountEmail(t *testing.T) {
	subscriptionService := openTestService(t)

	_, err := subscriptionService.Create(service.CreateInput{
		Name:           "Plus customer",
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CronExpr:       "interval:30d",
		CustomerWechat: "wx-customer",
		BoardedAt:      "2026-07-01",
	})
	if err == nil || !strings.Contains(err.Error(), "出租账号邮箱") {
		t.Fatalf("create error = %v, want rented account email validation", err)
	}
}

func TestPlusRentalRequiresCustomerName(t *testing.T) {
	subscriptionService := openTestService(t)

	_, err := subscriptionService.Create(service.CreateInput{
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CronExpr:       "interval:30d",
		CustomerEmail:  "rented-plus@example.com",
		CustomerWechat: "wx-customer",
		BoardedAt:      "2026-07-01",
	})
	if err == nil || !strings.Contains(err.Error(), "客户名称") {
		t.Fatalf("create error = %v, want customer name validation", err)
	}
}

func TestPlusRentalCannotBeCopiedIntoTeamSeat(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "Team owner", "车位1")
	plusID, err := subscriptionService.Create(service.CreateInput{
		Name:           "Plus customer",
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CostYuan:       "20.00",
		CronExpr:       "interval:30d",
		CustomerEmail:  "rented-plus@example.com",
		CustomerWechat: "wx-customer",
		BoardedAt:      "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := subscriptionService.Copy(plusID, seatIDs[0]); err == nil || !strings.Contains(err.Error(), "Plus 出租不能复制") {
		t.Fatalf("copy error = %v, want Plus copy rejection", err)
	}
	if occupant, err := subscriptionService.Store.GetActiveSubscriptionBySeatID(seatIDs[0]); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("Team seat was occupied by rejected Plus copy: occupant=%#v err=%v", occupant, err)
	}
}

func TestTeamSubscriptionStillRequiresAccount(t *testing.T) {
	subscriptionService := openTestService(t)

	_, err := subscriptionService.Create(service.CreateInput{
		Name:         "Team customer",
		BusinessType: model.SubscriptionBusinessTeam,
		PriceYuan:    "25.00",
		CronExpr:     "interval:30d",
		BoardedAt:    "2026-07-01",
	})
	if err == nil || !strings.Contains(err.Error(), "请选择所属账号") {
		t.Fatalf("create error = %v, want Team account validation", err)
	}
}

func TestPlusRentalOnlySendsDueDayOperatorReminder(t *testing.T) {
	subscriptionService := openTestService(t)
	clock := time.Date(2026, time.July, 12, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return clock }
	iyuuRecorder := &recordingSender{}
	smtpRecorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: iyuuRecorder, SMTP: smtpRecorder}

	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "Plus reminder customer",
		BusinessType:     model.SubscriptionBusinessPlus,
		PriceYuan:        "68.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "rented-plus@example.com",
		CustomerWechat:   "wx-customer",
		BoardedAt:        "2000-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if smtpRecorder.calls != 0 || iyuuRecorder.calls != 0 {
		t.Fatalf("early sends: SMTP=%d IYUU=%d, want 0/0", smtpRecorder.calls, iyuuRecorder.calls)
	}
	_, err = subscriptionService.Store.GetNotificationLog(
		subscriptionID,
		"2026-07-15",
		3,
		model.ChannelSMTP,
		model.NotificationKindScheduled,
	)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("SMTP notification log error = %v, want sql.ErrNoRows", err)
	}

	clock = time.Date(2026, time.July, 15, 10, 0, 0, 0, cycle.Location)
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if iyuuRecorder.calls != 1 {
		t.Fatalf("IYUU sends = %d, want 1", iyuuRecorder.calls)
	}
	if smtpRecorder.calls != 0 {
		t.Fatalf("SMTP sends = %d, want 0", smtpRecorder.calls)
	}
}

func TestPlusRentalRejectsManualAndLegacyCustomerEmail(t *testing.T) {
	subscriptionService := openTestService(t)
	clock := time.Date(2026, time.July, 12, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return clock }
	smtpRecorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{SMTP: smtpRecorder}

	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:           "Plus email guard",
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CronExpr:       "0 0 15 * *",
		CustomerEmail:  "rented-plus@example.com",
		CustomerWechat: "wx-customer",
		BoardedAt:      "2000-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SendCustomerEmail(context.Background(), subscriptionID); err == nil || !strings.Contains(err.Error(), "不发送客户邮件") {
		t.Fatalf("manual send error = %v", err)
	}
	if _, _, _, err := subscriptionService.PreviewCustomerEmail(subscriptionID); err == nil || !strings.Contains(err.Error(), "不发送客户邮件") {
		t.Fatalf("preview error = %v", err)
	}

	legacyLog, err := subscriptionService.Store.UpsertPendingNotification(
		subscriptionID,
		"2026-07-15",
		3,
		model.ChannelSMTP,
		model.NotificationKindScheduled,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if smtpRecorder.calls != 0 {
		t.Fatalf("legacy Plus SMTP sends = %d, want 0", smtpRecorder.calls)
	}
	legacyLog, err = subscriptionService.Store.GetNotificationLogByID(legacyLog.ID)
	if err != nil {
		t.Fatal(err)
	}
	if legacyLog.Status != model.NotificationStatusFailed {
		t.Fatalf("legacy log status = %q, want failed", legacyLog.Status)
	}
}

func TestPlusRentalBillsContributeRevenueCostAndProfit(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:           "Plus finance customer",
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CostYuan:       "20.50",
		CronExpr:       "interval:30d",
		CustomerEmail:  "finance-plus@example.com",
		CustomerWechat: "wx-finance",
		BoardedAt:      "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	dashboard, err := subscriptionService.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.TotalAmountYuan != "68.00" || dashboard.TotalCostYuan != "20.50" || dashboard.TotalProfitYuan != "47.50" {
		t.Fatalf(
			"initial Plus dashboard amount/cost/profit = %s/%s/%s, want 68.00/20.50/47.50",
			dashboard.TotalAmountYuan,
			dashboard.TotalCostYuan,
			dashboard.TotalProfitYuan,
		)
	}
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-01", true); err != nil {
		t.Fatal(err)
	}
	idempotentDashboard, err := subscriptionService.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if idempotentDashboard.TotalAmountYuan != "68.00" || idempotentDashboard.TotalCostYuan != "20.50" {
		t.Fatalf(
			"re-marking initial Plus payment duplicated finance totals: amount/cost = %s/%s",
			idempotentDashboard.TotalAmountYuan,
			idempotentDashboard.TotalCostYuan,
		)
	}

	page, err := subscriptionService.ListBillsPage()
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Bills) != 1 {
		t.Fatalf("Plus bills = %d, want 1", len(page.Bills))
	}
	if page.Bills[0].BusinessType != model.SubscriptionBusinessPlus || page.Bills[0].CostCents != 2050 || page.Bills[0].ProfitYuan != "47.50" {
		t.Fatalf("Plus bill snapshot = %#v", page.Bills[0])
	}
	if page.Summary.TotalCostYuan != "20.50" || page.Summary.TotalProfitYuan != "47.50" {
		t.Fatalf("bill summary cost/profit = %s/%s", page.Summary.TotalCostYuan, page.Summary.TotalProfitYuan)
	}

	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-31", true); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-31", true); err != nil {
		t.Fatal(err)
	}
	dashboard, err = subscriptionService.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.TotalAmountYuan != "136.00" || dashboard.TotalCostYuan != "41.00" || dashboard.TotalProfitYuan != "95.00" {
		t.Fatalf(
			"renewed Plus dashboard amount/cost/profit = %s/%s/%s, want 136.00/41.00/95.00",
			dashboard.TotalAmountYuan,
			dashboard.TotalCostYuan,
			dashboard.TotalProfitYuan,
		)
	}
}

func TestPlusRentalInitialPaymentStateAndNextUnpaidPeriod(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 11, 12, 0, 0, 0, cycle.Location)
	}
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:           "Plus paid-state customer",
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CostYuan:       "20.50",
		CronExpr:       "interval:30d",
		CustomerEmail:  "paid-state-plus@example.com",
		CustomerWechat: "wx-paid-state",
		BoardedAt:      "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	views, err := subscriptionService.ListView()
	if err != nil {
		t.Fatal(err)
	}
	var initialView service.SubscriptionView
	for _, view := range views {
		if view.Subscription.ID == subscriptionID {
			initialView = view
			break
		}
	}
	if initialView.Subscription.ID == 0 {
		t.Fatal("Plus subscription view not found")
	}
	if !initialView.CurrentPeriodPaid || initialView.CurrentPeriodStartDate != "2026-07-01" || initialView.CurrentPeriodEndDate != "2026-07-31" {
		t.Fatalf("initial current period = %#v", initialView)
	}
	if initialView.NextDueDate != "2026-07-31" || initialView.DaysRemaining != 20 {
		t.Fatalf("initial next unpaid = %s / %d days, want 2026-07-31 / 20", initialView.NextDueDate, initialView.DaysRemaining)
	}
	if billCount, err := subscriptionService.Store.CountBillsForSubscription(subscriptionID); err != nil || billCount != 1 {
		t.Fatalf("initial bill count = %d, err = %v, want 1", billCount, err)
	}

	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-31", true); err != nil {
		t.Fatal(err)
	}
	views, err = subscriptionService.ListView()
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		if view.Subscription.ID == subscriptionID {
			if view.NextDueDate != "2026-08-30" || view.DaysRemaining != 50 {
				t.Fatalf("renewed next unpaid = %s / %d days, want 2026-08-30 / 50", view.NextDueDate, view.DaysRemaining)
			}
			return
		}
	}
	t.Fatal("renewed Plus subscription view not found")
}

func TestPlusRentalScheduleEditMovesOnlyInitialBill(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 11, 12, 0, 0, 0, cycle.Location)
	}
	input := service.CreateInput{
		Name:           "Plus schedule correction",
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CostYuan:       "20.50",
		CronExpr:       "interval:30d",
		CustomerEmail:  "schedule-plus@example.com",
		CustomerWechat: "wx-schedule",
		BoardedAt:      "2026-07-01",
	}
	subscriptionID, err := subscriptionService.CreateWithInitialBill(input)
	if err != nil {
		t.Fatal(err)
	}

	input.BoardedAt = "2026-07-02"
	if err := subscriptionService.Update(subscriptionID, input); err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-01"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("old initial bill lookup error = %v, want sql.ErrNoRows", err)
	}
	movedBill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-02")
	if err != nil {
		t.Fatal(err)
	}
	if movedBill.AmountCents != 6800 || movedBill.CostCents != 2050 {
		t.Fatalf("moved bill financials = %d/%d, want 6800/2050", movedBill.AmountCents, movedBill.CostCents)
	}
	if billCount, err := subscriptionService.Store.CountBillsForSubscription(subscriptionID); err != nil || billCount != 1 {
		t.Fatalf("bill count after schedule edit = %d, err = %v, want 1", billCount, err)
	}
	dashboard, err := subscriptionService.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.TotalAmountYuan != "68.00" || dashboard.TotalCostYuan != "20.50" {
		t.Fatalf("dashboard after schedule edit = amount %s / cost %s", dashboard.TotalAmountYuan, dashboard.TotalCostYuan)
	}
}

func TestPlusRentalScheduleEditRejectsMultipleBillingPeriods(t *testing.T) {
	subscriptionService := openTestService(t)
	input := service.CreateInput{
		Name:           "Plus historical periods",
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CostYuan:       "20.50",
		CronExpr:       "interval:30d",
		CustomerEmail:  "history-plus@example.com",
		CustomerWechat: "wx-history",
		BoardedAt:      "2026-07-01",
	}
	subscriptionID, err := subscriptionService.CreateWithInitialBill(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-31", true); err != nil {
		t.Fatal(err)
	}

	input.BoardedAt = "2026-07-02"
	err = subscriptionService.Update(subscriptionID, input)
	if err == nil || !strings.Contains(err.Error(), "已有多期账单") {
		t.Fatalf("schedule edit error = %v, want historical bill guard", err)
	}
	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.BoardedAt != "2026-07-01" {
		t.Fatalf("boarded_at changed despite rejected edit: %s", subscription.BoardedAt)
	}
	if billCount, err := subscriptionService.Store.CountBillsForSubscription(subscriptionID); err != nil || billCount != 2 {
		t.Fatalf("bill count after rejected edit = %d, err = %v, want 2", billCount, err)
	}
}

func TestPlusRentalAfterSalesRefundArchivesAndAdjustsFinance(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.July, 11, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:           "Plus after-sales customer",
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CostYuan:       "20.50",
		CronExpr:       "interval:30d",
		CustomerEmail:  "after-sales-plus@example.com",
		CustomerWechat: "wx-after-sales",
		BoardedAt:      "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	request, err := subscriptionService.RequestCancellation(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	caseItem, err := subscriptionService.Store.GetAfterSalesCase(request.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if caseItem.BusinessType != model.SubscriptionBusinessPlus || caseItem.AccountID != 0 {
		t.Fatalf("Plus after-sales identity = business %q account %d", caseItem.BusinessType, caseItem.AccountID)
	}
	if caseItem.AccountName != "Plus after-sales customer" || caseItem.AccountEmail != "after-sales-plus@example.com" || caseItem.CustomerEmail != "" {
		t.Fatalf("Plus after-sales snapshot = %#v", caseItem)
	}
	if caseItem.PeriodStart != "2026-07-01" || caseItem.PeriodEnd != "2026-07-31" || caseItem.WarrantyDays != 30 || caseItem.UsedDays != 10 || caseItem.RemainingDays != 20 {
		t.Fatalf("Plus after-sales period = %#v", caseItem)
	}
	if caseItem.RefundAmountCents != 4533 {
		t.Fatalf("Plus refund = %d, want 4533", caseItem.RefundAmountCents)
	}

	if err := subscriptionService.SetAfterSalesCaseRefunded(caseItem.ID, true); err != nil {
		t.Fatal(err)
	}
	archived, err := subscriptionService.Store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.ArchivedAt == nil || archived.CancellationCaseID != 0 {
		t.Fatalf("Plus subscription was not archived cleanly: %#v", archived)
	}

	dashboard, err := subscriptionService.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.TotalRefundYuan != "45.33" || dashboard.NetRevenueYuan != "22.67" || dashboard.TotalCostYuan != "20.50" || dashboard.TotalProfitYuan != "2.17" {
		t.Fatalf(
			"refunded Plus dashboard refund/net/cost/profit = %s/%s/%s/%s",
			dashboard.TotalRefundYuan,
			dashboard.NetRevenueYuan,
			dashboard.TotalCostYuan,
			dashboard.TotalProfitYuan,
		)
	}
}

func TestOneMonthPlusRentalHasOneChargeAndNoRenewalPeriod(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 11, 12, 0, 0, 0, cycle.Location)
	}
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:           "One-month Plus customer",
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CostYuan:       "20.50",
		CronExpr:       cycle.OneMonthRentalExpression,
		CustomerEmail:  "one-month-plus@example.com",
		CustomerWechat: "wx-one-month",
		BoardedAt:      "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	if billCount, countErr := subscriptionService.Store.CountBillsForSubscription(subscriptionID); countErr != nil || billCount != 1 {
		t.Fatalf("initial bill count = %d, err = %v, want 1", billCount, countErr)
	}
	views, err := subscriptionService.ListView()
	if err != nil {
		t.Fatal(err)
	}
	var rentalView service.SubscriptionView
	for _, view := range views {
		if view.Subscription.ID == subscriptionID {
			rentalView = view
			break
		}
	}
	if rentalView.Subscription.ID == 0 {
		t.Fatal("one-month Plus rental view not found")
	}
	if rentalView.NextDueDate != "2026-07-31" || rentalView.DaysRemaining != 20 || rentalView.CycleDays != 30 {
		t.Fatalf("one-month end = %s / remaining %d / cycle %d", rentalView.NextDueDate, rentalView.DaysRemaining, rentalView.CycleDays)
	}
	if !rentalView.CurrentPeriodPaid || rentalView.CurrentPeriodStartDate != "2026-07-01" || rentalView.CurrentPeriodEndDate != "2026-07-31" {
		t.Fatalf("one-month current period = %#v", rentalView)
	}

	options, err := subscriptionService.ListDuePeriodOptions(subscriptionID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || options[0].StartDate != "2026-07-01" || options[0].EndDate != "2026-07-31" || !options[0].Paid {
		t.Fatalf("one-month due options = %#v", options)
	}
	nextUnpaid, err := subscriptionService.NextUnpaidDueDate(subscriptionID, "")
	if err != nil {
		t.Fatal(err)
	}
	if nextUnpaid != "" {
		t.Fatalf("next unpaid due = %q, want none", nextUnpaid)
	}
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-31", true); err == nil || !strings.Contains(err.Error(), "不能登记续租") {
		t.Fatalf("renewal bill error = %v, want one-month guard", err)
	}
	if billCount, countErr := subscriptionService.Store.CountBillsForSubscription(subscriptionID); countErr != nil || billCount != 1 {
		t.Fatalf("bill count after rejected renewal = %d, err = %v, want 1", billCount, countErr)
	}

	for _, testCase := range []struct {
		month       time.Month
		wantPresent bool
	}{
		{month: time.July, wantPresent: true},
		{month: time.August, wantPresent: false},
	} {
		calendarView, calendarErr := subscriptionService.CalendarMonth(time.Date(2026, testCase.month, 1, 0, 0, 0, 0, cycle.Location))
		if calendarErr != nil {
			t.Fatal(calendarErr)
		}
		present := false
		for _, occurrence := range calendarView.Occurrences {
			if occurrence.SubscriptionID == subscriptionID {
				present = true
			}
		}
		if present != testCase.wantPresent {
			t.Fatalf("month %s occurrence present = %v, want %v", testCase.month, present, testCase.wantPresent)
		}
	}
}

func TestOneMonthPlusRentalOnlyRemindsAtEndDate(t *testing.T) {
	subscriptionService := openTestService(t)
	clock := time.Date(2026, time.July, 30, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return clock }
	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: recorder}

	_, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:           "One-month reminder customer",
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CronExpr:       cycle.OneMonthRentalExpression,
		CustomerEmail:  "one-month-reminder@example.com",
		CustomerWechat: "wx-one-month-reminder",
		BoardedAt:      "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 0 {
		t.Fatalf("pre-expiry reminders = %d, want 0", recorder.calls)
	}

	clock = time.Date(2026, time.July, 31, 10, 0, 0, 0, cycle.Location)
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 1 {
		t.Fatalf("end-date reminders = %d, want 1", recorder.calls)
	}

	clock = time.Date(2026, time.August, 30, 10, 0, 0, 0, cycle.Location)
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 1 {
		t.Fatalf("reminders after another 30 days = %d, want still 1", recorder.calls)
	}
}

func TestOneMonthPlusRentalCompletesWithoutAfterSales(t *testing.T) {
	subscriptionService := openTestService(t)
	clock := time.Date(2026, time.July, 30, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return clock }
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:           "One-month completion customer",
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CronExpr:       cycle.OneMonthRentalExpression,
		CustomerEmail:  "one-month-completion@example.com",
		CustomerWechat: "wx-one-month-completion",
		BoardedAt:      "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.CompleteOneMonthRental(subscriptionID); err == nil || !strings.Contains(err.Error(), "提前结束请走售后") {
		t.Fatalf("early completion error = %v", err)
	}

	clock = time.Date(2026, time.July, 31, 12, 0, 0, 0, cycle.Location)
	if err := subscriptionService.CompleteOneMonthRental(subscriptionID); err != nil {
		t.Fatal(err)
	}
	archived, err := subscriptionService.Store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("completed one-month rental was not archived")
	}
	afterSalesCount, err := subscriptionService.Store.CountAfterSalesCasesBySubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if afterSalesCount != 0 {
		t.Fatalf("after-sales count = %d, want 0", afterSalesCount)
	}
}

func TestRecurringPlusRentalEndsWithoutAfterSalesOnDueDate(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.July, 31, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:           "Recurring Plus natural expiry",
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CostYuan:       "20.00",
		CronExpr:       "interval:30d",
		CustomerEmail:  "recurring-plus-expiry@example.com",
		CustomerWechat: "wx-recurring-plus-expiry",
		BoardedAt:      "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := subscriptionService.RequestCancellation(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Archived || result.CaseID != 0 {
		t.Fatalf("Plus natural-expiry result = %#v", result)
	}
	archived, err := subscriptionService.Store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.ArchivedAt == nil || archived.SeatFrozenUntil != nil {
		t.Fatalf("naturally expired Plus rental = %#v", archived)
	}
	afterSalesCount, err := subscriptionService.Store.CountAfterSalesCasesBySubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if afterSalesCount != 0 {
		t.Fatalf("Plus natural expiry created %d after-sales cases", afterSalesCount)
	}
}
