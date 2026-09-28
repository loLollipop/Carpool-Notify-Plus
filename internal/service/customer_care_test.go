package service

import (
	"fmt"
	"reflect"
	"strings"
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
		BenefitType:        model.CustomerBenefitTypeExtension,
		BenefitName:        "赠送延期福利",
		ExtensionDays:      7,
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

func TestRecordCustomerBenefitsAcceptsNewAndLegacyTypesAndRejectsInvalid(t *testing.T) {
	tests := []struct {
		name               string
		benefitType        string
		wantType           string
		wantScheduledPrice bool
		wantError          bool
	}{
		{name: "extension", benefitType: model.CustomerBenefitTypeExtension, wantType: model.CustomerBenefitTypeExtension},
		{name: "price discount", benefitType: model.CustomerBenefitTypePriceDiscount, wantType: model.CustomerBenefitTypePriceDiscount, wantScheduledPrice: true},
		{name: "legacy renewal milestone", benefitType: model.CustomerBenefitTypeRenewalMilestone, wantType: model.CustomerBenefitTypeExtension},
		{name: "legacy loyalty care", benefitType: model.CustomerBenefitTypeLoyaltyCare, wantType: model.CustomerBenefitTypeExtension},
		{name: "legacy price increase", benefitType: model.CustomerBenefitTypePriceIncrease, wantType: model.CustomerBenefitTypePriceDiscount},
		{name: "legacy service recovery", benefitType: model.CustomerBenefitTypeServiceRecovery, wantType: model.CustomerBenefitTypeExtension},
		{name: "legacy manual", benefitType: model.CustomerBenefitTypeManual, wantType: model.CustomerBenefitTypeExtension},
		{name: "invalid", benefitType: "coupon", wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := openGoalTestService(t)
			ids := createCustomerCareTestSubscriptions(t, service, "benefit-test@example.com", "", 1)
			input := RecordCustomerBenefitsInput{
				SubscriptionIDs: ids,
				BenefitType:     test.benefitType,
				BenefitName:     "已线下发放",
				BenefitDate:     "2026-08-15",
			}
			switch test.benefitType {
			case model.CustomerBenefitTypeExtension:
				input.ExtensionDays = 7
			case model.CustomerBenefitTypePriceDiscount:
				input.PriceDiscountYuan = "10.00"
				input.OperationKey = "test-price-discount-accept"
			}
			recorded, err := service.RecordCustomerBenefits(input)
			if test.wantError {
				if err == nil {
					t.Fatalf("RecordCustomerBenefits(%q) succeeded, want error", test.benefitType)
				}
				return
			}
			if err != nil || recorded != 1 {
				t.Fatalf("RecordCustomerBenefits(%q) = %d, %v; want 1, nil", test.benefitType, recorded, err)
			}
			benefits, listErr := service.Store.ListCustomerBenefits()
			if listErr != nil {
				t.Fatal(listErr)
			}
			if len(benefits) != 1 || benefits[0].BenefitType != test.wantType {
				t.Fatalf("stored benefits = %#v", benefits)
			}
			subscription, getErr := service.Store.GetSubscription(ids[0])
			if getErr != nil {
				t.Fatal(getErr)
			}
			if test.wantScheduledPrice {
				if subscription.NextPriceCents == nil || *subscription.NextPriceCents != 9000 {
					t.Fatalf("scheduled price = %v; want 9000", subscription.NextPriceCents)
				}
			} else if subscription.NextPriceCents != nil || subscription.NextPriceEffectiveDueDate != "" {
				t.Fatalf("legacy record unexpectedly scheduled a price: %#v", subscription)
			}
		})
	}
}

