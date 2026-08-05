package db_test

import (
	"path/filepath"
	"strings"
	"testing"

	"carpool-notify/internal/db"
	"carpool-notify/internal/model"
)

func TestOpenMigratesTemplateNameVariableToCustomerEmail(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "carpool.db")
	store := openStore(t, databasePath)
	if err := store.SetSetting(model.SettingNotifyTemplate, "notify {{.Name}}"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(model.SettingCustomerEmailTemplate, "email {{ .Name }}"); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	defer store.Close()

	for key, wantPrefix := range map[string]string{
		model.SettingNotifyTemplate:        "notify ",
		model.SettingCustomerEmailTemplate: "email ",
	} {
		templateBody, err := store.GetSetting(key)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(templateBody, ".Name") {
			t.Fatalf("%s = %q, should not keep .Name", key, templateBody)
		}
		if templateBody != wantPrefix+"{{.CustomerEmail}}" {
			t.Fatalf("%s = %q, want customer email variable", key, templateBody)
		}
	}
}

func TestOpenKeepsTemplateThatAlreadyUsesCustomerEmail(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "carpool.db")
	store := openStore(t, databasePath)
	templateBody := "notify {{.CustomerEmail}} / legacy {{.Name}}"
	if err := store.SetSetting(model.SettingNotifyTemplate, templateBody); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	defer store.Close()

	got, err := store.GetSetting(model.SettingNotifyTemplate)
	if err != nil {
		t.Fatal(err)
	}
	if got != templateBody {
		t.Fatalf("template = %q, want preserved custom template", got)
	}
}

func TestOpenUpdatesLegacyDefaultCustomerEmailTemplate(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "carpool.db")
	store := openStore(t, databasePath)
	legacyDefault := `您好，这是关于「{{.CustomerEmail}}」的拼车提醒。

本期应收：¥{{.AmountDue}}
周期：{{.CycleDesc}}
到期：{{.NextDueDate}}
{{if .Remark}}备注：{{.Remark}}{{end}}
{{if .TradeURL}}链接：{{.TradeURL}}{{end}}

请按时缴费，谢谢。`
	if err := store.SetSetting(model.SettingCustomerEmailTemplate, legacyDefault); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	defer store.Close()

	got, err := store.GetSetting(model.SettingCustomerEmailTemplate)
	if err != nil {
		t.Fatal(err)
	}
	if got != model.DefaultCustomerEmailTemplate {
		t.Fatalf("customer template = %q, want new default", got)
	}
}

func TestOpenUpdatesDueSoonDefaultTemplatesToDueInText(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "carpool.db")
	store := openStore(t, databasePath)
	oldNotify := `【拼车收钱】{{.CustomerEmail}}
本期应收：¥{{.AmountDue}}
周期：{{.CycleDesc}}
到期：{{.NextDueDate}}
{{if .Remark}}备注：{{.Remark}}{{end}}
{{if .TradeURL}}链接：{{.TradeURL}}{{end}}`
	oldCustomer := `您好，您的拼车服务即将到期，请及时续费，以免影响正常使用。

客户邮箱：{{.CustomerEmail}}
本期应收：¥{{.AmountDue}}
计费周期：{{.CycleDesc}}
到期日期：{{.NextDueDate}}
{{if .Remark}}备注：{{.Remark}}{{end}}
{{if .TradeURL}}续费链接：{{.TradeURL}}{{end}}

如需续费或有疑问，请添加 / 联系微信：Jerrylove_Bom
谢谢。`
	if err := store.SetSetting(model.SettingNotifyTemplate, oldNotify); err != nil {
		t.Fatal(err)
	}
	if err := store.SetSetting(model.SettingCustomerEmailTemplate, oldCustomer); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store = openStore(t, databasePath)
	defer store.Close()

	for key, want := range map[string]string{
		model.SettingNotifyTemplate:        model.DefaultNotifyTemplate,
		model.SettingCustomerEmailTemplate: model.DefaultCustomerEmailTemplate,
	} {
		got, err := store.GetSetting(key)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
}

func TestRedemptionApplicationLifecycle(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "carpool.db"))
	defer store.Close()

	applicationID, err := store.CreateRedemptionApplication(model.RedemptionApplication{
		TrackingToken:   "token-abc",
		CustomerEmail:   "customer@example.com",
		CustomerContact: "wx-customer",
		RedeemCode:      "CODE701",
		RequestNote:     "please invite",
	})
	if err != nil {
		t.Fatal(err)
	}

	application, err := store.GetRedemptionApplicationByToken("token-abc")
	if err != nil {
		t.Fatal(err)
	}
	if application.ID != applicationID || application.Status != model.RedemptionStatusPending {
		t.Fatalf("application = id %d status %q, want id %d pending", application.ID, application.Status, applicationID)
	}
	if application.CustomerEmail != "customer@example.com" || application.CustomerContact != "wx-customer" {
		t.Fatalf("customer fields = %q / %q", application.CustomerEmail, application.CustomerContact)
	}

	pendingCount, err := store.CountRedemptionApplicationsByStatus(model.RedemptionStatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if pendingCount != 1 {
		t.Fatalf("pending count = %d, want 1", pendingCount)
	}

	if err := store.MarkRedemptionApplicationInvited(applicationID, 11, 22, 33, "sent"); err != nil {
		t.Fatal(err)
	}

	updated, err := store.GetRedemptionApplication(applicationID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != model.RedemptionStatusInvited {
		t.Fatalf("status = %q, want invited", updated.Status)
	}
	if updated.AssignedAccountID != 11 || updated.AssignedSeatID != 22 || updated.AssignedSubscriptionID != 33 {
		t.Fatalf("assigned ids = %d/%d/%d", updated.AssignedAccountID, updated.AssignedSeatID, updated.AssignedSubscriptionID)
	}
	if updated.OperatorNote != "sent" || updated.InvitedAt == nil {
		t.Fatalf("operator note/invited_at = %q/%v", updated.OperatorNote, updated.InvitedAt)
	}
}

func openStore(t *testing.T, databasePath string) *db.Store {
	t.Helper()
	store, err := db.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
