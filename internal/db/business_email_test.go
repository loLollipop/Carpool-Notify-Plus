package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"carpool-notify/internal/model"
)

func TestBusinessEmailHistoryCleanupKeepsActiveMessagesAndRecentCompletions(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "email-retention.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, time.October, 28, 12, 0, 0, 123, time.FixedZone("CST", 8*60*60))
	cutoff := now.Add(-24 * time.Hour)
	fixtures := []struct {
		title, status string
		updatedAt     time.Time
		remove        bool
	}{
		{"old success", "success", cutoff.Add(-time.Nanosecond), true},
		{"24-hour success", "success", cutoff, true},
		{"recent success", "success", cutoff.Add(time.Nanosecond), false},
		{"old queue sent just now", "success", now, false},
		{"old cancellation", "cancelled", cutoff.Add(-time.Hour), true},
		{"24-hour cancellation", "cancelled", cutoff, true},
		{"recent cancellation", "cancelled", cutoff.Add(time.Nanosecond), false},
		{"old pending", "pending", cutoff.Add(-time.Hour), false},
		{"old retry", "pending", cutoff.Add(-time.Hour), false},
	}
	for _, fixture := range fixtures {
		if _, err := store.database.Exec(`INSERT INTO business_email_outbox
			(subscription_id, recipient, title, body, status, attempt_count, next_retry_at, created_at, updated_at)
			VALUES (1, 'customer@example.com', ?, 'original body', ?, 3, ?, ?, ?)`,
			fixture.title, fixture.status, businessEmailTime(cutoff), businessEmailTime(now.Add(-7*24*time.Hour)),
			businessEmailTime(fixture.updatedAt)); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.DeleteExpiredBusinessEmailHistory(ctx, now); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled cleanup = %v", err)
	}
	if err := store.DeleteExpiredBusinessEmailHistory(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		var status, body string
		err := store.database.QueryRow(`SELECT status, body FROM business_email_outbox WHERE title = ?`, fixture.title).Scan(&status, &body)
		if fixture.remove {
			if !errors.Is(err, sql.ErrNoRows) {
				t.Errorf("expired %q still exists: %v", fixture.title, err)
			}
		} else if err != nil || status != fixture.status || body != "original body" {
			t.Errorf("retained %q changed: %q %q, %v", fixture.title, status, body, err)
		}
	}
	pending, err := store.PendingBusinessEmails(now, 8)
	if err != nil || len(pending) != 1 || pending[0].Title != "old pending" {
		t.Fatalf("oldest pending changed after cleanup: %#v, %v", pending, err)
	}
	if err := store.FinishBusinessEmail(pending[0].ID, 4, nil, now); err != nil {
		t.Fatal(err)
	}
	pending, err = store.PendingBusinessEmails(now, 8)
	if err != nil || len(pending) != 1 || pending[0].Title != "old retry" || pending[0].AttemptCount != 3 {
		t.Fatalf("retry order changed after cleanup: %#v, %v", pending, err)
	}
}

func TestBusinessEmailOutboxSurvivesReopenAndPreservesRetryOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "business-email.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 7, 4, 0, 0, 0, time.UTC)
	if err := store.QueueBusinessEmails([]BusinessEmail{
		{SubscriptionID: 1, Recipient: "one@example.com", Title: "grant", Body: "first"},
		{SubscriptionID: 1, Recipient: "one@example.com", Title: "revoke", Body: "second"},
		{SubscriptionID: 2, Recipient: "two@example.com", Title: "other", Body: "independent"},
	}, now); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	pending, err := store.PendingBusinessEmails(now.Add(time.Nanosecond), 8)
	if err != nil || len(pending) != 2 || pending[0].Title != "grant" || pending[1].Title != "other" {
		t.Fatalf("reopened queue: %#v, %v", pending, err)
	}
	if err := store.FinishBusinessEmail(pending[0].ID, 1, errors.New("temporary SMTP failure"), now); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishBusinessEmail(pending[1].ID, 1, nil, now); err != nil {
		t.Fatal(err)
	}
	pending, err = store.PendingBusinessEmails(now.Add(30*time.Second), 8)
	if err != nil || len(pending) != 0 {
		t.Fatalf("retry was bypassed: %#v, %v", pending, err)
	}
	pending, err = store.PendingBusinessEmails(now.Add(time.Minute), 8)
	if err != nil || len(pending) != 1 || pending[0].Title != "grant" || pending[0].AttemptCount != 1 {
		t.Fatalf("retry: %#v, %v", pending, err)
	}
	if err := store.FinishBusinessEmail(pending[0].ID, 2, nil, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	pending, err = store.PendingBusinessEmails(now.Add(time.Minute), 8)
	if err != nil || len(pending) != 1 || pending[0].Title != "revoke" {
		t.Fatalf("next event: %#v, %v", pending, err)
	}
}

