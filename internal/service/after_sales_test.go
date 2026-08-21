package service_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
	"carpool-notify/internal/notify"
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

func TestCancellationFreezesSeatForDefaultWindowAfterRefund(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }
	accountID, subscriptionID := createWarrantyCustomer(t, subscriptionService, "2026-08-01", true)

	request, err := subscriptionService.RequestCancellation(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if request.CaseID <= 0 || request.ExpiresAtLabel != "2026-08-11 12:00" {
		t.Fatalf("request = %#v", request)
	}
	active, err := subscriptionService.Store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if active.CancellationCaseID != request.CaseID || active.CancellationExpiresAt == nil {
		t.Fatalf("active cancellation state = %#v", active)
	}
	freeSeats, err := subscriptionService.Store.ListFreeSeats(accountID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(freeSeats) != 0 {
		t.Fatalf("pending cancellation released %d seats", len(freeSeats))
	}

	page, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Cases) != 1 {
		t.Fatalf("after-sales cases = %d, want 1", len(page.Cases))
	}
	caseItem := page.Cases[0].Case
	if caseItem.Source != model.AfterSalesSourceCustomerCancellation || caseItem.RefundAmountCents != 2100 {
		t.Fatalf("cancellation case = %#v", caseItem)
	}

	if err := subscriptionService.SetAfterSalesCaseRefunded(request.CaseID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.Store.GetSubscription(subscriptionID); err != sql.ErrNoRows {
		t.Fatalf("active subscription after refund error = %v, want sql.ErrNoRows", err)
	}
	archived, err := subscriptionService.Store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.ArchivedAt == nil || archived.CancellationCaseID != 0 || archived.SeatFrozenUntil == nil {
		t.Fatalf("archived subscription = %#v", archived)
	}
	wantFrozenUntil := now.AddDate(0, 0, model.DefaultSeatFreezeDays)
	if !archived.SeatFrozenUntil.Equal(wantFrozenUntil) {
		t.Fatalf("seat frozen until = %v, want %v", archived.SeatFrozenUntil, wantFrozenUntil)
	}
	freeSeats, err = subscriptionService.Store.ListFreeSeatsAt(accountID, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(freeSeats) != 0 {
		t.Fatalf("free seats during freeze = %d, want 0", len(freeSeats))
	}
	accountView, err := subscriptionService.GetAccountView(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if accountView.SeatUsed != 1 || len(accountView.Seats) != 1 || !accountView.Seats[0].Frozen {
		t.Fatalf("frozen account view = %#v", accountView)
	}
	_, fallbackSeatIDs := createTestAccountWithSeats(
		t,
		subscriptionService,
		"冻结期备用母号",
		"车位1",
	)
	firstFree, err := subscriptionService.Store.GetFirstFreeSeatByAccountImportOrderAt(now)
	if err != nil {
		t.Fatal(err)
	}
	if firstFree.ID != fallbackSeatIDs[0] {
		t.Fatalf("auto assignment chose seat %d, want fallback %d", firstFree.ID, fallbackSeatIDs[0])
	}

	now = wantFrozenUntil.Add(time.Second)
	freeSeats, err = subscriptionService.Store.ListFreeSeatsAt(accountID, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(freeSeats) != 1 {
		t.Fatalf("free seats after freeze = %d, want 1", len(freeSeats))
	}
	firstFree, err = subscriptionService.Store.GetFirstFreeSeatByAccountImportOrderAt(now)
	if err != nil {
		t.Fatal(err)
	}
	if firstFree.ID != freeSeats[0].ID {
		t.Fatalf("released seat ordering = %d, want original %d", firstFree.ID, freeSeats[0].ID)
	}
	completed, err := subscriptionService.Store.GetAfterSalesCase(request.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != model.AfterSalesStatusRefunded || completed.ProcessedAt == nil {
		t.Fatalf("completed case = %#v", completed)
	}
}

func TestCancellationUsesConfiguredSeatFreezeWindow(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }
	if err := subscriptionService.Store.SetSetting(model.SettingSeatFreezeDays, "3"); err != nil {
		t.Fatal(err)
	}
	accountID, subscriptionID := createWarrantyCustomer(
		t,
		subscriptionService,
		"2026-08-01",
		true,
	)
	request, err := subscriptionService.RequestCancellation(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetAfterSalesCaseRefunded(request.CaseID, true); err != nil {
		t.Fatal(err)
	}
	archived, err := subscriptionService.Store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	wantFrozenUntil := now.AddDate(0, 0, 3)
	if archived.SeatFrozenUntil == nil || !archived.SeatFrozenUntil.Equal(wantFrozenUntil) {
		t.Fatalf("configured freeze = %v, want %v", archived.SeatFrozenUntil, wantFrozenUntil)
	}
	if free, err := subscriptionService.Store.ListFreeSeatsAt(
		accountID,
		0,
		wantFrozenUntil.Add(-time.Second),
	); err != nil || len(free) != 0 {
		t.Fatalf("seat before configured expiry = %#v, %v", free, err)
	}
	if free, err := subscriptionService.Store.ListFreeSeatsAt(
		accountID,
		0,
		wantFrozenUntil.Add(time.Second),
	); err != nil || len(free) != 1 {
		t.Fatalf("seat after configured expiry = %#v, %v", free, err)
	}
}

func TestCancellationFreezeBlocksArchivedRecordDeletion(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }
	_, subscriptionID := createWarrantyCustomer(t, subscriptionService, "2026-08-01", false)

	request, err := subscriptionService.RequestCancellation(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetAfterSalesCaseRefunded(request.CaseID, true); err != nil {
		t.Fatal(err)
	}

	archived, err := subscriptionService.ListArchivedView()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, view := range archived {
		if view.Subscription.ID != subscriptionID {
			continue
		}
		found = true
		if view.BillCount != 0 || view.CanSoftDelete {
			t.Fatalf("frozen no-bill archive view = %#v", view)
		}
	}
	if !found {
		t.Fatal("expected frozen subscription in archived list")
	}
	if err := subscriptionService.SoftDeleteArchived(subscriptionID); err == nil ||
		!strings.Contains(err.Error(), "原车位冻结至 2026-08-17 12:00") {
		t.Fatalf("soft delete during freeze error = %v", err)
	}
	if _, err := subscriptionService.Store.GetSubscriptionIncludingArchived(subscriptionID); err != nil {
		t.Fatalf("frozen subscription was deleted: %v", err)
	}

	now = now.AddDate(0, 0, model.DefaultSeatFreezeDays).Add(time.Second)
	archived, err = subscriptionService.ListArchivedView()
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range archived {
		if view.Subscription.ID == subscriptionID && view.CanSoftDelete {
			t.Fatal("completed after-sales history must still keep archived record non-deletable")
		}
	}
}

func TestExpiredCancellationRestoresSubscriptionAndRemovesTemporaryCase(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }
	accountID, subscriptionID := createWarrantyCustomer(t, subscriptionService, "2026-08-01", true)

	if _, err := subscriptionService.RequestCancellation(subscriptionID); err != nil {
		t.Fatal(err)
	}
	now = now.Add(25 * time.Hour)
	restored, err := subscriptionService.RestoreExpiredCancellationRequests()
	if err != nil {
		t.Fatal(err)
	}
	if restored != 1 {
		t.Fatalf("restored = %d, want 1", restored)
	}
	active, err := subscriptionService.Store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if active.CancellationCaseID != 0 || active.CancellationRequestedAt != nil || active.CancellationExpiresAt != nil {
		t.Fatalf("restored cancellation state = %#v", active)
	}
	page, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Cases) != 0 {
		t.Fatalf("expired cases = %d, want 0", len(page.Cases))
	}
	freeSeats, err := subscriptionService.Store.ListFreeSeats(accountID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(freeSeats) != 0 {
		t.Fatalf("restored subscription released %d seats", len(freeSeats))
	}
}

func TestAccountBanSupersedesPendingCancellationWithoutLeavingOrphanedCase(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }
	accountID, subscriptionID := createWarrantyCustomer(t, subscriptionService, "2026-08-01", true)

	cancellation, err := subscriptionService.RequestCancellation(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{
		BannedDate: "2026-08-10",
		Note:       "平台封禁",
	}); err != nil {
		t.Fatal(err)
	}

	cases, err := subscriptionService.Store.ListAfterSalesCases()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 {
		t.Fatalf("after-sales cases = %d, want only the account-ban case", len(cases))
	}
	if cases[0].ID == cancellation.CaseID || cases[0].Source != model.AfterSalesSourceAccountBan {
		t.Fatalf("remaining case = %#v, want account ban replacing cancellation %d", cases[0], cancellation.CaseID)
	}
	active, err := subscriptionService.Store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if active.CancellationCaseID != 0 || active.CancellationRequestedAt != nil || active.CancellationExpiresAt != nil {
		t.Fatalf("stale cancellation state after account ban = %#v", active)
	}

	now = now.Add(25 * time.Hour)
	restored, err := subscriptionService.RestoreExpiredCancellationRequests()
	if err != nil {
		t.Fatal(err)
	}
	if restored != 0 {
		t.Fatalf("expired cancellation restore count = %d, want 0", restored)
	}
	cases, err = subscriptionService.Store.ListAfterSalesCases()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || cases[0].Source != model.AfterSalesSourceAccountBan {
		t.Fatalf("account-ban case disappeared after cancellation grace period: %#v", cases)
	}
}

func TestCancellationIsRejectedWhileAccountBanAfterSalesIsPending(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 10, 12, 0, 0, 0, cycle.Location)
	}
	accountID, subscriptionID := createWarrantyCustomer(t, subscriptionService, "2026-08-01", true)
	if _, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{BannedDate: "2026-08-10"}); err != nil {
		t.Fatal(err)
	}

	if _, err := subscriptionService.RequestCancellation(subscriptionID); err == nil {
		t.Fatal("cancellation succeeded while account-ban after-sales was pending")
	}
	cases, err := subscriptionService.Store.ListAfterSalesCases()
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 || cases[0].Source != model.AfterSalesSourceAccountBan {
		t.Fatalf("conflicting cancellation changed after-sales cases: %#v", cases)
	}
	active, err := subscriptionService.Store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if active.CancellationCaseID != 0 {
		t.Fatalf("rejected cancellation left case id %d", active.CancellationCaseID)
	}
}

