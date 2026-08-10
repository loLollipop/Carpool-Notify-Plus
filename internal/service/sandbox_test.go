package service_test

import (
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"
)

func TestSandboxFixturesSupportCompleteRehearsalWithoutLeakingState(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 8, 12, 0, 0, 0, cycle.Location)
	}

	status, err := subscriptionService.ResetSandboxFixtures()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Ready || status.AccessToken == "" {
		t.Fatalf("sandbox status = %#v, want ready with token", status)
	}
	if len(status.Accounts) != 4 || status.SubscriptionCount != 3 || len(status.RedemptionCodes) != 3 {
		t.Fatalf("sandbox fixture counts = accounts %d subscriptions %d codes %d", len(status.Accounts), status.SubscriptionCount, len(status.RedemptionCodes))
	}
	valid, err := subscriptionService.ValidateSandboxAccessToken(status.AccessToken)
	if err != nil || !valid {
		t.Fatalf("validate sandbox token = %v, %v", valid, err)
	}

	accountIDs := map[string]int64{}
	for _, account := range status.Accounts {
		accountIDs[account.Name] = account.ID
	}

	redemptionResult, err := subscriptionService.SubmitRedemptionApplication(service.RedemptionSubmitInput{
		CustomerEmail:   "sandbox-customer@example.com",
		CustomerContact: "sandbox_wechat",
		RedeemCode:      status.RedemptionCodes[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	applications, err := subscriptionService.ListRedemptionApplicationsView(model.RedemptionStatusPending)
	if err != nil || len(applications) != 1 {
		t.Fatalf("pending applications = %d, %v", len(applications), err)
	}
	redeemSeats, err := subscriptionService.Store.ListFreeSeats(accountIDs["沙盒兑换主号"], 0)
	if err != nil || len(redeemSeats) == 0 {
		t.Fatalf("redemption seats = %d, %v", len(redeemSeats), err)
	}
	_, err = subscriptionService.InviteRedemptionApplication(
		applications[0].Application.ID,
		service.RedemptionInviteInput{
			SeatID:           redeemSeats[0].ID,
			PriceYuan:        "30.00",
			CronExpr:         "interval:30d",
			NotifyOffsetsRaw: "3",
			BoardedAt:        "2026-08-08",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	redemptionStatus, err := subscriptionService.GetRedemptionStatus(redemptionResult.TrackingToken)
	if err != nil || redemptionStatus.Status != model.RedemptionStatusInvited {
		t.Fatalf("redemption status = %#v, %v", redemptionStatus, err)
	}

	views, err := subscriptionService.ListView()
	if err != nil {
		t.Fatal(err)
	}
	var cancellationSubscriptionID int64
	for _, view := range views {
		if view.Subscription.CustomerEmail == "cancel-demo@example.com" {
			cancellationSubscriptionID = view.Subscription.ID
		}
	}
	if cancellationSubscriptionID == 0 {
		t.Fatal("cancellation fixture subscription not found")
	}
	cancellation, err := subscriptionService.RequestCancellation(cancellationSubscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetAfterSalesCaseRefunded(cancellation.CaseID, true); err != nil {
		t.Fatal(err)
	}

	created, err := subscriptionService.BanAccount(accountIDs["沙盒封禁主号"], service.BanAccountInput{
		BannedDate: "2026-08-08",
		Note:       "演练账号封禁",
	})
	if err != nil || created != 2 {
		t.Fatalf("ban account created %d cases, %v", created, err)
	}
	afterSales, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	pendingBanCases := make([]int64, 0, 2)
	for _, item := range afterSales.Cases {
		if item.Case.AccountID == accountIDs["沙盒封禁主号"] && item.Case.Status == model.AfterSalesStatusPending {
			pendingBanCases = append(pendingBanCases, item.Case.ID)
		}
	}
	if len(pendingBanCases) != 2 {
		t.Fatalf("pending ban cases = %d, want 2", len(pendingBanCases))
	}
	replacementSeats, err := subscriptionService.Store.ListFreeSeats(accountIDs["沙盒备用主号"], 0)
	if err != nil || len(replacementSeats) < 1 {
		t.Fatalf("replacement seats = %d, %v", len(replacementSeats), err)
	}
	if err := subscriptionService.ReassignAfterSalesCase(pendingBanCases[0], service.ReassignAfterSalesCaseInput{
		AccountID: accountIDs["沙盒备用主号"],
		SeatID:    replacementSeats[0].ID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetAfterSalesCaseRefunded(pendingBanCases[1], true); err != nil {
		t.Fatal(err)
	}
	activeOnBannedAccount, err := subscriptionService.Store.CountActiveSubscriptionsByAccount(accountIDs["沙盒封禁主号"])
	if err != nil {
		t.Fatal(err)
	}
	if activeOnBannedAccount != 0 {
		t.Fatalf("handled sandbox ban still occupies %d seats", activeOnBannedAccount)
	}
	accountViews, err := subscriptionService.ListAccountsView()
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range accountViews {
		if view.Account.ID == accountIDs["沙盒封禁主号"] {
			t.Fatal("handled sandbox banned account remains visible")
		}
	}

	oldToken := status.AccessToken
	resetStatus, err := subscriptionService.ResetSandboxFixtures()
	if err != nil {
		t.Fatal(err)
	}
	if resetStatus.AccessToken == oldToken {
		t.Fatal("reset must rotate the public sandbox token")
	}
	valid, err = subscriptionService.ValidateSandboxAccessToken(oldToken)
	if err != nil || valid {
		t.Fatalf("old token remains valid = %v, %v", valid, err)
	}
	if resetStatus.SubscriptionCount != 3 || len(resetStatus.RedemptionCodes) != 3 {
		t.Fatalf("reset fixture = %#v", resetStatus)
	}
}
