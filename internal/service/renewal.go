package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

const maxRenewalOperatorNoteLength = 500

type RenewalLookupInput struct {
	CustomerEmail string
}

type RenewalSubmitInput struct {
	CustomerEmail  string
	SubscriptionID int64
	PeriodCount    int
}

type RenewalSubmitResult struct {
	TrackingToken string `json:"tracking_token"`
	Status        string `json:"status"`
}

type RenewalSubscriptionView struct {
	SubscriptionID    int64                 `json:"subscription_id"`
	BusinessType      string                `json:"business_type"`
	ServiceLabel      string                `json:"service_label"`
	AccountSerial     int64                 `json:"account_serial"`
	SeatName          string                `json:"seat_name"`
	DueDate           string                `json:"due_date"`
	PeriodEndDate     string                `json:"period_end_date"`
	DaysRemaining     int                   `json:"days_remaining"`
	StatusLabel       string                `json:"status_label"`
	AmountYuan        string                `json:"amount_yuan"`
	CycleDesc         string                `json:"cycle_desc"`
	PeriodCount       int                   `json:"period_count"`
	PeriodOptions     []RenewalPeriodOption `json:"period_options"`
	Renewable         bool                  `json:"renewable"`
	PendingReview     bool                  `json:"pending_review"`
	UnavailableReason string                `json:"unavailable_reason"`
}

type RenewalPeriodOption struct {
	PeriodCount   int    `json:"period_count"`
	AmountYuan    string `json:"amount_yuan"`
	PeriodEndDate string `json:"period_end_date"`
}

type RenewalLookupView struct {
	CustomerEmail string                    `json:"customer_email"`
	Subscriptions []RenewalSubscriptionView `json:"subscriptions"`
}

type RenewalStatusView struct {
	Status           string `json:"status"`
	CustomerEmail    string `json:"customer_email"`
	BusinessType     string `json:"business_type"`
	ServiceLabel     string `json:"service_label"`
	DueDate          string `json:"due_date"`
	PeriodCount      int    `json:"period_count"`
	PeriodEndDate    string `json:"period_end_date"`
	AmountYuan       string `json:"amount_yuan"`
	CycleDesc        string `json:"cycle_desc"`
	CreatedAtLabel   string `json:"created_at_label"`
	ProcessedAtLabel string `json:"processed_at_label"`
	OperatorNote     string `json:"operator_note"`
}

type RenewalApplicationView struct {
	Application      model.RenewalApplication `json:"application"`
	BusinessType     string                   `json:"business_type"`
	ServiceLabel     string                   `json:"service_label"`
	AmountYuan       string                   `json:"amount_yuan"`
	CycleDesc        string                   `json:"cycle_desc"`
	AccountSerial    int64                    `json:"account_serial"`
	AccountEmail     string                   `json:"account_email"`
	SeatName         string                   `json:"seat_name"`
	CreatedAtLabel   string                   `json:"created_at_label"`
	ProcessedAtLabel string                   `json:"processed_at_label"`
}

type RenewalDecisionInput struct {
	OperatorNote string
}

func (service *SubscriptionService) LookupRenewalSubscriptions(input RenewalLookupInput) (RenewalLookupView, error) {
	if _, err := service.Store.RestoreExpiredCancellationRequests(service.now()); err != nil {
		return RenewalLookupView{}, err
	}
	email, err := normalizeCustomerEmail(input.CustomerEmail)
	if err != nil || email == "" {
		return RenewalLookupView{}, fmt.Errorf("请输入有效的订阅邮箱")
	}
	if len([]rune(email)) > maxRedemptionEmailLength {
		return RenewalLookupView{}, fmt.Errorf("订阅邮箱最多 %d 个字", maxRedemptionEmailLength)
	}

	subscriptions, err := service.Store.ListSubscriptions()
	if err != nil {
		return RenewalLookupView{}, err
	}
	pendingApplications, err := service.Store.ListRenewalApplications(model.RenewalStatusPending)
	if err != nil {
		return RenewalLookupView{}, err
	}
	pendingPeriods := make(map[string]struct{}, len(pendingApplications))
	for _, application := range pendingApplications {
		pendingPeriods[renewalPeriodKey(application.SubscriptionID, application.DueDate)] = struct{}{}
	}

	result := RenewalLookupView{
		CustomerEmail: email,
		Subscriptions: make([]RenewalSubscriptionView, 0),
	}
	for _, subscription := range subscriptions {
		if !strings.EqualFold(strings.TrimSpace(subscription.CustomerEmail), email) {
			continue
		}
		view, err := service.buildRenewalSubscriptionView(subscription, pendingPeriods)
		if err != nil {
			return RenewalLookupView{}, err
		}
		result.Subscriptions = append(result.Subscriptions, view)
	}
	if len(result.Subscriptions) == 0 {
		return RenewalLookupView{}, fmt.Errorf("没有找到该邮箱对应的有效订阅，请核对邮箱或联系客服")
	}
	return result, nil
}

