package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"carpool-notify/internal/config"
	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"

	"github.com/gin-gonic/gin"
)

// ---- Data queries -----------------------------------------------------------

func (server *Server) getCalendar(context *gin.Context) {
	now := cycle.Now()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, cycle.Location)
	if rawMonth := strings.TrimSpace(context.Query("month")); rawMonth != "" {
		parsedMonth, err := time.ParseInLocation("2006-01", rawMonth, cycle.Location)
		if err != nil {
			respondError(context, http.StatusBadRequest, "无效的月份，请使用 YYYY-MM")
			return
		}
		month = parsedMonth
	}
	calendarView, err := server.Service.CalendarMonth(month)
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(context, gin.H{"calendar": calendarView})
}

func (server *Server) getDashboard(context *gin.Context) {
	dashboard, err := server.Service.ComputeDashboard()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(context, gin.H{"dashboard": dashboard})
}

func (server *Server) getGoals(context *gin.Context) {
	center, err := server.Service.GetGoalCenter()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(context, gin.H{"goals": center})
}

type businessGoalRequest struct {
	Name             string `json:"name"`
	TargetProfitYuan string `json:"target_profit_yuan"`
}

func (request businessGoalRequest) toInput() service.BusinessGoalInput {
	return service.BusinessGoalInput{
		Name:             request.Name,
		TargetProfitYuan: request.TargetProfitYuan,
	}
}

func (server *Server) postCreateGoal(context *gin.Context) {
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 4096)
	var request businessGoalRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的目标内容")
		return
	}
	goalID, err := server.Service.CreateBusinessGoal(request.toInput())
	if err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "目标已创建", "goal_id": goalID})
}

func (server *Server) putUpdateGoal(context *gin.Context) {
	goalID, ok := parseIDParam(context, "id", "无效的目标 ID")
	if !ok {
		return
	}
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 4096)
	var request businessGoalRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的目标内容")
		return
	}
	if err := server.Service.UpdateBusinessGoal(goalID, request.toInput()); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "目标已更新"})
}

func (server *Server) postCompleteGoal(context *gin.Context) {
	goalID, ok := parseIDParam(context, "id", "无效的目标 ID")
	if !ok {
		return
	}
	if err := server.Service.CompleteBusinessGoal(goalID); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "目标已结束并归档"})
}

func (server *Server) postRefreshGoalMarket(context *gin.Context) {
	market, err := server.Service.RefreshMarketPrice()
	if err != nil {
		respondError(context, http.StatusBadGateway, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "市场行情已更新", "market": market})
}

type bulkNextPriceRequest struct {
	SubscriptionIDs []int64 `json:"subscription_ids"`
	NextPriceYuan   string  `json:"next_price_yuan"`
}

func (server *Server) postGoalBulkNextPrice(context *gin.Context) {
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 32<<10)
	var request bulkNextPriceRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的批量调价内容")
		return
	}
	updated, err := server.Service.ScheduleBulkNextPrice(service.BulkNextPriceInput{
		SubscriptionIDs: request.SubscriptionIDs,
		NextPriceYuan:   request.NextPriceYuan,
	})
	if err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{
		"message":       fmt.Sprintf("已为 %d 位 Team 用户安排下周期调价", updated),
		"updated_count": updated,
	})
}

type bulkPricingExemptionRequest struct {
	SubscriptionIDs []int64 `json:"subscription_ids"`
	ReviewCycles    int     `json:"review_cycles"`
	ReasonCode      string  `json:"reason_code"`
	Note            string  `json:"note"`
}

func (server *Server) postGoalBulkPricingExemption(context *gin.Context) {
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 32<<10)
	var request bulkPricingExemptionRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的批量调价豁免内容")
		return
	}
	updated, err := server.Service.ExemptBulkPricing(service.BulkPricingExemptionInput{
		SubscriptionIDs: request.SubscriptionIDs,
		ReviewCycles:    request.ReviewCycles,
		ReasonCode:      request.ReasonCode,
		Note:            request.Note,
	})
	if err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{
		"message":       fmt.Sprintf("已豁免 %d 位 Team 用户的本轮调价，并安排后续复评", updated),
		"updated_count": updated,
	})
}

