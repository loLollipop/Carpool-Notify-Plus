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
	minimumRepricingPaidPeriods      = 3
	minimumRepricingRelationshipDays = 60
	repricingCooldownDays            = 180
	repricingAfterSalesRecoveryDays  = 30
	maximumRepricingIncreasePercent  = 8
	maximumRepricingIncreaseCents    = int64(1000)
	marketRenewalDiscountPercent     = 5
	minimumPriceIncreaseNoticeDays   = 30
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
}

// PricingCandidate exposes one active Team customer for market comparison and
// optional next-cycle repricing. Current-period prices and bills are never
// mutated by the bulk action.
type PricingCandidate struct {
	SubscriptionID         int64  `json:"subscription_id"`
	Name                   string `json:"name"`
	CustomerEmail          string `json:"customer_email"`
	CustomerWechat         string `json:"customer_wechat"`
	AccountName            string `json:"account_name"`
	SeatName               string `json:"seat_name"`
	CurrentPriceCents      int64  `json:"current_price_cents"`
	NextPriceCents         *int64 `json:"next_price_cents"`
	NextPriceEffectiveDate string `json:"next_price_effective_date"`
	NextDueDate            string `json:"next_due_date"`
	MarketPosition         string `json:"market_position"`
	GapToMarketMedianCents int64  `json:"gap_to_market_median_cents"`
	SuggestedPriceCents    int64  `json:"suggested_price_cents"`
	MaxIncreasePriceCents  int64  `json:"max_increase_price_cents"`
	PaidPeriodCount        int    `json:"paid_period_count"`
	RelationshipDays       int    `json:"relationship_days"`
	LastPriceIncreaseDate  string `json:"last_price_increase_date"`
	BlockedCode            string `json:"blocked_code"`
	NextReviewDate         string `json:"next_review_date"`
	SuggestedMonthlyUplift int64  `json:"suggested_monthly_uplift_cents"`
	ScheduledMonthlyUplift int64  `json:"scheduled_monthly_uplift_cents"`
	Recommended            bool   `json:"recommended"`
	Eligible               bool   `json:"eligible"`
	BlockedReason          string `json:"blocked_reason"`
}

type RepricingWindow struct {
	Key                string `json:"key"`
	Count              int    `json:"count"`
	MonthlyUpliftCents int64  `json:"monthly_uplift_cents"`
}

type RepricingAnalysis struct {
	TotalCount                  int               `json:"total_count"`
	EligibleCount               int               `json:"eligible_count"`
	RecommendedCount            int               `json:"recommended_count"`
	ScheduledCount              int               `json:"scheduled_count"`
	ProtectedCount              int               `json:"protected_count"`
	BelowMarketCount            int               `json:"below_market_count"`
	EstimatedMonthlyUpliftCents int64             `json:"estimated_monthly_uplift_cents"`
	PipelineMonthlyUpliftCents  int64             `json:"pipeline_monthly_uplift_cents"`
	ScheduledMonthlyUpliftCents int64             `json:"scheduled_monthly_uplift_cents"`
	Windows                     []RepricingWindow `json:"windows"`
}

