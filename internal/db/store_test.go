package db_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

func TestOpenBackfillsOneCurrentCostRecordForLegacyAccount(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy.db")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE accounts (
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
		);
		INSERT INTO accounts (
			name, opened_at, cost_cents, created_at, updated_at
		) VALUES ('owner@example.com', '2026-07-01', 1850, '2026-08-01T00:00:00Z', '2026-08-01T00:00:00Z');
	`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	store := openStore(t, databasePath)
	defer store.Close()
	account, err := store.GetAccount(1)
	if err != nil {
		t.Fatal(err)
	}
	if account.TotalCostCents != 1850 {
		t.Fatalf("TotalCostCents = %d, want 1850", account.TotalCostCents)
	}
	if account.BannedAt != "" || account.BanNote != "" {
		t.Fatalf("legacy account ban fields = %q/%q, want empty", account.BannedAt, account.BanNote)
	}
	afterSalesCases, err := store.ListAfterSalesCases()
	if err != nil {
		t.Fatalf("after-sales migration: %v", err)
	}
	if len(afterSalesCases) != 0 {
		t.Fatalf("after-sales cases = %d, want 0", len(afterSalesCases))
	}
	records, err := store.ListAccountCostRecords(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Source != model.AccountCostSourceInitial ||
		records[0].PeriodDate != "2026-07-01" || records[0].AmountCents != 1850 {
		t.Fatalf("backfilled records = %#v", records)
	}
}

func TestOpenRepairsImportedInitialAccountCostToOpeningPeriod(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "misdated-initial-account-cost.db")
	store := openStore(t, databasePath)
	accountID, err := store.CreateAccount(model.Account{
		Name:      "Historical Team account",
		OpenedAt:  "2026-07-18",
		CostCents: 4500,
	}, 4500, "2026-08-07")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	records, err := store.ListAccountCostRecords(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].PeriodDate != "2026-07-18" || records[0].AmountCents != 4500 {
		t.Fatalf("repaired initial cost = %#v", records)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	defer store.Close()
	records, err = store.ListAccountCostRecords(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].PeriodDate != "2026-07-18" {
		t.Fatalf("idempotent repaired initial cost = %#v", records)
	}
}

func TestOpenDoesNotMoveHistoricalCumulativeAccountCostToOpeningPeriod(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "historical-cumulative-account-cost.db")
	store := openStore(t, databasePath)
	accountID, err := store.CreateAccount(model.Account{
		Name:      "Historical Team account",
		OpenedAt:  "2026-01-18",
		CostCents: 2000,
	}, 7500, "2026-08-07")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	defer store.Close()
	records, err := store.ListAccountCostRecords(accountID)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].PeriodDate != "2026-08-07" ||
		records[0].AmountCents != 7500 || records[0].Note != "Historical cumulative account cost" {
		t.Fatalf("historical cumulative cost = %#v", records)
	}
}

func TestOpenReconcilesOnlySingleOrdinaryInitialAccountCost(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "reconcile-initial-account-cost.db")
	store := openStore(t, databasePath)
	singleInitialID, err := store.CreateAccount(model.Account{
		Name:      "single-initial@example.com",
		OpenedAt:  "2026-08-19",
		CostCents: 7500,
	}, 7500, "2026-08-19")
	if err != nil {
		t.Fatal(err)
	}
	renewedID, err := store.CreateAccount(model.Account{
		Name:      "renewed@example.com",
		OpenedAt:  "2026-07-19",
		CostCents: 2000,
	}, 2000, "2026-07-19")
	if err != nil {
		t.Fatal(err)
	}
	if inserted, err := store.AccrueAccountRenewal(renewedID, "2026-08-19"); err != nil || !inserted {
		t.Fatalf("insert renewal = %v, %v", inserted, err)
	}
	historicalID, err := store.CreateAccount(model.Account{
		Name:      "historical@example.com",
		OpenedAt:  "2026-01-19",
		CostCents: 2000,
	}, 7500, "2026-08-19")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		`UPDATE accounts SET cost_cents = 11000 WHERE id = ?`, singleInitialID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(
		`UPDATE accounts SET cost_cents = 2500 WHERE id IN (?, ?)`, renewedID, historicalID,
	); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	defer store.Close()
	singleRecords, err := store.ListAccountCostRecords(singleInitialID)
	if err != nil {
		t.Fatal(err)
	}
	if len(singleRecords) != 1 || singleRecords[0].AmountCents != 11000 {
		t.Fatalf("single ordinary initial records = %#v, want amount 11000", singleRecords)
	}
	renewedRecords, err := store.ListAccountCostRecords(renewedID)
	if err != nil {
		t.Fatal(err)
	}
	if len(renewedRecords) != 2 || renewedRecords[0].AmountCents != 2000 || renewedRecords[1].AmountCents != 2000 {
		t.Fatalf("renewed records = %#v, want historical amounts unchanged", renewedRecords)
	}
	historicalRecords, err := store.ListAccountCostRecords(historicalID)
	if err != nil {
		t.Fatal(err)
	}
	if len(historicalRecords) != 1 || historicalRecords[0].AmountCents != 7500 ||
		historicalRecords[0].Note != "Historical cumulative account cost" {
		t.Fatalf("historical cumulative records = %#v, want unchanged", historicalRecords)
	}
}

func TestOpenAddsReplacementColumnsToLegacyAfterSalesTable(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy-after-sales.db")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`
		CREATE TABLE after_sales_cases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			account_id INTEGER NOT NULL,
			subscription_id INTEGER NOT NULL,
			bill_id INTEGER NOT NULL DEFAULT 0,
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
			status TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			processed_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(account_id, subscription_id, banned_date)
		);
	`); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	store := openStore(t, databasePath)
	cases, err := store.ListAfterSalesCases()
	if err != nil {
		t.Fatalf("list migrated after-sales cases: %v", err)
	}
	if len(cases) != 0 {
		t.Fatalf("cases = %d, want 0", len(cases))
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	database, err = sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	rows, err := database.Query(`PRAGMA table_info(after_sales_cases)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	accountIDNullable := false
	businessTypeFound := false
	for rows.Next() {
		var columnID int
		var name string
		var columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&columnID, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == "account_id" {
			accountIDNullable = notNull == 0
		}
		if name == "business_type" {
			businessTypeFound = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !accountIDNullable || !businessTypeFound {
		t.Fatalf(
			"after-sales schema account nullable/business type = %v/%v, want true/true",
			accountIDNullable,
			businessTypeFound,
		)
	}
	var foreignKeyViolations int
	if err := database.QueryRow(`SELECT COUNT(1) FROM pragma_foreign_key_check('after_sales_cases')`).Scan(&foreignKeyViolations); err != nil {
		t.Fatal(err)
	}
	if foreignKeyViolations != 0 {
		t.Fatalf("after-sales foreign key violations = %d, want 0", foreignKeyViolations)
	}
}

func TestOpenMigratesTemplateNameVariableToCustomerEmail(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "carpool.db")
	store := openStore(t, databasePath)
	if err := store.SetSetting(model.SettingNotifyTemplate, "notify {{.Name}}"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(model.SettingCustomerEmailTemplate, "email {{ .Name }}"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	defer store.Close()

	for key, wantPrefix := range map[string]string{
		model.SettingNotifyTemplate:        "notify ",
		model.SettingCustomerEmailTemplate: "email ",
	} {
		templateBody, err := store.GetSetting(key)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(templateBody, ".Name") {
			t.Fatalf("%s = %q, should not keep .Name", key, templateBody)
		}
		if templateBody != wantPrefix+"{{.CustomerEmail}}" {
			t.Fatalf("%s = %q, want customer email variable", key, templateBody)
		}
	}
}

func TestOpenKeepsTemplateThatAlreadyUsesCustomerEmail(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "carpool.db")
	store := openStore(t, databasePath)
	templateBody := "notify {{.CustomerEmail}} / legacy {{.Name}}"
	if err := store.SetSetting(model.SettingNotifyTemplate, templateBody); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	defer store.Close()

	got, err := store.GetSetting(model.SettingNotifyTemplate)
	if err != nil {
		t.Fatal(err)
	}
	if got != templateBody {
		t.Fatalf("template = %q, want preserved custom template", got)
	}
}

func TestOpenUpdatesLegacyDefaultCustomerEmailTemplate(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "carpool.db")
	store := openStore(t, databasePath)
	legacyDefault := `您好，这是关于「{{.CustomerEmail}}」的拼车提醒。

本期应收：¥{{.AmountDue}}
周期：{{.CycleDesc}}
到期：{{.NextDueDate}}
{{if .Remark}}备注：{{.Remark}}{{end}}
{{if .TradeURL}}链接：{{.TradeURL}}{{end}}

请按时缴费，谢谢。`
	if err := store.SetSetting(model.SettingCustomerEmailTemplate, legacyDefault); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	defer store.Close()

	got, err := store.GetSetting(model.SettingCustomerEmailTemplate)
	if err != nil {
		t.Fatal(err)
	}
	if got != model.DefaultCustomerEmailTemplate {
		t.Fatalf("customer template = %q, want new default", got)
	}
}

