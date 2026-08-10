package service

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

// BillView is one bill row for the bills list page.
type BillView struct {
	ID               int64     `json:"id"`
	SubscriptionID   int64     `json:"subscription_id"`
	SubscriptionName string    `json:"subscription_name"`
	AccountName      string    `json:"account_name"`
	AccountEmail     string    `json:"account_email"`
	AccountSpaceName string    `json:"account_space_name"`
	AccountOpenedAt  string    `json:"account_opened_at"`
	SeatName         string    `json:"seat_name"`
	CustomerEmail    string    `json:"customer_email"`
	CustomerWechat   string    `json:"customer_wechat"`
	DueDate          string    `json:"due_date"`
	AmountYuan       string    `json:"amount_yuan"`
	AmountCents      int64     `json:"amount_cents"`
	RefundYuan       string    `json:"refund_yuan"`
	RefundCents      int64     `json:"refund_cents"`
	NetAmountYuan    string    `json:"net_amount_yuan"`
	NetAmountCents   int64     `json:"net_amount_cents"`
	Note             string    `json:"note"`
	PaidAtLabel      string    `json:"paid_at_label"`
	PaidAt           time.Time `json:"paid_at"`
	Archived         bool      `json:"archived"`
	StatusLabel      string    `json:"status_label"`
	TradeURL         string    `json:"trade_url"`
	PriceYuan        string    `json:"price_yuan"`
	CostYuan         string    `json:"cost_yuan"`
	AgencyFeeYuan    string    `json:"agency_fee_yuan"`
	IsResale         bool      `json:"is_resale"`
	ProfitYuan       string    `json:"profit_yuan"`
	CycleDesc        string    `json:"cycle_desc"`
	CronExpr         string    `json:"cron_expr"`
	OffsetsText      string    `json:"offsets_text"`
	Remark           string    `json:"remark"`
	BoardedAt        string    `json:"boarded_at"`
	ArchivedAtLabel  string    `json:"archived_at_label"`
	ChannelLabels    string    `json:"channel_labels"`
	AccountID        int64     `json:"account_id"`
	SeatID           int64     `json:"seat_id"`
}

// BillsSummary is the top KPI + chart model for the bills page.
type BillsSummary struct {
	BillCount              int                `json:"bill_count"`
	ActiveCount            int                `json:"active_count"`
	ArchivedCount          int                `json:"archived_count"`
	ResaleBillCount        int                `json:"resale_bill_count"`
	TotalAmountYuan        string             `json:"total_amount_yuan"`
	TotalRefundYuan        string             `json:"total_refund_yuan"`
	NetAmountYuan          string             `json:"net_amount_yuan"`
	TotalAgencyFeeYuan     string             `json:"total_agency_fee_yuan"`
	ThisMonthCount         int                `json:"this_month_count"`
	ThisMonthAmountYuan    string             `json:"this_month_amount_yuan"`
	ThisMonthRefundYuan    string             `json:"this_month_refund_yuan"`
	ThisMonthNetAmountYuan string             `json:"this_month_net_amount_yuan"`
	ThisMonthAgencyFeeYuan string             `json:"this_month_agency_fee_yuan"`
	AverageAmountYuan      string             `json:"average_amount_yuan"`
	AmountBySubscription   []AmountBar        `json:"amount_by_subscription"`
	Accounts               []AccountBreakdown `json:"accounts"`
	MonthlyTrend           []MonthAmountBar   `json:"monthly_trend"`
	MaxMonthCents          int64              `json:"max_month_cents"`
}

// MonthAmountBar is one month bucket in the bills trend chart.
type MonthAmountBar struct {
	Month            string `json:"month"`
	Label            string `json:"label"`
	Count            int    `json:"count"`
	AmountYuan       string `json:"amount_yuan"`
	AmountCents      int64  `json:"amount_cents"`
	GrossAmountYuan  string `json:"gross_amount_yuan"`
	GrossAmountCents int64  `json:"gross_amount_cents"`
	RefundYuan       string `json:"refund_yuan"`
	RefundCents      int64  `json:"refund_cents"`
	WidthPercent     int    `json:"width_percent"`
}

