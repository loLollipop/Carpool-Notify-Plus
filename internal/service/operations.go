package service

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

const operationsTaskLimit = 24

// OperationTask is one prioritized piece of work shown across the admin UI.
// Kind and object IDs stay structured so the client can localize labels and
// open the right action without parsing presentation text.
type OperationTask struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	Tone             string `json:"tone"`
	Priority         int    `json:"-"`
	SubscriptionID   int64  `json:"subscription_id"`
	AfterSalesCaseID int64  `json:"after_sales_case_id"`
	RedemptionID     int64  `json:"redemption_id"`
	AccountID        int64  `json:"account_id"`
	SeatID           int64  `json:"seat_id"`
	Name             string `json:"name"`
	CustomerEmail    string `json:"customer_email"`
	CustomerWechat   string `json:"customer_wechat"`
	AccountName      string `json:"account_name"`
	SeatName         string `json:"seat_name"`
	DueDate          string `json:"due_date"`
	DueAtLabel       string `json:"due_at_label"`
	DaysRemaining    int    `json:"days_remaining"`
	AmountYuan       string `json:"amount_yuan"`
	CycleDesc        string `json:"cycle_desc"`
	OneMonthRental   bool   `json:"one_month_rental"`
	Route            string `json:"route"`
	Unread           bool   `json:"unread"`
}

type OperationsUnreadSummary struct {
	DashboardCount  int `json:"dashboard_count"`
	CalendarCount   int `json:"calendar_count"`
	TeamCount       int `json:"team_count"`
	PlusCount       int `json:"plus_count"`
	RedemptionCount int `json:"redemption_count"`
	AccountCount    int `json:"account_count"`
	AfterSalesCount int `json:"after_sales_count"`
}

type OperationsCapacitySummary struct {
	AccountCount       int `json:"account_count"`
	SeatTotal          int `json:"seat_total"`
	SeatUsed           int `json:"seat_used"`
	SeatFree           int `json:"seat_free"`
	SeatFrozen         int `json:"seat_frozen"`
	SeatReleasing7Days int `json:"seat_releasing_7d"`
	UtilizationPercent int `json:"utilization_percent"`
}

type OperationsWorkSummary struct {
	UrgentCount             int    `json:"urgent_count"`
	OverdueCount            int    `json:"overdue_count"`
	OverdueAmountYuan       string `json:"overdue_amount_yuan"`
	Due7DaysCount           int    `json:"due_7d_count"`
	Due7DaysAmountYuan      string `json:"due_7d_amount_yuan"`
	TeamDueCount            int    `json:"team_due_count"`
	PlusDueCount            int    `json:"plus_due_count"`
	AccountRenewalCount     int    `json:"account_renewal_count"`
	PendingRedemptionCount  int    `json:"pending_redemption_count"`
	PendingAfterSalesCount  int    `json:"pending_after_sales_count"`
	FailedNotificationCount int    `json:"failed_notification_count"`
}

type OperationsGoalSummary struct {
	Name                        string `json:"name"`
	TargetProfitCents           int64  `json:"target_profit_cents"`
	CurrentProfitCents          int64  `json:"current_profit_cents"`
	RemainingProfitCents        int64  `json:"remaining_profit_cents"`
	ProgressPercent             int    `json:"progress_percent"`
	ProjectedMonthlyProfitCents int64  `json:"projected_monthly_profit_cents"`
	ProjectedDate               string `json:"projected_date"`
}

