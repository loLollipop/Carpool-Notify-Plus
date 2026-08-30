package service

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

const afterSalesWarrantyDays = 30
const cancellationGracePeriod = 24 * time.Hour
const processedAfterSalesRetention = 24 * time.Hour

type BanAccountInput struct {
	BannedDate string
	Note       string
}

type AfterSalesCaseView struct {
	Case             model.AfterSalesCase `json:"case"`
	PaidAmountYuan   string               `json:"paid_amount_yuan"`
	RefundAmountYuan string               `json:"refund_amount_yuan"`
	StatusLabel      string               `json:"status_label"`
	ProcessedAtLabel string               `json:"processed_at_label"`
	ExpiresAtLabel   string               `json:"expires_at_label"`
}

type CancellationRequestResult struct {
	CaseID         int64  `json:"case_id"`
	ExpiresAt      string `json:"expires_at"`
	ExpiresAtLabel string `json:"expires_at_label"`
	Archived       bool   `json:"archived"`
}

type AfterSalesSummary struct {
	TotalCount          int    `json:"total_count"`
	PendingCount        int    `json:"pending_count"`
	ReviewCount         int    `json:"review_count"`
	RefundedCount       int    `json:"refunded_count"`
	ReassignedCount     int    `json:"reassigned_count"`
	PendingRefundCents  int64  `json:"pending_refund_cents"`
	PendingRefundYuan   string `json:"pending_refund_yuan"`
	RefundedAmountCents int64  `json:"refunded_amount_cents"`
	RefundedAmountYuan  string `json:"refunded_amount_yuan"`
}

type AfterSalesPage struct {
	Cases        []AfterSalesCaseView `json:"cases"`
	SummaryCases []AfterSalesCaseView `json:"summary_cases"`
	Summary      AfterSalesSummary    `json:"summary"`
}

type UpdateAfterSalesCaseInput struct {
	RefundAmountYuan string
	Note             string
}

type ReassignAfterSalesCaseInput struct {
	AccountID int64
	SeatID    int64
}

func (service *SubscriptionService) BanAccount(accountID int64, input BanAccountInput) (int, error) {
	if _, err := service.Store.GetAccount(accountID); err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("账号不存在")
		}
		return 0, err
	}
	bannedDate := strings.TrimSpace(input.BannedDate)
	if bannedDate == "" {
		bannedDate = cycle.FormatDate(service.now())
	}
	bannedDay, err := time.ParseInLocation("2006-01-02", bannedDate, cycle.Location)
	if err != nil {
		return 0, fmt.Errorf("封禁日期无效，请使用 YYYY-MM-DD")
	}
	today, _ := time.ParseInLocation("2006-01-02", cycle.FormatDate(service.now()), cycle.Location)
	if bannedDay.After(today) {
		return 0, fmt.Errorf("封禁日期不能晚于今天")
	}
	return service.Store.BanAccountAndCreateAfterSalesCases(
		accountID,
		bannedDate,
		strings.TrimSpace(input.Note),
		afterSalesWarrantyDays,
	)
}

func (service *SubscriptionService) RequestCancellation(subscriptionID int64) (CancellationRequestResult, error) {
	if _, err := service.Store.RestoreExpiredCancellationRequests(service.now()); err != nil {
		return CancellationRequestResult{}, err
	}
	requestedAt := service.now()
	subscription, err := service.Store.GetSubscription(subscriptionID)
	if err != nil {
		if err == sql.ErrNoRows {
			return CancellationRequestResult{}, fmt.Errorf("订阅不存在或已经退订")
		}
		return CancellationRequestResult{}, err
	}
	view, err := service.buildView(subscription, requestedAt, "")
	if err != nil {
		return CancellationRequestResult{}, err
	}
	// The first unpaid due date is the end of the customer's paid service.
	// Ending on or after that date is natural expiry, not an after-sales event.
	// If that due date has already been paid, buildView advances to the next
	// unpaid cycle and an immediate cancellation still follows after-sales.
	if view.DaysRemaining <= 0 {
		if err := service.Archive(subscriptionID); err != nil {
			return CancellationRequestResult{}, err
		}
		return CancellationRequestResult{Archived: true}, nil
	}
	expiresAt := requestedAt.Add(cancellationGracePeriod)
	caseItem, err := service.Store.RequestSubscriptionCancellation(
		subscriptionID,
		requestedAt,
		expiresAt,
		afterSalesWarrantyDays,
	)
	if err != nil {
		switch {
		case errors.Is(err, db.ErrCancellationPending):
			return CancellationRequestResult{}, fmt.Errorf("该订阅已经在等待售后处理")
		case errors.Is(err, db.ErrCancellationCaseConflict):
			return CancellationRequestResult{}, fmt.Errorf("该客户今天已有售后记录，请先到售后处理中核对")
		case errors.Is(err, sql.ErrNoRows):
			return CancellationRequestResult{}, fmt.Errorf("订阅不存在或已经退订")
		}
		return CancellationRequestResult{}, err
	}
	return CancellationRequestResult{
		CaseID:         caseItem.ID,
		ExpiresAt:      expiresAt.UTC().Format(time.RFC3339),
		ExpiresAtLabel: expiresAt.In(cycle.Location).Format("2006-01-02 15:04"),
	}, nil
}

