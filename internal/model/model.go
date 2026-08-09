package model

import "time"

const (
	ChannelGotify = "gotify"
	ChannelIYUU   = "iyuu"
	ChannelSMTP   = "smtp"

	NotificationStatusPending = "pending"
	NotificationStatusSuccess = "success"
	NotificationStatusFailed  = "failed"

	NotificationKindScheduled = "scheduled"
	NotificationKindTest      = "test"

	RedemptionStatusPending  = "pending"
	RedemptionStatusInvited  = "invited"
	RedemptionStatusRejected = "rejected"

	RedemptionCodeStatusUnused   = "unused"
	RedemptionCodeStatusUsed     = "used"
	RedemptionCodeStatusDisabled = "disabled"

	SettingNotifyTemplate        = "notify_template"
	SettingCustomerEmailTemplate = "customer_email_template"
	SettingEnabledChannels       = "enabled_channels"
	SettingRedeemPageSettings    = "redeem_page_settings"

	// SubscriptionTypeOther is a legacy default for the deprecated subscription_type column.
	// New code uses AccountName for display; this remains for migration/export compatibility.
	SubscriptionTypeOther = "其它"

	// UnclassifiedAccountName is the account name used when migrating legacy "其它" types.
	UnclassifiedAccountName = "未分类"

	// MaxAccountNameLength is the max rune count for an account name.
	MaxAccountNameLength = 120
	// MaxSeatNameLength is the max rune count for a seat name.
	MaxSeatNameLength = 40
	// MinInitialSeatCount / MaxInitialSeatCount bound how many seats can be created with a new account.
	MinInitialSeatCount = 1
	MaxInitialSeatCount = 1000
	// MaxSubscriptionTypeLength is kept for legacy column length during migration.
	MaxSubscriptionTypeLength = 40
)

// DefaultEnabledChannels keeps new open-source installs from using unconfigured private channels.
var DefaultEnabledChannels = []string{}

// DefaultNotifyTemplate is the initial global operator message template.
const DefaultNotifyTemplate = `【到期提醒】{{.CustomerEmail}}
到期状态：{{.DueInText}}
本期应收：¥{{.AmountDue}}
计费周期：{{.CycleDesc}}
到期日期：{{.NextDueDate}}
{{if .CustomerWechat}}客户微信：{{.CustomerWechat}}{{end}}
{{if .Remark}}备注：{{.Remark}}{{end}}
{{if .TradeURL}}链接：{{.TradeURL}}{{end}}`

// DefaultCustomerEmailTemplate is the initial template for emails to customers.
const DefaultCustomerEmailTemplate = `您好，您的 ChatGPT Team 拼车服务{{.DueInText}}，请留意续费时间。

客户邮箱：{{.CustomerEmail}}
本期应收：¥{{.AmountDue}}
计费周期：{{.CycleDesc}}
到期日期：{{.NextDueDate}}（{{.DueInText}}）
{{if .Remark}}备注：{{.Remark}}{{end}}
{{if .TradeURL}}续费链接：{{.TradeURL}}{{end}}

为避免到期后影响使用，请在到期前完成续费。
如需续费或售后协助，请联系管理员。`

// RedeemPageSettings is public, non-secret copy shown on the customer redemption page.
type RedeemPageSettings struct {
	AnnouncementTitle    string   `json:"announcement_title"`
	AnnouncementIntro    string   `json:"announcement_intro"`
	AnnouncementItems    []string `json:"announcement_items"`
	SupportTitle         string   `json:"support_title"`
	SupportDescription   string   `json:"support_description"`
	SupportContactLabel  string   `json:"support_contact_label"`
	SupportWechatID      string   `json:"support_wechat_id"`
	SupportQRCodeDataURL string   `json:"support_qr_data_url"`
}

