package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

func extensionTestService(t *testing.T, now string, count int) (*SubscriptionService, []int64) {
	t.Helper()
	service := openGoalTestService(t)
	moment, err := time.ParseInLocation("2006-01-02", now, cycle.Location)
	if err != nil {
		t.Fatal(err)
	}
	service.Clock = func() time.Time { return moment.Add(12 * time.Hour) }
	ids := createCustomerCareTestSubscriptions(t, service, "extension@example.com", "", count)
	for _, id := range ids {
		if err := service.Store.SetDuePaid(id, "2026-06-01", true, 10000); err != nil {
			t.Fatal(err)
		}
	}
	return service, ids
}

func extensionReviewSnapshots(t testing.TB, service *SubscriptionService, ids []int64) []ExtensionReviewSnapshot {
	t.Helper()
	views, err := service.ListView()
	if err != nil {
		t.Fatal(err)
	}
	viewsByID := make(map[int64]SubscriptionView, len(views))
	for _, view := range views {
		viewsByID[view.Subscription.ID] = view
	}
	snapshots := make([]ExtensionReviewSnapshot, 0, len(ids))
	for _, id := range ids {
		view, exists := viewsByID[id]
		if !exists {
			t.Fatalf("subscription %d missing from review view", id)
		}
		snapshots = append(snapshots, ExtensionReviewSnapshot{
			SubscriptionID:    id,
			ExpectedUpdatedAt: view.Subscription.UpdatedAt.Format(time.RFC3339Nano),
			ExpectedDueDate:   view.NextDueDate,
		})
	}
	return snapshots
}

func extensionInput(t testing.TB, service *SubscriptionService, ids []int64, now string, days int, operation string) RecordCustomerBenefitsInput {
	t.Helper()
	return RecordCustomerBenefitsInput{SubscriptionIDs: ids, BenefitType: model.CustomerBenefitTypeExtension,
		ExtensionDays: days, OperationKey: operation, BenefitDate: now, ActualCostYuan: "2.00",
		ExtensionReviewSnapshots: extensionReviewSnapshots(t, service, ids)}
}

