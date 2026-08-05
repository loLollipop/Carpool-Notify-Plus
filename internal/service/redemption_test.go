package service_test

import (
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"
)

func TestRedemptionInviteCreatesSubscriptionAndInitialBill(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 5, 10, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "兑换母号", "车位1")

	result, err := subscriptionService.SubmitRedemptionApplication(service.RedemptionSubmitInput{
		CustomerEmail:   "Customer <customer@example.com>",
		CustomerContact: "wx-customer-701",
		RedeemCode:      "CODE701",
		RequestNote:     "今天购买",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TrackingToken == "" || result.Status != model.RedemptionStatusPending {
		t.Fatalf("submit result = %#v, want token + pending", result)
	}

	applications, err := subscriptionService.ListRedemptionApplicationsView(model.RedemptionStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if len(applications) != 1 {
		t.Fatalf("pending applications = %d, want 1", len(applications))
	}

	subscriptionID, err := subscriptionService.InviteRedemptionApplication(
		applications[0].Application.ID,
		service.RedemptionInviteInput{
			SeatID:           seatIDs[0],
			PriceYuan:        "25.50",
			CronExpr:         "interval:30d",
			NotifyOffsetsRaw: "3",
			BoardedAt:        "2026-08-05",
			OperatorNote:     "OpenAI invite sent",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	status, err := subscriptionService.GetRedemptionStatus(result.TrackingToken)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != model.RedemptionStatusInvited || status.InvitedAtLabel == "" {
		t.Fatalf("status = %#v, want invited with invited time", status)
	}

	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.CustomerEmail != "customer@example.com" {
		t.Fatalf("customer email = %q, want normalized customer@example.com", subscription.CustomerEmail)
	}
	if subscription.CustomerWechat != "wx-customer-701" {
		t.Fatalf("customer contact = %q, want wx-customer-701", subscription.CustomerWechat)
	}
	if subscription.SeatID != seatIDs[0] || subscription.PricePerPersonCents != 2550 || subscription.BoardedAt != "2026-08-05" {
		t.Fatalf("subscription assignment/price/boarded = seat %d price %d boarded %q", subscription.SeatID, subscription.PricePerPersonCents, subscription.BoardedAt)
	}
	if !strings.Contains(subscription.Remark, "兑换码：CODE701") || !strings.Contains(subscription.Remark, "申请备注：今天购买") {
		t.Fatalf("remark = %q, want redemption details", subscription.Remark)
	}

	paid, err := subscriptionService.Store.IsDuePaid(subscriptionID, "2026-08-05")
	if err != nil {
		t.Fatal(err)
	}
	if !paid {
		t.Fatal("initial redemption period should be paid")
	}
}
