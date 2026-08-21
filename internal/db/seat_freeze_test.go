package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"carpool-notify/internal/model"
)

func TestSoftDeleteArchivedSubscriptionWaitsForSeatFreezeExpiry(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "seat-freeze.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	currentTime := time.Date(2026, time.August, 10, 4, 0, 0, 0, time.UTC)
	subscriptionID, err := store.CreateSubscription(model.Subscription{
		Name:                "frozen archived customer",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 3000,
		CronExpr:            "interval:30d",
		NotifyOffsets:       []int{},
		BoardedAt:           "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	frozenUntil := formatTime(currentTime.Add(7 * 24 * time.Hour))
	if _, err := store.database.Exec(`
		UPDATE subscriptions
		SET archived_at = ?, seat_frozen_until = ?, updated_at = ?
		WHERE id = ?`,
		formatTime(currentTime), frozenUntil, formatTime(currentTime), subscriptionID,
	); err != nil {
		t.Fatal(err)
	}

	if err := store.SoftDeleteArchivedSubscription(subscriptionID, currentTime); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("delete during freeze error = %v, want sql.ErrNoRows", err)
	}
	if _, err := store.GetSubscriptionIncludingArchived(subscriptionID); err != nil {
		t.Fatalf("subscription disappeared during freeze: %v", err)
	}

	if err := store.SoftDeleteArchivedSubscription(
		subscriptionID,
		currentTime.Add(7*24*time.Hour),
	); err != nil {
		t.Fatalf("delete at freeze expiry: %v", err)
	}
	if _, err := store.GetSubscriptionIncludingArchived(subscriptionID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("subscription after allowed delete error = %v, want sql.ErrNoRows", err)
	}
}

func TestOpenBackfillsCompletedCancellationFreezeFromOriginalRefundTime(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy-completed-cancellation.db")
	store, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	accountID, err := store.CreateAccount(model.Account{Name: "legacy owner"}, 0, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	seatID, err := store.CreateSeat(model.Seat{AccountID: accountID, Name: "车位1"})
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, err := store.CreateSubscription(model.Subscription{
		Name:                "legacy canceled customer",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 3000,
		CronExpr:            "interval:30d",
		NotifyOffsets:       []int{},
		BoardedAt:           "2026-08-01",
		AccountID:           accountID,
		SeatID:              seatID,
	})
	if err != nil {
		t.Fatal(err)
	}
	processedAt := time.Date(2026, time.August, 20, 4, 30, 0, 0, time.UTC)
	processedText := formatTime(processedAt)
	if _, err := store.database.Exec(`
		UPDATE subscriptions
		SET archived_at = ?, seat_frozen_until = NULL, updated_at = ?
		WHERE id = ?`, processedText, processedText, subscriptionID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.database.Exec(`
		INSERT INTO after_sales_cases (
			account_id, subscription_id, business_type, banned_date,
			source, status, processed_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		accountID,
		subscriptionID,
		model.SubscriptionBusinessTeam,
		"2026-08-20",
		model.AfterSalesSourceCustomerCancellation,
		model.AfterSalesStatusRefunded,
		processedText,
		processedText,
		processedText,
	); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(model.SettingSeatFreezeDays, "3"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	archived, err := store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	wantUntil := processedAt.Add(3 * 24 * time.Hour)
	if archived.SeatFrozenUntil == nil || !archived.SeatFrozenUntil.Equal(wantUntil) {
		t.Fatalf("backfilled deadline = %v, want %v", archived.SeatFrozenUntil, wantUntil)
	}
	if free, err := store.ListFreeSeatsAt(accountID, 0, wantUntil.Add(-time.Second)); err != nil || len(free) != 0 {
		t.Fatalf("seat before legacy deadline = %#v, %v", free, err)
	}
	if free, err := store.ListFreeSeatsAt(accountID, 0, wantUntil.Add(time.Second)); err != nil || len(free) != 1 {
		t.Fatalf("seat after legacy deadline = %#v, %v", free, err)
	}
}
