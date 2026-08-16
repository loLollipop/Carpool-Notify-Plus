package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"sort"
	"strings"
	"text/template"
	"time"

	"carpool-notify/internal/config"
	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/notify"
)

const (
	maxAttempts = 5

	maxRedeemAnnouncementTitleLength   = 80
	maxRedeemAnnouncementIntroLength   = 300
	maxRedeemAnnouncementItemLength    = 300
	maxRedeemAnnouncementItemCount     = 6
	maxRedeemSupportTitleLength        = 60
	maxRedeemSupportDescriptionLength  = 160
	maxRedeemSupportContactLabelLength = 30
	maxRedeemSupportWechatIDLength     = 80
	maxRedeemQRCodeDataURLLength       = 1500000
)

// SubscriptionService handles subscription CRUD and presentation helpers.
type SubscriptionService struct {
	Store          *db.Store
	Config         config.Config
	Notify         notify.Registry
	Clock          func() time.Time
	MarketClient   MarketHTTPDoer
	MarketPriceURL string
}

// NewNotifyRegistry builds senders from runtime configuration.
func NewNotifyRegistry(configuration config.Config) notify.Registry {
	registry := notify.Registry{}
	if configuration.GotifyConfigured() {
		registry.Gotify = notify.GotifySender{
			BaseURL: configuration.GotifyURL,
			Token:   configuration.GotifyToken,
		}
	}
	if configuration.IYUUConfigured() {
		registry.IYUU = notify.IYUUSender{Token: configuration.IYUUToken}
	}
	if configuration.SMTPConfigured() {
		registry.SMTP = notify.SMTPSender{
			Host:     configuration.SMTPHost,
			Port:     configuration.SMTPPort,
			Username: configuration.SMTPUsername,
			Password: configuration.SMTPPassword,
			From:     configuration.SMTPFrom,
			To:       notify.ParseSMTPRecipients(configuration.SMTPTo),
		}
	}
	return registry
}

// ApplyConfig refreshes runtime notification senders after config.toml changes.
func (service *SubscriptionService) ApplyConfig(configuration config.Config) {
	service.Config = configuration
	service.Notify = NewNotifyRegistry(configuration)
}

func (service *SubscriptionService) now() time.Time {
	if service.Clock != nil {
		return service.Clock().In(cycle.Location)
	}
	return cycle.Now()
}

// SubscriptionView is a list/card presentation model.
type SubscriptionView struct {
	Subscription               model.Subscription `json:"subscription"`
	PriceYuan                  string             `json:"price_yuan"`
	NextPriceYuan              string             `json:"next_price_yuan"`
	NextPriceEffectiveDueDate  string             `json:"next_price_effective_due_date"`
	CostYuan                   string             `json:"cost_yuan"`
	AllocatedCostYuan          string             `json:"allocated_cost_yuan"`
	AgencyFeeYuan              string             `json:"agency_fee_yuan"`
	ProfitYuan                 string             `json:"profit_yuan"`
	AllocatedProfitYuan        string             `json:"allocated_profit_yuan"`
	CycleDesc                  string             `json:"cycle_desc"`
	NextDueDate                string             `json:"next_due_date"`
	DaysRemaining              int                `json:"days_remaining"`
	CycleDays                  int                `json:"cycle_days"`
	CurrentPeriodStartDate     string             `json:"current_period_start_date"`
	CurrentPeriodEndDate       string             `json:"current_period_end_date"`
	CurrentPeriodPaid          bool               `json:"current_period_paid"`
	ChannelLabels              []string           `json:"channel_labels"`
	OffsetsText                string             `json:"offsets_text"`
	LastError                  string             `json:"last_error"`
	AccountID                  int64              `json:"account_id"`
	AccountName                string             `json:"account_name"`
	SeatID                     int64              `json:"seat_id"`
	SeatName                   string             `json:"seat_name"`
	BoardedAt                  string             `json:"boarded_at"`
	ArchivedAtLabel            string             `json:"archived_at_label"`
	CancellationPending        bool               `json:"cancellation_pending"`
	CancellationCaseID         int64              `json:"cancellation_case_id"`
	CancellationExpiresAtLabel string             `json:"cancellation_expires_at_label"`
	// BillCount is set for archived rows (used to gate soft-delete).
	BillCount int `json:"bill_count"`
	// CanSoftDelete is true when archived and BillCount == 0.
	CanSoftDelete bool `json:"can_soft_delete"`
}

// ListView returns subscriptions with computed display fields.
func (service *SubscriptionService) ListView() ([]SubscriptionView, error) {
	if _, err := service.Store.RestoreExpiredCancellationRequests(service.now()); err != nil {
		return nil, err
	}
	subscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		return nil, err
	}
	errorsBySubscription, err := service.Store.LatestErrorsBySubscription()
	if err != nil {
		return nil, err
	}

	now := service.now()
	views := make([]SubscriptionView, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		view, err := service.buildView(subscription, now, errorsBySubscription[subscription.ID])
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	if err := service.allocateActiveAccountCosts(views); err != nil {
		return nil, err
	}
	return views, nil
}

// allocateActiveAccountCosts spreads each owner account's monthly cost across
// its active non-resale customers. The allocated cents always add back up to
// exactly one account cost, including costs that do not divide evenly.
func (service *SubscriptionService) allocateActiveAccountCosts(views []SubscriptionView) error {
	accounts, err := service.Store.ListAccounts()
	if err != nil {
		return err
	}
	accountCosts := make(map[int64]int64, len(accounts))
	for _, account := range accounts {
		accountCosts[account.ID] = account.CostCents
	}

	groups := make(map[string][]int)
	groupCosts := make(map[string]int64)
	for index := range views {
		view := &views[index]
		if isPlusSubscription(view.Subscription) {
			view.AllocatedCostYuan = cycle.FormatCents(view.Subscription.CostCents)
			view.AllocatedProfitYuan = cycle.FormatCents(
				view.Subscription.PricePerPersonCents - view.Subscription.CostCents,
			)
			continue
		}
		if view.Subscription.IsResale {
			view.AllocatedCostYuan = cycle.FormatCents(0)
			view.AllocatedProfitYuan = cycle.FormatCents(view.Subscription.AgencyFeeCents)
			continue
		}
		key := fmt.Sprintf("name:%s", displayAccountName(view.Subscription))
		useLegacyCost := true
		if view.AccountID > 0 {
			key = fmt.Sprintf("id:%d", view.AccountID)
			if accountCosts[view.AccountID] > 0 {
				groupCosts[key] = accountCosts[view.AccountID]
				useLegacyCost = false
			}
		}
		if useLegacyCost && view.Subscription.CostCents > groupCosts[key] {
			groupCosts[key] = view.Subscription.CostCents
		}
		groups[key] = append(groups[key], index)
	}

	for key, indexes := range groups {
		sort.SliceStable(indexes, func(left int, right int) bool {
			return views[indexes[left]].Subscription.ID < views[indexes[right]].Subscription.ID
		})
		costCents := groupCosts[key]
		baseShare := costCents / int64(len(indexes))
		remainder := costCents % int64(len(indexes))
		for position, index := range indexes {
			share := baseShare
			if int64(position) < remainder {
				share++
			}
			views[index].AllocatedCostYuan = cycle.FormatCents(share)
			views[index].AllocatedProfitYuan = cycle.FormatCents(
				views[index].Subscription.PricePerPersonCents - share,
			)
		}
	}
	return nil
}

func (service *SubscriptionService) buildView(
	subscription model.Subscription,
	now time.Time,
	lastError string,
) (SubscriptionView, error) {
	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		return SubscriptionView{}, err
	}
	nextDue := schedule.NextDue(now)
	lastDue, hasLastDue := schedule.LastDue(now)
	oneMonthRental := isOneMonthRental(subscription)
	var oneMonthStart time.Time
	var oneMonthEnd time.Time
	if oneMonthRental {
		oneMonthStart, oneMonthEnd, err = oneMonthRentalPeriod(subscription)
		if err != nil {
			return SubscriptionView{}, err
		}
		lastDue = oneMonthStart
		hasLastDue = true
		nextDue = oneMonthEnd
	}
	displayDue := nextDue
	cycleDays := cycle.DaysRemaining(nextDue, now)
	if hasLastDue {
		cycleDays = cycle.DaysRemaining(nextDue, lastDue)
	}

	// Drive every list/filter reminder from the first relevant unpaid period,
	// instead of from the currently selected calendar month. This keeps overdue
	// subscriptions visible after a month rollover and includes next-month dues
	// that fall within the upcoming seven-day window.
	periodStart := nextDue
	if hasLastDue {
		periodStart = lastDue
	}
	paidDueDates, err := service.Store.ListPaidDueDatesForSubscription(subscription.ID)
	if err != nil {
		return SubscriptionView{}, err
	}
	paidDueSet := make(map[string]struct{}, len(paidDueDates))
	for _, dueDate := range paidDueDates {
		paidDueSet[dueDate] = struct{}{}
	}
	_, periodStartPaid := paidDueSet[cycle.FormatDate(periodStart)]
	if oneMonthRental {
		displayDue = oneMonthEnd
	} else if len(paidDueDates) > 0 {
		// The earliest recorded bill is the beginning of trustworthy ledger
		// history. This avoids treating every period before a legacy import as
		// unpaid while still detecting gaps between recorded renewals.
		unpaidSearchStart, parseErr := time.ParseInLocation("2006-01-02", paidDueDates[0], cycle.Location)
		if parseErr != nil {
			return SubscriptionView{}, fmt.Errorf("invalid stored bill due date %q: %w", paidDueDates[0], parseErr)
		}
		// A future period may have been paid out of order while the current period
		// is still unpaid. Start no later than the current/next relevant period.
		if periodStart.Before(unpaidSearchStart) {
			unpaidSearchStart = periodStart
		}
		displayDue, err = firstUnpaidDue(schedule, unpaidSearchStart, paidDueSet)
		if err != nil {
			return SubscriptionView{}, err
		}
	} else {
		// Legacy/imported Team rows may intentionally have no initial bill. Their
		// established behavior is to start reminders at the next due occurrence;
		// Plus still treats its current rental period as unpaid.
		if isPlusSubscription(subscription) {
			displayDue = periodStart
		} else {
			displayDue = nextDue
		}
	}
	currentPeriodStartDate := ""
	currentPeriodEndDate := ""
	currentPeriodPaid := false
	if isPlusSubscription(subscription) {
		periodEnd := nextDue
		if oneMonthRental {
			periodStart = oneMonthStart
			periodEnd = oneMonthEnd
			cycleDays = cycle.DaysRemaining(periodEnd, periodStart)
			_, periodStartPaid = paidDueSet[cycle.FormatDate(periodStart)]
		} else if !hasLastDue {
			periodEnd = schedule.NextDue(periodStart)
			cycleDays = cycle.DaysRemaining(periodEnd, periodStart)
		}
		currentPeriodStartDate = cycle.FormatDate(periodStart)
		currentPeriodEndDate = cycle.FormatDate(periodEnd)
		currentPeriodPaid = periodStartPaid
	}
	if cycleDays < 1 {
		cycleDays = 1
	}
	profitCents := countedProfitCents(subscription)
	nextPriceYuan := ""
	if subscription.NextPriceCents != nil {
		nextPriceYuan = cycle.FormatCents(*subscription.NextPriceCents)
	}
	archivedAtLabel := ""
	if subscription.ArchivedAt != nil {
		archivedAtLabel = subscription.ArchivedAt.In(cycle.Location).Format("2006-01-02 15:04")
	}
	cancellationPending := subscription.CancellationCaseID > 0 &&
		subscription.CancellationExpiresAt != nil &&
		subscription.CancellationExpiresAt.After(now)
	cancellationExpiresAtLabel := ""
	if cancellationPending {
		cancellationExpiresAtLabel = subscription.CancellationExpiresAt.In(cycle.Location).Format("2006-01-02 15:04")
	}
	return SubscriptionView{
		Subscription:               subscription,
		PriceYuan:                  cycle.FormatCents(subscription.PricePerPersonCents),
		NextPriceYuan:              nextPriceYuan,
		NextPriceEffectiveDueDate:  subscription.NextPriceEffectiveDueDate,
		CostYuan:                   cycle.FormatCents(subscription.CostCents),
		AllocatedCostYuan:          cycle.FormatCents(subscription.CostCents),
		AgencyFeeYuan:              cycle.FormatCents(subscription.AgencyFeeCents),
		ProfitYuan:                 cycle.FormatCents(profitCents),
		AllocatedProfitYuan:        cycle.FormatCents(profitCents),
		CycleDesc:                  cycle.DescribeCron(subscription.CronExpr),
		NextDueDate:                cycle.FormatDate(displayDue),
		DaysRemaining:              cycle.DaysRemaining(displayDue, now),
		CycleDays:                  cycleDays,
		CurrentPeriodStartDate:     currentPeriodStartDate,
		CurrentPeriodEndDate:       currentPeriodEndDate,
		CurrentPeriodPaid:          currentPeriodPaid,
		ChannelLabels:              scheduledNotificationLabels(subscription),
		OffsetsText:                cycle.FormatOffsets(subscription.NotifyOffsets),
		LastError:                  lastError,
		AccountID:                  subscription.AccountID,
		AccountName:                displayAccountName(subscription),
		SeatID:                     subscription.SeatID,
		SeatName:                   subscription.SeatName,
		BoardedAt:                  subscription.BoardedAt,
		ArchivedAtLabel:            archivedAtLabel,
		CancellationPending:        cancellationPending,
		CancellationCaseID:         subscription.CancellationCaseID,
		CancellationExpiresAtLabel: cancellationExpiresAtLabel,
	}, nil
}

