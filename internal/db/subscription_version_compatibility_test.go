package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"carpool-notify/internal/model"
)

var legacySubscriptionVersions = []struct {
	name, stored, canonical string
}{
	{"sqlite", "2026-08-01 00:00:00", "2026-08-01T00:00:00Z"},
	{"timezone", "2026-08-01T08:00:00+08:00", "2026-08-01T00:00:00Z"},
	{"fraction", "2026-08-01T00:00:00.120000000Z", "2026-08-01T00:00:00.12Z"},
	{"future", "2099-01-01 00:00:00", "2099-01-01T00:00:00Z"},
}

func legacyVersionTestStore(t *testing.T) (*Store, model.Subscription) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "versions.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	accountID, err := store.CreateAccount(model.Account{Name: "owner"}, 0, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	seatID, err := store.CreateSeat(model.Seat{AccountID: accountID, Name: "seat"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateSubscription(model.Subscription{
		Name: "customer", BusinessType: model.SubscriptionBusinessTeam,
		CustomerEmail: "customer@example.com", PricePerPersonCents: 3000, CostCents: 500,
		CronExpr: "interval:30d", BoardedAt: "2026-08-01", SeatID: seatID,
	})
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := store.GetSubscription(id)
	if err != nil {
		t.Fatal(err)
	}
	return store, subscription
}

func setLegacySubscriptionVersion(t *testing.T, store *Store, subscription model.Subscription, stored, canonical string) model.Subscription {
	t.Helper()
	if _, err := store.database.Exec(`UPDATE subscriptions SET updated_at = ? WHERE id = ?`, stored, subscription.ID); err != nil {
		t.Fatal(err)
	}
	version, err := time.Parse(time.RFC3339Nano, canonical)
	if err != nil {
		t.Fatal(err)
	}
	// This is the normalized version returned to a browser, not the DB text.
	subscription.UpdatedAt = version
	return subscription
}

func TestSubscriptionEditsAcceptLegacyVersionsAndRejectStale(t *testing.T) {
	for _, version := range legacySubscriptionVersions {
		for _, path := range []string{"edit", "sync", "move", "next_price"} {
			t.Run(version.name+"/"+path, func(t *testing.T) {
				store, subscription := legacyVersionTestStore(t)
				if path == "sync" || path == "move" {
					if err := store.SetDuePaid(subscription.ID, "2026-08-01", true, 3000, 500); err != nil {
						t.Fatal(err)
					}
				}
				subscription = setLegacySubscriptionVersion(t, store, subscription, version.stored, version.canonical)
				subscription.Remark = "edited"
				switch path {
				case "move":
					subscription.BoardedAt = "2026-08-02"
				case "next_price":
					price := int64(3500)
					subscription.NextPriceCents = &price
					subscription.NextPriceEffectiveDueDate = "2026-08-31"
				}
				write := func(candidate model.Subscription) error {
					switch path {
					case "sync":
						return store.UpdateSubscriptionAndSyncBill(candidate, "2026-08-01", 3500, 600)
					case "move":
						return store.UpdateSubscriptionAndMoveInitialBill(candidate, "2026-08-01", "2026-08-02", 3500, 600)
					case "next_price":
						return store.UpdateSubscriptionNextPrices([]model.Subscription{candidate}, "2026-08-01")
					default:
						return store.UpdateSubscription(candidate)
					}
				}
				var billWriteBefore time.Time
				if version.name == "future" && (path == "sync" || path == "move") {
					billWriteBefore = time.Now().UTC().Add(-time.Second)
				}
				if err := write(subscription); err != nil {
					t.Fatalf("first write with legacy version: %v", err)
				}
				billWriteAfter := time.Now().UTC().Add(time.Second)
				current, err := store.GetSubscription(subscription.ID)
				if err != nil || !current.UpdatedAt.After(subscription.UpdatedAt) {
					t.Fatalf("version did not advance: %#v, %v", current, err)
				}
				if path == "next_price" {
					if current.NextPriceCents == nil || *current.NextPriceCents != 3500 {
						t.Fatalf("next price not saved: %#v", current)
					}
					// Restore eligibility so the stale retry is rejected by its version,
					// rather than by the existing-next-price condition.
					current.NextPriceCents = nil
					current.NextPriceEffectiveDueDate = ""
					if err := store.UpdateSubscription(current); err != nil {
						t.Fatal(err)
					}
					current, err = store.GetSubscription(subscription.ID)
					if err != nil {
						t.Fatal(err)
					}
				}
				bills, err := store.ListBills()
				if err != nil {
					t.Fatal(err)
				}
				if !billWriteBefore.IsZero() {
					if len(bills) != 1 {
						t.Fatalf("bills = %#v, want one updated bill", bills)
					}
					if bills[0].UpdatedAt.Before(billWriteBefore) || bills[0].UpdatedAt.After(billWriteAfter) {
						t.Fatalf("bill updated_at = %v, want wall-clock time in [%v, %v]", bills[0].UpdatedAt, billWriteBefore, billWriteAfter)
					}
				}
				if err := write(subscription); !errors.Is(err, ErrSubscriptionStateChanged) {
					t.Fatalf("stale retry = %v, want version conflict", err)
				}
				after, err := store.GetSubscription(subscription.ID)
				if err != nil || !reflect.DeepEqual(current, after) {
					t.Fatalf("stale retry changed subscription: %#v, %v", after, err)
				}
				afterBills, err := store.ListBills()
				if err != nil || !reflect.DeepEqual(bills, afterBills) {
					t.Fatalf("stale retry changed bills: %#v, %v", afterBills, err)
				}
			})
		}
	}
}

