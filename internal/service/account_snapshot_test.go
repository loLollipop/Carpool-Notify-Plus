package service_test

import (
	"reflect"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/service"
)

func TestAccountListSnapshotMatchesSingleAccountView(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }

	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "snapshot-owner@example.com",
		Email:     "snapshot-owner@example.com",
		OpenedAt:  "2026-07-25",
		CostYuan:  "75.00",
		SeatCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := subscriptionService.Store.ListSeatsByAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(seats) != 2 {
		t.Fatalf("seat count = %d, want 2", len(seats))
	}

	if _, err := subscriptionService.Create(service.CreateInput{
		Name:             "active-customer",
		PriceYuan:        "95.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3,1,0",
		SeatID:           seats[0].ID,
		BoardedAt:        "2026-08-01",
		CustomerEmail:    "active@example.com",
		CustomerWechat:   "active-wechat",
	}); err != nil {
		t.Fatal(err)
	}

	for index := 0; index < 5; index++ {
		subscriptionID, createErr := subscriptionService.Create(service.CreateInput{
			Name:             "archived-customer",
			PriceYuan:        "90.00",
			CronExpr:         "interval:30d",
			NotifyOffsetsRaw: "3,1,0",
			SeatID:           seats[1].ID,
			BoardedAt:        "2026-07-01",
			CustomerEmail:    "archived@example.com",
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		archivedAt := now.Add(time.Duration(index-4) * time.Hour)
		if archiveErr := subscriptionService.Store.ArchiveSubscriptionWithSeatFreeze(
			subscriptionID,
			archivedAt,
			now.Add(7*24*time.Hour),
		); archiveErr != nil {
			t.Fatal(archiveErr)
		}
	}

	singleView, err := subscriptionService.GetAccountView(accountID)
	if err != nil {
		t.Fatal(err)
	}
	listViews, err := subscriptionService.ListAccountsView()
	if err != nil {
		t.Fatal(err)
	}
	var listView *service.AccountView
	for index := range listViews {
		if listViews[index].Account.ID == accountID {
			listView = &listViews[index]
			break
		}
	}
	if listView == nil {
		t.Fatalf("account %d missing from list view", accountID)
	}
	if !reflect.DeepEqual(*listView, singleView) {
		t.Fatalf("snapshot view differs from single-account view:\nlist=%#v\nsingle=%#v", *listView, singleView)
	}
}
