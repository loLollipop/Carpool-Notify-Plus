package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

const (
	defaultMarketPriceURL  = "https://priceai.cc/api/products/chatgpt-team-business/offers?limit=100&offset=0&tags=team_official"
	marketSourcePageURL    = "https://priceai.cc/products/chatgpt-team-business?back=platform%3DChatGPT&tags=team_official"
	marketProvider         = "priceai"
	marketProduct          = "chatgpt-team-business"
	marketCacheTTL         = 6 * time.Hour
	goalTrendMonths        = 6
	goalHistoryLimit       = 20
	marketHistoryLimit     = 24
	defaultMarketTimeout   = 8 * time.Second
	maxMarketResponseBytes = 2 << 20

	// Repricing safeguards favor retention over extracting the full market gap
	// immediately. The thresholds are intentionally conservative for monthly,
	// relationship-based Team subscriptions.
	minimumRepricingPaidPeriods        = 3
	minimumRepricingRelationshipDays   = 60
	expeditedRepricingPaidPeriods      = 2
	expeditedRepricingRelationshipDays = 30
	repricingCooldownDays              = 180
	repricingAfterSalesRecoveryDays    = 30
	maximumRepricingIncreasePercent    = 8
	maximumRepricingIncreaseCents      = int64(1000)
	marketRenewalDiscountPercent       = 5
	minimumPriceIncreaseNoticeDays     = 30
	newSaleFillDiscountPercent         = int64(7)
	newSaleBalancedDiscountPercent     = int64(5)
	newSaleScarceDiscountPercent       = int64(3)
	minimumNewSaleRangeCents           = int64(300)
	maximumBulkPricingSelection        = 200
	maximumPricingExemptionCycles      = 6
	maximumPricingExemptionNoteRunes   = 500
)

// MarketHTTPDoer makes the external market client replaceable in tests.
type MarketHTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

type BusinessGoalInput struct {
	Name             string
	TargetProfitYuan string
}

type BusinessGoalProgress struct {
	Goal                 model.BusinessGoal `json:"goal"`
	CurrentProfitCents   int64              `json:"current_profit_cents"`
	EarnedProfitCents    int64              `json:"earned_profit_cents"`
	RemainingProfitCents int64              `json:"remaining_profit_cents"`
	ProgressPercent      int                `json:"progress_percent"`
	Reached              bool               `json:"reached"`
}

type CompletedGoalView struct {
	Goal            model.BusinessGoal `json:"goal"`
	ProgressPercent int                `json:"progress_percent"`
	Reached         bool               `json:"reached"`
}

type ProfitMonth struct {
	Month        string `json:"month"`
	RevenueCents int64  `json:"revenue_cents"`
	CostCents    int64  `json:"cost_cents"`
	RefundCents  int64  `json:"refund_cents"`
	ProfitCents  int64  `json:"profit_cents"`
}

type ForecastScenario struct {
	MonthlyProfitCents int64   `json:"monthly_profit_cents"`
	MonthsNeeded       float64 `json:"months_needed"`
	ProjectedDate      string  `json:"projected_date"`
}

type ProfitForecast struct {
	Source                    string           `json:"source"`
	ActiveRecurringCount      int              `json:"active_recurring_count"`
	RunRateMonthlyProfitCents int64            `json:"run_rate_monthly_profit_cents"`
	Conservative              ForecastScenario `json:"conservative"`
	Baseline                  ForecastScenario `json:"baseline"`
	Optimistic                ForecastScenario `json:"optimistic"`
}

type MarketPriceView struct {
	Available bool                        `json:"available"`
	Stale     bool                        `json:"stale"`
	Warning   string                      `json:"warning"`
	SourceURL string                      `json:"source_url"`
	Snapshot  *model.MarketPriceSnapshot  `json:"snapshot"`
	History   []model.MarketPriceSnapshot `json:"history"`
}

type PricingRecommendation struct {
	Action                   string   `json:"action"`
	ReasonCodes              []string `json:"reason_codes"`
	InternalMedianPriceCents int64    `json:"internal_median_price_cents"`
	SuggestedLowPriceCents   int64    `json:"suggested_low_price_cents"`
	SuggestedHighPriceCents  int64    `json:"suggested_high_price_cents"`
	SeatCostFloorCents       int64    `json:"seat_cost_floor_cents"`
	SeatTotal                int      `json:"seat_total"`
	SeatUsed                 int      `json:"seat_used"`
	SeatAvailable            int      `json:"seat_available"`
	UtilizationPercent       int      `json:"utilization_percent"`
	NewSaleDiscountPercent   int      `json:"new_sale_discount_percent"`
}

// PricingCandidate exposes one active Team customer for market comparison and
// optional next-cycle repricing. Current-period prices and bills are never
// mutated by the bulk action.
type PricingCandidate struct {
	SubscriptionID                   int64    `json:"subscription_id"`
	Name                             string   `json:"name"`
	CustomerEmail                    string   `json:"customer_email"`
	CustomerWechat                   string   `json:"customer_wechat"`
	AccountName                      string   `json:"account_name"`
	SeatName                         string   `json:"seat_name"`
	CurrentPriceCents                int64    `json:"current_price_cents"`
	NextPriceCents                   *int64   `json:"next_price_cents"`
	NextPriceEffectiveDate           string   `json:"next_price_effective_date"`
	NextDueDate                      string   `json:"next_due_date"`
	MarketPosition                   string   `json:"market_position"`
	GapToMarketMedianCents           int64    `json:"gap_to_market_median_cents"`
	SuggestedPriceCents              int64    `json:"suggested_price_cents"`
	MaxIncreasePriceCents            int64    `json:"max_increase_price_cents"`
	PaidPeriodCount                  int      `json:"paid_period_count"`
	RelationshipDays                 int      `json:"relationship_days"`
	LastPriceIncreaseDate            string   `json:"last_price_increase_date"`
	BlockedCode                      string   `json:"blocked_code"`
	NextReviewDate                   string   `json:"next_review_date"`
	SuggestedMonthlyUplift           int64    `json:"suggested_monthly_uplift_cents"`
	ScheduledMonthlyUplift           int64    `json:"scheduled_monthly_uplift_cents"`
	MonthlyRevenueCents              int64    `json:"monthly_revenue_cents"`
	CustomerGroupID                  int64    `json:"-"`
	CustomerGroupSize                int      `json:"customer_group_size"`
	CustomerGroupMonthlyRevenueCents int64    `json:"customer_group_monthly_revenue_cents"`
	CustomerTier                     string   `json:"customer_tier"`
	RelationshipStage                string   `json:"relationship_stage"`
	AdjustmentRisk                   string   `json:"adjustment_risk"`
	ReadinessScore                   int      `json:"readiness_score"`
	PriceGapPercent                  int      `json:"price_gap_percent"`
	SuggestedIncreasePct             int      `json:"suggested_increase_percent"`
	AnalysisCodes                    []string `json:"analysis_codes"`
	ExpeditedReview                  bool     `json:"expedited_review"`
	ExemptionCount                   int      `json:"exemption_count"`
	LastExemptedAt                   string   `json:"last_exempted_at"`
	ExemptionReviewDate              string   `json:"exemption_review_date"`
	ExemptionReasonCode              string   `json:"exemption_reason_code"`
	LoyaltyScore                     int      `json:"loyalty_score"`
	RelationshipAssetScore           int      `json:"relationship_asset_score"`
	PricePressureScore               int      `json:"price_pressure_score"`
	PriceStableDays                  int      `json:"price_stable_days"`
	PaidPeriodsAfterIncrease         int      `json:"paid_periods_after_increase"`
	AfterSalesCaseCount              int      `json:"after_sales_case_count"`
	Recommended                      bool     `json:"recommended"`
	Eligible                         bool     `json:"eligible"`
	BlockedReason                    string   `json:"blocked_reason"`
}

type RepricingWindow struct {
	Key                string `json:"key"`
	Count              int    `json:"count"`
	MonthlyUpliftCents int64  `json:"monthly_uplift_cents"`
}

type RepricingSegment struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// CustomerTierSummary groups Team customers by total monthly customer value.
// It is intentionally separate from market position and repricing risk: the
// tier describes customer value, while the safeguards decide whether and when
// a price change is appropriate.
type CustomerTierSummary struct {
	Key                 string `json:"key"`
	Count               int    `json:"count"`
	CustomerCount       int    `json:"customer_count"`
	MonthlyRevenueCents int64  `json:"monthly_revenue_cents"`
	RevenueSharePercent int    `json:"revenue_share_percent"`
	AveragePriceCents   int64  `json:"average_price_cents"`
	LowestPriceCents    int64  `json:"lowest_price_cents"`
	HighestPriceCents   int64  `json:"highest_price_cents"`
	RecommendedCount    int    `json:"recommended_count"`
	ScheduledCount      int    `json:"scheduled_count"`
}

type RepricingAnalysis struct {
	TotalCount                  int                   `json:"total_count"`
	CustomerCount               int                   `json:"customer_count"`
	EligibleCount               int                   `json:"eligible_count"`
	RecommendedCount            int                   `json:"recommended_count"`
	ScheduledCount              int                   `json:"scheduled_count"`
	ProtectedCount              int                   `json:"protected_count"`
	BelowMarketCount            int                   `json:"below_market_count"`
	EstimatedMonthlyUpliftCents int64                 `json:"estimated_monthly_uplift_cents"`
	PipelineMonthlyUpliftCents  int64                 `json:"pipeline_monthly_uplift_cents"`
	ScheduledMonthlyUpliftCents int64                 `json:"scheduled_monthly_uplift_cents"`
	Windows                     []RepricingWindow     `json:"windows"`
	AverageRelationshipDays     int                   `json:"average_relationship_days"`
	AveragePaidPeriods          float64               `json:"average_paid_periods"`
	AverageLoyaltyScore         int                   `json:"average_loyalty_score"`
	AverageRelationshipAsset    int                   `json:"average_relationship_asset_score"`
	AveragePricePressure        int                   `json:"average_price_pressure_score"`
	ActiveExemptionCount        int                   `json:"active_exemption_count"`
	RelationshipSegments        []RepricingSegment    `json:"relationship_segments"`
	RiskSegments                []RepricingSegment    `json:"risk_segments"`
	PriceSegments               []RepricingSegment    `json:"price_segments"`
	CustomerTiers               []CustomerTierSummary `json:"customer_tiers"`
}

