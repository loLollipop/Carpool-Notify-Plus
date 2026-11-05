package service

import (
	"reflect"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

func activityTime(value string) time.Time {
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return at
}

func TestCalendarActivityUsesOperationTimeAndRetainsArchivedHistory(t *testing.T) {
	start := activityTime("2026-11-01T00:00:00+08:00")
	end := start.AddDate(0, 1, 0)
	created := activityTime("2026-11-05T09:00:00+08:00")
	archivedAt := activityTime("2026-11-05T15:00:00+08:00")
	views := []SubscriptionView{{
		Subscription: model.Subscription{ID: 1, Name: "new", CronExpr: "interval:30d", BoardedAt: "2026-10-01", CreatedAt: created},
	}, {
		Subscription: model.Subscription{ID: 2, Name: "without bill", CronExpr: "interval:30d", BoardedAt: "2026-11-05", CreatedAt: created.Add(time.Minute)},
	}}
	archived := []SubscriptionView{{
		Subscription: model.Subscription{ID: 3, Name: "archived", CronExpr: "interval:30d", BoardedAt: "2026-09-01", CreatedAt: start.AddDate(0, -2, 0), ArchivedAt: &archivedAt},
	}}
	bills := []model.Bill{
		{ID: 11, SubscriptionID: 1, DueDate: "2026-10-01", AmountCents: 10000, CreatedAt: created, PaidAt: created},
		{ID: 12, SubscriptionID: 1, DueDate: "2026-12-01", AmountCents: 8800, CreatedAt: created.Add(time.Hour), PaidAt: created.Add(time.Hour)},
		// A paid historical key need not exist on the current edited schedule.
		{ID: 13, SubscriptionID: 3, DueDate: "2026-10-02", AmountCents: 7500, CreatedAt: created.Add(2 * time.Hour), PaidAt: created.Add(2 * time.Hour)},
		// This bill belongs to October despite a November billing period.
		{ID: 14, SubscriptionID: 1, DueDate: "2026-11-01", AmountCents: 10000, CreatedAt: start.Add(-time.Minute), PaidAt: start.Add(-time.Minute)},
	}
	svc := &SubscriptionService{Clock: func() time.Time { return created }}
	items, err := svc.buildCalendarActivity(start, end, views, archived, bills, nil)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	if want := []string{"archived:3", "paid:13", "paid:12", "created:2", "created:1"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("items = %v, want %v", ids, want)
	}
	if items[1].AmountYuan != "75.00" || !items[1].Archived || items[1].Occurrence.DueDate != "2026-10-02" {
		t.Fatalf("archived payment = %#v", items[1])
	}
	if items[1].Occurrence.CostYuan != "" || items[1].Occurrence.ProfitYuan != "" {
		t.Fatal("unknown archived Team cost must not be reported as zero")
	}
	if items[2].AmountYuan != "88.00" || items[2].Occurrence.DueDate != "2026-12-01" {
		t.Fatalf("prepayment = %#v", items[2])
	}
	if items[3].AmountYuan != "" || items[3].Occurrence.Paid || items[4].AmountYuan != "100.00" || !items[4].Occurrence.Paid {
		t.Fatalf("creation amounts = %#v", items[3:])
	}
}

func TestCalendarActivityKeepsPlusBillAmountAndCostSnapshot(t *testing.T) {
	start := activityTime("2026-11-01T00:00:00+08:00")
	archivedAt := start.Add(5 * time.Hour)
	view := SubscriptionView{Subscription: model.Subscription{
		ID: 1, BusinessType: model.SubscriptionBusinessPlus, CronExpr: "interval:30d", BoardedAt: "2026-09-01",
		CreatedAt: start.AddDate(0, -2, 0), ArchivedAt: &archivedAt, PricePerPersonCents: 15000, CostCents: 5000,
	}}
	bill := model.Bill{ID: 1, SubscriptionID: 1, DueDate: "2026-10-01", AmountCents: 8800, CostCents: 2000, PaidAt: start.Add(time.Hour)}
	svc := &SubscriptionService{Clock: func() time.Time { return start }}
	items, err := svc.buildCalendarActivity(start, start.AddDate(0, 1, 0), nil, []SubscriptionView{view}, []model.Bill{bill}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[1].AmountYuan != "88.00" || items[1].Occurrence.PriceYuan != "88.00" || items[1].Occurrence.CostYuan != "20.00" || items[1].Occurrence.ProfitYuan != "68.00" {
		t.Fatalf("historical Plus payment = %#v", items)
	}
	if items[0].Kind != "archived" || items[0].Occurrence.CostYuan != "" || items[0].Occurrence.ProfitYuan != "" {
		t.Fatalf("unavailable Plus archive cost must remain unknown: %#v", items[0])
	}
}

func TestCalendarActivityUsesShanghaiMonthBoundaries(t *testing.T) {
	start := time.Date(2026, time.November, 1, 0, 0, 0, 0, cycle.Location)
	end := start.AddDate(0, 1, 0)
	view := SubscriptionView{Subscription: model.Subscription{
		ID: 1, Name: "boundary", CronExpr: "interval:30d", BoardedAt: "2026-10-01", CreatedAt: start.AddDate(0, -1, 0),
	}}
	bills := []model.Bill{
		{ID: 1, SubscriptionID: 1, DueDate: "2026-10-01", PaidAt: activityTime("2026-10-31T16:00:00Z")},
		{ID: 2, SubscriptionID: 1, DueDate: "2026-10-02", PaidAt: activityTime("2026-11-30T15:59:59Z")},
		{ID: 3, SubscriptionID: 1, DueDate: "2026-11-01", PaidAt: activityTime("2026-11-30T16:00:00Z")},
		{ID: 4, SubscriptionID: 1, DueDate: "2026-11-02", PaidAt: activityTime("2026-10-31T15:59:59Z")},
	}
	svc := &SubscriptionService{Clock: func() time.Time { return start }}
	items, err := svc.buildCalendarActivity(start, end, []SubscriptionView{view}, nil, bills, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "paid:2" || items[1].ID != "paid:1" || items[1].OccurredAtLabel != "2026-11-01 00:00" {
		t.Fatalf("month boundary activity = %#v", items)
	}
}

func TestCalendarActivityDoesNotMergeFirstLaterPaymentOrGuessFromDueDate(t *testing.T) {
	start := activityTime("2026-11-01T00:00:00+08:00")
	created := start.Add(time.Hour)
	view := SubscriptionView{Subscription: model.Subscription{
		ID: 1, CronExpr: "interval:30d", BoardedAt: "2026-11-01", CreatedAt: created,
	}}
	bill := model.Bill{ID: 1, SubscriptionID: 1, DueDate: "2026-11-01", CreatedAt: created.Add(time.Hour), PaidAt: created.Add(time.Hour)}
	svc := &SubscriptionService{Clock: func() time.Time { return created }}
	items, err := svc.buildCalendarActivity(start, start.AddDate(0, 1, 0), []SubscriptionView{view}, nil, []model.Bill{bill}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Kind != "paid" || items[1].Kind != "created" || items[1].AmountYuan != "" {
		t.Fatalf("separate creation and later payment = %#v", items)
	}
	items, err = svc.buildCalendarActivity(start.AddDate(0, 1, 0), start.AddDate(0, 2, 0), []SubscriptionView{view}, nil, []model.Bill{bill}, nil)
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("empty month = %#v, %v", items, err)
	}
}
