package buildcontext

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed LICENSE NOTICE
//go:embed images/php-nginx
//go:embed build/composer.json build/composer.lock
//go:embed build/src
//go:embed build/bin
var embedded embed.FS

const alphaPlatform = "linux/amd64"

var forbiddenEmbedPath = []string{"/vendor/", "/tests/", "/auth/", "/.git/"}

// WriteContext copies the bundled recipe alongside a private prepared rootfs.
// Existing recipe paths are refused so the fingerprint always names our bytes.
func WriteContext(destination string) error {
	paths, err := listEmbeddedFiles()
	if err != nil {
		return err
	}
	tops := map[string]bool{}
	for _, path := range paths {
		top, _, _ := strings.Cut(path, "/")
		tops[top] = true
	}
	for top := range tops {
		if _, err := os.Lstat(filepath.Join(destination, top)); !errors.Is(err, os.ErrNotExist) {
			if err == nil {
				return fmt.Errorf("recipe path already exists: %s", top)
			}
			return err
		}
	}
	return os.CopyFS(destination, embedded)
}

// MaterializePrivateContext writes the embedded Magento image build context to a
// private directory under tempRoot and returns a content fingerprint.
func MaterializePrivateContext(tempRoot, platform string) (contextDir string, fingerprint string, cleanup func(), err error) {
	if err := validatePlatform(platform); err != nil {
		return "", "", nil, err
	}
	fingerprint, err = ContentFingerprint(platform)
	if err != nil {
		return "", "", nil, err
	}
	root := tempRoot
	if root == "" {
		root = os.TempDir()
	}
	contextDir, err = os.MkdirTemp(root, "magelift-buildcontext-")
	if err != nil {
		return "", "", nil, fmt.Errorf("create private build context directory: %w", err)
	}
	cleanup = func() { os.RemoveAll(contextDir) }
	if err := writeEmbeddedTree(contextDir); err != nil {
		cleanup()
		return "", "", nil, err
	}
	return contextDir, fingerprint, cleanup, nil
}

// ContentFingerprint returns a deterministic Docker-tag-safe fingerprint for the
// embedded build context and target platform.
func ContentFingerprint(platform string) (string, error) {
	if err := validatePlatform(platform); err != nil {
		return "", err
	}
	paths, err := listEmbeddedFiles()
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	hasher.Write([]byte(platform))
	hasher.Write([]byte{0})
	for _, name := range paths {
		contents, err := fs.ReadFile(embedded, name)
		if err != nil {
			return "", fmt.Errorf("read embedded file %q: %w", name, err)
		}
		hasher.Write([]byte(name))
		hasher.Write([]byte{0})
		hasher.Write(contents)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// EmbeddedPaths returns the relative paths shipped in the embedded context.
func EmbeddedPaths() ([]string, error) {
	return listEmbeddedFiles()
}

func validatePlatform(platform string) error {
	if platform != alphaPlatform {
		return fmt.Errorf("unsupported platform %q: community alpha builds %s only", platform, alphaPlatform)
	}
	return nil
}

func listEmbeddedFiles() ([]string, error) {
	paths := make([]string, 0)
	err := walkEmbedded("", func(path string, info fs.DirEntry) error {
		if info.IsDir() {
			return nil
		}
		if err := rejectForbiddenEmbedPath(path); err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, errors.New("embedded build context is empty")
	}
	return paths, nil
}

func rejectForbiddenEmbedPath(path string) error {
	base := filepath.Base(path)
	if base == "auth.json" || base == ".env" || strings.HasPrefix(base, ".env.") {
		return fmt.Errorf("embedded secret path %q is not allowed", path)
	}
	normalized := "/" + strings.ReplaceAll(filepath.ToSlash(path), "\\", "/") + "/"
	for _, fragment := range forbiddenEmbedPath {
		if strings.Contains(normalized, fragment) {
			return fmt.Errorf("embedded path %q is not allowed", path)
		}
	}
	return nil
}

func walkEmbedded(dir string, visit func(path string, info fs.DirEntry) error) error {
	readDir := dir
	if readDir == "" {
		readDir = "."
	}
	entries, err := fs.ReadDir(embedded, readDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := entry.Name()
		if dir != "" {
			path = dir + "/" + entry.Name()
		}
		if err := visit(path, entry); err != nil {
			return err
		}
		if entry.IsDir() {
			if err := walkEmbedded(path, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeEmbeddedTree(contextDir string) error {
	return walkEmbedded("", func(path string, info fs.DirEntry) error {
		target := filepath.Join(contextDir, filepath.FromSlash(path))
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := rejectForbiddenEmbedPath(path); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		source, err := embedded.Open(path)
		if err != nil {
			return fmt.Errorf("open embedded file %q: %w", path, err)
		}
		defer source.Close()
		destination, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("create %q: %w", target, err)
		}
		if _, err := io.Copy(destination, source); err != nil {
			destination.Close()
			return fmt.Errorf("write %q: %w", target, err)
		}
		return destination.Close()
	})
}
