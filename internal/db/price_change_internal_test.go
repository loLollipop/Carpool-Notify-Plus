package db

import (
	"path/filepath"
	"testing"
)

func TestBackfillSubscriptionPriceChangesFromLegacyBills(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "price-change-backfill.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	result, err := store.database.Exec(`
		INSERT INTO subscriptions (
			name, business_type, price_per_person_cents, cron_expr,
			notify_offsets, channels, created_at, updated_at
		) VALUES ('legacy-price-customer', 'team', 9500, 'interval:30d', '[]', '[]', ?, ?)`,
		"2026-01-01T00:00:00Z",
		"2026-01-01T00:00:00Z",
	)
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	for _, bill := range []struct {
		dueDate string
		amount  int64
	}{
		{dueDate: "2026-01-01", amount: 9000},
		{dueDate: "2026-01-31", amount: 9500},
		{dueDate: "2026-03-02", amount: 9200},
	} {
		if _, err := store.database.Exec(`
			INSERT INTO bills (
				subscription_id, due_date, amount_cents, cost_cents,
				note, paid_at, created_at, updated_at
			) VALUES (?, ?, ?, 0, '', ?, ?, ?)`,
			subscriptionID,
			bill.dueDate,
			bill.amount,
			bill.dueDate+"T00:00:00Z",
			bill.dueDate+"T00:00:00Z",
			bill.dueDate+"T00:00:00Z",
		); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.backfillSubscriptionPriceChanges(); err != nil {
		t.Fatal(err)
	}
	if err := store.backfillSubscriptionPriceChanges(); err != nil {
		t.Fatalf("idempotent backfill: %v", err)
	}
	changes, err := store.ListSubscriptionPriceChanges()
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 ||
		changes[0].PreviousPriceCents != 9000 || changes[0].NewPriceCents != 9500 ||
		changes[0].EffectiveDueDate != "2026-01-31" ||
		changes[1].PreviousPriceCents != 9500 || changes[1].NewPriceCents != 9200 ||
		changes[1].EffectiveDueDate != "2026-03-02" {
		t.Fatalf("backfilled price changes = %#v", changes)
	}
}
