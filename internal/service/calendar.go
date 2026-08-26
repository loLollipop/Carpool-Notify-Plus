package service

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

// CalendarMonthView is the collection of due occurrences in one calendar month.
type CalendarMonthView struct {
	MonthValue    string `json:"month_value"`
	MonthLabel    string `json:"month_label"`
	PreviousMonth string `json:"previous_month"`
	NextMonth     string `json:"next_month"`
	CurrentMonth  string `json:"current_month"`
	// Occurrences is the right-side agenda focus list: unpaid in-month dues, plus
	// the latest in-month paid row when that subscription has no unpaid dues left
	// in the month. Does not inject next-period unpaid rows.
	Occurrences []CalendarOccurrenceView `json:"occurrences"`
	// PaidInMonthOccurrences lists in-month paid rows for the「已交费」filter only.
	PaidInMonthOccurrences []CalendarOccurrenceView `json:"paid_in_month_occurrences"`
	// ActionableOccurrences follows the same first-unpaid-period rule as the
	// dashboard queue. It includes overdue items and the next seven days even
	// when that window crosses a calendar-month boundary.
	ActionableOccurrences []CalendarOccurrenceView `json:"actionable_occurrences"`
	// ArchivedSubscriptions are 已下车 items for the agenda tab (not on the month grid).
	ArchivedSubscriptions []SubscriptionView `json:"archived_subscriptions"`
	Days                  []CalendarDayView  `json:"days"`
	// TotalCount / PaidCount summarize all dues in the month; PendingCount only
	// counts unpaid dues that are due today or overdue.
	TotalCount   int `json:"total_count"`
	PaidCount    int `json:"paid_count"`
	PendingCount int `json:"pending_count"`
	// PendingMonthCount / PendingMonthAmountYuan include every unpaid due in
	// the selected month, including future dues that PendingCount excludes.
	PendingMonthCount      int    `json:"pending_month_count"`
	PendingMonthAmountYuan string `json:"pending_month_amount_yuan"`
	ArchivedCount          int    `json:"archived_count"`
}

// CalendarDayView is one visible day in the month grid, including adjacent-month days.
type CalendarDayView struct {
	Date        string                   `json:"date"`
	DateLabel   string                   `json:"date_label"`
	DayNumber   int                      `json:"day_number"`
	InMonth     bool                     `json:"in_month"`
	IsWeekend   bool                     `json:"is_weekend"`
	IsToday     bool                     `json:"is_today"`
	Occurrences []CalendarOccurrenceView `json:"occurrences"`
}

// CalendarOccurrenceView is one subscription occurrence on its due date.
type CalendarOccurrenceView struct {
	SubscriptionID            int64    `json:"subscription_id"`
	Name                      string   `json:"name"`
	BusinessType              string   `json:"business_type"`
	DueDate                   string   `json:"due_date"`
	DayNumber                 int      `json:"day_number"`
	WeekdayLabel              string   `json:"weekday_label"`
	PriceYuan                 string   `json:"price_yuan"`
	CurrentPriceYuan          string   `json:"current_price_yuan"`
	NextPriceYuan             string   `json:"next_price_yuan"`
	NextPriceEffectiveDueDate string   `json:"next_price_effective_due_date"`
	AmountCents               int64    `json:"-"`
	CostYuan                  string   `json:"cost_yuan"`
	AgencyFeeYuan             string   `json:"agency_fee_yuan"`
	IsResale                  bool     `json:"is_resale"`
	ProfitYuan                string   `json:"profit_yuan"`
	CycleDesc                 string   `json:"cycle_desc"`
	ReminderLabel             string   `json:"reminder_label"`
	ChannelLabels             string   `json:"channel_labels"`
	Paid                      bool     `json:"paid"`
	AccountName               string   `json:"account_name"`
	SeatName                  string   `json:"seat_name"`
	AccountID                 int64    `json:"account_id"`
	SeatID                    int64    `json:"seat_id"`
	DaysRemaining             int      `json:"days_remaining"`
	TradeURL                  string   `json:"trade_url"`
	CronExpr                  string   `json:"cron_expr"`
	OffsetsText               string   `json:"offsets_text"`
	CustomerEmail             string   `json:"customer_email"`
	CustomerWechat            string   `json:"customer_wechat"`
	Channels                  []string `json:"channels"`
	Remark                    string   `json:"remark"`
	BoardedAt                 string   `json:"boarded_at"`
}

