package service_test

import (
	"context"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
	"carpool-notify/internal/notify"
	"carpool-notify/internal/service"
)

func TestCreateRequiresSeatAndPreservesAccount(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "SaaS 工具", "位置1")

	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "Cursor Pro 拼车",
		PriceYuan:        "35.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "3,1,0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}

	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.SeatID != seatIDs[0] {
		t.Fatalf("SeatID = %d, want %d", subscription.SeatID, seatIDs[0])
	}
	if subscription.AccountName != "SaaS 工具" {
		t.Fatalf("AccountName = %q, want SaaS 工具", subscription.AccountName)
	}
}

func TestCreateRejectsMissingSeat(t *testing.T) {
	subscriptionService := openTestService(t)
	_, err := subscriptionService.Create(service.CreateInput{
		Name:             "无车位",
		PriceYuan:        "10.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
	})
	if err == nil {
		t.Fatal("Create accepted missing seat")
	}
}

func TestCreateAutoAssignsFreeSeatFromAccount(t *testing.T) {
	subscriptionService := openTestService(t)
	accountID, seatIDs := createTestAccountWithSeats(t, subscriptionService, "自动占位账号", "车位A", "车位B")

	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "只选账号",
		PriceYuan:        "18.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		AccountID:        accountID,
	})
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.SeatID != seatIDs[0] {
		t.Fatalf("SeatID = %d, want first free seat %d", subscription.SeatID, seatIDs[0])
	}
	if subscription.AccountID != accountID {
		t.Fatalf("AccountID = %d, want %d", subscription.AccountID, accountID)
	}
}

func TestCreateRejectsFullAccountWithoutSeat(t *testing.T) {
	subscriptionService := openTestService(t)
	accountID, seatIDs := createTestAccountWithSeats(t, subscriptionService, "已满账号", "唯一车位")
	_, err := subscriptionService.Create(service.CreateInput{
		Name:             "占满",
		PriceYuan:        "10.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = subscriptionService.Create(service.CreateInput{
		Name:             "再占",
		PriceYuan:        "11.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		AccountID:        accountID,
	})
	if err == nil {
		t.Fatal("Create accepted full account")
	}
}

func TestCreateRejectsOccupiedSeat(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "满位账号", "唯一车位")

	_, err := subscriptionService.Create(service.CreateInput{
		Name:             "第一位",
		PriceYuan:        "10.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = subscriptionService.Create(service.CreateInput{
		Name:             "第二位",
		PriceYuan:        "12.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err == nil {
		t.Fatal("Create accepted occupied seat")
	}
}

func TestArchiveFreesSeatForReuse(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "可复用账号", "车位A")

	firstID, err := subscriptionService.Create(service.CreateInput{
		Name:             "先上车",
		PriceYuan:        "10.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Archive(firstID); err != nil {
		t.Fatal(err)
	}

	secondID, err := subscriptionService.Create(service.CreateInput{
		Name:             "再上车",
		PriceYuan:        "11.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	if secondID == firstID {
		t.Fatal("expected a new subscription id after reusing seat")
	}
}

func TestAccountFullBlocksNewSubscription(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "双位账号", "位1", "位2")

	for index, seatID := range seatIDs {
		_, err := subscriptionService.Create(service.CreateInput{
			Name:             "订阅" + string(rune('A'+index)),
			PriceYuan:        "10.00",
			CronExpr:         "0 0 1 * *",
			NotifyOffsetsRaw: "0",
			SeatID:           seatID,
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// No free seats: extra seat would be needed; creating on either occupied seat fails.
	_, err := subscriptionService.Create(service.CreateInput{
		Name:             "超额",
		PriceYuan:        "10.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err == nil {
		t.Fatal("Create should fail when all seats are occupied")
	}
}

func TestDeleteSeatBlockedWhenOccupied(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "删位账号", "忙车位")
	_, err := subscriptionService.Create(service.CreateInput{
		Name:             "占位",
		PriceYuan:        "10.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.DeleteSeat(seatIDs[0]); err == nil {
		t.Fatal("DeleteSeat should fail for occupied seat")
	}
}

func TestDeleteSeatClearsHistoricalLinks(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "历史车位账号", "可删位")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "已下车",
		PriceYuan:        "10.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Archive(subscriptionID); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.DeleteSeat(seatIDs[0]); err != nil {
		t.Fatalf("DeleteSeat free seat with history: %v", err)
	}
	archived, err := subscriptionService.Store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.SeatID != 0 {
		t.Fatalf("SeatID after clear = %d, want 0", archived.SeatID)
	}
}

func TestDeleteAccountBlockedWhenActive(t *testing.T) {
	subscriptionService := openTestService(t)
	accountID, seatIDs := createTestAccountWithSeats(t, subscriptionService, "活跃账号", "车位1")
	_, err := subscriptionService.Create(service.CreateInput{
		Name:             "占用中",
		PriceYuan:        "10.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.DeleteAccount(accountID); err == nil {
		t.Fatal("DeleteAccount should fail while active subscriptions remain")
	}
}

func TestDeleteAccountCascadesFreeSeats(t *testing.T) {
	subscriptionService := openTestService(t)
	accountID, _ := createTestAccountWithSeats(t, subscriptionService, "空闲账号", "车位1", "车位2")
	if err := subscriptionService.DeleteAccount(accountID); err != nil {
		t.Fatalf("DeleteAccount free account: %v", err)
	}
	if _, err := subscriptionService.Store.GetAccount(accountID); err == nil {
		t.Fatal("expected account deleted")
	}
}

func TestCreateAccountBySeatCount(t *testing.T) {
	subscriptionService := openTestService(t)
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "数量建号",
		SeatCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := subscriptionService.Store.ListSeatsByAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(seats) != 3 {
		t.Fatalf("seat count = %d, want 3", len(seats))
	}
	if seats[0].Name != "车位1" || seats[2].Name != "车位3" {
		t.Fatalf("seat names = %q, %q, %q", seats[0].Name, seats[1].Name, seats[2].Name)
	}

	if _, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "数量非法",
		SeatCount: 0,
	}); err == nil {
		t.Fatal("expected error for seat_count=0")
	}
	if _, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "数量过大",
		SeatCount: 1001,
	}); err == nil {
		t.Fatal("expected error for seat_count=1001")
	}
}

