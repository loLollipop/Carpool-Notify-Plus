package db

import (
	"database/sql"
	"fmt"
	"time"

	"carpool-notify/internal/model"
)

// ListOperatingExpenses returns the owner-entered operating expense ledger,
// newest accounting date first.
func (store *Store) ListOperatingExpenses() ([]model.OperatingExpense, error) {
	rows, err := store.database.Query(`
		SELECT id, category, occurred_on, amount_cents, note, created_at, updated_at
		FROM operating_expenses
		ORDER BY occurred_on DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	expenses := make([]model.OperatingExpense, 0)
	for rows.Next() {
		expense, scanErr := scanOperatingExpense(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		expenses = append(expenses, expense)
	}
	return expenses, rows.Err()
}

func (store *Store) GetOperatingExpense(expenseID int64) (model.OperatingExpense, error) {
	return scanOperatingExpense(store.database.QueryRow(`
		SELECT id, category, occurred_on, amount_cents, note, created_at, updated_at
		FROM operating_expenses
		WHERE id = ?`, expenseID))
}

func (store *Store) CreateOperatingExpense(expense model.OperatingExpense) (int64, error) {
	now := time.Now().UTC()
	if expense.CreatedAt.IsZero() {
		expense.CreatedAt = now
	}
	if expense.UpdatedAt.IsZero() {
		expense.UpdatedAt = expense.CreatedAt
	}
	result, err := store.database.Exec(`
		INSERT INTO operating_expenses (
			category, occurred_on, amount_cents, note, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		expense.Category,
		expense.OccurredOn,
		expense.AmountCents,
		expense.Note,
		formatTime(expense.CreatedAt.UTC()),
		formatTime(expense.UpdatedAt.UTC()),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (store *Store) UpdateOperatingExpense(expense model.OperatingExpense) error {
	result, err := store.database.Exec(`
		UPDATE operating_expenses
		SET category = ?, occurred_on = ?, amount_cents = ?, note = ?, updated_at = ?
		WHERE id = ?`,
		expense.Category,
		expense.OccurredOn,
		expense.AmountCents,
		expense.Note,
		formatTime(time.Now().UTC()),
		expense.ID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (store *Store) DeleteOperatingExpense(expenseID int64) error {
	result, err := store.database.Exec(`DELETE FROM operating_expenses WHERE id = ?`, expenseID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanOperatingExpense(scanner scannable) (model.OperatingExpense, error) {
	var expense model.OperatingExpense
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(
		&expense.ID,
		&expense.Category,
		&expense.OccurredOn,
		&expense.AmountCents,
		&expense.Note,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.OperatingExpense{}, err
	}
	var err error
	expense.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.OperatingExpense{}, fmt.Errorf("parse operating expense created_at: %w", err)
	}
	expense.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.OperatingExpense{}, fmt.Errorf("parse operating expense updated_at: %w", err)
	}
	return expense, nil
}
