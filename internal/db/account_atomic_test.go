package db

import (
	"errors"
	"path/filepath"
	"testing"

	"carpool-notify/internal/model"
)

func TestCreateAccountWithSeatsRollsBackOnSeatFailure(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "create-account-atomic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.database.Exec(`
		CREATE TRIGGER reject_second_initial_seat
		BEFORE INSERT ON seats
		WHEN NEW.name = '车佉2'
		BEGIN SELECT RAISE(ABORT, 'test seat insert failure'); END;`); err != nil {
		t.Fatal(err)
	}

	_, err = store.CreateAccountWithSeats(
		model.Account{Name: "atomic", CostCents: 2500},
		2500,
		"2026-09-01",
		[]string{"车佉1", "车佉2", "车佉3"},
	)
	if err == nil {
		t.Fatal("CreateAccountWithSeats unexpectedly succeeded")
	}
	assertTableCounts(t, store, map[string]int{
		"accounts":             0,
		"account_cost_records": 0,
		"seats":                0,
	})
}

func TestUpdateAccountWithSeatCountRollsBackPriorSeatDeleteAndMetadata(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "update-account-atomic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	accountID, err := store.CreateAccountWithSeats(
		model.Account{Name: "before", Remark: "original", CostCents: 1000},
		1000,
		"2026-09-01",
		[]string{"车佉1", "车佉2", "车佉3"},
	)
	if err != nil {
		t.Fatal(err)
	}
	// Shrinking to one deletes 车佉3 first, then reaches this failure. The
	// earlier delete and all account/cost changes must be rolled back together.
	if _, err := store.database.Exec(`
		CREATE TRIGGER reject_second_seat_delete
		BEFORE DELETE ON seats
		WHEN OLD.name = '车佉2'
		BEGIN SELECT RAISE(ABORT, 'test seat delete failure'); END;`); err != nil {
		t.Fatal(err)
	}

	err = store.UpdateAccountWithSeatCount(model.Account{
		ID: accountID, Name: "after", Remark: "changed", CostCents: 3200,
	}, 1)
	if err == nil {
		t.Fatal("UpdateAccountWithSeatCount unexpectedly succeeded")
	}
	account, getErr := store.GetAccount(accountID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if account.Name != "before" || account.Remark != "original" || account.CostCents != 1000 {
		t.Fatalf("account changed after rollback: %#v", account)
	}
	records, listErr := store.ListAccountCostRecords(accountID)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(records) != 1 || records[0].AmountCents != 1000 {
		t.Fatalf("cost records changed after rollback: %#v", records)
	}
	seats, listErr := store.ListSeatsByAccount(accountID)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(seats) != 3 || seats[0].Name != "车佉1" || seats[1].Name != "车佉2" || seats[2].Name != "车佉3" {
		t.Fatalf("seats changed after rollback: %#v", seats)
	}
}

func TestWALSeatSnapshotWriteConflictIsMapped(t *testing.T) {
	if mapped := seatStateWriteError(errors.New("database is locked")); errors.Is(mapped, ErrSeatStateChanged) {
		t.Fatalf("plain lock text must not be classified as a SQLite write conflict: %v", mapped)
	}
	for _, operation := range []string{"redemption-insert", "after-sales-update"} {
		t.Run(operation, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "snapshot.db")
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
			seatID, err := first.CreateSeat(model.Seat{AccountID: accountID, Name: "target"})
			if err != nil {
				t.Fatal(err)
			}

			var subscriptionID int64
			if operation == "after-sales-update" {
				currentSeatID, createErr := first.CreateSeat(model.Seat{AccountID: accountID, Name: "current"})
				if createErr != nil {
					t.Fatal(createErr)
				}
				sub := integritySubscription()
				sub.SeatID = currentSeatID
				subscriptionID, err = first.CreateSubscription(sub)
				if err != nil {
					t.Fatal(err)
				}
			}

			transaction, err := first.database.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer transaction.Rollback()
			var seenAccountID int64
			if err := transaction.QueryRow(`SELECT account_id FROM seats WHERE id = ?`, seatID).Scan(&seenAccountID); err != nil {
				t.Fatal(err)
			}
			deleted := make(chan error, 1)
			go func() { deleted <- second.DeleteSeat(seatID) }()
			if err := <-deleted; err != nil {
				t.Fatal(err)
			}

			var writeErr error
			if operation == "redemption-insert" {
				sub := integritySubscription()
				sub.SeatID = seatID
				_, writeErr = insertSubscription(transaction, sub, "2026-09-24T00:00:00Z")
			} else {
				_, writeErr = transaction.Exec(`UPDATE subscriptions SET seat_id = ? WHERE id = ?`, seatID, subscriptionID)
			}
			writeErr = seatStateWriteError(writeErr)
			if !errors.Is(writeErr, ErrSeatStateChanged) {
				t.Fatalf("write error = %v, want ErrSeatStateChanged", writeErr)
			}
			if err := transaction.Rollback(); err != nil {
				t.Fatal(err)
			}
			var dangling int
			if err := first.database.QueryRow(`
				SELECT COUNT(*) FROM subscriptions
				WHERE seat_id IS NOT NULL
				  AND NOT EXISTS (SELECT 1 FROM seats WHERE id = subscriptions.seat_id)`).Scan(&dangling); err != nil {
				t.Fatal(err)
			}
			if dangling != 0 {
				t.Fatalf("dangling subscriptions = %d", dangling)
			}
		})
	}
}

func TestSQLiteBusyWriteConflictIsMappedByCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "busy.db")
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
	if _, err := first.database.Exec(`PRAGMA busy_timeout = 0`); err != nil {
		t.Fatal(err)
	}

	writer, err := second.database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if _, err := writer.Exec(`UPDATE accounts SET remark = 'writer' WHERE id = ?`, accountID); err != nil {
		t.Fatal(err)
	}
	_, err = first.database.Exec(`UPDATE accounts SET remark = 'contender' WHERE id = ?`, accountID)
	if mapped := seatStateWriteError(err); !errors.Is(mapped, ErrSeatStateChanged) {
		t.Fatalf("write error = %v, mapped = %v, want ErrSeatStateChanged", err, mapped)
	}
}

func assertTableCounts(t *testing.T, store *Store, expected map[string]int) {
	t.Helper()
	for table, want := range expected {
		var got int
		if err := store.database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s count = %d, want %d", table, got, want)
		}
	}
}
