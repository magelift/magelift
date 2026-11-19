package buildcontext

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	buildrunner "github.com/magelift/magelift/internal/build/runner"
)

//go:embed LICENSE NOTICE
//go:embed images/php-nginx
//go:embed build/composer.json build/composer.lock
//go:embed build/src
//go:embed build/bin
var embedded embed.FS

//go:embed docker-bake.hcl
var bakeDefinition string

const alphaPlatform = "linux/amd64"

var forbiddenEmbedPath = []string{"/vendor/", "/tests/", "/auth/", "/.git/"}
var phpVersionPattern = regexp.MustCompile(`^(8\.[345])(?:\.[0-9]+)?$`)
var bakePHPBasePattern = regexp.MustCompile(`\{\s*branch\s*=\s*"(8\.[345])",\s*base\s*=\s*"(docker\.io/library/php:[^"\s]+@sha256:[a-f0-9]{64})"\s*\}`)
var recipeComposerBaseLine = regexp.MustCompile(`(?m)^ARG COMPOSER_BASE=[^\n]+$`)
var recipePHPBaseLine = regexp.MustCompile(`(?m)^ARG PHP_BASE=docker\.io/library/php:[^\n]+$`)
var recipePHPBranchLine = regexp.MustCompile(`(?m)^ARG PHP_BRANCH=8\.5$`)

// ComposerRecipe returns the exact pinned source copied into the bundled builder.
func ComposerRecipe(override string) (string, error) {
	if override != "" {
		if err := buildrunner.ValidatePinnedRepositoryDigest("Composer image", override); err != nil {
			return "", err
		}
		return override, nil
	}
	data, err := fs.ReadFile(embedded, "images/php-nginx/Dockerfile")
	if err != nil {
		return "", err
	}
	lines := recipeComposerBaseLine.FindAllString(string(data), -1)
	if len(lines) != 1 {
		return "", errors.New("bundled PHP recipe has an unexpected Composer base declaration")
	}
	image := strings.TrimPrefix(lines[0], "ARG COMPOSER_BASE=")
	if err := buildrunner.ValidatePinnedRepositoryDigest("Composer image", image); err != nil {
		return "", err
	}
	return image, nil
}

// PHPRecipe resolves a bundled PHP branch and its digest-pinned Bake base.
func PHPRecipe(version string) (string, string, error) {
	match := phpVersionPattern.FindStringSubmatch(strings.TrimSpace(version))
	if match == nil {
		return "", "", fmt.Errorf("no bundled PHP image for %q; supported branches are 8.3, 8.4, and 8.5", version)
	}
	branch, base := match[1], ""
	for _, entry := range bakePHPBasePattern.FindAllStringSubmatch(bakeDefinition, -1) {
		if entry[1] != branch {
			continue
		}
		if base != "" && base != entry[2] {
			return "", "", fmt.Errorf("conflicting bundled PHP %s base images", branch)
		}
		base = entry[2]
	}
	if base == "" {
		return "", "", fmt.Errorf("bundled PHP %s base image is missing", branch)
	}
	return branch, base, nil
}

func recipeContents(name string, contents []byte, branch, base, composer string) ([]byte, error) {
	if name != "images/php-nginx/Dockerfile" {
		return contents, nil
	}
	text := string(contents)
	if len(recipePHPBaseLine.FindAllStringIndex(text, -1)) != 1 || len(recipePHPBranchLine.FindAllStringIndex(text, -1)) != 2 {
		return nil, errors.New("bundled PHP recipe has unexpected base or branch declarations")
	}
	if composer != "" {
		if err := buildrunner.ValidatePinnedRepositoryDigest("Composer image", composer); err != nil {
			return nil, err
		}
		if len(recipeComposerBaseLine.FindAllStringIndex(text, -1)) != 1 {
			return nil, errors.New("bundled PHP recipe has an unexpected Composer base declaration")
		}
		text = recipeComposerBaseLine.ReplaceAllLiteralString(text, "ARG COMPOSER_BASE="+composer)
	}
	text = recipePHPBaseLine.ReplaceAllString(text, "ARG PHP_BASE="+base)
	text = recipePHPBranchLine.ReplaceAllString(text, "ARG PHP_BRANCH="+branch)
	return []byte(text), nil
}

// WriteContext copies the bundled recipe alongside a private prepared rootfs.
// Existing recipe paths are refused so the fingerprint always names our bytes.
func WriteContext(destination string) error {
	return WriteContextForPHP(destination, "8.5")
}

// WriteContextForPHP writes the recipe with the selected digest-pinned PHP base.
func WriteContextForPHP(destination, version string) error {
	return WriteContextForPHPAndComposer(destination, version, "")
}

func WriteContextForPHPAndComposer(destination, version, composer string) error {
	branch, base, err := PHPRecipe(version)
	if err != nil {
		return err
	}
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
	if err := os.CopyFS(destination, embedded); err != nil {
		return err
	}
	path := filepath.Join(destination, "images", "php-nginx", "Dockerfile")
	contents, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	contents, err = recipeContents("images/php-nginx/Dockerfile", contents, branch, base, composer)
	if err != nil {
		return err
	}
	return os.WriteFile(path, contents, 0o644)
}

// MaterializePrivateContext writes the embedded Magento image build context to a
// private directory under tempRoot and returns a content fingerprint.
func MaterializePrivateContext(tempRoot, platform string) (contextDir string, fingerprint string, cleanup func(), err error) {
	return MaterializePrivateContextForPHP(tempRoot, platform, "8.5")
}

func MaterializePrivateContextForPHP(tempRoot, platform, version string) (contextDir string, fingerprint string, cleanup func(), err error) {
	return MaterializePrivateContextForPHPAndComposer(tempRoot, platform, version, "")
}

func MaterializePrivateContextForPHPAndComposer(tempRoot, platform, version, composer string) (contextDir string, fingerprint string, cleanup func(), err error) {
	if err := validatePlatform(platform); err != nil {
		return "", "", nil, err
	}
	fingerprint, err = ContentFingerprintForPHPAndComposer(platform, version, composer)
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
	if err := WriteContextForPHPAndComposer(contextDir, version, composer); err != nil {
		cleanup()
		return "", "", nil, err
	}
	return contextDir, fingerprint, cleanup, nil
}

// ContentFingerprint returns a deterministic Docker-tag-safe fingerprint for the
// embedded build context and target platform.
func ContentFingerprint(platform string) (string, error) {
	return ContentFingerprintForPHP(platform, "8.5")
}

// ContentFingerprintForPHP hashes the exact selected recipe and target platform.
func ContentFingerprintForPHP(platform, version string) (string, error) {
	return ContentFingerprintForPHPAndComposer(platform, version, "")
}

func ContentFingerprintForPHPAndComposer(platform, version, composer string) (string, error) {
	if err := validatePlatform(platform); err != nil {
		return "", err
	}
	branch, base, err := PHPRecipe(version)
	if err != nil {
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
		contents, err = recipeContents(name, contents, branch, base, composer)
		if err != nil {
			return "", err
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
