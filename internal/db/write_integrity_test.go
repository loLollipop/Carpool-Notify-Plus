package db

import (
	"errors"
	"path/filepath"
	"testing"

	"carpool-notify/internal/model"
)

func TestRedemptionStatusRejectsConsumptionAfterOperatorRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redemption-status.db")
	operator, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = operator.Close() })
	customer, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = customer.Close() })
	codeID, err := operator.CreateRedemptionCode(model.RedemptionCode{Code: "STATUS-RACE"})
	if err != nil {
		t.Fatal(err)
	}
	// Pause the operator after its service-level read, then let consumption commit.
	stale, err := operator.GetRedemptionCode(codeID)
	if err != nil || stale.Status != model.RedemptionCodeStatusUnused {
		t.Fatalf("operator snapshot = %#v, %v", stale, err)
	}
	applicationID, err := customer.CreateRedemptionApplicationUsingCode(model.RedemptionApplication{
		TrackingToken: "first-application", CustomerEmail: "first@example.com", RedeemCode: stale.Code,
	})
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := customer.GetRedemptionCode(codeID)
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{model.RedemptionCodeStatusDisabled, model.RedemptionCodeStatusUnused} {
		if err := operator.SetRedemptionCodeStatus(codeID, status); !errors.Is(err, ErrRedemptionCodeNotUnused) {
			t.Fatalf("set consumed code to %s: %v", status, err)
		}
	}
	if _, err := customer.CreateRedemptionApplicationUsingCode(model.RedemptionApplication{
		TrackingToken: "second-application", CustomerEmail: "second@example.com", RedeemCode: stale.Code,
	}); !errors.Is(err, ErrRedemptionCodeUsed) {
		t.Fatalf("second consumption error = %v", err)
	}
	after, err := operator.GetRedemptionCode(codeID)
	if err != nil || after.Status != model.RedemptionCodeStatusUsed || after.UsedByApplicationID != applicationID ||
		after.UsedAt == nil || !after.UsedAt.Equal(*consumed.UsedAt) || !after.UpdatedAt.Equal(consumed.UpdatedAt) {
		t.Fatalf("consumption association changed: %#v, %v", after, err)
	}
	var count int
	if err := operator.database.QueryRow(`SELECT COUNT(*) FROM redemption_applications`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("application count = %d, %v", count, err)
	}
}

func TestRedemptionStatusOnlyAcceptsUnusedAndDisabled(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "redemption-status-validation.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	codeID, err := store.CreateRedemptionCode(model.RedemptionCode{Code: "STATUS-VALIDATION"})
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{model.RedemptionCodeStatusUsed, "", "invalid"} {
		if err := store.SetRedemptionCodeStatus(codeID, status); err == nil {
			t.Fatalf("invalid target status %q accepted", status)
		}
	}
	for _, status := range []string{model.RedemptionCodeStatusDisabled, model.RedemptionCodeStatusDisabled, model.RedemptionCodeStatusUnused} {
		if err := store.SetRedemptionCodeStatus(codeID, status); err != nil {
			t.Fatalf("valid target status %q rejected: %v", status, err)
		}
		code, err := store.GetRedemptionCode(codeID)
		if err != nil || code.Status != status || code.UsedByApplicationID != 0 || code.UsedAt != nil {
			t.Fatalf("status transition = %#v, %v", code, err)
		}
	}
	if err := store.SetRedemptionCodeStatus(codeID+1, model.RedemptionCodeStatusDisabled); !errors.Is(err, ErrRedemptionCodeNotUnused) {
		t.Fatalf("missing code error = %v", err)
	}
}