type BulkNextPriceInput struct {
	SubscriptionIDs []int64
	NextPriceYuan   string
}

type BulkPricingExemptionInput struct {
	SubscriptionIDs []int64
	ReviewCycles    int
	ReasonCode      string
	Note            string
}

type GoalCenter struct {
	ActiveGoal *BusinessGoalProgress `json:"active_goal"`
	History    []CompletedGoalView   `json:"history"`
	Trend      []ProfitMonth         `json:"trend"`
	Forecast   *ProfitForecast       `json:"forecast"`
	Market     MarketPriceView       `json:"market"`
	Pricing    PricingRecommendation `json:"pricing"`
	Candidates []PricingCandidate    `json:"pricing_candidates"`
	Repricing  RepricingAnalysis     `json:"repricing_analysis"`
}

func (service *SubscriptionService) CreateBusinessGoal(input BusinessGoalInput) (int64, error) {
	name, targetProfitCents, err := service.normalizeBusinessGoalInput(input)
	if err != nil {
		return 0, err
	}
	if _, err := service.Store.GetActiveBusinessGoal(); err == nil {
		return 0, fmt.Errorf("已有进行中的目标，请先完成后再创建")
	} else if err != sql.ErrNoRows {
		return 0, err
	}
	return service.Store.CreateBusinessGoal(model.BusinessGoal{
		Name:              name,
		TargetProfitCents: targetProfitCents,
	})
}

func (service *SubscriptionService) UpdateBusinessGoal(goalID int64, input BusinessGoalInput) error {
	name, targetProfitCents, err := service.normalizeBusinessGoalInput(input)
	if err != nil {
		return err
	}
	if err := service.Store.UpdateBusinessGoal(goalID, name, targetProfitCents); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("进行中的目标不存在")
		}
		return err
	}
	return nil
}

func (service *SubscriptionService) CompleteBusinessGoal(goalID int64) error {
	goal, err := service.Store.GetActiveBusinessGoal()
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("进行中的目标不存在")
		}
		return err
	}
	if goal.ID != goalID {
		return fmt.Errorf("进行中的目标不存在")
	}
	dashboard, err := service.ComputeDashboard()
	if err != nil {
		return err
	}
	return service.Store.CompleteBusinessGoal(goalID, dashboard.TotalProfitCents)
}

func (service *SubscriptionService) normalizeBusinessGoalInput(input BusinessGoalInput) (string, int64, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return "", 0, fmt.Errorf("请填写目标名称")
	}
	if len([]rune(name)) > 80 {
		return "", 0, fmt.Errorf("目标名称不能超过 80 个字符")
	}
	targetProfitCents, err := cycle.ParseYuanToCents(strings.TrimSpace(input.TargetProfitYuan))
	if err != nil || targetProfitCents <= 0 {
		return "", 0, fmt.Errorf("目标利润须大于 0 元")
	}
	return name, targetProfitCents, nil
}

func (service *SubscriptionService) GetGoalCenter() (GoalCenter, error) {
	dashboard, err := service.ComputeDashboard()
	if err != nil {
		return GoalCenter{}, err
	}
	goals, err := service.Store.ListBusinessGoals(goalHistoryLimit)
	if err != nil {
		return GoalCenter{}, err
	}
	trend, err := service.buildProfitTrend(goalTrendMonths)
	if err != nil {
		return GoalCenter{}, err
	}

	center := GoalCenter{
		History: make([]CompletedGoalView, 0),
		Trend:   trend,
	}
	for _, goal := range goals {
		if goal.Status == model.BusinessGoalStatusActive && center.ActiveGoal == nil {
			progress := service.buildGoalProgress(goal, dashboard.TotalProfitCents)
			center.ActiveGoal = &progress
			continue
		}
		if goal.Status == model.BusinessGoalStatusCompleted {
			center.History = append(center.History, CompletedGoalView{
				Goal:            goal,
				ProgressPercent: percentOf(goal.ResultProfitCents, goal.TargetProfitCents),
				Reached:         goal.ResultProfitCents >= goal.TargetProfitCents,
			})
		}
	}

	runRateCents, activeRecurringCount, err := service.activeMonthlyProfitRunRate()
	if err != nil {
		return GoalCenter{}, err
	}
	if center.ActiveGoal != nil {
		forecast := service.buildProfitForecast(*center.ActiveGoal, runRateCents, activeRecurringCount)
		center.Forecast = &forecast
	}

	market, marketErr := service.loadMarketPrice(false)
	if marketErr != nil {
		market.Warning = marketErr.Error()
	}
	center.Market = market
	center.Pricing, err = service.buildPricingRecommendation(market.Snapshot)
	if err != nil {
		return GoalCenter{}, err
	}
	minimumHealthyPrice := center.Pricing.SeatCostFloorCents * 120 / 100
	center.Candidates, err = service.buildPricingCandidates(market.Snapshot, minimumHealthyPrice)
	if err != nil {
		return GoalCenter{}, err
	}
	center.Repricing = buildRepricingAnalysis(center.Candidates, service.now())
	return center, nil
}

func (service *SubscriptionService) RefreshMarketPrice() (MarketPriceView, error) {
	return service.loadMarketPrice(true)
}

func (service *SubscriptionService) buildGoalProgress(goal model.BusinessGoal, currentProfitCents int64) BusinessGoalProgress {
	earnedProfitCents := currentProfitCents
	remainingProfitCents := goal.TargetProfitCents - earnedProfitCents
	if remainingProfitCents < 0 {
		remainingProfitCents = 0
	}
	return BusinessGoalProgress{
		Goal:                 goal,
		CurrentProfitCents:   currentProfitCents,
		EarnedProfitCents:    earnedProfitCents,
		RemainingProfitCents: remainingProfitCents,
		ProgressPercent:      percentOf(earnedProfitCents, goal.TargetProfitCents),
		Reached:              earnedProfitCents >= goal.TargetProfitCents,
	}
}

func (service *SubscriptionService) buildProfitTrend(monthCount int) ([]ProfitMonth, error) {
	if monthCount < 1 {
		monthCount = goalTrendMonths
	}
	now := service.now().In(cycle.Location)
	firstMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, cycle.Location).AddDate(0, -(monthCount - 1), 0)
	months := make([]ProfitMonth, 0, monthCount)
	monthIndex := make(map[string]int, monthCount)
	for index := 0; index < monthCount; index++ {
		key := firstMonth.AddDate(0, index, 0).Format("2006-01")
		monthIndex[key] = len(months)
		months = append(months, ProfitMonth{Month: key})
	}

	bills, err := service.Store.ListBills()
	if err != nil {
		return nil, err
	}
	activeSubscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		return nil, err
	}
	archivedSubscriptions, err := service.Store.ListArchivedSubscriptions()
	if err != nil {
		return nil, err
	}
	businessTypeBySubscriptionID := make(map[int64]string, len(activeSubscriptions)+len(archivedSubscriptions))
	for _, subscription := range activeSubscriptions {
		businessTypeBySubscriptionID[subscription.ID] = subscription.BusinessType
	}
	for _, subscription := range archivedSubscriptions {
		businessTypeBySubscriptionID[subscription.ID] = subscription.BusinessType
	}
	for _, bill := range bills {
		// Profit trend is an accounting-period view. Historical bills are often
		// imported later, so using paid_at would move July revenue into the import
		// month and rewrite history. due_date remains the stable period identity.
		key := monthFromDate(bill.DueDate)
		if key == "" {
			key = bill.PaidAt.In(cycle.Location).Format("2006-01")
		}
		if index, exists := monthIndex[key]; exists {
			months[index].RevenueCents += bill.AmountCents
			// Team costs belong to the owner-account ledger. Ignore legacy Team
			// bill snapshots here or the same cost would be counted twice.
			if businessTypeBySubscriptionID[bill.SubscriptionID] == model.SubscriptionBusinessPlus {
				months[index].CostCents += bill.CostCents
			}
		}
	}

	costRecords, err := service.Store.ListAllAccountCostRecords()
	if err != nil {
		return nil, err
	}
	accounts, err := service.Store.ListAccounts()
	if err != nil {
		return nil, err
	}
	type accountOpeningCost struct {
		month            string
		monthlyCostCents int64
	}
	accountOpeningCosts := make(map[int64]accountOpeningCost, len(accounts))
	for _, account := range accounts {
		if key := monthFromDate(account.OpenedAt); key != "" {
			accountOpeningCosts[account.ID] = accountOpeningCost{
				month:            key,
				monthlyCostCents: account.CostCents,
			}
		}
	}
	for _, record := range costRecords {
		key := monthFromDate(record.PeriodDate)
		// Imported initial owner-account costs belong to the opening period, not
		// the later date on which the historical account was entered. Keep this
		// reporting fallback even after the storage repair for conflict safety.
		// Historical cumulative balances deliberately stay in their import month.
		if record.Source == model.AccountCostSourceInitial {
			if openingCost, exists := accountOpeningCosts[record.AccountID]; exists &&
				record.AmountCents == openingCost.monthlyCostCents &&
				record.Note != "Historical cumulative account cost" {
				key = openingCost.month
			}
		}
		if index, exists := monthIndex[key]; exists {
			months[index].CostCents += record.AmountCents
		}
	}

	afterSalesCases, err := service.Store.ListAfterSalesCases()
	if err != nil {
		return nil, err
	}
	for _, caseItem := range afterSalesCases {
		if caseItem.Status != model.AfterSalesStatusRefunded || caseItem.ProcessedAt == nil {
			continue
		}
		key := caseItem.ProcessedAt.In(cycle.Location).Format("2006-01")
		if index, exists := monthIndex[key]; exists {
			months[index].RefundCents += caseItem.RefundAmountCents
		}
	}
	for index := range months {
		months[index].ProfitCents = months[index].RevenueCents - months[index].CostCents - months[index].RefundCents
	}
	return months, nil
}