func TestAccountDetailsAndDefaultSubscriptionCost(t *testing.T) {
	subscriptionService := openTestService(t)
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:                 "Team 母号",
		Email:                "Owner <owner@example.com>",
		SpaceName:            "Team Workspace",
		OpenedAt:             "2026-07-01",
		CostYuan:             "18.50",
		ZeroRenewalNextMonth: true,
		SeatCount:            1,
	})
	if err != nil {
		t.Fatal(err)
	}
	account, err := subscriptionService.Store.GetAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if account.Email != "owner@example.com" || account.SpaceName != "Team Workspace" || account.OpenedAt != "2026-07-01" {
		t.Fatalf("account details = email %q space %q opened %q", account.Email, account.SpaceName, account.OpenedAt)
	}
	if account.CostCents != 1850 || !account.ZeroRenewalNextMonth {
		t.Fatalf("account cost = %d zero = %v, want 1850 / true", account.CostCents, account.ZeroRenewalNextMonth)
	}

	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		PriceYuan:        "30.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "0",
		AccountID:        accountID,
		BoardedAt:        "2026-07-15",
	})
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.CostCents != 1850 {
		t.Fatalf("subscription cost = %d, want account default 1850", subscription.CostCents)
	}
}

func TestUpdateAccountResizesSeatCount(t *testing.T) {
	subscriptionService := openTestService(t)
	accountID, seatIDs := createTestAccountWithSeats(t, subscriptionService, "可调容量", "位1", "位2")
	_, err := subscriptionService.Create(service.CreateInput{
		Name:             "占一位",
		PriceYuan:        "10.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := subscriptionService.UpdateAccount(accountID, service.UpdateAccountInput{
		Name:      "可调容量",
		SeatCount: 4,
	}); err != nil {
		t.Fatal(err)
	}
	seats, err := subscriptionService.Store.ListSeatsByAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(seats) != 4 {
		t.Fatalf("after grow seat count = %d, want 4", len(seats))
	}

	if err := subscriptionService.UpdateAccount(accountID, service.UpdateAccountInput{
		Name:      "可调容量",
		SeatCount: 1,
	}); err != nil {
		t.Fatal(err)
	}
	seats, err = subscriptionService.Store.ListSeatsByAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(seats) != 1 {
		t.Fatalf("after shrink seat count = %d, want 1", len(seats))
	}

	// Cannot shrink below active occupancy.
	if err := subscriptionService.UpdateAccount(accountID, service.UpdateAccountInput{
		Name:      "可调容量",
		SeatCount: 0, // API: 0 means leave seat count unchanged
	}); err != nil {
		t.Fatal(err)
	}
	seats, err = subscriptionService.Store.ListSeatsByAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(seats) != 1 {
		t.Fatalf("seat count after SeatCount=0 = %d, want unchanged 1", len(seats))
	}
}