type dueOccurrenceKey struct {
	subscriptionID int64
	dueDate        string
}

// SetDuePaid marks or unmarks one subscription due date as paid (creates/deletes a bill).
// dueDate is the period start (cron occurrence that anchors this billing cycle).
func (service *SubscriptionService) SetDuePaid(subscriptionID int64, dueDate string, paid bool) error {
	subscription, err := service.Store.GetSubscription(subscriptionID)
	if err != nil {
		return err
	}
	action := "续费记账"
	if isPlusSubscription(subscription) {
		action = "续租记账"
	}
	if err := service.ensureNoPendingAfterSales(subscriptionID, action); err != nil {
		return err
	}
	if _, err := time.ParseInLocation("2006-01-02", dueDate, cycle.Location); err != nil {
		return fmt.Errorf("无效的到期日: %w", err)
	}
	if isOneMonthRental(subscription) {
		periodStart, _, periodErr := oneMonthRentalPeriod(subscription)
		if periodErr != nil {
			return periodErr
		}
		if strings.TrimSpace(dueDate) != cycle.FormatDate(periodStart) {
			return fmt.Errorf("单月短租只有首期租金，不能登记续租；到期后请确认结束出租")
		}
	}
	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		return err
	}
	ok, err := schedule.IsDueDate(dueDate)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s 不是该订阅的到期日", dueDate)
	}
	if err := service.Store.SetDuePaidForSubscription(
		subscription,
		dueDate,
		paid,
		billAmountCentsForDueDate(subscription, dueDate),
		billDefaultCostCents(subscription),
	); err != nil {
		if errors.Is(err, db.ErrBillHasAfterSalesCase) {
			return fmt.Errorf("该期账单已关联售后处理记录，不能取消缴费")
		}
		if errors.Is(err, db.ErrSubscriptionFinancialStateChanged) {
			return fmt.Errorf("订阅价格或状态已变化，请刷新后重新确认本期金额")
		}
		return err
	}
	return nil
}

// DuePeriodOption is one billing window between consecutive cron due dates.
// StartDate is the bill/paid key; EndDate is the next cron occurrence (period end, exclusive of the next bill).
type DuePeriodOption struct {
	StartDate          string `json:"start_date"`
	EndDate            string `json:"end_date"`
	Label              string `json:"label"`
	PriceYuan          string `json:"price_yuan"`
	PriceChangeApplies bool   `json:"price_change_applies"`
	Paid               bool   `json:"paid"`
	Preferred          bool   `json:"preferred"`
}

