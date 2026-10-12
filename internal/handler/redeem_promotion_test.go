package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"

	"github.com/gin-gonic/gin"
)

func TestRedeemPromotionEditorSavePreservesOmittedLegacySettings(t *testing.T) {
	server, _ := subscriptionEditTestServer(t)
	settings := model.DefaultRedeemPageSettings
	settings.RenewalAnnouncementTitle = "历史公告"
	settings.RenewalAnnouncementIntro = "历史说明"
	settings.RenewalAnnouncementItems = []string{"历史付款说明"}
	settings.PaymentTitle = "历史收款码"
	settings.PaymentDescription = "历史付款提示"
	settings.PaymentQRCodeDataURL = "data:image/png;base64,iVBORw0KGgo="
	if err := server.Service.SaveRedeemPageSettings(settings); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	var editorSettings map[string]any
	if err := json.Unmarshal(encoded, &editorSettings); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"renewal_announcement_title", "renewal_announcement_intro",
		"renewal_announcement_items", "payment_title", "payment_description", "payment_qr_data_url"} {
		delete(editorSettings, key)
	}
	editorSettings["promotion_enabled"] = false
	editorSettings["promotion_text"] = "修改后的推广文案"
	requestBody, err := json.Marshal(map[string]any{
		"notify_template":                        model.DefaultNotifyTemplate,
		"customer_email_template":                model.DefaultCustomerEmailTemplate,
		"price_increase_customer_email_template": model.DefaultPriceIncreaseCustomerEmailTemplate,
		"price_decrease_customer_email_template": model.DefaultPriceDecreaseCustomerEmailTemplate,
		"channels":                               []string{}, "redeem_page": editorSettings,
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(requestBody))
	context.Request.Header.Set("Content-Type", "application/json")
	server.putSettings(context)
	if recorder.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	stored, err := server.Service.GetRedeemPageSettings()
	if err != nil {
		t.Fatal(err)
	}
	if stored.PromotionEnabled || stored.PromotionText != "修改后的推广文案" ||
		stored.RenewalAnnouncementTitle != settings.RenewalAnnouncementTitle ||
		stored.RenewalAnnouncementIntro != settings.RenewalAnnouncementIntro ||
		len(stored.RenewalAnnouncementItems) != 1 || stored.RenewalAnnouncementItems[0] != settings.RenewalAnnouncementItems[0] ||
		stored.PaymentTitle != settings.PaymentTitle || stored.PaymentDescription != settings.PaymentDescription ||
		stored.PaymentQRCodeDataURL != settings.PaymentQRCodeDataURL {
		t.Fatalf("editor save did not preserve legacy settings or explicit false: %#v", stored)
	}
	input, err := server.decodeRedeemPageSettingsUpdate(json.RawMessage(`{"payment_qr_data_url":""}`))
	if err != nil || input.PaymentQRCodeDataURL != "" {
		t.Fatalf("explicit legacy clear = %#v, err = %v", input, err)
	}
	for _, raw := range []string{`[]`, `false`, `null`, `{"promotion_enabled":"false"}`} {
		if _, err := server.decodeRedeemPageSettingsUpdate(json.RawMessage(raw)); err == nil {
			t.Fatalf("accepted invalid settings %s", raw)
		}
	}
}

func TestRedeemPromotionPublicSettingsResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, enabled := range []bool{true, false} {
		name := "enabled"
		if !enabled {
			name = "disabled"
		}
		t.Run(name, func(t *testing.T) {
			fixtureDB, err := db.Open(filepath.Join(t.TempDir(), "redeem-promotion.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = fixtureDB.Close() })
			subscriptionService := &service.SubscriptionService{Store: fixtureDB}
			settings := model.DefaultRedeemPageSettings
			settings.PromotionEnabled = enabled
			settings.PromotionBadge = "推荐"
			settings.PromotionText = "自营服务，欢迎对接～"
			settings.PromotionURL = "https://example.com/?from=redeem#promotion"
			settings.PromotionCTALabel = "去看看"
			if err := subscriptionService.SaveRedeemPageSettings(settings); err != nil {
				t.Fatal(err)
			}
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodGet, "/api/redeem-settings", nil)
			server := &Server{Service: subscriptionService}
			server.getRedeemSettings(context)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			var response struct {
				OK         bool                       `json:"ok"`
				RedeemPage map[string]json.RawMessage `json:"redeem_page"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if !response.OK {
				t.Fatalf("response = %s", recorder.Body.String())
			}
			for key, expected := range map[string]any{
				"promotion_enabled": enabled, "promotion_badge": settings.PromotionBadge,
				"promotion_text": settings.PromotionText, "promotion_url": settings.PromotionURL,
				"promotion_cta_label": settings.PromotionCTALabel,
			} {
				value, ok := response.RedeemPage[key]
				want, err := json.Marshal(expected)
				if err != nil {
					t.Fatal(err)
				}
				if !ok || string(value) != string(want) {
					t.Fatalf("%s = %s, present=%v, want %s", key, value, ok, want)
				}
			}
		})
	}
}
