package refresher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"codex-auth-refresher/internal/authfile"
	"codex-auth-refresher/internal/jwtutil"
	"codex-auth-refresher/internal/oauth"
	"codex-auth-refresher/internal/storage"
)

type FileState string

const (
	StateOK             FileState = "ok"
	StateDegraded       FileState = "degraded"
	StateReauthRequired FileState = "reauth_required"
	StateInvalidJSON    FileState = "invalid_json"
)

var (
	ErrMissingRefreshToken = errors.New("missing refresh token")
	ErrMissingClientID     = errors.New("missing client id")
	ErrMissingClientSecret = errors.New("missing client secret")
	ErrMissingTokenURL     = errors.New("missing token endpoint")
	ErrUnsupportedAuth     = errors.New("auth file is not supported")
	ErrNonCodexAuth        = ErrUnsupportedAuth
	ErrUnknownExpiry       = errors.New("unable to determine token expiry")
)

type TokenRefresher interface {
	Refresh(ctx context.Context, request oauth.Request) (*oauth.Response, error)
}

type ProviderConfig struct {
	CodexTokenEndpoint       string
	CodexClientID            string
	AntigravityTokenEndpoint string
	AntigravityClientID      string
	AntigravityClientSecret  string
}

type Inspection struct {
	Path                string
	File                string
	Type                string
	AccountID           string
	AccountKey          string
	Schema              string
	Disabled            bool
	RefreshDue          bool
	RefreshTokenPresent bool
	ClientID            string
	ExpiresAt           *time.Time
	NextRefreshAt       *time.Time
	LastRefreshAt       *time.Time
}

type Result struct {
	Inspection Inspection
	Refreshed  bool
}

type Service struct {
	client        TokenRefresher
	refreshBefore time.Duration
	refreshMaxAge time.Duration
	config        ProviderConfig
	now           func() time.Time
}

func NewService(client TokenRefresher, refreshBefore, refreshMaxAge time.Duration, config ProviderConfig) *Service {
	return &Service{
		client:        client,
		refreshBefore: refreshBefore,
		refreshMaxAge: refreshMaxAge,
		config:        config,
		now:           func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) InspectFile(path string) (Inspection, error) {
	doc, err := authfile.Load(path)
	if err != nil {
		return Inspection{Path: path, File: path}, err
	}
	if !doc.IsSupportedAuth() {
		return Inspection{Path: path, File: path}, ErrUnsupportedAuth
	}
	return s.inspectDocument(doc), nil
}

func (s *Service) RefreshFile(ctx context.Context, path string) (Result, error) {
	doc, err := authfile.Load(path)
	if err != nil {
		return Result{Inspection: Inspection{Path: path, File: path}}, err
	}
	if !doc.IsSupportedAuth() {
		return Result{Inspection: Inspection{Path: path, File: path}}, ErrUnsupportedAuth
	}
	inspection := s.inspectDocument(doc)
	if inspection.Disabled {
		return Result{Inspection: inspection, Refreshed: false}, nil
	}
	if !inspection.RefreshTokenPresent {
		return Result{Inspection: inspection}, ErrMissingRefreshToken
	}
	request, err := s.buildRefreshRequest(doc, inspection)
	if err != nil {
		return Result{Inspection: inspection}, err
	}

	response, err := s.client.Refresh(ctx, request)
	if err != nil {
		return Result{Inspection: inspection}, err
	}
	expiresAt, ok, err := s.resolveExpiry(response.AccessToken, response.IDToken, response.ExpiresIn)
	if err != nil {
		return Result{Inspection: inspection}, err
	}
	if !ok {
		return Result{Inspection: inspection}, ErrUnknownExpiry
	}

	accessToken := response.AccessToken
	refreshToken := doc.RefreshToken()
	if response.RefreshToken != "" {
		refreshToken = response.RefreshToken
	}
	idToken := doc.IDToken()
	if response.IDToken != "" {
		idToken = response.IDToken
	}
	lastRefresh := s.now().UTC()
	doc.SetTokens(accessToken, refreshToken, idToken)
	doc.SetTimestamps(lastRefresh, expiresAt, expiresInForWrite(response.ExpiresIn, lastRefresh, expiresAt))
	data, err := doc.MarshalPreservingUnknownFields()
	if err != nil {
		return Result{Inspection: inspection}, err
	}
	perm := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}
	if err := storage.WriteFileAtomic(path, data, perm); err != nil {
		return Result{Inspection: inspection}, fmt.Errorf("write auth file: %w", err)
	}
	updatedInspection := s.inspectDocument(doc)
	updatedInspection.LastRefreshAt = timePointer(lastRefresh)
	updatedInspection.ExpiresAt = timePointer(expiresAt)
	updatedInspection.NextRefreshAt, _ = s.computeSchedule(updatedInspection.ExpiresAt, updatedInspection.LastRefreshAt, lastRefresh)
	updatedInspection.RefreshDue = false
	return Result{Inspection: updatedInspection, Refreshed: true}, nil
}

func (s *Service) inspectDocument(doc *authfile.Document) Inspection {
	now := s.now().UTC()
	provider := doc.Provider()
	inspection := Inspection{
		Path:                doc.FilePath(),
		File:                doc.BaseName(),
		Type:                provider,
		AccountID:           doc.AccountID(),
		Schema:              doc.SchemaName(),
		Disabled:            doc.Disabled(),
		RefreshTokenPresent: doc.RefreshToken() != "",
	}
	if inspection.AccountID != "" {
		inspection.AccountKey = inspection.AccountID
	} else {
		inspection.AccountKey = inspection.Path
	}
	inspection.ClientID = s.resolveClientID(doc, provider)
	if lastRefresh, ok := doc.LastRefresh(); ok {
		inspection.LastRefreshAt = timePointer(lastRefresh)
	}
	if expiresAt, ok := resolveDocumentExpiry(doc); ok {
		inspection.ExpiresAt = timePointer(expiresAt)
	}
	inspection.NextRefreshAt, inspection.RefreshDue = s.computeSchedule(inspection.ExpiresAt, inspection.LastRefreshAt, now)
	return inspection
}

