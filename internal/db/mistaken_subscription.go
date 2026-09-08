package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"carpool-notify/internal/model"
)

var (
	ErrMistakenSubscriptionNotActive     = errors.New("mistaken subscription is not active")
	ErrMistakenSubscriptionNotTeam       = errors.New("mistaken subscription is not a Team subscription")
	ErrMistakenSubscriptionHasAfterSales = errors.New("mistaken subscription has after-sales history")
)

// DeleteMistakenTeamSubscription permanently removes an active Team
// registration and all financial/activity records created for it. A linked
// redemption application is returned to pending so it can be assigned again.
func (store *Store) DeleteMistakenTeamSubscription(subscriptionID int64) error {
	now := formatTime(time.Now().UTC())
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	var businessType string
	var archivedAt sql.NullString
	if err := transaction.QueryRow(`
		SELECT COALESCE(business_type, 'team'), archived_at
		FROM subscriptions
		WHERE id = ? AND deleted_at IS NULL`, subscriptionID).Scan(
		&businessType,
		&archivedAt,
	); err != nil {
		return err
	}
	if normalizeStoredBusinessType(businessType) != model.SubscriptionBusinessTeam {
		return ErrMistakenSubscriptionNotTeam
	}
	if archivedAt.Valid && strings.TrimSpace(archivedAt.String) != "" {
		return ErrMistakenSubscriptionNotActive
	}

	var afterSalesCount int
	if err := transaction.QueryRow(`
		SELECT COUNT(1)
		FROM after_sales_cases
		WHERE subscription_id = ?`, subscriptionID).Scan(&afterSalesCount); err != nil {
		return err
	}
	if afterSalesCount > 0 {
		return ErrMistakenSubscriptionHasAfterSales
	}

	// A redemption request is customer-owned input, so rewind it instead of
	// deleting it together with the operator's mistaken registration.
	if _, err := transaction.Exec(`
		UPDATE redemption_applications
		SET status = ?,
			assigned_account_id = 0,
			assigned_seat_id = 0,
			assigned_subscription_id = 0,
			operator_note = '',
			invited_at = NULL,
			updated_at = ?
		WHERE assigned_subscription_id = ?`,
		model.RedemptionStatusPending,
		now,
		subscriptionID,
	); err != nil {
		return err
	}

	childTables := []string{
		"renewal_applications",
		"notification_log",
		"paid_due_occurrences",
		"customer_benefits",
		"pricing_exemptions",
		"subscription_price_changes",
		"bills",
	}
	for _, table := range childTables {
		if _, err := transaction.Exec(
			"DELETE FROM "+table+" WHERE subscription_id = ?",
			subscriptionID,
		); err != nil {
			return err
		}
	}

	result, err := transaction.Exec(`
		DELETE FROM subscriptions
		WHERE id = ?
		  AND deleted_at IS NULL
		  AND archived_at IS NULL
		  AND COALESCE(business_type, 'team') <> ?`,
		subscriptionID,
		model.SubscriptionBusinessPlus,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected != 1 {
		return ErrSubscriptionFinancialStateChanged
	}

	return transaction.Commit()
}
