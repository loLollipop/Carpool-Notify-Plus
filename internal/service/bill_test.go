package service_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/service"
)

func TestSetDuePaidCreatesAndDeletesBill(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "账单账号", "车位1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "账单测试",
		PriceYuan:        "42.50",
		CronExpr:         "0 0 * * 1",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-13", true); err != nil {
		t.Fatal(err)
	}

	bill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-13")
	if err != nil {
		t.Fatalf("expected bill after mark paid: %v", err)
	}
	if bill.AmountCents != 4250 {
		t.Fatalf("bill amount cents = %d, want 4250", bill.AmountCents)
	}

	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-13", false); err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-13"); err == nil {
		t.Fatal("expected bill deleted after unmark paid")
	}
}

func TestSetDuePaidRejectsStaleSubscriptionPriceSnapshot(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "并发账单账号", "车位1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "并发账单用户",
		PriceYuan:        "42.50",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	current := stale
	current.PricePerPersonCents = 5000
	if err := subscriptionService.Store.UpdateSubscription(current); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Store.SetDuePaidForSubscription(
		stale,
		"2026-07-01",
		true,
		4250,
	); !errors.Is(err, db.ErrSubscriptionFinancialStateChanged) {
		t.Fatalf("stale payment error = %v, want financial state change", err)
	}
	if _, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-01"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale payment created a bill: %v", err)
	}
}

func TestBillsSummaryCountsSubscriptionsInsteadOfRenewalBills(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "多期续费账号", "车位1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "多期续费客户",
		PriceYuan:        "42.50",
		CronExpr:         "0 0 * * 1",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, dueDate := range []string{"2026-07-13", "2026-07-20"} {
		if err := subscriptionService.SetDuePaid(subscriptionID, dueDate, true); err != nil {
			t.Fatal(err)
		}
	}

	page, err := subscriptionService.ListBillsPage()
	if err != nil {
		t.Fatal(err)
	}
	if page.Summary.BillCount != 2 {
		t.Fatalf("bill count = %d, want 2", page.Summary.BillCount)
	}
	if page.Summary.ActiveCount != 1 || page.Summary.ArchivedCount != 0 {
		t.Fatalf("subscription counts = active %d archived %d, want 1 and 0", page.Summary.ActiveCount, page.Summary.ArchivedCount)
	}

	if err := subscriptionService.Archive(subscriptionID); err != nil {
		t.Fatal(err)
	}
	page, err = subscriptionService.ListBillsPage()
	if err != nil {
		t.Fatal(err)
	}
	if page.Summary.ActiveCount != 0 || page.Summary.ArchivedCount != 1 {
		t.Fatalf("archived subscription counts = active %d archived %d, want 0 and 1", page.Summary.ActiveCount, page.Summary.ArchivedCount)
	}
}

func TestCreateWithInitialBillMarksIntervalBoardedAtPaid(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "initial bill account", "seat1")
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "initial bill customer",
		PriceYuan:        "30.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-20",
	})
	if err != nil {
		t.Fatal(err)
	}

	paid, err := subscriptionService.Store.IsDuePaid(subscriptionID, "2026-07-20")
	if err != nil {
		t.Fatal(err)
	}
	if !paid {
		t.Fatal("initial interval period should be marked paid")
	}
	bill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-20")
	if err != nil {
		t.Fatal(err)
	}
	if bill.AmountCents != 3000 {
		t.Fatalf("initial bill amount = %d, want 3000", bill.AmountCents)
	}
}

func TestCreateWithInitialBillUsesFirstCronDueAfterBoardedAt(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "cron initial bill account", "seat1")
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "cron initial bill customer",
		PriceYuan:        "42.50",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-10",
	})
	if err != nil {
		t.Fatal(err)
	}

	paidAtBoarded, err := subscriptionService.Store.IsDuePaid(subscriptionID, "2026-07-10")
	if err != nil {
		t.Fatal(err)
	}
	if paidAtBoarded {
		t.Fatal("boarded_at is not a cron due date and should not be marked paid")
	}
	bill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-15")
	if err != nil {
		t.Fatal(err)
	}
	if bill.AmountCents != 4250 {
		t.Fatalf("initial cron bill amount = %d, want 4250", bill.AmountCents)
	}
}