func TestDistinctDiscountAndExtensionBenefitsCanBeGivenOnDueDay(t *testing.T) {
	for _, discountFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("discount_first_%t", discountFirst), func(t *testing.T) {
			service, ids := extensionTestService(t, "2026-07-01", 1)
			billsBefore, err := service.Store.ListBills()
			if err != nil {
				t.Fatal(err)
			}
			discount := RecordCustomerBenefitsInput{
				SubscriptionIDs: ids, BenefitType: model.CustomerBenefitTypePriceDiscount,
				PriceDiscountYuan: "10.00", OperationKey: "test-combined-discount-operation",
				BenefitDate: "2026-07-01",
			}
			giveDiscount := func() {
				t.Helper()
				if n, err := service.RecordCustomerBenefits(discount); err != nil || n != 1 {
					t.Fatalf("discount delivery = %d, %v", n, err)
				}
			}
			giveExtension := func() {
				t.Helper()
				if n, err := service.RecordCustomerBenefits(extensionInput(t, service, ids,
					"2026-07-01", 7, "test-combined-extension-operation")); err != nil || n != 1 {
					t.Fatalf("extension delivery = %d, %v", n, err)
				}
			}
			if discountFirst {
				giveDiscount()
				giveExtension()
			} else {
				giveExtension()
				giveDiscount()
			}
			views, err := service.ListView()
			if err != nil || len(views) != 1 || views[0].NextDueDate != "2026-07-08" ||
				views[0].Subscription.NextPriceCents == nil || *views[0].Subscription.NextPriceCents != 9000 ||
				views[0].NextPriceEffectiveDueDate != "2026-07-08" {
				t.Fatalf("combined benefit state = %#v, %v", views, err)
			}
			benefits, err := service.Store.ListCustomerBenefits()
			if err != nil || len(benefits) != 2 || benefits[0].BatchID == benefits[1].BatchID {
				t.Fatalf("combined benefit records = %#v, %v", benefits, err)
			}
			for _, benefit := range benefits {
				if benefit.BenefitType == model.CustomerBenefitTypePriceDiscount &&
					(benefit.PriceEffectiveDueDate != "2026-07-08" || benefit.PriceAfterCents != 9000) {
					t.Fatalf("discount fact did not follow extension = %#v", benefit)
				}
			}
			if _, err := service.RecordCustomerBenefits(discount); err == nil {
				t.Fatal("discount operation replay accepted")
			}
			duplicateDiscount := discount
			duplicateDiscount.OperationKey = "test-combined-duplicate-discount"
			if _, err := service.RecordCustomerBenefits(duplicateDiscount); !errors.Is(err, db.ErrCustomerBenefitAlreadyRecorded) {
				t.Fatalf("same discount fact with a new operation key error = %v", err)
			}
			if _, err := service.RecordCustomerBenefits(extensionInput(t, service, ids,
				"2026-07-01", 7, "test-combined-extension-operation")); err == nil {
				t.Fatal("extension operation replay accepted")
			}
			benefits, err = service.Store.ListCustomerBenefits()
			if err != nil || len(benefits) != 2 {
				t.Fatalf("replays duplicated benefits = %#v, %v", benefits, err)
			}
			events, err := service.Store.ListSubscriptionDueExtensions()
			if err != nil || len(events) != 1 || events[0].ExtensionDays != 7 {
				t.Fatalf("replays duplicated extension = %#v, %v", events, err)
			}
			billsAfter, err := service.Store.ListBills()
			if err != nil || !reflect.DeepEqual(billsBefore, billsAfter) {
				t.Fatalf("combined benefits changed historical bills: %#v, %v", billsAfter, err)
			}
			if err := service.SetDuePaid(ids[0], "2026-07-08", true); err != nil {
				t.Fatal(err)
			}
			bill, err := service.Store.GetBillByOccurrence(ids[0], "2026-07-08")
			if err != nil || bill.AmountCents != 9000 {
				t.Fatalf("renewal at extended boundary = %#v, %v; want discounted amount 9000", bill, err)
			}
		})
	}
}

func TestDistinctExtensionBenefitsAccumulateOnSameDay(t *testing.T) {
	service, ids := extensionTestService(t, "2026-07-01", 1)
	for _, operation := range []string{"test-same-day-extension-first", "test-same-day-extension-second"} {
		if n, err := service.RecordCustomerBenefits(extensionInput(t, service, ids,
			"2026-07-01", 7, operation)); err != nil || n != 1 {
			t.Fatalf("distinct extension delivery = %d, %v", n, err)
		}
	}
	views, err := service.ListView()
	if err != nil || len(views) != 1 || views[0].NextDueDate != "2026-07-15" {
		t.Fatalf("accumulated extension state = %#v, %v", views, err)
	}
	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil || len(benefits) != 2 {
		t.Fatalf("separate same-day extension records = %#v, %v", benefits, err)
	}
	events, err := service.Store.ListSubscriptionDueExtensions()
	if err != nil || len(events) != 2 {
		t.Fatalf("separate same-day extension events = %#v, %v", events, err)
	}
}

