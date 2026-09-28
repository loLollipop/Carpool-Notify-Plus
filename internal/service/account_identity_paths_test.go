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
	check := func(path string, serial int64, email string, role AccountSpaceRole) {
		t.Helper()
		if serial != 47 || email != "source@example.com" || role != AccountSpaceRoleSecondary {
			t.Fatalf("%s identity = %d %q %q", path, serial, email, role)
		}
	}
	options, err := svc.ListAccountOptionsForForm(0)
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range options {
		expectedRole := AccountSpaceRoleSecondary
		if option.ID == first {
			expectedRole = AccountSpaceRolePrimary
		}
		if option.SpaceRole != expectedRole {
			t.Fatalf("option %d role = %q, want %q", option.ID, option.SpaceRole, expectedRole)
		}
	}
	views, err := svc.ListAccountsView()
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		expectedRole := AccountSpaceRoleSecondary
		if view.Account.ID == first {
			expectedRole = AccountSpaceRolePrimary
		}
		if view.SpaceRole != expectedRole {
			t.Fatalf("list %d role = %q, want %q", view.Account.ID, view.SpaceRole, expectedRole)
		}
		single, err := svc.GetAccountView(view.Account.ID)
		if err != nil {
			t.Fatal(err)
		}
		if single.SpaceRole != expectedRole {
			t.Fatalf("get %d role = %q, want %q", single.Account.ID, single.SpaceRole, expectedRole)
		}
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
	check("bill", bills.Bills[0].AccountSerial, bills.Bills[0].AccountDisplayEmail, bills.Bills[0].AccountSpaceRole)
	redemptions, err := svc.ListRedemptionApplicationsView("")
	if err != nil {
		t.Fatal(err)
	}
	check("redemption", redemptions[0].AccountSerial, redemptions[0].AccountDisplayEmail, redemptions[0].AccountSpaceRole)
	lookup, err := svc.LookupRenewalSubscriptions(RenewalLookupInput{CustomerEmail: "customer@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	check("lookup", lookup.Subscriptions[0].AccountSerial, lookup.Subscriptions[0].AccountDisplayEmail, lookup.Subscriptions[0].AccountSpaceRole)
	_, err = store.CreateRenewalApplication(model.RenewalApplication{TrackingToken: "renewal", SubscriptionID: subscriptionID, CustomerEmail: "customer@example.com", DueDate: lookup.Subscriptions[0].DueDate, AmountCents: 9000})
	if err != nil {
		t.Fatal(err)
	}
	renewals, err := svc.ListRenewalApplicationsView("")
	if err != nil {
		t.Fatal(err)
	}
	check("renewal", renewals[0].AccountSerial, renewals[0].AccountDisplayEmail, renewals[0].AccountSpaceRole)
	active, err := svc.ListView()
	if err != nil {
		t.Fatal(err)
	}
	check("subscription", active[0].AccountSerial, active[0].AccountDisplayEmail, active[0].AccountSpaceRole)
	sub, err := store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := svc.buildView(sub, svc.now(), "")
	if err != nil {
		t.Fatal(err)
	}
	check("detail", detail.AccountSerial, detail.AccountDisplayEmail, detail.AccountSpaceRole)
	legacyPlus := sub
	legacyPlus.BusinessType = model.SubscriptionBusinessPlus
	legacyPlusView, err := svc.buildView(legacyPlus, svc.now(), "")
	if err != nil {
		t.Fatal(err)
	}
	if legacyPlusView.AccountSpaceRole != AccountSpaceRoleStandalone {
		t.Fatalf("legacy Plus role = %q, want standalone", legacyPlusView.AccountSpaceRole)
	}
	overview, err := svc.GetOperationsOverview()
	if err != nil {
		t.Fatal(err)
	}
	check("dashboard", overview.Dashboard.AmountBySubscription[0].AccountSerial, overview.Dashboard.AmountBySubscription[0].AccountDisplayEmail, overview.Dashboard.AmountBySubscription[0].AccountSpaceRole)
	foundRenewal := false
	for _, task := range overview.Tasks {
		if task.Kind == "renewal_review" {
			foundRenewal = true
			check("operation", task.AccountSerial, task.AccountDisplayEmail, task.AccountSpaceRole)
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
	check("archived", archived[0].AccountSerial, archived[0].AccountDisplayEmail, archived[0].AccountSpaceRole)
}

func TestAccountSpaceRolesUseEffectiveEmailAndImportOrder(t *testing.T) {
	accounts := []model.Account{
		{ID: 10, Name: "first", Email: " Owner@Example.com ", Remark: "(29)"},
		{ID: 20, Name: "second", Email: "owner@example.com", Remark: "(88)"},
		{ID: 30, Name: "remark-first", Email: "login@example.com", Remark: "Remark@Example.com（47）"},
		{ID: 40, Name: "remark-second", Email: " remark@example.com "},
		{ID: 50, Name: "single@example.com", Email: ""},
		{ID: 60, Name: "no-email", Email: ""},
	}
	identities := newAccountIdentityIndex(accounts)

	wants := map[int64]AccountSpaceRole{
		10: AccountSpaceRolePrimary,
		20: AccountSpaceRoleSecondary,
		30: AccountSpaceRolePrimary,
		40: AccountSpaceRoleSecondary,
		50: AccountSpaceRoleStandalone,
		60: AccountSpaceRoleStandalone,
	}
	for accountID, want := range wants {
		if got := identities.identity(accountID).Role; got != want {
			t.Errorf("account %d role = %q, want %q", accountID, got, want)
		}
	}
	if identities.identity(10).Serial != 29 || identities.identity(20).Serial != 88 {
		t.Fatalf("explicit serials changed: %#v %#v", identities.identity(10), identities.identity(20))
	}
	if identities.identity(30).Email != "Remark@Example.com" {
		t.Fatalf("remark display email changed: %q", identities.identity(30).Email)
	}
}
