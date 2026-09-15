package db

import (
	"path/filepath"
	"testing"

	"carpool-notify/internal/model"
)

func TestLatestErrorsBySubscriptionUsesNanosecondOutcomeOrder(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "notification-error-order.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	result, err := store.database.Exec(`
		INSERT INTO subscriptions (
			name, price_per_person_cents, cron_expr, notify_offsets, channels,
			created_at, updated_at
		) VALUES (?, 3500, 'interval:30d', '[3]', '["smtp"]', ?, ?)`,
		"nanosecond outcome order",
		"2026-09-15T12:00:00Z",
		"2026-09-15T12:00:00Z",
	)
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	// The scheduled row was created first but failed after the manual success.
	// Both outcomes are deliberately within one millisecond: julianday() cannot
	// reliably distinguish them, and an id tie-breaker would choose success.
	if _, err := store.database.Exec(`
		INSERT INTO notification_log (
			subscription_id, due_date, offset_days, channel, status, attempt_count,
			next_retry_at, last_error, kind, created_at, updated_at
		) VALUES (?, '2026-09-18', 3, ?, ?, 5, NULL, 'retry failed', ?, ?, ?)`,
		subscriptionID,
		model.ChannelSMTP,
		model.NotificationStatusFailed,
		model.NotificationKindScheduled,
		"2026-09-15T12:00:00.100000Z",
		"2026-09-15T12:00:00.100200Z",
	); err != nil {
		t.Fatal(err)
	}
	manualResult, err := store.database.Exec(`
		INSERT INTO notification_log (
			subscription_id, due_date, offset_days, channel, status, attempt_count,
			next_retry_at, last_error, kind, created_at, updated_at
		) VALUES (?, 'manual-order-test', 0, ?, ?, 1, NULL, '', ?, ?, ?)`,
		subscriptionID,
		model.ChannelSMTP,
		model.NotificationStatusSuccess,
		model.NotificationKindManualCustomerEmail,
		"2026-09-15T12:00:00.100100Z",
		"2026-09-15T12:00:00.100100Z",
	)
	if err != nil {
		t.Fatal(err)
	}
	manualID, err := manualResult.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	errorsBySubscription, err := store.LatestErrorsBySubscription()
	if err != nil {
		t.Fatal(err)
	}
	if got := errorsBySubscription[subscriptionID]; got != "retry failed" {
		t.Fatalf("latest error = %q, want retry failure after manual success", got)
	}

	// A genuinely newer manual success must then resolve the SMTP warning.
	if _, err := store.database.Exec(`
		UPDATE notification_log SET updated_at = ? WHERE id = ?`,
		"2026-09-15T12:00:00.100300Z",
		manualID,
	); err != nil {
		t.Fatal(err)
	}
	errorsBySubscription, err = store.LatestErrorsBySubscription()
	if err != nil {
		t.Fatal(err)
	}
	if got := errorsBySubscription[subscriptionID]; got != "" {
		t.Fatalf("latest error after newer manual success = %q, want cleared warning", got)
	}
}
