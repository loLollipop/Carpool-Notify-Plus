package service

import (
	"reflect"
	"testing"

	"carpool-notify/internal/model"
)

func TestPaidDueDatesBySubscriptionGroupsAndSorts(t *testing.T) {
	bills := []model.Bill{
		{SubscriptionID: 2, DueDate: "2026-09-02"},
		{SubscriptionID: 1, DueDate: "2026-10-01"},
		{SubscriptionID: 1, DueDate: "2026-08-01"},
		{SubscriptionID: 2, DueDate: "2026-07-02"},
	}
	want := map[int64][]string{
		1: {"2026-08-01", "2026-10-01"},
		2: {"2026-07-02", "2026-09-02"},
	}
	if got := paidDueDatesBySubscription(bills); !reflect.DeepEqual(got, want) {
		t.Fatalf("paid due dates = %#v, want %#v", got, want)
	}
}