func (service *SubscriptionService) activeMonthlyProfitRunRate() (int64, int, error) {
	subscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		return 0, 0, err
	}
	afterSalesCases, err := service.Store.ListAfterSalesCases()
	if err != nil {
		return 0, 0, err
	}
	frozenSubscriptions := make(map[int64]struct{})
	for _, caseItem := range afterSalesCases {
		if caseItem.Status == model.AfterSalesStatusPending || caseItem.Status == model.AfterSalesStatusReview {
			frozenSubscriptions[caseItem.SubscriptionID] = struct{}{}
		}
	}

	var monthlyProfitCents int64
	activeRecurringCount := 0
	for _, subscription := range subscriptions {
		if _, frozen := frozenSubscriptions[subscription.ID]; frozen {
			continue
		}
		if cycle.IsOneMonthRentalExpression(subscription.CronExpr) {
			continue
		}
		factorNumerator, factorDenominator := monthlyCycleFactor(subscription, service.now())
		if factorDenominator <= 0 {
			continue
		}
		futureAmountCents := countedAmountCents(subscription)
		if !subscription.IsResale && subscription.NextPriceCents != nil {
			futureAmountCents = *subscription.NextPriceCents
		}
		monthlyProfitCents += futureAmountCents * factorNumerator / factorDenominator
		if isPlusSubscription(subscription) {
			monthlyProfitCents -= subscription.CostCents * factorNumerator / factorDenominator
		}
		activeRecurringCount++
	}
	accounts, err := service.Store.ListAccounts()
	if err != nil {
		return 0, 0, err
	}
	for _, account := range accounts {
		if strings.TrimSpace(account.BannedAt) == "" {
			monthlyProfitCents -= account.CostCents
		}
	}
	return monthlyProfitCents, activeRecurringCount, nil
}

func monthlyCycleFactor(subscription model.Subscription, now time.Time) (int64, int64) {
	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		return 0, 0
	}
	dueDates := schedule.NextDueTimes(now, 2)
	if len(dueDates) < 2 {
		return 0, 0
	}
	days := int64(math.Round(dueDates[1].Sub(dueDates[0]).Hours() / 24))
	if days < 1 {
		return 0, 0
	}
	return 365, days * 12
}

func (service *SubscriptionService) buildProfitForecast(
	goal BusinessGoalProgress,
	runRateCents int64,
	activeRecurringCount int,
) ProfitForecast {
	baseMonthlyProfitCents := int64(0)
	source := "unavailable"
	if runRateCents > 0 && activeRecurringCount > 0 {
		baseMonthlyProfitCents = runRateCents
		source = "run_rate"
	}
	conservativeMonthly := baseMonthlyProfitCents * 75 / 100
	optimisticMonthly := baseMonthlyProfitCents * 125 / 100
	return ProfitForecast{
		Source:                    source,
		ActiveRecurringCount:      activeRecurringCount,
		RunRateMonthlyProfitCents: runRateCents,
		Conservative:              service.buildForecastScenario(goal, conservativeMonthly),
		Baseline:                  service.buildForecastScenario(goal, baseMonthlyProfitCents),
		Optimistic:                service.buildForecastScenario(goal, optimisticMonthly),
	}
}

func (service *SubscriptionService) buildForecastScenario(goal BusinessGoalProgress, monthlyProfitCents int64) ForecastScenario {
	scenario := ForecastScenario{MonthlyProfitCents: monthlyProfitCents}
	if goal.RemainingProfitCents <= 0 {
		scenario.ProjectedDate = cycle.FormatDate(service.now())
		return scenario
	}
	if monthlyProfitCents <= 0 {
		return scenario
	}
	daysNeeded := divideRoundUp(goal.RemainingProfitCents*30, monthlyProfitCents)
	projected := cycle.StartOfDay(service.now()).AddDate(0, 0, int(daysNeeded))
	scenario.MonthsNeeded = math.Round(float64(daysNeeded)/30*10) / 10
	scenario.ProjectedDate = cycle.FormatDate(projected)
	return scenario
}

func (service *SubscriptionService) loadMarketPrice(force bool) (MarketPriceView, error) {
	latest, latestErr := service.Store.LatestMarketPriceSnapshot(marketProvider, marketProduct)
	if latestErr != nil && latestErr != sql.ErrNoRows {
		return MarketPriceView{SourceURL: marketSourcePageURL}, latestErr
	}
	hasLatest := latestErr == nil
	if !force && hasLatest && service.now().Sub(latest.CreatedAt.In(cycle.Location)) < marketCacheTTL {
		return service.marketPriceView(latest, false, ""), nil
	}

	refreshed, err := service.fetchMarketPriceSnapshot()
	if err == nil {
		if _, insertErr := service.Store.InsertMarketPriceSnapshot(refreshed); insertErr != nil {
			return MarketPriceView{SourceURL: marketSourcePageURL}, insertErr
		}
		return service.marketPriceView(refreshed, false, ""), nil
	}
	if hasLatest {
		return service.marketPriceView(latest, true, "行情更新失败，当前显示最近一次缓存"), nil
	}
	return MarketPriceView{SourceURL: marketSourcePageURL}, fmt.Errorf("暂时无法获取市场行情：%w", err)
}

func (service *SubscriptionService) marketPriceView(
	snapshot model.MarketPriceSnapshot,
	stale bool,
	warning string,
) MarketPriceView {
	history, err := service.Store.ListMarketPriceSnapshots(marketProvider, marketProduct, marketHistoryLimit)
	if err != nil {
		history = []model.MarketPriceSnapshot{snapshot}
	}
	for left, right := 0, len(history)-1; left < right; left, right = left+1, right-1 {
		history[left], history[right] = history[right], history[left]
	}
	return MarketPriceView{
		Available: true,
		Stale:     stale,
		Warning:   warning,
		SourceURL: marketSourcePageURL,
		Snapshot:  &snapshot,
		History:   history,
	}
}

type priceAIOffersResponse struct {
	Offers      []priceAIOffer `json:"offers"`
	GeneratedAt time.Time      `json:"generatedAt"`
}

type priceAIOffer struct {
	SourceID        string     `json:"sourceId"`
	SourceTitle     string     `json:"sourceTitle"`
	Price           float64    `json:"price"`
	Currency        string     `json:"currency"`
	Status          string     `json:"status"`
	EffectiveStatus string     `json:"effectiveStatus"`
	StockCount      *int       `json:"stockCount"`
	Hidden          bool       `json:"hidden"`
	SourceUpdatedAt *time.Time `json:"sourceUpdatedAt"`
}

func (service *SubscriptionService) fetchMarketPriceSnapshot() (model.MarketPriceSnapshot, error) {
	marketURL := strings.TrimSpace(service.MarketPriceURL)
	if marketURL == "" {
		marketURL = defaultMarketPriceURL
	}
	request, err := http.NewRequest(http.MethodGet, marketURL, nil)
	if err != nil {
		return model.MarketPriceSnapshot{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "CarpoolNotify/1.0 market-monitor")
	client := service.MarketClient
	if client == nil {
		client = &http.Client{Timeout: defaultMarketTimeout}
	}
	response, err := client.Do(request)
	if err != nil {
		return model.MarketPriceSnapshot{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return model.MarketPriceSnapshot{}, fmt.Errorf("PriceAI returned HTTP %d", response.StatusCode)
	}
	var payload priceAIOffersResponse
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxMarketResponseBytes))
	if err := decoder.Decode(&payload); err != nil {
		return model.MarketPriceSnapshot{}, fmt.Errorf("decode PriceAI response: %w", err)
	}
	prices, sourceUpdatedAt := comparableMarketPrices(payload.Offers)
	if len(prices) < 3 {
		return model.MarketPriceSnapshot{}, fmt.Errorf("可比的在售 Team 单席位报价不足 3 条")
	}
	if sourceUpdatedAt.IsZero() {
		sourceUpdatedAt = payload.GeneratedAt
	}
	if sourceUpdatedAt.IsZero() {
		sourceUpdatedAt = service.now().UTC()
	}
	return model.MarketPriceSnapshot{
		Provider:         marketProvider,
		Product:          marketProduct,
		LowPriceCents:    percentileCents(prices, 0.25),
		MedianPriceCents: percentileCents(prices, 0.50),
		HighPriceCents:   percentileCents(prices, 0.75),
		SampleCount:      len(prices),
		SourceUpdatedAt:  sourceUpdatedAt.UTC(),
		CreatedAt:        service.now().UTC(),
	}, nil
}

func comparableMarketPrices(offers []priceAIOffer) ([]int64, time.Time) {
	prices := make([]int64, 0, len(offers))
	seen := make(map[string]struct{})
	var newest time.Time
	for _, offer := range offers {
		if !isComparableMarketOffer(offer) {
			continue
		}
		priceCents := int64(math.Round(offer.Price * 100))
		key := strings.ToLower(strings.TrimSpace(offer.SourceID)) + "|" + normalizeOfferTitle(offer.SourceTitle) + fmt.Sprintf("|%d", priceCents)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		prices = append(prices, priceCents)
		if offer.SourceUpdatedAt != nil && offer.SourceUpdatedAt.After(newest) {
			newest = *offer.SourceUpdatedAt
		}
	}
	sort.Slice(prices, func(left int, right int) bool { return prices[left] < prices[right] })
	return prices, newest
}

func isComparableMarketOffer(offer priceAIOffer) bool {
	if offer.Hidden || !strings.EqualFold(strings.TrimSpace(offer.Currency), "CNY") || offer.Price <= 0 || offer.Price > 300 {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(offer.Status))
	effectiveStatus := strings.ToLower(strings.TrimSpace(offer.EffectiveStatus))
	if status == "out_of_stock" || effectiveStatus == "unavailable" || (offer.StockCount != nil && *offer.StockCount <= 0) {
		return false
	}
	title := strings.ToLower(strings.TrimSpace(offer.SourceTitle))
	for _, excluded := range []string{
		"premium", "母号", "管理员", "管理号", "admin", "两席", "双席", "二席", "2席", "2 席", "two seat", "2 seat",
	} {
		if strings.Contains(title, excluded) {
			return false
		}
	}
	for _, included := range []string{"席位", "slot", "激活码", "续费码"} {
		if strings.Contains(title, included) {
			return true
		}
	}
	return false
}

func normalizeOfferTitle(raw string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsSpace(character) || unicode.IsPunct(character) {
			return -1
		}
		return unicode.ToLower(character)
	}, raw)
}

func percentileCents(sortedPrices []int64, percentile float64) int64 {
	if len(sortedPrices) == 0 {
		return 0
	}
	if len(sortedPrices) == 1 {
		return sortedPrices[0]
	}
	position := percentile * float64(len(sortedPrices)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sortedPrices[lower]
	}
	weight := position - float64(lower)
	value := float64(sortedPrices[lower])*(1-weight) + float64(sortedPrices[upper])*weight
	return int64(math.Round(value))
}

