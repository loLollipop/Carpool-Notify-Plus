package handler

import (
	"fmt"
	"net/http"
	"testing"

	"carpool-notify/internal/service"
)

func TestAccountSerialAnchorHTTPContract(t *testing.T) {
	server, router := subscriptionEditTestServer(t)
	router.PUT("/accounts/:id/serial", server.putAccountSerial)
	id, err := server.Service.CreateAccount(service.CreateAccountInput{Name: "anchor@example.test", SeatCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("/accounts/%d/serial", id)
	for _, payload := range []map[string]any{
		{"serial": 47},
		{"serial": 0, "expected_serial": id},
		{"serial": "47", "expected_serial": id},
	} {
		response := subscriptionEditRequest(t, router, http.MethodPut, url, payload)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid serial accepted: %s", response.Body.String())
		}
	}
	response := subscriptionEditRequest(t, router, http.MethodPut, url, map[string]any{"serial": 47, "expected_serial": id})
	if response.Code != http.StatusOK {
		t.Fatalf("anchor failed: %s", response.Body.String())
	}
	view, err := server.Service.GetAccountView(id)
	if err != nil || view.DisplaySerial != 47 {
		t.Fatalf("serial = %d, %v", view.DisplaySerial, err)
	}
	response = subscriptionEditRequest(t, router, http.MethodPut, url, map[string]any{"serial": 50, "expected_serial": id})
	if response.Code != http.StatusBadRequest {
		t.Fatal("stale adjustment accepted")
	}
}
