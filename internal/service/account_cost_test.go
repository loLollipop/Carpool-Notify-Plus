package service_test

import (
	"errors"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"
)

func TestAccountRenewalsRejectOpeningDateEditAfterReadingSchedule(t *testing.T) {
	for _, path := range []string{"scheduler", "manual"} {
		for _, zero := range []bool{false, true} {
			t.Run(path+map[bool]string{false: "/paid", true: "/zero"}[zero], func(t *testing.T) {
				subscriptionService := openTestService(t)
				now := time.Date(2026, time.September, 2, 12, 0, 0, 0, cycle.Location)
				subscriptionService.Clock = func() time.Time { return now }
				accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
					Name: "owner@example.com", OpenedAt: "2026-08-01", CostYuan: "20.00",
					ZeroRenewalNextMonth: zero, SeatCount: 1,
				})
				if err != nil {
					t.Fatal(err)
				}
				oldView, err := subscriptionService.GetAccountView(accountID)
				if err != nil || oldView.NextRenewalDate != "2026-09-01" {
					t.Fatalf("original schedule = %#v, %v", oldView, err)
				}
				// Both renewal paths read the account before reading the clock.
				// Commit the edit at this boundary to deterministically exercise the
				// reverse ordering without timing-dependent goroutines or DB mocks.
				edited := false
				subscriptionService.Clock = func() time.Time {
					if !edited {
						edited = true
						if err := subscriptionService.UpdateAccount(accountID, service.UpdateAccountInput{
							Name: "owner@example.com", OpenedAt: "2026-08-02", CostYuan: "25.00",
							ZeroRenewalNextMonth: zero, SeatCount: 1,
						}); err != nil {
							t.Fatal(err)
						}
					}
					return now
				}
				if path == "scheduler" {
					err = subscriptionService.ProcessAccountCostRenewals()
				} else {
					var inserted bool
					inserted, err = subscriptionService.MarkAccountRenewed(accountID, oldView.NextRenewalDate)
					if inserted {
						t.Fatal("stale manual renewal was inserted")
					}
				}
				if !edited || !errors.Is(err, db.ErrAccountRenewalStateChanged) {
					t.Fatalf("edited = %v, stale renewal error = %v", edited, err)
				}
				records, err := subscriptionService.Store.ListAccountCostRecords(accountID)
				if err != nil || len(records) != 1 || records[0].PeriodDate != "2026-08-02" || records[0].AmountCents != 2500 {
					t.Fatalf("stale renewal affected ledger: %#v, %v", records, err)
				}
				freshView, err := subscriptionService.GetAccountView(accountID)
				if err != nil || freshView.NextRenewalDate != "2026-09-02" {
					t.Fatalf("fresh schedule = %#v, %v", freshView, err)
				}
				// Recompute, then repeat: only one renewal in this month is allowed.
				for attempt := 0; attempt < 2; attempt++ {
					if path == "scheduler" {
						err = subscriptionService.ProcessAccountCostRenewals()
					} else {
						var inserted bool
						inserted, err = subscriptionService.MarkAccountRenewed(accountID, freshView.NextRenewalDate)
						if inserted != (attempt == 0) {
							t.Fatalf("attempt %d inserted = %v", attempt, inserted)
						}
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				records, err = subscriptionService.Store.ListAccountCostRecords(accountID)
				if err != nil || len(records) != 2 || records[1].PeriodDate != "2026-09-02" {
					t.Fatalf("recomputed ledger = %#v, %v", records, err)
				}
				wantAmount, wantSource := int64(2500), model.AccountCostSourceRenewal
				if zero {
					wantAmount, wantSource = 0, model.AccountCostSourceZeroRenewal
				}
				if records[1].AmountCents != wantAmount || records[1].Source != wantSource {
					t.Fatalf("recomputed renewal = %#v", records[1])
				}
			})
		}
	}
}

