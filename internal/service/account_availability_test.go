package service_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"
)

func TestEmailGroupPauseBlocksEverySpaceAndKeepsExistingCustomers(t *testing.T) {
	svc := openTestService(t)
	svc.Clock = func() time.Time { return time.Date(2026, 8, 1, 12, 0, 0, 0, cycle.Location) }
	create := func(name, email string) int64 {
		t.Helper()
		id, err := svc.CreateAccount(service.CreateAccountInput{Name: name, Email: email, SeatCount: 2})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	primary := create("primary", "owner@example.com")
	secondary := create("secondary", "Owner@example.com")
	other := create("other", "other@example.com")
	input := service.CreateInput{Name: "existing", AccountID: secondary, PriceYuan: "30", CronExpr: "interval:30d", BoardedAt: "2026-07-01", NotifyOffsetsRaw: "0"}
	id, err := svc.CreateWithInitialBill(input)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := svc.Get(id)
	billsBefore, _ := svc.Store.ListBills()
	if err := svc.SetAccountPaused(primary, true); err != nil {
		t.Fatal(err)
	}
	added := create("new secondary", "OWNER@example.com")
	for _, accountID := range []int64{primary, secondary, added} {
		account, err := svc.Store.GetAccount(accountID)
		if err != nil || !account.Paused {
			t.Fatalf("member %d not paused: %v", accountID, err)
		}
		seats, err := svc.Store.ListSeatsByAccount(accountID)
		if err != nil {
			t.Fatal(err)
		}
		for _, seatID := range []int64{0, seats[len(seats)-1].ID} {
			attempt := input
			attempt.AccountID, attempt.SeatID = accountID, seatID
			if _, err := svc.CreateWithInitialBill(attempt); err == nil {
				t.Fatalf("new allocation accepted in %d, seat %d", accountID, seatID)
			}
		}
		if _, err := svc.Copy(id, seats[len(seats)-1].ID); err == nil {
			t.Fatalf("copy accepted in %d", accountID)
		}
	}
	current, _ := svc.Get(id)
	billsAfter, _ := svc.Store.ListBills()
	// AccountPaused is joined owner state, not a customer lifecycle mutation.
	expected := before
	expected.AccountPaused = true
	if !reflect.DeepEqual(expected, current) || !reflect.DeepEqual(billsBefore, billsAfter) {
		t.Fatal("pause or rejected allocation changed the occupant or ledger")
	}
	options, err := svc.ListAccountOptionsForForm(0)
	if err != nil || len(options) != 1 || options[0].ID != other {
		t.Fatalf("new options = %#v, %v", options, err)
	}
	options, err = svc.ListAccountOptionsForForm(current.SeatID)
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range options {
		if option.ID == secondary && (len(option.Seats) != 1 || option.Seats[0].ID != current.SeatID) {
			t.Fatalf("edit offered other paused seats: %#v", option)
		}
	}
	input.SeatID = current.SeatID
	input.ExpectedUpdatedAt = current.UpdatedAt.Format(time.RFC3339Nano)
	input.Remark = "correct contact details"
	if err := svc.Update(id, input); err != nil {
		t.Fatalf("existing customer edit: %v", err)
	}
	if err := svc.SetDuePaid(id, "2026-07-31", true); err != nil {
		t.Fatalf("existing customer payment: %v", err)
	}
	appID, err := svc.Store.CreateRedemptionApplication(model.RedemptionApplication{TrackingToken: "email-group", CustomerEmail: "new@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	redeemedID, err := svc.InviteRedemptionApplication(appID, service.RedemptionInviteInput{PriceYuan: "30", CronExpr: "interval:30d", BoardedAt: "2026-08-01", NotifyOffsetsRaw: "0"})
	if err != nil {
		t.Fatal(err)
	}
	redeemed, _ := svc.Get(redeemedID)
	if redeemed.AccountID != other {
		t.Fatalf("auto redemption selected paused owner %d", redeemed.AccountID)
	}
	if err := svc.SetAccountPaused(added, false); err != nil {
		t.Fatal(err)
	}
	for _, accountID := range []int64{primary, secondary, added} {
		account, err := svc.Store.GetAccount(accountID)
		if err != nil || account.Paused {
			t.Fatalf("member %d not resumed: %v", accountID, err)
		}
		input.Name, input.AccountID, input.SeatID = fmt.Sprint("new-", accountID), accountID, 0
		if _, err := svc.CreateWithInitialBill(input); err != nil {
			t.Fatalf("resumed allocation %d: %v", accountID, err)
		}
	}
}