func TestPendingAccountBanAfterSalesBlocksSubscriptionSideDoors(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 10, 12, 0, 0, 0, cycle.Location)
	}
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "待售后母号",
		Email:     "owner@example.com",
		SpaceName: "Pending Space",
		SeatCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := subscriptionService.Store.ListSeatsByAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "待售后客户",
		PriceYuan:        "30.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "0",
		CustomerEmail:    "customer@example.com",
		CustomerWechat:   "wx-customer",
		SeatID:           seats[0].ID,
		BoardedAt:        "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{BannedDate: "2026-08-10"}); err != nil {
		t.Fatal(err)
	}

	mutations := []struct {
		name string
		run  func() error
	}{
		{
			name: "edit",
			run: func() error {
				return subscriptionService.Update(subscriptionID, service.CreateInput{
					Name:             "不应保存的新名称",
					PriceYuan:        "35.00",
					CronExpr:         "interval:30d",
					NotifyOffsetsRaw: "0",
					CustomerEmail:    "customer@example.com",
					CustomerWechat:   "wx-customer",
					SeatID:           seats[0].ID,
					BoardedAt:        "2026-08-01",
				})
			},
		},
		{name: "copy", run: func() error {
			_, copyErr := subscriptionService.Copy(subscriptionID, seats[1].ID)
			return copyErr
		}},
		{name: "archive", run: func() error { return subscriptionService.Archive(subscriptionID) }},
		{name: "mark paid", run: func() error {
			return subscriptionService.SetDuePaid(subscriptionID, "2026-08-01", true)
		}},
		{name: "manual customer email", run: func() error {
			return subscriptionService.SendCustomerEmail(context.Background(), subscriptionID)
		}},
		{name: "manual test notification", run: func() error {
			return subscriptionService.TestNotify(context.Background(), subscriptionID)
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			if err := mutation.run(); err == nil {
				t.Fatalf("%s succeeded while after-sales was pending", mutation.name)
			}
		})
	}

	active, err := subscriptionService.Store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if active.Name != "待售后客户" || active.SeatID != seats[0].ID {
		t.Fatalf("blocked operations changed subscription: %#v", active)
	}
	freeSeats, err := subscriptionService.Store.ListFreeSeats(accountID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(freeSeats) != 1 || freeSeats[0].ID != seats[1].ID {
		t.Fatalf("blocked operations changed seat occupancy: %#v", freeSeats)
	}
}