func TestExtensionEffectiveDueAcrossBillingConsumers(t *testing.T) {
	for _, now := range []string{"2026-06-29", "2026-07-01", "2026-07-05", "2026-08-15"} {
		t.Run(now, func(t *testing.T) {
			service, ids := extensionTestService(t, now, 1)
			id := ids[0]
			before, _ := service.Store.ListBills()
			subscription, _ := service.Store.GetSubscription(id)
			price := int64(9000)
			subscription.NextPriceCents = &price
			subscription.NextPriceEffectiveDueDate = "2026-07-01"
			if err := service.Store.UpdateSubscriptionNextPrices([]model.Subscription{subscription}, "2026-03-01"); err != nil {
				t.Fatal(err)
			}
			input := extensionInput(t, service, ids, now, 9, "test-effective-extension-0001")
			if n, err := service.RecordCustomerBenefits(input); err != nil || n != 1 {
				t.Fatalf("apply = %d, %v", n, err)
			}
			views, err := service.ListView()
			if err != nil {
				t.Fatal(err)
			}
			view := views[0]
			if view.NextDueDate != "2026-07-10" || view.CycleDays != 39 || view.NextPriceEffectiveDueDate != "2026-07-10" {
				t.Fatalf("view = %#v", view)
			}
			if view.DaysRemaining != cycle.DaysRemaining(time.Date(2026, 7, 10, 0, 0, 0, 0, cycle.Location), service.now()) {
				t.Fatalf("days = %d", view.DaysRemaining)
			}
			after, _ := service.Store.ListBills()
			if !reflect.DeepEqual(before, after) {
				t.Fatal("extension changed historical bills")
			}
			benefits, err := service.Store.ListCustomerBenefits()
			if err != nil || len(benefits) != 2 || benefits[1].PriceEffectiveDueDate != "2026-07-10" {
				t.Fatalf("shifted discount benefit = %#v, %v", benefits, err)
			}
			if _, err := service.RecordCustomerBenefits(input); err == nil {
				t.Fatal("operation replay accepted")
			}
			if err := service.SetDuePaid(id, "2026-07-01", true); err == nil {
				t.Fatal("old billing boundary accepted")
			}
			calendar, err := service.CalendarMonth(time.Date(2026, 7, 1, 0, 0, 0, 0, cycle.Location))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, day := range calendar.Days {
				for _, occurrence := range day.Occurrences {
					if occurrence.SubscriptionID == id && occurrence.DueDate == "2026-07-10" {
						found = true
					}
					if occurrence.SubscriptionID == id && occurrence.DueDate == "2026-07-01" {
						t.Fatal("calendar kept old date")
					}
				}
			}
			if !found {
				t.Fatal("calendar omitted effective due")
			}
			plan, err := buildRenewalPeriodPlan(view.Subscription, view.NextDueDate, 2)
			if err != nil || plan.Bills[1].DueDate != "2026-08-09" || plan.PeriodEndDate != "2026-09-08" {
				t.Fatalf("plan = %#v, %v", plan, err)
			}
			if err := service.SetDuePaid(id, "2026-07-10", true); err != nil {
				t.Fatal(err)
			}
			views, err = service.ListView()
			if err != nil || views[0].NextDueDate != "2026-08-09" {
				t.Fatalf("paid next = %#v, %v", views, err)
			}
			if views[0].Subscription.PricePerPersonCents != 9000 {
				t.Fatal("scheduled price was not applied at shifted boundary")
			}
		})
	}
}

func TestExtensionKeepsPendingNotificationWhenLaterBoundaryBecomesEffectiveDue(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	id := ids[0]
	oldLog, err := service.Store.UpsertPendingNotification(
		id, "2026-07-01", 3, model.ChannelGotify, model.NotificationKindScheduled,
	)
	if err != nil {
		t.Fatal(err)
	}
	reusedLog, err := service.Store.UpsertPendingNotification(
		id, "2026-07-31", 3, model.ChannelGotify, model.NotificationKindScheduled,
	)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.RecordCustomerBenefits(extensionInput(
		t, service, ids, "2026-06-20", 30, "test-notification-date-reuse",
	)); err != nil {
		t.Fatal(err)
	}

	oldLog, err = service.Store.GetNotificationLogByID(oldLog.ID)
	if err != nil || oldLog.Status != model.NotificationStatusCanceled {
		t.Fatalf("old notification = %#v, %v", oldLog, err)
	}
	reusedLog, err = service.Store.GetNotificationLogByID(reusedLog.ID)
	if err != nil || reusedLog.Status != model.NotificationStatusPending {
		t.Fatalf("reused notification = %#v, %v", reusedLog, err)
	}
	view, err := service.ListView()
	if err != nil || len(view) != 1 || view[0].NextDueDate != "2026-07-31" {
		t.Fatalf("extended view = %#v, %v", view, err)
	}
}

