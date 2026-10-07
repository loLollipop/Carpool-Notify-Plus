package db

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"carpool-notify/internal/model"
)

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