func TestRecordCustomerBenefitsCanonicalizesAliasesBeforeDuplicateCheck(t *testing.T) {
	tests := []struct {
		name        string
		legacyType  string
		currentType string
	}{
		{
			name:        "extension alias",
			legacyType:  model.CustomerBenefitTypeServiceRecovery,
			currentType: model.CustomerBenefitTypeExtension,
		},
		{
			name:        "price discount alias",
			legacyType:  model.CustomerBenefitTypePriceIncrease,
			currentType: model.CustomerBenefitTypePriceDiscount,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := openGoalTestService(t)
			ids := createCustomerCareTestSubscriptions(t, service, "duplicate-alias@example.com", "", 1)
			benefitName := "同一份福利"
			if test.currentType == model.CustomerBenefitTypeExtension {
				benefitName = "赠送延期 7 天"
			} else {
				benefitName = "续费每期降价 ¥10.00（¥100.00 → ¥90.00）"
			}
			subscription, err := service.Store.GetSubscription(ids[0])
			if err != nil {
				t.Fatal(err)
			}
			if err := service.Store.CreateCustomerBenefits([]model.CustomerBenefit{{
				BatchID:                   "legacy-benefit",
				SubscriptionID:            ids[0],
				BenefitType:               test.legacyType,
				BenefitName:               benefitName,
				BenefitDate:               "2026-08-15",
				CustomerGroupSizeSnapshot: 1,
				CurrentPriceCentsSnapshot: subscription.PricePerPersonCents,
				CreatedAt:                 time.Date(2026, time.August, 15, 4, 0, 0, 0, time.UTC),
			}}); err != nil {
				t.Fatal(err)
			}
			input := RecordCustomerBenefitsInput{
				SubscriptionIDs: ids,
				BenefitType:     test.currentType,
				BenefitName:     benefitName,
				ExtensionDays:   7,
				BenefitDate:     "2026-08-15",
			}
			if test.currentType == model.CustomerBenefitTypePriceDiscount {
				input.PriceDiscountYuan = "10.00"
				input.OperationKey = "test-alias-price-discount"
			}
			if _, err := service.RecordCustomerBenefits(input); err == nil {
				t.Fatal("RecordCustomerBenefits() succeeded against historical alias, want duplicate error")
			}
			input.BenefitType = test.legacyType
			if _, err := service.RecordCustomerBenefits(input); err == nil {
				t.Fatal("legacy RecordCustomerBenefits() retry succeeded, want duplicate error")
			}
			benefits, err := service.Store.ListCustomerBenefits()
			if err != nil {
				t.Fatal(err)
			}
			if len(benefits) != 1 {
				t.Fatalf("benefits = %#v; want only the historical row", benefits)
			}
		})
	}
}

func TestRecordCustomerBenefitsRollsBackBatchWhenHistoricalAliasExists(t *testing.T) {
	service := openGoalTestService(t)
	ids := createCustomerCareTestSubscriptions(t, service, "duplicate-batch@example.com", "", 2)
	subscription, err := service.Store.GetSubscription(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Store.CreateCustomerBenefits([]model.CustomerBenefit{{
		BatchID:                   "legacy-batch",
		SubscriptionID:            ids[0],
		BenefitType:               model.CustomerBenefitTypeServiceRecovery,
		BenefitName:               "赠送延期 7 天",
		BenefitDate:               "2026-08-15",
		CustomerGroupSizeSnapshot: 1,
		CurrentPriceCentsSnapshot: subscription.PricePerPersonCents,
		CreatedAt:                 time.Date(2026, time.August, 15, 4, 0, 0, 0, time.UTC),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
		SubscriptionIDs: []int64{ids[1], ids[0]},
		BenefitType:     model.CustomerBenefitTypeExtension,
		BenefitName:     "赠送延期 7 天",
		ExtensionDays:   7,
		BenefitDate:     "2026-08-15",
	}); err == nil {
		t.Fatal("RecordCustomerBenefits() succeeded for batch containing a historical alias")
	}
	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil {
		t.Fatal(err)
	}
	if len(benefits) != 1 || benefits[0].SubscriptionID != ids[0] {
		t.Fatalf("benefits after rollback = %#v; want only the historical row", benefits)
	}
}

func TestRecordCustomerBenefitsRejectsStaleUITranslationKey(t *testing.T) {
	service := openGoalTestService(t)
	ids := createCustomerCareTestSubscriptions(t, service, "stale-ui@example.com", "", 1)
	if _, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
		SubscriptionIDs: ids,
		BenefitType:     model.CustomerBenefitTypeExtension,
		BenefitName:     "goals.care.defaultBenefitName.extension",
		BenefitDate:     "2026-08-15",
	}); err == nil || !strings.Contains(err.Error(), "刷新") {
		t.Fatalf("RecordCustomerBenefits() error = %v; want refresh error", err)
	}
}

func TestRecordCustomerBenefitsRequiresValidExtensionDays(t *testing.T) {
	tests := []struct {
		name      string
		days      int
		wantError bool
	}{
		{name: "missing", days: 0, wantError: true},
		{name: "negative", days: -1, wantError: true},
		{name: "above maximum", days: maximumBenefitExtensionDays + 1, wantError: true},
		{name: "valid", days: 7},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := openGoalTestService(t)
			ids := createCustomerCareTestSubscriptions(t, service, "extension-days@example.com", "", 1)
			recorded, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
				SubscriptionIDs: ids,
				BenefitType:     model.CustomerBenefitTypeExtension,
				BenefitName:     "客户端传入的名称不会覆盖延期天数",
				ExtensionDays:   test.days,
				BenefitDate:     "2026-08-15",
			})
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "延期天数") {
					t.Fatalf("RecordCustomerBenefits(days=%d) error = %v; want extension-days error", test.days, err)
				}
				return
			}
			if err != nil || recorded != 1 {
				t.Fatalf("RecordCustomerBenefits(days=%d) = %d, %v; want 1, nil", test.days, recorded, err)
			}
			benefits, listErr := service.Store.ListCustomerBenefits()
			if listErr != nil {
				t.Fatal(listErr)
			}
			if len(benefits) != 1 || benefits[0].BenefitName != "赠送延期 7 天" {
				t.Fatalf("stored benefits = %#v; want canonical extension name", benefits)
			}
			subscription, getErr := service.Store.GetSubscription(ids[0])
			if getErr != nil || subscription.NextPriceCents != nil || subscription.NextPriceEffectiveDueDate != "" {
				t.Fatalf("extension changed future price: %#v, %v", subscription, getErr)
			}
		})
	}
}

