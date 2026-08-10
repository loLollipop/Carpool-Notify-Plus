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
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "Plus account", "seat1")

	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "Plus customer",
		BusinessType:     model.SubscriptionBusinessPlus,
		PriceYuan:        "68.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "14,7,3",
		CustomerEmail:    "not-an-email",
		CustomerWechat:   "wx-plus-customer",
		IsResale:         true,
		AgencyFeeYuan:    "not-an-amount",
		SeatID:           seatIDs[0],
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
	if subscription.CustomerEmail != "" {
		t.Fatalf("customer email = %q, want empty", subscription.CustomerEmail)
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
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "Plus account", "seat1")

	_, err := subscriptionService.Create(service.CreateInput{
		BusinessType: model.SubscriptionBusinessPlus,
		PriceYuan:    "68.00",
		CronExpr:     "interval:30d",
		SeatID:       seatIDs[0],
		BoardedAt:    "2026-07-01",
	})
	if err == nil || !strings.Contains(err.Error(), "客户微信") {
		t.Fatalf("create error = %v, want customer WeChat validation", err)
	}
}

func TestPlusRentalOnlySendsDueDayOperatorReminder(t *testing.T) {
	subscriptionService := openTestService(t)
	clock := time.Date(2026, time.July, 12, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return clock }
	iyuuRecorder := &recordingSender{}
	smtpRecorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: iyuuRecorder, SMTP: smtpRecorder}

	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "Plus reminder account", "seat1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		BusinessType:     model.SubscriptionBusinessPlus,
		PriceYuan:        "68.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "customer@example.com",
		CustomerWechat:   "wx-customer",
		SeatID:           seatIDs[0],
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

	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "Plus email guard", "seat1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		BusinessType:   model.SubscriptionBusinessPlus,
		PriceYuan:      "68.00",
		CronExpr:       "0 0 15 * *",
		CustomerWechat: "wx-customer",
		SeatID:         seatIDs[0],
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