func firstUnpaidDue(
	schedule cycle.BillingSchedule,
	start time.Time,
	paidDueDates map[string]struct{},
) (time.Time, error) {
	candidate := start
	// At most every stored bill can match one schedule occurrence; one extra
	// iteration must therefore find the first gap or the next unpaid period.
	for range len(paidDueDates) + 1 {
		if _, paid := paidDueDates[cycle.FormatDate(candidate)]; !paid {
			return candidate, nil
		}
		next := schedule.NextDue(candidate)
		if !next.After(candidate) {
			return time.Time{}, fmt.Errorf("billing schedule did not advance after %s", cycle.FormatDate(candidate))
		}
		candidate = next
	}
	return time.Time{}, fmt.Errorf("unable to find an unpaid billing period")
}

func channelDisplayLabels(channels []string) []string {
	labels := make([]string, 0, len(channels))
	for _, channel := range channels {
		switch channel {
		case model.ChannelGotify:
			labels = append(labels, "Gotify")
		case model.ChannelIYUU:
			labels = append(labels, "IYUU")
		case model.ChannelSMTP:
			labels = append(labels, "SMTP")
		default:
			labels = append(labels, channel)
		}
	}
	return labels
}

func scheduledNotificationLabels(subscription model.Subscription) []string {
	if isOneMonthRental(subscription) {
		return []string{"IYUU 短租到期确认"}
	}
	if isPlusSubscription(subscription) {
		return []string{"IYUU 到期当天（微信人工续费）"}
	}
	labels := make([]string, 0, 2)
	for _, offset := range subscription.NotifyOffsets {
		if offset > 0 {
			labels = append(labels, "SMTP 客户邮件")
			break
		}
	}
	return append(labels, "IYUU 到期当天")
}

func scheduledNotificationLabelText(subscription model.Subscription) string {
	return strings.Join(scheduledNotificationLabels(subscription), " · ")
}

// Stats holds dashboard counters.
type Stats struct {
	ActiveCount    int
	DueIn7Days     int
	FailedNotifies int
}

// ComputeStats derives list header stats.
func (service *SubscriptionService) ComputeStats(views []SubscriptionView) (Stats, error) {
	failedCount, err := service.Store.CountFailedNotifications()
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{
		ActiveCount:    len(views),
		FailedNotifies: failedCount,
	}
	for _, view := range views {
		if view.DaysRemaining >= 0 && view.DaysRemaining <= 7 {
			stats.DueIn7Days++
		}
	}
	return stats, nil
}

// ComputeDashboard builds KPI counters and chart series for the home page.
func (service *SubscriptionService) ComputeDashboard() (Dashboard, error) {
	subscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		return Dashboard{}, err
	}
	archivedSubscriptions, err := service.Store.ListArchivedSubscriptions()
	if err != nil {
		return Dashboard{}, err
	}
	accountRows, err := service.Store.ListAccounts()
	if err != nil {
		return Dashboard{}, err
	}
	var totalCostCents int64
	for _, account := range accountRows {
		totalCostCents += account.TotalCostCents
	}
	bills, err := service.Store.ListBills()
	if err != nil {
		return Dashboard{}, err
	}
	subscriptionsByID := make(map[int64]model.Subscription, len(subscriptions)+len(archivedSubscriptions))
	for _, subscription := range subscriptions {
		subscriptionsByID[subscription.ID] = subscription
	}
	for _, subscription := range archivedSubscriptions {
		subscriptionsByID[subscription.ID] = subscription
	}
	var totalReceivedCents int64
	var totalAgencyFeeCents int64
	for _, bill := range bills {
		totalReceivedCents += bill.AmountCents
		if subscription, exists := subscriptionsByID[bill.SubscriptionID]; exists {
			if subscription.IsResale {
				totalAgencyFeeCents += bill.AmountCents
			}
			if isPlusSubscription(subscription) {
				totalCostCents += bill.CostCents
			}
		}
	}
	afterSalesCases, err := service.Store.ListAfterSalesCases()
	if err != nil {
		return Dashboard{}, err
	}
	var totalRefundCents int64
	for _, caseItem := range afterSalesCases {
		if caseItem.Status == model.AfterSalesStatusRefunded {
			totalRefundCents += caseItem.RefundAmountCents
		}
	}
	netRevenueCents := totalReceivedCents - totalRefundCents

	now := service.now()
	since := now.AddDate(0, 0, -30)
	successCount, err := service.Store.CountNotificationsByStatusSince(model.NotificationStatusSuccess, since)
	if err != nil {
		return Dashboard{}, err
	}
	failedCount, err := service.Store.CountNotificationsByStatusSince(model.NotificationStatusFailed, since)
	if err != nil {
		return Dashboard{}, err
	}

	amountBars := make([]AmountBar, 0, len(subscriptions))
	accountTotals := map[string]struct {
		count int
		cents int64
	}{}

	for _, subscription := range subscriptions {
		accountName := displayAccountName(subscription)
		amountCents := countedAmountCents(subscription)
		amountBars = append(amountBars, AmountBar{
			Name:        subscription.Name,
			AccountName: accountName,
			AmountYuan:  cycle.FormatCents(amountCents),
			AmountCents: amountCents,
		})
		bucket := accountTotals[accountName]
		bucket.count++
		bucket.cents += amountCents
		accountTotals[accountName] = bucket
	}

	sort.Slice(amountBars, func(left int, right int) bool {
		if amountBars[left].AmountCents == amountBars[right].AmountCents {
			return amountBars[left].Name < amountBars[right].Name
		}
		return amountBars[left].AmountCents > amountBars[right].AmountCents
	})

	accounts := make([]AccountBreakdown, 0, len(accountTotals))
	for accountName, bucket := range accountTotals {
		accounts = append(accounts, AccountBreakdown{
			AccountName: accountName,
			Type:        accountName,
			Count:       bucket.count,
			AmountYuan:  cycle.FormatCents(bucket.cents),
			AmountCents: bucket.cents,
		})
	}
	sort.Slice(accounts, func(left int, right int) bool {
		if accounts[left].AmountCents == accounts[right].AmountCents {
			return accounts[left].AccountName < accounts[right].AccountName
		}
		return accounts[left].AmountCents > accounts[right].AmountCents
	})

	totalProfitCents := netRevenueCents - totalCostCents
	profitMarginPercent := "—"
	if netRevenueCents > 0 {
		// One decimal percent: profit / price * 100 (integer arithmetic).
		marginTimesTen := totalProfitCents * 1000 / netRevenueCents
		negativeMargin := marginTimesTen < 0
		if negativeMargin {
			marginTimesTen = -marginTimesTen
		}
		whole := marginTimesTen / 10
		fraction := marginTimesTen % 10
		if negativeMargin {
			profitMarginPercent = fmt.Sprintf("-%d.%d%%", whole, fraction)
		} else {
			profitMarginPercent = fmt.Sprintf("%d.%d%%", whole, fraction)
		}
	}

	return Dashboard{
		SubscriptionCount:    len(subscriptions),
		ActiveCount:          len(subscriptions),
		ArchivedCount:        len(archivedSubscriptions),
		TotalAmountYuan:      cycle.FormatCents(totalReceivedCents),
		TotalRefundCents:     totalRefundCents,
		TotalRefundYuan:      cycle.FormatCents(totalRefundCents),
		NetRevenueCents:      netRevenueCents,
		NetRevenueYuan:       cycle.FormatCents(netRevenueCents),
		TotalCostYuan:        cycle.FormatCents(totalCostCents),
		TotalProfitYuan:      cycle.FormatCents(totalProfitCents),
		TotalProfitCents:     totalProfitCents,
		TotalAgencyFeeYuan:   cycle.FormatCents(totalAgencyFeeCents),
		ProfitMarginPercent:  profitMarginPercent,
		NotifySuccess30d:     successCount,
		NotifyFailed30d:      failedCount,
		AmountBySubscription: amountBars,
		Accounts:             accounts,
	}, nil
}