func (service *SubscriptionService) SubmitRenewalApplication(input RenewalSubmitInput) (RenewalSubmitResult, error) {
	settings, err := service.GetRedeemPageSettings()
	if err != nil {
		return RenewalSubmitResult{}, err
	}
	if strings.TrimSpace(settings.PaymentQRCodeDataURL) == "" {
		return RenewalSubmitResult{}, fmt.Errorf("续费收款码尚未配置，请联系客服处理")
	}
	lookup, err := service.LookupRenewalSubscriptions(RenewalLookupInput{CustomerEmail: input.CustomerEmail})
	if err != nil {
		return RenewalSubmitResult{}, err
	}
	var selected *RenewalSubscriptionView
	for index := range lookup.Subscriptions {
		if lookup.Subscriptions[index].SubscriptionID == input.SubscriptionID {
			selected = &lookup.Subscriptions[index]
			break
		}
	}
	if selected == nil {
		return RenewalSubmitResult{}, fmt.Errorf("该订阅不属于当前邮箱，请重新查询")
	}
	if !selected.Renewable {
		return RenewalSubmitResult{}, fmt.Errorf("%s", selected.UnavailableReason)
	}
	periodCount, err := normalizeRenewalPeriodCount(input.PeriodCount)
	if err != nil {
		return RenewalSubmitResult{}, err
	}
	var selectedOption *RenewalPeriodOption
	for index := range selected.PeriodOptions {
		if selected.PeriodOptions[index].PeriodCount == periodCount {
			selectedOption = &selected.PeriodOptions[index]
			break
		}
	}
	if selectedOption == nil {
		return RenewalSubmitResult{}, fmt.Errorf("无法计算所选续费周期，请重新查询")
	}

	amountCents, err := cycle.ParseYuanToCents(selectedOption.AmountYuan)
	if err != nil {
		return RenewalSubmitResult{}, fmt.Errorf("读取续费金额失败: %w", err)
	}
	for attempt := 0; attempt < 5; attempt++ {
		token, err := newRedemptionTrackingToken()
		if err != nil {
			return RenewalSubmitResult{}, err
		}
		_, err = service.Store.CreateRenewalApplication(model.RenewalApplication{
			TrackingToken:  token,
			SubscriptionID: selected.SubscriptionID,
			CustomerEmail:  lookup.CustomerEmail,
			DueDate:        selected.DueDate,
			PeriodCount:    selectedOption.PeriodCount,
			PeriodEndDate:  selectedOption.PeriodEndDate,
			AmountCents:    amountCents,
		})
		if err == nil {
			if alertErr := service.sendRenewalApplicationAlert(*selected, *selectedOption, lookup.CustomerEmail); alertErr != nil {
				// The review request is already durable. Notification delivery is
				// best-effort and must never make the customer resubmit or create a duplicate.
				log.Printf("send self-service renewal application alert: %v", alertErr)
			}
			return RenewalSubmitResult{TrackingToken: token, Status: model.RenewalStatusPending}, nil
		}
		if errors.Is(err, db.ErrRenewalAlreadyPending) {
			return RenewalSubmitResult{}, fmt.Errorf("该账期已有待审核的续费申请，请勿重复提交")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "tracking_token") {
			return RenewalSubmitResult{}, err
		}
	}
	return RenewalSubmitResult{}, fmt.Errorf("生成续费审核编号失败，请重试")
}

