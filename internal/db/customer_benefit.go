package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"carpool-notify/internal/model"
)

const customerBenefitSelectColumns = `
	id, batch_id, subscription_id, benefit_type, benefit_name,
	actual_cost_cents, perceived_value_cents, benefit_date,
	next_due_date_snapshot, customer_email_snapshot,
	customer_wechat_snapshot, customer_tier_snapshot,
	customer_group_size_snapshot, current_price_cents_snapshot,
	renewal_count_snapshot, recommendation_code, note, created_at`

// ListCustomerBenefits returns immutable delivery history, newest first.
func (store *Store) ListCustomerBenefits() ([]model.CustomerBenefit, error) {
	rows, err := store.database.Query(`
		SELECT ` + customerBenefitSelectColumns + `
		FROM customer_benefits
		ORDER BY benefit_date DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	benefits := make([]model.CustomerBenefit, 0)
	for rows.Next() {
		benefit, scanErr := scanCustomerBenefit(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		benefits = append(benefits, benefit)
	}
	return benefits, rows.Err()
}

// CreateCustomerBenefits records a whole delivered batch or none of it. The
// INSERT ... SELECT guard prevents stale clients from attaching care costs to
// archived, banned, resale, Plus, or currently after-sales-blocked records.
func (store *Store) CreateCustomerBenefits(benefits []model.CustomerBenefit) error {
	if len(benefits) == 0 {
		return nil
	}
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	for _, benefit := range benefits {
		createdAt := benefit.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		result, insertErr := transaction.Exec(`
			INSERT INTO customer_benefits (
				batch_id, subscription_id, benefit_type, benefit_name,
				actual_cost_cents, perceived_value_cents, benefit_date,
				next_due_date_snapshot, customer_email_snapshot,
				customer_wechat_snapshot, customer_tier_snapshot,
				customer_group_size_snapshot, current_price_cents_snapshot,
				renewal_count_snapshot, recommendation_code, note, created_at
			)
			SELECT ?, subscription.id, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
			FROM subscriptions AS subscription
			JOIN seats AS seat ON seat.id = subscription.seat_id
			JOIN accounts AS account ON account.id = seat.account_id
			WHERE subscription.id = ?
			  AND subscription.deleted_at IS NULL
			  AND subscription.archived_at IS NULL
			  AND LOWER(TRIM(COALESCE(subscription.business_type, 'team'))) = ?
			  AND subscription.seat_id > 0
			  AND COALESCE(subscription.is_resale, 0) = 0
			  AND NULLIF(TRIM(COALESCE(account.banned_at, '')), '') IS NULL
			  AND subscription.price_per_person_cents = ?
			  AND NOT EXISTS (
				SELECT 1 FROM after_sales_cases
				WHERE subscription_id = subscription.id
				  AND status IN (?, ?)
			  )`,
			benefit.BatchID,
			benefit.BenefitType,
			benefit.BenefitName,
			benefit.ActualCostCents,
			benefit.PerceivedValueCents,
			benefit.BenefitDate,
			benefit.NextDueDateSnapshot,
			benefit.CustomerEmailSnapshot,
			benefit.CustomerWechatSnapshot,
			benefit.CustomerTierSnapshot,
			benefit.CustomerGroupSizeSnapshot,
			benefit.CurrentPriceCentsSnapshot,
			benefit.RenewalCountSnapshot,
			benefit.RecommendationCode,
			strings.TrimSpace(benefit.Note),
			formatTime(createdAt.UTC()),
			benefit.SubscriptionID,
			model.SubscriptionBusinessTeam,
			benefit.CurrentPriceCentsSnapshot,
			model.AfterSalesStatusPending,
			model.AfterSalesStatusReview,
		)
		if insertErr != nil {
			if strings.Contains(strings.ToLower(insertErr.Error()), "unique constraint") {
				return ErrCustomerBenefitAlreadyRecorded
			}
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

func scanCustomerBenefit(scanner scannable) (model.CustomerBenefit, error) {
	var benefit model.CustomerBenefit
	var createdAt string
	if err := scanner.Scan(
		&benefit.ID,
		&benefit.BatchID,
		&benefit.SubscriptionID,
		&benefit.BenefitType,
		&benefit.BenefitName,
		&benefit.ActualCostCents,
		&benefit.PerceivedValueCents,
		&benefit.BenefitDate,
		&benefit.NextDueDateSnapshot,
		&benefit.CustomerEmailSnapshot,
		&benefit.CustomerWechatSnapshot,
		&benefit.CustomerTierSnapshot,
		&benefit.CustomerGroupSizeSnapshot,
		&benefit.CurrentPriceCentsSnapshot,
		&benefit.RenewalCountSnapshot,
		&benefit.RecommendationCode,
		&benefit.Note,
		&createdAt,
	); err != nil {
		return model.CustomerBenefit{}, err
	}
	parsed, err := parseTime(createdAt)
	if err != nil {
		return model.CustomerBenefit{}, fmt.Errorf("parse customer benefit created_at: %w", err)
	}
	benefit.CreatedAt = parsed
	return benefit, nil
}
