package service_test

import (
	"fmt"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"
)

func TestOperationsOverviewPrioritizesWorkAndSummarizesCapacity(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 22, 12, 0, 0, 0, cycle.Location)
	}
	accountID, seatIDs := createTestAccountWithSeats(
		t,
		subscriptionService,
		"owner@example.com",
		"车位1",
		"车位2",
	)
	if err := subscriptionService.UpdateAccount(accountID, service.UpdateAccountInput{
		Name:                 "owner@example.com",
		Email:                "owner@example.com",
		OpenedAt:             "2026-07-25",
		CostYuan:             "75.00",
		ZeroRenewalNextMonth: true,
		SeatCount:            2,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "Team 逾期客户",
		PriceYuan:        "100.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "7,3,1",
		CustomerEmail:    "customer@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-01",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.CreateBusinessGoal(service.BusinessGoalInput{
		Name:             "年度利润",
		TargetProfitYuan: "10000",
	}); err != nil {
		t.Fatal(err)
	}

	overview, err := subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Capacity.SeatTotal != 2 || overview.Capacity.SeatUsed != 1 || overview.Capacity.SeatFree != 1 {
		t.Fatalf("capacity = %#v, want total=2 used=1 free=1", overview.Capacity)
	}
	if overview.Work.OverdueCount != 1 || overview.Work.OverdueAmountYuan != "100.00" {
		t.Fatalf("overdue summary = %#v, want one ¥100 task", overview.Work)
	}
	if overview.Work.TeamDueCount != 1 || overview.Work.PlusDueCount != 0 || overview.Work.AccountRenewalCount != 1 {
		t.Fatalf("work counts = %#v, want team=1 plus=0 account renewal=1", overview.Work)
	}
	if overview.Goal == nil || overview.Goal.Name != "年度利润" {
		t.Fatalf("goal summary = %#v, want active goal", overview.Goal)
	}

	foundOverdue := false
	foundAccountRenewal := false
	for _, task := range overview.Tasks {
		switch task.Kind {
		case "team_overdue":
			foundOverdue = task.CustomerEmail == "customer@example.com" && task.AmountYuan == "100.00"
		case "account_renewal":
			foundAccountRenewal = task.AccountID == accountID &&
				task.DueDate == "2026-08-25" &&
				task.AmountYuan == "0.00"
		}
	}
	if !foundOverdue || !foundAccountRenewal {
		t.Fatalf("tasks = %#v, want overdue subscription and upcoming account renewal", overview.Tasks)
	}
}

func TestOperationsOverviewWorkCountsAreNotCappedWithTaskList(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 22, 12, 0, 0, 0, cycle.Location)
	}

	for index := 0; index < 25; index++ {
		if _, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
			Name:             fmt.Sprintf("Plus customer %02d", index+1),
			BusinessType:     model.SubscriptionBusinessPlus,
			PriceYuan:        "68.00",
			CostYuan:         "20.00",
			CronExpr:         "interval:30d",
			NotifyOffsetsRaw: "7,3,1",
			CustomerEmail:    fmt.Sprintf("plus-%02d@example.com", index+1),
			CustomerWechat:   fmt.Sprintf("wx-plus-%02d", index+1),
			BoardedAt:        "2026-07-01",
		}); err != nil {
			t.Fatal(err)
		}
	}

	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "Team owner", "车位1")
	if _, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "Team customer",
		PriceYuan:        "100.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "7,3,1",
		CustomerEmail:    "team@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-01",
	}); err != nil {
		t.Fatal(err)
	}

	overview, err := subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Tasks) != 24 {
		t.Fatalf("task list length = %d, want display cap 24", len(overview.Tasks))
	}
	if overview.Work.PlusDueCount != 25 || overview.Work.TeamDueCount != 1 {
		t.Fatalf("work counts = %#v, want all 25 Plus and 1 Team tasks", overview.Work)
	}
}
