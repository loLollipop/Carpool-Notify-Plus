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

	SettingNotifyTemplate        = "notify_template"
	SettingCustomerEmailTemplate = "customer_email_template"
	SettingEnabledChannels       = "enabled_channels"

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

// DefaultEnabledChannels is the initial global notify-channel selection.
// Gotify remains optional; new installs default to IYUU only.
var DefaultEnabledChannels = []string{ChannelIYUU}

// DefaultNotifyTemplate is the initial global operator message template.
const DefaultNotifyTemplate = `【拼车收钱】{{.CustomerEmail}}
本期应收：¥{{.AmountDue}}
周期：{{.CycleDesc}}
到期：{{.NextDueDate}}
{{if .Remark}}备注：{{.Remark}}{{end}}
{{if .TradeURL}}链接：{{.TradeURL}}{{end}}`

// DefaultCustomerEmailTemplate is the initial template for emails to customers.
const DefaultCustomerEmailTemplate = `您好，您的拼车服务即将到期，请及时续费，以免影响正常使用。

客户邮箱：{{.CustomerEmail}}
本期应收：¥{{.AmountDue}}
计费周期：{{.CycleDesc}}
到期日期：{{.NextDueDate}}
{{if .Remark}}备注：{{.Remark}}{{end}}
{{if .TradeURL}}续费链接：{{.TradeURL}}{{end}}

如需续费或有疑问，请添加 / 联系微信：Jerrylove_Bom
谢谢。`

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
	ZeroRenewalNextMonth bool      `json:"zero_renewal_next_month"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
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
	ArchivedAt *time.Time `json:"archived_at"`
	DeletedAt  *time.Time `json:"deleted_at"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
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
	AmountDue   string
	CycleDesc   string
	NextDueDate string
	Remark      string
	TradeURL    string
}

// ExportPayload is the JSON export shape (no secrets).
type ExportPayload struct {
	ExportedAt      string               `json:"exported_at"`
	NotifyTemplate  string               `json:"notify_template"`
	EnabledChannels []string             `json:"enabled_channels"`
	Accounts        []ExportAccount      `json:"accounts"`
	Subscriptions   []ExportSubscription `json:"subscriptions"`
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