func TestCopyRequiresFreeSeat(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "复制账号", "源位", "空闲位")

	sourceID, err := subscriptionService.Create(service.CreateInput{
		Name:             "源订阅",
		PriceYuan:        "10.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := subscriptionService.Copy(sourceID, seatIDs[0]); err == nil {
		t.Fatal("Copy onto occupied seat should fail")
	}
	copiedID, err := subscriptionService.Copy(sourceID, seatIDs[1])
	if err != nil {
		t.Fatal(err)
	}
	copied, err := subscriptionService.Get(copiedID)
	if err != nil {
		t.Fatal(err)
	}
	if copied.SeatID != seatIDs[1] {
		t.Fatalf("copied SeatID = %d, want %d", copied.SeatID, seatIDs[1])
	}
}

func TestComputeDashboardAggregatesByAccount(t *testing.T) {
	subscriptionService := openTestService(t)
	fixedNow := time.Date(2026, time.July, 10, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return fixedNow }
	subscriptionService.Notify = notify.Registry{Gotify: &recordingSender{}}

	_, saasSeats := createTestAccountWithSeats(t, subscriptionService, "SaaS 工具", "位1")
	_, mediaSeats := createTestAccountWithSeats(t, subscriptionService, "流媒体", "位1")

	_, err := subscriptionService.Create(service.CreateInput{
		Name:             "Cursor Pro 拼车",
		PriceYuan:        "35.00",
		CostYuan:         "20.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           saasSeats[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "Netflix 共享",
		PriceYuan:        "12.50",
		CostYuan:         "10.00",
		CronExpr:         "0 0 * * 1",
		NotifyOffsetsRaw: "0",
		SeatID:           mediaSeats[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SaveEnabledChannels([]string{model.ChannelGotify}); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.TestNotify(context.Background(), subscriptionID); err != nil {
		t.Fatal(err)
	}

	dashboard, err := subscriptionService.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}

	if dashboard.SubscriptionCount != 2 {
		t.Fatalf("SubscriptionCount = %d, want 2", dashboard.SubscriptionCount)
	}
	if dashboard.TotalAmountYuan != "47.50" {
		t.Fatalf("TotalAmountYuan = %q, want %q", dashboard.TotalAmountYuan, "47.50")
	}
	if dashboard.TotalCostYuan != "30.00" {
		t.Fatalf("TotalCostYuan = %q, want %q", dashboard.TotalCostYuan, "30.00")
	}
	if dashboard.TotalProfitYuan != "17.50" {
		t.Fatalf("TotalProfitYuan = %q, want %q", dashboard.TotalProfitYuan, "17.50")
	}
	if dashboard.ProfitMarginPercent != "36.8%" {
		t.Fatalf("ProfitMarginPercent = %q, want %q", dashboard.ProfitMarginPercent, "36.8%")
	}
	if dashboard.NotifySuccess30d != 1 {
		t.Fatalf("NotifySuccess30d = %d, want 1", dashboard.NotifySuccess30d)
	}
	if len(dashboard.Accounts) != 2 {
		t.Fatalf("Accounts len = %d, want 2", len(dashboard.Accounts))
	}
}

func TestCreateAndGetPreservesCostCents(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "成本账号", "位1")

	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "带成本的拼车",
		PriceYuan:        "30.00",
		CostYuan:         "18.50",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.CostCents != 1850 {
		t.Fatalf("CostCents = %d, want 1850", subscription.CostCents)
	}
}

func TestCreateDefaultsEmptyCostToZero(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "无成本账号", "位1")

	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "无成本",
		PriceYuan:        "10.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.CostCents != 0 {
		t.Fatalf("CostCents = %d, want 0", subscription.CostCents)
	}
}

func TestCalendarOccurrenceIncludesAccountAndDaysRemaining(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 10, 12, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "SaaS 工具", "席位")
	_, err := subscriptionService.Create(service.CreateInput{
		Name:             "Cursor Pro 拼车（年度席位）",
		PriceYuan:        "35.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "3,1,0",
		SeatID:           seatIDs[0],
		TradeURL:         "https://example.com/cursor",
		BoardedAt:        "2000-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	view, err := subscriptionService.CalendarMonth(time.Date(2026, time.July, 1, 0, 0, 0, 0, cycle.Location))
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Occurrences) != 1 {
		t.Fatalf("occurrences = %d, want 1", len(view.Occurrences))
	}
	occurrence := view.Occurrences[0]
	if occurrence.AccountName != "SaaS 工具" {
		t.Fatalf("AccountName = %q, want SaaS 工具", occurrence.AccountName)
	}
	if occurrence.DaysRemaining != 5 {
		t.Fatalf("DaysRemaining = %d, want 5", occurrence.DaysRemaining)
	}
	if occurrence.TradeURL != "https://example.com/cursor" {
		t.Fatalf("TradeURL = %q", occurrence.TradeURL)
	}
}
