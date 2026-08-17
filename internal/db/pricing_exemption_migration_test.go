package db_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"carpool-notify/internal/db"
)

func TestOpenCreatesPricingExemptionHistoryAndIndex(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "pricing-exemptions.db")
	store, err := db.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	var tableName string
	if err := database.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name = 'pricing_exemptions'`,
	).Scan(&tableName); err != nil {
		t.Fatal(err)
	}
	var indexName string
	if err := database.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type = 'index' AND name = 'idx_pricing_exemptions_subscription'`,
	).Scan(&indexName); err != nil {
		t.Fatal(err)
	}
	if tableName != "pricing_exemptions" || indexName != "idx_pricing_exemptions_subscription" {
		t.Fatalf("migration objects = %q / %q", tableName, indexName)
	}
}
