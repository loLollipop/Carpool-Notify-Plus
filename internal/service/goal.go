package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
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
	MarketMonthlyPriceCents          int64    `json:"market_monthly_price_cents"`
	NextPriceCents                   *int64   `json:"next_price_cents"`
	NextPriceEffectiveDate           string   `json:"next_price_effective_date"`
	NextDueDate                      string   `json:"next_due_date"`
	MarketPosition                   string   `json:"market_position"`
	GapToMarketMedianCents           int64    `json:"gap_to_market_median_cents"`
	SuggestedPriceCents              int64    `json:"suggested_price_cents"`
	SuggestedMonthlyPriceCents       int64    `json:"suggested_monthly_price_cents"`
	MaxIncreasePriceCents            int64    `json:"max_increase_price_cents"`
	PaidPeriodCount                  int      `json:"paid_period_count"`
	LastPaidDate                     string   `json:"last_paid_date"`
	RelationshipDays                 int      `json:"relationship_days"`
	LastPriceIncreaseDate            string   `json:"last_price_increase_date"`
	BlockedCode                      string   `json:"blocked_code"`
	NextReviewDate                   string   `json:"next_review_date"`
	SuggestedMonthlyUplift           int64    `json:"suggested_monthly_uplift_cents"`
	ScheduledMonthlyUplift           int64    `json:"scheduled_monthly_uplift_cents"`
	MonthlyRevenueCents              int64    `json:"monthly_revenue_cents"`
	CustomerGroupID                  int64    `json:"customer_group_id"`
	CustomerGroupSize                int      `json:"customer_group_size"`
	CustomerGroupCurrentPriceCents   int64    `json:"customer_group_current_price_cents"`
	CustomerGroupMonthlyRevenueCents int64    `json:"customer_group_monthly_revenue_cents"`
	CustomerTier                     string   `json:"customer_tier"`
	RelationshipStage                string   `json:"relationship_stage"`
	CustomerQualityScore             int      `json:"customer_quality_score"`
	RelationshipScore                int      `json:"relationship_score"`
	LoyaltyScore                     int      `json:"loyalty_score"`
	ContactStrengthScore             int      `json:"contact_strength_score"`
	RelationshipHealthScore          int      `json:"relationship_health_score"`
	RelationshipLevel                string   `json:"relationship_level"`
	RelationshipProfileConfidence    string   `json:"relationship_profile_confidence"`
	PrimaryRelationshipTask          string   `json:"primary_relationship_task"`
	NeedsContactFollowup             bool     `json:"needs_contact_followup"`
	RelationshipSignalCodes          []string `json:"relationship_signal_codes"`
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
	RenewalCount                     int      `json:"renewal_count"`
	RenewalEvidence                  string   `json:"renewal_evidence"`
	VerifiedPriceCents               int64    `json:"verified_price_cents"`
	VerifiedMonthlyPriceCents        int64    `json:"verified_monthly_price_cents"`
	VerifiedPriceIndex               *int     `json:"verified_price_index"`
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
	AveragePricePressure        int                   `json:"average_price_pressure_score"`
	FirstCycleSubscriptionCount int                   `json:"first_cycle_subscription_count"`
	RepeatSubscriptionCount     int                   `json:"repeat_subscription_count"`
	IncreasedPriceAcceptedCount int                   `json:"increased_price_accepted_count"`
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

type ManualNextPriceItemInput struct {
	SubscriptionID int64
	NextPriceYuan  string
}

type ManualNextPricesInput struct {
	Items []ManualNextPriceItemInput
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
	Care       CustomerCareCenter    `json:"customer_care"`
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
	center.Care, err = service.buildCustomerCare(center.Candidates)
	if err != nil {
		return GoalCenter{}, err
	}
	return center, nil
}

func (service *SubscriptionService) RefreshMarketPrice() (MarketPriceView, error) {
	return service.loadMarketPrice(true)
}