func (server *Server) getSubscriptions(context *gin.Context) {
	views, err := server.Service.ListView()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	archived, err := server.Service.ListArchivedView()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(context, gin.H{"subscriptions": views, "archived": archived})
}

func (server *Server) getRedemptions(context *gin.Context) {
	views, err := server.Service.ListRedemptionApplicationsView(context.Query("status"))
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	pendingCount, err := server.Service.Store.CountRedemptionApplicationsByStatus(model.RedemptionStatusPending)
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(context, gin.H{"redemptions": views, "pending_count": pendingCount})
}

func (server *Server) getRedemptionCodes(context *gin.Context) {
	views, err := server.Service.ListRedemptionCodesView()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	availableCount := 0
	usedCount := 0
	disabledCount := 0
	for _, view := range views {
		switch view.Code.Status {
		case model.RedemptionCodeStatusUnused:
			availableCount++
		case model.RedemptionCodeStatusUsed:
			usedCount++
		case model.RedemptionCodeStatusDisabled:
			disabledCount++
		}
	}
	respondOK(context, gin.H{
		"codes":           views,
		"available_count": availableCount,
		"used_count":      usedCount,
		"disabled_count":  disabledCount,
	})
}

func (server *Server) getAccounts(context *gin.Context) {
	accounts, err := server.Service.ListAccountsView()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(context, gin.H{"accounts": accounts})
}

func (server *Server) getAfterSales(context *gin.Context) {
	page, err := server.Service.ListAfterSalesPage()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(context, gin.H{"cases": page.Cases, "summary": page.Summary})
}

func (server *Server) getAccountOptions(context *gin.Context) {
	includeSeatID, _ := strconv.ParseInt(context.Query("include_seat_id"), 10, 64)
	options, err := server.Service.ListAccountOptionsForForm(includeSeatID)
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(context, gin.H{"accounts": options})
}

func (server *Server) getBills(context *gin.Context) {
	page, err := server.Service.ListBillsPage()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(context, gin.H{"bills": page.Bills, "summary": page.Summary})
}

type channelSettingDTO struct {
	Key                string `json:"key"`
	Label              string `json:"label"`
	Enabled            bool   `json:"enabled"`
	Configured         bool   `json:"configured"`
	OperatorConfigured bool   `json:"operator_configured"`
}

func (server *Server) getSettings(context *gin.Context) {
	server.settingsPageMu.Lock()
	defer server.settingsPageMu.Unlock()
	configuration := server.currentConfig()

	notifyTemplate, err := server.Service.GetNotifyTemplate()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	customerTemplate, err := server.Service.GetCustomerEmailTemplate()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	enabledChannels, err := server.Service.GetEnabledChannels()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	redeemPageSettings, err := server.Service.GetRedeemPageSettings()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	enabledSet := map[string]struct{}{}
	for _, channel := range enabledChannels {
		enabledSet[channel] = struct{}{}
	}
	isEnabled := func(channel string) bool {
		_, enabled := enabledSet[channel]
		return enabled
	}
	channels := []channelSettingDTO{
		{
			Key:                model.ChannelIYUU,
			Label:              "IYUU",
			Enabled:            isEnabled(model.ChannelIYUU),
			Configured:         configuration.IYUUConfigured(),
			OperatorConfigured: configuration.IYUUConfigured(),
		},
		{
			Key:                model.ChannelSMTP,
			Label:              "SMTP",
			Enabled:            isEnabled(model.ChannelSMTP),
			Configured:         configuration.SMTPConfigured(),
			OperatorConfigured: configuration.SMTPOperatorConfigured(),
		},
		{
			Key:                model.ChannelGotify,
			Label:              "Gotify",
			Enabled:            isEnabled(model.ChannelGotify),
			Configured:         configuration.GotifyConfigured(),
			OperatorConfigured: configuration.GotifyConfigured(),
		},
	}
	respondOK(context, gin.H{
		"notify_template":         notifyTemplate,
		"customer_email_template": customerTemplate,
		"enabled_channels":        enabledChannels,
		"channels":                channels,
		"notification_config":     configuration.NotificationConfig(),
		"redeem_page":             redeemPageSettings,
	})
}

