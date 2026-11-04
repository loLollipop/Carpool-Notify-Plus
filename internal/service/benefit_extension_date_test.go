package service

import (
	"errors"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

func calendarExtensionService(t *testing.T, boardedDates ...string) (*SubscriptionService, []int64) {
	t.Helper()
	svc := openGoalTestService(t)
	svc.Clock = func() time.Time { return time.Date(2026, 11, 4, 12, 0, 0, 0, cycle.Location) }
	account, err := svc.CreateAccount(CreateAccountInput{Name: "calendar-owner@example.com", SeatCount: len(boardedDates)})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, date := range boardedDates {
		id, err := svc.CreateWithInitialBill(CreateInput{Name: "calendar-customer", CustomerEmail: "calendar@example.com", AccountID: account,
			PriceYuan: "100", CronExpr: "interval:30d", BoardedAt: date, NotifyOffsetsRaw: "3,1,0"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return svc, ids
}

func TestCalendarBenefitExtendsEverySeatToSelectedPaymentDate(t *testing.T) {
	for _, test := range []struct {
		name    string
		boards  []string
		prior   bool
		wantDay []int
	}{
		{name: "November 4 to November 9", boards: []string{"2026-10-05"}, wantDay: []int{5}},
		{name: "different seat dates reach the same day", boards: []string{"2026-10-05", "2026-10-07"}, wantDay: []int{5, 3}},
		{name: "existing extension is not added twice", boards: []string{"2026-10-05"}, prior: true, wantDay: []int{3}},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, ids := calendarExtensionService(t, test.boards...)
			if test.prior {
				if _, err := svc.RecordCustomerBenefits(extensionInput(t, svc, ids, "2026-11-04", 2, "calendar-prior-gift")); err != nil {
					t.Fatal(err)
				}
			}
			input := extensionInput(t, svc, ids, "2026-11-04", 0, "calendar-target-date")
			input.ExtensionUntil = "2026-11-09"
			recorded, err := svc.RecordCustomerBenefits(input)
			if err != nil || recorded != len(ids) {
				t.Fatalf("record = %d, %v", recorded, err)
			}
			views, err := svc.ListView()
			if err != nil {
				t.Fatal(err)
			}
			for _, view := range views {
				if view.NextDueDate != input.ExtensionUntil {
					t.Fatalf("subscription %d due = %s, want %s", view.Subscription.ID, view.NextDueDate, input.ExtensionUntil)
				}
			}
			benefits, err := svc.Store.ListCustomerBenefits()
			if err != nil {
				t.Fatal(err)
			}
			byID := make(map[int64]model.CustomerBenefit)
			for _, benefit := range benefits {
				if benefit.BatchID == "benefit-operation-v1:"+input.OperationKey {
					byID[benefit.SubscriptionID] = benefit
				}
			}
			for index, id := range ids {
				benefit := byID[id]
				if benefit.ExtensionDays != test.wantDay[index] {
					t.Fatalf("seat %d days = %d, want %d", id, benefit.ExtensionDays, test.wantDay[index])
				}
				message := customerBenefitEmail(benefit, model.Subscription{ID: id}, input.ExtensionReviewSnapshots[index].ExpectedDueDate)
				if message.data.NextDueDate != input.ExtensionUntil || message.data.ExtensionDays != test.wantDay[index] {
					t.Fatalf("email has wrong date or length: %#v", message.data)
				}
			}
			if _, err := svc.RecordCustomerBenefits(input); err == nil {
				t.Fatal("repeated operation must not create another extension")
			}
			after, _ := svc.Store.ListCustomerBenefits()
			if len(after) != len(benefits) {
				t.Fatal("repeated operation created more benefit records")
			}
		})
	}
}

func TestCalendarBenefitRejectsInvalidTargetsWithoutPartialWrites(t *testing.T) {
	for _, test := range []struct {
		name, target string
		days         int
	}{
		{name: "invalid day", target: "2026-11-31"},
		{name: "noncanonical format", target: "2026-11-9"},
		{name: "earlier than one seat", target: "2026-11-05"},
		{name: "same as one seat", target: "2026-11-06"},
		{name: "over 365 for one seat", target: "2027-11-05"},
		{name: "conflicting date and days", target: "2026-11-09", days: 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, ids := calendarExtensionService(t, "2026-10-05", "2026-10-07")
			input := extensionInput(t, svc, ids, "2026-11-04", test.days, "calendar-invalid-date")
			input.ExtensionUntil = test.target
			if _, err := svc.RecordCustomerBenefits(input); err == nil {
				t.Fatal("invalid calendar target accepted")
			}
			benefits, err := svc.Store.ListCustomerBenefits()
			if err != nil || len(benefits) != 0 {
				t.Fatalf("benefits after rejection = %#v, %v", benefits, err)
			}
			events, err := svc.Store.ListSubscriptionDueExtensions()
			if err != nil || len(events) != 0 {
				t.Fatalf("extensions after rejection = %#v, %v", events, err)
			}
		})
	}
}

func TestCalendarBenefitRejectsPaymentAfterDateSelection(t *testing.T) {
	svc, ids := calendarExtensionService(t, "2026-10-05")
	input := extensionInput(t, svc, ids, "2026-11-04", 0, "calendar-stale-date")
	input.ExtensionUntil = "2026-11-09"
	if err := svc.SetDuePaid(ids[0], "2026-11-04", true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RecordCustomerBenefits(input); !errors.Is(err, db.ErrSubscriptionStateChanged) {
		t.Fatalf("paid period must reject stale calendar selection: %v", err)
	}
	benefits, err := svc.Store.ListCustomerBenefits()
	if err != nil || len(benefits) != 0 {
		t.Fatalf("stale selection persisted benefits = %#v, %v", benefits, err)
	}
}
