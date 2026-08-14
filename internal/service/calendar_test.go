package service_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/notify"
	"carpool-notify/internal/service"
)

func TestCalendarMonthListsMonthlyAndWeeklyDueOccurrences(t *testing.T) {
	subscriptionService := openTestService(t)
	createTestSubscription(t, subscriptionService, "流媒体家庭组", "0 0 * * 1")
	createTestSubscription(t, subscriptionService, "Cursor Pro 拼车", "0 0 15 * *")

	month := time.Date(2026, time.July, 1, 0, 0, 0, 0, cycle.Location)
	view, err := subscriptionService.CalendarMonth(month)
	if err != nil {
		t.Fatal(err)
	}

	got := make([]string, 0, len(view.Occurrences))
	for _, occurrence := range view.Occurrences {
		got = append(got, fmt.Sprintf("%s:%s", occurrence.DueDate, occurrence.Name))
	}
	want := []string{
		"2026-07-06:流媒体家庭组",
		"2026-07-13:流媒体家庭组",
		"2026-07-15:Cursor Pro 拼车",
		"2026-07-20:流媒体家庭组",
		"2026-07-27:流媒体家庭组",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("occurrences = %#v, want %#v", got, want)
	}
}

func TestCalendarMonthBuildsGridAndCurrentMonthSummary(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 10, 12, 0, 0, 0, cycle.Location)
	}
	weeklyID := createTestSubscription(t, subscriptionService, "流媒体家庭组", "0 0 * * 1")
	createTestSubscription(t, subscriptionService, "Cursor Pro 拼车", "0 0 15 * *")
	createTestSubscription(t, subscriptionService, "ChatGPT Team", "0 0 1 8 *")
	if err := subscriptionService.SetDuePaid(weeklyID, "2026-07-06", true); err != nil {
		t.Fatal(err)
	}

	month := time.Date(2026, time.July, 1, 0, 0, 0, 0, cycle.Location)
	view, err := subscriptionService.CalendarMonth(month)
	if err != nil {
		t.Fatal(err)
	}

	if len(view.Days) != 35 {
		t.Fatalf("calendar days = %d, want 35", len(view.Days))
	}
	// Week starts Monday: July 2026 grid is 2026-06-29 (Mon) … 2026-08-02 (Sun).
	if view.Days[0].Date != "2026-06-29" || view.Days[34].Date != "2026-08-02" {
		t.Fatalf("grid range = %s...%s, want 2026-06-29...2026-08-02", view.Days[0].Date, view.Days[34].Date)
	}
	var augustFirst service.CalendarDayView
	var foundAugustFirst bool
	for _, day := range view.Days {
		if day.Date == "2026-08-01" {
			augustFirst = day
			foundAugustFirst = true
			break
		}
	}
	if !foundAugustFirst {
		t.Fatal("expected 2026-08-01 in Monday-first grid")
	}
	if len(augustFirst.Occurrences) != 1 || augustFirst.Occurrences[0].Name != "ChatGPT Team" {
		t.Fatalf("2026-08-01 occurrences = %#v, want ChatGPT Team", augustFirst.Occurrences)
	}
	if view.TotalCount != 5 || view.PaidCount != 1 || view.PendingCount != 0 {
		t.Fatalf(
			"summary = total %d, paid %d, pending %d; want 5, 1, 0",
			view.TotalCount,
			view.PaidCount,
			view.PendingCount,
		)
	}
}

func TestCalendarPendingCountOnlyIncludesDueOrOverdueUnpaid(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 4, 12, 0, 0, 0, cycle.Location)
	}
	createTestSubscription(t, subscriptionService, "Due account", "0 0 3 * *")
	createTestSubscription(t, subscriptionService, "Future account", "0 0 19 * *")

	month := time.Date(2026, time.August, 1, 0, 0, 0, 0, cycle.Location)
	view, err := subscriptionService.CalendarMonth(month)
	if err != nil {
		t.Fatal(err)
	}

	if view.TotalCount != 2 || view.PaidCount != 0 || view.PendingCount != 1 {
		t.Fatalf(
			"summary = total %d, paid %d, pending %d; want 2, 0, 1",
			view.TotalCount,
			view.PaidCount,
			view.PendingCount,
		)
	}
	if len(view.Occurrences) != 2 {
		t.Fatalf("agenda occurrences = %d, want 2 so future dues remain available for prepay", len(view.Occurrences))
	}
	if view.PendingMonthCount != 2 || view.PendingMonthAmountYuan != "40.00" {
		t.Fatalf(
			"pending month = %d / %s, want 2 / 40.00",
			view.PendingMonthCount,
			view.PendingMonthAmountYuan,
		)
	}
}

