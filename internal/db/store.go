package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"carpool-notify/internal/model"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite connection and data access methods.
type Store struct {
	database *sql.DB
}

var (
	ErrRedemptionCodeNotFound  = errors.New("redemption code not found")
	ErrRedemptionCodeUsed      = errors.New("redemption code used")
	ErrRedemptionCodeDisabled  = errors.New("redemption code disabled")
	ErrRedemptionCodeNotUnused = errors.New("redemption code not unused")
)

// Open creates the database file if needed, opens a connection, and migrates.
func Open(databasePath string) (*Store, error) {
	directory := filepath.Dir(databasePath)
	if directory != "." && directory != "" {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}

	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	database.SetMaxOpenConns(1)

	if _, err := database.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	store := &Store{database: database}
	if err := store.migrate(); err != nil {
		_ = database.Close()
		return nil, err
	}
	return store, nil
}

// Close closes the underlying database.
func (store *Store) Close() error {
	return store.database.Close()
}

func (store *Store) migrate() error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			remark TEXT NOT NULL DEFAULT '',
			payment_method TEXT NOT NULL DEFAULT '',
			email TEXT NOT NULL DEFAULT '',
			space_name TEXT NOT NULL DEFAULT '',
			opened_at TEXT NOT NULL DEFAULT '',
			cost_cents INTEGER NOT NULL DEFAULT 0,
			zero_renewal_next_month INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS seats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY(account_id) REFERENCES accounts(id)
		);`,
		`CREATE TABLE IF NOT EXISTS subscriptions (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        name TEXT NOT NULL,
                        price_per_person_cents INTEGER NOT NULL,
                        cron_expr TEXT NOT NULL,
                        notify_offsets TEXT NOT NULL,
                        channels TEXT NOT NULL,
                        remark TEXT NOT NULL DEFAULT '',
                        trade_url TEXT NOT NULL DEFAULT '',
                        deleted_at TEXT,
                        created_at TEXT NOT NULL,
                        updated_at TEXT NOT NULL
                );`,
		`CREATE TABLE IF NOT EXISTS notification_log (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        subscription_id INTEGER NOT NULL,
                        due_date TEXT NOT NULL,
                        offset_days INTEGER NOT NULL,
                        channel TEXT NOT NULL,
                        status TEXT NOT NULL,
                        attempt_count INTEGER NOT NULL DEFAULT 0,
                        next_retry_at TEXT,
                        last_error TEXT NOT NULL DEFAULT '',
                        kind TEXT NOT NULL,
                        created_at TEXT NOT NULL,
                        updated_at TEXT NOT NULL,
                        UNIQUE(subscription_id, due_date, offset_days, channel, kind),
				FOREIGN KEY(subscription_id) REFERENCES subscriptions(id)
		);`,
		`CREATE TABLE IF NOT EXISTS paid_due_occurrences (
				subscription_id INTEGER NOT NULL,
				due_date TEXT NOT NULL,
				paid_at TEXT NOT NULL,
				PRIMARY KEY(subscription_id, due_date),
				FOREIGN KEY(subscription_id) REFERENCES subscriptions(id)
		);`,
		`CREATE TABLE IF NOT EXISTS bills (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				subscription_id INTEGER NOT NULL,
				due_date TEXT NOT NULL,
				amount_cents INTEGER NOT NULL,
				note TEXT NOT NULL DEFAULT '',
				paid_at TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				UNIQUE(subscription_id, due_date),
				FOREIGN KEY(subscription_id) REFERENCES subscriptions(id)
		);`,
		`CREATE TABLE IF NOT EXISTS settings (
                        key TEXT PRIMARY KEY,
                        value TEXT NOT NULL
                );`,
		`CREATE TABLE IF NOT EXISTS redemption_applications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tracking_token TEXT NOT NULL UNIQUE,
			customer_email TEXT NOT NULL,
			customer_contact TEXT NOT NULL,
			redeem_code TEXT NOT NULL,
			request_note TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			assigned_account_id INTEGER NOT NULL DEFAULT 0,
			assigned_seat_id INTEGER NOT NULL DEFAULT 0,
			assigned_subscription_id INTEGER NOT NULL DEFAULT 0,
			operator_note TEXT NOT NULL DEFAULT '',
			invited_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_redemption_applications_status
			ON redemption_applications(status, created_at);`,
		`CREATE TABLE IF NOT EXISTS redemption_codes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			code TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			used_by_application_id INTEGER NOT NULL DEFAULT 0,
			used_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_redemption_codes_status
			ON redemption_codes(status, created_at);`,
	}
	for _, statement := range statements {
		if _, err := store.database.Exec(statement); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}

	if err := store.ensureSubscriptionTypeColumn(); err != nil {
		return err
	}
	if err := store.ensureCostCentsColumn(); err != nil {
		return err
	}
	if err := store.ensureResaleColumns(); err != nil {
		return err
	}
	if err := store.ensureArchivedAtColumn(); err != nil {
		return err
	}
	if err := store.ensureBoardedAtColumn(); err != nil {
		return err
	}
	if err := store.ensureSeatIDColumn(); err != nil {
		return err
	}
	if err := store.ensureCustomerEmailColumn(); err != nil {
		return err
	}
	if err := store.ensureCustomerWechatColumn(); err != nil {
		return err
	}
	if err := store.ensureAccountPaymentMethodColumn(); err != nil {
		return err
	}
	if err := store.ensureAccountDetailColumns(); err != nil {
		return err
	}
	// Legacy migrations (subscription_type → accounts/seats, paid_due → bills) are not
	// re-run: production bills and accounts are the source of truth and must not grow
	// from stale paid_due_occurrences rows on every process start.

	var templateCount int
	err := store.database.QueryRow(
		`SELECT COUNT(1) FROM settings WHERE key = ?`,
		model.SettingNotifyTemplate,
	).Scan(&templateCount)
	if err != nil {
		return fmt.Errorf("check default template: %w", err)
	}
	if templateCount == 0 {
		if err := store.SetSetting(model.SettingNotifyTemplate, model.DefaultNotifyTemplate); err != nil {
			return err
		}
	}

	var customerTemplateCount int
	err = store.database.QueryRow(
		`SELECT COUNT(1) FROM settings WHERE key = ?`,
		model.SettingCustomerEmailTemplate,
	).Scan(&customerTemplateCount)
	if err != nil {
		return fmt.Errorf("check default customer email template: %w", err)
	}
	if customerTemplateCount == 0 {
		if err := store.SetSetting(model.SettingCustomerEmailTemplate, model.DefaultCustomerEmailTemplate); err != nil {
			return err
		}
	}
	for _, templateKey := range []string{
		model.SettingNotifyTemplate,
		model.SettingCustomerEmailTemplate,
	} {
		if err := store.migrateTemplateNameToCustomerEmail(templateKey); err != nil {
			return err
		}
	}
	if err := store.migrateDefaultCustomerEmailTemplate(); err != nil {
		return err
	}
	if err := store.migrateDefaultTemplatesToDueInText(); err != nil {
		return err
	}

	var channelsCount int
	err = store.database.QueryRow(
		`SELECT COUNT(1) FROM settings WHERE key = ?`,
		model.SettingEnabledChannels,
	).Scan(&channelsCount)
	if err != nil {
		return fmt.Errorf("check default enabled channels: %w", err)
	}
	if channelsCount == 0 {
		channelsJSON, marshalErr := json.Marshal(model.DefaultEnabledChannels)
		if marshalErr != nil {
			return fmt.Errorf("marshal default enabled channels: %w", marshalErr)
		}
		if err := store.SetSetting(model.SettingEnabledChannels, string(channelsJSON)); err != nil {
			return err
		}
	}
	return nil
}

const legacyDefaultCustomerEmailTemplate = `您好，这是关于「{{.CustomerEmail}}」的拼车提醒。

本期应收：¥{{.AmountDue}}
周期：{{.CycleDesc}}
到期：{{.NextDueDate}}
{{if .Remark}}备注：{{.Remark}}{{end}}
{{if .TradeURL}}链接：{{.TradeURL}}{{end}}