func (service *SubscriptionService) sendRenewalApplicationAlert(
	selected RenewalSubscriptionView,
	option RenewalPeriodOption,
	customerEmail string,
) error {
	recipient, err := service.GetRenewalApplicationAlertEmail()
	if err != nil || recipient == "" {
		return err
	}
	_, registry := service.runtimeConfigSnapshot()
	sender, ok := registry.Get(model.ChannelSMTP)
	if !ok {
		return fmt.Errorf("自助续费提醒邮箱已设置，但 SMTP 发送器未配置")
	}
	addressed, ok := sender.(smtpAddressedSender)
	if !ok {
		return fmt.Errorf("SMTP 发送器不支持指定自助续费提醒收件人")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	title := "[Carpool Notify Plus] 新的自助续费申请"
	body := strings.Join([]string{
		"有一条新的自助续费申请等待处理。",
		"",
		"客户邮箱：" + customerEmail,
		"服务类型：" + selected.ServiceLabel,
		"应收总额：¥" + option.AmountYuan,
		"固定套餐：" + selected.CycleDesc,
		fmt.Sprintf("购买周期：%d 个计费周期", option.PeriodCount),
		"续费范围：" + selected.DueDate + " 至 " + option.PeriodEndDate,
		"提交时间：" + cycle.FormatDateTime(service.now()),
		"",
		"请前往管理后台的“兑换申请 → 续费审核”尽快核对款项。",
	}, "\n")
	return addressed.SendTo(ctx, []string{recipient}, title, body)
}

func (service *SubscriptionService) GetRenewalStatus(token string) (RenewalStatusView, error) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 128 {
		return RenewalStatusView{}, fmt.Errorf("续费申请编号无效")
	}
	application, err := service.Store.GetRenewalApplicationByToken(token)
	if err != nil {
		if err == sql.ErrNoRows {
			return RenewalStatusView{}, fmt.Errorf("没有找到这条续费申请")
		}
		return RenewalStatusView{}, err
	}
	return service.buildRenewalStatusView(application), nil
}

func (service *SubscriptionService) ListRenewalApplicationsView(status string) ([]RenewalApplicationView, error) {
	applications, err := service.Store.ListRenewalApplications(strings.TrimSpace(status))
	if err != nil {
		return nil, err
	}
	views := make([]RenewalApplicationView, 0, len(applications))
	for _, application := range applications {
		if application.PeriodCount <= 0 {
			application.PeriodCount = 1
		}
		view := RenewalApplicationView{
			Application:      application,
			AmountYuan:       cycle.FormatCents(application.AmountCents),
			CreatedAtLabel:   cycle.FormatDateTime(application.CreatedAt),
			ProcessedAtLabel: formatOptionalTime(application.ProcessedAt),
		}
		subscription, getErr := service.Store.GetSubscriptionIncludingArchived(application.SubscriptionID)
		if getErr != nil && getErr != sql.ErrNoRows {
			return nil, getErr
		}
		if getErr == nil {
			view.BusinessType = subscription.BusinessType
			view.ServiceLabel = renewalServiceLabel(subscription)
			view.CycleDesc = cycle.DescribeCron(subscription.CronExpr)
			view.SeatName = subscription.SeatName
			if application.PeriodEndDate == "" {
				if plan, planErr := buildRenewalPeriodPlan(subscription, application.DueDate, application.PeriodCount); planErr == nil {
					view.Application.PeriodEndDate = plan.PeriodEndDate
				}
			}
			if subscription.AccountID > 0 {
				account, accountErr := service.Store.GetAccount(subscription.AccountID)
				if accountErr != nil && accountErr != sql.ErrNoRows {
					return nil, accountErr
				}
				if accountErr == nil {
					view.AccountSerial = accountDisplaySerial(account)
					view.AccountEmail = account.Email
				}
			}
		}
		views = append(views, view)
	}
	return views, nil
}

func (service *SubscriptionService) ApproveRenewalApplication(applicationID int64, input RenewalDecisionInput) error {
	application, err := service.Store.GetRenewalApplication(applicationID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("续费申请不存在")
		}
		return err
	}
	if application.Status != model.RenewalStatusPending {
		return fmt.Errorf("这条续费申请已经处理过")
	}
	subscription, err := service.Store.GetSubscription(application.SubscriptionID)
	if err != nil {
		return fmt.Errorf("订阅状态已变化，请驳回申请并让客户重新查询")
	}
	if !strings.EqualFold(strings.TrimSpace(subscription.CustomerEmail), application.CustomerEmail) {
		return fmt.Errorf("订阅邮箱已变化，请驳回申请并让客户重新查询")
	}
	view, err := service.buildRenewalSubscriptionView(subscription, nil)
	if err != nil {
		return err
	}
	periodCount, countErr := normalizeRenewalPeriodCount(application.PeriodCount)
	plan, planErr := buildRenewalPeriodPlan(subscription, view.DueDate, periodCount)
	if countErr != nil || planErr != nil || view.DueDate != application.DueDate ||
		plan.AmountCents != application.AmountCents ||
		(application.PeriodEndDate != "" && plan.PeriodEndDate != application.PeriodEndDate) || !view.Renewable {
		return fmt.Errorf("订阅账期、金额或状态已变化，请驳回申请并让客户重新查询")
	}
	note, err := trimLimited("审核备注", input.OperatorNote, maxRenewalOperatorNoteLength)
	if err != nil {
		return err
	}
	if err := service.Store.ApproveRenewalApplication(application, subscription, plan.Bills, plan.PeriodEndDate, note); err != nil {
		switch {
		case errors.Is(err, db.ErrRenewalAlreadyProcessed):
			return fmt.Errorf("续费申请状态已变化，请刷新后重试")
		case errors.Is(err, db.ErrRenewalFinancialStateChanged):
			return fmt.Errorf("订阅账期、金额或状态已变化，请驳回申请并让客户重新查询")
		default:
			return err
		}
	}
	return nil
}

