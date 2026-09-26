package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
session_secret = "test-only-session-secret-at-least-32-bytes"

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
		"CARPOOL_SESSION_COOKIE_SECURE",
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
	if !configuration.SessionCookieSecure {
		t.Fatal("session cookie should default to secure")
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
session_secret = "test-only-session-secret-at-least-32-bytes"
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	t.Cleanup(func() { os.Args = oldArgs })
	os.Args = []string{"carpool-notify", "-config", configPath}
	t.Setenv("CARPOOL_PASSWORD", "env-pass")
	t.Setenv("CARPOOL_SESSION_COOKIE_SECURE", "false")

	configuration, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Password != "env-pass" {
		t.Fatalf("expected env override, got %q", configuration.Password)
	}
	if configuration.SessionCookieSecure {
		t.Fatal("session cookie secure env override was ignored")
	}
}

func TestUpdateNotificationConfigKeepsBlankSecrets(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.toml")
	content := `
[server]
password = "toml-pass"
session_secret = "test-only-session-secret-at-least-32-bytes"

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
		"CARPOOL_SESSION_COOKIE_SECURE",
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

func TestValidateNotificationConfigRejectsOutOfRangeSMTPPort(t *testing.T) {
	for _, port := range []int{-1, 65536} {
		err := config.ValidateNotificationConfig(config.NotificationConfigInput{
			SMTP: config.SMTPConfigInput{Port: port},
		})
		if err == nil {
			t.Fatalf("out-of-range SMTP port %d unexpectedly passed validation", port)
		}
	}
}

func TestUpdateNotificationConfigRollbackRestoresExactFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.toml")
	original := []byte(`[server]
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
	if err := os.WriteFile(configPath, original, 0o640); err != nil {
		t.Fatal(err)
	}

	updated, rollback, err := config.UpdateNotificationConfigWithRollback(
		configPath,
		config.NotificationConfigInput{SMTP: config.SMTPConfigInput{
			Host:     "smtp.new.example",
			Port:     465,
			Username: "new-user",
			Password: "new-secret",
			From:     "new@example.com",
			To:       "new-operator@example.com",
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ConfigPath != configPath || updated.SMTPHost != "smtp.new.example" {
		t.Fatalf("updated config = %#v", updated)
	}
	if err := rollback(); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(original) {
		t.Fatalf("restored config differs:\n%s", restored)
	}
}

func TestValidateNotificationConfigRejectsInvalidSMTPAddresses(t *testing.T) {
	for name, input := range map[string]config.NotificationConfigInput{
		"from": {
			SMTP: config.SMTPConfigInput{From: "sender@example.com\r\nBcc: hidden@example.com"},
		},
		"recipient": {
			SMTP: config.SMTPConfigInput{To: "customer@example.com, broken address"},
		},
		"host": {
			SMTP: config.SMTPConfigInput{Host: "smtp.example.com\ninvalid"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := config.ValidateNotificationConfig(input); err == nil {
				t.Fatal("invalid SMTP configuration was accepted")
			}
		})
	}
}

func TestLoadRejectsWeakCredentials(t *testing.T) {
	const secret = "test-only-session-secret-at-least-32-bytes"
	for _, test := range []struct {
		name, password, session, field string
	}{
		{"placeholder password", "change-me", secret, "server.password"},
		{"long placeholder password", "change-me-to-a-long-random-string", secret, "server.password"},
		{"case and whitespace", " CHANGE-ME ", secret, "server.password"},
		{"short password", "1234567", secret, "server.password"},
		{"placeholder secret", "valid-password", "change-me", "server.session_secret"},
		{"long placeholder secret", "valid-password", "change-me-to-a-long-random-string", "server.session_secret"},
		{"short secret", "valid-password", strings.Repeat("x", 31), "server.session_secret"},
		{"compatible minimum", "12345678", strings.Repeat("x", 32), ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, fromEnv := range []bool{false, true} {
				configPath := filepath.Join(t.TempDir(), "credentials.toml")
				password, session := test.password, test.session
				if fromEnv {
					password, session = "valid-password", secret
					t.Setenv("CARPOOL_PASSWORD", test.password)
					t.Setenv("CARPOOL_SESSION_SECRET", test.session)
				} else {
					t.Setenv("CARPOOL_PASSWORD", "")
					t.Setenv("CARPOOL_SESSION_SECRET", "")
				}
				t.Setenv("SMTP_PORT", "")
				t.Setenv("CARPOOL_SESSION_COOKIE_SECURE", "")
				content := fmt.Sprintf("[server]\npassword = %q\nsession_secret = %q\n", password, session)
				if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				oldArgs := os.Args
				os.Args = []string{"carpool-notify", "-config", configPath}
				_, err := config.Load()
				os.Args = oldArgs
				if test.field == "" {
					if err != nil {
						t.Fatalf("env=%v: valid credentials rejected: %v", fromEnv, err)
					}
				} else if err == nil || !strings.Contains(err.Error(), test.field) {
					t.Fatalf("env=%v: error = %v, want %s validation", fromEnv, err, test.field)
				}
			}
		})
	}
}
