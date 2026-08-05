package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"carpool-notify/internal/config"
)

func TestLoadFromTOML(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.toml")
	content := `
[server]
listen = "127.0.0.1:9090"
db_path = "./tmp.db"
password = "toml-pass"
session_secret = "toml-secret"

[gotify]
url = "https://gotify.example.com/"
token = "gotify-token"

[iyuu]
token = "iyuu-token"
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"carpool-notify", "-config", configPath}

	// Ensure env does not override for this test.
	for _, key := range []string{
		"CARPOOL_PASSWORD", "CARPOOL_SESSION_SECRET", "CARPOOL_LISTEN", "CARPOOL_DB_PATH",
		"GOTIFY_URL", "GOTIFY_TOKEN", "IYUU_TOKEN", "CARPOOL_CONFIG",
	} {
		t.Setenv(key, "")
		_ = os.Unsetenv(key)
	}

	configuration, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Password != "toml-pass" {
		t.Fatalf("password: %q", configuration.Password)
	}
	if configuration.ListenAddress != "127.0.0.1:9090" {
		t.Fatalf("listen: %q", configuration.ListenAddress)
	}
	if configuration.GotifyURL != "https://gotify.example.com" {
		t.Fatalf("gotify url: %q", configuration.GotifyURL)
	}
	if !configuration.GotifyConfigured() || !configuration.IYUUConfigured() {
		t.Fatal("channels should be configured")
	}
}

func TestEnvOverridesTOML(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.toml")
	content := `
[server]
password = "toml-pass"
session_secret = "toml-secret"
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"carpool-notify", "-config", configPath}
	t.Setenv("CARPOOL_PASSWORD", "env-pass")

	configuration, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Password != "env-pass" {
		t.Fatalf("expected env override, got %q", configuration.Password)
	}
}

func TestUpdateNotificationConfigKeepsBlankSecrets(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.toml")
	content := `
[server]
password = "toml-pass"
session_secret = "toml-secret"

[smtp]
host = "smtp.old.example.com"
port = 465
username = "old-user"
password = "old-smtp-secret"
from = "old@example.com"
to = "ops@example.com"

[iyuu]
token = "old-iyuu-secret"

[gotify]
url = "https://gotify.old.example.com"
token = "old-gotify-secret"
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"carpool-notify", "-config", configPath}
	for _, key := range []string{
		"CARPOOL_PASSWORD", "CARPOOL_SESSION_SECRET", "CARPOOL_LISTEN", "CARPOOL_DB_PATH",
		"GOTIFY_URL", "GOTIFY_TOKEN", "IYUU_TOKEN", "CARPOOL_CONFIG",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_TO",
	} {
		t.Setenv(key, "")
		_ = os.Unsetenv(key)
	}

	configuration, err := config.UpdateNotificationConfig(configPath, config.NotificationConfigInput{
		SMTP: config.SMTPConfigInput{
			Host:     "smtp.qq.com",
			Port:     587,
			Username: "new-user",
			From:     "new@example.com",
			To:       "new-ops@example.com",
		},
		IYUU: config.SecretConfigInput{},
		Gotify: config.GotifyConfigInput{
			URL: "https://gotify.new.example.com/",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if configuration.SMTPHost != "smtp.qq.com" || configuration.SMTPPassword != "old-smtp-secret" {
		t.Fatalf("smtp config = %#v", configuration)
	}
	if configuration.IYUUToken != "old-iyuu-secret" {
		t.Fatalf("iyuu token was not preserved")
	}
	if configuration.GotifyURL != "https://gotify.new.example.com" || configuration.GotifyToken != "old-gotify-secret" {
		t.Fatalf("gotify config = %#v", configuration)
	}
}
