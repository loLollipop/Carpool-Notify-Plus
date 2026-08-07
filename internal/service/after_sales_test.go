package service_test

import (
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"
)

func TestBanAccountCalculatesThirtyDayWarrantyRefund(t *testing.T) {
	tests := []struct {
		name           string
		bannedDate     string
		wantUsed       int
		wantRemaining  int
		wantRefundCent int64
	}{
		{name: "same day full refund", bannedDate: "2026-07-01", wantUsed: 0, wantRemaining: 30, wantRefundCent: 3000},
		{name: "ten days used", bannedDate: "2026-07-11", wantUsed: 10, wantRemaining: 20, wantRefundCent: 2000},
		{name: "thirty days used", bannedDate: "2026-07-31", wantUsed: 30, wantRemaining: 0, wantRefundCent: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			subscriptionService := openTestService(t)
			subscriptionService.Clock = func() time.Time {
				return time.Date(2026, time.August, 1, 12, 0, 0, 0, cycle.Location)
			}
			accountID, subscriptionID := createWarrantyCustomer(t, subscriptionService, "2026-07-01", true)

			created, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{
				BannedDate: test.bannedDate,
				Note:       "平台停用",
			})
			if err != nil {
				t.Fatal(err)
			}
			if created != 1 {
				t.Fatalf("created cases = %d, want 1", created)
			}

			page, err := subscriptionService.ListAfterSalesPage()
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Cases) != 1 {
				t.Fatalf("case count = %d, want 1", len(page.Cases))
			}
			caseItem := page.Cases[0].Case
			if caseItem.SubscriptionID != subscriptionID {
				t.Fatalf("subscription id = %d, want %d", caseItem.SubscriptionID, subscriptionID)
			}
			if caseItem.UsedDays != test.wantUsed || caseItem.RemainingDays != test.wantRemaining {
				t.Fatalf("days = used %d remaining %d, want %d/%d", caseItem.UsedDays, caseItem.RemainingDays, test.wantUsed, test.wantRemaining)
			}
			if caseItem.PaidAmountCents != 3000 || caseItem.RefundAmountCents != test.wantRefundCent {
				t.Fatalf("amount = paid %d refund %d, want 3000/%d", caseItem.PaidAmountCents, caseItem.RefundAmountCents, test.wantRefundCent)
			}
			if caseItem.Status != model.AfterSalesStatusPending {
				t.Fatalf("status = %q, want pending", caseItem.Status)
			}

			account, err := subscriptionService.Store.GetAccount(accountID)
			if err != nil {
				t.Fatal(err)
			}
			if account.BannedAt != test.bannedDate || account.BanNote != "平台停用" {
				t.Fatalf("ban snapshot = %q/%q", account.BannedAt, account.BanNote)
			}
			if _, err := subscriptionService.Store.GetSubscription(subscriptionID); err != nil {
				t.Fatalf("active subscription changed after ban: %v", err)
			}
			if _, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-01"); err != nil {
				t.Fatalf("bill changed after ban: %v", err)
			}
		})
	}
}

func TestBanAccountUsesLatestEligiblePaidBillAndIsIdempotent(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 20, 12, 0, 0, 0, cycle.Location)
	}
	accountID, subscriptionID := createWarrantyCustomer(t, subscriptionService, "2026-07-01", true)
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-31", true); err != nil {
		t.Fatal(err)
	}
	latestBill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-07-31")
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.UpdateBill(latestBill.ID, service.BillEditInput{
		AmountYuan: "45.00",
		Note:       "第二期实付",
	}); err != nil {
		t.Fatal(err)
	}

	created, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{BannedDate: "2026-08-10"})
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatalf("first created = %d, want 1", created)
	}
	created, err = subscriptionService.BanAccount(accountID, service.BanAccountInput{BannedDate: "2026-08-11"})
	if err != nil {
		t.Fatal(err)
	}
	if created != 0 {
		t.Fatalf("repeated created = %d, want 0", created)
	}

	page, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(page.Cases))
	}
	caseItem := page.Cases[0].Case
	if caseItem.BillID != latestBill.ID || caseItem.PeriodStart != "2026-07-31" {
		t.Fatalf("latest bill snapshot = id %d period %q", caseItem.BillID, caseItem.PeriodStart)
	}
	if caseItem.BannedDate != "2026-08-10" {
		t.Fatalf("banned date changed on retry: %q", caseItem.BannedDate)
	}
	if caseItem.PaidAmountCents != 4500 || caseItem.UsedDays != 10 || caseItem.RefundAmountCents != 3000 {
		t.Fatalf("refund snapshot = paid %d used %d refund %d", caseItem.PaidAmountCents, caseItem.UsedDays, caseItem.RefundAmountCents)
	}
}

