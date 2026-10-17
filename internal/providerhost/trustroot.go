package providerhost

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// embeddedTrustedRootDigest pins the public Sigstore trusted_root.json
// embedded in the binary. ROTATION: replace assets/trusted_root.json only
// after cosign initialize (or equivalent) against the public TUF root and
// update this digest to match.
const embeddedTrustedRootDigest = "6494e21ea73fa7ee769f85f57d5a3e6a08725eae1e38c755fc3517c9e6bc0b66"

const trustedRootFileName = "trusted_root.json"

//go:embed assets/trusted_root.json
var embeddedTrustedRoot []byte

var errTrustedRootUnavailable = errors.New("no cached sigstore trusted root")

func init() {
	if err := verifyDigestBytes(embeddedTrustedRoot, "sha256:"+embeddedTrustedRootDigest); err != nil {
		panic("embedded sigstore trusted root digest mismatch: " + err.Error())
	}
}

// TrustedRootPath returns the cache location for the pinned Sigstore root.
func TrustedRootPath(cacheDir string) (string, error) {
	if strings.TrimSpace(cacheDir) == "" {
		var err error
		cacheDir, err = DefaultCacheDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(cacheDir, "_verifier", "sigstore", "trusted_root", embeddedTrustedRootDigest, trustedRootFileName), nil
}

func trustedRootCorrupt(cacheDir string) bool {
	path, err := TrustedRootPath(cacheDir)
	if err != nil {
		return false
	}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return false
	}
	return !digestMatches(path, "sha256:"+embeddedTrustedRootDigest)
}

// CachedTrustedRootPath returns the materialized trusted root when valid.
func CachedTrustedRootPath(cacheDir string) (string, error) {
	path, err := TrustedRootPath(cacheDir)
	if err != nil {
		return "", err
	}
	if digestMatches(path, "sha256:"+embeddedTrustedRootDigest) {
		return path, nil
	}
	if _, err := os.Lstat(path); err == nil {
		return "", fmt.Errorf("%w: cached sigstore trusted root is corrupt; run `magelift providers install` to repair it", ErrChecksumMismatch)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect cached sigstore trusted root: %w", err)
	}
	return "", errTrustedRootUnavailable
}

func (v *VerifierInstaller) ensureTrustedRoot(ctx context.Context, cacheDir string) (string, error) {
	path, err := TrustedRootPath(cacheDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create trusted root directory: %w", err)
	}
	release, err := withCacheInstallLock(ctx, path+".lock")
	if err != nil {
		return "", err
	}
	defer release()
	if digestMatches(path, "sha256:"+embeddedTrustedRootDigest) {
		return path, nil
	}
	if _, err := os.Lstat(path); err == nil && !v.Repair {
		return "", fmt.Errorf("%w: cached sigstore trusted root is corrupt; run `magelift providers install` to repair it", ErrChecksumMismatch)
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect cached sigstore trusted root: %w", err)
	}
	temporary, err := writeTempFile(filepath.Dir(path), ".trusted-root-*", embeddedTrustedRoot, 0o644)
	if err != nil {
		return "", err
	}
	defer os.Remove(temporary)
	if err := os.Rename(temporary, path); err != nil {
		return "", fmt.Errorf("install sigstore trusted root: %w", err)
	}
	return path, nil
}