// OperationsOverview is the lightweight admin landing-page response. Detail
// lists remain behind their existing endpoints and load only after drill-down.
type OperationsOverview struct {
	GeneratedAt                 string                    `json:"generated_at"`
	Dashboard                   Dashboard                 `json:"dashboard"`
	Finance                     BillsSummary              `json:"finance"`
	ThisMonthPendingCount       int                       `json:"this_month_pending_count"`
	ThisMonthPendingAmountYuan  string                    `json:"this_month_pending_amount_yuan"`
	ProjectedMonthlyProfitCents int64                     `json:"projected_monthly_profit_cents"`
	ActiveRecurringCount        int                       `json:"active_recurring_count"`
	Capacity                    OperationsCapacitySummary `json:"capacity"`
	Work                        OperationsWorkSummary     `json:"work"`
	Goal                        *OperationsGoalSummary    `json:"goal"`
	Unread                      OperationsUnreadSummary   `json:"unread"`
	Notifications               []OperationTask           `json:"notifications"`
	Tasks                       []OperationTask           `json:"tasks"`
}

func (service *SubscriptionService) GetOperationsOverview() (OperationsOverview, error) {
	dashboard, err := service.ComputeDashboard()
	if err != nil {
		return OperationsOverview{}, err
	}
	billsPage, err := service.ListBillsPage()
	if err != nil {
		return OperationsOverview{}, err
	}
	subscriptions, err := service.ListView()
	if err != nil {
		return OperationsOverview{}, err
	}
	accounts, err := service.ListAccountsView()
	if err != nil {
		return OperationsOverview{}, err
	}
	afterSales, err := service.ListAfterSalesPage()
	if err != nil {
		return OperationsOverview{}, err
	}
	redemptions, err := service.ListRedemptionApplicationsView(model.RedemptionStatusPending)
	if err != nil {
		return OperationsOverview{}, err
	}

	now := service.now().In(cycle.Location)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, cycle.Location)
	calendar, err := service.CalendarMonth(month)
	if err != nil {
		return OperationsOverview{}, err
	}
	runRateCents, recurringCount, err := service.activeMonthlyProfitRunRate()
	if err != nil {
		return OperationsOverview{}, err
	}

	overview := OperationsOverview{
		GeneratedAt:                 now.Format(time.RFC3339),
		Dashboard:                   dashboard,
		Finance:                     billsPage.Summary,
		ThisMonthPendingCount:       calendar.PendingMonthCount,
		ThisMonthPendingAmountYuan:  calendar.PendingMonthAmountYuan,
		ProjectedMonthlyProfitCents: runRateCents,
		ActiveRecurringCount:        recurringCount,
		Tasks:                       make([]OperationTask, 0),
	}

	overview.Capacity = buildOperationsCapacity(accounts, now, &overview.Tasks)
	buildSubscriptionOperationTasks(subscriptions, &overview.Work, &overview.Tasks)
	buildRedemptionOperationTasks(redemptions, &overview.Tasks)
	buildAfterSalesOperationTasks(afterSales.Cases, &overview.Tasks)
	buildNotificationOperationTask(dashboard, &overview.Tasks)
	buildAccountRenewalOperationTasks(accounts, now, &overview.Work, &overview.Tasks)

	overview.Work.PendingRedemptionCount = len(redemptions)
	overview.Work.PendingAfterSalesCount = afterSales.Summary.PendingCount + afterSales.Summary.ReviewCount
	overview.Work.FailedNotificationCount = dashboard.NotifyFailed30d
	overview.Work.UrgentCount = overview.Work.OverdueCount +
		overview.Work.PendingRedemptionCount +
		overview.Work.PendingAfterSalesCount +
		overview.Work.FailedNotificationCount

	if goal, goalErr := service.Store.GetActiveBusinessGoal(); goalErr == nil {
		progress := service.buildGoalProgress(goal, dashboard.TotalProfitCents)
		forecast := service.buildProfitForecast(progress, runRateCents, recurringCount)
		overview.Goal = &OperationsGoalSummary{
			Name:                        goal.Name,
			TargetProfitCents:           goal.TargetProfitCents,
			CurrentProfitCents:          progress.CurrentProfitCents,
			RemainingProfitCents:        progress.RemainingProfitCents,
			ProgressPercent:             progress.ProgressPercent,
			ProjectedMonthlyProfitCents: forecast.Baseline.MonthlyProfitCents,
			ProjectedDate:               forecast.Baseline.ProjectedDate,
		}
	} else if goalErr != sql.ErrNoRows {
		return OperationsOverview{}, goalErr
	}

	sort.SliceStable(overview.Tasks, func(left, right int) bool {
		if overview.Tasks[left].Priority == overview.Tasks[right].Priority {
			if overview.Tasks[left].DueDate == overview.Tasks[right].DueDate {
				return overview.Tasks[left].ID < overview.Tasks[right].ID
			}
			return overview.Tasks[left].DueDate < overview.Tasks[right].DueDate
		}
		return overview.Tasks[left].Priority > overview.Tasks[right].Priority
	})
	acknowledged, err := service.Store.ListAcknowledgedOperationTaskIDs()
	if err != nil {
		return OperationsOverview{}, err
	}
	for index := range overview.Tasks {
		_, seen := acknowledged[overview.Tasks[index].ID]
		overview.Tasks[index].Unread = !seen
		if overview.Tasks[index].Unread {
			addUnreadOperationTask(&overview.Unread, overview.Tasks[index])
		}
	}
	// Tasks remain ordered by operational priority. Whether an item was viewed
	// must never push unresolved work below routine records.
	overview.Notifications = append([]OperationTask(nil), overview.Tasks...)
	if len(overview.Tasks) > operationsTaskLimit {
		overview.Tasks = overview.Tasks[:operationsTaskLimit]
	}
	return overview, nil
}

