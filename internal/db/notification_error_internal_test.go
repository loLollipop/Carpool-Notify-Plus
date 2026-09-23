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
	failureResult, err := store.database.Exec(`
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
	)
	if err != nil {
		t.Fatal(err)
	}
	failureID, err := failureResult.LastInsertId()
	if err != nil {
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
	summary, err := store.GetUnresolvedNotificationFailureSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Count != 1 || summary.LatestFailureID != failureID {
		t.Fatalf("unresolved summary = %#v, want the nanosecond-newer failure", summary)
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
	summary, err = store.GetUnresolvedNotificationFailureSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Count != 0 || summary.LatestFailureID != 0 {
		t.Fatalf("unresolved summary after newer success = %#v, want empty", summary)
	}
}

func TestUnresolvedNotificationFailureSummaryGroupsBySubscriptionAndChannel(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "notification-error-summary.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	insertSubscription := func(name string) int64 {
		t.Helper()
		result, insertErr := store.database.Exec(`
			INSERT INTO subscriptions (
				name, price_per_person_cents, cron_expr, notify_offsets, channels,
				created_at, updated_at
			) VALUES (?, 3500, 'interval:30d', '[3]', '["smtp","iyuu"]', ?, ?)`,
			name,
			"2026-09-15T12:00:00Z",
			"2026-09-15T12:00:00Z",
		)
		if insertErr != nil {
			t.Fatal(insertErr)
		}
		id, insertErr := result.LastInsertId()
		if insertErr != nil {
			t.Fatal(insertErr)
		}
		return id
	}
	insertOutcome := func(subscriptionID int64, dueDate, channel, status, lastError, kind, updatedAt string) int64 {
		t.Helper()
		result, insertErr := store.database.Exec(`
			INSERT INTO notification_log (
				subscription_id, due_date, offset_days, channel, status, attempt_count,
				next_retry_at, last_error, kind, created_at, updated_at
			) VALUES (?, ?, 3, ?, ?, 1, NULL, ?, ?, ?, ?)`,
			subscriptionID, dueDate, channel, status, lastError, kind, updatedAt, updatedAt,
		)
		if insertErr != nil {
			t.Fatal(insertErr)
		}
		id, insertErr := result.LastInsertId()
		if insertErr != nil {
			t.Fatal(insertErr)
		}
		return id
	}

	firstSubscriptionID := insertSubscription("first")
	secondSubscriptionID := insertSubscription("second")
	insertOutcome(
		firstSubscriptionID, "2026-09-18", model.ChannelSMTP,
		model.NotificationStatusFailed, "smtp failed", model.NotificationKindScheduled,
		"2026-09-15T12:00:00.100000Z",
	)
	insertOutcome(
		firstSubscriptionID, "manual-recovery", model.ChannelSMTP,
		model.NotificationStatusSuccess, "", model.NotificationKindManualCustomerEmail,
		"2026-09-15T12:00:00.200000Z",
	)
	iyuuFailureID := insertOutcome(
		firstSubscriptionID, "2026-09-18", model.ChannelIYUU,
		model.NotificationStatusFailed, "iyuu failed", model.NotificationKindScheduled,
		"2026-09-15T12:00:00.150000Z",
	)
	insertOutcome(
		firstSubscriptionID, "2026-10-18", model.ChannelIYUU,
		model.NotificationStatusPending, "", model.NotificationKindScheduled,
		"2026-09-15T12:00:00.300000Z",
	)
	insertOutcome(
		firstSubscriptionID, "2026-11-18", model.ChannelIYUU,
		model.NotificationStatusCanceled, "", model.NotificationKindScheduled,
		"2026-09-15T12:00:00.400000Z",
	)
	secondFailureID := insertOutcome(
		secondSubscriptionID, "2026-09-18", model.ChannelSMTP,
		model.NotificationStatusFailed, "second failed", model.NotificationKindPriceIncreaseNotice,
		"2026-09-15T12:00:00.250000Z",
	)

	summary, err := store.GetUnresolvedNotificationFailureSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Count != 2 || summary.LatestFailureID != secondFailureID {
		t.Fatalf("unresolved summary = %#v, want two channel failures and latest ID %d (IYUU ID %d)", summary, secondFailureID, iyuuFailureID)
	}

	insertOutcome(
		secondSubscriptionID, "manual-recovery", model.ChannelSMTP,
		model.NotificationStatusSuccess, "", model.NotificationKindManualCustomerEmail,
		"2026-09-15T12:00:00.260000Z",
	)
	summary, err = store.GetUnresolvedNotificationFailureSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.Count != 1 || summary.LatestFailureID != iyuuFailureID {
		t.Fatalf("summary after one channel recovers = %#v, want only IYUU failure %d", summary, iyuuFailureID)
	}
}
