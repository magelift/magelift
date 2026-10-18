package providerhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// AcquireOptions configures lazy first-party provider acquisition.
type AcquireOptions struct {
	Resolve ResolveOptions
	// ReleaseBaseURL overrides FetchLock's download base (tests only).
	ReleaseBaseURL string
	HTTP           *http.Client
	// FetchLock bootstraps the release lock when no project or beside-CLI lock exists.
	FetchLock func(ctx context.Context, version string) (Lockfile, error)
	// EnsureVerifier installs or returns the cached Cosign bootstrap.
	EnsureVerifier func(ctx context.Context, cacheDir string) (string, error)
	Verifier       BlobVerifier
	// Progress receives acquisition messages, separate from structured stdout.
	Progress io.Writer
}

// Acquire resolves, downloads when needed, verifies, and loads a first-party
// provider subprocess. Only GCP is acquired automatically; other providers must
// be installed explicitly. Project locks win, then beside-CLI locks, then the
// CLI release version's fetched lock (cached under the user cache, never
// written into the project). There is no latest or dev fallback.
func Acquire(ctx context.Context, provider, cliVersion string, opts AcquireOptions) (Loaded, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	id := strings.TrimSpace(provider)
	if id != "gcp" {
		return Loaded{}, fmt.Errorf("lazy provider acquisition supports only gcp, got %q", provider)
	}
	cacheDir, err := EffectiveCacheDir(opts.Resolve.CacheDir)
	if err != nil {
		return Loaded{}, err
	}
	resolveOpts := opts.Resolve
	lockPath, lock, _, err := ResolveLock(resolveOpts)
	if err != nil {
		if !errors.Is(err, ErrNoLockfile) {
			return Loaded{}, err
		}
		tag, err := NormalizeReleaseTag(cliVersion)
		if err != nil {
			return Loaded{}, fmt.Errorf("no magelift.providers.lock and CLI version %q is not a release tag: pass an explicit project lock or install providers beside the CLI", cliVersion)
		}
		lock, lockPath, err = ensureReleaseLock(ctx, cacheDir, tag, opts)
		if err != nil {
			return Loaded{}, err
		}
		if err := validateBootstrapIdentity(lock, id, tag); err != nil {
			return Loaded{}, err
		}
		resolveOpts.LockPath = lockPath
	}
	if _, err := lock.Artifact(id); err != nil {
		return Loaded{}, err
	}
	verifier, err := acquireVerifier(ctx, cacheDir, opts)
	if err != nil {
		return Loaded{}, err
	}
	resolved, resolveErr := Resolve(id, resolveOpts)
	if resolveErr != nil && shouldLazyDownload(resolveErr, lock, id) {
		artifact, err := lock.Artifact(id)
		if err != nil {
			return Loaded{}, err
		}
		if opts.Progress != nil {
			fmt.Fprintf(opts.Progress, "MageLift: acquiring GCP provider %s for %s\n", artifact.Version, CachePlatform())
		}
		if _, err := (&Downloader{
			HTTP:     opts.HTTP,
			Verifier: verifier,
			CacheDir: cacheDir,
			LockDir:  filepath.Dir(lockPath),
			Repair:   false,
		}).Install(ctx, id, artifact); err != nil {
			return Loaded{}, err
		}
		resolved, resolveErr = Resolve(id, resolveOpts)
	}
	if resolveErr != nil {
		return Loaded{}, resolveErr
	}
	bundle := resolved.Bundle
	if strings.TrimSpace(bundle) == "" && strings.TrimSpace(resolved.Artifact.Cosign.Bundle) != "" {
		bundle = filepath.Join(filepath.Dir(resolved.LockPath), resolved.Artifact.Cosign.Bundle)
	}
	return Load(ctx, Options{
		Mode:       ModeSubprocess,
		Provider:   id,
		LockPath:   resolved.LockPath,
		BinaryPath: resolved.Binary,
		BundlePath: bundle,
		Verifier:   verifier,
	})
}

