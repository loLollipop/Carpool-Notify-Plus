package service

import (
	"errors"
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
		Deadline:         "2026-12-31",
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