func TestCalendarPendingMonthRevenueExcludesPaidAndOtherMonths(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 4, 12, 0, 0, 0, cycle.Location)
	}
	paidID := createTestSubscription(t, subscriptionService, "Paid account", "0 0 2 * *")
	createTestSubscription(t, subscriptionService, "Due account", "0 0 3 * *")
	createTestSubscription(t, subscriptionService, "Future account", "0 0 19 * *")
	createTestSubscription(t, subscriptionService, "Next month account", "0 0 5 9 *")
	if err := subscriptionService.SetDuePaid(paidID, "2026-08-02", true); err != nil {
		t.Fatal(err)
	}

	view, err := subscriptionService.CalendarMonth(
		time.Date(2026, time.August, 1, 0, 0, 0, 0, cycle.Location),
	)
	if err != nil {
		t.Fatal(err)
	}

	if view.PendingMonthCount != 2 || view.PendingMonthAmountYuan != "40.00" {
		t.Fatalf(
			"pending month = %d / %s, want 2 / 40.00",
			view.PendingMonthCount,
			view.PendingMonthAmountYuan,
		)
	}
}

func TestSubscriptionViewsShowScheduledNotificationRoutes(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 12, 12, 0, 0, 0, cycle.Location)
	}
	if err := subscriptionService.SaveEnabledChannels([]string{model.ChannelIYUU}); err != nil {
		t.Fatal(err)
	}
	subscriptionID := createTestSubscription(t, subscriptionService, "路由展示", "0 0 15 * *")

	views, err := subscriptionService.ListView()
	if err != nil {
		t.Fatal(err)
	}
	var foundView *service.SubscriptionView
	for index := range views {
		if views[index].Subscription.ID == subscriptionID {
			foundView = &views[index]
			break
		}
	}
	if foundView == nil {
		t.Fatal("subscription view not found")
	}
	wantLabels := []string{"SMTP 客户邮件", "IYUU 到期当天"}
	if !reflect.DeepEqual(foundView.ChannelLabels, wantLabels) {
		t.Fatalf("channel labels = %#v, want %#v", foundView.ChannelLabels, wantLabels)
	}

	month := time.Date(2026, time.July, 1, 0, 0, 0, 0, cycle.Location)
	calendarView, err := subscriptionService.CalendarMonth(month)
	if err != nil {
		t.Fatal(err)
	}
	for _, occurrence := range calendarView.Occurrences {
		if occurrence.SubscriptionID == subscriptionID {
			if occurrence.ChannelLabels != "SMTP 客户邮件 · IYUU 到期当天" {
				t.Fatalf("occurrence channel labels = %q", occurrence.ChannelLabels)
			}
			return
		}
	}
	t.Fatal("calendar occurrence not found")
}

