package service_test

import (
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"
)

func TestAccountCostRenewalsUseMonthlyAnniversaryAndConsumeZeroOnce(t *testing.T) {
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
	if account.TotalCostCents != 4000 {
		t.Fatalf("TotalCostCents = %d, want 4000", account.TotalCostCents)
	}
	if account.ZeroRenewalNextMonth {
		t.Fatal("zero renewal flag was not consumed")
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
	if records[2].PeriodDate != "2026-03-31" || records[2].AmountCents != 2000 || records[2].Source != model.AccountCostSourceRenewal {
		t.Fatalf("paid renewal record = %#v", records[2])
	}
}

func TestAccountTotalCostCanBeInitializedAndCorrected(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 10, 12, 0, 0, 0, cycle.Location)
	}
	initialTotal := "75.00"
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:          "owner@example.com",
		Email:         "owner@example.com",
		OpenedAt:      "2026-07-01",
		CostYuan:      "20.00",
		TotalCostYuan: &initialTotal,
		SeatCount:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	correctedTotal := "80.00"
	if err := subscriptionService.UpdateAccount(accountID, service.UpdateAccountInput{
		Name:          "owner@example.com",
		Email:         "owner@example.com",
		OpenedAt:      "2026-07-01",
		CostYuan:      "20.00",
		TotalCostYuan: &correctedTotal,
	}); err != nil {
		t.Fatal(err)
	}

	account, err := subscriptionService.Store.GetAccount(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if account.CostCents != 2000 || account.TotalCostCents != 8000 {
		t.Fatalf("monthly/total cost = %d/%d, want 2000/8000", account.CostCents, account.TotalCostCents)
	}
	records, err := subscriptionService.Store.ListAccountCostRecords(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].PeriodDate != "2026-07-10" ||
		records[1].Source != model.AccountCostSourceManual || records[1].AmountCents != 500 {
		t.Fatalf("manual correction records = %#v", records)
	}
}

func TestHistoricalAccountTotalCostStaysInImportPeriod(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.August, 7, 12, 0, 0, 0, cycle.Location)
	}
	historicalTotal := "75.00"
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:          "historical-owner@example.com",
		Email:         "historical-owner@example.com",
		OpenedAt:      "2026-01-18",
		CostYuan:      "20.00",
		TotalCostYuan: &historicalTotal,
		SeatCount:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	records, err := subscriptionService.Store.ListAccountCostRecords(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].PeriodDate != "2026-08-07" || records[0].AmountCents != 7500 {
		t.Fatalf("historical account cost records = %#v", records)
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
	if len(records) != 1 || records[0].PeriodDate != "2026-07-02" || records[0].AmountCents != 2000 {
		t.Fatalf("opening-date and monthly-cost edit records = %#v", records)
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
