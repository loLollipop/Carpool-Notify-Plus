package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"carpool-notify/internal/service"
)

func TestAvailabilityAndRenewalDeferralHTTPContract(t *testing.T) {
	server, router := subscriptionEditTestServer(t)
	router.PUT("/accounts/:id/availability", server.putAccountAvailability)
	router.POST("/subscriptions/:id/defer", server.postDeferRenewal)
	router.DELETE("/subscriptions/:id/defer", server.deleteDeferRenewal)
	router.POST("/subscriptions/deferred/complete", server.postCompleteDeferred)
	router.GET("/calendar", server.getCalendar)
	accountID, err := server.Service.CreateAccount(service.CreateAccountInput{Name: "owner", SeatCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	id, err := server.Service.CreateWithInitialBill(service.CreateInput{Name: "customer", AccountID: accountID, PriceYuan: "30", CronExpr: "interval:30d", BoardedAt: "2026-07-01", NotifyOffsetsRaw: "0"})
	if err != nil {
		t.Fatal(err)
	}
	availabilityURL := fmt.Sprintf("/accounts/%d/availability", accountID)
	for _, invalid := range []any{map[string]any{}, map[string]any{"paused": nil}, map[string]any{"paused": "true"}} {
		response := subscriptionEditRequest(t, router, http.MethodPut, availabilityURL, invalid)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid availability accepted: %d %s", response.Code, response.Body.String())
		}
	}
	response := subscriptionEditRequest(t, router, http.MethodPut, availabilityURL, map[string]any{"paused": true})
	if response.Code != http.StatusOK {
		t.Fatalf("pause: %s", response.Body.String())
	}
	current, _ := server.Service.Get(id)
	deferURL := fmt.Sprintf("/subscriptions/%d/defer", id)
	request := map[string]any{"expected_due_date": "2026-07-31", "expected_updated_at": current.UpdatedAt.Format(time.RFC3339Nano)}
	response = subscriptionEditRequest(t, router, http.MethodPost, deferURL, request)
	if response.Code != http.StatusOK {
		t.Fatalf("defer: %d %s", response.Code, response.Body.String())
	}
	response = subscriptionEditRequest(t, router, http.MethodPost, deferURL, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("stale duplicate: %d %s", response.Code, response.Body.String())
	}
	response = subscriptionEditRequest(t, router, http.MethodGet, "/calendar?month=2027-01", nil)
	var payload struct {
		Calendar service.CalendarMonthView `json:"calendar"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &payload) != nil || len(payload.Calendar.DeferredSubscriptions) != 1 {
		t.Fatalf("calendar: %s", response.Body.String())
	}
	view := payload.Calendar.DeferredSubscriptions[0]
	if !view.AccountPaused || !view.RenewalDeferred || view.DeferredAtLabel == "" || view.Subscription.RenewalDeferredDueDate != "2026-07-31" {
		t.Fatalf("view: %#v", view)
	}
	response = subscriptionEditRequest(t, router, http.MethodDelete, deferURL, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("restore: %s", response.Body.String())
	}
	current, _ = server.Service.Get(id)
	request["expected_updated_at"] = current.UpdatedAt.Format(time.RFC3339Nano)
	response = subscriptionEditRequest(t, router, http.MethodPost, deferURL, request)
	if response.Code != http.StatusOK {
		t.Fatalf("second defer: %s", response.Body.String())
	}
	current, _ = server.Service.Get(id)
	response = subscriptionEditRequest(t, router, http.MethodPost, "/subscriptions/deferred/complete", map[string]any{
		"subscriptions": []map[string]any{{"subscription_id": id, "expected_updated_at": current.UpdatedAt.Format(time.RFC3339Nano)}},
	})
	var result struct {
		CompletedCount int `json:"completed_count"`
	}
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.CompletedCount != 1 {
		t.Fatalf("complete: %d %s", response.Code, response.Body.String())
	}
	archived, _ := server.Service.Store.GetSubscriptionIncludingArchived(id)
	if archived.ArchivedAt == nil || archived.RenewalDeferredAt != nil {
		t.Fatalf("archive: %#v", archived)
	}
}
