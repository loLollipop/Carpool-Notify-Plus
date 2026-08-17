package service

import (
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
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

func TestBillsSummaryDoesNotDoubleCountLegacyTeamBillCost(t *testing.T) {
	summary := buildBillsSummaryWithRefunds([]BillView{
		{
			SubscriptionID:   1,
			SubscriptionName: "legacy-team",
			BusinessType:     model.SubscriptionBusinessTeam,
			AccountID:        7,
			AccountName:      "owner",
			DueDate:          "2026-07-20",
			AmountCents:      9000,
			CostCents:        4500,
		},
	}, nil, 4500, time.Date(2026, time.August, 3, 12, 0, 0, 0, cycle.Location))

	if summary.TotalCostCents != 4500 || summary.TotalCostYuan != "45.00" {
		t.Fatalf("total cost = %d/%s, want account ledger only 4500/45.00", summary.TotalCostCents, summary.TotalCostYuan)
	}
	if summary.TotalProfitCents != 4500 || summary.TotalProfitYuan != "45.00" {
		t.Fatalf("total profit = %d/%s, want 4500/45.00", summary.TotalProfitCents, summary.TotalProfitYuan)
	}
}

func TestBillsSummarySeparatesSameNamedAccountsAndStabilizesDuplicateLabels(t *testing.T) {
	summary := buildBillsSummary([]BillView{
		{
			SubscriptionID:   2,
			SubscriptionName: "same-customer",
			BusinessType:     model.SubscriptionBusinessTeam,
			AccountID:        12,
			AccountName:      "same-owner",
			DueDate:          "2026-08-02",
			AmountCents:      9000,
		},
		{
			SubscriptionID:   1,
			SubscriptionName: "same-customer",
			BusinessType:     model.SubscriptionBusinessTeam,
			AccountID:        11,
			AccountName:      "same-owner",
			DueDate:          "2026-08-01",
			AmountCents:      9000,
		},
	}, time.Date(2026, time.August, 3, 12, 0, 0, 0, cycle.Location))

	if len(summary.Accounts) != 2 {
		t.Fatalf("same-named physical accounts were merged: %#v", summary.Accounts)
	}
	if summary.Accounts[0].Key == summary.Accounts[1].Key || summary.Accounts[0].AccountID == summary.Accounts[1].AccountID {
		t.Fatalf("account identities are not distinct: %#v", summary.Accounts)
	}
	if len(summary.AmountBySubscription) != 2 ||
		summary.AmountBySubscription[0].SubscriptionID != 1 ||
		summary.AmountBySubscription[1].SubscriptionID != 2 {
		t.Fatalf("duplicate-label subscription order is unstable: %#v", summary.AmountBySubscription)
	}
}