func TestSubscriptionViewsIncludeCurrentCycleDays(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 25, 12, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(
		t,
		subscriptionService,
		"进度测试账号",
		"固定周期车位",
		"首期车位",
	)
	intervalID, err := subscriptionService.Create(service.CreateInput{
		Name:             "固定周期订阅",
		PriceYuan:        "20.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3,1,0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-20",
	})
	if err != nil {
		t.Fatal(err)
	}
	firstPeriodID, err := subscriptionService.Create(service.CreateInput{
		Name:             "首期订阅",
		PriceYuan:        "20.00",
		CronExpr:         "0 0 1 * *",
		NotifyOffsetsRaw: "3,1,0",
		SeatID:           seatIDs[1],
		BoardedAt:        "2026-07-25",
	})
	if err != nil {
		t.Fatal(err)
	}

	views, err := subscriptionService.ListView()
	if err != nil {
		t.Fatal(err)
	}
	viewsByID := make(map[int64]service.SubscriptionView, len(views))
	for _, view := range views {
		viewsByID[view.Subscription.ID] = view
	}

	intervalView := viewsByID[intervalID]
	if intervalView.CycleDays != 30 || intervalView.DaysRemaining != 25 {
		t.Fatalf(
			"interval progress = %d/%d days, want 25/30",
			intervalView.DaysRemaining,
			intervalView.CycleDays,
		)
	}
	firstPeriodView := viewsByID[firstPeriodID]
	if firstPeriodView.CycleDays != 7 || firstPeriodView.DaysRemaining != 7 {
		t.Fatalf(
			"first-period progress = %d/%d days, want 7/7",
			firstPeriodView.DaysRemaining,
			firstPeriodView.CycleDays,
		)
	}
}

func TestTeamSubscriptionViewTracksUnpaidDueAcrossMonthBoundary(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.July, 28, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "cross-month account", "seat1")
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "cross-month customer",
		PriceYuan:        "30.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3,1,0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	findView := func() service.SubscriptionView {
		views, listErr := subscriptionService.ListView()
		if listErr != nil {
			t.Fatal(listErr)
		}
		for _, view := range views {
			if view.Subscription.ID == subscriptionID {
				return view
			}
		}
		t.Fatal("subscription view not found")
		return service.SubscriptionView{}
	}

	view := findView()
	if view.NextDueDate != "2026-07-31" || view.DaysRemaining != 3 {
		t.Fatalf("upcoming unpaid due = %s / %d days, want 2026-07-31 / 3", view.NextDueDate, view.DaysRemaining)
	}

	now = time.Date(2026, time.August, 2, 12, 0, 0, 0, cycle.Location)
	view = findView()
	if view.NextDueDate != "2026-07-31" || view.DaysRemaining != -2 {
		t.Fatalf("overdue after month rollover = %s / %d days, want 2026-07-31 / -2", view.NextDueDate, view.DaysRemaining)
	}

	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-31", true); err != nil {
		t.Fatal(err)
	}
	view = findView()
	if view.NextDueDate != "2026-08-30" || view.DaysRemaining != 28 {
		t.Fatalf("next unpaid after renewal = %s / %d days, want 2026-08-30 / 28", view.NextDueDate, view.DaysRemaining)
	}
}

func TestTeamSubscriptionViewKeepsOldestUnpaidGapAcrossMultiplePeriods(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.September, 10, 12, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "multi-period overdue account", "seat1")
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name:             "multi-period overdue customer",
		PriceYuan:        "30.00",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "3,1,0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Paying a newer period out of order must not hide the older missed period.
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-08-30", true); err != nil {
		t.Fatal(err)
	}
	views, err := subscriptionService.ListView()
	if err != nil {
		t.Fatal(err)
	}
	view := views[0]
	if view.NextDueDate != "2026-07-31" || view.DaysRemaining != -41 {
		t.Fatalf("oldest unpaid gap = %s / %d days, want 2026-07-31 / -41", view.NextDueDate, view.DaysRemaining)
	}

	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-31", true); err != nil {
		t.Fatal(err)
	}
	views, err = subscriptionService.ListView()
	if err != nil {
		t.Fatal(err)
	}
	if views[0].NextDueDate != "2026-09-29" || views[0].DaysRemaining != 19 {
		t.Fatalf("next unpaid after closing gap = %s / %d days, want 2026-09-29 / 19", views[0].NextDueDate, views[0].DaysRemaining)
	}
}