func TestRecordCustomerBenefitsSchedulesPriceDiscountForNextCycle(t *testing.T) {
	service := openGoalTestService(t)
	ids := createCustomerCareTestSubscriptions(t, service, "discount@example.com", "", 1)

	recorded, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
		SubscriptionIDs:   ids,
		BenefitType:       model.CustomerBenefitTypePriceDiscount,
		BenefitName:       "客户端名称不应被信任",
		OperationKey:      "test-discount-next-cycle",
		PriceDiscountYuan: "10.00",
		BenefitDate:       "2026-08-15",
	})
	if err != nil || recorded != 1 {
		t.Fatalf("RecordCustomerBenefits() = %d, %v; want 1, nil", recorded, err)
	}

	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil {
		t.Fatal(err)
	}
	const wantName = "续费每期降价 ¥10.00（¥100.00 → ¥90.00）"
	if len(benefits) != 1 || benefits[0].BenefitName != wantName {
		t.Fatalf("benefits = %#v; want generated name %q", benefits, wantName)
	}
	subscription, err := service.Store.GetSubscription(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if subscription.PricePerPersonCents != 10000 || subscription.NextPriceCents == nil ||
		*subscription.NextPriceCents != 9000 || subscription.NextPriceEffectiveDueDate != "2026-08-30" {
		t.Fatalf("subscription after discount = %#v", subscription)
	}
	if err := service.SetDuePaid(ids[0], "2026-08-30", true); err != nil {
		t.Fatal(err)
	}
	benefitPeriodBill, err := service.Store.GetBillByOccurrence(ids[0], "2026-08-30")
	if err != nil || benefitPeriodBill.AmountCents != 9000 {
		t.Fatalf("discounted bill = %#v, %v; want amount 9000", benefitPeriodBill, err)
	}
	applied, err := service.Store.GetSubscription(ids[0])
	if err != nil || applied.PricePerPersonCents != 9000 || applied.NextPriceCents != nil ||
		applied.NextPriceEffectiveDueDate != "" {
		t.Fatalf("subscription after discounted renewal = %#v, %v", applied, err)
	}
}

func TestRecordCustomerBenefitsPriceDiscountSkipsPrepaidCycles(t *testing.T) {
	service := openGoalTestService(t)
	subscriptionID := createCustomerCareTestSubscriptions(
		t,
		service,
		"prepaid-discount@example.com",
		"",
		1,
	)[0]
	for _, dueDate := range []string{"2026-08-30", "2026-09-29"} {
		if err := service.SetDuePaid(subscriptionID, dueDate, true); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
		SubscriptionIDs:   []int64{subscriptionID},
		BenefitType:       model.CustomerBenefitTypePriceDiscount,
		OperationKey:      "test-prepaid-cycle-discount",
		PriceDiscountYuan: "10.00",
		BenefitDate:       "2026-08-15",
	}); err != nil {
		t.Fatal(err)
	}
	scheduled, err := service.Store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if scheduled.NextPriceCents == nil || *scheduled.NextPriceCents != 9000 ||
		scheduled.NextPriceEffectiveDueDate != "2026-10-29" {
		t.Fatalf("scheduled discount = %#v; want first unpaid cycle 2026-10-29", scheduled)
	}
	repaired, err := service.NormalizeScheduledNextPriceEffectiveDates()
	if err != nil || repaired != 0 {
		t.Fatalf("startup normalization repaired prepaid discount = %d, %v; want 0, nil", repaired, err)
	}
	scheduled, err = service.Store.GetSubscription(subscriptionID)
	if err != nil || scheduled.NextPriceEffectiveDueDate != "2026-10-29" {
		t.Fatalf("discount after startup normalization = %#v, %v", scheduled, err)
	}
	for _, dueDate := range []string{"2026-08-30", "2026-09-29"} {
		bill, billErr := service.Store.GetBillByOccurrence(subscriptionID, dueDate)
		if billErr != nil || bill.AmountCents != 10000 {
			t.Fatalf("historical prepaid bill %s = %#v, %v", dueDate, bill, billErr)
		}
	}
	if err := service.SetDuePaid(subscriptionID, "2026-10-29", true); err != nil {
		t.Fatal(err)
	}
	discountedBill, err := service.Store.GetBillByOccurrence(subscriptionID, "2026-10-29")
	if err != nil || discountedBill.AmountCents != 9000 {
		t.Fatalf("first unpaid-cycle bill = %#v, %v; want 9000", discountedBill, err)
	}
	changes, err := service.Store.ListSubscriptionPriceChanges()
	if err != nil || len(changes) != 1 || changes[0].EffectiveDueDate != "2026-10-29" {
		t.Fatalf("price changes = %#v, %v", changes, err)
	}
}

