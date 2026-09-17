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
	VerifierVersion  = "v3.1.3"
	verifierBaseURL  = "https://github.com/sigstore/cosign/releases/download"
	maxVerifierBytes = 128 << 20
	verifierTimeout  = 5 * time.Minute
)

var verifierPins = map[string]string{
	"linux/amd64":   "4629c757b7618056f8ddd7e2625ae9fdd94c0372a65049520bc7d9df9efc7f71",
	"linux/arm64":   "c5d324e091826b0d7a78eb16fef316450b4eb9aaec045611c08ba06f5e73220a",
	"darwin/amd64":  "2347488e5d5b25336644024dfeca5601b190e91197a71a917bda44744aff106c",
	"darwin/arm64":  "5cf948c2f4dfe59687bdd0b8523709067383e03982cc543475c8a7dc70e92a76",
	"windows/amd64": "9fe59be0eca1271873ce019061335eb1ac419b7059202e797828467ddabe33be",
	// No windows/arm64 Cosign build is published; that platform fails
	// closed with "no verifier pin" until Sigstore ships one.
}

// VerifierInstaller persists the pinned Cosign bootstrap into the
// provider cache so runtime verification needs nothing preinstalled.
type VerifierInstaller struct {
	HTTP     *http.Client
	BaseURL  string
	CacheDir string
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
	return filepath.Join(cacheDir, "_verifier", name), nil
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
	want := verifierPins[runtime.GOOS+"/"+runtime.GOARCH]
	if digestMatches(path, "sha256:"+want) {
		return path, nil
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
	if !ok || !digestMatches(path, "sha256:"+want) {
		return "", errVerifierUnavailable
	}
	return path, nil
}
