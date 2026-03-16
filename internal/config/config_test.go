package config

import (
	"reflect"
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

func TestParseReadsAntigravityEndpointDefaultAndAllowsOverrides(t *testing.T) {
	t.Parallel()
	cfg, err := Parse([]string{"--antigravity-client-id=override-client"}, []string{
		"CODEX_AUTH_DIR=/tmp/auth",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.AntigravityTokenEndpoint != "https://oauth2.googleapis.com/token" {
		t.Fatalf("AntigravityTokenEndpoint = %q, want Google token endpoint", cfg.AntigravityTokenEndpoint)
	}
	if cfg.AntigravityClientID != "override-client" {
		t.Fatalf("AntigravityClientID = %q, want override-client", cfg.AntigravityClientID)
	}
	if cfg.AntigravityClientSecret != "" {
		t.Fatalf("AntigravityClientSecret = %q, want empty by default", cfg.AntigravityClientSecret)
	}
}

func TestParseDisablesAuthTypesByDefault(t *testing.T) {
	t.Parallel()
	cfg, err := Parse(nil, []string{
		"CODEX_AUTH_DIR=/tmp/auth",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.AuthEnableCodex {
		t.Fatal("AuthEnableCodex = true, want false by default")
	}
	if cfg.AuthEnableGemini {
		t.Fatal("AuthEnableGemini = true, want false by default")
	}
	if cfg.AuthEnableAntigravity {
		t.Fatal("AuthEnableAntigravity = true, want false by default")
	}
	wantProviders := map[string]bool{
		"codex":       false,
		"gemini":      false,
		"antigravity": false,
	}
	if got := cfg.EnabledProviders(); !reflect.DeepEqual(got, wantProviders) {
		t.Fatalf("EnabledProviders() = %#v, want %#v", got, wantProviders)
	}
}

func TestParseReadsAuthTypeEnableFlagsFromEnvAndFlag(t *testing.T) {
	t.Parallel()
	cfg, err := Parse([]string{"--auth-enable-gemini=true"}, []string{
		"CODEX_AUTH_DIR=/tmp/auth",
		"CODEX_AUTH_ENABLE_CODEX=true",
		"CODEX_AUTH_ENABLE_GEMINI=false",
		"CODEX_AUTH_ENABLE_ANTIGRAVITY=false",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !cfg.AuthEnableCodex {
		t.Fatal("AuthEnableCodex = false, want true")
	}
	if !cfg.AuthEnableGemini {
		t.Fatal("AuthEnableGemini = false, want true after flag override")
	}
	if cfg.AuthEnableAntigravity {
		t.Fatal("AuthEnableAntigravity = true, want false")
	}
	wantProviders := map[string]bool{
		"codex":       true,
		"gemini":      true,
		"antigravity": false,
	}
	if got := cfg.EnabledProviders(); !reflect.DeepEqual(got, wantProviders) {
		t.Fatalf("EnabledProviders() = %#v, want %#v", got, wantProviders)
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

func TestParseReadsEmailSettingsFromEnvAndFlag(t *testing.T) {
	t.Parallel()
	cfg, err := Parse([]string{
		"--email-enable=true",
		"--email-to=override@example.com, second@example.com",
		"--email-smtp-port=2525",
	}, []string{
		"CODEX_AUTH_DIR=/tmp/auth",
		"CODEX_EMAIL_ENABLE=false",
		"CODEX_EMAIL_SMTP_HOST=smtp.example.com",
		"CODEX_EMAIL_SMTP_PORT=587",
		"CODEX_EMAIL_SMTP_TLS_MODE=starttls",
		"CODEX_EMAIL_FROM=alerts@example.com",
		"CODEX_EMAIL_TO=env@example.com",
		"CODEX_EMAIL_TIMEOUT=20s",
	})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if !cfg.EmailEnable {
		t.Fatal("EmailEnable = false, want true after flag override")
	}
	if cfg.EmailSMTPPort != 2525 {
		t.Fatalf("EmailSMTPPort = %d, want 2525", cfg.EmailSMTPPort)
	}
	wantRecipients := []string{"override@example.com", "second@example.com"}
	if !reflect.DeepEqual(cfg.EmailTo, wantRecipients) {
		t.Fatalf("EmailTo = %#v, want %#v", cfg.EmailTo, wantRecipients)
	}
}

func TestValidateAllowsEmailConfigWhenComplete(t *testing.T) {
	t.Parallel()
	cfg := Config{
		AuthDir:          "/tmp/auth",
		ListenAddr:       ":8080",
		RefreshBefore:    6 * time.Hour,
		RefreshMaxAge:    0,
		ScanInterval:     time.Minute,
		MaxParallel:      1,
		HTTPTimeout:      15 * time.Second,
		TokenEndpoint:    "https://auth.openai.com/oauth/token",
		LogFormat:        "json",
		StatusEnable:     true,
		WebEnable:        false,
		EmailEnable:      true,
		EmailSMTPHost:    "smtp.example.com",
		EmailSMTPPort:    587,
		EmailSMTPTLSMode: "starttls",
		EmailFrom:        "alerts@example.com",
		EmailTo:          []string{"ops@example.com"},
		EmailTimeout:     15 * time.Second,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsIncompleteEmailAuth(t *testing.T) {
	t.Parallel()
	cfg := Config{
		AuthDir:           "/tmp/auth",
		ListenAddr:        ":8080",
		RefreshBefore:     6 * time.Hour,
		ScanInterval:      time.Minute,
		MaxParallel:       1,
		HTTPTimeout:       15 * time.Second,
		TokenEndpoint:     "https://auth.openai.com/oauth/token",
		LogFormat:         "json",
		StatusEnable:      true,
		EmailEnable:       true,
		EmailSMTPHost:     "smtp.example.com",
		EmailSMTPPort:     587,
		EmailSMTPTLSMode:  "starttls",
		EmailSMTPUsername: "user",
		EmailFrom:         "alerts@example.com",
		EmailTo:           []string{"ops@example.com"},
		EmailTimeout:      15 * time.Second,
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "email-smtp-username") {
		t.Fatalf("Validate() error = %v, want email auth validation failure", err)
	}
}

func TestValidateRejectsMissingEmailRecipientWhenEnabled(t *testing.T) {
	t.Parallel()
	cfg := Config{
		AuthDir:          "/tmp/auth",
		ListenAddr:       ":8080",
		RefreshBefore:    6 * time.Hour,
		ScanInterval:     time.Minute,
		MaxParallel:      1,
		HTTPTimeout:      15 * time.Second,
		TokenEndpoint:    "https://auth.openai.com/oauth/token",
		LogFormat:        "json",
		StatusEnable:     true,
		EmailEnable:      true,
		EmailSMTPHost:    "smtp.example.com",
		EmailSMTPPort:    587,
		EmailSMTPTLSMode: "starttls",
		EmailFrom:        "alerts@example.com",
		EmailTimeout:     15 * time.Second,
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "email-to") {
		t.Fatalf("Validate() error = %v, want email-to validation failure", err)
	}
}
