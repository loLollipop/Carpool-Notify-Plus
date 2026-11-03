package service

import (
	"testing"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

func TestGoalForecastDoesNotCountPaidPeriodsAgain(t *testing.T) {
	for _, test := range []struct {
		name        string
		business    string
		cost        int64
		paidPeriods int
	}{
		{"unpaid today", model.SubscriptionBusinessTeam, 0, 0},
		{"paid today", model.SubscriptionBusinessTeam, 0, 1},
		{"three prepaid Team periods", model.SubscriptionBusinessTeam, 0, 3},
		{"three prepaid Plus periods", model.SubscriptionBusinessPlus, 3000, 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := openGoalTestService(t)
			today := cycle.StartOfDay(service.now())
			id, err := service.Store.CreateSubscription(model.Subscription{
				Name: "forecast regression", BusinessType: test.business,
				PricePerPersonCents: 10000, CostCents: test.cost,
				CronExpr: "interval:30d", BoardedAt: cycle.FormatDate(today),
			})
			if err != nil {
				t.Fatal(err)
			}
			for index := 0; index < test.paidPeriods; index++ {
				if err := service.Store.SetDuePaid(id, cycle.FormatDate(today.AddDate(0, 0, index*30)), true, 10000, test.cost); err != nil {
					t.Fatal(err)
				}
			}
			forecast, err := service.buildProfitForecast(BusinessGoalProgress{RemainingProfitCents: 5000})
			if err != nil {
				t.Fatal(err)
			}
			want := cycle.FormatDate(today.AddDate(0, 0, test.paidPeriods*30))
			for name, scenario := range map[string]ForecastScenario{"baseline": forecast.Baseline, "optimistic": forecast.Optimistic, "conservative": forecast.Conservative} {
				if scenario.ProjectedDate != want {
					t.Fatalf("%s date = %q, want first unpaid %s", name, scenario.ProjectedDate, want)
				}
			}
		})
	}
}

func TestGoalForecastUsesOneBillingPeriodPerDay(t *testing.T) {
	forecasts := make([]ProfitForecast, 0, 2)
	for _, expression := range []string{"0 9 * * *", "0 9,18 * * *"} {
		service := openGoalTestService(t)
		id, err := service.Store.CreateSubscription(model.Subscription{
			Name: expression, BusinessType: model.SubscriptionBusinessTeam,
			PricePerPersonCents: 10000, CronExpr: expression, BoardedAt: "2026-08-15",
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := service.Store.SetDuePaid(id, "2026-08-15", true, 10000, 0); err != nil {
			t.Fatal(err)
		}
		forecast, err := service.buildProfitForecast(BusinessGoalProgress{RemainingProfitCents: 5000})
		if err != nil {
			t.Fatal(err)
		}
		if forecast.ActiveRecurringCount != 1 || forecast.Baseline.ProjectedDate != "2026-08-16" {
			t.Fatalf("%s forecast = %#v", expression, forecast)
		}
		forecasts = append(forecasts, forecast)
	}
	if forecasts[0] != forecasts[1] {
		t.Fatalf("daily and twice-daily forecasts differ: %#v", forecasts)
	}
}

func TestGoalForecastIncludesAccountCostWithoutOpeningDate(t *testing.T) {
	forecasts := make([]ProfitForecast, 0, 2)
	for _, openedAt := range []string{"2026-08-15", ""} {
		service := openGoalTestService(t)
		if _, err := service.CreateAccount(CreateAccountInput{Name: "cost anchor", OpenedAt: openedAt, CostYuan: "30.00", SeatCount: 1}); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Store.CreateSubscription(model.Subscription{Name: "monthly revenue", BusinessType: model.SubscriptionBusinessTeam,
			PricePerPersonCents: 10000, CronExpr: "interval:30d", BoardedAt: "2026-08-16"}); err != nil {
			t.Fatal(err)
		}
		forecast, err := service.buildProfitForecast(BusinessGoalProgress{RemainingProfitCents: 100000})
		if err != nil {
			t.Fatal(err)
		}
		forecasts = append(forecasts, forecast)
	}
	if forecasts[0] != forecasts[1] {
		t.Fatalf("missing opening date dropped future costs: %#v", forecasts)
	}
}

func TestSandboxResetClearsGoalsAndOperatingExpenses(t *testing.T) {
	service := openGoalTestService(t)
	live := openGoalTestService(t)
	liveGoal, err := live.CreateBusinessGoal(BusinessGoalInput{Name: "live target", TargetProfitYuan: "1000"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateBusinessGoal(BusinessGoalInput{Name: "old target", TargetProfitYuan: "1000"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateOperatingExpense(OperatingExpenseInput{OccurredOn: "2026-08-15", AmountYuan: "80"}); err != nil {
		t.Fatal(err)
	}
	if err := service.Store.SetSetting("reset-test", "preserved"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResetSandboxFixtures(); err != nil {
		t.Fatal(err)
	}
	goals, err := service.Store.ListBusinessGoals(20)
	if err != nil || len(goals) != 0 {
		t.Fatalf("goals after reset = %#v, %v", goals, err)
	}
	expenses, err := service.Store.ListOperatingExpenses()
	if err != nil || len(expenses) != 0 {
		t.Fatalf("expenses after reset = %#v, %v", expenses, err)
	}
	if _, err := service.CreateBusinessGoal(BusinessGoalInput{Name: "new target", TargetProfitYuan: "1000"}); err != nil {
		t.Fatal(err)
	}
	if goal, err := live.Store.GetActiveBusinessGoal(); err != nil || goal.ID != liveGoal {
		t.Fatalf("live goal changed: %#v, %v", goal, err)
	}
	if value, err := service.Store.GetSetting("reset-test"); err != nil || value != "preserved" {
		t.Fatalf("settings after reset = %q, %v", value, err)
	}
}

func TestForecastMissingOpeningDateIncludesTodaysUnrecordedCost(t *testing.T) {
	forecasts := make([]ProfitForecast, 0, 2)
	for _, missing := range []bool{false, true} {
		service := openGoalTestService(t)
		id, err := service.CreateAccount(CreateAccountInput{Name: "due cost", OpenedAt: "2026-07-15", CostYuan: "60", SeatCount: 1})
		if err != nil {
			t.Fatal(err)
		}
		if missing {
			account, err := service.Store.GetAccount(id)
			if err != nil {
				t.Fatal(err)
			}
			account.OpenedAt = ""
			if err := service.Store.UpdateAccount(account); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := service.Store.CreateSubscription(model.Subscription{Name: "today revenue", BusinessType: model.SubscriptionBusinessTeam,
			PricePerPersonCents: 10000, CronExpr: "interval:30d", BoardedAt: "2026-08-15"}); err != nil {
			t.Fatal(err)
		}
		forecast, err := service.buildProfitForecast(BusinessGoalProgress{RemainingProfitCents: 5000})
		if err != nil {
			t.Fatal(err)
		}
		if forecast.Baseline.ProjectedDate == "2026-08-15" {
			t.Fatalf("missing=%t omitted today's cost: %#v", missing, forecast)
		}
		forecasts = append(forecasts, forecast)
	}
	if forecasts[0] != forecasts[1] {
		t.Fatalf("missing opening date changes cost forecast: %#v", forecasts)
	}
}
