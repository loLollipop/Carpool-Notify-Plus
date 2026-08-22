package service

import (
	"fmt"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

func createCustomerCareTestSubscriptions(
	t *testing.T,
	service *SubscriptionService,
	email string,
	wechat string,
	count int,
) []int64 {
	t.Helper()
	seatNames := make([]string, 0, count)
	for index := 0; index < count; index++ {
		seatNames = append(seatNames, fmt.Sprintf("seat-%d", index+1))
	}
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "care-owner",
		OpenedAt:  "2026-06-01",
		SeatNames: seatNames,
	})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := service.Store.ListSeatsByAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, count)
	for index := 0; index < count; index++ {
		id, createErr := service.Create(CreateInput{
			Name:             "care-customer",
			PriceYuan:        "100.00",
			CronExpr:         "interval:30d",
			NotifyOffsetsRaw: "3,1,0",
			SeatID:           seats[index].ID,
			BoardedAt:        "2026-06-01",
			CustomerEmail:    email,
			CustomerWechat:   wechat,
		})
		if createErr != nil {
			t.Fatal(createErr)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestCustomerBenefitCostFlowsThroughProfitReporting(t *testing.T) {
	service := openGoalTestService(t)
	ids := createCustomerCareTestSubscriptions(t, service, "care@example.com", "care-wechat", 1)
	if err := service.Store.SetDuePaid(ids[0], "2026-07-01", true, 10000, 0); err != nil {
		t.Fatal(err)
	}

	recorded, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
		SubscriptionIDs:    ids,
		BenefitType:        model.CustomerBenefitTypeManual,
		BenefitName:        "测试福利",
		ActualCostYuan:     "5.00",
		PerceivedValueYuan: "20.00",
		BenefitDate:        "2026-08-10",
	})
	if err != nil {
		t.Fatal(err)
	}
	if recorded != 1 {
		t.Fatalf("recorded = %d, want 1", recorded)
	}

	dashboard, err := service.ComputeDashboard()
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.TotalCostYuan != "5.00" || dashboard.TotalProfitCents != 9500 {
		t.Fatalf("dashboard after benefit = %#v", dashboard)
	}
	billsPage, err := service.ListBillsPage()
	if err != nil {
		t.Fatal(err)
	}
	if billsPage.Summary.TotalCostCents != 500 || billsPage.Summary.TotalProfitCents != 9500 {
		t.Fatalf("bills summary after benefit = %#v", billsPage.Summary)
	}
	trend, err := service.buildProfitTrend(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(trend) != 2 || trend[1].Month != "2026-08" || trend[1].CostCents != 500 || trend[1].ProfitCents != -500 {
		t.Fatalf("profit trend after benefit = %#v", trend)
	}
	export, err := service.Export()
	if err != nil {
		t.Fatal(err)
	}
	if len(export.CustomerBenefits) != 1 || export.CustomerBenefits[0].ActualCostCents != 500 {
		t.Fatalf("exported benefits = %#v", export.CustomerBenefits)
	}
}

func TestCustomerCareMergesMultiSeatIdentityAndStartsCooldown(t *testing.T) {
	service := openGoalTestService(t)
	ids := createCustomerCareTestSubscriptions(t, service, "multi@example.com", "same-wechat", 2)
	for _, subscriptionID := range ids {
		if err := service.Store.SetDuePaid(subscriptionID, "2026-07-02", true, 10000, 0); err != nil {
			t.Fatal(err)
		}
		if err := service.Store.SetDuePaid(subscriptionID, "2026-08-01", true, 10000, 0); err != nil {
			t.Fatal(err)
		}
	}

	pricingCandidates, err := service.buildPricingCandidates(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	care, err := service.buildCustomerCare(pricingCandidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(care.Candidates) != 1 {
		t.Fatalf("care candidates = %#v", care.Candidates)
	}
	if care.Prediction.RepeatSubscriptionCount != 2 || care.Prediction.RepeatCustomerCount != 1 {
		t.Fatalf("repeat subscription/customer counts = %#v", care.Prediction)
	}
	candidate := care.Candidates[0]
	if candidate.SeatCount != 2 || candidate.RenewalCount != 1 || !candidate.Recommended ||
		candidate.ReasonCode != "first_renewal" || candidate.CurrentCycleValueCents != 20000 {
		t.Fatalf("merged care candidate = %#v", candidate)
	}

	if _, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
		SubscriptionIDs:    []int64{candidate.SubscriptionID},
		BenefitType:        model.CustomerBenefitTypeRenewalMilestone,
		BenefitName:        "首次续费礼",
		ActualCostYuan:     "2.00",
		PerceivedValueYuan: "10.00",
		BenefitDate:        "2026-08-15",
	}); err != nil {
		t.Fatal(err)
	}
	pricingCandidates, err = service.buildPricingCandidates(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	care, err = service.buildCustomerCare(pricingCandidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(care.Candidates) != 1 || care.Candidates[0].Status != "cooldown" ||
		care.Candidates[0].NextEligibleDate != "2026-11-13" {
		t.Fatalf("care cooldown = %#v", care.Candidates)
	}
}

func TestCreateCustomerBenefitsRollsBackWholeBatch(t *testing.T) {
	service := openGoalTestService(t)
	ids := createCustomerCareTestSubscriptions(t, service, "atomic@example.com", "", 1)
	createdAt := time.Date(2026, time.August, 15, 4, 0, 0, 0, time.UTC)
	base := model.CustomerBenefit{
		BatchID:                   "atomic-batch",
		SubscriptionID:            ids[0],
		BenefitType:               model.CustomerBenefitTypeManual,
		BenefitName:               "atomic",
		BenefitDate:               "2026-08-15",
		CustomerTierSnapshot:      "mainstay",
		CustomerGroupSizeSnapshot: 1,
		CurrentPriceCentsSnapshot: 10000,
		CreatedAt:                 createdAt,
	}
	invalid := base
	invalid.SubscriptionID = 999999
	if err := service.Store.CreateCustomerBenefits([]model.CustomerBenefit{base, invalid}); err == nil {
		t.Fatal("expected stale batch to fail")
	}
	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil {
		t.Fatal(err)
	}
	if len(benefits) != 0 {
		t.Fatalf("partial customer benefit batch committed: %#v", benefits)
	}
}

func TestPredictionReadinessEnablesBetaBinomialAfterObservedCycles(t *testing.T) {
	active := []model.Subscription{{
		ID:           1,
		BusinessType: model.SubscriptionBusinessTeam,
	}}
	bills := make([]model.Bill, 0, 21)
	for index := 0; index < 21; index++ {
		bills = append(bills, model.Bill{
			ID:             int64(index + 1),
			SubscriptionID: 1,
			DueDate:        time.Date(2025, time.January, 1, 0, 0, 0, 0, cycle.Location).AddDate(0, index, 0).Format("2006-01-02"),
			AmountCents:    10000,
		})
	}
	readiness := buildPredictionReadiness(
		active,
		nil,
		bills,
		nil,
		nil,
		time.Date(2026, time.August, 15, 12, 0, 0, 0, cycle.Location),
	)
	if readiness.ActiveModel != "beta_binomial" || readiness.RenewalOutcomeCount != 20 ||
		readiness.EstimatedRenewalPercent == nil || readiness.EstimateLowPercent == nil ||
		readiness.EstimateHighPercent == nil {
		t.Fatalf("prediction readiness = %#v", readiness)
	}
	if *readiness.EstimatedRenewalPercent < 95 || *readiness.EstimateHighPercent > 100 {
		t.Fatalf("beta posterior estimate = %d [%d, %d]", *readiness.EstimatedRenewalPercent, *readiness.EstimateLowPercent, *readiness.EstimateHighPercent)
	}
}

func TestPredictionReadinessCountsCompletedCustomerCancellationAsNaturalChurn(t *testing.T) {
	archivedAt := time.Date(2026, time.August, 21, 20, 26, 0, 0, cycle.Location)
	archived := []model.Subscription{{
		ID:           1,
		BusinessType: model.SubscriptionBusinessTeam,
		BoardedAt:    "2026-07-22",
		ArchivedAt:   &archivedAt,
	}}
	bills := []model.Bill{{
		ID:             10,
		SubscriptionID: 1,
		DueDate:        "2026-07-22",
		AmountCents:    10000,
	}}
	cases := []model.AfterSalesCase{{
		SubscriptionID:    1,
		BillID:            10,
		BusinessType:      model.SubscriptionBusinessTeam,
		Source:            model.AfterSalesSourceCustomerCancellation,
		Status:            model.AfterSalesStatusRefunded,
		RefundAmountCents: 0,
	}}

	readiness := buildPredictionReadiness(
		nil,
		archived,
		bills,
		cases,
		nil,
		time.Date(2026, time.August, 22, 9, 0, 0, 0, cycle.Location),
	)
	if readiness.ChurnOutcomeCount != 1 || readiness.RenewalOutcomeCount != 1 {
		t.Fatalf("cancellation readiness = %#v", readiness)
	}
	if len(readiness.Lifecycle) != customerLifecycleMonths {
		t.Fatalf("lifecycle months = %d, want %d", len(readiness.Lifecycle), customerLifecycleMonths)
	}
	july := readiness.Lifecycle[len(readiness.Lifecycle)-2]
	august := readiness.Lifecycle[len(readiness.Lifecycle)-1]
	if july.Month != "2026-07" || july.NewSeatCount != 1 || july.ActiveSeatCount != 1 {
		t.Fatalf("July lifecycle = %#v", july)
	}
	if august.Month != "2026-08" || august.NaturalChurnCount != 1 || august.ActiveSeatCount != 0 {
		t.Fatalf("August lifecycle = %#v", august)
	}
}

func TestPredictionReadinessExcludesFullyRefundedRenewalButKeepsChurn(t *testing.T) {
	archivedAt := time.Date(2026, time.August, 21, 12, 0, 0, 0, cycle.Location)
	archived := []model.Subscription{{
		ID:           1,
		BusinessType: model.SubscriptionBusinessTeam,
		BoardedAt:    "2026-07-01",
		ArchivedAt:   &archivedAt,
	}}
	bills := []model.Bill{
		{ID: 1, SubscriptionID: 1, DueDate: "2026-07-01", AmountCents: 10000},
		{ID: 2, SubscriptionID: 1, DueDate: "2026-07-31", AmountCents: 10000},
	}
	cases := []model.AfterSalesCase{{
		SubscriptionID:    1,
		BillID:            2,
		BusinessType:      model.SubscriptionBusinessTeam,
		Source:            model.AfterSalesSourceCustomerCancellation,
		Status:            model.AfterSalesStatusRefunded,
		RefundAmountCents: 10000,
	}}

	readiness := buildPredictionReadiness(
		nil,
		archived,
		bills,
		cases,
		nil,
		time.Date(2026, time.August, 22, 9, 0, 0, 0, cycle.Location),
	)
	if readiness.RenewalSuccessCount != 0 || readiness.ChurnOutcomeCount != 1 {
		t.Fatalf("fully refunded renewal evidence = %#v", readiness)
	}
	for _, month := range readiness.Lifecycle {
		if month.RenewalSuccessCount != 0 {
			t.Fatalf("fully refunded lifecycle renewal = %#v", readiness.Lifecycle)
		}
	}
}

func TestPredictionReadinessDoesNotTreatAccountBanAsNaturalChurn(t *testing.T) {
	archivedAt := time.Date(2026, time.August, 21, 12, 0, 0, 0, cycle.Location)
	archived := []model.Subscription{{
		ID:           1,
		BusinessType: model.SubscriptionBusinessTeam,
		BoardedAt:    "2026-07-01",
		ArchivedAt:   &archivedAt,
	}}
	bills := []model.Bill{{ID: 1, SubscriptionID: 1, DueDate: "2026-07-01", AmountCents: 10000}}
	cases := []model.AfterSalesCase{{
		SubscriptionID: 1,
		BillID:         1,
		BusinessType:   model.SubscriptionBusinessTeam,
		Source:         model.AfterSalesSourceAccountBan,
		Status:         model.AfterSalesStatusRefunded,
	}}

	readiness := buildPredictionReadiness(
		nil,
		archived,
		bills,
		cases,
		nil,
		time.Date(2026, time.August, 22, 9, 0, 0, 0, cycle.Location),
	)
	if readiness.ChurnOutcomeCount != 0 {
		t.Fatalf("account-ban readiness = %#v", readiness)
	}
	if readiness.Lifecycle[len(readiness.Lifecycle)-1].NaturalChurnCount != 0 {
		t.Fatalf("account-ban lifecycle = %#v", readiness.Lifecycle)
	}
}

func TestCustomerLifecycleIncludesActiveSeatWithoutBill(t *testing.T) {
	now := time.Date(2026, time.August, 22, 9, 0, 0, 0, cycle.Location)
	createdAt := time.Date(2026, time.August, 20, 12, 0, 0, 0, cycle.Location)
	archivedAt := time.Date(2026, time.August, 21, 12, 0, 0, 0, cycle.Location)
	subscriptions := []model.Subscription{
		{
			ID:           1,
			BusinessType: model.SubscriptionBusinessTeam,
			BoardedAt:    "2026-08-20",
			CreatedAt:    createdAt,
		},
		{
			ID:           2,
			BusinessType: model.SubscriptionBusinessTeam,
			BoardedAt:    "2026-08-20",
			CreatedAt:    createdAt,
			ArchivedAt:   &archivedAt,
		},
	}

	lifecycle := buildCustomerLifecycle(subscriptions, nil, nil, nil, nil, now)
	august := lifecycle[len(lifecycle)-1]
	if august.NewSeatCount != 2 || august.ActiveSeatCount != 1 || august.TotalSeatCount != 1 {
		t.Fatalf("unbilled seat lifecycle = %#v", august)
	}
	if august.NaturalChurnCount != 0 {
		t.Fatalf("unbilled archive must not create churn evidence: %#v", august)
	}
}

func TestCustomerLifecycleIncludesFrozenSeatInTotal(t *testing.T) {
	now := time.Date(2026, time.August, 22, 9, 0, 0, 0, cycle.Location)
	archivedAt := time.Date(2026, time.August, 21, 12, 0, 0, 0, cycle.Location)
	frozenUntil := time.Date(2026, time.August, 28, 12, 0, 0, 0, cycle.Location)
	subscriptions := []model.Subscription{
		{
			ID:           1,
			BusinessType: model.SubscriptionBusinessTeam,
			BoardedAt:    "2026-08-01",
		},
		{
			ID:              2,
			BusinessType:    model.SubscriptionBusinessTeam,
			BoardedAt:       "2026-08-01",
			ArchivedAt:      &archivedAt,
			SeatFrozenUntil: &frozenUntil,
		},
	}

	lifecycle := buildCustomerLifecycle(subscriptions, nil, nil, nil, nil, now)
	august := lifecycle[len(lifecycle)-1]
	if august.ActiveSeatCount != 1 || august.TotalSeatCount != 2 {
		t.Fatalf("frozen seat lifecycle = %#v", august)
	}

	afterFreeze := buildCustomerLifecycle(
		subscriptions,
		nil,
		nil,
		nil,
		nil,
		time.Date(2026, time.August, 29, 9, 0, 0, 0, cycle.Location),
	)
	afterFreezeAugust := afterFreeze[len(afterFreeze)-1]
	if afterFreezeAugust.ActiveSeatCount != 1 || afterFreezeAugust.TotalSeatCount != 1 {
		t.Fatalf("expired frozen seat lifecycle = %#v", afterFreezeAugust)
	}
}
