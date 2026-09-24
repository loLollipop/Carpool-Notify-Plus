package service

import (
	"path/filepath"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

func TestAccountDisplaySerialUsesTrailingRemarkOverride(t *testing.T) {
	testCases := []struct {
		name    string
		account model.Account
		want    int64
	}{
		{
			name:    "ascii parentheses",
			account: model.Account{ID: 8, Remark: "ttg@lollipop.elementfx.com(23)"},
			want:    23,
		},
		{
			name:    "full width parentheses",
			account: model.Account{ID: 9, Remark: "来源账号（31）  "},
			want:    31,
		},
		{
			name:    "ordinary remark digits are ignored",
			account: model.Account{ID: 10, Remark: "2026 年导入，第 3 批"},
			want:    10,
		},
		{
			name:    "bare numeric remark is ignored",
			account: model.Account{ID: 13, Remark: "48"},
			want:    13,
		},
		{
			name:    "non numeric suffix is ignored",
			account: model.Account{ID: 11, Remark: "provider(bypass)"},
			want:    11,
		},
		{name: "max safe integer", account: model.Account{ID: 14, Remark: "owner@example.com(9007199254740991)"}, want: 9007199254740991},
		{name: "unsafe integer", account: model.Account{ID: 15, Remark: "owner@example.com(9007199254740992)"}, want: 15},
		{
			name:    "zero override is ignored",
			account: model.Account{ID: 12, Remark: "provider(0)"},
			want:    12,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := accountDisplaySerial(testCase.account); got != testCase.want {
				t.Fatalf("accountDisplaySerial(%#v) = %d, want %d", testCase.account, got, testCase.want)
			}
		})
	}
}

func TestAccountDisplaySerialsGroupsByEffectiveEmail(t *testing.T) {
	testCases := []struct {
		name     string
		accounts []model.Account
		want     map[int64]int64
	}{
		{
			name: "duplicate email uses earliest id despite reverse result order",
			accounts: []model.Account{
				{ID: 57, Email: "cranium@example.com", Remark: "48"},
				{ID: 48, Email: "cranium@example.com"},
			},
			want: map[int64]int64{48: 48, 57: 48},
		},
		{
			name: "email grouping trims whitespace and ignores case",
			accounts: []model.Account{
				{ID: 22, Email: " OWNER@Example.com "},
				{ID: 17, Email: "owner@example.COM"},
			},
			want: map[int64]int64{17: 17, 22: 17},
		},
		{
			name: "explicit serial on current row wins over group serial",
			accounts: []model.Account{
				{ID: 4, Email: "owner@example.com"},
				{ID: 9, Email: "owner@example.com", Remark: "manual source (88)"},
			},
			want: map[int64]int64{4: 4, 9: 88},
		},
		{
			name: "later space inherits earliest explicit serial",
			accounts: []model.Account{
				{ID: 5, Email: "shared@example.com", Remark: "first space (77)"},
				{ID: 8, Email: "shared@example.com"},
			},
			want: map[int64]int64{5: 77, 8: 77},
		},
		{
			name: "standalone email account name is a fallback group key",
			accounts: []model.Account{
				{ID: 31, Name: "fallback@example.com"},
				{ID: 35, Email: "fallback@example.com"},
			},
			want: map[int64]int64{31: 31, 35: 31},
		},
		{
			name: "empty emails are not grouped",
			accounts: []model.Account{
				{ID: 41, Name: "first owner"},
				{ID: 42, Name: "second owner"},
			},
			want: map[int64]int64{41: 41, 42: 42},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := accountDisplaySerials(testCase.accounts)
			for accountID, want := range testCase.want {
				if got[accountID] != want {
					t.Fatalf("accountDisplaySerials(%#v)[%d] = %d, want %d", testCase.accounts, accountID, got[accountID], want)
				}
			}
		})
	}
}

