package service

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
)

type DeferRenewalInput struct {
	ExpectedDueDate   string `json:"expected_due_date"`
	ExpectedUpdatedAt string `json:"expected_updated_at"`
}

type CompleteDeferredInput struct {
	SubscriptionID    int64  `json:"subscription_id"`
	ExpectedUpdatedAt string `json:"expected_updated_at"`
}

func (service *SubscriptionService) SetAccountPaused(accountID int64, paused bool) error {
	return service.Store.SetAccountPaused(accountID, paused)
}

func (service *SubscriptionService) DeferSubscriptionRenewal(subscriptionID int64, input DeferRenewalInput) error {
	version, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(input.ExpectedUpdatedAt))
	if err != nil {
		return fmt.Errorf("缺少有效的客户版本，请刷新后重试")
	}
	dueDate := strings.TrimSpace(input.ExpectedDueDate)
	if _, err := time.ParseInLocation("2006-01-02", dueDate, cycle.Location); err != nil {
		return fmt.Errorf("到期日期无效")
	}
	service.dueNotificationMu.Lock()
	defer service.dueNotificationMu.Unlock()
	return renewalDeferralError(service.Store.DeferSubscriptionRenewal(db.RenewalDeferralExpectation{
		SubscriptionID: subscriptionID, UpdatedAt: version, DueDate: dueDate,
	}, service.now()))
}

func (service *SubscriptionService) RestoreSubscriptionRenewal(subscriptionID int64) error {
	service.dueNotificationMu.Lock()
	defer service.dueNotificationMu.Unlock()
	return renewalDeferralError(service.Store.RestoreSubscriptionRenewal(subscriptionID))
}

func (service *SubscriptionService) CompleteDeferredSubscriptions(input []CompleteDeferredInput) (int, error) {
	if len(input) == 0 || len(input) > 1000 {
		return 0, fmt.Errorf("请选择 1 至 1000 位已手动移出的客户")
	}
	expected := make([]db.RenewalDeferralExpectation, 0, len(input))
	for _, item := range input {
		version, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(item.ExpectedUpdatedAt))
		if item.SubscriptionID <= 0 || err != nil {
			return 0, fmt.Errorf("缺少有效的客户版本，请刷新后重试")
		}
		expected = append(expected, db.RenewalDeferralExpectation{SubscriptionID: item.SubscriptionID, UpdatedAt: version})
	}
	service.dueNotificationMu.Lock()
	defer service.dueNotificationMu.Unlock()
	freezeDays, err := service.GetSeatFreezeDays()
	if err != nil {
		return 0, err
	}
	if err := service.Store.CompleteDeferredSubscriptions(expected, service.now(), freezeDays); err != nil {
		return 0, renewalDeferralError(err)
	}
	return len(expected), nil
}

func renewalDeferralError(err error) error {
	if errors.Is(err, db.ErrSubscriptionStateChanged) || errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("客户状态、到期日或缴费记录已变化，请刷新后重新选择到期的 Team 客户: %w", db.ErrSubscriptionStateChanged)
	}
	return err
}