func (service *SubscriptionService) buildPricingRecommendation(snapshot *model.MarketPriceSnapshot) (PricingRecommendation, error) {
	accounts, err := service.Store.ListAccounts()
	if err != nil {
		return PricingRecommendation{}, err
	}
	activeAccountIDs := make(map[int64]struct{})
	var monthlyAccountCostCents int64
	for _, account := range accounts {
		if strings.TrimSpace(account.BannedAt) != "" {
			continue
		}
		activeAccountIDs[account.ID] = struct{}{}
		monthlyAccountCostCents += account.CostCents
	}
	seats, err := service.Store.ListAllSeats()
	if err != nil {
		return PricingRecommendation{}, err
	}
	seatTotal := 0
	for _, seat := range seats {
		if _, active := activeAccountIDs[seat.AccountID]; active {
			seatTotal++
		}
	}
	subscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		return PricingRecommendation{}, err
	}
	teamPrices := make([]int64, 0)
	seatUsed := 0
	for _, subscription := range subscriptions {
		if subscription.BusinessType != model.SubscriptionBusinessTeam || subscription.SeatID <= 0 {
			continue
		}
		if _, active := activeAccountIDs[subscription.AccountID]; !active {
			continue
		}
		seatUsed++
		if !subscription.IsResale && subscription.PricePerPersonCents > 0 {
			teamPrices = append(teamPrices, subscription.PricePerPersonCents)
		}
	}
	sort.Slice(teamPrices, func(left int, right int) bool { return teamPrices[left] < teamPrices[right] })
	utilizationPercent := 0
	if seatTotal > 0 {
		utilizationPercent = int(math.Round(float64(seatUsed) * 100 / float64(seatTotal)))
	}
	seatCostFloorCents := int64(0)
	if seatTotal > 0 {
		seatCostFloorCents = divideRoundUp(monthlyAccountCostCents, int64(seatTotal))
	}
	recommendation := PricingRecommendation{
		Action:                   "insufficient",
		ReasonCodes:              []string{"insufficient_data"},
		InternalMedianPriceCents: percentileCents(teamPrices, 0.5),
		SeatCostFloorCents:       seatCostFloorCents,
		SeatTotal:                seatTotal,
		SeatUsed:                 seatUsed,
		SeatAvailable:            maxInt(seatTotal-seatUsed, 0),
		UtilizationPercent:       utilizationPercent,
	}
	if snapshot == nil || snapshot.SampleCount < 3 || len(teamPrices) == 0 || seatTotal == 0 {
		return recommendation, nil
	}

	internalMedian := recommendation.InternalMedianPriceCents
	marketLow := snapshot.LowPriceCents
	marketMedian := snapshot.MedianPriceCents
	marketHigh := snapshot.HighPriceCents
	minimumHealthyPrice := seatCostFloorCents * 120 / 100
	discountPercent := newSaleDiscountPercent(utilizationPercent)
	suggestedLow, suggestedHigh, discountApplied := attractiveNewSaleRange(
		marketLow,
		marketMedian,
		minimumHealthyPrice,
		discountPercent,
	)
	recommendation.SuggestedLowPriceCents = suggestedLow
	recommendation.SuggestedHighPriceCents = suggestedHigh
	if discountApplied {
		recommendation.NewSaleDiscountPercent = int(discountPercent)
	}
	switch {
	case utilizationPercent < 70:
		recommendation.Action = "fill"
		recommendation.ReasonCodes = []string{"low_utilization", "protect_occupancy"}
	case utilizationPercent >= 85 && internalMedian < marketLow*95/100:
		recommendation.Action = "raise"
		recommendation.ReasonCodes = []string{"high_utilization", "below_market"}
	case utilizationPercent < 85 && internalMedian > marketHigh*110/100:
		recommendation.Action = "lower_test"
		recommendation.ReasonCodes = []string{"above_market", "available_seats"}
	default:
		recommendation.Action = "hold"
		recommendation.ReasonCodes = []string{"price_in_range", "stable_utilization"}
	}
	if discountApplied {
		recommendation.ReasonCodes = append(recommendation.ReasonCodes, "new_sale_advantage")
	} else {
		recommendation.ReasonCodes = append(recommendation.ReasonCodes, "margin_floor_limits_discount")
	}
	return recommendation, nil
}

func (service *SubscriptionService) buildPricingCandidates(
	snapshot *model.MarketPriceSnapshot,
	minimumHealthyPriceCents int64,
) ([]PricingCandidate, error) {
	accounts, err := service.Store.ListAccounts()
	if err != nil {
		return nil, err
	}
	activeAccountIDs := make(map[int64]struct{}, len(accounts))
	for _, account := range accounts {
		if strings.TrimSpace(account.BannedAt) == "" {
			activeAccountIDs[account.ID] = struct{}{}
		}
	}

	afterSalesCases, err := service.Store.ListAfterSalesCases()
	if err != nil {
		return nil, err
	}
	frozenSubscriptions := make(map[int64]struct{})
	afterSalesRecoveryEnds := make(map[int64]time.Time)
	refundedBillIDs := make(map[int64]struct{})
	afterSalesCaseCounts := make(map[int64]int)
	for _, caseItem := range afterSalesCases {
		afterSalesCaseCounts[caseItem.SubscriptionID]++
		if caseItem.Status == model.AfterSalesStatusRefunded && caseItem.BillID > 0 {
			refundedBillIDs[caseItem.BillID] = struct{}{}
		}
		if caseItem.Status == model.AfterSalesStatusPending || caseItem.Status == model.AfterSalesStatusReview {
			frozenSubscriptions[caseItem.SubscriptionID] = struct{}{}
			continue
		}
		resolvedAt := caseItem.UpdatedAt
		if caseItem.ProcessedAt != nil {
			resolvedAt = *caseItem.ProcessedAt
		}
		if !resolvedAt.IsZero() {
			recoveryEndsAt := cycle.StartOfDay(resolvedAt.In(cycle.Location)).AddDate(0, 0, repricingAfterSalesRecoveryDays)
			if cycle.StartOfDay(service.now()).Before(recoveryEndsAt) &&
				recoveryEndsAt.After(afterSalesRecoveryEnds[caseItem.SubscriptionID]) {
				afterSalesRecoveryEnds[caseItem.SubscriptionID] = recoveryEndsAt
			}
		}
	}
	bills, err := service.Store.ListBills()
	if err != nil {
		return nil, err
	}
	billHistories := summarizePricingBillHistories(bills, refundedBillIDs)
	exemptions, err := service.Store.ListPricingExemptions()
	if err != nil {
		return nil, err
	}
	exemptionCounts := make(map[int64]int)
	latestExemptions := make(map[int64]model.PricingExemption)
	for _, exemption := range exemptions {
		exemptionCounts[exemption.SubscriptionID]++
		latest := latestExemptions[exemption.SubscriptionID]
		if latest.ID == 0 || exemption.CreatedAt.After(latest.CreatedAt) ||
			(exemption.CreatedAt.Equal(latest.CreatedAt) && exemption.ID > latest.ID) {
			latestExemptions[exemption.SubscriptionID] = exemption
		}
	}

	subscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		return nil, err
	}
	subscriptionsByID := make(map[int64]model.Subscription, len(subscriptions))
	candidates := make([]PricingCandidate, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		if subscription.BusinessType != model.SubscriptionBusinessTeam || subscription.SeatID <= 0 || subscription.IsResale {
			continue
		}
		subscriptionsByID[subscription.ID] = subscription
		candidate := PricingCandidate{
			SubscriptionID:         subscription.ID,
			Name:                   subscription.Name,
			CustomerEmail:          subscription.CustomerEmail,
			CustomerWechat:         subscription.CustomerWechat,
			AccountName:            subscription.AccountName,
			SeatName:               subscription.SeatName,
			CurrentPriceCents:      subscription.PricePerPersonCents,
			NextPriceCents:         subscription.NextPriceCents,
			NextPriceEffectiveDate: subscription.NextPriceEffectiveDueDate,
			Eligible:               true,
			BlockedCode:            "eligible",
			MarketPosition:         "unavailable",
			MaxIncreasePriceCents:  maximumGradualPriceCents(subscription.PricePerPersonCents),
		}
		history := billHistories[subscription.ID]
		candidate.PaidPeriodCount = history.PaidPeriodCount
		candidate.LastPriceIncreaseDate = history.LastIncreaseDate
		candidate.PaidPeriodsAfterIncrease = history.PaidPeriodsAfterIncrease
		candidate.RelationshipDays = subscriptionRelationshipDays(subscription, service.now())
		candidate.PriceStableDays = candidate.RelationshipDays
		if history.LastIncreaseDate != "" {
			if increasedAt, parseErr := time.ParseInLocation("2006-01-02", history.LastIncreaseDate, cycle.Location); parseErr == nil {
				candidate.PriceStableDays = maxInt(
					int(cycle.StartOfDay(service.now()).Sub(cycle.StartOfDay(increasedAt)).Hours()/24),
					0,
				)
			}
		}
		candidate.AfterSalesCaseCount = afterSalesCaseCounts[subscription.ID]
		candidate.ExemptionCount = exemptionCounts[subscription.ID]
		latestExemption := latestExemptions[subscription.ID]
		if latestExemption.ID > 0 {
			candidate.LastExemptedAt = cycle.FormatDate(latestExemption.CreatedAt.In(cycle.Location))
			candidate.ExemptionReasonCode = latestExemption.ReasonCode
		}
		block := func(code string, reason string, nextReviewDate string) {
			if candidate.Eligible {
				candidate.Eligible = false
				candidate.BlockedCode = code
				candidate.BlockedReason = reason
				candidate.NextReviewDate = nextReviewDate
			}
		}
		if _, active := activeAccountIDs[subscription.AccountID]; !active {
			block("account_banned", "所属 Team 账号已封禁", "")
		}
		if _, frozen := frozenSubscriptions[subscription.ID]; frozen {
			block("after_sales", "正在等待售后处理", "")
		}
		if recoveryEndsAt, recovering := afterSalesRecoveryEnds[subscription.ID]; recovering {
			block(
				"after_sales_recovery",
				"售后恢复期：处理完成后至少保持 30 天再评估调价",
				cycle.FormatDate(recoveryEndsAt),
			)
		}
		view, viewErr := service.buildView(subscription, service.now(), "")
		if viewErr != nil {
			block("invalid_schedule", "计费周期无效，无法确定下次续费日", "")
		} else {
			candidate.NextDueDate = view.NextDueDate
		}
		if subscription.NextPriceCents != nil {
			block(
				"scheduled",
				"已有调价安排，请先取消原安排后再重新评估",
				subscription.NextPriceEffectiveDueDate,
			)
		}
		if history.LastIncreaseDate != "" {
			lastIncreaseAt, parseErr := time.ParseInLocation("2006-01-02", history.LastIncreaseDate, cycle.Location)
			if parseErr == nil {
				cooldownEndsAt := lastIncreaseAt.AddDate(0, 0, repricingCooldownDays)
				if cycle.StartOfDay(service.now()).Before(cooldownEndsAt) {
					block(
						"cooldown",
						"调价冷静期：上次涨价后至少保持 6 个月再评估",
						cycle.FormatDate(cooldownEndsAt),
					)
				}
			}
		}
		if history.PaidPeriodCount < minimumRepricingPaidPeriods ||
			candidate.RelationshipDays < minimumRepricingRelationshipDays {
			block(
				"protection",
				fmt.Sprintf(
					"新用户保护期：至少保持 %d 天并完成 %d 个付费周期（当前 %d 天 / %d 期）",
					minimumRepricingRelationshipDays,
					minimumRepricingPaidPeriods,
					candidate.RelationshipDays,
					history.PaidPeriodCount,
				),
				earliestRepricingReviewDate(
					subscription,
					history.PaidPeriodCount,
					service.now(),
					minimumRepricingRelationshipDays,
					minimumRepricingPaidPeriods,
				),
			)
		}
		if latestExemption.ID > 0 && latestExemption.ReviewAfter > cycle.FormatDate(service.now()) {
			candidate.ExemptionReviewDate = latestExemption.ReviewAfter
			block(
				"exempted",
				fmt.Sprintf("管理员已豁免本轮调价，将于 %s 重新评估", latestExemption.ReviewAfter),
				latestExemption.ReviewAfter,
			)
		}
		if snapshot != nil && snapshot.SampleCount >= 3 {
			candidate.GapToMarketMedianCents = snapshot.MedianPriceCents - subscription.PricePerPersonCents
			marketTarget := attractiveRenewalPriceCents(snapshot.MedianPriceCents)
			financiallyHealthyTarget := maxInt64(marketTarget, minimumHealthyPriceCents)
			candidate.SuggestedPriceCents = minInt64(financiallyHealthyTarget, candidate.MaxIncreasePriceCents)
			if candidate.SuggestedPriceCents < subscription.PricePerPersonCents {
				candidate.SuggestedPriceCents = subscription.PricePerPersonCents
			}
			switch {
			case subscription.PricePerPersonCents < snapshot.LowPriceCents:
				candidate.MarketPosition = "below_low"
			case subscription.PricePerPersonCents < snapshot.MedianPriceCents:
				candidate.MarketPosition = "below_median"
			case subscription.PricePerPersonCents > snapshot.HighPriceCents:
				candidate.MarketPosition = "above_high"
			default:
				candidate.MarketPosition = "market_range"
			}
		}
		factorNumerator, factorDenominator := monthlyCycleFactor(subscription, service.now())
		candidate.MonthlyRevenueCents = subscription.PricePerPersonCents
		if factorDenominator > 0 {
			candidate.MonthlyRevenueCents =
				subscription.PricePerPersonCents * factorNumerator / factorDenominator
			if candidate.SuggestedPriceCents > subscription.PricePerPersonCents {
				candidate.SuggestedMonthlyUplift =
					(candidate.SuggestedPriceCents - subscription.PricePerPersonCents) * factorNumerator / factorDenominator
			}
			if subscription.NextPriceCents != nil && *subscription.NextPriceCents > subscription.PricePerPersonCents {
				candidate.ScheduledMonthlyUplift =
					(*subscription.NextPriceCents - subscription.PricePerPersonCents) * factorNumerator / factorDenominator
			}
		}
		candidates = append(candidates, candidate)
	}
	finalizePricingCandidates(candidates, subscriptionsByID, service.now())
	sort.SliceStable(candidates, func(left int, right int) bool {
		if candidates[left].Recommended != candidates[right].Recommended {
			return candidates[left].Recommended
		}
		if candidates[left].CurrentPriceCents != candidates[right].CurrentPriceCents {
			return candidates[left].CurrentPriceCents < candidates[right].CurrentPriceCents
		}
		return candidates[left].SubscriptionID < candidates[right].SubscriptionID
	})
	return candidates, nil
}

