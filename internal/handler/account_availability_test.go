package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"carpool-notify/internal/service"
)

func TestEmailGroupAvailabilityHTTPContract(t *testing.T) {
	server, router := subscriptionEditTestServer(t)
	router.PUT("/accounts/:id/availability", server.putAccountAvailability)
	router.GET("/account-options", server.getAccountOptions)
	var ids []int64
	for _, email := range []string{"owner@example.com", "OWNER@example.com"} {
		id, err := server.Service.CreateAccount(service.CreateAccountInput{Name: email, Email: email, SeatCount: 1})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	response := subscriptionEditRequest(t, router, http.MethodPut, fmt.Sprintf("/accounts/%d/availability", ids[0]), map[string]any{"paused": true})
	if response.Code != http.StatusOK {
		t.Fatalf("pause: %s", response.Body.String())
	}
	response = subscriptionEditRequest(t, router, http.MethodGet, "/account-options", nil)
	var options struct {
		Accounts []service.AccountOption `json:"accounts"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &options); err != nil || response.Code != http.StatusOK || len(options.Accounts) != 0 {
		t.Fatalf("paused options: %s, %v", response.Body.String(), err)
	}
	payload := map[string]any{"account_id": ids[1], "price_yuan": "30", "cron_expr": "interval:30d", "boarded_at": "2026-08-01"}
	response = subscriptionEditRequest(t, router, http.MethodPost, "/subscriptions", payload)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("stale form accepted secondary: %s", response.Body.String())
	}
	bills, err := server.Service.Store.ListBills()
	if err != nil || len(bills) != 0 {
		t.Fatalf("rejected creation wrote bill: %v", err)
	}
	response = subscriptionEditRequest(t, router, http.MethodPut, fmt.Sprintf("/accounts/%d/availability", ids[1]), map[string]any{"paused": false})
	if response.Code != http.StatusOK {
		t.Fatalf("resume from secondary: %s", response.Body.String())
	}
	response = subscriptionEditRequest(t, router, http.MethodGet, "/account-options", nil)
	if err := json.Unmarshal(response.Body.Bytes(), &options); err != nil || response.Code != http.StatusOK || len(options.Accounts) != 2 {
		t.Fatalf("resumed options: %s, %v", response.Body.String(), err)
	}
	response = subscriptionEditRequest(t, router, http.MethodPost, "/subscriptions", payload)
	if response.Code != http.StatusOK {
		t.Fatalf("resumed secondary creation: %s", response.Body.String())
	}
}
