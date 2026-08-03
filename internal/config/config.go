package config

import (
	"flag"
	"fmt"
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
