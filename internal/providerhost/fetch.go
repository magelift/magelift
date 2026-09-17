package providerhost

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/cosign"
)

const (
	// ReleaseDownloadBase serves release assets including the
	// per-platform provider lockfiles.
	ReleaseDownloadBase = "https://github.com/magelift/magelift/releases/download"
	maxLockfileBytes    = 1 << 20
	fetchTimeout        = 2 * time.Minute
)

// releaseTagPattern accepts full release tags only. Same policy as the
// updater and the CI generator: no branches, no mutable names.
var releaseTagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

// ValidReleaseTag reports whether version names a fetchable release tag.
func ValidReleaseTag(version string) bool {
	return releaseTagPattern.MatchString(strings.TrimSpace(version))
}

// FetchLock downloads the platform lockfile for a release version.
// baseURL defaults to the first-party release base; tests override it.
// The pinned-publisher rule in ParseLock authenticates the metadata:
// a lockfile naming any other publisher is refused.
func FetchLock(ctx context.Context, version, baseURL string, client *http.Client) (Lockfile, error) {
	if !ValidReleaseTag(version) {
		return Lockfile{}, fmt.Errorf("provider lockfile version %q is not a release tag", version)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	base := strings.TrimSpace(baseURL)
	if base == "" {
		base = ReleaseDownloadBase
	}
	platform := runtime.GOOS + "_" + runtime.GOARCH
	endpoint := strings.TrimRight(base, "/") + "/" + strings.TrimSpace(version) + "/magelift.providers.lock." + platform
	if !isRemoteReference(endpoint) {
		return Lockfile{}, fmt.Errorf("lockfile base URL must be HTTPS: %s", base)
	}
	if client == nil {
		client = &http.Client{Timeout: fetchTimeout}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Lockfile{}, fmt.Errorf("create lockfile request: %w", err)
	}
	request.Header.Set("User-Agent", "magelift-provider-bootstrap")
	response, err := client.Do(request)
	if err != nil {
		return Lockfile{}, fmt.Errorf("download provider lockfile: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Lockfile{}, fmt.Errorf("download provider lockfile: HTTP %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxLockfileBytes+1))
	if err != nil {
		return Lockfile{}, fmt.Errorf("read provider lockfile: %w", err)
	}
	if int64(len(body)) > maxLockfileBytes {
		return Lockfile{}, fmt.Errorf("provider lockfile exceeds %d bytes", maxLockfileBytes)
	}
	lock, err := ParseLock(strings.NewReader(string(body)))
	if err != nil {
		return Lockfile{}, err
	}
	return lock, nil
}

// cacheDirEnv overrides the user cache for installs and loads alike, so a
// custom cache is honored consistently by command execution.
const cacheDirEnv = "MAGELIFT_PROVIDER_CACHE_DIR"

// CacheDirFromEnv returns the cache override when set.
func CacheDirFromEnv() string {
	return strings.TrimSpace(os.Getenv(cacheDirEnv))
}

// EffectiveCacheDir resolves flag, environment, then default cache.
func EffectiveCacheDir(flag string) (string, error) {
	if trimmed := strings.TrimSpace(flag); trimmed != "" {
		return trimmed, nil
	}
	if env := CacheDirFromEnv(); env != "" {
		return env, nil
	}
	return DefaultCacheDir()
}

// NewPreferredVerifier returns a verifier backed by the cached bootstrap
// when present, else the system cosign. Pinned beats arbitrary: the
// cached copy is the tested version.
func NewPreferredVerifier(cacheDir string) BlobVerifier {
	resolved := strings.TrimSpace(cacheDir)
	if resolved == "" {
		if env := CacheDirFromEnv(); env != "" {
			resolved = env
		} else if def, err := DefaultCacheDir(); err == nil {
			resolved = def
		}
	}
	if resolved != "" {
		if path, err := CachedVerifierPath(resolved); err == nil {
			return cosignVerifier{client: cosign.NewWithBinary(path)}
		}
	}
	return NewCosignVerifier()
}
