package db

import (
	"fmt"

	"carpool-notify/internal/model"
)

// SetAccountDisplaySerialAnchor commits only against the account snapshot used
// to validate the resulting numbers, including accounts added in the meantime.
func (store *Store) SetAccountDisplaySerialAnchor(accountID, serial int64, expected []model.Account) error {
	tx, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	byID := make(map[int64]model.Account, len(expected))
	for _, account := range expected {
		byID[account.ID] = account
	}
	rows, err := tx.Query(`SELECT id, updated_at, display_serial_anchor FROM accounts`)
	if err != nil {
		return err
	}
	count := 0
	for rows.Next() {
		var id, anchor int64
		var updatedAt string
		if err := rows.Scan(&id, &updatedAt, &anchor); err != nil {
			rows.Close()
			return err
		}
		account, exists := byID[id]
		version, err := parseTime(updatedAt)
		if err != nil {
			rows.Close()
			return err
		}
		if !exists || !account.UpdatedAt.Equal(version) || account.DisplaySerialAnchor != anchor {
			rows.Close()
			return fmt.Errorf("账号列表已变化，请刷新后重试")
		}
		count++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if count != len(expected) {
		return fmt.Errorf("账号列表已变化，请刷新后重试")
	}
	if _, exists := byID[accountID]; !exists {
		return fmt.Errorf("账号不存在")
	}
	if _, err := tx.Exec(`UPDATE accounts SET display_serial_anchor = ?, updated_at = ? WHERE id = ?`, serial, nextWriteTime(byID[accountID].UpdatedAt), accountID); err != nil {
		return err
	}
	return tx.Commit()
}