请按时缴费，谢谢。`

const defaultNotifyTemplateBeforeDueInText = `【拼车收钱】{{.CustomerEmail}}
本期应收：¥{{.AmountDue}}
周期：{{.CycleDesc}}
到期：{{.NextDueDate}}
{{if .Remark}}备注：{{.Remark}}{{end}}
{{if .TradeURL}}链接：{{.TradeURL}}{{end}}`

const defaultCustomerEmailTemplateBeforeDueInText = `您好，您的拼车服务即将到期，请及时续费，以免影响正常使用。

客户邮箱：{{.CustomerEmail}}
本期应收：¥{{.AmountDue}}
计费周期：{{.CycleDesc}}
到期日期：{{.NextDueDate}}
{{if .Remark}}备注：{{.Remark}}{{end}}
{{if .TradeURL}}续费链接：{{.TradeURL}}{{end}}

如需续费或有疑问，请添加 / 联系微信：Jerrylove_Bom
谢谢。`

func (store *Store) migrateDefaultCustomerEmailTemplate() error {
	templateBody, err := store.GetSetting(model.SettingCustomerEmailTemplate)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	if normalizeTemplateForMigration(templateBody) != normalizeTemplateForMigration(legacyDefaultCustomerEmailTemplate) {
		return nil
	}
	return store.SetSetting(model.SettingCustomerEmailTemplate, model.DefaultCustomerEmailTemplate)
}

func (store *Store) migrateDefaultTemplatesToDueInText() error {
	migrations := map[string]struct {
		oldValue string
		newValue string
	}{
		model.SettingNotifyTemplate: {
			oldValue: defaultNotifyTemplateBeforeDueInText,
			newValue: model.DefaultNotifyTemplate,
		},
		model.SettingCustomerEmailTemplate: {
			oldValue: defaultCustomerEmailTemplateBeforeDueInText,
			newValue: model.DefaultCustomerEmailTemplate,
		},
	}
	for key, migration := range migrations {
		templateBody, err := store.GetSetting(key)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return err
		}
		if strings.Contains(templateBody, ".DueInText") {
			continue
		}
		if normalizeTemplateForMigration(templateBody) != normalizeTemplateForMigration(migration.oldValue) {
			continue
		}
		if err := store.SetSetting(key, migration.newValue); err != nil {
			return err
		}
	}
	return nil
}

func normalizeTemplateForMigration(templateBody string) string {
	return strings.TrimSpace(strings.ReplaceAll(templateBody, "\r\n", "\n"))
}

func (store *Store) migrateTemplateNameToCustomerEmail(key string) error {
	templateBody, err := store.GetSetting(key)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	if strings.Contains(templateBody, ".CustomerEmail") || !strings.Contains(templateBody, ".Name") {
		return nil
	}
	replacements := map[string]string{
		"{{.Name}}":   "{{.CustomerEmail}}",
		"{{ .Name}}":  "{{.CustomerEmail}}",
		"{{.Name }}":  "{{.CustomerEmail}}",
		"{{ .Name }}": "{{.CustomerEmail}}",
	}
	for oldValue, newValue := range replacements {
		templateBody = strings.ReplaceAll(templateBody, oldValue, newValue)
	}
	return store.SetSetting(key, templateBody)
}

func (store *Store) ensureSubscriptionTypeColumn() error {
	hasColumn, err := store.subscriptionsHasColumn("subscription_type")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	_, err = store.database.Exec(
		`ALTER TABLE subscriptions ADD COLUMN subscription_type TEXT NOT NULL DEFAULT '其它'`,
	)
	if err != nil {
		return fmt.Errorf("add subscription_type: %w", err)
	}
	return nil
}

func (store *Store) ensureCostCentsColumn() error {
	hasColumn, err := store.subscriptionsHasColumn("cost_cents")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	_, err = store.database.Exec(
		`ALTER TABLE subscriptions ADD COLUMN cost_cents INTEGER NOT NULL DEFAULT 0`,
	)
	if err != nil {
		return fmt.Errorf("add cost_cents: %w", err)
	}
	return nil
}

func (store *Store) ensureResaleColumns() error {
	hasResale, err := store.subscriptionsHasColumn("is_resale")
	if err != nil {
		return err
	}
	if !hasResale {
		_, err = store.database.Exec(
			`ALTER TABLE subscriptions ADD COLUMN is_resale INTEGER NOT NULL DEFAULT 0`,
		)
		if err != nil {
			return fmt.Errorf("add is_resale: %w", err)
		}
	}
	hasFee, err := store.subscriptionsHasColumn("agency_fee_cents")
	if err != nil {
		return err
	}
	if !hasFee {
		_, err = store.database.Exec(
			`ALTER TABLE subscriptions ADD COLUMN agency_fee_cents INTEGER NOT NULL DEFAULT 0`,
		)
		if err != nil {
			return fmt.Errorf("add agency_fee_cents: %w", err)
		}
	}
	return nil
}

func (store *Store) ensureArchivedAtColumn() error {
	hasColumn, err := store.subscriptionsHasColumn("archived_at")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	_, err = store.database.Exec(
		`ALTER TABLE subscriptions ADD COLUMN archived_at TEXT`,
	)
	if err != nil {
		return fmt.Errorf("add archived_at: %w", err)
	}
	return nil
}

func (store *Store) ensureBoardedAtColumn() error {
	hasColumn, err := store.subscriptionsHasColumn("boarded_at")
	if err != nil {
		return err
	}
	if !hasColumn {
		_, err = store.database.Exec(
			`ALTER TABLE subscriptions ADD COLUMN boarded_at TEXT NOT NULL DEFAULT ''`,
		)
		if err != nil {
			return fmt.Errorf("add boarded_at: %w", err)
		}
	}

	// Backfill empty boarded_at from created_at calendar day (UTC date is fine as fallback).
	_, err = store.database.Exec(`
		UPDATE subscriptions
		SET boarded_at = substr(created_at, 1, 10)
		WHERE boarded_at = '' OR boarded_at IS NULL`)
	if err != nil {
		return fmt.Errorf("backfill boarded_at: %w", err)
	}
	return nil
}

func (store *Store) ensureSeatIDColumn() error {
	hasColumn, err := store.subscriptionsHasColumn("seat_id")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	_, err = store.database.Exec(
		`ALTER TABLE subscriptions ADD COLUMN seat_id INTEGER`,
	)
	if err != nil {
		return fmt.Errorf("add seat_id: %w", err)
	}
	return nil
}

func (store *Store) ensureCustomerEmailColumn() error {
	hasColumn, err := store.subscriptionsHasColumn("customer_email")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	_, err = store.database.Exec(
		`ALTER TABLE subscriptions ADD COLUMN customer_email TEXT NOT NULL DEFAULT ''`,
	)
	if err != nil {
		return fmt.Errorf("add customer_email: %w", err)
	}
	return nil
}

func (store *Store) ensureCustomerWechatColumn() error {
	hasColumn, err := store.subscriptionsHasColumn("customer_wechat")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	_, err = store.database.Exec(
		`ALTER TABLE subscriptions ADD COLUMN customer_wechat TEXT NOT NULL DEFAULT ''`,
	)
	if err != nil {
		return fmt.Errorf("add customer_wechat: %w", err)
	}
	return nil
}

func (store *Store) ensureAccountPaymentMethodColumn() error {
	_, err := store.ensureAccountColumn(
		"payment_method",
		`ALTER TABLE accounts ADD COLUMN payment_method TEXT NOT NULL DEFAULT ''`,
	)
	return err
}

func (store *Store) ensureAccountDetailColumns() error {
	if _, err := store.ensureAccountColumn(
		"email",
		`ALTER TABLE accounts ADD COLUMN email TEXT NOT NULL DEFAULT ''`,
	); err != nil {
		return err
	}
	if _, err := store.ensureAccountColumn(
		"space_name",
		`ALTER TABLE accounts ADD COLUMN space_name TEXT NOT NULL DEFAULT ''`,
	); err != nil {
		return err
	}
	if _, err := store.ensureAccountColumn(
		"opened_at",
		`ALTER TABLE accounts ADD COLUMN opened_at TEXT NOT NULL DEFAULT ''`,
	); err != nil {
		return err
	}
	addedCost, err := store.ensureAccountColumn(
		"cost_cents",
		`ALTER TABLE accounts ADD COLUMN cost_cents INTEGER NOT NULL DEFAULT 0`,
	)
	if err != nil {
		return err
	}
	if _, err := store.ensureAccountColumn(
		"zero_renewal_next_month",
		`ALTER TABLE accounts ADD COLUMN zero_renewal_next_month INTEGER NOT NULL DEFAULT 0`,
	); err != nil {
		return err
	}
	if addedCost {
		if err := store.backfillAccountCostCents(); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) ensureAccountColumn(columnName string, statement string) (bool, error) {
	hasColumn, err := store.accountsHasColumn(columnName)
	if err != nil {
		return false, err
	}
	if hasColumn {
		return false, nil
	}
	_, err = store.database.Exec(statement)
	if err != nil {
		return false, fmt.Errorf("add accounts.%s: %w", columnName, err)
	}
	return true, nil
}

func (store *Store) backfillAccountCostCents() error {
	_, err := store.database.Exec(`
		UPDATE accounts
		SET cost_cents = COALESCE((
			SELECT MAX(subscription.cost_cents)
			FROM seats AS seat
			INNER JOIN subscriptions AS subscription ON subscription.seat_id = seat.id
			WHERE seat.account_id = accounts.id
			  AND subscription.deleted_at IS NULL
			  AND subscription.archived_at IS NULL
			  AND subscription.cost_cents > 0
		), 0)
		WHERE cost_cents = 0`)
	if err != nil {
		return fmt.Errorf("backfill accounts.cost_cents: %w", err)
	}
	return nil
}

func (store *Store) subscriptionsHasColumn(columnName string) (bool, error) {
	return store.tableHasColumn("subscriptions", columnName)
}

func (store *Store) accountsHasColumn(columnName string) (bool, error) {
	return store.tableHasColumn("accounts", columnName)
}

func (store *Store) tableHasColumn(tableName string, columnName string) (bool, error) {
	var pragma string
	switch tableName {
	case "subscriptions":
		pragma = `PRAGMA table_info(subscriptions)`
	case "accounts":
		pragma = `PRAGMA table_info(accounts)`
	default:
		return false, fmt.Errorf("unknown table for column check: %s", tableName)
	}
	rows, err := store.database.Query(pragma)
	if err != nil {
		return false, fmt.Errorf("pragma %s: %w", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}
	return false, rows.Err()
}

const subscriptionSelectColumns = `
	subscription.id,
	subscription.name,
	subscription.price_per_person_cents,
	subscription.cost_cents,
	COALESCE(subscription.is_resale, 0),
	COALESCE(subscription.agency_fee_cents, 0),
	subscription.cron_expr,
	subscription.notify_offsets,
	subscription.channels,
	subscription.remark,
	subscription.trade_url,
	COALESCE(subscription.customer_email, ''),
	COALESCE(subscription.customer_wechat, ''),
	COALESCE(subscription.seat_id, 0),
	COALESCE(seat.account_id, 0),
	COALESCE(account.name, ''),
	COALESCE(seat.name, ''),
	subscription.subscription_type,
	subscription.boarded_at,
	subscription.archived_at,
	subscription.deleted_at,
	subscription.created_at,
	subscription.updated_at`

const subscriptionFromJoin = `
	FROM subscriptions AS subscription
	LEFT JOIN seats AS seat ON seat.id = subscription.seat_id
	LEFT JOIN accounts AS account ON account.id = seat.account_id`

// ListSubscriptions returns active (non-deleted, non-archived) subscriptions ordered by id.
func (store *Store) ListSubscriptions() ([]model.Subscription, error) {
	rows, err := store.database.Query(`
                SELECT ` + subscriptionSelectColumns + `
                ` + subscriptionFromJoin + `
                WHERE subscription.deleted_at IS NULL AND subscription.archived_at IS NULL
                ORDER BY subscription.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	subscriptions := make([]model.Subscription, 0)
	for rows.Next() {
		subscription, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, subscription)
	}
	return subscriptions, rows.Err()
}