func TestSubscriptionAssignmentRechecksBanAfterAccountRead(t *testing.T) {
	for _, operation := range []string{"create-or-copy", "initial-bill", "edit", "sync", "move"} {
		t.Run(operation, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "late-assignment.db")
			writer, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = writer.Close() })
			banner, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = banner.Close() })
			accountID, err := writer.CreateAccount(model.Account{Name: "target"}, 0, "2026-09-01")
			if err != nil {
				t.Fatal(err)
			}
			seatID, err := writer.CreateSeat(model.Seat{AccountID: accountID, Name: "target seat"})
			if err != nil {
				t.Fatal(err)
			}
			subscriptionID, err := writer.CreateSubscriptionWithInitialBill(integritySubscription(), "2026-09-01", 3000)
			if err != nil {
				t.Fatal(err)
			}
			subscription, err := writer.GetSubscription(subscriptionID)
			if err != nil {
				t.Fatal(err)
			}
			account, err := writer.GetAccount(accountID)
			if err != nil || account.BannedAt != "" {
				t.Fatalf("prevalidation account = %#v, %v", account, err)
			}
			// The ban commits after input validation and before persistence.
			if cases, err := banner.BanAccountAndCreateAfterSalesCases(accountID, "2026-09-10", "ban", 30); err != nil || cases != 0 {
				t.Fatalf("ban cases = %d, %v", cases, err)
			}
			subscription.SeatID = seatID
			subscription.Remark = "late assignment"
			switch operation {
			case "create-or-copy":
				_, err = writer.CreateSubscription(subscription)
			case "initial-bill":
				_, err = writer.CreateSubscriptionWithInitialBill(subscription, "2026-09-01", 4000)
			case "edit":
				err = writer.UpdateSubscription(subscription)
			case "sync":
				err = writer.UpdateSubscriptionAndSyncBill(subscription, "2026-09-01", 4000, 500)
			case "move":
				subscription.BoardedAt = "2026-09-02"
				err = writer.UpdateSubscriptionAndMoveInitialBill(subscription, "2026-09-01", "2026-09-02", 4000, 500)
			}
			if !errors.Is(err, ErrReplacementAccountBanned) {
				t.Fatalf("late assignment error = %v", err)
			}
			stored, err := writer.GetSubscription(subscriptionID)
			if err != nil || stored.SeatID != 0 || stored.Remark != "" || stored.BoardedAt != "2026-09-01" || !stored.UpdatedAt.Equal(subscription.UpdatedAt) {
				t.Fatalf("subscription changed: %#v, %v", stored, err)
			}
			var subscriptions, bills int
			if err := writer.database.QueryRow(`SELECT (SELECT COUNT(*) FROM subscriptions), (SELECT COUNT(*) FROM bills)`).Scan(&subscriptions, &bills); err != nil || subscriptions != 1 || bills != 1 {
				t.Fatalf("subscription/bill count = %d/%d, %v", subscriptions, bills, err)
			}
			bill, err := writer.GetBillByOccurrence(subscriptionID, "2026-09-01")
			if err != nil || bill.AmountCents != 3000 || bill.CostCents != 0 {
				t.Fatalf("bill changed: %#v, %v", bill, err)
			}
		})
	}
}