func addUnreadOperationTask(summary *OperationsUnreadSummary, task OperationTask) {
	switch task.Kind {
	case "team_due", "team_overdue":
		summary.CalendarCount++
		summary.TeamCount++
	case "plus_due", "plus_overdue":
		summary.CalendarCount++
		summary.PlusCount++
	case "redemption":
		summary.RedemptionCount++
	case "after_sales":
		summary.AfterSalesCount++
	case "seat_release", "account_renewal":
		summary.AccountCount++
	}
	if task.Tone == "critical" {
		summary.DashboardCount++
	}
}

// AcknowledgeOperationTasks persists well-formed IDs sent by the authenticated
// admin UI. It deliberately avoids rebuilding the expensive operations
// overview on every click.
func (service *SubscriptionService) AcknowledgeOperationTasks(taskIDs []string) (int, error) {
	matched := make([]string, 0, len(taskIDs))
	seen := make(map[string]struct{}, len(taskIDs))
	for _, rawTaskID := range taskIDs {
		taskID := strings.TrimSpace(rawTaskID)
		if !validOperationTaskID(taskID) {
			continue
		}
		if _, duplicate := seen[taskID]; duplicate {
			continue
		}
		seen[taskID] = struct{}{}
		matched = append(matched, taskID)
	}
	if len(matched) == 0 {
		return 0, nil
	}
	if err := service.Store.AcknowledgeOperationTasks(matched, service.now()); err != nil {
		return 0, err
	}
	return len(matched), nil
}

func validOperationTaskID(taskID string) bool {
	if taskID == "" || len(taskID) > 256 {
		return false
	}
	for _, prefix := range []string{
		"subscription:",
		"redemption:",
		"after-sales:",
		"notification-failures:",
		"seat-release:",
		"account-renewal:",
	} {
		if strings.HasPrefix(taskID, prefix) {
			return true
		}
	}
	return false
}

