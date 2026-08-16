package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenAddsNextPriceColumnsToLegacySubscriptions(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy-next-price.db")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			price_per_person_cents INTEGER NOT NULL,
			cron_expr TEXT NOT NULL,
			notify_offsets TEXT NOT NULL,
			channels TEXT NOT NULL,
			remark TEXT NOT NULL DEFAULT '',
			trade_url TEXT NOT NULL DEFAULT '',
			deleted_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	store := openStore(t, databasePath)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	rows, err := database.Query(`PRAGMA table_info(subscriptions)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var columnID int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&columnID, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == "next_price_cents" || name == "next_price_effective_due_date" {
			found[name] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !found["next_price_cents"] || !found["next_price_effective_due_date"] {
		t.Fatalf("next-price migration columns = %#v", found)
	}
}