func TestSetDuePaidOnlyChangesSelectedOccurrence(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 28, 12, 0, 0, 0, cycle.Location)
	}
	subscriptionID := createTestSubscription(t, subscriptionService, "流媒体家庭组", "0 0 * * 1")

	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-13", true); err != nil {
		t.Fatal(err)
	}

	month := time.Date(2026, time.July, 1, 0, 0, 0, 0, cycle.Location)
	view, err := subscriptionService.CalendarMonth(month)
	if err != nil {
		t.Fatal(err)
	}

	// Focus list omits in-month paid when same subscription still has unpaid dues.
	gotFocus := map[string]bool{}
	for _, occurrence := range view.Occurrences {
		gotFocus[occurrence.DueDate] = occurrence.Paid
	}
	wantFocus := map[string]bool{
		"2026-07-06": false,
		"2026-07-20": false,
		"2026-07-27": false,
	}
	if !reflect.DeepEqual(gotFocus, wantFocus) {
		t.Fatalf("focus agenda = %#v, want %#v", gotFocus, wantFocus)
	}
	gotPaidTab := map[string]bool{}
	for _, occurrence := range view.PaidInMonthOccurrences {
		gotPaidTab[occurrence.DueDate] = occurrence.Paid
	}
	if len(gotPaidTab) != 1 || !gotPaidTab["2026-07-13"] {
		t.Fatalf("paid tab rows = %#v, want 2026-07-13 paid", gotPaidTab)
	}

	// Month grid still shows the paid occurrence on its due date.
	var julyThirteenthPaid bool
	for _, day := range view.Days {
		if day.Date != "2026-07-13" {
			continue
		}
		for _, occurrence := range day.Occurrences {
			if occurrence.SubscriptionID == subscriptionID {
				julyThirteenthPaid = occurrence.Paid
			}
		}
	}
	if !julyThirteenthPaid {
		t.Fatal("expected 2026-07-13 grid cell to remain marked paid")
	}
	if view.PaidCount != 1 || view.PendingCount != 3 {
		t.Fatalf("summary paid/pending = %d/%d, want 1/3", view.PaidCount, view.PendingCount)
	}
}

func TestListDuePeriodOptionsBuildsRangesFromCron(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 11, 12, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "时段账号", "车位1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "时段订阅",
		PriceYuan:        "80.00",
		CronExpr:         "0 0 10 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-06-10",
	})
	if err != nil {
		t.Fatal(err)
	}

	periods, err := subscriptionService.ListDuePeriodOptions(subscriptionID, "2026-07-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(periods) == 0 {
		t.Fatal("expected at least one period option")
	}

	foundPreferred := false
	for _, period := range periods {
		if period.StartDate == "2026-07-10" {
			foundPreferred = true
			if period.EndDate != "2026-08-10" {
				t.Fatalf("preferred end = %q, want 2026-08-10", period.EndDate)
			}
			if period.Label != "2026-07-10 至 2026-08-10" {
				t.Fatalf("preferred label = %q", period.Label)
			}
			if !period.Preferred {
				t.Fatal("expected preferred flag on 2026-07-10 period")
			}
			if period.Paid {
				t.Fatal("preferred period should be unpaid before marking paid")
			}
		}
	}
	if !foundPreferred {
		t.Fatalf("periods = %#v, missing 2026-07-10 至 2026-08-10", periods)
	}

	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-10", true); err != nil {
		t.Fatal(err)
	}
	afterPaid, err := subscriptionService.ListDuePeriodOptions(subscriptionID, "2026-07-10")
	if err != nil {
		t.Fatal(err)
	}
	for _, period := range afterPaid {
		if period.StartDate == "2026-07-10" && !period.Paid {
			t.Fatal("expected 2026-07-10 period marked paid")
		}
	}
}