// DefaultRedeemPageSettings keeps open-source installs free of operator-specific data.
var DefaultRedeemPageSettings = RedeemPageSettings{
	AnnouncementTitle: "加入 ChatGPT Team 前请先确认",
	AnnouncementIntro: "为保护工作空间中的内容与到期后的使用连续性，请先阅读以下说明。",
	AnnouncementItems: []string{
		"工作空间与个人空间的记录相互独立，请及时备份工作空间中的重要对话、文件和资料。",
		"长期使用建议添加管理员微信，方便接收续费提醒、售后协助和异常通知。",
		"到期后若未及时续费，账号可能会被移出 Team；未备份的工作空间内容可能无法找回。",
	},
	SupportTitle:         "客服微信",
	SupportDescription:   "续费提醒与售后协助",
	SupportContactLabel:  "微信号",
	SupportWechatID:      "",
	SupportQRCodeDataURL: "",
}

// Account is a carpool identity (e.g. a ChatGPT Team owner account) that owns seats.
type Account struct {
	ID                   int64     `json:"id"`
	Name                 string    `json:"name"`
	Remark               string    `json:"remark"`
	PaymentMethod        string    `json:"payment_method"`
	Email                string    `json:"email"`
	SpaceName            string    `json:"space_name"`
	OpenedAt             string    `json:"opened_at"`
	CostCents            int64     `json:"cost_cents"`
	TotalCostCents       int64     `json:"total_cost_cents"`
	ZeroRenewalNextMonth bool      `json:"zero_renewal_next_month"`
	BannedAt             string    `json:"banned_at"`
	BanNote              string    `json:"ban_note"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

const (
	AccountCostSourceInitial     = "initial"
	AccountCostSourceRenewal     = "renewal"
	AccountCostSourceZeroRenewal = "zero_renewal"
	AccountCostSourceManual      = "manual"
)

// AccountCostRecord is one immutable owner-account cost ledger entry.
type AccountCostRecord struct {
	ID          int64     `json:"id"`
	AccountID   int64     `json:"account_id"`
	PeriodDate  string    `json:"period_date"`
	AmountCents int64     `json:"amount_cents"`
	Source      string    `json:"source"`
	Note        string    `json:"note"`
	CreatedAt   time.Time `json:"created_at"`
}

// Seat is a named parking slot under an account. At most one active subscription may occupy a seat.
type Seat struct {
	ID        int64     `json:"id"`
	AccountID int64     `json:"account_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Subscription is a carpool collect-payment reminder record.
type Subscription struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	PricePerPersonCents int64  `json:"price_per_person_cents"`
	// CostCents is the per-seat cost price in integer cents (0 if unset).
	// For normal seats, profit = PricePerPersonCents - CostCents.
	// For resale seats, dashboard totals ignore price/cost and use AgencyFeeCents only.
	CostCents int64 `json:"cost_cents"`
	// IsResale marks a seat arranged for a friend (串货): list price and cost
	// are excluded from dashboard totals; AgencyFeeCents is counted instead.
	IsResale bool `json:"is_resale"`
	// AgencyFeeCents is the middleman fee in integer cents (may be 0).
	// Used as the default bill amount when IsResale is true.
	AgencyFeeCents int64  `json:"agency_fee_cents"`
	CronExpr       string `json:"cron_expr"`
	NotifyOffsets  []int  `json:"notify_offsets"`
	// Channels is a legacy per-subscription field kept for DB/export compatibility.
	// Runtime send/display uses the global settings enabled_channels instead.
	Channels       []string `json:"channels"`
	Remark         string   `json:"remark"`
	TradeURL       string   `json:"trade_url"`
	CustomerEmail  string   `json:"customer_email"`
	CustomerWechat string   `json:"customer_wechat"`
	// SeatID links this subscription to a named seat under an account.
	// Active subscriptions must occupy a free seat; archive releases the seat.
	SeatID int64 `json:"seat_id"`
	// AccountID / AccountName / SeatName are joined presentation fields (not always stored).
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name"`
	SeatName    string `json:"seat_name"`
	// SubscriptionType is a legacy classification label kept for migration/export.
	// Display and stats use AccountName instead.
	SubscriptionType string `json:"subscription_type"`
	// BoardedAt is the 上车日期 (Asia/Shanghai calendar day, YYYY-MM-DD).
	// Due occurrences and reminders before this date are ignored.
	BoardedAt string `json:"boarded_at"`
	// ArchivedAt is set when the user gets off (下车); archived subscriptions
	// leave the active list and scheduler but keep bills linked.
	ArchivedAt              *time.Time `json:"archived_at"`
	CancellationRequestedAt *time.Time `json:"cancellation_requested_at"`
	CancellationExpiresAt   *time.Time `json:"cancellation_expires_at"`
	CancellationCaseID      int64      `json:"cancellation_case_id"`
	DeletedAt               *time.Time `json:"deleted_at"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

// Bill is one paid occurrence for a subscription due date.
// Presence of a bill means that (subscription_id, due_date) is paid.
type Bill struct {
	ID             int64
	SubscriptionID int64
	DueDate        string
	AmountCents    int64
	Note           string
	PaidAt         time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

const (
	AfterSalesStatusPending    = "pending"
	AfterSalesStatusReview     = "review"
	AfterSalesStatusRefunded   = "refunded"
	AfterSalesStatusReassigned = "reassigned"

	AfterSalesSourceAccountBan           = "account_ban"
	AfterSalesSourceCustomerCancellation = "customer_cancellation"
)

// AfterSalesCase is an after-sales snapshot created when an owner account is banned.
// Contact, pricing, and replacement fields preserve the original handling history.
type AfterSalesCase struct {
	ID                      int64      `json:"id"`
	AccountID               int64      `json:"account_id"`
	SubscriptionID          int64      `json:"subscription_id"`
	BillID                  int64      `json:"bill_id"`
	AccountName             string     `json:"account_name"`
	AccountEmail            string     `json:"account_email"`
	AccountSpaceName        string     `json:"account_space_name"`
	CustomerEmail           string     `json:"customer_email"`
	CustomerWechat          string     `json:"customer_wechat"`
	PeriodStart             string     `json:"period_start"`
	PeriodEnd               string     `json:"period_end"`
	BannedDate              string     `json:"banned_date"`
	WarrantyDays            int        `json:"warranty_days"`
	UsedDays                int        `json:"used_days"`
	RemainingDays           int        `json:"remaining_days"`
	PaidAmountCents         int64      `json:"paid_amount_cents"`
	RefundAmountCents       int64      `json:"refund_amount_cents"`
	ReplacementAccountID    int64      `json:"replacement_account_id"`
	ReplacementSeatID       int64      `json:"replacement_seat_id"`
	ReplacementAccountName  string     `json:"replacement_account_name"`
	ReplacementAccountEmail string     `json:"replacement_account_email"`
	ReplacementSpaceName    string     `json:"replacement_space_name"`
	ReplacementSeatName     string     `json:"replacement_seat_name"`
	Source                  string     `json:"source"`
	ExpiresAt               *time.Time `json:"expires_at"`
	Status                  string     `json:"status"`
	Note                    string     `json:"note"`
	ProcessedAt             *time.Time `json:"processed_at"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

// PaidDueOccurrence records that one subscription due date has been paid.
// Kept for calendar paid-status maps; backed by bills rows.
type PaidDueOccurrence struct {
	SubscriptionID int64
	DueDate        string
}

// NotificationLog tracks a per-channel send attempt for a due/offset pair.
type NotificationLog struct {
	ID             int64
	SubscriptionID int64
	DueDate        string
	OffsetDays     int
	Channel        string
	Status         string
	AttemptCount   int
	NextRetryAt    *time.Time
	LastError      string
	Kind           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// RedemptionApplication is a customer-submitted request to redeem a paid code
// and join one assigned account/seat.
type RedemptionApplication struct {
	ID                     int64      `json:"id"`
	TrackingToken          string     `json:"tracking_token"`
	CustomerEmail          string     `json:"customer_email"`
	CustomerContact        string     `json:"customer_contact"`
	RedeemCode             string     `json:"redeem_code"`
	RequestNote            string     `json:"request_note"`
	Status                 string     `json:"status"`
	AssignedAccountID      int64      `json:"assigned_account_id"`
	AssignedSeatID         int64      `json:"assigned_seat_id"`
	AssignedSubscriptionID int64      `json:"assigned_subscription_id"`
	OperatorNote           string     `json:"operator_note"`
	InvitedAt              *time.Time `json:"invited_at"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

// RedemptionCode is an operator-generated one-time code. Customers must submit
// an unused code before a redemption application can be created.
type RedemptionCode struct {
	ID                  int64      `json:"id"`
	Code                string     `json:"code"`
	Status              string     `json:"status"`
	Note                string     `json:"note"`
	UsedByApplicationID int64      `json:"used_by_application_id"`
	UsedAt              *time.Time `json:"used_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// TemplateData is passed into Go text/template for notifications.
type TemplateData struct {
	Name             string
	SubscriptionName string
	CustomerEmail    string
	CustomerWechat   string
	AccountName      string
	SeatName         string
	PricePerPerson   string
	// AmountDue is the amount to collect for this subscription period. It is
	// currently an explicit alias of PricePerPerson for clearer templates.
	AmountDue    string
	CycleDesc    string
	NextDueDate  string
	DaysUntilDue int
	DueInText    string
	Remark       string
	TradeURL     string
}

// ExportPayload is the JSON export shape (no secrets).
type ExportPayload struct {
	ExportedAt            string               `json:"exported_at"`
	NotifyTemplate        string               `json:"notify_template"`
	CustomerEmailTemplate string               `json:"customer_email_template"`
	EnabledChannels       []string             `json:"enabled_channels"`
	RedeemPageSettings    RedeemPageSettings   `json:"redeem_page_settings"`
	Accounts              []ExportAccount      `json:"accounts"`
	Subscriptions         []ExportSubscription `json:"subscriptions"`
}

// ExportAccount is one account with seats in an export file.
type ExportAccount struct {
	ID                   int64        `json:"id"`
	Name                 string       `json:"name"`
	Remark               string       `json:"remark"`
	PaymentMethod        string       `json:"payment_method"`
	Email                string       `json:"email"`
	SpaceName            string       `json:"space_name"`
	OpenedAt             string       `json:"opened_at"`
	CostCents            int64        `json:"cost_cents"`
	CostYuan             string       `json:"cost_yuan"`
	TotalCostCents       int64        `json:"total_cost_cents"`
	TotalCostYuan        string       `json:"total_cost_yuan"`
	ZeroRenewalNextMonth bool         `json:"zero_renewal_next_month"`
	Seats                []ExportSeat `json:"seats"`
}

// ExportSeat is one seat under an account in an export file.
type ExportSeat struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ExportSubscription is one subscription in an export file.
type ExportSubscription struct {
	ID                  int64    `json:"id"`
	Name                string   `json:"name"`
	PricePerPersonCents int64    `json:"price_per_person_cents"`
	PricePerPersonYuan  string   `json:"price_per_person_yuan"`
	CostCents           int64    `json:"cost_cents"`
	CostYuan            string   `json:"cost_yuan"`
	IsResale            bool     `json:"is_resale"`
	AgencyFeeCents      int64    `json:"agency_fee_cents"`
	AgencyFeeYuan       string   `json:"agency_fee_yuan"`
	ProfitCents         int64    `json:"profit_cents"`
	ProfitYuan          string   `json:"profit_yuan"`
	CronExpr            string   `json:"cron_expr"`
	NotifyOffsets       []int    `json:"notify_offsets"`
	Channels            []string `json:"channels"`
	Remark              string   `json:"remark"`
	TradeURL            string   `json:"trade_url"`
	CustomerEmail       string   `json:"customer_email"`
	CustomerWechat      string   `json:"customer_wechat"`
	SeatID              int64    `json:"seat_id"`
	SeatName            string   `json:"seat_name"`
	AccountID           int64    `json:"account_id"`
	AccountName         string   `json:"account_name"`
	// SubscriptionType is dual-written as account name for older importers.
	SubscriptionType string `json:"subscription_type"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}
