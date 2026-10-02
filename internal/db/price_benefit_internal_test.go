package db

import (
	"path/filepath"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

func createPriceBenefitTestSubscription(
	t *testing.T,
	store *Store,
	accountID int64,
	name string,
	email string,
	price int64,
) model.Subscription {
	t.Helper()
	seatID, err := store.CreateSeat(model.Seat{AccountID: accountID, Name: name + "-seat"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateSubscription(model.Subscription{
		Name: name, BusinessType: model.SubscriptionBusinessTeam,
		CustomerEmail: email, PricePerPersonCents: price,
		CronExpr: "interval:30d", BoardedAt: "2026-08-01", SeatID: seatID,
	})
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := store.GetSubscription(id)
	if err != nil {
		t.Fatal(err)
	}
	return subscription
}

func newPriceBenefitTestStore(t *testing.T) (*Store, int64) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "price-benefits.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	accountID, err := store.CreateAccount(model.Account{Name: "price-owner"}, 0, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	return store, accountID
}

func createExplicitPendingPriceDiscount(
	t *testing.T,
	store *Store,
	subscription model.Subscription,
	after int64,
) (model.Subscription, model.CustomerBenefit) {
	t.Helper()
	now := time.Now()
	createdAt := now.UTC().Add(-time.Minute).Truncate(time.Second)
	subscription.NextPriceCents = &after
	benefit := model.CustomerBenefit{
		BatchID:                   "benefit-operation-v1:explicit-price-discount",
		SubscriptionID:            subscription.ID,
		BenefitType:               model.CustomerBenefitTypePriceDiscount,
		BenefitName:               "人工确认的续费优惠",
		ActualCostCents:           321,
		PerceivedValueCents:       654,
		BenefitDate:               cycle.FormatDate(now.In(cycle.Location)),
		CustomerEmailSnapshot:     subscription.CustomerEmail,
		CustomerWechatSnapshot:    subscription.CustomerWechat,
		CustomerTierSnapshot:      "important",
		CustomerGroupSizeSnapshot: 1,
		CurrentPriceCentsSnapshot: subscription.PricePerPersonCents,
		RenewalCountSnapshot:      2,
		RecommendationCode:        "operator_retention_offer",
		Note:                      "保留人工核定的业务信息",
		CreatedAt:                 createdAt,
	}
	if err := store.CreateCustomerBenefitsAndUpdateSubscriptionNextPrices(
		[]model.CustomerBenefit{benefit},
		[]model.Subscription{subscription},
		now,
		benefit.BenefitDate,
	); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetSubscription(subscription.ID)
	if err != nil {
		t.Fatal(err)
	}
	benefits, err := store.ListCustomerBenefits()
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range benefits {
		if candidate.SubscriptionID == subscription.ID {
			return stored, candidate
		}
	}
	t.Fatal("explicit price-discount benefit was not stored")
	return model.Subscription{}, model.CustomerBenefit{}
}

func TestNextPriceDiscountsSynchronizeBenefitsButIncreasesDoNot(t *testing.T) {
	store, accountID := newPriceBenefitTestStore(t)
	discount := createPriceBenefitTestSubscription(t, store, accountID, "discount", "discount@example.com", 10000)
	discountPrice := int64(9000)
	discount.NextPriceCents = &discountPrice
	discount.NextPriceEffectiveDueDate = "2026-08-31"
	if err := store.UpdateSubscriptionNextPrices([]model.Subscription{discount}, "2026-08-15"); err != nil {
		t.Fatal(err)
	}

	increase := createPriceBenefitTestSubscription(t, store, accountID, "increase", "increase@example.com", 10000)
	increasePrice := int64(11000)
	increase.NextPriceCents = &increasePrice
	increase.NextPriceEffectiveDueDate = "2026-08-31"
	if err := store.UpdateSubscriptionNextPrices([]model.Subscription{increase}, "2026-08-15"); err != nil {
		t.Fatal(err)
	}

	benefits, err := store.ListCustomerBenefits()
	if err != nil {
		t.Fatal(err)
	}
	if len(benefits) != 1 {
		t.Fatalf("benefits = %#v; want only the discount", benefits)
	}
	benefit := benefits[0]
	if benefit.SubscriptionID != discount.ID || benefit.ActualCostCents != 0 ||
		benefit.PerceivedValueCents != 1000 || benefit.PriceBeforeCents != 10000 ||
		benefit.PriceAfterCents != 9000 || benefit.PriceEffectiveDueDate != "2026-08-31" ||
		benefit.NextDueDateSnapshot != "2026-08-31" || benefit.PriceAdjustmentKey == "" {
		t.Fatalf("synchronized benefit = %#v", benefit)
	}
}

func TestOrdinarySubscriptionEditSynchronizesNextPriceDiscount(t *testing.T) {
	store, accountID := newPriceBenefitTestStore(t)
	subscription := createPriceBenefitTestSubscription(t, store, accountID, "edited", "edited@example.com", 10000)
	nextPrice := int64(8500)
	subscription.NextPriceCents = &nextPrice
	subscription.NextPriceEffectiveDueDate = "2026-08-31"
	if err := store.UpdateSubscription(subscription); err != nil {
		t.Fatal(err)
	}
	benefits, err := store.ListCustomerBenefits()
	if err != nil || len(benefits) != 1 || benefits[0].PerceivedValueCents != 1500 {
		t.Fatalf("benefits after ordinary edit = %#v, %v", benefits, err)
	}
	stored, err := store.GetSubscription(subscription.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.NextPriceCents = nil
	stored.NextPriceEffectiveDueDate = ""
	if err := store.UpdateSubscription(stored); err != nil {
		t.Fatal(err)
	}
	benefits, err = store.ListCustomerBenefits()
	if err != nil || len(benefits) != 0 {
		t.Fatalf("canceled pending discount left a benefit = %#v, %v", benefits, err)
	}
	currentPriceOnly := createPriceBenefitTestSubscription(t, store, accountID, "current-only", "current@example.com", 10000)
	currentPriceOnly.PricePerPersonCents = 9000
	if err := store.UpdateSubscription(currentPriceOnly); err != nil {
		t.Fatal(err)
	}
	benefits, err = store.ListCustomerBenefits()
	if err != nil || len(benefits) != 0 {
		t.Fatalf("current-price edit created a benefit = %#v, %v", benefits, err)
	}
}

func TestCurrentPriceEditReclassifiesPendingDiscount(t *testing.T) {
	store, accountID := newPriceBenefitTestStore(t)
	subscription := createPriceBenefitTestSubscription(t, store, accountID, "repriced", "repriced@example.com", 10000)
	nextPrice := int64(9000)
	subscription.NextPriceCents = &nextPrice
	subscription.NextPriceEffectiveDueDate = "2026-10-01"
	if err := store.UpdateSubscription(subscription); err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetSubscription(subscription.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.PricePerPersonCents = 11000
	if err := store.UpdateSubscription(stored); err != nil {
		t.Fatal(err)
	}
	benefits, err := store.ListCustomerBenefits()
	if err != nil || len(benefits) != 1 || benefits[0].PriceBeforeCents != 11000 || benefits[0].PriceAfterCents != 9000 {
		t.Fatalf("reclassified discount = %#v, %v", benefits, err)
	}
	stored, err = store.GetSubscription(subscription.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.PricePerPersonCents = 8000
	if err := store.UpdateSubscription(stored); err != nil {
		t.Fatal(err)
	}
	benefits, err = store.ListCustomerBenefits()
	if err != nil || len(benefits) != 0 {
		t.Fatalf("pending increase retained discount = %#v, %v", benefits, err)
	}
}

func TestCurrentPriceEditRekeysExplicitDiscountWithoutReplacingMetadata(t *testing.T) {
	store, accountID := newPriceBenefitTestStore(t)
	subscription := createPriceBenefitTestSubscription(t, store, accountID, "explicit-reprice", "explicit@example.com", 10000)
	subscription.CustomerWechat = "explicit-old-wechat"
	if err := store.UpdateSubscription(subscription); err != nil {
		t.Fatal(err)
	}
	subscription, err := store.GetSubscription(subscription.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored, original := createExplicitPendingPriceDiscount(t, store, subscription, 9000)
	stored.PricePerPersonCents = 11000
	if err := store.UpdateSubscription(stored); err != nil {
		t.Fatal(err)
	}
	benefits, err := store.ListCustomerBenefits()
	if err != nil || len(benefits) != 1 {
		t.Fatalf("benefits after current-price correction = %#v, %v", benefits, err)
	}
	updated := benefits[0]
	if updated.ID != original.ID || updated.PriceBeforeCents != 11000 || updated.PriceAfterCents != 9000 ||
		updated.PriceAdjustmentKey != priceAdjustmentKey(subscription.ID, updated.PriceEffectiveDueDate, 11000, 9000) ||
		updated.CurrentPriceCentsSnapshot != 11000 {
		t.Fatalf("updated structured adjustment = %#v", updated)
	}
	if updated.ActualCostCents != original.ActualCostCents ||
		updated.PerceivedValueCents != original.PerceivedValueCents ||
		updated.BenefitName != original.BenefitName || updated.Note != original.Note ||
		updated.RecommendationCode != original.RecommendationCode || updated.BatchID != original.BatchID ||
		updated.BenefitDate != original.BenefitDate || !updated.CreatedAt.Equal(original.CreatedAt) {
		t.Fatalf("explicit metadata changed: before=%#v after=%#v", original, updated)
	}
}

func TestCustomerIdentityEditUpdatesExplicitDiscountSnapshotsWithoutReplacingMetadata(t *testing.T) {
	store, accountID := newPriceBenefitTestStore(t)
	subscription := createPriceBenefitTestSubscription(t, store, accountID, "explicit-identity", "old@example.com", 10000)
	subscription.CustomerWechat = "old-wechat"
	if err := store.UpdateSubscription(subscription); err != nil {
		t.Fatal(err)
	}
	subscription, err := store.GetSubscription(subscription.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored, original := createExplicitPendingPriceDiscount(t, store, subscription, 9000)
	stored.CustomerEmail = "new@example.com"
	stored.CustomerWechat = "new-wechat"
	if err := store.UpdateSubscription(stored); err != nil {
		t.Fatal(err)
	}
	benefits, err := store.ListCustomerBenefits()
	if err != nil || len(benefits) != 1 {
		t.Fatalf("benefits after identity correction = %#v, %v", benefits, err)
	}
	updated := benefits[0]
	if updated.ID != original.ID || updated.CustomerEmailSnapshot != "new@example.com" ||
		updated.CustomerWechatSnapshot != "new-wechat" {
		t.Fatalf("updated identity snapshots = %#v", updated)
	}
	if updated.ActualCostCents != original.ActualCostCents ||
		updated.PerceivedValueCents != original.PerceivedValueCents || updated.Note != original.Note ||
		updated.RecommendationCode != original.RecommendationCode || updated.BatchID != original.BatchID ||
		updated.BenefitDate != original.BenefitDate || !updated.CreatedAt.Equal(original.CreatedAt) {
		t.Fatalf("explicit metadata changed: before=%#v after=%#v", original, updated)
	}
}

func TestPendingDiscountCorrectionIgnoresSiblingBenefitCooldown(t *testing.T) {
	store, accountID := newPriceBenefitTestStore(t)
	target := createPriceBenefitTestSubscription(t, store, accountID, "group-target", "group@example.com", 10000)
	sibling := createPriceBenefitTestSubscription(t, store, accountID, "group-sibling", "sibling@example.com", 10000)
	nextPrice := int64(9000)
	target.NextPriceCents = &nextPrice
	target.NextPriceEffectiveDueDate = "2026-10-31"
	if err := store.UpdateSubscription(target); err != nil {
		t.Fatal(err)
	}
	today := cycle.FormatDate(time.Now().In(cycle.Location))
	if err := store.CreateCustomerBenefits([]model.CustomerBenefit{{
		BatchID:                   "care-sibling-benefit",
		SubscriptionID:            sibling.ID,
		BenefitType:               model.CustomerBenefitTypeExtension,
		BenefitName:               "同组席位近期福利",
		BenefitDate:               today,
		CustomerEmailSnapshot:     sibling.CustomerEmail,
		CustomerGroupSizeSnapshot: 1,
		CurrentPriceCentsSnapshot: sibling.PricePerPersonCents,
	}}); err != nil {
		t.Fatal(err)
	}
	sibling, err := store.GetSubscription(sibling.ID)
	if err != nil {
		t.Fatal(err)
	}
	sibling.CustomerEmail = target.CustomerEmail
	if err := store.UpdateSubscription(sibling); err != nil {
		t.Fatal(err)
	}
	target, err = store.GetSubscription(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	target.PricePerPersonCents = 11000
	if err := store.UpdateSubscription(target); err != nil {
		t.Fatalf("existing pending discount correction was blocked by sibling cooldown: %v", err)
	}
	benefits, err := store.ListCustomerBenefits()
	if err != nil || len(benefits) != 2 {
		t.Fatalf("group benefits = %#v, %v", benefits, err)
	}
	for _, benefit := range benefits {
		if benefit.SubscriptionID == target.ID && benefit.PriceBeforeCents != 11000 {
			t.Fatalf("target discount was not rekeyed = %#v", benefit)
		}
	}
}

func TestNextPriceDiscountAllowsRecentCustomerBenefit(t *testing.T) {
	store, accountID := newPriceBenefitTestStore(t)
	subscription := createPriceBenefitTestSubscription(t, store, accountID, "cooldown", "cooldown@example.com", 10000)
	today := cycle.FormatDate(time.Now().In(cycle.Location))
	if err := store.CreateCustomerBenefits([]model.CustomerBenefit{{
		BatchID:                   "care-existing-cooldown",
		SubscriptionID:            subscription.ID,
		BenefitType:               model.CustomerBenefitTypeExtension,
		BenefitName:               "已有福利",
		BenefitDate:               today,
		CustomerEmailSnapshot:     subscription.CustomerEmail,
		CustomerGroupSizeSnapshot: 1,
		CurrentPriceCentsSnapshot: subscription.PricePerPersonCents,
	}}); err != nil {
		t.Fatal(err)
	}
	nextPrice := int64(9000)
	subscription.NextPriceCents = &nextPrice
	subscription.NextPriceEffectiveDueDate = "2026-10-01"
	if err := store.UpdateSubscriptionNextPrices([]model.Subscription{subscription}, today); err != nil {
		t.Fatalf("discount after recent extension error = %v", err)
	}
	stored, err := store.GetSubscription(subscription.ID)
	if err != nil || stored.NextPriceCents == nil || *stored.NextPriceCents != 9000 {
		t.Fatalf("discount after recent extension = %#v, %v", stored, err)
	}
	benefits, err := store.ListCustomerBenefits()
	if err != nil || len(benefits) != 2 {
		t.Fatalf("separate extension and discount benefits = %#v, %v", benefits, err)
	}
}

func TestNextPriceDiscountAllowsTransitiveCustomerBenefit(t *testing.T) {
	store, accountID := newPriceBenefitTestStore(t)
	first := createPriceBenefitTestSubscription(t, store, accountID, "bridge-a", "shared@example.com", 10000)
	bridge := createPriceBenefitTestSubscription(t, store, accountID, "bridge-b", "shared@example.com", 10000)
	target := createPriceBenefitTestSubscription(t, store, accountID, "bridge-c", "target@example.com", 10000)
	bridge.CustomerWechat = "shared-wechat"
	if err := store.UpdateSubscription(bridge); err != nil {
		t.Fatal(err)
	}
	target.CustomerWechat = "shared-wechat"
	if err := store.UpdateSubscription(target); err != nil {
		t.Fatal(err)
	}
	today := cycle.FormatDate(time.Now().In(cycle.Location))
	if err := store.CreateCustomerBenefits([]model.CustomerBenefit{{
		BatchID: "care-transitive-cooldown", SubscriptionID: first.ID,
		BenefitType: model.CustomerBenefitTypeExtension, BenefitName: "已有福利", BenefitDate: today,
		CustomerEmailSnapshot: first.CustomerEmail, CustomerGroupSizeSnapshot: 3,
		CurrentPriceCentsSnapshot: first.PricePerPersonCents,
	}}); err != nil {
		t.Fatal(err)
	}
	target, err := store.GetSubscription(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	nextPrice := int64(9000)
	target.NextPriceCents = &nextPrice
	target.NextPriceEffectiveDueDate = "2026-10-01"
	if err := store.UpdateSubscriptionNextPrices([]model.Subscription{target}, today); err != nil {
		t.Fatalf("discount blocked by another connected seat's benefit: %v", err)
	}
	benefits, err := store.ListCustomerBenefits()
	if err != nil || len(benefits) != 2 {
		t.Fatalf("benefits across connected seats = %#v, %v", benefits, err)
	}
}

func TestOpenBackfillsPendingAndAppliedDiscountBenefitsIdempotently(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "price-benefit-backfill.db")
	store, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	accountID, err := store.CreateAccount(model.Account{Name: "backfill-owner"}, 0, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	pending := createPriceBenefitTestSubscription(t, store, accountID, "pending", "pending@example.com", 10000)
	applied := createPriceBenefitTestSubscription(t, store, accountID, "applied", "applied@example.com", 9000)
	ambiguous := createPriceBenefitTestSubscription(t, store, accountID, "ambiguous", "ambiguous@example.com", 9000)
	if _, err := store.database.Exec(`
		UPDATE subscriptions
		SET next_price_cents = 9000,
		    next_price_effective_due_date = '2026-08-31',
		    updated_at = '2026-08-15T04:00:00Z'
		WHERE id = ?`, pending.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.database.Exec(`
		INSERT INTO subscription_price_changes (
			subscription_id, previous_price_cents, new_price_cents,
			effective_due_date, created_at
		) VALUES (?, 10000, 9000, '2026-08-31', '2026-08-15T16:30:00Z')`, applied.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.database.Exec(`
		INSERT INTO scheduled_price_discount_history (
			subscription_id, price_before_cents, price_after_cents,
			effective_due_date, scheduled_at
		) VALUES (?, 10000, 9000, '2026-08-31', '2026-08-15T04:00:00Z')`, applied.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.database.Exec(`
		INSERT INTO subscription_price_changes (
			subscription_id, previous_price_cents, new_price_cents,
			effective_due_date, created_at
		) VALUES (?, 10000, 9000, '2026-08-15', '2026-08-15T04:00:00Z')`, ambiguous.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	benefits, err := store.ListCustomerBenefits()
	if err != nil {
		t.Fatal(err)
	}
	if len(benefits) != 2 {
		t.Fatalf("backfilled benefits = %#v", benefits)
	}
	benefitDates := map[int64]string{}
	for _, benefit := range benefits {
		benefitDates[benefit.SubscriptionID] = benefit.BenefitDate
	}
	if benefitDates[pending.ID] != "2026-08-15" || benefitDates[applied.ID] != "2026-08-16" {
		t.Fatalf("backfilled benefit dates = %#v", benefitDates)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	benefits, err = store.ListCustomerBenefits()
	if err != nil || len(benefits) != 2 {
		t.Fatalf("benefits after repeated Open = %#v, %v", benefits, err)
	}
}

func TestStructuredDiscountsWithSameDisplayFieldsSurviveRestart(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "price-benefit-index-restart.db")
	store, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	accountID, err := store.CreateAccount(model.Account{Name: "restart-owner"}, 0, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	subscription := createPriceBenefitTestSubscription(t, store, accountID, "restart", "restart@example.com", 9000)
	for _, dueDate := range []string{"2026-08-31", "2026-09-30"} {
		if _, err := store.database.Exec(`
			INSERT INTO subscription_price_changes (
				subscription_id, previous_price_cents, new_price_cents,
				effective_due_date, created_at
			) VALUES (?, 10000, 9000, ?, '2026-08-15T04:00:00Z')`, subscription.ID, dueDate); err != nil {
			t.Fatal(err)
		}
		if _, err := store.database.Exec(`
			INSERT INTO scheduled_price_discount_history (
				subscription_id, price_before_cents, price_after_cents,
				effective_due_date, scheduled_at
			) VALUES (?, 10000, 9000, ?, '2026-08-15T04:00:00Z')`, subscription.ID, dueDate); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		store, err = Open(databasePath)
		if err != nil {
			t.Fatal(err)
		}
		benefits, listErr := store.ListCustomerBenefits()
		if listErr != nil || len(benefits) != 2 {
			t.Fatalf("restart benefits = %#v, %v", benefits, listErr)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExplicitHistoricalDiscountSyncIsVerifiedAndIdempotent(t *testing.T) {
	store, accountID := newPriceBenefitTestStore(t)
	subscription := createPriceBenefitTestSubscription(t, store, accountID, "confirmed", "confirmed@example.com", 9000)
	if _, err := store.database.Exec(`
		INSERT INTO subscription_price_changes (
			subscription_id, previous_price_cents, new_price_cents,
			effective_due_date, created_at
		) VALUES (?, 10000, 9000, '2026-08-31', '2026-08-15T04:00:00Z')`, subscription.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SyncHistoricalPriceDiscountBenefit(subscription.ID, 10000, 8000, "2026-08-31"); err == nil {
		t.Fatal("mismatched historical adjustment was accepted")
	}
	created, err := store.SyncHistoricalPriceDiscountBenefit(subscription.ID, 10000, 9000, "2026-08-31")
	if err != nil || !created {
		t.Fatalf("first historical sync = %v, %v", created, err)
	}
	created, err = store.SyncHistoricalPriceDiscountBenefit(subscription.ID, 10000, 9000, "2026-08-31")
	if err != nil || created {
		t.Fatalf("replayed historical sync = %v, %v", created, err)
	}
	benefits, err := store.ListCustomerBenefits()
	if err != nil || len(benefits) != 1 || benefits[0].PriceAdjustmentKey == "" {
		t.Fatalf("historical benefits = %#v, %v", benefits, err)
	}
}

func TestOpenClaimsLegacyPopupDiscountInsteadOfDuplicating(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy-popup-benefit.db")
	store, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	accountID, err := store.CreateAccount(model.Account{Name: "legacy-owner"}, 0, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	subscription := createPriceBenefitTestSubscription(t, store, accountID, "legacy", "legacy@example.com", 10000)
	if err := store.CreateCustomerBenefits([]model.CustomerBenefit{{
		BatchID:                   "benefit-operation-v1:legacy-popup-operation",
		SubscriptionID:            subscription.ID,
		BenefitType:               model.CustomerBenefitTypePriceDiscount,
		BenefitName:               priceDiscountBenefitName(10000, 9000),
		BenefitDate:               "2026-08-15",
		NextDueDateSnapshot:       "2026-08-31",
		CustomerEmailSnapshot:     subscription.CustomerEmail,
		CustomerGroupSizeSnapshot: 1,
		CurrentPriceCentsSnapshot: 10000,
		CreatedAt:                 time.Date(2026, time.August, 15, 4, 0, 0, 0, time.UTC),
	}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.database.Exec(`
		UPDATE subscriptions
		SET next_price_cents = 9000,
		    next_price_effective_due_date = '2026-08-31',
		    updated_at = '2026-08-15T04:00:00Z'
		WHERE id = ?`, subscription.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	benefits, err := store.ListCustomerBenefits()
	if err != nil || len(benefits) != 1 || benefits[0].PriceAdjustmentKey == "" ||
		benefits[0].BatchID != "benefit-operation-v1:legacy-popup-operation" {
		t.Fatalf("claimed legacy benefit = %#v, %v", benefits, err)
	}
}

func TestNewDiscountDoesNotClaimLegacyBenefitFromDifferentEffectiveDueDate(t *testing.T) {
	store, accountID := newPriceBenefitTestStore(t)
	subscription := createPriceBenefitTestSubscription(t, store, accountID, "legacy-other-due", "legacy-other@example.com", 10000)
	today := cycle.FormatDate(time.Now().In(cycle.Location))
	if err := store.CreateCustomerBenefits([]model.CustomerBenefit{{
		BatchID:                   "benefit-operation-v1:old-discount",
		SubscriptionID:            subscription.ID,
		BenefitType:               model.CustomerBenefitTypePriceDiscount,
		BenefitName:               priceDiscountBenefitName(10000, 9000),
		BenefitDate:               today,
		NextDueDateSnapshot:       "2026-10-01",
		CustomerEmailSnapshot:     subscription.CustomerEmail,
		CustomerGroupSizeSnapshot: 1,
		CurrentPriceCentsSnapshot: 10000,
	}}); err != nil {
		t.Fatal(err)
	}
	nextPrice := int64(9000)
	subscription.NextPriceCents = &nextPrice
	subscription.NextPriceEffectiveDueDate = "2026-10-31"
	if err := store.UpdateSubscriptionNextPrices([]model.Subscription{subscription}, today); err != nil {
		t.Fatalf("new discount with distinct effective date error = %v", err)
	}
	benefits, err := store.ListCustomerBenefits()
	if err != nil || len(benefits) != 2 {
		t.Fatalf("legacy benefits = %#v, %v", benefits, err)
	}
	for _, benefit := range benefits {
		if benefit.BatchID == "benefit-operation-v1:old-discount" {
			if benefit.PriceAdjustmentKey != "" || benefit.PriceEffectiveDueDate != "" ||
				benefit.NextDueDateSnapshot != "2026-10-01" {
				t.Fatalf("legacy benefit was mutated = %#v", benefit)
			}
		} else if benefit.PriceAdjustmentKey == "" || benefit.PriceEffectiveDueDate != "2026-10-31" {
			t.Fatalf("new discount is not independently linked = %#v", benefit)
		}
	}
	stored, err := store.GetSubscription(subscription.ID)
	if err != nil || stored.NextPriceCents == nil || *stored.NextPriceCents != 9000 {
		t.Fatalf("new discount was not scheduled = %#v, %v", stored, err)
	}
}
