package db

import (
	"database/sql"
	"errors"
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

func readDueExtensionsFiltered(queryer extensionQueryer, subscriptionIDs []int64, includeRevised bool) ([]model.SubscriptionDueExtension, error) {
	query := `SELECT id, subscription_id, customer_benefit_id, base_due_date, extension_days,
		previous_effective_due_date, effective_due_date, created_at FROM subscription_due_extensions`
	args := make([]any, 0, len(subscriptionIDs))
	clauses := make([]string, 0, 2)
	if !includeRevised {
		clauses = append(clauses, `NOT EXISTS (SELECT 1 FROM subscription_due_extension_revisions AS revision WHERE revision.extension_id = subscription_due_extensions.id)`)
	}
	if len(subscriptionIDs) > 0 {
		clauses = append(clauses, `subscription_id IN (`+strings.TrimSuffix(strings.Repeat("?,", len(subscriptionIDs)), ",")+`)`)
		for _, id := range subscriptionIDs {
			args = append(args, id)
		}
	}
	if len(clauses) > 0 {
		query += ` WHERE ` + strings.Join(clauses, ` AND `)
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

func readDueExtensions(queryer extensionQueryer, subscriptionIDs []int64) ([]model.SubscriptionDueExtension, error) {
	return readDueExtensionsFiltered(queryer, subscriptionIDs, false)
}

// currentFirstUnpaidDueDate resolves the one logical billing boundary that is
// currently actionable. Checking IsDueDate alone is insufficient because an
// old effective date can also be a later occurrence of the base schedule.
func currentFirstUnpaidDueDate(
	queryer extensionQueryer,
	subscriptionID int64,
	schedule cycle.BillingSchedule,
	now time.Time,
) (string, bool, error) {
	rows, err := queryer.Query(`SELECT due_date FROM bills WHERE subscription_id = ? ORDER BY due_date`, subscriptionID)
	if err != nil {
		return "", false, err
	}
	paidDates := make([]string, 0)
	for rows.Next() {
		var dueDate string
		if err := rows.Scan(&dueDate); err != nil {
			_ = rows.Close()
			return "", false, err
		}
		paidDates = append(paidDates, strings.TrimSpace(dueDate))
	}
	readErr := rows.Err()
	_ = rows.Close()
	if readErr != nil {
		return "", false, readErr
	}

	history, err := readDueExtensionsFiltered(queryer, []int64{subscriptionID}, true)
	if err != nil {
		return "", false, err
	}
	// Pre-ledger subscriptions have no durable fact from which to recover an
	// old unpaid gap. Preserve their legacy schedule-membership behavior until
	// either a payment or an extension establishes that anchor.
	if len(paidDates) == 0 && len(history) == 0 {
		return "", false, nil
	}
	historicalBaseDate := ""
	if len(history) > 0 {
		historicalBaseDate = history[0].BaseDueDate
	}
	dueAt, err := schedule.FirstUnpaid(now, paidDates, historicalBaseDate)
	if err != nil {
		return "", false, err
	}
	return cycle.FormatDate(dueAt), true, nil
}

// IsCurrentFirstUnpaidDueDate verifies the current actionable billing boundary
// using both the payment ledger and extension audit history. Pre-ledger legacy
// subscriptions retain schedule-membership validation for compatibility.
func (store *Store) IsCurrentFirstUnpaidDueDate(subscription model.Subscription, dueDate string, now time.Time) (bool, error) {
	schedule, err := subscription.BillingSchedule()
	if err != nil {
		return false, err
	}
	valid, err := schedule.IsDueDate(dueDate)
	if err != nil || !valid {
		return valid, err
	}
	currentDueDate, anchored, err := currentFirstUnpaidDueDate(store.database, subscription.ID, schedule, now)
	if err != nil || !anchored {
		return valid, err
	}
	return strings.TrimSpace(dueDate) == currentDueDate, nil
}

func (store *Store) ListSubscriptionDueExtensions() ([]model.SubscriptionDueExtension, error) {
	return readDueExtensionsFiltered(store.database, nil, true)
}

func (store *Store) ListSubscriptionDueExtensionRevisions() ([]model.SubscriptionDueExtensionRevision, error) {
	rows, err := store.database.Query(`SELECT id, extension_id, action, COALESCE(replacement_extension_id, 0),
		previous_days, replacement_days, reason, operation_key, created_at
		FROM subscription_due_extension_revisions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	revisions := make([]model.SubscriptionDueExtensionRevision, 0)
	for rows.Next() {
		var revision model.SubscriptionDueExtensionRevision
		var createdAt string
		if err := rows.Scan(&revision.ID, &revision.ExtensionID, &revision.Action,
			&revision.ReplacementExtensionID, &revision.PreviousDays, &revision.ReplacementDays,
			&revision.Reason, &revision.OperationKey, &createdAt); err != nil {
			return nil, err
		}
		revision.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	return revisions, rows.Err()
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
	history, err := readDueExtensionsFiltered(store.database, ids, true)
	if err != nil {
		return nil, err
	}
	for _, event := range history {
		i := byID[event.SubscriptionID]
		if subscriptions[i].DueExtensionUnpaidBaseDate == "" {
			subscriptions[i].DueExtensionUnpaidBaseDate = event.BaseDueDate
		}
	}
	return subscriptions, nil
}

func (store *Store) subscriptionWithExtensions(scanner scannable) (model.Subscription, error) {
	subscription, err := scanSubscription(scanner)
	if err != nil {
		return subscription, err
	}
	subscription.DueExtensions, err = readDueExtensions(store.database, []int64{subscription.ID})
	if err != nil {
		return subscription, err
	}
	history, err := readDueExtensionsFiltered(store.database, []int64{subscription.ID}, true)
	if err == nil && len(history) > 0 {
		subscription.DueExtensionUnpaidBaseDate = history[0].BaseDueDate
	}
	return subscription, err
}

// CreateCustomerBenefitsAndExtendDueDates commits costs, extension facts, price
// boundary shifts and version changes together. Bills are only read.
func (store *Store) CreateCustomerBenefitsAndExtendDueDates(
	benefits []model.CustomerBenefit,
	expectations []DueExtensionExpectation,
	now time.Time,
	emails ...BusinessEmailBatch,
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
	if err := queueBusinessEmailBatches(transaction, emails...); err != nil {
		return err
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
	// Replays must inspect the immutable history as well as active events. A
	// revoked or superseded event is still the original successful application.
	events, err := readDueExtensionsFiltered(transaction, []int64{expectedSubscriptionID}, true)
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
	history, err := readDueExtensionsFiltered(transaction, []int64{subscription.ID}, true)
	if err != nil {
		return event, err
	}
	if len(history) > 0 {
		subscription.DueExtensionUnpaidBaseDate = history[0].BaseDueDate
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
	due, err := schedule.FirstUnpaid(now, paid, subscription.DueExtensionUnpaidBaseDate)
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
	previousNextPriceDate := subscription.NextPriceEffectiveDueDate
	nextPriceDate := previousNextPriceDate
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
	if subscription.NextPriceCents != nil && *subscription.NextPriceCents < subscription.PricePerPersonCents &&
		strings.TrimSpace(previousNextPriceDate) != strings.TrimSpace(nextPriceDate) {
		linked, err := movePriceDiscountBenefitEffectiveDateWithTransaction(
			transaction,
			subscription.ID,
			subscription.PricePerPersonCents,
			*subscription.NextPriceCents,
			previousNextPriceDate,
			nextPriceDate,
		)
		if err != nil {
			return event, err
		}
		if !linked {
			if err := syncPriceDiscountBenefitWithTransaction(
				transaction,
				subscription.ID,
				subscription.PricePerPersonCents,
				*subscription.NextPriceCents,
				nextPriceDate,
				cycle.FormatDate(now.In(cycle.Location)),
				now,
				"",
			); err != nil {
				return event, err
			}
		}
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
	currentDueDate, anchored, err := currentFirstUnpaidDueDate(transaction, subscriptionID, schedule, now)
	if err != nil {
		return err
	}
	rows, err := transaction.Query(`SELECT id, due_date, kind FROM notification_log
		WHERE subscription_id = ? AND due_date >= ? AND status <> ? AND kind IN (?, ?)`,
		subscriptionID, fromDueDate, model.NotificationStatusSuccess,
		model.NotificationKindScheduled, model.NotificationKindPriceIncreaseNotice)
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
		if anchored {
			valid = dueDate == currentDueDate
		}
		if kind == model.NotificationKindPriceIncreaseNotice {
			valid, _ = schedule.IsDueDate(dueDate)
			valid = valid && dueDate == nextPriceDate
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

type ReviseDueExtensionInput struct {
	BenefitID     int64
	ExtensionDays int
	Reason        string
	OperationKey  string
	Emails        func(ReviseDueExtensionResult) BusinessEmailBatch
}

type ReviseDueExtensionResult struct {
	ReplacementBenefitID int64
	Replayed             bool
	// Committed facts for the notification; replays intentionally leave these empty.
	SubscriptionID   int64
	CustomerEmail    string
	PreviousDays     int
	PreviousDueDate  string
	EffectiveDueDate string
}

// ReviseCustomerBenefitExtension atomically revokes or replaces the latest
// active extension. The original benefit and extension rows remain immutable.
func (store *Store) ReviseCustomerBenefitExtension(input ReviseDueExtensionInput, now time.Time) (result ReviseDueExtensionResult, operationErr error) {
	defer func() {
		if operationErr != nil {
			operationErr = subscriptionStateWriteError(operationErr)
		}
	}()
	transaction, err := store.database.Begin()
	if err != nil {
		return ReviseDueExtensionResult{}, subscriptionStateWriteError(err)
	}
	defer func() { _ = transaction.Rollback() }()

	action := model.DueExtensionRevisionRevoked
	if input.ExtensionDays > 0 {
		action = model.DueExtensionRevisionSuperseded
	}
	reason := strings.TrimSpace(input.Reason)
	operationKey := strings.TrimSpace(input.OperationKey)

	var existingAction, existingReason string
	var existingExtensionID int64
	var existingReplacementDays int
	var existingReplacementBenefitID int64
	err = transaction.QueryRow(`SELECT revision.action, revision.reason, revision.extension_id,
		revision.replacement_days, COALESCE(replacement.customer_benefit_id, 0)
		FROM subscription_due_extension_revisions AS revision
		LEFT JOIN subscription_due_extensions AS replacement ON replacement.id = revision.replacement_extension_id
		WHERE revision.operation_key = ?`, operationKey).Scan(
		&existingAction, &existingReason, &existingExtensionID, &existingReplacementDays, &existingReplacementBenefitID,
	)
	if err == nil {
		var targetExtensionID int64
		lookupErr := transaction.QueryRow(`SELECT id FROM subscription_due_extensions WHERE customer_benefit_id = ?`, input.BenefitID).Scan(&targetExtensionID)
		if lookupErr != nil {
			return ReviseDueExtensionResult{}, lookupErr
		}
		if existingAction != action || existingReason != reason || existingExtensionID != targetExtensionID ||
			existingReplacementDays != input.ExtensionDays {
			return ReviseDueExtensionResult{}, ErrExtensionRevisionOperationConflict
		}
		return ReviseDueExtensionResult{ReplacementBenefitID: existingReplacementBenefitID, Replayed: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ReviseDueExtensionResult{}, err
	}

	benefit, err := scanCustomerBenefit(transaction.QueryRow(`SELECT `+customerBenefitSelectColumns+`
		FROM customer_benefits WHERE id = ?`, input.BenefitID))
	if err != nil {
		return ReviseDueExtensionResult{}, err
	}
	if benefit.BenefitType != model.CustomerBenefitTypeExtension || benefit.ExtensionStatus == "" {
		return ReviseDueExtensionResult{}, fmt.Errorf("仅活动中的延期福利可以修改或撤回")
	}
	if benefit.ExtensionStatus != "active" {
		return ReviseDueExtensionResult{}, fmt.Errorf("该延期福利已%s，不能重复操作", map[string]string{"revoked": "撤回", "superseded": "被修改替代"}[benefit.ExtensionStatus])
	}
	var target model.SubscriptionDueExtension
	var targetCreatedAt string
	if err := transaction.QueryRow(`SELECT id, subscription_id, customer_benefit_id, base_due_date,
		extension_days, previous_effective_due_date, effective_due_date, created_at
		FROM subscription_due_extensions WHERE customer_benefit_id = ?`, input.BenefitID).Scan(
		&target.ID, &target.SubscriptionID, &target.CustomerBenefitID, &target.BaseDueDate,
		&target.ExtensionDays, &target.PreviousEffectiveDueDate, &target.EffectiveDueDate, &targetCreatedAt,
	); err != nil {
		return ReviseDueExtensionResult{}, err
	}

	activeEvents, err := readDueExtensions(transaction, []int64{target.SubscriptionID})
	if err != nil {
		return ReviseDueExtensionResult{}, err
	}
	if len(activeEvents) == 0 || activeEvents[len(activeEvents)-1].ID != target.ID {
		return ReviseDueExtensionResult{}, fmt.Errorf("只能操作该订阅最后一个活动延期")
	}

	subscription, err := scanSubscription(transaction.QueryRow(`SELECT `+subscriptionSelectColumns+`
		`+subscriptionFromJoin+` WHERE subscription.id = ?
		AND subscription.deleted_at IS NULL AND subscription.archived_at IS NULL
		AND subscription.cancellation_requested_at IS NULL
		AND COALESCE(subscription.cancellation_case_id, 0) = 0
		AND (subscription.seat_id = 0 OR NULLIF(TRIM(COALESCE(account.banned_at, '')), '') IS NULL)
		AND NOT EXISTS (SELECT 1 FROM after_sales_cases WHERE subscription_id = subscription.id AND status IN (?, ?))`,
		target.SubscriptionID, model.AfterSalesStatusPending, model.AfterSalesStatusReview))
	if errors.Is(err, sql.ErrNoRows) {
		return ReviseDueExtensionResult{}, fmt.Errorf("订阅正在取消、售后、归档、删除或账号已封禁，不能调整延期")
	}
	if err != nil {
		return ReviseDueExtensionResult{}, err
	}
	var pendingRenewal bool
	if err := transaction.QueryRow(`SELECT EXISTS(SELECT 1 FROM renewal_applications WHERE subscription_id = ? AND status = ?)`,
		subscription.ID, model.RenewalStatusPending).Scan(&pendingRenewal); err != nil {
		return ReviseDueExtensionResult{}, err
	}
	if pendingRenewal {
		return ReviseDueExtensionResult{}, fmt.Errorf("该订阅有待审核续费，请先处理后再调整延期")
	}
	var hasBill, hasSuccessNotification, hasAppliedPrice bool
	if err := transaction.QueryRow(`SELECT EXISTS(SELECT 1 FROM bills WHERE subscription_id = ? AND due_date >= ?)`,
		subscription.ID, target.PreviousEffectiveDueDate).Scan(&hasBill); err != nil {
		return ReviseDueExtensionResult{}, err
	}
	if hasBill {
		return ReviseDueExtensionResult{}, fmt.Errorf("受影响账期已有账单，不能调整延期")
	}
	if err := transaction.QueryRow(`SELECT EXISTS(SELECT 1 FROM notification_log
		WHERE subscription_id = ? AND due_date >= ? AND status = ? AND kind IN (?, ?))`,
		subscription.ID, target.PreviousEffectiveDueDate, model.NotificationStatusSuccess,
		model.NotificationKindScheduled, model.NotificationKindPriceIncreaseNotice).Scan(&hasSuccessNotification); err != nil {
		return ReviseDueExtensionResult{}, err
	}
	if hasSuccessNotification {
		return ReviseDueExtensionResult{}, fmt.Errorf("受影响账期已有成功通知，不能调整延期")
	}
	if err := transaction.QueryRow(`SELECT EXISTS(SELECT 1 FROM subscription_price_changes WHERE subscription_id = ? AND effective_due_date >= ?)`,
		subscription.ID, target.PreviousEffectiveDueDate).Scan(&hasAppliedPrice); err != nil {
		return ReviseDueExtensionResult{}, err
	}
	if hasAppliedPrice {
		return ReviseDueExtensionResult{}, fmt.Errorf("受影响账期已有落地调价记录，不能调整延期")
	}

	subscription.DueExtensions = activeEvents
	currentSchedule, err := subscription.BillingSchedule()
	if err != nil {
		return ReviseDueExtensionResult{}, err
	}
	candidateExtensions := make([]cycle.DueExtension, 0, len(activeEvents))
	for _, event := range activeEvents {
		if event.ID == target.ID {
			continue
		}
		candidateExtensions = append(candidateExtensions, cycle.DueExtension{BaseDueDate: event.BaseDueDate, ExtensionDays: event.ExtensionDays})
	}
	baseSchedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		return ReviseDueExtensionResult{}, err
	}
	withoutTargetSchedule, err := baseSchedule.WithExtensions(candidateExtensions)
	if err != nil {
		return ReviseDueExtensionResult{}, err
	}
	if input.ExtensionDays > 0 {
		candidateExtensions = append(candidateExtensions, cycle.DueExtension{BaseDueDate: target.BaseDueDate, ExtensionDays: input.ExtensionDays})
	}
	candidateSchedule, err := baseSchedule.WithExtensions(candidateExtensions)
	if err != nil {
		return ReviseDueExtensionResult{}, err
	}

	previousNextPriceDate := strings.TrimSpace(subscription.NextPriceEffectiveDueDate)
	nextPriceDate := previousNextPriceDate
	if subscription.NextPriceCents != nil && previousNextPriceDate != "" {
		priceBase, resolveErr := currentSchedule.BaseDueDate(previousNextPriceDate)
		if resolveErr != nil {
			return ReviseDueExtensionResult{}, fmt.Errorf("无法安全反解待生效调价账期，已拒绝调整延期: %w", resolveErr)
		}
		priceBaseAt, _ := time.ParseInLocation("2006-01-02", priceBase, cycle.Location)
		nextPriceDate = cycle.FormatDate(candidateSchedule.EffectiveDue(priceBaseAt))
	}

	replacementBenefitID := int64(0)
	replacementExtensionID := int64(0)
	if input.ExtensionDays > 0 {
		baseAt, _ := time.ParseInLocation("2006-01-02", target.BaseDueDate, cycle.Location)
		previousEffective := cycle.FormatDate(withoutTargetSchedule.EffectiveDue(baseAt))
		effective := cycle.FormatDate(candidateSchedule.EffectiveDue(baseAt))
		batchID := "extension-revision-v1:" + operationKey
		insertResult, insertErr := transaction.Exec(`INSERT INTO customer_benefits (
			batch_id, subscription_id, benefit_type, benefit_name, actual_cost_cents, perceived_value_cents,
			benefit_date, next_due_date_snapshot, customer_email_snapshot, customer_wechat_snapshot,
			customer_tier_snapshot, customer_group_size_snapshot, current_price_cents_snapshot,
			renewal_count_snapshot, price_before_cents, price_after_cents, price_effective_due_date,
			price_adjustment_key, recommendation_code, note, created_at)
		SELECT ?, subscription_id, benefit_type, ?, actual_cost_cents, perceived_value_cents,
			?, ?, customer_email_snapshot, customer_wechat_snapshot, customer_tier_snapshot,
			customer_group_size_snapshot, current_price_cents_snapshot, renewal_count_snapshot,
			price_before_cents, price_after_cents, price_effective_due_date, '', recommendation_code, note, ?
		FROM customer_benefits WHERE id = ?`, batchID, fmt.Sprintf("赠送延期 %d 天", input.ExtensionDays),
			cycle.FormatDate(now.In(cycle.Location)), effective, formatTime(now.UTC()), benefit.ID)
		if insertErr != nil {
			return ReviseDueExtensionResult{}, insertErr
		}
		replacementBenefitID, err = insertResult.LastInsertId()
		if err != nil {
			return ReviseDueExtensionResult{}, err
		}
		insertResult, err = transaction.Exec(`INSERT INTO subscription_due_extensions
			(subscription_id, customer_benefit_id, base_due_date, extension_days, previous_effective_due_date, effective_due_date, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, subscription.ID, replacementBenefitID, target.BaseDueDate,
			input.ExtensionDays, previousEffective, effective, formatTime(now.UTC()))
		if err != nil {
			return ReviseDueExtensionResult{}, err
		}
		replacementExtensionID, err = insertResult.LastInsertId()
		if err != nil {
			return ReviseDueExtensionResult{}, err
		}
	}

	if _, err := transaction.Exec(`INSERT INTO subscription_due_extension_revisions
		(extension_id, action, replacement_extension_id, previous_days, replacement_days, reason, operation_key, created_at)
		VALUES (?, ?, NULLIF(?, 0), ?, ?, ?, ?, ?)`, target.ID, action, replacementExtensionID,
		target.ExtensionDays, input.ExtensionDays, reason, operationKey, formatTime(now.UTC())); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
			return ReviseDueExtensionResult{}, ErrExtensionRevisionOperationConflict
		}
		return ReviseDueExtensionResult{}, err
	}
	if _, err := transaction.Exec(`UPDATE subscriptions SET next_price_effective_due_date = ?, updated_at = ? WHERE id = ?`,
		nextPriceDate, nextWriteTime(subscription.UpdatedAt), subscription.ID); err != nil {
		return ReviseDueExtensionResult{}, err
	}
	if subscription.NextPriceCents != nil && *subscription.NextPriceCents < subscription.PricePerPersonCents && previousNextPriceDate != nextPriceDate {
		linked, moveErr := movePriceDiscountBenefitEffectiveDateWithTransaction(transaction, subscription.ID,
			subscription.PricePerPersonCents, *subscription.NextPriceCents, previousNextPriceDate, nextPriceDate)
		if moveErr != nil {
			return ReviseDueExtensionResult{}, moveErr
		}
		if !linked {
			return ReviseDueExtensionResult{}, fmt.Errorf("无法安全同步关联的降价福利，已拒绝调整延期")
		}
	}
	if err := cancelInvalidDueNotifications(transaction, subscription.ID, target.PreviousEffectiveDueDate, nextPriceDate, candidateSchedule, now); err != nil {
		return ReviseDueExtensionResult{}, err
	}
	baseAt, _ := time.ParseInLocation("2006-01-02", target.BaseDueDate, cycle.Location)
	result = ReviseDueExtensionResult{
		ReplacementBenefitID: replacementBenefitID,
		SubscriptionID:       subscription.ID, CustomerEmail: subscription.CustomerEmail,
		PreviousDays: target.ExtensionDays, PreviousDueDate: target.EffectiveDueDate,
		EffectiveDueDate: cycle.FormatDate(candidateSchedule.EffectiveDue(baseAt)),
	}
	if input.Emails != nil {
		if err := queueBusinessEmailBatches(transaction, input.Emails(result)); err != nil {
			return ReviseDueExtensionResult{}, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return ReviseDueExtensionResult{}, subscriptionStateWriteError(err)
	}
	return result, nil
}
