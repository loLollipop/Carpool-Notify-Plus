package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

const automaticPriceDiscountRecommendation = "auto_next_price_discount"

func (store *Store) ensureCustomerBenefitPriceAdjustmentColumns() error {
	columns := []struct {
		name      string
		statement string
	}{
		{"price_before_cents", `ALTER TABLE customer_benefits ADD COLUMN price_before_cents INTEGER NOT NULL DEFAULT 0 CHECK(price_before_cents >= 0)`},
		{"price_after_cents", `ALTER TABLE customer_benefits ADD COLUMN price_after_cents INTEGER NOT NULL DEFAULT 0 CHECK(price_after_cents >= 0)`},
		{"price_effective_due_date", `ALTER TABLE customer_benefits ADD COLUMN price_effective_due_date TEXT NOT NULL DEFAULT ''`},
		{"price_adjustment_key", `ALTER TABLE customer_benefits ADD COLUMN price_adjustment_key TEXT NOT NULL DEFAULT ''`},
	}
	for _, column := range columns {
		hasColumn, err := store.tableHasColumn("customer_benefits", column.name)
		if err != nil {
			return err
		}
		if hasColumn {
			continue
		}
		if _, err := store.database.Exec(column.statement); err != nil {
			return fmt.Errorf("add customer_benefits.%s: %w", column.name, err)
		}
	}
	if _, err := store.database.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_customer_benefits_price_adjustment
		ON customer_benefits(price_adjustment_key)
		WHERE TRIM(price_adjustment_key) <> ''`); err != nil {
		return fmt.Errorf("create customer benefit price-adjustment index: %w", err)
	}
	// Structured price adjustments have their own stable unique key. Excluding
	// them from the legacy display-field index lets two real, separately dated
	// transitions share the same human-readable name without blocking startup
	// backfill.
	if _, err := store.database.Exec(`DROP INDEX IF EXISTS idx_customer_benefits_delivery`); err != nil {
		return fmt.Errorf("drop legacy customer benefit delivery index: %w", err)
	}
	if _, err := store.database.Exec(`
		CREATE UNIQUE INDEX idx_customer_benefits_delivery
		ON customer_benefits(subscription_id, benefit_date, benefit_type, benefit_name)
		WHERE TRIM(price_adjustment_key) = ''
		  AND NOT (benefit_type = 'extension' AND (
			batch_id LIKE 'benefit-operation-v1:%'
			OR batch_id LIKE 'extension-revision-v1:%'
		  ))`); err != nil {
		return fmt.Errorf("create customer benefit delivery index: %w", err)
	}
	if _, err := store.database.Exec(`
		CREATE TABLE IF NOT EXISTS scheduled_price_discount_history (
			subscription_id INTEGER NOT NULL,
			price_before_cents INTEGER NOT NULL,
			price_after_cents INTEGER NOT NULL,
			effective_due_date TEXT NOT NULL,
			scheduled_at TEXT NOT NULL,
			PRIMARY KEY(subscription_id, effective_due_date, price_before_cents, price_after_cents),
			FOREIGN KEY(subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE
		)`); err != nil {
		return fmt.Errorf("create scheduled price discount history: %w", err)
	}
	return nil
}

func priceAdjustmentKey(subscriptionID int64, effectiveDueDate string, before, after int64) string {
	return fmt.Sprintf(
		"price-adjustment-v1:%d:%s:%d:%d",
		subscriptionID,
		strings.TrimSpace(effectiveDueDate),
		before,
		after,
	)
}

func priceDiscountBenefitName(before, after int64) string {
	return fmt.Sprintf(
		"续费每期降价 ¥%s（¥%s → ¥%s）",
		cycle.FormatCents(before-after),
		cycle.FormatCents(before),
		cycle.FormatCents(after),
	)
}

func movePriceDiscountBenefitEffectiveDateWithTransaction(
	transaction *sql.Tx,
	subscriptionID int64,
	before int64,
	after int64,
	previousDueDate string,
	nextDueDate string,
) (bool, error) {
	previousDueDate = strings.TrimSpace(previousDueDate)
	nextDueDate = strings.TrimSpace(nextDueDate)
	if subscriptionID <= 0 || before <= after || previousDueDate == "" || nextDueDate == "" || previousDueDate == nextDueDate {
		return false, nil
	}
	if _, err := transaction.Exec(`
		UPDATE scheduled_price_discount_history
		SET effective_due_date = ?
		WHERE subscription_id = ?
		  AND price_before_cents = ?
		  AND price_after_cents = ?
		  AND effective_due_date = ?`,
		nextDueDate,
		subscriptionID,
		before,
		after,
		previousDueDate,
	); err != nil {
		return false, err
	}
	result, err := transaction.Exec(`
		UPDATE customer_benefits
		SET price_effective_due_date = ?,
		    price_adjustment_key = ?,
		    next_due_date_snapshot = ?
		WHERE price_adjustment_key = ?`,
		nextDueDate,
		priceAdjustmentKey(subscriptionID, nextDueDate, before, after),
		nextDueDate,
		priceAdjustmentKey(subscriptionID, previousDueDate, before, after),
	)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func deletePendingPriceDiscountBenefitWithTransaction(
	transaction *sql.Tx,
	subscriptionID int64,
	before int64,
	after int64,
	effectiveDueDate string,
) error {
	if subscriptionID <= 0 || before <= after || strings.TrimSpace(effectiveDueDate) == "" {
		return nil
	}
	if _, err := transaction.Exec(`
		DELETE FROM customer_benefits
		WHERE price_adjustment_key = ?`,
		priceAdjustmentKey(subscriptionID, effectiveDueDate, before, after),
	); err != nil {
		return err
	}
	_, err := transaction.Exec(`
		DELETE FROM scheduled_price_discount_history
		WHERE subscription_id = ?
		  AND price_before_cents = ?
		  AND price_after_cents = ?
		  AND effective_due_date = ?`,
		subscriptionID,
		before,
		after,
		strings.TrimSpace(effectiveDueDate),
	)
	return err
}

// revisePendingPriceDiscountBenefitWithTransaction keeps an already issued
// benefit attached to a mutable pending adjustment. Operator-entered business
// fields are deliberately left untouched; only the adjustment coordinates and
// the subscription identity snapshots are corrected in place.
func revisePendingPriceDiscountBenefitWithTransaction(
	transaction *sql.Tx,
	subscriptionID int64,
	previousBefore int64,
	previousAfter int64,
	previousEffectiveDueDate string,
	nextBefore int64,
	nextAfter *int64,
	nextEffectiveDueDate string,
	customerEmail string,
	customerWechat string,
) (bool, error) {
	previousEffectiveDueDate = strings.TrimSpace(previousEffectiveDueDate)
	previousKey := priceAdjustmentKey(
		subscriptionID,
		previousEffectiveDueDate,
		previousBefore,
		previousAfter,
	)
	var benefitID int64
	var recommendationCode string
	if err := transaction.QueryRow(`
		SELECT id, recommendation_code
		FROM customer_benefits
		WHERE price_adjustment_key = ?`, previousKey).Scan(
		&benefitID,
		&recommendationCode,
	); err != nil {
		if err == sql.ErrNoRows {
			if deleteErr := deletePendingPriceDiscountBenefitWithTransaction(
				transaction,
				subscriptionID,
				previousBefore,
				previousAfter,
				previousEffectiveDueDate,
			); deleteErr != nil {
				return false, deleteErr
			}
			return false, nil
		}
		return false, err
	}

	nextEffectiveDueDate = strings.TrimSpace(nextEffectiveDueDate)
	if nextAfter == nil || nextEffectiveDueDate == "" {
		if err := deletePendingPriceDiscountBenefitWithTransaction(
			transaction,
			subscriptionID,
			previousBefore,
			previousAfter,
			previousEffectiveDueDate,
		); err != nil {
			return false, err
		}
		return true, nil
	}
	if *nextAfter >= nextBefore {
		// An automatically inferred benefit disappears when the pending change
		// stops being a discount. An explicit operator-issued benefit remains as
		// audit evidence of what was communicated to the customer.
		if recommendationCode == automaticPriceDiscountRecommendation {
			if err := deletePendingPriceDiscountBenefitWithTransaction(
				transaction,
				subscriptionID,
				previousBefore,
				previousAfter,
				previousEffectiveDueDate,
			); err != nil {
				return false, err
			}
		}
		return true, nil
	}

	nextKey := priceAdjustmentKey(subscriptionID, nextEffectiveDueDate, nextBefore, *nextAfter)
	name := priceDiscountBenefitName(nextBefore, *nextAfter)
	if _, err := transaction.Exec(`
		UPDATE customer_benefits
		SET price_before_cents = ?,
		    price_after_cents = ?,
		    price_effective_due_date = ?,
		    price_adjustment_key = ?,
		    next_due_date_snapshot = ?,
		    current_price_cents_snapshot = ?,
		    customer_email_snapshot = ?,
		    customer_wechat_snapshot = ?,
		    benefit_name = CASE WHEN recommendation_code = ? THEN ? ELSE benefit_name END,
		    perceived_value_cents = CASE WHEN recommendation_code = ? THEN ? ELSE perceived_value_cents END
		WHERE id = ?`,
		nextBefore,
		*nextAfter,
		nextEffectiveDueDate,
		nextKey,
		nextEffectiveDueDate,
		nextBefore,
		strings.TrimSpace(customerEmail),
		strings.TrimSpace(customerWechat),
		automaticPriceDiscountRecommendation,
		name,
		automaticPriceDiscountRecommendation,
		nextBefore-*nextAfter,
		benefitID,
	); err != nil {
		return false, err
	}
	if _, err := transaction.Exec(`
		UPDATE scheduled_price_discount_history
		SET price_before_cents = ?,
		    price_after_cents = ?,
		    effective_due_date = ?
		WHERE subscription_id = ?
		  AND price_before_cents = ?
		  AND price_after_cents = ?
		  AND effective_due_date = ?`,
		nextBefore,
		*nextAfter,
		nextEffectiveDueDate,
		subscriptionID,
		previousBefore,
		previousAfter,
		previousEffectiveDueDate,
	); err != nil {
		return false, err
	}
	return true, nil
}

func recordScheduledPriceDiscountWithTransaction(
	transaction *sql.Tx,
	subscriptionID int64,
	before int64,
	after int64,
	effectiveDueDate string,
	scheduledAt time.Time,
) error {
	if subscriptionID <= 0 || before <= after || strings.TrimSpace(effectiveDueDate) == "" {
		return nil
	}
	if scheduledAt.IsZero() {
		scheduledAt = time.Now()
	}
	_, err := transaction.Exec(`
		INSERT INTO scheduled_price_discount_history (
			subscription_id, price_before_cents, price_after_cents,
			effective_due_date, scheduled_at
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(subscription_id, effective_due_date, price_before_cents, price_after_cents)
		DO NOTHING`,
		subscriptionID,
		before,
		after,
		strings.TrimSpace(effectiveDueDate),
		formatTime(scheduledAt.UTC()),
	)
	return err
}

func normalizedBenefitDate(value string, occurredAt time.Time) string {
	today := cycle.StartOfDay(time.Now().In(cycle.Location))
	date := strings.TrimSpace(value)
	parsed, err := time.ParseInLocation("2006-01-02", date, cycle.Location)
	if err != nil {
		date = cycle.FormatDate(occurredAt.In(cycle.Location))
		parsed, err = time.ParseInLocation("2006-01-02", date, cycle.Location)
	}
	if err != nil || cycle.StartOfDay(parsed).After(today) {
		return cycle.FormatDate(today)
	}
	return date
}

func normalizeBenefitIdentity(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "-", "--", "无", "暂无", "未知", "未填写", "none", "null", "n/a":
		return ""
	default:
		return normalized
	}
}

func syncPriceDiscountBenefitWithTransaction(
	transaction *sql.Tx,
	subscriptionID int64,
	before int64,
	after int64,
	effectiveDueDate string,
	benefitDate string,
	occurredAt time.Time,
	operationBatchID string,
) error {
	effectiveDueDate = strings.TrimSpace(effectiveDueDate)
	if subscriptionID <= 0 || after < 0 || before <= after || effectiveDueDate == "" {
		return nil
	}
	key := priceAdjustmentKey(subscriptionID, effectiveDueDate, before, after)
	var exists bool
	if err := transaction.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM customer_benefits WHERE price_adjustment_key = ?
		)`, key).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}

	var businessType string
	var isResale bool
	var customerEmail string
	var customerWechat string
	if err := transaction.QueryRow(`
		SELECT COALESCE(NULLIF(LOWER(TRIM(business_type)), ''), 'team'),
		       COALESCE(is_resale, 0),
		       COALESCE(customer_email, ''),
		       COALESCE(customer_wechat, '')
		FROM subscriptions
		WHERE id = ?`, subscriptionID).Scan(
		&businessType,
		&isResale,
		&customerEmail,
		&customerWechat,
	); err != nil {
		return err
	}
	if businessType != model.SubscriptionBusinessTeam || isResale {
		return nil
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	benefitDate = normalizedBenefitDate(benefitDate, occurredAt)

	name := priceDiscountBenefitName(before, after)
	result, err := transaction.Exec(`
		UPDATE customer_benefits
		SET price_before_cents = ?,
			price_after_cents = ?,
			price_effective_due_date = ?,
			price_adjustment_key = ?
		WHERE id = (
			SELECT id
			FROM customer_benefits
			WHERE subscription_id = ?
			  AND TRIM(price_adjustment_key) = ''
			  AND benefit_type IN (?, ?)
			  AND benefit_name = ?
			  AND current_price_cents_snapshot = ?
			  AND TRIM(next_due_date_snapshot) = ?
			ORDER BY id ASC
			LIMIT 1
		)`,
		before,
		after,
		effectiveDueDate,
		key,
		subscriptionID,
		model.CustomerBenefitTypePriceDiscount,
		model.CustomerBenefitTypePriceIncrease,
		name,
		before,
		effectiveDueDate,
	)
	if err != nil {
		return err
	}
	claimed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if claimed == 1 {
		return nil
	}

	emailIdentity := normalizeBenefitIdentity(customerEmail)
	wechatIdentity := normalizeBenefitIdentity(customerWechat)
	groupSize := 1
	var matchedGroupSize int
	if err := transaction.QueryRow(`
		SELECT COUNT(1)
		FROM subscriptions
		WHERE deleted_at IS NULL
		  AND archived_at IS NULL
		  AND COALESCE(NULLIF(LOWER(TRIM(business_type)), ''), 'team') = ?
		  AND COALESCE(is_resale, 0) = 0
		  AND (
			id = ?
			OR (? <> '' AND LOWER(TRIM(COALESCE(customer_email, ''))) = ?)
			OR (? <> '' AND LOWER(TRIM(COALESCE(customer_wechat, ''))) = ?)
		  )`,
		model.SubscriptionBusinessTeam,
		subscriptionID,
		emailIdentity,
		emailIdentity,
		wechatIdentity,
		wechatIdentity,
	).Scan(&matchedGroupSize); err != nil {
		return err
	}
	if matchedGroupSize > groupSize {
		groupSize = matchedGroupSize
	}
	var paidPeriods int
	if err := transaction.QueryRow(
		`SELECT COUNT(1) FROM bills WHERE subscription_id = ?`,
		subscriptionID,
	).Scan(&paidPeriods); err != nil {
		return err
	}
	renewalCount := paidPeriods - 1
	if renewalCount < 0 {
		renewalCount = 0
	}
	batchID := strings.TrimSpace(operationBatchID)
	if batchID == "" {
		batchID = "price-adjustment:" + key
	}
	_, err = transaction.Exec(`
		INSERT INTO customer_benefits (
			batch_id, subscription_id, benefit_type, benefit_name,
			actual_cost_cents, perceived_value_cents, benefit_date,
			next_due_date_snapshot, customer_email_snapshot,
			customer_wechat_snapshot, customer_tier_snapshot,
			customer_group_size_snapshot, current_price_cents_snapshot,
			renewal_count_snapshot, price_before_cents, price_after_cents,
			price_effective_due_date, price_adjustment_key,
			recommendation_code, note, created_at
		) VALUES (?, ?, ?, ?, 0, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(price_adjustment_key) WHERE TRIM(price_adjustment_key) <> '' DO NOTHING`,
		batchID,
		subscriptionID,
		model.CustomerBenefitTypePriceDiscount,
		name,
		before-after,
		benefitDate,
		effectiveDueDate,
		strings.TrimSpace(customerEmail),
		strings.TrimSpace(customerWechat),
		groupSize,
		before,
		renewalCount,
		before,
		after,
		effectiveDueDate,
		key,
		automaticPriceDiscountRecommendation,
		"由下期调价自动同步；未重复调整价格，实际成本按 0 记录",
		formatTime(occurredAt.UTC()),
	)
	return err
}

// backfillPriceDiscountBenefits links legacy popup-created rows when possible,
// then creates missing benefit facts for applied and still-pending discounts.
func (store *Store) backfillPriceDiscountBenefits() error {
	type adjustment struct {
		subscriptionID   int64
		before           int64
		after            int64
		effectiveDueDate string
		occurredAt       time.Time
	}
	adjustments := make([]adjustment, 0)
	rows, err := store.database.Query(`
		SELECT change.subscription_id,
		       change.previous_price_cents,
		       change.new_price_cents,
		       change.effective_due_date,
		       change.created_at
		FROM subscription_price_changes AS change
		JOIN subscriptions AS subscription ON subscription.id = change.subscription_id
		JOIN scheduled_price_discount_history AS scheduled
		  ON scheduled.subscription_id = change.subscription_id
		 AND scheduled.price_before_cents = change.previous_price_cents
		 AND scheduled.price_after_cents = change.new_price_cents
		 AND scheduled.effective_due_date = change.effective_due_date
		WHERE change.new_price_cents < change.previous_price_cents
		  AND COALESCE(NULLIF(LOWER(TRIM(subscription.business_type)), ''), 'team') = ?
		  AND COALESCE(subscription.is_resale, 0) = 0
		ORDER BY change.id ASC`, model.SubscriptionBusinessTeam)
	if err != nil {
		return fmt.Errorf("list historical price discounts: %w", err)
	}
	for rows.Next() {
		var candidate adjustment
		var createdAt string
		if err := rows.Scan(
			&candidate.subscriptionID,
			&candidate.before,
			&candidate.after,
			&candidate.effectiveDueDate,
			&createdAt,
		); err != nil {
			_ = rows.Close()
			return err
		}
		candidate.occurredAt, err = parseTime(createdAt)
		if err != nil {
			_ = rows.Close()
			return fmt.Errorf("parse historical price discount time: %w", err)
		}
		adjustments = append(adjustments, candidate)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	rows, err = store.database.Query(`
		SELECT id, price_per_person_cents, next_price_cents,
		       next_price_effective_due_date, updated_at
		FROM subscriptions
		WHERE next_price_cents IS NOT NULL
		  AND next_price_cents < price_per_person_cents
		  AND TRIM(next_price_effective_due_date) <> ''
		  AND COALESCE(NULLIF(LOWER(TRIM(business_type)), ''), 'team') = ?
		  AND COALESCE(is_resale, 0) = 0
		ORDER BY id ASC`, model.SubscriptionBusinessTeam)
	if err != nil {
		return fmt.Errorf("list pending price discounts: %w", err)
	}
	for rows.Next() {
		var candidate adjustment
		var updatedAt string
		if err := rows.Scan(
			&candidate.subscriptionID,
			&candidate.before,
			&candidate.after,
			&candidate.effectiveDueDate,
			&updatedAt,
		); err != nil {
			_ = rows.Close()
			return err
		}
		candidate.occurredAt, err = parseTime(updatedAt)
		if err != nil {
			_ = rows.Close()
			return fmt.Errorf("parse pending price discount time: %w", err)
		}
		adjustments = append(adjustments, candidate)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	for _, candidate := range adjustments {
		if err := syncPriceDiscountBenefitWithTransaction(
			transaction,
			candidate.subscriptionID,
			candidate.before,
			candidate.after,
			candidate.effectiveDueDate,
			cycle.FormatDate(candidate.occurredAt.In(cycle.Location)),
			candidate.occurredAt,
			"",
		); err != nil {
			return fmt.Errorf("backfill price discount benefit: %w", err)
		}
	}
	return transaction.Commit()
}

// SyncHistoricalPriceDiscountBenefit records one operator-confirmed historical
// scheduled discount. Legacy price-change rows do not carry a source, so this
// explicit path refuses to guess from bill amounts alone.
func (store *Store) SyncHistoricalPriceDiscountBenefit(
	subscriptionID int64,
	before int64,
	after int64,
	effectiveDueDate string,
) (bool, error) {
	effectiveDueDate = strings.TrimSpace(effectiveDueDate)
	if subscriptionID <= 0 || before <= after || after <= 0 {
		return false, fmt.Errorf("invalid historical price discount")
	}
	if _, err := time.ParseInLocation("2006-01-02", effectiveDueDate, cycle.Location); err != nil {
		return false, fmt.Errorf("invalid historical price discount due date: %w", err)
	}
	transaction, err := store.database.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = transaction.Rollback() }()
	var createdAtRaw string
	if err := transaction.QueryRow(`
		SELECT created_at
		FROM subscription_price_changes
		WHERE subscription_id = ?
		  AND previous_price_cents = ?
		  AND new_price_cents = ?
		  AND effective_due_date = ?`,
		subscriptionID,
		before,
		after,
		effectiveDueDate,
	).Scan(&createdAtRaw); err != nil {
		return false, err
	}
	createdAt, err := parseTime(createdAtRaw)
	if err != nil {
		return false, err
	}
	key := priceAdjustmentKey(subscriptionID, effectiveDueDate, before, after)
	var existed bool
	if err := transaction.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM customer_benefits WHERE price_adjustment_key = ?)`,
		key,
	).Scan(&existed); err != nil {
		return false, err
	}
	if err := recordScheduledPriceDiscountWithTransaction(
		transaction,
		subscriptionID,
		before,
		after,
		effectiveDueDate,
		createdAt,
	); err != nil {
		return false, err
	}
	if err := syncPriceDiscountBenefitWithTransaction(
		transaction,
		subscriptionID,
		before,
		after,
		effectiveDueDate,
		cycle.FormatDate(createdAt.In(cycle.Location)),
		createdAt,
		"",
	); err != nil {
		return false, err
	}
	if err := transaction.Commit(); err != nil {
		return false, err
	}
	return !existed, nil
}
