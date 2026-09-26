package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"

	"github.com/gin-gonic/gin"
)

func subscriptionEditTestServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store, err := db.Open(filepath.Join(t.TempDir(), "subscription-edit.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	subscriptionService := &service.SubscriptionService{
		Store: store,
		Clock: func() time.Time {
			return time.Date(2026, time.August, 20, 10, 0, 0, 0, cycle.Location)
		},
	}
	settings := model.DefaultRedeemPageSettings
	settings.PaymentQRCodeDataURL = "data:image/png;base64,iVBORw0KGgo="
	if err := subscriptionService.SaveRedeemPageSettings(settings); err != nil {
		t.Fatal(err)
	}
	server := &Server{Service: subscriptionService}
	router := gin.New()
	router.GET("/subscriptions", server.getSubscriptions)
	router.GET("/accounts", server.getAccounts)
	router.POST("/subscriptions", server.postCreateSubscription)
	router.PUT("/subscriptions/:id", server.putUpdateSubscription)
	return server, router
}

func subscriptionEditRequest(t *testing.T, router *gin.Engine, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestPutSubscriptionRejectsStaleBrowserFormAfterRenewalApproval(t *testing.T) {
	for _, entry := range []string{"team_list", "team_seat", "plus_list"} {
		for _, nextPrice := range []string{"", "35.00"} {
			t.Run(entry+"/next_price="+nextPrice, func(t *testing.T) {
				server, router := subscriptionEditTestServer(t)
				subscriptionService := server.Service
				businessType := model.SubscriptionBusinessTeam
				var accountID int64
				if entry == "plus_list" {
					businessType = model.SubscriptionBusinessPlus
				} else {
					var err error
					accountID, err = subscriptionService.CreateAccount(service.CreateAccountInput{
						Name: "owner@example.com", OpenedAt: "2026-08-01", CostYuan: "5.00", SeatCount: 1,
					})
					if err != nil {
						t.Fatal(err)
					}
				}
				payload := map[string]any{
					"name": "customer", "business_type": businessType,
					"customer_email": "customer@example.com", "customer_wechat": "customer",
					"price_yuan": "30.00", "cost_yuan": "5.00", "cron_expr": "interval:30d",
					"boarded_at": "2026-08-01", "remark": "original note", "account_id": accountID,
				}
				// Creation deliberately omits expected_updated_at.
				response := subscriptionEditRequest(t, router, http.MethodPost, "/subscriptions", payload)
				if response.Code != http.StatusOK {
					t.Fatalf("create: %d %s", response.Code, response.Body.String())
				}
				var listed struct {
					Subscriptions []service.SubscriptionView `json:"subscriptions"`
				}
				readList := func() {
					t.Helper()
					response := subscriptionEditRequest(t, router, http.MethodGet, "/subscriptions", nil)
					if response.Code != http.StatusOK {
						t.Fatalf("list: %d %s", response.Code, response.Body.String())
					}
					if err := json.Unmarshal(response.Body.Bytes(), &listed); err != nil || len(listed.Subscriptions) != 1 {
						t.Fatalf("list payload = %s, %v", response.Body.String(), err)
					}
				}
				readList()
				subscriptionID := listed.Subscriptions[0].Subscription.ID
				path := fmt.Sprintf("/subscriptions/%d", subscriptionID)
				payload["seat_id"] = listed.Subscriptions[0].SeatID
				payload["expected_updated_at"] = listed.Subscriptions[0].Subscription.UpdatedAt.Format(time.RFC3339Nano)
				payload["next_price_yuan"] = nextPrice
				if nextPrice != "" {
					response = subscriptionEditRequest(t, router, http.MethodPut, path, payload)
					if response.Code != http.StatusOK {
						t.Fatalf("schedule price: %d %s", response.Code, response.Body.String())
					}
				}
				subscriptionService.Clock = func() time.Time {
					return time.Date(2026, time.August, 31, 10, 0, 0, 0, cycle.Location)
				}
				readList()
				payload["expected_updated_at"] = listed.Subscriptions[0].Subscription.UpdatedAt.Format(time.RFC3339Nano)
				if entry == "team_seat" {
					var accounts struct {
						Accounts []service.AccountView `json:"accounts"`
					}
					response = subscriptionEditRequest(t, router, http.MethodGet, "/accounts", nil)
					if err := json.Unmarshal(response.Body.Bytes(), &accounts); err != nil || response.Code != http.StatusOK || len(accounts.Accounts) != 1 || len(accounts.Accounts[0].Seats) != 1 {
						t.Fatalf("account prefill: %s, %v", response.Body.String(), err)
					}
					seat := accounts.Accounts[0].Seats[0]
					if seat.ActiveSubscriptionUpdatedAt != payload["expected_updated_at"] {
						t.Fatalf("seat version %q differs from list version %q", seat.ActiveSubscriptionUpdatedAt, payload["expected_updated_at"])
					}
					payload["expected_updated_at"] = seat.ActiveSubscriptionUpdatedAt
					payload["price_yuan"] = seat.ActivePriceYuan
					payload["next_price_yuan"] = seat.ActiveNextPriceYuan
				}
				// The form is now open. Approve a real renewal before submitting it.
				if _, err := subscriptionService.SubmitRenewalApplication(service.RenewalSubmitInput{
					SubscriptionID: subscriptionID, CustomerEmail: "customer@example.com",
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
				wantPrice := int64(3000)
				if nextPrice != "" {
					wantPrice = 3500
				}
				if err != nil || approved.PricePerPersonCents != wantPrice || approved.NextPriceCents != nil || approved.NextPriceEffectiveDueDate != "" || !approved.UpdatedAt.After(listed.Subscriptions[0].Subscription.UpdatedAt) {
					t.Fatalf("approved subscription = %#v, %v", approved, err)
				}
				bills, err := subscriptionService.Store.ListBills()
				if err != nil || len(bills) != 2 {
					t.Fatalf("approved bills = %#v, %v", bills, err)
				}
				approvedBill, err := subscriptionService.Store.GetBillByOccurrence(subscriptionID, "2026-08-31")
				if err != nil || approvedBill.AmountCents != wantPrice {
					t.Fatalf("current renewal bill = %#v, %v", approvedBill, err)
				}
				for _, field := range []string{"price_yuan", "cost_yuan", "remark"} {
					previous := payload[field]
					payload[field] = "40.00"
					response = subscriptionEditRequest(t, router, http.MethodPut, path, payload)
					if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "刷新") {
						t.Fatalf("stale %s edit: %d %s", field, response.Code, response.Body.String())
					}
					payload[field] = previous
				}
				after, err := subscriptionService.Get(subscriptionID)
				if err != nil || !reflect.DeepEqual(approved, after) {
					t.Fatalf("stale request changed subscription: %#v, %v", after, err)
				}
				afterBills, err := subscriptionService.Store.ListBills()
				if err != nil || !reflect.DeepEqual(bills, afterBills) {
					t.Fatalf("stale request changed bills: %#v, %v", afterBills, err)
				}
				// Reloading the version and financial fields allows a normal edit.
				payload["expected_updated_at"] = approved.UpdatedAt.Format(time.RFC3339Nano)
				payload["price_yuan"], payload["next_price_yuan"] = cycle.FormatCents(wantPrice), ""
				payload["remark"] = "refreshed form note"
				response = subscriptionEditRequest(t, router, http.MethodPut, path, payload)
				if response.Code != http.StatusOK {
					t.Fatalf("refreshed edit: %d %s", response.Code, response.Body.String())
				}
			})
		}
	}
}

func TestPutSubscriptionRequiresValidBrowserVersion(t *testing.T) {
	server, router := subscriptionEditTestServer(t)
	id, err := server.Service.CreateWithInitialBill(service.CreateInput{
		Name: "customer", BusinessType: model.SubscriptionBusinessPlus, CustomerEmail: "customer@example.com",
		CustomerWechat: "customer", PriceYuan: "30.00", CronExpr: "interval:30d", BoardedAt: "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := server.Service.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []any{nil, "", "invalid", "2026-08-01", "0001-01-01T00:00:00Z"} {
		payload := map[string]any{"remark": "must not persist"}
		if version != nil {
			payload["expected_updated_at"] = version
		}
		response := subscriptionEditRequest(t, router, http.MethodPut, fmt.Sprintf("/subscriptions/%d", id), payload)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "刷新") {
			t.Fatalf("version %#v: %d %s", version, response.Code, response.Body.String())
		}
	}
	after, err := server.Service.Get(id)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("unversioned request changed subscription: %#v, %v", after, err)
	}
}
