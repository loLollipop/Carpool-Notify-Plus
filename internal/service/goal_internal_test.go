package service

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

type countingMarketClient struct {
	calls atomic.Int32
	delay time.Duration
}

func (client *countingMarketClient) Do(*http.Request) (*http.Response, error) {
	client.calls.Add(1)
	if client.delay > 0 {
		time.Sleep(client.delay)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(`{
			"offers": [
				{"sourceId":"a","sourceTitle":"ChatGPT Business 席位","price":110,"currency":"CNY","status":"in_stock","effectiveStatus":"available"},
				{"sourceId":"b","sourceTitle":"ChatGPT Team 激活码","price":130,"currency":"CNY","status":"in_stock","effectiveStatus":"available"},
				{"sourceId":"c","sourceTitle":"ChatGPT Business Slot","price":150,"currency":"CNY","status":"in_stock","effectiveStatus":"available"},
				{"sourceId":"r1","sourceTitle":"ChatGPT Team 续费码","price":135,"currency":"CNY","status":"in_stock","effectiveStatus":"available"},
				{"sourceId":"r2","sourceTitle":"ChatGPT Business renewal","price":145,"currency":"CNY","status":"in_stock","effectiveStatus":"available"},
				{"sourceId":"r3","sourceTitle":"ChatGPT Team renew slot","price":155,"currency":"CNY","status":"in_stock","effectiveStatus":"available"}
			],
			"generatedAt":"2026-08-15T04:00:00Z"
		}`)),
	}, nil
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

func TestMarketRefreshIsCachedConcurrentAndRefreshesAfterTTL(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "market-refresh-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, cycle.Location)
	client := &countingMarketClient{delay: 15 * time.Millisecond}
	service := &SubscriptionService{
		Store:        store,
		Clock:        func() time.Time { return now },
		MarketClient: client,
	}

	const workers = 8
	var wait sync.WaitGroup
	errorsByWorker := make(chan error, workers)
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			view, refreshErr := service.RefreshMarketPriceIfStale()
			if refreshErr != nil {
				errorsByWorker <- refreshErr
				return
			}
			if !view.Available || view.AcquisitionSnapshot == nil || view.RenewalSnapshot == nil {
				errorsByWorker <- fmt.Errorf("market view unavailable: %#v", view)
			}
		}()
	}
	wait.Wait()
	close(errorsByWorker)
	for workerErr := range errorsByWorker {
		t.Fatal(workerErr)
	}
	if calls := client.calls.Load(); calls != 1 {
		t.Fatalf("concurrent refresh calls = %d, want 1", calls)
	}
	history, err := store.ListMarketPriceSnapshots(marketProvider, marketProduct, 10)
	if err != nil || len(history) != 1 {
		t.Fatalf("market history after concurrent refresh = %#v, err = %v", history, err)
	}
	acquisitionHistory, err := store.ListMarketPriceSnapshots(marketProvider, marketAcquisitionProduct, 10)
	if err != nil || len(acquisitionHistory) != 1 {
		t.Fatalf("acquisition history after concurrent refresh = %#v, err = %v", acquisitionHistory, err)
	}

	if _, err := service.RefreshMarketPriceIfStale(); err != nil {
		t.Fatal(err)
	}
	if calls := client.calls.Load(); calls != 1 {
		t.Fatalf("fresh cache triggered another request: %d", calls)
	}

	now = now.Add(MarketRefreshInterval() + time.Minute)
	if _, err := service.RefreshMarketPriceIfStale(); err != nil {
		t.Fatal(err)
	}
	if calls := client.calls.Load(); calls != 2 {
		t.Fatalf("expired cache requests = %d, want 2", calls)
	}
	history, err = store.ListMarketPriceSnapshots(marketProvider, marketProduct, 10)
	if err != nil || len(history) != 2 {
		t.Fatalf("market history after TTL refresh = %#v, err = %v", history, err)
	}
	acquisitionHistory, err = store.ListMarketPriceSnapshots(marketProvider, marketAcquisitionProduct, 10)
	if err != nil || len(acquisitionHistory) != 2 {
		t.Fatalf("acquisition history after TTL refresh = %#v, err = %v", acquisitionHistory, err)
	}
}

func TestMarketRefreshFailureKeepsLatestSnapshot(t *testing.T) {
	service := openGoalTestService(t)
	latest := model.MarketPriceSnapshot{
		Provider:         marketProvider,
		Product:          marketProduct,
		LowPriceCents:    11000,
		MedianPriceCents: 13000,
		HighPriceCents:   15000,
		SampleCount:      8,
		SourceUpdatedAt:  service.now().Add(-8 * time.Hour),
		CreatedAt:        service.now().Add(-7 * time.Hour),
	}
	if _, err := service.Store.InsertMarketPriceSnapshot(latest); err != nil {
		t.Fatal(err)
	}

	view, err := service.RefreshMarketPriceIfStale()
	if err != nil {
		t.Fatal(err)
	}
	if !view.Available || !view.Stale || view.Snapshot == nil ||
		view.Snapshot.MedianPriceCents != latest.MedianPriceCents || view.Warning == "" {
		t.Fatalf("fallback market view = %#v", view)
	}
}

func TestProfitTrendUsesBillingPeriodInsteadOfImportMonth(t *testing.T) {
	service := openGoalTestService(t)
	subscriptionID, err := service.Store.CreateSubscription(model.Subscription{
		Name:                "Imported July Plus rental",
		BusinessType:        model.SubscriptionBusinessPlus,
		PricePerPersonCents: 4200,
		CostCents:           1000,
		CronExpr:            "interval:30d",
		CustomerEmail:       "july-plus@example.com",
		BoardedAt:           "2026-07-15",
	})
	if err != nil {
		t.Fatal(err)
	}
	// SetDuePaid records paid_at at import time. The trend must still place the
	// bill in its July accounting period via due_date.
	if err := service.Store.SetDuePaid(subscriptionID, "2026-07-15", true, 4200, 1000); err != nil {
		t.Fatal(err)
	}
	trend, err := service.buildProfitTrend(6)
	if err != nil {
		t.Fatal(err)
	}
	byMonth := make(map[string]ProfitMonth, len(trend))
	for _, month := range trend {
		byMonth[month.Month] = month
	}
	if july := byMonth["2026-07"]; july.RevenueCents != 4200 || july.CostCents != 1000 || july.ProfitCents != 3200 {
		t.Fatalf("July trend = %#v, want revenue=4200 cost=1000 profit=3200", july)
	}
	if august := byMonth["2026-08"]; august.RevenueCents != 0 {
		t.Fatalf("import month incorrectly received July revenue: %#v", august)
	}
}