func TestAgendaShowsLatestPaidDueWhenMonthFullyPaid(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 11, 12, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "lunaechoio账号", "车位1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "lunaechoio",
		PriceYuan:        "80.00",
		CronExpr:         "0 0 10 * *",
		NotifyOffsetsRaw: "1,0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-10",
	})
	if err != nil {
		t.Fatal(err)
	}

	month := time.Date(2026, time.July, 1, 0, 0, 0, 0, cycle.Location)
	before, err := subscriptionService.CalendarMonth(month)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Occurrences) != 1 || before.Occurrences[0].DueDate != "2026-07-10" {
		t.Fatalf("before agenda = %#v, want single 2026-07-10", before.Occurrences)
	}

	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-10", true); err != nil {
		t.Fatal(err)
	}

	after, err := subscriptionService.CalendarMonth(month)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Occurrences) != 1 || after.Occurrences[0].DueDate != "2026-07-10" || !after.Occurrences[0].Paid {
		t.Fatalf("after focus agenda = %#v, want single paid 2026-07-10", after.Occurrences)
	}
	if len(after.PaidInMonthOccurrences) != 1 || after.PaidInMonthOccurrences[0].DueDate != "2026-07-10" || !after.PaidInMonthOccurrences[0].Paid {
		t.Fatalf("paid in month = %#v, want paid 2026-07-10", after.PaidInMonthOccurrences)
	}
	if after.PaidCount != 1 || after.PendingCount != 0 {
		t.Fatalf("July summary paid/pending = %d/%d, want 1/0", after.PaidCount, after.PendingCount)
	}

	nextDue, err := subscriptionService.NextUnpaidDueDate(subscriptionID, "2026-07-10")
	if err != nil {
		t.Fatal(err)
	}
	if nextDue != "2026-08-10" {
		t.Fatalf("NextUnpaidDueDate = %q, want 2026-08-10", nextDue)
	}
}

func TestAgendaShowsOnlyLatestPaidWhenMultipleInMonthPaid(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 28, 12, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "周付账号", "车位1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "周付订阅",
		PriceYuan:        "50.00",
		CronExpr:         "0 0 * * 1",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-06",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, dueDate := range []string{"2026-07-06", "2026-07-13", "2026-07-20", "2026-07-27"} {
		if err := subscriptionService.SetDuePaid(subscriptionID, dueDate, true); err != nil {
			t.Fatal(err)
		}
	}

	month := time.Date(2026, time.July, 1, 0, 0, 0, 0, cycle.Location)
	view, err := subscriptionService.CalendarMonth(month)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Occurrences) != 1 || view.Occurrences[0].DueDate != "2026-07-27" || !view.Occurrences[0].Paid {
		t.Fatalf("focus agenda = %#v, want single paid 2026-07-27", view.Occurrences)
	}
	if len(view.PaidInMonthOccurrences) != 4 {
		t.Fatalf("paid in month count = %d, want 4", len(view.PaidInMonthOccurrences))
	}
	if view.PaidCount != 4 || view.PendingCount != 0 {
		t.Fatalf("summary paid/pending = %d/%d, want 4/0", view.PaidCount, view.PendingCount)
	}
}

func TestSetDuePaidRejectsDateOutsideSubscriptionSchedule(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionID := createTestSubscription(t, subscriptionService, "Cursor Pro 拼车", "0 0 15 * *")

	err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-16", true)
	if err == nil {
		t.Fatal("SetDuePaid accepted a date that is not a due occurrence")
	}
}

func TestProcessDueNotificationsSkipsPaidOccurrence(t *testing.T) {
	subscriptionService := openTestService(t)
	fixedNow := time.Date(2026, time.July, 15, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return fixedNow }
	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: recorder}

	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "Cursor账号", "车位1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "Cursor Pro 拼车",
		PriceYuan:        "35.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2000-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-15", true); err != nil {
		t.Fatal(err)
	}

	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 0 {
		t.Fatalf("notification sends = %d, want 0", recorder.calls)
	}
	_, err = subscriptionService.Store.GetNotificationLog(
		subscriptionID,
		"2026-07-15",
		0,
		model.ChannelIYUU,
		model.NotificationKindScheduled,
	)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("notification log error = %v, want sql.ErrNoRows", err)
	}
}

func TestProcessDueNotificationsSkipsQueuedRetryAfterOccurrenceIsPaid(t *testing.T) {
	subscriptionService := openTestService(t)
	clock := time.Date(2026, time.July, 15, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return clock }
	failing := &failingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: failing}

	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "Cursor重试账号", "车位1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "Cursor Pro 拼车",
		PriceYuan:        "35.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2000-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if failing.calls != 1 {
		t.Fatalf("initial notification sends = %d, want 1", failing.calls)
	}

	if err := subscriptionService.SetDuePaid(subscriptionID, "2026-07-15", true); err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(2 * time.Minute)
	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: recorder}
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 0 {
		t.Fatalf("queued retry sends after paid = %d, want 0", recorder.calls)
	}
}

