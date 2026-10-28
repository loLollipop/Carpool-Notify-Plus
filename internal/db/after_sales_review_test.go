package db

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

func reviewSubscription(t *testing.T, business string, amount int64) (*Store, int64, int64) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "review.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	subscription := model.Subscription{
		Name: "customer", BusinessType: business, CustomerEmail: "customer@example.com",
		PricePerPersonCents: amount, CronExpr: "interval:30d", BoardedAt: "2026-08-01",
	}
	if business == model.SubscriptionBusinessTeam {
		subscription.AccountID, err = store.CreateAccountWithSeats(model.Account{Name: "owner"}, 0, "2026-08-01", []string{"seat"})
		if err != nil {
			t.Fatal(err)
		}
		seats, err := store.ListSeatsByAccount(subscription.AccountID)
		if err != nil || len(seats) != 1 {
			t.Fatalf("seats = %#v, %v", seats, err)
		}
		subscription.SeatID = seats[0].ID
	}
	id, err := store.CreateSubscriptionWithInitialBill(subscription, "2026-08-01", amount)
	if err != nil {
		t.Fatal(err)
	}
	return store, id, subscription.AccountID
}

func TestNaturalExpiryCannotArchiveRenewedSubscription(t *testing.T) {
	for _, business := range []string{model.SubscriptionBusinessTeam, model.SubscriptionBusinessPlus} {
		t.Run(business, func(t *testing.T) {
			store, id, _ := reviewSubscription(t, business, 10000)
			stale, err := store.GetSubscription(id)
			if err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 8, 31, 12, 0, 0, 0, cycle.Location)
			if err := store.SetDuePaidForSubscription(stale, "2026-08-31", true, 10000); err != nil {
				t.Fatal(err)
			}
			if err := store.ArchiveExpiredSubscription(stale, now, nil); !errors.Is(err, ErrSubscriptionStateChanged) {
				t.Fatalf("stale expiry = %v", err)
			}
			fresh, err := store.GetSubscription(id)
			if err != nil {
				t.Fatalf("renewed subscription archived: %v", err)
			}
			if err := store.ArchiveExpiredSubscription(fresh, now, nil); !errors.Is(err, ErrSubscriptionStateChanged) {
				t.Fatalf("unexpired subscription archive = %v", err)
			}
			if err := store.ArchiveExpiredSubscription(fresh, now.AddDate(0, 0, 30), nil); err != nil {
				t.Fatalf("actual expiry archive = %v", err)
			}
		})
	}
}

func TestZeroReceiptRejectsPositiveRefundAtEditAndCompletion(t *testing.T) {
	store, id, _ := reviewSubscription(t, model.SubscriptionBusinessPlus, 0)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, cycle.Location)
	item, err := store.RequestSubscriptionCancellation(id, now, now.Add(24*time.Hour), 30)
	if err != nil || item.BillID == 0 {
		t.Fatalf("case = %#v, %v", item, err)
	}
	if err := store.UpdateAfterSalesCase(item.ID, 1, "invalid refund"); !errors.Is(err, ErrAfterSalesRefundExceedsPayment) {
		t.Fatalf("positive refund edit = %v", err)
	}
	// Legacy data may already contain an invalid amount. Completion rechecks it.
	if _, err := store.database.Exec(`UPDATE after_sales_cases SET refund_amount_cents = 1 WHERE id = ?`, item.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAfterSalesCaseRefunded(item.ID, true, now, now); !errors.Is(err, ErrAfterSalesRefundExceedsPayment) {
		t.Fatalf("positive refund completion = %v", err)
	}
	if err := store.UpdateAfterSalesCase(item.ID, 0, "no payment received"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAfterSalesCaseRefunded(item.ID, true, now, now); err != nil {
		t.Fatalf("zero refund completion = %v", err)
	}
}

func TestPrepaidPeriodsBlockIncompleteAfterSalesAtomically(t *testing.T) {
	for _, business := range []string{model.SubscriptionBusinessTeam, model.SubscriptionBusinessPlus} {
		t.Run(business, func(t *testing.T) {
			store, id, accountID := reviewSubscription(t, business, 10000)
			for _, date := range []string{"2026-08-31", "2026-09-30"} {
				subscription, err := store.GetSubscription(id)
				if err != nil {
					t.Fatal(err)
				}
				if err := store.SetDuePaidForSubscription(subscription, date, true, 10000); err != nil {
					t.Fatal(err)
				}
			}
			now := time.Date(2026, 8, 10, 12, 0, 0, 0, cycle.Location)
			if _, err := store.RequestSubscriptionCancellation(id, now, now.Add(24*time.Hour), 30); !errors.Is(err, ErrAfterSalesHasPrepaidPeriods) {
				t.Fatalf("prepaid cancellation = %v", err)
			}
			if accountID != 0 {
				if _, err := store.BanAccountAndCreateAfterSalesCases(accountID, "2026-08-10", "ban", 30); !errors.Is(err, ErrAfterSalesHasPrepaidPeriods) {
					t.Fatalf("prepaid ban = %v", err)
				}
				account, err := store.GetAccount(accountID)
				if err != nil || account.BannedAt != "" {
					t.Fatalf("failed ban changed account = %#v, %v", account, err)
				}
			}
			subscription, err := store.GetSubscription(id)
			if err != nil || subscription.CancellationCaseID != 0 {
				t.Fatalf("failed cancellation changed subscription = %#v, %v", subscription, err)
			}
			cases, err := store.ListAfterSalesCases()
			if err != nil || len(cases) != 0 {
				t.Fatalf("partial after-sales cases = %#v, %v", cases, err)
			}
		})
	}
}
