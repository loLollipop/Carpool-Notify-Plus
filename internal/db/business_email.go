package db

import (
	"database/sql"
	"time"
)

// BusinessEmail is a committed notification waiting for SMTP delivery.
type BusinessEmail struct {
	ID                     int64
	SubscriptionID         int64
	Recipient, Title, Body string
	AttemptCount           int
}

// Build is called after the mutation has computed its final facts, before
// commit. It must use those in-memory facts and must not query the Store.
type BusinessEmailBatch struct {
	Build func() []BusinessEmail
	Now   time.Time
}

func queueBusinessEmailBatches(executor sqlExecer, batches ...BusinessEmailBatch) error {
	for _, batch := range batches {
		if batch.Build == nil {
			continue
		}
		stamp := businessEmailTime(batch.Now)
		for _, message := range batch.Build() {
			if _, err := executor.Exec(`INSERT INTO business_email_outbox
				(subscription_id, recipient, title, body, next_retry_at, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, ?)`, message.SubscriptionID, message.Recipient,
				message.Title, message.Body, stamp, stamp, stamp); err != nil {
				return err
			}
		}
	}
	return nil
}

func (store *Store) ensureBusinessEmailOutbox() error {
	_, err := store.database.Exec(`CREATE TABLE IF NOT EXISTS business_email_outbox (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		subscription_id INTEGER NOT NULL,
		recipient TEXT NOT NULL,
		title TEXT NOT NULL,
		body TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		attempt_count INTEGER NOT NULL DEFAULT 0,
		next_retry_at TEXT NOT NULL,
		last_error TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	); CREATE INDEX IF NOT EXISTS idx_business_email_pending
		ON business_email_outbox(status, subscription_id, id)`)
	return err
}

func (store *Store) QueueBusinessEmails(messages []BusinessEmail, now time.Time) error {
	if len(messages) == 0 {
		return nil
	}
	tx, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := queueBusinessEmailBatches(tx, BusinessEmailBatch{Build: func() []BusinessEmail { return messages }, Now: now}); err != nil {
		return err
	}
	return tx.Commit()
}

// PendingBusinessEmails selects the oldest message per subscription. A later
// change must not overtake an older send, including one waiting for a retry.
func (store *Store) PendingBusinessEmails(now time.Time, limit int) ([]BusinessEmail, error) {
	rows, err := store.database.Query(`SELECT current.id, current.subscription_id,
		current.recipient, current.title, current.body, current.attempt_count
		FROM business_email_outbox AS current
		WHERE current.status = 'pending' AND current.next_retry_at <= ?
		AND NOT EXISTS (SELECT 1 FROM business_email_outbox AS previous
			WHERE previous.subscription_id = current.subscription_id
			AND previous.status = 'pending' AND previous.id < current.id)
		ORDER BY current.id LIMIT ?`, businessEmailTime(now), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []BusinessEmail
	for rows.Next() {
		var message BusinessEmail
		if err := rows.Scan(&message.ID, &message.SubscriptionID, &message.Recipient,
			&message.Title, &message.Body, &message.AttemptCount); err != nil {
			return nil, err
		}
		result = append(result, message)
	}
	return result, rows.Err()
}

func (store *Store) FinishBusinessEmail(id int64, attempt int, sendErr error, now time.Time) error {
	status, lastError := "success", ""
	if sendErr != nil {
		status, lastError = "pending", sendErr.Error()
	}
	delay := time.Minute * time.Duration(1<<min(max(attempt-1, 0), 6))
	result, err := store.database.Exec(`UPDATE business_email_outbox
		SET status = ?, attempt_count = ?, last_error = ?, next_retry_at = ?, updated_at = ?
		WHERE id = ? AND status = 'pending'`, status, attempt, lastError,
		businessEmailTime(now.Add(delay)), businessEmailTime(now), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return sql.ErrNoRows
	}
	return err
}

// Fixed precision makes lexical SQLite comparisons chronological even when
// the test clock or an OS timestamp has zero fractional seconds.
func businessEmailTime(moment time.Time) string {
	return moment.UTC().Format("2006-01-02T15:04:05.000000000Z")
}