func TestRenewalApprovalAcceptsLegacyVersionAndAlwaysAdvances(t *testing.T) {
	for _, version := range legacySubscriptionVersions {
		for _, nextPriceMode := range []string{"none", "same", "future"} {
			t.Run(version.name+"/"+nextPriceMode, func(t *testing.T) {
				store, subscription := legacyVersionTestStore(t)
				if nextPriceMode != "none" {
					price, dueDate := int64(3000), "2026-08-31"
					if nextPriceMode == "future" {
						price, dueDate = 3500, "2026-09-30"
					}
					subscription.NextPriceCents = &price
					subscription.NextPriceEffectiveDueDate = dueDate
					if err := store.UpdateSubscription(subscription); err != nil {
						t.Fatal(err)
					}
				}
				subscription = setLegacySubscriptionVersion(t, store, subscription, version.stored, version.canonical)
				application := model.RenewalApplication{
					SubscriptionID: subscription.ID, TrackingToken: "first", CustomerEmail: subscription.CustomerEmail,
					DueDate: "2026-08-31", PeriodCount: 1, PeriodEndDate: "2026-09-30", AmountCents: 3000,
				}
				var err error
				application.ID, err = store.CreateRenewalApplication(application)
				if err != nil {
					t.Fatal(err)
				}
				approve := func(application model.RenewalApplication) error {
					return store.ApproveRenewalApplication(application, subscription, []model.Bill{{
						SubscriptionID: subscription.ID, DueDate: application.DueDate, AmountCents: 3000, CostCents: 500,
					}}, application.PeriodEndDate, "")
				}
				if err := approve(application); err != nil {
					t.Fatalf("legacy approval: %v", err)
				}
				current, err := store.GetSubscription(subscription.ID)
				if err != nil || !current.UpdatedAt.After(subscription.UpdatedAt) || current.PricePerPersonCents != 3000 {
					t.Fatalf("approval did not preserve price and advance version: %#v, %v", current, err)
				}
				if nextPriceMode == "future" {
					if current.NextPriceCents == nil || *current.NextPriceCents != 3500 || current.NextPriceEffectiveDueDate != "2026-09-30" {
						t.Fatalf("future scheduled price changed: %#v", current)
					}
				} else if current.NextPriceCents != nil || current.NextPriceEffectiveDueDate != "" {
					t.Fatalf("scheduled price was not consumed: %#v", current)
				}
				if err := store.UpdateSubscription(subscription); !errors.Is(err, ErrSubscriptionStateChanged) {
					t.Fatalf("pre-approval edit = %v", err)
				}
				// A different pending period eliminates duplicate-application and
				// existing-bill guards: the stale subscription alone must reject it.
				application.TrackingToken = "second"
				application.DueDate, application.PeriodEndDate = "2026-09-30", "2026-10-30"
				application.ID, err = store.CreateRenewalApplication(application)
				if err != nil {
					t.Fatal(err)
				}
				if err := approve(application); !errors.Is(err, ErrRenewalFinancialStateChanged) {
					t.Fatalf("stale approval = %v", err)
				}
				bills, err := store.ListBills()
				if err != nil || len(bills) != 1 || bills[0].AmountCents != 3000 || bills[0].CostCents != 500 {
					t.Fatalf("approval bills = %#v, %v", bills, err)
				}
				after, err := store.GetSubscription(subscription.ID)
				if err != nil || !reflect.DeepEqual(current, after) {
					t.Fatalf("stale approval changed subscription: %#v, %v", after, err)
				}
				pending, err := store.GetRenewalApplication(application.ID)
				if err != nil || pending.Status != model.RenewalStatusPending {
					t.Fatalf("stale approval changed application: %#v, %v", pending, err)
				}
			})
		}
	}
}