func TestCumulativeExtensionReactivatesNotificationOnlyWhenPlannerRequestsIt(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	id := ids[0]
	logEntry, err := service.Store.UpsertPendingNotification(
		id, "2026-07-31", 3, model.ChannelSMTP, model.NotificationKindScheduled,
	)
	if err != nil {
		t.Fatal(err)
	}
	retryAt := time.Date(2026, time.June, 20, 13, 0, 0, 0, cycle.Location)
	if err := service.Store.MarkNotificationFailure(logEntry.ID, 2, "temporary SMTP failure", &retryAt, false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordCustomerBenefits(extensionInput(
		t, service, ids, "2026-06-20", 9, "test-notification-cancel-first",
	)); err != nil {
		t.Fatal(err)
	}
	logEntry, err = service.Store.GetNotificationLogByID(logEntry.ID)
	if err != nil || logEntry.Status != model.NotificationStatusCanceled {
		t.Fatalf("notification after first extension = %#v, %v", logEntry, err)
	}
	service.Clock = func() time.Time {
		return time.Date(2026, time.September, 19, 12, 0, 0, 0, cycle.Location)
	}
	if _, err := service.RecordCustomerBenefits(extensionInput(
		t, service, ids, "2026-09-19", 21, "test-notification-revalidate-later",
	)); err != nil {
		t.Fatal(err)
	}
	logEntry, err = service.Store.GetNotificationLogByID(logEntry.ID)
	if err != nil || logEntry.Status != model.NotificationStatusCanceled {
		t.Fatalf("notification changed without planner request = %#v, %v", logEntry, err)
	}

	subscription, err := service.Store.GetSubscription(id)
	if err != nil {
		t.Fatal(err)
	}
	planAt := time.Date(2026, time.July, 28, 12, 0, 0, 0, cycle.Location)
	if err := service.planSubscription(context.Background(), subscription, planAt, cycle.FormatDate(planAt)); err != nil {
		t.Fatal(err)
	}
	reactivated, err := service.Store.GetNotificationLogByID(logEntry.ID)
	if err != nil || reactivated.Status != model.NotificationStatusPending || reactivated.AttemptCount != 0 ||
		reactivated.NextRetryAt != nil || reactivated.LastError != "" {
		t.Fatalf("reactivated notification = %#v, %v", reactivated, err)
	}
}

func TestExtensionCumulativePrepaymentAndRollback(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 2)
	if err := service.Store.SetDuePaid(ids[0], "2026-07-01", true, 10000); err != nil {
		t.Fatal(err)
	}
	for index := range 2 {
		benefitDate := "2026-06-20"
		if index == 1 {
			benefitDate = "2026-09-19"
			service.Clock = func() time.Time {
				return time.Date(2026, time.September, 19, 12, 0, 0, 0, cycle.Location)
			}
		}
		if _, err := service.RecordCustomerBenefits(extensionInput(t, service, ids[:1], benefitDate, 9, fmt.Sprintf("test-cumulative-operation-%d", index))); err != nil {
			t.Fatal(err)
		}
	}
	subscription, _ := service.Store.GetSubscription(ids[0])
	if len(subscription.DueExtensions) != 2 || subscription.DueExtensions[1].EffectiveDueDate != "2026-08-18" {
		t.Fatalf("extensions = %#v", subscription.DueExtensions)
	}
	if err := service.Store.SetDuePaid(ids[1], "2026-07-31", true, 10000); err != nil {
		t.Fatal(err)
	}
	before, _ := service.Store.ListCustomerBenefits()
	if _, err := service.RecordCustomerBenefits(extensionInput(t, service, ids, "2026-06-20", 3, "test-batch-out-of-order")); err == nil {
		t.Fatal("out of order prepaid gap accepted")
	}
	after, _ := service.Store.ListCustomerBenefits()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed batch recorded costs")
	}
	current, _ := service.Store.GetSubscription(ids[0])
	if !reflect.DeepEqual(subscription, current) {
		t.Fatal("failed batch changed first target")
	}
}

