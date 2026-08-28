package db

import (
	"database/sql"
	"fmt"
	"time"

	"carpool-notify/internal/model"
)

func (store *Store) CreateBusinessGoal(goal model.BusinessGoal) (int64, error) {
	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
		INSERT INTO business_goals (
			name, target_profit_cents, baseline_profit_cents, result_profit_cents, deadline,
			status, completed_at, created_at, updated_at
		) VALUES (?, ?, ?, 0, ?, ?, NULL, ?, ?)`,
		goal.Name,
		goal.TargetProfitCents,
		goal.BaselineProfitCents,
		goal.Deadline,
		model.BusinessGoalStatusActive,
		now,
		now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (store *Store) UpdateBusinessGoal(goalID int64, name string, targetProfitCents int64) error {
	result, err := store.database.Exec(`
		UPDATE business_goals
		SET name = ?, target_profit_cents = ?, deadline = ?, updated_at = ?
		WHERE id = ? AND status = ?`,
		name,
		targetProfitCents,
		"",
		formatTime(time.Now().UTC()),
		goalID,
		model.BusinessGoalStatusActive,
	)
	if err != nil {
		return err
	}
	return requireAffectedGoal(result)
}

func (store *Store) CompleteBusinessGoal(goalID int64, resultProfitCents int64) error {
	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
		UPDATE business_goals
		SET status = ?, result_profit_cents = ?, completed_at = ?, updated_at = ?
		WHERE id = ? AND status = ?`,
		model.BusinessGoalStatusCompleted,
		resultProfitCents,
		now,
		now,
		goalID,
		model.BusinessGoalStatusActive,
	)
	if err != nil {
		return err
	}
	return requireAffectedGoal(result)
}

