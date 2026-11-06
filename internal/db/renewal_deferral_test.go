package db

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

func TestSubscriptionAssignmentRechecksPauseAfterAccountRead(t *testing.T) {
	for _, operation := range []string{"create", "initial-bill", "edit", "sync", "move", "redeem"} {
		t.Run(operation, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pause.db")
			writer, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = writer.Close() })
			pauser, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = pauser.Close() })
			ownerID := pauseTestAccount(t, writer, model.Account{Name: "primary", Email: "owner@example.com"})
			accountID, err := writer.CreateAccountWithSeats(model.Account{Name: "target", Email: "OWNER@example.com"}, 0, "2026-09-01", []string{"new seat"})
			if err != nil {
				t.Fatal(err)
			}
			seats, _ := writer.ListSeatsByAccount(accountID)
			id, err := writer.CreateSubscriptionWithInitialBill(integritySubscription(), "2026-09-01", 3000)
			if err != nil {
				t.Fatal(err)
			}
			subscription, _ := writer.GetSubscription(id)
			account, _ := writer.GetAccount(accountID)
			if account.Paused {
				t.Fatal("initially paused")
			}
			if err := pauser.SetAccountPaused(ownerID, true); err != nil {
				t.Fatal(err)
			}
			subscription.SeatID = seats[0].ID
			switch operation {
			case "create":
				_, err = writer.CreateSubscription(subscription)
			case "initial-bill":
				_, err = writer.CreateSubscriptionWithInitialBill(subscription, "2026-09-01", 3000)
			case "edit":
				err = writer.UpdateSubscription(subscription)
			case "sync":
				err = writer.UpdateSubscriptionAndSyncBill(subscription, "2026-09-01", 4000, 0)
			case "move":
				subscription.BoardedAt = "2026-09-02"
				err = writer.UpdateSubscriptionAndMoveInitialBill(subscription, "2026-09-01", "2026-09-02", 4000, 0)
			case "redeem":
				appID, createErr := writer.CreateRedemptionApplication(model.RedemptionApplication{TrackingToken: "pause", CustomerEmail: "pause@example.com"})
				if createErr != nil {
					t.Fatal(createErr)
				}
				_, err = writer.CreateSubscriptionAndInviteRedemption(appID, accountID, seats[0].ID, subscription, "2026-09-01", 3000, "")
			}
			if !errors.Is(err, ErrAccountPaused) {
				t.Fatalf("late paused allocation: %v", err)
			}
			stored, err := writer.GetSubscription(id)
			if err != nil || stored.SeatID != 0 || !stored.UpdatedAt.Equal(subscription.UpdatedAt) {
				t.Fatalf("failed write changed subscription: %#v, %v", stored, err)
			}
			var count int
			if err := writer.database.QueryRow(`SELECT COUNT(*) FROM bills`).Scan(&count); err != nil || count != 1 {
				t.Fatalf("bill count %d, %v", count, err)
			}
		})
	}
}

