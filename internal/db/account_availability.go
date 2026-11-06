package db

import (
	"database/sql"
	"strings"
	"time"

	"carpool-notify/internal/model"
)

// SetAccountPaused changes every space of the same owner together. The first
// write serializes this with account creation and subscription assignment.
func (store *Store) SetAccountPaused(accountID int64, paused bool) error {
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	now := formatTime(time.Now().UTC())
	result, err := transaction.Exec(`UPDATE accounts SET paused = ?, updated_at = ? WHERE id = ?`, paused, now, accountID)
	if err != nil {
		return seatStateWriteError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	accounts, err := readAccountPauseStates(transaction)
	if err != nil {
		return err
	}
	owner := ""
	for _, account := range accounts {
		if account.ID == accountID {
			owner = strings.ToLower(model.AccountGroupingEmail(account))
			break
		}
	}
	if owner != "" {
		for _, account := range accounts {
			if strings.ToLower(model.AccountGroupingEmail(account)) != owner || account.Paused == paused {
				continue
			}
			if _, err := transaction.Exec(`UPDATE accounts SET paused = ?, updated_at = ? WHERE id = ?`, paused, now, account.ID); err != nil {
				return seatStateWriteError(err)
			}
		}
	}
	return seatStateWriteError(transaction.Commit())
}

func readAccountPauseStates(transaction *sql.Tx) ([]model.Account, error) {
	rows, err := transaction.Query(`SELECT id, name, email, remark, paused FROM accounts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := make([]model.Account, 0)
	for rows.Next() {
		var account model.Account
		if err := rows.Scan(&account.ID, &account.Name, &account.Email, &account.Remark, &account.Paused); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

// syncAccountPauseGroups upgrades old single-space pauses and makes spaces
// created or moved into a paused owner's group inherit that pause atomically.
// Empty owner emails never link otherwise unrelated spaces.
func syncAccountPauseGroups(transaction *sql.Tx, now string) error {
	accounts, err := readAccountPauseStates(transaction)
	if err != nil {
		return err
	}
	pausedOwners := make(map[string]bool)
	for _, account := range accounts {
		if account.Paused {
			pausedOwners[strings.ToLower(model.AccountGroupingEmail(account))] = true
		}
	}
	for _, account := range accounts {
		owner := strings.ToLower(model.AccountGroupingEmail(account))
		if owner == "" || account.Paused || !pausedOwners[owner] {
			continue
		}
		if _, err := transaction.Exec(`UPDATE accounts SET paused = 1, updated_at = ? WHERE id = ?`, now, account.ID); err != nil {
			return seatStateWriteError(err)
		}
	}
	return nil
}

func (store *Store) alignAccountPauseGroups() error {
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	if err := syncAccountPauseGroups(transaction, formatTime(time.Now().UTC())); err != nil {
		return err
	}
	return seatStateWriteError(transaction.Commit())
}