// BillsPage is the full bills page model.
type BillsPage struct {
	Bills   []BillView
	Summary BillsSummary
}

// BillEditInput is validated form input for editing a bill.
type BillEditInput struct {
	AmountYuan string
	Note       string
}

// ListBillsPage returns bill rows plus summary KPIs and chart series.
func (service *SubscriptionService) ListBillsPage() (BillsPage, error) {
	bills, err := service.Store.ListBills()
	if err != nil {
		return BillsPage{}, err
	}

	afterSalesCases, err := service.Store.ListAfterSalesCases()
	if err != nil {
		return BillsPage{}, err
	}
	refundsByBill := completedRefundsByBill(afterSalesCases)
	views := make([]BillView, 0, len(bills))
	for _, bill := range bills {
		view, err := service.buildBillView(bill, refundsByBill[bill.ID])
		if err != nil {
			return BillsPage{}, err
		}
		views = append(views, view)
	}

	return BillsPage{
		Bills:   views,
		Summary: buildBillsSummaryWithRefunds(views, afterSalesCases, service.now()),
	}, nil
}

// ListBillsView returns all bills with subscription names (including archived).
func (service *SubscriptionService) ListBillsView() ([]BillView, error) {
	page, err := service.ListBillsPage()
	if err != nil {
		return nil, err
	}
	return page.Bills, nil
}

func (service *SubscriptionService) buildBillView(bill model.Bill, refundCents int64) (BillView, error) {
	subscription, err := service.Store.GetSubscriptionIncludingArchived(bill.SubscriptionID)
	subscriptionName := fmt.Sprintf("订阅 #%d", bill.SubscriptionID)
	accountName := model.UnclassifiedAccountName
	accountEmail := ""
	accountSpaceName := ""
	accountOpenedAt := ""
	seatName := ""
	customerEmail := ""
	customerWechat := ""
	accountID := int64(0)
	seatID := int64(0)
	archived := false
	tradeURL := ""
	priceYuan := ""
	costYuan := ""
	agencyFeeYuan := ""
	isResale := false
	profitCents := int64(0)
	profitYuan := ""
	cycleDesc := ""
	cronExpr := ""
	offsetsText := ""
	remark := ""
	boardedAt := ""
	archivedAtLabel := ""
	channelLabels := ""
	if err == nil {
		subscriptionName = subscription.Name
		accountName = displayAccountName(subscription)
		seatName = subscription.SeatName
		customerEmail = subscription.CustomerEmail
		customerWechat = subscription.CustomerWechat
		accountID = subscription.AccountID
		seatID = subscription.SeatID
		archived = subscription.ArchivedAt != nil
		tradeURL = subscription.TradeURL
		priceYuan = cycle.FormatCents(subscription.PricePerPersonCents)
		costYuan = cycle.FormatCents(subscription.CostCents)
		agencyFeeYuan = cycle.FormatCents(subscription.AgencyFeeCents)
		isResale = subscription.IsResale
		profitCents = countedProfitCents(subscription) - refundCents
		profitYuan = cycle.FormatCents(profitCents)
		cycleDesc = cycle.DescribeCron(subscription.CronExpr)
		cronExpr = subscription.CronExpr
		offsetsText = cycle.FormatOffsets(subscription.NotifyOffsets)
		channelLabels = scheduledNotificationLabelText(subscription.NotifyOffsets)
		remark = subscription.Remark
		boardedAt = subscription.BoardedAt
		if subscription.ArchivedAt != nil {
			archivedAtLabel = subscription.ArchivedAt.In(cycle.Location).Format("2006-01-02 15:04")
		}
		if accountID > 0 {
			account, accountErr := service.Store.GetAccount(accountID)
			if accountErr == nil {
				accountEmail = account.Email
				accountSpaceName = account.SpaceName
				accountOpenedAt = account.OpenedAt
			}
		}
	} else if err != sql.ErrNoRows {
		return BillView{}, err
	}

	statusLabel := "进行中"
	if archived {
		statusLabel = "已下车"
	}

	netAmountCents := bill.AmountCents - refundCents
	return BillView{
		ID:               bill.ID,
		SubscriptionID:   bill.SubscriptionID,
		SubscriptionName: subscriptionName,
		AccountName:      accountName,
		AccountEmail:     accountEmail,
		AccountSpaceName: accountSpaceName,
		AccountOpenedAt:  accountOpenedAt,
		SeatName:         seatName,
		CustomerEmail:    customerEmail,
		CustomerWechat:   customerWechat,
		AccountID:        accountID,
		SeatID:           seatID,
		DueDate:          bill.DueDate,
		AmountYuan:       cycle.FormatCents(bill.AmountCents),
		AmountCents:      bill.AmountCents,
		RefundYuan:       cycle.FormatCents(refundCents),
		RefundCents:      refundCents,
		NetAmountYuan:    cycle.FormatCents(netAmountCents),
		NetAmountCents:   netAmountCents,
		Note:             bill.Note,
		PaidAtLabel:      formatPaidAtLabel(bill.PaidAt),
		PaidAt:           bill.PaidAt,
		Archived:         archived,
		StatusLabel:      statusLabel,
		TradeURL:         tradeURL,
		PriceYuan:        priceYuan,
		CostYuan:         costYuan,
		AgencyFeeYuan:    agencyFeeYuan,
		IsResale:         isResale,
		ProfitYuan:       profitYuan,
		CycleDesc:        cycleDesc,
		CronExpr:         cronExpr,
		OffsetsText:      offsetsText,
		Remark:           remark,
		BoardedAt:        boardedAt,
		ArchivedAtLabel:  archivedAtLabel,
		ChannelLabels:    channelLabels,
	}, nil
}

