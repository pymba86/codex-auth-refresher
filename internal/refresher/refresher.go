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
	ErrUnknownExpiry       = errors.New("unable to determine token expiry")
)

type TokenRefresher interface {
	Refresh(ctx context.Context, refreshToken, clientID string) (*oauth.Response, error)
}

type Inspection struct {
	Path                string
	File                string
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
	client          TokenRefresher
	refreshBefore   time.Duration
	refreshMaxAge   time.Duration
	defaultClientID string
	now             func() time.Time
}

func NewService(client TokenRefresher, refreshBefore, refreshMaxAge time.Duration, defaultClientID string) *Service {
	return &Service{
		client:          client,
		refreshBefore:   refreshBefore,
		refreshMaxAge:   refreshMaxAge,
		defaultClientID: defaultClientID,
		now:             func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) InspectFile(path string) (Inspection, error) {
	doc, err := authfile.Load(path)
	if err != nil {
		return Inspection{Path: path, File: path}, err
	}
	return s.inspectDocument(doc), nil
}

func (s *Service) RefreshFile(ctx context.Context, path string) (Result, error) {
	doc, err := authfile.Load(path)
	if err != nil {
		return Result{Inspection: Inspection{Path: path, File: path}}, err
	}
	inspection := s.inspectDocument(doc)
	if inspection.Disabled {
		return Result{Inspection: inspection, Refreshed: false}, nil
	}
	if !inspection.RefreshTokenPresent {
		return Result{Inspection: inspection}, ErrMissingRefreshToken
	}
	if inspection.ClientID == "" {
		return Result{Inspection: inspection}, ErrMissingClientID
	}

	response, err := s.client.Refresh(ctx, doc.RefreshToken(), inspection.ClientID)
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
	doc.SetTimestamps(lastRefresh, expiresAt)
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
	inspection := Inspection{
		Path:                doc.FilePath(),
		File:                doc.BaseName(),
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
	if clientID, ok, err := jwtutil.ExtractClientID(doc.AccessToken()); err == nil && ok {
		inspection.ClientID = clientID
	} else {
		inspection.ClientID = s.defaultClientID
	}
	if lastRefresh, ok := doc.LastRefresh(); ok {
		inspection.LastRefreshAt = timePointer(lastRefresh)
	}
	if expiresAt, ok := resolveDocumentExpiry(doc); ok {
		inspection.ExpiresAt = timePointer(expiresAt)
	}
	inspection.NextRefreshAt, inspection.RefreshDue = s.computeSchedule(inspection.ExpiresAt, inspection.LastRefreshAt, now)
	return inspection
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

func (s *Service) computeSchedule(expiresAt, lastRefreshAt *time.Time, now time.Time) (*time.Time, bool) {
	candidates := make([]time.Time, 0, 2)
	refreshDue := false

	if expiresAt != nil {
		expiry := expiresAt.UTC()
		nextFromExpiry := expiry.Add(-s.refreshBefore)
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