func TestRecordCustomerBenefitsPriceDiscountUsesReopenedUnpaidCycle(t *testing.T) {
	service := openGoalTestService(t)
	subscriptionID := createCustomerCareTestSubscriptions(
		t,
		service,
		"reopened-cycle-discount@example.com",
		"",
		1,
	)[0]
	for _, dueDate := range []string{"2026-08-30", "2026-09-29"} {
		if err := service.SetDuePaid(subscriptionID, dueDate, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.SetDuePaid(subscriptionID, "2026-08-30", false); err != nil {
		t.Fatal(err)
	}

	if _, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
		SubscriptionIDs:   []int64{subscriptionID},
		BenefitType:       model.CustomerBenefitTypePriceDiscount,
		OperationKey:      "test-reopened-cycle-discount",
		PriceDiscountYuan: "10.00",
		BenefitDate:       "2026-08-15",
	}); err != nil {
		t.Fatal(err)
	}
	scheduled, err := service.Store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if scheduled.NextPriceCents == nil || *scheduled.NextPriceCents != 9000 ||
		scheduled.NextPriceEffectiveDueDate != "2026-08-30" {
		t.Fatalf("scheduled discount = %#v; want reopened first unpaid cycle 2026-08-30", scheduled)
	}
}

func TestRecordCustomerBenefitsPriceDiscountOperationKeyIsIdempotentAfterApply(t *testing.T) {
	service := openGoalTestService(t)
	subscriptionID := createCustomerCareTestSubscriptions(
		t,
		service,
		"idempotent-discount@example.com",
		"",
		1,
	)[0]
	input := RecordCustomerBenefitsInput{
		SubscriptionIDs:   []int64{subscriptionID},
		BenefitType:       model.CustomerBenefitTypePriceDiscount,
		OperationKey:      "test-idempotent-discount",
		PriceDiscountYuan: "10.00",
		BenefitDate:       "2026-08-15",
	}
	if _, err := service.RecordCustomerBenefits(input); err != nil {
		t.Fatal(err)
	}
	if err := service.SetDuePaid(subscriptionID, "2026-08-30", true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordCustomerBenefits(input); err == nil || !strings.Contains(err.Error(), "已登记") {
		t.Fatalf("replayed operation key error = %v; want already-recorded error", err)
	}
	subscription, err := service.Store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.PricePerPersonCents != 9000 || subscription.NextPriceCents != nil {
		t.Fatalf("replay scheduled or applied a second discount: %#v", subscription)
	}
	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil || len(benefits) != 1 {
		t.Fatalf("benefits after replay = %#v, %v", benefits, err)
	}
}

func TestRecordCustomerBenefitsRejectsInvalidPriceDiscount(t *testing.T) {
	tests := []string{"", "0", "-1", "1.001", "100.00", "101.00"}
	for _, discount := range tests {
		t.Run(discount, func(t *testing.T) {
			service := openGoalTestService(t)
			ids := createCustomerCareTestSubscriptions(t, service, "invalid-discount@example.com", "", 1)
			if _, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
				SubscriptionIDs:   ids,
				BenefitType:       model.CustomerBenefitTypePriceDiscount,
				OperationKey:      "test-invalid-discount",
				PriceDiscountYuan: discount,
				BenefitDate:       "2026-08-15",
			}); err == nil {
				t.Fatalf("RecordCustomerBenefits(discount=%q) succeeded", discount)
			}
			benefits, listErr := service.Store.ListCustomerBenefits()
			if listErr != nil || len(benefits) != 0 {
				t.Fatalf("benefits after invalid discount = %#v, %v", benefits, listErr)
			}
			subscription, getErr := service.Store.GetSubscription(ids[0])
			if getErr != nil || subscription.NextPriceCents != nil {
				t.Fatalf("subscription after invalid discount = %#v, %v", subscription, getErr)
			}
		})
	}
}

