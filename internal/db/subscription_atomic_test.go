package db

import (
	"path/filepath"
	"testing"

	"carpool-notify/internal/model"
)

func TestUpdateSubscriptionAndSyncBillRollsBackTogether(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "atomic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	subscription := model.Subscription{
		Name:                "Plus customer",
		BusinessType:        model.SubscriptionBusinessPlus,
		PricePerPersonCents: 3000,
		CostCents:           1200,
		CronExpr:            "interval:30d",
		NotifyOffsets:       []int{},
		BoardedAt:           "2026-08-01",
	}
	subscriptionID, err := store.CreateSubscriptionWithInitialBill(
		subscription,
		"2026-08-01",
		3000,
	)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	stored.PricePerPersonCents = 3500
	stored.CostCents = 1400

	if _, err := store.database.Exec(`
		CREATE TRIGGER reject_bill_financial_update
		BEFORE UPDATE OF amount_cents, cost_cents ON bills
		BEGIN
			SELECT RAISE(ABORT, 'injected bill failure');
		END;`); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSubscriptionAndSyncBill(
		stored,
		"2026-08-01",
		3500,
		1400,
	); err == nil {
		t.Fatal("update succeeded despite injected bill failure")
	}

	afterSubscription, err := store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if afterSubscription.PricePerPersonCents != 3000 || afterSubscription.CostCents != 1200 {
		t.Fatalf(
			"subscription financials changed after rollback: price=%d cost=%d",
			afterSubscription.PricePerPersonCents,
			afterSubscription.CostCents,
		)
	}
	afterBill, err := store.GetBillByOccurrence(subscriptionID, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	if afterBill.AmountCents != 3000 || afterBill.CostCents != 1200 {
		t.Fatalf(
			"bill financials changed after rollback: amount=%d cost=%d",
			afterBill.AmountCents,
			afterBill.CostCents,
		)
	}
}

func TestSetSettingsRollsBackWholePageOnFailure(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "settings-atomic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.SetSettings(map[string]string{
		"first":  "old-first",
		"second": "old-second",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.database.Exec(`
		CREATE TRIGGER reject_setting_value
		BEFORE UPDATE ON settings
		WHEN NEW.value = 'reject'
		BEGIN
			SELECT RAISE(ABORT, 'injected setting failure');
		END;`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSettings(map[string]string{
		"first":  "new-first",
		"second": "reject",
	}); err == nil {
		t.Fatal("settings update succeeded despite injected failure")
	}
	for key, want := range map[string]string{
		"first":  "old-first",
		"second": "old-second",
	} {
		got, err := store.GetSetting(key)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("setting %q = %q, want %q", key, got, want)
		}
	}
}