func TestExtensionRepairExplicitAndIdempotent(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	if _, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
		SubscriptionIDs: ids, BenefitType: model.CustomerBenefitTypeManual, BenefitName: "赠送延期 9 天",
		BenefitDate: "2026-06-20", ActualCostYuan: "2.00",
	}); err != nil {
		t.Fatal(err)
	}
	benefits, _ := service.Store.ListCustomerBenefits()
	subscription, _ := service.Store.GetSubscription(ids[0])
	if len(subscription.DueExtensions) != 0 {
		t.Fatal("legacy name was automatically applied")
	}
	if _, _, err := service.ApplyCustomerBenefitExtension(benefits[0].ID, ids[0], 9, "2026-07-02"); err == nil {
		t.Fatal("stale repair accepted")
	}
	event, applied, err := service.ApplyCustomerBenefitExtension(benefits[0].ID, ids[0], 9, "2026-07-01")
	if err != nil || !applied || event.EffectiveDueDate != "2026-07-10" {
		t.Fatalf("repair = %#v %v %v", event, applied, err)
	}
	if _, applied, err = service.ApplyCustomerBenefitExtension(benefits[0].ID, ids[0], 9, "2026-07-01"); err != nil || applied {
		t.Fatalf("replay = %v %v", applied, err)
	}
	benefits, _ = service.Store.ListCustomerBenefits()
	if len(benefits) != 1 || benefits[0].ActualCostCents != 200 || benefits[0].ExtensionDays != 9 || benefits[0].ExtensionAppliedAt == nil {
		t.Fatalf("benefit = %#v", benefits)
	}
	exported, err := service.Export()
	if err != nil || len(exported.SubscriptionDueExtensions) != 1 {
		t.Fatalf("export = %#v %v", exported.SubscriptionDueExtensions, err)
	}
}

func TestExtensionRejectsPendingRenewalAndStalePayment(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 1)
	stale, _ := service.Store.GetSubscription(ids[0])
	applicationID, err := service.Store.CreateRenewalApplication(model.RenewalApplication{
		TrackingToken: "pending-extension-test", SubscriptionID: ids[0], DueDate: "2026-07-01", PeriodEndDate: "2026-07-31", AmountCents: 10000,
	})
	if err != nil {
		t.Fatal(err)
	}
	input := extensionInput(t, service, ids, "2026-06-20", 9, "test-pending-extension")
	if _, err := service.RecordCustomerBenefits(input); err == nil {
		t.Fatal("pending renewal was not rejected")
	}
	if err := service.Store.RejectRenewalApplication(applicationID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordCustomerBenefits(input); err != nil {
		t.Fatal(err)
	}
	if err := service.Store.SetDuePaidForSubscription(stale, "2026-07-01", true, 10000); err == nil {
		t.Fatal("stale payment accepted")
	}
	if _, err := service.Store.CreateRenewalApplication(model.RenewalApplication{TrackingToken: "stale-extension-renewal", SubscriptionID: ids[0], DueDate: "2026-07-01", AmountCents: 10000}); err == nil {
		t.Fatal("stale renewal request accepted")
	}
}