func TestPendingAccountBanAfterSalesSuppressesQueuedNotificationRetry(t *testing.T) {
	subscriptionService := openTestService(t)
	clock := time.Date(2026, time.July, 15, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return clock }
	failing := &failingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: failing}

	accountID, seatIDs := createTestAccountWithSeats(t, subscriptionService, "通知母号", "车位1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "通知客户",
		PriceYuan:        "35.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2000-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if failing.calls != 1 {
		t.Fatalf("initial notification attempts = %d, want 1", failing.calls)
	}
	if _, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{BannedDate: "2026-07-15"}); err != nil {
		t.Fatal(err)
	}

	clock = clock.Add(2 * time.Minute)
	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: recorder}
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 0 {
		t.Fatalf("queued retry sent %d notifications during pending after-sales", recorder.calls)
	}
	logEntry, err := subscriptionService.Store.GetNotificationLog(
		subscriptionID,
		"2026-07-15",
		0,
		model.ChannelIYUU,
		model.NotificationKindScheduled,
	)
	if err != nil {
		t.Fatal(err)
	}
	if logEntry.Status != model.NotificationStatusPending || logEntry.AttemptCount != 1 {
		t.Fatalf("suppressed retry changed notification log: %#v", logEntry)
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
	if err := subscriptionService.UpdateAfterSalesCase(caseItem.ID, service.UpdateAfterSalesCaseInput{
		RefundAmountYuan: "12.34",
		Note:             "线下账单待核对",
	}); err != nil {
		t.Fatalf("review case without a linked bill should allow a manual refund: %v", err)
	}
}

