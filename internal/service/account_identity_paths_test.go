package service

import (
	"path/filepath"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

func TestAccountIdentityAcrossOperationalViews(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "identities.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc := &SubscriptionService{Store: store, Clock: func() time.Time { return time.Date(2026, 9, 24, 12, 0, 0, 0, cycle.Location) }}
	first, err := svc.CreateAccount(CreateAccountInput{Name: "first", Email: "login@example.com", Remark: "source@example.com（47）", SeatCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.CreateAccount(CreateAccountInput{Name: "second", Email: "source@example.com", SeatCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	check := func(path string, serial int64, email string) {
		t.Helper()
		if serial != 47 || email != "source@example.com" {
			t.Fatalf("%s identity = %d %q", path, serial, email)
		}
	}
	options, err := svc.ListAccountOptionsForForm(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range options {
		check("option", option.DisplaySerial, option.DisplayEmail)
	}
	views, err := svc.ListAccountsView()
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		check("list", view.DisplaySerial, view.DisplayEmail)
		single, err := svc.GetAccountView(view.Account.ID)
		if err != nil {
			t.Fatal(err)
		}
		check("get", single.DisplaySerial, single.DisplayEmail)
	}
	if raw, err := store.GetAccount(first); err != nil || raw.Email != "login@example.com" {
		t.Fatalf("login email changed: %#v %v", raw, err)
	}
	seats, err := store.ListSeatsByAccount(second)
	if err != nil {
		t.Fatal(err)
	}
	applicationID, err := store.CreateRedemptionApplication(model.RedemptionApplication{TrackingToken: "redemption", CustomerEmail: "customer@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, err := svc.InviteRedemptionApplication(applicationID, RedemptionInviteInput{SeatID: seats[0].ID, PriceYuan: "90", CronExpr: "interval:30d", BoardedAt: "2026-09-01", NotifyOffsetsRaw: "3"})
	if err != nil {
		t.Fatal(err)
	}
	bills, err := svc.ListBillsPage()
	if err != nil {
		t.Fatal(err)
	}
	if len(bills.Bills) != 1 {
		t.Fatalf("bills = %d", len(bills.Bills))
	}
	check("bill", bills.Bills[0].AccountSerial, bills.Bills[0].AccountDisplayEmail)
	redemptions, err := svc.ListRedemptionApplicationsView("")
	if err != nil {
		t.Fatal(err)
	}
	check("redemption", redemptions[0].AccountSerial, redemptions[0].AccountDisplayEmail)
	lookup, err := svc.LookupRenewalSubscriptions(RenewalLookupInput{CustomerEmail: "customer@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	check("lookup", lookup.Subscriptions[0].AccountSerial, lookup.Subscriptions[0].AccountDisplayEmail)
	_, err = store.CreateRenewalApplication(model.RenewalApplication{TrackingToken: "renewal", SubscriptionID: subscriptionID, CustomerEmail: "customer@example.com", DueDate: lookup.Subscriptions[0].DueDate, AmountCents: 9000})
	if err != nil {
		t.Fatal(err)
	}
	renewals, err := svc.ListRenewalApplicationsView("")
	if err != nil {
		t.Fatal(err)
	}
	check("renewal", renewals[0].AccountSerial, renewals[0].AccountDisplayEmail)
	active, err := svc.ListView()
	if err != nil {
		t.Fatal(err)
	}
	check("subscription", active[0].AccountSerial, active[0].AccountDisplayEmail)
	sub, err := store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := svc.buildView(sub, svc.now(), "")
	if err != nil {
		t.Fatal(err)
	}
	check("detail", detail.AccountSerial, detail.AccountDisplayEmail)
	overview, err := svc.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	check("dashboard", overview.Dashboard.AmountBySubscription[0].AccountSerial, overview.Dashboard.AmountBySubscription[0].AccountDisplayEmail)
	foundRenewal := false
	for _, task := range overview.Tasks {
		if task.Kind == "renewal_review" {
			foundRenewal = true
			check("operation", task.AccountSerial, task.AccountDisplayEmail)
		}
	}
	if !foundRenewal {
		t.Fatal("renewal task missing")
	}
	if err := svc.Archive(subscriptionID); err != nil {
		t.Fatal(err)
	}
	archived, err := svc.ListArchivedView()
	if err != nil {
		t.Fatal(err)
	}
	check("archived", archived[0].AccountSerial, archived[0].AccountDisplayEmail)
}
