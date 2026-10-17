package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"carpool-notify/internal/config"
	"carpool-notify/internal/cycle"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func renewalRoutesTestServer(t *testing.T) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "index.html"), []byte("<html>SPA fallback</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	newService := func(name string) *service.SubscriptionService {
		store, err := db.Open(filepath.Join(directory, name+".db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = store.Close() })
		return &service.SubscriptionService{
			Store: store,
			Clock: func() time.Time {
				return time.Date(2026, time.August, 20, 10, 0, 0, 0, cycle.Location)
			},
		}
	}
	server, err := NewServer(newService("live"), config.Config{
		Password: "test-password", SessionSecret: "test-session-secret",
	}, directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(server.Close)
	server.SandboxService = newService("sandbox")
	if err := server.SandboxService.Store.SetSetting("sandbox_access_token", "sandbox-token"); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(sessions.Sessions("csrf_test_session", cookie.NewStore([]byte("test-session-secret"))))
	server.RegisterRoutes(router)
	return server, router
}

func seedHistoricalRenewal(t *testing.T, subscriptionService *service.SubscriptionService, token string) int64 {
	t.Helper()
	subscriptionID, err := subscriptionService.CreateWithInitialBill(service.CreateInput{
		Name: "legacy customer", BusinessType: model.SubscriptionBusinessPlus,
		CustomerEmail: "customer@example.com", CustomerWechat: "customer",
		PriceYuan: "90.00", CronExpr: "interval:30d", BoardedAt: "2026-08-01",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Seed an existing application directly, without the removed public flow.
	applicationID, err := subscriptionService.Store.CreateRenewalApplication(model.RenewalApplication{
		TrackingToken: token, SubscriptionID: subscriptionID,
		CustomerEmail: "customer@example.com", DueDate: "2026-08-31",
		PeriodCount: 1, PeriodEndDate: "2026-09-30", AmountCents: 9000,
	})
	if err != nil {
		t.Fatal(err)
	}
	return applicationID
}

func TestRemovedRenewalRoutesReturnJSONNotFound(t *testing.T) {
	server, router := renewalRoutesTestServer(t)
	seedHistoricalRenewal(t, server.Service, "legacy-live-token")
	seedHistoricalRenewal(t, server.SandboxService, "legacy-sandbox-token")
	client := csrfTestClient{router: router}
	clientIndex := 1
	for _, authenticated := range []bool{false, true} {
		if authenticated {
			if response := client.do(t, http.MethodPost, "/api/login", `{"password":"test-password"}`, ""); response.Code != http.StatusOK {
				t.Fatalf("login failed: %d %s", response.Code, response.Body.String())
			}
		}
		for _, prefix := range []string{"/api", "/api/sandbox"} {
			for _, route := range []struct {
				method string
				path   string
			}{
				{http.MethodGet, "/renewal-applications"},
				{http.MethodPost, "/renewal-applications/1/approve"},
				{http.MethodPost, "/renewal-applications/1/reject"},
				{http.MethodPost, "/renewal/lookup"},
				{http.MethodPost, "/renewal"},
				{http.MethodGet, "/renewal/legacy-live-token"},
				{http.MethodGet, "/renewal/legacy-sandbox-token"},
			} {
				for _, query := range []string{"", "?sandbox_token=sandbox-token"} {
					path := prefix + route.path + query
					t.Run(route.method+" "+path, func(t *testing.T) {
						// Each case checks one removed route independently; avoid
						// simulating a single client probing dozens of missing APIs.
						client.remoteAddr = "198.51.100." + strconv.Itoa(clientIndex) + ":1234"
						clientIndex++
						response := client.do(t, route.method, path,
							`{"customer_email":"customer@example.com","subscription_id":1,"period_count":1}`, "")
						if response.Code != http.StatusNotFound {
							t.Fatalf("status = %d, want 404: %s", response.Code, response.Body.String())
						}
						if !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
							t.Fatalf("response is not JSON: %s", response.Body.String())
						}
						var payload struct {
							OK    bool   `json:"ok"`
							Error string `json:"error"`
						}
						if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.OK || payload.Error != "not found" {
							t.Fatalf("unexpected response: %s, error = %v", response.Body.String(), err)
						}
					})
				}
			}
		}
	}
	for _, subscriptionService := range []*service.SubscriptionService{server.Service, server.SandboxService} {
		applications, err := subscriptionService.Store.ListRenewalApplications("")
		if err != nil || len(applications) != 1 || applications[0].Status != model.RenewalStatusPending {
			t.Fatalf("removed routes changed historical applications: %#v, error = %v", applications, err)
		}
		bills, err := subscriptionService.Store.ListBills()
		if err != nil || len(bills) != 1 {
			t.Fatalf("removed routes changed bills: %#v, error = %v", bills, err)
		}
	}
	if response := client.do(t, http.MethodGet, "/redeem", "", ""); response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), "SPA fallback") {
		t.Fatalf("SPA route no longer works: %d %s", response.Code, response.Body.String())
	}
}

func TestManualRenewalRoutesRemainAvailableAndProtected(t *testing.T) {
	server, router := renewalRoutesTestServer(t)
	seedHistoricalRenewal(t, server.Service, "legacy-live-token")
	seedHistoricalRenewal(t, server.SandboxService, "legacy-sandbox-token")
	client := csrfTestClient{router: router}
	for _, prefix := range []string{"/api", "/api/sandbox"} {
		for _, path := range []string{"/subscriptions/1/due/2026-08-31/paid", "/accounts/1/renew"} {
			response := client.do(t, http.MethodPost, prefix+path, "{}", "")
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated %s: %d %s", prefix+path, response.Code, response.Body.String())
			}
		}
	}
	if response := client.do(t, http.MethodPost, "/api/login", `{"password":"test-password"}`, ""); response.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", response.Code, response.Body.String())
	}
	csrfToken := decodeCSRFToken(t, client.do(t, http.MethodGet, "/api/csrf-token", "", ""))
	for _, prefix := range []string{"/api", "/api/sandbox"} {
		path := prefix + "/subscriptions/1/due/2026-08-31/paid"
		if response := client.do(t, http.MethodPost, path, `{"paid":true}`, ""); response.Code != http.StatusForbidden {
			t.Fatalf("missing CSRF for %s: %d %s", path, response.Code, response.Body.String())
		}
		if response := client.do(t, http.MethodPost, path, `{"paid":true}`, csrfToken); response.Code != http.StatusOK {
			t.Fatalf("manual renewal failed for %s: %d %s", path, response.Code, response.Body.String())
		}
	}
	for _, subscriptionService := range []*service.SubscriptionService{server.Service, server.SandboxService} {
		bills, err := subscriptionService.Store.ListBills()
		if err != nil || len(bills) != 2 {
			t.Fatalf("manual renewal bills: %#v, error = %v", bills, err)
		}
	}
}