func TestAfterSalesCaseCanBeAdjustedCompletedAndReopened(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 20, 12, 0, 0, 0, cycle.Location)
	}
	accountID, subscriptionID := createWarrantyCustomer(t, subscriptionService, "2026-07-01", true)
	if _, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{BannedDate: "2026-07-11"}); err != nil {
		t.Fatal(err)
	}
	page, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	caseID := page.Cases[0].Case.ID
	if err := subscriptionService.UpdateAfterSalesCase(caseID, service.UpdateAfterSalesCaseInput{
		RefundAmountYuan: "30.01",
		Note:             "错误的超额退款",
	}); err == nil || !strings.Contains(err.Error(), "剩余可退金额") {
		t.Fatalf("over-refund error = %v, want remaining-refundable validation", err)
	}
	unchanged, err := subscriptionService.Store.GetAfterSalesCase(caseID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.RefundAmountCents == 3001 || unchanged.Note == "错误的超额退款" {
		t.Fatalf("invalid over-refund was persisted: %#v", unchanged)
	}
	if err := subscriptionService.UpdateAfterSalesCase(caseID, service.UpdateAfterSalesCaseInput{
		RefundAmountYuan: "18.88",
		Note:             "微信已核对",
	}); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetAfterSalesCaseRefunded(caseID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.Store.GetSubscription(subscriptionID); err != sql.ErrNoRows {
		t.Fatalf("active subscription after account-ban refund error = %v, want sql.ErrNoRows", err)
	}
	archived, err := subscriptionService.Store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if archived.ArchivedAt == nil {
		t.Fatalf("subscription was not archived after refund: %#v", archived)
	}
	freeSeats, err := subscriptionService.Store.ListFreeSeats(accountID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(freeSeats) != 1 {
		t.Fatalf("free seats after account-ban refund = %d, want 1", len(freeSeats))
	}
	accountViews, err := subscriptionService.ListAccountsView()
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range accountViews {
		if view.Account.ID == accountID {
			t.Fatal("fully handled banned account remains visible")
		}
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
	if _, err := subscriptionService.Store.GetSubscription(subscriptionID); err != nil {
		t.Fatalf("subscription was not restored after undo: %v", err)
	}
	freeSeats, err = subscriptionService.Store.ListFreeSeats(accountID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(freeSeats) != 0 {
		t.Fatalf("undo left %d free seats, want 0", len(freeSeats))
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

func TestCompletedAfterSalesCaseLeavesPageAfter24HoursButKeepsRefundAccounting(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }
	accountID, _ := createWarrantyCustomer(t, subscriptionService, "2026-08-01", true)
	if _, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{
		BannedDate: "2026-08-10",
	}); err != nil {
		t.Fatal(err)
	}
	page, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	caseID := page.Cases[0].Case.ID
	refundCents := page.Cases[0].Case.RefundAmountCents
	if err := subscriptionService.SetAfterSalesCaseRefunded(caseID, true); err != nil {
		t.Fatal(err)
	}

	now = now.Add(23*time.Hour + 59*time.Minute)
	page, err = subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Cases) != 1 {
		t.Fatalf("case disappeared before 24 hours: %d", len(page.Cases))
	}

	now = now.Add(2 * time.Minute)
	page, err = subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Cases) != 0 {
		t.Fatalf("expired processed cases remain visible: %#v", page)
	}
	if page.Summary.TotalCount != 1 || page.Summary.RefundedCount != 1 || page.Summary.RefundedAmountCents != refundCents {
		t.Fatalf("historical after-sales summary was lost: %#v", page.Summary)
	}
	if len(page.SummaryCases) != 1 || page.SummaryCases[0].Case.ID != caseID {
		t.Fatalf("historical statistic detail was lost: %#v", page.SummaryCases)
	}
	stored, err := subscriptionService.Store.GetAfterSalesCase(caseID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != model.AfterSalesStatusRefunded {
		t.Fatalf("stored case status = %q, want refunded", stored.Status)
	}
	dashboard, err := subscriptionService.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.TotalRefundCents != refundCents {
		t.Fatalf("refund accounting after page cleanup = %d, want %d", dashboard.TotalRefundCents, refundCents)
	}
}

func TestPendingAfterSalesCaseRemainsVisibleAfter24Hours(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }
	accountID, _ := createWarrantyCustomer(t, subscriptionService, "2026-08-01", true)
	if _, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{
		BannedDate: "2026-08-10",
	}); err != nil {
		t.Fatal(err)
	}

	now = now.Add(48 * time.Hour)
	page, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Cases) != 1 || page.Cases[0].Case.Status != model.AfterSalesStatusPending {
		t.Fatalf("pending case was removed by retention cleanup: %#v", page.Cases)
	}
}

