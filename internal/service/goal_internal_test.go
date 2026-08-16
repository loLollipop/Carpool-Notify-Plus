package service

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

type failingMarketClient struct{}

func (failingMarketClient) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("market unavailable in test")
}

func openGoalTestService(t *testing.T) *SubscriptionService {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "goal-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, cycle.Location)
	return &SubscriptionService{
		Store:        store,
		Clock:        func() time.Time { return now },
		MarketClient: failingMarketClient{},
	}
}

func TestBusinessGoalProgressUsesProfitBaselineAcrossTeamPlusAndRefunds(t *testing.T) {
	service := openGoalTestService(t)
	goalID, err := service.CreateBusinessGoal(BusinessGoalInput{
		Name:             "August profit",
		TargetProfitYuan: "1000.00",
	})
	if err != nil {
		t.Fatal(err)
	}

	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "team-owner@example.com",
		CostYuan:  "30.00",
		SeatCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := service.Store.ListSeatsByAccount(accountID)
	if err != nil || len(seats) != 1 {
		t.Fatalf("seats = %#v, err = %v", seats, err)
	}
	teamID, err := service.Store.CreateSubscription(model.Subscription{
		Name:                "Team customer",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 10000,
		CronExpr:            "interval:30d",
		CustomerEmail:       "team-customer@example.com",
		SeatID:              seats[0].ID,
		BoardedAt:           "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Store.SetDuePaid(teamID, "2026-08-01", true, 10000); err != nil {
		t.Fatal(err)
	}

	plusID, err := service.Store.CreateSubscription(model.Subscription{
		Name:                "Plus customer",
		BusinessType:        model.SubscriptionBusinessPlus,
		PricePerPersonCents: 12000,
		CostCents:           2000,
		CronExpr:            "interval:30d",
		CustomerEmail:       "plus-account@example.com",
		CustomerWechat:      "plus-contact",
		BoardedAt:           "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Store.SetDuePaid(plusID, "2026-08-01", true, 12000, 2000); err != nil {
		t.Fatal(err)
	}
	cancellation, err := service.RequestCancellation(plusID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateAfterSalesCase(cancellation.CaseID, UpdateAfterSalesCaseInput{
		RefundAmountYuan: "10.00",
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.SetAfterSalesCaseRefunded(cancellation.CaseID, true); err != nil {
		t.Fatal(err)
	}

	center, err := service.GetGoalCenter()
	if err != nil {
		t.Fatal(err)
	}
	if center.ActiveGoal == nil || center.ActiveGoal.Goal.ID != goalID {
		t.Fatalf("active goal = %#v, want id %d", center.ActiveGoal, goalID)
	}
	// Team: 100 - 30 owner cost. Plus: 120 - 20 period cost - 10 refund.
	if center.ActiveGoal.EarnedProfitCents != 16000 {
		t.Fatalf("earned profit = %d, want 16000", center.ActiveGoal.EarnedProfitCents)
	}
	if center.ActiveGoal.RemainingProfitCents != 84000 || center.ActiveGoal.ProgressPercent != 16 {
		t.Fatalf("goal progress = %#v", center.ActiveGoal)
	}
}

func TestGoalForecastUsesActiveFutureRevenueAndScheduledPrices(t *testing.T) {
	service := openGoalTestService(t)
	if _, err := service.CreateBusinessGoal(BusinessGoalInput{
		Name:             "Future revenue goal",
		TargetProfitYuan: "1000.00",
	}); err != nil {
		t.Fatal(err)
	}
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "forecast-owner@example.com",
		CostYuan:  "30.00",
		SeatCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := service.Store.ListSeatsByAccount(accountID)
	if err != nil || len(seats) != 1 {
		t.Fatalf("seats = %#v, err = %v", seats, err)
	}
	subscriptionID, err := service.Store.CreateSubscription(model.Subscription{
		Name:                "Scheduled price customer",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 10000,
		CronExpr:            "interval:30d",
		SeatID:              seats[0].ID,
		BoardedAt:           "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ScheduleBulkNextPrice(BulkNextPriceInput{
		SubscriptionIDs: []int64{subscriptionID},
		NextPriceYuan:   "120.00",
	}); err != nil {
		t.Fatal(err)
	}

	center, err := service.GetGoalCenter()
	if err != nil {
		t.Fatal(err)
	}
	if center.Forecast == nil {
		t.Fatal("forecast is nil")
	}
	// A 30-day cycle is normalized with 365/(30*12), then the monthly
	// owner-account cost is deducted: 12000*365/360 - 3000 = 9166.
	if center.Forecast.RunRateMonthlyProfitCents != 9166 {
		t.Fatalf("run rate = %d, want 9166", center.Forecast.RunRateMonthlyProfitCents)
	}
	if center.Forecast.ActiveRecurringCount != 1 || center.Forecast.Source != "run_rate" {
		t.Fatalf("forecast basis = %#v", center.Forecast)
	}
	if center.Forecast.Baseline.ProjectedDate == "" {
		t.Fatalf("baseline forecast = %#v", center.Forecast.Baseline)
	}
}

func TestPricingCandidatesAndBulkNextPriceAreMarketAwareAndAtomic(t *testing.T) {
	service := openGoalTestService(t)
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "bulk-owner@example.com",
		CostYuan:  "50.00",
		SeatCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := service.Store.ListSeatsByAccount(accountID)
	if err != nil || len(seats) != 2 {
		t.Fatalf("seats = %#v, err = %v", seats, err)
	}
	teamIDs := make([]int64, 0, 2)
	for index, price := range []int64{9000, 12000} {
		subscriptionID, createErr := service.Store.CreateSubscription(model.Subscription{
			Name:                fmt.Sprintf("Team customer %d", index+1),
			BusinessType:        model.SubscriptionBusinessTeam,
			PricePerPersonCents: price,
			CronExpr:            "interval:30d",
			CustomerWechat:      fmt.Sprintf("wechat-%d", index+1),
			SeatID:              seats[index].ID,
			BoardedAt:           "2026-08-01",
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		teamIDs = append(teamIDs, subscriptionID)
	}
	if _, err := service.Store.InsertMarketPriceSnapshot(model.MarketPriceSnapshot{
		Provider:         marketProvider,
		Product:          marketProduct,
		LowPriceCents:    10000,
		MedianPriceCents: 13000,
		HighPriceCents:   15000,
		SampleCount:      10,
		SourceUpdatedAt:  service.now(),
		CreatedAt:        service.now(),
	}); err != nil {
		t.Fatal(err)
	}
	center, err := service.GetGoalCenter()
	if err != nil {
		t.Fatal(err)
	}
	if len(center.Candidates) != 2 || !center.Candidates[0].Recommended || center.Candidates[0].MarketPosition != "below_low" {
		t.Fatalf("pricing candidates = %#v", center.Candidates)
	}
	if center.Candidates[1].MarketPosition != "below_median" {
		t.Fatalf("second candidate = %#v", center.Candidates[1])
	}

	updated, err := service.ScheduleBulkNextPrice(BulkNextPriceInput{
		SubscriptionIDs: teamIDs,
		NextPriceYuan:   "135.00",
	})
	if err != nil || updated != 2 {
		t.Fatalf("bulk result = %d, err = %v", updated, err)
	}
	for index, subscriptionID := range teamIDs {
		subscription, getErr := service.Get(subscriptionID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if subscription.PricePerPersonCents != []int64{9000, 12000}[index] || subscription.NextPriceCents == nil || *subscription.NextPriceCents != 13500 {
			t.Fatalf("scheduled subscription = %#v", subscription)
		}
		if subscription.NextPriceEffectiveDueDate == "" {
			t.Fatalf("missing effective due date: %#v", subscription)
		}
	}

	plusID, err := service.Store.CreateSubscription(model.Subscription{
		Name:                "Plus is not eligible",
		BusinessType:        model.SubscriptionBusinessPlus,
		PricePerPersonCents: 6800,
		CronExpr:            "interval:30d",
		CustomerEmail:       "plus@example.com",
		CustomerWechat:      "plus-wechat",
		BoardedAt:           "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := service.Get(teamIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.ScheduleBulkNextPrice(BulkNextPriceInput{
		SubscriptionIDs: []int64{teamIDs[0], plusID},
		NextPriceYuan:   "140.00",
	})
	if err == nil {
		t.Fatal("bulk update with Plus subscription unexpectedly succeeded")
	}
	after, err := service.Get(teamIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if after.NextPriceCents == nil || before.NextPriceCents == nil || *after.NextPriceCents != *before.NextPriceCents {
		t.Fatalf("failed bulk update partially changed Team price: before=%#v after=%#v", before, after)
	}
}

func TestComparableMarketPricesFiltersMixedProductsAndDeduplicates(t *testing.T) {
	newest := time.Date(2026, time.August, 15, 10, 0, 0, 0, time.UTC)
	older := newest.Add(-time.Hour)
	stock := 5
	zeroStock := 0
	offers := []priceAIOffer{
		{SourceID: "seller-a", SourceTitle: "ChatGPT Business 标准席位", Price: 113, Currency: "CNY", Status: "in_stock", EffectiveStatus: "available", StockCount: &stock, SourceUpdatedAt: &older},
		{SourceID: "seller-a", SourceTitle: "ChatGPT Business 标准席位", Price: 113, Currency: "CNY", Status: "in_stock", EffectiveStatus: "available", StockCount: &stock, SourceUpdatedAt: &newest},
		{SourceID: "seller-b", SourceTitle: "ChatGPT Team 激活码", Price: 130, Currency: "CNY", Status: "low_stock", EffectiveStatus: "available", StockCount: &stock, SourceUpdatedAt: &newest},
		{SourceID: "seller-c", SourceTitle: "ChatGPT Business Slot 30 days", Price: 150, Currency: "CNY", Status: "in_stock", EffectiveStatus: "available", StockCount: &stock, SourceUpdatedAt: &newest},
		{SourceID: "seller-d", SourceTitle: "ChatGPT Business Premium 高级席位", Price: 200, Currency: "CNY", Status: "in_stock", EffectiveStatus: "available", StockCount: &stock},
		{SourceID: "seller-e", SourceTitle: "ChatGPT Team 2 席位母号", Price: 240, Currency: "CNY", Status: "in_stock", EffectiveStatus: "available", StockCount: &stock},
		{SourceID: "seller-f", SourceTitle: "ChatGPT Business 席位", Price: 100, Currency: "CNY", Status: "out_of_stock", EffectiveStatus: "unavailable", StockCount: &zeroStock},
	}

	prices, updatedAt := comparableMarketPrices(offers)
	if len(prices) != 3 || prices[0] != 11300 || prices[1] != 13000 || prices[2] != 15000 {
		t.Fatalf("comparable prices = %#v", prices)
	}
	if !updatedAt.Equal(newest) {
		t.Fatalf("source updated at = %s, want %s", updatedAt, newest)
	}
	if low, median, high := percentileCents(prices, 0.25), percentileCents(prices, 0.5), percentileCents(prices, 0.75); low != 12150 || median != 13000 || high != 14000 {
		t.Fatalf("quartiles = %d/%d/%d, want 12150/13000/14000", low, median, high)
	}
}

func TestPricingRecommendationRespectsUtilizationBeforeRaisingPrice(t *testing.T) {
	snapshot := &model.MarketPriceSnapshot{
		LowPriceCents:    13000,
		MedianPriceCents: 14000,
		HighPriceCents:   15000,
		SampleCount:      8,
	}

	lowUtilization := openGoalTestService(t)
	seedPricingSeats(t, lowUtilization, 10, 4, 10000)
	lowAdvice, err := lowUtilization.buildPricingRecommendation(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if lowAdvice.Action != "fill" || lowAdvice.UtilizationPercent != 40 {
		t.Fatalf("low-utilization advice = %#v", lowAdvice)
	}

	highUtilization := openGoalTestService(t)
	seedPricingSeats(t, highUtilization, 10, 9, 10000)
	highAdvice, err := highUtilization.buildPricingRecommendation(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if highAdvice.Action != "raise" || highAdvice.UtilizationPercent != 90 {
		t.Fatalf("high-utilization advice = %#v", highAdvice)
	}
}

func seedPricingSeats(t *testing.T, service *SubscriptionService, total int, used int, priceCents int64) {
	t.Helper()
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "pricing-owner@example.com",
		CostYuan:  "100.00",
		SeatCount: total,
	})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := service.Store.ListSeatsByAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < used; index++ {
		if _, err := service.Store.CreateSubscription(model.Subscription{
			Name:                "Team pricing customer",
			BusinessType:        model.SubscriptionBusinessTeam,
			PricePerPersonCents: priceCents,
			CronExpr:            "interval:30d",
			SeatID:              seats[index].ID,
			BoardedAt:           "2026-08-01",
		}); err != nil {
			t.Fatal(err)
		}
	}
}