func TestRecordCustomerBenefitsPriceDiscountRequiresValidOperationKey(t *testing.T) {
	for _, operationKey := range []string{"", "short", "invalid operation key"} {
		t.Run(operationKey, func(t *testing.T) {
			service := openGoalTestService(t)
			ids := createCustomerCareTestSubscriptions(t, service, "operation-key@example.com", "", 1)
			if _, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
				SubscriptionIDs:   ids,
				BenefitType:       model.CustomerBenefitTypePriceDiscount,
				OperationKey:      operationKey,
				PriceDiscountYuan: "10.00",
				BenefitDate:       "2026-08-15",
			}); err == nil {
				t.Fatalf("RecordCustomerBenefits(operation key %q) succeeded", operationKey)
			}
		})
	}
}

func TestRecordCustomerBenefitsDiscountsOnlyExplicitSubscriptionIDs(t *testing.T) {
	service := openGoalTestService(t)
	ids := createCustomerCareTestSubscriptions(t, service, "shared-discount@example.com", "", 2)
	if _, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
		SubscriptionIDs:   []int64{ids[1]},
		BenefitType:       model.CustomerBenefitTypePriceDiscount,
		OperationKey:      "test-explicit-subscription",
		PriceDiscountYuan: "10.00",
		BenefitDate:       "2026-08-15",
	}); err != nil {
		t.Fatal(err)
	}
	first, err := service.Store.GetSubscription(ids[0])
	if err != nil || first.NextPriceCents != nil {
		t.Fatalf("unselected related subscription changed: %#v, %v", first, err)
	}
	second, err := service.Store.GetSubscription(ids[1])
	if err != nil || second.NextPriceCents == nil || *second.NextPriceCents != 9000 {
		t.Fatalf("selected subscription not discounted: %#v, %v", second, err)
	}
	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil || len(benefits) != 1 || benefits[0].SubscriptionID != ids[1] {
		t.Fatalf("benefits expanded beyond explicit target: %#v, %v", benefits, err)
	}
}

func TestRecordCustomerBenefitsPriceDiscountBatchSharesOperationKey(t *testing.T) {
	service := openGoalTestService(t)
	ids := createCustomerCareTestSubscriptions(t, service, "batch-discount@example.com", "", 2)
	if recorded, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
		SubscriptionIDs:   ids,
		BenefitType:       model.CustomerBenefitTypePriceDiscount,
		OperationKey:      "test-batch-discount-operation",
		PriceDiscountYuan: "10.00",
		BenefitDate:       "2026-08-15",
	}); err != nil || recorded != 2 {
		t.Fatalf("RecordCustomerBenefits() = %d, %v; want 2, nil", recorded, err)
	}
	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil || len(benefits) != 2 || benefits[0].BatchID != benefits[1].BatchID {
		t.Fatalf("batch benefits = %#v, %v", benefits, err)
	}
}

func TestRecordCustomerBenefitsRollsBackDiscountBatchWhenOneHasScheduledPrice(t *testing.T) {
	service := openGoalTestService(t)
	firstID := createCustomerCareTestSubscriptions(t, service, "discount-first@example.com", "", 1)[0]
	secondID := createCustomerCareTestSubscriptions(t, service, "discount-second@example.com", "", 1)[0]
	second, err := service.Store.GetSubscription(secondID)
	if err != nil {
		t.Fatal(err)
	}
	alreadyScheduledPrice := int64(9500)
	second.NextPriceCents = &alreadyScheduledPrice
	if err := service.configureNextPrice(second, &second, false); err != nil {
		t.Fatal(err)
	}
	if err := service.Store.UpdateSubscriptionNextPrices([]model.Subscription{second}, "2026-08-15"); err != nil {
		t.Fatal(err)
	}

	if _, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
		SubscriptionIDs:   []int64{firstID, secondID},
		BenefitType:       model.CustomerBenefitTypePriceDiscount,
		OperationKey:      "test-scheduled-price-rollback",
		PriceDiscountYuan: "10.00",
		BenefitDate:       "2026-08-15",
	}); err == nil {
		t.Fatal("RecordCustomerBenefits() succeeded with an existing next-price schedule")
	}
	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil || len(benefits) != 0 {
		t.Fatalf("partial benefits committed: %#v, %v", benefits, err)
	}
	first, err := service.Store.GetSubscription(firstID)
	if err != nil || first.NextPriceCents != nil {
		t.Fatalf("first subscription was partially scheduled: %#v, %v", first, err)
	}
	second, err = service.Store.GetSubscription(secondID)
	if err != nil || second.NextPriceCents == nil || *second.NextPriceCents != alreadyScheduledPrice {
		t.Fatalf("existing second schedule changed: %#v, %v", second, err)
	}
}

