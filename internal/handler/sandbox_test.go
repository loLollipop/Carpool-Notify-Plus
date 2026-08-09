package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"

	"github.com/gin-gonic/gin"
)

func TestSandboxSettingsTestNotifyIsBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/sandbox/settings/test-notify", nil)

	server := &Server{SandboxMode: true}
	server.postSettingsTestNotify(context)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if !strings.Contains(recorder.Body.String(), "演练模式不会发送真实通知") {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}

func TestSandboxRedeemSettingsUseSandboxStore(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tempDir := t.TempDir()
	liveStore, err := db.Open(filepath.Join(tempDir, "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = liveStore.Close() })
	sandboxStore, err := db.Open(filepath.Join(tempDir, "sandbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sandboxStore.Close() })

	liveService := &service.SubscriptionService{Store: liveStore}
	sandboxService := &service.SubscriptionService{Store: sandboxStore}
	liveSettings := model.DefaultRedeemPageSettings
	liveSettings.AnnouncementTitle = "live settings"
	if err := liveService.SaveRedeemPageSettings(liveSettings); err != nil {
		t.Fatal(err)
	}
	sandboxSettings := model.DefaultRedeemPageSettings
	sandboxSettings.AnnouncementTitle = "sandbox settings"
	if err := sandboxService.SaveRedeemPageSettings(sandboxSettings); err != nil {
		t.Fatal(err)
	}
	if err := sandboxStore.SetSetting("sandbox_access_token", "sandbox-token"); err != nil {
		t.Fatal(err)
	}

	server := &Server{Service: liveService, SandboxService: sandboxService}
	router := gin.New()
	server.RegisterRoutes(router)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/sandbox/redeem-settings?sandbox_token=sandbox-token",
		nil,
	)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		OK         bool                     `json:"ok"`
		RedeemPage model.RedeemPageSettings `json:"redeem_page"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK {
		t.Fatal("response is not ok")
	}
	if payload.RedeemPage.AnnouncementTitle != sandboxSettings.AnnouncementTitle {
		t.Fatalf(
			"title = %q, want %q",
			payload.RedeemPage.AnnouncementTitle,
			sandboxSettings.AnnouncementTitle,
		)
	}
}