func (server *Server) getRedeemSettings(context *gin.Context) {
	redeemPageSettings, err := server.Service.GetRedeemPageSettings()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	respondOK(context, gin.H{"redeem_page": redeemPageSettings})
}

func (server *Server) getCronPreview(context *gin.Context) {
	expression := strings.TrimSpace(context.Query("cron_expr"))
	if expression == "" {
		respondError(context, http.StatusBadRequest, "请填写 cron 表达式")
		return
	}
	boardedAt := strings.TrimSpace(context.Query("boarded_at"))
	if boardedAt == "" {
		boardedAt = cycle.FormatDate(cycle.Now())
	}
	count := 5
	if rawCount := strings.TrimSpace(context.Query("count")); rawCount != "" {
		parsedCount, err := strconv.Atoi(rawCount)
		if err != nil || parsedCount < 1 || parsedCount > 20 {
			respondError(context, http.StatusBadRequest, "count 须为 1～20 的整数")
			return
		}
		count = parsedCount
	}

	schedule, err := cycle.ParseBillingSchedule(expression, boardedAt)
	if err != nil {
		// Invalid expression is an expected user state, keep HTTP 200 like the legacy API.
		context.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}

	occurrences := schedule.NextDueTimes(cycle.Now(), count)
	formattedTimes := make([]string, 0, len(occurrences))
	for _, occurrence := range occurrences {
		formattedTimes = append(formattedTimes, cycle.FormatDateTime(occurrence))
	}
	respondOK(context, gin.H{
		"description": cycle.DescribeCron(expression),
		"timezone":    "Asia/Shanghai",
		"times":       formattedTimes,
	})
}

func (server *Server) getReminderPreview(context *gin.Context) {
	subscriptionID, ok := parseIDParam(context, "id", "无效的订阅 ID")
	if !ok {
		return
	}
	to, subject, body, err := server.Service.PreviewCustomerEmail(subscriptionID)
	if err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	payload := gin.H{"to": to, "subject": subject, "body": body}
	if subscription, getErr := server.Service.Get(subscriptionID); getErr == nil && subscription.NextPriceCents != nil {
		payload["current_price_yuan"] = cycle.FormatCents(subscription.PricePerPersonCents)
		payload["next_price_yuan"] = cycle.FormatCents(*subscription.NextPriceCents)
		payload["next_price_effective_due_date"] = subscription.NextPriceEffectiveDueDate
	}
	respondOK(context, payload)
}

func (server *Server) getDuePeriods(context *gin.Context) {
	subscriptionID, ok := parseIDParam(context, "id", "无效的订阅 ID")
	if !ok {
		return
	}
	preferredStart := strings.TrimSpace(context.Query("preferred"))
	periods, err := server.Service.ListDuePeriodOptions(subscriptionID, preferredStart)
	if err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"periods": periods})
}

func (server *Server) getExport(context *gin.Context) {
	payload, err := server.Service.Export()
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		respondError(context, http.StatusInternalServerError, err.Error())
		return
	}
	filename := "carpool-export-" + cycle.Now().Format("20060102") + ".json"
	context.Header("Cache-Control", "no-store")
	context.Header("Pragma", "no-cache")
	context.Header("X-Content-Type-Options", "nosniff")
	context.Header("Content-Disposition", "attachment; filename="+filename)
	context.Data(http.StatusOK, "application/json; charset=utf-8", encoded)
}

// ---- Public redemption -------------------------------------------------------

type redemptionSubmitRequest struct {
	CustomerEmail   string `json:"customer_email"`
	CustomerContact string `json:"customer_contact"`
	RedeemCode      string `json:"redeem_code"`
	RequestNote     string `json:"request_note"`
}