func TestTeamScheduleCorrectionMovesAndUpdatesOnlyInitialBill(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 11, 12, 0, 0, 0, cycle.Location)
	}
	accountID, seatIDs := createTestAccountWithSeats(t, subscriptionService, "schedule correction account", "seat1")
	input := service.CreateInput{
		Name:             "schedule correction customer",
		PriceYuan:        "30.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3,1,0",
		AccountID:        accountID,
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-01",
	}
	subscriptionID, err := subscriptionService.CreateWithInitialBill(input)
	if err != nil {
		t.Fatal(err)
	}

	input.BoardedAt = "2026-07-02"
	input.PriceYuan = "45.00"
	if err := subscriptionService.Update(subscriptionID, input); err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-01"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("old bill lookup error = %v, want sql.ErrNoRows", err)
	}
	movedBill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-02")
	if err != nil {
		t.Fatal(err)
	}
	if movedBill.AmountCents != 4500 {
		t.Fatalf("moved initial bill amount = %d, want 4500", movedBill.AmountCents)
	}
}

func TestBillViewIncludesCustomerAndAccountDetails(t *testing.T) {
	subscriptionService := openTestService(t)
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "提醒母号",
		Email:     "owner@example.com",
		SpaceName: "Notify Space",
		OpenedAt:  "2026-07-01",
		SeatCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		PriceYuan:        "42.50",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "0",
		CustomerEmail:    "customer@example.com",
		CustomerWechat:   "wx-customer-701",
		AccountID:        accountID,
		BoardedAt:        "2026-07-15",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-15", true); err != nil {
		t.Fatal(err)
	}

	page, err := subscriptionService.ListBillsPage()
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Bills) != 1 {
		t.Fatalf("bill count = %d, want 1", len(page.Bills))
	}
	bill := page.Bills[0]
	if bill.CustomerEmail != "customer@example.com" {
		t.Fatalf("customer email = %q, want customer@example.com", bill.CustomerEmail)
	}
	if bill.CustomerWechat != "wx-customer-701" {
		t.Fatalf("customer wechat = %q, want wx-customer-701", bill.CustomerWechat)
	}
	if bill.AccountEmail != "owner@example.com" || bill.AccountSpaceName != "Notify Space" || bill.AccountOpenedAt != "2026-07-01" {
		t.Fatalf("account details = email %q space %q opened %q", bill.AccountEmail, bill.AccountSpaceName, bill.AccountOpenedAt)
	}
}

func TestUpdateBillDoesNotChangeSubscriptionPrice(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "金额账号", "车位1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "金额独立",
		PriceYuan:        "20.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-15", true); err != nil {
		t.Fatal(err)
	}
	bill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-15")
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.UpdateBill(bill.ID, service.BillEditInput{
		AmountYuan: "18.80",
		Note:       "优惠后实收",
	}); err != nil {
		t.Fatal(err)
	}

	updatedBill, err := subscriptionService.Store.GetBill(bill.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedBill.AmountCents != 1880 {
		t.Fatalf("bill amount = %d, want 1880", updatedBill.AmountCents)
	}
	if updatedBill.Note != "优惠后实收" {
		t.Fatalf("bill note = %q, want 优惠后实收", updatedBill.Note)
	}

	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.PricePerPersonCents != 2000 {
		t.Fatalf("subscription price = %d, want 2000 (unchanged)", subscription.PricePerPersonCents)
	}
}