func TestRenewalDeferralRejectsStaleDueAndLifecycleConflicts(t *testing.T) {
	for _, conflict := range []string{"due", "version", "future", "plus", "cancellation", "after-sales", "review", "archived"} {
		t.Run(conflict, func(t *testing.T) {
			store, err := Open(filepath.Join(t.TempDir(), "conflict.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			subscription := integritySubscription()
			id, err := store.CreateSubscriptionWithInitialBill(subscription, "2026-09-01", 3000)
			if err != nil {
				t.Fatal(err)
			}
			subscription, _ = store.GetSubscription(id)
			now := time.Date(2026, 10, 2, 12, 0, 0, 0, cycle.Location)
			expected := RenewalDeferralExpectation{SubscriptionID: id, UpdatedAt: subscription.UpdatedAt, DueDate: "2026-10-01"}
			switch conflict {
			case "due":
				expected.DueDate = "2026-09-01"
			case "version":
				expected.UpdatedAt = expected.UpdatedAt.Add(-time.Second)
			case "future":
				now = time.Date(2026, 9, 15, 12, 0, 0, 0, cycle.Location)
			case "plus":
				_, err = store.database.Exec(`UPDATE subscriptions SET business_type = 'plus' WHERE id = ?`, id)
			case "cancellation":
				_, err = store.database.Exec(`UPDATE subscriptions SET cancellation_requested_at = ? WHERE id = ?`, formatTime(now), id)
			case "archived":
				_, err = store.database.Exec(`UPDATE subscriptions SET archived_at = ? WHERE id = ?`, formatTime(now), id)
			case "after-sales", "review":
				status := model.AfterSalesStatusPending
				if conflict == "review" {
					status = model.AfterSalesStatusReview
				}
				_, err = store.database.Exec(`INSERT INTO after_sales_cases (subscription_id, banned_date, status, created_at, updated_at) VALUES (?, '2026-10-02', ?, ?, ?)`, id, status, formatTime(now), formatTime(now))
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := store.DeferSubscriptionRenewal(expected, now); err == nil {
				t.Fatal("invalid deferral accepted")
			}
			stored, err := store.GetSubscriptionIncludingArchived(id)
			if err != nil || stored.RenewalDeferredAt != nil {
				t.Fatalf("invalid deferral changed row: %#v, %v", stored, err)
			}
		})
	}
}

func TestDeferredScheduleAndExtensionChangesRequireRestore(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "schedule.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	subscription := integritySubscription()
	id, err := store.CreateSubscriptionWithInitialBill(subscription, "2026-09-01", 3000)
	if err != nil {
		t.Fatal(err)
	}
	subscription, _ = store.GetSubscription(id)
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, cycle.Location)
	if err := store.DeferSubscriptionRenewal(RenewalDeferralExpectation{SubscriptionID: id, UpdatedAt: subscription.UpdatedAt, DueDate: "2026-10-01"}, now); err != nil {
		t.Fatal(err)
	}
	subscription, _ = store.GetSubscription(id)
	for _, change := range []string{"schedule", "boarded", "type"} {
		changed := subscription
		switch change {
		case "schedule":
			changed.CronExpr = "interval:15d"
		case "boarded":
			changed.BoardedAt = "2026-09-02"
		case "type":
			changed.BusinessType = model.SubscriptionBusinessPlus
		}
		if err := store.UpdateSubscription(changed); !errors.Is(err, ErrRenewalDeferred) {
			t.Fatalf("%s accepted: %v", change, err)
		}
	}
	subscription.Remark = "contact edit"
	if err := store.UpdateSubscription(subscription); err != nil {
		t.Fatalf("contact edit: %v", err)
	}
	tx, err := store.database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := applyDueExtension(tx, model.CustomerBenefit{SubscriptionID: id}, 7, now, nil); !errors.Is(err, ErrRenewalDeferred) {
		t.Fatalf("extension accepted: %v", err)
	}
}

func TestAvailabilityMigrationPreservesExistingStateOnReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	accountID, err := store.CreateAccount(model.Account{Name: "legacy"}, 0, "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateSubscriptionWithInitialBill(integritySubscription(), "2026-09-01", 3000)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`ALTER TABLE accounts DROP COLUMN paused`, `ALTER TABLE subscriptions DROP COLUMN renewal_deferred_at`, `ALTER TABLE subscriptions DROP COLUMN renewal_deferred_due_date`} {
		if _, err := store.database.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	account, err := store.GetAccount(accountID)
	if err != nil || account.Paused {
		t.Fatalf("legacy account = %#v, %v", account, err)
	}
	subscription, err := store.GetSubscription(id)
	if err != nil || subscription.RenewalDeferredAt != nil || subscription.RenewalDeferredDueDate != "" {
		t.Fatalf("legacy subscription = %#v, %v", subscription, err)
	}
	if err := store.SetAccountPaused(accountID, true); err != nil {
		t.Fatal(err)
	}
	if err := store.DeferSubscriptionRenewal(RenewalDeferralExpectation{SubscriptionID: id, UpdatedAt: subscription.UpdatedAt, DueDate: "2026-10-01"}, time.Date(2026, 10, 2, 12, 0, 0, 0, cycle.Location)); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	account, _ = store.GetAccount(accountID)
	subscription, _ = store.GetSubscription(id)
	if !account.Paused || subscription.RenewalDeferredAt == nil || subscription.RenewalDeferredDueDate != "2026-10-01" {
		t.Fatal("reopen lost state")
	}
}

func TestDeferredPaymentAndOutboxRollbackTogether(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "outbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	id, err := store.CreateSubscriptionWithInitialBill(integritySubscription(), "2026-09-01", 3000)
	if err != nil {
		t.Fatal(err)
	}
	subscription, _ := store.GetSubscription(id)
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, cycle.Location)
	if err := store.DeferSubscriptionRenewal(RenewalDeferralExpectation{SubscriptionID: id, UpdatedAt: subscription.UpdatedAt, DueDate: "2026-10-01"}, now); err != nil {
		t.Fatal(err)
	}
	subscription, _ = store.GetSubscription(id)
	batch := BusinessEmailBatch{Now: now, BuildChecked: func() ([]BusinessEmail, error) { return nil, errors.New("invalid template") }}
	if err := store.SetDuePaidForSubscriptionWithEmails(subscription, "2026-10-01", true, 3000, batch); err == nil {
		t.Fatal("invalid outbox accepted")
	}
	current, _ := store.GetSubscription(id)
	if current.RenewalDeferredAt == nil || !current.UpdatedAt.Equal(subscription.UpdatedAt) {
		t.Fatal("failed payment cleared deferral")
	}
	if paid, err := store.IsDuePaid(id, "2026-10-01"); err != nil || paid {
		t.Fatalf("failed payment wrote bill: %v, %v", paid, err)
	}
	batch.BuildChecked = func() ([]BusinessEmail, error) {
		return []BusinessEmail{{SubscriptionID: id, Recipient: "customer@example.com", Title: "续费成功", Body: "receipt"}}, nil
	}
	if err := store.SetDuePaidForSubscriptionWithEmails(subscription, "2026-10-01", true, 3000, batch); err != nil {
		t.Fatal(err)
	}
	current, _ = store.GetSubscription(id)
	if current.RenewalDeferredAt != nil {
		t.Fatal("payment did not clear")
	}
	pending, err := store.PendingBusinessEmails(now, 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("receipt outbox: %#v, %v", pending, err)
	}
	if err := store.SetDuePaidForSubscription(current, "2026-10-01", false, 0); err != nil {
		t.Fatal(err)
	}
	pending, err = store.PendingBusinessEmails(now, 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("canceled receipt retried: %#v, %v", pending, err)
	}
}