func TestUndoAccountBanRefundRollsBackWhenOriginalSeatIsOccupied(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 10, 12, 0, 0, 0, cycle.Location)
	}
	accountID, subscriptionID := createWarrantyCustomer(t, subscriptionService, "2026-08-01", true)
	if _, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{
		BannedDate: "2026-08-10",
	}); err != nil {
		t.Fatal(err)
	}
	page, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	caseID := page.Cases[0].Case.ID
	if err := subscriptionService.SetAfterSalesCaseRefunded(caseID, true); err != nil {
		t.Fatal(err)
	}

	archived, err := subscriptionService.Store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	archived.ID = 0
	archived.Name = "replacement occupant"
	if _, err := subscriptionService.Store.CreateSubscription(archived); err != nil {
		t.Fatal(err)
	}

	if err := subscriptionService.SetAfterSalesCaseRefunded(caseID, false); err == nil {
		t.Fatal("undo succeeded even though the original seat was occupied")
	}
	storedCase, err := subscriptionService.Store.GetAfterSalesCase(caseID)
	if err != nil {
		t.Fatal(err)
	}
	if storedCase.Status != model.AfterSalesStatusRefunded || storedCase.ProcessedAt == nil {
		t.Fatalf("failed undo did not roll back case state: %#v", storedCase)
	}
	stillArchived, err := subscriptionService.Store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if stillArchived.ArchivedAt == nil {
		t.Fatal("failed undo unexpectedly restored the refunded subscription")
	}
}

