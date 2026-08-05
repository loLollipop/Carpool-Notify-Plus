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

func openStore(t *testing.T, databasePath string) *db.Store {
	t.Helper()
	store, err := db.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	return store
}