func TestDeleteBillRemovesPaidOccurrence(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "删账单账号", "车位1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "删账单",
		PriceYuan:        "15.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-01", true); err != nil {
		t.Fatal(err)
	}
	bill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-01")
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.DeleteBill(bill.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.Store.GetBill(bill.ID); err == nil {
		t.Fatal("expected bill gone after DeleteBill")
	}
	paid, err := subscriptionService.Store.IsDuePaid(subscriptionID, "2026-07-01")
	if err != nil {
		t.Fatal(err)
	}
	if paid {
		t.Fatal("due should be unpaid after bill delete")
	}
}

func TestArchiveRemovesFromCalendarKeepsBill(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 10, 12, 0, 0, 0, cycle.Location)
	}
	subscriptionID := createTestSubscription(t, subscriptionService, "即将下车", "0 0 * * 1")
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-13", true); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Archive(subscriptionID); err != nil {
		t.Fatal(err)
	}

	month := time.Date(2026, time.July, 1, 0, 0, 0, 0, cycle.Location)
	calendarView, err := subscriptionService.CalendarMonth(month)
	if err != nil {
		t.Fatal(err)
	}
	for _, occurrence := range calendarView.Occurrences {
		if occurrence.SubscriptionID == subscriptionID {
			t.Fatalf("archived subscription still on calendar: %+v", occurrence)
		}
	}

	page, err := subscriptionService.ListBillsPage()
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Bills) != 1 {
		t.Fatalf("bills = %d, want 1", len(page.Bills))
	}
	if !page.Bills[0].Archived || page.Bills[0].StatusLabel != "已下车" {
		t.Fatalf("bill view = %+v, want archived 已下车", page.Bills[0])
	}
	if page.Bills[0].SubscriptionName != "即将下车" {
		t.Fatalf("bill subscription name = %q", page.Bills[0].SubscriptionName)
	}
	if page.Summary.BillCount != 1 || page.Summary.ArchivedCount != 1 || page.Summary.ActiveCount != 0 {
		t.Fatalf("summary counts = %+v", page.Summary)
	}
	if page.Summary.TotalAmountYuan != "20.00" {
		t.Fatalf("total amount = %q, want 20.00", page.Summary.TotalAmountYuan)
	}
}

