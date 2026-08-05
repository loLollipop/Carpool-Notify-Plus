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
	codes, err := subscriptionService.GenerateRedemptionCodes(service.RedemptionCodeGenerateInput{
		Count: 1,
		Note:  "测试订单",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := subscriptionService.SubmitRedemptionApplication(service.RedemptionSubmitInput{
		CustomerEmail:   "Customer <customer@example.com>",
		CustomerContact: "wx-customer-701",
		RedeemCode:      strings.ToLower(codes[0].Code.Code),
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
	if !strings.Contains(subscription.Remark, "兑换码："+codes[0].Code.Code) || !strings.Contains(subscription.Remark, "申请备注：今天购买") {
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

func TestRedemptionSubmitRequiresUnusedGeneratedCode(t *testing.T) {
	subscriptionService := openTestService(t)
	codes, err := subscriptionService.GenerateRedemptionCodes(service.RedemptionCodeGenerateInput{
		Count: 1,
		Note:  "一次性订单",
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = subscriptionService.SubmitRedemptionApplication(service.RedemptionSubmitInput{
		CustomerEmail:   "first@example.com",
		CustomerContact: "微信：first",
		RedeemCode:      codes[0].Code.Code,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = subscriptionService.SubmitRedemptionApplication(service.RedemptionSubmitInput{
		CustomerEmail:   "second@example.com",
		CustomerContact: "微信：second",
		RedeemCode:      codes[0].Code.Code,
	})
	if err == nil || !strings.Contains(err.Error(), "已经被使用") {
		t.Fatalf("second submit error = %v, want used code message", err)
	}

	_, err = subscriptionService.SubmitRedemptionApplication(service.RedemptionSubmitInput{
		CustomerEmail:   "missing@example.com",
		CustomerContact: "微信：missing",
		RedeemCode:      "CPN-NOT-A-CODE",
	})
	if err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("missing code error = %v, want not found message", err)
	}
}