func TestProcessDueNotificationsSkipsMissedReminderDate(t *testing.T) {
	subscriptionService := openTestService(t)
	fixedNow := time.Date(2026, time.July, 16, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return fixedNow }
	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: recorder}

	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "Missed reminder account", "seat1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "Missed reminder",
		PriceYuan:        "35.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2000-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 0 {
		t.Fatalf("notification sends = %d, want 0 for missed reminder date", recorder.calls)
	}
	_, err = subscriptionService.Store.GetNotificationLog(
		subscriptionID,
		"2026-07-15",
		0,
		model.ChannelIYUU,
		model.NotificationKindScheduled,
	)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("notification log error = %v, want sql.ErrNoRows", err)
	}
}

func TestProcessDueNotificationsSendsUpcomingReminderDate(t *testing.T) {
	subscriptionService := openTestService(t)
	fixedNow := time.Date(2026, time.July, 12, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return fixedNow }
	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{SMTP: recorder}

	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "Upcoming reminder account", "seat1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "Upcoming reminder",
		PriceYuan:        "35.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "customer@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2000-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 1 {
		t.Fatalf("notification sends = %d, want 1 for configured reminder date", recorder.calls)
	}
	if !reflect.DeepEqual(recorder.lastRecipients, []string{"customer@example.com"}) {
		t.Fatalf("recipients = %#v, want customer email", recorder.lastRecipients)
	}
	if !strings.Contains(recorder.lastTitle, "拼车续费提醒") {
		t.Fatalf("title = %q, want renewal subject", recorder.lastTitle)
	}
	if !strings.Contains(recorder.lastBody, "联系管理员") {
		t.Fatalf("body = %q, want renewal contact text", recorder.lastBody)
	}
	logEntry, err := subscriptionService.Store.GetNotificationLog(
		subscriptionID,
		"2026-07-15",
		3,
		model.ChannelSMTP,
		model.NotificationKindScheduled,
	)
	if err != nil {
		t.Fatal(err)
	}
	if logEntry.Status != model.NotificationStatusSuccess {
		t.Fatalf("notification status = %q, want success", logEntry.Status)
	}
}

func TestProcessDueNotificationsSendsDueDayIYUUWithoutTodayOffset(t *testing.T) {
	subscriptionService := openTestService(t)
	fixedNow := time.Date(2026, time.July, 15, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return fixedNow }
	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: recorder}

	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "Due day account", "seat1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "Due day reminder",
		PriceYuan:        "35.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "3",
		CustomerEmail:    "customer@example.com",
		SeatID:           seatIDs[0],
		BoardedAt:        "2000-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 1 {
		t.Fatalf("notification sends = %d, want 1 due-day IYUU send", recorder.calls)
	}
	logEntry, err := subscriptionService.Store.GetNotificationLog(
		subscriptionID,
		"2026-07-15",
		0,
		model.ChannelIYUU,
		model.NotificationKindScheduled,
	)
	if err != nil {
		t.Fatal(err)
	}
	if logEntry.Status != model.NotificationStatusSuccess {
		t.Fatalf("notification status = %q, want success", logEntry.Status)
	}
}

func TestProcessDueNotificationsSchedulesRetryFromServiceClock(t *testing.T) {
	subscriptionService := openTestService(t)
	clock := time.Date(2000, time.July, 15, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return clock }
	failing := &failingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: failing}

	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "Cursor时钟账号", "车位1")
	_, err := subscriptionService.Create(service.CreateInput{
		Name:             "Cursor Pro 拼车",
		PriceYuan:        "35.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2000-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if failing.calls != 1 {
		t.Fatalf("initial notification sends = %d, want 1", failing.calls)
	}

	clock = clock.Add(2 * time.Minute)
	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: recorder}
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 1 {
		t.Fatalf("retry sends = %d, want 1", recorder.calls)
	}
}

