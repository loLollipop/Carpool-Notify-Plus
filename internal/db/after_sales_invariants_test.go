package db

import (
	"errors"
	"math"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"carpool-notify/internal/model"
)

func TestAfterSalesRefundLimitIsEnforcedInsideWriteTransaction(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "refund-invariant.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	subscriptionID, err := store.CreateSubscriptionWithInitialBill(model.Subscription{
		Name:                "Plus customer",
		BusinessType:        model.SubscriptionBusinessPlus,
		PricePerPersonCents: 1000,
		CronExpr:            "interval:30d",
		NotifyOffsets:       []int{},
		BoardedAt:           "2026-08-01",
	}, "2026-08-01", 1000)
	if err != nil {
		t.Fatal(err)
	}
	bill, err := store.GetBillByOccurrence(subscriptionID, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	now := formatTime(time.Now().UTC())
	insertCase := func(status string, refundCents int64, bannedDate string) int64 {
		result, insertErr := store.database.Exec(`
			INSERT INTO after_sales_cases (
				subscription_id, bill_id, business_type, banned_date,
				paid_amount_cents, refund_amount_cents, status,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			subscriptionID,
			bill.ID,
			model.SubscriptionBusinessPlus,
			bannedDate,
			int64(1000),
			refundCents,
			status,
			now,
			now,
		)
		if insertErr != nil {
			t.Fatal(insertErr)
		}
		caseID, insertErr := result.LastInsertId()
		if insertErr != nil {
			t.Fatal(insertErr)
		}
		return caseID
	}

	completedID := insertCase(model.AfterSalesStatusRefunded, 800, "2026-08-10")
	pendingID := insertCase(model.AfterSalesStatusPending, 100, "2026-08-11")
	if err := store.UpdateAfterSalesCase(completedID, 700, "must stay frozen"); !errors.Is(err, ErrAfterSalesProcessed) {
		t.Fatalf("editing completed case error = %v, want ErrAfterSalesProcessed", err)
	}
	if err := store.UpdateAfterSalesCase(pendingID, 300, "too much"); !errors.Is(err, ErrAfterSalesRefundExceedsPayment) {
		t.Fatalf("over-refund update error = %v, want ErrAfterSalesRefundExceedsPayment", err)
	}
	if err := store.UpdateAfterSalesCase(pendingID, 200, "remaining amount"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAfterSalesCaseRefunded(pendingID, true, time.Now()); err != nil {
		t.Fatalf("completing exact remaining refund: %v", err)
	}
}

func TestProratedRefundDoesNotOverflowLargeAmounts(t *testing.T) {
	got := proratedRefundCents(math.MaxInt64, 29, 30)
	wantBig := new(big.Int).Mul(big.NewInt(math.MaxInt64), big.NewInt(29))
	wantBig.Add(wantBig, big.NewInt(15))
	wantBig.Div(wantBig, big.NewInt(30))
	if !wantBig.IsInt64() || got != wantBig.Int64() {
		t.Fatalf("prorated refund = %d, want %s", got, wantBig.String())
	}
}
