package providerhost

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
	// CacheDir overrides the user cache. Empty uses the default.
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

// Resolve locates the lockfile and binary for a provider. Beside-CLI
// locks load beside-CLI binaries (shipped or dev bundles); project and
// explicit locks load the locked version from the cache. A missing
// binary names providers install; verification failures stay loud at
// Load time.
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
	if besideCLI {
		paths := DiscoverArtifactPaths(opts.ExecutablePath, id)
		if !fileExists(paths.Binary) {
			return Resolved{}, fmt.Errorf("provider %q is not installed beside the CLI: run `magelift providers install` to download the locked version", id)
		}
		return Resolved{LockPath: lockPath, Lock: lock, Artifact: artifact, Binary: paths.Binary}, nil
	}
	cacheDir := opts.CacheDir
	if strings.TrimSpace(cacheDir) == "" {
		cacheDir, err = DefaultCacheDir()
		if err != nil {
			return Resolved{}, err
		}
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
// Callers that install (rather than load) use it directly.
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
	}
	dir := "."
	if strings.TrimSpace(opts.ExecutablePath) != "" {
		dir = filepath.Dir(opts.ExecutablePath)
	}
	beside := filepath.Join(dir, "magelift.providers.lock")
	lock, err := openLock(beside)
	if err != nil {
		return "", Lockfile{}, false, fmt.Errorf("no magelift.providers.lock in the project or beside the CLI: run `magelift providers install` to bootstrap one: %v", err)
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
