package db

import (
	"fmt"

	"carpool-notify/internal/model"
)

// backfillSubscriptionPriceChanges preserves price transitions that happened
// before the immutable ledger existed. Adjacent non-refunded paid periods are
// the only legacy evidence strong enough to infer an applied change; a first
// bill by itself is intentionally not guessed.
func (store *Store) backfillSubscriptionPriceChanges() error {
	_, err := store.database.Exec(`
		WITH ordered_bills AS (
			SELECT bill.subscription_id,
			       bill.due_date,
			       bill.amount_cents,
			       bill.created_at,
			       LAG(bill.amount_cents) OVER (
					PARTITION BY bill.subscription_id
					ORDER BY bill.due_date ASC, bill.id ASC
			       ) AS previous_price_cents
			FROM bills AS bill
			WHERE NOT EXISTS (
				SELECT 1
				FROM after_sales_cases AS after_sales
				WHERE after_sales.bill_id = bill.id
				  AND after_sales.status = ?
			)
		)
		INSERT INTO subscription_price_changes (
			subscription_id, previous_price_cents, new_price_cents,
			effective_due_date, created_at
		)
		SELECT subscription_id, previous_price_cents, amount_cents,
		       due_date, created_at
		FROM ordered_bills
		WHERE previous_price_cents IS NOT NULL
		  AND previous_price_cents <> amount_cents
		ON CONFLICT(subscription_id, effective_due_date) DO NOTHING`,
		model.AfterSalesStatusRefunded,
	)
	if err != nil {
		return fmt.Errorf("backfill subscription price changes: %w", err)
	}
	return nil
}

// ListSubscriptionPriceChanges returns immutable applied-price history. Future
// schedules do not appear here until a bill in their effective period is paid.
func (store *Store) ListSubscriptionPriceChanges() ([]model.SubscriptionPriceChange, error) {
	rows, err := store.database.Query(`
		SELECT id, subscription_id, previous_price_cents, new_price_cents,
		       effective_due_date, created_at
		FROM subscription_price_changes
		ORDER BY subscription_id ASC, effective_due_date ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	changes := make([]model.SubscriptionPriceChange, 0)
	for rows.Next() {
		var change model.SubscriptionPriceChange
		var createdAt string
		if err := rows.Scan(
			&change.ID,
			&change.SubscriptionID,
			&change.PreviousPriceCents,
			&change.NewPriceCents,
			&change.EffectiveDueDate,
			&createdAt,
		); err != nil {
			return nil, err
		}
		change.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse subscription price change created_at: %w", err)
		}
		changes = append(changes, change)
	}
	return changes, rows.Err()
}