func TestRecordCustomerBenefitsRollsBackDiscountWhenPricingExemptionIsActive(t *testing.T) {
	service := openGoalTestService(t)
	subscriptionID := createCustomerCareTestSubscriptions(
		t,
		service,
		"discount-exemption@example.com",
		"",
		1,
	)[0]
	if err := service.Store.CreatePricingExemptions([]model.PricingExemption{{
		SubscriptionID:     subscriptionID,
		ReasonCode:         "manual",
		ReviewAfter:        "2026-09-30",
		ReviewCycles:       1,
		PriceCentsSnapshot: 10000,
	}}, "2026-08-15"); err != nil {
		t.Fatal(err)
	}

	if _, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
		SubscriptionIDs:   []int64{subscriptionID},
		BenefitType:       model.CustomerBenefitTypePriceDiscount,
		OperationKey:      "test-pricing-exemption-rollback",
		PriceDiscountYuan: "10.00",
		BenefitDate:       "2026-08-15",
	}); err == nil {
		t.Fatal("RecordCustomerBenefits() succeeded during an active pricing exemption")
	}
	benefits, err := service.Store.ListCustomerBenefits()
	if err != nil || len(benefits) != 0 {
		t.Fatalf("benefit committed despite pricing exemption: %#v, %v", benefits, err)
	}
	subscription, err := service.Store.GetSubscription(subscriptionID)
	if err != nil || subscription.NextPriceCents != nil || subscription.NextPriceEffectiveDueDate != "" {
		t.Fatalf("price scheduled despite pricing exemption: %#v, %v", subscription, err)
	}
}

func TestCustomerBenefitRecommendationsUseOnlyCurrentTypes(t *testing.T) {
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, cycle.Location)
	tests := []struct {
		name     string
		member   PricingCandidate
		wantType string
	}{
		{
			name:     "default review",
			member:   PricingCandidate{RenewalCount: 2},
			wantType: model.CustomerBenefitTypeExtension,
		},
		{
			name:     "first cycle",
			member:   PricingCandidate{RenewalCount: 0},
			wantType: model.CustomerBenefitTypeExtension,
		},
		{
			name: "service recovery",
			member: PricingCandidate{
				RenewalCount:   2,
				BlockedCode:    "after_sales_recovery",
				NextReviewDate: "2026-08-20",
			},
			wantType: model.CustomerBenefitTypeExtension,
		},
		{
			name: "accepted increase",
			member: PricingCandidate{
				RenewalCount:             2,
				PaidPeriodsAfterIncrease: 1,
				LastPriceIncreaseDate:    "2026-07-01",
			},
			wantType: model.CustomerBenefitTypePriceDiscount,
		},
		{
			name:     "first renewal",
			member:   PricingCandidate{RenewalCount: 1},
			wantType: model.CustomerBenefitTypeExtension,
		},
		{
			name:     "core retention",
			member:   PricingCandidate{RenewalCount: 2, CustomerTier: "core"},
			wantType: model.CustomerBenefitTypeExtension,
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			member := test.member
			member.SubscriptionID = int64(index + 1)
			member.NextDueDate = "2026-09-15"
			candidate := buildCustomerBenefitCandidate(
				customerBenefitGroup{ID: member.SubscriptionID, Members: []PricingCandidate{member}},
				nil,
				now,
			)
			if candidate.SuggestedBenefitType != test.wantType {
				t.Fatalf("suggested type = %q, want %q; candidate = %#v", candidate.SuggestedBenefitType, test.wantType, candidate)
			}
			if candidate.SuggestedBenefitType != model.CustomerBenefitTypeExtension &&
				candidate.SuggestedBenefitType != model.CustomerBenefitTypePriceDiscount {
				t.Fatalf("legacy recommendation type returned: %q", candidate.SuggestedBenefitType)
			}
		})
	}
}