// CreateInput is validated form input for create/update.
type CreateInput struct {
	Name          string
	BusinessType  string
	PriceYuan     string
	NextPriceYuan string
	CostYuan      string
	// IsResale marks 串货; AgencyFeeYuan is the middleman fee (may be empty/0).
	IsResale         bool
	AgencyFeeYuan    string
	CronExpr         string
	NotifyOffsetsRaw string
	Remark           string
	TradeURL         string
	CustomerEmail    string
	CustomerWechat   string
	// AccountID selects which shared account to occupy. When SeatID is 0,
	// Create/Update auto-assigns the first free seat under this account.
	AccountID int64
	// SeatID is the named seat this subscription occupies.
	// Optional on create when AccountID is set; required path for explicit assignment.
	SeatID int64
	// BoardedAt is 上车日期 as YYYY-MM-DD; empty means today (Asia/Shanghai).
	BoardedAt string
}

// Dashboard is the home-page statistics model (wide KPI + charts).
type Dashboard struct {
	SubscriptionCount    int                `json:"subscription_count"`
	ActiveCount          int                `json:"active_count"`
	ArchivedCount        int                `json:"archived_count"`
	TotalAmountYuan      string             `json:"total_amount_yuan"`
	TotalRefundCents     int64              `json:"total_refund_cents"`
	TotalRefundYuan      string             `json:"total_refund_yuan"`
	NetRevenueCents      int64              `json:"net_revenue_cents"`
	NetRevenueYuan       string             `json:"net_revenue_yuan"`
	TotalCostYuan        string             `json:"total_cost_yuan"`
	TotalProfitYuan      string             `json:"total_profit_yuan"`
	TotalProfitCents     int64              `json:"total_profit_cents"`
	TotalAgencyFeeYuan   string             `json:"total_agency_fee_yuan"`
	ProfitMarginPercent  string             `json:"profit_margin_percent"`
	NotifySuccess30d     int                `json:"notify_success_30d"`
	NotifyFailed30d      int                `json:"notify_failed_30d"`
	AmountBySubscription []AmountBar        `json:"amount_by_subscription"`
	Accounts             []AccountBreakdown `json:"accounts"`
}

// AmountBar is one row in the amount distribution chart.
type AmountBar struct {
	Name        string `json:"name"`
	AccountName string `json:"account_name"`
	AmountYuan  string `json:"amount_yuan"`
	AmountCents int64  `json:"amount_cents"`
}

// AccountBreakdown is one slice in the account chart.
type AccountBreakdown struct {
	AccountName string `json:"account_name"`
	// Type is a legacy alias of AccountName retained for export compatibility.
	Type        string `json:"type"`
	Count       int    `json:"count"`
	AmountYuan  string `json:"amount_yuan"`
	AmountCents int64  `json:"amount_cents"`
}

// Create validates and inserts a subscription.
func (service *SubscriptionService) Create(input CreateInput) (int64, error) {
	subscription, err := service.parseInput(input, 0)
	if err != nil {
		return 0, err
	}
	if subscription.NextPriceCents != nil {
		return 0, fmt.Errorf("请先创建订阅，再在编辑中安排下周期调价")
	}
	subscriptionID, err := service.Store.CreateSubscription(subscription)
	if err != nil {
		return 0, publicSubscriptionMutationError(err)
	}
	return subscriptionID, nil
}

// CreateWithInitialBill creates a subscription from the operator form/import flow
// and records the first billing period as paid.
func (service *SubscriptionService) CreateWithInitialBill(input CreateInput) (int64, error) {
	subscription, err := service.parseInput(input, 0)
	if err != nil {
		return 0, err
	}
	if subscription.NextPriceCents != nil {
		return 0, fmt.Errorf("请先创建订阅，再在编辑中安排下周期调价")
	}
	initialDueDate, err := initialBillDueDate(subscription)
	if err != nil {
		return 0, err
	}
	subscriptionID, err := service.Store.CreateSubscriptionWithInitialBill(
		subscription,
		initialDueDate,
		billDefaultAmountCents(subscription),
	)
	if err != nil {
		return 0, publicSubscriptionMutationError(err)
	}
	return subscriptionID, nil
}

func initialBillDueDate(subscription model.Subscription) (string, error) {
	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		return "", err
	}
	boardedAt, err := time.ParseInLocation("2006-01-02", subscription.BoardedAt, cycle.Location)
	if err != nil {
		return "", fmt.Errorf("invalid boarded_at: %w", err)
	}
	dueAt := schedule.NextDue(cycle.StartOfDay(boardedAt).Add(-time.Nanosecond))
	return cycle.FormatDate(dueAt), nil
}

// Update validates and updates a subscription.
func (service *SubscriptionService) Update(subscriptionID int64, input CreateInput) error {
	previous, err := service.Store.GetSubscription(subscriptionID)
	if err != nil {
		return err
	}
	if err := service.ensureNoPendingAfterSales(subscriptionID, "编辑"); err != nil {
		return err
	}
	subscription, err := service.parseInput(input, subscriptionID)
	if err != nil {
		return err
	}
	// Resale fields are no longer editable, but legacy rows keep their original
	// accounting semantics so editing an unrelated field cannot rewrite history.
	if previous.IsResale {
		subscription.IsResale = true
		subscription.AgencyFeeCents = previous.AgencyFeeCents
	}
	subscription.ID = subscriptionID
	scheduleChanged :=
		(strings.TrimSpace(previous.BoardedAt) != strings.TrimSpace(subscription.BoardedAt) ||
			strings.TrimSpace(previous.CronExpr) != strings.TrimSpace(subscription.CronExpr))
	if err := service.configureNextPrice(previous, &subscription, scheduleChanged); err != nil {
		return err
	}
	var updateErr error
	if scheduleChanged {
		billCount, err := service.Store.CountBillsForSubscription(subscriptionID)
		if err != nil {
			return err
		}
		switch billCount {
		case 0:
			updateErr = service.Store.UpdateSubscription(subscription)
		case 1:
			oldDueDate, err := initialBillDueDate(previous)
			if err != nil {
				return err
			}
			newDueDate, err := initialBillDueDate(subscription)
			if err != nil {
				return err
			}
			updateErr = service.Store.UpdateSubscriptionAndMoveInitialBill(subscription, oldDueDate, newDueDate)
		default:
			return fmt.Errorf("该订阅已有多期账单，为保护历史收入，不能直接修改开始日期或计费周期")
		}
	} else {
		updateErr = service.Store.UpdateSubscription(subscription)
	}
	if updateErr != nil {
		return publicSubscriptionMutationError(updateErr)
	}
	previousBillAmount := billDefaultAmountCents(previous)
	newBillAmount := billDefaultAmountCents(subscription)
	previousBillCost := billDefaultCostCents(previous)
	newBillCost := billDefaultCostCents(subscription)
	if previousBillAmount != newBillAmount || previousBillCost != newBillCost || previous.IsResale != subscription.IsResale {
		if err := service.syncCurrentPeriodBillAmount(subscription); err != nil {
			return err
		}
	}
	return nil
}

func (service *SubscriptionService) configureNextPrice(
	previous model.Subscription,
	subscription *model.Subscription,
	scheduleChanged bool,
) error {
	if subscription.NextPriceCents == nil {
		subscription.NextPriceEffectiveDueDate = ""
		return nil
	}
	if previous.IsResale {
		return fmt.Errorf("历史代订记录不能安排下周期调价")
	}
	if isOneMonthRental(*subscription) {
		return fmt.Errorf("单月短租没有下一续租周期，不能安排调价")
	}
	if *subscription.NextPriceCents == subscription.PricePerPersonCents {
		return fmt.Errorf("下周期价格与当前价格相同，无需安排调价")
	}
	if !scheduleChanged &&
		previous.NextPriceCents != nil &&
		*previous.NextPriceCents == *subscription.NextPriceCents &&
		strings.TrimSpace(previous.NextPriceEffectiveDueDate) != "" {
		subscription.NextPriceEffectiveDueDate = previous.NextPriceEffectiveDueDate
		return nil
	}
	if scheduleChanged {
		billCount, err := service.Store.CountBillsForSubscription(subscription.ID)
		if err != nil {
			return err
		}
		if billCount == 1 {
			firstDueDate, err := initialBillDueDate(*subscription)
			if err != nil {
				return err
			}
			firstDue, err := time.ParseInLocation("2006-01-02", firstDueDate, cycle.Location)
			if err != nil {
				return err
			}
			schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
			if err != nil {
				return err
			}
			subscription.NextPriceEffectiveDueDate = cycle.FormatDate(schedule.NextDue(firstDue))
			return nil
		}
	}
	view, err := service.buildView(*subscription, service.now(), "")
	if err != nil {
		return err
	}
	effectiveDueDate := strings.TrimSpace(view.NextDueDate)
	if effectiveDueDate == "" {
		return fmt.Errorf("暂时无法确定下一个续费周期，请检查开始日期和计费周期")
	}
	subscription.NextPriceEffectiveDueDate = effectiveDueDate
	return nil
}

// syncCurrentPeriodBillAmount updates the bill for the current period start when present.
func (service *SubscriptionService) syncCurrentPeriodBillAmount(subscription model.Subscription) error {
	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		return nil
	}
	lastDue, found := schedule.LastDue(service.now())
	dueDate := ""
	if found {
		dueDate = cycle.FormatDate(lastDue)
	} else {
		// A newly created subscription can have a paid first period whose due
		// date is still in the future. Price/cost corrections must update that
		// bill as well, otherwise the form and finance ledger disagree.
		dueDate, err = initialBillDueDate(subscription)
		if err != nil {
			return nil
		}
	}
	bill, err := service.Store.GetBillByOccurrence(subscription.ID, dueDate)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	amountCents := billDefaultAmountCents(subscription)
	costCents := billDefaultCostCents(subscription)
	if bill.AmountCents == amountCents && bill.CostCents == costCents {
		return nil
	}
	if err := service.Store.UpdateBillFinancials(bill.ID, amountCents, costCents, bill.Note); err != nil {
		// An after-sales snapshot freezes the historical bill, but a completed
		// reassignment may continue using the subscription for future periods.
		if errors.Is(err, db.ErrBillHasAfterSalesCase) {
			return nil
		}
		return err
	}
	return nil
}

// SoftDelete soft-deletes a subscription (legacy unrestricted helper).
func (service *SubscriptionService) SoftDelete(subscriptionID int64) error {
	return service.Store.SoftDeleteSubscription(subscriptionID)
}

