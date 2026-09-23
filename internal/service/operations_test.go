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
		ZeroRenewalNextMonth: false,
		SeatCount:            2,
	}); err != nil {
		t.Fatal(err)
	}
	zeroAccountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:                 "zero-renewal@example.com",
		Email:                "zero-renewal@example.com",
		OpenedAt:             "2026-07-25",
		CostYuan:             "75.00",
		ZeroRenewalNextMonth: true,
		SeatCount:            1,
	})
	if err != nil {
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
	if overview.Capacity.SeatTotal != 3 || overview.Capacity.SeatUsed != 1 || overview.Capacity.SeatFree != 2 {
		t.Fatalf("capacity = %#v, want total=3 used=1 free=2", overview.Capacity)
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
	foundZeroRenewal := false
	for _, task := range overview.Tasks {
		switch task.Kind {
		case "team_overdue":
			foundOverdue = task.CustomerEmail == "customer@example.com" && task.AmountYuan == "100.00"
		case "account_renewal":
			foundAccountRenewal = task.AccountID == accountID &&
				task.DueDate == "2026-08-25" &&
				task.AmountYuan == "75.00"
			foundZeroRenewal = foundZeroRenewal || task.AccountID == zeroAccountID
		}
	}
	if !foundOverdue || !foundAccountRenewal || foundZeroRenewal {
		t.Fatalf("tasks = %#v, want paid renewal work and no $0 renewal task", overview.Tasks)
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

	acknowledgedID := overview.Notifications[0].ID
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
	if overview.Tasks[0].ID != acknowledgedID {
		t.Fatalf("viewed task lost its operational priority: first=%q, want %q", overview.Tasks[0].ID, acknowledgedID)
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

func TestOperationsOverviewNotificationFailureRecoveryAndNewOccurrence(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.September, 23, 12, 0, 0, 0, cycle.Location)
	}
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "notification recovery",
		BusinessType:     model.SubscriptionBusinessPlus,
		PriceYuan:        "68.00",
		CostYuan:         "20.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "recovery@example.com",
		CustomerWechat:   "recovery-wechat",
		BoardedAt:        "2026-09-20",
	})
	if err != nil {
		t.Fatal(err)
	}

	firstFailure, err := subscriptionService.Store.UpsertPendingNotification(
		subscriptionID, "2026-10-20", 3, model.ChannelSMTP, model.NotificationKindScheduled,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Store.MarkNotificationFailure(firstFailure.ID, 5, "smtp failed", nil, true); err != nil {
		t.Fatal(err)
	}

	findFailureTask := func(overview service.OperationsOverview) *service.OperationTask {
		t.Helper()
		for index := range overview.Notifications {
			if overview.Notifications[index].Kind == "notification_failed" {
				return &overview.Notifications[index]
			}
		}
		return nil
	}

	overview, err := subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	firstTask := findFailureTask(overview)
	if overview.Work.FailedNotificationCount != 1 || overview.Work.UrgentCount != 1 || firstTask == nil {
		t.Fatalf("overview with failure = %#v, want one urgent notification task", overview)
	}
	firstTaskID := fmt.Sprintf("notification-failures:%d", firstFailure.ID)
	if firstTask.ID != firstTaskID || firstTask.Name != "1" {
		t.Fatalf("failure task = %#v, want occurrence %q with count 1", firstTask, firstTaskID)
	}
	if !firstTask.Unread {
		t.Fatalf("initial failure task = %#v, want unread", firstTask)
	}
	if count, acknowledgeErr := subscriptionService.AcknowledgeOperationTasks([]string{firstTaskID}); acknowledgeErr != nil || count != 1 {
		t.Fatalf("acknowledge first failure occurrence = %d, %v", count, acknowledgeErr)
	}
	overview, err = subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	firstTask = findFailureTask(overview)
	if firstTask == nil || firstTask.Unread {
		t.Fatalf("acknowledged failure task = %#v, want unresolved but read", firstTask)
	}

	if err := subscriptionService.Store.RecordManualCustomerEmailSuccess(subscriptionID); err != nil {
		t.Fatal(err)
	}
	overview, err = subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Dashboard.NotifyFailed30d != 1 || len(overview.Dashboard.NotificationActivity) != 1 {
		t.Fatalf("historical dashboard after recovery = %#v, want preserved failed audit row", overview.Dashboard)
	}
	if overview.Work.FailedNotificationCount != 0 || overview.Work.UrgentCount != 0 || findFailureTask(overview) != nil {
		t.Fatalf("operations after recovery = %#v, want no unresolved failure", overview.Work)
	}

	secondFailure, err := subscriptionService.Store.UpsertPendingNotification(
		subscriptionID, "2026-11-19", 3, model.ChannelSMTP, model.NotificationKindScheduled,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Store.MarkNotificationFailure(secondFailure.ID, 1, "smtp failed again", nil, true); err != nil {
		t.Fatal(err)
	}
	overview, err = subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	secondTask := findFailureTask(overview)
	secondTaskID := fmt.Sprintf("notification-failures:%d", secondFailure.ID)
	if overview.Work.FailedNotificationCount != 1 || secondTask == nil || secondTask.ID != secondTaskID || !secondTask.Unread {
		t.Fatalf("new failure occurrence = %#v / %#v, want unread task %q", overview.Work, secondTask, secondTaskID)
	}
	if secondTask.ID == firstTaskID {
		t.Fatalf("new failure reused acknowledged occurrence ID %q", secondTask.ID)
	}
}

func TestOperationsOverviewNotificationFailuresAreIsolatedBySubscriptionAndChannel(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.September, 23, 12, 0, 0, 0, cycle.Location)
	}
	createSubscription := func(name, email string) int64 {
		t.Helper()
		id, createErr := subscriptionService.CreateWithInitialBill(service.CreateInput{
			Name:             name,
			BusinessType:     model.SubscriptionBusinessPlus,
			PriceYuan:        "68.00",
			CostYuan:         "20.00",
			CronExpr:         "interval:30d",
			NotifyOffsetsRaw: "3",
			CustomerEmail:    email,
			CustomerWechat:   name + "-wechat",
			BoardedAt:        "2026-09-20",
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return id
	}
	fail := func(subscriptionID int64, dueDate, channel, message string) {
		t.Helper()
		log, logErr := subscriptionService.Store.UpsertPendingNotification(
			subscriptionID, dueDate, 3, channel, model.NotificationKindScheduled,
		)
		if logErr != nil {
			t.Fatal(logErr)
		}
		if logErr = subscriptionService.Store.MarkNotificationFailure(log.ID, 5, message, nil, true); logErr != nil {
			t.Fatal(logErr)
		}
	}

	firstSubscriptionID := createSubscription("first channels", "first@example.com")
	fail(firstSubscriptionID, "2026-10-20", model.ChannelSMTP, "smtp failed")
	fail(firstSubscriptionID, "2026-10-20", model.ChannelIYUU, "iyuu failed")
	if err := subscriptionService.Store.RecordManualCustomerEmailSuccess(firstSubscriptionID); err != nil {
		t.Fatal(err)
	}
	overview, err := subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Work.FailedNotificationCount != 1 {
		t.Fatalf("failure count after SMTP recovery = %d, want unresolved IYUU failure", overview.Work.FailedNotificationCount)
	}

	secondSubscriptionID := createSubscription("second channels", "second@example.com")
	fail(secondSubscriptionID, "2026-10-20", model.ChannelSMTP, "second smtp failed")
	overview, err = subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Work.FailedNotificationCount != 2 {
		t.Fatalf("failure count across subscription/channel pairs = %d, want 2", overview.Work.FailedNotificationCount)
	}
	if err := subscriptionService.Store.RecordManualCustomerEmailSuccess(secondSubscriptionID); err != nil {
		t.Fatal(err)
	}
	overview, err = subscriptionService.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	if overview.Work.FailedNotificationCount != 1 {
		t.Fatalf("failure count after one subscription recovers = %d, want 1", overview.Work.FailedNotificationCount)
	}
}
