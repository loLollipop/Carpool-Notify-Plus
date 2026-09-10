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
			name:    "non numeric suffix is ignored",
			account: model.Account{ID: 11, Remark: "provider(bypass)"},
			want:    11,
		},
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
