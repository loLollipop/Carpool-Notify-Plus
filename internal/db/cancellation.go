package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

// RequestSubscriptionCancellation creates a temporary after-sales case while
// keeping the subscription active and its seat occupied until processing.
func (store *Store) RequestSubscriptionCancellation(
	subscriptionID int64,
	requestedAt time.Time,
	expiresAt time.Time,
	warrantyDays int,
) (model.AfterSalesCase, error) {
	transaction, err := store.database.Begin()
	if err != nil {
		return model.AfterSalesCase{}, err
	}
	defer func() { _ = transaction.Rollback() }()

	var accountID int64
	var accountName string
	var accountEmail string
	var accountSpaceName string
	var customerEmail string
	var customerWechat string
	var cancellationCaseID int64
	var businessType string
	var subscriptionName string
	var cronExpr string
	var boardedAt string
	if err := transaction.QueryRow(`
		SELECT COALESCE(seat.account_id, 0),
		       COALESCE(account.name, ''),
		       COALESCE(account.email, ''),
		       COALESCE(account.space_name, ''),
		       COALESCE(subscription.customer_email, ''),
		       COALESCE(subscription.customer_wechat, ''),
		       COALESCE(subscription.cancellation_case_id, 0),
		       COALESCE(subscription.business_type, 'team'),
		       subscription.name,
		       subscription.cron_expr,
		       subscription.boarded_at
		FROM subscriptions AS subscription
		LEFT JOIN seats AS seat ON seat.id = subscription.seat_id
		LEFT JOIN accounts AS account ON account.id = seat.account_id
		WHERE subscription.id = ?
		  AND subscription.deleted_at IS NULL
		  AND subscription.archived_at IS NULL`, subscriptionID).Scan(
		&accountID,
		&accountName,
		&accountEmail,
		&accountSpaceName,
		&customerEmail,
		&customerWechat,
		&cancellationCaseID,
		&businessType,
		&subscriptionName,
		&cronExpr,
		&boardedAt,
	); err != nil {
		return model.AfterSalesCase{}, err
	}
	if cancellationCaseID > 0 {
		return model.AfterSalesCase{}, ErrCancellationPending
	}
	var pendingAfterSalesCount int
	if err := transaction.QueryRow(`
		SELECT COUNT(1)
		FROM after_sales_cases
		WHERE subscription_id = ? AND status IN (?, ?)`,
		subscriptionID,
		model.AfterSalesStatusPending,
		model.AfterSalesStatusReview,
	).Scan(&pendingAfterSalesCount); err != nil {
		return model.AfterSalesCase{}, err
	}
	if pendingAfterSalesCount > 0 {
		return model.AfterSalesCase{}, ErrCancellationCaseConflict
	}
	businessType = normalizeStoredBusinessType(businessType)
	plusRental := businessType == model.SubscriptionBusinessPlus
	if accountID <= 0 && !plusRental {
		return model.AfterSalesCase{}, fmt.Errorf("subscription has no owner account")
	}
	var storedAccountID any = accountID
	if plusRental {
		storedAccountID = nil
		accountName = strings.TrimSpace(subscriptionName)
		accountEmail = strings.TrimSpace(customerEmail)
		accountSpaceName = ""
		// Plus stores the rented login email in subscription.customer_email. Keep
		// it as the affected account snapshot, not as the customer's contact email.
		customerEmail = ""
	}

	requestedAt = requestedAt.In(cycle.Location)
	requestDate := cycle.FormatDate(requestedAt)
	billID := int64(0)
	periodStart := ""
	periodEnd := ""
	paidAmountCents := int64(0)
	refundAmountCents := int64(0)
	usedDays := 0
	remainingDays := 0
	status := model.AfterSalesStatusReview
	note := "未找到退订日前的已缴账单，请人工核对退款金额"

	err = transaction.QueryRow(`
		SELECT id, due_date, amount_cents
		FROM bills
		WHERE subscription_id = ? AND due_date <= ?
		ORDER BY due_date DESC, id DESC
		LIMIT 1`, subscriptionID, requestDate).Scan(
		&billID,
		&periodStart,
		&paidAmountCents,
	)
	if err != nil && err != sql.ErrNoRows {
		return model.AfterSalesCase{}, err
	}
	if err == nil {
		periodDay, parseErr := time.ParseInLocation("2006-01-02", periodStart, cycle.Location)
		if parseErr != nil {
			return model.AfterSalesCase{}, fmt.Errorf("parse bill due date: %w", parseErr)
		}
		requestDay, _ := time.ParseInLocation("2006-01-02", requestDate, cycle.Location)
		usedDays = int(requestDay.Sub(periodDay).Hours() / 24)
		if usedDays < 0 {
			usedDays = 0
		}
		periodDays := warrantyDays
		periodEndDay := periodDay.AddDate(0, 0, periodDays)
		if plusRental {
			schedule, scheduleErr := cycle.ParseBillingSchedule(cronExpr, boardedAt)
			if scheduleErr != nil {
				return model.AfterSalesCase{}, fmt.Errorf("parse Plus billing schedule: %w", scheduleErr)
			}
			periodEndDay = schedule.NextDue(cycle.StartOfDay(periodDay))
			periodDays = int(periodEndDay.Sub(periodDay).Hours() / 24)
			if periodDays <= 0 {
				return model.AfterSalesCase{}, fmt.Errorf("invalid Plus billing period")
			}
			warrantyDays = periodDays
		}
		if usedDays > periodDays {
			usedDays = periodDays
		}
		remainingDays = periodDays - usedDays
		periodEnd = periodEndDay.Format("2006-01-02")
		refundAmountCents = proratedRefundCents(paidAmountCents, remainingDays, periodDays)
		status = model.AfterSalesStatusPending
		note = ""
	}

	nowText := formatTime(requestedAt.UTC())
	expiresText := formatTime(expiresAt.UTC())
	result, err := transaction.Exec(`
		INSERT INTO after_sales_cases (
			account_id, subscription_id, bill_id, business_type,
			account_name, account_email, account_space_name, customer_email, customer_wechat,
			period_start, period_end, banned_date, warranty_days,
			used_days, remaining_days, paid_amount_cents, refund_amount_cents,
			source, expires_at, status, note, processed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
		storedAccountID,
		subscriptionID,
		billID,
		businessType,
		strings.TrimSpace(accountName),
		strings.TrimSpace(accountEmail),
		strings.TrimSpace(accountSpaceName),
		strings.TrimSpace(customerEmail),
		strings.TrimSpace(customerWechat),
		periodStart,
		periodEnd,
		requestDate,
		warrantyDays,
		usedDays,
		remainingDays,
		paidAmountCents,
		refundAmountCents,
		model.AfterSalesSourceCustomerCancellation,
		expiresText,
		status,
		note,
		nowText,
		nowText,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return model.AfterSalesCase{}, ErrCancellationCaseConflict
		}
		return model.AfterSalesCase{}, err
	}
	caseID, err := result.LastInsertId()
	if err != nil {
		return model.AfterSalesCase{}, err
	}

	result, err = transaction.Exec(`
		UPDATE subscriptions
		SET cancellation_requested_at = ?, cancellation_expires_at = ?,
		    cancellation_case_id = ?, updated_at = ?
		WHERE id = ?
		  AND deleted_at IS NULL
		  AND archived_at IS NULL
		  AND COALESCE(cancellation_case_id, 0) = 0`,
		nowText,
		expiresText,
		caseID,
		nowText,
		subscriptionID,
	)
	if err != nil {
		return model.AfterSalesCase{}, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return model.AfterSalesCase{}, err
	}
	if rowsAffected != 1 {
		return model.AfterSalesCase{}, ErrCancellationPending
	}
	if err := transaction.Commit(); err != nil {
		return model.AfterSalesCase{}, err
	}
	return store.GetAfterSalesCase(caseID)
}

func (store *Store) GetAfterSalesCase(caseID int64) (model.AfterSalesCase, error) {
	row := store.database.QueryRow(`
		SELECT `+afterSalesSelectColumns+`
		FROM after_sales_cases
		WHERE id = ?`, caseID)
	return scanAfterSalesCase(row)
}

// RestoreExpiredCancellationRequests reactivates untouched cancellations and
// removes their temporary after-sales cases after the 24-hour grace period.
func (store *Store) RestoreExpiredCancellationRequests(now time.Time) (int, error) {
	transaction, err := store.database.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = transaction.Rollback() }()

	nowText := formatTime(now.UTC())
	if _, err := transaction.Exec(`
		DELETE FROM after_sales_cases
		WHERE source = ?
		  AND status IN (?, ?)
		  AND id IN (
			SELECT cancellation_case_id
			FROM subscriptions
			WHERE archived_at IS NULL
			  AND deleted_at IS NULL
			  AND cancellation_expires_at IS NOT NULL
			  AND cancellation_expires_at <= ?
		  )`,
		model.AfterSalesSourceCustomerCancellation,
		model.AfterSalesStatusPending,
		model.AfterSalesStatusReview,
		nowText,
	); err != nil {
		return 0, err
	}

	result, err := transaction.Exec(`
		UPDATE subscriptions
		SET cancellation_requested_at = NULL,
		    cancellation_expires_at = NULL,
		    cancellation_case_id = 0,
		    seat_frozen_until = NULL,
		    updated_at = ?
		WHERE archived_at IS NULL
		  AND deleted_at IS NULL
		  AND cancellation_expires_at IS NOT NULL
		  AND cancellation_expires_at <= ?`,
		nowText,
		nowText,
	)
	if err != nil {
		return 0, err
	}
	restored, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := transaction.Commit(); err != nil {
		return 0, err
	}
	return int(restored), nil
}

// CompleteCancellationRefund closes a customer cancellation and archives the
// subscription atomically while reserving its Team seat until the protection
// window expires.
func (store *Store) CompleteCancellationRefund(
	caseID int64,
	processedTime time.Time,
	seatFreezeUntil time.Time,
) error {
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	var subscriptionID int64
	var source string
	var status string
	if err := transaction.QueryRow(`
		SELECT subscription_id, source, status
		FROM after_sales_cases
		WHERE id = ?`, caseID).Scan(&subscriptionID, &source, &status); err != nil {
		return err
	}
	if source != model.AfterSalesSourceCustomerCancellation {
		return ErrAfterSalesProcessed
	}
	if status != model.AfterSalesStatusPending && status != model.AfterSalesStatusReview {
		return ErrAfterSalesProcessed
	}
	if err := ensureAfterSalesRefundWithinPayment(transaction, caseID, -1); err != nil {
		return err
	}

	nowText := formatTime(processedTime.UTC())
	result, err := transaction.Exec(`
		UPDATE after_sales_cases
		SET status = ?, processed_at = ?, expires_at = NULL, updated_at = ?
		WHERE id = ? AND status IN (?, ?)`,
		model.AfterSalesStatusRefunded,
		nowText,
		nowText,
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
	if rowsAffected != 1 {
		return ErrAfterSalesProcessed
	}
	freezeUntilText := formatTime(seatFreezeUntil.UTC())
	if err := archiveSubscriptionInTransaction(
		transaction,
		subscriptionID,
		nowText,
		caseID,
		freezeUntilText,
	); err != nil {
		return err
	}
	return transaction.Commit()
}

func archiveSubscriptionInTransaction(
	transaction *sql.Tx,
	subscriptionID int64,
	nowText string,
	preserveCancellationCaseID int64,
	seatFreezeUntil any,
) error {
	var cancellationCaseID int64
	if err := transaction.QueryRow(`
		SELECT COALESCE(cancellation_case_id, 0)
		FROM subscriptions
		WHERE id = ? AND deleted_at IS NULL`, subscriptionID).Scan(&cancellationCaseID); err != nil {
		return err
	}
	if preserveCancellationCaseID > 0 && cancellationCaseID != preserveCancellationCaseID {
		return ErrAfterSalesProcessed
	}

	result, err := transaction.Exec(`
		UPDATE subscriptions
		SET archived_at = COALESCE(archived_at, ?),
		    seat_frozen_until = CASE
				WHEN seat_id IS NOT NULL THEN ?
				ELSE NULL
			END,
		    cancellation_requested_at = NULL,
		    cancellation_expires_at = NULL,
		    cancellation_case_id = 0,
		    updated_at = ?
		WHERE id = ? AND deleted_at IS NULL`,
		nowText,
		seatFreezeUntil,
		nowText,
		subscriptionID,
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

	if cancellationCaseID > 0 && cancellationCaseID != preserveCancellationCaseID {
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
	if _, err := transaction.Exec(`
		DELETE FROM redemption_codes
		WHERE used_by_application_id IN (
			SELECT id FROM redemption_applications
			WHERE assigned_subscription_id = ?
		)`, subscriptionID); err != nil {
		return err
	}
	if _, err := transaction.Exec(`
		DELETE FROM redemption_applications
		WHERE assigned_subscription_id = ?`, subscriptionID); err != nil {
		return err
	}
	return nil
}