// assignCustomerTiers first merges subscriptions that belong to the same
// customer. Matching a non-empty email or WeChat identity is enough, and the
// union is transitive so a customer cannot be split across operating tiers.
// Tiers then use total monthly customer value rather than treating every seat
// as an unrelated order. Two-seat customers are never placed below mainstay,
// while customers with at least three seats are always core.
func assignCustomerTiers(candidates []PricingCandidate) {
	if len(candidates) == 0 {
		return
	}

	parents := make([]int, len(candidates))
	for index := range parents {
		parents[index] = index
	}
	var find func(int) int
	find = func(index int) int {
		if parents[index] != index {
			parents[index] = find(parents[index])
		}
		return parents[index]
	}
	union := func(left int, right int) {
		leftRoot := find(left)
		rightRoot := find(right)
		if leftRoot != rightRoot {
			parents[rightRoot] = leftRoot
		}
	}

	byEmail := make(map[string]int)
	byWechat := make(map[string]int)
	for index, candidate := range candidates {
		if email := normalizeCustomerIdentity(candidate.CustomerEmail); email != "" {
			if previous, exists := byEmail[email]; exists {
				union(index, previous)
			} else {
				byEmail[email] = index
			}
		}
		if wechat := normalizeCustomerIdentity(candidate.CustomerWechat); wechat != "" {
			if previous, exists := byWechat[wechat]; exists {
				union(index, previous)
			} else {
				byWechat[wechat] = index
			}
		}
	}

	groups := make(map[int][]int)
	for index := range candidates {
		root := find(index)
		groups[root] = append(groups[root], index)
	}
	groupValues := make(map[int]int64, len(groups))
	values := make([]int64, 0, len(groups))
	for root, memberIndexes := range groups {
		var monthlyRevenueCents int64
		groupID := int64(0)
		for _, index := range memberIndexes {
			candidateValue := candidates[index].MonthlyRevenueCents
			if candidateValue <= 0 {
				candidateValue = candidates[index].CurrentPriceCents
			}
			monthlyRevenueCents += candidateValue
			if subscriptionID := candidates[index].SubscriptionID; subscriptionID > 0 &&
				(groupID == 0 || subscriptionID < groupID) {
				groupID = subscriptionID
			}
		}
		if groupID == 0 {
			groupID = int64(root + 1)
		}
		groupValues[root] = monthlyRevenueCents
		values = append(values, monthlyRevenueCents)
		for _, index := range memberIndexes {
			candidates[index].CustomerGroupID = groupID
			candidates[index].CustomerGroupSize = len(memberIndexes)
			candidates[index].CustomerGroupMonthlyRevenueCents = monthlyRevenueCents
		}
	}

	sort.Slice(values, func(left int, right int) bool {
		return values[left] < values[right]
	})
	lowerThird := values[0]
	upperThird := values[len(values)-1]
	uniformValues := lowerThird == upperThird
	if !uniformValues {
		lowerThird = percentileCents(values, 1.0/3.0)
		upperThird = percentileCents(values, 2.0/3.0)
	}
	for root, memberIndexes := range groups {
		tier := "mainstay"
		if !uniformValues {
			switch value := groupValues[root]; {
			case value >= upperThird:
				tier = "core"
			case value < lowerThird:
				tier = "optimize"
			}
		}
		switch {
		case len(memberIndexes) >= 3:
			tier = "core"
		case len(memberIndexes) == 2 && tier == "optimize":
			tier = "mainstay"
		}
		for _, index := range memberIndexes {
			candidates[index].CustomerTier = tier
		}
	}
}

func normalizeCustomerIdentity(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "-", "--", "无", "暂无", "未知", "未填写", "none", "null", "n/a":
		return ""
	default:
		return normalized
	}
}

func finalizePricingCandidates(
	candidates []PricingCandidate,
	subscriptionsByID map[int64]model.Subscription,
	now time.Time,
) {
	assignCustomerTiers(candidates)
	for index := range candidates {
		candidate := &candidates[index]
		if candidate.BlockedCode == "protection" &&
			candidate.CustomerTier == "optimize" &&
			candidate.CustomerGroupSize == 1 {
			candidate.ExpeditedReview = true
			if candidate.RelationshipDays >= expeditedRepricingRelationshipDays &&
				candidate.PaidPeriodCount >= expeditedRepricingPaidPeriods {
				candidate.Eligible = true
				candidate.BlockedCode = "eligible"
				candidate.BlockedReason = ""
				candidate.NextReviewDate = ""
			} else if subscription, exists := subscriptionsByID[candidate.SubscriptionID]; exists {
				candidate.BlockedReason = fmt.Sprintf(
					"低价单席客户观察期：至少保持 %d 天并完成 %d 个付费周期（当前 %d 天 / %d 期）",
					expeditedRepricingRelationshipDays,
					expeditedRepricingPaidPeriods,
					candidate.RelationshipDays,
					candidate.PaidPeriodCount,
				)
				candidate.NextReviewDate = earliestRepricingReviewDate(
					subscription,
					candidate.PaidPeriodCount,
					now,
					expeditedRepricingRelationshipDays,
					expeditedRepricingPaidPeriods,
				)
			}
		}
		candidate.Recommended = candidate.Eligible &&
			candidate.NextPriceCents == nil &&
			candidate.MarketPosition == "below_low" &&
			candidate.SuggestedPriceCents > candidate.CurrentPriceCents
		populateRepricingInsights(candidate)
	}
}

