package providerhost

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/magelift/magelift/internal/cosign"
)

// ArtifactPaths locates the installed provider artifacts beside a CLI
// executable: the lockfile plus the provider binary. The Cosign bundle
// path resolves from the lockfile entry at load time.
type ArtifactPaths struct {
	Lock   string
	Binary string
}

// DiscoverArtifactPaths resolves provider artifact paths beside the given
// executable. An empty executable resolves against the working directory.
func DiscoverArtifactPaths(executablePath, provider string) ArtifactPaths {
	dir := "."
	if executablePath != "" {
		dir = filepath.Dir(executablePath)
	}
	binary := filepath.Join(dir, ExtractName(provider))
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	return ArtifactPaths{
		Lock:   filepath.Join(dir, "magelift.providers.lock"),
		Binary: binary,
	}
}

// BlobVerifier verifies a Cosign bundle against a binary. It is satisfied
// by the production Cosign runner and by test fakes.
type BlobVerifier interface {
	VerifyBlob(context.Context, string, string, cosign.VerifyOptions) error
}

// NewCosignVerifier builds the production blob verifier.
func NewCosignVerifier() BlobVerifier {
	return cosignVerifier{client: cosign.New()}
}

type cosignVerifier struct {
	client *cosign.Client
}

func (v cosignVerifier) VerifyBlob(ctx context.Context, bundlePath, binaryPath string, options cosign.VerifyOptions) error {
	if v.client == nil {
		return cosign.ErrRunnerRequired
	}
	return v.client.VerifyBlob(ctx, bundlePath, binaryPath, options)
}

// CachedDialer dials once and shares the session. The first caller supplies
// the dial context; later callers reuse the cached session (or its error).
type CachedDialer struct {
	Dial DialFunc

	mu        sync.Mutex
	cached    *Client
	cachedErr error
}

// Do dials once and returns the cached session.
func (d *CachedDialer) Do(ctx context.Context) (*Client, error) {
	if d == nil || d.Dial == nil {
		return nil, errors.New("provider dialer is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cached != nil || d.cachedErr != nil {
		return d.cached, d.cachedErr
	}
	d.cached, d.cachedErr = d.Dial(ctx)
	return d.cached, d.cachedErr
}

// Close reaps the cached provider process. It is safe to call more than once
// and safe on a nil dialer. The cached client is dropped so a later Do dials
// again instead of returning a dead session.
func (d *CachedDialer) Close() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cached != nil {
		d.cached.Close()
		d.cached = nil
	}
}
