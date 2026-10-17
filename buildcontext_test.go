package buildcontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContentFingerprintIsStableAndPlatformBound(t *testing.T) {
	first, err := ContentFingerprint(alphaPlatform)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ContentFingerprint(alphaPlatform)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || len(first) != 64 {
		t.Fatalf("unexpected fingerprint: %q", first)
	}
	other, err := ContentFingerprint("linux/arm64")
	if err == nil || other == first {
		t.Fatalf("platform binding missing: err=%v other=%q", err, other)
	}
}

func TestMaterializePrivateContextWritesExpectedLayout(t *testing.T) {
	dir, fingerprint, cleanup, err := MaterializePrivateContext(t.TempDir(), alphaPlatform)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	for _, relative := range []string{
		"LICENSE",
		"NOTICE",
		"images/php-nginx/Dockerfile",
		"build/composer.json",
		"build/bin/magelift-build-runner",
		"build/src/Artifact/ArtifactManifest.php",
	} {
		if _, err := os.Stat(filepath.Join(dir, relative)); err != nil {
			t.Fatalf("missing %q: %v", relative, err)
		}
	}
	if _, err := ContentFingerprint(alphaPlatform); err != nil {
		t.Fatal(err)
	}
	if fingerprint == "" {
		t.Fatal("missing fingerprint")
	}
}

func TestEmbeddedPathsExcludeVendorAuthAndGit(t *testing.T) {
	paths, err := EmbeddedPaths()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		normalized := "/" + strings.ReplaceAll(filepath.ToSlash(path), "\\", "/") + "/"
		for _, forbidden := range []string{"/vendor/", "/tests/", "/auth/", "/.git/"} {
			if strings.Contains(normalized, forbidden) {
				t.Fatalf("forbidden embedded path %q", path)
			}
		}
		if strings.Contains(strings.ToLower(path), "ghcr.io/magelift") {
			t.Fatalf("embedded path references MageLift GHCR: %q", path)
		}
	}
}

func TestMaterializePrivateContextRejectsUnsupportedPlatform(t *testing.T) {
	_, _, cleanup, err := MaterializePrivateContext(t.TempDir(), "linux/arm64")
	if cleanup != nil {
		cleanup()
	}
	if err == nil || !strings.Contains(err.Error(), alphaPlatform) {
		t.Fatalf("unexpected error: %v", err)
	}
}