// ListDuePeriodOptions returns selectable billing periods around preferredStart
// (typically the agenda row due date). Each option is "start 至 end" from consecutive cron dues.
func (service *SubscriptionService) ListDuePeriodOptions(subscriptionID int64, preferredStart string) ([]DuePeriodOption, error) {
	subscription, err := service.Store.GetSubscription(subscriptionID)
	if err != nil {
		return nil, err
	}
	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		return nil, err
	}

	preferredStart = strings.TrimSpace(preferredStart)
	if preferredStart != "" {
		if _, parseErr := time.ParseInLocation("2006-01-02", preferredStart, cycle.Location); parseErr != nil {
			return nil, fmt.Errorf("无效的默认周期起点: %w", parseErr)
		}
	}
	if isOneMonthRental(subscription) {
		periodStart, periodEnd, periodErr := oneMonthRentalPeriod(subscription)
		if periodErr != nil {
			return nil, periodErr
		}
		startDate := cycle.FormatDate(periodStart)
		paid, paidErr := service.Store.IsDuePaid(subscriptionID, startDate)
		if paidErr != nil {
			return nil, paidErr
		}
		return []DuePeriodOption{{
			StartDate:          startDate,
			EndDate:            cycle.FormatDate(periodEnd),
			PriceYuan:          cycle.FormatCents(billAmountCentsForDueDate(subscription, startDate)),
			PriceChangeApplies: nextPriceAppliesForDueDate(subscription, startDate),
			Label:              startDate + " 至 " + cycle.FormatDate(periodEnd),
			Paid:               paid,
			Preferred:          preferredStart == "" || preferredStart == startDate,
		}}, nil
	}

	now := service.now().In(cycle.Location)
	// Walk from boarded_at when set; otherwise look back a year so late payments still appear.
	cursor := now.AddDate(-1, 0, 0).Add(-time.Minute)
	if subscription.BoardedAt != "" {
		if boardedDay, parseErr := time.ParseInLocation("2006-01-02", subscription.BoardedAt, cycle.Location); parseErr == nil {
			cursor = cycle.StartOfDay(boardedDay).Add(-time.Minute)
		}
	}
	if preferredStart != "" {
		if preferredDay, parseErr := time.ParseInLocation("2006-01-02", preferredStart, cycle.Location); parseErr == nil {
			// Include a few periods before the preferred row so the user can pick nearby cycles.
			lookback := preferredDay.AddDate(0, -6, 0)
			if !lookback.Before(cursor) {
				cursor = cycle.StartOfDay(lookback).Add(-time.Minute)
			}
		}
	}

	const maxDues = 36
	dueAts := make([]time.Time, 0, maxDues)
	for range maxDues {
		dueAt := schedule.NextDue(cursor)
		dueAts = append(dueAts, dueAt)
		// Enough future periods after preferred/now for prepay / next cycle.
		horizon := now.AddDate(0, 8, 0)
		if preferredStart != "" {
			if preferredDay, parseErr := time.ParseInLocation("2006-01-02", preferredStart, cycle.Location); parseErr == nil {
				if preferredHorizon := preferredDay.AddDate(0, 8, 0); preferredHorizon.After(horizon) {
					horizon = preferredHorizon
				}
			}
		}
		if dueAt.After(horizon) && len(dueAts) >= 3 {
			break
		}
		cursor = cycle.StartOfDay(dueAt).AddDate(0, 0, 1).Add(-time.Nanosecond)
	}

	if len(dueAts) < 2 {
		return nil, fmt.Errorf("该订阅暂无可选交费周期（需要至少两次 cron 到期日）")
	}

	options := make([]DuePeriodOption, 0, len(dueAts)-1)
	for index := 0; index < len(dueAts)-1; index++ {
		startDate := cycle.FormatDate(dueAts[index])
		endDate := cycle.FormatDate(dueAts[index+1])
		paid, paidErr := service.Store.IsDuePaid(subscriptionID, startDate)
		if paidErr != nil {
			return nil, paidErr
		}
		options = append(options, DuePeriodOption{
			StartDate:          startDate,
			EndDate:            endDate,
			PriceYuan:          cycle.FormatCents(billAmountCentsForDueDate(subscription, startDate)),
			PriceChangeApplies: nextPriceAppliesForDueDate(subscription, startDate),
			Label:              startDate + " 至 " + endDate,
			Paid:               paid,
			Preferred:          preferredStart != "" && startDate == preferredStart,
		})
	}

	// Prefer showing a focused window: unpaid near preferred, plus a few paid for context.
	if preferredStart != "" {
		focused := focusDuePeriodOptions(options, preferredStart)
		if len(focused) > 0 {
			return focused, nil
		}
	}
	return options, nil
}

// focusDuePeriodOptions keeps periods around preferredStart so the select stays short.
func focusDuePeriodOptions(options []DuePeriodOption, preferredStart string) []DuePeriodOption {
	preferredIndex := -1
	for index, option := range options {
		if option.StartDate == preferredStart {
			preferredIndex = index
			break
		}
	}
	if preferredIndex < 0 {
		// Fall back to first unpaid, else middle of list.
		for index, option := range options {
			if !option.Paid {
				preferredIndex = index
				break
			}
		}
		if preferredIndex < 0 {
			preferredIndex = len(options) / 2
		}
	}

	startIndex := preferredIndex - 2
	if startIndex < 0 {
		startIndex = 0
	}
	endIndex := preferredIndex + 6
	if endIndex > len(options) {
		endIndex = len(options)
	}
	// Ensure at least one unpaid option is visible when possible.
	window := options[startIndex:endIndex]
	hasUnpaid := false
	for _, option := range window {
		if !option.Paid {
			hasUnpaid = true
			break
		}
	}
	if !hasUnpaid {
		for index := preferredIndex + 1; index < len(options); index++ {
			if !options[index].Paid {
				endIndex = index + 1
				if endIndex-startIndex > 10 {
					startIndex = endIndex - 10
				}
				return options[startIndex:endIndex]
			}
		}
	}
	return window
}

