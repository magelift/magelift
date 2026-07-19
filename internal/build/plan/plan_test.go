package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	buildrunner "github.com/acourtiol/magelift/internal/build/runner"
	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/source"
)

func TestPrepareRequestContainsOnlyImmutableInputs(t *testing.T) {
	root := t.TempDir()
	lock := []byte(`{"packages":[]}`)
	if err := os.WriteFile(filepath.Join(root, "composer.lock"), lock, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := config.Load([]byte(buildConfig))
	if err != nil {
		t.Fatal(err)
	}

	request, err := PrepareRequest(file, source.Repository{Root: root, Revision: strings.Repeat("a", 40)}, "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	if request.Prepare.RepositoryRoot != "/workspace" || len(request.Prepare.InputFiles) != 1 {
		t.Fatalf("unexpected request: %#v", request)
	}
	sum := sha256.Sum256(lock)
	if request.Prepare.InputFiles[0].SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("composer.lock checksum mismatch")
	}
	if len(request.Prepare.StaticContent) != 4 || request.Prepare.StaticContent[0].Locale != "en_US" {
		t.Fatalf("unexpected static content: %#v", request.Prepare.StaticContent)
	}
	encoded, err := buildrunner.EncodeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(encoded)), "account") || strings.Contains(strings.ToLower(string(encoded)), "credential") {
		t.Fatal("request contains environment or credential data")
	}
}

func TestPrepareRequestRequiresComposerLock(t *testing.T) {
	file, err := config.Load([]byte(buildConfig))
	if err != nil {
		t.Fatal(err)
	}
	_, err = PrepareRequest(file, source.Repository{Root: t.TempDir(), Revision: strings.Repeat("a", 40)}, "/workspace")
	if err == nil || !strings.Contains(err.Error(), "composer.lock") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareRequestCarriesSortedLifecycleHooks(t *testing.T) {
	input := strings.Replace(buildConfig, "  staticContent:", "  hooks:\n    zeta:\n      phase: build\n      relationship: after\n      target: build\n      command: {executable: composer, arguments: [run-script, zeta]}\n    alpha:\n      phase: validate\n      relationship: before\n      target: validate\n      command: {executable: magento, arguments: [cache:flush]}\n  staticContent:", 1)
	file, err := config.Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "composer.lock"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	request, err := PrepareRequest(file, source.Repository{Root: root, Revision: strings.Repeat("a", 40)}, "/workspace")
	if err != nil {
		t.Fatal(err)
	}
	if got := request.Prepare.LifecycleHooks[0].ID; got != "alpha" {
		t.Fatalf("hooks are not sorted: %#v", request.Prepare.LifecycleHooks)
	}
}

const buildConfig = `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.9, mode: integrated}
build:
  php: "8.5"
  staticContent:
    locales: [fr_FR, en_US]
    themes: [Magento/luma, Magento/blank]
target: {provider: aws, runtime: ecs-fargate}
defaults: {region: eu-west-3, preset: preview}
environments:
  staging: {account: "123456789012", domain: staging.example.com}
`
