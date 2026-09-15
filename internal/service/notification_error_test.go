package service_test

import (
	"context"
	"testing"
	"time"

	"carpool-notify/internal/config"
	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
	"carpool-notify/internal/notify"
	"carpool-notify/internal/service"
)

func TestManualCustomerEmailHidesRecoveredErrorWithoutRewritingHistory(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.September, 15, 12, 0, 0, 0, cycle.Location)
	}
	recorder := &recordingSender{}
	subscriptionService.Config = config.Config{
		SMTPHost:     "smtp.example.com",
		SMTPPort:     587,
		SMTPUsername: "sender@example.com",
		SMTPPassword: "secret",
		SMTPFrom:     "sender@example.com",
	}
	subscriptionService.Notify = notify.Registry{SMTP: recorder}

	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "recovered account", "seat1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "recovered customer",
		PriceYuan:        "115.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "customer@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-08-19",
	})
	if err != nil {
		t.Fatal(err)
	}

	oldLog, err := subscriptionService.Store.UpsertPendingNotification(
		subscriptionID,
		"2026-08-19",
		3,
		model.ChannelSMTP,
		model.NotificationKindScheduled,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Store.MarkNotificationFailure(oldLog.ID, 5, "old smtp failure", nil, true); err != nil {
		t.Fatal(err)
	}
	currentLog, err := subscriptionService.Store.UpsertPendingNotification(
		subscriptionID,
		"2026-09-18",
		3,
		model.ChannelSMTP,
		model.NotificationKindScheduled,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Store.MarkNotificationFailure(currentLog.ID, 5, "domain not verified", nil, true); err != nil {
		t.Fatal(err)
	}

	views, err := subscriptionService.ListView()
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].LastError != "domain not verified" {
		t.Fatalf("last error before recovery = %#v, want current SMTP failure", views)
	}

	if err := subscriptionService.SendCustomerEmail(context.Background(), subscriptionID); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 1 {
		t.Fatalf("manual SMTP sends = %d, want 1", recorder.calls)
	}

	for attempt := 0; attempt < 2; attempt++ {
		currentLog, err = subscriptionService.Store.GetNotificationLogByID(currentLog.ID)
		if err != nil {
			t.Fatal(err)
		}
		if currentLog.Status != model.NotificationStatusFailed || currentLog.LastError != "domain not verified" {
			t.Fatalf("current historical log after recovery %d = %#v, want preserved failure", attempt, currentLog)
		}
		oldLog, err = subscriptionService.Store.GetNotificationLogByID(oldLog.ID)
		if err != nil {
			t.Fatal(err)
		}
		if oldLog.Status != model.NotificationStatusFailed || oldLog.LastError != "old smtp failure" {
			t.Fatalf("older historical log after recovery %d = %#v, want preserved failure", attempt, oldLog)
		}

		views, err = subscriptionService.ListView()
		if err != nil {
			t.Fatal(err)
		}
		if len(views) != 1 || views[0].LastError != "" {
			t.Fatalf("last error after recovery %d = %#v, want cleared card warning", attempt, views)
		}
		if attempt == 0 {
			if err := subscriptionService.SendCustomerEmail(context.Background(), subscriptionID); err != nil {
				t.Fatal(err)
			}
		}
	}
	if recorder.calls != 2 {
		t.Fatalf("manual SMTP sends after repeat = %d, want 2", recorder.calls)
	}
}

func TestLatestErrorRequiresSuccessfulRecoveryOutcome(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "outcome account", "seat1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "outcome customer",
		PriceYuan:        "35.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "customer@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-08-19",
	})
	if err != nil {
		t.Fatal(err)
	}

	failed, err := subscriptionService.Store.UpsertPendingNotification(
		subscriptionID, "2026-09-18", 3, model.ChannelSMTP, model.NotificationKindScheduled,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Store.MarkNotificationFailure(failed.ID, 5, "smtp failure", nil, true); err != nil {
		t.Fatal(err)
	}
	pending, err := subscriptionService.Store.UpsertPendingNotification(
		subscriptionID, "2026-10-18", 3, model.ChannelSMTP, model.NotificationKindScheduled,
	)
	if err != nil {
		t.Fatal(err)
	}

	assertLastError := func(want string) {
		t.Helper()
		views, listErr := subscriptionService.ListView()
		if listErr != nil {
			t.Fatal(listErr)
		}
		if len(views) != 1 || views[0].LastError != want {
			t.Fatalf("last error = %#v, want %q", views, want)
		}
	}
	assertLastError("smtp failure")
	if err := subscriptionService.Store.MarkNotificationCanceled(pending.ID); err != nil {
		t.Fatal(err)
	}
	assertLastError("smtp failure")
	if err := subscriptionService.Store.MarkNotificationSuccess(pending.ID, 1); err != nil {
		t.Fatal(err)
	}
	assertLastError("")

	iyuuFailure, err := subscriptionService.Store.UpsertPendingNotification(
		subscriptionID, "2026-10-18", 0, model.ChannelIYUU, model.NotificationKindScheduled,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Store.MarkNotificationFailure(iyuuFailure.ID, 5, "iyuu failure", nil, true); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.Store.RecordManualCustomerEmailSuccess(subscriptionID); err != nil {
		t.Fatal(err)
	}
	assertLastError("iyuu failure")
}
