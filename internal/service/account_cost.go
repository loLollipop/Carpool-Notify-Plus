package service

import (
	"errors"
	"fmt"
	"time"

	"carpool-notify/internal/cycle"
)

// ProcessAccountCostRenewals accrues all owner-account renewals due through today.
func (service *SubscriptionService) ProcessAccountCostRenewals() error {
	accounts, err := service.Store.ListAccounts()
	if err != nil {
		return err
	}
	today := cycle.StartOfDay(service.now())
	var renewalErrors []error
	for _, account := range accounts {
		if account.BannedAt != "" {
			continue
		}
		if account.OpenedAt == "" {
			continue
		}
		openedAt, parseErr := time.ParseInLocation("2006-01-02", account.OpenedAt, cycle.Location)
		if parseErr != nil {
			renewalErrors = append(renewalErrors, fmt.Errorf("account %d opened_at: %w", account.ID, parseErr))
			continue
		}
		latestPeriod, latestErr := service.Store.LatestAutomaticAccountCostPeriod(account.ID)
		if latestErr != nil {
			renewalErrors = append(renewalErrors, fmt.Errorf("account %d latest cost period: %w", account.ID, latestErr))
			continue
		}
		if latestPeriod == "" {
			continue
		}
		latestAt, parseErr := time.ParseInLocation("2006-01-02", latestPeriod, cycle.Location)
		if parseErr != nil {
			renewalErrors = append(renewalErrors, fmt.Errorf("account %d cost period: %w", account.ID, parseErr))
			continue
		}

		for renewalAt := nextMonthlyAnniversary(openedAt, latestAt); !renewalAt.After(today); renewalAt = nextMonthlyAnniversary(openedAt, renewalAt) {
			if _, accrueErr := service.Store.AccrueAccountRenewal(account.ID, cycle.FormatDate(renewalAt)); accrueErr != nil {
				renewalErrors = append(renewalErrors, fmt.Errorf("account %d renewal %s: %w", account.ID, cycle.FormatDate(renewalAt), accrueErr))
				break
			}
		}
	}
	return errors.Join(renewalErrors...)
}

func nextMonthlyAnniversary(openedAt time.Time, after time.Time) time.Time {
	openedAt = cycle.StartOfDay(openedAt)
	after = cycle.StartOfDay(after)
	year, month := after.Year(), after.Month()
	candidate := monthlyAnniversary(year, month, openedAt.Day())
	for !candidate.After(openedAt) || !candidate.After(after) {
		month++
		if month > time.December {
			month = time.January
			year++
		}
		candidate = monthlyAnniversary(year, month, openedAt.Day())
	}
	return candidate
}

func monthlyAnniversary(year int, month time.Month, openedDay int) time.Time {
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, cycle.Location).Day()
	if openedDay > lastDay {
		openedDay = lastDay
	}
	return time.Date(year, month, openedDay, 0, 0, 0, 0, cycle.Location)
}
