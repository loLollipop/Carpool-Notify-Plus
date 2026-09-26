package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"carpool-notify/internal/model"
)

func TestSubscriptionEditsRejectSnapshotBeforeRenewalApproval(t *testing.T) {
	for _, path := range []string{"edit", "sync", "move"} {
		t.Run(path, func(t *testing.T) {
			store, err := Open(filepath.Join(t.TempDir(), "renewal-conflict.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			nextPrice := int64(3500)
			id, err := store.CreateSubscription(model.Subscription{
				Name: "customer", BusinessType: model.SubscriptionBusinessPlus,
				CustomerEmail: "customer@example.com", PricePerPersonCents: 3000,
				NextPriceCents: &nextPrice, NextPriceEffectiveDueDate: "2026-08-31",
				CronExpr: "interval:30d", BoardedAt: "2026-08-01",
			})
			if err != nil {
				t.Fatal(err)
			}
			// Force a version beyond the wall clock: renewal must still advance it.
			if _, err := store.database.Exec(`UPDATE subscriptions SET updated_at = ? WHERE id = ?`, "2099-01-01T00:00:00Z", id); err != nil {
				t.Fatal(err)
			}
			stale, err := store.GetSubscription(id)
			if err != nil {
				t.Fatal(err)
			}
			application := model.RenewalApplication{
				SubscriptionID: id, TrackingToken: "renewal-test", CustomerEmail: stale.CustomerEmail,
				DueDate: "2026-08-31", PeriodCount: 1, PeriodEndDate: "2026-09-30", AmountCents: 3500,
			}
			application.ID, err = store.CreateRenewalApplication(application)
			if err != nil {
				t.Fatal(err)
			}
			if err := store.ApproveRenewalApplication(application, stale, []model.Bill{{
				SubscriptionID: id, DueDate: application.DueDate, AmountCents: 3500,
			}}, application.PeriodEndDate, ""); err != nil {
				t.Fatal(err)
			}
			stale.Remark = "stale edit"
			switch path {
			case "edit":
				err = store.UpdateSubscription(stale)
			case "sync":
				err = store.UpdateSubscriptionAndSyncBill(stale, application.DueDate, 3000, 500)
			case "move":
				stale.BoardedAt = "2026-08-02"
				err = store.UpdateSubscriptionAndMoveInitialBill(stale, application.DueDate, "2026-09-01", 3000, 500)
			}
			if !errors.Is(err, ErrSubscriptionStateChanged) || errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("stale %s error = %v, want identifiable edit conflict", path, err)
			}
			stored, err := store.GetSubscription(id)
			if err != nil {
				t.Fatal(err)
			}
			if stored.PricePerPersonCents != 3500 || stored.NextPriceCents != nil || stored.NextPriceEffectiveDueDate != "" || stored.BoardedAt != "2026-08-01" || stored.Remark != "" || !stored.UpdatedAt.After(stale.UpdatedAt) {
				t.Fatalf("renewal state overwritten: %#v", stored)
			}
			bill, err := store.GetBillByOccurrence(id, application.DueDate)
			if err != nil || bill.AmountCents != 3500 || bill.CostCents != 0 {
				t.Fatalf("renewal bill changed: %#v, %v", bill, err)
			}
		})
	}
}

func TestSubscriptionScheduleEditRechecksBillCount(t *testing.T) {
	for _, path := range []string{"edit", "sync", "move"} {
		t.Run(path, func(t *testing.T) {
			store, err := Open(filepath.Join(t.TempDir(), "bill-conflict.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			id, err := store.CreateSubscription(integritySubscription())
			if err != nil {
				t.Fatal(err)
			}
			if path == "move" {
				if err := store.SetDuePaid(id, "2026-09-01", true, 3000); err != nil {
					t.Fatal(err)
				}
			}
			stale, err := store.GetSubscription(id)
			if err != nil {
				t.Fatal(err)
			}
			// A new bill need not modify the subscription's timestamp.
			if err := store.SetDuePaid(id, "2026-10-01", true, 3000); err != nil {
				t.Fatal(err)
			}
			stale.BoardedAt = "2026-09-02"
			switch path {
			case "edit":
				err = store.UpdateSubscription(stale)
			case "sync":
				err = store.UpdateSubscriptionAndSyncBill(stale, "2026-10-01", 4000, 500)
			case "move":
				err = store.UpdateSubscriptionAndMoveInitialBill(stale, "2026-09-01", "2026-09-02", 4000, 500)
			}
			if !errors.Is(err, ErrSubscriptionStateChanged) {
				t.Fatalf("%s error = %v, want bill-count conflict", path, err)
			}
			stored, err := store.GetSubscription(id)
			if err != nil || stored.BoardedAt != "2026-09-01" || !stored.UpdatedAt.Equal(stale.UpdatedAt) {
				t.Fatalf("schedule changed: %#v, %v", stored, err)
			}
			bill, err := store.GetBillByOccurrence(id, "2026-10-01")
			if err != nil || bill.AmountCents != 3000 {
				t.Fatalf("bill changed: %#v, %v", bill, err)
			}
		})
	}
}

func TestSubscriptionEditRequiresVersionAndPreservesAfterSalesGuard(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "edit-guards.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	id, err := store.CreateSubscription(integritySubscription())
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := store.GetSubscription(id)
	if err != nil {
		t.Fatal(err)
	}
	unversioned := subscription
	unversioned.UpdatedAt = time.Time{}
	if err := store.UpdateSubscription(unversioned); !errors.Is(err, ErrSubscriptionStateChanged) {
		t.Fatalf("unversioned error = %v", err)
	}
	if _, err := store.database.Exec(`INSERT INTO after_sales_cases
		(subscription_id, banned_date, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		id, "2026-09-01", model.AfterSalesStatusPending, formatTime(time.Now()), formatTime(time.Now())); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSubscription(subscription); !errors.Is(err, ErrSubscriptionHasPendingAfterSales) {
		t.Fatalf("pending after-sales error = %v", err)
	}
	subscription.ID += 1000
	if err := store.UpdateSubscription(subscription); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing subscription error = %v, want sql.ErrNoRows", err)
	}
}
