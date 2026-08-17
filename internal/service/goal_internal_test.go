package service

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
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
	// A 30-day cycle is normalized with 365/(30*12), then the monthly
	// owner-account cost is deducted: 10800*365/360 - 3000 = 7950.
	if center.Forecast.RunRateMonthlyProfitCents != 7950 {
		t.Fatalf("run rate = %d, want 7950", center.Forecast.RunRateMonthlyProfitCents)
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
	if center.Repricing.RecommendedCount != 2 || center.Repricing.EligibleCount != 2 ||
		center.Repricing.BelowMarketCount != 2 || center.Repricing.EstimatedMonthlyUpliftCents != 1476 ||
		len(center.Repricing.Windows) != 5 || center.Repricing.Windows[0].Key != "ready" ||
		center.Repricing.Windows[0].Count != 2 || center.Repricing.RiskSegments[0].Count != 2 ||
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
