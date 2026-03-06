package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	endpoint   string
	httpClient *http.Client
}

type Response struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	IDToken          string `json:"id_token"`
	ExpiresIn        int64  `json:"expires_in"`
	ErrorCode        string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type Error struct {
	StatusCode  int
	Code        string
	Description string
	Retryable   bool
}

func (e *Error) Error() string {
	summary := e.Code
	if summary == "" {
		summary = "oauth_refresh_failed"
	}
	if e.Description == "" {
		return summary
	}
	return summary + ": " + e.Description
}

func (e *Error) InvalidGrant() bool {
	return e.Code == "invalid_grant" || strings.Contains(strings.ToLower(e.Description), "invalid grant")
}

func NewClient(endpoint string, httpClient *http.Client) *Client {
	return &Client{endpoint: endpoint, httpClient: httpClient}
}

func (c *Client) Refresh(ctx context.Context, refreshToken, clientID string) (*Response, error) {
	values := url.Values{}
	values.Set("grant_type", "refresh_token")
	values.Set("refresh_token", refreshToken)
	values.Set("client_id", clientID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewBufferString(values.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, &Error{Code: "network_error", Description: err.Error(), Retryable: true}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var parsed Response
	if len(body) > 0 {
		_ = json.Unmarshal(body, &parsed)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		message := parsed.ErrorDescription
		if message == "" {
			message = strings.TrimSpace(string(body))
		}
		return nil, &Error{
			StatusCode:  resp.StatusCode,
			Code:        parsed.ErrorCode,
			Description: message,
			Retryable:   resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError,
		}
	}
	if parsed.ErrorCode != "" {
		return nil, &Error{StatusCode: resp.StatusCode, Code: parsed.ErrorCode, Description: parsed.ErrorDescription}
	}
	if parsed.AccessToken == "" {
		return nil, &Error{StatusCode: resp.StatusCode, Code: "missing_access_token", Description: fmt.Sprintf("response missing access_token at %s", time.Now().UTC().Format(time.RFC3339))}
	}
	return &parsed, nil
}