// SoftDeleteArchived soft-deletes an archived subscription only when it has no bills.
func (service *SubscriptionService) SoftDeleteArchived(subscriptionID int64) error {
	subscription, err := service.Store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("订阅不存在或已删除")
		}
		return err
	}
	if subscription.ArchivedAt == nil {
		return fmt.Errorf("只能伪删除已下车的订阅；请先下车归档")
	}
	afterSalesCount, err := service.Store.CountAfterSalesCasesBySubscription(subscriptionID)
	if err != nil {
		return err
	}
	if afterSalesCount > 0 {
		return fmt.Errorf("该订阅仍关联 %d 条售后记录，为保留退款与处理历史不能删除", afterSalesCount)
	}

	billCount, err := service.Store.CountBillsForSubscription(subscriptionID)
	if err != nil {
		return err
	}
	if billCount > 0 {
		return fmt.Errorf("仍有 %d 笔关联账单，无法删除；请先在账单页取消对应「已交费」或处理账单", billCount)
	}

	if err := service.Store.SoftDeleteArchivedSubscription(subscriptionID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("订阅不存在、未下车或已删除")
		}
		return err
	}
	return nil
}

// Archive marks a subscription as 下车 (archived): leaves list and scheduler, bills remain.
func (service *SubscriptionService) Archive(subscriptionID int64) error {
	if err := service.ensureNoPendingAfterSales(subscriptionID, "下车归档"); err != nil {
		return err
	}
	if err := service.Store.ArchiveSubscription(subscriptionID); err != nil {
		return publicSubscriptionMutationError(err)
	}
	return nil
}

// CompleteOneMonthRental archives a naturally expired one-month Plus rental
// without creating an after-sales refund case.
func (service *SubscriptionService) CompleteOneMonthRental(subscriptionID int64) error {
	subscription, err := service.Store.GetSubscription(subscriptionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("出租记录不存在或已经结束")
		}
		return err
	}
	if !isOneMonthRental(subscription) {
		return fmt.Errorf("只有单月短租可以直接确认到期结束")
	}
	_, periodEnd, err := oneMonthRentalPeriod(subscription)
	if err != nil {
		return err
	}
	if cycle.StartOfDay(service.now()).Before(periodEnd) {
		return fmt.Errorf("该短租将在 %s 到期；提前结束请走售后处理", cycle.FormatDate(periodEnd))
	}
	return service.Archive(subscriptionID)
}

// Copy duplicates an active or archived subscription as a new active subscription.
// targetSeatID must be a free seat; historical bills and notification logs are not copied.
func (service *SubscriptionService) Copy(subscriptionID int64, targetSeatID int64) (int64, error) {
	source, err := service.Store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		return 0, err
	}
	if err := service.ensureNoPendingAfterSales(subscriptionID, "复制"); err != nil {
		return 0, err
	}
	if targetSeatID <= 0 {
		return 0, fmt.Errorf("请选择空闲车位")
	}
	seat, err := service.Store.GetSeat(targetSeatID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("车位不存在")
		}
		return 0, err
	}
	if err := service.ensureSeatAvailable(targetSeatID, 0); err != nil {
		return 0, err
	}
	account, err := service.Store.GetAccount(seat.AccountID)
	if err != nil {
		return 0, err
	}
	if account.BannedAt != "" {
		return 0, fmt.Errorf("该账号已封禁，不能再分配新用户")
	}

	copyName := strings.TrimSpace(source.Name)
	if copyName == "" {
		copyName = "未命名"
	}
	if !strings.HasSuffix(copyName, "（副本）") {
		copyName = copyName + "（副本）"
	}
	copiedSubscriptionID, err := service.Store.CreateSubscription(model.Subscription{
		Name:                copyName,
		BusinessType:        source.BusinessType,
		PricePerPersonCents: source.PricePerPersonCents,
		CostCents:           source.CostCents,
		IsResale:            source.IsResale,
		AgencyFeeCents:      source.AgencyFeeCents,
		CronExpr:            source.CronExpr,
		NotifyOffsets:       append([]int(nil), source.NotifyOffsets...),
		// Legacy column retained for schema compatibility; sends use global settings.
		Channels:         append([]string(nil), model.DefaultEnabledChannels...),
		Remark:           source.Remark,
		TradeURL:         source.TradeURL,
		CustomerEmail:    source.CustomerEmail,
		CustomerWechat:   source.CustomerWechat,
		SeatID:           targetSeatID,
		AccountID:        seat.AccountID,
		AccountName:      account.Name,
		SeatName:         seat.Name,
		SubscriptionType: account.Name,
		// Keep source 上车日期 so the same due schedule still appears on the calendar.
		// Resetting to "today" would hide this month's already-passed due dates.
		BoardedAt: source.BoardedAt,
	})
	if err != nil {
		return 0, publicSubscriptionMutationError(err)
	}
	return copiedSubscriptionID, nil
}

func publicSubscriptionMutationError(err error) error {
	if errors.Is(err, db.ErrActiveSeatOccupied) {
		return fmt.Errorf("所选车位已被其他活跃订阅占用，请刷新后重试")
	}
	if errors.Is(err, db.ErrSubscriptionHasPendingAfterSales) {
		return fmt.Errorf("该订阅正在等待售后处理，暂时不能修改")
	}
	if errors.Is(err, db.ErrBillHasAfterSalesCase) {
		return fmt.Errorf("当前首期账单已关联售后记录，不能修改开始日期或计费周期")
	}
	if errors.Is(err, db.ErrBillOccurrenceConflict) {
		return fmt.Errorf("修改后的账期已有账单，不能合并或重复计费")
	}
	if errors.Is(err, db.ErrInitialBillNotMovable) {
		return fmt.Errorf("首期账单与当前计费周期不一致，请先检查账单记录")
	}
	return err
}

func (service *SubscriptionService) ensureNoPendingAfterSales(subscriptionID int64, action string) error {
	pendingCount, err := service.Store.CountPendingAfterSalesCasesBySubscription(subscriptionID)
	if err != nil {
		return err
	}
	if pendingCount > 0 {
		return fmt.Errorf("该订阅正在等待售后处理，暂时不能%s", action)
	}
	return nil
}

// ListArchivedView returns archived subscriptions for the 已下车 tab.
func (service *SubscriptionService) ListArchivedView() ([]SubscriptionView, error) {
	subscriptions, err := service.Store.ListArchivedSubscriptions()
	if err != nil {
		return nil, err
	}
	now := service.now()
	views := make([]SubscriptionView, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		view, err := service.buildView(subscription, now, "")
		if err != nil {
			return nil, err
		}
		billCount, err := service.Store.CountBillsForSubscription(subscription.ID)
		if err != nil {
			return nil, err
		}
		view.BillCount = billCount
		view.CanSoftDelete = billCount == 0
		views = append(views, view)
	}
	return views, nil
}

// Get returns one subscription.
func (service *SubscriptionService) Get(subscriptionID int64) (model.Subscription, error) {
	return service.Store.GetSubscription(subscriptionID)
}

func displayAccountName(subscription model.Subscription) string {
	if isPlusSubscription(subscription) {
		return "Plus 出租"
	}
	if name := strings.TrimSpace(subscription.AccountName); name != "" {
		return name
	}
	if name := strings.TrimSpace(subscription.SubscriptionType); name != "" {
		return name
	}
	return model.UnclassifiedAccountName
}

func (service *SubscriptionService) parseInput(input CreateInput, existingSubscriptionID int64) (model.Subscription, error) {
	businessType, err := normalizeBusinessType(input.BusinessType)
	if err != nil {
		return model.Subscription{}, err
	}
	plusRental := businessType == model.SubscriptionBusinessPlus
	cents, err := cycle.ParseYuanToCents(input.PriceYuan)
	if err != nil {
		return model.Subscription{}, fmt.Errorf("人均价格无效: %w", err)
	}
	var nextPriceCents *int64
	if strings.TrimSpace(input.NextPriceYuan) != "" {
		value, parseErr := cycle.ParseYuanToCents(input.NextPriceYuan)
		if parseErr != nil {
			return model.Subscription{}, fmt.Errorf("下周期价格无效: %w", parseErr)
		}
		nextPriceCents = &value
	}
	costCents := int64(0)
	if strings.TrimSpace(input.CostYuan) != "" {
		costCents, err = cycle.ParseYuanToCents(input.CostYuan)
		if err != nil {
			return model.Subscription{}, fmt.Errorf("成本价无效: %w", err)
		}
	}
	agencyFeeCents := int64(0)
	if !plusRental && strings.TrimSpace(input.AgencyFeeYuan) != "" {
		agencyFeeCents, err = cycle.ParseYuanToCents(input.AgencyFeeYuan)
		if err != nil {
			return model.Subscription{}, fmt.Errorf("中介费无效: %w", err)
		}
	}
	offsets := []int{}
	if !plusRental {
		offsets, err = cycle.ParseOffsets(input.NotifyOffsetsRaw)
		if err != nil {
			return model.Subscription{}, err
		}
	}
	customerEmail, err := normalizeCustomerEmail(input.CustomerEmail)
	if err != nil {
		return model.Subscription{}, err
	}
	if plusRental && customerEmail == "" {
		return model.Subscription{}, fmt.Errorf("Plus 出租必须填写出租账号邮箱")
	}
	customerWechat := strings.TrimSpace(input.CustomerWechat)
	if plusRental && customerWechat == "" {
		return model.Subscription{}, fmt.Errorf("Plus 出租必须填写客户微信，方便到期后人工联系续费")
	}

	name := strings.TrimSpace(input.Name)
	seatID := int64(0)
	accountID := int64(0)
	accountName := ""
	seatName := ""
	subscriptionType := "Plus 出租"
	if plusRental {
		if name == "" {
			return model.Subscription{}, fmt.Errorf("Plus 出租必须填写客户名称")
		}
	} else {
		seatID, err = service.resolveSeatID(input.AccountID, input.SeatID, existingSubscriptionID)
		if err != nil {
			return model.Subscription{}, err
		}
		seat, seatErr := service.Store.GetSeat(seatID)
		if seatErr != nil {
			if seatErr == sql.ErrNoRows {
				return model.Subscription{}, fmt.Errorf("车位不存在")
			}
			return model.Subscription{}, seatErr
		}
		if err := service.ensureSeatAvailable(seatID, existingSubscriptionID); err != nil {
			return model.Subscription{}, err
		}
		account, accountErr := service.Store.GetAccount(seat.AccountID)
		if accountErr != nil {
			return model.Subscription{}, accountErr
		}
		if account.BannedAt != "" {
			keepsExistingSeat := false
			if existingSubscriptionID > 0 {
				existing, existingErr := service.Store.GetSubscription(existingSubscriptionID)
				if existingErr == nil && existing.SeatID == seat.ID {
					keepsExistingSeat = true
				}
			}
			if !keepsExistingSeat {
				return model.Subscription{}, fmt.Errorf("该账号已封禁，不能再分配新用户")
			}
		}
		if strings.TrimSpace(input.CostYuan) == "" {
			costCents = account.CostCents
		}
		if name == "" {
			name, err = service.defaultSubscriptionName(account, existingSubscriptionID)
			if err != nil {
				return model.Subscription{}, err
			}
		}
		accountID = seat.AccountID
		accountName = account.Name
		seatName = seat.Name
		subscriptionType = account.Name
	}

	boardedAt := strings.TrimSpace(input.BoardedAt)
	if boardedAt == "" {
		boardedAt = cycle.FormatDate(service.now())
	} else {
		if _, err := time.ParseInLocation("2006-01-02", boardedAt, cycle.Location); err != nil {
			return model.Subscription{}, fmt.Errorf("上车日期无效，请使用 YYYY-MM-DD")
		}
	}
	cronExpr := strings.TrimSpace(input.CronExpr)
	if cycle.IsOneMonthRentalExpression(cronExpr) {
		if !plusRental {
			return model.Subscription{}, fmt.Errorf("仅 Plus 出租支持单月短租")
		}
		cronExpr = cycle.OneMonthRentalExpression
	}
	if _, err := cycle.ParseBillingSchedule(cronExpr, boardedAt); err != nil {
		return model.Subscription{}, err
	}
	return model.Subscription{
		Name:                name,
		BusinessType:        businessType,
		PricePerPersonCents: cents,
		NextPriceCents:      nextPriceCents,
		CostCents:           costCents,
		IsResale:            input.IsResale && !plusRental,
		AgencyFeeCents:      agencyFeeCents,
		CronExpr:            cronExpr,
		NotifyOffsets:       offsets,
		// Legacy column retained for schema compatibility; sends use global settings.
		Channels:         append([]string(nil), model.DefaultEnabledChannels...),
		Remark:           strings.TrimSpace(input.Remark),
		TradeURL:         strings.TrimSpace(input.TradeURL),
		CustomerEmail:    customerEmail,
		CustomerWechat:   customerWechat,
		SeatID:           seatID,
		AccountID:        accountID,
		AccountName:      accountName,
		SeatName:         seatName,
		SubscriptionType: subscriptionType,
		BoardedAt:        boardedAt,
	}, nil
}