func (server *Server) postRedeemApplication(context *gin.Context) {
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 4096)
	var request redemptionSubmitRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的申请内容")
		return
	}
	result, err := server.Service.SubmitRedemptionApplication(service.RedemptionSubmitInput{
		CustomerEmail:   request.CustomerEmail,
		CustomerContact: request.CustomerContact,
		RedeemCode:      request.RedeemCode,
		RequestNote:     request.RequestNote,
	})
	if err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{
		"tracking_token": result.TrackingToken,
		"status":         result.Status,
		"message":        "申请已提交，请耐心等待 1-2 分钟",
	})
}

func (server *Server) getRedeemStatus(context *gin.Context) {
	view, err := server.Service.GetRedemptionStatus(context.Param("token"))
	if err != nil {
		respondError(context, http.StatusNotFound, err.Error())
		return
	}
	respondOK(context, gin.H{"redemption": view})
}

// ---- Subscription mutations ---------------------------------------------------

type subscriptionRequest struct {
	Name           string `json:"name"`
	BusinessType   string `json:"business_type"`
	PriceYuan      string `json:"price_yuan"`
	NextPriceYuan  string `json:"next_price_yuan"`
	CostYuan       string `json:"cost_yuan"`
	CronExpr       string `json:"cron_expr"`
	NotifyOffsets  []int  `json:"notify_offsets"`
	Remark         string `json:"remark"`
	TradeURL       string `json:"trade_url"`
	CustomerEmail  string `json:"customer_email"`
	CustomerWechat string `json:"customer_wechat"`
	AccountID      int64  `json:"account_id"`
	SeatID         int64  `json:"seat_id"`
	BoardedAt      string `json:"boarded_at"`
}

func offsetsToRaw(offsets []int) string {
	parts := make([]string, 0, len(offsets))
	for _, offset := range offsets {
		parts = append(parts, strconv.Itoa(offset))
	}
	return strings.Join(parts, ",")
}

func (request subscriptionRequest) toCreateInput() service.CreateInput {
	return service.CreateInput{
		Name:             request.Name,
		BusinessType:     request.BusinessType,
		PriceYuan:        request.PriceYuan,
		NextPriceYuan:    request.NextPriceYuan,
		CostYuan:         request.CostYuan,
		CronExpr:         request.CronExpr,
		NotifyOffsetsRaw: offsetsToRaw(request.NotifyOffsets),
		Remark:           request.Remark,
		TradeURL:         request.TradeURL,
		CustomerEmail:    request.CustomerEmail,
		CustomerWechat:   request.CustomerWechat,
		AccountID:        request.AccountID,
		SeatID:           request.SeatID,
		BoardedAt:        request.BoardedAt,
	}
}

type redemptionInviteRequest struct {
	SeatID        int64  `json:"seat_id"`
	PriceYuan     string `json:"price_yuan"`
	CronExpr      string `json:"cron_expr"`
	NotifyOffsets []int  `json:"notify_offsets"`
	BoardedAt     string `json:"boarded_at"`
	Remark        string `json:"remark"`
	TradeURL      string `json:"trade_url"`
	OperatorNote  string `json:"operator_note"`
}

type redemptionCodeGenerateRequest struct {
	Count int    `json:"count"`
	Note  string `json:"note"`
}

func (request redemptionInviteRequest) toInviteInput() service.RedemptionInviteInput {
	return service.RedemptionInviteInput{
		SeatID:           request.SeatID,
		PriceYuan:        request.PriceYuan,
		CronExpr:         request.CronExpr,
		NotifyOffsetsRaw: offsetsToRaw(request.NotifyOffsets),
		BoardedAt:        request.BoardedAt,
		Remark:           request.Remark,
		TradeURL:         request.TradeURL,
		OperatorNote:     request.OperatorNote,
	}
}

