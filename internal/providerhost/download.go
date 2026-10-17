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
	cacheInstallLockName   = ".install.lock"
	cacheStagingPrefix     = ".staging-"
)

// CachePlatform is the GOOS_GOARCH segment in the versioned provider cache.
func CachePlatform() string {
	return runtime.GOOS + "_" + runtime.GOARCH
}

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

// CachedArtifactPaths resolves the canonical cache locations for one provider
// version on this machine. Layout:
// <cache>/<id>/<version>/<goos>_<goarch>/<binary> plus the bundle beside it.
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
	dir := filepath.Join(cacheDir, id, strings.TrimSpace(version), CachePlatform())
	return ArtifactPaths{
		Lock:   "",
		Binary: filepath.Join(dir, binary),
	}, nil
}

func legacyCachedArtifactPaths(cacheDir, provider, version string) (ArtifactPaths, error) {
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

func cacheVersionDir(cacheDir, provider, version string) (string, error) {
	id := strings.TrimSpace(provider)
	if id == "" {
		return "", errors.New("provider is required")
	}
	if !safePathSegment(version) {
		return "", fmt.Errorf("provider version %q is not a safe path segment", version)
	}
	return filepath.Join(cacheDir, id, strings.TrimSpace(version)), nil
}

func cacheStagingDir(versionDir string) string {
	return filepath.Join(versionDir, cacheStagingPrefix+CachePlatform())
}

type platformCacheState int

const (
	platformCacheAbsent   platformCacheState = 0
	platformCacheComplete platformCacheState = 1
	platformCacheCorrupt  platformCacheState = 2
)

func platformCacheStateFor(binaryPath string) platformCacheState {
	if cachePairComplete(binaryPath) {
		return platformCacheComplete
	}
	dir := filepath.Dir(binaryPath)
	if err := inspectCacheDir(dir, binaryPath); err != nil {
		return platformCacheCorrupt
	}
	if hasStagingDir(filepath.Dir(dir)) {
		return platformCacheCorrupt
	}
	if fileExists(binaryPath) || fileExists(CachedBundlePath(binaryPath)) || hasPartialDownloadTemps(dir) {
		return platformCacheCorrupt
	}
	return platformCacheAbsent
}

func legacyCacheState(binaryPath string) platformCacheState {
	if cachePairComplete(binaryPath) {
		return platformCacheComplete
	}
	dir := filepath.Dir(binaryPath)
	if err := inspectCacheDir(dir, binaryPath); err != nil {
		return platformCacheCorrupt
	}
	if fileExists(binaryPath) || fileExists(CachedBundlePath(binaryPath)) || hasPartialDownloadTemps(dir) {
		return platformCacheCorrupt
	}
	return platformCacheAbsent
}

func locateCachedArtifacts(cacheDir, provider, version string) (ArtifactPaths, error) {
	primary, err := CachedArtifactPaths(cacheDir, provider, version)
	if err != nil {
		return ArtifactPaths{}, err
	}
	switch platformCacheStateFor(primary.Binary) {
	case platformCacheComplete:
		return primary, nil
	case platformCacheCorrupt:
		return ArtifactPaths{}, fmt.Errorf("%w: platform cache for %s is corrupt or incomplete", ErrCacheCorrupt, CachePlatform())
	}
	legacy, err := legacyCachedArtifactPaths(cacheDir, provider, version)
	if err != nil {
		return ArtifactPaths{}, err
	}
	switch legacyCacheState(legacy.Binary) {
	case platformCacheComplete:
		return legacy, nil
	case platformCacheCorrupt:
		return ArtifactPaths{}, fmt.Errorf("%w: legacy provider cache is corrupt or incomplete", ErrCacheCorrupt)
	}
	return primary, ErrCacheMiss
}

func cachePairComplete(binary string) bool {
	return fileExists(binary) && fileExists(CachedBundlePath(binary))
}

// ResolveCached returns the cached binary and bundle for the locked provider
// version. A missing cache entry wraps ErrCacheMiss so callers can tell
// "not installed" from "lockfile broken" (a missing lockfile stays a plain
// open error). Corrupt or partial entries wrap ErrCacheCorrupt.
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
	paths, err := locateCachedArtifacts(cacheDir, provider, artifact.Version)
	if err != nil {
		if errors.Is(err, ErrCacheMiss) {
			return "", "", fmt.Errorf("provider %q version %s: %w", strings.TrimSpace(provider), strings.TrimSpace(artifact.Version), err)
		}
		return "", "", err
	}
	if err := inspectCacheDir(filepath.Dir(paths.Binary), paths.Binary); err != nil {
		return "", "", err
	}
	return paths.Binary, CachedBundlePath(paths.Binary), nil
}