func buildBillsSummary(views []BillView, now time.Time) BillsSummary {
	return buildBillsSummaryWithRefunds(views, nil, now)
}

func buildBillsSummaryWithRefunds(
	views []BillView,
	afterSalesCases []model.AfterSalesCase,
	now time.Time,
) BillsSummary {
	now = now.In(cycle.Location)
	thisMonthKey := now.Format("2006-01")

	var totalCents int64
	var totalRefundCents int64
	var thisMonthCents int64
	var thisMonthRefundCents int64
	var totalAgencyFeeCents int64
	var thisMonthAgencyFeeCents int64
	thisMonthCount := 0
	activeCount := 0
	archivedCount := 0
	resaleBillCount := 0

	subscriptionTotals := map[int64]*AmountBar{}
	accountTotals := map[string]struct {
		count int
		cents int64
	}{}
	monthTotals := map[string]struct {
		count       int
		grossCents  int64
		refundCents int64
	}{}

	for _, view := range views {
		netAmountCents := view.NetAmountCents
		if view.NetAmountYuan == "" {
			netAmountCents = view.AmountCents - view.RefundCents
		}
		totalCents += view.AmountCents
		if view.Archived {
			archivedCount++
		} else {
			activeCount++
		}
		if view.IsResale {
			resaleBillCount++
			totalAgencyFeeCents += view.AmountCents
		}

		billingMonth := billingMonthKey(view.DueDate)
		if billingMonth != "" {
			bucket := monthTotals[billingMonth]
			bucket.count++
			bucket.grossCents += view.AmountCents
			monthTotals[billingMonth] = bucket
			if billingMonth == thisMonthKey {
				thisMonthCount++
				thisMonthCents += view.AmountCents
				if view.IsResale {
					thisMonthAgencyFeeCents += view.AmountCents
				}
			}
		}

		bar, exists := subscriptionTotals[view.SubscriptionID]
		if !exists {
			bar = &AmountBar{
				Name:        view.SubscriptionName,
				AccountName: view.AccountName,
			}
			subscriptionTotals[view.SubscriptionID] = bar
		}
		bar.AmountCents += netAmountCents
		bar.AmountYuan = cycle.FormatCents(bar.AmountCents)

		accountBucket := accountTotals[view.AccountName]
		accountBucket.count++
		accountBucket.cents += netAmountCents
		accountTotals[view.AccountName] = accountBucket
	}

	for _, caseItem := range afterSalesCases {
		if caseItem.Status != model.AfterSalesStatusRefunded {
			continue
		}
		totalRefundCents += caseItem.RefundAmountCents
		if caseItem.ProcessedAt == nil {
			continue
		}
		monthKey := caseItem.ProcessedAt.In(cycle.Location).Format("2006-01")
		bucket := monthTotals[monthKey]
		bucket.refundCents += caseItem.RefundAmountCents
		monthTotals[monthKey] = bucket
		if monthKey == thisMonthKey {
			thisMonthRefundCents += caseItem.RefundAmountCents
		}
	}

	amountBars := make([]AmountBar, 0, len(subscriptionTotals))
	for _, bar := range subscriptionTotals {
		amountBars = append(amountBars, *bar)
	}
	sort.Slice(amountBars, func(left int, right int) bool {
		if amountBars[left].AmountCents == amountBars[right].AmountCents {
			return amountBars[left].Name < amountBars[right].Name
		}
		return amountBars[left].AmountCents > amountBars[right].AmountCents
	})

	accounts := make([]AccountBreakdown, 0, len(accountTotals))
	for accountName, bucket := range accountTotals {
		accounts = append(accounts, AccountBreakdown{
			AccountName: accountName,
			Type:        accountName,
			Count:       bucket.count,
			AmountYuan:  cycle.FormatCents(bucket.cents),
			AmountCents: bucket.cents,
		})
	}
	sort.Slice(accounts, func(left int, right int) bool {
		if accounts[left].AmountCents == accounts[right].AmountCents {
			return accounts[left].AccountName < accounts[right].AccountName
		}
		return accounts[left].AmountCents > accounts[right].AmountCents
	})

	// Last 6 calendar months including current, oldest first.
	monthlyTrend := make([]MonthAmountBar, 0, 6)
	var maxMonthCents int64
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, cycle.Location).AddDate(0, -5, 0)
	for index := 0; index < 6; index++ {
		month := monthStart.AddDate(0, index, 0)
		monthKey := month.Format("2006-01")
		bucket := monthTotals[monthKey]
		netCents := bucket.grossCents - bucket.refundCents
		absoluteNetCents := netCents
		if absoluteNetCents < 0 {
			absoluteNetCents = -absoluteNetCents
		}
		if absoluteNetCents > maxMonthCents {
			maxMonthCents = absoluteNetCents
		}
		monthlyTrend = append(monthlyTrend, MonthAmountBar{
			Month:            monthKey,
			Label:            fmt.Sprintf("%d月", month.Month()),
			Count:            bucket.count,
			AmountYuan:       cycle.FormatCents(netCents),
			AmountCents:      netCents,
			GrossAmountYuan:  cycle.FormatCents(bucket.grossCents),
			GrossAmountCents: bucket.grossCents,
			RefundYuan:       cycle.FormatCents(bucket.refundCents),
			RefundCents:      bucket.refundCents,
		})
	}
	if maxMonthCents == 0 {
		maxMonthCents = 1
	}
	for index := range monthlyTrend {
		amountCents := monthlyTrend[index].AmountCents
		if amountCents < 0 {
			amountCents = -amountCents
		}
		width := int(amountCents * 100 / maxMonthCents)
		if amountCents > 0 && width < 4 {
			width = 4
		}
		monthlyTrend[index].WidthPercent = width
	}

	averageYuan := "0.00"
	if len(views) > 0 {
		averageYuan = cycle.FormatCents(totalCents / int64(len(views)))
	}

	return BillsSummary{
		BillCount:              len(views),
		ActiveCount:            activeCount,
		ArchivedCount:          archivedCount,
		ResaleBillCount:        resaleBillCount,
		TotalAmountYuan:        cycle.FormatCents(totalCents),
		TotalRefundYuan:        cycle.FormatCents(totalRefundCents),
		NetAmountYuan:          cycle.FormatCents(totalCents - totalRefundCents),
		TotalAgencyFeeYuan:     cycle.FormatCents(totalAgencyFeeCents),
		ThisMonthCount:         thisMonthCount,
		ThisMonthAmountYuan:    cycle.FormatCents(thisMonthCents),
		ThisMonthRefundYuan:    cycle.FormatCents(thisMonthRefundCents),
		ThisMonthNetAmountYuan: cycle.FormatCents(thisMonthCents - thisMonthRefundCents),
		ThisMonthAgencyFeeYuan: cycle.FormatCents(thisMonthAgencyFeeCents),
		AverageAmountYuan:      averageYuan,
		AmountBySubscription:   amountBars,
		Accounts:               accounts,
		MonthlyTrend:           monthlyTrend,
		MaxMonthCents:          maxMonthCents,
	}
}