func (request redemptionCodeGenerateRequest) toGenerateInput() service.RedemptionCodeGenerateInput {
	return service.RedemptionCodeGenerateInput{
		Count: request.Count,
		Note:  request.Note,
	}
}

func (server *Server) postCreateSubscription(context *gin.Context) {
	var request subscriptionRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的请求")
		return
	}
	if _, err := server.Service.CreateWithInitialBill(request.toCreateInput()); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "已创建订阅"})
}

func (server *Server) postInviteRedemption(context *gin.Context) {
	applicationID, ok := parseIDParam(context, "id", "无效的兑换申请 ID")
	if !ok {
		return
	}
	var request redemptionInviteRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的处理内容")
		return
	}
	subscriptionID, err := server.Service.InviteRedemptionApplication(applicationID, request.toInviteInput())
	if err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{
		"message":         "已邀请，并已自动创建订阅",
		"subscription_id": subscriptionID,
	})
}

func (server *Server) postGenerateRedemptionCodes(context *gin.Context) {
	var request redemptionCodeGenerateRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的生成内容")
		return
	}
	views, err := server.Service.GenerateRedemptionCodes(request.toGenerateInput())
	if err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{
		"codes":   views,
		"message": "兑换码已生成",
	})
}

func (server *Server) postDisableRedemptionCode(context *gin.Context) {
	codeID, ok := parseIDParam(context, "id", "无效的兑换码 ID")
	if !ok {
		return
	}
	if err := server.Service.DisableRedemptionCode(codeID); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "兑换码已停用"})
}

func (server *Server) postEnableRedemptionCode(context *gin.Context) {
	codeID, ok := parseIDParam(context, "id", "无效的兑换码 ID")
	if !ok {
		return
	}
	if err := server.Service.EnableRedemptionCode(codeID); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "兑换码已启用"})
}

func (server *Server) deleteRedemptionCode(context *gin.Context) {
	codeID, ok := parseIDParam(context, "id", "无效的兑换码 ID")
	if !ok {
		return
	}
	if err := server.Service.DeleteRedemptionCode(codeID); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "兑换码已删除"})
}

func (server *Server) putUpdateSubscription(context *gin.Context) {
	subscriptionID, ok := parseIDParam(context, "id", "无效的订阅 ID")
	if !ok {
		return
	}
	var request subscriptionRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的请求")
		return
	}
	if err := server.Service.Update(subscriptionID, request.toCreateInput()); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "已保存修改"})
}

func (server *Server) deleteSubscription(context *gin.Context) {
	subscriptionID, ok := parseIDParam(context, "id", "无效的订阅 ID")
	if !ok {
		return
	}
	if err := server.Service.SoftDeleteArchived(subscriptionID); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "已伪删除该已下车订阅"})
}

func (server *Server) postArchiveSubscription(context *gin.Context) {
	subscriptionID, ok := parseIDParam(context, "id", "无效的订阅 ID")
	if !ok {
		return
	}
	subscription, err := server.Service.Get(subscriptionID)
	if err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	result, err := server.Service.RequestCancellation(subscriptionID)
	if err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	message := "已进入退订售后，处理完成前不会释放车位"
	if subscription.BusinessType == model.SubscriptionBusinessPlus {
		message = "Plus 出租已进入售后处理，确认退款后将自动归档"
	}
	respondOK(context, gin.H{
		"message":          message,
		"case_id":          result.CaseID,
		"expires_at":       result.ExpiresAt,
		"expires_at_label": result.ExpiresAtLabel,
	})
}

func (server *Server) postCompleteOneMonthRental(context *gin.Context) {
	subscriptionID, ok := parseIDParam(context, "id", "无效的订阅 ID")
	if !ok {
		return
	}
	if err := server.Service.CompleteOneMonthRental(subscriptionID); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "单月短租已到期结束"})
}

