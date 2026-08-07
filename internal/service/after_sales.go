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
	Cases   []AfterSalesCaseView `json:"cases"`
	Summary AfterSalesSummary    `json:"summary"`
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

func (service *SubscriptionService) ListAfterSalesPage() (AfterSalesPage, error) {
	cases, err := service.Store.ListAfterSalesCases()
	if err != nil {
		return AfterSalesPage{}, err
	}
	page := AfterSalesPage{Cases: make([]AfterSalesCaseView, 0, len(cases))}
	for _, caseItem := range cases {
		view := AfterSalesCaseView{
			Case:             caseItem,
			PaidAmountYuan:   cycle.FormatCents(caseItem.PaidAmountCents),
			RefundAmountYuan: cycle.FormatCents(caseItem.RefundAmountCents),
			StatusLabel:      afterSalesStatusLabel(caseItem.Status),
		}
		if caseItem.ProcessedAt != nil {
			view.ProcessedAtLabel = caseItem.ProcessedAt.In(cycle.Location).Format("2006-01-02 15:04")
		}
		page.Cases = append(page.Cases, view)
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

func (service *SubscriptionService) UpdateAfterSalesCase(
	caseID int64,
	input UpdateAfterSalesCaseInput,
) error {
	refundAmountCents, err := cycle.ParseYuanToCents(strings.TrimSpace(input.RefundAmountYuan))
	if err != nil || refundAmountCents < 0 {
		return fmt.Errorf("退款金额无效")
	}
	return service.Store.UpdateAfterSalesCase(caseID, refundAmountCents, input.Note)
}

func (service *SubscriptionService) SetAfterSalesCaseRefunded(caseID int64, refunded bool) error {
	if err := service.Store.SetAfterSalesCaseRefunded(caseID, refunded, service.now()); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("售后记录不存在")
		}
		return err
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
