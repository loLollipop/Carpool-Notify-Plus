package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"carpool-notify/internal/model"
)

func TestDeleteMistakenTeamSubscriptionClearsDependentRecords(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "mistaken-subscription.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := formatTime(time.Date(2026, time.September, 8, 4, 0, 0, 0, time.UTC))
	subscriptionID, err := store.CreateSubscriptionWithInitialBill(model.Subscription{
		Name:                "mistaken customer",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 9000,
		CronExpr:            "interval:30d",
		NotifyOffsets:       []int{3},
		Channels:            append([]string(nil), model.DefaultEnabledChannels...),
		CustomerEmail:       "mistaken@example.com",
		CustomerWechat:      "mistaken-wechat",
		BoardedAt:           "2026-09-01",
	}, "2026-09-01", 9000)
	if err != nil {
		t.Fatal(err)
	}

	var billID int64
	if err := store.database.QueryRow(
		`SELECT id FROM bills WHERE subscription_id = ?`,
		subscriptionID,
	).Scan(&billID); err != nil {
		t.Fatal(err)
	}

	statements := []struct {
		query string
		args  []any
	}{
		{
			`INSERT INTO notification_log (
				subscription_id, due_date, offset_days, channel, status,
				attempt_count, next_retry_at, last_error, kind, created_at, updated_at
			) VALUES (?, '2026-10-01', 3, 'smtp', 'success', 1, NULL, '', 'customer', ?, ?)`,
			[]any{subscriptionID, now, now},
		},
		{
			`INSERT INTO paid_due_occurrences (subscription_id, due_date, paid_at)
			 VALUES (?, '2026-10-01', ?)`,
			[]any{subscriptionID, now},
		},
		{
			`INSERT INTO subscription_price_changes (
				subscription_id, previous_price_cents, new_price_cents,
				effective_due_date, created_at
			) VALUES (?, 9000, 9500, '2026-10-01', ?)`,
			[]any{subscriptionID, now},
		},
		{
			`INSERT INTO pricing_exemptions (
				subscription_id, reason_code, note, review_after, review_cycles,
				price_cents_snapshot, market_median_cents_snapshot, created_at
			) VALUES (?, 'manual', '', '2026-10-01', 1, 9000, 9500, ?)`,
			[]any{subscriptionID, now},
		},
		{
			`INSERT INTO customer_benefits (
				batch_id, subscription_id, benefit_type, benefit_name,
				actual_cost_cents, perceived_value_cents, benefit_date, created_at
			) VALUES ('batch-1', ?, 'manual', 'gift', 500, 1000, '2026-09-08', ?)`,
			[]any{subscriptionID, now},
		},
		{
			`INSERT INTO renewal_applications (
				tracking_token, subscription_id, customer_email, due_date,
				amount_cents, status, created_at, updated_at
			) VALUES ('renewal-token', ?, 'mistaken@example.com', '2026-10-01', 9000, 'pending', ?, ?)`,
			[]any{subscriptionID, now, now},
		},
	}
	for _, statement := range statements {
		if _, err := store.database.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}

	applicationResult, err := store.database.Exec(`
		INSERT INTO redemption_applications (
			tracking_token, customer_email, customer_contact, redeem_code,
			status, assigned_account_id, assigned_seat_id, assigned_subscription_id,
			operator_note, invited_at, created_at, updated_at
		) VALUES (
			'redemption-token', 'mistaken@example.com', 'mistaken-wechat', 'CODE-1',
			?, 10, 20, ?, 'assigned', ?, ?, ?
		)`, model.RedemptionStatusInvited, subscriptionID, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	applicationID, err := applicationResult.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.database.Exec(`
		INSERT INTO redemption_codes (
			code, status, used_by_application_id, used_at, created_at, updated_at
		) VALUES ('CODE-1', ?, ?, ?, ?, ?)`,
		model.RedemptionCodeStatusUsed, applicationID, now, now, now,
	); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteMistakenTeamSubscription(subscriptionID); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{
		"subscriptions",
		"renewal_applications",
		"notification_log",
		"paid_due_occurrences",
		"customer_benefits",
		"pricing_exemptions",
		"subscription_price_changes",
		"bills",
	} {
		var count int
		if err := store.database.QueryRow(
			"SELECT COUNT(1) FROM "+table+" WHERE "+subscriptionReferenceColumn(table)+" = ?",
			subscriptionID,
		).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Fatalf("%s rows = %d, want 0", table, count)
		}
	}

	var redemptionStatus string
	var assignedAccountID, assignedSeatID, assignedSubscriptionID int64
	var operatorNote string
	var invitedAt sql.NullString
	if err := store.database.QueryRow(`
		SELECT status, assigned_account_id, assigned_seat_id,
		       assigned_subscription_id, operator_note, invited_at
		FROM redemption_applications
		WHERE id = ?`, applicationID).Scan(
		&redemptionStatus,
		&assignedAccountID,
		&assignedSeatID,
		&assignedSubscriptionID,
		&operatorNote,
		&invitedAt,
	); err != nil {
		t.Fatal(err)
	}
	if redemptionStatus != model.RedemptionStatusPending ||
		assignedAccountID != 0 || assignedSeatID != 0 || assignedSubscriptionID != 0 ||
		operatorNote != "" || invitedAt.Valid {
		t.Fatalf(
			"rewound redemption = status %q account %d seat %d subscription %d note %q invited %#v",
			redemptionStatus,
			assignedAccountID,
			assignedSeatID,
			assignedSubscriptionID,
			operatorNote,
			invitedAt,
		)
	}
	var codeCount int
	if err := store.database.QueryRow(
		`SELECT COUNT(1) FROM redemption_codes WHERE used_by_application_id = ?`,
		applicationID,
	).Scan(&codeCount); err != nil {
		t.Fatal(err)
	}
	if codeCount != 1 {
		t.Fatalf("redemption code rows = %d, want 1", codeCount)
	}

	if _, err := store.GetBill(billID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("bill lookup after delete error = %v, want sql.ErrNoRows", err)
	}
}

