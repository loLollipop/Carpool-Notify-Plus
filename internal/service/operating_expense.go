package service

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
)

const operatingExpenseRunRateDays = 90

type OperatingExpenseView struct {
	ID             int64  `json:"id"`
	Category       string `json:"category"`
	OccurredOn     string `json:"occurred_on"`
	AmountCents    int64  `json:"amount_cents"`
	AmountYuan     string `json:"amount_yuan"`
	Note           string `json:"note"`
	CreatedAtLabel string `json:"created_at_label"`
}

type OperatingExpenseInput struct {
	OccurredOn string
	AmountYuan string
	Note       string
}

func (service *SubscriptionService) CreateOperatingExpense(input OperatingExpenseInput) (int64, error) {
	expense, err := service.normalizeOperatingExpense(0, input)
	if err != nil {
		return 0, err
	}
	return service.Store.CreateOperatingExpense(expense)
}

func (service *SubscriptionService) UpdateOperatingExpense(
	expenseID int64,
	input OperatingExpenseInput,
) error {
	if expenseID <= 0 {
		return errors.New("无效的推流支出 ID")
	}
	if _, err := service.Store.GetOperatingExpense(expenseID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("推流支出不存在")
		}
		return err
	}
	expense, err := service.normalizeOperatingExpense(expenseID, input)
	if err != nil {
		return err
	}
	return service.Store.UpdateOperatingExpense(expense)
}

func (service *SubscriptionService) DeleteOperatingExpense(expenseID int64) error {
	if expenseID <= 0 {
		return errors.New("无效的推流支出 ID")
	}
	if err := service.Store.DeleteOperatingExpense(expenseID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("推流支出不存在")
		}
		return err
	}
	return nil
}

func (service *SubscriptionService) normalizeOperatingExpense(
	expenseID int64,
	input OperatingExpenseInput,
) (model.OperatingExpense, error) {
	occurredOn := strings.TrimSpace(input.OccurredOn)
	if occurredOn == "" {
		occurredOn = cycle.FormatDate(service.now())
	}
	date, err := time.ParseInLocation("2006-01-02", occurredOn, cycle.Location)
	if err != nil || date.Format("2006-01-02") != occurredOn {
		return model.OperatingExpense{}, errors.New("推流日期无效")
	}
	today := service.now().In(cycle.Location)
	todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, cycle.Location)
	if date.After(todayStart) {
		return model.OperatingExpense{}, errors.New("推流日期不能晚于今天")
	}
	amountCents, err := cycle.ParseYuanToCents(strings.TrimSpace(input.AmountYuan))
	if err != nil || amountCents <= 0 {
		return model.OperatingExpense{}, errors.New("推流金额必须大于 0，且最多保留两位小数")
	}
	return model.OperatingExpense{
		ID:          expenseID,
		Category:    model.OperatingExpenseCategoryXianyuPromotion,
		OccurredOn:  occurredOn,
		AmountCents: amountCents,
		Note:        strings.TrimSpace(input.Note),
	}, nil
}

func buildOperatingExpenseView(expense model.OperatingExpense) OperatingExpenseView {
	return OperatingExpenseView{
		ID:             expense.ID,
		Category:       expense.Category,
		OccurredOn:     expense.OccurredOn,
		AmountCents:    expense.AmountCents,
		AmountYuan:     cycle.FormatCents(expense.AmountCents),
		Note:           expense.Note,
		CreatedAtLabel: expense.CreatedAt.In(cycle.Location).Format("2006-01-02 15:04"),
	}
}

func operatingExpenseTotal(expenses []model.OperatingExpense) int64 {
	var total int64
	for _, expense := range expenses {
		total += expense.AmountCents
	}
	return total
}

// operatingExpenseMonthlyRunRate uses the last 90 calendar days as a
// conservative monthly acquisition-spend estimate. One-off historical entries
// remain in actual profit but naturally age out of future forecasts.
func operatingExpenseMonthlyRunRate(expenses []model.OperatingExpense, now time.Time) int64 {
	today := now.In(cycle.Location)
	end := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, cycle.Location)
	start := end.AddDate(0, 0, -(operatingExpenseRunRateDays - 1))
	var total int64
	var earliest time.Time
	for _, expense := range expenses {
		date, err := time.ParseInLocation("2006-01-02", expense.OccurredOn, cycle.Location)
		if err != nil || date.Before(start) || date.After(end) {
			continue
		}
		total += expense.AmountCents
		if earliest.IsZero() || date.Before(earliest) {
			earliest = date
		}
	}
	if total == 0 || earliest.IsZero() {
		return 0
	}
	observedDays := int(end.Sub(earliest).Hours()/24) + 1
	// New ledgers have no reliable zero-spend history before their first entry.
	// Use at least one month so a single day's spend is not multiplied by 30,
	// while avoiding the opposite error of diluting it across 90 unknown days.
	if observedDays < 30 {
		observedDays = 30
	}
	return total * 30 / int64(observedDays)
}