func acquireVerifier(ctx context.Context, cacheDir string, opts AcquireOptions) (BlobVerifier, error) {
	if opts.Verifier != nil {
		return opts.Verifier, nil
	}
	ensure := opts.EnsureVerifier
	if ensure == nil {
		ensure = func(ctx context.Context, dir string) (string, error) {
			return (&VerifierInstaller{HTTP: opts.HTTP, CacheDir: dir, Repair: false}).Ensure(ctx)
		}
	}
	if _, err := ensure(ctx, cacheDir); err != nil {
		return nil, err
	}
	return NewPreferredVerifier(cacheDir), nil
}

func ensureReleaseLock(ctx context.Context, cacheDir, tag string, opts AcquireOptions) (Lockfile, string, error) {
	path, err := cachedReleaseLockPath(cacheDir, tag)
	if err != nil {
		return Lockfile{}, "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Lockfile{}, "", fmt.Errorf("create release lock cache directory: %w", err)
	}
	release, err := withCacheInstallLock(ctx, path+".lock")
	if err != nil {
		return Lockfile{}, "", err
	}
	defer release()
	if _, err := os.Lstat(path); err == nil {
		lock, err := readReleaseLockFile(path)
		if err != nil {
			return Lockfile{}, "", fmt.Errorf("cached release lock %s: %w", path, err)
		}
		return lock, path, nil
	} else if !os.IsNotExist(err) {
		return Lockfile{}, "", fmt.Errorf("inspect cached release lock: %w", err)
	}
	fetch := opts.FetchLock
	if fetch == nil {
		fetch = func(ctx context.Context, version string) (Lockfile, error) {
			return FetchLock(ctx, version, opts.ReleaseBaseURL, opts.HTTP)
		}
	}
	lock, err := fetch(ctx, tag)
	if err != nil {
		return Lockfile{}, "", err
	}
	if err := validateBootstrapIdentity(lock, "gcp", tag); err != nil {
		return Lockfile{}, "", err
	}
	if err := publishReleaseLock(path, lock); err != nil {
		return Lockfile{}, "", err
	}
	return lock, path, nil
}

func cachedReleaseLockPath(cacheDir, tag string) (string, error) {
	trimmed := strings.TrimSpace(tag)
	if !ValidReleaseTag(trimmed) {
		return "", fmt.Errorf("release tag %q is not safe to cache", tag)
	}
	return filepath.Join(cacheDir, "_release-locks", trimmed, "magelift.providers.lock."+CachePlatform()), nil
}

func readReleaseLockFile(path string) (Lockfile, error) {
	file, err := os.Open(path)
	if err != nil {
		return Lockfile{}, fmt.Errorf("open: %w", err)
	}
	defer file.Close()
	lock, err := ParseLock(file)
	if err != nil {
		return Lockfile{}, err
	}
	return lock, nil
}

func publishReleaseLock(path string, lock Lockfile) error {
	encoded, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("encode release lock: %w", err)
	}
	encoded = append(encoded, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create release lock cache directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".staging-release-lock-*")
	if err != nil {
		return fmt.Errorf("stage release lock: %w", err)
	}
	staged := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(staged)
		}
	}()
	if _, err := temp.Write(encoded); err != nil {
		temp.Close()
		return fmt.Errorf("write release lock: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync release lock: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close release lock: %w", err)
	}
	if err := os.Rename(staged, path); err != nil {
		return fmt.Errorf("publish release lock: %w", err)
	}
	cleanup = false
	return nil
}

func validateBootstrapIdentity(lock Lockfile, provider, tag string) error {
	artifact, err := lock.Artifact(provider)
	if err != nil {
		return err
	}
	wantSuffix := "@refs/tags/" + strings.TrimSpace(tag)
	identity := strings.TrimSpace(artifact.Cosign.Identity)
	if !strings.HasSuffix(identity, wantSuffix) {
		return fmt.Errorf("%w: bootstrap lock identity %q does not match release tag %s", ErrUnsigned, identity, tag)
	}
	return nil
}

func shouldLazyDownload(resolveErr error, lock Lockfile, provider string) bool {
	if resolveErr == nil {
		return false
	}
	artifact, err := lock.Artifact(provider)
	if err != nil || strings.TrimSpace(artifact.URL) == "" {
		return false
	}
	return errors.Is(resolveErr, ErrCacheMiss) && !errors.Is(resolveErr, ErrCacheCorrupt)
}
