package service_test

import (
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/service"
)

func TestCalendarActivityIncludesCreatedRenewedAndArchivedSubscriptions(t *testing.T) {
	svc := openTestService(t)
	// Store operations use real UTC timestamps. Query the month they wrote,
	// independently of the deliberately historical billing dates.
	now := time.Now().In(cycle.Location)
	svc.Clock = func() time.Time { return now }
	_, seats := createTestAccountWithSeats(t, svc, "activity@example.test", "one", "two")
	input := service.CreateInput{
		Name: "activity customer", CustomerEmail: "customer@example.test", PriceYuan: "100", CronExpr: "interval:30d",
		BoardedAt: "2026-07-01", NotifyOffsetsRaw: "0", SeatID: seats[0],
	}
	id, err := svc.CreateWithInitialBill(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Name = "no initial bill"
	input.SeatID = seats[1]
	unbilled, err := svc.Create(input)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDuePaid(id, "2026-07-31", true); err != nil {
		t.Fatal(err)
	}
	if err := svc.Archive(id); err != nil {
		t.Fatal(err)
	}
	view, err := svc.CalendarMonth(now)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, item := range view.Activity {
		counts[item.Kind]++
		if item.Occurrence.SubscriptionID == id && !item.Archived {
			t.Fatalf("archived history lost archived state: %#v", item)
		}
		if item.Kind == "created" && item.Occurrence.SubscriptionID == unbilled && item.AmountYuan != "" {
			t.Fatalf("unbilled creation reports payment: %#v", item)
		}
	}
	if len(view.Activity) != 4 || counts["created"] != 2 || counts["paid"] != 1 || counts["archived"] != 1 {
		t.Fatalf("activity counts = %v, want 2 creations, 1 renewal and 1 archive", counts)
	}
	if err := svc.SetDuePaid(unbilled, "2026-07-31", true); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetDuePaid(unbilled, "2026-07-31", false); err != nil {
		t.Fatal(err)
	}
	view, err = svc.CalendarMonth(now)
	if err != nil || len(view.Activity) != 4 {
		t.Fatalf("cancelled payment remains in activity: %d, %v", len(view.Activity), err)
	}
	previous, err := svc.CalendarMonth(now.AddDate(0, -1, 0))
	if err != nil || len(previous.Activity) != 0 {
		t.Fatalf("archive leaked into other month: %d, %v", len(previous.Activity), err)
	}
}