func TestProcessDueNotificationsSkipsRetryAfterReminderDate(t *testing.T) {
	subscriptionService := openTestService(t)
	clock := time.Date(2026, time.July, 15, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return clock }
	failing := &failingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: failing}

	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "Expired retry account", "seat1")
	_, err := subscriptionService.Create(service.CreateInput{
		Name:             "Expired retry",
		PriceYuan:        "35.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2000-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if failing.calls != 1 {
		t.Fatalf("initial notification sends = %d, want 1", failing.calls)
	}

	clock = time.Date(2026, time.July, 16, 10, 0, 0, 0, cycle.Location)
	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: recorder}
	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 0 {
		t.Fatalf("expired retry sends = %d, want 0", recorder.calls)
	}
}

type recordingSender struct {
	calls          int
	lastRecipients []string
	lastTitle      string
	lastBody       string
}

func (sender *recordingSender) Send(_ context.Context, title string, body string) error {
	sender.calls++
	sender.lastTitle = title
	sender.lastBody = body
	return nil
}

func (sender *recordingSender) SendTo(_ context.Context, recipients []string, title string, body string) error {
	sender.calls++
	sender.lastRecipients = append([]string(nil), recipients...)
	sender.lastTitle = title
	sender.lastBody = body
	return nil
}

type failingSender struct {
	calls int
}

func (sender *failingSender) Send(_ context.Context, _ string, _ string) error {
	sender.calls++
	return errors.New("temporary failure")
}

func TestBoardedAtExcludesEarlierDueDatesFromCalendar(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 10, 12, 0, 0, 0, cycle.Location)
	}
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, "晚上车账号", "车位1")
	_, err := subscriptionService.Create(service.CreateInput{
		Name:             "晚上车",
		PriceYuan:        "20.00",
		CronExpr:         "0 0 * * 1",
		NotifyOffsetsRaw: "0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2026-07-15",
	})
	if err != nil {
		t.Fatal(err)
	}

	month := time.Date(2026, time.July, 1, 0, 0, 0, 0, cycle.Location)
	view, err := subscriptionService.CalendarMonth(month)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(view.Occurrences))
	for _, occurrence := range view.Occurrences {
		got = append(got, occurrence.DueDate)
	}
	// Mondays in July 2026 after boarded_at 2026-07-15: 20, 27.
	want := []string{"2026-07-20", "2026-07-27"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("due dates = %#v, want %#v", got, want)
	}
}

func TestArchivedSubscriptionsAppearInCalendarTab(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 10, 12, 0, 0, 0, cycle.Location)
	}
	subscriptionID := createTestSubscription(t, subscriptionService, "已下车的", "0 0 * * 1")
	if err := subscriptionService.Archive(subscriptionID); err != nil {
		t.Fatal(err)
	}

	month := time.Date(2026, time.July, 1, 0, 0, 0, 0, cycle.Location)
	view, err := subscriptionService.CalendarMonth(month)
	if err != nil {
		t.Fatal(err)
	}
	if view.ArchivedCount != 1 {
		t.Fatalf("archived count = %d, want 1", view.ArchivedCount)
	}
	if len(view.ArchivedSubscriptions) != 1 || view.ArchivedSubscriptions[0].Subscription.Name != "已下车的" {
		t.Fatalf("archived list = %#v", view.ArchivedSubscriptions)
	}
	for _, occurrence := range view.Occurrences {
		if occurrence.SubscriptionID == subscriptionID {
			t.Fatal("archived subscription must not appear as active occurrence")
		}
	}
}

func openTestService(t *testing.T) *service.SubscriptionService {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "carpool-test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	subscriptionService := &service.SubscriptionService{Store: store}
	// Keep legacy operator-channel settings available for tests that exercise
	// manual operator notifications; scheduled reminders now use fixed channels.
	if err := subscriptionService.SaveEnabledChannels([]string{model.ChannelGotify, model.ChannelIYUU}); err != nil {
		t.Fatal(err)
	}
	return subscriptionService
}

func createTestAccountWithSeats(t *testing.T, subscriptionService *service.SubscriptionService, accountName string, seatNames ...string) (accountID int64, seatIDs []int64) {
	t.Helper()
	if len(seatNames) == 0 {
		seatNames = []string{"车位1"}
	}
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      accountName,
		SeatNames: seatNames,
	})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := subscriptionService.Store.ListSeatsByAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	seatIDs = make([]int64, 0, len(seats))
	for _, seat := range seats {
		seatIDs = append(seatIDs, seat.ID)
	}
	return accountID, seatIDs
}

