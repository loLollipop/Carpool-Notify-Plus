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
	account_id,
	subscription_id,
	bill_id,
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
	var existingBanNote string
	if err := transaction.QueryRow(`
		SELECT COALESCE(name, ''), COALESCE(email, ''), COALESCE(space_name, ''),
		       COALESCE(banned_at, ''), COALESCE(ban_note, '')
		FROM accounts
		WHERE id = ?`, accountID).Scan(
		&accountName,
		&accountEmail,
		&accountSpaceName,
		&existingBannedAt,
		&existingBanNote,
	); err != nil {
		return 0, err
	}

	// The first ban date is the refund event date. Repeated requests reuse it so
	// the unique key remains stable and cannot create duplicate refund cases.
	if strings.TrimSpace(existingBannedAt) != "" {
		bannedDate = existingBannedAt
		banNote = existingBanNote
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
		ID             int64
		CustomerEmail  string
		CustomerWechat string
	}
	rows, err := transaction.Query(`
		SELECT subscription.id,
		       COALESCE(subscription.customer_email, ''),
		       COALESCE(subscription.customer_wechat, '')
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
			refundAmountCents = (paidAmountCents*int64(remainingDays) + int64(warrantyDays/2)) / int64(warrantyDays)
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
	rows, err := store.database.Query(`
		SELECT ` + afterSalesSelectColumns + `
		FROM after_sales_cases
		ORDER BY banned_date DESC, id DESC`)
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

// UpdateAfterSalesCase updates the operator-adjustable refund and note fields.
func (store *Store) UpdateAfterSalesCase(caseID int64, refundAmountCents int64, note string) error {
	result, err := store.database.Exec(`
		UPDATE after_sales_cases
		SET refund_amount_cents = ?, note = ?, updated_at = ?
		WHERE id = ?`,
		refundAmountCents,
		strings.TrimSpace(note),
		formatTime(time.Now().UTC()),
		caseID,
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
	return nil
}

// SetAfterSalesCaseRefunded marks or unmarks one case as completed.
func (store *Store) SetAfterSalesCaseRefunded(caseID int64, refunded bool) error {
	status := model.AfterSalesStatusPending
	processedAt := any(nil)
	if refunded {
		status = model.AfterSalesStatusRefunded
		processedAt = formatTime(time.Now().UTC())
	} else {
		var billID int64
		if err := store.database.QueryRow(
			`SELECT bill_id FROM after_sales_cases WHERE id = ?`,
			caseID,
		).Scan(&billID); err != nil {
			return err
		}
		if billID == 0 {
			status = model.AfterSalesStatusReview
		}
	}
	result, err := store.database.Exec(`
		UPDATE after_sales_cases
		SET status = ?, processed_at = ?, updated_at = ?
		WHERE id = ?`,
		status,
		processedAt,
		formatTime(time.Now().UTC()),
		caseID,
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
	return nil
}

func scanAfterSalesCase(scanner scannable) (model.AfterSalesCase, error) {
	var caseItem model.AfterSalesCase
	var processedAt sql.NullString
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(
		&caseItem.ID,
		&caseItem.AccountID,
		&caseItem.SubscriptionID,
		&caseItem.BillID,
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
		&caseItem.Status,
		&caseItem.Note,
		&processedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.AfterSalesCase{}, err
	}
	var err error
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