// GetSubscription returns an active (non-deleted, non-archived) subscription by id.
func (store *Store) GetSubscription(subscriptionID int64) (model.Subscription, error) {
	row := store.database.QueryRow(`
                SELECT `+subscriptionSelectColumns+`
                `+subscriptionFromJoin+`
                WHERE subscription.id = ? AND subscription.deleted_at IS NULL AND subscription.archived_at IS NULL`, subscriptionID)
	return scanSubscription(row)
}

// GetSubscriptionIncludingArchived returns a non-deleted subscription, including archived ones.
func (store *Store) GetSubscriptionIncludingArchived(subscriptionID int64) (model.Subscription, error) {
	row := store.database.QueryRow(`
                SELECT `+subscriptionSelectColumns+`
                `+subscriptionFromJoin+`
                WHERE subscription.id = ? AND subscription.deleted_at IS NULL`, subscriptionID)
	return scanSubscription(row)
}

// ListArchivedSubscriptions returns non-deleted archived subscriptions ordered by archive time.
func (store *Store) ListArchivedSubscriptions() ([]model.Subscription, error) {
	rows, err := store.database.Query(`
                SELECT ` + subscriptionSelectColumns + `
                ` + subscriptionFromJoin + `
                WHERE subscription.deleted_at IS NULL AND subscription.archived_at IS NOT NULL
                ORDER BY subscription.archived_at DESC, subscription.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	subscriptions := make([]model.Subscription, 0)
	for rows.Next() {
		subscription, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subscriptions = append(subscriptions, subscription)
	}
	return subscriptions, rows.Err()
}

// CreateSubscription inserts a new active subscription.
func (store *Store) CreateSubscription(subscription model.Subscription) (int64, error) {
	now := formatTime(time.Now().UTC())
	offsetsJSON, err := json.Marshal(subscription.NotifyOffsets)
	if err != nil {
		return 0, err
	}
	channelsJSON, err := json.Marshal(subscription.Channels)
	if err != nil {
		return 0, err
	}
	subscriptionType := subscription.SubscriptionType
	if subscriptionType == "" {
		subscriptionType = subscription.AccountName
	}
	if subscriptionType == "" {
		subscriptionType = model.SubscriptionTypeOther
	}
	boardedAt := strings.TrimSpace(subscription.BoardedAt)
	var seatID interface{}
	if subscription.SeatID > 0 {
		seatID = subscription.SeatID
	}
	isResale := 0
	if subscription.IsResale {
		isResale = 1
	}
	result, err := store.database.Exec(`
                INSERT INTO subscriptions (
                        name, price_per_person_cents, cost_cents, is_resale, agency_fee_cents, cron_expr, notify_offsets, channels,
                        remark, trade_url, customer_email, customer_wechat, subscription_type, seat_id, boarded_at, archived_at, deleted_at, created_at, updated_at
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)`,
		subscription.Name,
		subscription.PricePerPersonCents,
		subscription.CostCents,
		isResale,
		subscription.AgencyFeeCents,
		subscription.CronExpr,
		string(offsetsJSON),
		string(channelsJSON),
		subscription.Remark,
		subscription.TradeURL,
		strings.TrimSpace(subscription.CustomerEmail),
		strings.TrimSpace(subscription.CustomerWechat),
		subscriptionType,
		seatID,
		boardedAt,
		now,
		now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateSubscription updates an existing active (non-deleted, non-archived) subscription.
func (store *Store) UpdateSubscription(subscription model.Subscription) error {
	now := formatTime(time.Now().UTC())
	offsetsJSON, err := json.Marshal(subscription.NotifyOffsets)
	if err != nil {
		return err
	}
	channelsJSON, err := json.Marshal(subscription.Channels)
	if err != nil {
		return err
	}
	subscriptionType := subscription.SubscriptionType
	if subscriptionType == "" {
		subscriptionType = subscription.AccountName
	}
	if subscriptionType == "" {
		subscriptionType = model.SubscriptionTypeOther
	}
	boardedAt := strings.TrimSpace(subscription.BoardedAt)
	var seatID interface{}
	if subscription.SeatID > 0 {
		seatID = subscription.SeatID
	}
	isResale := 0
	if subscription.IsResale {
		isResale = 1
	}
	result, err := store.database.Exec(`
                UPDATE subscriptions
                SET name = ?, price_per_person_cents = ?, cost_cents = ?, is_resale = ?, agency_fee_cents = ?, cron_expr = ?, notify_offsets = ?,
                    channels = ?, remark = ?, trade_url = ?, customer_email = ?, customer_wechat = ?, subscription_type = ?, seat_id = ?, boarded_at = ?, updated_at = ?
                WHERE id = ? AND deleted_at IS NULL AND archived_at IS NULL`,
		subscription.Name,
		subscription.PricePerPersonCents,
		subscription.CostCents,
		isResale,
		subscription.AgencyFeeCents,
		subscription.CronExpr,
		string(offsetsJSON),
		string(channelsJSON),
		subscription.Remark,
		subscription.TradeURL,
		strings.TrimSpace(subscription.CustomerEmail),
		strings.TrimSpace(subscription.CustomerWechat),
		subscriptionType,
		seatID,
		boardedAt,
		now,
		subscription.ID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SoftDeleteSubscription marks a subscription as deleted (legacy; prefer ArchiveSubscription).
func (store *Store) SoftDeleteSubscription(subscriptionID int64) error {
	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
                UPDATE subscriptions
                SET deleted_at = ?, updated_at = ?
                WHERE id = ? AND deleted_at IS NULL`,
		now, now, subscriptionID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SoftDeleteArchivedSubscription soft-deletes an already-archived subscription.
// Returns sql.ErrNoRows when the id is missing, not archived, or already deleted.
func (store *Store) SoftDeleteArchivedSubscription(subscriptionID int64) error {
	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
                UPDATE subscriptions
                SET deleted_at = ?, updated_at = ?
                WHERE id = ?
                  AND deleted_at IS NULL
                  AND archived_at IS NOT NULL`,
		now, now, subscriptionID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CountBillsForSubscription returns how many bills are linked to the subscription.
func (store *Store) CountBillsForSubscription(subscriptionID int64) (int, error) {
	var count int
	err := store.database.QueryRow(
		`SELECT COUNT(1) FROM bills WHERE subscription_id = ?`,
		subscriptionID,
	).Scan(&count)
	return count, err
}

// ArchiveSubscription marks a subscription as archived (下车).
// Works for active non-deleted subscriptions; already-archived is a no-op success.
func (store *Store) ArchiveSubscription(subscriptionID int64) error {
	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
                UPDATE subscriptions
                SET archived_at = COALESCE(archived_at, ?), updated_at = ?
                WHERE id = ? AND deleted_at IS NULL`,
		now, now, subscriptionID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetDuePaid marks or unmarks one subscription due date as paid via bills.
// When paid=true, creates a bill with the given amount if none exists (keeps existing bill).
// When paid=false, deletes the bill for that occurrence.
func (store *Store) SetDuePaid(subscriptionID int64, dueDate string, paid bool, amountCents int64) error {
	if !paid {
		_, err := store.database.Exec(`
			DELETE FROM bills
			WHERE subscription_id = ? AND due_date = ?`,
			subscriptionID, dueDate,
		)
		return err
	}

	existing, err := store.GetBillByOccurrence(subscriptionID, dueDate)
	if err == nil && existing.ID > 0 {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	now := formatTime(time.Now().UTC())
	_, err = store.database.Exec(`
		INSERT INTO bills (
			subscription_id, due_date, amount_cents, note, paid_at, created_at, updated_at
		) VALUES (?, ?, ?, '', ?, ?, ?)
		ON CONFLICT(subscription_id, due_date) DO NOTHING`,
		subscriptionID, dueDate, amountCents, now, now, now,
	)
	return err
}

// IsDuePaid reports whether one subscription due date has a bill.
func (store *Store) IsDuePaid(subscriptionID int64, dueDate string) (bool, error) {
	var paid bool
	err := store.database.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM bills
			WHERE subscription_id = ? AND due_date = ?
		)`,
		subscriptionID, dueDate,
	).Scan(&paid)
	return paid, err
}

// ListPaidDueOccurrences returns paid due dates in the half-open date range (from bills).
func (store *Store) ListPaidDueOccurrences(startDate string, endDate string) ([]model.PaidDueOccurrence, error) {
	rows, err := store.database.Query(`
		SELECT subscription_id, due_date
		FROM bills
		WHERE due_date >= ? AND due_date < ?
		ORDER BY due_date, subscription_id`,
		startDate, endDate,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	occurrences := make([]model.PaidDueOccurrence, 0)
	for rows.Next() {
		var occurrence model.PaidDueOccurrence
		if err := rows.Scan(&occurrence.SubscriptionID, &occurrence.DueDate); err != nil {
			return nil, err
		}
		occurrences = append(occurrences, occurrence)
	}
	return occurrences, rows.Err()
}

// GetBill returns a bill by id.
func (store *Store) GetBill(billID int64) (model.Bill, error) {
	row := store.database.QueryRow(`
		SELECT id, subscription_id, due_date, amount_cents, note, paid_at, created_at, updated_at
		FROM bills
		WHERE id = ?`, billID)
	return scanBill(row)
}

// GetBillByOccurrence returns the bill for one subscription due date.
func (store *Store) GetBillByOccurrence(subscriptionID int64, dueDate string) (model.Bill, error) {
	row := store.database.QueryRow(`
		SELECT id, subscription_id, due_date, amount_cents, note, paid_at, created_at, updated_at
		FROM bills
		WHERE subscription_id = ? AND due_date = ?`,
		subscriptionID, dueDate,
	)
	return scanBill(row)
}

// ListBills returns all bills newest-paid first.
func (store *Store) ListBills() ([]model.Bill, error) {
	rows, err := store.database.Query(`
		SELECT id, subscription_id, due_date, amount_cents, note, paid_at, created_at, updated_at
		FROM bills
		ORDER BY paid_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bills := make([]model.Bill, 0)
	for rows.Next() {
		bill, err := scanBill(rows)
		if err != nil {
			return nil, err
		}
		bills = append(bills, bill)
	}
	return bills, rows.Err()
}

// UpdateBill updates amount and note for a bill.
func (store *Store) UpdateBill(billID int64, amountCents int64, note string) error {
	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
		UPDATE bills
		SET amount_cents = ?, note = ?, updated_at = ?
		WHERE id = ?`,
		amountCents, note, now, billID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteBill removes one bill by id (same effect as unmarking that due as paid).
func (store *Store) DeleteBill(billID int64) error {
	result, err := store.database.Exec(`DELETE FROM bills WHERE id = ?`, billID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ClearSeatLinksForSeat nulls seat_id on all subscriptions still pointing at the seat.
// Used before deleting a free seat so archived/historical rows no longer block removal.
func (store *Store) ClearSeatLinksForSeat(seatID int64) error {
	now := formatTime(time.Now().UTC())
	_, err := store.database.Exec(`
		UPDATE subscriptions
		SET seat_id = NULL, updated_at = ?
		WHERE seat_id = ?`,
		now, seatID,
	)
	return err
}

// GetSetting returns a settings value by key.
func (store *Store) GetSetting(key string) (string, error) {
	var value string
	err := store.database.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	return value, err
}

// SetSetting upserts a settings value.
func (store *Store) SetSetting(key string, value string) error {
	_, err := store.database.Exec(`
                INSERT INTO settings(key, value) VALUES(?, ?)
                ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

const redemptionSelectColumns = `
	id,
	tracking_token,
	customer_email,
	customer_contact,
	redeem_code,
	request_note,
	status,
	assigned_account_id,
	assigned_seat_id,
	assigned_subscription_id,
	operator_note,
	invited_at,
	created_at,
	updated_at`

const redemptionCodeSelectColumns = `
	id,
	code,
	status,
	note,
	used_by_application_id,
	used_at,
	created_at,
	updated_at`

// CreateRedemptionCode inserts one operator-generated one-time redemption code.
func (store *Store) CreateRedemptionCode(code model.RedemptionCode) (int64, error) {
	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
		INSERT INTO redemption_codes (code, status, note, used_by_application_id, used_at, created_at, updated_at)
		VALUES (?, ?, ?, 0, NULL, ?, ?)`,
		strings.TrimSpace(code.Code),
		model.RedemptionCodeStatusUnused,
		strings.TrimSpace(code.Note),
		now,
		now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetRedemptionCode returns one generated redemption code by id.
func (store *Store) GetRedemptionCode(codeID int64) (model.RedemptionCode, error) {
	row := store.database.QueryRow(`
		SELECT `+redemptionCodeSelectColumns+`
		FROM redemption_codes
		WHERE id = ?`, codeID)
	return scanRedemptionCode(row)
}

// ListRedemptionCodes returns generated codes with still-usable codes first.
func (store *Store) ListRedemptionCodes() ([]model.RedemptionCode, error) {
	rows, err := store.database.Query(`
		SELECT ` + redemptionCodeSelectColumns + `
		FROM redemption_codes
		ORDER BY
			CASE status
				WHEN 'unused' THEN 0
				WHEN 'disabled' THEN 1
				ELSE 2
			END ASC,
			created_at DESC,
			id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	codes := make([]model.RedemptionCode, 0)
	for rows.Next() {
		code, err := scanRedemptionCode(rows)
		if err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}

// SetRedemptionCodeStatus changes a generated code between unused/disabled.
func (store *Store) SetRedemptionCodeStatus(codeID int64, status string) error {
	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
		UPDATE redemption_codes
		SET status = ?, updated_at = ?
		WHERE id = ?`,
		strings.TrimSpace(status),
		now,
		codeID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteRedemptionCode permanently removes an unused or disabled generated code.
func (store *Store) DeleteRedemptionCode(codeID int64) error {
	result, err := store.database.Exec(`
		DELETE FROM redemption_codes
		WHERE id = ? AND status != ?`,
		codeID,
		model.RedemptionCodeStatusUsed,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// CreateRedemptionApplication inserts a customer-submitted redemption request.
func (store *Store) CreateRedemptionApplication(application model.RedemptionApplication) (int64, error) {
	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
		INSERT INTO redemption_applications (
			tracking_token, customer_email, customer_contact, redeem_code, request_note,
			status, assigned_account_id, assigned_seat_id, assigned_subscription_id,
			operator_note, invited_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, 0, 0, 0, '', NULL, ?, ?)`,
		strings.TrimSpace(application.TrackingToken),
		strings.TrimSpace(application.CustomerEmail),
		strings.TrimSpace(application.CustomerContact),
		strings.TrimSpace(application.RedeemCode),
		strings.TrimSpace(application.RequestNote),
		model.RedemptionStatusPending,
		now,
		now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// CreateRedemptionApplicationUsingCode atomically creates an application and
// consumes a previously generated unused redemption code.
func (store *Store) CreateRedemptionApplicationUsingCode(application model.RedemptionApplication) (int64, error) {
	now := formatTime(time.Now().UTC())
	redeemCode := strings.TrimSpace(application.RedeemCode)

	transaction, err := store.database.Begin()
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = transaction.Rollback()
	}()

	var codeID int64
	var codeStatus string
	err = transaction.QueryRow(`
		SELECT id, status
		FROM redemption_codes
		WHERE code = ?`, redeemCode).Scan(&codeID, &codeStatus)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, ErrRedemptionCodeNotFound
		}
		return 0, err
	}
	switch strings.TrimSpace(codeStatus) {
	case model.RedemptionCodeStatusUnused:
	case model.RedemptionCodeStatusUsed:
		return 0, ErrRedemptionCodeUsed
	case model.RedemptionCodeStatusDisabled:
		return 0, ErrRedemptionCodeDisabled
	default:
		return 0, ErrRedemptionCodeNotUnused
	}

	result, err := transaction.Exec(`
		INSERT INTO redemption_applications (
			tracking_token, customer_email, customer_contact, redeem_code, request_note,
			status, assigned_account_id, assigned_seat_id, assigned_subscription_id,
			operator_note, invited_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, 0, 0, 0, '', NULL, ?, ?)`,
		strings.TrimSpace(application.TrackingToken),
		strings.TrimSpace(application.CustomerEmail),
		strings.TrimSpace(application.CustomerContact),
		redeemCode,
		strings.TrimSpace(application.RequestNote),
		model.RedemptionStatusPending,
		now,
		now,
	)
	if err != nil {
		return 0, err
	}
	applicationID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	result, err = transaction.Exec(`
		UPDATE redemption_codes
		SET status = ?,
			used_by_application_id = ?,
			used_at = ?,
			updated_at = ?
		WHERE id = ? AND status = ?`,
		model.RedemptionCodeStatusUsed,
		applicationID,
		now,
		now,
		codeID,
		model.RedemptionCodeStatusUnused,
	)
	if err != nil {
		return 0, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if rowsAffected == 0 {
		return 0, ErrRedemptionCodeNotUnused
	}

	if err := transaction.Commit(); err != nil {
		return 0, err
	}
	return applicationID, nil
}

// GetRedemptionApplication returns one redemption application by id.
func (store *Store) GetRedemptionApplication(applicationID int64) (model.RedemptionApplication, error) {
	row := store.database.QueryRow(`
		SELECT `+redemptionSelectColumns+`
		FROM redemption_applications
		WHERE id = ?`, applicationID)
	return scanRedemptionApplication(row)
}

// GetRedemptionApplicationByToken returns a public status view source.
func (store *Store) GetRedemptionApplicationByToken(token string) (model.RedemptionApplication, error) {
	row := store.database.QueryRow(`
		SELECT `+redemptionSelectColumns+`
		FROM redemption_applications
		WHERE tracking_token = ?`, strings.TrimSpace(token))
	return scanRedemptionApplication(row)
}

// ListRedemptionApplications returns redemption applications, optionally filtered by status.
func (store *Store) ListRedemptionApplications(status string) ([]model.RedemptionApplication, error) {
	status = strings.TrimSpace(status)
	query := `
		SELECT ` + redemptionSelectColumns + `
		FROM redemption_applications`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += `
		ORDER BY
			CASE status WHEN 'pending' THEN 0 ELSE 1 END ASC,
			created_at DESC,
			id DESC`
	rows, err := store.database.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applications := make([]model.RedemptionApplication, 0)
	for rows.Next() {
		application, err := scanRedemptionApplication(rows)
		if err != nil {
			return nil, err
		}
		applications = append(applications, application)
	}
	return applications, rows.Err()
}

// CountRedemptionApplicationsByStatus counts applications with a given status.
func (store *Store) CountRedemptionApplicationsByStatus(status string) (int, error) {
	var count int
	err := store.database.QueryRow(
		`SELECT COUNT(1) FROM redemption_applications WHERE status = ?`,
		strings.TrimSpace(status),
	).Scan(&count)
	return count, err
}

// MarkRedemptionApplicationInvited marks a pending application as invited.
func (store *Store) MarkRedemptionApplicationInvited(
	applicationID int64,
	accountID int64,
	seatID int64,
	subscriptionID int64,
	operatorNote string,
) error {
	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
		UPDATE redemption_applications
		SET status = ?,
			assigned_account_id = ?,
			assigned_seat_id = ?,
			assigned_subscription_id = ?,
			operator_note = ?,
			invited_at = ?,
			updated_at = ?
		WHERE id = ? AND status = ?`,
		model.RedemptionStatusInvited,
		accountID,
		seatID,
		subscriptionID,
		strings.TrimSpace(operatorNote),
		now,
		now,
		applicationID,
		model.RedemptionStatusPending,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

const accountSelectColumns = `
	id,
	name,
	remark,
	payment_method,
	COALESCE(email, ''),
	COALESCE(space_name, ''),
	COALESCE(opened_at, ''),
	COALESCE(cost_cents, 0),
	COALESCE(zero_renewal_next_month, 0),
	created_at,
	updated_at`

// ListAccounts returns all accounts ordered by opened date, newest first.
func (store *Store) ListAccounts() ([]model.Account, error) {
	rows, err := store.database.Query(`
		SELECT ` + accountSelectColumns + `
		FROM accounts
		ORDER BY
			CASE WHEN COALESCE(opened_at, '') = '' THEN 1 ELSE 0 END ASC,
			opened_at DESC,
			id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make([]model.Account, 0)
	for rows.Next() {
		account, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

// GetAccount returns one account by id.
func (store *Store) GetAccount(accountID int64) (model.Account, error) {
	row := store.database.QueryRow(`
		SELECT `+accountSelectColumns+`
		FROM accounts
		WHERE id = ?`, accountID)
	return scanAccount(row)
}

// GetAccountByName returns one account by exact name.
func (store *Store) GetAccountByName(name string) (model.Account, error) {
	row := store.database.QueryRow(`
		SELECT `+accountSelectColumns+`
		FROM accounts
		WHERE name = ?`, name)
	return scanAccount(row)
}

// CreateAccount inserts a new account.
func (store *Store) CreateAccount(account model.Account) (int64, error) {
	now := formatTime(time.Now().UTC())
	zeroRenewalNextMonth := 0
	if account.ZeroRenewalNextMonth {
		zeroRenewalNextMonth = 1
	}
	result, err := store.database.Exec(`
		INSERT INTO accounts (
			name, remark, payment_method, email, space_name, opened_at, cost_cents, zero_renewal_next_month, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(account.Name),
		strings.TrimSpace(account.Remark),
		strings.TrimSpace(account.PaymentMethod),
		strings.TrimSpace(account.Email),
		strings.TrimSpace(account.SpaceName),
		strings.TrimSpace(account.OpenedAt),
		account.CostCents,
		zeroRenewalNextMonth,
		now,
		now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateAccount updates account name, remark, and payment method.
func (store *Store) UpdateAccount(account model.Account) error {
	now := formatTime(time.Now().UTC())
	zeroRenewalNextMonth := 0
	if account.ZeroRenewalNextMonth {
		zeroRenewalNextMonth = 1
	}
	result, err := store.database.Exec(`
		UPDATE accounts
		SET name = ?, remark = ?, payment_method = ?, email = ?, space_name = ?, opened_at = ?,
		    cost_cents = ?, zero_renewal_next_month = ?, updated_at = ?
		WHERE id = ?`,
		strings.TrimSpace(account.Name),
		strings.TrimSpace(account.Remark),
		strings.TrimSpace(account.PaymentMethod),
		strings.TrimSpace(account.Email),
		strings.TrimSpace(account.SpaceName),
		strings.TrimSpace(account.OpenedAt),
		account.CostCents,
		zeroRenewalNextMonth,
		now,
		account.ID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteAccount removes an account. Callers must ensure no seats remain.
func (store *Store) DeleteAccount(accountID int64) error {
	result, err := store.database.Exec(`DELETE FROM accounts WHERE id = ?`, accountID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ListSeatsByAccount returns seats for one account ordered by id.
func (store *Store) ListSeatsByAccount(accountID int64) ([]model.Seat, error) {
	rows, err := store.database.Query(`
		SELECT id, account_id, name, created_at, updated_at
		FROM seats
		WHERE account_id = ?
		ORDER BY id ASC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seats := make([]model.Seat, 0)
	for rows.Next() {
		seat, err := scanSeat(rows)
		if err != nil {
			return nil, err
		}
		seats = append(seats, seat)
	}
	return seats, rows.Err()
}

// ListAllSeats returns all seats ordered by account then id.
func (store *Store) ListAllSeats() ([]model.Seat, error) {
	rows, err := store.database.Query(`
		SELECT id, account_id, name, created_at, updated_at
		FROM seats
		ORDER BY account_id ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seats := make([]model.Seat, 0)
	for rows.Next() {
		seat, err := scanSeat(rows)
		if err != nil {
			return nil, err
		}
		seats = append(seats, seat)
	}
	return seats, rows.Err()
}

// GetSeat returns one seat by id.
func (store *Store) GetSeat(seatID int64) (model.Seat, error) {
	row := store.database.QueryRow(`
		SELECT id, account_id, name, created_at, updated_at
		FROM seats
		WHERE id = ?`, seatID)
	return scanSeat(row)
}

// CreateSeat inserts a new seat under an account.
func (store *Store) CreateSeat(seat model.Seat) (int64, error) {
	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
		INSERT INTO seats (account_id, name, created_at, updated_at)
		VALUES (?, ?, ?, ?)`,
		seat.AccountID,
		strings.TrimSpace(seat.Name),
		now,
		now,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateSeat renames a seat.
func (store *Store) UpdateSeat(seat model.Seat) error {
	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
		UPDATE seats
		SET name = ?, updated_at = ?
		WHERE id = ?`,
		strings.TrimSpace(seat.Name),
		now,
		seat.ID,
	)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteSeat removes a seat. Callers must ensure it is not occupied by an active subscription.
func (store *Store) DeleteSeat(seatID int64) error {
	result, err := store.database.Exec(`DELETE FROM seats WHERE id = ?`, seatID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetActiveSubscriptionBySeatID returns the active subscription occupying a seat, if any.
func (store *Store) GetActiveSubscriptionBySeatID(seatID int64) (model.Subscription, error) {
	row := store.database.QueryRow(`
		SELECT `+subscriptionSelectColumns+`
		`+subscriptionFromJoin+`
		WHERE subscription.seat_id = ?
		  AND subscription.deleted_at IS NULL
		  AND subscription.archived_at IS NULL
		LIMIT 1`, seatID)
	return scanSubscription(row)
}

// CountActiveSubscriptionsByAccount returns how many active subscriptions occupy seats on the account.
func (store *Store) CountActiveSubscriptionsByAccount(accountID int64) (int, error) {
	var count int
	err := store.database.QueryRow(`
		SELECT COUNT(1)
		FROM subscriptions AS subscription
		INNER JOIN seats AS seat ON seat.id = subscription.seat_id
		WHERE seat.account_id = ?
		  AND subscription.deleted_at IS NULL
		  AND subscription.archived_at IS NULL`,
		accountID,
	).Scan(&count)
	return count, err
}

// CountSeatsByAccount returns the total number of seats under an account.
func (store *Store) CountSeatsByAccount(accountID int64) (int, error) {
	var count int
	err := store.database.QueryRow(
		`SELECT COUNT(1) FROM seats WHERE account_id = ?`,
		accountID,
	).Scan(&count)
	return count, err
}

// ListFreeSeats returns seats under an account that have no active subscription.
// When includeSeatID > 0, that seat is always included (for edit forms keeping current seat).
func (store *Store) ListFreeSeats(accountID int64, includeSeatID int64) ([]model.Seat, error) {
	rows, err := store.database.Query(`
		SELECT seat.id, seat.account_id, seat.name, seat.created_at, seat.updated_at
		FROM seats AS seat
		WHERE seat.account_id = ?
		  AND (
			seat.id = ?
			OR NOT EXISTS (
				SELECT 1 FROM subscriptions AS subscription
				WHERE subscription.seat_id = seat.id
				  AND subscription.deleted_at IS NULL
				  AND subscription.archived_at IS NULL
			)
		  )
		ORDER BY seat.id ASC`,
		accountID, includeSeatID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seats := make([]model.Seat, 0)
	for rows.Next() {
		seat, err := scanSeat(rows)
		if err != nil {
			return nil, err
		}
		seats = append(seats, seat)
	}
	return seats, rows.Err()
}

// CountSubscriptionsLinkedToSeat counts non-deleted subscriptions (active or archived) on a seat.
func (store *Store) CountSubscriptionsLinkedToSeat(seatID int64) (int, error) {
	var count int
	err := store.database.QueryRow(`
		SELECT COUNT(1)
		FROM subscriptions
		WHERE seat_id = ? AND deleted_at IS NULL`,
		seatID,
	).Scan(&count)
	return count, err
}

// GetNotificationLog fetches a scheduled log row by unique key.
func (store *Store) GetNotificationLog(
	subscriptionID int64,
	dueDate string,
	offsetDays int,
	channel string,
	kind string,
) (model.NotificationLog, error) {
	row := store.database.QueryRow(`
                SELECT id, subscription_id, due_date, offset_days, channel, status, attempt_count,
                       next_retry_at, last_error, kind, created_at, updated_at
                FROM notification_log
                WHERE subscription_id = ? AND due_date = ? AND offset_days = ? AND channel = ? AND kind = ?`,
		subscriptionID, dueDate, offsetDays, channel, kind,
	)
	return scanNotificationLog(row)
}

// UpsertPendingNotification creates a pending log if missing; returns the log.
func (store *Store) UpsertPendingNotification(
	subscriptionID int64,
	dueDate string,
	offsetDays int,
	channel string,
	kind string,
) (model.NotificationLog, error) {
	existing, err := store.GetNotificationLog(subscriptionID, dueDate, offsetDays, channel, kind)
	if err == nil {
		return existing, nil
	}
	if err != sql.ErrNoRows {
		return model.NotificationLog{}, err
	}

	now := formatTime(time.Now().UTC())
	result, err := store.database.Exec(`
                INSERT INTO notification_log (
                        subscription_id, due_date, offset_days, channel, status, attempt_count,
                        next_retry_at, last_error, kind, created_at, updated_at
                ) VALUES (?, ?, ?, ?, ?, 0, NULL, '', ?, ?, ?)`,
		subscriptionID, dueDate, offsetDays, channel, model.NotificationStatusPending, kind, now, now,
	)
	if err != nil {
		// Race: another worker inserted first.
		return store.GetNotificationLog(subscriptionID, dueDate, offsetDays, channel, kind)
	}
	insertedID, err := result.LastInsertId()
	if err != nil {
		return model.NotificationLog{}, err
	}
	return store.GetNotificationLogByID(insertedID)
}

// GetNotificationLogByID loads a log row by primary key.
func (store *Store) GetNotificationLogByID(logID int64) (model.NotificationLog, error) {
	row := store.database.QueryRow(`
                SELECT id, subscription_id, due_date, offset_days, channel, status, attempt_count,
                       next_retry_at, last_error, kind, created_at, updated_at
                FROM notification_log
                WHERE id = ?`, logID)
	return scanNotificationLog(row)
}

// MarkNotificationSuccess marks a log as success.
func (store *Store) MarkNotificationSuccess(logID int64, attemptCount int) error {
	now := formatTime(time.Now().UTC())
	_, err := store.database.Exec(`
                UPDATE notification_log
                SET status = ?, attempt_count = ?, next_retry_at = NULL, last_error = '', updated_at = ?
                WHERE id = ?`,
		model.NotificationStatusSuccess, attemptCount, now, logID,
	)
	return err
}

// MarkNotificationFailure records a failed attempt and optional retry time.
func (store *Store) MarkNotificationFailure(logID int64, attemptCount int, lastError string, nextRetryAt *time.Time, finalFailed bool) error {
	now := formatTime(time.Now().UTC())
	status := model.NotificationStatusPending
	if finalFailed {
		status = model.NotificationStatusFailed
	}
	var nextRetryValue interface{}
	if nextRetryAt != nil {
		nextRetryValue = formatTime(nextRetryAt.UTC())
	}
	_, err := store.database.Exec(`
                UPDATE notification_log
                SET status = ?, attempt_count = ?, next_retry_at = ?, last_error = ?, updated_at = ?
                WHERE id = ?`,
		status, attemptCount, nextRetryValue, lastError, now, logID,
	)
	return err
}

// ListRetryableNotifications returns pending logs ready for retry or first attempt.
func (store *Store) ListRetryableNotifications(now time.Time) ([]model.NotificationLog, error) {
	nowText := formatTime(now.UTC())
	rows, err := store.database.Query(`
                SELECT id, subscription_id, due_date, offset_days, channel, status, attempt_count,
                       next_retry_at, last_error, kind, created_at, updated_at
                FROM notification_log
                WHERE kind = ?
                  AND status = ?
                  AND (next_retry_at IS NULL OR next_retry_at <= ?)`,
		model.NotificationKindScheduled, model.NotificationStatusPending, nowText,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]model.NotificationLog, 0)
	for rows.Next() {
		entry, err := scanNotificationLog(rows)
		if err != nil {
			return nil, err
		}
		logs = append(logs, entry)
	}
	return logs, rows.Err()
}

// CountFailedNotifications returns recent failed scheduled notifications for active subscriptions.
func (store *Store) CountFailedNotifications() (int, error) {
	var count int
	err := store.database.QueryRow(`
                SELECT COUNT(1)
                FROM notification_log AS log
                INNER JOIN subscriptions AS subscription ON subscription.id = log.subscription_id
                WHERE subscription.deleted_at IS NULL
                  AND subscription.archived_at IS NULL
                  AND log.kind = ?
                  AND log.status = ?`,
		model.NotificationKindScheduled, model.NotificationStatusFailed,
	).Scan(&count)
	return count, err
}

// CountNotificationsByStatusSince counts scheduled notification rows for active
// subscriptions with the given status whose updated_at is on or after since.
func (store *Store) CountNotificationsByStatusSince(status string, since time.Time) (int, error) {
	var count int
	err := store.database.QueryRow(`
                SELECT COUNT(1)
                FROM notification_log AS log
                INNER JOIN subscriptions AS subscription ON subscription.id = log.subscription_id
                WHERE subscription.deleted_at IS NULL
                  AND subscription.archived_at IS NULL
                  AND log.kind = ?
                  AND log.status = ?
                  AND log.updated_at >= ?`,
		model.NotificationKindScheduled, status, formatTime(since.UTC()),
	).Scan(&count)
	return count, err
}

// LatestErrorsBySubscription returns the latest error message per active subscription.
func (store *Store) LatestErrorsBySubscription() (map[int64]string, error) {
	rows, err := store.database.Query(`
                SELECT log.subscription_id, log.last_error
                FROM notification_log AS log
                INNER JOIN (
                        SELECT subscription_id, MAX(id) AS max_id
                        FROM notification_log
                        WHERE kind = ? AND last_error != ''
                        GROUP BY subscription_id
                ) AS latest ON latest.max_id = log.id`,
		model.NotificationKindScheduled,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := map[int64]string{}
	for rows.Next() {
		var subscriptionID int64
		var lastError string
		if err := rows.Scan(&subscriptionID, &lastError); err != nil {
			return nil, err
		}
		result[subscriptionID] = lastError
	}
	return result, rows.Err()
}

// InsertTestNotificationLog records a test send result (not tied to due-date uniqueness beyond attempt).
func (store *Store) InsertTestNotificationLog(
	subscriptionID int64,
	channel string,
	status string,
	lastError string,
) error {
	now := formatTime(time.Now().UTC())
	// Use unique due_date stamp so tests never collide with scheduled uniqueness.
	dueDate := "test-" + now
	_, err := store.database.Exec(`
                INSERT INTO notification_log (
                        subscription_id, due_date, offset_days, channel, status, attempt_count,
                        next_retry_at, last_error, kind, created_at, updated_at
                ) VALUES (?, ?, 0, ?, ?, 1, NULL, ?, ?, ?, ?)`,
		subscriptionID, dueDate, channel, status, lastError, model.NotificationKindTest, now, now,
	)
	return err
}

type scannable interface {
	Scan(dest ...any) error
}

func scanRedemptionApplication(scanner scannable) (model.RedemptionApplication, error) {
	var application model.RedemptionApplication
	var invitedAt sql.NullString
	var createdAt string
	var updatedAt string
	err := scanner.Scan(
		&application.ID,
		&application.TrackingToken,
		&application.CustomerEmail,
		&application.CustomerContact,
		&application.RedeemCode,
		&application.RequestNote,
		&application.Status,
		&application.AssignedAccountID,
		&application.AssignedSeatID,
		&application.AssignedSubscriptionID,
		&application.OperatorNote,
		&invitedAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.RedemptionApplication{}, err
	}
	application.CustomerEmail = strings.TrimSpace(application.CustomerEmail)
	application.CustomerContact = strings.TrimSpace(application.CustomerContact)
	application.RedeemCode = strings.TrimSpace(application.RedeemCode)
	application.RequestNote = strings.TrimSpace(application.RequestNote)
	application.OperatorNote = strings.TrimSpace(application.OperatorNote)
	if invitedAt.Valid && invitedAt.String != "" {
		parsed, err := parseTime(invitedAt.String)
		if err != nil {
			return model.RedemptionApplication{}, err
		}
		application.InvitedAt = &parsed
	}
	application.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.RedemptionApplication{}, err
	}
	application.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.RedemptionApplication{}, err
	}
	return application, nil
}

func scanRedemptionCode(scanner scannable) (model.RedemptionCode, error) {
	var code model.RedemptionCode
	var usedAt sql.NullString
	var createdAt string
	var updatedAt string
	err := scanner.Scan(
		&code.ID,
		&code.Code,
		&code.Status,
		&code.Note,
		&code.UsedByApplicationID,
		&usedAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.RedemptionCode{}, err
	}
	code.Code = strings.TrimSpace(code.Code)
	code.Status = strings.TrimSpace(code.Status)
	code.Note = strings.TrimSpace(code.Note)
	if usedAt.Valid && usedAt.String != "" {
		parsed, err := parseTime(usedAt.String)
		if err != nil {
			return model.RedemptionCode{}, err
		}
		code.UsedAt = &parsed
	}
	code.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.RedemptionCode{}, err
	}
	code.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.RedemptionCode{}, err
	}
	return code, nil
}

func scanAccount(scanner scannable) (model.Account, error) {
	var account model.Account
	var zeroRenewalNextMonth int
	var createdAt string
	var updatedAt string
	err := scanner.Scan(
		&account.ID,
		&account.Name,
		&account.Remark,
		&account.PaymentMethod,
		&account.Email,
		&account.SpaceName,
		&account.OpenedAt,
		&account.CostCents,
		&zeroRenewalNextMonth,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.Account{}, err
	}
	account.Email = strings.TrimSpace(account.Email)
	account.SpaceName = strings.TrimSpace(account.SpaceName)
	account.OpenedAt = strings.TrimSpace(account.OpenedAt)
	account.ZeroRenewalNextMonth = zeroRenewalNextMonth != 0
	account.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.Account{}, err
	}
	account.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.Account{}, err
	}
	return account, nil
}

func scanSeat(scanner scannable) (model.Seat, error) {
	var seat model.Seat
	var createdAt string
	var updatedAt string
	err := scanner.Scan(
		&seat.ID,
		&seat.AccountID,
		&seat.Name,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.Seat{}, err
	}
	seat.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.Seat{}, err
	}
	seat.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.Seat{}, err
	}
	return seat, nil
}

func scanSubscription(scanner scannable) (model.Subscription, error) {
	var subscription model.Subscription
	var offsetsJSON string
	var channelsJSON string
	var isResale int
	var seatID int64
	var accountID int64
	var accountName string
	var seatName string
	var boardedAt string
	var archivedAt sql.NullString
	var deletedAt sql.NullString
	var createdAt string
	var updatedAt string

	var customerEmail string
	var customerWechat string
	err := scanner.Scan(
		&subscription.ID,
		&subscription.Name,
		&subscription.PricePerPersonCents,
		&subscription.CostCents,
		&isResale,
		&subscription.AgencyFeeCents,
		&subscription.CronExpr,
		&offsetsJSON,
		&channelsJSON,
		&subscription.Remark,
		&subscription.TradeURL,
		&customerEmail,
		&customerWechat,
		&seatID,
		&accountID,
		&accountName,
		&seatName,
		&subscription.SubscriptionType,
		&boardedAt,
		&archivedAt,
		&deletedAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.Subscription{}, err
	}
	subscription.IsResale = isResale != 0
	subscription.CustomerEmail = strings.TrimSpace(customerEmail)
	subscription.CustomerWechat = strings.TrimSpace(customerWechat)
	subscription.SeatID = seatID
	subscription.AccountID = accountID
	subscription.AccountName = strings.TrimSpace(accountName)
	subscription.SeatName = strings.TrimSpace(seatName)
	if subscription.AccountName == "" && subscription.SubscriptionType != "" {
		subscription.AccountName = subscription.SubscriptionType
	}
	if subscription.SubscriptionType == "" {
		if subscription.AccountName != "" {
			subscription.SubscriptionType = subscription.AccountName
		} else {
			subscription.SubscriptionType = model.SubscriptionTypeOther
		}
	}
	subscription.BoardedAt = strings.TrimSpace(boardedAt)

	if err := json.Unmarshal([]byte(offsetsJSON), &subscription.NotifyOffsets); err != nil {
		return model.Subscription{}, fmt.Errorf("decode offsets: %w", err)
	}
	if err := json.Unmarshal([]byte(channelsJSON), &subscription.Channels); err != nil {
		return model.Subscription{}, fmt.Errorf("decode channels: %w", err)
	}
	if archivedAt.Valid && archivedAt.String != "" {
		parsed, err := parseTime(archivedAt.String)
		if err != nil {
			return model.Subscription{}, err
		}
		subscription.ArchivedAt = &parsed
	}
	if deletedAt.Valid && deletedAt.String != "" {
		parsed, err := parseTime(deletedAt.String)
		if err != nil {
			return model.Subscription{}, err
		}
		subscription.DeletedAt = &parsed
	}
	subscription.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.Subscription{}, err
	}
	subscription.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.Subscription{}, err
	}
	return subscription, nil
}

func scanBill(scanner scannable) (model.Bill, error) {
	var bill model.Bill
	var paidAt string
	var createdAt string
	var updatedAt string

	err := scanner.Scan(
		&bill.ID,
		&bill.SubscriptionID,
		&bill.DueDate,
		&bill.AmountCents,
		&bill.Note,
		&paidAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.Bill{}, err
	}
	bill.PaidAt, err = parseTime(paidAt)
	if err != nil {
		return model.Bill{}, err
	}
	bill.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.Bill{}, err
	}
	bill.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.Bill{}, err
	}
	return bill, nil
}

func scanNotificationLog(scanner scannable) (model.NotificationLog, error) {
	var entry model.NotificationLog
	var nextRetryAt sql.NullString
	var createdAt string
	var updatedAt string

	err := scanner.Scan(
		&entry.ID,
		&entry.SubscriptionID,
		&entry.DueDate,
		&entry.OffsetDays,
		&entry.Channel,
		&entry.Status,
		&entry.AttemptCount,
		&nextRetryAt,
		&entry.LastError,
		&entry.Kind,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.NotificationLog{}, err
	}
	if nextRetryAt.Valid && nextRetryAt.String != "" {
		parsed, err := parseTime(nextRetryAt.String)
		if err != nil {
			return model.NotificationLog{}, err
		}
		entry.NextRetryAt = &parsed
	}
	entry.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return model.NotificationLog{}, err
	}
	entry.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return model.NotificationLog{}, err
	}
	return entry, nil
}

func formatTime(moment time.Time) string {
	return moment.UTC().Format(time.RFC3339Nano)
}

// parseTime accepts RFC3339 / RFC3339Nano and SQLite datetime('now') style strings.
func parseTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty time")
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	var lastErr error
	for _, layout := range layouts {
		// SQLite datetime is typically local/wall-clock without zone; treat as UTC for storage consistency.
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed, nil
		} else {
			lastErr = err
		}
	}
	return time.Time{}, fmt.Errorf("parse time %q: %w", value, lastErr)
}