// NextUnpaidDueDate returns the next unpaid cron due date strictly after afterDueDate
// (or from today when afterDueDate is empty), skipping 上车日 and already-paid occurrences.
func (service *SubscriptionService) NextUnpaidDueDate(subscriptionID int64, afterDueDate string) (string, error) {
	subscription, err := service.Store.GetSubscription(subscriptionID)
	if err != nil {
		return "", err
	}
	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		return "", err
	}
	if isOneMonthRental(subscription) {
		periodStart, _, periodErr := oneMonthRentalPeriod(subscription)
		if periodErr != nil {
			return "", periodErr
		}
		startDate := cycle.FormatDate(periodStart)
		paid, paidErr := service.Store.IsDuePaid(subscriptionID, startDate)
		if paidErr != nil {
			return "", paidErr
		}
		if paid || strings.TrimSpace(afterDueDate) >= startDate {
			return "", nil
		}
		return startDate, nil
	}
	now := service.now()
	cursor := cycle.StartOfDay(now).Add(-time.Minute)
	if afterDueDate != "" {
		if afterDay, parseErr := time.ParseInLocation("2006-01-02", afterDueDate, cycle.Location); parseErr == nil {
			// Walk strictly after the paid (or current) due so the agenda advances.
			cursor = cycle.StartOfDay(afterDay).AddDate(0, 0, 1).Add(-time.Nanosecond)
		}
	}
	return service.findNextUnpaidDue(subscription, schedule, cursor)
}

// CalendarMonth returns active subscription occurrences for the supplied month.
func (service *SubscriptionService) CalendarMonth(month time.Time) (CalendarMonthView, error) {
	subscriptionViews, err := service.ListView()
	if err != nil {
		return CalendarMonthView{}, err
	}
	return service.calendarMonth(month, subscriptionViews)
}