func (service *SubscriptionService) RejectRenewalApplication(applicationID int64, input RenewalDecisionInput) error {
	application, err := service.Store.GetRenewalApplication(applicationID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("续费申请不存在")
		}
		return err
	}
	if application.Status != model.RenewalStatusPending {
		return fmt.Errorf("这条续费申请已经处理过")
	}
	note, err := trimLimited("驳回原因", input.OperatorNote, maxRenewalOperatorNoteLength)
	if err != nil {
		return err
	}
	if note == "" {
		note = "未核对到对应款项，请检查付款金额和邮箱备注后联系客服"
	}
	if err := service.Store.RejectRenewalApplication(applicationID, note); err != nil {
		if errors.Is(err, db.ErrRenewalAlreadyProcessed) {
			return fmt.Errorf("续费申请状态已变化，请刷新后重试")
		}
		return err
	}
	return nil
}

func (service *SubscriptionService) buildRenewalSubscriptionView(
	subscription model.Subscription,
	pendingPeriods map[string]struct{},
) (RenewalSubscriptionView, error) {
	view, err := service.buildView(subscription, service.now(), "")
	if err != nil {
		return RenewalSubscriptionView{}, err
	}
	result := RenewalSubscriptionView{
		SubscriptionID: subscription.ID,
		BusinessType:   subscription.BusinessType,
		ServiceLabel:   renewalServiceLabel(subscription),
		SeatName:       subscription.SeatName,
		DueDate:        view.NextDueDate,
		DaysRemaining:  view.DaysRemaining,
		CycleDesc:      view.CycleDesc,
		PeriodCount:    1,
		PeriodOptions:  make([]RenewalPeriodOption, 0, model.MaxRenewalPeriodCount),
		Renewable:      true,
	}
	for periodCount := 1; periodCount <= model.MaxRenewalPeriodCount; periodCount++ {
		plan, planErr := buildRenewalPeriodPlan(subscription, result.DueDate, periodCount)
		if planErr != nil {
			return RenewalSubscriptionView{}, planErr
		}
		result.PeriodOptions = append(result.PeriodOptions, RenewalPeriodOption{
			PeriodCount:   periodCount,
			AmountYuan:    cycle.FormatCents(plan.AmountCents),
			PeriodEndDate: plan.PeriodEndDate,
		})
	}
	result.AmountYuan = result.PeriodOptions[0].AmountYuan
	result.PeriodEndDate = result.PeriodOptions[0].PeriodEndDate
	if subscription.AccountID > 0 {
		account, accountErr := service.Store.GetAccount(subscription.AccountID)
		if accountErr != nil {
			return RenewalSubscriptionView{}, accountErr
		}
		result.AccountSerial = accountDisplaySerial(account)
	}
	switch {
	case result.DaysRemaining < 0:
		result.StatusLabel = fmt.Sprintf("已逾期 %d 天", -result.DaysRemaining)
	case result.DaysRemaining == 0:
		result.StatusLabel = "今天到期"
	case result.DaysRemaining <= 7:
		result.StatusLabel = fmt.Sprintf("%d 天后到期", result.DaysRemaining)
	default:
		result.StatusLabel = "正常使用"
	}
	if isOneMonthRental(subscription) {
		result.Renewable = false
		result.UnavailableReason = "单月短租不支持自助续费，请联系客服重新开通"
	}
	if subscription.CancellationCaseID > 0 || subscription.CancellationRequestedAt != nil {
		result.Renewable = false
		result.UnavailableReason = "该订阅正在退订处理中，暂不能提交续费"
	}
	pendingAfterSales, err := service.Store.CountPendingAfterSalesCasesBySubscription(subscription.ID)
	if err != nil {
		return RenewalSubscriptionView{}, err
	}
	if pendingAfterSales > 0 {
		result.Renewable = false
		result.UnavailableReason = "该订阅正在售后处理中，请联系客服处理续费"
	}
	if pendingPeriods != nil {
		if _, exists := pendingPeriods[renewalPeriodKey(subscription.ID, result.DueDate)]; exists {
			result.Renewable = false
			result.PendingReview = true
			result.UnavailableReason = "该账期已提交续费审核，请等待管理员处理"
		}
	}
	return result, nil
}