func TestBannedAccountOccupantMayEditButCannotChangeSeat(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "existing-ban-occupant.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	accountID, err := store.CreateAccount(model.Account{Name: "legacy banned account"}, 0, "2026-09-01")
	if err != nil {
		t.Fatal(err)
	}
	seatID, err := store.CreateSeat(model.Seat{AccountID: accountID, Name: "occupied"})
	if err != nil {
		t.Fatal(err)
	}
	otherSeatID, err := store.CreateSeat(model.Seat{AccountID: accountID, Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	subscription := integritySubscription()
	subscription.SeatID = seatID
	subscriptionID, err := store.CreateSubscription(subscription)
	if err != nil {
		t.Fatal(err)
	}
	// Preserve compatibility with legacy banned occupants without pending cases.
	if _, err := store.database.Exec(`UPDATE accounts SET banned_at = '2026-09-10' WHERE id = ?`, accountID); err != nil {
		t.Fatal(err)
	}
	subscription, err = store.GetSubscription(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	subscription.Remark = "corrected remark"
	if err := store.UpdateSubscription(subscription); err != nil {
		t.Fatalf("same-seat edit rejected: %v", err)
	}
	subscription, err = store.GetSubscription(subscriptionID)
	if err != nil || subscription.Remark != "corrected remark" || subscription.SeatID != seatID {
		t.Fatalf("same-seat edit = %#v, %v", subscription, err)
	}
	subscription.SeatID = otherSeatID
	if err := store.UpdateSubscription(subscription); !errors.Is(err, ErrReplacementAccountBanned) {
		t.Fatalf("move within banned account error = %v", err)
	}
}

func TestManualBillCorrectionInvalidatesSubscriptionEdit(t *testing.T) {
	for _, correction := range []string{"amount", "financials"} {
		for _, edit := range []string{"edit", "sync", "move"} {
			t.Run(correction+"/"+edit, func(t *testing.T) {
				store, err := Open(filepath.Join(t.TempDir(), "manual-bill-conflict.db"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = store.Close() })
				subscription := integritySubscription()
				subscription.PricePerPersonCents = 3000
				subscription.CostCents = 500
				id, err := store.CreateSubscriptionWithInitialBill(subscription, "2026-09-01", 3000)
				if err != nil {
					t.Fatal(err)
				}
				// Advancing versions must also work when the clock is behind storage.
				if _, err := store.database.Exec(`UPDATE subscriptions SET updated_at = '2099-01-01T00:00:00Z' WHERE id = ?`, id); err != nil {
					t.Fatal(err)
				}
				stale, err := store.GetSubscription(id)
				if err != nil {
					t.Fatal(err)
				}
				bill, err := store.GetBillByOccurrence(id, "2026-09-01")
				if err != nil {
					t.Fatal(err)
				}
				wantCost := bill.CostCents
				if correction == "amount" {
					err = store.UpdateBill(bill.ID, 2800, "manual correction")
				} else {
					wantCost = 700
					err = store.UpdateBillFinancials(bill.ID, 2800, wantCost, "manual correction")
				}
				if err != nil {
					t.Fatal(err)
				}
				stale.PricePerPersonCents = 3500
				stale.CostCents = 900
				switch edit {
				case "edit":
					err = store.UpdateSubscription(stale)
				case "sync":
					err = store.UpdateSubscriptionAndSyncBill(stale, bill.DueDate, 3500, 900)
				case "move":
					stale.BoardedAt = "2026-09-02"
					err = store.UpdateSubscriptionAndMoveInitialBill(stale, bill.DueDate, "2026-09-02", 3500, 900)
				}
				if !errors.Is(err, ErrSubscriptionStateChanged) {
					t.Fatalf("stale edit error = %v", err)
				}
				stored, err := store.GetSubscription(id)
				if err != nil || !stored.UpdatedAt.After(stale.UpdatedAt) || stored.PricePerPersonCents != 3000 || stored.CostCents != 500 || stored.BoardedAt != "2026-09-01" {
					t.Fatalf("subscription correction lost: %#v, %v", stored, err)
				}
				bill, err = store.GetBill(bill.ID)
				if err != nil || bill.AmountCents != 2800 || bill.CostCents != wantCost || bill.Note != "manual correction" || bill.DueDate != "2026-09-01" {
					t.Fatalf("bill correction lost: %#v, %v", bill, err)
				}
			})
		}
	}
}

func TestManualBillCorrectionRollsBackWhenVersionWriteFails(t *testing.T) {
	for _, correction := range []string{"amount", "financials"} {
		t.Run(correction, func(t *testing.T) {
			store, err := Open(filepath.Join(t.TempDir(), "manual-bill-atomic.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			id, err := store.CreateSubscriptionWithInitialBill(integritySubscription(), "2026-09-01", 3000)
			if err != nil {
				t.Fatal(err)
			}
			before, err := store.GetSubscription(id)
			if err != nil {
				t.Fatal(err)
			}
			bill, err := store.GetBillByOccurrence(id, "2026-09-01")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.database.Exec(`CREATE TRIGGER reject_manual_bill_version
				BEFORE UPDATE OF updated_at ON subscriptions
				BEGIN SELECT RAISE(ABORT, 'injected version failure'); END`); err != nil {
				t.Fatal(err)
			}
			if correction == "amount" {
				err = store.UpdateBill(bill.ID, 2800, "correction")
			} else {
				err = store.UpdateBillFinancials(bill.ID, 2800, 700, "correction")
			}
			if err == nil {
				t.Fatal("bill correction succeeded despite injected version failure")
			}
			after, err := store.GetSubscription(id)
			if err != nil || !after.UpdatedAt.Equal(before.UpdatedAt) {
				t.Fatalf("subscription changed after rollback: %#v, %v", after, err)
			}
			storedBill, err := store.GetBill(bill.ID)
			if err != nil || storedBill.AmountCents != bill.AmountCents || storedBill.CostCents != bill.CostCents || storedBill.Note != bill.Note || !storedBill.UpdatedAt.Equal(bill.UpdatedAt) {
				t.Fatalf("bill changed after rollback: %#v, %v", storedBill, err)
			}
		})
	}
}
