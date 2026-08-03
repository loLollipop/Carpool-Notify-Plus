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
