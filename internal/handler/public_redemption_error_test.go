package handler

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"carpool-notify/internal/db"
	"carpool-notify/internal/service"

	"github.com/gin-gonic/gin"
)

func TestPublicRedemptionSeparatesValidationFromStorageFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := db.Open(filepath.Join(t.TempDir(), "public-errors.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	server := &Server{Service: &service.SubscriptionService{Store: store}}
	router := gin.New()
	router.GET("/api/redeem-settings", server.getRedeemSettings)
	router.POST("/api/redeem", server.postRedeemApplication)
	router.GET("/api/redeem/:token", server.getRedeemStatus)
	body := `{"customer_email":"customer@example.com","customer_contact":"wx-customer","redeem_code":"MISSING-CODE"}`
	request := func(method, path, body string, want int) {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		if recorder.Code != want {
			t.Fatalf("%s %s = %d: %s; want %d", method, path, recorder.Code, recorder.Body.String(), want)
		}
		if want == http.StatusServiceUnavailable && !strings.Contains(recorder.Body.String(), "服务暂时不可用，请稍后重试") {
			t.Fatalf("storage failure leaked details: %s", recorder.Body.String())
		}
	}
	request(http.MethodPost, "/api/redeem", body, http.StatusBadRequest)
	request(http.MethodGet, "/api/redeem/unknown-token", "", http.StatusNotFound)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	request(http.MethodGet, "/api/redeem-settings", "", http.StatusServiceUnavailable)
	request(http.MethodPost, "/api/redeem", body, http.StatusServiceUnavailable)
	request(http.MethodGet, "/api/redeem/unknown-token", "", http.StatusServiceUnavailable)
}