func createTestSubscription(t *testing.T, subscriptionService *service.SubscriptionService, name string, cronExpression string) int64 {
	t.Helper()
	_, seatIDs := createTestAccountWithSeats(t, subscriptionService, name+"账号", "车位1")
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             name,
		PriceYuan:        "20.00",
		CronExpr:         cronExpression,
		NotifyOffsetsRaw: "3,1,0",
		SeatID:           seatIDs[0],
		BoardedAt:        "2000-01-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	return subscriptionID
}

func TestProcessDueNotificationsMergesSameDaySends(t *testing.T) {
	subscriptionService := openTestService(t)
	fixedNow := time.Date(2026, time.July, 15, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return fixedNow }
	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: recorder}

	_, seatA := createTestAccountWithSeats(t, subscriptionService, "合并账号A", "车位1")
	_, seatB := createTestAccountWithSeats(t, subscriptionService, "合并账号B", "车位1")
	if _, err := subscriptionService.Create(service.CreateInput{
		Name:             "订阅甲",
		PriceYuan:        "10.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatA[0],
		BoardedAt:        "2000-01-01",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.Create(service.CreateInput{
		Name:             "订阅乙",
		PriceYuan:        "20.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "0",
		SeatID:           seatB[0],
		BoardedAt:        "2000-01-01",
	}); err != nil {
		t.Fatal(err)
	}

	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 1 {
		t.Fatalf("notification sends = %d, want 1 merged send", recorder.calls)
	}
	if !strings.Contains(recorder.lastTitle, "2 条") {
		t.Fatalf("title = %q, want digest count", recorder.lastTitle)
	}
	if strings.Contains(recorder.lastBody, "【出售】") || strings.Contains(recorder.lastBody, "【串货】") {
		t.Fatalf("body unexpectedly contains legacy business sections: %q", recorder.lastBody)
	}
}

func TestProcessDueNotificationsMergesLegacyResaleWithoutSpecialLabel(t *testing.T) {
	subscriptionService := openTestService(t)
	fixedNow := time.Date(2026, time.July, 15, 10, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return fixedNow }
	recorder := &recordingSender{}
	subscriptionService.Notify = notify.Registry{IYUU: recorder}

	_, seatA := createTestAccountWithSeats(t, subscriptionService, "分组账号A", "车位1")
	_, seatB := createTestAccountWithSeats(t, subscriptionService, "分组账号B", "车位1")
	if _, err := subscriptionService.Create(service.CreateInput{
		Name:             "出售订阅",
		PriceYuan:        "10.00",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "0",
		CustomerEmail:    "sale@example.com",
		SeatID:           seatA[0],
		BoardedAt:        "2000-01-01",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := subscriptionService.Create(service.CreateInput{
		Name:             "串货订阅",
		PriceYuan:        "85.00",
		IsResale:         true,
		AgencyFeeYuan:    "0",
		CronExpr:         "0 0 15 * *",
		NotifyOffsetsRaw: "0",
		CustomerEmail:    "resale@example.com",
		SeatID:           seatB[0],
		BoardedAt:        "2000-01-01",
	}); err != nil {
		t.Fatal(err)
	}

	if err := subscriptionService.ProcessDueNotifications(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recorder.calls != 1 {
		t.Fatalf("notification sends = %d, want 1 merged send", recorder.calls)
	}
	if !strings.Contains(recorder.lastTitle, "2 条") {
		t.Fatalf("title = %q, want merged count", recorder.lastTitle)
	}
	if strings.Contains(recorder.lastBody, "【出售】") || strings.Contains(recorder.lastBody, "【串货】") {
		t.Fatalf("body unexpectedly contains legacy business sections: %q", recorder.lastBody)
	}
	if !strings.Contains(recorder.lastBody, "sale@example.com") {
		t.Fatalf("body should contain first customer email: %q", recorder.lastBody)
	}
	if !strings.Contains(recorder.lastBody, "resale@example.com") {
		t.Fatalf("body should contain legacy customer email: %q", recorder.lastBody)
	}
}
