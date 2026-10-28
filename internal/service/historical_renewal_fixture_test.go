package service

import (
	"database/sql"
	"path/filepath"
	"testing"

	"carpool-notify/internal/db"
)

func openExtensionHistoryService(t *testing.T) (*SubscriptionService, *sql.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "extension-history.db")
	store, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	history, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = history.Close() })
	return &SubscriptionService{Store: store, MarketClient: failingMarketClient{}}, history
}

func seedPendingExtensionRenewal(t *testing.T, history *sql.DB, subscriptionID int64, dueDate string) {
	t.Helper()
	_, err := history.Exec(`INSERT INTO renewal_applications (
        tracking_token, subscription_id, customer_email, due_date, period_count, period_end_date,
        amount_cents, status, created_at, updated_at
    ) VALUES ('historical-extension', ?, 'extension@example.com', ?, 1, '', 10000, 'pending', '2026-06-20T00:00:00Z', '2026-06-20T00:00:00Z')`, subscriptionID, dueDate)
	if err != nil {
		t.Fatal(err)
	}
}