func TestRenewalApprovalKeepsEventTimesAtWallClockWhenVersionIsFuture(t *testing.T) {
	store, subscription := legacyVersionTestStore(t)
	price := int64(3500)
	subscription.NextPriceCents = &price
	subscription.NextPriceEffectiveDueDate = "2026-08-31"
	if err := store.UpdateSubscription(subscription); err != nil {
		t.Fatal(err)
	}
	subscription = setLegacySubscriptionVersion(t, store, subscription, "2099-01-01 00:00:00", "2099-01-01T00:00:00Z")
	application := model.RenewalApplication{
		SubscriptionID: subscription.ID, TrackingToken: "event-times", CustomerEmail: subscription.CustomerEmail,
		DueDate: "2026-08-31", PeriodCount: 1, PeriodEndDate: "2026-09-30", AmountCents: price,
	}
	var err error
	application.ID, err = store.CreateRenewalApplication(application)
	if err != nil {
		t.Fatal(err)
	}

	before := time.Now().UTC().Add(-time.Second)
	if err := store.ApproveRenewalApplication(application, subscription, []model.Bill{{
		SubscriptionID: subscription.ID, DueDate: application.DueDate, AmountCents: price, CostCents: 500,
	}}, application.PeriodEndDate, ""); err != nil {
		t.Fatal(err)
	}
	after := time.Now().UTC().Add(time.Second)

	current, err := store.GetSubscription(subscription.ID)
	if err != nil || !current.UpdatedAt.After(subscription.UpdatedAt) {
		t.Fatalf("subscription version = %v, err = %v; want after %v", current.UpdatedAt, err, subscription.UpdatedAt)
	}
	bill, err := store.GetBillByOccurrence(subscription.ID, application.DueDate)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := store.GetRenewalApplication(application.ID)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := store.ListSubscriptionPriceChanges()
	if err != nil || len(changes) != 1 {
		t.Fatalf("price changes = %#v, err = %v", changes, err)
	}
	eventTimes := map[string]time.Time{
		"bill paid_at":            bill.PaidAt,
		"bill created_at":         bill.CreatedAt,
		"bill updated_at":         bill.UpdatedAt,
		"application updated_at":  approved.UpdatedAt,
		"price change created_at": changes[0].CreatedAt,
	}
	if approved.ProcessedAt == nil {
		t.Fatal("approved application has no processed_at")
	}
	eventTimes["application processed_at"] = *approved.ProcessedAt
	for field, timestamp := range eventTimes {
		if timestamp.Before(before) || timestamp.After(after) {
			t.Errorf("%s = %v, want wall-clock time in [%v, %v]", field, timestamp, before, after)
		}
	}
}

