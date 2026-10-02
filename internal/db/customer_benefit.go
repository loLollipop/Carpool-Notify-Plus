package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

const customerBenefitSelectColumns = `
	id, batch_id, subscription_id, benefit_type, benefit_name,
	actual_cost_cents, perceived_value_cents, benefit_date,
	next_due_date_snapshot, customer_email_snapshot,
	customer_wechat_snapshot, customer_tier_snapshot,
	customer_group_size_snapshot, current_price_cents_snapshot,
	renewal_count_snapshot, price_before_cents, price_after_cents,
	price_effective_due_date, price_adjustment_key,
	recommendation_code, note, created_at,
	COALESCE((SELECT extension_days FROM subscription_due_extensions WHERE customer_benefit_id = customer_benefits.id), 0),
	(SELECT created_at FROM subscription_due_extensions WHERE customer_benefit_id = customer_benefits.id)`

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

// HasCustomerBenefitBatchOverlap reports whether any requested subscription
// was already recorded under the same client operation batch.
func (store *Store) HasCustomerBenefitBatchOverlap(
	batchID string,
	subscriptionIDs []int64,
) (bool, error) {
	if strings.TrimSpace(batchID) == "" || len(subscriptionIDs) == 0 {
		return false, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(subscriptionIDs)), ",")
	args := make([]any, 0, len(subscriptionIDs)+1)
	args = append(args, strings.TrimSpace(batchID))
	for _, subscriptionID := range subscriptionIDs {
		args = append(args, subscriptionID)
	}
	var exists bool
	err := store.database.QueryRow(`
		SELECT EXISTS (
			SELECT 1
			FROM customer_benefits
			WHERE batch_id = ?
			  AND subscription_id IN (`+placeholders+`)
		)`, args...).Scan(&exists)
	return exists, err
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
	if err := createCustomerBenefitsWithTransaction(transaction, benefits); err != nil {
		return err
	}
	return transaction.Commit()
}

// CreateCustomerBenefitsAndUpdateSubscriptionNextPrices records delivered
// benefits and schedules their future prices as one all-or-nothing operation.
func (store *Store) CreateCustomerBenefitsAndUpdateSubscriptionNextPrices(
	benefits []model.CustomerBenefit,
	subscriptions []model.Subscription,
	effectiveAt time.Time,
	reviewDates ...string,
) error {
	if len(benefits) == 0 && len(subscriptions) == 0 {
		return nil
	}
	if len(benefits) != len(subscriptions) {
		return fmt.Errorf("customer benefit and next-price counts do not match")
	}
	for index := range benefits {
		if benefits[index].SubscriptionID != subscriptions[index].ID {
			return fmt.Errorf("customer benefit and next-price subscriptions do not match")
		}
	}
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	scheduledSubscriptions, err := nextPricesAtFirstUnpaidDueWithTransaction(
		transaction,
		subscriptions,
		effectiveAt,
	)
	if err != nil {
		return err
	}
	for index := range benefits {
		before := scheduledSubscriptions[index].PricePerPersonCents
		after := *scheduledSubscriptions[index].NextPriceCents
		effectiveDueDate := strings.TrimSpace(scheduledSubscriptions[index].NextPriceEffectiveDueDate)
		benefits[index].PriceBeforeCents = before
		benefits[index].PriceAfterCents = after
		benefits[index].PriceEffectiveDueDate = effectiveDueDate
		benefits[index].PriceAdjustmentKey = priceAdjustmentKey(
			benefits[index].SubscriptionID,
			effectiveDueDate,
			before,
			after,
		)
		benefits[index].NextDueDateSnapshot = effectiveDueDate
	}
	if err := createCustomerBenefitsWithTransaction(transaction, benefits); err != nil {
		return err
	}
	if err := updateSubscriptionNextPricesWithTransaction(
		transaction,
		scheduledSubscriptions,
		reviewDates...,
	); err != nil {
		return err
	}
	return subscriptionStateWriteError(transaction.Commit())
}

