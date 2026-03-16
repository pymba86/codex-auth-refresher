package authfile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Schema string

const (
	SchemaFlat   Schema = "flat"
	SchemaNested Schema = "nested"
)

const (
	ProviderCodex       = "codex"
	ProviderGemini      = "gemini"
	ProviderAntigravity = "antigravity"
)

var ErrUnknownSchema = errors.New("unknown auth file schema")

type Document struct {
	path         string
	schema       Schema
	raw          map[string]json.RawMessage
	tokens       map[string]json.RawMessage
	tokenKeyName string
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
	doc.loadTokenContainer()

	switch {
	case hasTokenFields(raw):
		doc.schema = SchemaFlat
	case hasTokenFields(doc.tokens):
		doc.schema = SchemaNested
	default:
		return nil, ErrUnknownSchema
	}
	if doc.schema == SchemaNested && doc.tokenKeyName == "" {
		doc.tokenKeyName = "tokens"
	}
	return doc, nil
}

func (d *Document) loadTokenContainer() {
	for _, key := range []string{"tokens", "token"} {
		rawValue, ok := d.raw[key]
		if !ok {
			continue
		}
		var parsed map[string]json.RawMessage
		if err := json.Unmarshal(rawValue, &parsed); err != nil {
			continue
		}
		if d.tokens == nil {
			d.tokens = parsed
			d.tokenKeyName = key
		}
		if hasTokenFields(parsed) {
			d.tokens = parsed
			d.tokenKeyName = key
			return
		}
	}
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

func (d *Document) Type() string {
	return normalizeType(d.stringField(d.raw, "type"))
}

func (d *Document) Provider() string {
	if provider := d.Type(); provider != "" {
		return provider
	}
	if provider := ProviderFromFilename(d.path); provider != "" {
		return provider
	}
	return ProviderCodex
}

func (d *Document) IsSupportedAuth() bool {
	return IsSupportedProvider(d.Provider())
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

func (d *Document) ClientID() string {
	if d.schema == SchemaNested {
		if value := d.stringField(d.tokens, "client_id"); value != "" {
			return value
		}
	}
	return d.stringField(d.raw, "client_id")
}

func (d *Document) ClientSecret() string {
	if d.schema == SchemaNested {
		if value := d.stringField(d.tokens, "client_secret"); value != "" {
			return value
		}
	}
	return d.stringField(d.raw, "client_secret")
}

func (d *Document) TokenURI() string {
	if d.schema == SchemaNested {
		if value := d.stringField(d.tokens, "token_uri"); value != "" {
			return value
		}
	}
	return d.stringField(d.raw, "token_uri")
}

func (d *Document) ExplicitExpiry() (time.Time, bool) {
	for _, field := range []string{"expired", "expires_at"} {
		if value := d.stringField(d.raw, field); value != "" {
			if parsed, ok := parseTimestamp(value); ok {
				return parsed.UTC(), true
			}
		}
	}
	if value := d.stringField(d.tokens, "expiry"); value != "" {
		if parsed, ok := parseTimestamp(value); ok {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func (d *Document) LastRefresh() (time.Time, bool) {
	value := d.stringField(d.raw, "last_refresh")
	if value == "" {
		return time.Time{}, false
	}
	parsed, ok := parseTimestamp(value)
	if !ok {
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

func (d *Document) SetTimestamps(lastRefresh, expiresAt time.Time, expiresIn int64) {
	d.setString(d.raw, "last_refresh", lastRefresh.UTC().Format(time.RFC3339))
	if d.schema == SchemaFlat {
		d.setString(d.raw, "expired", expiresAt.UTC().Format(time.RFC3339))
		if expiresIn > 0 || hasField(d.raw, "expires_in") {
			d.setInt64(d.raw, "expires_in", expiresIn)
		}
	} else if d.tokenKeyName == "token" || hasField(d.tokens, "expiry") {
		d.setString(d.tokens, "expiry", expiresAt.UTC().Format(time.RFC3339Nano))
		if expiresIn > 0 || hasField(d.tokens, "expires_in") {
			d.setInt64(d.tokens, "expires_in", expiresIn)
		}
	}
	if hasField(d.raw, "timestamp") {
		d.setInt64(d.raw, "timestamp", lastRefresh.UTC().UnixMilli())
	}
}

func (d *Document) MarshalPreservingUnknownFields() ([]byte, error) {
	if d.schema == SchemaNested {
		encodedTokens, err := json.Marshal(d.tokens)
		if err != nil {
			return nil, fmt.Errorf("marshal nested tokens: %w", err)
		}
		if d.tokenKeyName == "" {
			d.tokenKeyName = "tokens"
		}
		d.raw[d.tokenKeyName] = encodedTokens
		for _, key := range []string{"tokens", "token"} {
			if key != d.tokenKeyName {
				delete(d.raw, key)
			}
		}
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

func (d *Document) setInt64(raw map[string]json.RawMessage, name string, value int64) {
	encoded, _ := json.Marshal(value)
	raw[name] = encoded
}

func hasField(raw map[string]json.RawMessage, name string) bool {
	_, ok := raw[name]
	return ok
}

func hasTokenFields(raw map[string]json.RawMessage) bool {
	return hasField(raw, "access_token") || hasField(raw, "refresh_token") || hasField(raw, "id_token")
}

func IsTrackedFilename(path string) bool {
	return ProviderFromFilename(path) != ""
}

func ProviderFromFilename(path string) string {
	base := strings.ToLower(filepath.Base(path))
	if !strings.HasSuffix(base, ".json") {
		return ""
	}
	for _, provider := range []string{ProviderCodex, ProviderGemini, ProviderAntigravity} {
		if strings.HasPrefix(base, provider+"-") {
			return provider
		}
	}
	return ""
}

func IsSupportedProvider(value string) bool {
	switch normalizeType(value) {
	case ProviderCodex, ProviderGemini, ProviderAntigravity:
		return true
	default:
		return false
	}
}

func normalizeType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func parseTimestamp(value string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}
