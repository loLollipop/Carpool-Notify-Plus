package db

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenConfiguresSQLiteAndCreatesPerformanceIndexes(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "performance.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var foreignKeys int
	if err := store.database.QueryRow(`PRAGMA foreign_keys;`).Scan(&foreignKeys); err != nil {
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}

	var busyTimeout int
	if err := store.database.QueryRow(`PRAGMA busy_timeout;`).Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout != sqliteBusyTimeoutMilliseconds {
		t.Fatalf("busy_timeout = %d, want %d", busyTimeout, sqliteBusyTimeoutMilliseconds)
	}

	var journalMode string
	if err := store.database.QueryRow(`PRAGMA journal_mode;`).Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	wantedIndexes := []string{
		"idx_seats_account_id",
		"idx_subscriptions_seat_state",
		"idx_subscriptions_active",
		"idx_subscriptions_archived",
		"idx_bills_due_date",
		"idx_bills_paid_at",
		"idx_notification_retry",
		"idx_notification_activity",
		"idx_after_sales_subscription_status",
		"idx_pricing_exemptions_review",
	}
	for _, name := range wantedIndexes {
		var count int
		if err := store.database.QueryRow(
			`SELECT COUNT(1) FROM sqlite_master WHERE type = 'index' AND name = ?`,
			name,
		).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("index %s count = %d, want 1", name, count)
		}
	}
}

func TestNextWriteTimeAdvancesPastPreviousVersion(t *testing.T) {
	previous := time.Now().UTC().Add(time.Hour).Truncate(time.Nanosecond)
	parsed, err := parseTime(nextWriteTime(previous))
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.After(previous) {
		t.Fatalf("next write time %s did not advance past %s", parsed, previous)
	}
}
