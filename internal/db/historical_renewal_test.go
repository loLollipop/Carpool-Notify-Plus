package db

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestHistoricalRenewalProgressReadsOnlyApprovedMultiPeriodPackages(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "historical-renewals.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	first, err := store.CreateSubscription(integritySubscription())
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateSubscription(integritySubscription())
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		token, status, endDate string
		subscriptionID         int64
		periods                int
	}{
		{"approved", "approved", " 2026-10-30 ", first, 2},
		{"single", "approved", "2026-09-30", first, 1},
		{"pending", "pending", "2026-11-30", first, 2},
		{"rejected", "rejected", "2026-12-30", first, 2},
		{"missing-date", "approved", "", first, 2},
		{"other-subscription", "approved", "2026-10-30", second, 2},
	} {
		if _, err := store.database.Exec(`INSERT INTO renewal_applications (
			tracking_token, subscription_id, customer_email, due_date, period_count,
			period_end_date, amount_cents, status, created_at, updated_at
		) VALUES (?, ?, 'legacy@example.com', '2026-08-31', ?, ?, 18000, ?,
			'2026-08-20T00:00:00Z', '2026-08-20T00:00:00Z')`,
			fixture.token, fixture.subscriptionID, fixture.periods, fixture.endDate, fixture.status); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.ListApprovedMultiPeriodRenewalEndDates([]int64{first})
	want := map[int64]map[string]struct{}{first: {"2026-10-30": {}}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("historical progress dates = %#v, %v; want %#v", got, err, want)
	}
	if got, err := store.ListApprovedMultiPeriodRenewalEndDates(nil); err != nil || len(got) != 0 {
		t.Fatalf("empty subscription selection = %#v, %v", got, err)
	}
}