func completedRefundsByBill(cases []model.AfterSalesCase) map[int64]int64 {
	refunds := make(map[int64]int64)
	for _, caseItem := range cases {
		if caseItem.Status == model.AfterSalesStatusRefunded && caseItem.BillID > 0 {
			refunds[caseItem.BillID] += caseItem.RefundAmountCents
		}
	}
	return refunds
}

func billingMonthKey(dueDate string) string {
	if len(dueDate) >= len("2006-01") {
		return dueDate[:len("2006-01")]
	}
	return ""
}

func formatPaidAtLabel(paidAt time.Time) string {
	if paidAt.IsZero() {
		return "—"
	}
	return paidAt.In(cycle.Location).Format("2006-01-02 15:04")
}

// UpdateBill validates and updates a bill's amount and note.
// Does not change the linked subscription's price.
func (service *SubscriptionService) UpdateBill(billID int64, input BillEditInput) error {
	if _, err := service.Store.GetBill(billID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("账单不存在")
		}
		return err
	}

	amountCents, err := cycle.ParseYuanToCents(input.AmountYuan)
	if err != nil {
		return fmt.Errorf("金额无效: %w", err)
	}
	note := strings.TrimSpace(input.Note)
	if err := service.Store.UpdateBill(billID, amountCents, note); err != nil {
		if errors.Is(err, db.ErrBillHasAfterSalesCase) {
			return fmt.Errorf("该账单已关联售后处理记录，不能修改")
		}
		return err
	}
	return nil
}

// DeleteBill removes a bill permanently. The corresponding due date becomes unpaid again.
func (service *SubscriptionService) DeleteBill(billID int64) error {
	if _, err := service.Store.GetBill(billID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("账单不存在")
		}
		return err
	}
	if err := service.Store.DeleteBill(billID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("账单不存在")
		}
		if errors.Is(err, db.ErrBillHasAfterSalesCase) {
			return fmt.Errorf("该账单已关联售后处理记录，不能删除或取消缴费")
		}
		return err
	}
	return nil
}

// GetBillView returns one bill for edit forms.
func (service *SubscriptionService) GetBillView(billID int64) (BillView, error) {
	bill, err := service.Store.GetBill(billID)
	if err != nil {
		if err == sql.ErrNoRows {
			return BillView{}, fmt.Errorf("账单不存在")
		}
		return BillView{}, err
	}
	afterSalesCases, err := service.Store.ListAfterSalesCases()
	if err != nil {
		return BillView{}, err
	}
	return service.buildBillView(bill, completedRefundsByBill(afterSalesCases)[bill.ID])
}