func populateRepricingInsights(candidate *PricingCandidate) {
	switch {
	case candidate.RelationshipDays < 30 || candidate.PaidPeriodCount == 0:
		candidate.RelationshipStage = "new"
	case candidate.RelationshipDays < minimumRepricingRelationshipDays || candidate.PaidPeriodCount < minimumRepricingPaidPeriods:
		candidate.RelationshipStage = "developing"
	case candidate.RelationshipDays >= repricingCooldownDays && candidate.PaidPeriodCount >= 6:
		candidate.RelationshipStage = "loyal"
	default:
		candidate.RelationshipStage = "established"
	}

	switch candidate.BlockedCode {
	case "account_banned", "after_sales", "after_sales_recovery", "invalid_schedule", "protection", "cooldown":
		candidate.AdjustmentRisk = "high"
	default:
		if candidate.RelationshipDays >= 90 && candidate.PaidPeriodCount >= minimumRepricingPaidPeriods {
			candidate.AdjustmentRisk = "low"
		} else {
			candidate.AdjustmentRisk = "medium"
		}
	}
	if candidate.CustomerGroupSize > 1 && candidate.AdjustmentRisk == "low" {
		candidate.AdjustmentRisk = "medium"
	}

	marketMedianCents := candidate.CurrentPriceCents + candidate.GapToMarketMedianCents
	if marketMedianCents > 0 && candidate.MarketPosition != "unavailable" {
		candidate.PriceGapPercent = int(math.Round(
			float64(candidate.GapToMarketMedianCents) * 100 / float64(marketMedianCents),
		))
	}
	if candidate.CurrentPriceCents > 0 && candidate.SuggestedPriceCents > candidate.CurrentPriceCents {
		candidate.SuggestedIncreasePct = int(math.Round(
			float64(candidate.SuggestedPriceCents-candidate.CurrentPriceCents) * 100 /
				float64(candidate.CurrentPriceCents),
		))
	}

	candidate.AnalysisCodes = []string{
		"relationship_" + candidate.RelationshipStage,
		"market_" + candidate.MarketPosition,
	}
	if candidate.CustomerGroupSize > 1 {
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "multi_seat_customer")
	}
	switch candidate.BlockedCode {
	case "protection":
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "protect_reference_price")
	case "cooldown":
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "avoid_repeat_increase")
	case "after_sales", "after_sales_recovery":
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "repair_service_trust")
	case "scheduled":
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "change_already_scheduled")
	case "exempted":
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "current_round_exempted")
	case "eligible":
		if candidate.ExpeditedReview {
			candidate.AnalysisCodes = append(candidate.AnalysisCodes, "expedited_low_price_review")
		} else {
			candidate.AnalysisCodes = append(candidate.AnalysisCodes, "relationship_threshold_met")
		}
	}
	if candidate.SuggestedPriceCents > candidate.CurrentPriceCents {
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "gradual_increase_cap")
	}
	if candidate.ExemptionCount > 1 {
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "repeated_exemption_cost")
	}
	if candidate.PaidPeriodsAfterIncrease > 0 {
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "retained_after_increase")
	}

	marketScore := 0
	switch candidate.MarketPosition {
	case "below_low":
		marketScore = 35
	case "below_median":
		marketScore = 22
	case "market_range":
		marketScore = 8
	}
	tenureScore := minInt(candidate.RelationshipDays, repricingCooldownDays) * 25 / repricingCooldownDays
	paidScore := minInt(candidate.PaidPeriodCount, 6) * 20 / 6
	statusScore := 0
	if candidate.Eligible {
		statusScore = 20
	} else if candidate.BlockedCode == "scheduled" {
		statusScore = 15
	}
	candidate.ReadinessScore = marketScore + tenureScore + paidScore + statusScore
	if candidate.MarketPosition == "unavailable" {
		candidate.ReadinessScore = minInt(candidate.ReadinessScore, 40)
	}
	switch candidate.AdjustmentRisk {
	case "high":
		candidate.ReadinessScore = minInt(candidate.ReadinessScore, 39)
	case "medium":
		candidate.ReadinessScore = minInt(candidate.ReadinessScore, 74)
	}
	populatePricingDecisionScores(candidate)
}

// populatePricingDecisionScores keeps three concepts separate. Loyalty is
// based only on observed customer behavior. Relationship asset measures the
// operator's accumulated price stability and exemptions. Price pressure is the
// opportunity cost of keeping an occupied seat below its viable market price.
// This separation prevents an administrative concession from being mistaken
// for proof that the customer will renew.
func populatePricingDecisionScores(candidate *PricingCandidate) {
	tenureScore := minInt(candidate.RelationshipDays, 365) * 25 / 365
	paidScore := minInt(candidate.PaidPeriodCount, 12) * 30 / 12
	multiSeatScore := 0
	switch {
	case candidate.CustomerGroupSize >= 3:
		multiSeatScore = 15
	case candidate.CustomerGroupSize == 2:
		multiSeatScore = 10
	}
	postIncreaseScore := minInt(candidate.PaidPeriodsAfterIncrease, 3) * 5
	serviceHealthScore := maxInt(15-minInt(candidate.AfterSalesCaseCount, 3)*5, 0)
	candidate.LoyaltyScore = minInt(
		tenureScore+paidScore+multiSeatScore+postIncreaseScore+serviceHealthScore,
		100,
	)

	priceStabilityScore := minInt(candidate.PriceStableDays, 365) * 60 / 365
	concessionScore := minInt(candidate.ExemptionCount, 4) * 10
	candidate.RelationshipAssetScore = minInt(priceStabilityScore+concessionScore, 100)

	marketPressureScore := 0
	switch candidate.MarketPosition {
	case "below_low":
		marketPressureScore = 35
	case "below_median":
		marketPressureScore = 20
	}
	gapPressureScore := minInt(maxInt(candidate.PriceGapPercent, 0), 25)
	upliftPressureScore := minInt(maxInt(candidate.SuggestedIncreasePct, 0)*3, 24)
	deferralPressureScore := minInt(candidate.ExemptionCount, 3) * 8
	occupancyPressureScore := 0
	if candidate.CustomerTier == "optimize" && candidate.CustomerGroupSize <= 1 {
		occupancyPressureScore = 10
	}
	candidate.PricePressureScore = minInt(
		marketPressureScore+gapPressureScore+upliftPressureScore+
			deferralPressureScore+occupancyPressureScore,
		100,
	)
}

type pricingBillHistory struct {
	PaidPeriodCount          int
	LastIncreaseDate         string
	PaidPeriodsAfterIncrease int
}

func summarizePricingBillHistories(
	bills []model.Bill,
	refundedBillIDs map[int64]struct{},
) map[int64]pricingBillHistory {
	grouped := make(map[int64][]model.Bill)
	for _, bill := range bills {
		if _, refunded := refundedBillIDs[bill.ID]; refunded {
			continue
		}
		grouped[bill.SubscriptionID] = append(grouped[bill.SubscriptionID], bill)
	}
	histories := make(map[int64]pricingBillHistory, len(grouped))
	for subscriptionID, subscriptionBills := range grouped {
		sort.SliceStable(subscriptionBills, func(left int, right int) bool {
			if subscriptionBills[left].DueDate != subscriptionBills[right].DueDate {
				return subscriptionBills[left].DueDate < subscriptionBills[right].DueDate
			}
			return subscriptionBills[left].PaidAt.Before(subscriptionBills[right].PaidAt)
		})
		history := pricingBillHistory{PaidPeriodCount: len(subscriptionBills)}
		lastIncreaseIndex := -1
		for index := 1; index < len(subscriptionBills); index++ {
			if subscriptionBills[index].AmountCents > subscriptionBills[index-1].AmountCents {
				history.LastIncreaseDate = subscriptionBills[index].DueDate
				lastIncreaseIndex = index
			}
		}
		if lastIncreaseIndex >= 0 {
			history.PaidPeriodsAfterIncrease = len(subscriptionBills) - lastIncreaseIndex - 1
		}
		histories[subscriptionID] = history
	}
	return histories
}

func subscriptionRelationshipDays(subscription model.Subscription, now time.Time) int {
	startedAt, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(subscription.BoardedAt), cycle.Location)
	if err != nil {
		if subscription.CreatedAt.IsZero() {
			return 0
		}
		startedAt = subscription.CreatedAt.In(cycle.Location)
	}
	days := int(cycle.StartOfDay(now).Sub(cycle.StartOfDay(startedAt)).Hours() / 24)
	return maxInt(days, 0)
}

func earliestRepricingReviewDate(
	subscription model.Subscription,
	paidPeriodCount int,
	now time.Time,
	minimumRelationshipDays int,
	minimumPaidPeriods int,
) string {
	startedAt, err := time.ParseInLocation(
		"2006-01-02",
		strings.TrimSpace(subscription.BoardedAt),
		cycle.Location,
	)
	if err != nil {
		if subscription.CreatedAt.IsZero() {
			return ""
		}
		startedAt = subscription.CreatedAt.In(cycle.Location)
	}
	reviewAt := cycle.StartOfDay(startedAt).AddDate(0, 0, minimumRelationshipDays)
	remainingPeriods := minimumPaidPeriods - paidPeriodCount
	if remainingPeriods > 0 {
		schedule, parseErr := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
		if parseErr != nil {
			return ""
		}
		dueDates := schedule.NextDueTimes(cycle.StartOfDay(now), remainingPeriods)
		if len(dueDates) < remainingPeriods {
			return ""
		}
		paymentReviewAt := cycle.StartOfDay(dueDates[len(dueDates)-1])
		if paymentReviewAt.After(reviewAt) {
			reviewAt = paymentReviewAt
		}
	}
	return cycle.FormatDate(reviewAt)
}

