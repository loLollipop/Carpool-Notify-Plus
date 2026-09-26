package service_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"
)

func currentSubscriptionVersion(t *testing.T, subscriptionService *service.SubscriptionService, subscriptionID int64) string {
	t.Helper()
	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	return subscription.UpdatedAt.UTC().Format(time.RFC3339Nano)
}

// Existing CRUD tests represent a freshly opened form on each edit.
func withCurrentSubscriptionVersion(t *testing.T, subscriptionService *service.SubscriptionService, subscriptionID int64, input service.CreateInput) service.CreateInput {
	t.Helper()
	input.ExpectedUpdatedAt = currentSubscriptionVersion(t, subscriptionService, subscriptionID)
	return input
}

func TestSubscriptionUpdateRejectsBrowserSnapshotBeforeRenewalApproval(t *testing.T) {
	for _, businessType := range []string{model.SubscriptionBusinessTeam, model.SubscriptionBusinessPlus} {
		for _, nextPrice := range []string{"", "35.00"} {
			t.Run(businessType+"/next_price="+nextPrice, func(t *testing.T) {
				subscriptionService := openTestService(t)
				subscriptionService.Clock = func() time.Time {
					return time.Date(2026, time.August, 20, 10, 0, 0, 0, cycle.Location)
				}
				enableTestRenewalPayment(t, subscriptionService)
				input := service.CreateInput{
					Name: "customer", BusinessType: businessType, CustomerEmail: "customer@example.com",
					CustomerWechat: "customer", PriceYuan: "30.00", CostYuan: "5.00",
					CronExpr: "interval:30d", BoardedAt: "2026-08-01", Remark: "original note",
				}
				if businessType == model.SubscriptionBusinessTeam {
					accountID, seatIDs := createTestAccountWithSeats(t, subscriptionService, "owner", "seat")
					input.AccountID, input.SeatID = accountID, seatIDs[0]
				}
				subscriptionID, err := subscriptionService.CreateWithInitialBill(input)
				if err != nil {
					t.Fatal(err)
				}
				input.NextPriceYuan = nextPrice
				if nextPrice != "" {
					if err := subscriptionService.Update(subscriptionID, withCurrentSubscriptionVersion(t, subscriptionService, subscriptionID, input)); err != nil {
						t.Fatal(err)
					}
				}
				// Open the form in the renewal period so an accepted edit could rewrite
				// the bill just approved for this same period.
				subscriptionService.Clock = func() time.Time {
					return time.Date(2026, time.August, 31, 10, 0, 0, 0, cycle.Location)
				}
				views, err := subscriptionService.ListView()
				if err != nil || len(views) != 1 {
					t.Fatalf("form views = %#v, %v", views, err)
				}
				// Preserve the browser version before approving the renewal.
				input.ExpectedUpdatedAt = views[0].Subscription.UpdatedAt.Format(time.RFC3339Nano)
				if _, err := subscriptionService.SubmitRenewalApplication(service.RenewalSubmitInput{
					SubscriptionID: subscriptionID, CustomerEmail: input.CustomerEmail,
				}); err != nil {
					t.Fatal(err)
				}
				applications, err := subscriptionService.ListRenewalApplicationsView(model.RenewalStatusPending)
				if err != nil || len(applications) != 1 {
					t.Fatalf("applications = %#v, %v", applications, err)
				}
				if err := subscriptionService.ApproveRenewalApplication(applications[0].Application.ID, service.RenewalDecisionInput{}); err != nil {
					t.Fatal(err)
				}
				approved, err := subscriptionService.Get(subscriptionID)
				if err != nil {
					t.Fatal(err)
				}
				bills, err := subscriptionService.Store.ListBills()
				if err != nil || len(bills) != 2 {
					t.Fatalf("approved bills = %#v, %v", bills, err)
				}
				wantPrice := int64(3000)
				if nextPrice != "" {
					wantPrice = 3500
				}
				if approved.PricePerPersonCents != wantPrice || approved.NextPriceCents != nil || approved.NextPriceEffectiveDueDate != "" || !approved.UpdatedAt.After(views[0].Subscription.UpdatedAt) {
					t.Fatalf("approval price or version mismatch: %#v", approved)
				}
				approvedBill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-08-31")
				if err != nil || approvedBill.AmountCents != wantPrice {
					t.Fatalf("current renewal bill = %#v, %v", approvedBill, err)
				}
				for _, field := range []string{"price", "cost", "remark"} {
					staleInput := input
					switch field {
					case "price":
						staleInput.PriceYuan = "40.00"
					case "cost":
						staleInput.CostYuan = "8.00"
					case "remark":
						staleInput.Remark = "stale form changes only the note"
					}
					if err := subscriptionService.Update(subscriptionID, staleInput); !errors.Is(err, db.ErrSubscriptionStateChanged) {
						t.Fatalf("stale %s edit = %v, want state conflict", field, err)
					}
				}
				after, err := subscriptionService.Get(subscriptionID)
				if err != nil || !reflect.DeepEqual(approved, after) {
					t.Fatalf("stale edit changed approved subscription: %#v, %v", after, err)
				}
				afterBills, err := subscriptionService.Store.ListBills()
				if err != nil || !reflect.DeepEqual(bills, afterBills) {
					t.Fatalf("stale edit changed approved bills: %#v, %v", afterBills, err)
				}
			})
		}
	}
}