func (service *SubscriptionService) RestoreExpiredCancellationRequests() (int, error) {
	return service.Store.RestoreExpiredCancellationRequests(service.now())
}

func (service *SubscriptionService) ListAfterSalesPage() (AfterSalesPage, error) {
	if _, err := service.Store.RestoreExpiredCancellationRequests(service.now()); err != nil {
		return AfterSalesPage{}, err
	}
	cases, err := service.Store.ListVisibleAfterSalesCases(
		service.now().Add(-processedAfterSalesRetention),
	)
	if err != nil {
		return AfterSalesPage{}, err
	}
	allCases, err := service.Store.ListAfterSalesCases()
	if err != nil {
		return AfterSalesPage{}, err
	}
	page := AfterSalesPage{
		Cases:        make([]AfterSalesCaseView, 0, len(cases)),
		SummaryCases: make([]AfterSalesCaseView, 0, len(allCases)),
	}
	for _, caseItem := range cases {
		page.Cases = append(page.Cases, buildAfterSalesCaseView(caseItem))
	}
	// Completed rows leave the working list after 24 hours, but the KPI cards
	// are historical operational/financial totals and must not silently reset.
	for _, caseItem := range allCases {
		page.SummaryCases = append(page.SummaryCases, buildAfterSalesCaseView(caseItem))
		page.Summary.TotalCount++
		switch caseItem.Status {
		case model.AfterSalesStatusRefunded:
			page.Summary.RefundedCount++
			page.Summary.RefundedAmountCents += caseItem.RefundAmountCents
		case model.AfterSalesStatusReassigned:
			page.Summary.ReassignedCount++
		case model.AfterSalesStatusReview:
			page.Summary.ReviewCount++
			page.Summary.PendingRefundCents += caseItem.RefundAmountCents
		default:
			page.Summary.PendingCount++
			page.Summary.PendingRefundCents += caseItem.RefundAmountCents
		}
	}
	page.Summary.PendingRefundYuan = cycle.FormatCents(page.Summary.PendingRefundCents)
	page.Summary.RefundedAmountYuan = cycle.FormatCents(page.Summary.RefundedAmountCents)
	return page, nil
}

func buildAfterSalesCaseView(caseItem model.AfterSalesCase) AfterSalesCaseView {
	view := AfterSalesCaseView{
		Case:             caseItem,
		PaidAmountYuan:   cycle.FormatCents(caseItem.PaidAmountCents),
		RefundAmountYuan: cycle.FormatCents(caseItem.RefundAmountCents),
		StatusLabel:      afterSalesStatusLabel(caseItem.Status),
	}
	if caseItem.ProcessedAt != nil {
		view.ProcessedAtLabel = caseItem.ProcessedAt.In(cycle.Location).Format("2006-01-02 15:04")
	}
	if caseItem.ExpiresAt != nil {
		view.ExpiresAtLabel = caseItem.ExpiresAt.In(cycle.Location).Format("2006-01-02 15:04")
	}
	return view
}

func (service *SubscriptionService) UpdateAfterSalesCase(
	caseID int64,
	input UpdateAfterSalesCaseInput,
) error {
	refundAmountCents, err := cycle.ParseYuanToCents(strings.TrimSpace(input.RefundAmountYuan))
	if err != nil || refundAmountCents < 0 {
		return fmt.Errorf("退款金额无效")
	}
	if err := service.validateAfterSalesRefundAmount(caseID, refundAmountCents); err != nil {
		return err
	}
	if err := service.Store.UpdateAfterSalesCase(caseID, refundAmountCents, input.Note); err != nil {
		switch {
		case errors.Is(err, db.ErrAfterSalesProcessed):
			return fmt.Errorf("该售后记录已经处理完成，不能再修改")
		case errors.Is(err, db.ErrAfterSalesRefundExceedsPayment):
			return fmt.Errorf("退款金额超过该账单剩余可退金额，请刷新后重试")
		case errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("售后记录不存在")
		}
		return err
	}
	return nil
}

