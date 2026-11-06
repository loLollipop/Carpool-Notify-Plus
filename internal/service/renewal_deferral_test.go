package service_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/notify"
	"carpool-notify/internal/service"
)

func deferCustomer(t *testing.T, svc *service.SubscriptionService, id int64) model.Subscription {
	t.Helper()
	views, err := svc.ListView()
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		if view.Subscription.ID != id {
			continue
		}
		if err := svc.DeferSubscriptionRenewal(id, service.DeferRenewalInput{
			ExpectedDueDate: view.NextDueDate, ExpectedUpdatedAt: view.Subscription.UpdatedAt.Format(time.RFC3339Nano),
		}); err != nil {
			t.Fatal(err)
		}
		current, err := svc.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		return current
	}
	t.Fatal("customer not found")
	return model.Subscription{}
}

func TestPausedAccountKeepsOccupantAndExcludesAllOtherFormSeats(t *testing.T) {
	svc := openTestService(t)
	svc.Clock = func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, cycle.Location) }
	accountID, seats := createTestAccountWithSeats(t, svc, "paused space", "occupied", "empty")
	input := service.CreateInput{Name: "occupant", PriceYuan: "30", CronExpr: "interval:30d", BoardedAt: "2026-07-01", SeatID: seats[0], NotifyOffsetsRaw: "0"}
	id, err := svc.CreateWithInitialBill(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetAccountPaused(accountID, true); err != nil {
		t.Fatal(err)
	}
	options, err := svc.ListAccountOptionsForForm(0)
	if err != nil || len(options) != 0 {
		t.Fatalf("new allocation options: %#v, %v", options, err)
	}
	options, err = svc.ListAccountOptionsForForm(seats[0])
	if err != nil || len(options) != 1 || len(options[0].Seats) != 1 || options[0].Seats[0].ID != seats[0] {
		t.Fatalf("edit options: %#v, %v", options, err)
	}
	current, _ := svc.Get(id)
	input.ExpectedUpdatedAt = current.UpdatedAt.Format(time.RFC3339Nano)
	input.Remark = "contact corrected"
	if err := svc.Update(id, input); err != nil {
		t.Fatalf("same seat edit: %v", err)
	}
	input.SeatID = seats[1]
	if _, err := svc.Create(input); err == nil {
		t.Fatal("paused explicit creation succeeded")
	}
	input.SeatID, input.AccountID = 0, accountID
	if _, err := svc.Create(input); err == nil {
		t.Fatal("paused auto creation succeeded")
	}
	if _, err := svc.Copy(id, seats[1]); err == nil {
		t.Fatal("paused copy succeeded")
	}
	if err := svc.SetDuePaid(id, "2026-07-31", true); err != nil {
		t.Fatalf("paused occupant payment: %v", err)
	}
	views, _ := svc.ListView()
	if len(views) != 1 || !views[0].AccountPaused || views[0].RenewalDeferred {
		t.Fatalf("paused does not defer: %#v", views)
	}
	accounts, err := svc.ListAccountsView()
	if err != nil || len(accounts) != 1 || !accounts[0].Account.Paused || accounts[0].Account.BannedAt != "" {
		t.Fatalf("account: %#v, %v", accounts, err)
	}
	overview, err := svc.GetOperationsOverview()
	if err != nil || overview.Capacity.SeatTotal != 2 || overview.Capacity.SeatUsed != 1 || overview.Capacity.SeatFree != 0 || overview.Capacity.SeatPaused != 1 {
		t.Fatalf("capacity: %#v, %v", overview.Capacity, err)
	}
	if err := svc.SetAccountPaused(accountID, false); err != nil {
		t.Fatal(err)
	}
	options, err = svc.ListAccountOptionsForForm(0)
	if err != nil || len(options) != 1 || len(options[0].Seats) != 1 || options[0].Seats[0].ID != seats[1] {
		t.Fatalf("restored options: %#v, %v", options, err)
	}
}