func TestBanAccountCreatesReviewCaseWithoutPaidBill(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 20, 12, 0, 0, 0, cycle.Location)
	}
	accountID, subscriptionID := createWarrantyCustomer(t, subscriptionService, "2026-07-01", false)

	if _, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{BannedDate: "2026-07-10"}); err != nil {
		t.Fatal(err)
	}
	page, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	caseItem := page.Cases[0].Case
	if caseItem.SubscriptionID != subscriptionID || caseItem.BillID != 0 {
		t.Fatalf("review case ids = subscription %d bill %d", caseItem.SubscriptionID, caseItem.BillID)
	}
	if caseItem.Status != model.AfterSalesStatusReview || caseItem.RefundAmountCents != 0 || caseItem.Note == "" {
		t.Fatalf("review case = status %q refund %d note %q", caseItem.Status, caseItem.RefundAmountCents, caseItem.Note)
	}
	if page.Summary.ReviewCount != 1 {
		t.Fatalf("review summary = %d, want 1", page.Summary.ReviewCount)
	}
}

func TestAfterSalesCaseCanBeAdjustedCompletedAndReopened(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 20, 12, 0, 0, 0, cycle.Location)
	}
	accountID, _ := createWarrantyCustomer(t, subscriptionService, "2026-07-01", true)
	if _, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{BannedDate: "2026-07-11"}); err != nil {
		t.Fatal(err)
	}
	page, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	caseID := page.Cases[0].Case.ID
	if err := subscriptionService.UpdateAfterSalesCase(caseID, service.UpdateAfterSalesCaseInput{
		RefundAmountYuan: "18.88",
		Note:             "微信已核对",
	}); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetAfterSalesCaseRefunded(caseID, true); err != nil {
		t.Fatal(err)
	}
	page, err = subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	completed := page.Cases[0].Case
	if completed.Status != model.AfterSalesStatusRefunded || completed.ProcessedAt == nil || completed.RefundAmountCents != 1888 || completed.Note != "微信已核对" {
		t.Fatalf("completed case = %#v", completed)
	}
	if err := subscriptionService.SetAfterSalesCaseRefunded(caseID, false); err != nil {
		t.Fatal(err)
	}
	page, err = subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	reopened := page.Cases[0].Case
	if reopened.Status != model.AfterSalesStatusPending || reopened.ProcessedAt != nil {
		t.Fatalf("reopened case = status %q processed %v", reopened.Status, reopened.ProcessedAt)
	}
}

func TestBanAccountSnapshotsEveryActiveCustomerAndFreezesAssignments(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 20, 12, 0, 0, 0, cycle.Location)
	}
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "双人母号",
		Email:     "owner@example.com",
		SpaceName: "Warranty Space",
		SeatCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	for index, email := range []string{"one@example.com", "two@example.com"} {
		if _, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
			PriceYuan:        "30.00",
			CronExpr:         "interval:30d",
			NotifyOffsetsRaw: "0",
			CustomerEmail:    email,
			CustomerWechat:   "wx-" + email,
			AccountID:        accountID,
			BoardedAt:        "2026-07-01",
			Name:             email,
		}); err != nil {
			t.Fatalf("create customer %d: %v", index, err)
		}
	}
	created, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{BannedDate: "2026-07-11"})
	if err != nil {
		t.Fatal(err)
	}
	if created != 2 {
		t.Fatalf("created cases = %d, want 2", created)
	}
	page, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Cases) != 2 || page.Summary.PendingCount != 2 || page.Summary.PendingRefundCents != 4000 {
		t.Fatalf("page summary = cases %d pending %d refund %d", len(page.Cases), page.Summary.PendingCount, page.Summary.PendingRefundCents)
	}
	options, err := subscriptionService.ListAccountOptionsForForm(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range options {
		if option.ID == accountID {
			t.Fatal("banned account must not be available for new assignments")
		}
	}
}

func createWarrantyCustomer(
	t *testing.T,
	subscriptionService *service.SubscriptionService,
	boardedAt string,
	paid bool,
) (int64, int64) {
	t.Helper()
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "质保母号",
		Email:     "owner@example.com",
		SpaceName: "Warranty Space",
		SeatCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	input := service.CreateInput{
		Name:             "质保客户",
		PriceYuan:        "30.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "0",
		CustomerEmail:    "customer@example.com",
		CustomerWechat:   "wx-customer",
		AccountID:        accountID,
		BoardedAt:        boardedAt,
	}
	var subscriptionID int64
	if paid {
		subscriptionID, err = subscriptionService.CreateWithInitialBill(input)
	} else {
		subscriptionID, err = subscriptionService.Create(input)
	}
	if err != nil {
		t.Fatal(err)
	}
	return accountID, subscriptionID
}