func buildOperationsCapacity(
	accounts []AccountView,
	now time.Time,
	tasks *[]OperationTask,
) OperationsCapacitySummary {
	summary := OperationsCapacitySummary{AccountCount: len(accounts)}
	for _, account := range accounts {
		summary.SeatTotal += account.SeatTotal
		summary.SeatUsed += account.SeatUsed
		for _, seat := range account.Seats {
			if !seat.Occupied && !seat.Frozen {
				summary.SeatFree++
			}
			if !seat.Frozen {
				continue
			}
			summary.SeatFrozen++
			frozenUntil, err := time.Parse(time.RFC3339, seat.FrozenUntil)
			if err != nil {
				continue
			}
			days := cycle.DaysRemaining(frozenUntil.In(cycle.Location), now)
			if days < 0 || days > 7 {
				continue
			}
			summary.SeatReleasing7Days++
			*tasks = append(*tasks, OperationTask{
				ID:            fmt.Sprintf("seat-release:%d:%s", seat.Seat.ID, cycle.FormatDate(frozenUntil.In(cycle.Location))),
				Kind:          "seat_release",
				Tone:          "info",
				Priority:      45 - days,
				AccountID:     account.Account.ID,
				SeatID:        seat.Seat.ID,
				Name:          seat.FrozenSubscriptionName,
				CustomerEmail: seat.FrozenCustomerEmail,
				AccountName:   account.Account.Name,
				SeatName:      seat.Seat.Name,
				DueDate:       cycle.FormatDate(frozenUntil.In(cycle.Location)),
				DueAtLabel:    seat.FrozenUntilLabel,
				DaysRemaining: days,
				Route:         "/accounts",
			})
		}
	}
	if summary.SeatTotal > 0 {
		summary.UtilizationPercent = (summary.SeatUsed*100 + summary.SeatTotal/2) / summary.SeatTotal
	}
	return summary
}

func buildSubscriptionOperationTasks(
	views []SubscriptionView,
	work *OperationsWorkSummary,
	tasks *[]OperationTask,
) {
	var overdueAmountCents int64
	var dueAmountCents int64
	for _, view := range views {
		if view.CancellationPending || view.DaysRemaining > 7 {
			continue
		}
		plus := isPlusSubscription(view.Subscription)
		if plus {
			work.PlusDueCount++
		} else {
			work.TeamDueCount++
		}
		kind := "team_due"
		route := fmt.Sprintf("/users?filter=due&subscription=%d", view.Subscription.ID)
		if plus {
			kind = "plus_due"
			query := strings.TrimSpace(view.Subscription.CustomerEmail)
			if query == "" {
				query = strings.TrimSpace(view.Subscription.CustomerWechat)
			}
			route = "/plus-rentals"
			if query != "" {
				route += "?q=" + query
			}
		}
		tone := "warning"
		priority := 70 - view.DaysRemaining
		if view.DaysRemaining < 0 {
			kind = strings.TrimSuffix(kind, "_due") + "_overdue"
			tone = "critical"
			priority = 100 - view.DaysRemaining
		}
		amountCents := billAmountCentsForDueDate(view.Subscription, view.NextDueDate)
		if view.DaysRemaining < 0 {
			work.OverdueCount++
			overdueAmountCents += amountCents
		} else {
			work.Due7DaysCount++
			dueAmountCents += amountCents
		}
		*tasks = append(*tasks, OperationTask{
			ID:             fmt.Sprintf("subscription:%d:%s", view.Subscription.ID, view.NextDueDate),
			Kind:           kind,
			Tone:           tone,
			Priority:       priority,
			SubscriptionID: view.Subscription.ID,
			Name:           view.Subscription.Name,
			CustomerEmail:  view.Subscription.CustomerEmail,
			CustomerWechat: view.Subscription.CustomerWechat,
			AccountName:    view.AccountName,
			SeatName:       view.SeatName,
			DueDate:        view.NextDueDate,
			DaysRemaining:  view.DaysRemaining,
			AmountYuan:     cycle.FormatCents(amountCents),
			CycleDesc:      view.CycleDesc,
			OneMonthRental: isOneMonthRental(view.Subscription),
			Route:          route,
		})
	}
	work.OverdueAmountYuan = cycle.FormatCents(overdueAmountCents)
	work.Due7DaysAmountYuan = cycle.FormatCents(dueAmountCents)
}