func TestAccountCostRenewalsUseMonthlyAnniversaryAndKeepRecurringZero(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.January, 31, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }

	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:                 "owner@example.com",
		Email:                "owner@example.com",
		OpenedAt:             "2026-01-31",
		CostYuan:             "20.00",
		ZeroRenewalNextMonth: true,
		SeatCount:            1,
	})
	if err != nil {
		t.Fatal(err)
	}

	now = time.Date(2026, time.February, 22, 12, 0, 0, 0, cycle.Location)
	view, err := subscriptionService.GetAccountView(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if view.NextRenewalDate != "2026-02-28" || view.RenewalThisMonth || view.RenewalActionable {
		t.Fatalf("zero renewal manual state = %#v, want visible date without manual work", view)
	}

	now = time.Date(2026, time.March, 31, 12, 0, 0, 0, cycle.Location)
	if err := subscriptionService.ProcessAccountCostRenewals(); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.ProcessAccountCostRenewals(); err != nil {
		t.Fatal(err)
	}

	account, err := subscriptionService.Store.GetAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if account.TotalCostCents != 2000 {
		t.Fatalf("TotalCostCents = %d, want opening cost 2000", account.TotalCostCents)
	}
	if !account.ZeroRenewalNextMonth {
		t.Fatal("recurring zero-renewal setting was unexpectedly disabled")
	}
	records, err := subscriptionService.Store.ListAccountCostRecords(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("record count = %d, want 3", len(records))
	}
	if records[1].PeriodDate != "2026-02-28" || records[1].AmountCents != 0 || records[1].Source != model.AccountCostSourceZeroRenewal {
		t.Fatalf("zero renewal record = %#v", records[1])
	}
	if records[2].PeriodDate != "2026-03-31" || records[2].AmountCents != 0 || records[2].Source != model.AccountCostSourceZeroRenewal {
		t.Fatalf("second zero renewal record = %#v", records[2])
	}

	if err := subscriptionService.UpdateAccount(accountID, service.UpdateAccountInput{
		Name:                 "owner@example.com",
		Email:                "owner@example.com",
		OpenedAt:             "2026-01-31",
		CostYuan:             "20.00",
		ZeroRenewalNextMonth: false,
		SeatCount:            1,
	}); err != nil {
		t.Fatal(err)
	}
	now = time.Date(2026, time.April, 30, 12, 0, 0, 0, cycle.Location)
	if err := subscriptionService.ProcessAccountCostRenewals(); err != nil {
		t.Fatal(err)
	}
	account, err = subscriptionService.Store.GetAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if account.TotalCostCents != 4000 || account.ZeroRenewalNextMonth {
		t.Fatalf("resumed paid renewal state = cost %d, zero %v, want 4000/false", account.TotalCostCents, account.ZeroRenewalNextMonth)
	}
	records, err = subscriptionService.Store.ListAccountCostRecords(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 || records[3].PeriodDate != "2026-04-30" || records[3].AmountCents != 2000 || records[3].Source != model.AccountCostSourceRenewal {
		t.Fatalf("resumed paid renewal record = %#v", records)
	}
}

func TestMarkAccountRenewedUpdatesPendingStateAndIsIdempotent(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.August, 22, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "manual-renewal@example.com",
		Email:     "manual-renewal@example.com",
		OpenedAt:  "2026-07-25",
		CostYuan:  "75.00",
		SeatCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	now = time.Date(2026, time.August, 21, 12, 0, 0, 0, cycle.Location)
	view, err := subscriptionService.GetAccountView(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if !view.RenewalThisMonth || view.RenewalActionable {
		t.Fatalf("renewal state four days early = %#v, want visible but not actionable", view)
	}
	if _, err := subscriptionService.MarkAccountRenewed(accountID, "2026-08-25"); err == nil {
		t.Fatal("renewal was accepted four days early")
	}

	now = time.Date(2026, time.August, 22, 12, 0, 0, 0, cycle.Location)
	view, err = subscriptionService.GetAccountView(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if view.NextRenewalDate != "2026-08-25" || !view.RenewalThisMonth || !view.RenewalActionable {
		t.Fatalf("renewal state before marking = %#v", view)
	}

	inserted, err := subscriptionService.MarkAccountRenewed(accountID, "2026-08-25")
	if err != nil || !inserted {
		t.Fatalf("first mark = %v, %v", inserted, err)
	}
	inserted, err = subscriptionService.MarkAccountRenewed(accountID, "2026-08-25")
	if err != nil || inserted {
		t.Fatalf("repeated mark = %v, %v, want idempotent success", inserted, err)
	}

	account, err := subscriptionService.Store.GetAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if account.TotalCostCents != 15000 {
		t.Fatalf("total cost after repeated mark = %d, want 15000", account.TotalCostCents)
	}
	view, err = subscriptionService.GetAccountView(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if view.NextRenewalDate != "2026-09-25" || view.RenewalThisMonth || view.RenewalActionable {
		t.Fatalf("renewal state after marking = %#v", view)
	}
}

func TestAccountTotalCostIsDerivedFromOpeningAndRenewalCosts(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.January, 15, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "derived-cost@example.com",
		Email:     "derived-cost@example.com",
		OpenedAt:  "2026-01-15",
		CostYuan:  "75.00",
		SeatCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	account, err := subscriptionService.Store.GetAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if account.CostCents != 7500 || account.TotalCostCents != 7500 {
		t.Fatalf("opening monthly/total cost = %d/%d, want 7500/7500", account.CostCents, account.TotalCostCents)
	}

	if err := subscriptionService.UpdateAccount(accountID, service.UpdateAccountInput{
		Name:      "derived-cost@example.com",
		Email:     "derived-cost@example.com",
		OpenedAt:  "2026-01-15",
		CostYuan:  "110.00",
		SeatCount: 1,
	}); err != nil {
		t.Fatal(err)
	}
	account, err = subscriptionService.Store.GetAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if account.CostCents != 11000 || account.TotalCostCents != 11000 {
		t.Fatalf("edited monthly/total cost = %d/%d, want 11000/11000", account.CostCents, account.TotalCostCents)
	}

	now = time.Date(2026, time.February, 15, 12, 0, 0, 0, cycle.Location)
	if err := subscriptionService.ProcessAccountCostRenewals(); err != nil {
		t.Fatal(err)
	}
	account, err = subscriptionService.Store.GetAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if account.CostCents != 11000 || account.TotalCostCents != 22000 {
		t.Fatalf("renewed monthly/total cost = %d/%d, want 11000/22000", account.CostCents, account.TotalCostCents)
	}
	records, err := subscriptionService.Store.ListAccountCostRecords(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].PeriodDate != "2026-01-15" ||
		records[0].AmountCents != 11000 || records[0].Source != model.AccountCostSourceInitial ||
		records[1].PeriodDate != "2026-02-15" || records[1].AmountCents != 11000 ||
		records[1].Source != model.AccountCostSourceRenewal {
		t.Fatalf("derived cost records = %#v", records)
	}
}

func TestAccountOpeningDateAndMonthlyCostCanChangeBeforeRenewal(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 10, 12, 0, 0, 0, cycle.Location)
	}
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "owner-before-renewal@example.com",
		Email:     "owner-before-renewal@example.com",
		OpenedAt:  "2026-07-01",
		CostYuan:  "20.00",
		SeatCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.UpdateAccount(accountID, service.UpdateAccountInput{
		Name:      "owner-before-renewal@example.com",
		Email:     "owner-before-renewal@example.com",
		OpenedAt:  "2026-07-02",
		CostYuan:  "25.00",
		SeatCount: 1,
	}); err != nil {
		t.Fatal(err)
	}
	records, err := subscriptionService.Store.ListAccountCostRecords(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].PeriodDate != "2026-07-02" || records[0].AmountCents != 2500 {
		t.Fatalf("opening-date and monthly-cost edit records = %#v", records)
	}
}

func TestAccountMonthlyCostChangeAfterRenewalKeepsHistoricalCosts(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.January, 15, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "renewed-cost-change@example.com",
		Email:     "renewed-cost-change@example.com",
		OpenedAt:  "2026-01-15",
		CostYuan:  "20.00",
		SeatCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	now = time.Date(2026, time.February, 15, 12, 0, 0, 0, cycle.Location)
	if err := subscriptionService.ProcessAccountCostRenewals(); err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.UpdateAccount(accountID, service.UpdateAccountInput{
		Name:      "renewed-cost-change@example.com",
		Email:     "renewed-cost-change@example.com",
		OpenedAt:  "2026-01-15",
		CostYuan:  "25.00",
		SeatCount: 1,
	}); err != nil {
		t.Fatal(err)
	}
	account, err := subscriptionService.Store.GetAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if account.CostCents != 2500 || account.TotalCostCents != 4000 {
		t.Fatalf("edited renewed monthly/total cost = %d/%d, want 2500/4000", account.CostCents, account.TotalCostCents)
	}

	now = time.Date(2026, time.March, 15, 12, 0, 0, 0, cycle.Location)
	if err := subscriptionService.ProcessAccountCostRenewals(); err != nil {
		t.Fatal(err)
	}
	account, err = subscriptionService.Store.GetAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if account.CostCents != 2500 || account.TotalCostCents != 6500 {
		t.Fatalf("next renewal monthly/total cost = %d/%d, want 2500/6500", account.CostCents, account.TotalCostCents)
	}
	records, err := subscriptionService.Store.ListAccountCostRecords(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || records[0].AmountCents != 2000 ||
		records[1].AmountCents != 2000 || records[2].AmountCents != 2500 {
		t.Fatalf("cost records after monthly cost change = %#v", records)
	}
}

func TestAccountOpeningDateCannotChangeAfterRenewalCostsExist(t *testing.T) {
	subscriptionService := openTestService(t)
	now := time.Date(2026, time.January, 31, 12, 0, 0, 0, cycle.Location)
	subscriptionService.Clock = func() time.Time { return now }
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "renewed-owner@example.com",
		Email:     "renewed-owner@example.com",
		OpenedAt:  "2026-01-31",
		CostYuan:  "20.00",
		SeatCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	now = time.Date(2026, time.February, 28, 12, 0, 0, 0, cycle.Location)
	if err := subscriptionService.ProcessAccountCostRenewals(); err != nil {
		t.Fatal(err)
	}
	err = subscriptionService.UpdateAccount(accountID, service.UpdateAccountInput{
		Name:      "renewed-owner@example.com",
		Email:     "renewed-owner@example.com",
		OpenedAt:  "2026-01-30",
		CostYuan:  "20.00",
		SeatCount: 1,
	})
	if err == nil {
		t.Fatal("opening date change after renewal unexpectedly succeeded")
	}
	account, getErr := subscriptionService.Store.GetAccount(accountID)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if account.OpenedAt != "2026-01-31" || account.TotalCostCents != 4000 {
		t.Fatalf("account changed after rejected opening-date edit = %#v", account)
	}
}
