package db

import (
	"database/sql"
	"strings"
	"time"

	"carpool-notify/internal/model"
)

const renewalSelectColumns = `
	id,
	tracking_token,
	subscription_id,
	customer_email,
	due_date,
	amount_cents,
	status,
	operator_note,
	processed_at,
	created_at,
	updated_at`

func (store *Store) CreateRenewalApplication(application model.RenewalApplication) (int64, error) {
	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
		INSERT INTO renewal_applications (
			tracking_token, subscription_id, customer_email, due_date,
			amount_cents, status, operator_note, processed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, '', NULL, ?, ?)`,
		strings.TrimSpace(application.TrackingToken),
		application.SubscriptionID,
		strings.TrimSpace(application.CustomerEmail),
		strings.TrimSpace(application.DueDate),
		application.AmountCents,
		model.RenewalStatusPending,
		now,
		now,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "idx_renewal_applications_one_pending_period") ||
			(strings.Contains(strings.ToLower(err.Error()), "unique constraint") &&
				strings.Contains(strings.ToLower(err.Error()), "subscription_id")) {
			return 0, ErrRenewalAlreadyPending
		}
		return 0, err
	}
	return result.LastInsertId()
}

func (store *Store) GetRenewalApplication(applicationID int64) (model.RenewalApplication, error) {
	row := store.database.QueryRow(`
		SELECT `+renewalSelectColumns+`
		FROM renewal_applications
		WHERE id = ?`, applicationID)
	return scanRenewalApplication(row)
}

func (store *Store) GetRenewalApplicationByToken(token string) (model.RenewalApplication, error) {
	row := store.database.QueryRow(`
		SELECT `+renewalSelectColumns+`
		FROM renewal_applications
		WHERE tracking_token = ?`, strings.TrimSpace(token))
	return scanRenewalApplication(row)
}

func (store *Store) ListRenewalApplications(status string) ([]model.RenewalApplication, error) {
	status = strings.TrimSpace(status)
	query := `SELECT ` + renewalSelectColumns + ` FROM renewal_applications`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY CASE status WHEN 'pending' THEN 0 ELSE 1 END, created_at DESC, id DESC`
	rows, err := store.database.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applications := make([]model.RenewalApplication, 0)
	for rows.Next() {
		application, err := scanRenewalApplication(rows)
		if err != nil {
			return nil, err
		}
		applications = append(applications, application)
	}
	return applications, rows.Err()
}

func (store *Store) CountRenewalApplicationsByStatus(status string) (int, error) {
	var count int
	err := store.database.QueryRow(
		`SELECT COUNT(1) FROM renewal_applications WHERE status = ?`,
		strings.TrimSpace(status),
	).Scan(&count)
	return count, err
}