func buildRepricingAnalysis(candidates []PricingCandidate, now time.Time) RepricingAnalysis {
	windowOrder := []string{"ready", "next_30", "next_60", "later", "on_hold"}
	windowIndexes := make(map[string]int, len(windowOrder))
	analysis := RepricingAnalysis{
		RelationshipSegments: []RepricingSegment{
			{Key: "new"},
			{Key: "developing"},
			{Key: "established"},
			{Key: "loyal"},
		},
		RiskSegments: []RepricingSegment{
			{Key: "low"},
			{Key: "medium"},
			{Key: "high"},
		},
		PriceSegments: []RepricingSegment{
			{Key: "below_low"},
			{Key: "below_median"},
			{Key: "market_range"},
			{Key: "above_high"},
			{Key: "unavailable"},
		},
		CustomerTiers: []CustomerTierSummary{
			{Key: "core"},
			{Key: "mainstay"},
			{Key: "optimize"},
		},
	}
	relationshipIndexes := segmentIndexes(analysis.RelationshipSegments)
	riskIndexes := segmentIndexes(analysis.RiskSegments)
	priceIndexes := segmentIndexes(analysis.PriceSegments)
	tierIndexes := make(map[string]int, len(analysis.CustomerTiers))
	for index, tier := range analysis.CustomerTiers {
		tierIndexes[tier.Key] = index
	}
	tierPriceTotals := make([]int64, len(analysis.CustomerTiers))
	tierCustomerGroups := make([]map[int64]struct{}, len(analysis.CustomerTiers))
	for index := range tierCustomerGroups {
		tierCustomerGroups[index] = make(map[int64]struct{})
	}
	allCustomerGroups := make(map[int64]struct{})
	var totalMonthlyRevenueCents int64
	for _, key := range windowOrder {
		windowIndexes[key] = len(analysis.Windows)
		analysis.Windows = append(analysis.Windows, RepricingWindow{Key: key})
	}
	today := cycle.StartOfDay(now)
	totalRelationshipDays := 0
	totalPaidPeriods := 0
	totalLoyaltyScore := 0
	totalRelationshipAssetScore := 0
	totalPricePressureScore := 0
	for candidateIndex, candidate := range candidates {
		customerGroupID := candidate.CustomerGroupID
		if customerGroupID <= 0 {
			customerGroupID = -int64(candidateIndex + 1)
		}
		allCustomerGroups[customerGroupID] = struct{}{}
		analysis.TotalCount++
		totalRelationshipDays += candidate.RelationshipDays
		totalPaidPeriods += candidate.PaidPeriodCount
		totalLoyaltyScore += candidate.LoyaltyScore
		totalRelationshipAssetScore += candidate.RelationshipAssetScore
		totalPricePressureScore += candidate.PricePressureScore
		if candidate.BlockedCode == "exempted" {
			analysis.ActiveExemptionCount++
		}
		incrementSegment(analysis.RelationshipSegments, relationshipIndexes, candidate.RelationshipStage)
		incrementSegment(analysis.RiskSegments, riskIndexes, candidate.AdjustmentRisk)
		incrementSegment(analysis.PriceSegments, priceIndexes, candidate.MarketPosition)
		if tierIndex, exists := tierIndexes[candidate.CustomerTier]; exists {
			tier := &analysis.CustomerTiers[tierIndex]
			tier.Count++
			tierCustomerGroups[tierIndex][customerGroupID] = struct{}{}
			tier.MonthlyRevenueCents += candidate.MonthlyRevenueCents
			tierPriceTotals[tierIndex] += candidate.CurrentPriceCents
			totalMonthlyRevenueCents += candidate.MonthlyRevenueCents
			if tier.LowestPriceCents == 0 || candidate.CurrentPriceCents < tier.LowestPriceCents {
				tier.LowestPriceCents = candidate.CurrentPriceCents
			}
			if candidate.CurrentPriceCents > tier.HighestPriceCents {
				tier.HighestPriceCents = candidate.CurrentPriceCents
			}
			if candidate.Recommended {
				tier.RecommendedCount++
			}
			if candidate.NextPriceCents != nil {
				tier.ScheduledCount++
			}
		}
		if candidate.Eligible {
			analysis.EligibleCount++
		}
		if candidate.MarketPosition == "below_low" || candidate.MarketPosition == "below_median" {
			analysis.BelowMarketCount++
		}
		if candidate.BlockedCode == "protection" ||
			candidate.BlockedCode == "cooldown" ||
			candidate.BlockedCode == "after_sales_recovery" ||
			candidate.BlockedCode == "exempted" {
			analysis.ProtectedCount++
		}
		if candidate.NextPriceCents != nil {
			analysis.ScheduledCount++
			analysis.ScheduledMonthlyUpliftCents += candidate.ScheduledMonthlyUplift
			continue
		}
		if candidate.Recommended {
			analysis.RecommendedCount++
			analysis.EstimatedMonthlyUpliftCents += candidate.SuggestedMonthlyUplift
			analysis.PipelineMonthlyUpliftCents += candidate.SuggestedMonthlyUplift
			readyWindow := &analysis.Windows[windowIndexes["ready"]]
			readyWindow.Count++
			readyWindow.MonthlyUpliftCents += candidate.SuggestedMonthlyUplift
			continue
		}
		// Only prices below the lower market reference enter the future action
		// pipeline. A merely below-median user is useful context, but is not an
		// automatic repricing candidate once their protection period ends.
		underpriced := candidate.MarketPosition == "below_low"
		if !underpriced || candidate.SuggestedMonthlyUplift <= 0 || candidate.Eligible {
			continue
		}
		windowKey := "on_hold"
		if reviewAt, err := time.ParseInLocation("2006-01-02", candidate.NextReviewDate, cycle.Location); err == nil {
			daysUntilReview := int(math.Ceil(cycle.StartOfDay(reviewAt).Sub(today).Hours() / 24))
			switch {
			case daysUntilReview <= 30:
				windowKey = "next_30"
			case daysUntilReview <= 60:
				windowKey = "next_60"
			default:
				windowKey = "later"
			}
			analysis.PipelineMonthlyUpliftCents += candidate.SuggestedMonthlyUplift
		}
		window := &analysis.Windows[windowIndexes[windowKey]]
		window.Count++
		window.MonthlyUpliftCents += candidate.SuggestedMonthlyUplift
	}
	analysis.CustomerCount = len(allCustomerGroups)
	if analysis.TotalCount > 0 {
		analysis.AverageRelationshipDays = int(math.Round(
			float64(totalRelationshipDays) / float64(analysis.TotalCount),
		))
		analysis.AveragePaidPeriods = math.Round(
			float64(totalPaidPeriods)/float64(analysis.TotalCount)*10,
		) / 10
		analysis.AverageLoyaltyScore = int(math.Round(
			float64(totalLoyaltyScore) / float64(analysis.TotalCount),
		))
		analysis.AverageRelationshipAsset = int(math.Round(
			float64(totalRelationshipAssetScore) / float64(analysis.TotalCount),
		))
		analysis.AveragePricePressure = int(math.Round(
			float64(totalPricePressureScore) / float64(analysis.TotalCount),
		))
	}
	for index := range analysis.CustomerTiers {
		tier := &analysis.CustomerTiers[index]
		tier.CustomerCount = len(tierCustomerGroups[index])
		if tier.Count > 0 {
			tier.AveragePriceCents = int64(math.Round(
				float64(tierPriceTotals[index]) / float64(tier.Count),
			))
		}
	}
	allocateCustomerTierRevenueShares(analysis.CustomerTiers, totalMonthlyRevenueCents)
	return analysis
}

// allocateCustomerTierRevenueShares uses the largest-remainder method so the
// displayed integer percentages always add up to exactly 100 without changing
// the underlying cent amounts.
func allocateCustomerTierRevenueShares(tiers []CustomerTierSummary, totalRevenueCents int64) {
	if totalRevenueCents <= 0 {
		return
	}
	type tierRemainder struct {
		index     int
		remainder int64
	}
	remainders := make([]tierRemainder, 0, len(tiers))
	allocated := 0
	for index := range tiers {
		numerator := tiers[index].MonthlyRevenueCents * 100
		tiers[index].RevenueSharePercent = int(numerator / totalRevenueCents)
		allocated += tiers[index].RevenueSharePercent
		if tiers[index].MonthlyRevenueCents > 0 {
			remainders = append(remainders, tierRemainder{
				index:     index,
				remainder: numerator % totalRevenueCents,
			})
		}
	}
	sort.SliceStable(remainders, func(left int, right int) bool {
		return remainders[left].remainder > remainders[right].remainder
	})
	for index := 0; allocated < 100 && index < len(remainders); index++ {
		tiers[remainders[index].index].RevenueSharePercent++
		allocated++
	}
}

func segmentIndexes(segments []RepricingSegment) map[string]int {
	indexes := make(map[string]int, len(segments))
	for index, segment := range segments {
		indexes[segment.Key] = index
	}
	return indexes
}

func incrementSegment(segments []RepricingSegment, indexes map[string]int, key string) {
	if index, exists := indexes[key]; exists {
		segments[index].Count++
	}
}

func attractiveRenewalPriceCents(marketMedianCents int64) int64 {
	if marketMedianCents <= 0 {
		return 0
	}
	return marketMedianCents * (100 - marketRenewalDiscountPercent) / 100
}

func newSaleDiscountPercent(utilizationPercent int) int64 {
	switch {
	case utilizationPercent < 70:
		return newSaleFillDiscountPercent
	case utilizationPercent < 85:
		return newSaleBalancedDiscountPercent
	default:
		return newSaleScarceDiscountPercent
	}
}

// attractiveNewSaleRange keeps a deliberate acquisition advantage against
// both the lower and typical market references. The advantage narrows as
// utilization rises, while the rounded healthy floor prevents discounts from
// turning a new order unprofitable.
func attractiveNewSaleRange(
	marketLowCents int64,
	marketMedianCents int64,
	minimumHealthyPriceCents int64,
	discountPercent int64,
) (int64, int64, bool) {
	healthyFloor := roundPriceUpToYuan(minimumHealthyPriceCents)
	discountedLow := roundPriceDownToYuan(marketLowCents * (100 - discountPercent) / 100)
	discountedHigh := roundPriceDownToYuan(marketMedianCents * (100 - discountPercent) / 100)
	if discountedHigh < discountedLow {
		discountedHigh = discountedLow
	}

	suggestedLow := maxInt64(healthyFloor, discountedLow)
	suggestedHigh := maxInt64(suggestedLow, discountedHigh)
	if suggestedHigh > healthyFloor && suggestedHigh-suggestedLow < minimumNewSaleRangeCents {
		suggestedLow = maxInt64(healthyFloor, suggestedHigh-minimumNewSaleRangeCents)
	}
	return suggestedLow, suggestedHigh, healthyFloor <= discountedHigh
}

func roundPriceDownToYuan(cents int64) int64 {
	if cents <= 0 {
		return 0
	}
	return cents / 100 * 100
}

func roundPriceUpToYuan(cents int64) int64 {
	if cents <= 0 {
		return 0
	}
	return divideRoundUp(cents, 100) * 100
}