// Downloader fetches provider binaries plus their Cosign bundles into the
// user cache, verifying identity and content before anything is installed.
type Downloader struct {
	HTTP     *http.Client
	Verifier BlobVerifier
	CacheDir string
	// LockDir resolves relative bundle references (the lockfile directory).
	LockDir string
	// Repair removes corrupt or partial cache entries and re-downloads.
	// Default false: corruption fails closed.
	Repair bool
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
	return EffectiveCacheDir(d.CacheDir)
}

// Install verifies the lockfile entry and ensures it exists in the cache.
// A present entry whose digest matches is returned without network use after
// full signature verification. Otherwise the binary and bundle are downloaded
// (both must name remote locations, or the bundle must exist beside the
// lockfile), verified with VerifyLocal, and atomically installed. Nothing is
// executed.
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
	installPaths, err := CachedArtifactPaths(cacheDir, provider, artifact.Version)
	if err != nil {
		return Installed{}, err
	}
	bundle := CachedBundlePath(installPaths.Binary)
	installed := Installed{
		Provider: strings.TrimSpace(provider),
		Version:  strings.TrimSpace(artifact.Version),
		Binary:   installPaths.Binary,
		Bundle:   bundle,
	}
	versionDir, err := cacheVersionDir(cacheDir, provider, artifact.Version)
	if err != nil {
		return Installed{}, err
	}
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return Installed{}, fmt.Errorf("create provider cache directory: %w", err)
	}
	release, err := withCacheInstallLock(ctx, filepath.Join(versionDir, cacheInstallLockName))
	if err != nil {
		return Installed{}, err
	}
	defer release()

	if hit, err := d.tryCachedInstall(ctx, cacheDir, provider, artifact, &installed); err != nil {
		return Installed{}, err
	} else if hit {
		return installed, nil
	}

	binaryURL := strings.TrimSpace(artifact.URL)
	if binaryURL == "" {
		return Installed{}, fmt.Errorf("provider %q ships beside the CLI: %w", installed.Provider, ErrNotDownloadable)
	}
	if err := validDownloadURL(binaryURL); err != nil {
		return Installed{}, err
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
	stagingDir := cacheStagingDir(versionDir)
	if err := os.RemoveAll(stagingDir); err != nil {
		return Installed{}, fmt.Errorf("reset provider cache staging directory: %w", err)
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return Installed{}, fmt.Errorf("create provider cache staging directory: %w", err)
	}
	stagingCommitted := false
	defer func() {
		if !stagingCommitted {
			os.RemoveAll(stagingDir)
		}
	}()

	binaryName := filepath.Base(installPaths.Binary)
	bundleName := filepath.Base(bundle)
	tempBinary, err := writeTempFile(stagingDir, ".download-*-"+binaryName, binaryBody, 0o755)
	if err != nil {
		return Installed{}, err
	}
	defer os.Remove(tempBinary)
	tempBundle, err := writeTempFile(stagingDir, ".download-*-"+bundleName, bundleBody, 0o644)
	if err != nil {
		return Installed{}, err
	}
	defer os.Remove(tempBundle)
	if err := VerifyLocal(ctx, artifact, tempBinary, tempBundle, d.Verifier); err != nil {
		return Installed{}, err
	}
	if err := os.Rename(tempBinary, filepath.Join(stagingDir, binaryName)); err != nil {
		return Installed{}, fmt.Errorf("stage provider binary: %w", err)
	}
	if err := os.Rename(tempBundle, filepath.Join(stagingDir, bundleName)); err != nil {
		return Installed{}, fmt.Errorf("stage provider bundle: %w", err)
	}
	if err := publishStagedPlatformCache(installPaths.Binary, stagingDir); err != nil {
		return Installed{}, err
	}
	stagingCommitted = true
	return installed, nil
}

func publishStagedPlatformCache(binaryPath, stagingDir string) error {
	platformDir := filepath.Dir(binaryPath)
	if err := removeKnownPlatformArtifacts(binaryPath); err != nil {
		return err
	}
	if err := os.Rename(stagingDir, platformDir); err != nil {
		return fmt.Errorf("install provider cache: %w", err)
	}
	return nil
}

