package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"carpool-notify/internal/config"
	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
	"carpool-notify/internal/service"

	"github.com/gin-gonic/gin"
)

func TestPutSettingsRollsBackConfigFileWhenDatabaseSaveFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	directory := t.TempDir()
	store, err := db.Open(filepath.Join(directory, "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	subscriptionService := &service.SubscriptionService{Store: store}
	configPath := filepath.Join(directory, "config.toml")
	originalConfig := []byte(`[server]
password = "admin-password"
session_secret = "test-only-session-secret-at-least-32-bytes"

[smtp]
host = "smtp.old.example"
port = 587
username = "old-user"
password = "old-secret"
from = "old@example.com"
to = "operator@example.com"
`)
	if err := os.WriteFile(configPath, originalConfig, 0o600); err != nil {
		t.Fatal(err)
	}
	server := &Server{
		Service: subscriptionService,
		Config:  config.Config{ConfigPath: configPath},
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	payload := map[string]any{
		"notify_template":                        model.DefaultNotifyTemplate,
		"customer_email_template":                model.DefaultCustomerEmailTemplate,
		"price_increase_customer_email_template": model.DefaultPriceIncreaseCustomerEmailTemplate,
		"price_decrease_customer_email_template": model.DefaultPriceDecreaseCustomerEmailTemplate,
		"channels":                               []string{"smtp"},
		"notification_config": map[string]any{
			"smtp": map[string]any{
				"host": "smtp.new.example", "port": 465, "username": "new-user",
				"password": "new-secret", "from": "new@example.com", "to": "new-operator@example.com",
			},
			"iyuu":   map[string]any{},
			"gotify": map[string]any{},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	server.putSettings(context)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	restored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(originalConfig) {
		t.Fatalf("failed settings save left config partially updated:\n%s", restored)
	}
}

func TestPutSettingsValidatesWholeRequestBeforePersisting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := db.Open(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	subscriptionService := &service.SubscriptionService{Store: store}
	server := &Server{Service: subscriptionService}

	before, err := subscriptionService.GetNotifyTemplate()
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"notify_template":                        "this must not be persisted",
		"customer_email_template":                model.DefaultCustomerEmailTemplate,
		"price_increase_customer_email_template": model.DefaultPriceIncreaseCustomerEmailTemplate,
		"price_decrease_customer_email_template": model.DefaultPriceDecreaseCustomerEmailTemplate,
		"channels":                               []string{"smtp"},
		"notification_config": map[string]any{
			"smtp":   map[string]any{"port": 70000},
			"iyuu":   map[string]any{},
			"gotify": map[string]any{},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	server.putSettings(context)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "smtp port") {
		t.Fatalf("response = %d %s, want invalid SMTP port", recorder.Code, recorder.Body.String())
	}
	after, err := subscriptionService.GetNotifyTemplate()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("failed settings request partially persisted notify template: before=%q after=%q", before, after)
	}
}

func TestPriceIncreaseCustomerTemplatePreviewUsesDistinctPricesAndSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := db.Open(filepath.Join(t.TempDir(), "settings-preview.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	server := &Server{Service: &service.SubscriptionService{Store: store}}
	payload, err := json.Marshal(map[string]string{
		"kind":     "customer_price_increase",
		"template": "原价 ¥{{.PreviousPrice}}，新价 ¥{{.AmountDue}}",
	})
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/settings/template-preview",
		bytes.NewReader(payload),
	)
	context.Request.Header.Set("Content-Type", "application/json")

	server.postSettingsTemplatePreview(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	responseBody := recorder.Body.String()
	for _, expected := range []string{"价格调整通知", "20.00", "30.00"} {
		if !strings.Contains(responseBody, expected) {
			t.Fatalf("preview response = %s, missing %q", responseBody, expected)
		}
	}
}

func TestSandboxCustomerEmailTestNeverSendsRealMail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/sandbox/settings/test-customer-email",
		strings.NewReader(`{"recipient":"test@example.com","template_kind":"customer"}`),
	)
	context.Request.Header.Set("Content-Type", "application/json")
	server := &Server{SandboxMode: true}

	server.postSettingsTestCustomerEmail(context)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "演练模式不会发送真实邮件") {
		t.Fatalf("response = %d %s, want sandbox email rejection", recorder.Code, recorder.Body.String())
	}
}

func TestCopySubscriptionRejectsMalformedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: "1"}}
	context.Request = httptest.NewRequest(http.MethodPost, "/api/subscriptions/1/copy", strings.NewReader(`{"seat_id":`))
	context.Request.Header.Set("Content-Type", "application/json")

	server := &Server{}
	server.postCopySubscription(context)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "无效的请求") {
		t.Fatalf("response = %d %s, want malformed request error", recorder.Code, recorder.Body.String())
	}
}