func (service *SubscriptionService) buildRenewalStatusView(application model.RenewalApplication) RenewalStatusView {
	if application.PeriodCount <= 0 {
		application.PeriodCount = 1
	}
	view := RenewalStatusView{
		Status:           application.Status,
		CustomerEmail:    application.CustomerEmail,
		DueDate:          application.DueDate,
		PeriodCount:      application.PeriodCount,
		PeriodEndDate:    application.PeriodEndDate,
		AmountYuan:       cycle.FormatCents(application.AmountCents),
		CreatedAtLabel:   cycle.FormatDateTime(application.CreatedAt),
		ProcessedAtLabel: formatOptionalTime(application.ProcessedAt),
	}
	if application.Status == model.RenewalStatusRejected {
		view.OperatorNote = application.OperatorNote
	}
	if subscription, err := service.Store.GetSubscriptionIncludingArchived(application.SubscriptionID); err == nil {
		view.BusinessType = subscription.BusinessType
		view.ServiceLabel = renewalServiceLabel(subscription)
		view.CycleDesc = cycle.DescribeCron(subscription.CronExpr)
		if view.PeriodEndDate == "" {
			if plan, planErr := buildRenewalPeriodPlan(subscription, application.DueDate, application.PeriodCount); planErr == nil {
				view.PeriodEndDate = plan.PeriodEndDate
			}
		}
	}
	return view
}

type renewalPeriodPlan struct {
	Bills         []model.Bill
	AmountCents   int64
	PeriodEndDate string
}

func normalizeRenewalPeriodCount(periodCount int) (int, error) {
	if periodCount == 0 {
		return 1, nil
	}
	if periodCount < 1 || periodCount > model.MaxRenewalPeriodCount {
		return 0, fmt.Errorf("续费周期只能选择 1–%d 个原计费周期", model.MaxRenewalPeriodCount)
	}
	return periodCount, nil
}

func buildRenewalPeriodPlan(
	subscription model.Subscription,
	firstDueDate string,
	periodCount int,
) (renewalPeriodPlan, error) {
	periodCount, err := normalizeRenewalPeriodCount(periodCount)
	if err != nil {
		return renewalPeriodPlan{}, err
	}
	schedule, err := cycle.ParseBillingSchedule(subscription.CronExpr, subscription.BoardedAt)
	if err != nil {
		return renewalPeriodPlan{}, fmt.Errorf("读取原计费周期失败: %w", err)
	}
	dueAt, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(firstDueDate), cycle.Location)
	if err != nil {
		return renewalPeriodPlan{}, fmt.Errorf("读取续费日期失败: %w", err)
	}
	plan := renewalPeriodPlan{Bills: make([]model.Bill, 0, periodCount)}
	for index := 0; index < periodCount; index++ {
		dueDate := cycle.FormatDate(dueAt)
		amountCents := billAmountCentsForDueDate(subscription, dueDate)
		if amountCents <= 0 {
			return renewalPeriodPlan{}, fmt.Errorf("续费金额必须大于 0")
		}
		plan.Bills = append(plan.Bills, model.Bill{
			SubscriptionID: subscription.ID,
			DueDate:        dueDate,
			AmountCents:    amountCents,
			CostCents:      billDefaultCostCents(subscription),
		})
		plan.AmountCents += amountCents
		nextDue := schedule.NextDue(dueAt)
		if !nextDue.After(dueAt) {
			return renewalPeriodPlan{}, fmt.Errorf("原计费周期无法继续推进")
		}
		dueAt = nextDue
	}
	plan.PeriodEndDate = cycle.FormatDate(dueAt)
	return plan, nil
}

func renewalServiceLabel(subscription model.Subscription) string {
	if isPlusSubscription(subscription) {
		return "Plus 出租"
	}
	return "Team 席位"
}

func renewalPeriodKey(subscriptionID int64, dueDate string) string {
	return fmt.Sprintf("%d:%s", subscriptionID, strings.TrimSpace(dueDate))
}
