package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

const afterSalesSelectColumns = `
	id,
	COALESCE(account_id, 0),
	subscription_id,
	bill_id,
	COALESCE(business_type, 'team'),
	account_name,
	account_email,
	account_space_name,
	customer_email,
	customer_wechat,
	period_start,
	period_end,
	banned_date,
	warranty_days,
	used_days,
	remaining_days,
	paid_amount_cents,
	refund_amount_cents,
	replacement_account_id,
	replacement_seat_id,
	replacement_account_name,
	replacement_account_email,
	replacement_space_name,
	replacement_seat_name,
	source,
	expires_at,
	status,
	note,
	processed_at,
	created_at,
	updated_at`

// BanAccountAndCreateAfterSalesCases atomically marks an account as banned and
// snapshots one refund case for every active customer currently occupying it.
func (store *Store) BanAccountAndCreateAfterSalesCases(
	accountID int64,
	bannedDate string,
	banNote string,
	warrantyDays int,
) (int, error) {
	transaction, err := store.database.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = transaction.Rollback() }()

	var accountName string
	var accountEmail string
	var accountSpaceName string
	var existingBannedAt string
	if err := transaction.QueryRow(`
		SELECT COALESCE(name, ''), COALESCE(email, ''), COALESCE(space_name, ''),
		       COALESCE(banned_at, '')
		FROM accounts
		WHERE id = ?`, accountID).Scan(
		&accountName,
		&accountEmail,
		&accountSpaceName,
		&existingBannedAt,
	); err != nil {
		return 0, err
	}

	// The first ban date is the refund event date. Repeated requests reuse it so
	// the unique key remains stable and cannot create duplicate refund cases.
	if strings.TrimSpace(existingBannedAt) != "" {
		bannedDate = existingBannedAt
	} else {
		if _, err := transaction.Exec(`
			UPDATE accounts
			SET banned_at = ?, ban_note = ?, updated_at = ?
			WHERE id = ?`,
			bannedDate,
			strings.TrimSpace(banNote),
			formatTime(time.Now().UTC()),
			accountID,
		); err != nil {
			return 0, err
		}
	}

	type affectedSubscription struct {
		ID                 int64
		CustomerEmail      string
		CustomerWechat     string
		CancellationCaseID int64
	}
	rows, err := transaction.Query(`
		SELECT subscription.id,
		       COALESCE(subscription.customer_email, ''),
		       COALESCE(subscription.customer_wechat, ''),
		       COALESCE(subscription.cancellation_case_id, 0)
		FROM subscriptions AS subscription
		INNER JOIN seats AS seat ON seat.id = subscription.seat_id
		WHERE seat.account_id = ?
		  AND subscription.deleted_at IS NULL
		  AND subscription.archived_at IS NULL
		ORDER BY subscription.id ASC`, accountID)
	if err != nil {
		return 0, err
	}
	affected := make([]affectedSubscription, 0)
	for rows.Next() {
		var subscription affectedSubscription
		if err := rows.Scan(
			&subscription.ID,
			&subscription.CustomerEmail,
			&subscription.CustomerWechat,
			&subscription.CancellationCaseID,
		); err != nil {
			_ = rows.Close()
			return 0, err
		}
		affected = append(affected, subscription)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	bannedDay, err := time.ParseInLocation("2006-01-02", bannedDate, cycle.Location)
	if err != nil {
		return 0, fmt.Errorf("parse banned date: %w", err)
	}
	now := formatTime(time.Now().UTC())
	insertedCount := 0
	for _, subscription := range affected {
		// Account-ban handling supersedes a still-pending customer cancellation.
		// Keeping both cases lets the cancellation expire or complete first and
		// strands the account-ban case without an active subscription.
		if subscription.CancellationCaseID > 0 {
			if _, err := transaction.Exec(`
				DELETE FROM after_sales_cases
				WHERE id = ? AND source = ? AND status IN (?, ?)`,
				subscription.CancellationCaseID,
				model.AfterSalesSourceCustomerCancellation,
				model.AfterSalesStatusPending,
				model.AfterSalesStatusReview,
			); err != nil {
				return 0, err
			}
			if _, err := transaction.Exec(`
				UPDATE subscriptions
				SET cancellation_requested_at = NULL,
				    cancellation_expires_at = NULL,
				    cancellation_case_id = 0,
				    updated_at = ?
				WHERE id = ? AND cancellation_case_id = ?`,
				now,
				subscription.ID,
				subscription.CancellationCaseID,
			); err != nil {
				return 0, err
			}
		}

		billID := int64(0)
		periodStart := ""
		periodEnd := ""
		paidAmountCents := int64(0)
		refundAmountCents := int64(0)
		usedDays := 0
		remainingDays := 0
		status := model.AfterSalesStatusReview
		caseNote := "未找到封禁日前的已缴账单，请人工核对退款金额"

		err := transaction.QueryRow(`
			SELECT id, due_date, amount_cents
			FROM bills
			WHERE subscription_id = ? AND due_date <= ?
			ORDER BY due_date DESC, id DESC
			LIMIT 1`, subscription.ID, bannedDate).Scan(
			&billID,
			&periodStart,
			&paidAmountCents,
		)
		if err != nil && err != sql.ErrNoRows {
			return 0, err
		}
		if err == nil {
			periodDay, parseErr := time.ParseInLocation("2006-01-02", periodStart, cycle.Location)
			if parseErr != nil {
				return 0, fmt.Errorf("parse bill due date: %w", parseErr)
			}
			usedDays = int(bannedDay.Sub(periodDay).Hours() / 24)
			if usedDays < 0 {
				usedDays = 0
			}
			if usedDays > warrantyDays {
				usedDays = warrantyDays
			}
			remainingDays = warrantyDays - usedDays
			periodEnd = periodDay.AddDate(0, 0, warrantyDays).Format("2006-01-02")
			// Integer-cent half-up rounding keeps the result deterministic.
			refundAmountCents = proratedRefundCents(paidAmountCents, remainingDays, warrantyDays)
			status = model.AfterSalesStatusPending
			caseNote = ""
		}

		result, err := transaction.Exec(`
			INSERT OR IGNORE INTO after_sales_cases (
				account_id, subscription_id, bill_id,
				account_name, account_email, account_space_name, customer_email, customer_wechat,
				period_start, period_end, banned_date, warranty_days,
				used_days, remaining_days, paid_amount_cents, refund_amount_cents,
				status, note, processed_at, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
			accountID,
			subscription.ID,
			billID,
			strings.TrimSpace(accountName),
			strings.TrimSpace(accountEmail),
			strings.TrimSpace(accountSpaceName),
			strings.TrimSpace(subscription.CustomerEmail),
			strings.TrimSpace(subscription.CustomerWechat),
			periodStart,
			periodEnd,
			bannedDate,
			warrantyDays,
			usedDays,
			remainingDays,
			paidAmountCents,
			refundAmountCents,
			status,
			caseNote,
			now,
			now,
		)
		if err != nil {
			return 0, err
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		insertedCount += int(rowsAffected)
	}

	if err := transaction.Commit(); err != nil {
		return 0, err
	}
	return insertedCount, nil
}

// ListAfterSalesCases returns newest ban events first.
func (store *Store) ListAfterSalesCases() ([]model.AfterSalesCase, error) {
	return store.listAfterSalesCases("", nil)
}

// ListVisibleAfterSalesCases keeps pending work visible indefinitely and hides
// completed cases after the caller-provided retention cutoff. The underlying
// rows remain available for refund accounting and historical bill totals.
func (store *Store) ListVisibleAfterSalesCases(processedAfter time.Time) ([]model.AfterSalesCase, error) {
	return store.listAfterSalesCases(`
		WHERE status IN (?, ?)
		   OR processed_at IS NULL
		   OR processed_at > ?`, []any{
		model.AfterSalesStatusPending,
		model.AfterSalesStatusReview,
		formatTime(processedAfter.UTC()),
	})
}

func (store *Store) listAfterSalesCases(whereClause string, arguments []any) ([]model.AfterSalesCase, error) {
	rows, err := store.database.Query(`
		SELECT `+afterSalesSelectColumns+`
		FROM after_sales_cases `+whereClause+`
		ORDER BY banned_date DESC, id DESC`, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cases := make([]model.AfterSalesCase, 0)
	for rows.Next() {
		caseItem, err := scanAfterSalesCase(rows)
		if err != nil {
			return nil, err
		}
		cases = append(cases, caseItem)
	}
	return cases, rows.Err()
}

func (store *Store) CountAfterSalesCasesByAccount(accountID int64) (int, error) {
	var count int
	err := store.database.QueryRow(
		`SELECT COUNT(1) FROM after_sales_cases WHERE account_id = ?`,
		accountID,
	).Scan(&count)
	return count, err
}

func (store *Store) CountPendingAfterSalesCasesByAccount(accountID int64) (int, error) {
	var count int
	err := store.database.QueryRow(`
		SELECT COUNT(1)
		FROM after_sales_cases
		WHERE account_id = ? AND status IN (?, ?)`,
		accountID,
		model.AfterSalesStatusPending,
		model.AfterSalesStatusReview,
	).Scan(&count)
	return count, err
}

func (store *Store) CountAfterSalesCasesBySubscription(subscriptionID int64) (int, error) {
	var count int
	err := store.database.QueryRow(
		`SELECT COUNT(1) FROM after_sales_cases WHERE subscription_id = ?`,
		subscriptionID,
	).Scan(&count)
	return count, err
}

func (store *Store) CountPendingAfterSalesCasesBySubscription(subscriptionID int64) (int, error) {
	var count int
	err := store.database.QueryRow(`
		SELECT COUNT(1)
		FROM after_sales_cases
		WHERE subscription_id = ? AND status IN (?, ?)`,
		subscriptionID,
		model.AfterSalesStatusPending,
		model.AfterSalesStatusReview,
	).Scan(&count)
	return count, err
}

// UpdateAfterSalesCase updates the operator-adjustable refund and note fields.
func (store *Store) UpdateAfterSalesCase(caseID int64, refundAmountCents int64, note string) error {
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	var status string
	if err := transaction.QueryRow(
		`SELECT status FROM after_sales_cases WHERE id = ?`,
		caseID,
	).Scan(&status); err != nil {
		return err
	}
	if status != model.AfterSalesStatusPending && status != model.AfterSalesStatusReview {
		return ErrAfterSalesProcessed
	}
	if err := ensureAfterSalesRefundWithinPayment(transaction, caseID, refundAmountCents); err != nil {
		return err
	}

	result, err := transaction.Exec(`
		UPDATE after_sales_cases
		SET refund_amount_cents = ?, note = ?, updated_at = ?
		WHERE id = ? AND status IN (?, ?)`,
		refundAmountCents,
		strings.TrimSpace(note),
		formatTime(time.Now().UTC()),
		caseID,
		model.AfterSalesStatusPending,
		model.AfterSalesStatusReview,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrAfterSalesProcessed
	}
	return transaction.Commit()
}

// SetAfterSalesCaseRefunded marks or unmarks one case as completed.
func (store *Store) SetAfterSalesCaseRefunded(
	caseID int64,
	refunded bool,
	processedTime time.Time,
	cancellationSeatFreezeUntil time.Time,
) error {
	var source string
	if err := store.database.QueryRow(
		`SELECT source FROM after_sales_cases WHERE id = ?`,
		caseID,
	).Scan(&source); err != nil {
		return err
	}
	if source == model.AfterSalesSourceCustomerCancellation {
		if !refunded {
			return ErrAfterSalesProcessed
		}
		return store.CompleteCancellationRefund(caseID, processedTime, cancellationSeatFreezeUntil)
	}

	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	var subscriptionID int64
	var billID int64
	if err := transaction.QueryRow(`
		SELECT subscription_id, bill_id
		FROM after_sales_cases
		WHERE id = ?`, caseID).Scan(&subscriptionID, &billID); err != nil {
		return err
	}
	if refunded {
		if err := ensureAfterSalesRefundWithinPayment(transaction, caseID, -1); err != nil {
			return err
		}
	}

	status := model.AfterSalesStatusPending
	processedAt := any(nil)
	allowedStatus := model.AfterSalesStatusRefunded
	allowedStatus2 := model.AfterSalesStatusRefunded
	if refunded {
		status = model.AfterSalesStatusRefunded
		processedAt = formatTime(processedTime.UTC())
		allowedStatus = model.AfterSalesStatusPending
		allowedStatus2 = model.AfterSalesStatusReview
	} else if billID == 0 {
		status = model.AfterSalesStatusReview
	}
	nowText := formatTime(processedTime.UTC())
	result, err := transaction.Exec(`
		UPDATE after_sales_cases
		SET status = ?, processed_at = ?, updated_at = ?
		WHERE id = ?
		  AND status IN (?, ?)`,
		status,
		processedAt,
		nowText,
		caseID,
		allowedStatus,
		allowedStatus2,
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

	if refunded {
		var cancellationCaseID int64
		if err := transaction.QueryRow(`
			SELECT COALESCE(cancellation_case_id, 0)
			FROM subscriptions
			WHERE id = ? AND deleted_at IS NULL AND archived_at IS NULL`,
			subscriptionID,
		).Scan(&cancellationCaseID); err != nil {
			return err
		}
		result, err = transaction.Exec(`
			UPDATE subscriptions
			SET archived_at = ?, cancellation_requested_at = NULL,
			    cancellation_expires_at = NULL, cancellation_case_id = 0,
			    seat_frozen_until = NULL,
			    updated_at = ?
			WHERE id = ? AND deleted_at IS NULL AND archived_at IS NULL`,
			nowText,
			nowText,
			subscriptionID,
		)
		if err != nil {
			return err
		}
		rowsAffected, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if rowsAffected != 1 {
			return sql.ErrNoRows
		}
		if cancellationCaseID > 0 && cancellationCaseID != caseID {
			if _, err := transaction.Exec(`
				DELETE FROM after_sales_cases
				WHERE id = ? AND source = ? AND status IN (?, ?)`,
				cancellationCaseID,
				model.AfterSalesSourceCustomerCancellation,
				model.AfterSalesStatusPending,
				model.AfterSalesStatusReview,
			); err != nil {
				return err
			}
		}
	} else {
		var seatID sql.NullInt64
		if err := transaction.QueryRow(`
			SELECT seat_id
			FROM subscriptions
			WHERE id = ? AND deleted_at IS NULL AND archived_at IS NOT NULL`,
			subscriptionID,
		).Scan(&seatID); err != nil {
			return err
		}
		if !seatID.Valid || seatID.Int64 <= 0 {
			return ErrAfterSalesOriginalSeatBusy
		}
		var occupiedCount int
		if err := transaction.QueryRow(`
			SELECT COUNT(1)
			FROM subscriptions
			WHERE seat_id = ? AND id <> ?
			  AND deleted_at IS NULL AND archived_at IS NULL`,
			seatID.Int64,
			subscriptionID,
		).Scan(&occupiedCount); err != nil {
			return err
		}
		if occupiedCount > 0 {
			return ErrAfterSalesOriginalSeatBusy
		}
		result, err = transaction.Exec(`
			UPDATE subscriptions
			SET archived_at = NULL, updated_at = ?
			WHERE id = ? AND deleted_at IS NULL AND archived_at IS NOT NULL`,
			nowText,
			subscriptionID,
		)
		if err != nil {
			return err
		}
		rowsAffected, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if rowsAffected != 1 {
			return sql.ErrNoRows
		}
	}

	return transaction.Commit()
}

// ensureAfterSalesRefundWithinPayment protects the financial invariant inside
// the same transaction that completes or edits a refund. proposedCents < 0
// means use the amount currently stored on the target case.
func ensureAfterSalesRefundWithinPayment(
	transaction *sql.Tx,
	caseID int64,
	proposedCents int64,
) error {
	var billID int64
	var paidAmountCents int64
	var storedRefundCents int64
	if err := transaction.QueryRow(`
		SELECT bill_id, paid_amount_cents, refund_amount_cents
		FROM after_sales_cases
		WHERE id = ?`, caseID).Scan(
		&billID,
		&paidAmountCents,
		&storedRefundCents,
	); err != nil {
		return err
	}
	if proposedCents < 0 {
		proposedCents = storedRefundCents
	}
	if proposedCents < 0 {
		return ErrAfterSalesRefundExceedsPayment
	}
	if billID <= 0 || paidAmountCents <= 0 {
		return nil
	}

	var completedRefundCents int64
	if err := transaction.QueryRow(`
		SELECT COALESCE(SUM(refund_amount_cents), 0)
		FROM after_sales_cases
		WHERE bill_id = ? AND id <> ? AND status = ?`,
		billID,
		caseID,
		model.AfterSalesStatusRefunded,
	).Scan(&completedRefundCents); err != nil {
		return err
	}
	if completedRefundCents > paidAmountCents ||
		proposedCents > paidAmountCents-completedRefundCents {
		return ErrAfterSalesRefundExceedsPayment
	}
	return nil
}

func proratedRefundCents(paidAmountCents int64, remainingDays int, periodDays int) int64 {
	if paidAmountCents <= 0 || remainingDays <= 0 || periodDays <= 0 {
		return 0
	}
	if remainingDays >= periodDays {
		return paidAmountCents
	}
	period := int64(periodDays)
	remaining := int64(remainingDays)
	whole := (paidAmountCents / period) * remaining
	remainder := paidAmountCents % period
	return whole + (remainder*remaining+period/2)/period
}

// ReassignAfterSalesCase moves an affected active subscription to one free seat
// and closes the case without creating a refund transaction.
func (store *Store) ReassignAfterSalesCase(
	caseID int64,
	replacementAccountID int64,
	replacementSeatID int64,
	processedTime time.Time,
) error {
	processedAt := formatTime(processedTime.UTC())
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	var subscriptionID int64
	var status string
	var source string
	if err := transaction.QueryRow(`
		SELECT subscription_id, status, source
		FROM after_sales_cases
		WHERE id = ?`, caseID).Scan(&subscriptionID, &status, &source); err != nil {
		return err
	}
	if source == model.AfterSalesSourceCustomerCancellation {
		return ErrCancellationNotReassignable
	}
	if status == model.AfterSalesStatusRefunded || status == model.AfterSalesStatusReassigned {
		return ErrAfterSalesProcessed
	}

	var currentSeatID int64
	if err := transaction.QueryRow(`
		SELECT COALESCE(seat_id, 0)
		FROM subscriptions
		WHERE id = ? AND deleted_at IS NULL AND archived_at IS NULL`, subscriptionID).Scan(&currentSeatID); err != nil {
		return fmt.Errorf("active subscription unavailable: %w", err)
	}

	var accountName string
	var accountEmail string
	var spaceName string
	var bannedAt sql.NullString
	if err := transaction.QueryRow(`
		SELECT name, COALESCE(email, ''), COALESCE(space_name, ''), banned_at
		FROM accounts
		WHERE id = ?`, replacementAccountID).Scan(
		&accountName,
		&accountEmail,
		&spaceName,
		&bannedAt,
	); err != nil {
		return err
	}
	if bannedAt.Valid && strings.TrimSpace(bannedAt.String) != "" {
		return ErrReplacementAccountBanned
	}

	var seatName string
	if replacementSeatID > 0 {
		if err := transaction.QueryRow(`
			SELECT name
			FROM seats
			WHERE id = ? AND account_id = ?`, replacementSeatID, replacementAccountID).Scan(&seatName); err != nil {
			return fmt.Errorf("%w: %v", ErrReplacementSeatUnavailable, err)
		}
	} else {
		if err := transaction.QueryRow(`
			SELECT seat.id, seat.name
			FROM seats AS seat
			WHERE seat.account_id = ?
			  AND NOT EXISTS (
				SELECT 1
				FROM subscriptions AS subscription
				WHERE subscription.seat_id = seat.id
				  AND subscription.deleted_at IS NULL
				  AND (
					subscription.archived_at IS NULL
					OR (
						subscription.seat_frozen_until IS NOT NULL
						AND julianday(subscription.seat_frozen_until) > julianday(?)
					)
				  )
			  )
			ORDER BY seat.id ASC
			LIMIT 1`, replacementAccountID, processedAt).Scan(&replacementSeatID, &seatName); err != nil {
			return fmt.Errorf("%w: %v", ErrReplacementSeatUnavailable, err)
		}
	}

	var occupiedCount int
	if err := transaction.QueryRow(`
		SELECT COUNT(1)
		FROM subscriptions
		WHERE seat_id = ?
		  AND id <> ?
		  AND deleted_at IS NULL
		  AND (
			archived_at IS NULL
			OR (
				seat_frozen_until IS NOT NULL
				AND julianday(seat_frozen_until) > julianday(?)
			)
		  )`, replacementSeatID, subscriptionID, processedAt).Scan(&occupiedCount); err != nil {
		return err
	}
	if occupiedCount > 0 {
		return ErrReplacementSeatOccupied
	}
	if replacementSeatID == currentSeatID {
		return ErrReplacementSeatUnchanged
	}

	result, err := transaction.Exec(`
		UPDATE subscriptions
		SET seat_id = ?, subscription_type = ?, updated_at = ?
		WHERE id = ? AND deleted_at IS NULL AND archived_at IS NULL`,
		replacementSeatID,
		accountName,
		processedAt,
		subscriptionID,
	)
	if err != nil {
		if isActiveSeatOccupancyError(err) {
			return ErrReplacementSeatOccupied
		}
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected != 1 {
		return sql.ErrNoRows
	}

	result, err = transaction.Exec(`
		UPDATE after_sales_cases
		SET replacement_account_id = ?, replacement_seat_id = ?,
		    replacement_account_name = ?, replacement_account_email = ?,
		    replacement_space_name = ?, replacement_seat_name = ?,
		    status = ?, processed_at = ?, updated_at = ?
		WHERE id = ? AND status IN (?, ?)`,
		replacementAccountID,
		replacementSeatID,
		strings.TrimSpace(accountName),
		strings.TrimSpace(accountEmail),
		strings.TrimSpace(spaceName),
		strings.TrimSpace(seatName),
		model.AfterSalesStatusReassigned,
		processedAt,
		processedAt,
		caseID,
		model.AfterSalesStatusPending,
		model.AfterSalesStatusReview,
	)
	if err != nil {
		return err
	}
	rowsAffected, err = result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected != 1 {
		return ErrAfterSalesProcessed
	}

	return transaction.Commit()
}

func scanAfterSalesCase(scanner scannable) (model.AfterSalesCase, error) {
	var caseItem model.AfterSalesCase
	var expiresAt sql.NullString
	var processedAt sql.NullString
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(
		&caseItem.ID,
		&caseItem.AccountID,
		&caseItem.SubscriptionID,
		&caseItem.BillID,
		&caseItem.BusinessType,
		&caseItem.AccountName,
		&caseItem.AccountEmail,
		&caseItem.AccountSpaceName,
		&caseItem.CustomerEmail,
		&caseItem.CustomerWechat,
		&caseItem.PeriodStart,
		&caseItem.PeriodEnd,
		&caseItem.BannedDate,
		&caseItem.WarrantyDays,
		&caseItem.UsedDays,
		&caseItem.RemainingDays,
		&caseItem.PaidAmountCents,
		&caseItem.RefundAmountCents,
		&caseItem.ReplacementAccountID,
		&caseItem.ReplacementSeatID,
		&caseItem.ReplacementAccountName,
		&caseItem.ReplacementAccountEmail,
		&caseItem.ReplacementSpaceName,
		&caseItem.ReplacementSeatName,
		&caseItem.Source,
		&expiresAt,
		&caseItem.Status,
		&caseItem.Note,
		&processedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.AfterSalesCase{}, err
	}
	var err error
	if expiresAt.Valid && expiresAt.String != "" {
		parsed, parseErr := parseTime(expiresAt.String)
		if parseErr != nil {
			return model.AfterSalesCase{}, parseErr
		}
		caseItem.ExpiresAt = &parsed
	}
	if processedAt.Valid && processedAt.String != "" {
		parsed, parseErr := parseTime(processedAt.String)
		if parseErr != nil {
			return model.AfterSalesCase{}, parseErr
		}
		caseItem.ProcessedAt = &parsed
	}
	caseItem.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.AfterSalesCase{}, err
	}
	caseItem.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.AfterSalesCase{}, err
	}
	return caseItem, nil
}