func TestResetBusinessDataClearsPendingEmails(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business-email-reset.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.QueueBusinessEmails([]BusinessEmail{{SubscriptionID: 1, Recipient: "customer@example.com", Title: "benefit"}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.ResetBusinessData(); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingBusinessEmails(time.Now().Add(time.Hour), 8)
	if err != nil || len(pending) != 0 {
		t.Fatalf("stale reset emails: %#v, %v", pending, err)
	}
}

func TestOutboxInsertFailureRollsBackSubscriptionMutation(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "business-email-atomic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	id, err := store.CreateSubscription(model.Subscription{Name: "original", BusinessType: model.SubscriptionBusinessTeam,
		PricePerPersonCents: 10000, CronExpr: "interval:30d", BoardedAt: "2026-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetSubscription(id)
	if err != nil {
		t.Fatal(err)
	}
	updated.Name = "must roll back"
	batch := BusinessEmailBatch{Now: time.Now(), Build: func() []BusinessEmail {
		return []BusinessEmail{{SubscriptionID: id, Recipient: "customer@example.com", Title: "benefit"}}
	}}
	if _, err := store.database.Exec(`CREATE TRIGGER reject_business_email BEFORE INSERT ON business_email_outbox
		BEGIN SELECT RAISE(ABORT, 'simulated outbox storage failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSubscription(updated, batch); err == nil {
		t.Fatal("outbox failure not returned")
	}
	persisted, err := store.GetSubscription(id)
	if err != nil || persisted.Name != "original" || !persisted.UpdatedAt.Equal(updated.UpdatedAt) {
		t.Fatalf("mutation escaped rollback: %#v, %v", persisted, err)
	}
	if _, err := store.database.Exec(`DROP TRIGGER reject_business_email`); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSubscription(updated, batch); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingBusinessEmails(time.Now().Add(time.Minute), 8)
	if err != nil || len(pending) != 1 {
		t.Fatalf("atomic success: %#v, %v", pending, err)
	}
}

func TestRenewalBillAndEmailCommitAtomicallyAndOnlyOnce(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "renewal-email-atomic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	id, err := store.CreateSubscription(model.Subscription{Name: "customer", BusinessType: model.SubscriptionBusinessTeam,
		PricePerPersonCents: 10000, CronExpr: "interval:30d", BoardedAt: "2026-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := store.GetSubscription(id)
	if err != nil {
		t.Fatal(err)
	}
	batch := BusinessEmailBatch{Now: time.Now(), Build: func() []BusinessEmail {
		return []BusinessEmail{{SubscriptionID: id, Recipient: "customer@example.com", Title: "续费成功"}}
	}}
	if _, err := store.database.Exec(`CREATE TRIGGER reject_renewal_email BEFORE INSERT ON business_email_outbox
  BEGIN SELECT RAISE(ABORT, 'outbox failed'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetDuePaidForSubscriptionWithEmails(subscription, "2026-10-31", true, 10000, batch); err == nil {
		t.Fatal("outbox failure ignored")
	}
	if paid, err := store.IsDuePaid(id, "2026-10-31"); err != nil || paid {
		t.Fatalf("bill escaped rollback = %v, %v", paid, err)
	}
	if _, err := store.database.Exec(`DROP TRIGGER reject_renewal_email`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetDuePaidForSubscriptionWithEmails(subscription, "2026-10-31", true, 10000, batch); err != nil {
		t.Fatal(err)
	}
	if err := store.SetDuePaidForSubscriptionWithEmails(subscription, "2026-10-31", true, 10000, batch); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingBusinessEmails(time.Now().Add(time.Minute), 8)
	if err != nil || len(pending) != 1 {
		t.Fatalf("duplicate notifications = %#v, %v", pending, err)
	}
	if _, err := store.database.Exec(`UPDATE business_email_outbox SET status = 'success'`); err != nil {
		t.Fatal(err)
	}
	current, err := store.GetSubscription(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetDuePaidForSubscriptionWithEmails(current, "2026-10-31", false, 10000, batch); err != nil {
		t.Fatal(err)
	}
	pending, err = store.PendingBusinessEmails(time.Now().Add(time.Minute), 8)
	if err != nil || len(pending) != 0 {
		t.Fatalf("cancel paid queued notification = %#v, %v", pending, err)
	}
}

func TestBusinessEmailBillLinkMigratesExistingOutbox(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-outbox.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE business_email_outbox (
		id INTEGER PRIMARY KEY AUTOINCREMENT, subscription_id INTEGER NOT NULL,
		recipient TEXT NOT NULL, title TEXT NOT NULL, body TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending', attempt_count INTEGER NOT NULL DEFAULT 0,
		next_retry_at TEXT NOT NULL, last_error TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
		INSERT INTO business_email_outbox (subscription_id, recipient, title, body, next_retry_at, created_at, updated_at)
		VALUES (1, 'customer@example.com', 'legacy benefit', 'unchanged', '2026-01-01', '2026-01-01', '2026-01-01')`)
	closeErr := legacy.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("legacy fixture: %v, %v", err, closeErr)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var billID int64
	if err := store.database.QueryRow(`SELECT bill_id FROM business_email_outbox`).Scan(&billID); err != nil || billID != 0 {
		t.Fatalf("migrated bill reference = %d, %v", billID, err)
	}
	pending, err := store.PendingBusinessEmails(time.Now(), 8)
	if err != nil || len(pending) != 1 || pending[0].Body != "unchanged" {
		t.Fatalf("legacy benefit changed = %#v, %v", pending, err)
	}
}

func TestBillDeletionAndPendingConfirmationCancelAtomically(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "cancel-confirmation.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	id, err := store.CreateSubscription(model.Subscription{Name: "customer", BusinessType: model.SubscriptionBusinessTeam,
		PricePerPersonCents: 10000, CronExpr: "interval:30d", BoardedAt: "2026-10-01"})
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := store.GetSubscription(id)
	if err != nil {
		t.Fatal(err)
	}
	stamp := time.Now()
	batch := BusinessEmailBatch{Now: stamp, Build: func() []BusinessEmail {
		return []BusinessEmail{{SubscriptionID: id, Recipient: "customer@example.com", Title: "renewal"}}
	}}
	if err := store.SetDuePaidForSubscriptionWithEmails(subscription, "2026-10-31", true, 10000, batch); err != nil {
		t.Fatal(err)
	}
	bill, err := store.GetBillByOccurrence(id, "2026-10-31")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.QueueBusinessEmails([]BusinessEmail{{SubscriptionID: id, Recipient: "customer@example.com", Title: "benefit"}}, stamp); err != nil {
		t.Fatal(err)
	}
	if _, err := store.database.Exec(`CREATE TRIGGER reject_email_cancellation BEFORE UPDATE ON business_email_outbox
		WHEN NEW.status = 'cancelled' BEGIN SELECT RAISE(ABORT, 'outbox cancellation failed'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteBill(bill.ID); err == nil {
		t.Fatal("outbox cancellation failure ignored")
	}
	if _, err := store.GetBill(bill.ID); err != nil {
		t.Fatal("bill deletion escaped rollback", err)
	}
	if _, err := store.database.Exec(`DROP TRIGGER reject_email_cancellation`); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteBill(bill.ID); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingBusinessEmails(stamp.Add(time.Minute), 8)
	if err != nil || len(pending) != 1 || pending[0].Title != "benefit" {
		t.Fatalf("cancelled confirmation or unrelated benefit changed: %#v, %v", pending, err)
	}
	var status string
	if err := store.database.QueryRow(`SELECT status FROM business_email_outbox WHERE bill_id = ?`, bill.ID).Scan(&status); err != nil || status != "cancelled" {
		t.Fatalf("confirmation status = %s, %v", status, err)
	}
}