func (s *Service) resolveClientID(doc *authfile.Document, provider string) string {
	switch provider {
	case authfile.ProviderGemini:
		if value := doc.ClientID(); value != "" {
			return value
		}
		if clientID, ok, err := jwtutil.ExtractClientID(doc.AccessToken()); err == nil && ok {
			return clientID
		}
		return ""
	case authfile.ProviderAntigravity:
		if value := doc.ClientID(); value != "" {
			return value
		}
		return s.config.AntigravityClientID
	}
	if clientID, ok, err := jwtutil.ExtractClientID(doc.AccessToken()); err == nil && ok {
		return clientID
	}
	if value := doc.ClientID(); value != "" {
		return value
	}
	return s.config.CodexClientID
}

func (s *Service) buildRefreshRequest(doc *authfile.Document, inspection Inspection) (oauth.Request, error) {
	request := oauth.Request{RefreshToken: doc.RefreshToken()}
	switch inspection.Type {
	case authfile.ProviderCodex:
		request.Endpoint = s.config.CodexTokenEndpoint
		request.ClientID = inspection.ClientID
	case authfile.ProviderGemini:
		request.Endpoint = firstNonEmpty(doc.TokenURI(), s.config.AntigravityTokenEndpoint)
		request.ClientID = firstNonEmpty(doc.ClientID(), inspection.ClientID)
		request.ClientSecret = doc.ClientSecret()
		if request.ClientSecret == "" {
			return oauth.Request{}, ErrMissingClientSecret
		}
	case authfile.ProviderAntigravity:
		request.Endpoint = firstNonEmpty(doc.TokenURI(), s.config.AntigravityTokenEndpoint)
		request.ClientID = firstNonEmpty(doc.ClientID(), inspection.ClientID)
		request.ClientSecret = firstNonEmpty(doc.ClientSecret(), s.config.AntigravityClientSecret)
		if request.ClientSecret == "" {
			return oauth.Request{}, ErrMissingClientSecret
		}
	default:
		return oauth.Request{}, ErrUnsupportedAuth
	}
	if request.Endpoint == "" {
		return oauth.Request{}, ErrMissingTokenURL
	}
	if request.ClientID == "" {
		return oauth.Request{}, ErrMissingClientID
	}
	return request, nil
}

func resolveDocumentExpiry(doc *authfile.Document) (time.Time, bool) {
	for _, candidate := range []string{doc.AccessToken(), doc.IDToken()} {
		expiresAt, ok, err := jwtutil.ExtractExpiry(candidate)
		if err == nil && ok {
			return expiresAt, true
		}
	}
	if explicit, ok := doc.ExplicitExpiry(); ok {
		return explicit, true
	}
	return time.Time{}, false
}

func (s *Service) resolveExpiry(accessToken, idToken string, expiresIn int64) (time.Time, bool, error) {
	for _, candidate := range []string{accessToken, idToken} {
		expiresAt, ok, err := jwtutil.ExtractExpiry(candidate)
		if err == nil && ok {
			return expiresAt, true, nil
		}
	}
	if expiresIn > 0 {
		return s.now().UTC().Add(time.Duration(expiresIn) * time.Second), true, nil
	}
	return time.Time{}, false, nil
}

func expiresInForWrite(expiresIn int64, refreshedAt, expiresAt time.Time) int64 {
	if expiresIn > 0 {
		return expiresIn
	}
	if expiresAt.After(refreshedAt) {
		return int64(expiresAt.Sub(refreshedAt) / time.Second)
	}
	return 0
}

func (s *Service) computeSchedule(expiresAt, lastRefreshAt *time.Time, now time.Time) (*time.Time, bool) {
	candidates := make([]time.Time, 0, 2)
	refreshDue := false

	if expiresAt != nil {
		expiry := expiresAt.UTC()
		nextFromExpiry := expiry.Add(-s.refreshBefore)
		// Some providers issue tokens whose entire lifetime is shorter than refreshBefore.
		// Once such a token has just been refreshed, queueing it again immediately only creates
		// a tight refresh loop. In that case, expiry is the earliest future point we can derive.
		if lastRefreshAt != nil {
			refreshedAt := lastRefreshAt.UTC()
			if expiry.After(refreshedAt) && !nextFromExpiry.After(refreshedAt) {
				nextFromExpiry = expiry
			}
		}
		candidates = append(candidates, nextFromExpiry)
		if !expiry.After(now) || !nextFromExpiry.After(now) {
			refreshDue = true
		}
	}

	if s.refreshMaxAge > 0 {
		if lastRefreshAt != nil {
			nextFromAge := lastRefreshAt.UTC().Add(s.refreshMaxAge)
			candidates = append(candidates, nextFromAge)
			if !nextFromAge.After(now) {
				refreshDue = true
			}
		} else {
			candidates = append(candidates, now.UTC())
			refreshDue = true
		}
	}

	if len(candidates) == 0 {
		return nil, true
	}

	nextRefreshAt := candidates[0]
	for _, candidate := range candidates[1:] {
		if candidate.Before(nextRefreshAt) {
			nextRefreshAt = candidate
		}
	}
	return timePointer(nextRefreshAt), refreshDue
}

func timePointer(value time.Time) *time.Time {
	copy := value.UTC()
	return &copy
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
