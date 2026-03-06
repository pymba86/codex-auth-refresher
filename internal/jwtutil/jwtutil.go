package jwtutil

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrMalformedToken = errors.New("malformed JWT token")

func Claims(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, ErrMalformedToken
	}
	payload := parts[1]
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(payload)
		if err != nil {
			return nil, fmt.Errorf("decode JWT payload: %w", err)
		}
	}
	var claims map[string]any
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, fmt.Errorf("parse JWT claims: %w", err)
	}
	return claims, nil
}

func ExtractExpiry(token string) (time.Time, bool, error) {
	if token == "" {
		return time.Time{}, false, nil
	}
	claims, err := Claims(token)
	if err != nil {
		return time.Time{}, false, err
	}
	raw, ok := claims["exp"]
	if !ok {
		return time.Time{}, false, nil
	}
	switch value := raw.(type) {
	case float64:
		return time.Unix(int64(value), 0).UTC(), true, nil
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return time.Time{}, false, err
		}
		return time.Unix(parsed, 0).UTC(), true, nil
	default:
		return time.Time{}, false, fmt.Errorf("unsupported exp type %T", raw)
	}
}

func ExtractClientID(token string) (string, bool, error) {
	if token == "" {
		return "", false, nil
	}
	claims, err := Claims(token)
	if err != nil {
		return "", false, err
	}
	raw, ok := claims["client_id"]
	if !ok {
		return "", false, nil
	}
	clientID, ok := raw.(string)
	if !ok || clientID == "" {
		return "", false, nil
	}
	return clientID, true, nil
}