func TestSubscriptionNextPricesRollsBackLegacyBatchOnStaleVersion(t *testing.T) {
	store, first := legacyVersionTestStore(t)
	second := first
	seatID, err := store.CreateSeat(model.Seat{AccountID: first.AccountID, Name: "second seat"})
	if err != nil {
		t.Fatal(err)
	}
	second.SeatID = seatID
	second.ID, err = store.CreateSubscription(second)
	if err != nil {
		t.Fatal(err)
	}
	first = setLegacySubscriptionVersion(t, store, first, "2026-08-01 00:00:00", "2026-08-01T00:00:00Z")
	second = setLegacySubscriptionVersion(t, store, second, "2026-08-01T08:00:00+08:00", "2026-08-01T00:00:00Z")
	price := int64(3500)
	first.NextPriceCents, second.NextPriceCents = &price, &price
	first.NextPriceEffectiveDueDate, second.NextPriceEffectiveDueDate = "2026-08-31", "2026-08-31"
	// Only the second version changes, by one nanosecond. The first tentative
	// update must roll back and preserve even its legacy timestamp spelling.
	if _, err := store.database.Exec(`UPDATE subscriptions SET updated_at = ? WHERE id = ?`,
		formatTime(second.UpdatedAt.Add(time.Nanosecond)), second.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateSubscriptionNextPrices([]model.Subscription{first, second}); !errors.Is(err, ErrSubscriptionStateChanged) {
		t.Fatalf("stale batch = %v", err)
	}
	var raw string
	if err := store.database.QueryRow(`SELECT updated_at FROM subscriptions WHERE id = ?`, first.ID).Scan(&raw); err != nil || raw != "2026-08-01 00:00:00" {
		t.Fatalf("first version changed despite rollback: %q, %v", raw, err)
	}
	for _, id := range []int64{first.ID, second.ID} {
		current, err := store.GetSubscription(id)
		if err != nil || current.NextPriceCents != nil || current.NextPriceEffectiveDueDate != "" {
			t.Fatalf("partial batch update: %#v, %v", current, err)
		}
	}
	second.UpdatedAt = second.UpdatedAt.Add(time.Nanosecond)
	if err := store.UpdateSubscriptionNextPrices([]model.Subscription{first, second}); err != nil {
		t.Fatalf("refreshed mixed-format batch: %v", err)
	}
	for _, id := range []int64{first.ID, second.ID} {
		current, err := store.GetSubscription(id)
		if err != nil || current.NextPriceCents == nil || *current.NextPriceCents != price {
			t.Fatalf("refreshed batch not applied: %#v, %v", current, err)
		}
	}
}

func TestRenewalApprovalRollsBackVersionOnBillFailure(t *testing.T) {
	store, subscription := legacyVersionTestStore(t)
	subscription = setLegacySubscriptionVersion(t, store, subscription, "2026-08-01 00:00:00", "2026-08-01T00:00:00Z")
	application := model.RenewalApplication{
		SubscriptionID: subscription.ID, TrackingToken: "rollback", CustomerEmail: subscription.CustomerEmail,
		DueDate: "2026-08-31", PeriodCount: 1, PeriodEndDate: "2026-09-30", AmountCents: 3000,
	}
	var err error
	application.ID, err = store.CreateRenewalApplication(application)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.database.Exec(`CREATE TRIGGER reject_renewal_bill BEFORE INSERT ON bills
		BEGIN SELECT RAISE(ABORT, 'injected bill failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.ApproveRenewalApplication(application, subscription, []model.Bill{{
		SubscriptionID: subscription.ID, DueDate: application.DueDate, AmountCents: 3000,
	}}, application.PeriodEndDate, ""); err == nil {
		t.Fatal("approval succeeded despite injected bill failure")
	}
	current, err := store.GetSubscription(subscription.ID)
	if err != nil || !reflect.DeepEqual(subscription, current) {
		t.Fatalf("failed approval changed subscription: %#v, %v", current, err)
	}
	var raw string
	if err := store.database.QueryRow(`SELECT updated_at FROM subscriptions WHERE id = ?`, subscription.ID).Scan(&raw); err != nil || raw != "2026-08-01 00:00:00" {
		t.Fatalf("failed approval changed raw version: %q, %v", raw, err)
	}
	bills, err := store.ListBills()
	if err != nil || len(bills) != 0 {
		t.Fatalf("failed approval changed bills: %#v, %v", bills, err)
	}
	pending, err := store.GetRenewalApplication(application.ID)
	if err != nil || pending.Status != model.RenewalStatusPending {
		t.Fatalf("failed approval changed application: %#v, %v", pending, err)
	}
}

func TestSetDuePaidAcceptsLegacyVersionAndRejectsStale(t *testing.T) {
	for _, version := range legacySubscriptionVersions {
		t.Run(version.name, func(t *testing.T) {
			store, subscription := legacyVersionTestStore(t)
			subscription = setLegacySubscriptionVersion(t, store, subscription, version.stored, version.canonical)
			if err := store.SetDuePaidForSubscription(subscription, "2026-08-01", true, 3000, 500); err != nil {
				t.Fatalf("legacy accounting: %v", err)
			}
			current := subscription
			current.PricePerPersonCents = 3500
			if err := store.UpdateSubscription(current); err != nil {
				t.Fatal(err)
			}
			if err := store.SetDuePaidForSubscription(subscription, "2026-08-31", true, 3000, 500); !errors.Is(err, ErrSubscriptionFinancialStateChanged) {
				t.Fatalf("stale accounting = %v", err)
			}
			bills, err := store.ListBills()
			if err != nil || len(bills) != 1 || bills[0].DueDate != "2026-08-01" || bills[0].AmountCents != 3000 {
				t.Fatalf("stale accounting changed bills: %#v, %v", bills, err)
			}
		})
	}
}

func TestSubscriptionVersionComparisonRejectsInvalidAndDistinctInstants(t *testing.T) {
	version := time.Date(2026, time.August, 1, 0, 0, 0, 120000000, time.UTC)
	for _, stored := range []string{"", " ", "invalid", "0001-01-01T00:00:00Z", "2026-08-01T00:00:00.120000001Z"} {
		if versionTimeMatches(stored, version) {
			t.Errorf("invalid or distinct version %q accepted", stored)
		}
	}
	if versionTimeMatches("0001-01-01T00:00:00Z", time.Time{}) {
		t.Fatal("zero version accepted")
	}
	for _, stored := range []string{"", "invalid", "0001-01-01T00:00:00Z"} {
		t.Run(stored, func(t *testing.T) {
			store, subscription := legacyVersionTestStore(t)
			if _, err := store.database.Exec(`UPDATE subscriptions SET updated_at = ? WHERE id = ?`, stored, subscription.ID); err != nil {
				t.Fatal(err)
			}
			if err := store.UpdateSubscription(subscription); !errors.Is(err, ErrSubscriptionStateChanged) {
				t.Fatalf("invalid stored version error = %v", err)
			}
		})
	}
}

func TestSubscriptionLegacyVersionRejectsStaleWALSnapshot(t *testing.T) {
	store, subscription := legacyVersionTestStore(t)
	subscription = setLegacySubscriptionVersion(t, store, subscription, "2026-08-01 00:00:00", "2026-08-01T00:00:00Z")
	transaction, err := store.database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = transaction.Rollback() }()
	if _, err := subscriptionVersionForUpdate(transaction, subscription.ID, subscription.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	// Open an independent connection after pinning the original read snapshot.
	var databasePath string
	var sequence int
	var name string
	if err := transaction.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &databasePath); err != nil {
		t.Fatal(err)
	}
	other, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.Exec(`UPDATE subscriptions SET remark = 'concurrent edit', updated_at = ? WHERE id = ?`,
		formatTime(subscription.UpdatedAt.Add(time.Nanosecond)), subscription.ID); err != nil {
		t.Fatal(err)
	}
	subscription.Remark = "must not persist"
	if err := updateSubscriptionWithExecutor(transaction, subscription, nextWriteTime(subscription.UpdatedAt), 0); !errors.Is(err, ErrSubscriptionStateChanged) {
		t.Fatalf("stale WAL write = %v, want version conflict", err)
	}
	if err := transaction.Rollback(); err != nil {
		t.Fatal(err)
	}
	current, err := store.GetSubscription(subscription.ID)
	if err != nil || current.Remark != "concurrent edit" || !current.UpdatedAt.Equal(subscription.UpdatedAt.Add(time.Nanosecond)) {
		t.Fatalf("concurrent edit overwritten: %#v, %v", current, err)
	}
}
