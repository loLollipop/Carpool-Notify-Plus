package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"carpool-notify/internal/config"

	"github.com/pelletier/go-toml/v2"
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

func TestLoadTrustedProxies(t *testing.T) {
	for _, test := range []struct {
		name    string
		setting string
		env     *string
		want    []string
		invalid bool
	}{
		{name: "loopback default", want: []string{"127.0.0.1", "::1"}},
		{name: "explicit file disable", setting: "trusted_proxies = []", want: []string{}},
		{name: "IP and CIDR", setting: `trusted_proxies = ["192.0.2.10", "10.0.0.0/8", "2001:db8::/32", "::1"]`, want: []string{"192.0.2.10", "10.0.0.0/8", "2001:db8::/32", "::1"}},
		{name: "trim file entries", setting: `trusted_proxies = [" 192.0.2.1 "]`, want: []string{"192.0.2.1"}},
		{name: "env override", setting: `trusted_proxies = ["192.0.2.1"]`, env: stringPointer(" 10.0.0.0/8, ::1 "), want: []string{"10.0.0.0/8", "::1"}},
		{name: "empty env disables", setting: `trusted_proxies = ["192.0.2.1"]`, env: stringPointer(""), want: []string{}},
		{name: "whitespace env disables", env: stringPointer(" \t"), want: []string{}},
		{name: "invalid hostname", setting: `trusted_proxies = ["localhost"]`, invalid: true},
		{name: "invalid CIDR", setting: `trusted_proxies = ["10.0.0.0/33"]`, invalid: true},
		{name: "empty file entry", setting: `trusted_proxies = [""]`, invalid: true},
		{name: "empty env entry", env: stringPointer("127.0.0.1,"), invalid: true},
		{name: "IPv4 trust all", setting: `trusted_proxies = ["0.0.0.0/0"]`, invalid: true},
		{name: "IPv6 trust all", setting: `trusted_proxies = ["::/0"]`, invalid: true},
		{name: "noncanonical trust all", env: stringPointer("192.0.2.1/0"), invalid: true},
		{name: "env invalid hostname", env: stringPointer("localhost"), invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := trustedProxyTestConfig(t, test.setting)
			if test.env != nil {
				t.Setenv("CARPOOL_TRUSTED_PROXIES", *test.env)
			}
			configuration, err := config.Load()
			if test.invalid {
				if err == nil || !strings.Contains(err.Error(), "server.trusted_proxies") {
					t.Fatalf("expected trusted proxy validation error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("load %s: %v", filepath.Base(path), err)
			}
			if !reflect.DeepEqual(configuration.TrustedProxies, test.want) {
				t.Fatalf("trusted proxies = %v, want %v", configuration.TrustedProxies, test.want)
			}
		})
	}
}

func TestNotificationRewritePreservesTrustedProxies(t *testing.T) {
	for _, test := range []struct {
		name    string
		setting string
		present bool
		want    []string
	}{
		{name: "absent keeps default", want: []string{"127.0.0.1", "::1"}},
		{name: "explicit disable", setting: "trusted_proxies = []", present: true, want: []string{}},
		{name: "configured proxies", setting: `trusted_proxies = ["10.0.0.0/8", "::1"]`, present: true, want: []string{"10.0.0.0/8", "::1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := trustedProxyTestConfig(t, test.setting)
			updated, err := config.UpdateNotificationConfig(path, config.NotificationConfigInput{})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(updated.TrustedProxies, test.want) {
				t.Fatalf("rewritten proxies = %v, want %v", updated.TrustedProxies, test.want)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var persisted struct {
				Server struct {
					TrustedProxies *[]string `toml:"trusted_proxies"`
				} `toml:"server"`
			}
			if err := toml.Unmarshal(raw, &persisted); err != nil {
				t.Fatal(err)
			}
			if (persisted.Server.TrustedProxies != nil) != test.present {
				t.Fatal("rewrite changed whether trusted_proxies was explicitly configured")
			}
			if test.present && !reflect.DeepEqual(*persisted.Server.TrustedProxies, test.want) {
				t.Fatalf("persisted proxies = %v, want %v", *persisted.Server.TrustedProxies, test.want)
			}
		})
	}
}

func trustedProxyTestConfig(t *testing.T, setting string) string {
	t.Helper()
	for _, key := range []string{"CARPOOL_PASSWORD", "CARPOOL_SESSION_SECRET", "CARPOOL_TRUSTED_PROXIES", "CARPOOL_SESSION_COOKIE_SECURE", "SMTP_PORT"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "config.toml")
	content := "[server]\npassword = \"test-password\"\nsession_secret = \"test-only-session-secret-at-least-32-bytes\"\n" + setting + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	previousArgs := os.Args
	t.Cleanup(func() { os.Args = previousArgs })
	os.Args = []string{"carpool-notify", "-config", path}
	return path
}

func stringPointer(value string) *string { return &value }

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
