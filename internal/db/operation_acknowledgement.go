package db

import (
	"strings"
	"time"
)

// ListAcknowledgedOperationTaskIDs returns the persisted acknowledgement set.
// The service intersects it with current tasks in memory, avoiding SQLite
// placeholder limits when many subscriptions are due.
func (store *Store) ListAcknowledgedOperationTaskIDs() (map[string]struct{}, error) {
	rows, err := store.database.Query(`SELECT task_id FROM operation_acknowledgements`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]struct{})
	for rows.Next() {
		var taskID string
		if err := rows.Scan(&taskID); err != nil {
			return nil, err
		}
		result[taskID] = struct{}{}
	}
	return result, rows.Err()
}

// AcknowledgeOperationTasks records a batch atomically. Task IDs are stable for
// one business occurrence, so the next due date naturally becomes unread.
func (store *Store) AcknowledgeOperationTasks(taskIDs []string, acknowledgedAt time.Time) error {
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	timestamp := formatTime(acknowledgedAt.UTC())
	for _, rawTaskID := range taskIDs {
		taskID := strings.TrimSpace(rawTaskID)
		if taskID == "" {
			continue
		}
		if _, err := transaction.Exec(`
			INSERT INTO operation_acknowledgements(task_id, acknowledged_at)
			VALUES (?, ?)
			ON CONFLICT(task_id) DO UPDATE SET acknowledged_at = excluded.acknowledged_at`,
			taskID,
			timestamp,
		); err != nil {
			return err
		}
	}

	// Old occurrence IDs are never reused. Retaining a little over a year is
	// sufficient for monthly and annual views while keeping tiny servers tidy.
	cutoff := formatTime(acknowledgedAt.AddDate(-2, 0, 0).UTC())
	if _, err := transaction.Exec(
		`DELETE FROM operation_acknowledgements WHERE acknowledged_at < ?`,
		cutoff,
	); err != nil {
		return err
	}
	return transaction.Commit()
}
