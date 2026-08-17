package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"carpool-notify/internal/model"
)

// ListPricingExemptions returns immutable repricing-exemption history. The
// service decides which latest row is still active relative to its clock.
func (store *Store) ListPricingExemptions() ([]model.PricingExemption, error) {
	rows, err := store.database.Query(`
		SELECT id, subscription_id, reason_code, note, review_after,
		       review_cycles, price_cents_snapshot,
		       market_median_cents_snapshot, created_at
		FROM pricing_exemptions
		ORDER BY subscription_id ASC, created_at ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	exemptions := make([]model.PricingExemption, 0)
	for rows.Next() {
		var exemption model.PricingExemption
		var createdAt string
		if err := rows.Scan(
			&exemption.ID,
			&exemption.SubscriptionID,
			&exemption.ReasonCode,
			&exemption.Note,
			&exemption.ReviewAfter,
			&exemption.ReviewCycles,
			&exemption.PriceCentsSnapshot,
			&exemption.MarketMedianCentsSnapshot,
			&createdAt,
		); err != nil {
			return nil, err
		}
		exemption.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse pricing exemption created_at: %w", err)
		}
		exemptions = append(exemptions, exemption)
	}
	return exemptions, rows.Err()
}

// CreatePricingExemptions creates a whole exemption batch or none of it. Each
// insert rechecks the financial and operational snapshot inside the same
// transaction so a stale browser cannot exempt an archived, repriced, banned,
// after-sales, or already-exempted subscription.
func (store *Store) CreatePricingExemptions(exemptions []model.PricingExemption, today string) error {
	if len(exemptions) == 0 {
		return nil
	}
	today = strings.TrimSpace(today)
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	createdAt := formatTime(time.Now().UTC())
	for _, exemption := range exemptions {
		result, insertErr := transaction.Exec(`
			INSERT INTO pricing_exemptions (
				subscription_id, reason_code, note, review_after, review_cycles,
				price_cents_snapshot, market_median_cents_snapshot, created_at
			)
			SELECT subscription.id, ?, ?, ?, ?, ?, ?, ?
			FROM subscriptions AS subscription
			JOIN seats AS seat ON seat.id = subscription.seat_id
			JOIN accounts AS account ON account.id = seat.account_id
			WHERE subscription.id = ?
			  AND subscription.deleted_at IS NULL
			  AND subscription.archived_at IS NULL
			  AND LOWER(TRIM(COALESCE(subscription.business_type, 'team'))) = ?
			  AND subscription.seat_id > 0
			  AND COALESCE(subscription.is_resale, 0) = 0
			  AND subscription.next_price_cents IS NULL
			  AND subscription.price_per_person_cents = ?
			  AND NULLIF(TRIM(COALESCE(account.banned_at, '')), '') IS NULL
			  AND NOT EXISTS (
				SELECT 1 FROM after_sales_cases
				WHERE subscription_id = subscription.id
				  AND status IN (?, ?)
			  )
			  AND NOT EXISTS (
				SELECT 1 FROM pricing_exemptions
				WHERE subscription_id = subscription.id
				  AND review_after > ?
			  )`,
			exemption.ReasonCode,
			strings.TrimSpace(exemption.Note),
			strings.TrimSpace(exemption.ReviewAfter),
			exemption.ReviewCycles,
			exemption.PriceCentsSnapshot,
			exemption.MarketMedianCentsSnapshot,
			createdAt,
			exemption.SubscriptionID,
			model.SubscriptionBusinessTeam,
			exemption.PriceCentsSnapshot,
			model.AfterSalesStatusPending,
			model.AfterSalesStatusReview,
			today,
		)
		if insertErr != nil {
			return insertErr
		}
		affected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		if affected != 1 {
			return sql.ErrNoRows
		}
	}
	return transaction.Commit()
}