func requireAffectedGoal(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (store *Store) GetActiveBusinessGoal() (model.BusinessGoal, error) {
	row := store.database.QueryRow(`
		SELECT id, name, target_profit_cents, baseline_profit_cents, result_profit_cents, deadline,
		       status, completed_at, created_at, updated_at
		FROM business_goals
		WHERE status = ?
		ORDER BY id DESC
		LIMIT 1`,
		model.BusinessGoalStatusActive,
	)
	return scanBusinessGoal(row)
}

func (store *Store) ListBusinessGoals(limit int) ([]model.BusinessGoal, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := store.database.Query(`
		SELECT id, name, target_profit_cents, baseline_profit_cents, result_profit_cents, deadline,
		       status, completed_at, created_at, updated_at
		FROM business_goals
		ORDER BY id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	goals := make([]model.BusinessGoal, 0)
	for rows.Next() {
		goal, err := scanBusinessGoal(rows)
		if err != nil {
			return nil, err
		}
		goals = append(goals, goal)
	}
	return goals, rows.Err()
}

func (store *Store) InsertMarketPriceSnapshot(snapshot model.MarketPriceSnapshot) (int64, error) {
	createdAt := snapshot.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	result, err := store.database.Exec(`
		INSERT INTO market_price_snapshots (
			provider, product, low_price_cents, median_price_cents,
			high_price_cents, sample_count, source_updated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshot.Provider,
		snapshot.Product,
		snapshot.LowPriceCents,
		snapshot.MedianPriceCents,
		snapshot.HighPriceCents,
		snapshot.SampleCount,
		formatTime(snapshot.SourceUpdatedAt.UTC()),
		formatTime(createdAt.UTC()),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// InsertMarketPriceSnapshots stores one market refresh atomically. Acquisition
// and renewal benchmarks must never come from different refreshes because the
// goal center compares them side by side and uses them for different actions.
func (store *Store) InsertMarketPriceSnapshots(snapshots []model.MarketPriceSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	for _, snapshot := range snapshots {
		createdAt := snapshot.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}
		if _, err := transaction.Exec(`
			INSERT INTO market_price_snapshots (
				provider, product, low_price_cents, median_price_cents,
				high_price_cents, sample_count, source_updated_at, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			snapshot.Provider,
			snapshot.Product,
			snapshot.LowPriceCents,
			snapshot.MedianPriceCents,
			snapshot.HighPriceCents,
			snapshot.SampleCount,
			formatTime(snapshot.SourceUpdatedAt.UTC()),
			formatTime(createdAt.UTC()),
		); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

func (store *Store) LatestMarketPriceSnapshot(provider string, product string) (model.MarketPriceSnapshot, error) {
	row := store.database.QueryRow(`
		SELECT id, provider, product, low_price_cents, median_price_cents,
		       high_price_cents, sample_count, source_updated_at, created_at
		FROM market_price_snapshots
		WHERE provider = ? AND product = ?
		ORDER BY created_at DESC, id DESC
		LIMIT 1`, provider, product)
	return scanMarketPriceSnapshot(row)
}

func (store *Store) ListMarketPriceSnapshots(provider string, product string, limit int) ([]model.MarketPriceSnapshot, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	rows, err := store.database.Query(`
		SELECT id, provider, product, low_price_cents, median_price_cents,
		       high_price_cents, sample_count, source_updated_at, created_at
		FROM market_price_snapshots
		WHERE provider = ? AND product = ?
		ORDER BY created_at DESC, id DESC
		LIMIT ?`, provider, product, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	snapshots := make([]model.MarketPriceSnapshot, 0)
	for rows.Next() {
		snapshot, err := scanMarketPriceSnapshot(rows)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, rows.Err()
}

func (store *Store) ListAllAccountCostRecords() ([]model.AccountCostRecord, error) {
	rows, err := store.database.Query(`
		SELECT id, account_id, period_date, amount_cents, source, note, created_at
		FROM account_cost_records
		ORDER BY period_date ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]model.AccountCostRecord, 0)
	for rows.Next() {
		var record model.AccountCostRecord
		var createdAt string
		if err := rows.Scan(
			&record.ID,
			&record.AccountID,
			&record.PeriodDate,
			&record.AmountCents,
			&record.Source,
			&record.Note,
			&createdAt,
		); err != nil {
			return nil, err
		}
		parsed, err := parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse account cost created_at: %w", err)
		}
		record.CreatedAt = parsed
		records = append(records, record)
	}
	return records, rows.Err()
}

func scanBusinessGoal(scanner scannable) (model.BusinessGoal, error) {
	var goal model.BusinessGoal
	var completedAt sql.NullString
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(
		&goal.ID,
		&goal.Name,
		&goal.TargetProfitCents,
		&goal.BaselineProfitCents,
		&goal.ResultProfitCents,
		&goal.Deadline,
		&goal.Status,
		&completedAt,
		&createdAt,
		&updatedAt,
	); err != nil {
		return model.BusinessGoal{}, err
	}
	if completedAt.Valid && completedAt.String != "" {
		parsed, err := parseTime(completedAt.String)
		if err != nil {
			return model.BusinessGoal{}, err
		}
		goal.CompletedAt = &parsed
	}
	var err error
	goal.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.BusinessGoal{}, err
	}
	goal.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.BusinessGoal{}, err
	}
	return goal, nil
}

func scanMarketPriceSnapshot(scanner scannable) (model.MarketPriceSnapshot, error) {
	var snapshot model.MarketPriceSnapshot
	var sourceUpdatedAt string
	var createdAt string
	if err := scanner.Scan(
		&snapshot.ID,
		&snapshot.Provider,
		&snapshot.Product,
		&snapshot.LowPriceCents,
		&snapshot.MedianPriceCents,
		&snapshot.HighPriceCents,
		&snapshot.SampleCount,
		&sourceUpdatedAt,
		&createdAt,
	); err != nil {
		return model.MarketPriceSnapshot{}, err
	}
	var err error
	snapshot.SourceUpdatedAt, err = parseTime(sourceUpdatedAt)
	if err != nil {
		return model.MarketPriceSnapshot{}, err
	}
	snapshot.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.MarketPriceSnapshot{}, err
	}
	return snapshot, nil
}
