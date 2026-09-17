package providerhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	maxDownloadBinaryBytes = 256 << 20
	maxDownloadBundleBytes = 1 << 20
	downloadTimeout        = 5 * time.Minute
)

// DefaultCacheDir is the user provider cache: the platform cache directory
// plus magelift/providers (XDG_CACHE_HOME honored on POSIX via
// os.UserCacheDir, LocalAppData on Windows).
func DefaultCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate user cache directory: %w", err)
	}
	return filepath.Join(base, "magelift", "providers"), nil
}

// CachedArtifactPaths resolves the cache locations for one provider version.
// Layout: <cache>/<id>/<version>/<binary> plus the bundle beside it.
func CachedArtifactPaths(cacheDir, provider, version string) (ArtifactPaths, error) {
	id := strings.TrimSpace(provider)
	if id == "" {
		return ArtifactPaths{}, errors.New("provider is required")
	}
	if !safePathSegment(version) {
		return ArtifactPaths{}, fmt.Errorf("provider version %q is not a safe path segment", version)
	}
	binary := ExtractName(id)
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	dir := filepath.Join(cacheDir, id, strings.TrimSpace(version))
	return ArtifactPaths{
		Lock:   "",
		Binary: filepath.Join(dir, binary),
	}, nil
}

// CachedBundlePath is the bundle location beside a cached binary.
func CachedBundlePath(cachedBinary string) string {
	return cachedBinary + ".sigstore.json"
}