func (server *Server) postCopySubscription(context *gin.Context) {
	subscriptionID, ok := parseIDParam(context, "id", "无效的订阅 ID")
	if !ok {
		return
	}
	var request struct {
		SeatID int64 `json:"seat_id"`
	}
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 4096)
	if err := context.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		respondError(context, http.StatusBadRequest, "无效的请求")
		return
	}
	if request.SeatID == 0 {
		// Prefer a free seat on the same account when the caller omits seat_id.
		source, getErr := server.Service.Store.GetSubscriptionIncludingArchived(subscriptionID)
		if getErr == nil && source.AccountID > 0 {
			freeSeats, freeErr := server.Service.Store.ListFreeSeats(source.AccountID, 0)
			if freeErr == nil && len(freeSeats) > 0 {
				request.SeatID = freeSeats[0].ID
			}
		}
	}
	if _, err := server.Service.Copy(subscriptionID, request.SeatID); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "已复制订阅，可在列表中修改备注与金额后继续使用"})
}

func (server *Server) postTestNotify(context *gin.Context) {
	if server.SandboxMode {
		respondError(context, http.StatusBadRequest, "演练模式不会发送真实通知")
		return
	}
	subscriptionID, ok := parseIDParam(context, "id", "无效的订阅 ID")
	if !ok {
		return
	}
	if err := server.Service.TestNotify(context.Request.Context(), subscriptionID); err != nil {
		respondError(context, http.StatusBadRequest, "测试发送失败: "+err.Error())
		return
	}
	respondOK(context, gin.H{"message": "测试通知已发送"})
}

func (server *Server) postSendCustomerEmail(context *gin.Context) {
	if server.SandboxMode {
		respondError(context, http.StatusBadRequest, "演练模式不会向客户发送真实邮件")
		return
	}
	subscriptionID, ok := parseIDParam(context, "id", "无效的订阅 ID")
	if !ok {
		return
	}
	if err := server.Service.SendCustomerEmail(context.Request.Context(), subscriptionID); err != nil {
		respondError(context, http.StatusBadRequest, "发送失败: "+err.Error())
		return
	}
	respondOK(context, gin.H{"message": "客户邮件已发送"})
}

func (server *Server) postDuePaid(context *gin.Context) {
	subscriptionID, ok := parseIDParam(context, "id", "无效的订阅 ID")
	if !ok {
		return
	}
	var request struct {
		Paid bool `json:"paid"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的交费状态")
		return
	}
	dueDate := context.Param("date")
	if err := server.Service.SetDuePaid(subscriptionID, dueDate, request.Paid); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}

	payload := gin.H{"paid": request.Paid, "due_date": dueDate}
	if request.Paid {
		if nextDue, nextErr := server.Service.NextUnpaidDueDate(subscriptionID, dueDate); nextErr == nil && nextDue != "" {
			payload["next_due_date"] = nextDue
		}
	}
	respondOK(context, payload)
}

// ---- Account mutations ----------------------------------------------------------

type accountRequest struct {
	Name                 string  `json:"name"`
	Remark               string  `json:"remark"`
	PaymentMethod        string  `json:"payment_method"`
	Email                string  `json:"email"`
	SpaceName            string  `json:"space_name"`
	OpenedAt             string  `json:"opened_at"`
	CostYuan             string  `json:"cost_yuan"`
	TotalCostYuan        *string `json:"total_cost_yuan"`
	ZeroRenewalNextMonth bool    `json:"zero_renewal_next_month"`
	SeatCount            int     `json:"seat_count"`
}

func (server *Server) postCreateAccount(context *gin.Context) {
	var request accountRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的请求")
		return
	}
	if _, err := server.Service.CreateAccount(service.CreateAccountInput{
		Name:                 request.Name,
		Remark:               request.Remark,
		PaymentMethod:        request.PaymentMethod,
		Email:                request.Email,
		SpaceName:            request.SpaceName,
		OpenedAt:             request.OpenedAt,
		CostYuan:             request.CostYuan,
		TotalCostYuan:        request.TotalCostYuan,
		ZeroRenewalNextMonth: request.ZeroRenewalNextMonth,
		SeatCount:            request.SeatCount,
	}); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "已创建账号"})
}

func (server *Server) putUpdateAccount(context *gin.Context) {
	accountID, ok := parseIDParam(context, "id", "无效的账号 ID")
	if !ok {
		return
	}
	var request accountRequest
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的请求")
		return
	}
	if err := server.Service.UpdateAccount(accountID, service.UpdateAccountInput{
		Name:                 request.Name,
		Remark:               request.Remark,
		PaymentMethod:        request.PaymentMethod,
		Email:                request.Email,
		SpaceName:            request.SpaceName,
		OpenedAt:             request.OpenedAt,
		CostYuan:             request.CostYuan,
		TotalCostYuan:        request.TotalCostYuan,
		ZeroRenewalNextMonth: request.ZeroRenewalNextMonth,
		SeatCount:            request.SeatCount,
	}); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "账号已保存"})
}

func (server *Server) postBanAccount(context *gin.Context) {
	accountID, ok := parseIDParam(context, "id", "无效的账号 ID")
	if !ok {
		return
	}
	var request struct {
		BannedDate string `json:"banned_date"`
		Note       string `json:"note"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的请求")
		return
	}
	created, err := server.Service.BanAccount(accountID, service.BanAccountInput{
		BannedDate: request.BannedDate,
		Note:       request.Note,
	})
	if err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{
		"message":       fmt.Sprintf("账号已封禁，已生成 %d 条售后记录", created),
		"created_count": created,
	})
}