func TestBillsSummaryAggregatesBySubscriptionAndMonth(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 20, 12, 0, 0, 0, cycle.Location)
	}
	_, aiSeats := createTestAccountWithSeats(t, subscriptionService, "AI 订阅", "位1")
	_, mediaSeats := createTestAccountWithSeats(t, subscriptionService, "流媒体", "位1")
	firstID, err := subscriptionService.Create(service.CreateInput{
		Name:             "甲",
		PriceYuan:        "30.00",
		CronExpr:         "0 0 * * 1",
		NotifyOffsetsRaw: "0",
		SeatID:           aiSeats[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := subscriptionService.Create(service.CreateInput{
		Name:             "乙",
		PriceYuan:        "20.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           mediaSeats[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetDuePaid(firstID, "2026-07-13", true); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetDuePaid(secondID, "2026-07-15", true); err != nil {
		t.Fatal(err)
	}

	page, err := subscriptionService.ListBillsPage()
	if err != nil {
		t.Fatal(err)
	}
	if page.Summary.BillCount != 2 {
		t.Fatalf("bill count = %d, want 2", page.Summary.BillCount)
	}
	if page.Summary.TotalAmountYuan != "50.00" {
		t.Fatalf("total = %q, want 50.00", page.Summary.TotalAmountYuan)
	}
	if page.Summary.ThisMonthCount != 2 || page.Summary.ThisMonthAmountYuan != "50.00" {
		t.Fatalf("this month = count %d amount %s", page.Summary.ThisMonthCount, page.Summary.ThisMonthAmountYuan)
	}
	if len(page.Summary.AmountBySubscription) != 2 {
		t.Fatalf("subscription bars = %d, want 2", len(page.Summary.AmountBySubscription))
	}
	if page.Summary.AmountBySubscription[0].Name != "甲" || page.Summary.AmountBySubscription[0].AmountCents != 3000 {
		t.Fatalf("top subscription bar = %+v", page.Summary.AmountBySubscription[0])
	}
	if len(page.Summary.Accounts) != 2 {
		t.Fatalf("accounts = %d, want 2", len(page.Summary.Accounts))
	}
	if len(page.Summary.MonthlyTrend) != 6 {
		t.Fatalf("monthly trend length = %d, want 6", len(page.Summary.MonthlyTrend))
	}
	var july service.MonthAmountBar
	for _, monthBar := range page.Summary.MonthlyTrend {
		if monthBar.Month == "2026-07" {
			july = monthBar
		}
	}
	if july.Count != 2 || july.AmountCents != 5000 {
		t.Fatalf("july trend = %+v", july)
	}
	if page.Summary.ResaleBillCount != 0 || page.Summary.TotalAgencyFeeYuan != "0.00" {
		t.Fatalf(
			"agency fee without resale = count %d total %s",
			page.Summary.ResaleBillCount,
			page.Summary.TotalAgencyFeeYuan,
		)
	}

	_, resaleSeats := createTestAccountWithSeats(t, subscriptionService, "串货账号", "位1")
	resaleID, err := subscriptionService.Create(service.CreateInput{
		Name:             "串货甲",
		PriceYuan:        "85.00",
		CronExpr:         "0 0 20 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           resaleSeats[0],
		IsResale:         true,
		AgencyFeeYuan:    "5.00",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetDuePaid(resaleID, "2026-07-20", true); err != nil {
		t.Fatal(err)
	}

	pageWithResale, err := subscriptionService.ListBillsPage()
	if err != nil {
		t.Fatal(err)
	}
	if pageWithResale.Summary.BillCount != 3 {
		t.Fatalf("bill count with resale = %d, want 3", pageWithResale.Summary.BillCount)
	}
	if pageWithResale.Summary.ResaleBillCount != 1 {
		t.Fatalf("resale bill count = %d, want 1", pageWithResale.Summary.ResaleBillCount)
	}
	if pageWithResale.Summary.TotalAgencyFeeYuan != "5.00" {
		t.Fatalf("total agency fee = %q, want 5.00", pageWithResale.Summary.TotalAgencyFeeYuan)
	}
	if pageWithResale.Summary.ThisMonthAgencyFeeYuan != "5.00" {
		t.Fatalf("this month agency fee = %q, want 5.00", pageWithResale.Summary.ThisMonthAgencyFeeYuan)
	}
	// Resale bill amount is agency fee; total collected = 30+20+5.
	if pageWithResale.Summary.TotalAmountYuan != "55.00" {
		t.Fatalf("total with resale = %q, want 55.00", pageWithResale.Summary.TotalAmountYuan)
	}
}

func TestCopyCreatesActiveSubscriptionWithoutBills(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 11, 12, 0, 0, 0, cycle.Location)
	}
	_, sourceSeats := createTestAccountWithSeats(t, subscriptionService, "AI 订阅", "源位", "副本位")
	sourceID, err := subscriptionService.Create(service.CreateInput{
		Name:             "源订阅",
		PriceYuan:        "30.00",
		CostYuan:         "10.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "3,1,0",
		Remark:           "原始备注",
		TradeURL:         "https://example.com/pay",
		SeatID:           sourceSeats[0],
		// Boarded before the monthly due day so the copy must keep this date
		// (defaulting to "today" would hide this month's already-passed due).
		BoardedAt: "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetDuePaid(sourceID, "2026-08-01", true); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Archive(sourceID); err != nil {
		t.Fatal(err)
	}

	copyID, err := subscriptionService.Copy(sourceID, sourceSeats[1])
	if err != nil {
		t.Fatal(err)
	}
	if copyID == sourceID {
		t.Fatal("copy should get a new id")
	}

	copied, err := subscriptionService.Get(copyID)
	if err != nil {
		t.Fatal(err)
	}
	if copied.Name != "源订阅（副本）" {
		t.Fatalf("copy name = %q, want 源订阅（副本）", copied.Name)
	}
	if copied.BoardedAt != "2026-07-01" {
		t.Fatalf("copy boarded_at = %q, want source boarded_at 2026-07-01", copied.BoardedAt)
	}
	if copied.PricePerPersonCents != 3000 || copied.CostCents != 1000 {
		t.Fatalf("copy prices = %d/%d, want 3000/1000", copied.PricePerPersonCents, copied.CostCents)
	}
	if copied.Remark != "原始备注" || copied.TradeURL != "https://example.com/pay" {
		t.Fatalf("copy remark/url unexpected: %#v", copied)
	}
	if copied.ArchivedAt != nil {
		t.Fatal("copy must be active (not archived)")
	}

	// Source stays archived and reachable for bill linkage.
	source, err := subscriptionService.Store.GetSubscriptionIncludingArchived(sourceID)
	if err != nil {
		t.Fatal(err)
	}
	if source.ArchivedAt == nil {
		t.Fatal("source should remain archived")
	}

	copyBills, err := subscriptionService.Store.ListBills()
	if err != nil {
		t.Fatal(err)
	}
	for _, bill := range copyBills {
		if bill.SubscriptionID == copyID {
			t.Fatal("copy must not inherit historical bills")
		}
	}

	// Monthly on the 10th; source boarded 2026-07-01 so July 10 must appear for the copy.
	// (If Copy defaulted boarded_at to "today" 2026-07-11, July 10 would be excluded.)
	month := time.Date(2026, time.July, 1, 0, 0, 0, 0, cycle.Location)
	// Re-create a mid-month due copy scenario with cron on the 10th.
	_, midSeats := createTestAccountWithSeats(t, subscriptionService, "tctenet账号", "源", "副本")
	midMonthSourceID, err := subscriptionService.Create(service.CreateInput{
		Name:             "tctenet",
		PriceYuan:        "90.00",
		CronExpr:         "0 0 10 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           midSeats[0],
		BoardedAt:        "2026-07-10",
	})
	if err != nil {
		t.Fatal(err)
	}
	midMonthCopyID, err := subscriptionService.Copy(midMonthSourceID, midSeats[1])
	if err != nil {
		t.Fatal(err)
	}
	view, err := subscriptionService.CalendarMonth(month)
	if err != nil {
		t.Fatal(err)
	}
	copyOccurrenceCount := 0
	for _, occurrence := range view.Occurrences {
		if occurrence.SubscriptionID == midMonthCopyID && occurrence.DueDate == "2026-07-10" {
			copyOccurrenceCount++
		}
	}
	if copyOccurrenceCount != 1 {
		t.Fatalf("copy mid-month due occurrences = %d, want 1 (boarded_at must match source)", copyOccurrenceCount)
	}
}

func TestSoftDeleteArchivedRequiresNoBills(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 10, 12, 0, 0, 0, cycle.Location)
	}

	// Active subscription cannot be soft-deleted via SoftDeleteArchived.
	activeID := createTestSubscription(t, subscriptionService, "仍在进行", "0 0 * * 1")
	if err := subscriptionService.SoftDeleteArchived(activeID); err == nil {
		t.Fatal("expected error soft-deleting active subscription")
	}

	// Archived with bills: blocked.
	withBillsID := createTestSubscription(t, subscriptionService, "有账单", "0 0 * * 1")
	if err := subscriptionService.SetDuePaid(withBillsID, "2026-07-13", true); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Archive(withBillsID); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SoftDeleteArchived(withBillsID); err == nil {
		t.Fatal("expected error when bills still linked")
	}

	// Archived without bills: allowed, disappears from 已下车 tab.
	noBillsID := createTestSubscription(t, subscriptionService, "无账单", "0 0 15 * *")
	if err := subscriptionService.Archive(noBillsID); err != nil {
		t.Fatal(err)
	}
	archived, err := subscriptionService.ListArchivedView()
	if err != nil {
		t.Fatal(err)
	}
	foundNoBills := false
	for _, view := range archived {
		if view.Subscription.ID == noBillsID {
			foundNoBills = true
			if !view.CanSoftDelete || view.BillCount != 0 {
				t.Fatalf("expected CanSoftDelete with 0 bills, got %+v", view)
			}
		}
		if view.Subscription.ID == withBillsID {
			if view.CanSoftDelete || view.BillCount != 1 {
				t.Fatalf("expected blocked delete with 1 bill, got %+v", view)
			}
		}
	}
	if !foundNoBills {
		t.Fatal("expected no-bills archived subscription in list")
	}

	if err := subscriptionService.SoftDeleteArchived(noBillsID); err != nil {
		t.Fatal(err)
	}
	archivedAfter, err := subscriptionService.ListArchivedView()
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range archivedAfter {
		if view.Subscription.ID == noBillsID {
			t.Fatal("soft-deleted subscription should leave archived list")
		}
	}

	// After clearing the only bill, soft-delete becomes allowed.
	if err := subscriptionService.Store.SetDuePaid(withBillsID, "2026-07-13", false, 0); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SoftDeleteArchived(withBillsID); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateSubscriptionPriceSyncsCurrentPeriodBill(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, 7, 20, 12, 0, 0, 0, cycle.Location)
	}
	accountID, seatIDs := createTestAccountWithSeats(t, subscriptionService, "改价账号", "车位1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "改价同步",
		PriceYuan:        "30.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "",
		AccountID:        accountID,
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-01", true); err != nil {
		t.Fatal(err)
	}
	bill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-01")
	if err != nil {
		t.Fatal(err)
	}
	if bill.AmountCents != 3000 {
		t.Fatalf("initial bill = %d, want 3000", bill.AmountCents)
	}

	if err := subscriptionService.Update(subscriptionID, service.CreateInput{
		Name:             "改价同步",
		PriceYuan:        "45.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "",
		AccountID:        accountID,
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-01-01",
	}); err != nil {
		t.Fatal(err)
	}

	updated, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-01")
	if err != nil {
		t.Fatal(err)
	}
	if updated.AmountCents != 4500 {
		t.Fatalf("synced bill = %d, want 4500", updated.AmountCents)
	}
}

func TestUpdateLegacyResaleSubscriptionPreservesAccountingFields(t *testing.T) {
	subscriptionService := openTestService(t)
	accountID, seatIDs := createTestAccountWithSeats(t, subscriptionService, "legacy-account", "seat-1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "legacy-resale",
		PriceYuan:        "30.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "",
		AccountID:        accountID,
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-01-01",
		IsResale:         true,
		AgencyFeeYuan:    "5.00",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := subscriptionService.Update(subscriptionID, service.CreateInput{
		Name:             "updated-name",
		PriceYuan:        "30.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "",
		AccountID:        accountID,
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-01-01",
	}); err != nil {
		t.Fatal(err)
	}

	updated, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.IsResale {
		t.Fatal("legacy resale marker was cleared during an unrelated edit")
	}
	if updated.AgencyFeeCents != 500 {
		t.Fatalf("agency fee = %d, want 500", updated.AgencyFeeCents)
	}
}

func TestCreateAllowsEmptyNotifyOffsets(t *testing.T) {
	subscriptionService := openTestService(t)
	accountID, seatIDs := createTestAccountWithSeats(t, subscriptionService, "无提醒账号", "车位1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		PriceYuan:        "10.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "",
		AccountID:        accountID,
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(subscription.NotifyOffsets) != 0 {
		t.Fatalf("offsets = %#v, want empty", subscription.NotifyOffsets)
	}
	if subscription.Name == "" {
		t.Fatal("expected auto-generated name")
	}
}