func buildRedemptionOperationTasks(views []RedemptionApplicationView, tasks *[]OperationTask) {
	for _, view := range views {
		application := view.Application
		*tasks = append(*tasks, OperationTask{
			ID:             fmt.Sprintf("redemption:%d", application.ID),
			Kind:           "redemption",
			Tone:           "critical",
			Priority:       95,
			RedemptionID:   application.ID,
			Name:           application.CustomerEmail,
			CustomerEmail:  application.CustomerEmail,
			CustomerWechat: application.CustomerContact,
			DueAtLabel:     view.CreatedAtLabel,
			Route:          "/redemptions",
		})
	}
}

func buildAfterSalesOperationTasks(views []AfterSalesCaseView, tasks *[]OperationTask) {
	for _, view := range views {
		caseItem := view.Case
		if caseItem.Status != model.AfterSalesStatusPending && caseItem.Status != model.AfterSalesStatusReview {
			continue
		}
		*tasks = append(*tasks, OperationTask{
			ID:               fmt.Sprintf("after-sales:%d", caseItem.ID),
			Kind:             "after_sales",
			Tone:             "critical",
			Priority:         92,
			SubscriptionID:   caseItem.SubscriptionID,
			AfterSalesCaseID: caseItem.ID,
			AccountID:        caseItem.AccountID,
			Name:             caseItem.CustomerEmail,
			CustomerEmail:    caseItem.CustomerEmail,
			CustomerWechat:   caseItem.CustomerWechat,
			AccountName:      caseItem.AccountName,
			DueDate:          caseItem.PeriodEnd,
			DueAtLabel:       view.ExpiresAtLabel,
			AmountYuan:       view.RefundAmountYuan,
			Route:            fmt.Sprintf("/after-sales?case=%d", caseItem.ID),
		})
	}
}

func buildNotificationOperationTask(dashboard Dashboard, tasks *[]OperationTask) {
	if dashboard.NotifyFailed30d <= 0 {
		return
	}
	latestFailureID := int64(0)
	for _, activity := range dashboard.NotificationActivity {
		if activity.Status == "failed" && activity.ID > latestFailureID {
			latestFailureID = activity.ID
		}
	}
	*tasks = append(*tasks, OperationTask{
		ID:       fmt.Sprintf("notification-failures:%d", latestFailureID),
		Kind:     "notification_failed",
		Tone:     "critical",
		Priority: 88,
		Name:     fmt.Sprintf("%d", dashboard.NotifyFailed30d),
		Route:    "/settings",
	})
}

func buildAccountRenewalOperationTasks(
	accounts []AccountView,
	now time.Time,
	work *OperationsWorkSummary,
	tasks *[]OperationTask,
) {
	today := cycle.StartOfDay(now)
	for _, view := range accounts {
		account := view.Account
		if account.BannedAt != "" || strings.TrimSpace(view.NextRenewalDate) == "" {
			continue
		}
		renewalAt, err := time.ParseInLocation("2006-01-02", view.NextRenewalDate, cycle.Location)
		if err != nil {
			continue
		}
		days := cycle.DaysRemaining(renewalAt, today)
		if days < 0 || days > 7 {
			continue
		}
		work.AccountRenewalCount++
		amountCents := account.CostCents
		if account.ZeroRenewalNextMonth {
			amountCents = 0
		}
		*tasks = append(*tasks, OperationTask{
			ID:            fmt.Sprintf("account-renewal:%d:%s", account.ID, cycle.FormatDate(renewalAt)),
			Kind:          "account_renewal",
			Tone:          "info",
			Priority:      55 - days,
			AccountID:     account.ID,
			Name:          account.Email,
			AccountName:   account.Name,
			DueDate:       cycle.FormatDate(renewalAt),
			DaysRemaining: days,
			AmountYuan:    cycle.FormatCents(amountCents),
			Route:         "/accounts",
		})
	}
}