func TestRenewalDeferralPreservesSeatLedgerAndIsGlobalAcrossMonths(t *testing.T) {
	svc := openTestService(t)
	svc.Clock = func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, cycle.Location) }
	accountID, id := createWarrantyCustomer(t, svc, "2026-07-01", true)
	before, _ := svc.Get(id)
	billsBefore, _ := svc.Store.ListBills()
	deferred := deferCustomer(t, svc, id)
	if deferred.ArchivedAt != nil || deferred.SeatID != before.SeatID || deferred.RenewalDeferredAt == nil || deferred.RenewalDeferredDueDate != "2026-07-31" {
		t.Fatalf("deferred: %#v", deferred)
	}
	billsAfter, _ := svc.Store.ListBills()
	if len(billsAfter) != len(billsBefore) {
		t.Fatal("deferral changed ledger")
	}
	cases, _ := svc.Store.ListAfterSalesCases()
	if len(cases) != 0 {
		t.Fatal("deferral created refund case")
	}
	if err := svc.SetAccountPaused(accountID, true); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetAccountPaused(accountID, false); err != nil {
		t.Fatal(err)
	}
	for _, month := range []time.Month{time.July, time.August, time.December} {
		calendar, err := svc.CalendarMonth(time.Date(2026, month, 1, 0, 0, 0, 0, cycle.Location))
		if err != nil {
			t.Fatal(err)
		}
		if len(calendar.DeferredSubscriptions) != 1 || calendar.DeferredSubscriptions[0].DeferredAtLabel == "" || !calendar.DeferredSubscriptions[0].RenewalDeferred {
			t.Fatalf("month %v deferred = %#v", month, calendar.DeferredSubscriptions)
		}
		if len(calendar.ActionableOccurrences) != 0 || calendar.PendingMonthCount != 0 {
			t.Fatalf("month %v actionable unpaid: %#v", month, calendar)
		}
		for _, day := range calendar.Days {
			for _, occurrence := range day.Occurrences {
				if !occurrence.Paid {
					t.Fatalf("deferred unpaid on grid: %#v", occurrence)
				}
			}
		}
	}
	overview, err := svc.GetOperationsOverview()
	if err != nil || overview.Work.TeamDueCount != 0 || overview.Capacity.SeatUsed != 1 || overview.Dashboard.TotalAmountYuan != "30.00" {
		t.Fatalf("overview: %#v, %v", overview, err)
	}
	if err := svc.RestoreSubscriptionRenewal(id); err != nil {
		t.Fatal(err)
	}
	if err := svc.RestoreSubscriptionRenewal(id); err != nil {
		t.Fatalf("restore replay: %v", err)
	}
	views, _ := svc.ListView()
	if len(views) != 1 || views[0].RenewalDeferred || views[0].NextDueDate != "2026-07-31" {
		t.Fatalf("restored due = %#v", views)
	}
}

func TestDeferredRenewalPaymentAndCancellationInvalidateRemoval(t *testing.T) {
	svc := openTestService(t)
	svc.Clock = func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, cycle.Location) }
	_, id := createWarrantyCustomer(t, svc, "2026-07-01", true)
	deferred := deferCustomer(t, svc, id)
	input := service.DeferRenewalInput{ExpectedDueDate: deferred.RenewalDeferredDueDate, ExpectedUpdatedAt: deferred.UpdatedAt.Format(time.RFC3339Nano)}
	if err := svc.DeferSubscriptionRenewal(id, input); !errors.Is(err, db.ErrSubscriptionStateChanged) {
		t.Fatalf("duplicate defer: %v", err)
	}
	if _, err := svc.RequestCancellation(id); !errors.Is(err, db.ErrRenewalDeferred) {
		t.Fatalf("deferred cancellation: %v", err)
	}
	if err := svc.Archive(id); !errors.Is(err, db.ErrRenewalDeferred) {
		t.Fatalf("deferred archive: %v", err)
	}
	if err := svc.SetDuePaid(id, deferred.RenewalDeferredDueDate, true); err != nil {
		t.Fatal(err)
	}
	current, _ := svc.Get(id)
	if current.RenewalDeferredAt != nil || current.RenewalDeferredDueDate != "" {
		t.Fatal("payment did not clear deferral")
	}
	if _, err := svc.CompleteDeferredSubscriptions([]service.CompleteDeferredInput{{SubscriptionID: id, ExpectedUpdatedAt: deferred.UpdatedAt.Format(time.RFC3339Nano)}}); !errors.Is(err, db.ErrSubscriptionStateChanged) {
		t.Fatalf("paid customer removal: %v", err)
	}
	if err := svc.SetDuePaid(id, "2026-07-31", false); err != nil {
		t.Fatal(err)
	}
	current, _ = svc.Get(id)
	if current.RenewalDeferredAt != nil {
		t.Fatal("payment cancellation resurrected old deferral")
	}
	deferred = deferCustomer(t, svc, id)
	if err := svc.SetDuePaid(id, "2026-07-01", false); err != nil {
		t.Fatal(err)
	}
	current, _ = svc.Get(id)
	if current.RenewalDeferredAt != nil {
		t.Fatal("ledger boundary changed without restoring follow-up")
	}
}

