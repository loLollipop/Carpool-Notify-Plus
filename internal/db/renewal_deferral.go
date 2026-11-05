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

var ErrRenewalDeferred = errors.New("客户续费已搁置，请先恢复跟进；确认已手动移出请在搁置列表操作")

func ensureRenewalNotDeferred(tx *sql.Tx, subscriptionID int64) error {
	var deferred sql.NullString
	if err := tx.QueryRow(`SELECT renewal_deferred_at FROM subscriptions WHERE id = ?`, subscriptionID).Scan(&deferred); err != nil {
		return err
	}
	if deferred.Valid {
		return ErrRenewalDeferred
	}
	return nil
}

func (store *Store) ensureAvailabilityColumns() error {
	if _, err := store.ensureAccountColumn("paused", `ALTER TABLE accounts ADD COLUMN paused INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	for _, column := range []struct{ name, definition string }{
		{"renewal_deferred_at", "TEXT"},
		{"renewal_deferred_due_date", "TEXT NOT NULL DEFAULT ''"},
	} {
		exists, err := store.subscriptionsHasColumn(column.name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := store.database.Exec(`ALTER TABLE subscriptions ADD COLUMN ` + column.name + ` ` + column.definition); err != nil {
				return err
			}
		}
	}
	return nil
}

func (store *Store) SetAccountPaused(accountID int64, paused bool) error {
	result, err := store.database.Exec(`UPDATE accounts SET paused = ?, updated_at = ? WHERE id = ?`, paused, formatTime(time.Now().UTC()), accountID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return sql.ErrNoRows
	}
	return err
}

func ensureSeatAccountAvailable(queryer interface{ QueryRow(string, ...any) *sql.Row }, seatID int64) error {
	var paused bool
	var banned string
	if err := queryer.QueryRow(`SELECT account.paused, COALESCE(account.banned_at, '')
		FROM seats AS seat JOIN accounts AS account ON account.id = seat.account_id WHERE seat.id = ?`, seatID).Scan(&paused, &banned); err != nil {
		return err
	}
	if paused {
		return ErrAccountPaused
	}
	if strings.TrimSpace(banned) != "" {
		return ErrReplacementAccountBanned
	}
	return nil
}

// RenewalDeferralExpectation is the exact snapshot reviewed by the operator.
type RenewalDeferralExpectation struct {
	SubscriptionID int64
	UpdatedAt      time.Time
	DueDate        string
}

// readDeferrableSubscription reads lifecycle state and schedule history in the
// same transaction as the mutation. Account availability never changes whether
// an existing occupant can be followed up or confirmed removed.
func readDeferrableSubscription(tx *sql.Tx, id int64) (model.Subscription, error) {
	subscription, err := scanSubscription(tx.QueryRow(`SELECT `+subscriptionSelectColumns+subscriptionFromJoin+`
		WHERE subscription.id = ? AND subscription.deleted_at IS NULL AND subscription.archived_at IS NULL
		AND subscription.business_type = 'team' AND subscription.cancellation_requested_at IS NULL
		AND COALESCE(subscription.cancellation_case_id, 0) = 0
		AND NOT EXISTS (SELECT 1 FROM after_sales_cases WHERE subscription_id = subscription.id AND status IN ('pending', 'review'))`, id))
	if err != nil {
		return subscription, err
	}
	subscription.DueExtensions, err = readDueExtensions(tx, []int64{id})
	return subscription, err
}

func firstUnpaidForDeferral(tx *sql.Tx, subscription model.Subscription, now time.Time) (string, error) {
	schedule, err := subscription.BillingSchedule()
	if err != nil {
		return "", err
	}
	dueDate, anchored, err := currentFirstUnpaidDueDate(tx, subscription.ID, schedule, now)
	if err != nil || anchored {
		return dueDate, err
	}
	// A legacy row may be deferred before today's cron trigger without any
	// ledger history. Its saved unpaid boundary must survive tomorrow as well.
	due, err := schedule.FirstUnpaid(now, nil, subscription.RenewalDeferredDueDate)
	return cycle.FormatDate(due), err
}

func cancelDeferredNotifications(tx *sql.Tx, subscriptionID int64, now time.Time) error {
	_, err := tx.Exec(`UPDATE notification_log SET status = ?, next_retry_at = NULL, updated_at = ?
		WHERE subscription_id = ? AND status = ? AND kind IN (?, ?)`,
		model.NotificationStatusCanceled, formatTime(now.UTC()), subscriptionID, model.NotificationStatusPending,
		model.NotificationKindScheduled, model.NotificationKindPriceIncreaseNotice)
	return err
}

func (store *Store) DeferSubscriptionRenewal(expected RenewalDeferralExpectation, now time.Time) error {
	tx, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	subscription, err := readDeferrableSubscription(tx, expected.SubscriptionID)
	if err != nil {
		return err
	}
	if !subscription.UpdatedAt.Equal(expected.UpdatedAt) || subscription.RenewalDeferredAt != nil {
		return ErrSubscriptionStateChanged
	}
	dueDate, err := firstUnpaidForDeferral(tx, subscription, now)
	if err != nil {
		return err
	}
	if dueDate != expected.DueDate || dueDate > cycle.FormatDate(now) {
		return ErrSubscriptionStateChanged
	}
	if _, err := tx.Exec(`UPDATE subscriptions SET renewal_deferred_at = ?, renewal_deferred_due_date = ?, updated_at = ? WHERE id = ?`,
		formatTime(now.UTC()), dueDate, nextWriteTime(subscription.UpdatedAt), subscription.ID); err != nil {
		return subscriptionStateWriteError(err)
	}
	if err := cancelDeferredNotifications(tx, subscription.ID, now); err != nil {
		return err
	}
	return subscriptionStateWriteError(tx.Commit())
}

// Restore is idempotent and changes no billing or seat state.
func (store *Store) RestoreSubscriptionRenewal(subscriptionID int64) error {
	tx, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	subscription, err := readDeferrableSubscription(tx, subscriptionID)
	if err != nil {
		return err
	}
	if subscription.RenewalDeferredAt == nil {
		return nil
	}
	// Keep the unpaid date as an anchor until the ledger changes. In particular,
	// a legacy customer without any bills must not skip the overdue period on resume.
	if _, err := tx.Exec(`UPDATE subscriptions SET renewal_deferred_at = NULL WHERE id = ?`, subscriptionID); err != nil {
		return err
	}
	if err := touchSubscriptionWithTransaction(tx, subscriptionID); err != nil {
		return err
	}
	return subscriptionStateWriteError(tx.Commit())
}

func clearRenewalDeferralWithTransaction(tx *sql.Tx, subscriptionID int64) error {
	_, err := tx.Exec(`UPDATE subscriptions SET renewal_deferred_at = NULL, renewal_deferred_due_date = '' WHERE id = ?`, subscriptionID)
	return err
}

// CompleteDeferredSubscriptions confirms external removals atomically. The
// original ledger, including future paid periods, is retained without refunds.
func (store *Store) CompleteDeferredSubscriptions(expected []RenewalDeferralExpectation, now time.Time, freezeDays int) error {
	if len(expected) == 0 {
		return fmt.Errorf("请选择已手动移出的客户")
	}
	tx, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	seen := make(map[int64]bool, len(expected))
	for _, item := range expected {
		if seen[item.SubscriptionID] {
			return fmt.Errorf("不能重复选择同一客户")
		}
		seen[item.SubscriptionID] = true
		subscription, err := readDeferrableSubscription(tx, item.SubscriptionID)
		if err != nil {
			return err
		}
		if !subscription.UpdatedAt.Equal(item.UpdatedAt) || subscription.RenewalDeferredAt == nil {
			return ErrSubscriptionStateChanged
		}
		dueDate, err := firstUnpaidForDeferral(tx, subscription, now)
		if err != nil {
			return err
		}
		if dueDate != subscription.RenewalDeferredDueDate || dueDate > cycle.FormatDate(now) {
			return ErrSubscriptionStateChanged
		}
		freezeUntil, err := teamSeatFreezeDeadline(tx, subscription.ID, now, now.AddDate(0, 0, freezeDays))
		if err != nil {
			return err
		}
		if err := archiveSubscriptionInTransaction(tx, subscription.ID, formatTime(now.UTC()), 0, freezeUntil); err != nil {
			return subscriptionStateWriteError(err)
		}
		if err := cancelDeferredNotifications(tx, subscription.ID, now); err != nil {
			return err
		}
	}
	return subscriptionStateWriteError(tx.Commit())
}
