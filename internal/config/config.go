package config

import (
	"errors"
	"flag"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultCodexTokenEndpoint       = "https://auth.openai.com/oauth/token"
	defaultAntigravityTokenEndpoint = "https://oauth2.googleapis.com/token"
)

type Config struct {
	AuthDir                  string
	ListenAddr               string
	RefreshBefore            time.Duration
	RefreshMaxAge            time.Duration
	ScanInterval             time.Duration
	MaxParallel              int
	HTTPTimeout              time.Duration
	TokenEndpoint            string
	ClientID                 string
	AntigravityTokenEndpoint string
	AntigravityClientID      string
	AntigravityClientSecret  string
	CAFile                   string
	LogFormat                string
	StatusEnable             bool
	WebEnable                bool
	EmailEnable              bool

	EmailSMTPHost     string
	EmailSMTPPort     int
	EmailSMTPTLSMode  string
	EmailSMTPUsername string
	EmailSMTPPassword string
	EmailFrom         string
	EmailTo           []string
	EmailTimeout      time.Duration
}

func Parse(args []string, env []string) (Config, error) {
	envMap := make(map[string]string, len(env))
	for _, item := range env {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	emailToRaw := envMap["CODEX_EMAIL_TO"]
	cfg := Config{
		AuthDir:       envMap["CODEX_AUTH_DIR"],
		ListenAddr:    getOrDefault(envMap["CODEX_LISTEN_ADDR"], ":8080"),
		RefreshBefore: getDuration(envMap["CODEX_REFRESH_BEFORE"], 6*time.Hour),
		RefreshMaxAge: getDuration(envMap["CODEX_REFRESH_MAX_AGE"], 0),
		ScanInterval:  getDuration(envMap["CODEX_SCAN_INTERVAL"], time.Minute),
		MaxParallel:   getInt(envMap["CODEX_MAX_PARALLEL"], 4),
		HTTPTimeout:   getDuration(envMap["CODEX_HTTP_TIMEOUT"], 15*time.Second),
		TokenEndpoint: getOrDefault(envMap["CODEX_TOKEN_ENDPOINT"], defaultCodexTokenEndpoint),
		ClientID:      envMap["CODEX_CLIENT_ID"],
		AntigravityTokenEndpoint: getOrDefault(
			envMap["CODEX_ANTIGRAVITY_TOKEN_ENDPOINT"],
			defaultAntigravityTokenEndpoint,
		),
		AntigravityClientID:     envMap["CODEX_ANTIGRAVITY_CLIENT_ID"],
		AntigravityClientSecret: envMap["CODEX_ANTIGRAVITY_CLIENT_SECRET"],
		CAFile:                  envMap["CODEX_CA_FILE"],
		LogFormat:               getOrDefault(envMap["CODEX_LOG_FORMAT"], "json"),
		StatusEnable:            getBool(envMap["CODEX_STATUS_ENABLE"], true),
		WebEnable:               getBool(envMap["CODEX_WEB_ENABLE"], false),
		EmailEnable:             getBool(envMap["CODEX_EMAIL_ENABLE"], false),
		EmailSMTPHost:           envMap["CODEX_EMAIL_SMTP_HOST"],
		EmailSMTPPort:           getInt(envMap["CODEX_EMAIL_SMTP_PORT"], 587),
		EmailSMTPTLSMode:        getOrDefault(envMap["CODEX_EMAIL_SMTP_TLS_MODE"], "starttls"),
		EmailSMTPUsername:       envMap["CODEX_EMAIL_SMTP_USERNAME"],
		EmailSMTPPassword:       envMap["CODEX_EMAIL_SMTP_PASSWORD"],
		EmailFrom:               envMap["CODEX_EMAIL_FROM"],
		EmailTo:                 splitAndTrimCSV(emailToRaw),
		EmailTimeout:            getDuration(envMap["CODEX_EMAIL_TIMEOUT"], 15*time.Second),
	}

	fs := flag.NewFlagSet("codex-auth-refresher", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfg.AuthDir, "auth-dir", cfg.AuthDir, "path to auth directory")
	fs.StringVar(&cfg.ListenAddr, "listen-addr", cfg.ListenAddr, "HTTP listen address")
	fs.DurationVar(&cfg.RefreshBefore, "refresh-before", cfg.RefreshBefore, "refresh threshold before token expiry")
	fs.DurationVar(&cfg.RefreshMaxAge, "refresh-max-age", cfg.RefreshMaxAge, "force refresh when last successful refresh reaches this age; 0 disables the mode")
	fs.DurationVar(&cfg.ScanInterval, "scan-interval", cfg.ScanInterval, "auth directory scan interval")
	fs.IntVar(&cfg.MaxParallel, "max-parallel", cfg.MaxParallel, "maximum concurrent refresh operations")
	fs.DurationVar(&cfg.HTTPTimeout, "http-timeout", cfg.HTTPTimeout, "HTTP client timeout")
	fs.StringVar(&cfg.TokenEndpoint, "token-endpoint", cfg.TokenEndpoint, "OAuth token refresh endpoint")
	fs.StringVar(&cfg.ClientID, "client-id", cfg.ClientID, "fallback OAuth client id")
	fs.StringVar(&cfg.AntigravityTokenEndpoint, "antigravity-token-endpoint", cfg.AntigravityTokenEndpoint, "override Antigravity OAuth token endpoint")
	fs.StringVar(&cfg.AntigravityClientID, "antigravity-client-id", cfg.AntigravityClientID, "override Antigravity OAuth client id")
	fs.StringVar(&cfg.AntigravityClientSecret, "antigravity-client-secret", cfg.AntigravityClientSecret, "override Antigravity OAuth client secret")
	fs.StringVar(&cfg.CAFile, "ca-file", cfg.CAFile, "custom CA PEM file")
	fs.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "log format: json or text")
	fs.BoolVar(&cfg.StatusEnable, "status-enable", cfg.StatusEnable, "enable GET /v1/status")
	fs.BoolVar(&cfg.WebEnable, "web-enable", cfg.WebEnable, "enable the web dashboard at GET /")
	fs.BoolVar(&cfg.EmailEnable, "email-enable", cfg.EmailEnable, "enable SMTP email alerts for auth problems")
	fs.StringVar(&cfg.EmailSMTPHost, "email-smtp-host", cfg.EmailSMTPHost, "SMTP host for email alerts")
	fs.IntVar(&cfg.EmailSMTPPort, "email-smtp-port", cfg.EmailSMTPPort, "SMTP port for email alerts")
	fs.StringVar(&cfg.EmailSMTPTLSMode, "email-smtp-tls-mode", cfg.EmailSMTPTLSMode, "SMTP TLS mode: starttls, implicit, or none")
	fs.StringVar(&cfg.EmailSMTPUsername, "email-smtp-username", cfg.EmailSMTPUsername, "SMTP username for email alerts")
	fs.StringVar(&cfg.EmailSMTPPassword, "email-smtp-password", cfg.EmailSMTPPassword, "SMTP password for email alerts")
	fs.StringVar(&cfg.EmailFrom, "email-from", cfg.EmailFrom, "From address for email alerts")
	fs.StringVar(&emailToRaw, "email-to", emailToRaw, "comma-separated recipient list for email alerts")
	fs.DurationVar(&cfg.EmailTimeout, "email-timeout", cfg.EmailTimeout, "SMTP timeout for email alerts")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	cfg.EmailTo = splitAndTrimCSV(emailToRaw)
	cfg.EmailSMTPTLSMode = strings.ToLower(strings.TrimSpace(cfg.EmailSMTPTLSMode))
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.AuthDir == "" {
		return errors.New("auth directory is required via CODEX_AUTH_DIR or --auth-dir")
	}
	if c.RefreshBefore <= 0 {
		return errors.New("refresh-before must be positive")
	}
	if c.RefreshMaxAge < 0 {
		return errors.New("refresh-max-age must be zero or positive")
	}
	if c.ScanInterval <= 0 {
		return errors.New("scan-interval must be positive")
	}
	if c.MaxParallel <= 0 {
		return errors.New("max-parallel must be positive")
	}
	if c.HTTPTimeout <= 0 {
		return errors.New("http-timeout must be positive")
	}
	if c.TokenEndpoint == "" {
		return errors.New("token-endpoint must be set")
	}
	if c.LogFormat != "json" && c.LogFormat != "text" {
		return fmt.Errorf("unsupported log format %q", c.LogFormat)
	}
	if c.CAFile != "" {
		if !filepath.IsAbs(c.CAFile) {
			abs, err := filepath.Abs(c.CAFile)
			if err != nil {
				return fmt.Errorf("resolve ca-file: %w", err)
			}
			c.CAFile = abs
		}
	}
	if !c.EmailEnable {
		return nil
	}
	if strings.TrimSpace(c.EmailSMTPHost) == "" {
		return errors.New("email-smtp-host is required when email alerts are enabled")
	}
	if c.EmailSMTPPort <= 0 || c.EmailSMTPPort > 65535 {
		return errors.New("email-smtp-port must be between 1 and 65535")
	}
	switch strings.ToLower(strings.TrimSpace(c.EmailSMTPTLSMode)) {
	case "starttls", "implicit", "none":
	default:
		return fmt.Errorf("unsupported email-smtp-tls-mode %q", c.EmailSMTPTLSMode)
	}
	usernameSet := strings.TrimSpace(c.EmailSMTPUsername) != ""
	passwordSet := strings.TrimSpace(c.EmailSMTPPassword) != ""
	if usernameSet != passwordSet {
		return errors.New("email-smtp-username and email-smtp-password must be set together")
	}
	if strings.TrimSpace(c.EmailFrom) == "" {
		return errors.New("email-from is required when email alerts are enabled")
	}
	if _, err := mail.ParseAddress(strings.TrimSpace(c.EmailFrom)); err != nil {
		return fmt.Errorf("invalid email-from address: %w", err)
	}
	if len(c.EmailTo) == 0 {
		return errors.New("email-to must contain at least one recipient when email alerts are enabled")
	}
	for _, address := range c.EmailTo {
		if _, err := mail.ParseAddress(address); err != nil {
			return fmt.Errorf("invalid email-to address %q: %w", address, err)
		}
	}
	if c.EmailTimeout <= 0 {
		return errors.New("email-timeout must be positive")
	}
	return nil
}

func getOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func getDuration(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}

func getInt(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func getBool(raw string, fallback bool) bool {
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func splitAndTrimCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}