func (service *SubscriptionService) calendarMonth(
	month time.Time,
	subscriptionViews []SubscriptionView,
) (CalendarMonthView, error) {
	month = month.In(cycle.Location)
	monthStart := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, cycle.Location)
	monthEnd := monthStart.AddDate(0, 1, 0)
	// Week starts Monday (ISO-style). Go's Sunday=0 … Saturday=6.
	daysFromMonday := (int(monthStart.Weekday()) + 6) % 7
	gridStart := monthStart.AddDate(0, 0, -daysFromMonday)
	lastDay := monthEnd.AddDate(0, 0, -1)
	daysUntilSunday := (7 - int(lastDay.Weekday())) % 7
	gridEnd := lastDay.AddDate(0, 0, daysUntilSunday+1)

	subscriptions := make([]model.Subscription, 0, len(subscriptionViews))
	for _, view := range subscriptionViews {
		subscriptions = append(subscriptions, view.Subscription)
	}
	allocatedCosts, err := service.activeAllocatedCostCents(subscriptions)
	if err != nil {
		return CalendarMonthView{}, err
	}
	bills, err := service.Store.ListBills()
	if err != nil {
		return CalendarMonthView{}, err
	}
	billsByOccurrence := make(map[dueOccurrenceKey]model.Bill, len(bills))
	for _, bill := range bills {
		billsByOccurrence[dueOccurrenceKey{
			subscriptionID: bill.SubscriptionID,
			dueDate:        bill.DueDate,
		}] = bill
	}
	paidOccurrences, err := service.Store.ListPaidDueOccurrences(
		cycle.FormatDate(gridStart),
		cycle.FormatDate(gridEnd),
	)
	if err != nil {
		return CalendarMonthView{}, err
	}
	paidByOccurrence := make(map[dueOccurrenceKey]struct{}, len(paidOccurrences))
	for _, occurrence := range paidOccurrences {
		paidByOccurrence[dueOccurrenceKey{
			subscriptionID: occurrence.SubscriptionID,
			dueDate:        occurrence.DueDate,
		}] = struct{}{}
	}

	gridOccurrences := make([]CalendarOccurrenceView, 0)
	for _, subscription := range subscriptions {
		if isOneMonthRental(subscription) {
			periodStart, _, periodErr := oneMonthRentalPeriod(subscription)
			if periodErr != nil {
				return CalendarMonthView{}, periodErr
			}
			if !periodStart.Before(gridStart) && periodStart.Before(gridEnd) {
				dueDate := cycle.FormatDate(periodStart)
				_, paid := paidByOccurrence[dueOccurrenceKey{
					subscriptionID: subscription.ID,
					dueDate:        dueDate,
				}]
				gridOccurrences = append(gridOccurrences, service.buildOccurrenceView(
					subscription,
					periodStart,
					paid,
					billsByOccurrence[dueOccurrenceKey{subscriptionID: subscription.ID, dueDate: dueDate}],
					allocatedCosts[subscription.ID],
				))
			}
			continue
		}
		schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
		if err != nil {
			return CalendarMonthView{}, err
		}
		cursor := gridStart.Add(-time.Minute)
		for {
			dueAt := schedule.NextDue(cursor)
			if !dueAt.Before(gridEnd) {
				break
			}
			dueDate := cycle.FormatDate(dueAt)
			_, paid := paidByOccurrence[dueOccurrenceKey{
				subscriptionID: subscription.ID,
				dueDate:        dueDate,
			}]
			gridOccurrences = append(gridOccurrences, service.buildOccurrenceView(
				subscription,
				dueAt,
				paid,
				billsByOccurrence[dueOccurrenceKey{subscriptionID: subscription.ID, dueDate: dueDate}],
				allocatedCosts[subscription.ID],
			))
			cursor = cycle.StartOfDay(dueAt).AddDate(0, 0, 1).Add(-time.Nanosecond)
		}
	}

	sort.Slice(gridOccurrences, func(left int, right int) bool {
		if gridOccurrences[left].DueDate == gridOccurrences[right].DueDate {
			return gridOccurrences[left].Name < gridOccurrences[right].Name
		}
		return gridOccurrences[left].DueDate < gridOccurrences[right].DueDate
	})

	occurrencesByDate := make(map[string][]CalendarOccurrenceView)
	monthOccurrences := make([]CalendarOccurrenceView, 0, len(gridOccurrences))
	paidCount := 0
	pendingCount := 0
	pendingMonthCount := 0
	var pendingMonthAmountCents int64
	for _, occurrence := range gridOccurrences {
		occurrencesByDate[occurrence.DueDate] = append(occurrencesByDate[occurrence.DueDate], occurrence)
		if occurrence.DueDate < cycle.FormatDate(monthStart) || occurrence.DueDate >= cycle.FormatDate(monthEnd) {
			continue
		}
		monthOccurrences = append(monthOccurrences, occurrence)
		if occurrence.Paid {
			paidCount++
		} else {
			pendingMonthCount++
			pendingMonthAmountCents += occurrence.AmountCents
			if occurrence.DaysRemaining <= 0 {
				pendingCount++
			}
		}
	}

	agendaOccurrences, paidInMonthOccurrences := buildAgendaOccurrences(monthOccurrences)
	actionableOccurrences, err := service.buildActionableCalendarOccurrences(
		subscriptionViews,
		allocatedCosts,
	)
	if err != nil {
		return CalendarMonthView{}, err
	}

	today := cycle.FormatDate(service.now())
	days := make([]CalendarDayView, 0, int(gridEnd.Sub(gridStart).Hours()/24))
	for day := gridStart; day.Before(gridEnd); day = day.AddDate(0, 0, 1) {
		date := cycle.FormatDate(day)
		days = append(days, CalendarDayView{
			Date:        date,
			DateLabel:   fmt.Sprintf("%d月%d日 %s", day.Month(), day.Day(), calendarWeekdayLabel(day.Weekday())),
			DayNumber:   day.Day(),
			InMonth:     !day.Before(monthStart) && day.Before(monthEnd),
			IsWeekend:   day.Weekday() == time.Sunday || day.Weekday() == time.Saturday,
			IsToday:     date == today,
			Occurrences: occurrencesByDate[date],
		})
	}

	archivedViews, err := service.ListArchivedView()
	if err != nil {
		return CalendarMonthView{}, err
	}

	return CalendarMonthView{
		MonthValue:             monthStart.Format("2006-01"),
		MonthLabel:             fmt.Sprintf("%d 年 %d 月", monthStart.Year(), monthStart.Month()),
		PreviousMonth:          monthStart.AddDate(0, -1, 0).Format("2006-01"),
		NextMonth:              monthStart.AddDate(0, 1, 0).Format("2006-01"),
		CurrentMonth:           service.now().Format("2006-01"),
		Occurrences:            agendaOccurrences,
		PaidInMonthOccurrences: paidInMonthOccurrences,
		ActionableOccurrences:  actionableOccurrences,
		ArchivedSubscriptions:  archivedViews,
		Days:                   days,
		TotalCount:             len(monthOccurrences),
		PaidCount:              paidCount,
		PendingCount:           pendingCount,
		PendingMonthCount:      pendingMonthCount,
		PendingMonthAmountYuan: cycle.FormatCents(pendingMonthAmountCents),
		ArchivedCount:          len(archivedViews),
	}, nil
}