func maximumGradualPriceCents(currentPriceCents int64) int64 {
	if currentPriceCents <= 0 {
		return 0
	}
	percentageIncrease := currentPriceCents * maximumRepricingIncreasePercent / 100
	return currentPriceCents + minInt64(percentageIncrease, maximumRepricingIncreaseCents)
}

// ScheduleBulkNextPrice atomically arranges the same future Team renewal price
// for selected established customers. Retention safeguards are rechecked here
// so stale UI state cannot bypass the protection period or gradual-increase cap.
func (service *SubscriptionService) ScheduleBulkNextPrice(input BulkNextPriceInput) (int, error) {
	if len(input.SubscriptionIDs) == 0 {
		return 0, fmt.Errorf("请至少选择一位 Team 用户")
	}
	if len(input.SubscriptionIDs) > maximumBulkPricingSelection {
		return 0, fmt.Errorf("单次最多调整 200 位 Team 用户")
	}
	nextPriceCents, err := cycle.ParseYuanToCents(strings.TrimSpace(input.NextPriceYuan))
	if err != nil || nextPriceCents <= 0 {
		return 0, fmt.Errorf("请填写有效的下周期价格")
	}
	candidates, err := service.buildPricingCandidates(nil, 0)
	if err != nil {
		return 0, err
	}
	candidateByID := make(map[int64]PricingCandidate, len(candidates))
	for _, candidate := range candidates {
		candidateByID[candidate.SubscriptionID] = candidate
	}

	seen := make(map[int64]struct{}, len(input.SubscriptionIDs))
	updates := make([]model.Subscription, 0, len(input.SubscriptionIDs))
	for _, subscriptionID := range input.SubscriptionIDs {
		if subscriptionID <= 0 {
			return 0, fmt.Errorf("包含无效的订阅 ID")
		}
		if _, duplicate := seen[subscriptionID]; duplicate {
			continue
		}
		seen[subscriptionID] = struct{}{}

		previous, getErr := service.Store.GetSubscription(subscriptionID)
		if getErr != nil {
			if getErr == sql.ErrNoRows {
				return 0, fmt.Errorf("所选 Team 用户不存在或已下车")
			}
			return 0, getErr
		}
		candidate, candidateExists := candidateByID[subscriptionID]
		if previous.BusinessType != model.SubscriptionBusinessTeam || previous.SeatID <= 0 || previous.IsResale || !candidateExists {
			return 0, fmt.Errorf("%s 不是可批量调价的 Team 席位", previous.Name)
		}
		if !candidate.Eligible {
			return 0, fmt.Errorf("%s 暂不适合涨价：%s", previous.Name, candidate.BlockedReason)
		}
		if err := service.ensureNoPendingAfterSales(subscriptionID, "安排调价"); err != nil {
			return 0, fmt.Errorf("%s：%w", previous.Name, err)
		}
		if nextPriceCents <= previous.PricePerPersonCents {
			return 0, fmt.Errorf("%s 的新价格须高于当前价格 ¥%s", previous.Name, cycle.FormatCents(previous.PricePerPersonCents))
		}
		if nextPriceCents > candidate.MaxIncreasePriceCents {
			return 0, fmt.Errorf(
				"%s 单次涨幅过大，建议本次不超过 ¥%s",
				previous.Name,
				cycle.FormatCents(candidate.MaxIncreasePriceCents),
			)
		}
		updated := previous
		updated.NextPriceCents = &nextPriceCents
		if err := service.configureNextPrice(previous, &updated, false); err != nil {
			return 0, fmt.Errorf("%s：%w", previous.Name, err)
		}
		if err := service.ensureMinimumPriceIncreaseNotice(&updated); err != nil {
			return 0, fmt.Errorf("%s：%w", previous.Name, err)
		}
		updates = append(updates, updated)
	}
	if len(updates) == 0 {
		return 0, fmt.Errorf("没有可调价的 Team 用户")
	}
	if err := service.Store.UpdateSubscriptionNextPrices(updates, cycle.FormatDate(service.now())); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("所选用户状态已变化，请刷新后重试")
		}
		return 0, err
	}
	return len(updates), nil
}

// ExemptBulkPricing records a bounded decision to keep the selected system
// candidates at their current price. The history contributes to relationship
// asset and also to future opportunity-cost pressure, so repeated exemptions
// never remove a customer from repricing assessment permanently.
func (service *SubscriptionService) ExemptBulkPricing(input BulkPricingExemptionInput) (int, error) {
	if len(input.SubscriptionIDs) == 0 {
		return 0, fmt.Errorf("请至少选择一位本轮建议调价的 Team 用户")
	}
	if len(input.SubscriptionIDs) > maximumBulkPricingSelection {
		return 0, fmt.Errorf("单次最多豁免 %d 位 Team 用户", maximumBulkPricingSelection)
	}
	if input.ReviewCycles < 1 || input.ReviewCycles > maximumPricingExemptionCycles {
		return 0, fmt.Errorf("复评周期必须在 1 到 %d 个账期之间", maximumPricingExemptionCycles)
	}
	reasonCode := strings.TrimSpace(input.ReasonCode)
	if !validPricingExemptionReason(reasonCode) {
		return 0, fmt.Errorf("请选择有效的豁免原因")
	}
	note := strings.TrimSpace(input.Note)
	if len([]rune(note)) > maximumPricingExemptionNoteRunes {
		return 0, fmt.Errorf("豁免备注最多 %d 个字", maximumPricingExemptionNoteRunes)
	}

	var snapshot *model.MarketPriceSnapshot
	latestSnapshot, latestErr := service.Store.LatestMarketPriceSnapshot(marketProvider, marketProduct)
	if latestErr == nil {
		snapshot = &latestSnapshot
	} else if latestErr != sql.ErrNoRows {
		return 0, latestErr
	}
	candidates, err := service.buildPricingCandidates(snapshot, 0)
	if err != nil {
		return 0, err
	}
	candidateByID := make(map[int64]PricingCandidate, len(candidates))
	for _, candidate := range candidates {
		candidateByID[candidate.SubscriptionID] = candidate
	}

	seen := make(map[int64]struct{}, len(input.SubscriptionIDs))
	exemptions := make([]model.PricingExemption, 0, len(input.SubscriptionIDs))
	marketMedianCents := int64(0)
	if snapshot != nil {
		marketMedianCents = snapshot.MedianPriceCents
	}
	for _, subscriptionID := range input.SubscriptionIDs {
		if subscriptionID <= 0 {
			return 0, fmt.Errorf("包含无效的订阅 ID")
		}
		if _, duplicate := seen[subscriptionID]; duplicate {
			continue
		}
		seen[subscriptionID] = struct{}{}

		candidate, exists := candidateByID[subscriptionID]
		if !exists || !candidate.Recommended || !candidate.Eligible {
			return 0, fmt.Errorf("所选用户已不在本轮建议调价名单，请刷新后重试")
		}
		subscription, getErr := service.Store.GetSubscription(subscriptionID)
		if getErr != nil {
			if getErr == sql.ErrNoRows {
				return 0, fmt.Errorf("所选 Team 用户不存在或已下车")
			}
			return 0, getErr
		}
		reviewAfter, reviewErr := pricingExemptionReviewDate(
			subscription,
			candidate.NextDueDate,
			input.ReviewCycles,
			service.now(),
		)
		if reviewErr != nil {
			return 0, fmt.Errorf("%s：无法计算下一次调价评估日期：%w", subscription.Name, reviewErr)
		}
		exemptions = append(exemptions, model.PricingExemption{
			SubscriptionID:            subscriptionID,
			ReasonCode:                reasonCode,
			Note:                      note,
			ReviewAfter:               reviewAfter,
			ReviewCycles:              input.ReviewCycles,
			PriceCentsSnapshot:        subscription.PricePerPersonCents,
			MarketMedianCentsSnapshot: marketMedianCents,
		})
	}
	if len(exemptions) == 0 {
		return 0, fmt.Errorf("没有可豁免的 Team 用户")
	}
	if err := service.Store.CreatePricingExemptions(exemptions, cycle.FormatDate(service.now())); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("所选用户状态已变化，请刷新后重新选择")
		}
		return 0, err
	}
	return len(exemptions), nil
}

func validPricingExemptionReason(reasonCode string) bool {
	switch reasonCode {
	case "loyalty_reward", "multi_seat_retention", "price_observation", "relationship_investment", "manual":
		return true
	default:
		return false
	}
}

func pricingExemptionReviewDate(
	subscription model.Subscription,
	nextDueDate string,
	reviewCycles int,
	now time.Time,
) (string, error) {
	reviewAt, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(nextDueDate), cycle.Location)
	if err != nil {
		return "", err
	}
	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		return "", err
	}
	if !cycle.StartOfDay(reviewAt).After(cycle.StartOfDay(now)) {
		reviewAt = schedule.NextDue(cycle.StartOfDay(now))
	}
	for cycleIndex := 1; cycleIndex < reviewCycles; cycleIndex++ {
		reviewAt = schedule.NextDue(reviewAt)
	}
	return cycle.FormatDate(reviewAt), nil
}

func (service *SubscriptionService) ensureMinimumPriceIncreaseNotice(subscription *model.Subscription) error {
	effectiveAt, err := time.ParseInLocation(
		"2006-01-02",
		strings.TrimSpace(subscription.NextPriceEffectiveDueDate),
		cycle.Location,
	)
	if err != nil {
		return fmt.Errorf("无法确定调价生效日")
	}
	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		return err
	}
	minimumEffectiveAt := cycle.StartOfDay(service.now()).AddDate(0, 0, minimumPriceIncreaseNoticeDays)
	for effectiveAt.Before(minimumEffectiveAt) {
		effectiveAt = schedule.NextDue(effectiveAt)
	}
	subscription.NextPriceEffectiveDueDate = cycle.FormatDate(effectiveAt)
	return nil
}

func monthFromDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 7 {
		return raw[:7]
	}
	return ""
}

func percentOf(value int64, target int64) int {
	if target <= 0 || value <= 0 {
		return 0
	}
	percent := value * 100 / target
	if percent > 100 {
		return 100
	}
	return int(percent)
}

func divideRoundUp(numerator int64, denominator int64) int64 {
	if numerator <= 0 || denominator <= 0 {
		return 0
	}
	return (numerator + denominator - 1) / denominator
}

func minInt64(left int64, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func maxInt64(left int64, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