// MarketRefreshInterval is the maximum intended lifetime of a cached market
// snapshot before a new network request is allowed.
func MarketRefreshInterval() time.Duration {
	return marketCacheTTL
}

// MarketRefreshCheckInterval keeps startup timing from extending a nearly
// expired snapshot by another full cache lifetime. Each check is cheap database
// work; network I/O still happens only after MarketRefreshInterval has elapsed.
func MarketRefreshCheckInterval() time.Duration {
	return time.Hour
}

// RefreshMarketPriceIfStale refreshes the market snapshot only when the shared
// cache has expired. It is intended for background checks and startup catch-up.
func (service *SubscriptionService) RefreshMarketPriceIfStale() (MarketPriceView, error) {
	return service.loadMarketPrice(false)
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
	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil {
		return nil, err
	}
	for _, benefit := range benefits {
		if index, exists := monthIndex[monthFromDate(benefit.BenefitDate)]; exists {
			months[index].CostCents += benefit.ActualCostCents
		}
	}
	operatingExpenses, err := service.Store.ListOperatingExpenses()
	if err != nil {
		return nil, err
	}
	for _, expense := range operatingExpenses {
		if index, exists := monthIndex[monthFromDate(expense.OccurredOn)]; exists {
			months[index].CostCents += expense.AmountCents
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
	operatingExpenses, err := service.Store.ListOperatingExpenses()
	if err != nil {
		return 0, 0, err
	}
	monthlyProfitCents -= operatingExpenseMonthlyRunRate(operatingExpenses, service.now())
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

// marketMonthlyCycleFactor converts a billing-period price to the monthly
// unit used by external Team-seat market snapshots. Standard subscription
// periods deliberately use customer-facing month counts: a 90-day quarterly
// bill is three months, so ¥330 is compared as exactly ¥110/month. Irregular
// schedules fall back to the annualized run-rate factor used by forecasting.
func marketMonthlyCycleFactor(subscription model.Subscription, now time.Time) (int64, int64) {
	expression := strings.ToLower(strings.TrimSpace(subscription.CronExpr))
	if strings.HasPrefix(expression, "interval:") && strings.HasSuffix(expression, "d") {
		daysText := strings.TrimSuffix(strings.TrimPrefix(expression, "interval:"), "d")
		if days, err := strconv.Atoi(daysText); err == nil && days > 0 {
			switch {
			case days == 365:
				return 1, 12
			case days%30 == 0:
				return 1, int64(days / 30)
			}
		}
	}

	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err == nil {
		dueDates := schedule.NextDueTimes(now, 2)
		if len(dueDates) == 2 {
			first := dueDates[0].In(cycle.Location)
			second := dueDates[1].In(cycle.Location)
			months := (second.Year()-first.Year())*12 + int(second.Month()-first.Month())
			if months > 0 && first.Day() == second.Day() &&
				first.Hour() == second.Hour() && first.Minute() == second.Minute() {
				return 1, int64(months)
			}
		}
	}
	return monthlyCycleFactor(subscription, now)
}

func scalePriceCents(priceCents int64, numerator int64, denominator int64) int64 {
	if priceCents <= 0 || numerator <= 0 || denominator <= 0 {
		return 0
	}
	return (priceCents*numerator + denominator/2) / denominator
}

func candidateMarketMonthlyPrice(candidate PricingCandidate) int64 {
	if candidate.MarketMonthlyPriceCents > 0 {
		return candidate.MarketMonthlyPriceCents
	}
	return candidate.CurrentPriceCents
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
	service.marketRefreshMu.Lock()
	defer service.marketRefreshMu.Unlock()

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
	seatUsed := 0
	for _, account := range accounts {
		if strings.TrimSpace(account.BannedAt) != "" {
			continue
		}
		activeAccountIDs[account.ID] = struct{}{}
		monthlyAccountCostCents += account.CostCents
		unavailable, countErr := service.Store.CountUnavailableSeatsByAccount(account.ID, service.now())
		if countErr != nil {
			return PricingRecommendation{}, countErr
		}
		seatUsed += unavailable
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
	for _, subscription := range subscriptions {
		if subscription.BusinessType != model.SubscriptionBusinessTeam || subscription.SeatID <= 0 {
			continue
		}
		if _, active := activeAccountIDs[subscription.AccountID]; !active {
			continue
		}
		if !subscription.IsResale && subscription.PricePerPersonCents > 0 {
			factorNumerator, factorDenominator := marketMonthlyCycleFactor(subscription, service.now())
			monthlyPriceCents := scalePriceCents(
				subscription.PricePerPersonCents,
				factorNumerator,
				factorDenominator,
			)
			if monthlyPriceCents <= 0 {
				monthlyPriceCents = subscription.PricePerPersonCents
			}
			teamPrices = append(teamPrices, monthlyPriceCents)
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
		marketFactorNumerator, marketFactorDenominator := marketMonthlyCycleFactor(subscription, service.now())
		marketMonthlyPriceCents := subscription.PricePerPersonCents
		maxIncreasePriceCents := maximumGradualPriceCents(subscription.PricePerPersonCents)
		if marketFactorNumerator > 0 && marketFactorDenominator > 0 {
			marketMonthlyPriceCents = scalePriceCents(
				subscription.PricePerPersonCents,
				marketFactorNumerator,
				marketFactorDenominator,
			)
			maxMonthlyPriceCents := maximumGradualPriceCents(marketMonthlyPriceCents)
			maxIncreasePriceCents = scalePriceCents(
				maxMonthlyPriceCents,
				marketFactorDenominator,
				marketFactorNumerator,
			)
			maxIncreasePriceCents = maxInt64(maxIncreasePriceCents, subscription.PricePerPersonCents)
		}
		candidate := PricingCandidate{
			SubscriptionID:          subscription.ID,
			Name:                    subscription.Name,
			CustomerEmail:           subscription.CustomerEmail,
			CustomerWechat:          subscription.CustomerWechat,
			AccountName:             subscription.AccountName,
			SeatName:                subscription.SeatName,
			CurrentPriceCents:       subscription.PricePerPersonCents,
			MarketMonthlyPriceCents: marketMonthlyPriceCents,
			NextPriceCents:          subscription.NextPriceCents,
			NextPriceEffectiveDate:  subscription.NextPriceEffectiveDueDate,
			Eligible:                true,
			BlockedCode:             "eligible",
			MarketPosition:          "unavailable",
			MaxIncreasePriceCents:   maxIncreasePriceCents,
		}
		history := billHistories[subscription.ID]
		candidate.PaidPeriodCount = history.PaidPeriodCount
		candidate.LastPaidDate = history.LastPaidDueDate
		candidate.LastPriceIncreaseDate = history.LastIncreaseDate
		candidate.PaidPeriodsAfterIncrease = history.PaidPeriodsAfterIncrease
		candidate.VerifiedPriceCents = history.LastPaidPriceCents
		candidate.VerifiedMonthlyPriceCents = scalePriceCents(
			history.LastPaidPriceCents,
			marketFactorNumerator,
			marketFactorDenominator,
		)
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
		// An active operator exemption must be evaluated before the standard
		// protection window. Optimize-tier customers can leave that window via
		// the expedited review below, but must never bypass an explicit exemption.
		if latestExemption.ID > 0 && latestExemption.ReviewAfter > cycle.FormatDate(service.now()) {
			candidate.ExemptionReviewDate = latestExemption.ReviewAfter
			block(
				"exempted",
				fmt.Sprintf("管理员已豁免本轮调价，将于 %s 重新评估", latestExemption.ReviewAfter),
				latestExemption.ReviewAfter,
			)
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
		if snapshot != nil && snapshot.SampleCount >= 3 {
			candidate.GapToMarketMedianCents = snapshot.MedianPriceCents - marketMonthlyPriceCents
			marketMonthlyTarget := attractiveRenewalPriceCents(snapshot.MedianPriceCents)
			financiallyHealthyMonthlyTarget := maxInt64(marketMonthlyTarget, minimumHealthyPriceCents)
			financiallyHealthyCycleTarget := financiallyHealthyMonthlyTarget
			if marketFactorNumerator > 0 && marketFactorDenominator > 0 {
				financiallyHealthyCycleTarget = scalePriceCents(
					financiallyHealthyMonthlyTarget,
					marketFactorDenominator,
					marketFactorNumerator,
				)
			}
			candidate.SuggestedPriceCents = minInt64(financiallyHealthyCycleTarget, candidate.MaxIncreasePriceCents)
			if candidate.SuggestedPriceCents < subscription.PricePerPersonCents {
				candidate.SuggestedPriceCents = subscription.PricePerPersonCents
			}
			candidate.SuggestedMonthlyPriceCents = scalePriceCents(
				candidate.SuggestedPriceCents,
				marketFactorNumerator,
				marketFactorDenominator,
			)
			switch {
			case marketMonthlyPriceCents < snapshot.LowPriceCents:
				candidate.MarketPosition = "below_low"
			case marketMonthlyPriceCents < snapshot.MedianPriceCents:
				candidate.MarketPosition = "below_median"
			case marketMonthlyPriceCents > snapshot.HighPriceCents:
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
		leftMonthlyPrice := candidateMarketMonthlyPrice(candidates[left])
		rightMonthlyPrice := candidateMarketMonthlyPrice(candidates[right])
		if leftMonthlyPrice != rightMonthlyPrice {
			return leftMonthlyPrice < rightMonthlyPrice
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
		var currentPriceCents int64
		groupID := int64(0)
		for _, index := range memberIndexes {
			// Customer profiles use the customer-facing monthly equivalent, not
			// the annualized 365-day run rate used by profit forecasting. This
			// keeps a ¥100 30-day bill at ¥100/month and a ¥330 quarterly bill
			// at exactly ¥110/month.
			candidateValue := candidateMarketMonthlyPrice(candidates[index])
			monthlyRevenueCents += candidateValue
			currentPriceCents += maxInt64(candidates[index].CurrentPriceCents, 0)
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
			candidates[index].CustomerGroupCurrentPriceCents = currentPriceCents
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
	populateCustomerRelationshipProfiles(candidates)
}

// populateCustomerRelationshipProfiles builds an observable-behavior index,
// not a psychological diagnosis or a renewal probability. It follows three
// operating rules: judge customers by accumulated behavior, separate the
// current relationship stage from the next management task, and lower model
// confidence when the evidence window is short. Scores are copied to every
// seat in a merged customer group so the UI can render one coherent profile.
func populateCustomerRelationshipProfiles(candidates []PricingCandidate) {
	type relationshipGroup struct {
		indexes                  []int
		tier                     string
		groupSize                int
		relationshipDays         int
		paidPeriods              int
		renewals                 int
		paidPeriodsAfterIncrease int
		afterSalesCases          int
		exemptions               int
		hasEmail                 bool
		hasWechat                bool
		serviceAtRisk            bool
	}

	groups := make(map[int64]*relationshipGroup)
	for index := range candidates {
		candidate := candidates[index]
		groupID := candidate.CustomerGroupID
		if groupID == 0 {
			groupID = candidate.SubscriptionID
		}
		if groupID == 0 {
			groupID = -int64(index + 1)
		}
		group := groups[groupID]
		if group == nil {
			group = &relationshipGroup{}
			groups[groupID] = group
		}
		group.indexes = append(group.indexes, index)
		if customerTierRank(candidate.CustomerTier) > customerTierRank(group.tier) {
			group.tier = candidate.CustomerTier
		}
		group.groupSize = maxInt(group.groupSize, candidate.CustomerGroupSize)
		group.relationshipDays = maxInt(group.relationshipDays, candidate.RelationshipDays)
		group.paidPeriods = maxInt(group.paidPeriods, candidate.PaidPeriodCount)
		group.renewals = maxInt(group.renewals, candidate.RenewalCount)
		group.paidPeriodsAfterIncrease = maxInt(
			group.paidPeriodsAfterIncrease,
			candidate.PaidPeriodsAfterIncrease,
		)
		group.afterSalesCases += candidate.AfterSalesCaseCount
		group.exemptions += candidate.ExemptionCount
		group.hasEmail = group.hasEmail || normalizeCustomerIdentity(candidate.CustomerEmail) != ""
		group.hasWechat = group.hasWechat || normalizeCustomerIdentity(candidate.CustomerWechat) != ""
		switch candidate.BlockedCode {
		case "after_sales", "after_sales_recovery":
			group.serviceAtRisk = true
		}
	}

	for _, group := range groups {
		if group.groupSize <= 0 {
			group.groupSize = len(group.indexes)
		}

		qualityScore := 40
		switch group.tier {
		case "core":
			qualityScore = 60
		case "mainstay":
			qualityScore = 48
		case "optimize":
			qualityScore = 35
		}
		qualityScore += minInt(group.paidPeriods, 5) * 4
		qualityScore += minInt(maxInt(group.groupSize-1, 0), 3) * 7
		if group.paidPeriods == 0 {
			qualityScore -= 10
		}
		qualityScore = clampRelationshipScore(qualityScore)

		relationshipScore := minInt(group.relationshipDays, 180) * 30 / 180
		relationshipScore += minInt(group.paidPeriods, 6) * 30 / 6
		if group.hasWechat {
			relationshipScore += 20
		}
		relationshipScore += minInt(maxInt(group.groupSize-1, 0), 2) * 5
		relationshipScore += minInt(group.exemptions, 2) * 5
		relationshipScore = clampRelationshipScore(relationshipScore)

		loyaltyScore := 10
		loyaltyScore += minInt(group.renewals, 4) * 12
		loyaltyScore += minInt(group.relationshipDays, 180) * 17 / 180
		loyaltyScore += minInt(maxInt(group.groupSize-1, 0), 2) * 7
		if group.hasWechat {
			loyaltyScore += 6
		}
		if group.paidPeriodsAfterIncrease > 0 {
			loyaltyScore += 10
		}
		if group.paidPeriods == 0 {
			loyaltyScore = minInt(loyaltyScore, 15)
		}
		loyaltyScore = clampRelationshipScore(loyaltyScore)

		contactScore := 0
		if group.hasEmail {
			contactScore += 20
		}
		if group.hasWechat {
			contactScore += 55
		}
		contactScore += minInt(group.renewals, 4) * 10 / 4
		contactScore += minInt(maxInt(group.groupSize-1, 0), 2) * 15 / 2
		contactScore = clampRelationshipScore(contactScore)

		healthScore := (qualityScore*25 + relationshipScore*30 + loyaltyScore*30 + contactScore*15) / 100
		// An unresolved service event affects the present relationship health and
		// trust-repair priority. It must not rewrite the customer's historical
		// loyalty or customer-quality evidence as if the customer caused the issue.
		if group.serviceAtRisk {
			healthScore -= 12
		}
		healthScore = clampRelationshipScore(healthScore)
		level := "fragile"
		switch {
		case healthScore >= 80:
			level = "trusted"
		case healthScore >= 60:
			level = "stable"
		case healthScore >= 45:
			level = "developing"
		}
		confidence := "low"
		switch {
		case group.paidPeriods >= 4 && group.relationshipDays >= 120:
			confidence = "high"
		case group.paidPeriods >= 2 && group.relationshipDays >= 45:
			confidence = "medium"
		}

		task := "strengthen_habit"
		switch {
		case group.serviceAtRisk:
			task = "repair_trust"
		case !group.hasWechat:
			task = "complete_contact"
		case group.renewals == 0:
			task = "observe_first_renewal"
		case group.tier == "core" || group.groupSize > 1:
			task = "protect_key_account"
		case level == "stable" || level == "trusted":
			task = "maintain_low_frequency"
		}

		signals := make([]string, 0, 8)
		if !group.hasWechat {
			signals = append(signals, "wechat_missing")
		}
		if group.groupSize > 1 {
			signals = append(signals, "multi_seat")
		}
		if group.renewals == 0 {
			signals = append(signals, "first_cycle")
		} else if group.renewals >= 2 {
			signals = append(signals, "repeat_renewal")
		}
		if group.relationshipDays >= 180 {
			signals = append(signals, "long_relationship")
		}
		if group.paidPeriodsAfterIncrease > 0 {
			signals = append(signals, "price_change_retained")
		}
		if group.serviceAtRisk || group.afterSalesCases > 0 {
			signals = append(signals, "service_history")
		}
		if group.exemptions > 0 {
			signals = append(signals, "relationship_investment")
		}

		for _, index := range group.indexes {
			candidate := &candidates[index]
			candidate.CustomerQualityScore = qualityScore
			candidate.RelationshipScore = relationshipScore
			candidate.LoyaltyScore = loyaltyScore
			candidate.ContactStrengthScore = contactScore
			candidate.RelationshipHealthScore = healthScore
			candidate.RelationshipLevel = level
			candidate.RelationshipProfileConfidence = confidence
			candidate.PrimaryRelationshipTask = task
			candidate.NeedsContactFollowup = !group.hasWechat
			candidate.RelationshipSignalCodes = append([]string(nil), signals...)
		}
	}
}

func customerTierRank(tier string) int {
	switch tier {
	case "core":
		return 3
	case "mainstay":
		return 2
	case "optimize":
		return 1
	default:
		return 0
	}
}

func clampRelationshipScore(score int) int {
	return minInt(maxInt(score, 0), 100)
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

	marketMedianCents := candidateMarketMonthlyPrice(*candidate) + candidate.GapToMarketMedianCents
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
	populatePricingEvidence(candidate)

	candidate.AnalysisCodes = []string{"relationship_" + candidate.RelationshipStage}
	switch candidate.RenewalEvidence {
	case "unpaid":
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "payment_unverified")
	case "initial":
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "first_cycle_only")
	case "renewed":
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "renewal_observed")
	case "increase_accepted":
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "retained_after_increase")
	}
	candidate.AnalysisCodes = append(candidate.AnalysisCodes, "market_"+candidate.MarketPosition)
	if (candidate.RenewalCount >= 2 || candidate.PaidPeriodsAfterIncrease > 0) &&
		candidate.VerifiedPriceIndex != nil && *candidate.VerifiedPriceIndex >= 70 {
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "proven_price_acceptance")
	} else if candidate.MarketPosition == "below_low" && candidate.CustomerGroupSize <= 1 {
		candidate.AnalysisCodes = append(candidate.AnalysisCodes, "low_price_entry")
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
	if candidate.MarketPosition == "below_low" &&
		candidate.CustomerGroupSize <= 1 &&
		candidate.AdjustmentRisk == "low" {
		candidate.AdjustmentRisk = "medium"
		candidate.ReadinessScore = minInt(candidate.ReadinessScore, 74)
	}
}

// populatePricingEvidence deliberately avoids estimating willingness-to-pay or
// renewal probability from a first order. Repeat-purchase models treat the
// first transaction as acquisition and start their frequency signal at the
// second paid period. Until that evidence exists, the API exposes facts: the
// latest price actually paid, its market-relative index, and an evidence stage.
func populatePricingEvidence(candidate *PricingCandidate) {
	candidate.RenewalCount = maxInt(candidate.PaidPeriodCount-1, 0)
	switch {
	case candidate.PaidPeriodCount <= 0:
		candidate.RenewalEvidence = "unpaid"
	case candidate.PaidPeriodsAfterIncrease > 0:
		candidate.RenewalEvidence = "increase_accepted"
	case candidate.RenewalCount > 0:
		candidate.RenewalEvidence = "renewed"
	default:
		candidate.RenewalEvidence = "initial"
	}
	candidate.VerifiedPriceIndex = verifiedPriceIndex(*candidate)

	// This is an operating opportunity-cost index, not a churn prediction.
	// The market gap is counted once; the previous formula counted the same gap
	// through three correlated terms and forced most low-price users to 94.
	gapPressureScore := minInt(maxInt(candidate.PriceGapPercent, 0)*2, 80)
	deferralPressureScore := minInt(candidate.ExemptionCount, 4) * 5
	candidate.PricePressureScore = minInt(gapPressureScore+deferralPressureScore, 100)
}

// verifiedPriceIndex reports the latest price actually paid relative to the
// current market median (market median = 100). A nil value means either the
// customer has not paid or the market sample is unavailable. It is a price
// benchmark, never a probability or a claim about an untested higher price.
func verifiedPriceIndex(candidate PricingCandidate) *int {
	marketMedianCents := candidateMarketMonthlyPrice(candidate) + candidate.GapToMarketMedianCents
	verifiedMonthlyPriceCents := candidate.VerifiedMonthlyPriceCents
	if verifiedMonthlyPriceCents <= 0 {
		verifiedMonthlyPriceCents = candidate.VerifiedPriceCents
	}
	if candidate.PaidPeriodCount <= 0 || verifiedMonthlyPriceCents <= 0 ||
		candidate.MarketPosition == "unavailable" || marketMedianCents <= 0 {
		return nil
	}
	index := maxInt(int(math.Round(
		float64(verifiedMonthlyPriceCents)*100/float64(marketMedianCents),
	)), 0)
	return &index
}

type pricingBillHistory struct {
	PaidPeriodCount          int
	LastIncreaseDate         string
	PaidPeriodsAfterIncrease int
	LastPaidPriceCents       int64
	LastPaidDueDate          string
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
		history := pricingBillHistory{
			PaidPeriodCount:    len(subscriptionBills),
			LastPaidPriceCents: subscriptionBills[len(subscriptionBills)-1].AmountCents,
			LastPaidDueDate:    subscriptionBills[len(subscriptionBills)-1].DueDate,
		}
		lastIncreaseIndex := -1
		for index := 1; index < len(subscriptionBills); index++ {
			if subscriptionBills[index].AmountCents > subscriptionBills[index-1].AmountCents {
				history.LastIncreaseDate = subscriptionBills[index].DueDate
				lastIncreaseIndex = index
			}
		}
		if lastIncreaseIndex >= 0 {
			// The bill on which the higher price takes effect is already a paid
			// acceptance outcome and must be included in the evidence count.
			history.PaidPeriodsAfterIncrease = len(subscriptionBills) - lastIncreaseIndex
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
		totalPricePressureScore += candidate.PricePressureScore
		if candidate.RenewalCount > 0 {
			analysis.RepeatSubscriptionCount++
		} else if candidate.PaidPeriodCount > 0 {
			analysis.FirstCycleSubscriptionCount++
		}
		if candidate.PaidPeriodsAfterIncrease > 0 {
			analysis.IncreasedPriceAcceptedCount++
		}
		if candidate.BlockedCode == "exempted" {
			analysis.ActiveExemptionCount++
		}
		incrementSegment(analysis.RelationshipSegments, relationshipIndexes, candidate.RelationshipStage)
		incrementSegment(analysis.RiskSegments, riskIndexes, candidate.AdjustmentRisk)
		incrementSegment(analysis.PriceSegments, priceIndexes, candidate.MarketPosition)
		if tierIndex, exists := tierIndexes[candidate.CustomerTier]; exists {
			tier := &analysis.CustomerTiers[tierIndex]
			marketMonthlyPriceCents := candidateMarketMonthlyPrice(candidate)
			tier.Count++
			tierCustomerGroups[tierIndex][customerGroupID] = struct{}{}
			tier.MonthlyRevenueCents += candidate.MonthlyRevenueCents
			tierPriceTotals[tierIndex] += marketMonthlyPriceCents
			totalMonthlyRevenueCents += candidate.MonthlyRevenueCents
			if tier.LowestPriceCents == 0 || marketMonthlyPriceCents < tier.LowestPriceCents {
				tier.LowestPriceCents = marketMonthlyPriceCents
			}
			if marketMonthlyPriceCents > tier.HighestPriceCents {
				tier.HighestPriceCents = marketMonthlyPriceCents
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

// ScheduleManualNextPrices lets the operator arrange per-seat future prices
// from a customer profile. It deliberately bypasses algorithmic timing gates
// (protection, cooldown, and recovery observation), while retaining hard
// operational safeguards, atomic writes, and the 30-day advance-notice rule.
func (service *SubscriptionService) ScheduleManualNextPrices(input ManualNextPricesInput) (int, error) {
	if len(input.Items) == 0 {
		return 0, fmt.Errorf("请至少填写一个人工调价价格")
	}
	if len(input.Items) > maximumBulkPricingSelection {
		return 0, fmt.Errorf("单次最多调整 %d 个 Team 席位", maximumBulkPricingSelection)
	}

	candidates, err := service.buildPricingCandidates(nil, 0)
	if err != nil {
		return 0, err
	}
	candidateByID := make(map[int64]PricingCandidate, len(candidates))
	for _, candidate := range candidates {
		candidateByID[candidate.SubscriptionID] = candidate
	}

	seen := make(map[int64]struct{}, len(input.Items))
	updates := make([]model.Subscription, 0, len(input.Items))
	for _, item := range input.Items {
		if item.SubscriptionID <= 0 {
			return 0, fmt.Errorf("包含无效的订阅 ID")
		}
		if _, duplicate := seen[item.SubscriptionID]; duplicate {
			return 0, fmt.Errorf("人工调价列表包含重复的 Team 席位")
		}
		seen[item.SubscriptionID] = struct{}{}

		candidate, exists := candidateByID[item.SubscriptionID]
		if !exists {
			return 0, fmt.Errorf("所选 Team 席位不存在或已下车")
		}
		switch candidate.BlockedCode {
		case "eligible", "protection", "cooldown", "after_sales_recovery":
			// Manual market judgment may override these algorithmic timing gates.
		case "account_banned", "after_sales", "invalid_schedule", "scheduled", "exempted":
			return 0, fmt.Errorf("%s 暂时不能人工调价：%s", candidate.Name, candidate.BlockedReason)
		default:
			return 0, fmt.Errorf("%s 当前状态不支持人工调价", candidate.Name)
		}

		previous, getErr := service.Store.GetSubscription(item.SubscriptionID)
		if getErr != nil {
			if getErr == sql.ErrNoRows {
				return 0, fmt.Errorf("所选 Team 席位不存在或已下车")
			}
			return 0, getErr
		}
		if previous.BusinessType != model.SubscriptionBusinessTeam || previous.SeatID <= 0 || previous.IsResale {
			return 0, fmt.Errorf("%s 不是可人工调价的 Team 席位", previous.Name)
		}
		if err := service.ensureNoPendingAfterSales(item.SubscriptionID, "安排人工调价"); err != nil {
			return 0, fmt.Errorf("%s：%w", previous.Name, err)
		}
		nextPriceCents, parseErr := cycle.ParseYuanToCents(strings.TrimSpace(item.NextPriceYuan))
		if parseErr != nil || nextPriceCents <= 0 {
			return 0, fmt.Errorf("%s 的人工调价金额无效", previous.Name)
		}
		if nextPriceCents <= previous.PricePerPersonCents {
			return 0, fmt.Errorf(
				"%s 的新价格须高于当前价格 ¥%s",
				previous.Name,
				cycle.FormatCents(previous.PricePerPersonCents),
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