func (service *SubscriptionService) buildActionableCalendarOccurrences(
	subscriptionViews []SubscriptionView,
	allocatedCosts map[int64]int64,
) ([]CalendarOccurrenceView, error) {
	occurrences := make([]CalendarOccurrenceView, 0)
	for _, view := range subscriptionViews {
		if !isActionableSubscriptionDue(view) {
			continue
		}
		dueAt, err := time.ParseInLocation("2006-01-02", view.NextDueDate, cycle.Location)
		if err != nil {
			return nil, fmt.Errorf("invalid actionable due date %q: %w", view.NextDueDate, err)
		}
		occurrence := service.buildOccurrenceView(
			view.Subscription,
			dueAt,
			false,
			model.Bill{},
			allocatedCosts[view.Subscription.ID],
		)
		// Preserve the exact value that made this row actionable so the calendar
		// and dashboard cannot disagree if a request crosses midnight.
		occurrence.DaysRemaining = view.DaysRemaining
		occurrences = append(occurrences, occurrence)
	}
	sort.Slice(occurrences, func(left int, right int) bool {
		if occurrences[left].DaysRemaining == occurrences[right].DaysRemaining {
			if occurrences[left].DueDate == occurrences[right].DueDate {
				return occurrences[left].Name < occurrences[right].Name
			}
			return occurrences[left].DueDate < occurrences[right].DueDate
		}
		return occurrences[left].DaysRemaining < occurrences[right].DaysRemaining
	})
	return occurrences, nil
}

func (service *SubscriptionService) buildOccurrenceView(
	subscription model.Subscription,
	dueAt time.Time,
	paid bool,
	bill model.Bill,
	allocatedCostCents int64,
) CalendarOccurrenceView {
	dueDate := cycle.FormatDate(dueAt)
	amountCents := billAmountCentsForDueDate(subscription, dueDate)
	costCents := calendarPeriodCostCents(subscription, dueAt, allocatedCostCents)
	if paid && bill.ID > 0 {
		amountCents = bill.AmountCents
		if isPlusSubscription(subscription) {
			costCents = bill.CostCents
		}
	}
	nextPriceYuan := ""
	if subscription.NextPriceCents != nil {
		nextPriceYuan = cycle.FormatCents(*subscription.NextPriceCents)
	}
	profitCents := amountCents - costCents
	return CalendarOccurrenceView{
		SubscriptionID:            subscription.ID,
		Name:                      subscription.Name,
		BusinessType:              subscription.BusinessType,
		DueDate:                   dueDate,
		DayNumber:                 dueAt.In(cycle.Location).Day(),
		WeekdayLabel:              calendarWeekdayLabel(dueAt.Weekday()),
		PriceYuan:                 cycle.FormatCents(amountCents),
		CurrentPriceYuan:          cycle.FormatCents(subscription.PricePerPersonCents),
		NextPriceYuan:             nextPriceYuan,
		NextPriceEffectiveDueDate: subscription.NextPriceEffectiveDueDate,
		AmountCents:               amountCents,
		CostYuan:                  cycle.FormatCents(costCents),
		AgencyFeeYuan:             cycle.FormatCents(subscription.AgencyFeeCents),
		IsResale:                  subscription.IsResale,
		ProfitYuan:                cycle.FormatCents(profitCents),
		CycleDesc:                 cycle.DescribeCron(subscription.CronExpr),
		ReminderLabel:             calendarReminderLabel(subscription.NotifyOffsets),
		ChannelLabels:             scheduledNotificationLabelText(subscription),
		Paid:                      paid,
		AccountName:               displayAccountName(subscription),
		SeatName:                  subscription.SeatName,
		AccountID:                 subscription.AccountID,
		SeatID:                    subscription.SeatID,
		DaysRemaining:             cycle.DaysRemaining(dueAt, service.now()),
		TradeURL:                  subscription.TradeURL,
		CronExpr:                  subscription.CronExpr,
		OffsetsText:               cycle.FormatOffsets(subscription.NotifyOffsets),
		Channels:                  append([]string(nil), subscription.Channels...),
		Remark:                    subscription.Remark,
		CustomerEmail:             subscription.CustomerEmail,
		CustomerWechat:            subscription.CustomerWechat,
		BoardedAt:                 subscription.BoardedAt,
	}
}

