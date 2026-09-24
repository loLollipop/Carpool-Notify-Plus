package db

import (
	"errors"
	"path/filepath"
	"testing"

	"carpool-notify/internal/model"
)

func integritySubscription() model.Subscription {
	return model.Subscription{Name: "history", CronExpr: "interval:30d", BoardedAt: "2026-09-01", NotifyOffsets: []int{0}}
}

func TestSeatReferenceConstraintsAndLegacyReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "integrity.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	accountID, err := store.CreateAccount(model.Account{Name: "owner"}, 1000, "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	seatID, err := store.CreateSeat(model.Seat{AccountID: accountID, Name: "seat"})
	if err != nil {
		t.Fatal(err)
	}
	sub := integritySubscription()
	sub.SeatID = seatID + 100
	if _, err := store.CreateSubscription(sub); !errors.Is(err, ErrSeatReferenceMissing) {
		t.Fatalf("insert error = %v", err)
	}
	sub.SeatID = seatID
	sub.ID, err = store.CreateSubscription(sub)
	if err != nil {
		t.Fatal(err)
	}
	sub.SeatID = seatID + 100
	if err := store.UpdateSubscription(sub); !errors.Is(err, ErrSeatReferenceMissing) {
		t.Fatalf("update error = %v", err)
	}
	for _, state := range []string{"active", "archived", "deleted"} {
		if state == "archived" {
			_, err = store.database.Exec(`UPDATE subscriptions SET archived_at = '2026-09-02T00:00:00Z' WHERE id = ?`, sub.ID)
		} else if state == "deleted" {
			_, err = store.database.Exec(`UPDATE subscriptions SET deleted_at = '2026-09-03T00:00:00Z' WHERE id = ?`, sub.ID)
		}
		if err != nil {
			t.Fatal(err)
		}
		if err := store.DeleteSeat(seatID); !errors.Is(err, ErrSeatReferenced) {
			t.Fatalf("%s delete error = %v", state, err)
		}
	}
	if err := store.DeleteAccount(accountID); !errors.Is(err, ErrAccountHistoryProtected) {
		t.Fatalf("account delete error = %v", err)
	}
	if _, err := store.database.Exec(`DROP TRIGGER require_subscription_seat_update`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.database.Exec(`UPDATE subscriptions SET seat_id = ? WHERE id = ?`, seatID+100, sub.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatalf("legacy dangling reference must not prevent startup: %v", err)
	}
	var gotSeat int64
	if err := store.database.QueryRow(`SELECT seat_id FROM subscriptions WHERE id = ?`, sub.ID).Scan(&gotSeat); err != nil {
		t.Fatal(err)
	}
	if gotSeat != seatID+100 {
		t.Fatalf("legacy reference changed: %d", gotSeat)
	}
	for _, trigger := range []string{"require_subscription_seat_insert", "require_subscription_seat_update", "require_subscription_seat_reactivation", "preserve_referenced_seat"} {
		var count int
		if err := store.database.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name = ?`, trigger).Scan(&count); err != nil || count != 1 {
			t.Fatalf("trigger %s count=%d err=%v", trigger, count, err)
		}
	}
	if _, err := store.database.Exec(`UPDATE subscriptions SET seat_id = seat_id WHERE id = ?`, sub.ID); !errors.Is(seatReferenceError(err), ErrSeatReferenceMissing) {
		t.Fatalf("restored update trigger error = %v", err)
	}
	if _, err := store.database.Exec(`UPDATE subscriptions SET archived_at = NULL, deleted_at = NULL WHERE id = ?`, sub.ID); !errors.Is(seatReferenceError(err), ErrSeatReferenceMissing) {
		t.Fatalf("reactivating a legacy dangling seat error = %v", err)
	}
}

func TestSeatDeleteAssignmentLinearizesAcrossConnections(t *testing.T) {
	for _, mode := range []string{"insert", "update"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "race.db")
			first, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer first.Close()
			second, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Close()
			accountID, err := first.CreateAccount(model.Account{Name: "owner"}, 0, "2026-09-01")
			if err != nil {
				t.Fatal(err)
			}
			for attempt := 0; attempt < 12; attempt++ {
				seatID, err := first.CreateSeat(model.Seat{AccountID: accountID, Name: "seat"})
				if err != nil {
					t.Fatal(err)
				}
				sub := integritySubscription()
				if mode == "update" {
					sub.ID, err = first.CreateSubscription(sub)
					if err != nil {
						t.Fatal(err)
					}
				}
				sub.SeatID = seatID
				start := make(chan struct{})
				deletion, assignment := make(chan error, 1), make(chan error, 1)
				go func() { <-start; deletion <- first.DeleteSeat(seatID) }()
				go func() {
					<-start
					if mode == "insert" {
						_, err := second.CreateSubscription(sub)
						assignment <- err
					} else {
						assignment <- second.UpdateSubscription(sub)
					}
				}()
				close(start)
				deleteErr, assignErr := <-deletion, <-assignment
				if !((deleteErr == nil && errors.Is(assignErr, ErrSeatReferenceMissing)) || (assignErr == nil && errors.Is(deleteErr, ErrSeatReferenced))) {
					t.Fatalf("delete=%v assignment=%v", deleteErr, assignErr)
				}
				var dangling int
				if err := first.database.QueryRow(`SELECT COUNT(*) FROM subscriptions WHERE seat_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM seats WHERE id = subscriptions.seat_id)`).Scan(&dangling); err != nil || dangling != 0 {
					t.Fatalf("dangling=%d err=%v", dangling, err)
				}
			}
		})
	}
}
