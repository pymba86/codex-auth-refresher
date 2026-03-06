package jwtutil

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestExtractExpiryAndClientID(t *testing.T) {
	t.Parallel()
	expiresAt := time.Unix(1893456000, 0).UTC()
	token := testJWT(expiresAt, "client-123")
	gotExpiry, ok, err := ExtractExpiry(token)
	if err != nil {
		t.Fatalf("ExtractExpiry() error = %v", err)
	}
	if !ok || !gotExpiry.Equal(expiresAt) {
		t.Fatalf("ExtractExpiry() = %v, %v, want %v", gotExpiry, ok, expiresAt)
	}
	clientID, ok, err := ExtractClientID(token)
	if err != nil {
		t.Fatalf("ExtractClientID() error = %v", err)
	}
	if !ok || clientID != "client-123" {
		t.Fatalf("ExtractClientID() = %q, %v", clientID, ok)
	}
}

func TestMalformedToken(t *testing.T) {
	t.Parallel()
	if _, _, err := ExtractExpiry("not-a-jwt"); err == nil {
		t.Fatal("expected malformed token error")
	}
}

func testJWT(exp time.Time, clientID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadMap := map[string]any{"exp": exp.Unix(), "client_id": clientID}
	payload, _ := json.Marshal(payloadMap)
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	return header + "." + encodedPayload + ".sig"
}