func TestAccountRemarkEmailOverridesDisplayAndGroupingIdentity(t *testing.T) {
	accounts := []model.Account{
		{ID: 12, Email: "login@example.com", Remark: " source@example.com（47） "},
		{ID: 15, Email: "source@example.com"},
		{ID: 16, Email: "login@example.com"},
		{ID: 17, Email: "other@example.com", Remark: "first@example.com second@example.com(51)"},
	}

	if got := accountDisplayEmail(accounts[0]); got != "source@example.com" {
		t.Fatalf("accountDisplayEmail() = %q, want source@example.com", got)
	}
	if got := accountDisplayEmail(accounts[3]); got != "other@example.com" {
		t.Fatalf("ambiguous remark accountDisplayEmail() = %q, want original email", got)
	}
	serials := accountDisplaySerials(accounts)
	if serials[12] != 47 {
		t.Fatalf("remark identity serial = %d, want 47", serials[12])
	}
	if serials[15] != 47 {
		t.Fatalf("remark identity group serial = %d, want 47", serials[15])
	}
	if serials[16] != 16 {
		t.Fatalf("original login email should not group overridden identity: got %d, want 16", serials[16])
	}
	if serials[17] != 51 {
		t.Fatalf("explicit serial remains valid for ambiguous email remark: got %d, want 51", serials[17])
	}
}

func TestGetAccountViewUsesGroupedDisplaySerial(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "grouped-account-view.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	subscriptionService := &SubscriptionService{
		Store: store,
		Clock: func() time.Time {
			return time.Date(2026, time.September, 24, 12, 0, 0, 0, cycle.Location)
		},
	}

	firstID, err := subscriptionService.CreateAccount(CreateAccountInput{
		Name:      "shared@example.com",
		SpaceName: "first-space",
		SeatCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := subscriptionService.CreateAccount(CreateAccountInput{
		Name:      "SHARED@example.com",
		SpaceName: "second-space",
		SeatCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	secondView, err := subscriptionService.GetAccountView(secondID)
	if err != nil {
		t.Fatal(err)
	}
	if secondView.DisplaySerial != firstID {
		t.Fatalf("single account view display serial = %d, want first space serial %d", secondView.DisplaySerial, firstID)
	}

	views, err := subscriptionService.ListAccountsView()
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		if view.Account.ID == secondID && view.DisplaySerial != secondView.DisplaySerial {
			t.Fatalf("list display serial = %d, single view = %d", view.DisplaySerial, secondView.DisplaySerial)
		}
	}
}

func TestAccountDisplaySerialIsExposedToAssignmentViews(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "assignment-serial.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, time.September, 10, 12, 0, 0, 0, cycle.Location)
	subscriptionService := &SubscriptionService{
		Store: store,
		Clock: func() time.Time { return now },
	}

	accountID, err := subscriptionService.CreateAccount(CreateAccountInput{
		Name:      "owner@example.com",
		Remark:    "source@example.com(47)",
		SeatCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	options, err := subscriptionService.ListAccountOptionsForForm(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(options) != 1 || options[0].DisplaySerial != 47 {
		t.Fatalf("account options = %#v, want display serial 47", options)
	}

	redemptionView, err := subscriptionService.buildRedemptionApplicationView(
		model.RedemptionApplication{AssignedAccountID: accountID},
	)
	if err != nil {
		t.Fatal(err)
	}
	if redemptionView.AccountSerial != 47 {
		t.Fatalf("redemption account serial = %d, want 47", redemptionView.AccountSerial)
	}

	accountViews, err := subscriptionService.ListAccountsView()
	if err != nil {
		t.Fatal(err)
	}
	if len(accountViews) != 1 || accountViews[0].DisplaySerial != 47 {
		t.Fatalf("account views = %#v, want display serial 47", accountViews)
	}
	if accountViews[0].DisplayEmail != "source@example.com" {
		t.Fatalf("account view display email = %q, want source@example.com", accountViews[0].DisplayEmail)
	}

	subscriptionID, err := subscriptionService.Create(CreateInput{
		Name:             "team-customer",
		PriceYuan:        "90",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3,1,0",
		SeatID:           options[0].Seats[0].ID,
		BoardedAt:        "2026-09-01",
		CustomerEmail:    "customer@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	subscriptionViews, err := subscriptionService.ListView()
	if err != nil {
		t.Fatal(err)
	}
	if len(subscriptionViews) != 1 || subscriptionViews[0].AccountSerial != 47 {
		t.Fatalf("subscription views = %#v, want account serial 47", subscriptionViews)
	}

	candidates, err := subscriptionService.buildPricingCandidates(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].SubscriptionID != subscriptionID || candidates[0].AccountSerial != 47 {
		t.Fatalf("pricing candidates = %#v, want subscription %d on account serial 47", candidates, subscriptionID)
	}

	dashboard, err := subscriptionService.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if len(dashboard.AmountBySubscription) != 1 || dashboard.AmountBySubscription[0].AccountSerial != 47 {
		t.Fatalf("dashboard amount rows = %#v, want account serial 47", dashboard.AmountBySubscription)
	}
}
