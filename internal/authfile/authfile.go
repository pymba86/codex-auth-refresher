package authfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Schema string

const (
	SchemaFlat   Schema = "flat"
	SchemaNested Schema = "nested"
)

var ErrUnknownSchema = errors.New("unknown auth file schema")

type Document struct {
	path   string
	schema Schema
	raw    map[string]json.RawMessage
	tokens map[string]json.RawMessage
}

func Load(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, data)
}

func Parse(path string, data []byte) (*Document, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	doc := &Document{path: filepath.Clean(path), raw: raw}
	if tokensRaw, ok := raw["tokens"]; ok {
		var tokens map[string]json.RawMessage
		if err := json.Unmarshal(tokensRaw, &tokens); err == nil {
			doc.tokens = tokens
		}
	}

	switch {
	case hasField(raw, "access_token") || hasField(raw, "refresh_token") || hasField(raw, "id_token"):
		doc.schema = SchemaFlat
	case len(doc.tokens) > 0 && (hasField(doc.tokens, "access_token") || hasField(doc.tokens, "refresh_token") || hasField(doc.tokens, "id_token")):
		doc.schema = SchemaNested
	default:
		return nil, ErrUnknownSchema
	}
	return doc, nil
}

func (d *Document) SchemaName() string {
	return string(d.schema)
}

func (d *Document) FilePath() string {
	return d.path
}

func (d *Document) BaseName() string {
	return filepath.Base(d.path)
}

func (d *Document) AccountID() string {
	return d.stringField(d.raw, "account_id")
}

func (d *Document) Disabled() bool {
	return d.boolField(d.raw, "disabled")
}

func (d *Document) AccessToken() string {
	if d.schema == SchemaFlat {
		return d.stringField(d.raw, "access_token")
	}
	return d.stringField(d.tokens, "access_token")
}

func (d *Document) RefreshToken() string {
	if d.schema == SchemaFlat {
		return d.stringField(d.raw, "refresh_token")
	}
	return d.stringField(d.tokens, "refresh_token")
}

func (d *Document) IDToken() string {
	if d.schema == SchemaFlat {
		return d.stringField(d.raw, "id_token")
	}
	return d.stringField(d.tokens, "id_token")
}

func (d *Document) ExplicitExpiry() (time.Time, bool) {
	for _, field := range []string{"expired", "expires_at"} {
		if value := d.stringField(d.raw, field); value != "" {
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				return parsed.UTC(), true
			}
		}
	}
	return time.Time{}, false
}

func (d *Document) LastRefresh() (time.Time, bool) {
	value := d.stringField(d.raw, "last_refresh")
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

func (d *Document) SetTokens(accessToken, refreshToken, idToken string) {
	if d.schema == SchemaFlat {
		d.setString(d.raw, "access_token", accessToken)
		d.setString(d.raw, "refresh_token", refreshToken)
		d.setString(d.raw, "id_token", idToken)
		return
	}
	if d.tokens == nil {
		d.tokens = make(map[string]json.RawMessage)
	}
	d.setString(d.tokens, "access_token", accessToken)
	d.setString(d.tokens, "refresh_token", refreshToken)
	d.setString(d.tokens, "id_token", idToken)
}

func (d *Document) SetTimestamps(lastRefresh, expiresAt time.Time) {
	d.setString(d.raw, "last_refresh", lastRefresh.UTC().Format(time.RFC3339))
	if d.schema == SchemaFlat {
		d.setString(d.raw, "expired", expiresAt.UTC().Format(time.RFC3339))
	}
}

func (d *Document) MarshalPreservingUnknownFields() ([]byte, error) {
	if d.schema == SchemaNested {
		encodedTokens, err := json.Marshal(d.tokens)
		if err != nil {
			return nil, fmt.Errorf("marshal nested tokens: %w", err)
		}
		d.raw["tokens"] = encodedTokens
	}
	data, err := json.MarshalIndent(d.raw, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func (d *Document) stringField(raw map[string]json.RawMessage, name string) string {
	value, ok := raw[name]
	if !ok {
		return ""
	}
	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return ""
	}
	return decoded
}

func (d *Document) boolField(raw map[string]json.RawMessage, name string) bool {
	value, ok := raw[name]
	if !ok {
		return false
	}
	var decoded bool
	if err := json.Unmarshal(value, &decoded); err != nil {
		return false
	}
	return decoded
}

func (d *Document) setString(raw map[string]json.RawMessage, name, value string) {
	encoded, _ := json.Marshal(value)
	raw[name] = encoded
}

func hasField(raw map[string]json.RawMessage, name string) bool {
	_, ok := raw[name]
	return ok
}