func nextPricesAtFirstUnpaidDueWithTransaction(
	transaction *sql.Tx,
	subscriptions []model.Subscription,
	effectiveAt time.Time,
) ([]model.Subscription, error) {
	scheduled := append([]model.Subscription(nil), subscriptions...)
	for index := range scheduled {
		subscription := &scheduled[index]
		schedule, err := subscription.BillingSchedule()
		if err != nil {
			return nil, err
		}
		// A benefit granted on a renewal date applies to that date while it is
		// still unpaid. Starting just before today's boundary keeps NextDue's
		// strict semantics while avoiding a one-cycle delay.
		candidate := schedule.NextDue(
			cycle.StartOfDay(effectiveAt.In(cycle.Location)).Add(-time.Nanosecond),
		)
		rows, err := transaction.Query(`
			SELECT due_date
			FROM bills
			WHERE subscription_id = ?
			  AND due_date >= ?
			ORDER BY due_date`,
			subscription.ID,
			cycle.FormatDate(candidate),
		)
		if err != nil {
			return nil, err
		}
		paidDueDates := make(map[string]struct{})
		for rows.Next() {
			var dueDate string
			if scanErr := rows.Scan(&dueDate); scanErr != nil {
				_ = rows.Close()
				return nil, scanErr
			}
			paidDueDates[strings.TrimSpace(dueDate)] = struct{}{}
		}
		rowsErr := rows.Err()
		closeErr := rows.Close()
		if rowsErr != nil {
			return nil, rowsErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		for range len(paidDueDates) + 1 {
			if _, paid := paidDueDates[cycle.FormatDate(candidate)]; !paid {
				subscription.NextPriceEffectiveDueDate = cycle.FormatDate(candidate)
				break
			}
			// Bills are unique per calendar date, so skip all cron occurrences on
			// a paid day before looking for the next unpaid period.
			next := schedule.NextDue(
				cycle.StartOfDay(candidate).AddDate(0, 0, 1).Add(-time.Nanosecond),
			)
			if !next.After(candidate) {
				return nil, fmt.Errorf(
					"billing schedule did not advance after %s",
					cycle.FormatDate(candidate),
				)
			}
			candidate = next
		}
		if strings.TrimSpace(subscription.NextPriceEffectiveDueDate) == "" {
			return nil, fmt.Errorf("unable to find an unpaid billing period")
		}
	}
	return scheduled, nil
}

func createCustomerBenefitsWithTransaction(
	transaction *sql.Tx,
	benefits []model.CustomerBenefit,
) error {
	for index := range benefits {
		benefit := benefits[index]
		createdAt := benefit.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		equivalentTypes := equivalentCustomerBenefitTypes(benefit.BenefitType)
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(equivalentTypes)), ",")
		duplicateArgs := make([]any, 0, 3+len(equivalentTypes))
		duplicateArgs = append(
			duplicateArgs,
			benefit.SubscriptionID,
			benefit.BenefitDate,
			benefit.BenefitName,
		)
		for _, benefitType := range equivalentTypes {
			duplicateArgs = append(duplicateArgs, benefitType)
		}
		duplicateArgs = append(duplicateArgs, benefit.ExtensionDays)
		var alreadyRecorded bool
		if queryErr := transaction.QueryRow(`
			SELECT EXISTS (
				SELECT 1
				FROM customer_benefits
				WHERE subscription_id = ?
				  AND benefit_date = ?
				  AND benefit_name = ?
				  AND benefit_type IN (`+placeholders+`)
				  AND (? = 0 OR batch_id NOT LIKE 'benefit-operation-v1:%')
			)`, duplicateArgs...).Scan(&alreadyRecorded); queryErr != nil {
			return queryErr
		}
		if alreadyRecorded {
			return ErrCustomerBenefitAlreadyRecorded
		}
		result, insertErr := transaction.Exec(`
			INSERT INTO customer_benefits (
				batch_id, subscription_id, benefit_type, benefit_name,
				actual_cost_cents, perceived_value_cents, benefit_date,
				next_due_date_snapshot, customer_email_snapshot,
				customer_wechat_snapshot, customer_tier_snapshot,
				customer_group_size_snapshot, current_price_cents_snapshot,
				renewal_count_snapshot, price_before_cents, price_after_cents,
				price_effective_due_date, price_adjustment_key,
				recommendation_code, note, created_at
			)
			SELECT ?, subscription.id, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
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
			benefit.PriceBeforeCents,
			benefit.PriceAfterCents,
			strings.TrimSpace(benefit.PriceEffectiveDueDate),
			strings.TrimSpace(benefit.PriceAdjustmentKey),
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
		benefits[index].ID, insertErr = result.LastInsertId()
		if insertErr != nil {
			return insertErr
		}
	}
	return nil
}

func equivalentCustomerBenefitTypes(value string) []string {
	switch value {
	case model.CustomerBenefitTypeExtension,
		model.CustomerBenefitTypeRenewalMilestone,
		model.CustomerBenefitTypeLoyaltyCare,
		model.CustomerBenefitTypeServiceRecovery,
		model.CustomerBenefitTypeManual:
		return []string{
			model.CustomerBenefitTypeExtension,
			model.CustomerBenefitTypeRenewalMilestone,
			model.CustomerBenefitTypeLoyaltyCare,
			model.CustomerBenefitTypeServiceRecovery,
			model.CustomerBenefitTypeManual,
		}
	case model.CustomerBenefitTypePriceDiscount,
		model.CustomerBenefitTypePriceIncrease:
		return []string{
			model.CustomerBenefitTypePriceDiscount,
			model.CustomerBenefitTypePriceIncrease,
		}
	default:
		return []string{value}
	}
}

func scanCustomerBenefit(scanner scannable) (model.CustomerBenefit, error) {
	var benefit model.CustomerBenefit
	var createdAt string
	var extensionAppliedAt sql.NullString
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
		&benefit.PriceBeforeCents,
		&benefit.PriceAfterCents,
		&benefit.PriceEffectiveDueDate,
		&benefit.PriceAdjustmentKey,
		&benefit.RecommendationCode,
		&benefit.Note,
		&createdAt,
		&benefit.ExtensionDays,
		&extensionAppliedAt,
	); err != nil {
		return model.CustomerBenefit{}, err
	}
	parsed, err := parseTime(createdAt)
	if err != nil {
		return model.CustomerBenefit{}, fmt.Errorf("parse customer benefit created_at: %w", err)
	}
	benefit.CreatedAt = parsed
	if extensionAppliedAt.Valid {
		appliedAt, err := parseTime(extensionAppliedAt.String)
		if err != nil {
			return model.CustomerBenefit{}, err
		}
		benefit.ExtensionAppliedAt = &appliedAt
	}
	return benefit, nil
}
