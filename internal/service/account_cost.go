package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

const accountRenewalNoticeDays = 3

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
		renewalAt, renewalErr := service.nextAccountCostRenewal(account)
		if renewalErr != nil {
			renewalErrors = append(renewalErrors, fmt.Errorf("account %d renewal period: %w", account.ID, renewalErr))
			continue
		}
		if renewalAt.IsZero() {
			continue
		}

		openedAt, _ := time.ParseInLocation("2006-01-02", account.OpenedAt, cycle.Location)
		for !renewalAt.After(today) {
			if _, accrueErr := service.Store.AccrueAccountRenewal(account.ID, cycle.FormatDate(renewalAt)); accrueErr != nil {
				renewalErrors = append(renewalErrors, fmt.Errorf("account %d renewal %s: %w", account.ID, cycle.FormatDate(renewalAt), accrueErr))
				break
			}
			renewalAt = nextMonthlyAnniversary(openedAt, renewalAt)
		}
	}
	return errors.Join(renewalErrors...)
}

// MarkAccountRenewed records one operator-confirmed owner-account renewal.
// The period is validated against the account ledger so stale or repeated
// clicks remain idempotent and can never skip an earlier renewal.
func (service *SubscriptionService) MarkAccountRenewed(accountID int64, periodDate string) (bool, error) {
	account, err := service.Store.GetAccount(accountID)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(account.BannedAt) != "" {
		return false, fmt.Errorf("已封禁账号不能登记续费")
	}
	periodAt, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(periodDate), cycle.Location)
	if err != nil {
		return false, fmt.Errorf("续费日期无效")
	}
	hasPeriod, err := service.Store.HasAutomaticAccountCostPeriod(accountID, cycle.FormatDate(periodAt))
	if err != nil {
		return false, err
	}
	if hasPeriod {
		return false, nil
	}

	nextAt, err := service.nextAccountCostRenewal(account)
	if err != nil {
		return false, err
	}
	if nextAt.IsZero() || !periodAt.Equal(nextAt) {
		return false, fmt.Errorf("续费状态已变化，请刷新后重试")
	}
	today := cycle.StartOfDay(service.now())
	if periodAt.After(today.AddDate(0, 0, accountRenewalNoticeDays)) {
		return false, fmt.Errorf("只能在到期前 %d 天内或逾期后登记续费", accountRenewalNoticeDays)
	}
	return service.Store.AccrueAccountRenewal(accountID, cycle.FormatDate(periodAt))
}

func (service *SubscriptionService) nextAccountCostRenewal(account model.Account) (time.Time, error) {
	openedAtText := strings.TrimSpace(account.OpenedAt)
	if openedAtText == "" {
		return time.Time{}, nil
	}
	openedAt, err := time.ParseInLocation("2006-01-02", openedAtText, cycle.Location)
	if err != nil {
		return time.Time{}, fmt.Errorf("opened_at: %w", err)
	}
	latestPeriod, err := service.Store.LatestAutomaticAccountCostPeriod(account.ID)
	if err != nil {
		return time.Time{}, fmt.Errorf("latest cost period: %w", err)
	}
	if latestPeriod == "" {
		return time.Time{}, nil
	}
	latestAt, err := time.ParseInLocation("2006-01-02", latestPeriod, cycle.Location)
	if err != nil {
		return time.Time{}, fmt.Errorf("cost period: %w", err)
	}
	return nextMonthlyAnniversary(openedAt, latestAt), nil
}

func accountRenewalActionable(renewalAt time.Time, now time.Time) bool {
	today := cycle.StartOfDay(now)
	return !renewalAt.After(today.AddDate(0, 0, accountRenewalNoticeDays))
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