func TestExtendedTimedCronRenewalPlanAdvancesToNextCalendarPeriod(t *testing.T) {
	subscription := model.Subscription{
		CronExpr:            "30 9 1 * *",
		BoardedAt:           "2026-09-01",
		PricePerPersonCents: 10000,
		DueExtensions: []model.SubscriptionDueExtension{{
			BaseDueDate:   "2026-10-01",
			ExtensionDays: 9,
		}},
	}
	plan, err := buildRenewalPeriodPlan(subscription, "2026-10-10", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{plan.Bills[0].DueDate, plan.Bills[1].DueDate, plan.PeriodEndDate}; !reflect.DeepEqual(got, []string{"2026-10-10", "2026-11-10", "2026-12-10"}) {
		t.Fatalf("renewal dates = %#v", got)
	}
}

func TestExtensionRejectsStaleReviewedSubscriptionAndDueDate(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 2)
	views, err := service.ListView()
	if err != nil {
		t.Fatal(err)
	}
	viewByID := make(map[int64]SubscriptionView, len(views))
	for _, view := range views {
		viewByID[view.Subscription.ID] = view
	}

	stale := viewByID[ids[0]].Subscription
	if err := service.SetDuePaid(ids[0], viewByID[ids[0]].NextDueDate, true); err != nil {
		t.Fatal(err)
	}
	current := viewByID[ids[1]].Subscription

	for index, test := range []struct {
		subscription model.Subscription
		expectedDue  string
	}{
		{subscription: stale, expectedDue: viewByID[ids[0]].NextDueDate},
		{subscription: current, expectedDue: "2026-07-02"},
	} {
		benefit := model.CustomerBenefit{
			BatchID:                   fmt.Sprintf("benefit-operation-v1:stale-extension-%d", index),
			SubscriptionID:            test.subscription.ID,
			BenefitType:               model.CustomerBenefitTypeExtension,
			BenefitName:               "赠送延期 9 天",
			BenefitDate:               "2026-06-20",
			CustomerGroupSizeSnapshot: 1,
			CurrentPriceCentsSnapshot: test.subscription.PricePerPersonCents,
			ExtensionDays:             9,
			CreatedAt:                 service.now().UTC(),
		}
		err := service.Store.CreateCustomerBenefitsAndExtendDueDates(
			[]model.CustomerBenefit{benefit},
			[]db.DueExtensionExpectation{{
				SubscriptionID:           test.subscription.ID,
				UpdatedAt:                test.subscription.UpdatedAt,
				PreviousEffectiveDueDate: test.expectedDue,
			}},
			service.now(),
		)
		if !errors.Is(err, db.ErrSubscriptionStateChanged) {
			t.Fatalf("case %d error = %v", index, err)
		}
	}
	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil || len(benefits) != 0 {
		t.Fatalf("rolled-back benefits = %#v, %v", benefits, err)
	}
}

func TestRecordExtensionRejectsBrowserSnapshotThatBecameStale(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, *SubscriptionService, int64, string)
	}{
		{
			name: "payment advanced unpaid boundary",
			mutate: func(t *testing.T, service *SubscriptionService, id int64, dueDate string) {
				t.Helper()
				if err := service.SetDuePaid(id, dueDate, true); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "subscription version changed",
			mutate: func(t *testing.T, service *SubscriptionService, id int64, _ string) {
				t.Helper()
				subscription, err := service.Store.GetSubscription(id)
				if err != nil {
					t.Fatal(err)
				}
				subscription.Remark = "changed after dialog opened"
				if err := service.Store.UpdateSubscription(subscription); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, ids := extensionTestService(t, "2026-06-20", 1)
			input := extensionInput(t, service, ids, "2026-06-20", 9, "test-stale-browser-snapshot")
			test.mutate(t, service, ids[0], input.ExtensionReviewSnapshots[0].ExpectedDueDate)

			if _, err := service.RecordCustomerBenefits(input); !errors.Is(err, db.ErrSubscriptionStateChanged) ||
				!strings.Contains(err.Error(), "刷新") {
				t.Fatalf("stale extension error = %v", err)
			}
			benefits, err := service.Store.ListCustomerBenefits()
			if err != nil || len(benefits) != 0 {
				t.Fatalf("stale request benefits = %#v, %v", benefits, err)
			}
			events, err := service.Store.ListSubscriptionDueExtensions()
			if err != nil || len(events) != 0 {
				t.Fatalf("stale request extensions = %#v, %v", events, err)
			}
		})
	}
}

