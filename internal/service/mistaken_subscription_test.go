package service_test

import (
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/service"
)

func TestDeleteMistakenTeamRegistrationReversesFinanceAndReleasesSeat(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.September, 8, 12, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "Team owner", "seat 1")
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "mistaken Team user",
		PriceYuan:        "90.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3,1,0",
		CustomerEmail:    "mistaken@example.com",
		CustomerWechat:   "mistaken-wechat",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-09-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	before, err := subscriptionService.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if before.SubscriptionCount != 1 || before.TotalAmountYuan != "90.00" || before.TotalProfitYuan != "90.00" {
		t.Fatalf("dashboard before delete = %#v", before)
	}

	if err := subscriptionService.DeleteMistakenTeamRegistration(subscriptionID); err != nil {
		t.Fatal(err)
	}
	after, err := subscriptionService.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if after.SubscriptionCount != 0 || after.TotalAmountYuan != "0.00" || after.TotalProfitYuan != "0.00" {
		t.Fatalf("dashboard after delete = %#v", after)
	}
	bills, err := subscriptionService.Store.ListBills()
	if err != nil {
		t.Fatal(err)
	}
	if len(bills) != 0 {
		t.Fatalf("bills after delete = %d, want 0", len(bills))
	}
	afterSales, err := subscriptionService.Store.ListAfterSalesCases()
	if err != nil {
		t.Fatal(err)
	}
	if len(afterSales) != 0 {
		t.Fatalf("after-sales after delete = %d, want 0", len(afterSales))
	}

	if _, err := subscriptionService.Create(service.CreateInput{
		Name:             "replacement Team user",
		PriceYuan:        "95.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3,1,0",
		CustomerEmail:    "replacement@example.com",
		CustomerWechat:   "replacement-wechat",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-09-08",
	}); err != nil {
		t.Fatalf("seat was not released immediately: %v", err)
	}

	if err := subscriptionService.DeleteMistakenTeamRegistration(subscriptionID); err == nil ||
		!strings.Contains(err.Error(), "不存在或已被删除") {
		t.Fatalf("second delete error = %v", err)
	}
}
