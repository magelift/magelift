package providerhost

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoLockfile marks lock auto-discovery that found no lockfile
// anywhere. Callers bootstrap only on this error: an existing lock
// that fails to parse or verify must fail closed, never be replaced.
var ErrNoLockfile = errors.New("no magelift.providers.lock in the project or beside the CLI")

// ResolveOptions selects how a provider artifact is located. All
// production loaders (registry, backend, hooks) resolve through this one
// function so installation and execution never disagree.
type ResolveOptions struct {
	// ExecutablePath anchors beside-CLI discovery. Empty resolves
	// against the working directory.
	ExecutablePath string
	// LockPath forces one lockfile. Empty auto-discovers:
	// ./magelift.providers.lock, then beside the CLI.
	LockPath string
	// CacheDir overrides the user cache. Empty honors
	// MAGELIFT_PROVIDER_CACHE_DIR, else the default.
	CacheDir string
}

// Resolved is one located provider ready for Load.
type Resolved struct {
	// LockPath is the lockfile actually used. Project locks control the
	// version run; beside-CLI locks are the release default.
	LockPath string
	Lock     Lockfile
	Artifact Artifact
	// Binary is beside the CLI (dev/release bundles, verified with the
	// beside-CLI lock) or in the versioned cache (project locks).
	Binary string
	// Bundle forces the bundle path. Empty resolves beside the lockfile.
	Bundle string
	// Cached reports a cache resolution.
	Cached bool
}

// Resolve locates the lockfile and binary for a provider. Binary order
// is beside-CLI, then versioned cache: `providers install` downloads
// into the cache, so a beside-CLI lock with no beside-CLI binary still
// loads the installed artifact. A missing binary names providers
// install; verification failures stay loud at Load time.
func Resolve(provider string, opts ResolveOptions) (Resolved, error) {
	id := strings.TrimSpace(provider)
	if id == "" {
		return Resolved{}, errors.New("provider is required")
	}
	lockPath, lock, besideCLI, err := ResolveLock(opts)
	if err != nil {
		return Resolved{}, err
	}
	artifact, err := lock.Artifact(id)
	if err != nil {
		return Resolved{}, err
	}
	cacheDir, err := EffectiveCacheDir(opts.CacheDir)
	if err != nil {
		return Resolved{}, err
	}
	if besideCLI {
		paths := DiscoverArtifactPaths(opts.ExecutablePath, id)
		if fileExists(paths.Binary) {
			return Resolved{LockPath: lockPath, Lock: lock, Artifact: artifact, Binary: paths.Binary}, nil
		}
		if binary, bundle, err := ResolveCached(lockPath, id, cacheDir); err == nil {
			return Resolved{LockPath: lockPath, Lock: lock, Artifact: artifact, Binary: binary, Bundle: bundle, Cached: true}, nil
		} else if !errors.Is(err, ErrCacheMiss) {
			return Resolved{}, fmt.Errorf("cached provider %q is unusable: %w (run `magelift providers install` to repair it)", id, err)
		}
		return Resolved{}, fmt.Errorf("provider %q is not installed beside the CLI or in the cache: run `magelift providers install` to download the locked version", id)
	}
	binary, bundle, err := ResolveCached(lockPath, id, cacheDir)
	if err != nil {
		if errors.Is(err, ErrCacheMiss) {
			return Resolved{}, fmt.Errorf("provider %q version %s is not in the cache: run `magelift providers install` to download it", id, strings.TrimSpace(artifact.Version))
		}
		return Resolved{}, err
	}
	return Resolved{LockPath: lockPath, Lock: lock, Artifact: artifact, Binary: binary, Bundle: bundle, Cached: true}, nil
}

// ResolveLock locates and parses the lockfile without resolving a binary.
// Callers that install (rather than load) use it directly. Absence wraps
// ErrNoLockfile; any existing lock that cannot be read, parsed, or
// trusted returns its own error so callers fail closed.
func ResolveLock(opts ResolveOptions) (string, Lockfile, bool, error) {
	if trimmed := strings.TrimSpace(opts.LockPath); trimmed != "" {
		lock, err := openLock(trimmed)
		if err != nil {
			return "", Lockfile{}, false, err
		}
		return trimmed, lock, false, nil
	}
	if _, err := os.Stat("magelift.providers.lock"); err == nil {
		lock, err := openLock("magelift.providers.lock")
		if err != nil {
			return "", Lockfile{}, false, err
		}
		return "magelift.providers.lock", lock, false, nil
	} else if !os.IsNotExist(err) {
		return "", Lockfile{}, false, fmt.Errorf("stat magelift.providers.lock: %w", err)
	}
	dir := "."
	if strings.TrimSpace(opts.ExecutablePath) != "" {
		dir = filepath.Dir(opts.ExecutablePath)
	}
	beside := filepath.Join(dir, "magelift.providers.lock")
	if _, err := os.Stat(beside); err != nil {
		if !os.IsNotExist(err) {
			return "", Lockfile{}, false, fmt.Errorf("stat %s: %w", beside, err)
		}
		return "", Lockfile{}, false, fmt.Errorf("%w: run `magelift providers install` to bootstrap one", ErrNoLockfile)
	}
	lock, err := openLock(beside)
	if err != nil {
		return "", Lockfile{}, false, err
	}
	return beside, lock, true, nil
}

func openLock(path string) (Lockfile, error) {
	file, err := os.Open(path)
	if err != nil {
		return Lockfile{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	lock, err := ParseLock(file)
	if err != nil {
		return Lockfile{}, err
	}
	return lock, nil
}