func TestCustomerBenefitRecommendationDeduplicatesLegacyAndCurrentTypes(t *testing.T) {
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, cycle.Location)
	tests := []struct {
		name          string
		member        PricingCandidate
		benefitType   string
		benefitDate   string
		notReasonCode string
	}{
		{
			name:          "legacy service recovery",
			member:        PricingCandidate{RenewalCount: 2, BlockedCode: "after_sales_recovery", NextReviewDate: "2026-08-20"},
			benefitType:   model.CustomerBenefitTypeServiceRecovery,
			benefitDate:   "2026-07-22",
			notReasonCode: "service_recovery",
		},
		{
			name:          "current extension after recovery",
			member:        PricingCandidate{RenewalCount: 2, BlockedCode: "after_sales_recovery", NextReviewDate: "2026-08-20"},
			benefitType:   model.CustomerBenefitTypeExtension,
			benefitDate:   "2026-07-22",
			notReasonCode: "service_recovery",
		},
		{
			name:          "legacy increase thank-you",
			member:        PricingCandidate{RenewalCount: 2, PaidPeriodsAfterIncrease: 1, LastPriceIncreaseDate: "2026-01-01"},
			benefitType:   model.CustomerBenefitTypePriceIncrease,
			benefitDate:   "2026-01-02",
			notReasonCode: "increase_accepted",
		},
		{
			name:          "current price discount",
			member:        PricingCandidate{RenewalCount: 2, PaidPeriodsAfterIncrease: 1, LastPriceIncreaseDate: "2026-01-01"},
			benefitType:   model.CustomerBenefitTypePriceDiscount,
			benefitDate:   "2026-01-02",
			notReasonCode: "increase_accepted",
		},
		{
			name:          "legacy first-renewal milestone",
			member:        PricingCandidate{RenewalCount: 1},
			benefitType:   model.CustomerBenefitTypeRenewalMilestone,
			benefitDate:   "2026-01-02",
			notReasonCode: "first_renewal",
		},
		{
			name:          "current extension after first renewal",
			member:        PricingCandidate{RenewalCount: 1},
			benefitType:   model.CustomerBenefitTypeExtension,
			benefitDate:   "2026-01-02",
			notReasonCode: "first_renewal",
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			member := test.member
			member.SubscriptionID = int64(index + 1)
			member.NextDueDate = "2026-09-15"
			candidate := buildCustomerBenefitCandidate(
				customerBenefitGroup{ID: member.SubscriptionID, Members: []PricingCandidate{member}},
				[]model.CustomerBenefit{{BenefitType: test.benefitType, BenefitDate: test.benefitDate}},
				now,
			)
			if candidate.ReasonCode == test.notReasonCode {
				t.Fatalf("duplicate recommendation was not suppressed: %#v", candidate)
			}
		})
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
	if !reflect.DeepEqual(candidate.SubscriptionIDs, ids) {
		t.Fatalf("candidate subscription IDs = %#v, want %#v", candidate.SubscriptionIDs, ids)
	}

	if _, err := service.RecordCustomerBenefits(RecordCustomerBenefitsInput{
		SubscriptionIDs:    []int64{candidate.SubscriptionID},
		BenefitType:        model.CustomerBenefitTypeRenewalMilestone,
		BenefitName:        "首次续费礼",
		ExtensionDays:      7,
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

func TestCustomerCareNormalizesMixedBillingCyclesToMonthlyValue(t *testing.T) {
	service := openGoalTestService(t)
	accountID, err := service.CreateAccount(CreateAccountInput{
		Name:      "mixed-cycle-owner",
		OpenedAt:  "2026-06-01",
		SeatNames: []string{"monthly", "quarterly"},
	})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := service.Store.ListSeatsByAccount(accountID)
	if err != nil || len(seats) != 2 {
		t.Fatalf("seats = %#v, err = %v", seats, err)
	}
	for index, input := range []struct {
		price string
		cycle string
	}{
		{price: "100.00", cycle: "interval:30d"},
		{price: "330.00", cycle: "interval:90d"},
	} {
		if _, createErr := service.Create(CreateInput{
			Name:             "mixed-cycle-customer",
			PriceYuan:        input.price,
			CronExpr:         input.cycle,
			NotifyOffsetsRaw: "3,1,0",
			SeatID:           seats[index].ID,
			BoardedAt:        "2026-06-01",
			CustomerEmail:    "mixed@example.com",
			CustomerWechat:   "mixed-wechat",
		}); createErr != nil {
			t.Fatal(createErr)
		}
	}
	candidates, err := service.buildPricingCandidates(nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	care, err := service.buildCustomerCare(candidates)
	if err != nil {
		t.Fatal(err)
	}
	if len(care.Candidates) != 1 || care.Candidates[0].MonthlyValueCents != 21000 ||
		care.Candidates[0].CurrentCycleValueCents != 21000 {
		t.Fatalf("normalized customer value = %#v, want 21000 cents/month", care.Candidates)
	}
}

func TestCustomerBenefitOutcomeTracksPartialMultiSeatRenewal(t *testing.T) {
	benefit := model.CustomerBenefit{
		SubscriptionID:            1,
		NextDueDateSnapshot:       "2026-08-01",
		CustomerWechatSnapshot:    "same-contact",
		CustomerGroupSizeSnapshot: 2,
	}
	subscriptions := []model.Subscription{
		{ID: 1, CustomerWechat: "same-contact"},
		{ID: 2, CustomerWechat: "same-contact"},
	}
	billsBySubscription := map[int64][]model.Bill{
		1: {{SubscriptionID: 1, DueDate: "2026-08-01", AmountCents: 10000}},
	}
	outcome := customerBenefitOutcome(
		benefit,
		subscriptions,
		billsBySubscription,
		time.Date(2026, time.August, 10, 12, 0, 0, 0, cycle.Location),
	)
	if outcome.Status != "partially_renewed" || outcome.RenewedSeatCount != 1 ||
		outcome.ExpectedSeatCount != 2 || outcome.RetainedSeatPercent != 50 {
		t.Fatalf("partial multi-seat outcome = %#v", outcome)
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
	if len(august.Outcomes) != 1 || august.Outcomes[0].Date != "2026-08-21" ||
		august.Outcomes[0].Kind != "churn" {
		t.Fatalf("August lifecycle outcomes = %#v", august.Outcomes)
	}
}

func TestCustomerLifecycleOrdersRenewalAndChurnOutcomesByDate(t *testing.T) {
	now := time.Date(2026, time.August, 31, 12, 0, 0, 0, cycle.Location)
	firstChurnAt := time.Date(2026, time.August, 10, 18, 0, 0, 0, cycle.Location)
	secondChurnAt := time.Date(2026, time.August, 25, 18, 0, 0, 0, cycle.Location)
	subscriptions := []model.Subscription{
		{ID: 1, BusinessType: model.SubscriptionBusinessTeam, BoardedAt: "2026-07-01"},
		{ID: 2, BusinessType: model.SubscriptionBusinessTeam, BoardedAt: "2026-07-02", ArchivedAt: &firstChurnAt},
		{ID: 3, BusinessType: model.SubscriptionBusinessTeam, BoardedAt: "2026-07-03"},
		{ID: 4, BusinessType: model.SubscriptionBusinessTeam, BoardedAt: "2026-07-04", ArchivedAt: &secondChurnAt},
	}
	bills := []model.Bill{
		{ID: 1, SubscriptionID: 1, DueDate: "2026-07-01"},
		{ID: 2, SubscriptionID: 1, DueDate: "2026-08-05"},
		{ID: 3, SubscriptionID: 2, DueDate: "2026-07-02"},
		{ID: 4, SubscriptionID: 3, DueDate: "2026-07-03"},
		{ID: 5, SubscriptionID: 3, DueDate: "2026-08-20"},
		{ID: 6, SubscriptionID: 4, DueDate: "2026-07-04"},
	}

	lifecycle := buildCustomerLifecycle(subscriptions, bills, nil, nil, nil, now)
	august := lifecycle[len(lifecycle)-1]
	if august.RenewalSuccessCount != 2 || august.NaturalChurnCount != 2 {
		t.Fatalf("August lifecycle counts = %#v, want two renewals and two churns", august)
	}
	want := []CustomerLifecycleOutcome{
		{Date: "2026-08-05", Kind: "renewal"},
		{Date: "2026-08-10", Kind: "churn"},
		{Date: "2026-08-20", Kind: "renewal"},
		{Date: "2026-08-25", Kind: "churn"},
	}
	if !reflect.DeepEqual(august.Outcomes, want) {
		t.Fatalf("August lifecycle outcomes = %#v, want %#v", august.Outcomes, want)
	}
}

func TestCustomerLifecycleUsesPaymentDateForCrossMonthRenewals(t *testing.T) {
	now := time.Date(2026, time.September, 30, 12, 0, 0, 0, cycle.Location)
	earlyPaidAt := time.Date(2026, time.August, 30, 18, 0, 0, 0, cycle.Location)
	latePaidAt := time.Date(2026, time.September, 2, 18, 0, 0, 0, cycle.Location)
	subscriptions := []model.Subscription{
		{ID: 1, BusinessType: model.SubscriptionBusinessTeam, BoardedAt: "2026-08-01"},
		{ID: 2, BusinessType: model.SubscriptionBusinessTeam, BoardedAt: "2026-08-02"},
	}
	bills := []model.Bill{
		{ID: 1, SubscriptionID: 1, DueDate: "2026-08-01"},
		{ID: 2, SubscriptionID: 1, DueDate: "2026-09-01", PaidAt: earlyPaidAt},
		{ID: 3, SubscriptionID: 2, DueDate: "2026-08-02"},
		{ID: 4, SubscriptionID: 2, DueDate: "2026-08-31", PaidAt: latePaidAt},
	}

	lifecycle := buildCustomerLifecycle(subscriptions, bills, nil, nil, nil, now)
	august := lifecycle[len(lifecycle)-2]
	september := lifecycle[len(lifecycle)-1]
	if august.RenewalSuccessCount != 1 || !reflect.DeepEqual(
		august.Outcomes,
		[]CustomerLifecycleOutcome{{Date: "2026-08-30", Kind: "renewal"}},
	) {
		t.Fatalf("August cross-month renewal = %#v", august)
	}
	if september.RenewalSuccessCount != 1 || !reflect.DeepEqual(
		september.Outcomes,
		[]CustomerLifecycleOutcome{{Date: "2026-09-02", Kind: "renewal"}},
	) {
		t.Fatalf("September cross-month renewal = %#v", september)
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
	if august.Outcomes == nil || len(august.Outcomes) != 0 {
		t.Fatalf("unbilled seat outcomes = %#v, want an initialized empty timeline", august.Outcomes)
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
