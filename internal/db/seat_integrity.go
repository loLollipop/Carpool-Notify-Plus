package db

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"modernc.org/sqlite"
)

var (
	ErrSeatReferenceMissing    = errors.New("seat reference missing")
	ErrSeatReferenced          = errors.New("seat is referenced")
	ErrSeatStateChanged        = errors.New("seat state changed during write")
	ErrAccountHistoryProtected = errors.New("为保留账单、成本及历史记录，账号不可删除；请使用封禁及隐藏流程")
)

// These constraints apply only to subsequent writes: legacy dangling references
// remain visible for diagnosis and never prevent a database from opening.
func (store *Store) ensureSeatReferenceTriggers() error {
	for _, statement := range []string{
		`CREATE TRIGGER IF NOT EXISTS require_subscription_seat_insert
		BEFORE INSERT ON subscriptions
		WHEN NEW.seat_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM seats WHERE id = NEW.seat_id)
		BEGIN SELECT RAISE(ABORT, 'seat reference missing'); END;`,
		`CREATE TRIGGER IF NOT EXISTS require_subscription_seat_update
		BEFORE UPDATE OF seat_id ON subscriptions
		WHEN NEW.seat_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM seats WHERE id = NEW.seat_id)
		BEGIN SELECT RAISE(ABORT, 'seat reference missing'); END;`,
		`CREATE TRIGGER IF NOT EXISTS require_subscription_seat_reactivation
		BEFORE UPDATE OF archived_at, deleted_at ON subscriptions
		WHEN NEW.seat_id IS NOT NULL
		 AND NEW.archived_at IS NULL
		 AND NEW.deleted_at IS NULL
		 AND NOT EXISTS (SELECT 1 FROM seats WHERE id = NEW.seat_id)
		BEGIN SELECT RAISE(ABORT, 'seat reference missing'); END;`,
		`CREATE TRIGGER IF NOT EXISTS preserve_referenced_seat
		BEFORE DELETE ON seats
		WHEN EXISTS (SELECT 1 FROM subscriptions WHERE seat_id = OLD.id)
		BEGIN SELECT RAISE(ABORT, 'seat is referenced'); END;`,
	} {
		if _, err := store.database.Exec(statement); err != nil {
			return fmt.Errorf("install seat reference constraint: %w", err)
		}
	}
	var count int
	if err := store.database.QueryRow(`SELECT COUNT(*) FROM subscriptions AS subscription
		WHERE seat_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM seats WHERE id = subscription.seat_id)`).Scan(&count); err != nil {
		return fmt.Errorf("diagnose missing seat references: %w", err)
	}
	if count > 0 {
		log.Printf("database contains %d historical subscription references to missing seats; preserved for investigation", count)
	}
	return nil
}

func seatReferenceError(err error) error {
	if err == nil {
		return nil
	}
	for _, sentinel := range []error{ErrSeatReferenceMissing, ErrSeatReferenced} {
		if strings.Contains(err.Error(), sentinel.Error()) {
			return sentinel
		}
	}
	return err
}

// seatStateWriteError translates SQLite write conflicts that can occur when a
// WAL read snapshot becomes stale before the transaction's first write. It is
// intentionally code-based: unrelated database errors containing "locked"
// must remain visible to callers.
func seatStateWriteError(err error) error {
	if err == nil {
		return nil
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code() {
		case 5, 517: // SQLITE_BUSY, SQLITE_BUSY_SNAPSHOT
			return fmt.Errorf("%w: %v", ErrSeatStateChanged, err)
		}
	}
	return err
}

// CountAllSubscriptionLinksBySeat includes soft-deleted history.
func (store *Store) CountAllSubscriptionLinksBySeat() (map[int64]int, error) {
	rows, err := store.database.Query(`SELECT seat_id, COUNT(*) FROM subscriptions WHERE seat_id IS NOT NULL GROUP BY seat_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[int64]int)
	for rows.Next() {
		var id int64
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, err
		}
		counts[id] = count
	}
	return counts, rows.Err()
}

func (store *Store) CountReferencedSeatsByAccount(accountID int64) (int, error) {
	var count int
	err := store.database.QueryRow(`SELECT COUNT(*) FROM seats WHERE account_id = ?
		AND EXISTS (SELECT 1 FROM subscriptions WHERE seat_id = seats.id)`, accountID).Scan(&count)
	return count, err
}
