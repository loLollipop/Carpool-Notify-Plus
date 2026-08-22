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

	inserted, err := subscriptionService.MarkAccountRenewed(accountID, "2026-08-25")
	if err != nil || !inserted {
		t.Fatalf("mark account renewed = %v, %v", inserted, err)
	}
	overview, err = subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Work.AccountRenewalCount != 0 {
		t.Fatalf("account renewal count after marking = %d, want 0", overview.Work.AccountRenewalCount)
	}
	for _, task := range overview.Tasks {
		if task.Kind == "account_renewal" && task.AccountID == accountID {
			t.Fatalf("renewed account still appears in tasks: %#v", task)
		}
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
	if len(overview.Notifications) != 26 {
		t.Fatalf("notification list length = %d, want all 26 tasks", len(overview.Notifications))
	}
	if overview.Unread.PlusCount != 25 || overview.Unread.TeamCount != 1 || overview.Unread.CalendarCount != 26 {
		t.Fatalf("unread counts = %#v, want all subscription tasks", overview.Unread)
	}

	acknowledgedID := overview.Notifications[len(overview.Notifications)-1].ID
	if count, err := subscriptionService.AcknowledgeOperationTasks([]string{acknowledgedID}); err != nil || count != 1 {
		t.Fatalf("acknowledge task = %d, %v, want 1, nil", count, err)
	}
	overview, err = subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Unread.CalendarCount != 25 {
		t.Fatalf("calendar unread after acknowledgement = %d, want 25", overview.Unread.CalendarCount)
	}
	if overview.Work.PlusDueCount != 25 || overview.Work.TeamDueCount != 1 {
		t.Fatalf("business work changed after acknowledgement: %#v", overview.Work)
	}
}

func TestOperationAcknowledgementIsOccurrenceSpecific(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 22, 12, 0, 0, 0, cycle.Location)
	}

	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "Daily Plus customer",
		BusinessType:     model.SubscriptionBusinessPlus,
		PriceYuan:        "68.00",
		CostYuan:         "20.00",
		CronExpr:         "interval:1d",
		NotifyOffsetsRaw: "1",
		CustomerEmail:    "daily@example.com",
		CustomerWechat:   "daily-wechat",
		BoardedAt:        "2026-08-20",
	})
	if err != nil {
		t.Fatal(err)
	}

	overview, err := subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Notifications) != 1 || !overview.Notifications[0].Unread {
		t.Fatalf("initial notifications = %#v, want one unread task", overview.Notifications)
	}
	firstTaskID := overview.Notifications[0].ID
	if count, acknowledgeErr := subscriptionService.AcknowledgeOperationTasks([]string{firstTaskID}); acknowledgeErr != nil || count != 1 {
		t.Fatalf("acknowledge first occurrence = %d, %v", count, acknowledgeErr)
	}

	overview, err = subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Unread.PlusCount != 0 || overview.Work.PlusDueCount != 1 || overview.Notifications[0].Unread {
		t.Fatalf("acknowledged overview = %#v, want unresolved but read", overview)
	}

	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-08-21", true); err != nil {
		t.Fatal(err)
	}
	overview, err = subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	if len(overview.Notifications) != 1 || !overview.Notifications[0].Unread {
		t.Fatalf("next occurrence notifications = %#v, want one unread task", overview.Notifications)
	}
	if overview.Notifications[0].ID == firstTaskID || overview.Notifications[0].DueDate != "2026-08-22" {
		t.Fatalf("next occurrence = %#v, want a new 2026-08-22 task", overview.Notifications[0])
	}
}