func (service *SubscriptionService) SetAfterSalesCaseRefunded(caseID int64, refunded bool) error {
	processedAt := service.now()
	seatFreezeUntil := processedAt
	if refunded {
		caseItem, err := service.Store.GetAfterSalesCase(caseID)
		if err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("售后记录不存在")
			}
			return err
		}
		if err := service.validateAfterSalesRefundAmount(caseID, caseItem.RefundAmountCents); err != nil {
			return err
		}
		if caseItem.Source == model.AfterSalesSourceCustomerCancellation &&
			caseItem.BusinessType == model.SubscriptionBusinessTeam {
			freezeDays, err := service.GetSeatFreezeDays()
			if err != nil {
				return err
			}
			seatFreezeUntil = processedAt.AddDate(0, 0, freezeDays)
		}
	}
	if err := service.Store.SetAfterSalesCaseRefunded(
		caseID,
		refunded,
		processedAt,
		seatFreezeUntil,
	); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("售后记录不存在")
		}
		if errors.Is(err, db.ErrAfterSalesProcessed) {
			return fmt.Errorf("该退订售后已处理完成，不能撤销")
		}
		if errors.Is(err, db.ErrAfterSalesOriginalSeatBusy) {
			return fmt.Errorf("原车位已被其他订阅占用，无法撤销退款状态")
		}
		if errors.Is(err, db.ErrAfterSalesRefundExceedsPayment) {
			return fmt.Errorf("退款金额超过该账单剩余可退金额，请刷新后重试")
		}
		return err
	}
	return nil
}

func (service *SubscriptionService) validateAfterSalesRefundAmount(caseID int64, refundAmountCents int64) error {
	caseItem, err := service.Store.GetAfterSalesCase(caseID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("售后记录不存在")
		}
		return err
	}
	// A review case without a linked bill intentionally permits an operator-entered
	// amount. Once a paid bill is known, however, refunds must never exceed the
	// actual receipt, including any other completed refund linked to that bill.
	if caseItem.BillID <= 0 || caseItem.PaidAmountCents <= 0 {
		return nil
	}
	completedRefundCents := int64(0)
	cases, err := service.Store.ListAfterSalesCases()
	if err != nil {
		return err
	}
	for _, other := range cases {
		if other.ID != caseID &&
			other.BillID == caseItem.BillID &&
			other.Status == model.AfterSalesStatusRefunded {
			completedRefundCents += other.RefundAmountCents
		}
	}
	if refundAmountCents > caseItem.PaidAmountCents-completedRefundCents {
		return fmt.Errorf(
			"退款金额不能超过该账单剩余可退金额 ¥%s",
			cycle.FormatCents(caseItem.PaidAmountCents-completedRefundCents),
		)
	}
	return nil
}

func (service *SubscriptionService) ReassignAfterSalesCase(
	caseID int64,
	input ReassignAfterSalesCaseInput,
) error {
	if input.AccountID <= 0 {
		return fmt.Errorf("请选择新的母号空间")
	}
	if err := service.Store.ReassignAfterSalesCase(caseID, input.AccountID, input.SeatID, service.now()); err != nil {
		switch {
		case errors.Is(err, db.ErrAfterSalesProcessed):
			return fmt.Errorf("该售后记录已经处理完成")
		case errors.Is(err, db.ErrReplacementAccountBanned):
			return fmt.Errorf("新的母号也已封禁，请选择其他空间")
		case errors.Is(err, db.ErrReplacementSeatUnavailable):
			return fmt.Errorf("所选母号没有可用车位")
		case errors.Is(err, db.ErrReplacementSeatOccupied):
			return fmt.Errorf("所选车位已被占用，请刷新后重试")
		case errors.Is(err, db.ErrReplacementSeatUnchanged):
			return fmt.Errorf("新车位不能与当前车位相同")
		case errors.Is(err, db.ErrCancellationNotReassignable):
			return fmt.Errorf("主动退订只能完成退款，不能安排新空间")
		case errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("售后记录、订阅或可用车位不存在")
		}
		return err
	}
	return nil
}

func afterSalesStatusLabel(status string) string {
	switch status {
	case model.AfterSalesStatusRefunded:
		return "已退款"
	case model.AfterSalesStatusReassigned:
		return "已换空间"
	case model.AfterSalesStatusReview:
		return "待核对"
	default:
		return "待退款"
	}
}
