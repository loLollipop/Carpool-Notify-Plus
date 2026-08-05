package service

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

const (
	defaultRedemptionCronExpr = "interval:30d"
	defaultRedemptionOffsets  = "3"

	maxRedemptionEmailLength        = 254
	maxRedemptionContactLength      = 80
	maxRedemptionCodeLength         = 120
	maxRedemptionCodeNoteLength     = 200
	maxRedemptionRequestNoteLength  = 500
	maxRedemptionOperatorNoteLength = 500
	maxRedemptionRemarkLength       = 500
	maxGenerateRedemptionCodeCount  = 20

	redemptionCodeAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

// RedemptionSubmitInput is the public customer form.
type RedemptionSubmitInput struct {
	CustomerEmail   string
	CustomerContact string
	RedeemCode      string
	RequestNote     string
}

// RedemptionSubmitResult returns the private browser-side tracking token.
type RedemptionSubmitResult struct {
	TrackingToken string `json:"tracking_token"`
	Status        string `json:"status"`
}

// RedemptionCodeGenerateInput is an operator request to mint one-time codes.
type RedemptionCodeGenerateInput struct {
	Count int
	Note  string
}

// RedemptionCodeView is the operator-facing presentation for a generated code.
type RedemptionCodeView struct {
	Code             model.RedemptionCode `json:"code"`
	StatusLabel      string               `json:"status_label"`
	CreatedAtLabel   string               `json:"created_at_label"`
	UsedAtLabel      string               `json:"used_at_label"`
	ApplicationEmail string               `json:"application_email"`
}

// RedemptionStatusView is safe for the unauthenticated public status page.
type RedemptionStatusView struct {
	Status         string `json:"status"`
	CustomerEmail  string `json:"customer_email"`
	CreatedAtLabel string `json:"created_at_label"`
	InvitedAtLabel string `json:"invited_at_label"`
}

// RedemptionApplicationView is the operator-facing row.
type RedemptionApplicationView struct {
	Application      model.RedemptionApplication `json:"application"`
	CreatedAtLabel   string                      `json:"created_at_label"`
	InvitedAtLabel   string                      `json:"invited_at_label"`
	AccountName      string                      `json:"account_name"`
	AccountEmail     string                      `json:"account_email"`
	AccountSpace     string                      `json:"account_space_name"`
	SeatName         string                      `json:"seat_name"`
	SubscriptionName string                      `json:"subscription_name"`
}

// RedemptionInviteInput creates the real subscription when the operator has
// invited the customer in OpenAI.
type RedemptionInviteInput struct {
	SeatID           int64
	PriceYuan        string
	IsResale         bool
	AgencyFeeYuan    string
	CronExpr         string
	NotifyOffsetsRaw string
	BoardedAt        string
	Remark           string
	TradeURL         string
	OperatorNote     string
}

// SubmitRedemptionApplication stores one public customer redemption request.
func (service *SubscriptionService) SubmitRedemptionApplication(input RedemptionSubmitInput) (RedemptionSubmitResult, error) {
	customerEmail, err := normalizeCustomerEmail(input.CustomerEmail)
	if err != nil {
		return RedemptionSubmitResult{}, err
	}
	if customerEmail == "" {
		return RedemptionSubmitResult{}, fmt.Errorf("请填写客户邮箱")
	}
	if len([]rune(customerEmail)) > maxRedemptionEmailLength {
		return RedemptionSubmitResult{}, fmt.Errorf("客户邮箱最多 %d 个字", maxRedemptionEmailLength)
	}
	customerContact, err := trimRequiredLimited("微信号 / QQ号", input.CustomerContact, maxRedemptionContactLength)
	if err != nil {
		return RedemptionSubmitResult{}, err
	}
	redeemCode, err := normalizeRedemptionCode(input.RedeemCode)
	if err != nil {
		return RedemptionSubmitResult{}, err
	}
	requestNote, err := trimLimited("备注", input.RequestNote, maxRedemptionRequestNoteLength)
	if err != nil {
		return RedemptionSubmitResult{}, err
	}

	for attempt := 0; attempt < 5; attempt++ {
		token, err := newRedemptionTrackingToken()
		if err != nil {
			return RedemptionSubmitResult{}, err
		}
		_, err = service.Store.CreateRedemptionApplicationUsingCode(model.RedemptionApplication{
			TrackingToken:   token,
			CustomerEmail:   customerEmail,
			CustomerContact: customerContact,
			RedeemCode:      redeemCode,
			RequestNote:     requestNote,
		})
		if err == nil {
			return RedemptionSubmitResult{
				TrackingToken: token,
				Status:        model.RedemptionStatusPending,
			}, nil
		}
		if redemptionCodeError := publicRedemptionCodeError(err); redemptionCodeError != nil {
			return RedemptionSubmitResult{}, redemptionCodeError
		}
		if !strings.Contains(strings.ToLower(err.Error()), "tracking_token") {
			return RedemptionSubmitResult{}, err
		}
	}
	return RedemptionSubmitResult{}, fmt.Errorf("生成申请编号失败，请重试")
}

// GenerateRedemptionCodes mints operator-managed one-time customer codes.
func (service *SubscriptionService) GenerateRedemptionCodes(input RedemptionCodeGenerateInput) ([]RedemptionCodeView, error) {
	count := input.Count
	if count == 0 {
		count = 1
	}
	if count < 1 || count > maxGenerateRedemptionCodeCount {
		return nil, fmt.Errorf("生成数量需为 1～%d", maxGenerateRedemptionCodeCount)
	}
	note, err := trimLimited("兑换码备注", input.Note, maxRedemptionCodeNoteLength)
	if err != nil {
		return nil, err
	}

	views := make([]RedemptionCodeView, 0, count)
	for index := 0; index < count; index++ {
		var created model.RedemptionCode
		for attempt := 0; attempt < 8; attempt++ {
			code, err := newRedemptionCode()
			if err != nil {
				return nil, err
			}
			codeID, err := service.Store.CreateRedemptionCode(model.RedemptionCode{
				Code: code,
				Note: note,
			})
			if err == nil {
				created, err = service.Store.GetRedemptionCode(codeID)
				if err != nil {
					return nil, err
				}
				break
			}
			if !strings.Contains(strings.ToLower(err.Error()), "unique") {
				return nil, err
			}
		}
		if created.ID == 0 {
			return nil, fmt.Errorf("生成兑换码失败，请重试")
		}
		view, err := service.buildRedemptionCodeView(created)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// ListRedemptionCodesView returns operator-generated redemption codes.
func (service *SubscriptionService) ListRedemptionCodesView() ([]RedemptionCodeView, error) {
	codes, err := service.Store.ListRedemptionCodes()
	if err != nil {
		return nil, err
	}
	views := make([]RedemptionCodeView, 0, len(codes))
	for _, code := range codes {
		view, err := service.buildRedemptionCodeView(code)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// DisableRedemptionCode prevents an unused code from being submitted.
func (service *SubscriptionService) DisableRedemptionCode(codeID int64) error {
	code, err := service.Store.GetRedemptionCode(codeID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("兑换码不存在")
		}
		return err
	}
	if code.Status == model.RedemptionCodeStatusUsed {
		return fmt.Errorf("已使用的兑换码不能停用")
	}
	if code.Status == model.RedemptionCodeStatusDisabled {
		return nil
	}
	return service.Store.SetRedemptionCodeStatus(code.ID, model.RedemptionCodeStatusDisabled)
}

// EnableRedemptionCode makes a disabled code usable again.
func (service *SubscriptionService) EnableRedemptionCode(codeID int64) error {
	code, err := service.Store.GetRedemptionCode(codeID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("兑换码不存在")
		}
		return err
	}
	if code.Status == model.RedemptionCodeStatusUsed {
		return fmt.Errorf("已使用的兑换码不能重新启用")
	}
	if code.Status == model.RedemptionCodeStatusUnused {
		return nil
	}
	return service.Store.SetRedemptionCodeStatus(code.ID, model.RedemptionCodeStatusUnused)
}

// DeleteRedemptionCode removes an unused or disabled code from the operator list.
func (service *SubscriptionService) DeleteRedemptionCode(codeID int64) error {
	code, err := service.Store.GetRedemptionCode(codeID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("兑换码不存在")
		}
		return err
	}
	if code.Status == model.RedemptionCodeStatusUsed {
		return fmt.Errorf("已使用的兑换码关联了客户申请，不能删除")
	}
	return service.Store.DeleteRedemptionCode(code.ID)
}

// GetRedemptionStatus returns the current public status for a tracking token.
func (service *SubscriptionService) GetRedemptionStatus(token string) (RedemptionStatusView, error) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 128 {
		return RedemptionStatusView{}, fmt.Errorf("申请编号无效")
	}
	application, err := service.Store.GetRedemptionApplicationByToken(token)
	if err != nil {
		if err == sql.ErrNoRows {
			return RedemptionStatusView{}, fmt.Errorf("没有找到这条申请")
		}
		return RedemptionStatusView{}, err
	}
	return service.buildRedemptionStatusView(application), nil
}

// ListRedemptionApplicationsView returns operator-facing redemption requests.
func (service *SubscriptionService) ListRedemptionApplicationsView(status string) ([]RedemptionApplicationView, error) {
	applications, err := service.Store.ListRedemptionApplications(strings.TrimSpace(status))
	if err != nil {
		return nil, err
	}
	views := make([]RedemptionApplicationView, 0, len(applications))
	for _, application := range applications {
		view, err := service.buildRedemptionApplicationView(application)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// InviteRedemptionApplication creates the subscription and marks the application invited.
func (service *SubscriptionService) InviteRedemptionApplication(applicationID int64, input RedemptionInviteInput) (int64, error) {
	application, err := service.Store.GetRedemptionApplication(applicationID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("兑换申请不存在")
		}
		return 0, err
	}
	if application.Status != model.RedemptionStatusPending {
		return 0, fmt.Errorf("这条兑换申请已经处理过")
	}
	if input.SeatID <= 0 {
		return 0, fmt.Errorf("请选择要分配的车位")
	}
	seat, err := service.Store.GetSeat(input.SeatID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("车位不存在")
		}
		return 0, err
	}

	operatorNote, err := trimLimited("处理备注", input.OperatorNote, maxRedemptionOperatorNoteLength)
	if err != nil {
		return 0, err
	}
	operatorRemark, err := trimLimited("订阅备注", input.Remark, maxRedemptionRemarkLength)
	if err != nil {
		return 0, err
	}

	cronExpr := strings.TrimSpace(input.CronExpr)
	if cronExpr == "" {
		cronExpr = defaultRedemptionCronExpr
	}
	notifyOffsets := strings.TrimSpace(input.NotifyOffsetsRaw)
	if notifyOffsets == "" {
		notifyOffsets = defaultRedemptionOffsets
	}
	boardedAt := strings.TrimSpace(input.BoardedAt)
	if boardedAt == "" {
		boardedAt = cycle.FormatDate(service.now())
	}

	subscriptionID, err := service.CreateWithInitialBill(CreateInput{
		Name:             application.CustomerEmail,
		PriceYuan:        strings.TrimSpace(input.PriceYuan),
		IsResale:         input.IsResale,
		AgencyFeeYuan:    strings.TrimSpace(input.AgencyFeeYuan),
		CronExpr:         cronExpr,
		NotifyOffsetsRaw: notifyOffsets,
		Remark:           redemptionSubscriptionRemark(application, operatorRemark),
		TradeURL:         strings.TrimSpace(input.TradeURL),
		CustomerEmail:    application.CustomerEmail,
		CustomerWechat:   application.CustomerContact,
		AccountID:        seat.AccountID,
		SeatID:           seat.ID,
		BoardedAt:        boardedAt,
	})
	if err != nil {
		return 0, err
	}

	if err := service.Store.MarkRedemptionApplicationInvited(
		application.ID,
		seat.AccountID,
		seat.ID,
		subscriptionID,
		operatorNote,
	); err != nil {
		_ = service.Store.SoftDeleteSubscription(subscriptionID)
		if err == sql.ErrNoRows {
			return 0, fmt.Errorf("兑换申请状态已变化，请刷新后再处理")
		}
		return 0, err
	}
	return subscriptionID, nil
}

func (service *SubscriptionService) buildRedemptionStatusView(application model.RedemptionApplication) RedemptionStatusView {
	return RedemptionStatusView{
		Status:         application.Status,
		CustomerEmail:  application.CustomerEmail,
		CreatedAtLabel: cycle.FormatDateTime(application.CreatedAt),
		InvitedAtLabel: formatOptionalTime(application.InvitedAt),
	}
}

func (service *SubscriptionService) buildRedemptionApplicationView(application model.RedemptionApplication) (RedemptionApplicationView, error) {
	view := RedemptionApplicationView{
		Application:    application,
		CreatedAtLabel: cycle.FormatDateTime(application.CreatedAt),
		InvitedAtLabel: formatOptionalTime(application.InvitedAt),
	}
	if application.AssignedAccountID > 0 {
		account, err := service.Store.GetAccount(application.AssignedAccountID)
		if err != nil && err != sql.ErrNoRows {
			return RedemptionApplicationView{}, err
		}
		if err == nil {
			view.AccountName = account.Name
			view.AccountEmail = account.Email
			view.AccountSpace = account.SpaceName
		}
	}
	if application.AssignedSeatID > 0 {
		seat, err := service.Store.GetSeat(application.AssignedSeatID)
		if err != nil && err != sql.ErrNoRows {
			return RedemptionApplicationView{}, err
		}
		if err == nil {
			view.SeatName = seat.Name
		}
	}
	if application.AssignedSubscriptionID > 0 {
		subscription, err := service.Store.GetSubscriptionIncludingArchived(application.AssignedSubscriptionID)
		if err != nil && err != sql.ErrNoRows {
			return RedemptionApplicationView{}, err
		}
		if err == nil {
			view.SubscriptionName = subscription.Name
		}
	}
	return view, nil
}

func (service *SubscriptionService) buildRedemptionCodeView(code model.RedemptionCode) (RedemptionCodeView, error) {
	view := RedemptionCodeView{
		Code:           code,
		StatusLabel:    redemptionCodeStatusLabel(code.Status),
		CreatedAtLabel: cycle.FormatDateTime(code.CreatedAt),
		UsedAtLabel:    formatOptionalTime(code.UsedAt),
	}
	if code.UsedByApplicationID > 0 {
		application, err := service.Store.GetRedemptionApplication(code.UsedByApplicationID)
		if err != nil && err != sql.ErrNoRows {
			return RedemptionCodeView{}, err
		}
		if err == nil {
			view.ApplicationEmail = application.CustomerEmail
		}
	}
	return view, nil
}

func newRedemptionTrackingToken() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func newRedemptionCode() (string, error) {
	parts := make([]string, 0, 3)
	for segment := 0; segment < 3; segment++ {
		value, err := randomRedemptionCodeSegment(4)
		if err != nil {
			return "", err
		}
		parts = append(parts, value)
	}
	return "CPN-" + strings.Join(parts, "-"), nil
}

func randomRedemptionCodeSegment(length int) (string, error) {
	var builder strings.Builder
	builder.Grow(length)
	limit := big.NewInt(int64(len(redemptionCodeAlphabet)))
	for builder.Len() < length {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		builder.WriteByte(redemptionCodeAlphabet[index.Int64()])
	}
	return builder.String(), nil
}

func normalizeRedemptionCode(raw string) (string, error) {
	value, err := trimRequiredLimited("兑换码", raw, maxRedemptionCodeLength)
	if err != nil {
		return "", err
	}
	value = strings.ToUpper(strings.ReplaceAll(value, " ", ""))
	value = strings.ReplaceAll(value, "－", "-")
	value = strings.ReplaceAll(value, "—", "-")
	value = strings.ReplaceAll(value, "–", "-")
	return value, nil
}

func publicRedemptionCodeError(err error) error {
	switch {
	case errors.Is(err, db.ErrRedemptionCodeNotFound):
		return fmt.Errorf("兑换码不存在或填写错误")
	case errors.Is(err, db.ErrRedemptionCodeUsed):
		return fmt.Errorf("这个兑换码已经被使用")
	case errors.Is(err, db.ErrRedemptionCodeDisabled):
		return fmt.Errorf("这个兑换码已停用，请联系管理员")
	case errors.Is(err, db.ErrRedemptionCodeNotUnused):
		return fmt.Errorf("兑换码暂时不可用，请稍后重试")
	default:
		return nil
	}
}

func redemptionCodeStatusLabel(status string) string {
	switch status {
	case model.RedemptionCodeStatusUnused:
		return "可用"
	case model.RedemptionCodeStatusUsed:
		return "已使用"
	case model.RedemptionCodeStatusDisabled:
		return "已停用"
	default:
		return "未知"
	}
}

func trimRequiredLimited(label string, raw string, maxLength int) (string, error) {
	value, err := trimLimited(label, raw, maxLength)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("请填写%s", label)
	}
	return value, nil
}

func trimLimited(label string, raw string, maxLength int) (string, error) {
	value := strings.TrimSpace(raw)
	if len([]rune(value)) > maxLength {
		return "", fmt.Errorf("%s最多 %d 个字", label, maxLength)
	}
	return value, nil
}

func redemptionSubscriptionRemark(application model.RedemptionApplication, operatorRemark string) string {
	parts := make([]string, 0, 3)
	if operatorRemark != "" {
		parts = append(parts, operatorRemark)
	}
	if application.RedeemCode != "" {
		parts = append(parts, "兑换码："+application.RedeemCode)
	}
	if application.RequestNote != "" {
		parts = append(parts, "申请备注："+application.RequestNote)
	}
	return strings.Join(parts, "\n")
}

func formatOptionalTime(moment *time.Time) string {
	if moment == nil {
		return ""
	}
	return cycle.FormatDateTime(*moment)
}