func TestDeleteMistakenTeamSubscriptionRejectsProtectedRecords(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "protected-mistaken-subscription.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	now := formatTime(time.Date(2026, time.September, 8, 4, 0, 0, 0, time.UTC))
	teamID, err := store.CreateSubscriptionWithInitialBill(model.Subscription{
		Name:                "protected team",
		BusinessType:        model.SubscriptionBusinessTeam,
		PricePerPersonCents: 9000,
		CronExpr:            "interval:30d",
		NotifyOffsets:       []int{},
		Channels:            append([]string(nil), model.DefaultEnabledChannels...),
		BoardedAt:           "2026-09-01",
	}, "2026-09-01", 9000)
	if err != nil {
		t.Fatal(err)
	}
	var billID int64
	if err := store.database.QueryRow(
		`SELECT id FROM bills WHERE subscription_id = ?`,
		teamID,
	).Scan(&billID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.database.Exec(`
		INSERT INTO after_sales_cases (
			subscription_id, bill_id, business_type, banned_date, status, created_at, updated_at
		) VALUES (?, ?, ?, '2026-09-08', ?, ?, ?)`,
		teamID,
		billID,
		model.SubscriptionBusinessTeam,
		model.AfterSalesStatusRefunded,
		now,
		now,
	); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteMistakenTeamSubscription(teamID); !errors.Is(err, ErrMistakenSubscriptionHasAfterSales) {
		t.Fatalf("delete protected Team error = %v", err)
	}
	if _, err := store.GetSubscription(teamID); err != nil {
		t.Fatalf("protected Team subscription disappeared: %v", err)
	}
	if _, err := store.GetBill(billID); err != nil {
		t.Fatalf("protected Team bill disappeared: %v", err)
	}

	plusID, err := store.CreateSubscription(model.Subscription{
		Name:                "Plus rental",
		BusinessType:        model.SubscriptionBusinessPlus,
		PricePerPersonCents: 6800,
		CronExpr:            "interval:30d",
		NotifyOffsets:       []int{},
		Channels:            append([]string(nil), model.DefaultEnabledChannels...),
		CustomerEmail:       "plus@example.com",
		CustomerWechat:      "plus-wechat",
		BoardedAt:           "2026-09-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteMistakenTeamSubscription(plusID); !errors.Is(err, ErrMistakenSubscriptionNotTeam) {
		t.Fatalf("delete Plus error = %v", err)
	}
}

func subscriptionReferenceColumn(table string) string {
	if table == "subscriptions" {
		return "id"
	}
	return "subscription_id"
}
