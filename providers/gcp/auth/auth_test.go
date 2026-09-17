package auth

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type scriptedSource struct {
	tokens []*oauth2.Token
	errs   []error
	calls  int
}

func (s *scriptedSource) Token() (*oauth2.Token, error) {
	s.calls++
	if len(s.errs) > 0 {
		err := s.errs[0]
		s.errs = s.errs[1:]
		if err != nil {
			return nil, err
		}
	}
	if len(s.tokens) == 0 {
		return nil, errors.New("no token scripted")
	}
	token := s.tokens[0]
	s.tokens = s.tokens[1:]
	return token, nil
}

func TestTokenServesFreshPerCall(t *testing.T) {
	t.Parallel()
	source := &scriptedSource{tokens: []*oauth2.Token{
		{AccessToken: "fresh-token-1", Expiry: time.Now().Add(time.Hour)},
		{AccessToken: "fresh-token-2", Expiry: time.Now().Add(time.Hour)},
	}}
	var logs bytes.Buffer
	refresher := NewRefresher(source, log.New(&logs, "", 0))
	first, err := refresher.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := refresher.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first != "fresh-token-1" || second != "fresh-token-2" {
		t.Fatalf("tokens = %q %q, want fresh per call", first, second)
	}
	if source.calls != 2 {
		t.Fatalf("source calls = %d, want 2 (no saved tokens)", source.calls)
	}
	if strings.Contains(logs.String(), "fresh-token") {
		t.Fatalf("logs contain token values: %q", logs.String())
	}
}

func TestTokenRefusesExpiredToken(t *testing.T) {
	t.Parallel()
	source := &scriptedSource{tokens: []*oauth2.Token{
		{AccessToken: "expired-token", Expiry: time.Now().Add(-time.Hour)},
	}}
	var logs bytes.Buffer
	refresher := NewRefresher(source, log.New(&logs, "", 0))
	if token, err := refresher.Token(context.Background()); err == nil {
		t.Fatalf("expired token %q was served", token)
	}
	if strings.Contains(logs.String(), "expired-token") {
		t.Fatalf("logs contain token values: %q", logs.String())
	}
}

func TestRefreshFailureSurfacesRawCause(t *testing.T) {
	t.Parallel()
	boom := &oauth2.RetrieveError{ErrorCode: "invalid_grant", ErrorDescription: "token expired or revoked"}
	source := &scriptedSource{errs: []error{boom}}
	var logs bytes.Buffer
	refresher := NewRefresher(source, log.New(&logs, "", 0))
	if _, err := refresher.Token(context.Background()); err == nil {
		t.Fatal("refresh failure was accepted")
	} else {
		var retrieve *oauth2.RetrieveError
		if !errors.As(err, &retrieve) {
			t.Fatalf("cause chain lost RetrieveError: %v", err)
		}
	}
	if !strings.Contains(logs.String(), "refresh failed") {
		t.Fatalf("logs = %q, want refresh failure", logs.String())
	}
}

func TestTokenRejectsWithoutSource(t *testing.T) {
	t.Parallel()
	if _, err := NewRefresher(nil, nil).Token(context.Background()); err == nil {
		t.Fatal("nil source was accepted")
	}
	var nilCtx context.Context
	if _, err := NewRefresher(&scriptedSource{}, nil).Token(nilCtx); err == nil {
		t.Fatal("nil context was accepted")
	}
	var nilRefresher *Refresher
	if _, err := nilRefresher.Token(context.Background()); err == nil {
		t.Fatal("nil refresher was accepted")
	}
}

func writeCredentials(t *testing.T, name, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const authorizedUserCredentials = `{
  "type": "authorized_user",
  "client_id": "test-client-id",
  "client_secret": "test-client-secret",
  "refresh_token": "test-refresh-token"
}`

func TestTokenSourceIsStatelessAcrossRestarts(t *testing.T) {
	// No t.Parallel: mutates process credential environment.
	first := writeCredentials(t, "a.json", authorizedUserCredentials)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", first)
	t.Setenv("HOME", t.TempDir())
	if _, err := DefaultTokenSource(context.Background()); err != nil {
		t.Fatalf("first build: %v", err)
	}
	// Simulate restart with credentials gone: the builder must re-read
	// ambient state (and fail) rather than serve anything cached.
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GCE_METADATA_HOST", "169.254.169.254")
	if _, err := DefaultTokenSource(context.Background()); err == nil {
		t.Fatal("second build succeeded without credentials; sources must be stateless")
	}
}

func TestTokenSourceAcceptsCIWorkloadIdentity(t *testing.T) {
	// No t.Parallel: mutates process credential environment.
	wif := writeCredentials(t, "wif.json", `{
  "type": "external_account",
  "audience": "//iam.googleapis.com/projects/123/locations/global/workloadIdentityPools/pool/providers/provider",
  "subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
  "token_url": "https://sts.googleapis.com/v1/token",
  "credential_source": {
    "file": "/var/run/secrets/goog.id/token"
  }
}`)
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", wif)
	t.Setenv("HOME", t.TempDir())
	// Construction only: no network. The CI identity must flow through
	// the same ADC builder as workstation credentials.
	if _, err := DefaultTokenSource(context.Background()); err != nil {
		t.Fatalf("workload identity build: %v", err)
	}
}

func TestDefaultTokenSourceRequiresContext(t *testing.T) {
	t.Parallel()
	var nilCtx context.Context
	if _, err := DefaultTokenSource(nilCtx); err == nil {
		t.Fatal("nil context was accepted")
	}
}