func (server *Server) deleteAccount(context *gin.Context) {
	accountID, ok := parseIDParam(context, "id", "无效的账号 ID")
	if !ok {
		return
	}
	if err := server.Service.DeleteAccount(accountID); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "账号已删除"})
}

// ---- After-sales mutations -----------------------------------------------------

func (server *Server) putAfterSalesCase(context *gin.Context) {
	caseID, ok := parseIDParam(context, "id", "无效的售后记录 ID")
	if !ok {
		return
	}
	var request struct {
		RefundAmountYuan string `json:"refund_amount_yuan"`
		Note             string `json:"note"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的请求")
		return
	}
	if err := server.Service.UpdateAfterSalesCase(caseID, service.UpdateAfterSalesCaseInput{
		RefundAmountYuan: request.RefundAmountYuan,
		Note:             request.Note,
	}); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "售后记录已保存"})
}

func (server *Server) postAfterSalesRefunded(context *gin.Context) {
	caseID, ok := parseIDParam(context, "id", "无效的售后记录 ID")
	if !ok {
		return
	}
	var request struct {
		Refunded bool `json:"refunded"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的请求")
		return
	}
	if err := server.Service.SetAfterSalesCaseRefunded(caseID, request.Refunded); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	message := "已恢复为待退款"
	if request.Refunded {
		message = "已标记为退款完成"
	}
	respondOK(context, gin.H{"message": message})
}

func (server *Server) postAfterSalesReassign(context *gin.Context) {
	caseID, ok := parseIDParam(context, "id", "无效的售后记录 ID")
	if !ok {
		return
	}
	var request struct {
		AccountID int64 `json:"account_id"`
		SeatID    int64 `json:"seat_id"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的请求")
		return
	}
	if err := server.Service.ReassignAfterSalesCase(caseID, service.ReassignAfterSalesCaseInput{
		AccountID: request.AccountID,
		SeatID:    request.SeatID,
	}); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "客户已安排到新的空间"})
}

// ---- Bill mutations ------------------------------------------------------------

func (server *Server) putUpdateBill(context *gin.Context) {
	billID, ok := parseIDParam(context, "id", "无效的账单 ID")
	if !ok {
		return
	}
	var request struct {
		AmountYuan string `json:"amount_yuan"`
		Note       string `json:"note"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的请求")
		return
	}
	if err := server.Service.UpdateBill(billID, service.BillEditInput{
		AmountYuan: request.AmountYuan,
		Note:       request.Note,
	}); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "账单已保存"})
}

