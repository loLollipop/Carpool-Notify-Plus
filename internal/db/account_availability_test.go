package db

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"carpool-notify/internal/model"
)

func pauseTestAccount(t *testing.T, store *Store, account model.Account) int64 {
	t.Helper()
	id, err := store.CreateAccountWithSeats(account, 0, "2026-09-01", []string{"free"})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func assertAccountPause(t *testing.T, store *Store, id int64, want bool) model.Account {
	t.Helper()
	account, err := store.GetAccount(id)
	if err != nil || account.Paused != want {
		t.Fatalf("account %d paused = %v, want %v, err %v", id, account.Paused, want, err)
	}
	return account
}

func TestEmailGroupPauseAndResumeFromAnySpace(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "group.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ids := []int64{
		pauseTestAccount(t, store, model.Account{Name: "primary", Email: " Owner@Example.com ", Remark: "unrelated@example.com(11)"}),
		pauseTestAccount(t, store, model.Account{Name: "secondary", Email: "owner@example.com"}),
		pauseTestAccount(t, store, model.Account{Name: "OWNER@example.com"}),
		pauseTestAccount(t, store, model.Account{Name: "legacy", Remark: "owner@example.com（47）"}),
	}
	unrelated := pauseTestAccount(t, store, model.Account{Name: "source", Email: "unrelated@example.com"})
	before := assertAccountPause(t, store, unrelated, false)
	if _, err := store.database.Exec(`UPDATE accounts SET banned_at = '2026-09-02', ban_note = 'keep ban' WHERE id = ?`, ids[1]); err != nil {
		t.Fatal(err)
	}
	for _, target := range ids {
		for _, paused := range []bool{true, false} {
			if err := store.SetAccountPaused(target, paused); err != nil {
				t.Fatal(err)
			}
			for _, id := range ids {
				assertAccountPause(t, store, id, paused)
			}
			if after := assertAccountPause(t, store, unrelated, false); !reflect.DeepEqual(before, after) {
				t.Fatal("unrelated account changed")
			}
		}
	}
	banned := assertAccountPause(t, store, ids[1], false)
	if banned.BannedAt != "2026-09-02" || banned.BanNote != "keep ban" {
		t.Fatal("resume removed ban")
	}
	if err := store.SetAccountPaused(99999, true); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing account: %v", err)
	}
	a := pauseTestAccount(t, store, model.Account{Name: "no-email"})
	b := pauseTestAccount(t, store, model.Account{Name: "no-email"})
	if err := store.SetAccountPaused(a, true); err != nil {
		t.Fatal(err)
	}
	assertAccountPause(t, store, a, true)
	assertAccountPause(t, store, b, false)
}

func TestEmailGroupPauseRollsBackTogether(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "atomic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := pauseTestAccount(t, store, model.Account{Name: "a", Email: "owner@example.com"})
	b := pauseTestAccount(t, store, model.Account{Name: "b", Email: "owner@example.com"})
	before, _ := store.ListAccounts()
	if _, err := store.database.Exec(fmt.Sprintf(`CREATE TRIGGER reject_group_pause BEFORE UPDATE OF paused ON accounts WHEN NEW.id = %d BEGIN SELECT RAISE(ABORT, 'test group pause failure'); END`, b)); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAccountPaused(a, true); err == nil {
		t.Fatal("group pause unexpectedly succeeded")
	}
	after, _ := store.ListAccounts()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed group pause changed a member")
	}
}

func TestEmailGroupPauseUpgradeAndInheritance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a := pauseTestAccount(t, store, model.Account{Name: "primary", Email: "Owner@example.com"})
	b := pauseTestAccount(t, store, model.Account{Name: "secondary", Email: "owner@example.com"})
	c := pauseTestAccount(t, store, model.Account{Name: "unrelated", Email: "other@example.com"})
	blankA := pauseTestAccount(t, store, model.Account{Name: "blank a"})
	blankB := pauseTestAccount(t, store, model.Account{Name: "blank b"})
	// Reproduce the old release: only one physical space was paused.
	if _, err := store.database.Exec(`UPDATE accounts SET paused = 1 WHERE id IN (?, ?)`, a, blankA); err != nil {
		t.Fatal(err)
	}
	before, _ := store.GetAccount(a)
	unrelated, _ := store.GetAccount(c)
	seatsBefore, _ := store.ListAllSeats()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	assertAccountPause(t, store, a, true)
	assertAccountPause(t, store, b, true)
	assertAccountPause(t, store, blankA, true)
	assertAccountPause(t, store, blankB, false)
	if after := assertAccountPause(t, store, a, true); !reflect.DeepEqual(before, after) {
		t.Fatal("upgrade rewrote already paused account")
	}
	if after := assertAccountPause(t, store, c, false); !reflect.DeepEqual(unrelated, after) {
		t.Fatal("upgrade changed unrelated account")
	}
	seatsAfter, _ := store.ListAllSeats()
	if !reflect.DeepEqual(seatsBefore, seatsAfter) {
		t.Fatal("upgrade changed seats")
	}
	accountsBefore, _ := store.ListAccounts()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	accountsAfter, _ := store.ListAccounts()
	if !reflect.DeepEqual(accountsBefore, accountsAfter) {
		t.Fatal("second startup changed normalized accounts")
	}
	newID := pauseTestAccount(t, store, model.Account{Name: "new secondary", Email: "OWNER@example.com"})
	assertAccountPause(t, store, newID, true)
	joining := assertAccountPause(t, store, c, false)
	joining.Email = "owner@example.com"
	if err := store.UpdateAccount(joining); err != nil {
		t.Fatal(err)
	}
	assertAccountPause(t, store, c, true)
	if err := store.SetAccountPaused(newID, false); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{a, b, c, newID} {
		assertAccountPause(t, store, id, false)
	}
	assertAccountPause(t, store, blankA, true)
}

func TestNewEmailGroupSpaceRollsBackIfPauseInheritanceFails(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "inherit-atomic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := pauseTestAccount(t, store, model.Account{Name: "primary", Email: "owner@example.com"})
	if err := store.SetAccountPaused(a, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.database.Exec(`CREATE TRIGGER reject_inherited_pause BEFORE UPDATE OF paused ON accounts WHEN NEW.name = 'new secondary' BEGIN SELECT RAISE(ABORT, 'test inherit failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateAccountWithSeats(model.Account{Name: "new secondary", Email: "owner@example.com"}, 0, "2026-09-01", []string{"free"})
	if err == nil {
		t.Fatal("unpaused secondary was created")
	}
	assertTableCounts(t, store, map[string]int{"accounts": 1, "seats": 1, "account_cost_records": 1})
}