func TestProfitTrendIgnoresLegacyTeamBillCostSnapshot(t *testing.T) {
	service := openGoalTestService(t)
	subscriptionID, err := service.Store.CreateSubscription(model.Subscription{
		Name:                "Legacy Team bill cost",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 9000,
		CronExpr:            "interval:30d",
		BoardedAt:           "2026-07-15",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Historical versions could persist the Team owner cost on every bill.
	// Reporting must use the account ledger only and ignore this stale snapshot.
	if err := service.Store.SetDuePaid(subscriptionID, "2026-07-15", true, 9000, 4500); err != nil {
		t.Fatal(err)
	}

	trend, err := service.buildProfitTrend(6)
	if err != nil {
		t.Fatal(err)
	}
	byMonth := make(map[string]ProfitMonth, len(trend))
	for _, month := range trend {
		byMonth[month.Month] = month
	}
	if july := byMonth["2026-07"]; july.RevenueCents != 9000 || july.CostCents != 0 || july.ProfitCents != 9000 {
		t.Fatalf("legacy Team bill cost was counted again: %#v", july)
	}
}

func TestProfitTrendUsesAccountOpeningPeriodForImportedInitialCost(t *testing.T) {
	service := openGoalTestService(t)
	accountID, err := service.Store.CreateAccount(model.Account{
		Name:      "Imported July Team account",
		OpenedAt:  "2026-07-18",
		CostCents: 4500,
	}, 4500, "2026-08-07")
	if err != nil {
		t.Fatal(err)
	}
	seatID, err := service.Store.CreateSeat(model.Seat{AccountID: accountID, Name: "seat1"})
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, err := service.Store.CreateSubscription(model.Subscription{
		Name:                "Imported July Team customer",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 9000,
		CronExpr:            "interval:30d",
		SeatID:              seatID,
		BoardedAt:           "2026-07-20",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Store.SetDuePaid(subscriptionID, "2026-07-20", true, 9000); err != nil {
		t.Fatal(err)
	}

	trend, err := service.buildProfitTrend(6)
	if err != nil {
		t.Fatal(err)
	}
	byMonth := make(map[string]ProfitMonth, len(trend))
	for _, month := range trend {
		byMonth[month.Month] = month
	}
	if july := byMonth["2026-07"]; july.RevenueCents != 9000 || july.CostCents != 4500 || july.ProfitCents != 4500 {
		t.Fatalf("July trend = %#v, want revenue=9000 cost=4500 profit=4500", july)
	}
	if august := byMonth["2026-08"]; august.CostCents != 0 {
		t.Fatalf("import month incorrectly received July account cost: %#v", august)
	}
}

func TestProfitTrendKeepsHistoricalCumulativeCostInImportMonth(t *testing.T) {
	service := openGoalTestService(t)
	if _, err := service.Store.CreateAccount(model.Account{
		Name:      "Historical Team account",
		OpenedAt:  "2026-01-18",
		CostCents: 2000,
	}, 7500, "2026-08-07"); err != nil {
		t.Fatal(err)
	}

	trend, err := service.buildProfitTrend(6)
	if err != nil {
		t.Fatal(err)
	}
	byMonth := make(map[string]ProfitMonth, len(trend))
	for _, month := range trend {
		byMonth[month.Month] = month
	}
	if august := byMonth["2026-08"]; august.CostCents != 7500 || august.ProfitCents != -7500 {
		t.Fatalf("historical cumulative cost import month = %#v", august)
	}
}

func TestBusinessGoalProgressIncludesExistingProfitAcrossTeamPlusAndRefunds(t *testing.T) {
	service := openGoalTestService(t)
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
	goalID, err := service.CreateBusinessGoal(BusinessGoalInput{
		Name:             "August cumulative profit",
		TargetProfitYuan: "1000.00",
	})
	if err != nil {
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
	if center.ActiveGoal.Goal.BaselineProfitCents != 0 {
		t.Fatalf("goal baseline = %d, want 0", center.ActiveGoal.Goal.BaselineProfitCents)
	}
	if err := service.CompleteBusinessGoal(goalID); err != nil {
		t.Fatal(err)
	}
	goals, err := service.Store.ListBusinessGoals(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(goals) != 1 || goals[0].ResultProfitCents != 16000 {
		t.Fatalf("completed goals = %#v, want cumulative result 16000", goals)
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
		BoardedAt:           "2026-05-17",
	})
	if err != nil {
		t.Fatal(err)
	}
	seedPaidPricingPeriods(t, service, subscriptionID, 10000)
	if _, err := service.ScheduleBulkNextPrice(BulkNextPriceInput{
		SubscriptionIDs: []int64{subscriptionID},
		NextPriceYuan:   "108.00",
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
	// The current run rate must keep the current price until the scheduled
	// effective due date: 10000*365/360 - 3000 = 7138.
	if center.Forecast.RunRateMonthlyProfitCents != 7138 {
		t.Fatalf("run rate = %d, want 7138", center.Forecast.RunRateMonthlyProfitCents)
	}
	if center.Forecast.ActiveRecurringCount != 1 || center.Forecast.Source != "cash_flow" {
		t.Fatalf("forecast basis = %#v", center.Forecast)
	}
	if center.Forecast.Optimistic.ProjectedDate == "" {
		t.Fatalf("optimistic forecast = %#v", center.Forecast.Optimistic)
	}
}

func TestCashflowForecastAppliesScheduledPriceOnlyFromEffectiveDueDate(t *testing.T) {
	today := time.Date(2026, time.August, 15, 0, 0, 0, 0, cycle.Location)
	nextPrice := int64(12000)
	subscription := model.Subscription{
		PricePerPersonCents:       10000,
		NextPriceCents:            &nextPrice,
		NextPriceEffectiveDueDate: "2026-10-14",
		CronExpr:                  "interval:30d",
		BoardedAt:                 "2026-08-15",
	}
	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		t.Fatal(err)
	}
	dueDates := schedule.NextDueTimes(today.Add(-time.Minute), 3)
	got := make([]int64, 0, len(dueDates))
	for _, dueAt := range dueDates {
		got = append(got, billAmountCentsForDueDate(subscription, cycle.FormatDate(dueAt)))
	}
	if len(got) != 3 || got[0] != 10000 || got[1] != 10000 || got[2] != 12000 {
		t.Fatalf("dated prices = %#v, want [10000 10000 12000]", got)
	}
}

func TestForecastRetentionUsesPlanningAssumptionsUntilSampleGate(t *testing.T) {
	testCases := []struct {
		name       string
		successes  int
		churns     int
		wantLow    int
		wantBase   int
		wantHigh   int
	}{
		{
			name:      "no outcomes",
			wantLow:   80,
			wantBase:  90,
			wantHigh:  98,
		},
		{
			name:      "early mixed outcomes",
			successes: 3,
			churns:    2,
			wantLow:   80,
			wantBase:  90,
			wantHigh:  98,
		},
		{
			name:      "sample gate reached",
			successes: 18,
			churns:    2,
			wantLow:   75,
			wantBase:  88,
			wantHigh:  100,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			low, baseline, high := forecastRetentionPercents(PredictionReadiness{
				RenewalOutcomeCount: testCase.successes + testCase.churns,
				RenewalSuccessCount: testCase.successes,
				ChurnOutcomeCount:   testCase.churns,
			})
			if low != testCase.wantLow || baseline != testCase.wantBase || high != testCase.wantHigh {
				t.Fatalf(
					"retention = (%d, %d, %d), want (%d, %d, %d)",
					low,
					baseline,
					high,
					testCase.wantLow,
					testCase.wantBase,
					testCase.wantHigh,
				)
			}
		})
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
	for index, price := range []int64{9000, 9200} {
		subscriptionID, createErr := service.Store.CreateSubscription(model.Subscription{
			Name:                fmt.Sprintf("Team customer %d", index+1),
			BusinessType:        model.SubscriptionBusinessTeam,
			PricePerPersonCents: price,
			CronExpr:            "interval:30d",
			CustomerWechat:      fmt.Sprintf("wechat-%d", index+1),
			SeatID:              seats[index].ID,
			BoardedAt:           "2026-05-17",
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		seedPaidPricingPeriods(t, service, subscriptionID, price)
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
	if center.Candidates[0].SuggestedPriceCents != 9720 || center.Candidates[0].MaxIncreasePriceCents != 9720 {
		t.Fatalf("first gradual suggestion = %#v", center.Candidates[0])
	}
	if center.Candidates[1].MarketPosition != "below_low" || !center.Candidates[1].Recommended {
		t.Fatalf("second candidate = %#v", center.Candidates[1])
	}
	if center.Candidates[0].CustomerTier != "optimize" || center.Candidates[1].CustomerTier != "core" ||
		center.Candidates[0].MonthlyRevenueCents <= 0 || center.Candidates[1].MonthlyRevenueCents <= 0 {
		t.Fatalf("goal-center customer tiers = %#v", center.Candidates)
	}
	if center.Repricing.RecommendedCount != 2 || center.Repricing.EligibleCount != 2 ||
		center.Repricing.BelowMarketCount != 2 || center.Repricing.EstimatedMonthlyUpliftCents != 1476 ||
		len(center.Repricing.Windows) != 5 || center.Repricing.Windows[0].Key != "ready" ||
		center.Repricing.Windows[0].Count != 2 || center.Repricing.RiskSegments[1].Count != 2 ||
		center.Repricing.RelationshipSegments[2].Count != 2 {
		t.Fatalf("repricing analysis = %#v", center.Repricing)
	}
	if _, err := service.ScheduleBulkNextPrice(BulkNextPriceInput{
		SubscriptionIDs: teamIDs,
		NextPriceYuan:   "110.00",
	}); err == nil {
		t.Fatal("unsafe bulk increase unexpectedly succeeded")
	}

	updated, err := service.ScheduleBulkNextPrice(BulkNextPriceInput{
		SubscriptionIDs: teamIDs,
		NextPriceYuan:   "97.00",
	})
	if err != nil || updated != 2 {
		t.Fatalf("bulk result = %d, err = %v", updated, err)
	}
	for index, subscriptionID := range teamIDs {
		subscription, getErr := service.Get(subscriptionID)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if subscription.PricePerPersonCents != []int64{9000, 9200}[index] || subscription.NextPriceCents == nil || *subscription.NextPriceCents != 9700 {
			t.Fatalf("scheduled subscription = %#v", subscription)
		}
		if subscription.NextPriceEffectiveDueDate != "2026-09-14" {
			t.Fatalf("effective due date = %q, want 2026-09-14", subscription.NextPriceEffectiveDueDate)
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

func TestQuarterlyPricingUsesMonthlyComparablePriceWithoutChangingCycleAmounts(t *testing.T) {
	service := openGoalTestService(t)
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "quarterly-owner@example.com",
		CostYuan:  "50.00",
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
		Name:                "Quarterly Team customer",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 33000,
		CronExpr:            "interval:90d",
		CustomerEmail:       "quarterly-customer@example.com",
		SeatID:              seats[0].ID,
		BoardedAt:           "2025-11-18",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, dueDate := range []string{"2026-02-16", "2026-05-17", "2026-08-15"} {
		if err := service.Store.SetDuePaid(subscriptionID, dueDate, true, 33000); err != nil {
			t.Fatal(err)
		}
	}

	candidates, err := service.buildPricingCandidates(&model.MarketPriceSnapshot{
		LowPriceCents:    10000,
		MedianPriceCents: 12000,
		HighPriceCents:   15000,
		SampleCount:      10,
	}, 0)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("quarterly candidates = %#v, err = %v", candidates, err)
	}
	candidate := candidates[0]
	if candidate.CurrentPriceCents != 33000 || candidate.MarketMonthlyPriceCents != 11000 ||
		candidate.MarketPosition != "below_median" || candidate.GapToMarketMedianCents != 1000 {
		t.Fatalf("quarterly market comparison = %#v", candidate)
	}
	if candidate.SuggestedPriceCents != 34200 || candidate.SuggestedMonthlyPriceCents != 11400 ||
		candidate.MaxIncreasePriceCents != 35640 {
		t.Fatalf("quarterly cycle suggestion = %#v", candidate)
	}
	if candidate.VerifiedPriceCents != 33000 || candidate.VerifiedMonthlyPriceCents != 11000 ||
		candidate.VerifiedPriceIndex == nil || *candidate.VerifiedPriceIndex != 92 {
		t.Fatalf("quarterly paid-price evidence = %#v", candidate)
	}
	recommendation, err := service.buildPricingRecommendation(&model.MarketPriceSnapshot{
		LowPriceCents:    10000,
		MedianPriceCents: 12000,
		HighPriceCents:   15000,
		SampleCount:      10,
	})
	if err != nil || recommendation.InternalMedianPriceCents != 11000 {
		t.Fatalf("quarterly internal median = %#v, err = %v", recommendation, err)
	}

	analysis := buildRepricingAnalysis(candidates, service.now())
	if len(analysis.CustomerTiers) != 3 || analysis.CustomerTiers[1].AveragePriceCents != 11000 ||
		analysis.CustomerTiers[1].LowestPriceCents != 11000 || analysis.CustomerTiers[1].HighestPriceCents != 11000 {
		t.Fatalf("quarterly tier comparison = %#v", analysis.CustomerTiers)
	}
}

func TestMarketMonthlyCycleFactorUsesStandardSubscriptionMonths(t *testing.T) {
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, cycle.Location)
	tests := []struct {
		name       string
		expression string
		cyclePrice int64
	}{
		{name: "monthly interval", expression: "interval:30d", cyclePrice: 12000},
		{name: "quarterly interval", expression: "interval:90d", cyclePrice: 36000},
		{name: "half-year interval", expression: "interval:180d", cyclePrice: 72000},
		{name: "annual interval", expression: "interval:365d", cyclePrice: 144000},
		{name: "quarterly cron", expression: "0 0 1 */3 *", cyclePrice: 36000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			subscription := model.Subscription{CronExpr: test.expression, BoardedAt: "2026-01-01"}
			numerator, denominator := marketMonthlyCycleFactor(subscription, now)
			if monthly := scalePriceCents(test.cyclePrice, numerator, denominator); monthly != 12000 {
				t.Fatalf("factor %d/%d converted %d to %d, want 12000", numerator, denominator, test.cyclePrice, monthly)
			}
		})
	}
}

func TestBulkNextPriceStoreRejectsStaleFinancialState(t *testing.T) {
	service := openGoalTestService(t)
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "stale-price-owner@example.com",
		CostYuan:  "50.00",
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
		Name:                "并发调价用户",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 9000,
		CronExpr:            "interval:30d",
		SeatID:              seats[0].ID,
		BoardedAt:           "2026-05-17",
	})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := service.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	current := stale
	current.PricePerPersonCents = 9500
	if err := service.Store.UpdateSubscription(current); err != nil {
		t.Fatal(err)
	}

	nextPriceCents := int64(9700)
	stale.NextPriceCents = &nextPriceCents
	stale.NextPriceEffectiveDueDate = "2026-09-14"
	if err := service.Store.UpdateSubscriptionNextPrices([]model.Subscription{stale}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("stale bulk update error = %v, want sql.ErrNoRows", err)
	}
	stored, err := service.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PricePerPersonCents != 9500 || stored.NextPriceCents != nil {
		t.Fatalf("stale update changed financial state: %#v", stored)
	}
}

func TestPricingCandidatesProtectNewCustomers(t *testing.T) {
	service := openGoalTestService(t)
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "new-customer-owner@example.com",
		CostYuan:  "50.00",
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
		Name:                "未满月新用户",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 9000,
		CronExpr:            "interval:30d",
		SeatID:              seats[0].ID,
		BoardedAt:           "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Store.SetDuePaid(subscriptionID, "2026-08-01", true, 9000); err != nil {
		t.Fatal(err)
	}
	candidates, err := service.buildPricingCandidates(&model.MarketPriceSnapshot{
		LowPriceCents: 10000, MedianPriceCents: 13000, HighPriceCents: 15000, SampleCount: 10,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Eligible || candidates[0].Recommended ||
		candidates[0].BlockedCode != "protection" || candidates[0].NextReviewDate != "2026-09-30" ||
		candidates[0].RelationshipStage != "new" || candidates[0].AdjustmentRisk != "high" ||
		candidates[0].ReadinessScore > 39 || !strings.Contains(candidates[0].BlockedReason, "新用户保护期") {
		t.Fatalf("new-customer candidate = %#v", candidates)
	}
	if _, err := service.ScheduleBulkNextPrice(BulkNextPriceInput{
		SubscriptionIDs: []int64{subscriptionID},
		NextPriceYuan:   "95.00",
	}); err == nil || !strings.Contains(err.Error(), "新用户保护期") {
		t.Fatalf("protected repricing error = %v", err)
	}
}

func TestPricingCandidatesRespectSixMonthCooldown(t *testing.T) {
	service := openGoalTestService(t)
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "cooldown-owner@example.com",
		CostYuan:  "50.00",
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
		Name:                "刚调过价的老用户",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 9700,
		CronExpr:            "interval:30d",
		SeatID:              seats[0].ID,
		BoardedAt:           "2026-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, bill := range []struct {
		dueDate string
		amount  int64
	}{
		{dueDate: "2026-05-17", amount: 9000},
		{dueDate: "2026-06-16", amount: 9000},
		{dueDate: "2026-07-16", amount: 9700},
	} {
		if err := service.Store.SetDuePaid(subscriptionID, bill.dueDate, true, bill.amount); err != nil {
			t.Fatal(err)
		}
	}
	candidates, err := service.buildPricingCandidates(&model.MarketPriceSnapshot{
		LowPriceCents: 10000, MedianPriceCents: 13000, HighPriceCents: 15000, SampleCount: 10,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Eligible || candidates[0].LastPriceIncreaseDate != "2026-07-16" ||
		candidates[0].BlockedCode != "cooldown" || candidates[0].NextReviewDate != "2027-01-12" ||
		!strings.Contains(candidates[0].BlockedReason, "6 个月") {
		t.Fatalf("cooldown candidate = %#v", candidates)
	}
}

func seedPaidPricingPeriods(t *testing.T, service *SubscriptionService, subscriptionID int64, amountCents int64) {
	t.Helper()
	for _, dueDate := range []string{"2026-05-17", "2026-06-16", "2026-07-16"} {
		if err := service.Store.SetDuePaid(subscriptionID, dueDate, true, amountCents); err != nil {
			t.Fatal(err)
		}
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
		{SourceID: "seller-b", SourceTitle: "ChatGPT Team 首次激活码，可无限续费", Price: 130, Currency: "CNY", Status: "low_stock", EffectiveStatus: "available", StockCount: &stock, SourceUpdatedAt: &newest},
		{SourceID: "seller-c", SourceTitle: "ChatGPT Business Slot 30 days", Price: 150, Currency: "CNY", Status: "in_stock", EffectiveStatus: "available", StockCount: &stock, SourceUpdatedAt: &newest},
		{SourceID: "renew-a", SourceTitle: "ChatGPT Business 续费码，初次请购买激活码", Price: 135, Currency: "CNY", Status: "in_stock", EffectiveStatus: "available", StockCount: &stock, SourceUpdatedAt: &older},
		{SourceID: "renew-b", SourceTitle: "ChatGPT Team renewal", Price: 145, Currency: "CNY", Status: "in_stock", EffectiveStatus: "available", StockCount: &stock, SourceUpdatedAt: &newest},
		{SourceID: "renew-c", SourceTitle: "ChatGPT Team renew slot", Price: 155, Currency: "CNY", Status: "in_stock", EffectiveStatus: "available", StockCount: &stock, SourceUpdatedAt: &newest},
		{SourceID: "seller-d", SourceTitle: "ChatGPT Business Premium 高级席位", Price: 200, Currency: "CNY", Status: "in_stock", EffectiveStatus: "available", StockCount: &stock},
		{SourceID: "seller-e", SourceTitle: "ChatGPT Team 2 席位母号", Price: 240, Currency: "CNY", Status: "in_stock", EffectiveStatus: "available", StockCount: &stock},
		{SourceID: "seller-f", SourceTitle: "ChatGPT Business 席位", Price: 100, Currency: "CNY", Status: "out_of_stock", EffectiveStatus: "unavailable", StockCount: &zeroStock},
	}

	prices, updatedAt := comparableMarketPrices(offers, marketOfferAcquisition)
	if len(prices) != 3 || prices[0] != 11300 || prices[1] != 13000 || prices[2] != 15000 {
		t.Fatalf("comparable prices = %#v", prices)
	}
	if !updatedAt.Equal(newest) {
		t.Fatalf("source updated at = %s, want %s", updatedAt, newest)
	}
	if low, median, high := percentileCents(prices, 0.25), percentileCents(prices, 0.5), percentileCents(prices, 0.75); low != 12150 || median != 13000 || high != 14000 {
		t.Fatalf("quartiles = %d/%d/%d, want 12150/13000/14000", low, median, high)
	}
	renewalPrices, renewalUpdatedAt := comparableMarketPrices(offers, marketOfferRenewal)
	if len(renewalPrices) != 3 || renewalPrices[0] != 13500 || renewalPrices[1] != 14500 || renewalPrices[2] != 15500 {
		t.Fatalf("renewal prices = %#v", renewalPrices)
	}
	if !renewalUpdatedAt.Equal(newest) {
		t.Fatalf("renewal source updated at = %s, want %s", renewalUpdatedAt, newest)
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
	if lowAdvice.Action != "fill" || lowAdvice.UtilizationPercent != 40 ||
		lowAdvice.SuggestedLowPriceCents != 12000 || lowAdvice.SuggestedHighPriceCents != 13000 ||
		lowAdvice.NewSaleDiscountPercent != 7 {
		t.Fatalf("low-utilization advice = %#v", lowAdvice)
	}

	highUtilization := openGoalTestService(t)
	seedPricingSeats(t, highUtilization, 10, 9, 10000)
	highAdvice, err := highUtilization.buildPricingRecommendation(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if highAdvice.Action != "raise" || highAdvice.UtilizationPercent != 90 ||
		highAdvice.SuggestedLowPriceCents != 12600 || highAdvice.SuggestedHighPriceCents != 13500 ||
		highAdvice.NewSaleDiscountPercent != 3 {
		t.Fatalf("high-utilization advice = %#v", highAdvice)
	}
}

func TestPricingRecommendationDoesNotCollapseCurrentMarketRange(t *testing.T) {
	service := openGoalTestService(t)
	seedPricingSeats(t, service, 10, 9, 9000)
	advice, err := service.buildPricingRecommendation(&model.MarketPriceSnapshot{
		LowPriceCents:    13133,
		MedianPriceCents: 13390,
		HighPriceCents:   14163,
		SampleCount:      7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if advice.Action != "raise" || advice.SuggestedLowPriceCents != 12600 ||
		advice.SuggestedHighPriceCents != 12900 || advice.NewSaleDiscountPercent != 3 {
		t.Fatalf("current-market new-sale range = %#v", advice)
	}
}

func TestAttractiveNewSaleRangePreservesMarginFloor(t *testing.T) {
	for utilization, want := range map[int]int64{0: 7, 69: 7, 70: 5, 84: 5, 85: 3, 100: 3} {
		if got := newSaleDiscountPercent(utilization); got != want {
			t.Fatalf("discount at %d%% utilization = %d, want %d", utilization, got, want)
		}
	}

	low, high, discountApplied := attractiveNewSaleRange(13000, 14000, 13850, 7)
	if low != 13900 || high != 13900 || discountApplied {
		t.Fatalf("margin-constrained range = %d/%d, discountApplied = %t", low, high, discountApplied)
	}

	low, high, discountApplied = attractiveNewSaleRange(13100, 13200, 10000, 3)
	if low != 12500 || high != 12800 || !discountApplied {
		t.Fatalf("minimum-width attractive range = %d/%d, discountApplied = %t", low, high, discountApplied)
	}
}

func TestRepricingAnalysisDoesNotCountBelowMedianProtectedUserAsFutureAction(t *testing.T) {
	analysis := buildRepricingAnalysis([]PricingCandidate{{
		MarketPosition:         "below_median",
		BlockedCode:            "protection",
		NextReviewDate:         "2026-09-01",
		SuggestedMonthlyUplift: 500,
		RelationshipStage:      "developing",
		AdjustmentRisk:         "high",
	}}, time.Date(2026, time.August, 15, 12, 0, 0, 0, cycle.Location))
	if analysis.BelowMarketCount != 1 || analysis.PipelineMonthlyUpliftCents != 0 ||
		analysis.Windows[1].Count != 0 || analysis.Windows[2].Count != 0 {
		t.Fatalf("below-median protected analysis = %#v", analysis)
	}
}

func TestPricingEvidenceSeparatesPaidPriceFromRenewalBehavior(t *testing.T) {
	highFirstOrder := PricingCandidate{
		CurrentPriceCents:      16000,
		VerifiedPriceCents:     16000,
		PaidPeriodCount:        1,
		RelationshipDays:       15,
		PriceStableDays:        15,
		CustomerGroupSize:      1,
		MarketPosition:         "above_high",
		GapToMarketMedianCents: -3000,
		BlockedCode:            "protection",
		SuggestedPriceCents:    16000,
	}
	lowRepeated := PricingCandidate{
		CurrentPriceCents:      8500,
		VerifiedPriceCents:     8500,
		PaidPeriodCount:        4,
		RelationshipDays:       180,
		PriceStableDays:        180,
		CustomerGroupSize:      1,
		MarketPosition:         "below_low",
		GapToMarketMedianCents: 4890,
		SuggestedPriceCents:    9180,
		BlockedCode:            "eligible",
		Eligible:               true,
	}

	populateRepricingInsights(&highFirstOrder)
	populateRepricingInsights(&lowRepeated)

	if highFirstOrder.RenewalEvidence != "initial" || highFirstOrder.RenewalCount != 0 ||
		highFirstOrder.VerifiedPriceIndex == nil || *highFirstOrder.VerifiedPriceIndex != 123 {
		t.Fatalf("high first-order evidence = %#v", highFirstOrder)
	}
	if lowRepeated.RenewalEvidence != "renewed" || lowRepeated.RenewalCount != 3 ||
		lowRepeated.VerifiedPriceIndex == nil || *lowRepeated.VerifiedPriceIndex != 63 {
		t.Fatalf("low repeated evidence = %#v", lowRepeated)
	}
	if strings.Contains(strings.Join(highFirstOrder.AnalysisCodes, ","), "proven_price_acceptance") ||
		!strings.Contains(strings.Join(highFirstOrder.AnalysisCodes, ","), "first_cycle_only") ||
		!strings.Contains(strings.Join(lowRepeated.AnalysisCodes, ","), "low_price_entry") {
		t.Fatalf("pricing evidence signals high=%#v low=%#v", highFirstOrder.AnalysisCodes, lowRepeated.AnalysisCodes)
	}
}

func TestRelationshipProfileUsesBehaviorAndTreatsMissingWechatAsRiskNotVerdict(t *testing.T) {
	candidates := []PricingCandidate{
		{
			SubscriptionID:         1,
			CustomerEmail:          "new@example.com",
			CustomerWechat:         "new-contact",
			CustomerTier:           "core",
			CustomerGroupSize:      1,
			PaidPeriodCount:        1,
			RelationshipDays:       20,
			BlockedCode:            "protection",
			MarketPosition:         "market_range",
			CurrentPriceCents:      12000,
			VerifiedPriceCents:     12000,
			GapToMarketMedianCents: 0,
		},
		{
			SubscriptionID:     2,
			CustomerEmail:      "loyal@example.com",
			CustomerTier:       "mainstay",
			CustomerGroupSize:  1,
			PaidPeriodCount:    6,
			RelationshipDays:   240,
			BlockedCode:        "eligible",
			MarketPosition:     "market_range",
			CurrentPriceCents:  9000,
			VerifiedPriceCents: 9000,
		},
		{
			SubscriptionID:     3,
			CustomerEmail:      "service@example.com",
			CustomerTier:       "mainstay",
			CustomerGroupSize:  1,
			PaidPeriodCount:    3,
			RelationshipDays:   100,
			BlockedCode:        "after_sales",
			MarketPosition:     "market_range",
			CurrentPriceCents:  9000,
			VerifiedPriceCents: 9000,
		},
		{
			SubscriptionID:     4,
			CustomerEmail:      "healthy@example.com",
			CustomerTier:       "mainstay",
			CustomerGroupSize:  1,
			PaidPeriodCount:    3,
			RelationshipDays:   100,
			BlockedCode:        "eligible",
			MarketPosition:     "market_range",
			CurrentPriceCents:  9000,
			VerifiedPriceCents: 9000,
		},
	}
	for index := range candidates {
		populateRepricingInsights(&candidates[index])
	}
	populateCustomerRelationshipProfiles(candidates)

	newCustomer := candidates[0]
	loyalWithoutWechat := candidates[1]
	serviceRecovery := candidates[2]
	healthyPeer := candidates[3]
	if newCustomer.RelationshipProfileConfidence != "low" ||
		newCustomer.PrimaryRelationshipTask != "observe_first_renewal" ||
		newCustomer.NeedsContactFollowup {
		t.Fatalf("new connected customer profile = %#v", newCustomer)
	}
	if !loyalWithoutWechat.NeedsContactFollowup ||
		loyalWithoutWechat.PrimaryRelationshipTask != "complete_contact" ||
		loyalWithoutWechat.ContactStrengthScore >= newCustomer.ContactStrengthScore ||
		loyalWithoutWechat.LoyaltyScore <= newCustomer.LoyaltyScore ||
		loyalWithoutWechat.RelationshipLevel != "stable" ||
		loyalWithoutWechat.RelationshipProfileConfidence != "high" {
		t.Fatalf("loyal customer without WeChat profile = %#v", loyalWithoutWechat)
	}
	if serviceRecovery.PrimaryRelationshipTask != "repair_trust" ||
		!strings.Contains(strings.Join(serviceRecovery.RelationshipSignalCodes, ","), "service_history") ||
		serviceRecovery.CustomerQualityScore != healthyPeer.CustomerQualityScore ||
		serviceRecovery.LoyaltyScore != healthyPeer.LoyaltyScore ||
		serviceRecovery.RelationshipHealthScore >= healthyPeer.RelationshipHealthScore {
		t.Fatalf("service recovery profile = %#v", serviceRecovery)
	}
}

func TestPaidIncreaseIsCountedAsObservedAcceptance(t *testing.T) {
	histories := summarizePricingBillHistories([]model.Bill{
		{ID: 1, SubscriptionID: 7, DueDate: "2026-05-01", AmountCents: 9000},
		{ID: 2, SubscriptionID: 7, DueDate: "2026-06-01", AmountCents: 9500},
	}, nil)
	history := histories[7]
	if history.PaidPeriodCount != 2 || history.LastPaidPriceCents != 9500 ||
		history.LastIncreaseDate != "2026-06-01" || history.PaidPeriodsAfterIncrease != 1 {
		t.Fatalf("paid increase evidence = %#v", history)
	}

	candidate := PricingCandidate{
		CurrentPriceCents:        9500,
		VerifiedPriceCents:       history.LastPaidPriceCents,
		PaidPeriodCount:          history.PaidPeriodCount,
		PaidPeriodsAfterIncrease: history.PaidPeriodsAfterIncrease,
		MarketPosition:           "below_median",
		GapToMarketMedianCents:   2500,
	}
	populatePricingEvidence(&candidate)
	if candidate.RenewalEvidence != "increase_accepted" || candidate.RenewalCount != 1 ||
		candidate.VerifiedPriceIndex == nil || *candidate.VerifiedPriceIndex != 79 {
		t.Fatalf("increase acceptance candidate = %#v", candidate)
	}
}

func TestFirstCyclePriceEvidenceIsMonotonicAndPressureDoesNotSaturate(t *testing.T) {
	tests := []struct {
		priceCents        int64
		wantPaidIndex     int
		wantPricePressure int
	}{
		{priceCents: 8500, wantPaidIndex: 63, wantPricePressure: 74},
		{priceCents: 9000, wantPaidIndex: 67, wantPricePressure: 66},
		{priceCents: 9500, wantPaidIndex: 71, wantPricePressure: 58},
	}
	const marketMedianCents = int64(13390)
	for _, test := range tests {
		candidate := PricingCandidate{
			CurrentPriceCents:      test.priceCents,
			VerifiedPriceCents:     test.priceCents,
			PaidPeriodCount:        1,
			RelationshipDays:       20,
			PriceStableDays:        20,
			CustomerGroupSize:      1,
			MarketPosition:         "below_low",
			GapToMarketMedianCents: marketMedianCents - test.priceCents,
			BlockedCode:            "protection",
		}
		populateRepricingInsights(&candidate)
		if candidate.RenewalEvidence != "initial" || candidate.RenewalCount != 0 ||
			candidate.VerifiedPriceIndex == nil || *candidate.VerifiedPriceIndex != test.wantPaidIndex ||
			candidate.PricePressureScore != test.wantPricePressure {
			t.Fatalf("first-cycle price %d evidence = %#v", test.priceCents, candidate)
		}
	}
}

func TestAssignCustomerTiersUsesInterpolatedInternalPriceBands(t *testing.T) {
	nextPriceCents := int64(9500)
	candidates := []PricingCandidate{
		{CurrentPriceCents: 8000, MonthlyRevenueCents: 8000, Recommended: true},
		{CurrentPriceCents: 9000, MonthlyRevenueCents: 9000, NextPriceCents: &nextPriceCents},
		{CurrentPriceCents: 10000, MonthlyRevenueCents: 10000},
		{CurrentPriceCents: 11000, MonthlyRevenueCents: 11000},
	}

	assignCustomerTiers(candidates)
	wantTiers := []string{"optimize", "mainstay", "core", "core"}
	for index, want := range wantTiers {
		if candidates[index].CustomerTier != want {
			t.Fatalf("candidate %d tier = %q, want %q", index, candidates[index].CustomerTier, want)
		}
	}

	analysis := buildRepricingAnalysis(candidates, time.Date(2026, time.August, 17, 12, 0, 0, 0, cycle.Location))
	if len(analysis.CustomerTiers) != 3 {
		t.Fatalf("customer tiers = %#v", analysis.CustomerTiers)
	}
	wants := []struct {
		key              string
		monthlyRevenue   int64
		revenueShare     int
		averagePrice     int64
		recommendedCount int
		scheduledCount   int
	}{
		{key: "core", monthlyRevenue: 21000, revenueShare: 55, averagePrice: 10500},
		{key: "mainstay", monthlyRevenue: 9000, revenueShare: 24, averagePrice: 9000, scheduledCount: 1},
		{key: "optimize", monthlyRevenue: 8000, revenueShare: 21, averagePrice: 8000, recommendedCount: 1},
	}
	for index, want := range wants {
		got := analysis.CustomerTiers[index]
		wantCount := 1
		if want.key == "core" {
			wantCount = 2
		}
		if got.Key != want.key || got.Count != wantCount || got.MonthlyRevenueCents != want.monthlyRevenue ||
			got.RevenueSharePercent != want.revenueShare || got.AveragePriceCents != want.averagePrice ||
			got.RecommendedCount != want.recommendedCount || got.ScheduledCount != want.scheduledCount {
			t.Fatalf("customer tier %d = %#v, want %#v", index, got, want)
		}
		if want.key == "core" && (got.LowestPriceCents != 10000 || got.HighestPriceCents != 11000) {
			t.Fatalf("core customer price range = %#v", got)
		}
		if want.key != "core" && (got.LowestPriceCents != want.averagePrice || got.HighestPriceCents != want.averagePrice) {
			t.Fatalf("customer tier price range = %#v, want %#v", got, want)
		}
	}
}

func TestAssignCustomerTiersKeepsUniformPricesMainstay(t *testing.T) {
	candidates := []PricingCandidate{
		{CurrentPriceCents: 10000},
		{CurrentPriceCents: 10000},
		{CurrentPriceCents: 10000},
	}
	assignCustomerTiers(candidates)
	for index, candidate := range candidates {
		if candidate.CustomerTier != "mainstay" {
			t.Fatalf("candidate %d tier = %q, want mainstay", index, candidate.CustomerTier)
		}
	}
}

func TestLowPriceSingleSeatCustomerGetsEarlierRepricingReview(t *testing.T) {
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, cycle.Location)
	candidates := []PricingCandidate{
		{
			SubscriptionID:      1,
			CustomerEmail:       "low@example.com",
			CurrentPriceCents:   8000,
			MonthlyRevenueCents: 8000,
			PaidPeriodCount:     2,
			RelationshipDays:    30,
			BlockedCode:         "protection",
			BlockedReason:       "standard protection",
			MarketPosition:      "below_low",
			SuggestedPriceCents: 8600,
		},
		{SubscriptionID: 2, CustomerEmail: "mid@example.com", CurrentPriceCents: 10000, MonthlyRevenueCents: 10000, Eligible: true, BlockedCode: "eligible"},
		{SubscriptionID: 3, CustomerEmail: "high@example.com", CurrentPriceCents: 12000, MonthlyRevenueCents: 12000, Eligible: true, BlockedCode: "eligible"},
	}
	finalizePricingCandidates(candidates, map[int64]model.Subscription{
		1: {ID: 1, BoardedAt: "2026-07-18", CronExpr: "interval:30d"},
	}, now)

	low := candidates[0]
	if low.CustomerTier != "optimize" || !low.ExpeditedReview || !low.Eligible ||
		low.BlockedCode != "eligible" || !low.Recommended || low.NextReviewDate != "" ||
		!strings.Contains(strings.Join(low.AnalysisCodes, ","), "expedited_low_price_review") {
		t.Fatalf("expedited low-price candidate = %#v", low)
	}
}

func TestBulkNextPriceRechecksEarlierLowPriceEligibility(t *testing.T) {
	service := openGoalTestService(t)
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "expedited-owner@example.com",
		CostYuan:  "50.00",
		SeatCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := service.Store.ListSeatsByAccount(accountID)
	if err != nil || len(seats) != 3 {
		t.Fatalf("seats = %#v, err = %v", seats, err)
	}

	type seededCustomer struct {
		name       string
		email      string
		priceCents int64
		boardedAt  string
		paidDates  []string
	}
	seeded := []seededCustomer{
		{name: "low", email: "low@example.com", priceCents: 8000, boardedAt: "2026-07-01", paidDates: []string{"2026-07-01", "2026-07-31"}},
		{name: "mid", email: "mid@example.com", priceCents: 10000, boardedAt: "2026-05-01", paidDates: []string{"2026-05-01", "2026-05-31", "2026-06-30"}},
		{name: "high", email: "high@example.com", priceCents: 12000, boardedAt: "2026-05-01", paidDates: []string{"2026-05-01", "2026-05-31", "2026-06-30"}},
	}
	var lowSubscriptionID int64
	for index, customer := range seeded {
		subscriptionID, createErr := service.Store.CreateSubscription(model.Subscription{
			Name:                customer.name,
			BusinessType:        model.SubscriptionBusinessTeam,
			PricePerPersonCents: customer.priceCents,
			CronExpr:            "interval:30d",
			SeatID:              seats[index].ID,
			CustomerEmail:       customer.email,
			BoardedAt:           customer.boardedAt,
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		for _, dueDate := range customer.paidDates {
			if setErr := service.Store.SetDuePaid(subscriptionID, dueDate, true, customer.priceCents); setErr != nil {
				t.Fatal(setErr)
			}
		}
		if customer.name == "low" {
			lowSubscriptionID = subscriptionID
		}
	}

	candidates, err := service.buildPricingCandidates(&model.MarketPriceSnapshot{
		LowPriceCents: 10000, MedianPriceCents: 13000, HighPriceCents: 15000, SampleCount: 10,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var lowCandidate PricingCandidate
	for _, candidate := range candidates {
		if candidate.SubscriptionID == lowSubscriptionID {
			lowCandidate = candidate
			break
		}
	}
	if !lowCandidate.ExpeditedReview || !lowCandidate.Eligible || !lowCandidate.Recommended ||
		lowCandidate.CustomerTier != "optimize" {
		t.Fatalf("built expedited candidate = %#v", lowCandidate)
	}

	updated, err := service.ScheduleBulkNextPrice(BulkNextPriceInput{
		SubscriptionIDs: []int64{lowSubscriptionID},
		NextPriceYuan:   "86.00",
	})
	if err != nil || updated != 1 {
		t.Fatalf("schedule expedited candidate: updated=%d err=%v", updated, err)
	}
	stored, err := service.Store.GetSubscription(lowSubscriptionID)
	if err != nil || stored.NextPriceCents == nil || *stored.NextPriceCents != 8600 {
		t.Fatalf("scheduled low subscription = %#v, err = %v", stored, err)
	}
}

func TestManualNextPriceBypassesAlgorithmicTimingAndIncreaseCap(t *testing.T) {
	service := openGoalTestService(t)
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "manual-pricing-owner@example.com",
		CostYuan:  "50.00",
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
		Name:                "First-cycle customer",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 8000,
		CronExpr:            "interval:30d",
		SeatID:              seats[0].ID,
		CustomerEmail:       "manual-pricing-customer@example.com",
		BoardedAt:           "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Store.SetDuePaid(subscriptionID, "2026-08-01", true, 8000); err != nil {
		t.Fatal(err)
	}

	candidates, err := service.buildPricingCandidates(nil, 0)
	if err != nil || len(candidates) != 1 || candidates[0].BlockedCode != "protection" || candidates[0].Eligible {
		t.Fatalf("protected first-cycle candidate = %#v, err = %v", candidates, err)
	}
	if candidates[0].MaxIncreasePriceCents >= 10000 {
		t.Fatalf("test price does not exceed algorithmic cap: %#v", candidates[0])
	}

	updated, err := service.ScheduleManualNextPrices(ManualNextPricesInput{Items: []ManualNextPriceItemInput{{
		SubscriptionID: subscriptionID,
		NextPriceYuan:  "100.00",
	}}})
	if err != nil || updated != 1 {
		t.Fatalf("manual pricing result = %d, err = %v", updated, err)
	}
	stored, err := service.Store.GetSubscription(subscriptionID)
	if err != nil || stored.NextPriceCents == nil || *stored.NextPriceCents != 10000 ||
		stored.NextPriceEffectiveDueDate != "2026-09-30" {
		t.Fatalf("manual pricing state = %#v, err = %v", stored, err)
	}
}

func TestManualNextPriceKeepsOperatorExemption(t *testing.T) {
	service := openGoalTestService(t)
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "manual-exemption-owner@example.com",
		CostYuan:  "50.00",
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
		Name:                "Exempted customer",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 8000,
		CronExpr:            "interval:30d",
		SeatID:              seats[0].ID,
		CustomerEmail:       "manual-exemption-customer@example.com",
		BoardedAt:           "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Store.CreatePricingExemptions([]model.PricingExemption{{
		SubscriptionID:            subscriptionID,
		ReasonCode:                "manual",
		ReviewAfter:               "2026-09-15",
		ReviewCycles:              1,
		PriceCentsSnapshot:        8000,
		MarketMedianCentsSnapshot: 10000,
	}}, "2026-08-15"); err != nil {
		t.Fatal(err)
	}

	if _, err := service.ScheduleManualNextPrices(ManualNextPricesInput{Items: []ManualNextPriceItemInput{{
		SubscriptionID: subscriptionID,
		NextPriceYuan:  "90.00",
	}}}); err == nil || !strings.Contains(err.Error(), "豁免") {
		t.Fatalf("manual pricing exemption error = %v", err)
	}
	stored, err := service.Store.GetSubscription(subscriptionID)
	if err != nil || stored.NextPriceCents != nil {
		t.Fatalf("exempted price was changed = %#v, err = %v", stored, err)
	}
}

func TestExpeditedOptimizeCustomerDoesNotBypassActiveExemption(t *testing.T) {
	service := openGoalTestService(t)
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "optimize-exemption-owner@example.com",
		CostYuan:  "50.00",
		SeatCount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := service.Store.ListSeatsByAccount(accountID)
	if err != nil || len(seats) != 3 {
		t.Fatalf("seats = %#v, err = %v", seats, err)
	}

	prices := []int64{8000, 10000, 12000}
	var optimizeSubscriptionID int64
	for index, priceCents := range prices {
		subscriptionID, createErr := service.Store.CreateSubscription(model.Subscription{
			Name:                fmt.Sprintf("customer-%d", index+1),
			BusinessType:        model.SubscriptionBusinessTeam,
			PricePerPersonCents: priceCents,
			CronExpr:            "interval:30d",
			SeatID:              seats[index].ID,
			CustomerEmail:       fmt.Sprintf("customer-%d@example.com", index+1),
			BoardedAt:           "2026-07-01",
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		if index == 0 {
			optimizeSubscriptionID = subscriptionID
			for _, dueDate := range []string{"2026-07-01", "2026-07-31"} {
				if paidErr := service.Store.SetDuePaid(subscriptionID, dueDate, true, priceCents); paidErr != nil {
					t.Fatal(paidErr)
				}
			}
		}
	}

	snapshot := model.MarketPriceSnapshot{
		LowPriceCents: 10000, MedianPriceCents: 13000, HighPriceCents: 15000, SampleCount: 10,
	}
	before, err := service.buildPricingCandidates(&snapshot, 0)
	if err != nil {
		t.Fatal(err)
	}
	var optimizeCandidate PricingCandidate
	for _, candidate := range before {
		if candidate.SubscriptionID == optimizeSubscriptionID {
			optimizeCandidate = candidate
			break
		}
	}
	if optimizeCandidate.CustomerTier != "optimize" || !optimizeCandidate.ExpeditedReview ||
		!optimizeCandidate.Eligible || !optimizeCandidate.Recommended {
		t.Fatalf("optimize candidate before exemption = %#v", optimizeCandidate)
	}

	if err := service.Store.CreatePricingExemptions([]model.PricingExemption{{
		SubscriptionID:            optimizeSubscriptionID,
		ReasonCode:                "price_observation",
		ReviewAfter:               "2026-09-15",
		ReviewCycles:              1,
		PriceCentsSnapshot:        8000,
		MarketMedianCentsSnapshot: 13000,
	}}, "2026-08-15"); err != nil {
		t.Fatal(err)
	}

	after, err := service.buildPricingCandidates(&snapshot, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range after {
		if candidate.SubscriptionID != optimizeSubscriptionID {
			continue
		}
		if candidate.Eligible || candidate.Recommended || candidate.BlockedCode != "exempted" ||
			candidate.NextReviewDate != "2026-09-15" {
			t.Fatalf("active exemption was bypassed by optimize review = %#v", candidate)
		}
		return
	}
	t.Fatal("optimize candidate missing after exemption")
}

func TestLowPriceSingleSeatCustomerMustMeetBothEarlierThresholds(t *testing.T) {
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, cycle.Location)
	tests := []struct {
		name           string
		days           int
		paidPeriods    int
		wantReviewDate string
	}{
		{name: "relationship too short", days: 29, paidPeriods: 2, wantReviewDate: "2026-08-18"},
		{name: "one payment only", days: 30, paidPeriods: 1, wantReviewDate: "2026-08-18"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidates := []PricingCandidate{
				{
					SubscriptionID:      1,
					CustomerEmail:       "low@example.com",
					CurrentPriceCents:   8000,
					MonthlyRevenueCents: 8000,
					PaidPeriodCount:     test.paidPeriods,
					RelationshipDays:    test.days,
					BlockedCode:         "protection",
					BlockedReason:       "standard protection",
					MarketPosition:      "below_low",
					SuggestedPriceCents: 8600,
				},
				{SubscriptionID: 2, CustomerEmail: "mid@example.com", CurrentPriceCents: 10000, MonthlyRevenueCents: 10000},
				{SubscriptionID: 3, CustomerEmail: "high@example.com", CurrentPriceCents: 12000, MonthlyRevenueCents: 12000},
			}
			finalizePricingCandidates(candidates, map[int64]model.Subscription{
				1: {ID: 1, BoardedAt: "2026-07-19", CronExpr: "interval:30d"},
			}, now)

			low := candidates[0]
			if !low.ExpeditedReview || low.Eligible || low.Recommended || low.BlockedCode != "protection" ||
				low.NextReviewDate != test.wantReviewDate || !strings.Contains(low.BlockedReason, "30 天") ||
				!strings.Contains(low.BlockedReason, "2 个付费周期") {
				t.Fatalf("protected low-price candidate = %#v", low)
			}
		})
	}
}

func TestLowPriceCustomerInCooldownCannotUseEarlierReview(t *testing.T) {
	candidates := []PricingCandidate{
		{
			SubscriptionID:      1,
			CustomerEmail:       "low@example.com",
			CurrentPriceCents:   8000,
			MonthlyRevenueCents: 8000,
			PaidPeriodCount:     2,
			RelationshipDays:    30,
			BlockedCode:         "cooldown",
			BlockedReason:       "cooldown active",
			MarketPosition:      "below_low",
			SuggestedPriceCents: 8600,
		},
		{SubscriptionID: 2, CustomerEmail: "mid@example.com", CurrentPriceCents: 10000, MonthlyRevenueCents: 10000},
		{SubscriptionID: 3, CustomerEmail: "high@example.com", CurrentPriceCents: 12000, MonthlyRevenueCents: 12000},
	}
	finalizePricingCandidates(candidates, nil, time.Date(2026, time.August, 17, 12, 0, 0, 0, cycle.Location))

	low := candidates[0]
	if low.ExpeditedReview || low.Eligible || low.Recommended || low.BlockedCode != "cooldown" {
		t.Fatalf("cooldown candidate bypassed safeguard = %#v", low)
	}
}

func TestAssignCustomerTiersMergesMultiSeatCustomersTransitively(t *testing.T) {
	candidates := []PricingCandidate{
		{SubscriptionID: 1, CustomerEmail: " SHARED@example.com ", CustomerWechat: "wx-a", CurrentPriceCents: 4000, MonthlyRevenueCents: 4000},
		{SubscriptionID: 2, CustomerEmail: "shared@example.com", CustomerWechat: "wx-b", CurrentPriceCents: 4000, MonthlyRevenueCents: 4000},
		{SubscriptionID: 3, CustomerEmail: "other@example.com", CustomerWechat: "WX-B", CurrentPriceCents: 4000, MonthlyRevenueCents: 4000},
		{SubscriptionID: 4, CustomerWechat: "-", CurrentPriceCents: 10000, MonthlyRevenueCents: 10000},
		{SubscriptionID: 5, CustomerWechat: "-", CurrentPriceCents: 12000, MonthlyRevenueCents: 12000},
		{SubscriptionID: 6, CustomerEmail: "pair@example.com", CurrentPriceCents: 1000, MonthlyRevenueCents: 1000},
		{SubscriptionID: 7, CustomerEmail: "pair@example.com", CurrentPriceCents: 1000, MonthlyRevenueCents: 1000},
	}
	assignCustomerTiers(candidates)

	for index := 0; index < 3; index++ {
		if candidates[index].CustomerGroupID != candidates[0].CustomerGroupID ||
			candidates[index].CustomerGroupSize != 3 || candidates[index].CustomerTier != "core" ||
			candidates[index].CustomerGroupCurrentPriceCents != 12000 ||
			candidates[index].CustomerGroupMonthlyRevenueCents != 12000 {
			t.Fatalf("transitive customer member %d = %#v", index, candidates[index])
		}
	}
	if candidates[3].CustomerGroupID == candidates[4].CustomerGroupID ||
		candidates[3].CustomerGroupSize != 1 || candidates[4].CustomerGroupSize != 1 {
		t.Fatalf("blank identities were merged: %#v / %#v", candidates[3], candidates[4])
	}
	for _, index := range []int{5, 6} {
		if candidates[index].CustomerGroupSize != 2 || candidates[index].CustomerTier == "optimize" ||
			candidates[index].CustomerGroupID != candidates[5].CustomerGroupID {
			t.Fatalf("two-seat customer member %d = %#v", index, candidates[index])
		}
	}

	analysis := buildRepricingAnalysis(candidates, time.Date(2026, time.August, 17, 12, 0, 0, 0, cycle.Location))
	if analysis.TotalCount != 7 || analysis.CustomerCount != 4 {
		t.Fatalf("customer/seat counts = %d/%d, analysis = %#v", analysis.CustomerCount, analysis.TotalCount, analysis)
	}
	seatCount := 0
	customerCount := 0
	for _, tier := range analysis.CustomerTiers {
		seatCount += tier.Count
		customerCount += tier.CustomerCount
	}
	if seatCount != 7 || customerCount != 4 {
		t.Fatalf("tier customer/seat counts = %d/%d: %#v", customerCount, seatCount, analysis.CustomerTiers)
	}
}

func TestAssignCustomerTiersUsesCustomerFacingMonthlyEquivalent(t *testing.T) {
	candidates := []PricingCandidate{
		{
			SubscriptionID:          1,
			CustomerEmail:           "monthly@example.com",
			CurrentPriceCents:       10000,
			MarketMonthlyPriceCents: 10000,
			MonthlyRevenueCents:     10138,
		},
		{
			SubscriptionID:          2,
			CustomerEmail:           "quarterly@example.com",
			CurrentPriceCents:       33000,
			MarketMonthlyPriceCents: 11000,
			MonthlyRevenueCents:     11145,
		},
	}

	assignCustomerTiers(candidates)

	if candidates[0].CustomerGroupMonthlyRevenueCents != 10000 {
		t.Fatalf(
			"monthly customer-facing value = %d, want 10000",
			candidates[0].CustomerGroupMonthlyRevenueCents,
		)
	}
	if candidates[1].CustomerGroupMonthlyRevenueCents != 11000 {
		t.Fatalf(
			"quarterly monthly-equivalent value = %d, want 11000",
			candidates[1].CustomerGroupMonthlyRevenueCents,
		)
	}
}

func TestMultiSeatCustomerNeverUsesEarlierLowPriceReview(t *testing.T) {
	candidates := []PricingCandidate{
		{SubscriptionID: 1, CustomerWechat: "multi", CurrentPriceCents: 5000, MonthlyRevenueCents: 5000, PaidPeriodCount: 2, RelationshipDays: 30, BlockedCode: "protection", MarketPosition: "below_low", SuggestedPriceCents: 5400},
		{SubscriptionID: 2, CustomerWechat: "multi", CurrentPriceCents: 5000, MonthlyRevenueCents: 5000, PaidPeriodCount: 2, RelationshipDays: 30, BlockedCode: "protection", MarketPosition: "below_low", SuggestedPriceCents: 5400},
		{SubscriptionID: 3, CustomerWechat: "single", CurrentPriceCents: 12000, MonthlyRevenueCents: 12000, Eligible: true, BlockedCode: "eligible"},
	}
	finalizePricingCandidates(candidates, nil, time.Date(2026, time.August, 17, 12, 0, 0, 0, cycle.Location))

	for index := 0; index < 2; index++ {
		candidate := candidates[index]
		if candidate.CustomerGroupSize != 2 || candidate.ExpeditedReview || candidate.Eligible ||
			candidate.Recommended || candidate.BlockedCode != "protection" ||
			!strings.Contains(strings.Join(candidate.AnalysisCodes, ","), "multi_seat_customer") {
			t.Fatalf("multi-seat candidate %d bypassed protection = %#v", index, candidate)
		}
	}
}

func TestCustomerTierRevenueSharesAlwaysTotalOneHundred(t *testing.T) {
	analysis := buildRepricingAnalysis([]PricingCandidate{
		{CustomerTier: "core", CurrentPriceCents: 1, MonthlyRevenueCents: 1},
		{CustomerTier: "mainstay", CurrentPriceCents: 1, MonthlyRevenueCents: 1},
		{CustomerTier: "optimize", CurrentPriceCents: 1, MonthlyRevenueCents: 1},
	}, time.Date(2026, time.August, 17, 12, 0, 0, 0, cycle.Location))

	total := 0
	for _, tier := range analysis.CustomerTiers {
		total += tier.RevenueSharePercent
	}
	if total != 100 {
		t.Fatalf("customer tier revenue shares total = %d, want 100: %#v", total, analysis.CustomerTiers)
	}
	if analysis.CustomerTiers[0].RevenueSharePercent != 34 ||
		analysis.CustomerTiers[1].RevenueSharePercent != 33 ||
		analysis.CustomerTiers[2].RevenueSharePercent != 33 {
		t.Fatalf("largest-remainder allocation = %#v", analysis.CustomerTiers)
	}
}

func TestPricingHistoryExcludesRefundedBills(t *testing.T) {
	bills := []model.Bill{
		{ID: 1, SubscriptionID: 9, DueDate: "2026-05-01", AmountCents: 9000},
		{ID: 2, SubscriptionID: 9, DueDate: "2026-06-01", AmountCents: 10000},
		{ID: 3, SubscriptionID: 9, DueDate: "2026-07-01", AmountCents: 10000},
	}
	histories := summarizePricingBillHistories(bills, map[int64]struct{}{2: {}})
	history := histories[9]
	if history.PaidPeriodCount != 2 || history.LastIncreaseDate != "2026-07-01" {
		t.Fatalf("pricing history excluding refund = %#v", history)
	}
}

func TestBulkPricingExemptionIsAtomicBlocksCurrentRoundAndReentersLater(t *testing.T) {
	service := openGoalTestService(t)
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "exemption-owner@example.com",
		CostYuan:  "50.00",
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
		Name:                "Exemption customer",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 9000,
		CronExpr:            "interval:30d",
		CustomerEmail:       "exemption-customer@example.com",
		SeatID:              seats[0].ID,
		BoardedAt:           "2026-05-17",
	})
	if err != nil {
		t.Fatal(err)
	}
	seedPaidPricingPeriods(t, service, subscriptionID, 9000)
	snapshot := model.MarketPriceSnapshot{
		Provider:         marketProvider,
		Product:          marketProduct,
		LowPriceCents:    10000,
		MedianPriceCents: 13000,
		HighPriceCents:   15000,
		SampleCount:      10,
		SourceUpdatedAt:  service.now(),
		CreatedAt:        service.now(),
	}
	if _, err := service.Store.InsertMarketPriceSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}

	beforeCandidates, err := service.buildPricingCandidates(&snapshot, 0)
	if err != nil || len(beforeCandidates) != 1 || !beforeCandidates[0].Recommended {
		t.Fatalf("before candidates = %#v, err = %v", beforeCandidates, err)
	}
	before := beforeCandidates[0]

	if _, err := service.ExemptBulkPricing(BulkPricingExemptionInput{
		SubscriptionIDs: []int64{subscriptionID, 999999},
		ReviewCycles:    1,
		ReasonCode:      "relationship_investment",
	}); err == nil {
		t.Fatal("mixed valid/invalid exemption batch unexpectedly succeeded")
	}
	if exemptions, listErr := service.Store.ListPricingExemptions(); listErr != nil || len(exemptions) != 0 {
		t.Fatalf("failed batch left exemptions = %#v, err = %v", exemptions, listErr)
	}
	for _, input := range []BulkPricingExemptionInput{
		{SubscriptionIDs: []int64{subscriptionID}, ReviewCycles: 0, ReasonCode: "manual"},
		{SubscriptionIDs: []int64{subscriptionID}, ReviewCycles: 1, ReasonCode: "invalid"},
	} {
		if _, err := service.ExemptBulkPricing(input); err == nil {
			t.Fatalf("invalid exemption input unexpectedly succeeded: %#v", input)
		}
	}

	updated, err := service.ExemptBulkPricing(BulkPricingExemptionInput{
		SubscriptionIDs: []int64{subscriptionID, subscriptionID},
		ReviewCycles:    1,
		ReasonCode:      "relationship_investment",
		Note:            "Preserve this billing round",
	})
	if err != nil || updated != 1 {
		t.Fatalf("exemption result = %d, err = %v", updated, err)
	}
	exemptions, err := service.Store.ListPricingExemptions()
	if err != nil || len(exemptions) != 1 || exemptions[0].ReviewAfter != "2026-09-14" {
		t.Fatalf("stored exemptions = %#v, err = %v", exemptions, err)
	}

	activeCandidates, err := service.buildPricingCandidates(&snapshot, 0)
	if err != nil || len(activeCandidates) != 1 {
		t.Fatalf("active candidates = %#v, err = %v", activeCandidates, err)
	}
	active := activeCandidates[0]
	if active.Eligible || active.Recommended || active.BlockedCode != "exempted" ||
		active.NextReviewDate != "2026-09-14" || active.ExemptionCount != 1 ||
		active.PricePressureScore <= before.PricePressureScore {
		t.Fatalf("active exemption candidate = %#v, before = %#v", active, before)
	}
	if _, err := service.ScheduleBulkNextPrice(BulkNextPriceInput{
		SubscriptionIDs: []int64{subscriptionID},
		NextPriceYuan:   "97.00",
	}); err == nil {
		t.Fatal("active exemption was bypassed by pricing service")
	}
	stale, err := service.Store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	stalePrice := int64(9700)
	stale.NextPriceCents = &stalePrice
	stale.NextPriceEffectiveDueDate = "2026-10-14"
	if err := service.Store.UpdateSubscriptionNextPrices([]model.Subscription{stale}, "2026-08-15"); err != sql.ErrNoRows {
		t.Fatalf("store active-exemption guard err = %v, want sql.ErrNoRows", err)
	}

	laterService := &SubscriptionService{
		Store:        service.Store,
		MarketClient: service.MarketClient,
		Clock: func() time.Time {
			return time.Date(2026, time.September, 14, 12, 0, 0, 0, cycle.Location)
		},
	}
	expiredCandidates, err := laterService.buildPricingCandidates(&snapshot, 0)
	if err != nil || len(expiredCandidates) != 1 || !expiredCandidates[0].Eligible ||
		!expiredCandidates[0].Recommended || expiredCandidates[0].BlockedCode != "eligible" {
		t.Fatalf("expired exemption did not reenter evaluation: %#v, err = %v", expiredCandidates, err)
	}
	updated, err = laterService.ExemptBulkPricing(BulkPricingExemptionInput{
		SubscriptionIDs: []int64{subscriptionID},
		ReviewCycles:    1,
		ReasonCode:      "price_observation",
	})
	if err != nil || updated != 1 {
		t.Fatalf("second exemption result = %d, err = %v", updated, err)
	}
	repeatedCandidates, err := laterService.buildPricingCandidates(&snapshot, 0)
	if err != nil || len(repeatedCandidates) != 1 {
		t.Fatalf("repeated candidates = %#v, err = %v", repeatedCandidates, err)
	}
	repeated := repeatedCandidates[0]
	if repeated.ExemptionCount != 2 || repeated.ExemptionReviewDate != "2026-10-14" ||
		repeated.PricePressureScore <= active.PricePressureScore {
		t.Fatalf("repeated exemption scores = %#v, first = %#v", repeated, active)
	}
	if err := laterService.Store.ResetBusinessData(); err != nil {
		t.Fatalf("reset business data with exemption history: %v", err)
	}
	if resetExemptions, listErr := laterService.Store.ListPricingExemptions(); listErr != nil || len(resetExemptions) != 0 {
		t.Fatalf("reset exemption history = %#v, err = %v", resetExemptions, listErr)
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