func TestExportResponseDisablesCaching(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := db.Open(filepath.Join(t.TempDir(), "export.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/export", nil)
	server := &Server{Service: &service.SubscriptionService{Store: store}}
	server.getExport(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if value := recorder.Header().Get("Cache-Control"); value != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", value)
	}
	if value := recorder.Header().Get("X-Content-Type-Options"); value != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", value)
	}
}

func TestBusinessEmailSettingsDefaultsPreservationValidationAndPreview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store, err := db.Open(filepath.Join(t.TempDir(), "business-email-settings.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	server := &Server{Service: &service.SubscriptionService{Store: store}}
	request := func(method, url string, input any, handler func(*gin.Context)) *httptest.ResponseRecorder {
		t.Helper()
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest(method, url, bytes.NewReader(encoded))
		context.Request.Header.Set("Content-Type", "application/json")
		handler(context)
		return recorder
	}
	defaults := request(http.MethodGet, "/api/settings", nil, server.getSettings)
	if defaults.Code != http.StatusOK || !strings.Contains(defaults.Body.String(), "renewal_success_customer_email_template") || !strings.Contains(defaults.Body.String(), "customer_benefit_email_template") {
		t.Fatalf("defaults = %d %s", defaults.Code, defaults.Body.String())
	}
	payload := map[string]any{
		"notify_template": model.DefaultNotifyTemplate, "customer_email_template": model.DefaultCustomerEmailTemplate,
		"price_increase_customer_email_template":  model.DefaultPriceIncreaseCustomerEmailTemplate,
		"price_decrease_customer_email_template":  model.DefaultPriceDecreaseCustomerEmailTemplate,
		"renewal_success_customer_email_template": "custom renewal {{.AmountPaid}}", "customer_benefit_email_template": "custom benefit {{.BenefitType}}",
	}
	saved := request(http.MethodPut, "/api/settings", payload, server.putSettings)
	if saved.Code != http.StatusOK {
		t.Fatalf("save = %d %s", saved.Code, saved.Body.String())
	}
	delete(payload, "renewal_success_customer_email_template")
	delete(payload, "customer_benefit_email_template")
	saved = request(http.MethodPut, "/api/settings", payload, server.putSettings)
	if saved.Code != http.StatusOK {
		t.Fatalf("legacy save = %d %s", saved.Code, saved.Body.String())
	}
	if got, err := server.Service.GetRenewalSuccessCustomerEmailTemplate(); err != nil || got != "custom renewal {{.AmountPaid}}" {
		t.Fatalf("legacy overwrite = %q %v", got, err)
	}
	if got, err := server.Service.GetCustomerBenefitEmailTemplate(); err != nil || got != "custom benefit {{.BenefitType}}" {
		t.Fatalf("legacy overwrite = %q %v", got, err)
	}
	payload["notify_template"] = "must not persist"
	payload["customer_benefit_email_template"] = `{{if false}}{{.Unknown}}{{end}}`
	rejected := request(http.MethodPut, "/api/settings", payload, server.putSettings)
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("invalid template = %d %s", rejected.Code, rejected.Body.String())
	}
	if got, err := server.Service.GetNotifyTemplate(); err != nil || got != model.DefaultNotifyTemplate {
		t.Fatalf("partial save = %q %v", got, err)
	}
	for _, kind := range []string{"customer_renewal_success", "customer_benefit_extension", "customer_benefit_discount", "customer_benefit_revision", "customer_benefit_revoke"} {
		body := model.DefaultCustomerBenefitEmailTemplate
		label := "福利信息"
		if kind == "customer_renewal_success" {
			body, label = model.DefaultRenewalSuccessCustomerEmailTemplate, "续费成功"
		}
		preview := request(http.MethodPost, "/api/settings/template-preview", map[string]string{"kind": kind, "template": body}, server.postSettingsTemplatePreview)
		if preview.Code != http.StatusOK || !strings.Contains(preview.Body.String(), "虚拟示例") || !strings.Contains(preview.Body.String(), label) {
			t.Fatalf("preview %s = %d %s", kind, preview.Code, preview.Body.String())
		}
	}
}