type BulkNextPriceInput struct {
	SubscriptionIDs []int64
	NextPriceYuan   string
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
			months[index].CostCents += bill.CostCents
		}
	}

	costRecords, err := service.Store.ListAllAccountCostRecords()
	if err != nil {
		return nil, err
	}
	for _, record := range costRecords {
		key := monthFromDate(record.PeriodDate)
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
	attractiveMarketPrice := attractiveRenewalPriceCents(marketMedian)
	minimumHealthyPrice := seatCostFloorCents * 120 / 100
	switch {
	case utilizationPercent < 70:
		recommendation.Action = "fill"
		recommendation.ReasonCodes = []string{"low_utilization", "protect_occupancy"}
		recommendation.SuggestedLowPriceCents = maxInt64(minimumHealthyPrice, minInt64(internalMedian, marketLow))
		recommendation.SuggestedHighPriceCents = maxInt64(recommendation.SuggestedLowPriceCents, minInt64(internalMedian, attractiveMarketPrice))
	case utilizationPercent >= 85 && internalMedian < marketLow*95/100:
		recommendation.Action = "raise"
		recommendation.ReasonCodes = []string{"high_utilization", "below_market"}
		recommendation.SuggestedLowPriceCents = maxInt64(minimumHealthyPrice, marketLow)
		recommendation.SuggestedHighPriceCents = maxInt64(recommendation.SuggestedLowPriceCents, attractiveMarketPrice)
	case utilizationPercent < 85 && internalMedian > marketHigh*110/100:
		recommendation.Action = "lower_test"
		recommendation.ReasonCodes = []string{"above_market", "available_seats"}
		recommendation.SuggestedLowPriceCents = maxInt64(minimumHealthyPrice, attractiveMarketPrice)
		recommendation.SuggestedHighPriceCents = maxInt64(recommendation.SuggestedLowPriceCents, attractiveMarketPrice)
	default:
		recommendation.Action = "hold"
		recommendation.ReasonCodes = []string{"price_in_range", "stable_utilization"}
		recommendation.SuggestedLowPriceCents = maxInt64(minimumHealthyPrice, marketLow)
		recommendation.SuggestedHighPriceCents = maxInt64(recommendation.SuggestedLowPriceCents, marketHigh)
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
	for _, caseItem := range afterSalesCases {
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
	billHistories := summarizePricingBillHistories(bills)

	subscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		return nil, err
	}
	candidates := make([]PricingCandidate, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		if subscription.BusinessType != model.SubscriptionBusinessTeam || subscription.SeatID <= 0 || subscription.IsResale {
			continue
		}
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
		candidate.RelationshipDays = subscriptionRelationshipDays(subscription, service.now())
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
				earliestProtectionReviewDate(subscription, history.PaidPeriodCount, service.now()),
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
			candidate.Recommended = candidate.Eligible &&
				subscription.NextPriceCents == nil &&
				subscription.PricePerPersonCents < snapshot.LowPriceCents &&
				candidate.SuggestedPriceCents > subscription.PricePerPersonCents
		}
		factorNumerator, factorDenominator := monthlyCycleFactor(subscription, service.now())
		if factorDenominator > 0 {
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

type pricingBillHistory struct {
	PaidPeriodCount  int
	LastIncreaseDate string
}

func summarizePricingBillHistories(bills []model.Bill) map[int64]pricingBillHistory {
	grouped := make(map[int64][]model.Bill)
	for _, bill := range bills {
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
		for index := 1; index < len(subscriptionBills); index++ {
			if subscriptionBills[index].AmountCents > subscriptionBills[index-1].AmountCents {
				history.LastIncreaseDate = subscriptionBills[index].DueDate
			}
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

func earliestProtectionReviewDate(
	subscription model.Subscription,
	paidPeriodCount int,
	now time.Time,
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
	reviewAt := cycle.StartOfDay(startedAt).AddDate(0, 0, minimumRepricingRelationshipDays)
	remainingPeriods := minimumRepricingPaidPeriods - paidPeriodCount
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
	analysis := RepricingAnalysis{}
	for _, key := range windowOrder {
		windowIndexes[key] = len(analysis.Windows)
		analysis.Windows = append(analysis.Windows, RepricingWindow{Key: key})
	}
	today := cycle.StartOfDay(now)
	for _, candidate := range candidates {
		analysis.TotalCount++
		if candidate.Eligible {
			analysis.EligibleCount++
		}
		if candidate.MarketPosition == "below_low" || candidate.MarketPosition == "below_median" {
			analysis.BelowMarketCount++
		}
		if candidate.BlockedCode == "protection" ||
			candidate.BlockedCode == "cooldown" ||
			candidate.BlockedCode == "after_sales_recovery" {
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
		underpriced := candidate.MarketPosition == "below_low" || candidate.MarketPosition == "below_median"
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
	return analysis
}

func attractiveRenewalPriceCents(marketMedianCents int64) int64 {
	if marketMedianCents <= 0 {
		return 0
	}
	return marketMedianCents * (100 - marketRenewalDiscountPercent) / 100
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
	if len(input.SubscriptionIDs) > 200 {
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
	if err := service.Store.UpdateSubscriptionNextPrices(updates); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("所选用户状态已变化，请刷新后重试")
		}
		return 0, err
	}
	return len(updates), nil
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