func safePathSegment(segment string) bool {
	trimmed := strings.TrimSpace(segment)
	if trimmed == "" || trimmed != segment {
		return false
	}
	if strings.ContainsAny(trimmed, `/\`) || trimmed == "." || trimmed == ".." {
		return false
	}
	return true
}

// ResolveCached returns the cached binary and bundle for the locked provider
// version. A missing cache entry wraps ErrCacheMiss so callers can tell
// "not installed" from "lockfile broken" (a missing lockfile stays a plain
// open error).
func ResolveCached(lockPath, provider, cacheDir string) (string, string, error) {
	file, err := os.Open(lockPath)
	if err != nil {
		return "", "", fmt.Errorf("open magelift.providers.lock: %w", err)
	}
	defer file.Close()
	lock, err := ParseLock(file)
	if err != nil {
		return "", "", err
	}
	artifact, err := lock.Artifact(provider)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(cacheDir) == "" {
		cacheDir, err = DefaultCacheDir()
		if err != nil {
			return "", "", err
		}
	}
	paths, err := CachedArtifactPaths(cacheDir, provider, artifact.Version)
	if err != nil {
		return "", "", err
	}
	bundle := CachedBundlePath(paths.Binary)
	if !fileExists(paths.Binary) || !fileExists(bundle) {
		return "", "", fmt.Errorf("provider %q version %s: %w", strings.TrimSpace(provider), strings.TrimSpace(artifact.Version), ErrCacheMiss)
	}
	return paths.Binary, bundle, nil
}

// Downloader fetches provider binaries plus their Cosign bundles into the
// user cache, verifying identity and content before anything is installed.
type Downloader struct {
	HTTP     *http.Client
	Verifier BlobVerifier
	CacheDir string
	// LockDir resolves relative bundle references (the lockfile directory).
	LockDir string
}

// Installed describes one cache entry written or confirmed by Install.
type Installed struct {
	Provider string
	Version  string
	Binary   string
	Bundle   string
	Cached   bool
}

func (d *Downloader) client() *http.Client {
	if d.HTTP != nil {
		return d.HTTP
	}
	return &http.Client{Timeout: downloadTimeout}
}

func (d *Downloader) cacheDir() (string, error) {
	if strings.TrimSpace(d.CacheDir) != "" {
		return d.CacheDir, nil
	}
	return DefaultCacheDir()
}

// Install verifies the lockfile entry and ensures it exists in the cache.
// A present entry whose digest matches is returned without network use.
// Otherwise the binary and bundle are downloaded (both must name remote
// locations, or the bundle must exist beside the lockfile), verified with
// VerifyLocal, and atomically installed. Nothing is executed.
func (d *Downloader) Install(ctx context.Context, provider string, artifact Artifact) (Installed, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := artifact.validate(); err != nil {
		return Installed{}, err
	}
	if d.Verifier == nil {
		return Installed{}, fmt.Errorf("%w: cosign verifier is required", ErrUnsigned)
	}
	cacheDir, err := d.cacheDir()
	if err != nil {
		return Installed{}, err
	}
	paths, err := CachedArtifactPaths(cacheDir, provider, artifact.Version)
	if err != nil {
		return Installed{}, err
	}
	bundle := CachedBundlePath(paths.Binary)
	installed := Installed{
		Provider: strings.TrimSpace(provider),
		Version:  strings.TrimSpace(artifact.Version),
		Binary:   paths.Binary,
		Bundle:   bundle,
	}
	if digestMatches(paths.Binary, artifact.Digest) && fileExists(bundle) {
		installed.Cached = true
		return installed, nil
	}
	binaryURL := strings.TrimSpace(artifact.URL)
	if binaryURL == "" {
		return Installed{}, fmt.Errorf("provider %q ships beside the CLI: %w", installed.Provider, ErrNotDownloadable)
	}
	if err := validDownloadURL(binaryURL); err != nil {
		return Installed{}, err
	}
	if err := os.MkdirAll(filepath.Dir(paths.Binary), 0o755); err != nil {
		return Installed{}, fmt.Errorf("create provider cache directory: %w", err)
	}
	binaryBody, err := d.download(ctx, binaryURL, maxDownloadBinaryBytes)
	if err != nil {
		return Installed{}, err
	}
	if err := verifyDigestBytes(binaryBody, artifact.Digest); err != nil {
		return Installed{}, err
	}
	bundleBody, err := d.bundleBytes(ctx, artifact)
	if err != nil {
		return Installed{}, err
	}
	tempBinary, err := writeTempFile(filepath.Dir(paths.Binary), ".download-*-"+filepath.Base(paths.Binary), binaryBody, 0o755)
	if err != nil {
		return Installed{}, err
	}
	defer os.Remove(tempBinary)
	tempBundle, err := writeTempFile(filepath.Dir(paths.Binary), ".download-*-"+filepath.Base(bundle), bundleBody, 0o644)
	if err != nil {
		return Installed{}, err
	}
	defer os.Remove(tempBundle)
	if err := VerifyLocal(ctx, artifact, tempBinary, tempBundle, d.Verifier); err != nil {
		return Installed{}, err
	}
	if err := os.Rename(tempBinary, paths.Binary); err != nil {
		return Installed{}, fmt.Errorf("install provider binary: %w", err)
	}
	if err := os.Rename(tempBundle, bundle); err != nil {
		os.Remove(paths.Binary)
		return Installed{}, fmt.Errorf("install provider bundle: %w", err)
	}
	return installed, nil
}

func (d *Downloader) bundleBytes(ctx context.Context, artifact Artifact) ([]byte, error) {
	reference := strings.TrimSpace(artifact.Cosign.Bundle)
	if reference == "" {
		return nil, fmt.Errorf("%w: cosign bundle reference is required", ErrUnsigned)
	}
	if isRemoteReference(reference) {
		if err := validDownloadURL(reference); err != nil {
			return nil, err
		}
		return d.download(ctx, reference, maxDownloadBundleBytes)
	}
	if strings.TrimSpace(d.LockDir) == "" {
		return nil, fmt.Errorf("%w: bundle %q is relative but no lockfile directory is configured", ErrUnsigned, reference)
	}
	path := filepath.Join(d.LockDir, reference)
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read provider bundle: %w", err)
	}
	if len(body) > maxDownloadBundleBytes {
		return nil, fmt.Errorf("provider bundle exceeds %d bytes", maxDownloadBundleBytes)
	}
	return body, nil
}

func isRemoteReference(reference string) bool {
	parsed, err := url.Parse(reference)
	if err != nil {
		return false
	}
	return parsed.Scheme == "https" && parsed.Host != ""
}

// validDownloadURL mirrors the updater's rule: HTTPS, no credentials, no
// query, no fragment. One policy; the predicate lives here for the provider
// path and in internal/upgrade for the CLI path.
func validDownloadURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("provider download URL must be an HTTPS URL without credentials or query parameters")
	}
	return nil
}

func (d *Downloader) download(ctx context.Context, endpoint string, limit int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create provider download request: %w", err)
	}
	request.Header.Set("User-Agent", "magelift-provider-download")
	response, err := d.client().Do(request)
	if err != nil {
		return nil, fmt.Errorf("download provider artifact: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download provider artifact: HTTP %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read provider artifact: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("provider artifact exceeds %d bytes", limit)
	}
	return body, nil
}

func verifyDigestBytes(body []byte, want string) error {
	sum := sha256.Sum256(body)
	got := "sha256:" + hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, strings.TrimSpace(want)) {
		return fmt.Errorf("%w: got %s want %s", ErrChecksumMismatch, got, strings.TrimSpace(want))
	}
	return nil
}

func digestMatches(path, want string) bool {
	sum, err := fileSHA256(path)
	if err != nil {
		return false
	}
	return strings.EqualFold("sha256:"+sum, strings.TrimSpace(want))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func writeTempFile(dir, pattern string, body []byte, perm os.FileMode) (string, error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("create temporary provider file: %w", err)
	}
	name := file.Name()
	if err := file.Chmod(perm); err != nil {
		file.Close()
		os.Remove(name)
		return "", fmt.Errorf("set provider file permissions: %w", err)
	}
	if _, err := file.Write(body); err != nil {
		file.Close()
		os.Remove(name)
		return "", fmt.Errorf("write provider file: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(name)
		return "", fmt.Errorf("sync provider file: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("close provider file: %w", err)
	}
	return name, nil
}