func normalizeBusinessType(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", model.SubscriptionBusinessTeam:
		return model.SubscriptionBusinessTeam, nil
	case model.SubscriptionBusinessPlus:
		return model.SubscriptionBusinessPlus, nil
	default:
		return "", fmt.Errorf("不支持的业务类型")
	}
}

func isPlusSubscription(subscription model.Subscription) bool {
	return strings.EqualFold(strings.TrimSpace(subscription.BusinessType), model.SubscriptionBusinessPlus)
}

func isOneMonthRental(subscription model.Subscription) bool {
	return isPlusSubscription(subscription) && cycle.IsOneMonthRentalExpression(subscription.CronExpr)
}

func oneMonthRentalPeriod(subscription model.Subscription) (time.Time, time.Time, error) {
	if !isOneMonthRental(subscription) {
		return time.Time{}, time.Time{}, fmt.Errorf("该订阅不是单月短租")
	}
	startedAt, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(subscription.BoardedAt), cycle.Location)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid boarded_at: %w", err)
	}
	startedAt = cycle.StartOfDay(startedAt)
	return startedAt, startedAt.AddDate(0, 0, 30), nil
}

// countedAmountCents is the amount that contributes to dashboard total amount.
func countedAmountCents(subscription model.Subscription) int64 {
	if subscription.IsResale {
		return subscription.AgencyFeeCents
	}
	return subscription.PricePerPersonCents
}

// countedProfitCents is the amount that contributes to dashboard / card profit.
func countedProfitCents(subscription model.Subscription) int64 {
	if subscription.IsResale {
		return subscription.AgencyFeeCents
	}
	return subscription.PricePerPersonCents - subscription.CostCents
}

// billDefaultAmountCents is the default bill amount when marking a due as paid.
func billDefaultAmountCents(subscription model.Subscription) int64 {
	if subscription.IsResale {
		return subscription.AgencyFeeCents
	}
	return subscription.PricePerPersonCents
}

func billAmountCentsForDueDate(subscription model.Subscription, dueDate string) int64 {
	if subscription.IsResale {
		return subscription.AgencyFeeCents
	}
	if subscription.NextPriceCents != nil &&
		strings.TrimSpace(subscription.NextPriceEffectiveDueDate) != "" &&
		strings.TrimSpace(dueDate) >= strings.TrimSpace(subscription.NextPriceEffectiveDueDate) {
		return *subscription.NextPriceCents
	}
	return subscription.PricePerPersonCents
}

func nextPriceAppliesForDueDate(subscription model.Subscription, dueDate string) bool {
	return !subscription.IsResale &&
		subscription.NextPriceCents != nil &&
		strings.TrimSpace(subscription.NextPriceEffectiveDueDate) != "" &&
		strings.TrimSpace(dueDate) >= strings.TrimSpace(subscription.NextPriceEffectiveDueDate)
}

func billDefaultCostCents(subscription model.Subscription) int64 {
	if isPlusSubscription(subscription) && subscription.CostCents > 0 {
		return subscription.CostCents
	}
	return 0
}

func normalizeCustomerEmail(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	address, err := mail.ParseAddress(raw)
	if err != nil {
		return "", fmt.Errorf("客户邮箱无效")
	}
	return strings.TrimSpace(address.Address), nil
}

func (service *SubscriptionService) defaultSubscriptionName(account model.Account, existingSubscriptionID int64) (string, error) {
	used, err := service.Store.CountActiveSubscriptionsByAccount(account.ID)
	if err != nil {
		return "", err
	}
	index := used + 1
	if existingSubscriptionID > 0 {
		// Editing without a name: keep ordinal stable relative to current occupancy.
		index = used
		if index < 1 {
			index = 1
		}
	}
	return fmt.Sprintf("%s-%d", strings.TrimSpace(account.Name), index), nil
}

// resolveSeatID picks an explicit seat or the first free seat under the account.
func (service *SubscriptionService) resolveSeatID(accountID, seatID, existingSubscriptionID int64) (int64, error) {
	if seatID > 0 {
		if accountID <= 0 {
			return seatID, nil
		}
		seat, err := service.Store.GetSeat(seatID)
		if err != nil {
			if err == sql.ErrNoRows {
				return 0, fmt.Errorf("车位不存在")
			}
			return 0, err
		}
		if seat.AccountID != accountID {
			// Account changed in the form; ignore the stale seat and pick a free one.
			return service.pickFreeSeat(accountID, existingSubscriptionID)
		}
		return seatID, nil
	}
	if accountID <= 0 {
		return 0, fmt.Errorf("请选择所属账号")
	}
	return service.pickFreeSeat(accountID, existingSubscriptionID)
}

// pickFreeSeat returns the first free seat under accountID (treating existingSubscriptionID as free).
func (service *SubscriptionService) pickFreeSeat(accountID, existingSubscriptionID int64) (int64, error) {
	freeSeats, err := service.Store.ListFreeSeats(accountID, existingSubscriptionID)
	if err != nil {
		return 0, err
	}
	if len(freeSeats) == 0 {
		return 0, fmt.Errorf("该账号没有空闲车位")
	}
	return freeSeats[0].ID, nil
}

// ensureSeatAvailable rejects seats already occupied by another active subscription.
func (service *SubscriptionService) ensureSeatAvailable(seatID int64, ignoreSubscriptionID int64) error {
	occupant, err := service.Store.GetActiveSubscriptionBySeatID(seatID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if ignoreSubscriptionID > 0 && occupant.ID == ignoreSubscriptionID {
		return nil
	}
	return fmt.Errorf("该车位已有活跃订阅")
}

func normalizeChannels(channels []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(channels))
	for _, channel := range channels {
		channel = strings.ToLower(strings.TrimSpace(channel))
		if channel != model.ChannelGotify && channel != model.ChannelIYUU && channel != model.ChannelSMTP {
			continue
		}
		if _, exists := seen[channel]; exists {
			continue
		}
		seen[channel] = struct{}{}
		result = append(result, channel)
	}
	return result
}

// GetNotifyTemplate returns the global operator template body.
func (service *SubscriptionService) GetNotifyTemplate() (string, error) {
	return service.Store.GetSetting(model.SettingNotifyTemplate)
}

// SaveNotifyTemplate validates and stores the global operator template.
func (service *SubscriptionService) SaveNotifyTemplate(body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Errorf("模板不能为空")
	}
	if _, err := template.New("notify").Parse(body); err != nil {
		return fmt.Errorf("模板语法错误: %w", err)
	}
	return service.Store.SetSetting(model.SettingNotifyTemplate, body)
}

// GetCustomerEmailTemplate returns the customer email template body.
func (service *SubscriptionService) GetCustomerEmailTemplate() (string, error) {
	raw, err := service.Store.GetSetting(model.SettingCustomerEmailTemplate)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.DefaultCustomerEmailTemplate, nil
		}
		return "", err
	}
	if strings.TrimSpace(raw) == "" {
		return model.DefaultCustomerEmailTemplate, nil
	}
	return raw, nil
}

// SaveCustomerEmailTemplate validates and stores the customer email template.
func (service *SubscriptionService) SaveCustomerEmailTemplate(body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Errorf("客户邮件模板不能为空")
	}
	if _, err := template.New("customer_email").Parse(body); err != nil {
		return fmt.Errorf("客户邮件模板语法错误: %w", err)
	}
	return service.Store.SetSetting(model.SettingCustomerEmailTemplate, body)
}

// GetEnabledChannels returns the global notify channel selection.
// Missing or blank settings fall back to defaults; an explicit empty list is preserved.
func (service *SubscriptionService) GetEnabledChannels() ([]string, error) {
	raw, err := service.Store.GetSetting(model.SettingEnabledChannels)
	if err != nil {
		if err == sql.ErrNoRows {
			return append([]string(nil), model.DefaultEnabledChannels...), nil
		}
		return nil, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return append([]string(nil), model.DefaultEnabledChannels...), nil
	}
	var channels []string
	if err := json.Unmarshal([]byte(raw), &channels); err != nil {
		return nil, fmt.Errorf("decode enabled channels: %w", err)
	}
	return normalizeChannels(channels), nil
}

