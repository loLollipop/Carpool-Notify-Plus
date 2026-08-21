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
