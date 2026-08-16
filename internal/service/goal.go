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
	Recommended            bool   `json:"recommended"`
	Eligible               bool   `json:"eligible"`
	BlockedReason          string `json:"blocked_reason"`
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
	dashboard, err := service.ComputeDashboard()
	if err != nil {
		return 0, err
	}
	return service.Store.CreateBusinessGoal(model.BusinessGoal{
		Name:                name,
		TargetProfitCents:   targetProfitCents,
		BaselineProfitCents: dashboard.TotalProfitCents,
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
	return service.Store.CompleteBusinessGoal(goalID, dashboard.TotalProfitCents-goal.BaselineProfitCents)
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
	center.Candidates, err = service.buildPricingCandidates(market.Snapshot)
	if err != nil {
		return GoalCenter{}, err
	}
	return center, nil
}

func (service *SubscriptionService) RefreshMarketPrice() (MarketPriceView, error) {
	return service.loadMarketPrice(true)
}

func (service *SubscriptionService) buildGoalProgress(goal model.BusinessGoal, currentProfitCents int64) BusinessGoalProgress {
	earnedProfitCents := currentProfitCents - goal.BaselineProfitCents
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
		key := bill.PaidAt.In(cycle.Location).Format("2006-01")
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
	minimumHealthyPrice := seatCostFloorCents * 120 / 100
	switch {
	case utilizationPercent < 70:
		recommendation.Action = "fill"
		recommendation.ReasonCodes = []string{"low_utilization", "protect_occupancy"}
		recommendation.SuggestedLowPriceCents = maxInt64(minimumHealthyPrice, minInt64(internalMedian, marketLow))
		recommendation.SuggestedHighPriceCents = maxInt64(recommendation.SuggestedLowPriceCents, minInt64(internalMedian, marketMedian))
	case utilizationPercent >= 85 && internalMedian < marketLow*95/100:
		recommendation.Action = "raise"
		recommendation.ReasonCodes = []string{"high_utilization", "below_market"}
		recommendation.SuggestedLowPriceCents = maxInt64(minimumHealthyPrice, marketLow)
		recommendation.SuggestedHighPriceCents = maxInt64(recommendation.SuggestedLowPriceCents, marketMedian)
	case utilizationPercent < 85 && internalMedian > marketHigh*110/100:
		recommendation.Action = "lower_test"
		recommendation.ReasonCodes = []string{"above_market", "available_seats"}
		recommendation.SuggestedLowPriceCents = maxInt64(minimumHealthyPrice, marketMedian)
		recommendation.SuggestedHighPriceCents = maxInt64(recommendation.SuggestedLowPriceCents, marketHigh)
	default:
		recommendation.Action = "hold"
		recommendation.ReasonCodes = []string{"price_in_range", "stable_utilization"}
		recommendation.SuggestedLowPriceCents = maxInt64(minimumHealthyPrice, marketLow)
		recommendation.SuggestedHighPriceCents = maxInt64(recommendation.SuggestedLowPriceCents, marketHigh)
	}
	return recommendation, nil
}

func (service *SubscriptionService) buildPricingCandidates(snapshot *model.MarketPriceSnapshot) ([]PricingCandidate, error) {
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
	for _, caseItem := range afterSalesCases {
		if caseItem.Status == model.AfterSalesStatusPending || caseItem.Status == model.AfterSalesStatusReview {
			frozenSubscriptions[caseItem.SubscriptionID] = struct{}{}
		}
	}

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
			MarketPosition:         "unavailable",
		}
		if _, active := activeAccountIDs[subscription.AccountID]; !active {
			candidate.Eligible = false
			candidate.BlockedReason = "所属 Team 账号已封禁"
		}
		if _, frozen := frozenSubscriptions[subscription.ID]; frozen {
			candidate.Eligible = false
			candidate.BlockedReason = "正在等待售后处理"
		}
		view, viewErr := service.buildView(subscription, service.now(), "")
		if viewErr != nil {
			candidate.Eligible = false
			candidate.BlockedReason = "计费周期无效，无法确定下次续费日"
		} else {
			candidate.NextDueDate = view.NextDueDate
		}
		if snapshot != nil && snapshot.SampleCount >= 3 {
			candidate.GapToMarketMedianCents = snapshot.MedianPriceCents - subscription.PricePerPersonCents
			candidate.SuggestedPriceCents = snapshot.MedianPriceCents
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
				subscription.PricePerPersonCents < snapshot.LowPriceCents
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

// ScheduleBulkNextPrice atomically arranges the same next-cycle Team price for
// selected customers. Each subscription keeps its own calculated effective due
// date, and neither the current price nor historical bills are changed.
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
		if previous.BusinessType != model.SubscriptionBusinessTeam || previous.SeatID <= 0 || previous.IsResale {
			return 0, fmt.Errorf("%s 不是可批量调价的 Team 席位", previous.Name)
		}
		if err := service.ensureNoPendingAfterSales(subscriptionID, "安排调价"); err != nil {
			return 0, fmt.Errorf("%s：%w", previous.Name, err)
		}
		updated := previous
		updated.NextPriceCents = &nextPriceCents
		if err := service.configureNextPrice(previous, &updated, false); err != nil {
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
