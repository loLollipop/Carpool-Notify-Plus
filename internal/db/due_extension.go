package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

type extensionQueryer interface {
	Query(string, ...any) (*sql.Rows, error)
}

// DueExtensionExpectation binds a delivery to the subscription version and
// unpaid boundary the operator actually reviewed before submitting it.
type DueExtensionExpectation struct {
	SubscriptionID           int64
	UpdatedAt                time.Time
	PreviousEffectiveDueDate string
}

func readDueExtensions(queryer extensionQueryer, subscriptionIDs []int64) ([]model.SubscriptionDueExtension, error) {
	query := `SELECT id, subscription_id, customer_benefit_id, base_due_date, extension_days,
		previous_effective_due_date, effective_due_date, created_at FROM subscription_due_extensions`
	args := make([]any, 0, len(subscriptionIDs))
	if len(subscriptionIDs) > 0 {
		query += ` WHERE subscription_id IN (` + strings.TrimSuffix(strings.Repeat("?,", len(subscriptionIDs)), ",") + `)`
		for _, id := range subscriptionIDs {
			args = append(args, id)
		}
	}
	rows, err := queryer.Query(query+` ORDER BY subscription_id, base_due_date, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.SubscriptionDueExtension, 0)
	for rows.Next() {
		var event model.SubscriptionDueExtension
		var createdAt string
		if err := rows.Scan(&event.ID, &event.SubscriptionID, &event.CustomerBenefitID,
			&event.BaseDueDate, &event.ExtensionDays, &event.PreviousEffectiveDueDate,
			&event.EffectiveDueDate, &createdAt); err != nil {
			return nil, err
		}
		event.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func (store *Store) ListSubscriptionDueExtensions() ([]model.SubscriptionDueExtension, error) {
	return readDueExtensions(store.database, nil)
}

func (store *Store) loadSubscriptionExtensions(subscriptions []model.Subscription) ([]model.Subscription, error) {
	if len(subscriptions) == 0 {
		return subscriptions, nil
	}
	ids := make([]int64, 0, len(subscriptions))
	byID := make(map[int64]int, len(subscriptions))
	for i := range subscriptions {
		ids = append(ids, subscriptions[i].ID)
		byID[subscriptions[i].ID] = i
	}
	events, err := readDueExtensions(store.database, ids)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		i := byID[event.SubscriptionID]
		subscriptions[i].DueExtensions = append(subscriptions[i].DueExtensions, event)
	}
	return subscriptions, nil
}

func (store *Store) subscriptionWithExtensions(scanner scannable) (model.Subscription, error) {
	subscription, err := scanSubscription(scanner)
	if err != nil {
		return subscription, err
	}
	subscription.DueExtensions, err = readDueExtensions(store.database, []int64{subscription.ID})
	return subscription, err
}

// CreateCustomerBenefitsAndExtendDueDates commits costs, extension facts, price
// boundary shifts and version changes together. Bills are only read.
func (store *Store) CreateCustomerBenefitsAndExtendDueDates(
	benefits []model.CustomerBenefit,
	expectations []DueExtensionExpectation,
	now time.Time,
) error {
	if len(benefits) == 0 {
		return nil
	}
	if len(benefits) != len(expectations) {
		return fmt.Errorf("customer benefit and extension expectation counts do not match")
	}
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	for index, benefit := range benefits {
		if benefit.BenefitType != model.CustomerBenefitTypeExtension ||
			!strings.HasPrefix(benefit.BatchID, "benefit-operation-v1:") || benefit.ExtensionDays < 1 || benefit.ExtensionDays > 365 ||
			expectations[index].SubscriptionID != benefit.SubscriptionID || expectations[index].UpdatedAt.IsZero() ||
			strings.TrimSpace(expectations[index].PreviousEffectiveDueDate) == "" {
			return fmt.Errorf("invalid structured extension benefit")
		}
	}
	if err := createCustomerBenefitsWithTransaction(transaction, benefits); err != nil {
		return err
	}
	for index, benefit := range benefits {
		if _, err := applyDueExtension(transaction, benefit, benefit.ExtensionDays, now, &expectations[index]); err != nil {
			return err
		}
	}
	return subscriptionStateWriteError(transaction.Commit())
}

// ApplyCustomerBenefitExtension explicitly repairs ONE known record, once.
// expectedSubscriptionID and expectedPreviousDueDate make operator approval
// concrete. Names are never parsed and no additional cost row is created.
// A matching replay returns the original event even after later renewals.
func (store *Store) ApplyCustomerBenefitExtension(benefitID, expectedSubscriptionID int64, days int,
	expectedPreviousDueDate string, now time.Time) (model.SubscriptionDueExtension, bool, error) {
	transaction, err := store.database.Begin()
	if err != nil {
		return model.SubscriptionDueExtension{}, false, err
	}
	defer func() { _ = transaction.Rollback() }()
	benefit, err := scanCustomerBenefit(transaction.QueryRow(`SELECT `+customerBenefitSelectColumns+` FROM customer_benefits WHERE id = ?`, benefitID))
	if err != nil {
		return model.SubscriptionDueExtension{}, false, err
	}
	if benefit.SubscriptionID != expectedSubscriptionID || expectedPreviousDueDate == "" || days < 1 || days > 365 ||
		(benefit.BenefitType != model.CustomerBenefitTypeExtension && benefit.BenefitType != model.CustomerBenefitTypeManual &&
			benefit.BenefitType != model.CustomerBenefitTypeRenewalMilestone && benefit.BenefitType != model.CustomerBenefitTypeLoyaltyCare &&
			benefit.BenefitType != model.CustomerBenefitTypeServiceRecovery) {
		return model.SubscriptionDueExtension{}, false, fmt.Errorf("福利补应用参数不匹配")
	}
	events, err := readDueExtensions(transaction, []int64{expectedSubscriptionID})
	if err != nil {
		return model.SubscriptionDueExtension{}, false, err
	}
	for _, event := range events {
		if event.CustomerBenefitID == benefitID {
			if event.ExtensionDays != days || event.PreviousEffectiveDueDate != expectedPreviousDueDate {
				return event, false, fmt.Errorf("该福利已按不同参数应用")
			}
			return event, false, nil
		}
	}
	event, err := applyDueExtension(transaction, benefit, days, now, nil)
	if err != nil {
		return event, false, err
	}
	if event.PreviousEffectiveDueDate != expectedPreviousDueDate {
		return model.SubscriptionDueExtension{}, false, ErrSubscriptionStateChanged
	}
	if err := transaction.Commit(); err != nil {
		return model.SubscriptionDueExtension{}, false, subscriptionStateWriteError(err)
	}
	return event, true, nil
}

func applyDueExtension(
	transaction *sql.Tx,
	benefit model.CustomerBenefit,
	days int,
	now time.Time,
	expectation *DueExtensionExpectation,
) (model.SubscriptionDueExtension, error) {
	var event model.SubscriptionDueExtension
	subscription, err := scanSubscription(transaction.QueryRow(`SELECT `+subscriptionSelectColumns+`
		`+subscriptionFromJoin+` WHERE subscription.id = ? AND subscription.deleted_at IS NULL
		AND subscription.archived_at IS NULL AND subscription.business_type = 'team'
		AND subscription.is_resale = 0 AND subscription.seat_id > 0
		AND account.id IS NOT NULL AND NULLIF(TRIM(COALESCE(account.banned_at, '')), '') IS NULL
		AND subscription.cancellation_requested_at IS NULL
		AND NOT EXISTS (SELECT 1 FROM after_sales_cases WHERE subscription_id = subscription.id AND status IN ('pending', 'review'))`, benefit.SubscriptionID))
	if err != nil {
		return event, err
	}
	if expectation != nil {
		if expectation.SubscriptionID != subscription.ID ||
			!subscription.UpdatedAt.Equal(expectation.UpdatedAt) {
			return event, ErrSubscriptionStateChanged
		}
	}
	var pending bool
	if err := transaction.QueryRow(`SELECT EXISTS(SELECT 1 FROM renewal_applications WHERE subscription_id = ? AND status = ?)`,
		subscription.ID, model.RenewalStatusPending).Scan(&pending); err != nil {
		return event, err
	}
	if pending {
		return event, fmt.Errorf("订阅 %d 有待审核续费，请先处理后再延期", subscription.ID)
	}
	subscription.DueExtensions, err = readDueExtensions(transaction, []int64{subscription.ID})
	if err != nil {
		return event, err
	}
	schedule, err := subscription.BillingSchedule()
	if err != nil {
		return event, err
	}
	rows, err := transaction.Query(`SELECT due_date FROM bills WHERE subscription_id = ? ORDER BY due_date`, subscription.ID)
	if err != nil {
		return event, err
	}
	paid := make([]string, 0)
	for rows.Next() {
		var date string
		if err := rows.Scan(&date); err != nil {
			_ = rows.Close()
			return event, err
		}
		paid = append(paid, date)
	}
	readErr := rows.Err()
	_ = rows.Close()
	if readErr != nil {
		return event, readErr
	}
	due, err := schedule.FirstUnpaid(now, paid)
	if err != nil {
		return event, err
	}
	previousDate := cycle.FormatDate(due)
	if expectation != nil && previousDate != strings.TrimSpace(expectation.PreviousEffectiveDueDate) {
		return event, ErrSubscriptionStateChanged
	}
	for _, date := range paid {
		if date >= previousDate {
			return event, fmt.Errorf("订阅 %d 的未付账期之后存在已付账单，请先处理乱序预付后再延期", subscription.ID)
		}
	}
	baseDate, err := schedule.BaseDueDate(previousDate)
	if err != nil {
		return event, err
	}
	nextPriceDate := subscription.NextPriceEffectiveDueDate
	if subscription.NextPriceCents != nil && nextPriceDate != "" {
		priceBase, err := schedule.BaseDueDate(nextPriceDate)
		if err != nil {
			return event, fmt.Errorf("调价日期不属于有效账期: %w", err)
		}
		if priceBase >= baseDate {
			priceDate, _ := time.ParseInLocation("2006-01-02", nextPriceDate, cycle.Location)
			nextPriceDate = cycle.FormatDate(priceDate.AddDate(0, 0, days))
		}
	}
	event = model.SubscriptionDueExtension{
		SubscriptionID: subscription.ID, CustomerBenefitID: benefit.ID, BaseDueDate: baseDate,
		ExtensionDays: days, PreviousEffectiveDueDate: previousDate,
		EffectiveDueDate: cycle.FormatDate(due.AddDate(0, 0, days)), CreatedAt: now.UTC(),
	}
	result, err := transaction.Exec(`INSERT INTO subscription_due_extensions
		(subscription_id, customer_benefit_id, base_due_date, extension_days, previous_effective_due_date, effective_due_date, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, event.SubscriptionID, event.CustomerBenefitID, event.BaseDueDate,
		event.ExtensionDays, event.PreviousEffectiveDueDate, event.EffectiveDueDate, formatTime(event.CreatedAt))
	if err != nil {
		return event, err
	}
	event.ID, err = result.LastInsertId()
	if err != nil {
		return event, err
	}
	if _, err := transaction.Exec(`UPDATE subscriptions SET next_price_effective_due_date = ?, updated_at = ? WHERE id = ?`,
		nextPriceDate, nextWriteTime(subscription.UpdatedAt), subscription.ID); err != nil {
		return event, err
	}
	// Logs already delivered remain historical evidence. Pending/retry work for
	// dates that disappeared is canceled atomically. A later base boundary can
	// become the newly extended due date, so a log whose date is still valid must
	// stay reusable instead of being permanently hidden by its unique key.
	subscription.DueExtensions = append(subscription.DueExtensions, event)
	effectiveSchedule, err := subscription.BillingSchedule()
	if err != nil {
		return event, err
	}
	if err := cancelInvalidDueNotifications(transaction, subscription.ID, previousDate, nextPriceDate, effectiveSchedule, now); err != nil {
		return event, err
	}
	return event, nil
}

func cancelInvalidDueNotifications(
	transaction *sql.Tx,
	subscriptionID int64,
	fromDueDate string,
	nextPriceDate string,
	schedule cycle.BillingSchedule,
	now time.Time,
) error {
	rows, err := transaction.Query(`SELECT id, due_date, kind FROM notification_log
		WHERE subscription_id = ? AND due_date >= ? AND status <> ?`,
		subscriptionID, fromDueDate, model.NotificationStatusSuccess)
	if err != nil {
		return err
	}
	invalidIDs := make([]int64, 0)
	for rows.Next() {
		var id int64
		var dueDate, kind string
		if err := rows.Scan(&id, &dueDate, &kind); err != nil {
			_ = rows.Close()
			return err
		}
		valid, _ := schedule.IsDueDate(dueDate)
		if kind == model.NotificationKindPriceIncreaseNotice && dueDate != nextPriceDate {
			valid = false
		}
		if !valid {
			invalidIDs = append(invalidIDs, id)
		}
	}
	readErr := rows.Err()
	_ = rows.Close()
	if readErr != nil {
		return readErr
	}
	for _, id := range invalidIDs {
		if _, err := transaction.Exec(`UPDATE notification_log
			SET status = ?, next_retry_at = NULL, updated_at = ? WHERE id = ?`,
			model.NotificationStatusCanceled, formatTime(now.UTC()), id); err != nil {
			return err
		}
	}
	return nil
}

func touchSubscriptionWithTransaction(transaction *sql.Tx, subscriptionID int64) error {
	var raw string
	if err := transaction.QueryRow(`SELECT updated_at FROM subscriptions WHERE id = ?`, subscriptionID).Scan(&raw); err != nil {
		return err
	}
	previous, err := parseTime(raw)
	if err != nil {
		return err
	}
	_, err = transaction.Exec(`UPDATE subscriptions SET updated_at = ? WHERE id = ?`, nextWriteTime(previous), subscriptionID)
	return err
}
