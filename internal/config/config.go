package config

import (
	"flag"
	"fmt"
	"net/mail"
	"os"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config holds process configuration.
type Config struct {
	Password      string
	SessionSecret string
	ListenAddress string
	DatabasePath  string
	GotifyURL     string
	GotifyToken   string
	IYUUToken     string
	SMTPHost      string
	SMTPPort      int
	SMTPUsername  string
	SMTPPassword  string
	SMTPFrom      string
	SMTPTo        string
	ConfigPath    string
}

// SMTPConfigView is the safe-to-display SMTP configuration shape.
type SMTPConfigView struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Username    string `json:"username"`
	From        string `json:"from"`
	To          string `json:"to"`
	PasswordSet bool   `json:"password_set"`
}

// SecretConfigView reports whether a write-only token already exists.
type SecretConfigView struct {
	TokenSet bool `json:"token_set"`
}

// GotifyConfigView is the safe-to-display Gotify configuration shape.
type GotifyConfigView struct {
	URL      string `json:"url"`
	TokenSet bool   `json:"token_set"`
}

// NotificationConfigView is returned by the settings API without secret values.
type NotificationConfigView struct {
	SMTP   SMTPConfigView   `json:"smtp"`
	IYUU   SecretConfigView `json:"iyuu"`
	Gotify GotifyConfigView `json:"gotify"`
}

// SMTPConfigInput is accepted by the settings API. Empty Password keeps the old secret.
type SMTPConfigInput struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// SecretConfigInput updates a write-only token. Empty Token keeps the old secret.
type SecretConfigInput struct {
	Token string `json:"token"`
}

// GotifyConfigInput updates Gotify. Empty Token keeps the old secret.
type GotifyConfigInput struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

// NotificationConfigInput is accepted by the settings API.
type NotificationConfigInput struct {
	SMTP   SMTPConfigInput   `json:"smtp"`
	IYUU   SecretConfigInput `json:"iyuu"`
	Gotify GotifyConfigInput `json:"gotify"`
}

// fileConfig is the on-disk TOML shape.
type fileConfig struct {
	Server struct {
		Listen        string `toml:"listen"`
		DBPath        string `toml:"db_path"`
		Password      string `toml:"password"`
		SessionSecret string `toml:"session_secret"`
	} `toml:"server"`
	Gotify struct {
		URL   string `toml:"url"`
		Token string `toml:"token"`
	} `toml:"gotify"`
	IYUU struct {
		Token string `toml:"token"`
	} `toml:"iyuu"`
	SMTP struct {
		Host     string `toml:"host"`
		Port     int    `toml:"port"`
		Username string `toml:"username"`
		Password string `toml:"password"`
		From     string `toml:"from"`
		To       string `toml:"to"`
	} `toml:"smtp"`
}

// Load reads configuration from TOML (primary) and optional environment overrides.
//
// Resolution order for each field:
//  1. non-empty environment variable (if set)
//  2. value from config.toml
//  3. built-in default (where applicable)
//
// Config file path resolution:
//  1. -config flag
//  2. CARPOOL_CONFIG environment variable
//  3. ./config.toml
func Load() (Config, error) {
	configPath := resolveConfigPath()
	return loadConfig(configPath)
}

