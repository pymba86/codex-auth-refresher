package config

import (
	"strings"
	"testing"
	"time"
)

func TestParseReadsRefreshMaxAgeFromEnvAndFlag(t *testing.T) {
	t.Parallel()
	cfg, err := Parse([]string{"--refresh-max-age=12h"}, []string{
		"CODEX_AUTH_DIR=/tmp/auth",
		"CODEX_REFRESH_MAX_AGE=24h",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.RefreshMaxAge != 12*time.Hour {
		t.Fatalf("RefreshMaxAge = %v, want 12h", cfg.RefreshMaxAge)
	}
}

func TestParseReadsWebEnableFromEnvAndFlag(t *testing.T) {
	t.Parallel()
	cfg, err := Parse([]string{"--web-enable=false"}, []string{
		"CODEX_AUTH_DIR=/tmp/auth",
		"CODEX_WEB_ENABLE=true",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.WebEnable {
		t.Fatal("WebEnable = true, want false after flag override")
	}
}

func TestValidateAllowsDisabledRefreshMaxAge(t *testing.T) {
	t.Parallel()
	cfg := Config{
		AuthDir:       "/tmp/auth",
		ListenAddr:    ":8080",
		RefreshBefore: 6 * time.Hour,
		RefreshMaxAge: 0,
		ScanInterval:  time.Minute,
		MaxParallel:   1,
		HTTPTimeout:   15 * time.Second,
		TokenEndpoint: "https://auth.openai.com/oauth/token",
		LogFormat:     "json",
		StatusEnable:  true,
		WebEnable:     false,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsNegativeRefreshMaxAge(t *testing.T) {
	t.Parallel()
	cfg := Config{
		AuthDir:       "/tmp/auth",
		ListenAddr:    ":8080",
		RefreshBefore: 6 * time.Hour,
		RefreshMaxAge: -time.Hour,
		ScanInterval:  time.Minute,
		MaxParallel:   1,
		HTTPTimeout:   15 * time.Second,
		TokenEndpoint: "https://auth.openai.com/oauth/token",
		LogFormat:     "json",
		StatusEnable:  true,
		WebEnable:     false,
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "refresh-max-age") {
		t.Fatalf("Validate() error = %v, want refresh-max-age validation failure", err)
	}
}