func TestAfterSalesReferencedBillCannotBeDeletedOrUnmarked(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 10, 12, 0, 0, 0, cycle.Location)
	}
	accountID, subscriptionID := createWarrantyCustomer(t, subscriptionService, "2026-08-01", true)
	if _, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{
		BannedDate: "2026-08-10",
	}); err != nil {
		t.Fatal(err)
	}
	page, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	caseItem := page.Cases[0].Case
	if caseItem.BillID <= 0 {
		t.Fatalf("after-sales case has no bill reference: %#v", caseItem)
	}
	if err := subscriptionService.SetDuePaid(subscriptionID, caseItem.PeriodStart, false); err == nil {
		t.Fatal("calendar unmarked a bill referenced by after-sales")
	}
	if err := subscriptionService.DeleteBill(caseItem.BillID); err == nil {
		t.Fatal("bills page deleted a bill referenced by after-sales")
	}
	if err := subscriptionService.UpdateBill(caseItem.BillID, service.BillEditInput{
		AmountYuan: "99.00",
		Note:       "不应写入",
	}); err == nil {
		t.Fatal("bills page edited a bill referenced by after-sales")
	}
	if _, err := subscriptionService.Store.GetBill(caseItem.BillID); err != nil {
		t.Fatalf("protected bill disappeared: %v", err)
	}
	protectedBill, err := subscriptionService.Store.GetBill(caseItem.BillID)
	if err != nil {
		t.Fatal(err)
	}
	if protectedBill.AmountCents != caseItem.PaidAmountCents || protectedBill.Note == "不应写入" {
		t.Fatalf("protected bill was modified: %#v", protectedBill)
	}
	if err := subscriptionService.SetAfterSalesCaseRefunded(caseItem.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.DeleteBill(caseItem.BillID); err == nil {
		t.Fatal("completed refund bill should remain protected")
	}
}