func loadConfig(configPath string) (Config, error) {

	fileValues, err := loadTOMLFile(configPath)
	if err != nil {
		return Config{}, err
	}

	smtpPort := fileValues.SMTP.Port
	if smtpPort == 0 {
		smtpPort = 587
	}
	if rawPort := strings.TrimSpace(os.Getenv("SMTP_PORT")); rawPort != "" {
		parsed, err := strconv.Atoi(rawPort)
		if err != nil {
			return Config{}, fmt.Errorf("SMTP_PORT: %w", err)
		}
		smtpPort = parsed
	}

	configuration := Config{
		ConfigPath:    configPath,
		Password:      firstNonEmpty(os.Getenv("CARPOOL_PASSWORD"), fileValues.Server.Password),
		SessionSecret: firstNonEmpty(os.Getenv("CARPOOL_SESSION_SECRET"), fileValues.Server.SessionSecret),
		ListenAddress: firstNonEmpty(os.Getenv("CARPOOL_LISTEN"), fileValues.Server.Listen, ":8080"),
		DatabasePath:  firstNonEmpty(os.Getenv("CARPOOL_DB_PATH"), fileValues.Server.DBPath, "./data/carpool.db"),
		GotifyURL:     strings.TrimRight(firstNonEmpty(os.Getenv("GOTIFY_URL"), fileValues.Gotify.URL), "/"),
		GotifyToken:   firstNonEmpty(os.Getenv("GOTIFY_TOKEN"), fileValues.Gotify.Token),
		IYUUToken:     firstNonEmpty(os.Getenv("IYUU_TOKEN"), fileValues.IYUU.Token),
		SMTPHost:      firstNonEmpty(os.Getenv("SMTP_HOST"), fileValues.SMTP.Host),
		SMTPPort:      smtpPort,
		SMTPUsername:  firstNonEmpty(os.Getenv("SMTP_USERNAME"), fileValues.SMTP.Username),
		SMTPPassword:  firstNonEmpty(os.Getenv("SMTP_PASSWORD"), fileValues.SMTP.Password),
		SMTPFrom:      firstNonEmpty(os.Getenv("SMTP_FROM"), fileValues.SMTP.From),
		SMTPTo:        firstNonEmpty(os.Getenv("SMTP_TO"), fileValues.SMTP.To),
	}

	if configuration.Password == "" {
		return Config{}, fmt.Errorf("server.password is required (config.toml or CARPOOL_PASSWORD)")
	}
	if configuration.SessionSecret == "" {
		return Config{}, fmt.Errorf("server.session_secret is required (config.toml or CARPOOL_SESSION_SECRET)")
	}

	return configuration, nil
}

// GotifyConfigured reports whether Gotify credentials are present.
func (configuration Config) GotifyConfigured() bool {
	return configuration.GotifyURL != "" && configuration.GotifyToken != ""
}

// IYUUConfigured reports whether the IYUU token is present.
func (configuration Config) IYUUConfigured() bool {
	return configuration.IYUUToken != ""
}

// SMTPConfigured reports whether SMTP can send operator and customer mail.
func (configuration Config) SMTPConfigured() bool {
	return configuration.SMTPHost != "" &&
		configuration.SMTPPort > 0 &&
		configuration.SMTPFrom != "" &&
		configuration.SMTPUsername != "" &&
		configuration.SMTPPassword != ""
}

// SMTPOperatorConfigured reports whether operator reminder recipients are set.
func (configuration Config) SMTPOperatorConfigured() bool {
	return configuration.SMTPConfigured() && strings.TrimSpace(configuration.SMTPTo) != ""
}

// NotificationConfig returns the safe, editable notification configuration.
func (configuration Config) NotificationConfig() NotificationConfigView {
	return NotificationConfigView{
		SMTP: SMTPConfigView{
			Host:        configuration.SMTPHost,
			Port:        configuration.SMTPPort,
			Username:    configuration.SMTPUsername,
			From:        configuration.SMTPFrom,
			To:          configuration.SMTPTo,
			PasswordSet: strings.TrimSpace(configuration.SMTPPassword) != "",
		},
		IYUU: SecretConfigView{
			TokenSet: strings.TrimSpace(configuration.IYUUToken) != "",
		},
		Gotify: GotifyConfigView{
			URL:      configuration.GotifyURL,
			TokenSet: strings.TrimSpace(configuration.GotifyToken) != "",
		},
	}
}

// UpdateNotificationConfig writes notification settings to config.toml and reloads
// the effective runtime config. Secret fields are write-only: empty input keeps
// the existing value on disk.
func UpdateNotificationConfig(path string, input NotificationConfigInput) (Config, error) {
	updated, _, err := UpdateNotificationConfigWithRollback(path, input)
	return updated, err
}