func TestRecordExtensionRequiresExactValidReviewSnapshots(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*RecordCustomerBenefitsInput)
	}{
		{name: "missing snapshot", mutate: func(input *RecordCustomerBenefitsInput) {
			input.ExtensionReviewSnapshots = input.ExtensionReviewSnapshots[:1]
		}},
		{name: "duplicate snapshot", mutate: func(input *RecordCustomerBenefitsInput) {
			input.ExtensionReviewSnapshots[1] = input.ExtensionReviewSnapshots[0]
		}},
		{name: "mismatched subscription", mutate: func(input *RecordCustomerBenefitsInput) {
			input.ExtensionReviewSnapshots[1].SubscriptionID = 999999
		}},
		{name: "invalid updated at", mutate: func(input *RecordCustomerBenefitsInput) {
			input.ExtensionReviewSnapshots[0].ExpectedUpdatedAt = "2026-06-20"
		}},
		{name: "invalid due date", mutate: func(input *RecordCustomerBenefitsInput) {
			input.ExtensionReviewSnapshots[0].ExpectedDueDate = "2026-7-1"
		}},
		{name: "duplicate selected subscription", mutate: func(input *RecordCustomerBenefitsInput) {
			input.SubscriptionIDs[1] = input.SubscriptionIDs[0]
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, ids := extensionTestService(t, "2026-06-20", 2)
			input := extensionInput(t, service, ids, "2026-06-20", 9, "test-review-snapshot-validation")
			test.mutate(&input)
			if _, err := service.RecordCustomerBenefits(input); err == nil || !strings.Contains(err.Error(), "刷新") {
				t.Fatalf("invalid review snapshots error = %v", err)
			}
			benefits, err := service.Store.ListCustomerBenefits()
			if err != nil || len(benefits) != 0 {
				t.Fatalf("invalid review snapshots benefits = %#v, %v", benefits, err)
			}
			events, err := service.Store.ListSubscriptionDueExtensions()
			if err != nil || len(events) != 0 {
				t.Fatalf("invalid review snapshots extensions = %#v, %v", events, err)
			}
		})
	}
}

func TestTimedCronExtensionWorksAcrossListCalendarRenewalAndPayment(t *testing.T) {
	service := openGoalTestService(t)
	service.Clock = func() time.Time {
		return time.Date(2026, time.September, 20, 12, 0, 0, 0, cycle.Location)
	}
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name: "timed-cron-account", OpenedAt: "2026-09-01", SeatNames: []string{"seat-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := service.Store.ListSeatsByAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, err := service.Create(CreateInput{
		Name: "timed-cron-customer", PriceYuan: "100", CronExpr: "30 9 1 * *",
		NotifyOffsetsRaw: "3,1,0", SeatID: seats[0].ID, BoardedAt: "2026-09-01",
		CustomerEmail: "timed-cron@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Store.SetDuePaid(subscriptionID, "2026-09-01", true, 10000); err != nil {
		t.Fatal(err)
	}
	views, err := service.ListView()
	if err != nil || len(views) != 1 || views[0].NextDueDate != "2026-10-01" {
		t.Fatalf("initial view = %#v, %v", views, err)
	}
	if _, err := service.RecordCustomerBenefits(extensionInput(t, service,
		[]int64{subscriptionID}, "2026-09-20", 9, "test-timed-cron-extension",
	)); err != nil {
		t.Fatal(err)
	}
	views, err = service.ListView()
	if err != nil || views[0].NextDueDate != "2026-10-10" || views[0].CycleDays != 39 {
		t.Fatalf("extended view = %#v, %v", views, err)
	}
	calendar, err := service.CalendarMonth(time.Date(2026, time.October, 1, 0, 0, 0, 0, cycle.Location))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, occurrence := range calendar.Occurrences {
		if occurrence.SubscriptionID == subscriptionID && occurrence.DueDate == "2026-10-10" {
			found = true
		}
		if occurrence.SubscriptionID == subscriptionID && occurrence.DueDate == "2026-10-01" {
			t.Fatal("calendar retained the original cron date")
		}
	}
	if !found {
		t.Fatal("calendar omitted the shifted cron date")
	}
	plan, err := buildRenewalPeriodPlan(views[0].Subscription, views[0].NextDueDate, 2)
	if err != nil || plan.Bills[1].DueDate != "2026-11-10" || plan.PeriodEndDate != "2026-12-10" {
		t.Fatalf("renewal plan = %#v, %v", plan, err)
	}
	if err := service.SetDuePaid(subscriptionID, "2026-10-10", true); err != nil {
		t.Fatal(err)
	}
	views, err = service.ListView()
	if err != nil || views[0].NextDueDate != "2026-11-10" {
		t.Fatalf("paid view = %#v, %v", views, err)
	}
}