func TestOpenUpdatesDueSoonDefaultTemplatesToDueInText(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "carpool.db")
	store := openStore(t, databasePath)
	oldNotify := `【拼车收钱】{{.CustomerEmail}}
本期应收：¥{{.AmountDue}}
周期：{{.CycleDesc}}
到期：{{.NextDueDate}}
{{if .Remark}}备注：{{.Remark}}{{end}}
{{if .TradeURL}}链接：{{.TradeURL}}{{end}}`
	oldCustomer := `您好，您的拼车服务即将到期，请及时续费，以免影响正常使用。

客户邮箱：{{.CustomerEmail}}
本期应收：¥{{.AmountDue}}
计费周期：{{.CycleDesc}}
到期日期：{{.NextDueDate}}
{{if .Remark}}备注：{{.Remark}}{{end}}
{{if .TradeURL}}续费链接：{{.TradeURL}}{{end}}

如需续费或有疑问，请联系管理员。
谢谢。`
	if err := store.SetSetting(model.SettingNotifyTemplate, oldNotify); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(model.SettingCustomerEmailTemplate, oldCustomer); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	defer store.Close()

	for key, want := range map[string]string{
		model.SettingNotifyTemplate:        model.DefaultNotifyTemplate,
		model.SettingCustomerEmailTemplate: model.DefaultCustomerEmailTemplate,
	} {
		got, err := store.GetSetting(key)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestRedemptionApplicationLifecycle(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "carpool.db"))
	defer store.Close()
	accountID, err := store.CreateAccount(model.Account{Name: "owner@example.com"}, 0, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	seatID, err := store.CreateSeat(model.Seat{AccountID: accountID, Name: "seat1"})
	if err != nil {
		t.Fatal(err)
	}

	applicationID, err := store.CreateRedemptionApplication(model.RedemptionApplication{
		TrackingToken:   "token-abc",
		CustomerEmail:   "customer@example.com",
		CustomerContact: "wx-customer",
		RedeemCode:      "CODE701",
		RequestNote:     "please invite",
	})
	if err != nil {
		t.Fatal(err)
	}

	application, err := store.GetRedemptionApplicationByToken("token-abc")
	if err != nil {
		t.Fatal(err)
	}
	if application.ID != applicationID || application.Status != model.RedemptionStatusPending {
		t.Fatalf("application = id %d status %q, want id %d pending", application.ID, application.Status, applicationID)
	}
	if application.CustomerEmail != "customer@example.com" || application.CustomerContact != "wx-customer" {
		t.Fatalf("customer fields = %q / %q", application.CustomerEmail, application.CustomerContact)
	}

	pendingCount, err := store.CountRedemptionApplicationsByStatus(model.RedemptionStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if pendingCount != 1 {
		t.Fatalf("pending count = %d, want 1", pendingCount)
	}

	subscriptionID, err := store.CreateSubscriptionAndInviteRedemption(
		applicationID,
		accountID,
		seatID,
		model.Subscription{
			Name:                "customer@example.com",
			PricePerPersonCents: 2500,
			CronExpr:            "interval:30d",
			NotifyOffsets:       []int{3},
			Channels:            append([]string(nil), model.DefaultEnabledChannels...),
			CustomerEmail:       "customer@example.com",
			SeatID:              seatID,
			AccountID:           accountID,
			SubscriptionType:    "owner@example.com",
			BoardedAt:           "2026-08-01",
		},
		"2026-08-01",
		2500,
		"sent",
	)
	if err != nil {
		t.Fatal(err)
	}

	updated, err := store.GetRedemptionApplication(applicationID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != model.RedemptionStatusInvited {
		t.Fatalf("status = %q, want invited", updated.Status)
	}
	if updated.AssignedAccountID != accountID || updated.AssignedSeatID != seatID || updated.AssignedSubscriptionID != subscriptionID {
		t.Fatalf("assigned ids = %d/%d/%d", updated.AssignedAccountID, updated.AssignedSeatID, updated.AssignedSubscriptionID)
	}
	if updated.OperatorNote != "sent" || updated.InvitedAt == nil {
		t.Fatalf("operator note/invited_at = %q/%v", updated.OperatorNote, updated.InvitedAt)
	}

	_, err = store.CreateSubscriptionAndInviteRedemption(
		applicationID,
		accountID,
		seatID,
		model.Subscription{SeatID: seatID},
		"2026-08-01",
		2500,
		"duplicate",
	)
	if !errors.Is(err, db.ErrRedemptionAlreadyProcessed) {
		t.Fatalf("second invite error = %v, want ErrRedemptionAlreadyProcessed", err)
	}
	bills, err := store.ListBills()
	if err != nil {
		t.Fatal(err)
	}
	if len(bills) != 1 || bills[0].SubscriptionID != subscriptionID {
		t.Fatalf("bills after duplicate invite = %#v, want one atomic initial bill", bills)
	}
}

func TestActiveSeatTriggerRejectsSecondSubscription(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "carpool.db"))
	defer store.Close()
	accountID, err := store.CreateAccount(model.Account{Name: "owner@example.com"}, 0, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	seatID, err := store.CreateSeat(model.Seat{AccountID: accountID, Name: "seat1"})
	if err != nil {
		t.Fatal(err)
	}
	base := model.Subscription{
		Name:                "first",
		PricePerPersonCents: 1000,
		CronExpr:            "interval:30d",
		NotifyOffsets:       []int{3},
		Channels:            append([]string(nil), model.DefaultEnabledChannels...),
		SeatID:              seatID,
		SubscriptionType:    "owner@example.com",
		BoardedAt:           "2026-08-01",
	}
	if _, err := store.CreateSubscription(base); err != nil {
		t.Fatal(err)
	}
	base.Name = "second"
	if _, err := store.CreateSubscription(base); !errors.Is(err, db.ErrActiveSeatOccupied) {
		t.Fatalf("second occupant error = %v, want ErrActiveSeatOccupied", err)
	}
}

func TestOpenDetachesLegacyPlusRentalSeat(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "carpool.db")
	store := openStore(t, databasePath)
	accountID, err := store.CreateAccount(model.Account{
		Name:  "Legacy Plus",
		Email: "rented-plus@example.com",
	}, 0, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	seatID, err := store.CreateSeat(model.Seat{AccountID: accountID, Name: "seat1"})
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, err := store.CreateSubscription(model.Subscription{
		Name:                "legacy Plus customer",
		BusinessType:        model.SubscriptionBusinessPlus,
		PricePerPersonCents: 6800,
		CronExpr:            "interval:30d",
		NotifyOffsets:       []int{},
		Channels:            append([]string(nil), model.DefaultEnabledChannels...),
		CustomerWechat:      "wx-customer",
		SeatID:              seatID,
		SubscriptionType:    "Legacy Plus",
		BoardedAt:           "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	defer store.Close()
	subscription, err := store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if subscription.SeatID != 0 || subscription.AccountID != 0 || subscription.SeatName != "" {
		t.Fatalf("legacy Plus placement = seat %d account %d seat name %q, want detached", subscription.SeatID, subscription.AccountID, subscription.SeatName)
	}
	if subscription.CustomerEmail != "rented-plus@example.com" {
		t.Fatalf("migrated rented account email = %q, want rented-plus@example.com", subscription.CustomerEmail)
	}
	freeSeats, err := store.ListFreeSeats(accountID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(freeSeats) != 1 || freeSeats[0].ID != seatID {
		t.Fatalf("free seats = %#v, want released seat %d", freeSeats, seatID)
	}
}

func TestRedemptionCodeCanBeUsedOnceForApplication(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "carpool.db"))
	defer store.Close()

	codeID, err := store.CreateRedemptionCode(model.RedemptionCode{
		Code: "CPN-TEST-CODE-701",
		Note: "order 701",
	})
	if err != nil {
		t.Fatal(err)
	}

	applicationID, err := store.CreateRedemptionApplicationUsingCode(model.RedemptionApplication{
		TrackingToken:   "token-with-code",
		CustomerEmail:   "customer@example.com",
		CustomerContact: "微信：customer",
		RedeemCode:      "CPN-TEST-CODE-701",
	})
	if err != nil {
		t.Fatal(err)
	}

	code, err := store.GetRedemptionCode(codeID)
	if err != nil {
		t.Fatal(err)
	}
	if code.Status != model.RedemptionCodeStatusUsed || code.UsedByApplicationID != applicationID || code.UsedAt == nil {
		t.Fatalf("code after use = %#v, want used by application %d", code, applicationID)
	}

	_, err = store.CreateRedemptionApplicationUsingCode(model.RedemptionApplication{
		TrackingToken:   "second-token",
		CustomerEmail:   "another@example.com",
		CustomerContact: "QQ：123456",
		RedeemCode:      "CPN-TEST-CODE-701",
	})
	if !errors.Is(err, db.ErrRedemptionCodeUsed) {
		t.Fatalf("second use error = %v, want ErrRedemptionCodeUsed", err)
	}
}

func TestOpenRepairsMisdatedPlusInitialBillAfterLegacyScheduleEdit(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "misdated-plus-initial-bill.db")
	store := openStore(t, databasePath)
	subscription := model.Subscription{
		Name:                "Legacy Plus import",
		BusinessType:        model.SubscriptionBusinessPlus,
		PricePerPersonCents: 10300,
		CronExpr:            "interval:30d",
		NotifyOffsets:       []int{},
		Channels:            append([]string(nil), model.DefaultEnabledChannels...),
		CustomerEmail:       "legacy-plus@example.com",
		CustomerWechat:      "wx-legacy-plus",
		SubscriptionType:    model.SubscriptionTypeOther,
		BoardedAt:           "2026-08-11",
	}
	subscriptionID, err := store.CreateSubscriptionWithInitialBill(
		subscription,
		"2026-08-11",
		10300,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Reproduce the old edit path: boarded_at changed, but the only bill did not.
	subscription.ID = subscriptionID
	subscription.BoardedAt = "2026-08-08"
	if err := store.UpdateSubscription(subscription); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	if _, err := store.GetBillByOccurrence(subscriptionID, "2026-08-11"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("legacy bill lookup error = %v, want sql.ErrNoRows", err)
	}
	repaired, err := store.GetBillByOccurrence(subscriptionID, "2026-08-08")
	if err != nil {
		t.Fatal(err)
	}
	if repaired.AmountCents != 10300 || repaired.CostCents != 0 {
		t.Fatalf("repaired bill financials = %d/%d, want 10300/0", repaired.AmountCents, repaired.CostCents)
	}
	if billCount, err := store.CountBillsForSubscription(subscriptionID); err != nil || billCount != 1 {
		t.Fatalf("bill count after repair = %d, err = %v, want 1", billCount, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// The repair runs on every open, so it must remain idempotent.
	store = openStore(t, databasePath)
	defer store.Close()
	if _, err := store.GetBillByOccurrence(subscriptionID, "2026-08-08"); err != nil {
		t.Fatal(err)
	}
	if billCount, err := store.CountBillsForSubscription(subscriptionID); err != nil || billCount != 1 {
		t.Fatalf("bill count after second open = %d, err = %v, want 1", billCount, err)
	}
}

func TestOpenRepairsMisdatedTeamInitialBillAfterLegacyScheduleEdit(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "misdated-team-initial-bill.db")
	store := openStore(t, databasePath)
	accountID, err := store.CreateAccount(model.Account{Name: "Legacy Team"}, 0, "2026-08-01")
	if err != nil {
		t.Fatal(err)
	}
	seatID, err := store.CreateSeat(model.Seat{AccountID: accountID, Name: "seat1"})
	if err != nil {
		t.Fatal(err)
	}
	subscription := model.Subscription{
		Name:                "Legacy Team customer",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 3500,
		CronExpr:            "interval:30d",
		NotifyOffsets:       []int{3, 1, 0},
		Channels:            append([]string(nil), model.DefaultEnabledChannels...),
		SeatID:              seatID,
		SubscriptionType:    "Legacy Team",
		BoardedAt:           "2026-08-11",
	}
	subscriptionID, err := store.CreateSubscriptionWithInitialBill(subscription, "2026-08-11", 3500)
	if err != nil {
		t.Fatal(err)
	}
	subscription.ID = subscriptionID
	subscription.BoardedAt = "2026-08-08"
	if err := store.UpdateSubscription(subscription); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	defer store.Close()
	if _, err := store.GetBillByOccurrence(subscriptionID, "2026-08-11"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("legacy Team bill lookup error = %v, want sql.ErrNoRows", err)
	}
	if _, err := store.GetBillByOccurrence(subscriptionID, "2026-08-08"); err != nil {
		t.Fatal(err)
	}
}

func openStore(t *testing.T, databasePath string) *db.Store {
	t.Helper()
	store, err := db.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