// SaveEnabledChannels validates and stores the global notify channel selection.
// An empty selection is allowed (notifications will not be sent until a channel is enabled).
func (service *SubscriptionService) SaveEnabledChannels(channels []string) error {
	normalized := normalizeChannels(channels)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	return service.Store.SetSetting(model.SettingEnabledChannels, string(encoded))
}

// GetRedeemPageSettings returns the public redemption-page copy and contact details.
func (service *SubscriptionService) GetRedeemPageSettings() (model.RedeemPageSettings, error) {
	raw, err := service.Store.GetSetting(model.SettingRedeemPageSettings)
	if err != nil {
		if err == sql.ErrNoRows {
			return model.DefaultRedeemPageSettings, nil
		}
		return model.RedeemPageSettings{}, err
	}
	if strings.TrimSpace(raw) == "" {
		return model.DefaultRedeemPageSettings, nil
	}

	var settings model.RedeemPageSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return model.RedeemPageSettings{}, fmt.Errorf("decode redeem page settings: %w", err)
	}
	return redeemPageSettingsWithDefaults(settings), nil
}

// SaveRedeemPageSettings validates and stores public redemption-page settings.
func (service *SubscriptionService) SaveRedeemPageSettings(input model.RedeemPageSettings) error {
	settings, err := normalizeRedeemPageSettings(input)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return service.Store.SetSetting(model.SettingRedeemPageSettings, string(encoded))
}

func redeemPageSettingsWithDefaults(input model.RedeemPageSettings) model.RedeemPageSettings {
	defaults := model.DefaultRedeemPageSettings
	if strings.TrimSpace(input.AnnouncementTitle) == "" {
		input.AnnouncementTitle = defaults.AnnouncementTitle
	}
	if strings.TrimSpace(input.AnnouncementIntro) == "" {
		input.AnnouncementIntro = defaults.AnnouncementIntro
	}
	if len(trimNonEmptyStrings(input.AnnouncementItems)) == 0 {
		input.AnnouncementItems = append([]string(nil), defaults.AnnouncementItems...)
	}
	if strings.TrimSpace(input.SupportTitle) == "" {
		input.SupportTitle = defaults.SupportTitle
	}
	if strings.TrimSpace(input.SupportDescription) == "" {
		input.SupportDescription = defaults.SupportDescription
	}
	if strings.TrimSpace(input.SupportContactLabel) == "" {
		input.SupportContactLabel = defaults.SupportContactLabel
	}
	return input
}

func normalizeRedeemPageSettings(input model.RedeemPageSettings) (model.RedeemPageSettings, error) {
	input = redeemPageSettingsWithDefaults(input)

	var err error
	if input.AnnouncementTitle, err = trimRequiredLimited("兑换页公告标题", input.AnnouncementTitle, maxRedeemAnnouncementTitleLength); err != nil {
		return model.RedeemPageSettings{}, err
	}
	if input.AnnouncementIntro, err = trimRequiredLimited("兑换页公告说明", input.AnnouncementIntro, maxRedeemAnnouncementIntroLength); err != nil {
		return model.RedeemPageSettings{}, err
	}

	items := trimNonEmptyStrings(input.AnnouncementItems)
	if len(items) == 0 {
		items = append([]string(nil), model.DefaultRedeemPageSettings.AnnouncementItems...)
	}
	if len(items) > maxRedeemAnnouncementItemCount {
		return model.RedeemPageSettings{}, fmt.Errorf("兑换页公告最多 %d 条", maxRedeemAnnouncementItemCount)
	}
	for _, item := range items {
		if len([]rune(item)) > maxRedeemAnnouncementItemLength {
			return model.RedeemPageSettings{}, fmt.Errorf("兑换页公告单条最多 %d 个字", maxRedeemAnnouncementItemLength)
		}
	}
	input.AnnouncementItems = items

	if input.SupportTitle, err = trimRequiredLimited("客服标题", input.SupportTitle, maxRedeemSupportTitleLength); err != nil {
		return model.RedeemPageSettings{}, err
	}
	if input.SupportDescription, err = trimLimited("客服说明", input.SupportDescription, maxRedeemSupportDescriptionLength); err != nil {
		return model.RedeemPageSettings{}, err
	}
	if input.SupportContactLabel, err = trimRequiredLimited("客服联系方式标签", input.SupportContactLabel, maxRedeemSupportContactLabelLength); err != nil {
		return model.RedeemPageSettings{}, err
	}
	if input.SupportWechatID, err = trimLimited("客服微信号", input.SupportWechatID, maxRedeemSupportWechatIDLength); err != nil {
		return model.RedeemPageSettings{}, err
	}
	input.SupportQRCodeDataURL = strings.TrimSpace(input.SupportQRCodeDataURL)
	if input.SupportQRCodeDataURL != "" {
		if len(input.SupportQRCodeDataURL) > maxRedeemQRCodeDataURLLength {
			return model.RedeemPageSettings{}, fmt.Errorf("客服二维码图片太大，请压缩到 1MB 左右后再上传")
		}
		if !isAllowedImageDataURL(input.SupportQRCodeDataURL) {
			return model.RedeemPageSettings{}, fmt.Errorf("客服二维码仅支持 PNG、JPG 或 WebP 图片")
		}
	}
	return input, nil
}

func trimNonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func isAllowedImageDataURL(value string) bool {
	return strings.HasPrefix(value, "data:image/png;base64,") ||
		strings.HasPrefix(value, "data:image/jpeg;base64,") ||
		strings.HasPrefix(value, "data:image/jpg;base64,") ||
		strings.HasPrefix(value, "data:image/webp;base64,")
}

// RenderMessage renders the operator notify template for a subscription.
func (service *SubscriptionService) RenderMessage(subscription model.Subscription) (string, error) {
	templateBody, err := service.GetNotifyTemplate()
	if err != nil {
		return "", err
	}
	dueAt, err := service.nextDueForTemplate(subscription)
	if err != nil {
		return "", err
	}
	return service.renderTemplateForDueDate("notify", templateBody, subscription, dueAt)
}

// RenderCustomerEmail renders the customer email template for a subscription.
func (service *SubscriptionService) RenderCustomerEmail(subscription model.Subscription) (string, error) {
	templateBody, err := service.GetCustomerEmailTemplate()
	if err != nil {
		return "", err
	}
	dueAt, err := service.nextDueForTemplate(subscription)
	if err != nil {
		return "", err
	}
	return service.renderTemplateForDueDate("customer_email", templateBody, subscription, dueAt)
}

func (service *SubscriptionService) renderMessageForDueDate(subscription model.Subscription, dueAt time.Time) (string, error) {
	templateBody, err := service.GetNotifyTemplate()
	if err != nil {
		return "", err
	}
	return service.renderTemplateForDueDate("notify", templateBody, subscription, dueAt)
}

func (service *SubscriptionService) renderCustomerEmailForDueDate(subscription model.Subscription, dueAt time.Time) (string, error) {
	templateBody, err := service.GetCustomerEmailTemplate()
	if err != nil {
		return "", err
	}
	return service.renderTemplateForDueDate("customer_email", templateBody, subscription, dueAt)
}

func parseTemplateDueDate(value string) (time.Time, error) {
	dueAt, err := time.ParseInLocation("2006-01-02", value, cycle.Location)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid due date %q", value)
	}
	return dueAt, nil
}

func (service *SubscriptionService) nextDueForTemplate(subscription model.Subscription) (time.Time, error) {
	if isOneMonthRental(subscription) {
		_, periodEnd, err := oneMonthRentalPeriod(subscription)
		return periodEnd, err
	}
	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		return time.Time{}, err
	}
	return schedule.NextDue(service.now()), nil
}

func (service *SubscriptionService) renderTemplateForDueDate(
	name string,
	templateBody string,
	subscription model.Subscription,
	dueAt time.Time,
) (string, error) {
	parsed, err := template.New(name).Parse(templateBody)
	if err != nil {
		return "", err
	}
	daysUntilDue := cycle.DaysRemaining(dueAt, service.now())
	amountDueCents := billAmountCentsForDueDate(subscription, cycle.FormatDate(dueAt))
	data := model.TemplateData{
		Name:             templateDisplayName(subscription),
		SubscriptionName: subscription.Name,
		CustomerEmail:    subscription.CustomerEmail,
		CustomerWechat:   subscription.CustomerWechat,
		AccountName:      displayAccountName(subscription),
		SeatName:         subscription.SeatName,
		PricePerPerson:   cycle.FormatCents(amountDueCents),
		AmountDue:        cycle.FormatCents(amountDueCents),
		CycleDesc:        cycle.DescribeCron(subscription.CronExpr),
		NextDueDate:      cycle.FormatDate(dueAt),
		DaysUntilDue:     daysUntilDue,
		DueInText:        dueInText(daysUntilDue),
		Remark:           subscription.Remark,
		TradeURL:         subscription.TradeURL,
	}
	var buffer bytes.Buffer
	if err := parsed.Execute(&buffer, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buffer.String()), nil
}

func dueInText(days int) string {
	switch {
	case days > 0:
		return fmt.Sprintf("还有 %d 天到期", days)
	case days == 0:
		return "今天到期"
	default:
		return fmt.Sprintf("已过期 %d 天", -days)
	}
}

// PreviewTemplate renders an arbitrary (possibly unsaved) template body with live
// sample data: the first active subscription, or synthetic values when none exist.
func (service *SubscriptionService) PreviewTemplate(name string, templateBody string) (rendered string, sampleName string, err error) {
	subscription, err := service.sampleSubscription()
	if err != nil {
		return "", "", err
	}
	dueAt, err := service.nextDueForTemplate(subscription)
	if err != nil {
		return "", "", err
	}
	rendered, err = service.renderTemplateForDueDate(name, templateBody, subscription, dueAt)
	if err != nil {
		return "", "", err
	}
	return rendered, templateDisplayName(subscription), nil
}

func (service *SubscriptionService) sampleSubscription() (model.Subscription, error) {
	subscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		return model.Subscription{}, err
	}
	if len(subscriptions) > 0 {
		return subscriptions[0], nil
	}
	return model.Subscription{
		Name:                "示例订阅",
		PricePerPersonCents: 2000,
		CronExpr:            "interval:30d",
		BoardedAt:           cycle.FormatDate(service.now()),
		CustomerEmail:       "customer@example.com",
		AccountName:         "owner@example.com",
		SeatName:            "seat1",
		Remark:              "示例备注",
		TradeURL:            "https://example.com/order/123",
	}, nil
}

func templateDisplayName(subscription model.Subscription) string {
	if customerEmail := strings.TrimSpace(subscription.CustomerEmail); customerEmail != "" {
		return customerEmail
	}
	if name := strings.TrimSpace(subscription.Name); name != "" {
		return name
	}
	return displayAccountName(subscription)
}

