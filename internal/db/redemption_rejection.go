package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"carpool-notify/internal/model"
)

// RejectRedemptionApplication rejects a pending application and atomically
// releases its one-time redemption code so the customer can submit again.
func (store *Store) RejectRedemptionApplication(applicationID int64, operatorNote string) error {
	now := formatTime(time.Now().UTC())
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = transaction.Rollback()
	}()

	result, err := transaction.Exec(`
		UPDATE redemption_applications
		SET status = ?,
			assigned_account_id = 0,
			assigned_seat_id = 0,
			assigned_subscription_id = 0,
			operator_note = ?,
			invited_at = NULL,
			updated_at = ?
		WHERE id = ? AND status = ?`,
		model.RedemptionStatusRejected,
		strings.TrimSpace(operatorNote),
		now,
		applicationID,
		model.RedemptionStatusPending,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	result, err = transaction.Exec(`
		UPDATE redemption_codes
		SET status = ?,
			used_by_application_id = 0,
			used_at = NULL,
			updated_at = ?
		WHERE used_by_application_id = ? AND status = ?`,
		model.RedemptionCodeStatusUnused,
		now,
		applicationID,
		model.RedemptionCodeStatusUsed,
	)
	if err != nil {
		return err
	}
	rowsAffected, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected != 1 {
		return fmt.Errorf("release redemption code for application %d: %w", applicationID, ErrRedemptionCodeNotUnused)
	}

	return transaction.Commit()
}
