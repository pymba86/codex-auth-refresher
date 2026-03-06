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
	doc.SetTimestamps(time.Unix(3000, 0), time.Unix(4000, 0))
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
