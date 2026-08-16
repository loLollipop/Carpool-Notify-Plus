package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"carpool-notify/internal/db"
)

func TestOpenNormalizesLegacyBusinessGoalProfitBaselines(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy-goals.db")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE business_goals (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			target_profit_cents INTEGER NOT NULL,
			baseline_profit_cents INTEGER NOT NULL,
			result_profit_cents INTEGER NOT NULL DEFAULT 0,
			deadline TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			completed_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		INSERT INTO business_goals (
			name, target_profit_cents, baseline_profit_cents, result_profit_cents,
			status, completed_at, created_at, updated_at
		) VALUES
			('active legacy goal', 100000, 16000, 0, 'active', NULL,
			 '2026-08-01T00:00:00Z', '2026-08-01T00:00:00Z'),
			('completed legacy goal', 50000, 12000, 7000, 'completed',
			 '2026-08-10T00:00:00Z', '2026-08-01T00:00:00Z', '2026-08-10T00:00:00Z');
	`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := db.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	active, err := store.GetActiveBusinessGoal()
	if err != nil {
		t.Fatal(err)
	}
	if active.BaselineProfitCents != 0 || active.ResultProfitCents != 0 {
		t.Fatalf("active goal = %#v, want cleared baseline and unchanged result", active)
	}
	goals, err := store.ListBusinessGoals(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(goals) != 2 || goals[0].BaselineProfitCents != 0 || goals[0].ResultProfitCents != 19000 {
		t.Fatalf("normalized goals = %#v", goals)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopening must not add the old baseline a second time.
	store, err = db.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	goals, err = store.ListBusinessGoals(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(goals) != 2 || goals[0].ResultProfitCents != 19000 {
		t.Fatalf("goals after second migration = %#v", goals)
	}
}
