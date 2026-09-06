package db

import (
	"database/sql"
	"fmt"
)

const sqliteBusyTimeoutMilliseconds = 5000

// configureSQLite applies connection-level safety and concurrency settings.
// The application deliberately keeps one pooled connection, so these PRAGMAs
// are guaranteed to cover every query made through the store.
func configureSQLite(database *sql.DB) error {
	if _, err := database.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		return fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := database.Exec(
		fmt.Sprintf(`PRAGMA busy_timeout = %d;`, sqliteBusyTimeoutMilliseconds),
	); err != nil {
		return fmt.Errorf("set sqlite busy timeout: %w", err)
	}
	// WAL lets external backup/read connections coexist with the running
	// process. In-memory test databases may report "memory" instead of "wal";
	// that is a valid SQLite behavior, so the returned mode is informational.
	var journalMode string
	if err := database.QueryRow(`PRAGMA journal_mode = WAL;`).Scan(&journalMode); err != nil {
		return fmt.Errorf("enable sqlite WAL: %w", err)
	}
	if _, err := database.Exec(`PRAGMA synchronous = NORMAL;`); err != nil {
		return fmt.Errorf("set sqlite synchronous mode: %w", err)
	}
	return nil
}

// ensurePerformanceIndexes adds indexes for the operational hot paths. It is
// called after legacy columns have been migrated, which keeps upgrades from
// trying to index a column that does not exist yet.
func (store *Store) ensurePerformanceIndexes() error {
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_seats_account_id
			ON seats(account_id, id);`,
		`CREATE INDEX IF NOT EXISTS idx_subscriptions_seat_state
			ON subscriptions(seat_id, deleted_at, archived_at, id);`,
		`CREATE INDEX IF NOT EXISTS idx_subscriptions_active
			ON subscriptions(id)
			WHERE deleted_at IS NULL AND archived_at IS NULL;`,
		`CREATE INDEX IF NOT EXISTS idx_subscriptions_archived
			ON subscriptions(archived_at DESC, id DESC)
			WHERE deleted_at IS NULL AND archived_at IS NOT NULL;`,
		`CREATE INDEX IF NOT EXISTS idx_bills_due_date
			ON bills(due_date, subscription_id, id);`,
		`CREATE INDEX IF NOT EXISTS idx_bills_paid_at
			ON bills(paid_at DESC, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_notification_retry
			ON notification_log(status, next_retry_at, id)
			WHERE status = 'pending';`,
		`CREATE INDEX IF NOT EXISTS idx_notification_activity
			ON notification_log(updated_at DESC, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_after_sales_subscription_status
			ON after_sales_cases(subscription_id, status, id);`,
		`CREATE INDEX IF NOT EXISTS idx_pricing_exemptions_review
			ON pricing_exemptions(subscription_id, review_after, id);`,
	}
	for _, statement := range statements {
		if _, err := store.database.Exec(statement); err != nil {
			return fmt.Errorf("create performance index: %w", err)
		}
	}
	return nil
}
