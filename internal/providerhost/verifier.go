package providerhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// VerifierVersion pins the Cosign release bootstrapped into the
	// cache. ROTATION: keep in sync with COSIGN_VERSION plus COSIGN_PIN
	// in website/public/install.sh (same release, same checksums).
	VerifierVersion = "v3.1.4"
	verifierBaseURL = "https://github.com/sigstore/cosign/releases/download"
	// Cosign v3.1.4 linux/amd64 is 145688404 bytes; windows/amd64 is
	// 191066624. 128MiB rejected every pinned platform.
	maxVerifierBytes = 256 << 20
	verifierTimeout  = 5 * time.Minute
)

var verifierPins = map[string]string{
	"linux/amd64":   "424e2c97d0b45fd7a6b608a87f090f35633f4c23ece5f092d48bba11aaf4bbfa",
	"linux/arm64":   "ade83b7aac62e131cacfe96d958948b2150cc9efec46a570a2649b8e27dedad2",
	"darwin/amd64":  "9d1e4abe2b6be279ed0d2ff706fde1ccd738747c834e3e6d44039c238867334f",
	"darwin/arm64":  "9f9b8cd557cf2e5b3df3373a00baa005a86b02391e3e06dd0ce37b3a1533d3a4",
	"windows/amd64": "cbe953787074611f1334cbf37dc1cf953995bb8c45398eb4fc7bb781d29c1f2a",
	// No windows/arm64 Cosign build is published; that platform fails
	// closed with "no verifier pin" until Sigstore ships one.
}

// VerifierInstaller persists the pinned Cosign bootstrap into the
// provider cache so runtime verification needs nothing preinstalled.
type VerifierInstaller struct {
	HTTP     *http.Client
	BaseURL  string
	CacheDir string
	// Repair permits replacement of a corrupt verifier only during explicit installation.
	Repair bool
}

// VerifierPath returns the cache location of the pinned verifier.
func VerifierPath(cacheDir string) (string, error) {
	if strings.TrimSpace(cacheDir) == "" {
		var err error
		cacheDir, err = DefaultCacheDir()
		if err != nil {
			return "", err
		}
	}
	key := runtime.GOOS + "/" + runtime.GOARCH
	if _, ok := verifierPins[key]; !ok {
		return "", fmt.Errorf("no verifier pin for platform %s", key)
	}
	name := "cosign-" + VerifierVersion
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(cacheDir, "_verifier", runtime.GOOS+"_"+runtime.GOARCH, name), nil
}

// Ensure downloads and checksum-verifies the pinned verifier unless a
// valid copy is already cached. Every failure fails closed.
func (v *VerifierInstaller) Ensure(ctx context.Context) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cacheDir := v.CacheDir
	if strings.TrimSpace(cacheDir) == "" {
		var err error
		cacheDir, err = DefaultCacheDir()
		if err != nil {
			return "", err
		}
	}
	path, err := VerifierPath(cacheDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create verifier directory: %w", err)
	}
	release, err := withCacheInstallLock(ctx, path+".lock")
	if err != nil {
		return "", err
	}
	defer release()
	want := verifierPins[runtime.GOOS+"/"+runtime.GOARCH]
	if digestMatches(path, "sha256:"+want) {
		if _, err := v.ensureTrustedRoot(ctx, cacheDir); err != nil {
			return "", err
		}
		return path, nil
	}
	if _, err := os.Lstat(path); err == nil && !v.Repair {
		return "", fmt.Errorf("%w: cached verifier is corrupt; run `magelift providers install` to repair it", ErrChecksumMismatch)
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect cached verifier: %w", err)
	}
	legacy := filepath.Join(filepath.Dir(filepath.Dir(path)), filepath.Base(path))
	if digestMatches(legacy, "sha256:"+want) {
		if _, err := v.ensureTrustedRoot(ctx, cacheDir); err != nil {
			return "", err
		}
		return legacy, nil
	}
	base := strings.TrimSpace(v.BaseURL)
	if base == "" {
		base = verifierBaseURL
	}
	file := "cosign-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		file += ".exe"
	}
	endpoint := strings.TrimRight(base, "/") + "/" + VerifierVersion + "/" + file
	if !isRemoteReference(endpoint) {
		return "", fmt.Errorf("verifier base URL must be HTTPS: %s", base)
	}
	body, err := v.download(ctx, endpoint)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != want {
		return "", fmt.Errorf("%w: verifier bootstrap checksum mismatch", ErrChecksumMismatch)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create verifier directory: %w", err)
	}
	temporary, err := writeTempFile(filepath.Dir(path), ".verifier-*", body, 0o755)
	if err != nil {
		return "", err
	}
	defer os.Remove(temporary)
	if err := os.Rename(temporary, path); err != nil {
		return "", fmt.Errorf("install verifier: %w", err)
	}
	if _, err := v.ensureTrustedRoot(ctx, cacheDir); err != nil {
		return "", err
	}
	return path, nil
}

func (v *VerifierInstaller) client() *http.Client {
	if v.HTTP != nil {
		return v.HTTP
	}
	return &http.Client{Timeout: verifierTimeout}
}

func (v *VerifierInstaller) download(ctx context.Context, endpoint string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create verifier request: %w", err)
	}
	request.Header.Set("User-Agent", "magelift-verifier-bootstrap")
	response, err := v.client().Do(request)
	if err != nil {
		return nil, fmt.Errorf("download verifier: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download verifier: HTTP %s", response.Status)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxVerifierBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read verifier: %w", err)
	}
	if int64(len(body)) > maxVerifierBytes {
		return nil, fmt.Errorf("verifier exceeds %d bytes", maxVerifierBytes)
	}
	return body, nil
}

// errVerifierUnavailable is returned when no cached verifier exists.
var errVerifierUnavailable = errors.New("no cached verifier")

// CachedVerifierPath returns the cached verifier path when a valid copy
// exists, or an error otherwise. It performs no downloads.
func CachedVerifierPath(cacheDir string) (string, error) {
	path, err := VerifierPath(cacheDir)
	if err != nil {
		return "", err
	}
	want, ok := verifierPins[runtime.GOOS+"/"+runtime.GOARCH]
	if ok && digestMatches(path, "sha256:"+want) {
		if trustedRootCorrupt(cacheDir) {
			return "", errVerifierUnavailable
		}
		return path, nil
	}
	// Legacy entries are usable only when their digest matches this platform.
	// Never fall back from a present, damaged platform-specific verifier.
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return "", errVerifierUnavailable
	}
	legacy := filepath.Join(filepath.Dir(filepath.Dir(path)), filepath.Base(path))
	if ok && digestMatches(legacy, "sha256:"+want) {
		if trustedRootCorrupt(cacheDir) {
			return "", errVerifierUnavailable
		}
		return legacy, nil
	}
	return "", errVerifierUnavailable
}
