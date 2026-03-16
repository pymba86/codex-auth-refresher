package authfile

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestParseFlatAndPreserveUnknownFields(t *testing.T) {
	t.Parallel()
	input := []byte(`{
  "access_token": "` + testJWT(time.Unix(2000, 0), "client-a") + `",
  "refresh_token": "rt-old",
  "id_token": "` + testJWT(time.Unix(2100, 0), "") + `",
  "expired": "2026-03-16T08:49:04Z",
  "last_refresh": "2026-03-06T08:49:04Z",
  "account_id": "account-1",
  "disabled": false,
  "email": "user@example.com",
  "type": "codex",
  "custom_field": "keep-me"
}`)
	doc, err := Parse("auth/user.json", input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := doc.SchemaName(); got != "flat" {
		t.Fatalf("schema = %q, want flat", got)
	}
	doc.SetTokens("new-access", "new-refresh", "new-id")
	doc.SetTimestamps(time.Unix(3000, 0), time.Unix(4000, 0), 1000)
	output, err := doc.MarshalPreservingUnknownFields()
	if err != nil {
		t.Fatalf("MarshalPreservingUnknownFields() error = %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := decoded["custom_field"]; got != "keep-me" {
		t.Fatalf("custom_field = %v, want keep-me", got)
	}
	if got := decoded["access_token"]; got != "new-access" {
		t.Fatalf("access_token = %v, want new-access", got)
	}
	if got := decoded["expires_in"]; got != float64(1000) {
		t.Fatalf("expires_in = %v, want 1000", got)
	}
}

func TestParseNested(t *testing.T) {
	t.Parallel()
	input := []byte(`{
  "tokens": {
    "access_token": "at",
    "refresh_token": "rt",
    "id_token": "it",
    "extra": "keep"
  },
  "last_refresh": "2026-03-06T08:49:04Z"
}`)
	doc, err := Parse("auth/nested.json", input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := doc.SchemaName(); got != "nested" {
		t.Fatalf("schema = %q, want nested", got)
	}
	doc.SetTokens("new-at", "new-rt", "new-it")
	doc.SetTimestamps(time.Unix(3000, 0), time.Unix(4000, 0), 1000)
	output, err := doc.MarshalPreservingUnknownFields()
	if err != nil {
		t.Fatalf("MarshalPreservingUnknownFields() error = %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	var tokens map[string]any
	if err := json.Unmarshal(decoded["tokens"], &tokens); err != nil {
		t.Fatalf("json.Unmarshal(tokens) error = %v", err)
	}
	if got := tokens["extra"]; got != "keep" {
		t.Fatalf("tokens.extra = %v, want keep", got)
	}
	if got := tokens["access_token"]; got != "new-at" {
		t.Fatalf("tokens.access_token = %v, want new-at", got)
	}
}

func TestParseGeminiTokenContainerPreservesKeyAndMetadata(t *testing.T) {
	t.Parallel()
	input := []byte(`{
  "type": "gemini",
  "timestamp": 123,
  "token": {
    "access_token": "at",
    "refresh_token": "rt",
    "client_id": "client-g",
    "client_secret": "secret-g",
    "expires_in": 3599,
    "expiry": "2026-03-16T14:05:18.621567873+05:00",
    "token_uri": "https://oauth2.googleapis.com/token",
    "extra": "keep"
  }
}`)
	doc, err := Parse("auth/gemini-user.json", input)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got := doc.Provider(); got != ProviderGemini {
		t.Fatalf("Provider() = %q, want %q", got, ProviderGemini)
	}
	if got := doc.ClientID(); got != "client-g" {
		t.Fatalf("ClientID() = %q, want client-g", got)
	}
	if got := doc.ClientSecret(); got != "secret-g" {
		t.Fatalf("ClientSecret() = %q, want secret-g", got)
	}
	if got := doc.TokenURI(); got != "https://oauth2.googleapis.com/token" {
		t.Fatalf("TokenURI() = %q, want Google token endpoint", got)
	}
	explicitExpiry, ok := doc.ExplicitExpiry()
	if !ok {
		t.Fatal("expected ExplicitExpiry() to resolve nested token.expiry")
	}
	doc.SetTokens("new-at", "new-rt", "")
	doc.SetTimestamps(time.Unix(3000, 0), explicitExpiry, 1800)
	output, err := doc.MarshalPreservingUnknownFields()
	if err != nil {
		t.Fatalf("MarshalPreservingUnknownFields() error = %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(output, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := decoded["token"]; !ok {
		t.Fatal("expected nested token container to remain named token")
	}
	if _, ok := decoded["tokens"]; ok {
		t.Fatal("did not expect tokens container to be created")
	}
	var token map[string]any
	if err := json.Unmarshal(decoded["token"], &token); err != nil {
		t.Fatalf("json.Unmarshal(token) error = %v", err)
	}
	if got := token["extra"]; got != "keep" {
		t.Fatalf("token.extra = %v, want keep", got)
	}
	if got := token["expires_in"]; got != float64(1800) {
		t.Fatalf("token.expires_in = %v, want 1800", got)
	}
	if got := token["access_token"]; got != "new-at" {
		t.Fatalf("token.access_token = %v, want new-at", got)
	}
	var timestamp int64
	if err := json.Unmarshal(decoded["timestamp"], &timestamp); err != nil {
		t.Fatalf("json.Unmarshal(timestamp) error = %v", err)
	}
	if timestamp != time.Unix(3000, 0).UTC().UnixMilli() {
		t.Fatalf("timestamp = %d, want %d", timestamp, time.Unix(3000, 0).UTC().UnixMilli())
	}
}

func TestIsTrackedFilename(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"codex-user.json":    true,
		"gemini-user.json":   true,
		"antigravity-a.json": true,
		"CODEX-user.JSON":    true,
		"/tmp/codex-a.json":  true,
		"user.json":          false,
		"claude-user.json":   false,
		"codex-user.txt":     false,
	}

	for input, want := range tests {
		if got := IsTrackedFilename(input); got != want {
			t.Fatalf("IsTrackedFilename(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestProviderFromFilename(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"codex-user.json":       ProviderCodex,
		"gemini-user.json":      ProviderGemini,
		"antigravity-user.json": ProviderAntigravity,
		"user.json":             "",
	}
	for input, want := range tests {
		if got := ProviderFromFilename(input); got != want {
			t.Fatalf("ProviderFromFilename(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestDocumentIsSupportedAuth(t *testing.T) {
	t.Parallel()

	doc, err := Parse("auth/codex-user.json", []byte(`{
  "access_token": "at",
  "refresh_token": "rt",
  "type": " Codex "
}`))
	if err != nil {
		t.Fatalf("Parse(codex) error = %v", err)
	}
	if !doc.IsSupportedAuth() {
		t.Fatal("expected codex document to be accepted")
	}

	gemini, err := Parse("auth/gemini-user.json", []byte(`{
  "type": "gemini",
  "token": {
    "access_token": "at",
    "refresh_token": "rt"
  }
}`))
	if err != nil {
		t.Fatalf("Parse(gemini) error = %v", err)
	}
	if !gemini.IsSupportedAuth() {
		t.Fatal("expected gemini document to be accepted")
	}
	if got := gemini.Provider(); got != ProviderGemini {
		t.Fatalf("gemini Provider() = %q, want %q", got, ProviderGemini)
	}

	other, err := Parse("auth/codex-other.json", []byte(`{
  "access_token": "at",
  "refresh_token": "rt",
  "type": "claude"
}`))
	if err != nil {
		t.Fatalf("Parse(other) error = %v", err)
	}
	if other.IsSupportedAuth() {
		t.Fatal("expected unsupported type to be rejected")
	}

	legacy, err := Parse("auth/codex-legacy.json", []byte(`{
  "access_token": "at",
  "refresh_token": "rt"
}`))
	if err != nil {
		t.Fatalf("Parse(legacy) error = %v", err)
	}
	if !legacy.IsSupportedAuth() {
		t.Fatal("expected document without type to be accepted")
	}
	if got := legacy.Provider(); got != ProviderCodex {
		t.Fatalf("legacy Provider() = %q, want %q", got, ProviderCodex)
	}
}

func testJWT(exp time.Time, clientID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadMap := map[string]any{"exp": exp.Unix()}
	if clientID != "" {
		payloadMap["client_id"] = clientID
	}
	payload, _ := json.Marshal(payloadMap)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + encodedPayload + ".sig"
}
