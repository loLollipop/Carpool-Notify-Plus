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

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"

	_ "modernc.org/sqlite"
)

// Store wraps the SQLite connection and data access methods.
type Store struct {
	database *sql.DB
}

var (
	ErrRedemptionCodeNotFound            = errors.New("redemption code not found")
	ErrRedemptionCodeUsed                = errors.New("redemption code used")
	ErrRedemptionCodeDisabled            = errors.New("redemption code disabled")
	ErrRedemptionCodeNotUnused           = errors.New("redemption code not unused")
	ErrRedemptionAlreadyProcessed        = errors.New("redemption application already processed")
	ErrActiveSeatOccupied                = errors.New("active seat already occupied")
	ErrBillHasAfterSalesCase             = errors.New("bill is referenced by an after-sales case")
	ErrBillOccurrenceConflict            = errors.New("bill occurrence already exists")
	ErrInitialBillNotMovable             = errors.New("initial bill cannot be moved")
	ErrSubscriptionFinancialStateChanged = errors.New("subscription financial state changed")
	ErrSubscriptionHasPendingAfterSales  = errors.New("subscription has a pending after-sales case")
	ErrAfterSalesProcessed               = errors.New("after-sales case already processed")
	ErrAfterSalesRefundExceedsPayment    = errors.New("after-sales refund exceeds remaining payment")
	ErrCancellationPending               = errors.New("subscription cancellation already pending")
	ErrCancellationCaseConflict          = errors.New("subscription already has an after-sales case for this date")
	ErrCancellationNotReassignable       = errors.New("cancellation case cannot be reassigned")
	ErrCustomerBenefitAlreadyRecorded    = errors.New("customer benefit already recorded")
	ErrAfterSalesOriginalSeatBusy        = errors.New("original after-sales seat is occupied")
	ErrReplacementAccountBanned          = errors.New("replacement account is banned")
	ErrReplacementSeatUnavailable        = errors.New("replacement seat unavailable")
	ErrReplacementSeatOccupied           = errors.New("replacement seat is occupied")
	ErrReplacementSeatUnchanged          = errors.New("replacement seat is unchanged")
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
			banned_at TEXT,
			ban_note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS account_cost_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER NOT NULL,
			period_date TEXT NOT NULL,
			amount_cents INTEGER NOT NULL,
			source TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(account_id) REFERENCES accounts(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_account_cost_records_account
			ON account_cost_records(account_id, period_date, id);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_account_cost_records_automatic
			ON account_cost_records(account_id, period_date)
			WHERE source IN ('initial', 'renewal', 'zero_renewal');`,
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
						business_type TEXT NOT NULL DEFAULT 'team',
                        price_per_person_cents INTEGER NOT NULL,
						next_price_cents INTEGER,
						next_price_effective_due_date TEXT NOT NULL DEFAULT '',
                        cron_expr TEXT NOT NULL,
                        notify_offsets TEXT NOT NULL,
                        channels TEXT NOT NULL,
                        remark TEXT NOT NULL DEFAULT '',
						trade_url TEXT NOT NULL DEFAULT '',
						cancellation_requested_at TEXT,
						cancellation_expires_at TEXT,
						cancellation_case_id INTEGER NOT NULL DEFAULT 0,
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
				cost_cents INTEGER NOT NULL DEFAULT 0,
				note TEXT NOT NULL DEFAULT '',
				paid_at TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				UNIQUE(subscription_id, due_date),
				FOREIGN KEY(subscription_id) REFERENCES subscriptions(id)
		);`,
		`CREATE TABLE IF NOT EXISTS after_sales_cases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER,
			subscription_id INTEGER NOT NULL,
			bill_id INTEGER NOT NULL DEFAULT 0,
			business_type TEXT NOT NULL DEFAULT 'team',
			account_name TEXT NOT NULL DEFAULT '',
			account_email TEXT NOT NULL DEFAULT '',
			account_space_name TEXT NOT NULL DEFAULT '',
			customer_email TEXT NOT NULL DEFAULT '',
			customer_wechat TEXT NOT NULL DEFAULT '',
			period_start TEXT NOT NULL DEFAULT '',
			period_end TEXT NOT NULL DEFAULT '',
			banned_date TEXT NOT NULL,
			warranty_days INTEGER NOT NULL DEFAULT 30,
			used_days INTEGER NOT NULL DEFAULT 0,
			remaining_days INTEGER NOT NULL DEFAULT 30,
			paid_amount_cents INTEGER NOT NULL DEFAULT 0,
			refund_amount_cents INTEGER NOT NULL DEFAULT 0,
			replacement_account_id INTEGER NOT NULL DEFAULT 0,
			replacement_seat_id INTEGER NOT NULL DEFAULT 0,
			replacement_account_name TEXT NOT NULL DEFAULT '',
			replacement_account_email TEXT NOT NULL DEFAULT '',
			replacement_space_name TEXT NOT NULL DEFAULT '',
			replacement_seat_name TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT 'account_ban',
			expires_at TEXT,
			status TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			processed_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(account_id, subscription_id, banned_date),
			FOREIGN KEY(account_id) REFERENCES accounts(id),
			FOREIGN KEY(subscription_id) REFERENCES subscriptions(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_after_sales_status
			ON after_sales_cases(status, banned_date DESC, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_after_sales_account
			ON after_sales_cases(account_id, banned_date DESC, id DESC);`,
		`CREATE TABLE IF NOT EXISTS settings (
                        key TEXT PRIMARY KEY,
                        value TEXT NOT NULL
                );`,
		`CREATE TABLE IF NOT EXISTS business_goals (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			target_profit_cents INTEGER NOT NULL,
			baseline_profit_cents INTEGER NOT NULL,
			result_profit_cents INTEGER NOT NULL DEFAULT 0,
			deadline TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			completed_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_business_goals_one_active
			ON business_goals(status) WHERE status = 'active';`,
		`CREATE TABLE IF NOT EXISTS market_price_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider TEXT NOT NULL,
			product TEXT NOT NULL,
			low_price_cents INTEGER NOT NULL,
			median_price_cents INTEGER NOT NULL,
			high_price_cents INTEGER NOT NULL,
			sample_count INTEGER NOT NULL,
			source_updated_at TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_market_price_snapshots_product
			ON market_price_snapshots(provider, product, created_at DESC, id DESC);`,
		`CREATE TABLE IF NOT EXISTS pricing_exemptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			subscription_id INTEGER NOT NULL,
			reason_code TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			review_after TEXT NOT NULL,
			review_cycles INTEGER NOT NULL,
			price_cents_snapshot INTEGER NOT NULL,
			market_median_cents_snapshot INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			FOREIGN KEY(subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_pricing_exemptions_subscription
			ON pricing_exemptions(subscription_id, created_at DESC, id DESC);`,
		`CREATE TABLE IF NOT EXISTS customer_benefits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			batch_id TEXT NOT NULL,
			subscription_id INTEGER NOT NULL,
			benefit_type TEXT NOT NULL,
			benefit_name TEXT NOT NULL,
			actual_cost_cents INTEGER NOT NULL DEFAULT 0 CHECK(actual_cost_cents >= 0),
			perceived_value_cents INTEGER NOT NULL DEFAULT 0 CHECK(perceived_value_cents >= 0),
			benefit_date TEXT NOT NULL,
			next_due_date_snapshot TEXT NOT NULL DEFAULT '',
			customer_email_snapshot TEXT NOT NULL DEFAULT '',
			customer_wechat_snapshot TEXT NOT NULL DEFAULT '',
			customer_tier_snapshot TEXT NOT NULL DEFAULT '',
			customer_group_size_snapshot INTEGER NOT NULL DEFAULT 1 CHECK(customer_group_size_snapshot >= 1),
			current_price_cents_snapshot INTEGER NOT NULL DEFAULT 0 CHECK(current_price_cents_snapshot >= 0),
			renewal_count_snapshot INTEGER NOT NULL DEFAULT 0 CHECK(renewal_count_snapshot >= 0),
			recommendation_code TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			FOREIGN KEY(subscription_id) REFERENCES subscriptions(id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_customer_benefits_subscription
			ON customer_benefits(subscription_id, benefit_date DESC, id DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_customer_benefits_date
			ON customer_benefits(benefit_date DESC, id DESC);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_customer_benefits_delivery
			ON customer_benefits(subscription_id, benefit_date, benefit_type, benefit_name);`,
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
	if err := store.ensureSubscriptionBusinessTypeColumn(); err != nil {
		return err
	}
	if err := store.ensureSubscriptionNextPriceColumns(); err != nil {
		return err
	}
	if err := store.ensureCostCentsColumn(); err != nil {
		return err
	}
	if err := store.ensureBillCostCentsColumn(); err != nil {
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
	if err := store.detachPlusRentalSeats(); err != nil {
		return err
	}
	if err := store.ensureAccountBanColumns(); err != nil {
		return err
	}
	if err := store.ensureAfterSalesReplacementColumns(); err != nil {
		return err
	}
	if err := store.ensureCancellationColumns(); err != nil {
		return err
	}
	if err := store.ensureAfterSalesSourceColumns(); err != nil {
		return err
	}
	if err := store.ensureAfterSalesBusinessTypeColumn(); err != nil {
		return err
	}
	if err := store.ensureActiveSeatOccupancyTriggers(); err != nil {
		return err
	}
	if err := store.backfillAccountCostRecords(); err != nil {
		return err
	}
	if err := store.repairMisdatedInitialAccountCosts(); err != nil {
		return err
	}
	if err := store.repairMisdatedInitialBills(); err != nil {
		return err
	}
	if err := store.normalizeBusinessGoalProfitBaselines(); err != nil {
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

// normalizeBusinessGoalProfitBaselines converts the original incremental-goal
// representation to cumulative-profit goals. Completed rows stored the profit
// earned after the goal was created, so their locked baseline is added once;
// active rows only need the now-unused baseline cleared. The baseline predicate
// makes this migration safe to run on every startup.
func (store *Store) normalizeBusinessGoalProfitBaselines() error {
	_, err := store.database.Exec(`
		UPDATE business_goals
		SET result_profit_cents = CASE
				WHEN status = ? THEN result_profit_cents + baseline_profit_cents
				ELSE result_profit_cents
			END,
			baseline_profit_cents = 0
		WHERE baseline_profit_cents <> 0`,
		model.BusinessGoalStatusCompleted,
	)
	if err != nil {
		return fmt.Errorf("normalize business goal profit baselines: %w", err)
	}
	return nil
}

// repairMisdatedInitialBills fixes the legacy edit bug where a subscription's
// boarded_at/cycle changed after creation but its automatically
// created first bill kept the original date. The created_at equality limits
// this repair to the bill inserted atomically with the subscription; later
// renewal history and after-sales snapshots are never moved.
func (store *Store) repairMisdatedInitialBills() error {
	type repairCandidate struct {
		subscriptionID int64
		billID         int64
		boardedAt      string
		cronExpr       string
		oldDueDate     string
	}

	rows, err := store.database.Query(`
		SELECT s.id, b.id, s.boarded_at, s.cron_expr, b.due_date
		FROM subscriptions AS s
		JOIN bills AS b ON b.subscription_id = s.id
		WHERE s.deleted_at IS NULL
		  AND s.archived_at IS NULL
		  AND b.created_at = s.created_at
		  AND (SELECT COUNT(1) FROM bills AS counted WHERE counted.subscription_id = s.id) = 1
		  AND NOT EXISTS (SELECT 1 FROM after_sales_cases AS linked WHERE linked.bill_id = b.id)`)
	if err != nil {
		return fmt.Errorf("find misdated initial bills: %w", err)
	}
	candidates := make([]repairCandidate, 0)
	for rows.Next() {
		var candidate repairCandidate
		if err := rows.Scan(
			&candidate.subscriptionID,
			&candidate.billID,
			&candidate.boardedAt,
			&candidate.cronExpr,
			&candidate.oldDueDate,
		); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan misdated initial bill: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("list misdated initial bills: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close misdated initial bills: %w", err)
	}

	for _, candidate := range candidates {
		schedule, err := cycle.ParseBillingSchedule(candidate.cronExpr, candidate.boardedAt)
		if err != nil {
			return fmt.Errorf("parse subscription %d schedule for bill repair: %w", candidate.subscriptionID, err)
		}
		boardedAt, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(candidate.boardedAt), cycle.Location)
		if err != nil {
			return fmt.Errorf("parse subscription %d boarded_at for bill repair: %w", candidate.subscriptionID, err)
		}
		expectedDueDate := cycle.FormatDate(
			schedule.NextDue(cycle.StartOfDay(boardedAt).Add(-time.Nanosecond)),
		)
		if expectedDueDate == strings.TrimSpace(candidate.oldDueDate) {
			continue
		}
		result, err := store.database.Exec(`
			UPDATE bills
			SET due_date = ?, updated_at = ?
			WHERE id = ? AND subscription_id = ? AND due_date = ?
			  AND created_at = (SELECT created_at FROM subscriptions WHERE id = ?)
			  AND NOT EXISTS (SELECT 1 FROM after_sales_cases WHERE bill_id = ?)`,
			expectedDueDate,
			formatTime(time.Now().UTC()),
			candidate.billID,
			candidate.subscriptionID,
			candidate.oldDueDate,
			candidate.subscriptionID,
			candidate.billID,
		)
		if err != nil {
			return fmt.Errorf("repair subscription %d initial bill date: %w", candidate.subscriptionID, err)
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("check subscription %d initial bill repair: %w", candidate.subscriptionID, err)
		}
		if rowsAffected != 1 {
			return fmt.Errorf("repair subscription %d initial bill date: %w", candidate.subscriptionID, sql.ErrNoRows)
		}
	}
	return nil
}

func (store *Store) backfillAccountCostRecords() error {
	now := time.Now()
	_, err := store.database.Exec(`
		INSERT INTO account_cost_records (
			account_id, period_date, amount_cents, source, note, created_at
		)
		SELECT id,
			CASE
				WHEN LENGTH(TRIM(opened_at)) = 10 AND date(TRIM(opened_at)) IS NOT NULL
					THEN TRIM(opened_at)
				ELSE ?
			END,
			COALESCE(cost_cents, 0), ?, 'Migrated current account cost', ?
		FROM accounts
		WHERE NOT EXISTS (
			SELECT 1 FROM account_cost_records WHERE account_id = accounts.id
		)`,
		cycle.FormatDate(now),
		model.AccountCostSourceInitial,
		formatTime(now.UTC()),
	)
	if err != nil {
		return fmt.Errorf("backfill account cost records: %w", err)
	}
	return nil
}

// repairMisdatedInitialAccountCosts moves imported single-period owner-account
// costs to the opening period. Older versions used the record/import date,
// which shifted historical profit into the month the account was entered.
// A cumulative historical balance is deliberately kept in its import period.
// Conflicting automatic records are left untouched here; period reporting also
// resolves initial costs by opened_at so a legacy conflict cannot distort profit.
func (store *Store) repairMisdatedInitialAccountCosts() error {
	_, err := store.database.Exec(`
		UPDATE account_cost_records AS cost
		SET period_date = (
			SELECT TRIM(account.opened_at)
			FROM accounts AS account
			WHERE account.id = cost.account_id
		)
		WHERE cost.source = ?
		  AND EXISTS (
			SELECT 1
			FROM accounts AS account
			WHERE account.id = cost.account_id
			  AND LENGTH(TRIM(account.opened_at)) = 10
			  AND date(TRIM(account.opened_at)) IS NOT NULL
			  AND cost.amount_cents = account.cost_cents
			  AND cost.note <> 'Historical cumulative account cost'
			  AND TRIM(account.opened_at) <> cost.period_date
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM account_cost_records AS other
			JOIN accounts AS account ON account.id = cost.account_id
			WHERE other.account_id = cost.account_id
			  AND other.id <> cost.id
			  AND other.period_date = TRIM(account.opened_at)
			  AND other.source IN (?, ?, ?)
		  )`,
		model.AccountCostSourceInitial,
		model.AccountCostSourceInitial,
		model.AccountCostSourceRenewal,
		model.AccountCostSourceZeroRenewal,
	)
	if err != nil {
		return fmt.Errorf("repair initial account cost periods: %w", err)
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

如需续费或有疑问，请联系管理员。
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

func (store *Store) ensureSubscriptionBusinessTypeColumn() error {
	hasColumn, err := store.subscriptionsHasColumn("business_type")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	_, err = store.database.Exec(
		`ALTER TABLE subscriptions ADD COLUMN business_type TEXT NOT NULL DEFAULT 'team'`,
	)
	if err != nil {
		return fmt.Errorf("add business_type: %w", err)
	}
	return nil
}

func (store *Store) ensureSubscriptionNextPriceColumns() error {
	columns := []struct {
		name      string
		statement string
	}{
		{"next_price_cents", `ALTER TABLE subscriptions ADD COLUMN next_price_cents INTEGER`},
		{"next_price_effective_due_date", `ALTER TABLE subscriptions ADD COLUMN next_price_effective_due_date TEXT NOT NULL DEFAULT ''`},
	}
	for _, column := range columns {
		hasColumn, err := store.subscriptionsHasColumn(column.name)
		if err != nil {
			return err
		}
		if hasColumn {
			continue
		}
		if _, err := store.database.Exec(column.statement); err != nil {
			return fmt.Errorf("add subscriptions.%s: %w", column.name, err)
		}
	}
	return nil
}

// detachPlusRentalSeats migrates Plus rentals created by older releases away
// from the Team account/seat model. It is idempotent and releases any legacy
// seat immediately on startup.
func (store *Store) detachPlusRentalSeats() error {
	_, err := store.database.Exec(`
		UPDATE subscriptions
		SET customer_email = CASE
				WHEN TRIM(COALESCE(customer_email, '')) <> '' THEN customer_email
				ELSE COALESCE((
					SELECT COALESCE(NULLIF(TRIM(account.email), ''), account.name)
					FROM seats AS seat
					JOIN accounts AS account ON account.id = seat.account_id
					WHERE seat.id = subscriptions.seat_id
				), '')
			END,
			seat_id = NULL,
			subscription_type = 'Plus 出租'
		WHERE business_type = 'plus' AND seat_id IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("detach Plus rental seats: %w", err)
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

func (store *Store) ensureBillCostCentsColumn() error {
	hasColumn, err := store.tableHasColumn("bills", "cost_cents")
	if err != nil {
		return err
	}
	if hasColumn {
		return nil
	}
	if _, err := store.database.Exec(
		`ALTER TABLE bills ADD COLUMN cost_cents INTEGER NOT NULL DEFAULT 0`,
	); err != nil {
		return fmt.Errorf("add bills.cost_cents: %w", err)
	}
	// Existing Plus bills predate cost snapshots. Seed them from the current
	// rental cost once; all new bills keep their own immutable period value.
	if _, err := store.database.Exec(`
		UPDATE bills
		SET cost_cents = COALESCE((
			SELECT subscription.cost_cents
			FROM subscriptions AS subscription
			WHERE subscription.id = bills.subscription_id
			  AND LOWER(TRIM(COALESCE(subscription.business_type, 'team'))) = 'plus'
		), 0)`); err != nil {
		return fmt.Errorf("backfill Plus bill costs: %w", err)
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

func (store *Store) ensureCancellationColumns() error {
	columns := []struct {
		name      string
		statement string
	}{
		{"cancellation_requested_at", `ALTER TABLE subscriptions ADD COLUMN cancellation_requested_at TEXT`},
		{"cancellation_expires_at", `ALTER TABLE subscriptions ADD COLUMN cancellation_expires_at TEXT`},
		{"cancellation_case_id", `ALTER TABLE subscriptions ADD COLUMN cancellation_case_id INTEGER NOT NULL DEFAULT 0`},
	}
	for _, column := range columns {
		hasColumn, err := store.subscriptionsHasColumn(column.name)
		if err != nil {
			return err
		}
		if hasColumn {
			continue
		}
		if _, err := store.database.Exec(column.statement); err != nil {
			return fmt.Errorf("add subscriptions.%s: %w", column.name, err)
		}
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

func (store *Store) ensureAccountBanColumns() error {
	if _, err := store.ensureAccountColumn(
		"banned_at",
		`ALTER TABLE accounts ADD COLUMN banned_at TEXT`,
	); err != nil {
		return err
	}
	_, err := store.ensureAccountColumn(
		"ban_note",
		`ALTER TABLE accounts ADD COLUMN ban_note TEXT NOT NULL DEFAULT ''`,
	)
	return err
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
	case "bills":
		pragma = `PRAGMA table_info(bills)`
	case "after_sales_cases":
		pragma = `PRAGMA table_info(after_sales_cases)`
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

func (store *Store) ensureAfterSalesReplacementColumns() error {
	columns := []struct {
		name      string
		statement string
	}{
		{"replacement_account_id", `ALTER TABLE after_sales_cases ADD COLUMN replacement_account_id INTEGER NOT NULL DEFAULT 0`},
		{"replacement_seat_id", `ALTER TABLE after_sales_cases ADD COLUMN replacement_seat_id INTEGER NOT NULL DEFAULT 0`},
		{"replacement_account_name", `ALTER TABLE after_sales_cases ADD COLUMN replacement_account_name TEXT NOT NULL DEFAULT ''`},
		{"replacement_account_email", `ALTER TABLE after_sales_cases ADD COLUMN replacement_account_email TEXT NOT NULL DEFAULT ''`},
		{"replacement_space_name", `ALTER TABLE after_sales_cases ADD COLUMN replacement_space_name TEXT NOT NULL DEFAULT ''`},
		{"replacement_seat_name", `ALTER TABLE after_sales_cases ADD COLUMN replacement_seat_name TEXT NOT NULL DEFAULT ''`},
	}
	for _, column := range columns {
		hasColumn, err := store.tableHasColumn("after_sales_cases", column.name)
		if err != nil {
			return err
		}
		if hasColumn {
			continue
		}
		if _, err := store.database.Exec(column.statement); err != nil {
			return fmt.Errorf("add after_sales_cases.%s: %w", column.name, err)
		}
	}
	return nil
}

func (store *Store) ensureAfterSalesSourceColumns() error {
	columns := []struct {
		name      string
		statement string
	}{
		{"source", `ALTER TABLE after_sales_cases ADD COLUMN source TEXT NOT NULL DEFAULT 'account_ban'`},
		{"expires_at", `ALTER TABLE after_sales_cases ADD COLUMN expires_at TEXT`},
	}
	for _, column := range columns {
		hasColumn, err := store.tableHasColumn("after_sales_cases", column.name)
		if err != nil {
			return err
		}
		if hasColumn {
			continue
		}
		if _, err := store.database.Exec(column.statement); err != nil {
			return fmt.Errorf("add after_sales_cases.%s: %w", column.name, err)
		}
	}
	_, err := store.database.Exec(`
		CREATE INDEX IF NOT EXISTS idx_after_sales_expiry
		ON after_sales_cases(source, status, expires_at)`)
	if err != nil {
		return fmt.Errorf("create after-sales expiry index: %w", err)
	}
	return nil
}

func (store *Store) ensureAfterSalesBusinessTypeColumn() error {
	hasColumn, err := store.tableHasColumn("after_sales_cases", "business_type")
	if err != nil {
		return err
	}
	if !hasColumn {
		// Legacy rows require account_id, which prevents Plus rentals (they have
		// no Team owner account) from entering after-sales. Rebuild atomically so
		// account_id can be NULL while retaining the foreign key for Team rows.
		if _, err := store.database.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
			return fmt.Errorf("disable foreign keys for after-sales migration: %w", err)
		}
		migrationErr := func() error {
			transaction, err := store.database.Begin()
			if err != nil {
				return err
			}
			defer func() { _ = transaction.Rollback() }()
			if _, err := transaction.Exec(`
				CREATE TABLE after_sales_cases_new (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					account_id INTEGER,
					subscription_id INTEGER NOT NULL,
					bill_id INTEGER NOT NULL DEFAULT 0,
					business_type TEXT NOT NULL DEFAULT 'team',
					account_name TEXT NOT NULL DEFAULT '',
					account_email TEXT NOT NULL DEFAULT '',
					account_space_name TEXT NOT NULL DEFAULT '',
					customer_email TEXT NOT NULL DEFAULT '',
					customer_wechat TEXT NOT NULL DEFAULT '',
					period_start TEXT NOT NULL DEFAULT '',
					period_end TEXT NOT NULL DEFAULT '',
					banned_date TEXT NOT NULL,
					warranty_days INTEGER NOT NULL DEFAULT 30,
					used_days INTEGER NOT NULL DEFAULT 0,
					remaining_days INTEGER NOT NULL DEFAULT 30,
					paid_amount_cents INTEGER NOT NULL DEFAULT 0,
					refund_amount_cents INTEGER NOT NULL DEFAULT 0,
					replacement_account_id INTEGER NOT NULL DEFAULT 0,
					replacement_seat_id INTEGER NOT NULL DEFAULT 0,
					replacement_account_name TEXT NOT NULL DEFAULT '',
					replacement_account_email TEXT NOT NULL DEFAULT '',
					replacement_space_name TEXT NOT NULL DEFAULT '',
					replacement_seat_name TEXT NOT NULL DEFAULT '',
					source TEXT NOT NULL DEFAULT 'account_ban',
					expires_at TEXT,
					status TEXT NOT NULL,
					note TEXT NOT NULL DEFAULT '',
					processed_at TEXT,
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL,
					UNIQUE(account_id, subscription_id, banned_date),
					FOREIGN KEY(account_id) REFERENCES accounts(id),
					FOREIGN KEY(subscription_id) REFERENCES subscriptions(id)
				)`); err != nil {
				return fmt.Errorf("create migrated after-sales table: %w", err)
			}
			if _, err := transaction.Exec(`
				INSERT INTO after_sales_cases_new (
					id, account_id, subscription_id, bill_id, business_type,
					account_name, account_email, account_space_name, customer_email, customer_wechat,
					period_start, period_end, banned_date, warranty_days, used_days, remaining_days,
					paid_amount_cents, refund_amount_cents,
					replacement_account_id, replacement_seat_id, replacement_account_name,
					replacement_account_email, replacement_space_name, replacement_seat_name,
					source, expires_at, status, note, processed_at, created_at, updated_at
				)
				SELECT old.id, NULLIF(old.account_id, 0), old.subscription_id, old.bill_id,
				       COALESCE(NULLIF((
					       SELECT LOWER(TRIM(subscription.business_type))
					       FROM subscriptions AS subscription
					       WHERE subscription.id = old.subscription_id
				       ), ''), 'team'),
				       old.account_name, old.account_email, old.account_space_name,
				       old.customer_email, old.customer_wechat,
				       old.period_start, old.period_end, old.banned_date,
				       old.warranty_days, old.used_days, old.remaining_days,
				       old.paid_amount_cents, old.refund_amount_cents,
				       old.replacement_account_id, old.replacement_seat_id,
				       old.replacement_account_name, old.replacement_account_email,
				       old.replacement_space_name, old.replacement_seat_name,
				       old.source, old.expires_at, old.status, old.note,
				       old.processed_at, old.created_at, old.updated_at
				FROM after_sales_cases AS old`); err != nil {
				return fmt.Errorf("copy after-sales rows: %w", err)
			}
			if _, err := transaction.Exec(`DROP TABLE after_sales_cases`); err != nil {
				return fmt.Errorf("drop legacy after-sales table: %w", err)
			}
			if _, err := transaction.Exec(`ALTER TABLE after_sales_cases_new RENAME TO after_sales_cases`); err != nil {
				return fmt.Errorf("rename migrated after-sales table: %w", err)
			}
			for _, statement := range []string{
				`CREATE INDEX idx_after_sales_status ON after_sales_cases(status, banned_date DESC, id DESC)`,
				`CREATE INDEX idx_after_sales_account ON after_sales_cases(account_id, banned_date DESC, id DESC)`,
				`CREATE INDEX idx_after_sales_expiry ON after_sales_cases(source, status, expires_at)`,
			} {
				if _, err := transaction.Exec(statement); err != nil {
					return fmt.Errorf("recreate after-sales indexes: %w", err)
				}
			}
			return transaction.Commit()
		}()
		if _, enableErr := store.database.Exec(`PRAGMA foreign_keys = ON`); enableErr != nil && migrationErr == nil {
			migrationErr = fmt.Errorf("re-enable foreign keys after after-sales migration: %w", enableErr)
		}
		if migrationErr != nil {
			return migrationErr
		}
	}

	if _, err := store.database.Exec(`
		UPDATE after_sales_cases
		SET business_type = COALESCE(NULLIF((
			SELECT LOWER(TRIM(subscription.business_type))
			FROM subscriptions AS subscription
			WHERE subscription.id = after_sales_cases.subscription_id
		), ''), 'team')
		WHERE TRIM(COALESCE(business_type, '')) = ''`); err != nil {
		return fmt.Errorf("backfill after-sales business type: %w", err)
	}
	if _, err := store.database.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_after_sales_plus_occurrence
		ON after_sales_cases(subscription_id, banned_date)
		WHERE business_type = 'plus'`); err != nil {
		return fmt.Errorf("create Plus after-sales uniqueness index: %w", err)
	}
	rows, err := store.database.Query(`PRAGMA foreign_key_check(after_sales_cases)`)
	if err != nil {
		return fmt.Errorf("check after-sales foreign keys: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		var tableName string
		var rowID int64
		var parentTable string
		var foreignKeyID int
		if err := rows.Scan(&tableName, &rowID, &parentTable, &foreignKeyID); err != nil {
			return fmt.Errorf("scan after-sales foreign key violation: %w", err)
		}
		return fmt.Errorf(
			"after-sales foreign key violation: table=%s rowid=%d parent=%s key=%d",
			tableName,
			rowID,
			parentTable,
			foreignKeyID,
		)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("check after-sales foreign keys: %w", err)
	}
	return nil
}

// ensureActiveSeatOccupancyTriggers enforces the core invariant that one seat
// can have at most one active subscription. Triggers protect all write paths,
// including concurrent requests that both observed the seat as free.
func (store *Store) ensureActiveSeatOccupancyTriggers() error {
	statements := []string{
		`CREATE TRIGGER IF NOT EXISTS prevent_duplicate_active_seat_insert
		BEFORE INSERT ON subscriptions
		WHEN NEW.seat_id IS NOT NULL
		 AND NEW.deleted_at IS NULL
		 AND NEW.archived_at IS NULL
		 AND EXISTS (
			SELECT 1 FROM subscriptions
			WHERE seat_id = NEW.seat_id
			  AND deleted_at IS NULL
			  AND archived_at IS NULL
		 )
		BEGIN
			SELECT RAISE(ABORT, 'active seat already occupied');
		END;`,
		`CREATE TRIGGER IF NOT EXISTS prevent_duplicate_active_seat_update
		BEFORE UPDATE OF seat_id, archived_at, deleted_at ON subscriptions
		WHEN NEW.seat_id IS NOT NULL
		 AND NEW.deleted_at IS NULL
		 AND NEW.archived_at IS NULL
		 AND EXISTS (
			SELECT 1 FROM subscriptions
			WHERE seat_id = NEW.seat_id
			  AND id <> NEW.id
			  AND deleted_at IS NULL
			  AND archived_at IS NULL
		 )
		BEGIN
			SELECT RAISE(ABORT, 'active seat already occupied');
		END;`,
	}
	for _, statement := range statements {
		if _, err := store.database.Exec(statement); err != nil {
			return fmt.Errorf("create active seat occupancy trigger: %w", err)
		}
	}
	return nil
}

const subscriptionSelectColumns = `
	subscription.id,
	subscription.name,
	COALESCE(subscription.business_type, 'team'),
	subscription.price_per_person_cents,
	subscription.next_price_cents,
	COALESCE(subscription.next_price_effective_due_date, ''),
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
	subscription.cancellation_requested_at,
	subscription.cancellation_expires_at,
	COALESCE(subscription.cancellation_case_id, 0),
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
	return insertSubscription(store.database, subscription, now)
}

// CreateSubscriptionWithInitialBill atomically creates a subscription and its
// first paid billing period. A failure cannot leave a soft-deleted subscription
// or an orphan bill behind.
func (store *Store) CreateSubscriptionWithInitialBill(
	subscription model.Subscription,
	dueDate string,
	amountCents int64,
) (int64, error) {
	now := formatTime(time.Now().UTC())
	transaction, err := store.database.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = transaction.Rollback() }()

	subscriptionID, err := insertSubscription(transaction, subscription, now)
	if err != nil {
		return 0, err
	}
	if err := insertInitialBill(
		transaction,
		subscriptionID,
		dueDate,
		amountCents,
		storedBillCostCents(subscription),
		now,
	); err != nil {
		return 0, err
	}
	if err := transaction.Commit(); err != nil {
		return 0, err
	}
	return subscriptionID, nil
}

type sqlExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func insertSubscription(
	executor sqlExecer,
	subscription model.Subscription,
	now string,
) (int64, error) {
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
	businessType := normalizeStoredBusinessType(subscription.BusinessType)
	boardedAt := strings.TrimSpace(subscription.BoardedAt)
	var seatID interface{}
	if subscription.SeatID > 0 {
		seatID = subscription.SeatID
	}
	isResale := 0
	if subscription.IsResale {
		isResale = 1
	}
	result, err := executor.Exec(`
                INSERT INTO subscriptions (
                        name, business_type, price_per_person_cents, next_price_cents, next_price_effective_due_date,
						cost_cents, is_resale, agency_fee_cents, cron_expr, notify_offsets, channels,
                        remark, trade_url, customer_email, customer_wechat, subscription_type, seat_id, boarded_at, archived_at, deleted_at, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)`,
		subscription.Name,
		businessType,
		subscription.PricePerPersonCents,
		subscription.NextPriceCents,
		strings.TrimSpace(subscription.NextPriceEffectiveDueDate),
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
		if isActiveSeatOccupancyError(err) {
			return 0, ErrActiveSeatOccupied
		}
		return 0, err
	}
	return result.LastInsertId()
}

func insertInitialBill(
	executor sqlExecer,
	subscriptionID int64,
	dueDate string,
	amountCents int64,
	costCents int64,
	now string,
) error {
	_, err := executor.Exec(`
		INSERT INTO bills (
			subscription_id, due_date, amount_cents, cost_cents, note, paid_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, '', ?, ?, ?)`,
		subscriptionID,
		strings.TrimSpace(dueDate),
		amountCents,
		costCents,
		now,
		now,
		now,
	)
	return err
}

func isActiveSeatOccupancyError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), ErrActiveSeatOccupied.Error())
}

// UpdateSubscription updates an existing active (non-deleted, non-archived) subscription.
func (store *Store) UpdateSubscription(subscription model.Subscription) error {
	now := formatTime(time.Now().UTC())
	err := updateSubscriptionWithExecutor(store.database, subscription, now)
	if err != sql.ErrNoRows {
		return err
	}
	pendingCount, pendingErr := store.CountPendingAfterSalesCasesBySubscription(subscription.ID)
	if pendingErr != nil {
		return pendingErr
	}
	if pendingCount > 0 {
		return ErrSubscriptionHasPendingAfterSales
	}
	return sql.ErrNoRows
}

// UpdateSubscriptionAndSyncBill atomically updates an active subscription and,
// when present, the bill for the current billing period. Bills referenced by an
// after-sales snapshot remain immutable while the subscription update proceeds.
func (store *Store) UpdateSubscriptionAndSyncBill(
	subscription model.Subscription,
	dueDate string,
	amountCents int64,
	costCents int64,
) error {
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	now := formatTime(time.Now().UTC())
	if err := updateSubscriptionWithExecutor(transaction, subscription, now); err != nil {
		if err == sql.ErrNoRows {
			var pendingCount int
			if pendingErr := transaction.QueryRow(`
				SELECT COUNT(1)
				FROM after_sales_cases
				WHERE subscription_id = ? AND status IN (?, ?)`,
				subscription.ID,
				model.AfterSalesStatusPending,
				model.AfterSalesStatusReview,
			).Scan(&pendingCount); pendingErr != nil {
				return pendingErr
			}
			if pendingCount > 0 {
				return ErrSubscriptionHasPendingAfterSales
			}
		}
		return err
	}
	if err := updateBillFinancialsForOccurrence(
		transaction,
		subscription.ID,
		dueDate,
		amountCents,
		costCents,
		now,
	); err != nil {
		return err
	}
	return transaction.Commit()
}

// UpdateSubscriptionNextPrices atomically updates only future pricing fields.
// Guarding eligibility again inside the transaction prevents a concurrent
// archive or after-sales case from producing a partial bulk update.
func (store *Store) UpdateSubscriptionNextPrices(subscriptions []model.Subscription, reviewDates ...string) error {
	if len(subscriptions) == 0 {
		return nil
	}
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	now := formatTime(time.Now().UTC())
	today := cycle.FormatDate(time.Now().In(cycle.Location))
	if len(reviewDates) > 0 && strings.TrimSpace(reviewDates[0]) != "" {
		today = strings.TrimSpace(reviewDates[0])
	}
	for _, subscription := range subscriptions {
		if subscription.NextPriceCents == nil || strings.TrimSpace(subscription.NextPriceEffectiveDueDate) == "" {
			return fmt.Errorf("subscription %d has incomplete next price", subscription.ID)
		}
		result, updateErr := transaction.Exec(`
			UPDATE subscriptions
			SET next_price_cents = ?, next_price_effective_due_date = ?, updated_at = ?
			WHERE id = ?
			  AND deleted_at IS NULL
			  AND archived_at IS NULL
			  AND LOWER(TRIM(COALESCE(business_type, 'team'))) = ?
			  AND COALESCE(is_resale, 0) = 0
			  AND price_per_person_cents = ?
			  AND cron_expr = ?
			  AND boarded_at = ?
			  AND seat_id = ?
			  AND next_price_cents IS NULL
			  AND NOT EXISTS (
				SELECT 1
				FROM after_sales_cases
				WHERE subscription_id = subscriptions.id
				  AND status IN (?, ?)
			  )
			  AND NOT EXISTS (
				SELECT 1
				FROM pricing_exemptions
				WHERE subscription_id = subscriptions.id
				  AND review_after > ?
			  )`,
			*subscription.NextPriceCents,
			strings.TrimSpace(subscription.NextPriceEffectiveDueDate),
			now,
			subscription.ID,
			model.SubscriptionBusinessTeam,
			subscription.PricePerPersonCents,
			subscription.CronExpr,
			strings.TrimSpace(subscription.BoardedAt),
			subscription.SeatID,
			model.AfterSalesStatusPending,
			model.AfterSalesStatusReview,
			today,
		)
		if updateErr != nil {
			return updateErr
		}
		affected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		if affected != 1 {
			return sql.ErrNoRows
		}
	}
	return transaction.Commit()
}

// UpdateSubscriptionAndMoveInitialBill atomically updates a subscription's
// schedule and re-keys its single initial bill to the corrected first period.
func (store *Store) UpdateSubscriptionAndMoveInitialBill(
	subscription model.Subscription,
	oldDueDate string,
	newDueDate string,
	amountCents int64,
	costCents int64,
) error {
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	var (
		billCount     int
		billID        int64
		storedDueDate string
	)
	err = transaction.QueryRow(`
		SELECT COUNT(1), COALESCE(MIN(id), 0), COALESCE(MIN(due_date), '')
		FROM bills
		WHERE subscription_id = ?`,
		subscription.ID,
	).Scan(&billCount, &billID, &storedDueDate)
	if err != nil {
		return err
	}
	if billCount != 1 || storedDueDate != strings.TrimSpace(oldDueDate) {
		return ErrInitialBillNotMovable
	}
	now := formatTime(time.Now().UTC())
	if strings.TrimSpace(oldDueDate) != strings.TrimSpace(newDueDate) {
		if err := ensureBillUnreferenced(transaction, billID); err != nil {
			return err
		}
		result, moveErr := transaction.Exec(`
			UPDATE bills
			SET due_date = ?, updated_at = ?
			WHERE id = ?`,
			strings.TrimSpace(newDueDate),
			now,
			billID,
		)
		if moveErr != nil {
			if strings.Contains(strings.ToLower(moveErr.Error()), "unique") {
				return ErrBillOccurrenceConflict
			}
			return moveErr
		}
		rowsAffected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		if rowsAffected != 1 {
			return sql.ErrNoRows
		}
	}

	if err := updateSubscriptionWithExecutor(transaction, subscription, now); err != nil {
		if err == sql.ErrNoRows {
			var pendingCount int
			if pendingErr := transaction.QueryRow(`
				SELECT COUNT(1)
				FROM after_sales_cases
				WHERE subscription_id = ? AND status IN (?, ?)`,
				subscription.ID,
				model.AfterSalesStatusPending,
				model.AfterSalesStatusReview,
			).Scan(&pendingCount); pendingErr != nil {
				return pendingErr
			}
			if pendingCount > 0 {
				return ErrSubscriptionHasPendingAfterSales
			}
		}
		return err
	}
	if err := updateBillFinancialsByID(
		transaction,
		billID,
		amountCents,
		costCents,
		now,
	); err != nil {
		return err
	}
	return transaction.Commit()
}

func updateBillFinancialsForOccurrence(
	executor sqlExecer,
	subscriptionID int64,
	dueDate string,
	amountCents int64,
	costCents int64,
	now string,
) error {
	_, err := executor.Exec(`
		UPDATE bills
		SET amount_cents = ?, cost_cents = ?, updated_at = ?
		WHERE subscription_id = ? AND due_date = ?
		  AND NOT EXISTS (
			SELECT 1 FROM after_sales_cases
			WHERE bill_id = bills.id
		  )`,
		amountCents,
		costCents,
		now,
		subscriptionID,
		strings.TrimSpace(dueDate),
	)
	return err
}

func updateBillFinancialsByID(
	executor sqlExecer,
	billID int64,
	amountCents int64,
	costCents int64,
	now string,
) error {
	_, err := executor.Exec(`
		UPDATE bills
		SET amount_cents = ?, cost_cents = ?, updated_at = ?
		WHERE id = ?
		  AND NOT EXISTS (
			SELECT 1 FROM after_sales_cases
			WHERE bill_id = bills.id
		  )`,
		amountCents,
		costCents,
		now,
		billID,
	)
	return err
}

func updateSubscriptionWithExecutor(
	executor sqlExecer,
	subscription model.Subscription,
	now string,
) error {
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
	businessType := normalizeStoredBusinessType(subscription.BusinessType)
	boardedAt := strings.TrimSpace(subscription.BoardedAt)
	var seatID interface{}
	if subscription.SeatID > 0 {
		seatID = subscription.SeatID
	}
	isResale := 0
	if subscription.IsResale {
		isResale = 1
	}
	result, err := executor.Exec(`
                UPDATE subscriptions
                SET name = ?, business_type = ?, price_per_person_cents = ?, next_price_cents = ?, next_price_effective_due_date = ?,
					cost_cents = ?, is_resale = ?, agency_fee_cents = ?, cron_expr = ?, notify_offsets = ?,
                    channels = ?, remark = ?, trade_url = ?, customer_email = ?, customer_wechat = ?, subscription_type = ?, seat_id = ?, boarded_at = ?, updated_at = ?
                WHERE id = ? AND deleted_at IS NULL AND archived_at IS NULL
                  AND NOT EXISTS (
					SELECT 1
					FROM after_sales_cases
					WHERE subscription_id = subscriptions.id
					  AND status IN (?, ?)
				  )`,
		subscription.Name,
		businessType,
		subscription.PricePerPersonCents,
		subscription.NextPriceCents,
		strings.TrimSpace(subscription.NextPriceEffectiveDueDate),
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
		model.AfterSalesStatusPending,
		model.AfterSalesStatusReview,
	)
	if err != nil {
		if isActiveSeatOccupancyError(err) {
			return ErrActiveSeatOccupied
		}
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

// ArchiveSubscription marks a subscription as archived (下车) and removes any
// redemption application/code records that created this subscription.
// Works for active non-deleted subscriptions; already-archived is a no-op success.
func (store *Store) ArchiveSubscription(subscriptionID int64) error {
	now := formatTime(time.Now().UTC())
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() {
		_ = transaction.Rollback()
	}()
	var pendingAfterSalesCount int
	if err := transaction.QueryRow(`
		SELECT COUNT(1)
		FROM after_sales_cases
		WHERE subscription_id = ? AND status IN (?, ?)`,
		subscriptionID,
		model.AfterSalesStatusPending,
		model.AfterSalesStatusReview,
	).Scan(&pendingAfterSalesCount); err != nil {
		return err
	}
	if pendingAfterSalesCount > 0 {
		return ErrSubscriptionHasPendingAfterSales
	}

	if err := archiveSubscriptionInTransaction(transaction, subscriptionID, now, 0); err != nil {
		return err
	}

	return transaction.Commit()
}

// SetDuePaid marks or unmarks one subscription due date as paid via bills.
// When paid=true, creates a bill with the given amount if none exists (keeps existing bill).
// When paid=false, deletes the bill for that occurrence.
func (store *Store) SetDuePaid(
	subscriptionID int64,
	dueDate string,
	paid bool,
	amountCents int64,
	costSnapshotCents ...int64,
) error {
	return store.setDuePaid(subscriptionID, dueDate, paid, amountCents, nil, costSnapshotCents...)
}

// SetDuePaidForSubscription records a bill only if the active subscription is
// still the same snapshot used to calculate its amount. This prevents a stale
// browser request from posting an old price after a concurrent edit.
func (store *Store) SetDuePaidForSubscription(
	subscription model.Subscription,
	dueDate string,
	paid bool,
	amountCents int64,
	costSnapshotCents ...int64,
) error {
	expectedUpdatedAt := subscription.UpdatedAt
	return store.setDuePaid(
		subscription.ID,
		dueDate,
		paid,
		amountCents,
		&expectedUpdatedAt,
		costSnapshotCents...,
	)
}

func (store *Store) setDuePaid(
	subscriptionID int64,
	dueDate string,
	paid bool,
	amountCents int64,
	expectedUpdatedAt *time.Time,
	costSnapshotCents ...int64,
) error {
	if !paid {
		bill, err := store.GetBillByOccurrence(subscriptionID, dueDate)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		return store.deleteBillIfUnreferenced(bill.ID)
	}

	existing, err := store.GetBillByOccurrence(subscriptionID, dueDate)
	if err == nil && existing.ID > 0 {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	costCents := int64(0)
	if len(costSnapshotCents) > 0 && costSnapshotCents[0] > 0 {
		costCents = costSnapshotCents[0]
	}
	now := formatTime(time.Now().UTC())
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	if expectedUpdatedAt != nil {
		var storedUpdatedAt string
		err := transaction.QueryRow(`
			SELECT updated_at
			FROM subscriptions AS subscription
			WHERE subscription.id = ?
			  AND subscription.deleted_at IS NULL
			  AND subscription.archived_at IS NULL
			  AND NOT EXISTS (
				SELECT 1
				FROM after_sales_cases AS after_sales
				WHERE after_sales.subscription_id = subscription.id
				  AND after_sales.status IN (?, ?)
			  )`,
			subscriptionID,
			model.AfterSalesStatusPending,
			model.AfterSalesStatusReview,
		).Scan(&storedUpdatedAt)
		if err != nil {
			if err == sql.ErrNoRows {
				return ErrSubscriptionFinancialStateChanged
			}
			return err
		}
		if storedUpdatedAt != formatTime(expectedUpdatedAt.UTC()) {
			return ErrSubscriptionFinancialStateChanged
		}
	}
	result, err := transaction.Exec(`
		INSERT INTO bills (
			subscription_id, due_date, amount_cents, cost_cents, note, paid_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, '', ?, ?, ?)
		ON CONFLICT(subscription_id, due_date) DO NOTHING`,
		subscriptionID, dueDate, amountCents, costCents, now, now, now,
	)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 1 {
		// A scheduled price becomes the regular price only after the first bill
		// in its effective period is actually recorded. Removing that bill later
		// deliberately does not rewrite the price or any historical bill.
		if _, err := transaction.Exec(`
			UPDATE subscriptions
			SET price_per_person_cents = next_price_cents,
				next_price_cents = NULL,
				next_price_effective_due_date = '',
				updated_at = ?
			WHERE id = ?
			  AND is_resale = 0
			  AND next_price_cents IS NOT NULL
			  AND next_price_effective_due_date <> ''
			  AND next_price_effective_due_date <= ?`,
			now,
			subscriptionID,
			strings.TrimSpace(dueDate),
		); err != nil {
			return err
		}
	}
	return transaction.Commit()
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

// ListPaidDueDatesForSubscription returns every recorded paid period in
// chronological order. Keeping the full set lets callers detect an unpaid gap
// between two later payments instead of only looking at the latest period.
func (store *Store) ListPaidDueDatesForSubscription(subscriptionID int64) ([]string, error) {
	rows, err := store.database.Query(`
		SELECT due_date
		FROM bills
		WHERE subscription_id = ?
		ORDER BY due_date ASC, id ASC`, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dueDates := make([]string, 0)
	for rows.Next() {
		var dueDate string
		if err := rows.Scan(&dueDate); err != nil {
			return nil, err
		}
		dueDates = append(dueDates, strings.TrimSpace(dueDate))
	}
	return dueDates, rows.Err()
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
		SELECT id, subscription_id, due_date, amount_cents, cost_cents, note, paid_at, created_at, updated_at
		FROM bills
		WHERE id = ?`, billID)
	return scanBill(row)
}

// GetBillByOccurrence returns the bill for one subscription due date.
func (store *Store) GetBillByOccurrence(subscriptionID int64, dueDate string) (model.Bill, error) {
	row := store.database.QueryRow(`
		SELECT id, subscription_id, due_date, amount_cents, cost_cents, note, paid_at, created_at, updated_at
		FROM bills
		WHERE subscription_id = ? AND due_date = ?`,
		subscriptionID, dueDate,
	)
	return scanBill(row)
}

// ListBills returns all bills newest-paid first.
func (store *Store) ListBills() ([]model.Bill, error) {
	rows, err := store.database.Query(`
		SELECT id, subscription_id, due_date, amount_cents, cost_cents, note, paid_at, created_at, updated_at
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

// UpdateBill updates amount and note for a bill that is not part of an
// after-sales snapshot. Referenced bills are immutable financial history.
func (store *Store) UpdateBill(billID int64, amountCents int64, note string) error {
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	if err := ensureBillUnreferenced(transaction, billID); err != nil {
		return err
	}
	now := formatTime(time.Now().UTC())
	result, err := transaction.Exec(`
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
	return transaction.Commit()
}

// UpdateBillFinancials updates the current period snapshot after an operator
// corrects a subscription's rent or cost. Referenced historical bills stay
// immutable once they participate in after-sales.
func (store *Store) UpdateBillFinancials(
	billID int64,
	amountCents int64,
	costCents int64,
	note string,
) error {
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	if err := ensureBillUnreferenced(transaction, billID); err != nil {
		return err
	}
	now := formatTime(time.Now().UTC())
	result, err := transaction.Exec(`
		UPDATE bills
		SET amount_cents = ?, cost_cents = ?, note = ?, updated_at = ?
		WHERE id = ?`,
		amountCents,
		costCents,
		note,
		now,
		billID,
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
	return transaction.Commit()
}

// DeleteBill removes one bill by id (same effect as unmarking that due as paid).
func (store *Store) DeleteBill(billID int64) error {
	return store.deleteBillIfUnreferenced(billID)
}

func (store *Store) deleteBillIfUnreferenced(billID int64) error {
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	if err := ensureBillUnreferenced(transaction, billID); err != nil {
		return err
	}
	result, err := transaction.Exec(`DELETE FROM bills WHERE id = ?`, billID)
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
	return transaction.Commit()
}

func ensureBillUnreferenced(transaction *sql.Tx, billID int64) error {
	var afterSalesCount int
	if err := transaction.QueryRow(`
		SELECT COUNT(1)
		FROM after_sales_cases
		WHERE bill_id = ?`, billID).Scan(&afterSalesCount); err != nil {
		return err
	}
	if afterSalesCount > 0 {
		return ErrBillHasAfterSalesCase
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

// SetSettings upserts a group of settings in one transaction so the settings
// page never exposes a mixture of old and new templates after a write failure.
func (store *Store) SetSettings(values map[string]string) error {
	if len(values) == 0 {
		return nil
	}
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	for key, value := range values {
		if _, err := transaction.Exec(`
			INSERT INTO settings(key, value) VALUES(?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			key,
			value,
		); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

// ResetBusinessData clears operational rows while preserving settings.
// It is used only by the isolated rehearsal database.
func (store *Store) ResetBusinessData() error {
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	tables := []string{
		"redemption_codes",
		"redemption_applications",
		"after_sales_cases",
		"notification_log",
		"bills",
		"paid_due_occurrences",
		"customer_benefits",
		"pricing_exemptions",
		"subscriptions",
		"seats",
		"account_cost_records",
		"accounts",
	}
	for _, table := range tables {
		if _, err := transaction.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("reset %s: %w", table, err)
		}
	}
	for _, table := range tables {
		if _, err := transaction.Exec(`DELETE FROM sqlite_sequence WHERE name = ?`, table); err != nil {
			return fmt.Errorf("reset sequence %s: %w", table, err)
		}
	}
	return transaction.Commit()
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

// CreateSubscriptionAndInviteRedemption atomically creates the assigned
// subscription and first bill, then marks the pending application invited.
// Concurrent handling can therefore never leave an extra subscription or bill.
func (store *Store) CreateSubscriptionAndInviteRedemption(
	applicationID int64,
	accountID int64,
	seatID int64,
	subscription model.Subscription,
	dueDate string,
	amountCents int64,
	operatorNote string,
) (int64, error) {
	now := formatTime(time.Now().UTC())
	transaction, err := store.database.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = transaction.Rollback() }()

	var applicationStatus string
	if err := transaction.QueryRow(`
		SELECT status
		FROM redemption_applications
		WHERE id = ?`, applicationID).Scan(&applicationStatus); err != nil {
		return 0, err
	}
	if applicationStatus != model.RedemptionStatusPending {
		return 0, ErrRedemptionAlreadyProcessed
	}

	var actualAccountID int64
	var bannedAt string
	if err := transaction.QueryRow(`
		SELECT seat.account_id, COALESCE(account.banned_at, '')
		FROM seats AS seat
		INNER JOIN accounts AS account ON account.id = seat.account_id
		WHERE seat.id = ?`, seatID).Scan(&actualAccountID, &bannedAt); err != nil {
		return 0, err
	}
	if actualAccountID != accountID || subscription.SeatID != seatID {
		return 0, ErrReplacementSeatUnavailable
	}
	if strings.TrimSpace(bannedAt) != "" {
		return 0, ErrReplacementAccountBanned
	}

	subscriptionID, err := insertSubscription(transaction, subscription, now)
	if err != nil {
		return 0, err
	}
	if err := insertInitialBill(
		transaction,
		subscriptionID,
		dueDate,
		amountCents,
		storedBillCostCents(subscription),
		now,
	); err != nil {
		return 0, err
	}

	result, err := transaction.Exec(`
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
		return 0, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if rowsAffected == 0 {
		return 0, ErrRedemptionAlreadyProcessed
	}
	if err := transaction.Commit(); err != nil {
		return 0, err
	}
	return subscriptionID, nil
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
	COALESCE((
		SELECT SUM(amount_cents)
		FROM account_cost_records
		WHERE account_id = accounts.id
	), 0),
	COALESCE(zero_renewal_next_month, 0),
	COALESCE(banned_at, ''),
	COALESCE(ban_note, ''),
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

// CreateAccount inserts a new account and its initial cumulative cost entry.
func (store *Store) CreateAccount(account model.Account, initialCostCents int64, periodDate string) (int64, error) {
	now := formatTime(time.Now().UTC())
	zeroRenewalNextMonth := 0
	if account.ZeroRenewalNextMonth {
		zeroRenewalNextMonth = 1
	}
	transaction, err := store.database.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = transaction.Rollback() }()

	result, err := transaction.Exec(`
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
	accountID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	initialCostNote := "Initial account cost"
	if initialCostCents != account.CostCents {
		initialCostNote = "Historical cumulative account cost"
	}
	if _, err := transaction.Exec(`
		INSERT INTO account_cost_records (
			account_id, period_date, amount_cents, source, note, created_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		accountID,
		periodDate,
		initialCostCents,
		model.AccountCostSourceInitial,
		initialCostNote,
		now,
	); err != nil {
		return 0, err
	}
	if err := transaction.Commit(); err != nil {
		return 0, err
	}
	return accountID, nil
}

// UpdateAccount updates account metadata and optionally adjusts cumulative cost to a target.
func (store *Store) UpdateAccount(account model.Account, targetTotalCostCents *int64, periodDate string) error {
	now := formatTime(time.Now().UTC())
	zeroRenewalNextMonth := 0
	if account.ZeroRenewalNextMonth {
		zeroRenewalNextMonth = 1
	}
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	var previousMonthlyCostCents int64
	if err := transaction.QueryRow(
		`SELECT cost_cents FROM accounts WHERE id = ?`,
		account.ID,
	).Scan(&previousMonthlyCostCents); err != nil {
		return err
	}

	result, err := transaction.Exec(`
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
	if openedAt := strings.TrimSpace(account.OpenedAt); openedAt != "" {
		if _, err := transaction.Exec(`
			UPDATE account_cost_records AS cost
			SET period_date = ?
			WHERE cost.account_id = ?
			  AND cost.source = ?
			  AND cost.amount_cents IN (?, ?)
			  AND cost.note <> 'Historical cumulative account cost'
			  AND cost.period_date <> ?
			  AND NOT EXISTS (
				SELECT 1
				FROM account_cost_records AS other
				WHERE other.account_id = cost.account_id
				  AND other.id <> cost.id
				  AND other.period_date = ?
				  AND other.source IN (?, ?, ?)
			  )`,
			openedAt,
			account.ID,
			model.AccountCostSourceInitial,
			previousMonthlyCostCents,
			account.CostCents,
			openedAt,
			openedAt,
			model.AccountCostSourceInitial,
			model.AccountCostSourceRenewal,
			model.AccountCostSourceZeroRenewal,
		); err != nil {
			return fmt.Errorf("align initial account cost period: %w", err)
		}
	}
	if targetTotalCostCents != nil {
		var currentTotal int64
		if err := transaction.QueryRow(`
			SELECT COALESCE(SUM(amount_cents), 0)
			FROM account_cost_records
			WHERE account_id = ?`, account.ID).Scan(&currentTotal); err != nil {
			return err
		}
		adjustment := *targetTotalCostCents - currentTotal
		if adjustment != 0 {
			if _, err := transaction.Exec(`
				INSERT INTO account_cost_records (
					account_id, period_date, amount_cents, source, note, created_at
				) VALUES (?, ?, ?, ?, ?, ?)`,
				account.ID,
				periodDate,
				adjustment,
				model.AccountCostSourceManual,
				"Manual cumulative cost correction",
				now,
			); err != nil {
				return err
			}
		}
	}
	return transaction.Commit()
}

// LatestAutomaticAccountCostPeriod returns the latest initialized or renewed period.
func (store *Store) LatestAutomaticAccountCostPeriod(accountID int64) (string, error) {
	var periodDate string
	err := store.database.QueryRow(`
		SELECT COALESCE(MAX(period_date), '')
		FROM account_cost_records
		WHERE account_id = ? AND source IN (?, ?, ?)`,
		accountID,
		model.AccountCostSourceInitial,
		model.AccountCostSourceRenewal,
		model.AccountCostSourceZeroRenewal,
	).Scan(&periodDate)
	return periodDate, err
}

// AccrueAccountRenewal inserts one idempotent renewal and consumes the $0 flag once.
func (store *Store) AccrueAccountRenewal(accountID int64, periodDate string) (bool, error) {
	transaction, err := store.database.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = transaction.Rollback() }()

	var monthlyCostCents int64
	var zeroRenewal int
	var bannedAt string
	if err := transaction.QueryRow(`
		SELECT COALESCE(cost_cents, 0), COALESCE(zero_renewal_next_month, 0), COALESCE(banned_at, '')
		FROM accounts
		WHERE id = ?`, accountID).Scan(&monthlyCostCents, &zeroRenewal, &bannedAt); err != nil {
		return false, err
	}
	if strings.TrimSpace(bannedAt) != "" {
		return false, transaction.Commit()
	}
	amountCents := monthlyCostCents
	source := model.AccountCostSourceRenewal
	note := "Monthly account renewal"
	if zeroRenewal != 0 {
		amountCents = 0
		source = model.AccountCostSourceZeroRenewal
		note = "One-time zero-cost account renewal"
	}
	result, err := transaction.Exec(`
		INSERT OR IGNORE INTO account_cost_records (
			account_id, period_date, amount_cents, source, note, created_at
		) VALUES (?, ?, ?, ?, ?, ?)`,
		accountID,
		periodDate,
		amountCents,
		source,
		note,
		formatTime(time.Now().UTC()),
	)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	inserted := rowsAffected > 0
	if inserted && zeroRenewal != 0 {
		if _, err := transaction.Exec(`
			UPDATE accounts
			SET zero_renewal_next_month = 0, updated_at = ?
			WHERE id = ?`, formatTime(time.Now().UTC()), accountID); err != nil {
			return false, err
		}
	}
	if err := transaction.Commit(); err != nil {
		return false, err
	}
	return inserted, nil
}

// ListAccountCostRecords returns one account's ledger in chronological order.
func (store *Store) ListAccountCostRecords(accountID int64) ([]model.AccountCostRecord, error) {
	rows, err := store.database.Query(`
		SELECT id, account_id, period_date, amount_cents, source, note, created_at
		FROM account_cost_records
		WHERE account_id = ?
		ORDER BY period_date ASC, id ASC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]model.AccountCostRecord, 0)
	for rows.Next() {
		var record model.AccountCostRecord
		var createdAt string
		if err := rows.Scan(
			&record.ID,
			&record.AccountID,
			&record.PeriodDate,
			&record.AmountCents,
			&record.Source,
			&record.Note,
			&createdAt,
		); err != nil {
			return nil, err
		}
		record.CreatedAt, err = parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
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

// MarkNotificationCanceled closes a pending notification without counting it
// as either a successful delivery or a failure.
func (store *Store) MarkNotificationCanceled(logID int64) error {
	now := formatTime(time.Now().UTC())
	_, err := store.database.Exec(`
		UPDATE notification_log
		SET status = ?, next_retry_at = NULL, last_error = '', updated_at = ?
		WHERE id = ? AND status = ?`,
		model.NotificationStatusCanceled,
		now,
		logID,
		model.NotificationStatusPending,
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
				SELECT log.id, log.subscription_id, log.due_date, log.offset_days, log.channel,
				       log.status, log.attempt_count, log.next_retry_at, log.last_error,
				       log.kind, log.created_at, log.updated_at
				FROM notification_log AS log
				INNER JOIN subscriptions AS subscription ON subscription.id = log.subscription_id
				WHERE log.kind IN (?, ?)
				  AND log.status = ?
				  AND subscription.deleted_at IS NULL
				  AND subscription.archived_at IS NULL
				  AND NOT EXISTS (
					SELECT 1
					FROM after_sales_cases AS after_sales
					WHERE after_sales.subscription_id = subscription.id
					  AND after_sales.status IN (?, ?)
				  )
				  AND (log.next_retry_at IS NULL OR log.next_retry_at <= ?)`,
		model.NotificationKindScheduled,
		model.NotificationKindPriceIncreaseNotice,
		model.NotificationStatusPending,
		model.AfterSalesStatusPending,
		model.AfterSalesStatusReview,
		nowText,
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
                  AND log.kind IN (?, ?)
                  AND log.status = ?`,
		model.NotificationKindScheduled, model.NotificationKindPriceIncreaseNotice, model.NotificationStatusFailed,
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
                  AND log.kind IN (?, ?)
                  AND log.status = ?
                  AND log.updated_at >= ?`,
		model.NotificationKindScheduled, model.NotificationKindPriceIncreaseNotice, status, formatTime(since.UTC()),
	).Scan(&count)
	return count, err
}

// ListNotificationActivitySince returns completed scheduled notification rows
// for active subscriptions. The result uses the same scope as the dashboard
// success/failure counters so UI detail rows always reconcile with the KPIs.
func (store *Store) ListNotificationActivitySince(since time.Time) ([]model.NotificationLog, error) {
	rows, err := store.database.Query(`
		SELECT log.id, log.subscription_id, log.due_date, log.offset_days, log.channel,
		       log.status, log.attempt_count, log.next_retry_at, log.last_error,
		       log.kind, log.created_at, log.updated_at
		FROM notification_log AS log
		INNER JOIN subscriptions AS subscription ON subscription.id = log.subscription_id
		WHERE subscription.deleted_at IS NULL
		  AND subscription.archived_at IS NULL
		  AND log.kind IN (?, ?)
		  AND log.status IN (?, ?)
		  AND log.updated_at >= ?
		ORDER BY log.updated_at DESC, log.id DESC`,
		model.NotificationKindScheduled,
		model.NotificationKindPriceIncreaseNotice,
		model.NotificationStatusSuccess,
		model.NotificationStatusFailed,
		formatTime(since.UTC()),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	activities := make([]model.NotificationLog, 0)
	for rows.Next() {
		activity, err := scanNotificationLog(rows)
		if err != nil {
			return nil, err
		}
		activities = append(activities, activity)
	}
	return activities, rows.Err()
}

// LatestErrorsBySubscription returns the latest error message per active subscription.
func (store *Store) LatestErrorsBySubscription() (map[int64]string, error) {
	rows, err := store.database.Query(`
                SELECT log.subscription_id, log.last_error
                FROM notification_log AS log
                INNER JOIN (
                        SELECT subscription_id, MAX(id) AS max_id
                        FROM notification_log
						WHERE kind IN (?, ?) AND last_error != ''
                        GROUP BY subscription_id
                ) AS latest ON latest.max_id = log.id`,
		model.NotificationKindScheduled,
		model.NotificationKindPriceIncreaseNotice,
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
		&account.TotalCostCents,
		&zeroRenewalNextMonth,
		&account.BannedAt,
		&account.BanNote,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.Account{}, err
	}
	account.Email = strings.TrimSpace(account.Email)
	account.SpaceName = strings.TrimSpace(account.SpaceName)
	account.OpenedAt = strings.TrimSpace(account.OpenedAt)
	account.BannedAt = strings.TrimSpace(account.BannedAt)
	account.BanNote = strings.TrimSpace(account.BanNote)
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
	var nextPriceCents sql.NullInt64
	var nextPriceEffectiveDueDate string
	var isResale int
	var seatID int64
	var accountID int64
	var accountName string
	var seatName string
	var boardedAt string
	var archivedAt sql.NullString
	var cancellationRequestedAt sql.NullString
	var cancellationExpiresAt sql.NullString
	var deletedAt sql.NullString
	var createdAt string
	var updatedAt string

	var customerEmail string
	var customerWechat string
	err := scanner.Scan(
		&subscription.ID,
		&subscription.Name,
		&subscription.BusinessType,
		&subscription.PricePerPersonCents,
		&nextPriceCents,
		&nextPriceEffectiveDueDate,
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
		&cancellationRequestedAt,
		&cancellationExpiresAt,
		&subscription.CancellationCaseID,
		&deletedAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return model.Subscription{}, err
	}
	if nextPriceCents.Valid {
		value := nextPriceCents.Int64
		subscription.NextPriceCents = &value
	}
	subscription.NextPriceEffectiveDueDate = strings.TrimSpace(nextPriceEffectiveDueDate)
	subscription.IsResale = isResale != 0
	subscription.BusinessType = normalizeStoredBusinessType(subscription.BusinessType)
	subscription.CustomerEmail = strings.TrimSpace(customerEmail)
	subscription.CustomerWechat = strings.TrimSpace(customerWechat)
	subscription.SeatID = seatID
	subscription.AccountID = accountID
	subscription.AccountName = strings.TrimSpace(accountName)
	subscription.SeatName = strings.TrimSpace(seatName)
	if subscription.AccountName == "" &&
		subscription.SubscriptionType != "" &&
		subscription.BusinessType != model.SubscriptionBusinessPlus {
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
	if cancellationRequestedAt.Valid && cancellationRequestedAt.String != "" {
		parsed, err := parseTime(cancellationRequestedAt.String)
		if err != nil {
			return model.Subscription{}, err
		}
		subscription.CancellationRequestedAt = &parsed
	}
	if cancellationExpiresAt.Valid && cancellationExpiresAt.String != "" {
		parsed, err := parseTime(cancellationExpiresAt.String)
		if err != nil {
			return model.Subscription{}, err
		}
		subscription.CancellationExpiresAt = &parsed
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

func normalizeStoredBusinessType(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), model.SubscriptionBusinessPlus) {
		return model.SubscriptionBusinessPlus
	}
	return model.SubscriptionBusinessTeam
}

func storedBillCostCents(subscription model.Subscription) int64 {
	if normalizeStoredBusinessType(subscription.BusinessType) == model.SubscriptionBusinessPlus && subscription.CostCents > 0 {
		return subscription.CostCents
	}
	return 0
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
		&bill.CostCents,
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
