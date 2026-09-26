package service

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/notify"
)

type persistenceTestSender struct {
	calls int
	err   error
}

func (sender *persistenceTestSender) Send(context.Context, string, string) error {
	sender.calls++
	return sender.err
}

func TestScheduledNotificationStateFailuresReachCaller(t *testing.T) {
	for _, channel := range []string{model.ChannelSMTP, model.ChannelIYUU} {
		for _, outcome := range []string{"success", "retry", "unconfigured", "exhausted", "paid", "expired", "obsolete-channel"} {
			t.Run(channel+"/"+outcome, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "notification.db")
				store, err := db.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = store.Close() })
				now := time.Date(2026, time.September, 15, 12, 0, 0, 0, cycle.Location)
				service := &SubscriptionService{Store: store, Clock: func() time.Time { return now }}
				sender := &persistenceTestSender{}
				if outcome == "retry" {
					sender.err = errors.New("transport failure")
				}
				if outcome != "unconfigured" {
					service.Notify = notify.Registry{SMTP: sender, IYUU: sender}
				}
				// Invalid schedule skips automatic planning; seeded logs exercise sending.
				id, err := store.CreateSubscription(model.Subscription{
					Name: "customer", PricePerPersonCents: 3500, CronExpr: "invalid",
					BoardedAt: "2026-08-01", CustomerEmail: "customer@example.com",
				})
				if err != nil {
					t.Fatal(err)
				}
				dueDate, offset := "2026-09-15", 0
				if channel == model.ChannelSMTP {
					dueDate, offset = "2026-09-18", 3
				}
				if outcome == "expired" {
					dueDate = "2026-09-01"
				}
				if outcome == "obsolete-channel" {
					if channel == model.ChannelSMTP {
						dueDate, offset = "2026-09-15", 0
					} else {
						dueDate, offset = "2026-09-18", 3
					}
				}
				entry, err := store.UpsertPendingNotification(id, dueDate, offset, channel, model.NotificationKindScheduled)
				if err != nil {
					t.Fatal(err)
				}
				database, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = database.Close() })
				if outcome == "exhausted" {
					if _, err := database.Exec(`UPDATE notification_log SET attempt_count = 5 WHERE id = ?`, entry.ID); err != nil {
						t.Fatal(err)
					}
				}
				if outcome == "paid" {
					if err := store.SetDuePaid(id, dueDate, true, 3500); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := database.Exec(`CREATE TRIGGER reject_notification_state
					BEFORE UPDATE ON notification_log
					BEGIN SELECT RAISE(ABORT, 'injected notification state failure'); END;`); err != nil {
					t.Fatal(err)
				}
				err = service.ProcessDueNotifications(context.Background())
				if err == nil || !strings.Contains(err.Error(), "injected notification state failure") || !strings.Contains(err.Error(), "persist notification") {
					t.Fatalf("scheduler error = %v, want state persistence failure", err)
				}
				if outcome == "success" && sender.calls != 1 {
					t.Fatalf("successful sends = %d, want 1", sender.calls)
				}
				if outcome == "retry" && (!strings.Contains(err.Error(), "transport failure") || sender.calls != 1) {
					t.Fatalf("transport error lost: calls=%d, error=%v", sender.calls, err)
				}
			})
		}
	}
}

func TestManualTestNotificationLogFailuresReachCallerAndDoNotStopChannels(t *testing.T) {
	for _, sendOutcome := range []string{"success", "failure", "unconfigured"} {
		t.Run(sendOutcome, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "notification.db")
			store, err := db.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			smtp := &persistenceTestSender{}
			if sendOutcome == "failure" {
				smtp.err = errors.New("transport failure")
			}
			iyuu := &persistenceTestSender{}
			registry := notify.Registry{SMTP: smtp, IYUU: iyuu}
			if sendOutcome == "unconfigured" {
				registry.SMTP = nil
			}
			service := &SubscriptionService{Store: store, Notify: registry}
			if err := service.SaveEnabledChannels([]string{model.ChannelSMTP, model.ChannelIYUU}); err != nil {
				t.Fatal(err)
			}
			subscriptionID, err := store.CreateSubscription(model.Subscription{
				Name: "customer", PricePerPersonCents: 3500, CronExpr: "interval:30d", BoardedAt: "2026-08-01",
			})
			if err != nil {
				t.Fatal(err)
			}
			database, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close() })
			if _, err := database.Exec(`CREATE TRIGGER reject_test_notification_log
				BEFORE INSERT ON notification_log
				BEGIN SELECT RAISE(ABORT, 'injected test log failure'); END;`); err != nil {
				t.Fatal(err)
			}

			err = service.TestNotify(context.Background(), subscriptionID)
			if err == nil || !strings.Contains(err.Error(), "injected test log failure") || !strings.Contains(err.Error(), "persist smtp test notification result") {
				t.Fatalf("manual test error = %v, want log persistence failure", err)
			}
			if sendOutcome == "unconfigured" {
				if smtp.calls != 0 || iyuu.calls != 1 {
					t.Fatalf("channel calls = smtp %d, iyuu %d; want unconfigured smtp skipped and iyuu attempted once", smtp.calls, iyuu.calls)
				}
				if !strings.Contains(err.Error(), "smtp: not configured") || !strings.Contains(err.Error(), "persist smtp test notification result") {
					t.Fatalf("manual test error = %v, want unconfigured and persistence failures", err)
				}
			} else if smtp.calls != 1 || iyuu.calls != 1 {
				t.Fatalf("channel calls = smtp %d, iyuu %d; want both attempted once", smtp.calls, iyuu.calls)
			}
			if sendOutcome == "failure" && !strings.Contains(err.Error(), "transport failure") {
				t.Fatalf("manual test error = %v, want transport and persistence failures", err)
			}
		})
	}
}