func (server *Server) deleteBill(context *gin.Context) {
	billID, ok := parseIDParam(context, "id", "无效的账单 ID")
	if !ok {
		return
	}
	if err := server.Service.DeleteBill(billID); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{"message": "账单已删除，该期恢复为未交费"})
}

// ---- Settings mutations ----------------------------------------------------------

func (server *Server) putSettings(context *gin.Context) {
	context.Request.Body = http.MaxBytesReader(context.Writer, context.Request.Body, 2<<20)
	var request struct {
		NotifyTemplate        string                          `json:"notify_template"`
		CustomerEmailTemplate string                          `json:"customer_email_template"`
		Channels              []string                        `json:"channels"`
		NotificationConfig    *config.NotificationConfigInput `json:"notification_config"`
		RedeemPage            *model.RedeemPageSettings       `json:"redeem_page"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的请求")
		return
	}
	server.settingsPageMu.Lock()
	defer server.settingsPageMu.Unlock()
	if err := server.Service.ValidateSettingsPage(
		request.NotifyTemplate,
		request.CustomerEmailTemplate,
		request.RedeemPage,
	); err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	if request.NotificationConfig != nil {
		if err := config.ValidateNotificationConfig(*request.NotificationConfig); err != nil {
			respondError(context, http.StatusBadRequest, err.Error())
			return
		}
	}
	var updatedConfig *config.Config
	var rollbackConfig func() error
	if request.NotificationConfig != nil {
		configuration := server.currentConfig()
		updated, rollback, err := config.UpdateNotificationConfigWithRollback(
			configuration.ConfigPath,
			*request.NotificationConfig,
		)
		if err != nil {
			respondError(context, http.StatusBadRequest, err.Error())
			return
		}
		updatedConfig = &updated
		rollbackConfig = rollback
	}
	if err := server.Service.SaveSettingsPage(
		request.NotifyTemplate,
		request.CustomerEmailTemplate,
		request.Channels,
		request.RedeemPage,
	); err != nil {
		if rollbackConfig != nil {
			if rollbackErr := rollbackConfig(); rollbackErr != nil {
				respondError(
					context,
					http.StatusInternalServerError,
					fmt.Sprintf("设置保存失败，并且通知配置回滚失败: %v", rollbackErr),
				)
				return
			}
		}
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	if updatedConfig != nil {
		server.applyConfig(*updatedConfig)
	}
	respondOK(context, gin.H{"message": "设置已保存"})
}

func (server *Server) postSettingsTestNotify(context *gin.Context) {
	if server.SandboxMode {
		respondError(context, http.StatusBadRequest, "演练模式不会发送真实通知")
		return
	}
	if err := server.Service.TestEnabledChannels(context.Request.Context()); err != nil {
		respondError(context, http.StatusBadRequest, "测试发送失败: "+err.Error())
		return
	}
	respondOK(context, gin.H{"message": "测试通知已发送"})
}

func (server *Server) postSettingsTemplatePreview(context *gin.Context) {
	var request struct {
		Kind     string `json:"kind"`
		Template string `json:"template"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		respondError(context, http.StatusBadRequest, "无效的请求")
		return
	}
	if strings.TrimSpace(request.Template) == "" {
		respondError(context, http.StatusBadRequest, "模板不能为空")
		return
	}
	name := "notify"
	if request.Kind == "customer" {
		name = "customer_email"
	}
	rendered, sampleName, err := server.Service.PreviewTemplate(name, request.Template)
	if err != nil {
		respondError(context, http.StatusBadRequest, err.Error())
		return
	}
	respondOK(context, gin.H{
		"rendered":    rendered,
		"sample_name": sampleName,
		"subject":     "拼车续费提醒 · " + sampleName,
	})
}

// ---- Helpers --------------------------------------------------------------------

func parseIDParam(context *gin.Context, name string, errorMessage string) (int64, bool) {
	value, err := strconv.ParseInt(context.Param(name), 10, 64)
	if err != nil || value <= 0 {
		respondError(context, http.StatusBadRequest, errorMessage)
		return 0, false
	}
	return value, true
}