func (d *Downloader) tryCachedInstall(ctx context.Context, cacheDir, provider string, artifact Artifact, installed *Installed) (bool, error) {
	paths, err := locateCachedArtifacts(cacheDir, provider, artifact.Version)
	if errors.Is(err, ErrCacheCorrupt) {
		if d.Repair {
			if clearErr := clearProviderCacheDirs(cacheDir, provider, artifact.Version); clearErr != nil {
				return false, clearErr
			}
			return false, nil
		}
		return false, err
	}
	if errors.Is(err, ErrCacheMiss) {
		if err := inspectVersionCacheDirs(cacheDir, provider, artifact.Version); err != nil {
			if d.Repair {
				if clearErr := clearProviderCacheDirs(cacheDir, provider, artifact.Version); clearErr != nil {
					return false, clearErr
				}
				return false, nil
			}
			return false, err
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := inspectCacheDir(filepath.Dir(paths.Binary), paths.Binary); err != nil {
		if d.Repair {
			if clearErr := clearProviderCacheDirs(cacheDir, provider, artifact.Version); clearErr != nil {
				return false, clearErr
			}
			return false, nil
		}
		return false, err
	}
	if !digestMatches(paths.Binary, artifact.Digest) {
		if d.Repair {
			if clearErr := clearProviderCacheDirs(cacheDir, provider, artifact.Version); clearErr != nil {
				return false, clearErr
			}
			return false, nil
		}
		return false, fmt.Errorf("%w: cached provider digest does not match the lockfile", ErrChecksumMismatch)
	}
	bundlePath := CachedBundlePath(paths.Binary)
	if err := VerifyLocal(ctx, artifact, paths.Binary, bundlePath, d.Verifier); err != nil {
		if d.Repair {
			if clearErr := clearProviderCacheDirs(cacheDir, provider, artifact.Version); clearErr != nil {
				return false, clearErr
			}
			return false, nil
		}
		return false, fmt.Errorf("%w: %v", ErrCacheCorrupt, err)
	}
	installed.Binary = paths.Binary
	installed.Bundle = bundlePath
	installed.Cached = true
	return true, nil
}

func inspectVersionCacheDirs(cacheDir, provider, version string) error {
	primary, err := CachedArtifactPaths(cacheDir, provider, version)
	if err != nil {
		return err
	}
	switch platformCacheStateFor(primary.Binary) {
	case platformCacheCorrupt:
		return fmt.Errorf("%w: platform cache for %s is corrupt or incomplete", ErrCacheCorrupt, CachePlatform())
	case platformCacheComplete:
		return nil
	}
	legacy, err := legacyCachedArtifactPaths(cacheDir, provider, version)
	if err != nil {
		return err
	}
	switch legacyCacheState(legacy.Binary) {
	case platformCacheCorrupt:
		return fmt.Errorf("%w: legacy provider cache is corrupt or incomplete", ErrCacheCorrupt)
	}
	return nil
}

func inspectCacheDir(dir, binaryPath string) error {
	if hasPartialDownloadTemps(dir) {
		return fmt.Errorf("%w: incomplete provider download in cache", ErrCacheCorrupt)
	}
	binaryExists := fileExists(binaryPath)
	bundleExists := fileExists(CachedBundlePath(binaryPath))
	if binaryExists != bundleExists {
		return fmt.Errorf("%w: provider binary and bundle are not both present", ErrCacheCorrupt)
	}
	return nil
}

func hasPartialDownloadTemps(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".download-") {
			return true
		}
	}
	return false
}

func clearProviderCacheDirs(cacheDir, provider, version string) error {
	versionDir, err := cacheVersionDir(cacheDir, provider, version)
	if err != nil {
		return err
	}
	primary, err := CachedArtifactPaths(cacheDir, provider, version)
	if err != nil {
		return err
	}
	if err := removeKnownPlatformArtifacts(primary.Binary); err != nil {
		return err
	}
	if err := removeStagingDirs(versionDir); err != nil {
		return err
	}
	legacy, err := legacyCachedArtifactPaths(cacheDir, provider, version)
	if err != nil {
		return err
	}
	return removeKnownLegacyArtifacts(versionDir, legacy.Binary)
}

func removeKnownPlatformArtifacts(binaryPath string) error {
	platformDir := filepath.Dir(binaryPath)
	binaryName := filepath.Base(binaryPath)
	bundleName := filepath.Base(CachedBundlePath(binaryPath))
	entries, err := os.ReadDir(platformDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == binaryName || name == bundleName || strings.HasPrefix(name, ".download-") {
			if err := os.Remove(filepath.Join(platformDir, name)); err != nil {
				return err
			}
		}
	}
	return removeEmptyDir(platformDir)
}

func removeKnownLegacyArtifacts(versionDir, legacyBinary string) error {
	binaryName := filepath.Base(legacyBinary)
	bundleName := filepath.Base(CachedBundlePath(legacyBinary))
	entries, err := os.ReadDir(versionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == cacheInstallLockName {
			continue
		}
		if name == binaryName || name == bundleName || strings.HasPrefix(name, ".download-") {
			if err := os.Remove(filepath.Join(versionDir, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeStagingDirs(versionDir string) error {
	entries, err := os.ReadDir(versionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), cacheStagingPrefix) {
			if err := os.RemoveAll(filepath.Join(versionDir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasStagingDir(versionDir string) bool {
	entries, err := os.ReadDir(versionDir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), cacheStagingPrefix) {
			return true
		}
	}
	return false
}

func removeEmptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(entries) > 0 {
		return nil
	}
	return os.Remove(dir)
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