func TestDeferredCompleteIsAtomicPreservesFutureBillsAndUsesRollingSeatFreeze(t *testing.T) {
	svc := openTestService(t)
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, cycle.Location)
	svc.Clock = func() time.Time { return now }
	accountID, firstID := createWarrantyCustomerAfterRecentDepartures(t, svc, "2026-07-01", true, 3)
	seatID, err := svc.Store.CreateSeat(model.Seat{AccountID: accountID, Name: "second"})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := svc.CreateWithInitialBill(service.CreateInput{Name: "second", PriceYuan: "30", CronExpr: "interval:30d", BoardedAt: "2026-07-01", SeatID: seatID, NotifyOffsetsRaw: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDuePaid(firstID, "2026-08-30", true); err != nil {
		t.Fatal(err)
	}
	first, second := deferCustomer(t, svc, firstID), deferCustomer(t, svc, secondID)
	requests := []service.CompleteDeferredInput{{SubscriptionID: firstID, ExpectedUpdatedAt: first.UpdatedAt.Format(time.RFC3339Nano)}, {SubscriptionID: secondID, ExpectedUpdatedAt: second.UpdatedAt.Add(-time.Second).Format(time.RFC3339Nano)}}
	if _, err := svc.CompleteDeferredSubscriptions(requests); !errors.Is(err, db.ErrSubscriptionStateChanged) {
		t.Fatalf("stale batch: %v", err)
	}
	if stillActive, err := svc.Get(firstID); err != nil || stillActive.RenewalDeferredAt == nil {
		t.Fatalf("partial completion: %#v, %v", stillActive, err)
	}
	if err := svc.SetAccountPaused(accountID, true); err != nil {
		t.Fatal(err)
	}
	requests[1].ExpectedUpdatedAt = second.UpdatedAt.Format(time.RFC3339Nano)
	count, err := svc.CompleteDeferredSubscriptions(requests)
	if err != nil || count != 2 {
		t.Fatalf("paused complete = %d, %v", count, err)
	}
	archivedFirst, _ := svc.Store.GetSubscriptionIncludingArchived(firstID)
	archivedSecond, _ := svc.Store.GetSubscriptionIncludingArchived(secondID)
	if archivedFirst.ArchivedAt == nil || archivedFirst.RenewalDeferredAt != nil || archivedFirst.SeatFrozenUntil != nil {
		t.Fatalf("fourth departure = %#v", archivedFirst)
	}
	if archivedSecond.ArchivedAt == nil || archivedSecond.RenewalDeferredAt != nil || archivedSecond.SeatFrozenUntil == nil || !archivedSecond.SeatFrozenUntil.After(now) {
		t.Fatalf("fifth departure = %#v", archivedSecond)
	}
	if paid, err := svc.Store.IsDuePaid(firstID, "2026-08-30"); err != nil || !paid {
		t.Fatalf("future bill removed: %v, %v", paid, err)
	}
	cases, _ := svc.Store.ListAfterSalesCases()
	if len(cases) != 0 {
		t.Fatal("complete created refund")
	}
	calendar, err := svc.CalendarMonth(now)
	if err != nil || len(calendar.DeferredSubscriptions) != 0 || len(calendar.ArchivedSubscriptions) != 5 {
		t.Fatalf("archive history: %#v, %v", calendar, err)
	}
	if _, err := svc.CompleteDeferredSubscriptions(requests); !errors.Is(err, db.ErrSubscriptionStateChanged) {
		t.Fatalf("complete replay: %v", err)
	}
}

func TestDeferredNotificationsCancelQueuedAndRevalidateBeforeSend(t *testing.T) {
	svc := openTestService(t)
	svc.Clock = func() time.Time { return time.Date(2026, 7, 31, 12, 0, 0, 0, cycle.Location) }
	_, id := createWarrantyCustomer(t, svc, "2026-07-01", true)
	recorder := &recordingSender{}
	svc.Notify = notify.Registry{IYUU: recorder}
	queued, err := svc.Store.UpsertPendingNotification(id, "2026-07-31", 0, model.ChannelIYUU, model.NotificationKindScheduled)
	if err != nil {
		t.Fatal(err)
	}
	deferCustomer(t, svc, id)
	log, err := svc.Store.GetNotificationLogByID(queued.ID)
	if err != nil || log.Status != model.NotificationStatusCanceled {
		t.Fatalf("queued: %#v, %v", log, err)
	}
	// Simulate a planner that had read the subscription just before deferral.
	if _, err := svc.Store.UpsertPendingNotification(id, "2026-07-31", 0, model.ChannelIYUU, model.NotificationKindScheduled); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 0 {
		t.Fatalf("deferred notified: %#v", recorder.calls)
	}
	if err := svc.SendCustomerEmail(context.Background(), id); !errors.Is(err, db.ErrRenewalDeferred) && (err == nil || err.Error() != "客户续费已搁置，请先恢复跟进") {
		t.Fatalf("manual reminder = %v", err)
	}
	if err := svc.RestoreSubscriptionRenewal(id); err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 1 {
		t.Fatalf("restored notification count = %d", recorder.calls)
	}
}

func TestPausedAccountRedemptionAndAfterSalesReplacement(t *testing.T) {
	svc := openTestService(t)
	svc.Clock = func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, cycle.Location) }
	pausedID, _ := createTestAccountWithSeats(t, svc, "paused@example.com", "free")
	_, pausedSeats := createTestAccountWithSeats(t, svc, "paused@example.com", "free")
	_, availableSeats := createTestAccountWithSeats(t, svc, "available", "free", "replacement")
	if err := svc.SetAccountPaused(pausedID, true); err != nil {
		t.Fatal(err)
	}
	for i, seatID := range []int64{pausedSeats[0], 0} {
		appID, err := svc.Store.CreateRedemptionApplication(model.RedemptionApplication{TrackingToken: fmt.Sprint(i), CustomerEmail: fmt.Sprintf("redeem%d@example.com", i)})
		if err != nil {
			t.Fatal(err)
		}
		id, err := svc.InviteRedemptionApplication(appID, service.RedemptionInviteInput{SeatID: seatID, PriceYuan: "30", CronExpr: "interval:30d", NotifyOffsetsRaw: "0", BoardedAt: "2026-08-01"})
		if i == 0 {
			if err == nil {
				t.Fatal("explicit redemption used paused seat")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		subscription, _ := svc.Get(id)
		if subscription.SeatID != availableSeats[0] {
			t.Fatalf("auto redemption seat = %d", subscription.SeatID)
		}
	}
	sourceID, customerID := createWarrantyCustomer(t, svc, "2026-07-01", true)
	if _, err := svc.BanAccount(sourceID, service.BanAccountInput{BannedDate: "2026-07-15"}); err != nil {
		t.Fatal(err)
	}
	cases, _ := svc.Store.ListAfterSalesCases()
	for _, seatID := range []int64{0, pausedSeats[0]} {
		if err := svc.ReassignAfterSalesCase(cases[0].ID, service.ReassignAfterSalesCaseInput{AccountID: pausedID, SeatID: seatID}); !errors.Is(err, db.ErrAccountPaused) {
			t.Fatalf("paused replacement: %v", err)
		}
	}
	current, _ := svc.Get(customerID)
	if current.AccountID != sourceID {
		t.Fatal("failed replacement moved occupant")
	}
}

func TestDeferredLegacyUnpaidBoundarySurvivesMonthRollover(t *testing.T) {
	svc := openTestService(t)
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, cycle.Location)
	svc.Clock = func() time.Time { return now }
	id := createTestSubscription(t, svc, "pre-ledger", "0 18 31 * *")
	deferred := deferCustomer(t, svc, id)
	if deferred.RenewalDeferredDueDate != "2026-07-31" {
		t.Fatalf("due %s", deferred.RenewalDeferredDueDate)
	}
	now = time.Date(2026, 8, 2, 12, 0, 0, 0, cycle.Location)
	views, err := svc.ListView()
	if err != nil || len(views) != 1 || views[0].NextDueDate != "2026-07-31" {
		t.Fatalf("legacy rollover: %#v, %v", views, err)
	}
	if err := svc.RestoreSubscriptionRenewal(id); err != nil {
		t.Fatal(err)
	}
	views, err = svc.ListView()
	if err != nil || len(views) != 1 || views[0].RenewalDeferred || views[0].NextDueDate != "2026-07-31" {
		t.Fatalf("legacy resume: %#v, %v", views, err)
	}
	deferred = deferCustomer(t, svc, id)
	if _, err := svc.CompleteDeferredSubscriptions([]service.CompleteDeferredInput{{SubscriptionID: id, ExpectedUpdatedAt: deferred.UpdatedAt.Format(time.RFC3339Nano)}}); err != nil {
		t.Fatalf("legacy complete: %v", err)
	}
}