// UpdateNotificationConfigWithRollback writes notification settings and
// returns a compensating action for callers that also persist settings in
// another data store. The rollback restores the exact previous file contents.
func UpdateNotificationConfigWithRollback(
	path string,
	input NotificationConfigInput,
) (Config, func() error, error) {
	previousRaw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, nil, fmt.Errorf("read config %s: %w", path, err)
	}
	previousMode := os.FileMode(0o600)
	if info, statErr := os.Stat(path); statErr == nil {
		previousMode = info.Mode().Perm()
	}
	fileValues, err := loadTOMLFile(path)
	if err != nil {
		return Config{}, nil, err
	}
	if err := ValidateNotificationConfig(input); err != nil {
		return Config{}, nil, err
	}

	smtpPort := input.SMTP.Port
	if smtpPort <= 0 {
		smtpPort = 587
	}
	fileValues.SMTP.Host = strings.TrimSpace(input.SMTP.Host)
	fileValues.SMTP.Port = smtpPort
	fileValues.SMTP.Username = strings.TrimSpace(input.SMTP.Username)
	if strings.TrimSpace(input.SMTP.Password) != "" {
		fileValues.SMTP.Password = strings.TrimSpace(input.SMTP.Password)
	}
	fileValues.SMTP.From = strings.TrimSpace(input.SMTP.From)
	fileValues.SMTP.To = strings.TrimSpace(input.SMTP.To)

	if strings.TrimSpace(input.IYUU.Token) != "" {
		fileValues.IYUU.Token = strings.TrimSpace(input.IYUU.Token)
	}

	fileValues.Gotify.URL = strings.TrimRight(strings.TrimSpace(input.Gotify.URL), "/")
	if strings.TrimSpace(input.Gotify.Token) != "" {
		fileValues.Gotify.Token = strings.TrimSpace(input.Gotify.Token)
	}

	if err := writeTOMLFile(path, fileValues); err != nil {
		return Config{}, nil, err
	}
	rollback := func() error {
		return writeFileAtomically(path, previousRaw, previousMode)
	}
	updated, err := loadConfig(path)
	if err != nil {
		if rollbackErr := rollback(); rollbackErr != nil {
			return Config{}, nil, fmt.Errorf("reload config: %v; restore previous config: %w", err, rollbackErr)
		}
		return Config{}, nil, err
	}
	return updated, rollback, nil
}

// ValidateNotificationConfig checks settings that would otherwise fail only
// after the database-backed settings on the same page had already been saved.
func ValidateNotificationConfig(input NotificationConfigInput) error {
	if input.SMTP.Port < 0 || input.SMTP.Port > 65535 {
		return fmt.Errorf("smtp port must be between 1 and 65535")
	}
	if strings.ContainsAny(input.SMTP.Host, "\r\n\t ") {
		return fmt.Errorf("smtp host must not contain whitespace")
	}
	if rawFrom := strings.TrimSpace(input.SMTP.From); rawFrom != "" {
		if _, err := mail.ParseAddress(rawFrom); err != nil {
			return fmt.Errorf("smtp from address is invalid")
		}
	}
	for _, rawRecipient := range strings.Split(input.SMTP.To, ",") {
		rawRecipient = strings.TrimSpace(rawRecipient)
		if rawRecipient == "" {
			continue
		}
		if _, err := mail.ParseAddress(rawRecipient); err != nil {
			return fmt.Errorf("smtp recipient address is invalid")
		}
	}
	return nil
}

func resolveConfigPath() string {
	flagSet := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flagSet.SetOutput(os.Stderr)
	configPathFlag := flagSet.String("config", "", "path to config.toml")
	// Ignore unknown flags so future flags can be added without breaking Load.
	_ = flagSet.Parse(os.Args[1:])

	if strings.TrimSpace(*configPathFlag) != "" {
		return strings.TrimSpace(*configPathFlag)
	}
	if fromEnv := strings.TrimSpace(os.Getenv("CARPOOL_CONFIG")); fromEnv != "" {
		return fromEnv
	}
	return "./config.toml"
}

func loadTOMLFile(path string) (fileConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileConfig{}, fmt.Errorf(
				"config file not found: %s (copy config.example.toml to config.toml, or pass -config)",
				path,
			)
		}
		return fileConfig{}, fmt.Errorf("read config %s: %w", path, err)
	}

	var parsed fileConfig
	if err := toml.Unmarshal(raw, &parsed); err != nil {
		return fileConfig{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return parsed, nil
}

func writeTOMLFile(path string, values fileConfig) error {
	raw, err := toml.Marshal(values)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return writeFileAtomically(path, raw, mode)
}

func writeFileAtomically(path string, raw []byte, mode os.FileMode) error {
	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, raw, mode); err != nil {
		return fmt.Errorf("write config temp: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
