package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AuthDir       string
	ListenAddr    string
	RefreshBefore time.Duration
	RefreshMaxAge time.Duration
	ScanInterval  time.Duration
	MaxParallel   int
	HTTPTimeout   time.Duration
	TokenEndpoint string
	ClientID      string
	CAFile        string
	LogFormat     string
	StatusEnable  bool
	WebEnable     bool
}

func Parse(args []string, env []string) (Config, error) {
	envMap := make(map[string]string, len(env))
	for _, item := range env {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	cfg := Config{
		AuthDir:       envMap["CODEX_AUTH_DIR"],
		ListenAddr:    getOrDefault(envMap["CODEX_LISTEN_ADDR"], ":8080"),
		RefreshBefore: getDuration(envMap["CODEX_REFRESH_BEFORE"], 6*time.Hour),
		RefreshMaxAge: getDuration(envMap["CODEX_REFRESH_MAX_AGE"], 0),
		ScanInterval:  getDuration(envMap["CODEX_SCAN_INTERVAL"], time.Minute),
		MaxParallel:   getInt(envMap["CODEX_MAX_PARALLEL"], 4),
		HTTPTimeout:   getDuration(envMap["CODEX_HTTP_TIMEOUT"], 15*time.Second),
		TokenEndpoint: getOrDefault(envMap["CODEX_TOKEN_ENDPOINT"], "https://auth.openai.com/oauth/token"),
		ClientID:      envMap["CODEX_CLIENT_ID"],
		CAFile:        envMap["CODEX_CA_FILE"],
		LogFormat:     getOrDefault(envMap["CODEX_LOG_FORMAT"], "json"),
		StatusEnable:  getBool(envMap["CODEX_STATUS_ENABLE"], true),
		WebEnable:     getBool(envMap["CODEX_WEB_ENABLE"], false),
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
	fs.StringVar(&cfg.CAFile, "ca-file", cfg.CAFile, "custom CA PEM file")
	fs.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "log format: json or text")
	fs.BoolVar(&cfg.StatusEnable, "status-enable", cfg.StatusEnable, "enable GET /v1/status")
	fs.BoolVar(&cfg.WebEnable, "web-enable", cfg.WebEnable, "enable the web dashboard at GET /")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
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
