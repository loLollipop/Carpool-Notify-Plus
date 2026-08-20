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

func TestRedemptionInviteAutoAssignsFirstFreeSeatByAccountImportOrder(t *testing.T) {
	subscriptionService := openTestService(t)
	_, firstAccountSeats := createTestAccountWithSeats(
		t, subscriptionService, "first imported account", "seat 1", "seat 2",
	)
	_, laterAccountSeats := createTestAccountWithSeats(
		t, subscriptionService, "later imported account", "seat 1",
	)
	if _, err := subscriptionService.Create(service.CreateInput{
		Name:             "existing customer",
		PriceYuan:        "20.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		SeatID:           firstAccountSeats[0],
	}); err != nil {
		t.Fatal(err)
	}

	codes, err := subscriptionService.GenerateRedemptionCodes(service.RedemptionCodeGenerateInput{Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := subscriptionService.SubmitRedemptionApplication(service.RedemptionSubmitInput{
		CustomerEmail:   "auto-assigned@example.com",
		CustomerContact: "wx-auto-assigned",
		RedeemCode:      codes[0].Code.Code,
	})
	if err != nil {
		t.Fatal(err)
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
			PriceYuan:        "25.00",
			CronExpr:         "interval:30d",
			NotifyOffsetsRaw: "3",
			BoardedAt:        "2026-08-20",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.SeatID != firstAccountSeats[1] {
		t.Fatalf(
			"auto-assigned seat = %d, want first free seat %d (later account seat was %d)",
			subscription.SeatID,
			firstAccountSeats[1],
			laterAccountSeats[0],
		)
	}
	status, err := subscriptionService.GetRedemptionStatus(result.TrackingToken)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != model.RedemptionStatusInvited {
		t.Fatalf("redemption status = %q, want invited", status.Status)
	}
}

func TestRedemptionInviteAutoAssignKeepsApplicationPendingWithoutFreeSeat(t *testing.T) {
	subscriptionService := openTestService(t)
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "full account", "seat 1")
	if _, err := subscriptionService.Create(service.CreateInput{
		Name:             "existing customer",
		PriceYuan:        "20.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3",
		SeatID:           seatIDs[0],
	}); err != nil {
		t.Fatal(err)
	}
	codes, err := subscriptionService.GenerateRedemptionCodes(service.RedemptionCodeGenerateInput{Count: 1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := subscriptionService.SubmitRedemptionApplication(service.RedemptionSubmitInput{
		CustomerEmail:   "waiting@example.com",
		CustomerContact: "wx-waiting",
		RedeemCode:      codes[0].Code.Code,
	})
	if err != nil {
		t.Fatal(err)
	}
	applications, err := subscriptionService.ListRedemptionApplicationsView(model.RedemptionStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	_, err = subscriptionService.InviteRedemptionApplication(
		applications[0].Application.ID,
		service.RedemptionInviteInput{
			PriceYuan:        "25.00",
			CronExpr:         "interval:30d",
			NotifyOffsetsRaw: "3",
		},
	)
	if err == nil || !strings.Contains(err.Error(), "暂无可用席位") {
		t.Fatalf("auto-assign error = %v, want no free seat", err)
	}
	status, err := subscriptionService.GetRedemptionStatus(result.TrackingToken)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != model.RedemptionStatusPending {
		t.Fatalf("redemption status = %q, want pending", status.Status)
	}
}

func TestArchiveRemovesLinkedRedemptionRecordAndCode(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 5, 10, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "redemption account", "seat1")
	codes, err := subscriptionService.GenerateRedemptionCodes(service.RedemptionCodeGenerateInput{
		Count: 1,
		Note:  "cleanup",
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := subscriptionService.SubmitRedemptionApplication(service.RedemptionSubmitInput{
		CustomerEmail:   "customer@example.com",
		CustomerContact: "wechat701",
		RedeemCode:      codes[0].Code.Code,
	})
	if err != nil {
		t.Fatal(err)
	}
	applications, err := subscriptionService.ListRedemptionApplicationsView(model.RedemptionStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, err := subscriptionService.InviteRedemptionApplication(
		applications[0].Application.ID,
		service.RedemptionInviteInput{
			SeatID:           seatIDs[0],
			PriceYuan:        "25.00",
			CronExpr:         "interval:30d",
			NotifyOffsetsRaw: "3",
			BoardedAt:        "2026-08-05",
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := subscriptionService.Archive(subscriptionID); err != nil {
		t.Fatal(err)
	}

	archived, err := subscriptionService.Store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.ArchivedAt == nil {
		t.Fatal("subscription should be archived")
	}
	paid, err := subscriptionService.Store.IsDuePaid(subscriptionID, "2026-08-05")
	if err != nil {
		t.Fatal(err)
	}
	if !paid {
		t.Fatal("archive should keep paid bills")
	}
	if _, err := subscriptionService.GetRedemptionStatus(result.TrackingToken); err == nil {
		t.Fatal("redemption status should be removed after archive")
	}
	remainingApplications, err := subscriptionService.ListRedemptionApplicationsView("")
	if err != nil {
		t.Fatal(err)
	}
	if len(remainingApplications) != 0 {
		t.Fatalf("remaining applications = %d, want 0", len(remainingApplications))
	}
	remainingCodes, err := subscriptionService.ListRedemptionCodesView()
	if err != nil {
		t.Fatal(err)
	}
	if len(remainingCodes) != 0 {
		t.Fatalf("remaining codes = %d, want 0", len(remainingCodes))
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

func TestRedemptionRejectReleasesCodeForCorrectedSubmission(t *testing.T) {
	subscriptionService := openTestService(t)
	codes, err := subscriptionService.GenerateRedemptionCodes(service.RedemptionCodeGenerateInput{
		Count: 1,
		Note:  "信息纠正",
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := subscriptionService.SubmitRedemptionApplication(service.RedemptionSubmitInput{
		CustomerEmail:   "wrong@example.com",
		CustomerContact: "微信：wrong",
		RedeemCode:      codes[0].Code.Code,
	})
	if err != nil {
		t.Fatal(err)
	}
	applications, err := subscriptionService.ListRedemptionApplicationsView(model.RedemptionStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if len(applications) != 1 {
		t.Fatalf("pending applications = %d, want 1", len(applications))
	}

	err = subscriptionService.RejectRedemptionApplication(
		applications[0].Application.ID,
		service.RedemptionRejectInput{Reason: "邮箱填写错误"},
	)
	if err != nil {
		t.Fatal(err)
	}
	status, err := subscriptionService.GetRedemptionStatus(first.TrackingToken)
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != model.RedemptionStatusRejected || status.RejectionReason != "邮箱填写错误" {
		t.Fatalf("rejected status = %#v, want rejected with reason", status)
	}

	code, err := subscriptionService.Store.GetRedemptionCode(codes[0].Code.ID)
	if err != nil {
		t.Fatal(err)
	}
	if code.Status != model.RedemptionCodeStatusUnused || code.UsedByApplicationID != 0 || code.UsedAt != nil {
		t.Fatalf("released code = %#v, want unused without application", code)
	}

	corrected, err := subscriptionService.SubmitRedemptionApplication(service.RedemptionSubmitInput{
		CustomerEmail:   "correct@example.com",
		CustomerContact: "微信：correct",
		RedeemCode:      codes[0].Code.Code,
	})
	if err != nil {
		t.Fatalf("submit corrected application: %v", err)
	}
	if corrected.Status != model.RedemptionStatusPending || corrected.TrackingToken == first.TrackingToken {
		t.Fatalf("corrected submit result = %#v, want new pending application", corrected)
	}

	err = subscriptionService.RejectRedemptionApplication(
		applications[0].Application.ID,
		service.RedemptionRejectInput{},
	)
	if err == nil || !strings.Contains(err.Error(), "已经处理过") {
		t.Fatalf("second reject error = %v, want already processed", err)
	}
}

func TestRedemptionCodeDeleteOnlyAllowsUnusedOrDisabled(t *testing.T) {
	subscriptionService := openTestService(t)
	codes, err := subscriptionService.GenerateRedemptionCodes(service.RedemptionCodeGenerateInput{
		Count: 2,
		Note:  "清理测试",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := subscriptionService.DeleteRedemptionCode(codes[0].Code.ID); err != nil {
		t.Fatalf("delete unused code: %v", err)
	}
	listed, err := subscriptionService.ListRedemptionCodesView()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Code.ID == codes[0].Code.ID {
		t.Fatalf("codes after delete = %#v, want first code removed", listed)
	}

	_, err = subscriptionService.SubmitRedemptionApplication(service.RedemptionSubmitInput{
		CustomerEmail:   "used@example.com",
		CustomerContact: "微信：used",
		RedeemCode:      codes[1].Code.Code,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = subscriptionService.DeleteRedemptionCode(codes[1].Code.ID)
	if err == nil || !strings.Contains(err.Error(), "不能删除") {
		t.Fatalf("delete used code error = %v, want protected used code", err)
	}
}
