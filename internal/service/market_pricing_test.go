package service

import (
	"carpool-notify/internal/model"
	"testing"
	"time"
)

func TestPricingWeightedSinglePriceRespondsToCustomersAndMarket(t *testing.T) {
	service := openGoalTestService(t)
	seedPricingSeats(t, service, 10, 9, 10000)
	snapshot := &model.MarketPriceSnapshot{Provider: marketProvider, Product: marketAcquisitionProduct, LowPriceCents: 13000, MedianPriceCents: 14000, HighPriceCents: 15000, SampleCount: 5, SourceUpdatedAt: service.now(), CreatedAt: service.now()}
	before, err := service.buildPricingRecommendation(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.LowPriceCents += 2000
	snapshot.MedianPriceCents += 2000
	snapshot.HighPriceCents += 2000
	higherMarket, err := service.buildPricingRecommendation(snapshot)
	if err != nil || higherMarket.SuggestedPriceCents <= before.SuggestedPriceCents {
		t.Fatalf("market failed to change weighted price: before=%#v after=%#v err=%v", before, higherMarket, err)
	}
	subscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		t.Fatal(err)
	}
	for _, subscription := range subscriptions {
		subscription.PricePerPersonCents += 1000
		if err := service.Store.UpdateSubscription(subscription); err != nil {
			t.Fatal(err)
		}
	}
	higherCustomers, err := service.buildPricingRecommendation(snapshot)
	if err != nil || higherCustomers.SuggestedPriceCents <= higherMarket.SuggestedPriceCents || higherCustomers.MarketWeightPercent != 40 || higherCustomers.Basis != "market_and_customers" {
		t.Fatalf("customers failed to change weighted price: before=%#v after=%#v err=%v", higherMarket, higherCustomers, err)
	}
	if before.InternalSampleCount != 9 || before.InternalMeanPriceCents != 10000 || before.InternalLowPriceCents != 10000 || before.InternalHighPriceCents != 10000 || before.SuggestedPriceCents < before.SuggestedLowPriceCents || before.SuggestedPriceCents > before.SuggestedHighPriceCents {
		t.Fatalf("single price or distribution=%#v", before)
	}
	customersOnly, err := service.buildPricingRecommendation(nil)
	if err != nil || customersOnly.Basis != "customers_only" || customersOnly.SuggestedPriceCents <= 0 || customersOnly.MarketWeightPercent != 0 || customersOnly.Action == "raise" {
		t.Fatalf("customers-only fallback=%#v, %v", customersOnly, err)
	}
	snapshot.SourceUpdatedAt = service.now().Add(-25 * time.Hour)
	stale, err := service.buildPricingRecommendation(snapshot)
	if err != nil || stale.Action == "raise" || stale.Basis != "customers_only" {
		t.Fatalf("expired source affected price=%#v, %v", stale, err)
	}
}
func TestPricingContractDistributionNormalizesQuarterAndExcludesInvalidSeats(t *testing.T) {
	service := openGoalTestService(t)
	seedPricingSeats(t, service, 12, 10, 10000)
	subscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		t.Fatal(err)
	}
	// Four retained monthly samples: 80, 100, 120 (quarterly 360), 200.
	for index, subscription := range subscriptions {
		switch index {
		case 0:
			subscription.PricePerPersonCents = 8000
		case 1:
			subscription.PricePerPersonCents = 10000
		case 2:
			subscription.PricePerPersonCents = 36000
			subscription.CronExpr = "interval:90d"
		case 3:
			subscription.PricePerPersonCents = 20000
		case 4:
			subscription.CronExpr = "invalid period"
			subscription.PricePerPersonCents = 999999
		case 5:
			subscription.IsResale = true
			subscription.PricePerPersonCents = 999999
		case 6:
			subscription.PricePerPersonCents = 0
		case 7:
			if err := service.Store.ArchiveSubscription(subscription.ID); err != nil {
				t.Fatal(err)
			}
			continue
		case 8, 9:
			subscription.PricePerPersonCents = 999999
		}
		if err := service.Store.UpdateSubscription(subscription); err != nil {
			t.Fatal(err)
		}
	}
	// One cancellation-pending seat and one banned-account seat are omitted.
	if _, err := service.Store.RequestSubscriptionCancellation(subscriptions[8].ID, service.now(), service.now().Add(24*time.Hour), 30); err != nil {
		t.Fatal(err)
	}
	// Move the final seat onto another account before banning that account.
	otherID, err := service.CreateAccount(CreateAccountInput{Name: "banned-pricing@example.com", SeatCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	seats, err := service.Store.ListSeatsByAccount(otherID)
	if err != nil {
		t.Fatal(err)
	}
	last, err := service.Store.GetSubscription(subscriptions[9].ID)
	if err != nil {
		t.Fatal(err)
	}
	last.SeatID = seats[0].ID
	if err := service.Store.UpdateSubscription(last); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Store.BanAccountAndCreateAfterSalesCases(otherID, "2026-08-14", "ban", 30); err != nil {
		t.Fatal(err)
	}
	advice, err := service.buildPricingRecommendation(nil)
	if err != nil || advice.InternalSampleCount != 4 || advice.InternalMedianPriceCents != 11000 || advice.InternalMeanPriceCents != 12500 || advice.InternalLowPriceCents != 9500 || advice.InternalHighPriceCents != 14000 {
		t.Fatalf("contract distribution=%#v, %v", advice, err)
	}
}
func TestPricingFallbackCostAndInsufficientData(t *testing.T) {
	service := openGoalTestService(t)
	empty, err := service.buildPricingRecommendation(nil)
	if err != nil || empty.Basis != "insufficient" || empty.SuggestedPriceCents != 0 {
		t.Fatalf("empty advice=%#v, %v", empty, err)
	}
	seedPricingSeats(t, service, 10, 0, 10000)
	cost, err := service.buildPricingRecommendation(nil)
	if err != nil || cost.Basis != "cost_only" || cost.SuggestedPriceCents < cost.SeatCostFloorCents*120/100 || cost.InternalSampleCount != 0 || cost.Action == "raise" {
		t.Fatalf("cost-only advice=%#v, %v", cost, err)
	}
}
func TestCustomerBenefitMinimumCurrentPriceIncludesZeroAndQuarterlyContract(t *testing.T) {
	group := customerBenefitGroup{Members: []PricingCandidate{{SubscriptionID: 1, CurrentPriceCents: 33000}, {SubscriptionID: 2, CurrentPriceCents: 9000}}}
	candidate := buildCustomerBenefitCandidate(group, nil, time.Now())
	if candidate.MinimumCurrentPriceCents != 9000 {
		t.Fatalf("minimum per-period contract price=%d", candidate.MinimumCurrentPriceCents)
	}
	group.Members = append(group.Members, PricingCandidate{SubscriptionID: 3, CurrentPriceCents: 0})
	candidate = buildCustomerBenefitCandidate(group, nil, time.Now())
	if candidate.MinimumCurrentPriceCents != 0 {
		t.Fatalf("free seat omitted from minimum=%d", candidate.MinimumCurrentPriceCents)
	}
}