func TestAfterSalesSubscriptionWithoutBillCannotBeSoftDeleted(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 10, 12, 0, 0, 0, cycle.Location)
	}
	accountID, subscriptionID := createWarrantyCustomer(t, subscriptionService, "2026-08-01", false)
	if _, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{
		BannedDate: "2026-08-10",
	}); err != nil {
		t.Fatal(err)
	}
	page, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	if page.Cases[0].Case.BillID != 0 {
		t.Fatalf("test case unexpectedly has a bill: %#v", page.Cases[0].Case)
	}
	if err := subscriptionService.SetAfterSalesCaseRefunded(page.Cases[0].Case.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SoftDeleteArchived(subscriptionID); err == nil {
		t.Fatal("soft delete removed a subscription still referenced by after-sales")
	}
	if _, err := subscriptionService.Store.GetSubscriptionIncludingArchived(subscriptionID); err != nil {
		t.Fatalf("protected archived subscription disappeared: %v", err)
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

func TestAfterSalesCaseCanReassignCustomerWithoutRefund(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 5, 12, 0, 0, 0, cycle.Location)
	}
	sourceAccountID, subscriptionID := createWarrantyCustomer(t, subscriptionService, "2026-07-20", true)
	sourceSubscription, err := subscriptionService.Store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	replacementAccountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "replacement@example.com",
		Email:     "replacement@example.com",
		SpaceName: "Replacement Space",
		SeatNames: []string{"新车位1", "新车位2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	replacementSeats, err := subscriptionService.Store.ListSeatsByAccount(replacementAccountID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.BanAccount(sourceAccountID, service.BanAccountInput{BannedDate: "2026-08-01"}); err != nil {
		t.Fatal(err)
	}
	page, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	caseID := page.Cases[0].Case.ID

	if err := subscriptionService.ReassignAfterSalesCase(caseID, service.ReassignAfterSalesCaseInput{
		AccountID: replacementAccountID,
		SeatID:    replacementSeats[1].ID,
	}); err != nil {
		t.Fatal(err)
	}

	moved, err := subscriptionService.Store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if moved.AccountID != replacementAccountID || moved.SeatID != replacementSeats[1].ID {
		t.Fatalf("moved subscription = account %d seat %d", moved.AccountID, moved.SeatID)
	}
	freeSourceSeats, err := subscriptionService.Store.ListFreeSeats(sourceAccountID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(freeSourceSeats) != 1 || freeSourceSeats[0].ID != sourceSubscription.SeatID {
		t.Fatalf("source seat not released: %#v", freeSourceSeats)
	}

	page, err = subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	completed := page.Cases[0].Case
	if completed.Status != model.AfterSalesStatusReassigned || completed.ProcessedAt == nil {
		t.Fatalf("reassigned status = %q / %v", completed.Status, completed.ProcessedAt)
	}
	if completed.ReplacementAccountID != replacementAccountID || completed.ReplacementSeatID != replacementSeats[1].ID {
		t.Fatalf("replacement ids = %d/%d", completed.ReplacementAccountID, completed.ReplacementSeatID)
	}
	if completed.ReplacementAccountEmail != "replacement@example.com" || completed.ReplacementSpaceName != "Replacement Space" || completed.ReplacementSeatName != "新车位2" {
		t.Fatalf("replacement snapshot = %#v", completed)
	}
	if page.Summary.ReassignedCount != 1 || page.Summary.RefundedCount != 0 || page.Summary.RefundedAmountCents != 0 {
		t.Fatalf("after-sales summary = %#v", page.Summary)
	}

	dashboard, err := subscriptionService.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.TotalRefundCents != 0 {
		t.Fatalf("reassignment refund = %d, want 0", dashboard.TotalRefundCents)
	}
	if err := subscriptionService.ReassignAfterSalesCase(caseID, service.ReassignAfterSalesCaseInput{
		AccountID: replacementAccountID,
		SeatID:    replacementSeats[0].ID,
	}); err == nil {
		t.Fatal("repeated reassignment should fail")
	}
}

func TestCompletedRefundUpdatesDashboardBillsAndProcessingMonth(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 5, 12, 0, 0, 0, cycle.Location)
	}
	accountID, subscriptionID := createWarrantyCustomer(t, subscriptionService, "2026-07-20", true, "10.00")
	if _, err := subscriptionService.BanAccount(accountID, service.BanAccountInput{BannedDate: "2026-07-30"}); err != nil {
		t.Fatal(err)
	}
	afterSalesPage, err := subscriptionService.ListAfterSalesPage()
	if err != nil {
		t.Fatal(err)
	}
	caseID := afterSalesPage.Cases[0].Case.ID
	if err := subscriptionService.SetAfterSalesCaseRefunded(caseID, true); err != nil {
		t.Fatal(err)
	}

	dashboard, err := subscriptionService.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.TotalAmountYuan != "30.00" || dashboard.TotalCostYuan != "10.00" || dashboard.TotalRefundYuan != "20.00" || dashboard.NetRevenueYuan != "10.00" || dashboard.TotalProfitYuan != "0.00" {
		t.Fatalf("dashboard amounts = gross %s refund %s net %s profit %s", dashboard.TotalAmountYuan, dashboard.TotalRefundYuan, dashboard.NetRevenueYuan, dashboard.TotalProfitYuan)
	}

	billsPage, err := subscriptionService.ListBillsPage()
	if err != nil {
		t.Fatal(err)
	}
	if len(billsPage.Bills) != 1 {
		t.Fatalf("bills = %d, want 1", len(billsPage.Bills))
	}
	bill := billsPage.Bills[0]
	if bill.SubscriptionID != subscriptionID || bill.RefundYuan != "20.00" || bill.NetAmountYuan != "10.00" {
		t.Fatalf("bill accounting = %#v", bill)
	}
	if billsPage.Summary.TotalRefundYuan != "20.00" || billsPage.Summary.NetAmountYuan != "10.00" || billsPage.Summary.ThisMonthRefundYuan != "20.00" || billsPage.Summary.ThisMonthNetAmountYuan != "-20.00" {
		t.Fatalf("bill summary = %#v", billsPage.Summary)
	}
	var july, august service.MonthAmountBar
	for _, month := range billsPage.Summary.MonthlyTrend {
		switch month.Month {
		case "2026-07":
			july = month
		case "2026-08":
			august = month
		}
	}
	if july.GrossAmountCents != 3000 || july.RefundCents != 0 || july.AmountCents != 3000 {
		t.Fatalf("july trend = %#v", july)
	}
	if august.GrossAmountCents != 0 || august.RefundCents != 2000 || august.AmountCents != -2000 {
		t.Fatalf("august trend = %#v", august)
	}

	if err := subscriptionService.SetAfterSalesCaseRefunded(caseID, false); err != nil {
		t.Fatal(err)
	}
	dashboard, err = subscriptionService.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.TotalRefundCents != 0 || dashboard.NetRevenueYuan != "30.00" || dashboard.TotalProfitYuan != "20.00" {
		t.Fatalf("dashboard after undo = %#v", dashboard)
	}
}

func createWarrantyCustomer(
	t *testing.T,
	subscriptionService *service.SubscriptionService,
	boardedAt string,
	paid bool,
	costYuan ...string,
) (int64, int64) {
	t.Helper()
	monthlyCostYuan := ""
	if len(costYuan) > 0 {
		monthlyCostYuan = costYuan[0]
	}
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "质保母号",
		Email:     "owner@example.com",
		SpaceName: "Warranty Space",
		CostYuan:  monthlyCostYuan,
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