// SendCustomerEmail sends the customer template to the subscription's customer email.
func (service *SubscriptionService) SendCustomerEmail(ctx context.Context, subscriptionID int64) error {
	subscription, err := service.Store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("订阅不存在")
		}
		return err
	}
	if isPlusSubscription(subscription) {
		return fmt.Errorf("Plus 出租仅使用微信人工续费，不发送客户邮件")
	}
	if strings.TrimSpace(subscription.CustomerEmail) == "" {
		return fmt.Errorf("该订阅未填写客户邮箱")
	}
	if err := service.ensureNoPendingAfterSales(subscriptionID, "发送提醒"); err != nil {
		return err
	}
	if !service.Config.SMTPConfigured() {
		return fmt.Errorf("SMTP 未配置（需 host/port/from/username/password）")
	}
	message, err := service.RenderCustomerEmail(subscription)
	if err != nil {
		return err
	}
	sender := notify.SMTPSender{
		Host:     service.Config.SMTPHost,
		Port:     service.Config.SMTPPort,
		Username: service.Config.SMTPUsername,
		Password: service.Config.SMTPPassword,
		From:     service.Config.SMTPFrom,
	}
	title := customerEmailSubject(subscription)
	return sender.SendTo(ctx, []string{subscription.CustomerEmail}, title, message)
}

func customerEmailSubject(subscription model.Subscription) string {
	return "拼车续费提醒 · " + templateDisplayName(subscription)
}

// Export builds a JSON-serializable export payload.
func (service *SubscriptionService) Export() (model.ExportPayload, error) {
	subscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		return model.ExportPayload{}, err
	}
	templateBody, err := service.GetNotifyTemplate()
	if err != nil {
		return model.ExportPayload{}, err
	}
	customerTemplateBody, err := service.GetCustomerEmailTemplate()
	if err != nil {
		return model.ExportPayload{}, err
	}
	enabledChannels, err := service.GetEnabledChannels()
	if err != nil {
		return model.ExportPayload{}, err
	}
	redeemPageSettings, err := service.GetRedeemPageSettings()
	if err != nil {
		return model.ExportPayload{}, err
	}
	accounts, err := service.Store.ListAccounts()
	if err != nil {
		return model.ExportPayload{}, err
	}

	exportAccounts := make([]model.ExportAccount, 0, len(accounts))
	for _, account := range accounts {
		seats, seatErr := service.Store.ListSeatsByAccount(account.ID)
		if seatErr != nil {
			return model.ExportPayload{}, seatErr
		}
		exportSeats := make([]model.ExportSeat, 0, len(seats))
		for _, seat := range seats {
			exportSeats = append(exportSeats, model.ExportSeat{
				ID:   seat.ID,
				Name: seat.Name,
			})
		}
		exportAccounts = append(exportAccounts, model.ExportAccount{
			ID:                   account.ID,
			Name:                 account.Name,
			Remark:               account.Remark,
			PaymentMethod:        account.PaymentMethod,
			Email:                account.Email,
			SpaceName:            account.SpaceName,
			OpenedAt:             account.OpenedAt,
			CostCents:            account.CostCents,
			CostYuan:             cycle.FormatCents(account.CostCents),
			TotalCostCents:       account.TotalCostCents,
			TotalCostYuan:        cycle.FormatCents(account.TotalCostCents),
			ZeroRenewalNextMonth: account.ZeroRenewalNextMonth,
			Seats:                exportSeats,
		})
	}

	payload := model.ExportPayload{
		ExportedAt:            cycle.Now().Format(time.RFC3339),
		NotifyTemplate:        templateBody,
		CustomerEmailTemplate: customerTemplateBody,
		EnabledChannels:       enabledChannels,
		RedeemPageSettings:    redeemPageSettings,
		Accounts:              exportAccounts,
		Subscriptions:         make([]model.ExportSubscription, 0, len(subscriptions)),
	}
	for _, subscription := range subscriptions {
		profitCents := countedProfitCents(subscription)
		accountName := displayAccountName(subscription)
		nextPriceYuan := ""
		if subscription.NextPriceCents != nil {
			nextPriceYuan = cycle.FormatCents(*subscription.NextPriceCents)
		}
		payload.Subscriptions = append(payload.Subscriptions, model.ExportSubscription{
			ID:                        subscription.ID,
			Name:                      subscription.Name,
			BusinessType:              subscription.BusinessType,
			PricePerPersonCents:       subscription.PricePerPersonCents,
			PricePerPersonYuan:        cycle.FormatCents(subscription.PricePerPersonCents),
			NextPriceCents:            subscription.NextPriceCents,
			NextPriceYuan:             nextPriceYuan,
			NextPriceEffectiveDueDate: subscription.NextPriceEffectiveDueDate,
			CostCents:                 subscription.CostCents,
			CostYuan:                  cycle.FormatCents(subscription.CostCents),
			IsResale:                  subscription.IsResale,
			AgencyFeeCents:            subscription.AgencyFeeCents,
			AgencyFeeYuan:             cycle.FormatCents(subscription.AgencyFeeCents),
			ProfitCents:               profitCents,
			ProfitYuan:                cycle.FormatCents(profitCents),
			CronExpr:                  subscription.CronExpr,
			NotifyOffsets:             subscription.NotifyOffsets,
			// Legacy per-subscription field; prefer EnabledChannels on the payload root.
			Channels:         subscription.Channels,
			Remark:           subscription.Remark,
			TradeURL:         subscription.TradeURL,
			CustomerEmail:    subscription.CustomerEmail,
			CustomerWechat:   subscription.CustomerWechat,
			SeatID:           subscription.SeatID,
			SeatName:         subscription.SeatName,
			AccountID:        subscription.AccountID,
			AccountName:      accountName,
			SubscriptionType: accountName,
			CreatedAt:        subscription.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:        subscription.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}
	return payload, nil
}

// TestNotify immediately sends to globally enabled channels.
func (service *SubscriptionService) TestNotify(ctx context.Context, subscriptionID int64) error {
	subscription, err := service.Store.GetSubscription(subscriptionID)
	if err != nil {
		return err
	}
	if err := service.ensureNoPendingAfterSales(subscriptionID, "发送通知"); err != nil {
		return err
	}
	message, err := service.RenderMessage(subscription)
	if err != nil {
		return err
	}
	return service.sendToEnabledChannels(ctx, "拼车收钱（测试）", message, subscriptionID)
}

// TestEnabledChannels sends a fixed test message to globally enabled channels.
func (service *SubscriptionService) TestEnabledChannels(ctx context.Context) error {
	message := "这是一条拼车通知渠道测试消息。若收到本条，说明当前启用渠道可用。"
	return service.sendToEnabledChannels(ctx, "拼车通知渠道测试", message, 0)
}

// PreviewCustomerEmail returns the filled customer-email template for confirmation UI.
func (service *SubscriptionService) PreviewCustomerEmail(subscriptionID int64) (to string, subject string, body string, err error) {
	subscription, err := service.Store.GetSubscriptionIncludingArchived(subscriptionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", "", fmt.Errorf("订阅不存在")
		}
		return "", "", "", err
	}
	if isPlusSubscription(subscription) {
		return "", "", "", fmt.Errorf("Plus 出租仅使用微信人工续费，不发送客户邮件")
	}
	if strings.TrimSpace(subscription.CustomerEmail) == "" {
		return "", "", "", fmt.Errorf("该订阅未填写客户邮箱")
	}
	body, err = service.RenderCustomerEmail(subscription)
	if err != nil {
		return "", "", "", err
	}
	return subscription.CustomerEmail, customerEmailSubject(subscription), body, nil
}

