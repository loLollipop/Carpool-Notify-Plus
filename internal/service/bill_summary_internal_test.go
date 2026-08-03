package service

import (
	"testing"
	"time"

	"carpool-notify/internal/cycle"
)

func TestBillsSummaryUsesDueDateMonthForBackfilledPayment(t *testing.T) {
	summary := buildBillsSummary([]BillView{
		{
			SubscriptionID:   1,
			SubscriptionName: "backfilled",
			AccountName:      "account",
			DueDate:          "2026-07-20",
			AmountCents:      9000,
			PaidAt:           time.Date(2026, time.August, 3, 12, 0, 0, 0, cycle.Location),
		},
	}, time.Date(2026, time.August, 3, 12, 0, 0, 0, cycle.Location))

	if summary.ThisMonthCount != 0 || summary.ThisMonthAmountYuan != "0.00" {
		t.Fatalf("august received = count %d amount %s, want 0 / 0.00", summary.ThisMonthCount, summary.ThisMonthAmountYuan)
	}

	var july MonthAmountBar
	for _, month := range summary.MonthlyTrend {
		if month.Month == "2026-07" {
			july = month
			break
		}
	}
	if july.Count != 1 || july.AmountYuan != "90.00" {
		t.Fatalf("july trend = %+v, want one 90.00 bill", july)
	}
}
