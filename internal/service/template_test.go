package service_test

import (
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/service"
)

func TestRenderMessageAmountDueUsesSubscriptionPrice(t *testing.T) {
	subscriptionService := openTestService(t)
	subscriptionService.Clock = func() time.Time {
		return time.Date(2026, time.July, 10, 12, 0, 0, 0, cycle.Location)
	}
	accountID, err := subscriptionService.CreateAccount(service.CreateAccountInput{
		Name:      "owner@example.com",
		Email:     "owner@example.com",
		CostYuan:  "5.00",
		SeatCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, err := subscriptionService.Create(service.CreateInput{
		Name:             "custom price customer",
		PriceYuan:        "42.50",
		CronExpr:         "interval:30d",
		NotifyOffsetsRaw: "0",
		AccountID:        accountID,
		BoardedAt:        "2026-07-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := subscriptionService.SaveNotifyTemplate("due={{.AmountDue}} / price={{.PricePerPerson}}"); err != nil {
		t.Fatal(err)
	}
	subscription, err := subscriptionService.Get(subscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := subscriptionService.RenderMessage(subscription)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "due=42.50") || !strings.Contains(rendered, "price=42.50") {
		t.Fatalf("rendered template = %q, want subscription price 42.50", rendered)
	}
	if strings.Contains(rendered, "5.00") {
		t.Fatalf("rendered template = %q, should not use account cost", rendered)
	}
}