func calendarPeriodCostCents(
	subscription model.Subscription,
	dueAt time.Time,
	monthlyAllocatedCostCents int64,
) int64 {
	if isPlusSubscription(subscription) || subscription.IsResale || monthlyAllocatedCostCents <= 0 {
		return monthlyAllocatedCostCents
	}
	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		return monthlyAllocatedCostCents
	}
	periodEnd := schedule.NextDue(cycle.StartOfDay(dueAt))
	periodDays := int64(math.Round(periodEnd.Sub(cycle.StartOfDay(dueAt)).Hours() / 24))
	if periodDays <= 0 {
		return monthlyAllocatedCostCents
	}
	// Account costs renew monthly while Team subscriptions may use weekly,
	// 30-day, or calendar-month periods. Prorating by 365/12 days keeps each
	// occurrence comparable and sums back to twelve monthly costs per year.
	numerator := monthlyAllocatedCostCents * periodDays * 12
	return (numerator + 365/2) / 365
}

// buildAgendaOccurrences builds the right-side list.
// Focus rows (Occurrences): all unpaid in-month dues, plus the latest in-month
// paid row when that subscription has no unpaid dues left in the month.
// PaidInMonthOccurrences holds every in-month paid row for the「已交费」tab.
func buildAgendaOccurrences(
	monthOccurrences []CalendarOccurrenceView,
) ([]CalendarOccurrenceView, []CalendarOccurrenceView) {
	focus := make([]CalendarOccurrenceView, 0, len(monthOccurrences))
	paidInMonth := make([]CalendarOccurrenceView, 0)
	hasUnpaidInMonth := make(map[int64]bool)
	latestPaidBySubscription := make(map[int64]CalendarOccurrenceView)

	for _, occurrence := range monthOccurrences {
		if occurrence.Paid {
			paidInMonth = append(paidInMonth, occurrence)
			if previous, exists := latestPaidBySubscription[occurrence.SubscriptionID]; !exists || occurrence.DueDate > previous.DueDate {
				latestPaidBySubscription[occurrence.SubscriptionID] = occurrence
			}
			continue
		}
		focus = append(focus, occurrence)
		hasUnpaidInMonth[occurrence.SubscriptionID] = true
	}

	for subscriptionID, paidOccurrence := range latestPaidBySubscription {
		if hasUnpaidInMonth[subscriptionID] {
			continue
		}
		focus = append(focus, paidOccurrence)
	}

	sort.Slice(focus, func(left int, right int) bool {
		if focus[left].DueDate == focus[right].DueDate {
			if focus[left].Paid != focus[right].Paid {
				return !focus[left].Paid && focus[right].Paid
			}
			return focus[left].Name < focus[right].Name
		}
		return focus[left].DueDate < focus[right].DueDate
	})
	return focus, paidInMonth
}

func (service *SubscriptionService) findNextUnpaidDue(
	subscription model.Subscription,
	schedule cycle.BillingSchedule,
	cursor time.Time,
) (string, error) {
	// Bound the walk so a fully-paid far future schedule cannot hang the request.
	for range 120 {
		dueAt := schedule.NextDue(cursor)
		dueDate := cycle.FormatDate(dueAt)
		paid, err := service.Store.IsDuePaid(subscription.ID, dueDate)
		if err != nil {
			return "", err
		}
		if !paid {
			return dueDate, nil
		}
		cursor = cycle.StartOfDay(dueAt).AddDate(0, 0, 1).Add(-time.Nanosecond)
	}
	return "", nil
}

func calendarWeekdayLabel(weekday time.Weekday) string {
	return []string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}[weekday]
}

func calendarReminderLabel(offsets []int) string {
	labels := make([]string, 0, len(offsets))
	for _, offset := range offsets {
		if offset == 0 {
			labels = append(labels, "当天")
			continue
		}
		labels = append(labels, fmt.Sprintf("提前 %d 天", offset))
	}
	return strings.Join(labels, "、")
}
