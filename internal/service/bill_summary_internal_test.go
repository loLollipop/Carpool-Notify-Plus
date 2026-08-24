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
			CustomerEmail:    "customer@example.com",
			AccountName:      "account",
			DueDate:          "2026-07-20",
			AmountCents:      9000,
			PaidAt:           time.Date(2026, time.August, 3, 12, 0, 0, 0, cycle.Location),
		},
	}, time.Date(2026, time.August, 3, 12, 0, 0, 0, cycle.Location))

	if summary.ThisMonthCount != 0 || summary.ThisMonthAmountYuan != "0.00" {
		t.Fatalf("august received = count %d amount %s, want 0 / 0.00", summary.ThisMonthCount, summary.ThisMonthAmountYuan)
	}
	if len(summary.AmountBySubscription) != 1 || summary.AmountBySubscription[0].CustomerEmail != "customer@example.com" {
		t.Fatalf("subscription customer email = %#v, want customer@example.com", summary.AmountBySubscription)
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

func TestBillsSummaryExposesUnlinkedRefundForDetailReconciliation(t *testing.T) {
	processedAt := time.Date(2026, time.August, 3, 12, 30, 0, 0, cycle.Location)
	summary := buildBillsSummaryWithRefunds([]BillView{
		{
			ID:               7,
			SubscriptionID:   1,
			SubscriptionName: "customer",
			BusinessType:     model.SubscriptionBusinessTeam,
			DueDate:          "2026-07-20",
			AmountCents:      3000,
		},
	}, []model.AfterSalesCase{
		{
			ID:                9,
			BillID:            0,
			SubscriptionID:    1,
			BusinessType:      model.SubscriptionBusinessTeam,
			CustomerEmail:     "customer@example.com",
			RefundAmountCents: 1000,
			Status:            model.AfterSalesStatusRefunded,
			ProcessedAt:       &processedAt,
		},
	}, 0, processedAt)

	if summary.TotalRefundYuan != "10.00" || summary.ThisMonthNetAmountYuan != "-10.00" {
		t.Fatalf("refund summary = total %s month net %s, want 10.00 / -10.00", summary.TotalRefundYuan, summary.ThisMonthNetAmountYuan)
	}
	if len(summary.RefundDetails) != 1 {
		t.Fatalf("refund details = %#v, want one row", summary.RefundDetails)
	}
	detail := summary.RefundDetails[0]
	if detail.BillID != 0 || detail.CustomerEmail != "customer@example.com" || detail.ProcessedMonth != "2026-08" || detail.AmountCents != 1000 {
		t.Fatalf("refund detail = %#v, want unlinked August customer refund", detail)
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

func TestBillsSummaryOmitsFullyRefundedSubscriptionFromAmountDistributionAfterSeatReuse(t *testing.T) {
	processedAt := time.Date(2026, time.August, 24, 16, 22, 0, 0, cycle.Location)
	summary := buildBillsSummaryWithRefunds([]BillView{
		{
			ID:               51,
			SubscriptionID:   48,
			SubscriptionName: "refunded-customer",
			BusinessType:     model.SubscriptionBusinessTeam,
			AccountID:        26,
			AccountName:      "owner",
			SeatID:           48,
			SeatName:         "seat-1",
			DueDate:          "2026-08-24",
			AmountCents:      12000,
			RefundCents:      12000,
			NetAmountCents:   0,
			NetAmountYuan:    "0.00",
			Archived:         true,
		},
		{
			ID:               54,
			SubscriptionID:   51,
			SubscriptionName: "replacement-customer",
			BusinessType:     model.SubscriptionBusinessTeam,
			AccountID:        26,
			AccountName:      "owner",
			SeatID:           48,
			SeatName:         "seat-1",
			DueDate:          "2026-08-24",
			AmountCents:      11500,
			NetAmountCents:   11500,
			NetAmountYuan:    "115.00",
		},
	}, []model.AfterSalesCase{
		{
			ID:                2,
			BillID:            51,
			SubscriptionID:    48,
			BusinessType:      model.SubscriptionBusinessTeam,
			PaidAmountCents:   12000,
			RefundAmountCents: 12000,
			Status:            model.AfterSalesStatusRefunded,
			ProcessedAt:       &processedAt,
		},
	}, 0, processedAt)

	if len(summary.AmountBySubscription) != 1 ||
		summary.AmountBySubscription[0].SubscriptionID != 51 ||
		summary.AmountBySubscription[0].AmountCents != 11500 {
		t.Fatalf("amount distribution = %#v, want only replacement subscription at 11500", summary.AmountBySubscription)
	}
	if summary.BillCount != 2 || summary.TotalAmountYuan != "235.00" ||
		summary.TotalRefundYuan != "120.00" || summary.NetAmountYuan != "115.00" {
		t.Fatalf(
			"financial history = bills %d gross %s refund %s net %s, want 2 / 235.00 / 120.00 / 115.00",
			summary.BillCount,
			summary.TotalAmountYuan,
			summary.TotalRefundYuan,
			summary.NetAmountYuan,
		)
	}
}
