package service

import (
	"path/filepath"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
)

func openOperatingExpenseTestService(t *testing.T) *SubscriptionService {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "operating-expense-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, time.August, 23, 12, 0, 0, 0, cycle.Location)
	return &SubscriptionService{
		Store: store,
		Clock: func() time.Time { return now },
	}
}

func TestOperatingExpenseFlowsThroughFinancialReporting(t *testing.T) {
	service := openOperatingExpenseTestService(t)
	expenseID, err := service.CreateOperatingExpense(OperatingExpenseInput{
		OccurredOn: "2026-08-20",
		AmountYuan: "12.34",
		Note:       "Xianyu exposure",
	})
	if err != nil {
		t.Fatal(err)
	}

	page, err := service.ListBillsPage()
	if err != nil {
		t.Fatal(err)
	}
	if len(page.OperatingExpenses) != 1 || page.OperatingExpenses[0].ID != expenseID {
		t.Fatalf("operating expense views = %#v", page.OperatingExpenses)
	}
	if page.Summary.TotalCostCents != 1234 || page.Summary.TotalProfitCents != -1234 {
		t.Fatalf("bill summary = %#v, want cost 1234 and profit -1234", page.Summary)
	}
	if page.Summary.ThisMonthOperatingExpenseCents != 1234 ||
		page.Summary.OperatingExpenseMonthlyAverageCents != 1234 {
		t.Fatalf("expense summary = %#v, want current and monthly average 1234", page.Summary)
	}

	dashboard, err := service.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.TotalCostCents != 1234 || dashboard.TotalProfitCents != -1234 {
		t.Fatalf("dashboard = %#v, want cost 1234 and profit -1234", dashboard)
	}
	exported, err := service.Export()
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.OperatingExpenses) != 1 || exported.OperatingExpenses[0].AmountCents != 1234 {
		t.Fatalf("exported operating expenses = %#v", exported.OperatingExpenses)
	}

	trend, err := service.buildProfitTrend(6)
	if err != nil {
		t.Fatal(err)
	}
	byMonth := make(map[string]ProfitMonth, len(trend))
	for _, month := range trend {
		byMonth[month.Month] = month
	}
	if august := byMonth["2026-08"]; august.CostCents != 1234 || august.ProfitCents != -1234 {
		t.Fatalf("August trend = %#v, want cost/profit 1234/-1234", august)
	}
	runRate, count, err := service.activeMonthlyProfitRunRate()
	if err != nil {
		t.Fatal(err)
	}
	if runRate != -1234 || count != 0 {
		t.Fatalf("run rate = %d, count = %d, want -1234/0", runRate, count)
	}
}

func TestOperatingExpenseCanBeUpdatedDeletedAndRejectsFutureDate(t *testing.T) {
	service := openOperatingExpenseTestService(t)
	if _, err := service.CreateOperatingExpense(OperatingExpenseInput{
		OccurredOn: "2026-08-24",
		AmountYuan: "10",
	}); err == nil {
		t.Fatal("expected future operating expense date to be rejected")
	}
	expenseID, err := service.CreateOperatingExpense(OperatingExpenseInput{
		OccurredOn: "2026-08-23",
		AmountYuan: "10",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateOperatingExpense(expenseID, OperatingExpenseInput{
		OccurredOn: "2026-08-22",
		AmountYuan: "8.50",
		Note:       "corrected",
	}); err != nil {
		t.Fatal(err)
	}
	expense, err := service.Store.GetOperatingExpense(expenseID)
	if err != nil {
		t.Fatal(err)
	}
	if expense.AmountCents != 850 || expense.OccurredOn != "2026-08-22" || expense.Note != "corrected" {
		t.Fatalf("updated expense = %#v", expense)
	}
	if err := service.DeleteOperatingExpense(expenseID); err != nil {
		t.Fatal(err)
	}
	expenses, err := service.Store.ListOperatingExpenses()
	if err != nil {
		t.Fatal(err)
	}
	if len(expenses) != 0 {
		t.Fatalf("expenses after delete = %#v", expenses)
	}
}