func (service *SubscriptionService) sendToEnabledChannels(ctx context.Context, title string, message string, subscriptionID int64) error {
	enabledChannels, err := service.GetEnabledChannels()
	if err != nil {
		return err
	}
	if len(enabledChannels) == 0 {
		return fmt.Errorf("请先在设置中启用至少一个通知渠道")
	}

	var failures []string
	for _, channel := range enabledChannels {
		sender, ok := service.Notify.Get(channel)
		if !ok {
			failures = append(failures, channel+": not configured")
			if subscriptionID > 0 {
				_ = service.Store.InsertTestNotificationLog(subscriptionID, channel, model.NotificationStatusFailed, "channel not configured")
			}
			continue
		}
		if err := sender.Send(ctx, title, message); err != nil {
			failures = append(failures, channel+": "+err.Error())
			if subscriptionID > 0 {
				_ = service.Store.InsertTestNotificationLog(subscriptionID, channel, model.NotificationStatusFailed, err.Error())
			}
			continue
		}
		if subscriptionID > 0 {
			_ = service.Store.InsertTestNotificationLog(subscriptionID, channel, model.NotificationStatusSuccess, "")
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

// ProcessDueNotifications plans and sends due scheduled notifications.
// Positive offsets email customers via SMTP; due-day unpaid items notify the operator via IYUU.
func (service *SubscriptionService) ProcessDueNotifications(ctx context.Context) error {
	if _, err := service.Store.RestoreExpiredCancellationRequests(service.now()); err != nil {
		return err
	}
	subscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		return err
	}
	now := service.now()
	today := cycle.FormatDate(now)

	for _, subscription := range subscriptions {
		pendingCount, err := service.Store.CountPendingAfterSalesCasesBySubscription(subscription.ID)
		if err != nil {
			return err
		}
		if pendingCount > 0 {
			continue
		}
		if err := service.planSubscription(ctx, subscription, now, today); err != nil {
			return err
		}
	}

	pendingLogs, err := service.Store.ListRetryableNotifications(now)
	if err != nil {
		return err
	}

	type digestGroup struct {
		channel string
		date    string
		logs    []model.NotificationLog
	}
	groups := make([]*digestGroup, 0)
	groupIndex := map[string]int{}

	for _, logEntry := range pendingLogs {
		if !notificationSendDateMatches(logEntry, now, today) {
			continue
		}
		expectedChannel, ok := scheduledChannelForOffset(logEntry.OffsetDays)
		if !ok || logEntry.Channel != expectedChannel {
			_ = service.Store.MarkNotificationFailure(
				logEntry.ID,
				logEntry.AttemptCount,
				"scheduled channel no longer used",
				nil,
				true,
			)
			continue
		}
		key := logEntry.Channel + "|" + cycle.FormatDate(now)
		index, ok := groupIndex[key]
		if !ok {
			index = len(groups)
			groupIndex[key] = index
			groups = append(groups, &digestGroup{
				channel: logEntry.Channel,
				date:    cycle.FormatDate(now),
				logs:    nil,
			})
		}
		groups[index].logs = append(groups[index].logs, logEntry)
	}

	for _, group := range groups {
		if err := service.attemptScheduledSend(ctx, group.channel, group.logs); err != nil {
			continue
		}
	}
	return nil
}

func (service *SubscriptionService) planSubscription(
	ctx context.Context,
	subscription model.Subscription,
	now time.Time,
	today string,
) error {
	_ = ctx
	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		return nil
	}

	candidates := make([]time.Time, 0, 2)
	if isOneMonthRental(subscription) {
		_, periodEnd, periodErr := oneMonthRentalPeriod(subscription)
		if periodErr != nil {
			return nil
		}
		candidates = append(candidates, periodEnd)
	} else {
		if lastDue, found := schedule.LastDue(now); found {
			candidates = append(candidates, lastDue)
		}
		nextDue := schedule.NextDue(now)
		candidates = append(candidates, nextDue)
	}

	for _, dueAt := range candidates {
		dueDate := cycle.FormatDate(dueAt)
		paid, err := service.Store.IsDuePaid(subscription.ID, dueDate)
		if err != nil {
			return err
		}
		if paid {
			continue
		}
		if err := service.planScheduledNotification(subscription.ID, dueAt, now, today, 0, model.ChannelIYUU); err != nil {
			return err
		}
		if isPlusSubscription(subscription) {
			continue
		}
		for _, offsetDays := range subscription.NotifyOffsets {
			if offsetDays <= 0 {
				continue
			}
			sendAt := cycle.SendAt(dueAt, offsetDays)
			if sendAt.After(now) || cycle.FormatDate(sendAt) != today {
				continue
			}
			if err := service.planScheduledNotification(subscription.ID, dueAt, now, today, offsetDays, model.ChannelSMTP); err != nil {
				return err
			}
		}
	}
	return nil
}

func (service *SubscriptionService) planScheduledNotification(
	subscriptionID int64,
	dueAt time.Time,
	now time.Time,
	today string,
	offsetDays int,
	channel string,
) error {
	sendAt := cycle.SendAt(dueAt, offsetDays)
	if sendAt.After(now) || cycle.FormatDate(sendAt) != today {
		return nil
	}
	logEntry, err := service.Store.UpsertPendingNotification(
		subscriptionID,
		cycle.FormatDate(dueAt),
		offsetDays,
		channel,
		model.NotificationKindScheduled,
	)
	if err != nil {
		return err
	}
	if logEntry.Status == model.NotificationStatusSuccess {
		return nil
	}
	if logEntry.Status == model.NotificationStatusFailed {
		return nil
	}
	return nil
}

func scheduledChannelForOffset(offsetDays int) (string, bool) {
	if offsetDays == 0 {
		return model.ChannelIYUU, true
	}
	if offsetDays > 0 {
		return model.ChannelSMTP, true
	}
	return "", false
}

func notificationSendDateMatches(logEntry model.NotificationLog, now time.Time, today string) bool {
	dueAt, err := time.ParseInLocation("2006-01-02", logEntry.DueDate, cycle.Location)
	if err != nil {
		return false
	}
	sendAt := cycle.SendAt(dueAt, logEntry.OffsetDays)
	return !sendAt.After(now) && cycle.FormatDate(sendAt) == today
}

func (service *SubscriptionService) attemptScheduledSend(ctx context.Context, channel string, logEntries []model.NotificationLog) error {
	if channel == model.ChannelSMTP {
		return service.attemptCustomerEmailSends(ctx, logEntries)
	}
	return service.attemptDigestSend(ctx, channel, logEntries)
}

func (service *SubscriptionService) attemptCustomerEmailSends(ctx context.Context, logEntries []model.NotificationLog) error {
	sender, ok := service.Notify.Get(model.ChannelSMTP)

	var failures []string
	for _, logEntry := range logEntries {
		if logEntry.Status == model.NotificationStatusSuccess || logEntry.Status == model.NotificationStatusFailed {
			continue
		}
		if logEntry.AttemptCount >= maxAttempts {
			_ = service.Store.MarkNotificationFailure(logEntry.ID, logEntry.AttemptCount, logEntry.LastError, nil, true)
			continue
		}
		paid, err := service.Store.IsDuePaid(logEntry.SubscriptionID, logEntry.DueDate)
		if err != nil {
			return err
		}
		if paid {
			continue
		}
		subscription, err := service.Store.GetSubscription(logEntry.SubscriptionID)
		if err != nil {
			if err == sql.ErrNoRows {
				_ = service.Store.MarkNotificationFailure(logEntry.ID, logEntry.AttemptCount, "subscription missing", nil, true)
				continue
			}
			return err
		}
		if isPlusSubscription(subscription) {
			_ = service.Store.MarkNotificationFailure(
				logEntry.ID,
				logEntry.AttemptCount+1,
				"Plus rentals do not send customer email",
				nil,
				true,
			)
			continue
		}
		if !ok {
			logEntry.AttemptCount++
			_ = service.failWithRetry(logEntry, "smtp is not configured")
			failures = append(failures, "smtp is not configured")
			continue
		}
		customerEmail := strings.TrimSpace(subscription.CustomerEmail)
		if customerEmail == "" {
			logEntry.AttemptCount = logEntry.AttemptCount + 1
			_ = service.failWithRetry(logEntry, "customer email missing")
			failures = append(failures, "customer email missing")
			continue
		}
		dueAt, err := parseTemplateDueDate(logEntry.DueDate)
		if err != nil {
			logEntry.AttemptCount = logEntry.AttemptCount + 1
			_ = service.failWithRetry(logEntry, err.Error())
			failures = append(failures, err.Error())
			continue
		}
		message, err := service.renderCustomerEmailForDueDate(subscription, dueAt)
		if err != nil {
			logEntry.AttemptCount = logEntry.AttemptCount + 1
			_ = service.failWithRetry(logEntry, err.Error())
			failures = append(failures, err.Error())
			continue
		}
		if err := sendCustomerSMTP(ctx, sender, customerEmail, customerEmailSubject(subscription), message); err != nil {
			logEntry.AttemptCount = logEntry.AttemptCount + 1
			_ = service.failWithRetry(logEntry, err.Error())
			failures = append(failures, err.Error())
			continue
		}
		_ = service.Store.MarkNotificationSuccess(logEntry.ID, logEntry.AttemptCount+1)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

type smtpAddressedSender interface {
	SendTo(ctx context.Context, recipients []string, title string, message string) error
}

func sendCustomerSMTP(ctx context.Context, sender notify.Sender, recipient string, title string, message string) error {
	if addressed, ok := sender.(smtpAddressedSender); ok {
		return addressed.SendTo(ctx, []string{recipient}, title, message)
	}
	return sender.Send(ctx, title, message)
}

func (service *SubscriptionService) attemptDigestSend(ctx context.Context, channel string, logEntries []model.NotificationLog) error {
	sender, ok := service.Notify.Get(channel)
	if !ok {
		for _, logEntry := range logEntries {
			logEntry.AttemptCount = logEntry.AttemptCount + 1
			_ = service.failWithRetry(logEntry, "channel not configured")
		}
		return fmt.Errorf("channel not configured")
	}

	type digestItem struct {
		logEntry     model.NotificationLog
		subscription model.Subscription
		message      string
	}
	items := make([]digestItem, 0, len(logEntries))
	for _, logEntry := range logEntries {
		if logEntry.Status == model.NotificationStatusSuccess || logEntry.Status == model.NotificationStatusFailed {
			continue
		}
		if logEntry.AttemptCount >= maxAttempts {
			_ = service.Store.MarkNotificationFailure(logEntry.ID, logEntry.AttemptCount, logEntry.LastError, nil, true)
			continue
		}
		paid, err := service.Store.IsDuePaid(logEntry.SubscriptionID, logEntry.DueDate)
		if err != nil {
			return err
		}
		if paid {
			continue
		}
		subscription, err := service.Store.GetSubscription(logEntry.SubscriptionID)
		if err != nil {
			if err == sql.ErrNoRows {
				_ = service.Store.MarkNotificationFailure(logEntry.ID, logEntry.AttemptCount, "subscription missing", nil, true)
				continue
			}
			return err
		}
		dueAt, err := parseTemplateDueDate(logEntry.DueDate)
		if err != nil {
			logEntry.AttemptCount = logEntry.AttemptCount + 1
			_ = service.failWithRetry(logEntry, err.Error())
			continue
		}
		message, err := service.renderMessageForDueDate(subscription, dueAt)
		if err != nil {
			logEntry.AttemptCount = logEntry.AttemptCount + 1
			_ = service.failWithRetry(logEntry, err.Error())
			continue
		}
		items = append(items, digestItem{
			logEntry:     logEntry,
			subscription: subscription,
			message:      message,
		})
	}
	if len(items) == 0 {
		return nil
	}

	seenSubscription := map[int64]struct{}{}
	parts := make([]string, 0)
	for _, item := range items {
		if _, exists := seenSubscription[item.subscription.ID]; exists {
			continue
		}
		seenSubscription[item.subscription.ID] = struct{}{}
		parts = append(parts, item.message)
	}

	body := buildDigestBody(parts)
	title := digestTitle(len(parts))

	if err := sender.Send(ctx, title, body); err != nil {
		for _, item := range items {
			logEntry := item.logEntry
			logEntry.AttemptCount = logEntry.AttemptCount + 1
			_ = service.failWithRetry(logEntry, err.Error())
		}
		return err
	}
	for _, item := range items {
		_ = service.Store.MarkNotificationSuccess(item.logEntry.ID, item.logEntry.AttemptCount+1)
	}
	return nil
}

// buildDigestBody joins rendered Team renewal and Plus renewal-rental messages.
func buildDigestBody(parts []string) string {
	return strings.Join(parts, "\n\n----------\n\n")
}

func digestTitle(count int) string {
	if count <= 1 {
		return "订阅到期收款"
	}
	return fmt.Sprintf("订阅到期收款（%d 条）", count)
}

func (service *SubscriptionService) failWithRetry(logEntry model.NotificationLog, lastError string) error {
	attemptCount := logEntry.AttemptCount
	if attemptCount == 0 {
		attemptCount = 1
	}
	if attemptCount >= maxAttempts {
		return service.Store.MarkNotificationFailure(logEntry.ID, attemptCount, lastError, nil, true)
	}
	// Exponential backoff: 1m, 2m, 4m, 8m, 16m based on attempt number.
	delayMinutes := 1 << (attemptCount - 1)
	if delayMinutes > 16 {
		delayMinutes = 16
	}
	nextRetry := service.now().UTC().Add(time.Duration(delayMinutes) * time.Minute)
	return service.Store.MarkNotificationFailure(logEntry.ID, attemptCount, lastError, &nextRetry, false)
}
