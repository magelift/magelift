// Package auth provides fresh GCP credentials per operation. Every call
// resolves ambient Application Default Credentials (workstation, GCE
// metadata, or CI workload identity federation) with no saved-token
// dependence: sources are built per operation, expiry self-heals through
// token refresh, and refresh failure surfaces the raw cause for typed
// mapping upstream. Refreshed secrets never land in logs.
package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// TokenRefresher resolves a fresh access token for GCP API calls.
type TokenRefresher interface {
	// Token returns a valid access token, refreshing when expired.
	Token(ctx context.Context) (string, error)
}

// CloudPlatformScope is the OAuth scope for full GCP API access.
const CloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"

// DefaultTokenSource builds a fresh ADC token source for one operation.
// Nothing is cached: restart re-reads ambient credentials statelessly, and
// CI workload identity flows through this same path.
func DefaultTokenSource(ctx context.Context, scopes ...string) (oauth2.TokenSource, error) {
	if ctx == nil {
		return nil, errors.New("GCP auth context is required")
	}
	if len(scopes) == 0 {
		scopes = []string{CloudPlatformScope}
	}
	source, err := google.DefaultTokenSource(ctx, scopes...)
	if err != nil {
		return nil, fmt.Errorf("resolve GCP application default credentials: %w", err)
	}
	return source, nil
}

// Refresher serves fresh access tokens from an ADC token source, reusing
// only currently valid tokens. Expiry always refetches; failures return
// the raw cause unwrapped so callers map refresh faults to typed
// credential errors. A nil logger discards operational logs.
type Refresher struct {
	source oauth2.TokenSource
	logger *log.Logger
}

// NewRefresher wraps a token source for per-operation access tokens.
func NewRefresher(source oauth2.TokenSource, logger *log.Logger) *Refresher {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	return &Refresher{source: source, logger: logger}
}

// Token returns a valid access token, refreshing when expired. Token
// values never enter logs; failures log the error text only. The refresh
// is a single local fetch guarded by ctx.
func (r *Refresher) Token(ctx context.Context) (string, error) {
	if r == nil || r.source == nil {
		return "", errors.New("GCP token source is required")
	}
	if ctx == nil {
		return "", errors.New("GCP auth context is required")
	}
	type outcome struct {
		token *oauth2.Token
		err   error
	}
	done := make(chan outcome, 1)
	go func() {
		reuse := oauth2.ReuseTokenSource(nil, r.source)
		token, err := reuse.Token()
		done <- outcome{token: token, err: err}
	}()
	select {
	case out := <-done:
		if out.err != nil {
			r.logger.Printf("gcp token refresh failed: %v", out.err)
			return "", out.err
		}
		if out.token == nil || !out.token.Valid() {
			r.logger.Printf("gcp token source returned an invalid token")
			return "", errors.New("GCP token source returned an invalid token")
		}
		r.logger.Printf("gcp token refreshed")
		return out.token.AccessToken, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