func (store *Store) RejectRenewalApplication(applicationID int64, operatorNote string) error {
	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
		UPDATE renewal_applications
		SET status = ?, operator_note = ?, processed_at = ?, updated_at = ?
		WHERE id = ? AND status = ?`,
		model.RenewalStatusRejected,
		strings.TrimSpace(operatorNote),
		now,
		now,
		applicationID,
		model.RenewalStatusPending,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrRenewalAlreadyProcessed
	}
	return nil
}

// ApproveRenewalApplication atomically records the bill, applies any scheduled
// price change and closes the review request. The expected subscription
// timestamp and application snapshots prevent stale or duplicate accounting.
func (store *Store) ApproveRenewalApplication(
	application model.RenewalApplication,
	subscription model.Subscription,
	costCents int64,
	operatorNote string,
) error {
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	var storedStatus string
	var storedSubscriptionID int64
	var storedCustomerEmail string
	var storedDueDate string
	var storedAmountCents int64
	if err := transaction.QueryRow(`
		SELECT status, subscription_id, customer_email, due_date, amount_cents
		FROM renewal_applications
		WHERE id = ?`, application.ID).Scan(
		&storedStatus,
		&storedSubscriptionID,
		&storedCustomerEmail,
		&storedDueDate,
		&storedAmountCents,
	); err != nil {
		return err
	}
	if storedStatus != model.RenewalStatusPending {
		return ErrRenewalAlreadyProcessed
	}
	if storedSubscriptionID != subscription.ID ||
		!strings.EqualFold(strings.TrimSpace(storedCustomerEmail), strings.TrimSpace(application.CustomerEmail)) ||
		strings.TrimSpace(storedDueDate) != strings.TrimSpace(application.DueDate) ||
		storedAmountCents != application.AmountCents {
		return ErrRenewalFinancialStateChanged
	}

	var storedUpdatedAt string
	var storedBusinessType string
	var storedPriceCents int64
	var storedNextPriceCents sql.NullInt64
	var storedNextPriceDueDate string
	var storedCostCents int64
	var storedIsResale int
	var storedAgencyFeeCents int64
	var storedCronExpr string
	var storedBoardedAt string
	if err := transaction.QueryRow(`
		SELECT updated_at,
		       COALESCE(business_type, 'team'),
		       price_per_person_cents,
		       next_price_cents,
		       COALESCE(next_price_effective_due_date, ''),
		       COALESCE(cost_cents, 0),
		       COALESCE(is_resale, 0),
		       COALESCE(agency_fee_cents, 0),
		       cron_expr,
		       boarded_at
		FROM subscriptions AS subscription
		WHERE subscription.id = ?
		  AND subscription.deleted_at IS NULL
		  AND subscription.archived_at IS NULL
		  AND subscription.cancellation_requested_at IS NULL
		  AND COALESCE(subscription.cancellation_case_id, 0) = 0
		  AND lower(trim(subscription.customer_email)) = lower(trim(?))
		  AND NOT EXISTS (
			SELECT 1 FROM after_sales_cases AS after_sales
			WHERE after_sales.subscription_id = subscription.id
			  AND after_sales.status IN (?, ?)
		  )`,
		subscription.ID,
		application.CustomerEmail,
		model.AfterSalesStatusPending,
		model.AfterSalesStatusReview,
	).Scan(
		&storedUpdatedAt,
		&storedBusinessType,
		&storedPriceCents,
		&storedNextPriceCents,
		&storedNextPriceDueDate,
		&storedCostCents,
		&storedIsResale,
		&storedAgencyFeeCents,
		&storedCronExpr,
		&storedBoardedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return ErrRenewalFinancialStateChanged
		}
		return err
	}
	nextPriceMatches := storedNextPriceCents.Valid == (subscription.NextPriceCents != nil)
	if nextPriceMatches && storedNextPriceCents.Valid {
		nextPriceMatches = storedNextPriceCents.Int64 == *subscription.NextPriceCents
	}
	if storedUpdatedAt != formatTime(subscription.UpdatedAt.UTC()) ||
		storedBusinessType != subscription.BusinessType ||
		storedPriceCents != subscription.PricePerPersonCents ||
		!nextPriceMatches ||
		strings.TrimSpace(storedNextPriceDueDate) != strings.TrimSpace(subscription.NextPriceEffectiveDueDate) ||
		storedCostCents != subscription.CostCents ||
		(storedIsResale != 0) != subscription.IsResale ||
		storedAgencyFeeCents != subscription.AgencyFeeCents ||
		strings.TrimSpace(storedCronExpr) != strings.TrimSpace(subscription.CronExpr) ||
		strings.TrimSpace(storedBoardedAt) != strings.TrimSpace(subscription.BoardedAt) {
		return ErrRenewalFinancialStateChanged
	}

	var existingBillCount int
	if err := transaction.QueryRow(`
		SELECT COUNT(1) FROM bills WHERE subscription_id = ? AND due_date = ?`,
		subscription.ID,
		application.DueDate,
	).Scan(&existingBillCount); err != nil {
		return err
	}
	if existingBillCount != 0 {
		return ErrRenewalFinancialStateChanged
	}

	if costCents < 0 {
		costCents = 0
	}
	now := formatTime(time.Now().UTC())
	if _, err := transaction.Exec(`
		INSERT INTO bills (
			subscription_id, due_date, amount_cents, cost_cents,
			note, paid_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, '', ?, ?, ?)`,
		subscription.ID,
		application.DueDate,
		application.AmountCents,
		costCents,
		now,
		now,
		now,
	); err != nil {
		return err
	}

	if _, err := transaction.Exec(`
		INSERT INTO subscription_price_changes (
			subscription_id, previous_price_cents, new_price_cents,
			effective_due_date, created_at
		)
		SELECT id, price_per_person_cents, next_price_cents,
		       next_price_effective_due_date, ?
		FROM subscriptions
		WHERE id = ?
		  AND is_resale = 0
		  AND next_price_cents IS NOT NULL
		  AND next_price_cents <> price_per_person_cents
		  AND next_price_effective_due_date <> ''
		  AND next_price_effective_due_date <= ?
		ON CONFLICT(subscription_id, effective_due_date) DO NOTHING`,
		now,
		subscription.ID,
		application.DueDate,
	); err != nil {
		return err
	}
	if _, err := transaction.Exec(`
		UPDATE subscriptions
		SET price_per_person_cents = next_price_cents,
			next_price_cents = NULL,
			next_price_effective_due_date = '',
			updated_at = ?
		WHERE id = ?
		  AND is_resale = 0
		  AND next_price_cents IS NOT NULL
		  AND next_price_effective_due_date <> ''
		  AND next_price_effective_due_date <= ?`,
		now,
		subscription.ID,
		application.DueDate,
	); err != nil {
		return err
	}

	result, err := transaction.Exec(`
		UPDATE renewal_applications
		SET status = ?, operator_note = ?, processed_at = ?, updated_at = ?
		WHERE id = ? AND status = ?`,
		model.RenewalStatusApproved,
		strings.TrimSpace(operatorNote),
		now,
		now,
		application.ID,
		model.RenewalStatusPending,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrRenewalAlreadyProcessed
	}
	return transaction.Commit()
}

func scanRenewalApplication(scanner scannable) (model.RenewalApplication, error) {
	var application model.RenewalApplication
	var processedAt sql.NullString
	var createdAt string
	var updatedAt string
	err := scanner.Scan(
		&application.ID,
		&application.TrackingToken,
		&application.SubscriptionID,
		&application.CustomerEmail,
		&application.DueDate,
		&application.AmountCents,
		&application.Status,
		&application.OperatorNote,
		&processedAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.RenewalApplication{}, err
	}
	if processedAt.Valid && strings.TrimSpace(processedAt.String) != "" {
		parsed, err := parseTime(processedAt.String)
		if err != nil {
			return model.RenewalApplication{}, err
		}
		application.ProcessedAt = &parsed
	}
	application.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.RenewalApplication{}, err
	}
	application.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.RenewalApplication{}, err
	}
	application.CustomerEmail = strings.TrimSpace(application.CustomerEmail)
	application.DueDate = strings.TrimSpace(application.DueDate)
	application.OperatorNote = strings.TrimSpace(application.OperatorNote)
	return application, nil
}
