package service

import (
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

func TestBenefitObservationFollowsEffectiveExtensionBoundary(t *testing.T) {
	service := openGoalTestService(t)
	ids := createCustomerCareTestSubscriptions(t, service, "extension-boundary@example.test", "", 1)
	input := RecordCustomerBenefitsInput{SubscriptionIDs: ids, BenefitType: model.CustomerBenefitTypeExtension, ExtensionDays: 30,
		BenefitDate: "2026-08-15", OperationKey: "observation-first-extension", ExtensionReviewSnapshots: extensionReviewSnapshots(t, service, ids)}
	if _, err := service.RecordCustomerBenefits(input); err != nil {
		t.Fatal(err)
	}
	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil {
		t.Fatal(err)
	}
	if len(benefits[0].ObservationSeats) != 1 {
		t.Fatalf("missing persisted observations: %#v", benefits)
	}
	subscription, err := service.Store.GetSubscription(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	base := benefits[0].ObservationSeats[0].BaseDueDate
	baseAt, _ := time.ParseInLocation("2006-01-02", base, cycle.Location)
	assertStatus := func(at time.Time, want string) {
		t.Helper()
		outcome := customerBenefitOutcome(benefits[0], []model.Subscription{subscription}, nil, at)
		if outcome.Status != want {
			t.Fatalf("at %s: %#v, want %s", at, outcome, want)
		}
	}
	assertStatus(baseAt.AddDate(0, 0, 7), "pending")
	assertStatus(baseAt.AddDate(0, 0, 37), "not_renewed")
	input.OperationKey = "observation-second-extension"
	input.ExtensionDays = 10
	input.ExtensionReviewSnapshots = extensionReviewSnapshots(t, service, ids)
	if _, err := service.RecordCustomerBenefits(input); err != nil {
		t.Fatal(err)
	}
	subscription, err = service.Store.GetSubscription(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(baseAt.AddDate(0, 0, 37), "pending")
	assertStatus(baseAt.AddDate(0, 0, 47), "not_renewed")
	// Removing the last event models the active-event view after revocation.
	subscription.DueExtensions = subscription.DueExtensions[:1]
	assertStatus(baseAt.AddDate(0, 0, 37), "not_renewed")
	// A later billing period's extension must not move this old observation.
	subscription.DueExtensions = append(subscription.DueExtensions, model.SubscriptionDueExtension{BaseDueDate: cycle.FormatDate(baseAt.AddDate(0, 0, 30)), ExtensionDays: 20})
	assertStatus(baseAt.AddDate(0, 0, 37), "not_renewed")
}

func TestBenefitObservationsUseEachSeatsDueAndExcludePrepayments(t *testing.T) {
	created := time.Date(2026, 7, 20, 12, 0, 0, 0, cycle.Location)
	benefit := model.CustomerBenefit{SubscriptionID: 1, CustomerGroupSizeSnapshot: 2, NextDueDateSnapshot: "2026-10-01", CreatedAt: created,
		ObservationSeats: []model.CustomerBenefitObservation{{SubscriptionID: 1, DueDate: "2026-08-01", LastBillID: 2}, {SubscriptionID: 2, DueDate: "2026-10-01", LastBillID: 3}}}
	bills := map[int64][]model.Bill{
		1: {{ID: 4, SubscriptionID: 1, DueDate: "2026-08-01", CreatedAt: created}},
		2: {{ID: 5, SubscriptionID: 2, DueDate: "2026-10-01", CreatedAt: created.Add(time.Hour)}},
		3: {{ID: 6, SubscriptionID: 3, DueDate: "2026-10-01", CreatedAt: created.Add(time.Hour)}},
	}
	now := created.AddDate(0, 3, 0)
	result := customerBenefitOutcome(benefit, nil, bills, now)
	if result.Status != "renewed" || result.RenewedSeatCount != 2 {
		t.Fatalf("staggered seats: %#v", result)
	}
	bills[1][0].ID = 2 // A preexisting bill cannot be counted, even on the same timestamp.
	result = customerBenefitOutcome(benefit, nil, bills, now)
	if result.Status != "partially_renewed" || result.RenewedSeatCount != 1 {
		t.Fatalf("prepayment/new seat counted: %#v", result)
	}
}

func TestLegacyBenefitOutcomeMergesTransitiveIdentities(t *testing.T) {
	benefit := model.CustomerBenefit{SubscriptionID: 1, CustomerEmailSnapshot: "a@example.test", CustomerWechatSnapshot: "a", CustomerGroupSizeSnapshot: 3, BenefitDate: "2026-07-20", NextDueDateSnapshot: "2026-08-01"}
	subs := []model.Subscription{
		{ID: 1, CustomerEmail: "a@example.test", CustomerWechat: "a", BoardedAt: "2026-07-01"},
		{ID: 2, CustomerEmail: "a@example.test", CustomerWechat: "b", BoardedAt: "2026-07-01"},
		{ID: 3, CustomerEmail: "b@example.test", CustomerWechat: "b", BoardedAt: "2026-07-01"},
		{ID: 4, CustomerEmail: "a@example.test", BoardedAt: "2026-08-01"},
	}
	bills := map[int64][]model.Bill{1: {{DueDate: "2026-08-01"}}, 2: {{DueDate: "2026-08-01"}}, 3: {{DueDate: "2026-08-01"}}, 4: {{DueDate: "2026-08-01"}}}
	now := time.Date(2026, 8, 9, 0, 0, 0, 0, cycle.Location)
	if result := customerBenefitOutcome(benefit, subs, bills, now); result.Status != "renewed" || result.RenewedSeatCount != 3 {
		t.Fatalf("transitive group: %#v", result)
	}
	delete(bills, 2)
	if result := customerBenefitOutcome(benefit, subs, bills, now); result.Status != "partially_renewed" || result.RenewedSeatCount != 2 {
		t.Fatalf("late newcomer replaced old seat: %#v", result)
	}
}

func TestExtensionRevisionPreservesOtherSeatsRenewalEvidence(t *testing.T) {
	service, ids := extensionTestService(t, "2026-06-20", 2)
	if _, err := service.RecordCustomerBenefits(extensionInput(t, service, ids, "2026-06-20", 7, "revision-seat-observation")); err != nil {
		t.Fatal(err)
	}
	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil {
		t.Fatal(err)
	}
	var target model.CustomerBenefit
	for _, benefit := range benefits {
		if benefit.SubscriptionID == ids[1] {
			target = benefit
		}
	}
	if err := service.SetDuePaid(ids[0], "2026-07-08", true); err != nil {
		t.Fatal(err)
	}
	service.Clock = func() time.Time { return time.Date(2026, 6, 21, 12, 0, 0, 0, cycle.Location) }
	revision, err := service.ReviseCustomerBenefitExtension(ReviseCustomerBenefitExtensionInput{BenefitID: target.ID, ExtensionDays: 9, Reason: "adjust test", OperationKey: "revision-preserve-other-seat"})
	if err != nil {
		t.Fatal(err)
	}
	benefits, err = service.Store.ListCustomerBenefits()
	if err != nil {
		t.Fatal(err)
	}
	var replacement model.CustomerBenefit
	for _, benefit := range benefits {
		if benefit.ID == revision.ReplacementBenefitID {
			replacement = benefit
		}
	}
	bills, err := service.Store.ListBills()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64][]model.Bill{}
	for _, bill := range bills {
		byID[bill.SubscriptionID] = append(byID[bill.SubscriptionID], bill)
	}
	subs, err := service.Store.ListSubscriptions()
	if err != nil {
		t.Fatal(err)
	}
	result := customerBenefitOutcome(replacement, subs, byID, time.Date(2026, 7, 18, 12, 0, 0, 0, cycle.Location))
	if result.RenewedSeatCount != 1 || result.ExpectedSeatCount != 2 || result.Status != "partially_renewed" {
		t.Fatalf("revision lost other seat: %#v; snapshot=%#v", result, replacement)
	}
}

func TestRecoveredAccountBanDoesNotHideLaterNaturalChurn(t *testing.T) {
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, cycle.Location)
	archived := now.AddDate(0, 0, -1)
	for _, recovered := range []bool{false, true} {
		status := model.AfterSalesStatusRefunded
		want := 0
		if recovered {
			status = model.AfterSalesStatusReassigned
			want = 1
		}
		readiness := buildPredictionReadiness(nil, []model.Subscription{{ID: 1, BusinessType: model.SubscriptionBusinessTeam, BoardedAt: "2026-07-01", ArchivedAt: &archived}},
			[]model.Bill{{ID: 1, SubscriptionID: 1, DueDate: "2026-07-01", AmountCents: 10000}},
			[]model.AfterSalesCase{{SubscriptionID: 1, Source: model.AfterSalesSourceAccountBan, Status: status}}, nil, now)
		if readiness.ChurnOutcomeCount != want || readiness.Lifecycle[len(readiness.Lifecycle)-1].NaturalChurnCount != want {
			t.Fatalf("recovered=%t: %#v", recovered, readiness)
		}
	}
}
